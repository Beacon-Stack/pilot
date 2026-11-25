package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/beacon-stack/pilot/internal/availability"
	"github.com/beacon-stack/pilot/internal/scheduler"
)

// AvailabilityRefresh returns a Job that re-reads which episodes the
// media servers hold, so an episode shows as available soon after the
// server picks it up. Runs every 3 minutes; it is two requests per TV
// library, one of them listing every episode.
func AvailabilityRefresh(svc *availability.Service, logger *slog.Logger) scheduler.Job {
	return scheduler.Job{
		Name:     "availability_refresh",
		Interval: 3 * time.Minute,
		Fn: func(ctx context.Context) {
			if err := svc.Refresh(ctx); err != nil {
				logger.Warn("task failed", "task", "availability_refresh", "error", err)
			}
		},
	}
}
