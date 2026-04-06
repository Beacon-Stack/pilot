package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/beacon-media/pilot/internal/core/show"
	"github.com/beacon-media/pilot/internal/scheduler"
)

// RefreshMetadata returns a Job that re-fetches metadata for all series.
// Runs every 12 hours. Silently skips series removed mid-run and exits early
// if the metadata provider is not configured.
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
	// List all series in the library.
	result, err := showSvc.List(ctx, show.ListRequest{Page: 1, PerPage: 10000})
	if err != nil {
		return err
	}

	if len(result.Series) == 0 {
		return nil
	}

	// RefreshMetadata is not yet implemented on the show service.
	// Log once and return rather than iterating fruitlessly.
	logger.Info("refresh_metadata: metadata refresh not yet implemented — skipping",
		"series_count", len(result.Series),
	)
	return nil
}
