package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/beacon-stack/pilot/internal/core/importlist"
	"github.com/beacon-stack/pilot/internal/scheduler"
)

// ImportListSync returns a Job that syncs all enabled import lists every 6 hours.
func ImportListSync(svc *importlist.Service, logger *slog.Logger) scheduler.Job {
	return scheduler.Job{
		Name:     "import_list_sync",
		Interval: 6 * time.Hour,
		Fn: func(ctx context.Context) {
			result := svc.Sync(ctx)
			logger.Info("import list sync completed",
				slog.Int("lists_processed", result.ListsProcessed),
				slog.Int("series_added", result.SeriesAdded),
				slog.Int("series_skipped", result.SeriesSkipped),
				slog.Int("errors", len(result.Errors)),
			)
		},
	}
}
