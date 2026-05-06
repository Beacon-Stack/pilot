package jobs

// file_reconciler_test.go — regression suite for QW5. Pins the
// behaviours an operator depends on:
//   - missing files DO flip has_file=FALSE
//   - permission/transient stat errors DO NOT flip has_file
//   - completed-grab-without-file (the Maul S01 class) is detected
//   - retry events are deduped per episode per run
//   - a single bad row never aborts the loop

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/events"
)

// reconcilerMockQuerier — local mock keeps the test file self-contained.
// We can't reuse rss_sync_test.go's mockQuerier directly because its
// methods are scoped to that file's needs.
type reconcilerMockQuerier struct {
	db.Querier

	files          []db.ListAllEpisodeFilesRow
	episodes       map[string]db.Episode
	completedGrabs []db.GrabHistory

	deletedFiles  []string
	flippedFlags  []db.UpdateEpisodeHasFileParams
	listFilesErr  error
	listGrabsErr  error
	getEpisodeErr error

	mu sync.Mutex
}

func (m *reconcilerMockQuerier) ListAllEpisodeFiles(_ context.Context) ([]db.ListAllEpisodeFilesRow, error) {
	if m.listFilesErr != nil {
		return nil, m.listFilesErr
	}
	return m.files, nil
}

func (m *reconcilerMockQuerier) DeleteEpisodeFile(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deletedFiles = append(m.deletedFiles, id)
	return nil
}

func (m *reconcilerMockQuerier) UpdateEpisodeHasFile(_ context.Context, p db.UpdateEpisodeHasFileParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.flippedFlags = append(m.flippedFlags, p)
	return nil
}

func (m *reconcilerMockQuerier) ListGrabHistoryByStatusSince(_ context.Context, _ db.ListGrabHistoryByStatusSinceParams) ([]db.GrabHistory, error) {
	if m.listGrabsErr != nil {
		return nil, m.listGrabsErr
	}
	return m.completedGrabs, nil
}

func (m *reconcilerMockQuerier) GetEpisode(_ context.Context, id string) (db.Episode, error) {
	if m.getEpisodeErr != nil {
		return db.Episode{}, m.getEpisodeErr
	}
	if ep, ok := m.episodes[id]; ok {
		return ep, nil
	}
	return db.Episode{}, errors.New("not found")
}

func (m *reconcilerMockQuerier) deleted() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.deletedFiles))
	copy(out, m.deletedFiles)
	return out
}

func (m *reconcilerMockQuerier) flipped() []db.UpdateEpisodeHasFileParams {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]db.UpdateEpisodeHasFileParams, len(m.flippedFlags))
	copy(out, m.flippedFlags)
	return out
}

func nullLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

// Headline test: a file that's definitively gone from disk MUST flip
// has_file=FALSE and delete the episode_files row. Without this, the
// UI keeps rendering the green "downloaded" pill on an episode whose
// underlying file is long gone — the exact symptom the lifecycle-trust
// plan is closing.
func TestFileReconciler_MissingFileFlipsHasFile(t *testing.T) {
	tmp := t.TempDir()
	gonePath := filepath.Join(tmp, "this-file-was-deleted-by-the-user.mkv")
	// Note: we never create the file → os.Stat will return ErrNotExist.

	q := &reconcilerMockQuerier{
		files: []db.ListAllEpisodeFilesRow{
			{ID: "ef-1", EpisodeID: "ep-1", SeriesID: "show-1", Path: gonePath},
		},
	}
	bus := events.New(nullLogger())

	missing, retry, err := runFileExistenceReconciler(context.Background(), q, bus, nullLogger(), time.Now().UTC())
	if err != nil {
		t.Fatalf("runFileExistenceReconciler: %v", err)
	}

	if missing != 1 {
		t.Errorf("missing count = %d, want 1", missing)
	}
	if retry != 1 {
		t.Errorf("retry count = %d, want 1 (one event per flipped episode)", retry)
	}

	if got := q.deleted(); len(got) != 1 || got[0] != "ef-1" {
		t.Errorf("expected ef-1 deleted; got %v", got)
	}
	if got := q.flipped(); len(got) != 1 || got[0].ID != "ep-1" || got[0].HasFile {
		t.Errorf("expected ep-1 flipped to has_file=false; got %+v", got)
	}
}

