# Feature: Watch Sync Scheduler Job

## Context

Luminarr has a `watch_sync` scheduler job (~38 lines) that syncs watch
history from media servers (Plex) every 6 hours. This enables automatic
unmonitoring of watched content. Pilot has no equivalent.

## Dependencies

This feature requires a `watchsync` service that:
1. Lists all configured media server plugins
2. For each server, fetches watch status for library items
3. Marks watched episodes as unmonitored (if configured)

This is a non-trivial feature because it requires:
- Media server plugins to expose a "get watched items" interface
- The show service to support bulk unmonitoring
- A user-facing setting to enable/disable auto-unmonitoring

## Files to Create

### `internal/scheduler/jobs/watch_sync.go` (~38 lines)

```go
package jobs

import (
    "context"
    "log/slog"
    "time"

    "github.com/pilot/pilot/internal/scheduler"
)

// WatchSyncService is the interface the job needs.
type WatchSyncService interface {
    Sync(ctx context.Context) error
}

func WatchSync(svc WatchSyncService, logger *slog.Logger) scheduler.Job {
    return scheduler.Job{
        Name:     "watch_sync",
        Interval: 6 * time.Hour,
        Fn: func(ctx context.Context) {
            start := time.Now()
            logger.Info("watch sync started")
            if err := svc.Sync(ctx); err != nil {
                logger.Warn("watch sync failed", "error", err)
                return
            }
            logger.Info("watch sync completed", "duration", time.Since(start))
        },
    }
}
```

### Future: `internal/core/watchsync/service.go`

This is the larger piece — the actual sync logic. Deferred until the media
server plugin interface supports watch status queries. For now, the scheduler
job can be registered with a no-op service or skipped entirely.

## Recommendation

**Defer this feature.** Register the scheduler job skeleton but don't wire
it up until the media server plugin interface is extended. The job itself
is trivial; the service layer is the complex part.

## Verification

If implemented:
1. `go build ./...` compiles
2. Job appears in `GET /api/v1/tasks`
3. Job runs without panic
