// Package stallwatcher_test — regression suite for the dead-torrent watcher.
//
// ⚠ These tests exist because the "847 seeders, actually 0" bug recurred.
// They cover the full Pilot side of the post-grab feedback loop:
//
//   - Fake Haul HTTP server returning a stalled torrent
//   - Real blocklist.Service writing through a mock querier
//   - Real stallwatcher.Service Tick() doing the whole flow
//
// When to run:
//
//	go test ./internal/core/stallwatcher/... -v
//
// Run this before editing any of:
//
//   - internal/core/stallwatcher/*
//   - internal/core/blocklist/*
//   - internal/core/indexer/service.go (grab creation, CreateGrab signature)
//   - plugins/downloaders/haul/haul.go (ListStalled method)
//   - internal/db/queries/postgres/blocklist.sql
//   - internal/db/queries/postgres/grab_history.sql
//
// DO NOT weaken these tests. Every subtest here exists because the
// corresponding failure mode cost someone real time in the dead-torrent
// bug. If a test fails, the right response is to fix the code, not the
// test.
package stallwatcher

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/beacon-stack/pilot/internal/core/blocklist"
	"github.com/beacon-stack/pilot/internal/core/downloader"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/events"
)

// ── Fake Haul HTTP server ─────────────────────────────────────────────────────

// fakeHaul mimics Haul's GET /api/v1/stalls endpoint. Tests control what
// stalled torrents it returns via setStalls().
type fakeHaul struct {
	mu      sync.Mutex
	stalled []map[string]any
	calls   atomic.Int32
}

func newFakeHaul(t *testing.T) *fakeHaul {
	t.Helper()
	return &fakeHaul{}
}

func (f *fakeHaul) setStalls(s []map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stalled = s
}

func (f *fakeHaul) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/v1/stalls", func(w http.ResponseWriter, _ *http.Request) {
		f.calls.Add(1)
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(f.stalled)
	})
	return mux
}

// ── Mock querier ──────────────────────────────────────────────────────────────

// mockQuerier implements db.Querier via embedding — any method the tests
// don't override panics if called. This is intentional: it forces the
// test to declare exactly which DB methods the code under test is allowed
// to use. A new DB call from stallwatcher shows up as a panic, not a
// silent pass.
type mockQuerier struct {
	db.Querier

	mu sync.Mutex

	// grabByInfoHash maps info_hash → GrabHistory row.
	grabByInfoHash map[string]db.GrabHistory
	// blocklist stores created rows keyed by release_guid. Unique index
	// simulation is enforced on insert.
	blocklistByGUID map[string]db.Blocklist
	// updatedStatuses tracks (grab_id, status, downloaded_bytes) triples.
	updatedStatuses []updateStatusCall
	// stallCountsByEpisode returns N for the recent-stalls count query.
	stallCountsByEpisode map[string]int64
}

type updateStatusCall struct {
	GrabID         string
	DownloadStatus string
	Bytes          int32
}

func newMockQuerier() *mockQuerier {
	return &mockQuerier{
		grabByInfoHash:       map[string]db.GrabHistory{},
		blocklistByGUID:      map[string]db.Blocklist{},
		stallCountsByEpisode: map[string]int64{},
	}
}

func (m *mockQuerier) GetGrabByInfoHash(ctx context.Context, infoHash sql.NullString) (db.GrabHistory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !infoHash.Valid {
		return db.GrabHistory{}, sql.ErrNoRows
	}
	row, ok := m.grabByInfoHash[infoHash.String]
	if !ok {
		return db.GrabHistory{}, sql.ErrNoRows
	}
	return row, nil
}

func (m *mockQuerier) UpdateGrabStatus(ctx context.Context, arg db.UpdateGrabStatusParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updatedStatuses = append(m.updatedStatuses, updateStatusCall{
		GrabID:         arg.ID,
		DownloadStatus: arg.DownloadStatus,
		Bytes:          arg.DownloadedBytes,
	})
	// Reflect back into the in-memory map so later queries see the status.
	for k, v := range m.grabByInfoHash {
		if v.ID == arg.ID {
			v.DownloadStatus = arg.DownloadStatus
			m.grabByInfoHash[k] = v
		}
	}
	return nil
}

