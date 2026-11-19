package health

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/beacon-stack/pilot/internal/config"
	"github.com/beacon-stack/pilot/internal/core/downloader"
	"github.com/beacon-stack/pilot/internal/core/indexer"
	dbpkg "github.com/beacon-stack/pilot/internal/db"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/diskspace"
)

type fakeLister[T any] struct {
	cfgs  []T
	fail  map[string]error
	tests atomic.Int32
}

func (f *fakeLister[T]) List(context.Context) ([]T, error) { return f.cfgs, nil }
func (f *fakeLister[T]) Test(_ context.Context, id string) error {
	f.tests.Add(1)
	return f.fail[id]
}

func newDB(t *testing.T) (*sql.DB, db.Querier) {
	t.Helper()
	d, err := dbpkg.Open(config.DatabaseConfig{Path: filepath.Join(t.TempDir(), "pilot.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := dbpkg.Migrate(d.SQL); err != nil {
		t.Fatal(err)
	}
	return d.SQL, db.New(d.SQL)
}

func addLibrary(t *testing.T, sqlDB *sql.DB, id, name, root string, minGB int) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := sqlDB.Exec(`INSERT OR IGNORE INTO quality_profiles (id, name, created_at, updated_at) VALUES ('qp', 'Any', ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO libraries (id, name, root_path, default_quality_profile_id, min_free_space_gb, created_at, updated_at)
		VALUES (?, ?, ?, 'qp', ?, ?, ?)`, id, name, root, minGB, now, now); err != nil {
		t.Fatal(err)
	}
}

func messages(r Report) string {
	var b strings.Builder
	for _, i := range r.Issues {
		b.WriteString(string(i.Level) + " " + i.Source + ": " + i.Message + "\n")
	}
	return b.String()
}

func healthyServices() (*fakeLister[downloader.Config], *fakeLister[indexer.Config]) {
	return &fakeLister[downloader.Config]{cfgs: []downloader.Config{{ID: "c1", Name: "qBit", Enabled: true}}},
		&fakeLister[indexer.Config]{cfgs: []indexer.Config{{ID: "i1", Name: "Idx", Enabled: true}}}
}

func TestReport_Healthy(t *testing.T) {
	sqlDB, q := newDB(t)
	addLibrary(t, sqlDB, "lib", "TV", t.TempDir(), 5)
	c, i := healthyServices()
	s := NewService(q, c, i)
	s.free = func(string) (uint64, error) { return 100 * diskspace.GB, nil }
	if r := s.Report(context.Background()); len(r.Issues) != 0 || r.ConnectivityCheckedAt.IsZero() {
		t.Fatalf("healthy setup reported:\n%s(checked at %v)", messages(r), r.ConnectivityCheckedAt)
	}
}

func TestReport_LibraryProblems(t *testing.T) {
	sqlDB, q := newDB(t)
	file := filepath.Join(t.TempDir(), "file")
	_ = os.WriteFile(file, nil, 0o644)
	addLibrary(t, sqlDB, "a", "Missing", filepath.Join(t.TempDir(), "gone"), 5)
	addLibrary(t, sqlDB, "b", "NotDir", file, 5)
	addLibrary(t, sqlDB, "c", "Full", t.TempDir(), 5)
	addLibrary(t, sqlDB, "d", "NoMinimum", t.TempDir(), 0)
	c, i := healthyServices()
	s := NewService(q, c, i)
	s.free = func(string) (uint64, error) { return 2 * diskspace.GB, nil }

	got := messages(s.Report(context.Background()))
	for _, want := range []string{
		`error library: Library "Missing": root folder`,
		`error library: Library "NotDir"`,
		`warning library: Library "Full" has 2.0 GB free, below its minimum of 5 GB`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "NoMinimum") {
		t.Errorf("a library with no minimum was flagged:\n%s", got)
	}
}

func TestReport_NoLibrary(t *testing.T) {
	_, q := newDB(t)
	c, i := healthyServices()
	got := messages(NewService(q, c, i).Report(context.Background()))
	if !strings.Contains(got, "No library is set up") {
		t.Fatalf("got:\n%s", got)
	}
}

func TestConnectivity(t *testing.T) {
	sqlDB, q := newDB(t)
	addLibrary(t, sqlDB, "lib", "TV", t.TempDir(), 0)
	clients := &fakeLister[downloader.Config]{
		cfgs: []downloader.Config{{ID: "c1", Name: "qBit", Enabled: true}, {ID: "c2", Name: "SAB", Enabled: true}, {ID: "c3", Name: "Off", Enabled: false}},
		fail: map[string]error{"c2": errors.New("connection refused"), "c3": errors.New("never tested")},
	}
	indexers := &fakeLister[indexer.Config]{
		cfgs: []indexer.Config{{ID: "i1", Name: "Only", Enabled: true}},
		fail: map[string]error{"i1": errors.New("401")},
	}
	s := NewService(q, clients, indexers)
	got := messages(s.Report(context.Background()))
	for _, want := range []string{
		// One of two clients down: a warning.
		`warning download_client: Can't reach download client "SAB": connection refused`,
		// The only indexer down: an error.
		`error indexer: Can't reach indexer "Only": 401`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Off") {
		t.Errorf("a disabled client was tested:\n%s", got)
	}
	// Errors sort first.
	if r := s.Report(context.Background()); r.Issues[0].Level != LevelError {
		t.Errorf("first issue is %s, want error:\n%s", r.Issues[0].Level, got)
	}
	// Report reuses the stored result instead of re-testing.
	n := clients.tests.Load()
	s.Report(context.Background())
	if clients.tests.Load() != n {
		t.Error("Report re-ran the connectivity tests")
	}
}

func TestConnectivity_NoneEnabled(t *testing.T) {
	_, q := newDB(t)
	s := NewService(q, &fakeLister[downloader.Config]{}, &fakeLister[indexer.Config]{cfgs: []indexer.Config{{ID: "i", Enabled: false}}})
	got := messages(s.Report(context.Background()))
	for _, want := range []string{"No download client is enabled", "No indexer is enabled"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
