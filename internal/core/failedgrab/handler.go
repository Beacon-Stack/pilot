// Package failedgrab handles downloads that a download client reports as
// failed: bad NZBs, missing usenet articles, torrent errors.
//
// For every failed grab it blocklists the release so the same release is
// never grabbed again, and marks the grab failed. If the grab came from
// automatic search, it asks for a re-search by publishing
// TypeAutoSearchRetry, capped by the same per-episode circuit breaker the
// stall watcher uses. Grabs the user picked interactively are never
// re-searched automatically: the user decides what to do next.
//
// This mirrors stallwatcher.handleStall for clients other than Haul's
// stall endpoint. Stalls and failures share one retry budget per episode
// (see CountRecentStallsForEpisode in blocklist.sql).
package failedgrab

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/beacon-stack/pilot/internal/core/blocklist"
	"github.com/beacon-stack/pilot/internal/core/dbutil"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/events"
)

// MaxRetriesPerEpisode is the circuit breaker: once this many stall or
// download-failure blocklist entries exist for a (series, episode) in the
// last 24 hours, automatic re-search stops. Matches
// stallwatcher.MaxStallRetriesPerEpisode.
const MaxRetriesPerEpisode = 3

// Blocklister is the slice of blocklist.Service this package needs.
type Blocklister interface {
	AddFromDownloadFailure(ctx context.Context, p blocklist.StallEntry) error
	CountRecentStalls(ctx context.Context, seriesID, episodeID string) (int64, error)
}

// Publisher is the slice of events.Bus this package needs.
type Publisher interface {
	Publish(ctx context.Context, e events.Event)
}

// Handler reacts to failed downloads.
type Handler struct {
	blocklist Blocklister
	bus       Publisher
	logger    *slog.Logger
}

// New creates a Handler.
func New(bl Blocklister, bus Publisher, logger *slog.Logger) *Handler {
	return &Handler{blocklist: bl, bus: bus, logger: logger}
}

// HandleDownloadFailed blocklists the grab's release, announces the
// failure, and requests a re-search for automatic grabs that are still
// under the circuit breaker. clientError is the download client's own
// message, if it gave one. The caller is responsible for having already
// recorded the grab's status as failed.
func (h *Handler) HandleDownloadFailed(ctx context.Context, g db.GrabHistory, clientError string) error {
	episodeID := dbutil.NullStringValue(g.EpisodeID)
	infoHash := dbutil.NullStringValue(g.InfoHash)

	notes := "download client reported the download as failed"
	if clientError != "" {
		notes += ": " + clientError
	}

	h.logger.Warn("failedgrab: blocklisting failed download",
		"grab_id", g.ID,
		"series_id", g.SeriesID,
		"release", g.ReleaseTitle,
		"client_error", clientError,
		"source", g.Source,
	)

	err := h.blocklist.AddFromDownloadFailure(ctx, blocklist.StallEntry{
		SeriesID:     g.SeriesID,
		EpisodeID:    episodeID,
		ReleaseGUID:  g.ReleaseGuid,
		ReleaseTitle: g.ReleaseTitle,
		IndexerID:    dbutil.NullStringValue(g.IndexerID),
		Protocol:     g.Protocol,
		Size:         g.Size,
		Notes:        notes,
		InfoHash:     infoHash,
	})
	if err != nil && !errors.Is(err, blocklist.ErrAlreadyBlocklisted) {
		return fmt.Errorf("blocklisting failed download: %w", err)
	}

	reason := "download failed"
	if clientError != "" {
		reason = clientError
	}
	h.bus.Publish(ctx, events.Event{
		Type:   events.TypeGrabFailed,
		ShowID: g.SeriesID,
		Data: map[string]any{
			"grab_id":       g.ID,
			"series_id":     g.SeriesID,
			"release_title": g.ReleaseTitle,
			"reason":        reason,
			"source":        g.Source,
		},
	})

	if g.Source != "auto_search" {
		return nil
	}

	recent, err := h.blocklist.CountRecentStalls(ctx, g.SeriesID, episodeID)
	if err != nil {
		h.logger.Warn("failedgrab: counting recent failures", "error", err)
		return nil
	}
	if recent >= MaxRetriesPerEpisode {
		h.logger.Info("failedgrab: circuit breaker tripped, not re-searching",
			"series_id", g.SeriesID, "episode_id", episodeID, "failure_count", recent)
		h.bus.Publish(ctx, events.Event{
			Type:   events.TypeGrabStalledGaveUp,
			ShowID: g.SeriesID,
			Data: map[string]any{
				"grab_id":     g.ID,
				"series_id":   g.SeriesID,
				"stall_count": recent,
			},
		})
		return nil
	}

	data := map[string]any{
		"series_id":   g.SeriesID,
		"episode_id":  episodeID,
		"retry_count": recent + 1,
	}
	if g.SeasonNumber != nil {
		data["season"] = *g.SeasonNumber
	}
	h.bus.Publish(ctx, events.Event{Type: events.TypeAutoSearchRetry, ShowID: g.SeriesID, Data: data})
	return nil
}
