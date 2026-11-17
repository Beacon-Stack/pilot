package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/beacon-stack/pilot/internal/core/importer"
	"github.com/beacon-stack/pilot/internal/scheduler"
)

// importRetryInterval is how often failed imports are reconsidered. Each
// grab also has its own backoff (see importer.RetryFailedImports), so a
// short tick only bounds how quickly Pilot notices a fixed library folder.
const importRetryInterval = 10 * time.Minute

// ImportRetry returns a Job that re-runs imports that failed after the
// download completed (unwritable library folder, transient I/O error,
// etc.), so they recover without anyone re-importing by hand.
func ImportRetry(svc *importer.Service, logger *slog.Logger) scheduler.Job {
	return scheduler.Job{
		Name:     "import_retry",
		Interval: importRetryInterval,
		Fn: func(ctx context.Context) {
			start := time.Now()
			res, err := svc.RetryFailedImports(ctx, time.Now().UTC())
			if err != nil {
				logger.Warn("task failed",
					"task", "import_retry",
					"error", err,
					"duration_ms", time.Since(start).Milliseconds(),
				)
				return
			}
			if res == (importer.RetryResult{}) {
				return // nothing pending; keep the log quiet
			}
			logger.Info("task finished",
				"task", "import_retry",
				"attempted", res.Attempted,
				"imported", res.Imported,
				"held_unwritable", res.Held,
				"not_due", res.NotDue,
				"gave_up", res.GaveUp,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		},
	}
}
