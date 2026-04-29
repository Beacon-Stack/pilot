package activity

import (
	"testing"

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
