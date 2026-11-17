package backup

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/beacon-stack/pilot/internal/config"
	dbpkg "github.com/beacon-stack/pilot/internal/db"
	db "github.com/beacon-stack/pilot/internal/db/generated"
)

type fixture struct {
	svc   *Service
	sqlDB *sql.DB
	clock time.Time
}

func newFixture(t *testing.T, dir string) *fixture {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "pilot.db")
	d, err := dbpkg.Open(config.DatabaseConfig{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := dbpkg.Migrate(d.SQL); err != nil {
		t.Fatal(err)
	}
	f := &fixture{sqlDB: d.SQL, clock: time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)}
	f.svc = NewService(d.SQL, db.New(d.SQL), path)
	f.svc.now = func() time.Time { return f.clock }
	return f
}

func (f *fixture) names(t *testing.T) []string {
	t.Helper()
	all, err := f.svc.List()
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(all))
	for i, b := range all {
		out[i] = b.Name
	}
	return out
}

func TestCreate_IsAReadableCopy(t *testing.T) {
	// A quote in the folder name must not break VACUUM INTO.
	f := newFixture(t, filepath.Join(t.TempDir(), "it's here"))
	if _, err := f.sqlDB.Exec(`INSERT INTO settings (key, value, updated_at) VALUES ('marker', 'v1', 'x')`); err != nil {
		t.Fatal(err)
	}
	b, err := f.svc.Create(context.Background(), KindManual)
	if err != nil {
		t.Fatal(err)
	}
	if b.Name != "pilot-manual-20261117-120000.db" || b.Size == 0 {
		t.Fatalf("backup = %+v", b)
	}
	p, err := f.svc.Path(b.Name)
	if err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(p); info.Mode().Perm() != 0o600 {
		t.Errorf("backup mode = %v, want 0600 (it holds secrets)", info.Mode().Perm())
	}
	copyDB, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer copyDB.Close()
	var v string
	if err := copyDB.QueryRow(`SELECT value FROM settings WHERE key = 'marker'`).Scan(&v); err != nil || v != "v1" {
		t.Fatalf("backup content: %q, %v", v, err)
	}

	// Same second: a second backup gets a distinct name.
	b2, err := f.svc.Create(context.Background(), KindManual)
	if err != nil || b2.Name == b.Name {
		t.Fatalf("second backup = %+v, %v", b2, err)
	}
}

func TestPath_RejectsTraversal(t *testing.T) {
	f := newFixture(t, t.TempDir())
	for _, name := range []string{"../pilot.db", "pilot.db", "pilot-manual-20261117-120000.db/../../x", "/etc/passwd"} {
		if _, err := f.svc.Path(name); !errors.Is(err, ErrInvalidName) {
			t.Errorf("Path(%q) err = %v, want ErrInvalidName", name, err)
		}
	}
	if _, err := f.svc.Path("pilot-manual-20200101-000000.db"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing backup: %v", err)
	}
}

func TestRunScheduled_IntervalAndRetention(t *testing.T) {
	f := newFixture(t, t.TempDir())
	ctx := context.Background()
	if err := f.svc.SaveSettings(ctx, Settings{IntervalDays: 7, RetentionDays: 14}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Create(ctx, KindManual); err != nil {
		t.Fatal(err)
	}

	made, err := f.svc.RunScheduled(ctx)
	if err != nil || !made {
		t.Fatalf("first run: made=%v err=%v", made, err)
	}
	f.clock = f.clock.Add(6 * 24 * time.Hour)
	if made, _ = f.svc.RunScheduled(ctx); made {
		t.Fatal("made a backup before the interval passed")
	}
	// Weekly for five weeks.
	for range 5 {
		f.clock = f.clock.Add(24 * time.Hour)
		if made, _ = f.svc.RunScheduled(ctx); !made {
			t.Fatal("no backup after the interval")
		}
		f.clock = f.clock.Add(6 * 24 * time.Hour)
	}
	// Backups were made on days 0, 7, 14, 21, 28 and 35; the last prune
	// ran on day 35, so those older than 14 days (0, 7, 14) are gone and
	// day 21 (exactly 14 days old) stays. The manual one is never pruned.
	got := f.names(t)
	scheduled, manual := 0, 0
	for _, n := range got {
		if nameRe.FindStringSubmatch(n)[1] == KindScheduled {
			scheduled++
		} else {
			manual++
		}
	}
	if scheduled != 3 || manual != 1 {
		t.Fatalf("after pruning: %v (want 3 scheduled within 14 days + 1 manual)", got)
	}
}

func TestRunScheduled_KeepsNewestAndOffSwitch(t *testing.T) {
	f := newFixture(t, t.TempDir())
	ctx := context.Background()
	if err := f.svc.SaveSettings(ctx, Settings{IntervalDays: 0, RetentionDays: 1}); err != nil {
		t.Fatal(err)
	}
	if made, _ := f.svc.RunScheduled(ctx); made {
		t.Fatal("interval 0 must turn scheduled backups off")
	}
	if _, err := f.svc.Create(ctx, KindScheduled); err != nil {
		t.Fatal(err)
	}
	// Long past retention, with backups off: the newest one is kept.
	_ = f.svc.SaveSettings(ctx, Settings{IntervalDays: 365, RetentionDays: 1})
	f.clock = f.clock.Add(30 * 24 * time.Hour)
	if _, err := f.svc.RunScheduled(ctx); err != nil {
		t.Fatal(err)
	}
	if n := len(f.names(t)); n != 1 {
		t.Fatalf("%d backups, want the newest kept", n)
	}
}

func TestSettings(t *testing.T) {
	f := newFixture(t, t.TempDir())
	ctx := context.Background()
	if got, _ := f.svc.GetSettings(ctx); got != DefaultSettings() {
		t.Fatalf("default = %+v", got)
	}
	for _, bad := range []Settings{{IntervalDays: -1, RetentionDays: 7}, {IntervalDays: 7, RetentionDays: 0}} {
		var verr *ValidationError
		if err := f.svc.SaveSettings(ctx, bad); !errors.As(err, &verr) {
			t.Errorf("%+v: err = %v", bad, err)
		}
	}
}

func TestRestore_StagesAndApplies(t *testing.T) {
	dir := t.TempDir()
	f := newFixture(t, dir)
	ctx := context.Background()
	b, err := f.svc.Create(ctx, KindManual)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Restore(b.Name); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "pilot.db")
	if _, err := os.Stat(StagingPath(dbPath)); err != nil {
		t.Fatalf("restore not staged: %v", err)
	}

	// Simulate the next start: a stale WAL and SHM from the old database.
	other := filepath.Join(t.TempDir(), "pilot.db")
	_ = os.WriteFile(other, []byte("old db"), 0o600)
	_ = os.WriteFile(other+"-wal", []byte("old wal"), 0o600)
	_ = os.WriteFile(other+"-shm", []byte("old shm"), 0o600)
	staged, _ := os.ReadFile(StagingPath(dbPath))
	_ = os.WriteFile(StagingPath(other), staged, 0o600)

	applied, err := ApplyStagedRestore(other)
	if err != nil || !applied {
		t.Fatalf("applied=%v err=%v", applied, err)
	}
	for _, p := range []string{other + "-wal", other + "-shm", StagingPath(other)} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s still exists", p)
		}
	}
	if got, _ := os.ReadFile(other); string(got) != string(staged) {
		t.Error("database was not replaced by the backup")
	}
	if applied, _ := ApplyStagedRestore(other); applied {
		t.Error("nothing staged, but a restore was applied")
	}
}
