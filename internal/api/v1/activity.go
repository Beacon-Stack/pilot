package v1

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pilot/internal/core/activity"
)

type listActivitiesInput struct {
	Category string `query:"category" doc:"Filter by category: grab, import, task, health, show"`
	Since    string `query:"since"    doc:"Only return activities after this ISO 8601 timestamp"`
	Limit    int64  `query:"limit"    doc:"Max results (default 100, max 500)" default:"100"`
}

type listActivitiesOutput struct {
	Body *activity.ListResult
}

// RegisterActivityRoutes registers the /api/v1/activity endpoints.
func RegisterActivityRoutes(humaAPI huma.API, svc *activity.Service) {
	huma.Register(humaAPI, huma.Operation{
		OperationID: "list-activity",
		Method:      http.MethodGet,
		Path:        "/api/v1/activity",
		Summary:     "List activity log",
		Description: "Returns a chronological feed of system events: grabs, imports, task runs, health changes, and series additions.",
		Tags:        []string{"Activity"},
	}, func(ctx context.Context, input *listActivitiesInput) (*listActivitiesOutput, error) {
		if input.Category != "" && !activity.ValidCategory(input.Category) {
			return nil, huma.NewError(http.StatusBadRequest, "invalid category: must be one of grab, import, task, health, show")
		}

		var catPtr, sincePtr *string
		if input.Category != "" {
			catPtr = &input.Category
		}
		if input.Since != "" {
			sincePtr = &input.Since
		}

		result, err := svc.List(ctx, catPtr, sincePtr, input.Limit)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list activities", err)
		}

		return &listActivitiesOutput{Body: result}, nil
	})
}
