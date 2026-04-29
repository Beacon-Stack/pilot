// Package activity provides a persistent activity log that records events
// from the in-memory event bus so they survive restarts and are queryable.
package activity

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/events"
)

// Activity is the domain representation of an activity log entry.
type Activity struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Category  string         `json:"category"`
	SeriesID  *string        `json:"series_id,omitempty"`
	Title     string         `json:"title"`
	Detail    map[string]any `json:"detail,omitempty"`
	CreatedAt string         `json:"created_at"`
}

// ListResult is the paginated response from List.
type ListResult struct {
	Activities []Activity `json:"activities"`
	Total      int64      `json:"total"`
}

// Service records and queries activity log entries.
type Service struct {
	q      db.Querier
	logger *slog.Logger
}

// NewService creates a new activity service.
func NewService(q db.Querier, logger *slog.Logger) *Service {
	return &Service{q: q, logger: logger}
}

// Subscribe registers the service as an event bus handler. Call this once
// during startup after constructing the service.
func (s *Service) Subscribe(bus *events.Bus) {
	bus.Subscribe(s.handleEvent)
}

// handleEvent converts an event bus event into a persistent activity record.
func (s *Service) handleEvent(ctx context.Context, e events.Event) {
	cat, title := s.classify(e)
	if cat == "" {
		return // unknown event type — skip
	}

	var detailStr *string
	if len(e.Data) > 0 {
		b, err := json.Marshal(e.Data)
		if err == nil {
			str := string(b)
			detailStr = &str
		}
	}

	seriesID := sql.NullString{}
	if e.ShowID != "" {
		seriesID = sql.NullString{String: e.ShowID, Valid: true}
	}

	detailNull := sql.NullString{}
	if detailStr != nil {
		detailNull = sql.NullString{String: *detailStr, Valid: true}
	}

	err := s.q.InsertActivity(ctx, db.InsertActivityParams{
		ID:        uuid.New().String(),
		Type:      string(e.Type),
		Category:  string(cat),
		SeriesID:  seriesID,
		Title:     title,
		Detail:    detailNull,
		CreatedAt: e.Timestamp.UTC().Format(time.RFC3339),
	})
	if err != nil {
		s.logger.Warn("failed to record activity", "type", e.Type, "error", err)
	}
}

// classify maps an event type to a category and human-readable title.
func (s *Service) classify(e events.Event) (Category, string) {
	data := e.Data
	str := func(key string) string {
		if v, ok := data[key].(string); ok {
			return v
		}
		return ""
	}

	switch e.Type {
	case events.TypeEpisodeGrabbed:
		// indexer.Service publishes only "title"; release_title/indexer
		// are accepted too in case a future emit site adds them.
		release := str("release_title")
		if release == "" {
			release = str("title")
		}
		indexer := str("indexer")
		if indexer != "" {
			return CategoryGrabSucceeded, fmt.Sprintf("Grabbed %s from %s", release, indexer)
		}
		return CategoryGrabSucceeded, fmt.Sprintf("Grabbed %s", release)

	case events.TypeGrabFailed:
		release := str("release_title")
		reason := str("reason")
		if reason != "" {
			return CategoryGrabFailed, fmt.Sprintf("Grab failed for %s: %s", release, reason)
		}
		return CategoryGrabFailed, fmt.Sprintf("Grab failed for %s", release)

	case events.TypeGrabStalled:
		release := str("release_title")
		reason := str("reason")
		if reason != "" {
			return CategoryStalled, fmt.Sprintf("Stalled: %s (%s)", release, reason)
		}
		return CategoryStalled, fmt.Sprintf("Stalled: %s", release)

	case events.TypeGrabStalledGaveUp:
		// Circuit breaker: stallwatcher gave up after MaxStallRetriesPerEpisode.
		// File this under grab_failed so the "Needs attention" rail surfaces it.
		return CategoryGrabFailed, "Auto-search gave up after repeated stalls"

	case events.TypeDownloadDone:
		release := str("title")
		if release == "" {
			release = str("release_title")
		}
		return CategoryImportSucceeded, fmt.Sprintf("Download complete: %s", release)

	case events.TypeImportComplete:
		title := str("series_title")
		quality := str("quality")
		if title == "" {
			// Importer publishes grab_id + content_path, not series_title;
			// fall back to a generic message rather than leaving an empty title.
			title = "release"
		}
		if quality != "" {
			return CategoryImportSucceeded, fmt.Sprintf("Imported %s — %s", title, quality)
		}
		return CategoryImportSucceeded, fmt.Sprintf("Imported %s", title)

	case events.TypeImportFailed:
		title := str("series_title")
		reason := str("error")
		if reason == "" {
			reason = str("reason")
		}
		if title == "" {
			title = "release"
		}
		if reason != "" {
			return CategoryImportFailed, fmt.Sprintf("Import failed for %s: %s", title, reason)
		}
		return CategoryImportFailed, fmt.Sprintf("Import failed for %s", title)

	case events.TypeShowAdded:
		title := str("title")
		return CategoryShow, fmt.Sprintf("Added %s to library", title)

	case events.TypeShowDeleted:
		title := str("title")
		return CategoryShow, fmt.Sprintf("Deleted %s", title)

	case events.TypeShowUpdated:
		title := str("title")
		return CategoryShow, fmt.Sprintf("Updated %s", title)

	case events.TypeTaskStarted:
		task := str("task")
		return CategoryTask, fmt.Sprintf("%s started", task)

	case events.TypeTaskFinished:
		task := str("task")
		return CategoryTask, fmt.Sprintf("%s completed", task)

	case events.TypeHealthIssue:
		check := str("check")
		message := str("message")
		return CategoryHealth, fmt.Sprintf("%s: %s", check, message)

	case events.TypeHealthOK:
		check := str("check")
		return CategoryHealth, fmt.Sprintf("%s: recovered", check)

	default:
		return "", ""
	}
}

