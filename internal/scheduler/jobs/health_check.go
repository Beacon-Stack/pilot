package jobs

import (
	"context"
	"time"

	"github.com/beacon-stack/pilot/internal/core/health"
	"github.com/beacon-stack/pilot/internal/scheduler"
)

// HealthCheck returns a Job that tests every enabled download client and
// indexer so the health list stays current.
func HealthCheck(svc *health.Service) scheduler.Job {
	return scheduler.Job{
		Name:     "health_check",
		Interval: time.Hour,
		Fn:       func(ctx context.Context) { svc.CheckConnectivity(ctx) },
	}
}
