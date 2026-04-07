package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/beacon-stack/pilot/internal/core/stats"
	"github.com/beacon-stack/pilot/internal/scheduler"
)

// StatsSnapshot returns a Job that records a point-in-time stats snapshot daily.
func StatsSnapshot(statsSvc *stats.Service, logger *slog.Logger) scheduler.Job {
	return scheduler.Job{
		Name:     "stats_snapshot",
		Interval: 24 * time.Hour,
		Fn: func(ctx context.Context) {
			if err := statsSvc.Snapshot(ctx); err != nil {
				logger.Warn("stats snapshot failed", "error", err)
			}
		},
	}
}