// Counterpart: a file that EXISTS must NOT be touched. This protects
// against a regression where the reconciler decides to nuke every row
// because (e.g.) the path field is being read incorrectly.
func TestFileReconciler_PresentFileNotTouched(t *testing.T) {
	tmp := t.TempDir()
	realPath := filepath.Join(tmp, "good.mkv")
	if err := os.WriteFile(realPath, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	q := &reconcilerMockQuerier{
		files: []db.ListAllEpisodeFilesRow{
			{ID: "ef-1", EpisodeID: "ep-1", SeriesID: "s-1", Path: realPath},
		},
	}
	missing, retry, err := runFileExistenceReconciler(context.Background(), q, events.New(nullLogger()), nullLogger(), time.Now().UTC())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if missing != 0 || retry != 0 {
		t.Errorf("expected zero changes for an existing file; got missing=%d retry=%d", missing, retry)
	}
	if len(q.deleted()) != 0 || len(q.flipped()) != 0 {
		t.Errorf("present file MUST NOT trigger DB writes; deletes=%v flips=%v", q.deleted(), q.flipped())
	}
}

// A non-ENOENT stat error (permission denied, mount stalled) MUST NOT
// flip has_file. Otherwise a brief NFS hiccup looks like every file
// in the library disappeared.
func TestFileReconciler_TransientStatErrorDoesNotFlip(t *testing.T) {
	// On Unix, a non-existent /dev/null/x path returns ENOTDIR when stat'd
	// because /dev/null is a file, not a dir — that's a non-ENOENT error.
	notDirPath := "/dev/null/notadir.mkv"

	q := &reconcilerMockQuerier{
		files: []db.ListAllEpisodeFilesRow{
			{ID: "ef-1", EpisodeID: "ep-1", SeriesID: "s-1", Path: notDirPath},
		},
	}
	missing, retry, err := runFileExistenceReconciler(context.Background(), q, events.New(nullLogger()), nullLogger(), time.Now().UTC())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if missing != 0 {
		t.Errorf("transient stat error must NOT count as missing; got %d", missing)
	}
	if retry != 0 {
		t.Errorf("transient stat error must NOT fire retry; got %d", retry)
	}
	if len(q.deleted()) != 0 || len(q.flipped()) != 0 {
		t.Errorf("transient stat error MUST NOT trigger DB writes; deletes=%v flips=%v", q.deleted(), q.flipped())
	}
}

// One bad row mustn't kill the loop. Mix one missing and one present
// — both branches must execute.
func TestFileReconciler_OneBadRowDoesNotAbortLoop(t *testing.T) {
	tmp := t.TempDir()
	good := filepath.Join(tmp, "good.mkv")
	if err := os.WriteFile(good, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(tmp, "gone.mkv")

	q := &reconcilerMockQuerier{
		files: []db.ListAllEpisodeFilesRow{
			{ID: "ef-gone", EpisodeID: "ep-gone", SeriesID: "s-1", Path: gone},
			{ID: "ef-good", EpisodeID: "ep-good", SeriesID: "s-1", Path: good},
		},
	}
	missing, _, err := runFileExistenceReconciler(context.Background(), q, events.New(nullLogger()), nullLogger(), time.Now().UTC())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if missing != 1 {
		t.Errorf("expected exactly 1 missing; got %d", missing)
	}
	flipped := q.flipped()
	if len(flipped) != 1 || flipped[0].ID != "ep-gone" {
		t.Errorf("expected only ep-gone flipped; got %+v", flipped)
	}
}

// Pass-2 test: the Maul S01 class. A `completed` grab whose episode
// has has_file=FALSE (and never had an episode_files row) MUST fire
// TypeImportRetryNeeded — that's the silent-import-failure case the
// reconciler is designed to surface to the activity log.
func TestFileReconciler_CompletedGrabWithoutFileQueuesRetry(t *testing.T) {
	q := &reconcilerMockQuerier{
		// No episode_files rows.
		files: nil,
		// One episode with has_file=FALSE.
		episodes: map[string]db.Episode{
			"ep-orphan": {ID: "ep-orphan", SeriesID: "s-1", HasFile: false},
		},
		// One completed grab pointing at it.
		completedGrabs: []db.GrabHistory{
			{
				ID:           "g-1",
				SeriesID:     "s-1",
				EpisodeID:    sql.NullString{String: "ep-orphan", Valid: true},
				ReleaseTitle: "Maul.S01E07",
			},
		},
	}
	bus := events.New(nullLogger())
	var got atomic.Int32
	bus.Subscribe(func(_ context.Context, e events.Event) {
		if e.Type == events.TypeImportRetryNeeded {
			got.Add(1)
		}
	})

	_, retry, err := runFileExistenceReconciler(context.Background(), q, bus, nullLogger(), time.Now().UTC())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if retry != 1 {
		t.Errorf("expected one retry; got %d", retry)
	}
	time.Sleep(20 * time.Millisecond) // events fire async
	if got.Load() != 1 {
		t.Errorf("expected one TypeImportRetryNeeded fired; got %d", got.Load())
	}
}

// Counterpart for pass 2: a `completed` grab whose episode DOES have a
// file (has_file=TRUE) must NOT fire a retry. Otherwise the reconciler
// would retry-spam every successfully-imported show.
func TestFileReconciler_CompletedGrabWithFileNoRetry(t *testing.T) {
	q := &reconcilerMockQuerier{
		files: nil,
		episodes: map[string]db.Episode{
			"ep-imported": {ID: "ep-imported", SeriesID: "s-1", HasFile: true},
		},
		completedGrabs: []db.GrabHistory{
			{
				ID:        "g-1",
				SeriesID:  "s-1",
				EpisodeID: sql.NullString{String: "ep-imported", Valid: true},
			},
		},
	}
	_, retry, err := runFileExistenceReconciler(context.Background(), q, events.New(nullLogger()), nullLogger(), time.Now().UTC())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if retry != 0 {
		t.Errorf("retry must NOT fire for a successfully-imported episode; got %d", retry)
	}
}

// Episodes that appear in BOTH passes (file just flipped + completed
// grab exists for them) get exactly one retry event. A double-fire
// would double-count in the activity log.
func TestFileReconciler_DoesNotDoubleFireForSameEpisode(t *testing.T) {
	tmp := t.TempDir()
	gone := filepath.Join(tmp, "gone.mkv")
	q := &reconcilerMockQuerier{
		files: []db.ListAllEpisodeFilesRow{
			{ID: "ef-1", EpisodeID: "ep-double", SeriesID: "s-1", Path: gone},
		},
		episodes: map[string]db.Episode{
			"ep-double": {ID: "ep-double", SeriesID: "s-1", HasFile: false},
		},
		completedGrabs: []db.GrabHistory{
			{
				ID:        "g-1",
				SeriesID:  "s-1",
				EpisodeID: sql.NullString{String: "ep-double", Valid: true},
			},
		},
	}
	bus := events.New(nullLogger())
	var fired atomic.Int32
	bus.Subscribe(func(_ context.Context, e events.Event) {
		if e.Type == events.TypeImportRetryNeeded {
			fired.Add(1)
		}
	})

	_, retry, err := runFileExistenceReconciler(context.Background(), q, bus, nullLogger(), time.Now().UTC())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if retry != 1 {
		t.Errorf("retry count must dedupe per episode; got %d (want 1)", retry)
	}
	time.Sleep(20 * time.Millisecond)
	if fired.Load() != 1 {
		t.Errorf("expected exactly ONE event for the duplicated episode; got %d", fired.Load())
	}
}

// ListAllEpisodeFiles failure short-circuits with a non-nil error.
// The job runner uses this to log task failure — without the explicit
// return, downstream queries would also fail and noise up the logs.
func TestFileReconciler_ListFilesErrorReturns(t *testing.T) {
	q := &reconcilerMockQuerier{listFilesErr: errors.New("db down")}
	_, _, err := runFileExistenceReconciler(context.Background(), q, events.New(nullLogger()), nullLogger(), time.Now().UTC())
	if err == nil {
		t.Fatal("expected error when ListAllEpisodeFiles fails; got nil")
	}
}
