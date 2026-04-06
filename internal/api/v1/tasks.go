package v1

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-media/pilot/internal/scheduler"
)

// ── Request / response shapes ────────────────────────────────────────────────

type taskBody struct {
	Name     string `json:"name"     doc:"Unique task name"`
	Interval string `json:"interval" doc:"How often the task runs, e.g. '15m0s'"`
}

type taskListOutput struct {
	Body []*taskBody
}

type taskRunInput struct {
	Name string `path:"name" doc:"Task name"`
}

type taskRunOutput struct{}

// ── Route registration ───────────────────────────────────────────────────────

// RegisterTaskRoutes registers the /api/v1/tasks endpoints.
func RegisterTaskRoutes(api huma.API, sched *scheduler.Scheduler) {
	// GET /api/v1/tasks
	huma.Register(api, huma.Operation{
		OperationID: "list-tasks",
		Method:      http.MethodGet,
		Path:        "/api/v1/tasks",
		Summary:     "List all scheduled background tasks",
		Tags:        []string{"Tasks"},
	}, func(_ context.Context, _ *struct{}) (*taskListOutput, error) {
		jobs := sched.Jobs()
		bodies := make([]*taskBody, len(jobs))
		for i, j := range jobs {
			bodies[i] = &taskBody{
				Name:     j.Name,
				Interval: j.Interval.String(),
			}
		}
		return &taskListOutput{Body: bodies}, nil
	})

	// POST /api/v1/tasks/{name}/run
	huma.Register(api, huma.Operation{
		OperationID:   "run-task",
		Method:        http.MethodPost,
		Path:          "/api/v1/tasks/{name}/run",
		Summary:       "Trigger a scheduled task immediately (runs in background)",
		Tags:          []string{"Tasks"},
		DefaultStatus: http.StatusAccepted,
	}, func(ctx context.Context, input *taskRunInput) (*taskRunOutput, error) {
		if err := sched.RunNow(ctx, input.Name); err != nil {
			return nil, huma.Error404NotFound(err.Error())
		}
		return &taskRunOutput{}, nil
	})
}
