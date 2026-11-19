package unmapped

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/beacon-stack/pilot/internal/config"
	"github.com/beacon-stack/pilot/internal/core/show"
	dbpkg "github.com/beacon-stack/pilot/internal/db"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/metadata/tmdbtv"
)

func TestFolderTitle(t *testing.T) {
	for in, want := range map[string]struct {
		title string
		year  int
	}{
		"Breaking Bad (2008)":                {"Breaking Bad", 2008},
		"Breaking.Bad.2008":                  {"Breaking Bad", 2008},
		"The.Office.US.S01-S09.1080p.BluRay": {"The Office US", 0},
		"[TGx] Severance":                    {"Severance", 0},
		"1923":                               {"1923", 0},
		"x":                                  {"", 0},
	} {
		title, year := FolderTitle(in)
		if title != want.title || year != want.year {
			t.Errorf("FolderTitle(%q) = %q, %d; want %q, %d", in, title, year, want.title, want.year)
		}
	}
}

type fakeShows struct {
	sqlDB   *sql.DB
	lookups []show.LookupRequest
	adds    []show.AddRequest
}

func (f *fakeShows) Lookup(_ context.Context, req show.LookupRequest) ([]tmdbtv.SearchResult, error) {
	f.lookups = append(f.lookups, req)
	if req.Query == "Unknowable" {
		return nil, nil
	}
	return []tmdbtv.SearchResult{{ID: 100 + len(f.lookups), Title: req.Query + " (TMDB)", Year: req.Year, PosterPath: "/p.jpg"}}, nil
}

func (f *fakeShows) Add(_ context.Context, req show.AddRequest) (show.Series, error) {
	f.adds = append(f.adds, req)
	id := "added-" + time.Now().Format("150405.000000000")
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := f.sqlDB.Exec(`INSERT INTO series (id, tmdb_id, title, sort_title, year, library_id, quality_profile_id, added_at, updated_at)
		VALUES (?, ?, 'Added', 'added', 2020, ?, ?, ?, ?)`, id, req.TMDBID, req.LibraryID, req.QualityProfileID, now, now); err != nil {
		return show.Series{}, err
	}
	return show.Series{ID: id, Title: "Added"}, nil
}