func (m *mockQuerier) CreateBlocklistEntry(ctx context.Context, arg db.CreateBlocklistEntryParams) (db.Blocklist, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.blocklistByGUID[arg.ReleaseGuid]; exists {
		// Simulate unique index on release_guid.
		return db.Blocklist{}, errUniqueViolation
	}
	row := db.Blocklist(arg)
	m.blocklistByGUID[arg.ReleaseGuid] = row
	return row, nil
}

func (m *mockQuerier) CountRecentStallsForEpisode(ctx context.Context, arg db.CountRecentStallsForEpisodeParams) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := arg.SeriesID + "|" + arg.EpisodeID.String
	return m.stallCountsByEpisode[key], nil
}

// ListStaleActiveGrabs returns rows whose grabbed_at is older than the
// supplied cutoff AND match the no-info_hash + non-terminal-status
// constraint. Tests that don't seed any stale grabs get an empty slice
// (the production behavior most tests want — sweep is a no-op).
func (m *mockQuerier) ListStaleActiveGrabs(ctx context.Context, olderThan string) ([]db.GrabHistory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	nonTerminal := map[string]bool{"downloading": true, "queued": true, "pending": true}
	var out []db.GrabHistory
	// Iterate the same in-memory map the other queries use. Tests that
	// want to seed stale grabs put them in `grabByInfoHash` keyed by ""
	// (empty string = no info_hash) — see TestSweepStaleGrabs_*.
	for _, g := range m.grabByInfoHash {
		if !nonTerminal[g.DownloadStatus] {
			continue
		}
		if g.InfoHash.Valid && g.InfoHash.String != "" {
			continue // has info_hash — handled by the haul-stall path
		}
		if g.GrabbedAt >= olderThan {
			continue // not stale yet
		}
		out = append(out, g)
	}
	return out, nil
}

//nolint:revive // param name must match sqlc-generated Querier interface
func (m *mockQuerier) DeleteBlocklistEntryByGUID(ctx context.Context, releaseGuid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.blocklistByGUID, releaseGuid)
	return nil
}

// errUniqueViolation is the sentinel error used by the mock to simulate
// Postgres's unique constraint violation. Uses the real pgconn.PgError
// type so dbutil.IsUniqueViolation's errors.As check matches and the
// blocklist service converts it to ErrAlreadyBlocklisted.
var errUniqueViolation = &pgconn.PgError{
	Code:    "23505",
	Message: "duplicate key value violates unique constraint",
}

// ── Fake downloader lister ────────────────────────────────────────────────────

// fakeDownloaderLister implements stallwatcher's downloadClientLister
// interface. Tests set the returned list of configs to control which
// download clients the stallwatcher sees.
type fakeDownloaderLister struct {
	configs []downloader.Config
}

func (f *fakeDownloaderLister) List(ctx context.Context) ([]downloader.Config, error) {
	return f.configs, nil
}

// ── Test helper: build a stallwatcher wired to a fake Haul + mock DB ─────────

type testRig struct {
	stall      *Service
	mock       *mockQuerier
	haul       *fakeHaul
	haulServer *httptest.Server
	bus        *events.Bus
}

