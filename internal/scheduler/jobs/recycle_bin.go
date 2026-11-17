package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/beacon-stack/pilot/internal/core/recyclebin"
	"github.com/beacon-stack/pilot/internal/scheduler"
)

// RecycleBinCleanup returns a Job that deletes files older than the
// cleanup period from the recycle bin.
func RecycleBinCleanup(svc *recyclebin.Service, logger *slog.Logger) scheduler.Job {
	return scheduler.Job{
		Name:     "recycle_bin_cleanup",
		Interval: 24 * time.Hour,
		Fn: func(ctx context.Context) {
			if _, err := svc.Cleanup(ctx); err != nil {
				logger.Warn("recycle bin cleanup failed", "error", err)
			}
		},
	}
}
