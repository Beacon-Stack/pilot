package v1

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/beacon-stack/pilot/internal/events"
)

type retryRecorder struct {
	mu    sync.Mutex
	calls []autoSearchParams
	gate  chan struct{} // when set, search blocks until closed
}

func (r *retryRecorder) search(_ context.Context, p autoSearchParams) (*autoSearchResultBody, error) {
	if r.gate != nil {
		<-r.gate
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, p)
	return &autoSearchResultBody{Result: "grabbed"}, nil
}

func newRetrier(rec *retryRecorder) *autoSearchRetrier {
	return &autoSearchRetrier{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		lookupEpisode: func(_ context.Context, id string) (int, int, error) {
			if id == "ep-5" {
				return 2, 5, nil
			}
			return 0, 0, errors.New("not found")
		},
		search: rec.search,
	}
}

func retryEvent(data map[string]any) events.Event {
	return events.Event{Type: events.TypeAutoSearchRetry, Data: data}
}

// Headline: TypeAutoSearchRetry used to have no subscriber, so dead
// automatic grabs were blocklisted and never replaced.
func TestAutoSearchRetry_SearchesTheEpisode(t *testing.T) {
	rec := &retryRecorder{}
	newRetrier(rec).handle(context.Background(), retryEvent(map[string]any{
		"series_id": "series-1", "episode_id": "ep-5", "retry_count": int64(1),
	}))
	if len(rec.calls) != 1 {
		t.Fatalf("search calls = %d, want 1", len(rec.calls))
	}
	got := rec.calls[0]
	if got.SeriesID != "series-1" || got.Season != 2 || got.Episode != 5 || got.EpisodeID != "ep-5" || got.Source != "auto_search" {
		t.Fatalf("search params = %+v", got)
	}
}

func TestAutoSearchRetry_SeasonPack(t *testing.T) {
	rec := &retryRecorder{}
	newRetrier(rec).handle(context.Background(), retryEvent(map[string]any{
		"series_id": "series-1", "episode_id": "", "season": int64(3),
	}))
	if len(rec.calls) != 1 || rec.calls[0].Season != 3 || rec.calls[0].Episode != 0 {
		t.Fatalf("season-pack search = %+v", rec.calls)
	}
}

func TestAutoSearchRetry_SkipsWhenNothingToSearch(t *testing.T) {
	rec := &retryRecorder{}
	r := newRetrier(rec)
	r.handle(context.Background(), retryEvent(map[string]any{"series_id": "series-1"}))                       // no episode, no season
	r.handle(context.Background(), retryEvent(map[string]any{"series_id": "series-1", "episode_id": "ep-x"})) // unknown episode
	r.handle(context.Background(), retryEvent(map[string]any{"episode_id": "ep-5"}))                          // no series
	r.handle(context.Background(), events.Event{Type: events.TypeGrabFailed, Data: map[string]any{"series_id": "s"}})
	if len(rec.calls) != 0 {
		t.Fatalf("unexpected searches: %+v", rec.calls)
	}
}

// Two retry events for the same episode at once must run one search.
func TestAutoSearchRetry_DropsDuplicateInFlight(t *testing.T) {
	rec := &retryRecorder{gate: make(chan struct{})}
	r := newRetrier(rec)
	ev := retryEvent(map[string]any{"series_id": "series-1", "episode_id": "ep-5"})

	done := make(chan struct{})
	go func() { r.handle(context.Background(), ev); close(done) }()
	// Wait until the first call has claimed the in-flight slot.
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, ok := r.inflight.Load("series-1/2/5"); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("first retry never started")
		}
		time.Sleep(time.Millisecond)
	}
	r.handle(context.Background(), ev) // duplicate: returns immediately
	close(rec.gate)
	<-done

	if len(rec.calls) != 1 {
		t.Fatalf("search calls = %d, want 1", len(rec.calls))
	}
}
