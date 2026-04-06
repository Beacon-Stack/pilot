package v1

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-media/pilot/internal/sonarrimport"
)

// ── Request / response shapes ────────────────────────────────────────────────

type sonarrPreviewBody struct {
	URL    string `json:"url"     minLength:"1" doc:"Sonarr base URL (e.g. http://localhost:8989)"`
	APIKey string `json:"api_key" minLength:"1" doc:"Sonarr API key"`
}

type sonarrPreviewInput struct {
	Body sonarrPreviewBody
}

type sonarrPreviewOutput struct {
	Body *sonarrimport.PreviewResult
}

type sonarrExecuteBody struct {
	URL     string                     `json:"url"     minLength:"1"`
	APIKey  string                     `json:"api_key" minLength:"1"`
	Options sonarrimport.ImportOptions `json:"options"`
}

type sonarrExecuteInput struct {
	Body sonarrExecuteBody
}

type sonarrExecuteOutput struct {
	Body *sonarrimport.ImportResult
}

// ── Registration ─────────────────────────────────────────────────────────────

func RegisterImportRoutes(api huma.API, svc *sonarrimport.Service) {
	tags := []string{"Import"}

	// POST /api/v1/import/sonarr/preview
	huma.Register(api, huma.Operation{
		OperationID: "preview-sonarr-import",
		Method:      http.MethodPost,
		Path:        "/api/v1/import/sonarr/preview",
		Summary:     "Preview Sonarr import",
		Tags:        tags,
	}, func(ctx context.Context, input *sonarrPreviewInput) (*sonarrPreviewOutput, error) {
		result, err := svc.Preview(ctx, input.Body.URL, input.Body.APIKey)
		if err != nil {
			return nil, huma.NewError(http.StatusBadRequest, "Sonarr preview failed", err)
		}
		return &sonarrPreviewOutput{Body: result}, nil
	})

	// POST /api/v1/import/sonarr/execute
	huma.Register(api, huma.Operation{
		OperationID: "execute-sonarr-import",
		Method:      http.MethodPost,
		Path:        "/api/v1/import/sonarr/execute",
		Summary:     "Execute Sonarr import",
		Tags:        tags,
	}, func(ctx context.Context, input *sonarrExecuteInput) (*sonarrExecuteOutput, error) {
		result, err := svc.Execute(ctx, input.Body.URL, input.Body.APIKey, input.Body.Options)
		if err != nil {
			return nil, huma.NewError(http.StatusBadRequest, "Sonarr import failed", err)
		}
		return &sonarrExecuteOutput{Body: result}, nil
	})
}
