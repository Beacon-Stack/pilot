# Feature: Activity Prune Scheduler Job

**Status: DONE** (completed 2026-03-30)

## Context

Luminarr has an `activity_prune` scheduler job that deletes activity log
entries older than 30 days, running once per 24 hours. Without this,
Screenarr's `activity_log` table grows unbounded.

## Files to Create

### `internal/scheduler/jobs/activity_prune.go` (~25 lines)

```go
package jobs

import (
    "context"
    "log/slog"
    "time"

    "github.com/screenarr/screenarr/internal/core/activity"
    "github.com/screenarr/screenarr/internal/scheduler"
)

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
```

## Files to Check/Modify

### `internal/core/activity/service.go`

Verify a `Prune(ctx, maxAge time.Duration) error` method exists. If not,
add it:

```go
func (s *Service) Prune(ctx context.Context, maxAge time.Duration) error {
    cutoff := time.Now().UTC().Add(-maxAge).Format(time.RFC3339)
    return s.q.DeleteActivityBefore(ctx, cutoff)
}
```

### `internal/db/queries/sqlite/activity.sql`

Verify a `DeleteActivityBefore` query exists. If not, add:

```sql
-- name: DeleteActivityBefore :exec
DELETE FROM activity_log WHERE created_at < ?;
```

Run `sqlc generate` after adding the query.

### `cmd/screenarr/main.go`

Add the job to the scheduler:

```go
sched.Add(jobs.ActivityPrune(activitySvc, logger))
```

## Verification

1. `go build ./...` compiles
2. `make check` passes
3. Job appears in `GET /api/v1/tasks` response
