// Package jobs provides the built-in scheduler job definitions.
package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/beacon-stack/pilot/internal/core/queue"
	"github.com/beacon-stack/pilot/internal/scheduler"
)

// QueuePoll returns a Job that polls all active downloads and updates their
// status in the database. Runs at the given interval (minimum 10 seconds).
func QueuePoll(svc *queue.Service, interval time.Duration, logger *slog.Logger) scheduler.Job {
	if interval < 10*time.Second {
		interval = 10 * time.Second
	}
	return scheduler.Job{
		Name:     "queue_poll",
		Interval: interval,
		Fn: func(ctx context.Context) {
			logger.Info("task started", "task", "queue_poll")
			start := time.Now()
			if err := svc.PollAndUpdate(ctx); err != nil {
				logger.Warn("task failed",
					"task", "queue_poll",
					"error", err,
					"duration_ms", time.Since(start).Milliseconds(),
				)
				return
			}
			logger.Info("task finished",
				"task", "queue_poll",
				"duration_ms", time.Since(start).Milliseconds(),
			)
		},
	}
}
