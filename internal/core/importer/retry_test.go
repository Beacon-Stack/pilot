package importer

// Regression tests for automatic import retry. The incident: the library
// folder was created by Docker as root:root 0755 while Pilot runs as uid
// 1000, so every import failed with "mkdir /tv/<Show>: permission denied".
// Downloads sat in /downloads with their raw names, and even after the
// operator fixed the ownership nothing re-imported them — each grab had to
// be re-imported by hand. These tests pin the recovery path end to end
// against a real (temp-file) SQLite database.

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/beacon-stack/pilot/internal/config"
	"github.com/beacon-stack/pilot/internal/core/mediamanagement"
	dbpkg "github.com/beacon-stack/pilot/internal/db"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/events"
)

type retryFixture struct {
	t        *testing.T
	sqlDB    *sql.DB
	q        db.Querier
	svc      *Service
	tvRoot   string
	packDir  string
	grabID   string
	episodes map[int]string // episode number -> episode id
}

const fixtureNow = "2026-11-16T12:00:00Z"

func newRetryFixture(t *testing.T) *retryFixture {
	t.Helper()
	dir := t.TempDir()

	d, err := dbpkg.Open(config.DatabaseConfig{Path: filepath.Join(dir, "pilot.db")})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	if err := dbpkg.Migrate(d.SQL); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	f := &retryFixture{
		t:        t,
		sqlDB:    d.SQL,
		q:        db.New(d.SQL),
		tvRoot:   filepath.Join(dir, "tv"),
		packDir:  filepath.Join(dir, "downloads", "Show.S01.1080p.WEB"),
		grabID:   "grab-1",
		episodes: map[int]string{1: "ep-1", 2: "ep-2"},
	}
	for _, p := range []string{f.tvRoot, f.packDir} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for n := range f.episodes {
		name := filepath.Join(f.packDir, "Show.S01E0"+string(rune('0'+n))+".1080p.WEB.mkv")
		if err := os.WriteFile(name, []byte("video bytes for episode"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	f.exec(`INSERT INTO quality_profiles (id, name, created_at, updated_at) VALUES ('qp', 'Any', ?, ?)`, fixtureNow, fixtureNow)
	f.exec(`INSERT INTO libraries (id, name, root_path, default_quality_profile_id, created_at, updated_at)
	        VALUES ('lib', 'TV', ?, 'qp', ?, ?)`, f.tvRoot, fixtureNow, fixtureNow)
	f.exec(`INSERT INTO series (id, tmdb_id, title, sort_title, year, library_id, quality_profile_id, added_at, updated_at)
	        VALUES ('series-1', 1, 'Show', 'show', 2022, 'lib', 'qp', ?, ?)`, fixtureNow, fixtureNow)
	f.exec(`INSERT INTO seasons (id, series_id, season_number) VALUES ('season-1', 'series-1', 1)`)
	for n, id := range f.episodes {
		f.exec(`INSERT INTO episodes (id, series_id, season_id, season_number, episode_number, title)
		        VALUES (?, 'series-1', 'season-1', 1, ?, ?)`, id, n, "Episode")
	}
	f.exec(`INSERT INTO grab_history (id, series_id, season_number, release_guid, release_title, download_status, grabbed_at)
	        VALUES (?, 'series-1', 1, 'guid-1', 'Show.S01.1080p.WEB', 'completed', ?)`, f.grabID, fixtureNow)

	logger := slog.New(slog.DiscardHandler)
	f.svc = NewService(f.q, events.New(logger), logger, mediamanagement.NewService(f.q), nil, nil)
	return f
}

func (f *retryFixture) exec(query string, args ...any) {
	f.t.Helper()
	if _, err := f.sqlDB.Exec(query, args...); err != nil {
		f.t.Fatalf("exec %q: %v", query, err)
	}
}

func (f *retryFixture) grab() db.GrabHistory {
	f.t.Helper()
	g, err := f.q.GetGrabByID(context.Background(), f.grabID)
	if err != nil {
		f.t.Fatalf("get grab: %v", err)
	}
	return g
}

func (f *retryFixture) episodeFileCount() int {
	f.t.Helper()
	var n int
	if err := f.sqlDB.QueryRow(`SELECT COUNT(*) FROM episode_files`).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

func (f *retryFixture) assertImported() {
	f.t.Helper()
	g := f.grab()
	if g.ImportStatus != "imported" {
		f.t.Fatalf("import_status = %v, want %q", g.ImportStatus, "imported")
	}
	for n, id := range f.episodes {
		ep, err := f.q.GetEpisode(context.Background(), id)
		if err != nil {
			f.t.Fatal(err)
		}
		if !ep.HasFile {
			f.t.Errorf("episode %d has_file = false after import", n)
		}
	}
	if got := f.episodeFileCount(); got != len(f.episodes) {
		f.t.Errorf("episode_files rows = %d, want %d", got, len(f.episodes))
	}
	seasonDir := filepath.Join(f.tvRoot, "Show (2022)", "Season 01")
	entries, err := os.ReadDir(seasonDir)
	if err != nil {
		f.t.Fatalf("season folder not created: %v", err)
	}
	if len(entries) != len(f.episodes) {
		f.t.Errorf("season folder has %d files, want %d", len(entries), len(f.episodes))
	}
}

// importLikeSubscribe mirrors the TypeDownloadDone handler: import, then
// record the outcome on the grab.
func (f *retryFixture) importLikeSubscribe(contentPath string) error {
	err := f.svc.ImportFile(context.Background(), f.grabID, contentPath)
	f.svc.recordImportOutcome(context.Background(), f.grabID, contentPath, err)
	return err
}

func skipIfRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("permission checks are bypassed when running as root")
	}
}

// Headline regression: unwritable library -> import fails and is recorded;
// retries hold (without burning attempts) while still unwritable; once the
// folder is fixed the next run imports and renames into a season folder.
func TestRetry_UnwritableLibraryRecoversAfterFix(t *testing.T) {
	skipIfRoot(t)
	f := newRetryFixture(t)
	ctx := context.Background()

	if err := os.Chmod(f.tvRoot, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(f.tvRoot, 0o755) })

	if err := f.importLikeSubscribe(f.packDir); err == nil {
		t.Fatal("ImportFile into a read-only library succeeded; want permission error")
	}
	g := f.grab()
	if g.ImportStatus != "failed" || g.ImportAttempts != 1 {
		t.Fatalf("after failure: status=%s attempts=%d, want failed/1", g.ImportStatus, g.ImportAttempts)
	}
	if g.ContentPath != f.packDir {
		t.Fatalf("content_path = %s, want %s (needed to retry)", g.ContentPath, f.packDir)
	}

	later := time.Now().UTC().Add(24 * time.Hour)
	res, err := f.svc.RetryFailedImports(ctx, later)
	if err != nil {
		t.Fatal(err)
	}
	if res.Held != 1 || res.Attempted != 0 {
		t.Fatalf("while unwritable: %+v, want Held=1 Attempted=0", res)
	}
	if got := f.grab().ImportAttempts; got != 1 {
		t.Fatalf("held retry spent an attempt: attempts=%d, want 1", got)
	}

	if err := os.Chmod(f.tvRoot, 0o755); err != nil { // operator fixes ownership
		t.Fatal(err)
	}
	res, err = f.svc.RetryFailedImports(ctx, later)
	if err != nil {
		t.Fatal(err)
	}
	if res.Imported != 1 {
		t.Fatalf("after fix: %+v, want Imported=1", res)
	}
	f.assertImported()

	// Imported grabs drop out of the retry set.
	res, err = f.svc.RetryFailedImports(ctx, later)
	if err != nil {
		t.Fatal(err)
	}
	if res != (RetryResult{}) {
		t.Fatalf("second run after success: %+v, want no-op", res)
	}
}

func TestRetry_RespectsBackoff(t *testing.T) {
	f := newRetryFixture(t)
	ctx := context.Background()
	now := time.Now().UTC()
	f.exec(`UPDATE grab_history SET import_status='failed', import_attempts=1, content_path=?, import_last_attempt_at=? WHERE id=?`,
		f.packDir, now.Format(time.RFC3339), f.grabID)

	res, err := f.svc.RetryFailedImports(ctx, now.Add(retryBaseDelay-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if res.NotDue != 1 || res.Attempted != 0 {
		t.Fatalf("before backoff elapsed: %+v, want NotDue=1", res)
	}

	res, err = f.svc.RetryFailedImports(ctx, now.Add(retryBaseDelay+time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if res.Imported != 1 {
		t.Fatalf("after backoff elapsed: %+v, want Imported=1", res)
	}
}

func TestRetryDelay_Schedule(t *testing.T) {
	cases := map[int64]time.Duration{
		0:  0,
		1:  15 * time.Minute,
		2:  30 * time.Minute,
		3:  time.Hour,
		5:  4 * time.Hour,
		6:  retryMaxDelay,
		50: retryMaxDelay,
	}
	for attempts, want := range cases {
		if got := retryDelay(attempts); got != want {
			t.Errorf("retryDelay(%d) = %s, want %s", attempts, got, want)
		}
	}
}

func TestRetry_GivesUpAfterMaxAttempts(t *testing.T) {
	f := newRetryFixture(t)
	ctx := context.Background()
	missing := filepath.Join(filepath.Dir(f.packDir), "deleted-by-user")
	f.exec(`UPDATE grab_history SET import_status='failed', import_attempts=?, content_path=?, import_last_attempt_at=? WHERE id=?`,
		MaxImportAttempts-1, missing, "2000-01-01T00:00:00Z", f.grabID)

	res, err := f.svc.RetryFailedImports(ctx, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if res.Attempted != 1 || res.GaveUp != 1 {
		t.Fatalf("final attempt: %+v, want Attempted=1 GaveUp=1", res)
	}
	res, err = f.svc.RetryFailedImports(ctx, time.Now().UTC().Add(365*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if res != (RetryResult{}) {
		t.Fatalf("after giving up: %+v, want grab excluded from retries", res)
	}
}

// A retry of a pack whose files were already imported must neither fail on
// the UNIQUE episode_files.path nor damage the (hardlinked) source files.
func TestRetry_AlreadyImportedPackIsIdempotent(t *testing.T) {
	f := newRetryFixture(t)
	ctx := context.Background()

	if err := f.importLikeSubscribe(f.packDir); err != nil {
		t.Fatalf("first import: %v", err)
	}
	f.exec(`UPDATE grab_history SET import_status='failed', import_last_attempt_at='2000-01-01T00:00:00Z' WHERE id=?`, f.grabID)

	res, err := f.svc.RetryFailedImports(ctx, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if res.Imported != 1 {
		t.Fatalf("retry of already-imported pack: %+v, want Imported=1", res)
	}
	f.assertImported()

	entries, _ := os.ReadDir(f.packDir)
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(f.packDir, e.Name()))
		if err != nil || !bytes.Equal(b, []byte("video bytes for episode")) {
			t.Errorf("source %s damaged by retry (err=%v, len=%d)", e.Name(), err, len(b))
		}
	}
}

// transferFile onto a destination that is already a hardlink of the source
// used to os.Create (O_TRUNC) the shared inode, wiping the download.
func TestTransferFile_DestAlreadyHardlinkedKeepsContent(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.mkv")
	dst := filepath.Join(dir, "dst.mkv")
	content := []byte("precious download")
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(src, dst); err != nil {
		t.Skipf("hardlinks unsupported here: %v", err)
	}
	if err := transferFile(src, dst); err != nil {
		t.Fatalf("transferFile: %v", err)
	}
	for _, p := range []string{src, dst} {
		b, err := os.ReadFile(p)
		if err != nil || !bytes.Equal(b, content) {
			t.Fatalf("%s content = %q (err=%v), want %q", p, b, err, content)
		}
	}
	if _, err := os.Stat(dst + ".pilot-tmp"); !os.IsNotExist(err) {
		t.Errorf("temp file left behind: %v", err)
	}
}

// End to end against SQLite: a fake single-file .exe is recorded with the
// retry budget exhausted, so the retry job never touches it.
func TestRetry_UnimportableContentIsNeverRetried(t *testing.T) {
	f := newRetryFixture(t)
	fake := filepath.Join(filepath.Dir(f.packDir), "Show.S01E01.1080p.WEB.exe")
	if err := os.WriteFile(fake, []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := f.importLikeSubscribe(fake); !errors.Is(err, ErrUnimportableContent) {
		t.Fatalf("import of .exe: err = %v, want ErrUnimportableContent", err)
	}
	if g := f.grab(); g.ImportAttempts != MaxImportAttempts {
		t.Fatalf("import_attempts = %d, want %d (budget exhausted)", g.ImportAttempts, MaxImportAttempts)
	}
	res, err := f.svc.RetryFailedImports(context.Background(), time.Now().UTC().Add(365*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if res != (RetryResult{}) {
		t.Fatalf("retry run: %+v, want no-op", res)
	}
}