type fixture struct {
	svc   *Service
	shows *fakeShows
	q     db.Querier
	sqlDB *sql.DB
	root  string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	d, err := dbpkg.Open(config.DatabaseConfig{Path: filepath.Join(t.TempDir(), "pilot.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := dbpkg.Migrate(d.SQL); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	now := time.Now().UTC().Format(time.RFC3339)
	for _, q := range []string{
		`INSERT INTO quality_profiles (id, name, created_at, updated_at) VALUES ('qp', 'Any', '` + now + `', '` + now + `')`,
		`INSERT INTO libraries (id, name, root_path, default_quality_profile_id, created_at, updated_at) VALUES ('lib', 'TV', '` + root + `', 'qp', '` + now + `', '` + now + `')`,
		`INSERT INTO series (id, tmdb_id, title, sort_title, year, library_id, quality_profile_id, added_at, updated_at) VALUES ('known', 1, 'Known Show', 'known show', 2020, 'lib', 'qp', '` + now + `', '` + now + `')`,
		`INSERT INTO series (id, tmdb_id, title, sort_title, year, library_id, quality_profile_id, path, added_at, updated_at) VALUES ('pointed', 2, 'Real Title', 'real title', 2020, 'lib', 'qp', '` + filepath.Join(root, "Odd Folder Name") + `', '` + now + `', '` + now + `')`,
	} {
		if _, err := d.SQL.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for _, dir := range []string{"Breaking Bad (2008)", "Known Show", "Odd Folder Name", "Unknowable", ".hidden"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	shows := &fakeShows{sqlDB: d.SQL}
	return &fixture{svc: New(db.New(d.SQL), shows), shows: shows, q: db.New(d.SQL), sqlDB: d.SQL, root: root}
}

func names(fs []Folder) map[string]Folder {
	m := map[string]Folder{}
	for _, f := range fs {
		m[f.Name] = f
	}
	return m
}

func TestDiscover_RecordsWithoutAdding(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.svc.Discover(ctx, "lib", f.root); err != nil {
		t.Fatal(err)
	}
	if len(f.shows.adds) != 0 {
		t.Fatalf("discover added series: %+v", f.shows.adds)
	}
	got, _ := f.svc.List(ctx, false)
	m := names(got)
	if len(m) != 2 {
		t.Fatalf("recorded %v, want Breaking Bad and Unknowable (Known Show and the pointed folder are claimed, .hidden skipped)", m)
	}
	bb := m["Breaking Bad (2008)"]
	if bb.GuessTMDBID == 0 || bb.GuessTitle != "Breaking Bad (TMDB)" || bb.GuessYear != 2008 {
		t.Fatalf("guess = %+v", bb)
	}
	if f.shows.lookups[0].Year != 2008 {
		t.Errorf("lookup year = %d, want 2008", f.shows.lookups[0].Year)
	}
	if m["Unknowable"].GuessTMDBID != 0 {
		t.Error("a folder with no TMDB hit should have no guess")
	}

	// A second scan looks nothing up again; a folder removed from disk
	// drops out.
	n := len(f.shows.lookups)
	_ = os.Remove(filepath.Join(f.root, "Unknowable"))
	if err := f.svc.Discover(ctx, "lib", f.root); err != nil {
		t.Fatal(err)
	}
	if len(f.shows.lookups) != n {
		t.Fatal("recorded folders were looked up again")
	}
	if got, _ = f.svc.List(ctx, false); len(got) != 1 {
		t.Fatalf("after removal = %+v", got)
	}
}

func TestAdd_UsesGuessAndPointsSeriesAtFolder(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_ = f.svc.Discover(ctx, "lib", f.root)
	bb := names(mustList(t, f))["Breaking Bad (2008)"]

	sr, err := f.svc.Add(ctx, bb.ID, AddRequest{})
	if err != nil {
		t.Fatal(err)
	}
	add := f.shows.adds[0]
	if add.TMDBID != bb.GuessTMDBID || add.LibraryID != "lib" || add.QualityProfileID != "qp" || add.MonitorType != "existing" {
		t.Fatalf("add request = %+v", add)
	}
	row, _ := f.q.GetSeries(ctx, sr.ID)
	if row.Path == nil || *row.Path != bb.Path {
		t.Fatalf("series path = %v, want %s", row.Path, bb.Path)
	}
	if _, ok := names(mustList(t, f))["Breaking Bad (2008)"]; ok {
		t.Fatal("the folder is still listed after adding")
	}

	// Picking another show.
	unk := names(mustList(t, f))["Unknowable"]
	if _, err := f.svc.Add(ctx, unk.ID, AddRequest{}); err == nil {
		t.Fatal("added a folder with no guess and no show chosen")
	}
	if _, err := f.svc.Add(ctx, unk.ID, AddRequest{TMDBID: 777, MonitorType: "all"}); err != nil {
		t.Fatal(err)
	}
	if a := f.shows.adds[len(f.shows.adds)-1]; a.TMDBID != 777 || a.MonitorType != "all" {
		t.Fatalf("add request = %+v", a)
	}
	if _, err := f.svc.Add(ctx, "nope", AddRequest{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown id: %v", err)
	}
}

func TestIgnore(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_ = f.svc.Discover(ctx, "lib", f.root)
	bb := names(mustList(t, f))["Breaking Bad (2008)"]
	if err := f.svc.SetIgnored(ctx, bb.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, ok := names(mustList(t, f))["Breaking Bad (2008)"]; ok {
		t.Fatal("ignored folder listed")
	}
	all, _ := f.svc.List(ctx, true)
	if !names(all)["Breaking Bad (2008)"].Ignored {
		t.Fatal("ignored folder missing from the full list")
	}
	// Ignored folders stay ignored across scans.
	_ = f.svc.Discover(ctx, "lib", f.root)
	if _, ok := names(mustList(t, f))["Breaking Bad (2008)"]; ok {
		t.Fatal("a scan un-ignored the folder")
	}
	if err := f.svc.SetIgnored(ctx, "nope", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown id: %v", err)
	}
}

func mustList(t *testing.T, f *fixture) []Folder {
	t.Helper()
	fs, err := f.svc.List(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	return fs
}
