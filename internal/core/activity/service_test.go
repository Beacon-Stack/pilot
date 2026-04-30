package activity

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/events"
)

// TestClassify_CategoryEnum locks the event-type → category mapping
// so a refactor of an emit site doesn't silently move an event into
// a different bucket and break the Activity-page rails. Add a row
// here when adding a new event type that should be persisted.
func TestClassify_CategoryEnum(t *testing.T) {
	svc := &Service{}

	tests := []struct {
		name    string
		evtType events.Type
		data    map[string]any
		want    Category
	}{
		{"grab succeeded", events.TypeEpisodeGrabbed, map[string]any{"title": "Some.S01E01"}, CategoryGrabSucceeded},
		{"grab failed", events.TypeGrabFailed, map[string]any{"release_title": "Bad.Release", "reason": "404"}, CategoryGrabFailed},
		{"stalled", events.TypeGrabStalled, map[string]any{"release_title": "Stuck", "reason": "no_peers_ever"}, CategoryStalled},
		{"stall gave up", events.TypeGrabStalledGaveUp, map[string]any{}, CategoryGrabFailed},
		{"download done", events.TypeDownloadDone, map[string]any{"title": "Done"}, CategoryImportSucceeded},
		{"import complete", events.TypeImportComplete, map[string]any{"series_title": "Show"}, CategoryImportSucceeded},
		{"import failed", events.TypeImportFailed, map[string]any{"error": "permission denied"}, CategoryImportFailed},
		{"show added", events.TypeShowAdded, map[string]any{"title": "Show"}, CategoryShow},
		{"show deleted", events.TypeShowDeleted, map[string]any{"title": "Show"}, CategoryShow},
		{"show updated", events.TypeShowUpdated, map[string]any{"title": "Show"}, CategoryShow},
		{"task started", events.TypeTaskStarted, map[string]any{"task": "rss_sync"}, CategoryTask},
		{"task finished", events.TypeTaskFinished, map[string]any{"task": "rss_sync"}, CategoryTask},
		{"health issue", events.TypeHealthIssue, map[string]any{"check": "disk_space", "message": "low"}, CategoryHealth},
		{"health ok", events.TypeHealthOK, map[string]any{"check": "disk_space"}, CategoryHealth},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cat, title := svc.classify(events.Event{Type: tt.evtType, Data: tt.data})
			if cat != tt.want {
				t.Errorf("classify(%q) category = %q, want %q", tt.evtType, cat, tt.want)
			}
			if title == "" {
				t.Errorf("classify(%q) returned empty title", tt.evtType)
			}
		})
	}
}

// TestClassify_UnknownEventReturnsEmpty ensures an unrecognised event
// type drops cleanly (handleEvent skips on empty category) rather than
// leaking an empty-category row into activity_log.
func TestClassify_UnknownEventReturnsEmpty(t *testing.T) {
	svc := &Service{}
	cat, title := svc.classify(events.Event{Type: "made_up_event"})
	if cat != "" || title != "" {
		t.Errorf("classify(unknown) = (%q, %q), want empty/empty", cat, title)
	}
}

// ── handleEvent ──────────────────────────────────────────────────────────────
//
// handleEvent is the bridge between the in-memory event bus and the
// persistent activity_log table. If it silently drops events or
// produces malformed rows, the user never sees their grab/stall
// history. classify is tested above; these tests pin the surrounding
// plumbing.

// activityRecorder embeds db.Querier so only InsertActivity needs a
// real implementation. Records every call so tests can assert on
// what handleEvent persisted.
type activityRecorder struct {
	db.Querier

	mu        sync.Mutex
	inserted  []db.InsertActivityParams
	insertErr error
}

func (r *activityRecorder) InsertActivity(_ context.Context, p db.InsertActivityParams) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inserted = append(r.inserted, p)
	return r.insertErr
}

func (r *activityRecorder) calls() []db.InsertActivityParams {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]db.InsertActivityParams, len(r.inserted))
	copy(out, r.inserted)
	return out
}