// List returns activity entries matching the given filters.
func (s *Service) List(ctx context.Context, category *string, since *string, limit int64) (*ListResult, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	catFilter := sql.NullString{}
	if category != nil && *category != "" {
		catFilter = sql.NullString{String: *category, Valid: true}
	}
	sinceFilter := sql.NullString{}
	if since != nil && *since != "" {
		sinceFilter = sql.NullString{String: *since, Valid: true}
	}

	rows, err := s.q.ListActivities(ctx, db.ListActivitiesParams{
		Category: catFilter,
		Since:    sinceFilter,
		Limit:    int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("listing activities: %w", err)
	}

	total, err := s.q.CountActivities(ctx, db.CountActivitiesParams{
		Category: catFilter,
		Since:    sinceFilter,
	})
	if err != nil {
		return nil, fmt.Errorf("counting activities: %w", err)
	}

	activities := make([]Activity, 0, len(rows))
	for _, r := range rows {
		var sid *string
		if r.SeriesID.Valid {
			sid = &r.SeriesID.String
		}
		a := Activity{
			ID:        r.ID,
			Type:      r.Type,
			Category:  r.Category,
			SeriesID:  sid,
			Title:     r.Title,
			CreatedAt: r.CreatedAt,
		}
		if r.Detail.Valid {
			_ = json.Unmarshal([]byte(r.Detail.String), &a.Detail)
		}
		activities = append(activities, a)
	}

	return &ListResult{
		Activities: activities,
		Total:      total,
	}, nil
}

// Prune deletes activity entries older than the given duration.
func (s *Service) Prune(ctx context.Context, olderThan time.Duration) error {
	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339)
	return s.q.PruneActivities(ctx, cutoff)
}

// AttentionItem is a single row in the "Needs attention" rail.
type AttentionItem struct {
	// Kind is one of "grab_failed", "import_failed", "stalled".
	Kind string `json:"kind"`
	// GrabID is set for grab/stall items; empty for import-only failures.
	GrabID       string `json:"grab_id,omitempty"`
	SeriesID     string `json:"series_id,omitempty"`
	EpisodeID    string `json:"episode_id,omitempty"`
	ReleaseTitle string `json:"release_title"`
	Detail       string `json:"detail,omitempty"`
	// InfoHash lets the UI deep-link "open in Haul" for stalled items.
	InfoHash  string `json:"info_hash,omitempty"`
	CreatedAt string `json:"created_at"`
}

