package v1

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pilot/internal/core/mediaserver"
	"github.com/beacon-stack/pilot/internal/registry"
)

// ── Request / response shapes ─────────────────────────────────────────────────

type mediaServerBody struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Kind      string          `json:"kind"     doc:"Plugin kind: plex, emby, jellyfin"`
	Enabled   bool            `json:"enabled"`
	Settings  json.RawMessage `json:"settings" doc:"Plugin-specific settings as JSON"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type mediaServerListOutput struct {
	Body []*mediaServerBody
}

type mediaServerGetOutput struct {
	Body *mediaServerBody
}

type mediaServerIDInput struct {
	ID string `path:"id"`
}

type mediaServerCreateBody struct {
	Name     string          `json:"name"     minLength:"1"`
	Kind     string          `json:"kind"     minLength:"1"`
	Enabled  bool            `json:"enabled"`
	Settings json.RawMessage `json:"settings,omitempty"`
}

type mediaServerCreateInput struct {
	Body mediaServerCreateBody
}

type mediaServerUpdateInput struct {
	ID   string `path:"id"`
	Body mediaServerCreateBody
}

type mediaServerDeleteInput struct {
	ID string `path:"id"`
}

type mediaServerDeleteOutput struct{}

type mediaServerTestInput struct {
	ID string `path:"id"`
}

type mediaServerTestOutput struct{}

// ── Helpers ───────────────────────────────────────────────────────────────────

func msToBody(cfg mediaserver.Config) *mediaServerBody {
	return &mediaServerBody{
		ID:        cfg.ID,
		Name:      cfg.Name,
		Kind:      cfg.Kind,
		Enabled:   cfg.Enabled,
		Settings:  registry.Default.SanitizeMediaServerSettings(cfg.Kind, cfg.Settings),
		CreatedAt: cfg.CreatedAt,
		UpdatedAt: cfg.UpdatedAt,
	}
}

// ── Route registration ─────────────────────────────────────────────────────────

// RegisterMediaServerRoutes registers the /api/v1/media-servers endpoints.
func RegisterMediaServerRoutes(api huma.API, svc *mediaserver.Service) {
	// GET /api/v1/media-servers
	huma.Register(api, huma.Operation{
		OperationID: "list-media-servers",
		Method:      http.MethodGet,
		Path:        "/api/v1/media-servers",
		Summary:     "List all media server configurations",
		Tags:        []string{"Media Servers"},
	}, func(ctx context.Context, _ *struct{}) (*mediaServerListOutput, error) {
		cfgs, err := svc.List(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list media servers", err)
		}
		bodies := make([]*mediaServerBody, len(cfgs))
		for i, c := range cfgs {
			bodies[i] = msToBody(c)
		}
		return &mediaServerListOutput{Body: bodies}, nil
	})

	// POST /api/v1/media-servers
	huma.Register(api, huma.Operation{
		OperationID:   "create-media-server",
		Method:        http.MethodPost,
		Path:          "/api/v1/media-servers",
		Summary:       "Create a media server configuration",
		Tags:          []string{"Media Servers"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, input *mediaServerCreateInput) (*mediaServerGetOutput, error) {
		cfg, err := svc.Create(ctx, mediaserver.CreateRequest{
			Name:     input.Body.Name,
			Kind:     input.Body.Kind,
			Enabled:  input.Body.Enabled,
			Settings: input.Body.Settings,
		})
		if err != nil {
			return nil, huma.NewError(http.StatusUnprocessableEntity, "failed to create media server", err)
		}
		return &mediaServerGetOutput{Body: msToBody(cfg)}, nil
	})

	// GET /api/v1/media-servers/{id}
	huma.Register(api, huma.Operation{
		OperationID: "get-media-server",
		Method:      http.MethodGet,
		Path:        "/api/v1/media-servers/{id}",
		Summary:     "Get a media server configuration",
		Tags:        []string{"Media Servers"},
	}, func(ctx context.Context, input *mediaServerIDInput) (*mediaServerGetOutput, error) {
		cfg, err := svc.Get(ctx, input.ID)
		if err != nil {
			if errors.Is(err, mediaserver.ErrNotFound) {
				return nil, huma.Error404NotFound("media server not found")
			}
			return nil, huma.NewError(http.StatusInternalServerError, "failed to get media server", err)
		}
		return &mediaServerGetOutput{Body: msToBody(cfg)}, nil
	})

	// PUT /api/v1/media-servers/{id}
	huma.Register(api, huma.Operation{
		OperationID: "update-media-server",
		Method:      http.MethodPut,
		Path:        "/api/v1/media-servers/{id}",
		Summary:     "Update a media server configuration",
		Tags:        []string{"Media Servers"},
	}, func(ctx context.Context, input *mediaServerUpdateInput) (*mediaServerGetOutput, error) {
		cfg, err := svc.Update(ctx, input.ID, mediaserver.UpdateRequest{
			Name:     input.Body.Name,
			Kind:     input.Body.Kind,
			Enabled:  input.Body.Enabled,
			Settings: input.Body.Settings,
		})
		if err != nil {
			if errors.Is(err, mediaserver.ErrNotFound) {
				return nil, huma.Error404NotFound("media server not found")
			}
			return nil, huma.NewError(http.StatusUnprocessableEntity, "failed to update media server", err)
		}
		return &mediaServerGetOutput{Body: msToBody(cfg)}, nil
	})

	// DELETE /api/v1/media-servers/{id}
	huma.Register(api, huma.Operation{
		OperationID:   "delete-media-server",
		Method:        http.MethodDelete,
		Path:          "/api/v1/media-servers/{id}",
		Summary:       "Delete a media server configuration",
		Tags:          []string{"Media Servers"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *mediaServerDeleteInput) (*mediaServerDeleteOutput, error) {
		if err := svc.Delete(ctx, input.ID); err != nil {
			if errors.Is(err, mediaserver.ErrNotFound) {
				return nil, huma.Error404NotFound("media server not found")
			}
			return nil, huma.NewError(http.StatusInternalServerError, "failed to delete media server", err)
		}
		return &mediaServerDeleteOutput{}, nil
	})

	// POST /api/v1/media-servers/{id}/test
	huma.Register(api, huma.Operation{
		OperationID:   "test-media-server",
		Method:        http.MethodPost,
		Path:          "/api/v1/media-servers/{id}/test",
		Summary:       "Test media server connectivity",
		Tags:          []string{"Media Servers"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *mediaServerTestInput) (*mediaServerTestOutput, error) {
		if err := svc.Test(ctx, input.ID); err != nil {
			if errors.Is(err, mediaserver.ErrNotFound) {
				return nil, huma.Error404NotFound("media server not found")
			}
			return nil, huma.NewError(http.StatusBadGateway, "test media server failed", err)
		}
		return &mediaServerTestOutput{}, nil
	})
}
