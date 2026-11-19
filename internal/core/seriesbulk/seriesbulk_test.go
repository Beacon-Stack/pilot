package seriesbulk

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/beacon-stack/pilot/internal/config"
	dbpkg "github.com/beacon-stack/pilot/internal/db"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/events"
)

type fixture struct {
	svc   *Service
	sqlDB *sql.DB
	q     db.Querier
	root  string
}

func newFixture(t *testing.T, rec Recycler, bus Publisher) *fixture {
	t.Helper()
	d, err := dbpkg.Open(config.DatabaseConfig{Path: filepath.Join(t.TempDir(), "pilot.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := dbpkg.Migrate(d.SQL); err != nil {
		t.Fatal(err)
	}
	f := &fixture{sqlDB: d.SQL, q: db.New(d.SQL), root: filepath.Join(t.TempDir(), "tv")}
	now := time.Now().UTC().Format(time.RFC3339)
	f.exec(t, `INSERT INTO quality_profiles (id, name, created_at, updated_at) VALUES ('qp1', 'HD', ?, ?), ('qp2', 'UHD', ?, ?)`, now, now, now, now)
	f.exec(t, `INSERT INTO libraries (id, name, root_path, default_quality_profile_id, created_at, updated_at) VALUES ('lib1', 'TV', ?, 'qp1', ?, ?), ('lib2', 'Kids', '/kids', 'qp1', ?, ?)`, f.root, now, now, now, now)
	for i, id := range []string{"a", "b", "c"} {
		f.exec(t, `INSERT INTO series (id, tmdb_id, title, sort_title, year, library_id, quality_profile_id, monitored, series_type, added_at, updated_at)
			VALUES (?, ?, ?, ?, 2020, 'lib1', 'qp1', TRUE, 'standard', ?, ?)`, id, i+1, "Show "+id, id, now, now)
	}
	f.svc = New(d.SQL, rec, bus, nil)
	return f
}

func (f *fixture) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := f.sqlDB.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func (f *fixture) series(t *testing.T, id string) db.Series {
	t.Helper()
	s, err := f.q.GetSeries(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func ptr[T any](v T) *T { return &v }

func TestEdit_OnlyGivenFieldsChange(t *testing.T) {
	f := newFixture(t, nil, nil)
	n, err := f.svc.Edit(context.Background(), Edit{IDs: []string{"a", "b"}, Monitored: ptr(false), QualityProfileID: ptr("qp2")})
	if err != nil || n != 2 {
		t.Fatalf("Edit = %d, %v", n, err)
	}
	for _, id := range []string{"a", "b"} {
		s := f.series(t, id)
		if s.Monitored || s.QualityProfileID != "qp2" || s.LibraryID != "lib1" || s.SeriesType != "standard" || s.Title != "Show "+id {
			t.Errorf("series %s = %+v", id, s)
		}
	}
	if c := f.series(t, "c"); !c.Monitored || c.QualityProfileID != "qp1" {
		t.Errorf("an unselected series changed: %+v", c)
	}
}

func TestEdit_AllOrNothing(t *testing.T) {
	f := newFixture(t, nil, nil)
	ctx := context.Background()
	for name, e := range map[string]Edit{
		"unknown series":  {IDs: []string{"a", "ghost"}, Monitored: ptr(false)},
		"unknown profile": {IDs: []string{"a"}, QualityProfileID: ptr("nope")},
		"unknown library": {IDs: []string{"a"}, LibraryID: ptr("nope")},
		"bad type":        {IDs: []string{"a"}, SeriesType: ptr("movie")},
		"duplicate id":    {IDs: []string{"a", "a"}, Monitored: ptr(false)},
		"empty":           {Monitored: ptr(false)},
		"nothing to do":   {IDs: []string{"a"}},
	} {
		var verr *ValidationError
		if _, err := f.svc.Edit(ctx, e); !errors.As(err, &verr) {
			t.Errorf("%s: err = %v, want ValidationError", name, err)
		}
	}
	if a := f.series(t, "a"); !a.Monitored {
		t.Fatal("series a changed although the request was rejected")
	}
}

type fakeRecycler struct {
	mu    sync.Mutex
	paths []string
	roots []string
}

func (r *fakeRecycler) Delete(_ context.Context, path, root string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paths = append(r.paths, path)
	r.roots = append(r.roots, root)
	return "", os.Remove(path)
}

type capture struct {
	mu  sync.Mutex
	evs []events.Event
}

func (c *capture) Publish(_ context.Context, e events.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evs = append(c.evs, e)
}

func TestDelete_WithFilesUsesRecycler(t *testing.T) {
	rec := &fakeRecycler{}
	bus := &capture{}
	f := newFixture(t, rec, bus)
	ctx := context.Background()
	file := filepath.Join(f.root, "Show a", "Season 01", "ep.mkv")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(file, []byte("x"), 0o644)
	f.exec(t, `INSERT INTO seasons (id, series_id, season_number) VALUES ('s1', 'a', 1)`)
	f.exec(t, `INSERT INTO episodes (id, series_id, season_id, season_number, episode_number, title) VALUES ('e1', 'a', 's1', 1, 1, 'Pilot')`)
	f.exec(t, `INSERT INTO episode_files (id, episode_id, series_id, path, size_bytes, quality_json, imported_at, indexed_at) VALUES ('f1', 'e1', 'a', ?, 1, '{}', 'x', 'x')`, file)

	n, err := f.svc.Delete(ctx, []string{"a", "b"}, true)
	if err != nil || n != 2 {
		t.Fatalf("Delete = %d, %v", n, err)
	}
	for _, id := range []string{"a", "b"} {
		if _, err := f.q.GetSeries(ctx, id); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("series %s still exists", id)
		}
	}
	if len(rec.paths) != 1 || rec.paths[0] != file || rec.roots[0] != f.root {
		t.Fatalf("recycler got %v / %v", rec.paths, rec.roots)
	}
	if len(bus.evs) != 2 || bus.evs[0].Type != events.TypeShowDeleted {
		t.Fatalf("events = %+v", bus.evs)
	}
	if _, err := f.q.GetSeries(ctx, "c"); err != nil {
		t.Fatal("an unselected series was deleted")
	}
}

func TestDelete_UnknownSeriesDeletesNothing(t *testing.T) {
	f := newFixture(t, &fakeRecycler{}, nil)
	var verr *ValidationError
	if _, err := f.svc.Delete(context.Background(), []string{"a", "ghost"}, false); !errors.As(err, &verr) {
		t.Fatalf("err = %v, want ValidationError", err)
	}
	if _, err := f.q.GetSeries(context.Background(), "a"); err != nil {
		t.Fatal("series a was deleted although the request was rejected")
	}
}

func TestDelete_KeepFilesLeavesDisk(t *testing.T) {
	rec := &fakeRecycler{}
	f := newFixture(t, rec, nil)
	if _, err := f.svc.Delete(context.Background(), []string{"a"}, false); err != nil {
		t.Fatal(err)
	}
	if len(rec.paths) != 0 {
		t.Fatalf("files removed without delete_files: %v", rec.paths)
	}
}