// AttentionResult is the response shape for /api/v1/activity/needs-attention.
type AttentionResult struct {
	Items []AttentionItem `json:"items"`
	// Counts breaks the items down by kind so the UI can show a summary
	// without re-counting client-side.
	Counts struct {
		GrabFailed   int `json:"grab_failed"`
		ImportFailed int `json:"import_failed"`
		Stalled      int `json:"stalled"`
	} `json:"counts"`
}

// NeedsAttention returns recent failures and stalls for the Activity-page
// "Needs attention" rail. window controls how far back to look; perKind
// caps each bucket so a flood of one type doesn't drown out the others.
func (s *Service) NeedsAttention(ctx context.Context, window time.Duration, perKind int) (*AttentionResult, error) {
	if window <= 0 {
		window = 48 * time.Hour
	}
	if perKind <= 0 || perKind > 200 {
		perKind = 50
	}
	since := time.Now().UTC().Add(-window).Format(time.RFC3339)

	out := &AttentionResult{Items: []AttentionItem{}}

	// Failed grabs: grab_history rows with download_status in ('failed','removed').
	for _, status := range []string{"failed", "removed"} {
		rows, err := s.q.ListGrabHistoryByStatusSince(ctx, db.ListGrabHistoryByStatusSinceParams{
			Status: status,
			Since:  since,
			Limit:  int32(perKind),
		})
		if err != nil {
			return nil, fmt.Errorf("listing grab history (%s): %w", status, err)
		}
		for _, r := range rows {
			out.Items = append(out.Items, AttentionItem{
				Kind:         "grab_failed",
				GrabID:       r.ID,
				SeriesID:     r.SeriesID,
				EpisodeID:    r.EpisodeID.String,
				ReleaseTitle: r.ReleaseTitle,
				Detail:       fmt.Sprintf("Download %s", r.DownloadStatus),
				InfoHash:     r.InfoHash.String,
				CreatedAt:    r.GrabbedAt,
			})
			out.Counts.GrabFailed++
		}
	}

	// Stalled grabs: download_status = 'stalled'.
	stalled, err := s.q.ListGrabHistoryByStatusSince(ctx, db.ListGrabHistoryByStatusSinceParams{
		Status: "stalled",
		Since:  since,
		Limit:  int32(perKind),
	})
	if err != nil {
		return nil, fmt.Errorf("listing stalled grabs: %w", err)
	}
	for _, r := range stalled {
		out.Items = append(out.Items, AttentionItem{
			Kind:         "stalled",
			GrabID:       r.ID,
			SeriesID:     r.SeriesID,
			EpisodeID:    r.EpisodeID.String,
			ReleaseTitle: r.ReleaseTitle,
			Detail:       "Auto-blocklisted by stall watcher",
			InfoHash:     r.InfoHash.String,
			CreatedAt:    r.GrabbedAt,
		})
		out.Counts.Stalled++
	}

	// Import failures: activity_log category=import_failed within window.
	cat := sql.NullString{String: string(CategoryImportFailed), Valid: true}
	sinceNS := sql.NullString{String: since, Valid: true}
	imports, err := s.q.ListActivities(ctx, db.ListActivitiesParams{
		Category: cat,
		Since:    sinceNS,
		Limit:    int32(perKind),
	})
	if err != nil {
		return nil, fmt.Errorf("listing import failures: %w", err)
	}
	for _, r := range imports {
		var detail string
		if r.Detail.Valid {
			var d map[string]any
			if json.Unmarshal([]byte(r.Detail.String), &d) == nil {
				if v, ok := d["error"].(string); ok {
					detail = v
				}
			}
		}
		seriesID := ""
		if r.SeriesID.Valid {
			seriesID = r.SeriesID.String
		}
		out.Items = append(out.Items, AttentionItem{
			Kind:         "import_failed",
			SeriesID:     seriesID,
			ReleaseTitle: r.Title,
			Detail:       detail,
			CreatedAt:    r.CreatedAt,
		})
		out.Counts.ImportFailed++
	}

	return out, nil
}