// Headline: a known event type (TypeEpisodeGrabbed) results in exactly
// one InsertActivity call with the classified category + title. Pin
// the contract so a regression that replaces the call site with a
// stub never silently disables activity logging.
func TestHandleEvent_KnownEventInsertsRow(t *testing.T) {
	rec := &activityRecorder{}
	svc := NewService(rec, slog.New(slog.DiscardHandler))

	svc.handleEvent(context.Background(), events.Event{
		Type:      events.TypeEpisodeGrabbed,
		ShowID:    "series-42",
		Timestamp: time.Date(2026, 1, 8, 10, 30, 0, 0, time.UTC),
		Data: map[string]any{
			"release_title": "Show.S01E01.1080p",
			"indexer":       "TGx",
		},
	})

	calls := rec.calls()
	if len(calls) != 1 {
		t.Fatalf("len(calls) = %d, want 1", len(calls))
	}
	c := calls[0]
	if c.Category != string(CategoryGrabSucceeded) {
		t.Errorf("Category = %q, want %q", c.Category, CategoryGrabSucceeded)
	}
	if c.Type != string(events.TypeEpisodeGrabbed) {
		t.Errorf("Type = %q, want %q", c.Type, events.TypeEpisodeGrabbed)
	}
	if !strings.Contains(c.Title, "Show.S01E01.1080p") {
		t.Errorf("Title = %q, want it to mention the release", c.Title)
	}
	if !c.SeriesID.Valid || c.SeriesID.String != "series-42" {
		t.Errorf("SeriesID = %+v, want valid string 'series-42'", c.SeriesID)
	}
	// Detail should be the JSON-encoded event data.
	if !c.Detail.Valid || !strings.Contains(c.Detail.String, "TGx") {
		t.Errorf("Detail = %+v, want JSON containing 'TGx'", c.Detail)
	}
	if c.CreatedAt != "2026-01-08T10:30:00Z" {
		t.Errorf("CreatedAt = %q, want RFC3339 UTC", c.CreatedAt)
	}
	if c.ID == "" {
		t.Errorf("ID is empty; expected uuid")
	}
}

// Unknown event types must NOT insert anything. Otherwise every
// internal/diagnostic event leaks into the user-facing activity feed.
func TestHandleEvent_UnknownEventDoesNotInsert(t *testing.T) {
	rec := &activityRecorder{}
	svc := NewService(rec, slog.New(slog.DiscardHandler))

	svc.handleEvent(context.Background(), events.Event{
		Type: "this_event_is_not_classified",
		Data: map[string]any{"x": "y"},
	})

	if got := len(rec.calls()); got != 0 {
		t.Errorf("expected 0 inserts for unknown event; got %d (%+v)", got, rec.calls())
	}
}

// Empty ShowID → SeriesID stays !Valid (NULL). Required for events
// that aren't series-scoped (TypeTaskStarted, TypeHealthIssue, etc.)
// — a Valid="" would violate the foreign-key on series.id.
func TestHandleEvent_NoShowIDProducesNullSeriesID(t *testing.T) {
	rec := &activityRecorder{}
	svc := NewService(rec, slog.New(slog.DiscardHandler))

	svc.handleEvent(context.Background(), events.Event{
		Type:      events.TypeTaskStarted,
		Timestamp: time.Now().UTC(),
		Data:      map[string]any{"task": "rss_sync"},
	})

	calls := rec.calls()
	if len(calls) != 1 {
		t.Fatalf("len(calls) = %d, want 1", len(calls))
	}
	if calls[0].SeriesID.Valid {
		t.Errorf("SeriesID.Valid = true for non-series event; want false (would FK-violate)")
	}
}

// DB error from InsertActivity must NOT crash the bus — handleEvent
// is invoked via bus.Subscribe and a panic would propagate. Just log
// and continue.
func TestHandleEvent_DBErrorIsSwallowed(t *testing.T) {
	rec := &activityRecorder{insertErr: errors.New("db unavailable")}
	svc := NewService(rec, slog.New(slog.DiscardHandler))

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("handleEvent panicked on DB error: %v", r)
		}
	}()
	svc.handleEvent(context.Background(), events.Event{
		Type:      events.TypeEpisodeGrabbed,
		Timestamp: time.Now().UTC(),
		Data:      map[string]any{"title": "x"},
	})
}

// TestValidCategory pins the closed-set of accepted categories AND the
// legacy back-compat values. Removing one without a corresponding
// migration breaks the API for clients filtering on the old name.
func TestValidCategory(t *testing.T) {
	valid := []string{
		"grab_succeeded", "grab_failed",
		"import_succeeded", "import_failed",
		"stalled", "show", "task", "health",
		"grab", "import", // legacy
	}
	for _, c := range valid {
		if !ValidCategory(c) {
			t.Errorf("ValidCategory(%q) = false, want true", c)
		}
	}

	if ValidCategory("nope") {
		t.Error("ValidCategory(nope) = true, want false")
	}
	if ValidCategory("") {
		t.Error("ValidCategory(empty) = true, want false")
	}
}
