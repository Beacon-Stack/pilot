// Package scheduler runs recurring background jobs at fixed intervals.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Job describes a recurring background task.
type Job struct {
	// Name is used in log output.
	Name string
	// Interval is how often the job runs.
	Interval time.Duration
	// Fn is the work to perform. It receives a context that is cancelled when
	// the scheduler is stopped.
	Fn func(ctx context.Context)
}

// Scheduler runs a set of Jobs at their configured intervals.
// Jobs run sequentially within their own goroutine; they do not block each other.
type Scheduler struct {
	jobs   []Job
	logger *slog.Logger
}

// New creates a Scheduler.
func New(logger *slog.Logger) *Scheduler {
	return &Scheduler{logger: logger}
}

// Add registers a job. Must be called before Start.
func (s *Scheduler) Add(j Job) {
	s.jobs = append(s.jobs, j)
}

// Jobs returns a snapshot of all registered jobs.
func (s *Scheduler) Jobs() []Job {
	result := make([]Job, len(s.jobs))
	copy(result, s.jobs)
	return result
}

// RunNow triggers the named job immediately in a new goroutine.
// Returns an error if no job with that name is registered.
func (s *Scheduler) RunNow(ctx context.Context, name string) error {
	for _, j := range s.jobs {
		if j.Name == name {
			go j.Fn(ctx)
			return nil
		}
	}
	return fmt.Errorf("task %q not found", name)
}

// Start launches all jobs and blocks until ctx is cancelled.
// Each job runs in its own goroutine on its own ticker.
func (s *Scheduler) Start(ctx context.Context) {
	for _, j := range s.jobs {
		j := j
		go func() {
			ticker := time.NewTicker(j.Interval)
			defer ticker.Stop()
			s.logger.Info("scheduler job registered", "job", j.Name, "interval", j.Interval)
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					j.Fn(ctx)
				}
			}
		}()
	}
	<-ctx.Done()
}
