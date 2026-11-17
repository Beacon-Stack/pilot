package importer

// Regression tests for upgrades. Before the fix:
//   - an upgrade whose filename changed (the default format includes
//     {Quality Full}) left the old file and its row behind, so the
//     episode had two files;
//   - an upgrade whose filename did not change was treated as "already
//     imported", so the new file was never moved into the library.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

var (
	q720  = plugin.Quality{Resolution: plugin.Resolution("720p"), Source: plugin.Source("webdl"), Name: "WEBDL-720p"}
	q1080 = plugin.Quality{Resolution: plugin.Resolution("1080p"), Source: plugin.Source("bluray"), Name: "Bluray-1080p"}
)

// writeSource creates a download file with the given content and returns
// its path and size.
func writeSource(t *testing.T, dir, name, content string) (string, int64) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p, int64(len(content))
}

func (f *retryFixture) filesFor(episodeID string) []string {
	f.t.Helper()
	files, err := f.q.ListEpisodeFilesByEpisodeID(context.Background(), episodeID)
	if err != nil {
		f.t.Fatal(err)
	}
	out := make([]string, len(files))
	for i, ef := range files {
		out[i] = ef.Path
	}
	return out
}

func TestUpgrade_DifferentFilenameReplacesOldFile(t *testing.T) {
	f := newRetryFixture(t)
	ctx := context.Background()
	dl := filepath.Join(t.TempDir(), "dl")

	oldSrc, oldSize := writeSource(t, dl, "Show.S01E01.720p.WEB.mkv", "old 720p bytes")
	if err := f.svc.AttachFile(ctx, "ep-1", "series-1", oldSrc, oldSize, q720); err != nil {
		t.Fatal(err)
	}
	before := f.filesFor("ep-1")
	if len(before) != 1 {
		t.Fatalf("after first import: %v", before)
	}
	oldLibFile := before[0]

	newSrc, newSize := writeSource(t, dl, "Show.S01E01.1080p.BluRay.mkv", "new 1080p bytes, larger")
	if err := f.svc.AttachFile(ctx, "ep-1", "series-1", newSrc, newSize, q1080); err != nil {
		t.Fatal(err)
	}

	after := f.filesFor("ep-1")
	if len(after) != 1 {
		t.Fatalf("episode has %d files after upgrade, want 1: %v", len(after), after)
	}
	if after[0] == oldLibFile {
		t.Fatalf("upgrade kept the old path %s", oldLibFile)
	}
	if _, err := os.Stat(oldLibFile); !os.IsNotExist(err) {
		t.Fatalf("old library file still on disk: %s", oldLibFile)
	}
	if got, _ := os.ReadFile(after[0]); string(got) != "new 1080p bytes, larger" {
		t.Fatalf("new library file content = %q", got)
	}
	// The download itself is never touched (the client seeds it).
	if _, err := os.Stat(oldSrc); err != nil {
		t.Fatalf("source download was removed: %v", err)
	}
}

func TestUpgrade_SameFilenameReplacesContent(t *testing.T) {
	f := newRetryFixture(t)
	ctx := context.Background()
	// A naming format without a quality token: upgrades land on the same path.
	f.exec(`UPDATE libraries SET naming_format = ? WHERE id = 'lib'`, "{Series Title} - S{Season:00}E{Episode:00} - {Episode Title}")
	dl := filepath.Join(t.TempDir(), "dl")

	oldSrc, oldSize := writeSource(t, dl, "Show.S01E01.720p.WEB.mkv", "old")
	if err := f.svc.AttachFile(ctx, "ep-1", "series-1", oldSrc, oldSize, q720); err != nil {
		t.Fatal(err)
	}
	path := f.filesFor("ep-1")[0]

	newSrc, newSize := writeSource(t, dl, "Show.S01E01.1080p.BluRay.mkv", "new and bigger")
	if err := f.svc.AttachFile(ctx, "ep-1", "series-1", newSrc, newSize, q1080); err != nil {
		t.Fatal(err)
	}

	after := f.filesFor("ep-1")
	if len(after) != 1 || after[0] != path {
		t.Fatalf("files after same-name upgrade = %v, want [%s]", after, path)
	}
	if got, _ := os.ReadFile(path); string(got) != "new and bigger" {
		t.Fatalf("library file still has old content %q: the upgrade was skipped", got)
	}
	ef, err := f.q.GetEpisodeFileByPath(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if ef.SizeBytes != newSize || !strings.Contains(ef.QualityJson, "1080p") {
		t.Fatalf("record not updated: size=%d quality=%s", ef.SizeBytes, ef.QualityJson)
	}
}

// A recorded file outside the library root (e.g. still in a download
// folder) loses its record but is never deleted from disk.
func TestUpgrade_FileOutsideLibraryIsNotDeleted(t *testing.T) {
	f := newRetryFixture(t)
	ctx := context.Background()
	outside, _ := writeSource(t, filepath.Join(t.TempDir(), "elsewhere"), "Show.S01E01.mkv", "outside")
	f.exec(`INSERT INTO episode_files (id, episode_id, series_id, path, size_bytes, quality_json, imported_at, indexed_at)
	        VALUES ('ef-out', 'ep-1', 'series-1', ?, 7, '{}', ?, ?)`, outside, fixtureNow, fixtureNow)

	newSrc, newSize := writeSource(t, filepath.Join(t.TempDir(), "dl"), "Show.S01E01.1080p.BluRay.mkv", "new")
	if err := f.svc.AttachFile(ctx, "ep-1", "series-1", newSrc, newSize, q1080); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("file outside the library was deleted: %v", err)
	}
	after := f.filesFor("ep-1")
	if len(after) != 1 || after[0] == outside {
		t.Fatalf("files after import = %v, want only the new library file", after)
	}
}

func TestPathWithin(t *testing.T) {
	cases := []struct {
		root, p string
		want    bool
	}{
		{"/tv", "/tv/Show/S01E01.mkv", true},
		{"/tv/", "/tv/Show/S01E01.mkv", true},
		{"/tv", "/tv", false},
		{"/tv", "/tvshows/x.mkv", false},
		{"/tv", "/tv/../etc/passwd", false},
		{"/tv", "/downloads/x.mkv", false},
	}
	for _, c := range cases {
		if got := pathWithin(c.root, c.p); got != c.want {
			t.Errorf("pathWithin(%q, %q) = %v, want %v", c.root, c.p, got, c.want)
		}
	}
}

// Regression: qualityFromGrab left Name empty, so {Quality Full} rendered
// as nothing and every grab import produced a filename with no quality.
func TestQualityFromGrab_SetsName(t *testing.T) {
	q := qualityFromGrab(db.GrabHistory{
		ReleaseResolution: string(plugin.Resolution("1080p")),
		ReleaseSource:     string(plugin.SourceBluRay),
		ReleaseCodec:      "x265",
	})
	if q.Name == "" {
		t.Fatal("quality Name is empty; {Quality Full} would render as nothing")
	}
	if !strings.Contains(q.Name, "1080p") {
		t.Fatalf("Name = %q, want it to include the resolution", q.Name)
	}
}
