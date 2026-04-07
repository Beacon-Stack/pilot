package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/beacon-media/pilot/internal/core/show"
	"github.com/beacon-media/pilot/internal/scheduler"
)

// RefreshMetadata returns a Job that re-fetches metadata for all series.
// Runs every 12 hours. Updates episode still images and runtimes from TMDB.
func RefreshMetadata(showSvc *show.Service, logger *slog.Logger) scheduler.Job {
	return scheduler.Job{
		Name:     "refresh_metadata",
		Interval: 12 * time.Hour,
		Fn: func(ctx context.Context) {
			logger.Info("task started", "task", "refresh_metadata")
			start := time.Now()

			if err := runRefreshMetadata(ctx, showSvc, logger); err != nil {
				logger.Warn("task failed",
					"task", "refresh_metadata",
					"error", err,
					"duration_ms", time.Since(start).Milliseconds(),
				)
				return
			}

			logger.Info("task finished",
				"task", "refresh_metadata",
				"duration_ms", time.Since(start).Milliseconds(),
			)
		},
	}
}

func runRefreshMetadata(ctx context.Context, showSvc *show.Service, logger *slog.Logger) error {
	result, err := showSvc.List(ctx, show.ListRequest{Page: 1, PerPage: 10000})
	if err != nil {
		return err
	}

	for _, s := range result.Series {
		if err := showSvc.RefreshEpisodeMetadata(ctx, s.ID, s.TMDBID); err != nil {
			logger.Warn("refresh_metadata: failed for series",
				"series", s.Title, "tmdb_id", s.TMDBID, "error", err)
			continue
		}
		logger.Info("refresh_metadata: updated", "series", s.Title)
	}
	return nil
}
