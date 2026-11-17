package failedgrab

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/beacon-stack/pilot/internal/core/blocklist"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/events"
)

type fakeBlocklist struct {
	added     []blocklist.StallEntry
	recent    int64
	addErr    error
	countArgs [][2]string
}

func (f *fakeBlocklist) AddFromDownloadFailure(_ context.Context, p blocklist.StallEntry) error {
	f.added = append(f.added, p)
	return f.addErr
}

func (f *fakeBlocklist) CountRecentStalls(_ context.Context, seriesID, episodeID string) (int64, error) {
	f.countArgs = append(f.countArgs, [2]string{seriesID, episodeID})
	return f.recent, nil
}

type fakeBus struct {
	mu     sync.Mutex
	events []events.Event
}

func (b *fakeBus) Publish(_ context.Context, e events.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, e)
}

func (b *fakeBus) types() []events.Type {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]events.Type, len(b.events))
	for i, e := range b.events {
		out[i] = e.Type
	}
	return out
}

func (b *fakeBus) find(t events.Type) (events.Event, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, e := range b.events {
		if e.Type == t {
			return e, true
		}
	}
	return events.Event{}, false
}

func ptr[T any](v T) *T { return &v }

func failedGrab(source string) db.GrabHistory {
	return db.GrabHistory{
		ID:           "grab-1",
		SeriesID:     "series-1",
		EpisodeID:    ptr("ep-1"),
		SeasonNumber: ptr(int64(2)),
		IndexerID:    ptr("idx-1"),
		ReleaseGuid:  "guid-1",
		ReleaseTitle: "Show.S02E05.1080p.WEB-DL",
		Protocol:     "usenet",
		Size:         1234,
		Source:       source,
		InfoHash:     ptr("abc123"),
	}
}

func newHandler(bl *fakeBlocklist, bus *fakeBus) *Handler {
	return New(bl, bus, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// Headline: an automatic grab that fails is blocklisted and re-searched.
func TestAutoGrabFailure_BlocklistsAndRequestsReSearch(t *testing.T) {
	bl, bus := &fakeBlocklist{recent: 1}, &fakeBus{}
	if err := newHandler(bl, bus).HandleDownloadFailed(context.Background(), failedGrab("auto_search"), "missing articles"); err != nil {
		t.Fatal(err)
	}

	if len(bl.added) != 1 {
		t.Fatalf("blocklist entries = %d, want 1", len(bl.added))
	}
	got := bl.added[0]
	if got.ReleaseGUID != "guid-1" || got.InfoHash != "abc123" || got.EpisodeID != "ep-1" || got.IndexerID != "idx-1" {
		t.Fatalf("blocklist entry missing identifiers: %+v", got)
	}

	retry, ok := bus.find(events.TypeAutoSearchRetry)
	if !ok {
		t.Fatalf("no re-search requested; events = %v", bus.types())
	}
	if retry.Data["episode_id"] != "ep-1" || retry.Data["season"] != int64(2) || retry.Data["retry_count"] != int64(2) {
		t.Fatalf("retry event data = %+v", retry.Data)
	}
	failed, ok := bus.find(events.TypeGrabFailed)
	if !ok || failed.Data["reason"] != "missing articles" {
		t.Fatalf("grab_failed event missing or wrong: %+v", failed.Data)
	}
}

// Interactive grabs: blocklist, but the user decides whether to retry.
func TestInteractiveGrabFailure_DoesNotReSearch(t *testing.T) {
	bl, bus := &fakeBlocklist{}, &fakeBus{}
	if err := newHandler(bl, bus).HandleDownloadFailed(context.Background(), failedGrab("interactive"), ""); err != nil {
		t.Fatal(err)
	}
	if len(bl.added) != 1 {
		t.Fatal("interactive failure must still be blocklisted")
	}
	if _, ok := bus.find(events.TypeAutoSearchRetry); ok {
		t.Fatal("interactive failure must not trigger an automatic re-search")
	}
	if len(bl.countArgs) != 0 {
		t.Fatal("circuit breaker should not be consulted for interactive grabs")
	}
}

// Circuit breaker: stop re-searching once the episode has burned its budget.
func TestCircuitBreaker_StopsReSearch(t *testing.T) {
	bl, bus := &fakeBlocklist{recent: MaxRetriesPerEpisode}, &fakeBus{}
	if err := newHandler(bl, bus).HandleDownloadFailed(context.Background(), failedGrab("auto_search"), ""); err != nil {
		t.Fatal(err)
	}
	if _, ok := bus.find(events.TypeAutoSearchRetry); ok {
		t.Fatal("re-search requested past the circuit breaker")
	}
	if _, ok := bus.find(events.TypeGrabStalledGaveUp); !ok {
		t.Fatalf("expected a gave-up event; events = %v", bus.types())
	}
}

// A second report of the same failure must not be treated as an error.
func TestAlreadyBlocklisted_IsSuccess(t *testing.T) {
	bl, bus := &fakeBlocklist{addErr: blocklist.ErrAlreadyBlocklisted}, &fakeBus{}
	if err := newHandler(bl, bus).HandleDownloadFailed(context.Background(), failedGrab("auto_search"), ""); err != nil {
		t.Fatalf("ErrAlreadyBlocklisted should be swallowed, got %v", err)
	}
}

// Season-pack grabs have no episode; the retry must carry the season.
func TestSeasonPackFailure_RetryCarriesSeason(t *testing.T) {
	g := failedGrab("auto_search")
	g.EpisodeID = nil
	bl, bus := &fakeBlocklist{}, &fakeBus{}
	if err := newHandler(bl, bus).HandleDownloadFailed(context.Background(), g, ""); err != nil {
		t.Fatal(err)
	}
	retry, ok := bus.find(events.TypeAutoSearchRetry)
	if !ok || retry.Data["episode_id"] != "" || retry.Data["season"] != int64(2) {
		t.Fatalf("season-pack retry data = %+v", retry.Data)
	}
}
