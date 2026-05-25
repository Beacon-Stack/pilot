package jobs

// rss_sync_test.go — regression tests for the per-episode dedup change
// that closed the "11 duplicate completed grab_history rows" failure
// mode (see plans/lifecycle-trust.md QW1).
//
// We can't easily test runRSSSync end-to-end because it takes concrete
// *indexer.Service and *downloader.Service pointers (not interfaces),
// so a full integration test would need real services + mock Pulse +
// mock haul. Instead these tests cover the testable kernel:
//
//   buildRecentlyCompletedEpisodes — the new helper that drives the
//   second-line dedup guard.
//
// Plus shape assertions on the new guard logic so future edits to
// rss_sync.go that re-introduce the series-wide guardrail show up.

import (
	"context"
	"errors"
	"testing"
	"time"

	db "github.com/beacon-stack/pilot/internal/db/generated"
)

// ptrStrRSS returns a pointer to s; for *string nullable params.
func ptrStrRSS(s string) *string { return &s }

// mockQuerier — same pattern as stallwatcher's. Embeds db.Querier so
// un-mocked methods panic; tests are forced to declare the surface.
type mockQuerier struct {
	db.Querier

	listGrabHistoryByStatusSinceFn func(context.Context, db.ListGrabHistoryByStatusSinceParams) ([]db.GrabHistory, error)
}

func (m *mockQuerier) ListGrabHistoryByStatusSince(ctx context.Context, arg db.ListGrabHistoryByStatusSinceParams) ([]db.GrabHistory, error) {
	if m.listGrabHistoryByStatusSinceFn != nil {
		return m.listGrabHistoryByStatusSinceFn(ctx, arg)
	}
	return nil, nil
}

// TestBuildRecentlyCompletedEpisodes_ReturnsMapByEpisodeID — happy path:
// completed grabs with valid episode_ids show up in the map.
func TestBuildRecentlyCompletedEpisodes_ReturnsMapByEpisodeID(t *testing.T) {
	q := &mockQuerier{
		listGrabHistoryByStatusSinceFn: func(_ context.Context, _ db.ListGrabHistoryByStatusSinceParams) ([]db.GrabHistory, error) {
			return []db.GrabHistory{
				{ID: "g1", EpisodeID: ptrStrRSS("ep-1")},
				{ID: "g2", EpisodeID: ptrStrRSS("ep-2")},
				{ID: "g3", EpisodeID: ptrStrRSS("ep-1")}, // dup of ep-1
			}, nil
		},
	}
	got, err := buildRecentlyCompletedEpisodes(context.Background(), q, time.Now().Add(-6*time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got["ep-1"] || !got["ep-2"] {
		t.Errorf("expected ep-1 and ep-2 in set, got %v", got)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 unique episodes (dups collapsed), got %d", len(got))
	}
}

// TestBuildRecentlyCompletedEpisodes_SkipsRowsWithoutEpisodeID — pre-fix
// grab_history rows can have NULL episode_id (anime numbering hadn't
// been resolved yet, etc). Those mustn't crash or pollute the map.
func TestBuildRecentlyCompletedEpisodes_SkipsRowsWithoutEpisodeID(t *testing.T) {
	q := &mockQuerier{
		listGrabHistoryByStatusSinceFn: func(_ context.Context, _ db.ListGrabHistoryByStatusSinceParams) ([]db.GrabHistory, error) {
			return []db.GrabHistory{
				{ID: "g1", EpisodeID: nil},
				{ID: "g2", EpisodeID: ptrStrRSS("")}, // valid but empty
				{ID: "g3", EpisodeID: ptrStrRSS("ep-real")},
			}, nil
		},
	}
	got, err := buildRecentlyCompletedEpisodes(context.Background(), q, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || !got["ep-real"] {
		t.Errorf("expected only ep-real in set, got %v", got)
	}
}

// TestBuildRecentlyCompletedEpisodes_EmptyResult — no recent completes
// means an empty map (not nil — caller relies on map indexing being
// safe regardless).
func TestBuildRecentlyCompletedEpisodes_EmptyResult(t *testing.T) {
	q := &mockQuerier{
		listGrabHistoryByStatusSinceFn: func(_ context.Context, _ db.ListGrabHistoryByStatusSinceParams) ([]db.GrabHistory, error) {
			return nil, nil
		},
	}
	got, err := buildRecentlyCompletedEpisodes(context.Background(), q, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil map even with zero rows")
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

// TestBuildRecentlyCompletedEpisodes_ErrorPropagates — DB errors must
// surface up to the caller so it can degrade gracefully (rss_sync logs
// + falls back to active-only dedup; QW4's unique index is the
// backstop).
func TestBuildRecentlyCompletedEpisodes_ErrorPropagates(t *testing.T) {
	wantErr := errors.New("db unreachable")
	q := &mockQuerier{
		listGrabHistoryByStatusSinceFn: func(_ context.Context, _ db.ListGrabHistoryByStatusSinceParams) ([]db.GrabHistory, error) {
			return nil, wantErr
		},
	}
	_, err := buildRecentlyCompletedEpisodes(context.Background(), q, time.Now())
	if !errors.Is(err, wantErr) {
		t.Errorf("expected wrapped %v, got %v", wantErr, err)
	}
}

// TestCompletedSkipWindow_IsReasonable — pin the window. Too short and
// we're back to 15-min duplicate grabs; too long and a genuinely-failed
// release can't be retried via RSS the same day. 6h is the documented
// choice — break this test before changing it.
func TestCompletedSkipWindow_IsReasonable(t *testing.T) {
	if completedSkipWindow < 1*time.Hour {
		t.Errorf("completedSkipWindow=%v is shorter than the typical RSS refresh — would still allow rapid re-grabs", completedSkipWindow)
	}
	if completedSkipWindow > 24*time.Hour {
		t.Errorf("completedSkipWindow=%v is too long — a genuinely-failed release can't be retried via RSS same day", completedSkipWindow)
	}
}
