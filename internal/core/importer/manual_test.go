package importer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func candidateByName(t *testing.T, cs []Candidate, name string) Candidate {
	t.Helper()
	for _, c := range cs {
		if filepath.Base(c.Path) == name {
			return c
		}
	}
	t.Fatalf("no candidate %s in %+v", name, cs)
	return Candidate{}
}

func TestScanManual_MatchesSeriesEpisodesAndQuality(t *testing.T) {
	f := newRetryFixture(t)
	cs, err := f.svc.ScanManual(context.Background(), f.packDir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 {
		t.Fatalf("%d candidates, want 2: %+v", len(cs), cs)
	}
	c := candidateByName(t, cs, "Show.S01E02.1080p.WEB.mkv")
	if c.SeriesID != "series-1" || c.Season != 1 || len(c.Episodes) != 1 || c.Episodes[0].ID != "ep-2" {
		t.Fatalf("candidate = %+v", c)
	}
	if c.Quality.Resolution != "1080p" || c.Quality.Name == "" || len(c.Rejections) != 0 {
		t.Fatalf("quality/rejections = %+v / %v", c.Quality, c.Rejections)
	}
}

func TestScanManual_Rejections(t *testing.T) {
	f := newRetryFixture(t)
	dir := filepath.Join(t.TempDir(), "incoming")
	for _, n := range []string{
		"Other.Show.S01E01.1080p.WEB.mkv",
		"Show.S01E01.1080p.WEB.sample.mkv",
		"Show.S05E09.1080p.WEB.mkv",
		"notes.txt",
	} {
		writeSource(t, dir, n, "x")
	}
	cs, err := f.svc.ScanManual(context.Background(), dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 3 {
		t.Fatalf("%d candidates, want the 3 videos: %+v", len(cs), cs)
	}
	want := map[string]string{
		"Other.Show.S01E01.1080p.WEB.mkv":  RejectUnknownSeries,
		"Show.S01E01.1080p.WEB.sample.mkv": RejectSample,
		"Show.S05E09.1080p.WEB.mkv":        RejectUnknownEpisode,
	}
	for name, reason := range want {
		c := candidateByName(t, cs, name)
		found := false
		for _, r := range c.Rejections {
			found = found || r == reason
		}
		if !found {
			t.Errorf("%s rejections = %v, want %s", name, c.Rejections, reason)
		}
	}

	// A forced series overrides the name.
	cs, _ = f.svc.ScanManual(context.Background(), filepath.Join(dir, "Other.Show.S01E01.1080p.WEB.mkv"), "series-1")
	if len(cs) != 1 || cs[0].SeriesID != "series-1" || len(cs[0].Episodes) != 1 {
		t.Fatalf("forced series = %+v", cs)
	}
}

func TestScanManual_InvalidPath(t *testing.T) {
	f := newRetryFixture(t)
	for _, p := range []string{"", "relative/dir", filepath.Join(t.TempDir(), "missing")} {
		if _, err := f.svc.ScanManual(context.Background(), p, ""); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("ScanManual(%q) err = %v", p, err)
		}
	}
}

func TestImportManual_CopyThenRescan(t *testing.T) {
	f := newRetryFixture(t)
	ctx := context.Background()
	src := filepath.Join(f.packDir, "Show.S01E01.1080p.WEB.mkv")

	res := f.svc.ImportManual(ctx, []ManualItem{{Path: src, SeriesID: "series-1", EpisodeIDs: []string{"ep-1"}}}, false)
	if len(res) != 1 || res[0].Error != "" {
		t.Fatalf("results = %+v", res)
	}
	files := f.filesFor("ep-1")
	if len(files) != 1 || filepath.Dir(filepath.Dir(files[0])) != filepath.Join(f.tvRoot, "Show (2022)") {
		t.Fatalf("library files = %v", files)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatal("copy mode removed the source")
	}

	// Scanning again: the library file is already imported (and matched
	// to its series through Pilot's own naming), and the source is no
	// upgrade over it.
	cs, _ := f.svc.ScanManual(ctx, files[0], "")
	if len(cs) != 1 || cs[0].SeriesID != "series-1" || !reflect.DeepEqual(cs[0].Rejections, []string{RejectAlreadyImported}) {
		t.Fatalf("rescan of library file = %+v", cs)
	}
	cs, _ = f.svc.ScanManual(ctx, src, "")
	if len(cs) != 1 || !reflect.DeepEqual(cs[0].Rejections, []string{RejectNotAnUpgrade}) {
		t.Fatalf("rescan of source = %+v", cs)
	}
}

func TestImportManual_MoveRemovesSource(t *testing.T) {
	f := newRetryFixture(t)
	src := filepath.Join(f.packDir, "Show.S01E02.1080p.WEB.mkv")
	res := f.svc.ImportManual(context.Background(), []ManualItem{{Path: src, SeriesID: "series-1", EpisodeIDs: []string{"ep-2"}}}, true)
	if res[0].Error != "" {
		t.Fatal(res[0].Error)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("move mode kept the source")
	}
	files := f.filesFor("ep-2")
	if len(files) != 1 {
		t.Fatalf("files = %v", files)
	}
	if b, err := os.ReadFile(files[0]); err != nil || len(b) == 0 {
		t.Fatalf("library file unreadable after move: %v", err)
	}
}

func TestImportManual_RejectsBadItems(t *testing.T) {
	f := newRetryFixture(t)
	f.exec(`INSERT INTO series (id, tmdb_id, title, sort_title, year, library_id, quality_profile_id, added_at, updated_at)
	        VALUES ('series-2', 2, 'Other', 'other', 2020, 'lib', 'qp', ?, ?)`, fixtureNow, fixtureNow)
	src := filepath.Join(f.packDir, "Show.S01E01.1080p.WEB.mkv")
	res := f.svc.ImportManual(context.Background(), []ManualItem{
		{Path: "relative.mkv", SeriesID: "series-1", EpisodeIDs: []string{"ep-1"}},
		{Path: src, SeriesID: "series-2", EpisodeIDs: []string{"ep-1"}}, // ep-1 belongs to series-1
		{Path: src, SeriesID: "series-1"},                               // no episodes
	}, true)
	for i, r := range res {
		if r.Error == "" {
			t.Errorf("item %d imported: %+v", i, r)
		}
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatal("a rejected move removed the source")
	}
	if len(f.filesFor("ep-1")) != 0 {
		t.Fatal("a rejected item was imported")
	}
}

// A file named the way Pilot names files ("Show - S01E02 - Title …"),
// in a folder that says nothing about the show, is matched by the name
// before the episode marker. The parser alone reads that title as
// "Show - - Title".
func TestScanManual_MatchesPilotStyleNameInFlatFolder(t *testing.T) {
	f := newRetryFixture(t)
	dir := filepath.Join(t.TempDir(), "misc")
	writeSource(t, dir, "Show - S01E02 - Episode WEBDL-1080p.mkv", "x")
	cs, err := f.svc.ScanManual(context.Background(), dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 1 || cs[0].SeriesID != "series-1" || len(cs[0].Episodes) != 1 || cs[0].Episodes[0].ID != "ep-2" {
		t.Fatalf("candidate = %+v", cs)
	}
}
