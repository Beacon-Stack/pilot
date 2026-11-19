package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/beacon-stack/pilot/internal/core/backup"
	"github.com/beacon-stack/pilot/internal/scheduler"
)

// ScheduledBackup returns a Job that makes a scheduled database backup
// when one is due and prunes old ones. It checks hourly; the backup
// interval itself is a setting (default weekly).
func ScheduledBackup(svc *backup.Service, logger *slog.Logger) scheduler.Job {
	return scheduler.Job{
		Name:     "scheduled_backup",
		Interval: time.Hour,
		Fn: func(ctx context.Context) {
			made, err := svc.RunScheduled(ctx)
			if err != nil {
				logger.Warn("scheduled backup failed", "error", err)
				return
			}
			if made {
				logger.Info("scheduled backup made", "folder", svc.Dir())
			}
		},
	}
}
