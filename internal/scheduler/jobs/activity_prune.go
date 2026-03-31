package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/screenarr/screenarr/internal/core/activity"
	"github.com/screenarr/screenarr/internal/scheduler"
)

// ActivityPrune returns a Job that deletes activity log entries older than 30 days.
func ActivityPrune(svc *activity.Service, logger *slog.Logger) scheduler.Job {
	return scheduler.Job{
		Name:     "activity_prune",
		Interval: 24 * time.Hour,
		Fn: func(ctx context.Context) {
			if err := svc.Prune(ctx, 30*24*time.Hour); err != nil {
				logger.Warn("activity prune failed", "error", err)
			}
		},
	}
}