func newRig(t *testing.T) *testRig {
	t.Helper()

	haul := newFakeHaul(t)
	haulServer := httptest.NewServer(haul.handler())
	t.Cleanup(haulServer.Close)

	mock := newMockQuerier()
	blocklistSvc := blocklist.NewService(mock)

	downloaderList := &fakeDownloaderLister{
		configs: []downloader.Config{
			{
				ID:      "haul-1",
				Name:    "Haul",
				Kind:    "haul",
				Enabled: true,
				Settings: json.RawMessage(
					`{"url":"` + haulServer.URL + `","api_key":"test"}`,
				),
			},
		},
	}

	bus := events.New(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	svc := &Service{
		q:          mock,
		blocklist:  blocklistSvc,
		downloader: downloaderList,
		bus:        bus,
		logger:     logger,
		startedAt:  time.Now().Add(-10 * time.Minute), // past grace so side effects fire
		interval:   time.Hour,
	}

	return &testRig{
		stall:      svc,
		mock:       mock,
		haul:       haul,
		haulServer: haulServer,
		bus:        bus,
	}
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestTick_BlocklistsStalledGrab is the headline regression test: a grab
// that matches a stalled info_hash gets blocklisted with the right reason,
// and the grab status is updated to "stalled".
func TestTick_BlocklistsStalledGrab(t *testing.T) {
	rig := newRig(t)

	// Pre-populate the mock with a grab that will match the stalled torrent.
	rig.mock.grabByInfoHash["deadbeef00"] = db.GrabHistory{
		ID:             "grab-1",
		SeriesID:       "series-1",
		EpisodeID:      sql.NullString{String: "ep-1", Valid: true},
		ReleaseGuid:    "guid-1",
		ReleaseTitle:   "Raised.by.Wolves.2020.S01E01.WEB.x264-PHOENiX[TGx]",
		IndexerID:      sql.NullString{String: "idx-1337x", Valid: true},
		Protocol:       "torrent",
		Size:           365599177,
		DownloadStatus: "queued",
		Source:         "interactive",
		InfoHash:       sql.NullString{String: "deadbeef00", Valid: true},
	}

	rig.haul.setStalls([]map[string]any{
		{
			"info_hash":     "deadbeef00",
			"name":          "Raised.by.Wolves.2020.S01E01.WEB.x264-PHOENiX[TGx]",
			"reason":        "no_peers_ever",
			"level":         4,
			"inactive_secs": 200,
			"added_at":      time.Now().Add(-4 * time.Minute).Format(time.RFC3339),
		},
	})

	if err := rig.stall.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	// Blocklist entry should exist with reason stall_no_peers_ever.
	rig.mock.mu.Lock()
	bl, ok := rig.mock.blocklistByGUID["guid-1"]
	rig.mock.mu.Unlock()
	if !ok {
		t.Fatal("expected blocklist entry for guid-1, got none — stallwatcher failed to blocklist. " +
			"Check stallwatcher.handleStall() and blocklist.AddFromStall().")
	}
	if bl.Reason != blocklist.ReasonStallNoPeersEver {
		t.Errorf("wrong blocklist reason: got %q want %q", bl.Reason, blocklist.ReasonStallNoPeersEver)
	}
	if bl.InfoHash.String != "deadbeef00" {
		t.Errorf("blocklist info_hash should be deadbeef00, got %q", bl.InfoHash.String)
	}

	// Grab status should be updated to "stalled".
	rig.mock.mu.Lock()
	var found bool
	for _, call := range rig.mock.updatedStatuses {
		if call.GrabID == "grab-1" && call.DownloadStatus == "stalled" {
			found = true
		}
	}
	rig.mock.mu.Unlock()
	if !found {
		t.Error("expected grab-1 to be marked stalled via UpdateGrabStatus, but no such call was made")
	}
}

// TestTick_SkipsNonMatchingInfoHash verifies that a stalled torrent that
// doesn't match any grab in history is silently ignored. This is the
// "test torrent added via Haul UI" case — not our problem to blocklist.
func TestTick_SkipsNonMatchingInfoHash(t *testing.T) {
	rig := newRig(t)

	rig.haul.setStalls([]map[string]any{
		{
			"info_hash": "orphanedhash00",
			"name":      "somebody-elses-torrent",
			"reason":    "no_peers_ever",
			"level":     4,
		},
	})

	if err := rig.stall.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if len(rig.mock.blocklistByGUID) != 0 {
		t.Errorf("expected no blocklist entries, got %d", len(rig.mock.blocklistByGUID))
	}
}

// TestTick_RespectsStartupGrace is the false-positive-suppression guard:
// a stall seen within the first 2 minutes of watcher uptime does NOT
// trigger a blocklist insert, because we can't distinguish "real dead
// torrent" from "Pilot just restarted and hasn't observed the torrent yet".
func TestTick_RespectsStartupGrace(t *testing.T) {
	rig := newRig(t)
	// Override startedAt so we're inside the grace window.
	rig.stall.startedAt = time.Now().Add(-30 * time.Second)

	rig.mock.grabByInfoHash["deadbeef00"] = db.GrabHistory{
		ID:             "grab-1",
		SeriesID:       "series-1",
		EpisodeID:      sql.NullString{String: "ep-1", Valid: true},
		ReleaseGuid:    "guid-1",
		ReleaseTitle:   "test",
		Protocol:       "torrent",
		DownloadStatus: "queued",
		Source:         "interactive",
		InfoHash:       sql.NullString{String: "deadbeef00", Valid: true},
	}
	rig.haul.setStalls([]map[string]any{
		{"info_hash": "deadbeef00", "reason": "no_peers_ever", "level": 4},
	})

	if err := rig.stall.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if len(rig.mock.blocklistByGUID) != 0 {
		t.Fatal("blocklist was touched during startup grace period — should not happen. " +
			"This suppresses false positives when Pilot restarts and Haul reports a torrent " +
			"as stalled before Pilot's grab_history has caught up.")
	}
}

// TestTick_SkipsAlreadyTerminalGrabs verifies the watcher doesn't
// re-blocklist a grab that's already in a final state (completed,
// failed, stalled, removed). Idempotency guard — Haul may keep
// reporting the same stall forever until the torrent is removed.
func TestTick_SkipsAlreadyTerminalGrabs(t *testing.T) {
	rig := newRig(t)
	rig.mock.grabByInfoHash["deadbeef00"] = db.GrabHistory{
		ID:             "grab-1",
		SeriesID:       "series-1",
		EpisodeID:      sql.NullString{String: "ep-1", Valid: true},
		ReleaseGuid:    "guid-1",
		ReleaseTitle:   "test",
		Protocol:       "torrent",
		DownloadStatus: "stalled", // already stalled
		Source:         "interactive",
		InfoHash:       sql.NullString{String: "deadbeef00", Valid: true},
	}
	rig.haul.setStalls([]map[string]any{
		{"info_hash": "deadbeef00", "reason": "no_peers_ever", "level": 4},
	})

	if err := rig.stall.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if len(rig.mock.blocklistByGUID) != 0 {
		t.Error("re-blocklisted an already-stalled grab — idempotency regression")
	}
}

// TestTick_CircuitBreaker verifies the auto-re-search circuit breaker:
// when there are already 3+ stall-reason blocklist entries for an
// episode in the last 24h, no further TypeAutoSearchRetry event fires
// even though the grab is blocklisted. Prevents infinite retry loops
// when every release for an episode is dead.
func TestTick_CircuitBreaker(t *testing.T) {
	rig := newRig(t)

	rig.mock.grabByInfoHash["deadbeef00"] = db.GrabHistory{
		ID:             "grab-1",
		SeriesID:       "series-1",
		EpisodeID:      sql.NullString{String: "ep-1", Valid: true},
		ReleaseGuid:    "guid-1",
		ReleaseTitle:   "test",
		Protocol:       "torrent",
		DownloadStatus: "queued",
		Source:         "auto_search",
		InfoHash:       sql.NullString{String: "deadbeef00", Valid: true},
	}
	// Simulate 3 already-stalled-and-blocklisted entries for this episode.
	rig.mock.stallCountsByEpisode["series-1|ep-1"] = 3

	rig.haul.setStalls([]map[string]any{
		{"info_hash": "deadbeef00", "reason": "no_peers_ever", "level": 4},
	})

	var retryFired, gaveUpFired atomic.Int32
	rig.bus.Subscribe(func(_ context.Context, e events.Event) {
		switch e.Type { //nolint:exhaustive // test only watches two event types
		case events.TypeAutoSearchRetry:
			retryFired.Add(1)
		case events.TypeGrabStalledGaveUp:
			gaveUpFired.Add(1)
		}
	})

	if err := rig.stall.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	// Allow async handlers to fire.
	time.Sleep(50 * time.Millisecond)

	if retryFired.Load() != 0 {
		t.Errorf("circuit breaker breach: TypeAutoSearchRetry fired %d times, should be 0",
			retryFired.Load())
	}
	if gaveUpFired.Load() != 1 {
		t.Errorf("expected TypeGrabStalledGaveUp to fire exactly once, got %d", gaveUpFired.Load())
	}
}

// TestTick_AutoSearchTriggersRetry confirms the positive case: a stalled
// auto_search grab DOES trigger TypeAutoSearchRetry when below the circuit
// breaker threshold.
func TestTick_AutoSearchTriggersRetry(t *testing.T) {
	rig := newRig(t)

	rig.mock.grabByInfoHash["deadbeef00"] = db.GrabHistory{
		ID:             "grab-1",
		SeriesID:       "series-1",
		EpisodeID:      sql.NullString{String: "ep-1", Valid: true},
		ReleaseGuid:    "guid-1",
		ReleaseTitle:   "test",
		Protocol:       "torrent",
		DownloadStatus: "queued",
		Source:         "auto_search",
		InfoHash:       sql.NullString{String: "deadbeef00", Valid: true},
	}
	// 0 existing stalls for this episode
	rig.haul.setStalls([]map[string]any{
		{"info_hash": "deadbeef00", "reason": "no_peers_ever", "level": 4},
	})

	var retryFired atomic.Int32
	rig.bus.Subscribe(func(_ context.Context, e events.Event) {
		if e.Type == events.TypeAutoSearchRetry {
			retryFired.Add(1)
		}
	})

	if err := rig.stall.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	if retryFired.Load() != 1 {
		t.Errorf("expected TypeAutoSearchRetry to fire exactly once, got %d", retryFired.Load())
	}
}

// TestTick_InteractiveDoesNotAutoRetry confirms that interactive grabs
// do NOT trigger auto-re-search on stall — the user picked that release
// deliberately, they own the decision.
func TestTick_InteractiveDoesNotAutoRetry(t *testing.T) {
	rig := newRig(t)

	rig.mock.grabByInfoHash["deadbeef00"] = db.GrabHistory{
		ID:             "grab-1",
		SeriesID:       "series-1",
		EpisodeID:      sql.NullString{String: "ep-1", Valid: true},
		ReleaseGuid:    "guid-1",
		ReleaseTitle:   "test",
		Protocol:       "torrent",
		DownloadStatus: "queued",
		Source:         "interactive",
		InfoHash:       sql.NullString{String: "deadbeef00", Valid: true},
	}
	rig.haul.setStalls([]map[string]any{
		{"info_hash": "deadbeef00", "reason": "no_peers_ever", "level": 4},
	})

	var retryFired, stalledFired atomic.Int32
	rig.bus.Subscribe(func(_ context.Context, e events.Event) {
		switch e.Type { //nolint:exhaustive // test only watches two event types
		case events.TypeAutoSearchRetry:
			retryFired.Add(1)
		case events.TypeGrabStalled:
			stalledFired.Add(1)
		}
	})

	if err := rig.stall.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	if retryFired.Load() != 0 {
		t.Errorf("interactive grab auto-retried; should not. got %d retries", retryFired.Load())
	}
	// The grab_stalled event (for UI toast) should still fire.
	if stalledFired.Load() != 1 {
		t.Errorf("expected TypeGrabStalled to fire once for interactive case too, got %d",
			stalledFired.Load())
	}
}

// TestMapStallReason verifies the haul→pilot reason translation.
// Unknown reasons must NOT crash — they fall back to activity_lost.
func TestMapStallReason(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"no_peers_ever", blocklist.ReasonStallNoPeersEver},
		{"no_peers", blocklist.ReasonStallActivityLost},
		{"no_seeders", blocklist.ReasonStallActivityLost},
		{"no_data_received", blocklist.ReasonStallActivityLost},
		{"brand_new_reason_not_in_the_map", blocklist.ReasonStallActivityLost},
		{"", blocklist.ReasonStallActivityLost},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			if got := mapStallReason(c.in); got != c.want {
				t.Errorf("mapStallReason(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestTick_NoHaulConfigured verifies graceful no-op when the user hasn't
// set up a haul download client. Watcher should not error, not log loud,
// and definitely not touch the blocklist.
func TestTick_NoHaulConfigured(t *testing.T) {
	rig := newRig(t)
	// Remove haul from the downloader list entirely.
	rig.stall.downloader = &fakeDownloaderLister{configs: nil}

	if err := rig.stall.Tick(context.Background()); err != nil {
		t.Fatalf("Tick should succeed when no haul configured, got: %v", err)
	}
	if len(rig.mock.blocklistByGUID) != 0 {
		t.Error("blocklist touched with no haul configured — impossible path, check resolveHaulClient")
	}
}

// TestTick_HaulUnreachable verifies graceful failure when Haul is down.
// The error is returned (logged by the caller) but the watcher doesn't
// crash, doesn't blocklist anything, and will retry on the next tick.
func TestTick_HaulUnreachable(t *testing.T) {
	rig := newRig(t)
	// Point at a closed port.
	rig.stall.downloader = &fakeDownloaderLister{
		configs: []downloader.Config{{
			ID:       "haul-1",
			Name:     "Haul",
			Kind:     "haul",
			Enabled:  true,
			Settings: json.RawMessage(`{"url":"http://127.0.0.1:1","api_key":"test"}`),
		}},
	}

	err := rig.stall.Tick(context.Background())
	if err == nil {
		t.Fatal("expected error when Haul is unreachable, got nil")
	}
	if !strings.Contains(err.Error(), "haul list stalled") {
		t.Errorf("error should mention haul list stalled, got: %v", err)
	}
	if len(rig.mock.blocklistByGUID) != 0 {
		t.Error("blocklist touched after Haul failure — should not be")
	}
}

// ── Stale-grab sweep tests ────────────────────────────────────────────
// The sweep handles the case where a grab gets recorded but never has
// its info_hash populated (download client never responded, or the
// grab raced with a haul restart). Those grabs are invisible to the
// haul-stall path (which keys by info_hash) and would sit in
// `downloading` forever without this sweep.

// TestTick_SweepsStuckGrabsWithoutInfoHash — the headline case for
// today's bug: a grab in `downloading` for >24h with no info_hash
// gets marked failed.
func TestTick_SweepsStuckGrabsWithoutInfoHash(t *testing.T) {
	rig := newRig(t)

	// Seed a stale grab — keyed by "" so the sweep query finds it via
	// the "no info_hash" filter.
	staleGrabbed := time.Now().Add(-30 * time.Hour).UTC().Format(time.RFC3339)
	rig.mock.grabByInfoHash[""] = db.GrabHistory{
		ID:             "stale-grab-1",
		SeriesID:       "series-1",
		EpisodeID:      sql.NullString{String: "ep-1", Valid: true},
		ReleaseGuid:    "guid-stale",
		ReleaseTitle:   "Star.Wars.Maul.S01E07.WEB.h264-ETHEL",
		Protocol:       "torrent",
		DownloadStatus: "downloading",
		Source:         "auto_search",
		InfoHash:       sql.NullString{Valid: false}, // explicit no-hash
		GrabbedAt:      staleGrabbed,
	}

	if err := rig.stall.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	// Should have seen exactly one UpdateGrabStatus("failed", ...) for the stale grab.
	if len(rig.mock.updatedStatuses) != 1 {
		t.Fatalf("expected 1 status update, got %d: %+v", len(rig.mock.updatedStatuses), rig.mock.updatedStatuses)
	}
	got := rig.mock.updatedStatuses[0]
	if got.GrabID != "stale-grab-1" {
		t.Errorf("wrong grab updated: got %q want stale-grab-1", got.GrabID)
	}
	if got.DownloadStatus != "failed" {
		t.Errorf("wrong terminal status: got %q want failed", got.DownloadStatus)
	}
	if len(rig.mock.blocklistByGUID) != 0 {
		t.Errorf("sweep should NOT blocklist — no info_hash to ban; got %d entries", len(rig.mock.blocklistByGUID))
	}
}

// TestTick_SweepLeavesFreshGrabsAlone — a grab without info_hash that's
// fresh (within the 24h cutoff) should NOT be touched, since we don't
// want to false-fail genuinely-slow startups (VPN port forwarding,
// remote tracker latency, etc).
func TestTick_SweepLeavesFreshGrabsAlone(t *testing.T) {
	rig := newRig(t)

	freshGrabbed := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	rig.mock.grabByInfoHash[""] = db.GrabHistory{
		ID:             "fresh-grab-1",
		DownloadStatus: "downloading",
		InfoHash:       sql.NullString{Valid: false},
		GrabbedAt:      freshGrabbed,
	}

	if err := rig.stall.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(rig.mock.updatedStatuses) != 0 {
		t.Errorf("sweep touched a fresh grab: %+v", rig.mock.updatedStatuses)
	}
}

// TestTick_SweepIgnoresGrabsWithInfoHash — grabs that DO have an
// info_hash are handled by the haul-stall path (they show up in
// /api/v1/stalls and get classified there). The sweep mustn't double-
// dip on them or there'd be two writers fighting over the same row.
func TestTick_SweepIgnoresGrabsWithInfoHash(t *testing.T) {
	rig := newRig(t)

	staleGrabbed := time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339)
	rig.mock.grabByInfoHash["abcd1234"] = db.GrabHistory{
		ID:             "with-hash-grab-1",
		DownloadStatus: "downloading",
		InfoHash:       sql.NullString{String: "abcd1234", Valid: true},
		GrabbedAt:      staleGrabbed,
	}

	if err := rig.stall.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	for _, u := range rig.mock.updatedStatuses {
		if u.GrabID == "with-hash-grab-1" {
			t.Errorf("sweep updated a grab that has an info_hash — that grab should be left for the haul-stall path: %+v", u)
		}
	}
}

// TestTick_SweepRespectsStartupGrace — the sweep mustn't fire during
// the 2-minute startup grace, same as the haul-stall path. A pilot
// restart with grabs in flight should NOT see them all expired before
// haul has a chance to report on them.
func TestTick_SweepRespectsStartupGrace(t *testing.T) {
	rig := newRig(t)
	rig.stall.startedAt = time.Now().Add(-30 * time.Second) // inside grace

	staleGrabbed := time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339)
	rig.mock.grabByInfoHash[""] = db.GrabHistory{
		ID:             "would-be-stale",
		DownloadStatus: "downloading",
		InfoHash:       sql.NullString{Valid: false},
		GrabbedAt:      staleGrabbed,
	}

	if err := rig.stall.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(rig.mock.updatedStatuses) != 0 {
		t.Errorf("sweep ran during startup grace: %+v", rig.mock.updatedStatuses)
	}
}

// Type assertion: mockQuerier must satisfy db.Querier at compile time.
// If this fails to compile, a new method was added to the querier and
// we've shadowed something. Fix: regenerate sqlc, ensure the embedding
// still works, possibly stub the new method if tests panic at runtime.
var _ db.Querier = (*mockQuerier)(nil)

// Compile-time check: blocklist.NewService must accept db.Querier.
var _ = func() *blocklist.Service { return blocklist.NewService(nil) }

// Suppress "imported and not used" errors from the parent package test
// guard pattern when the file is built without the mock querier.
var _ = errors.New
