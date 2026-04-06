package v1

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-media/pilot/internal/core/indexer"
	"github.com/beacon-media/pilot/internal/registry"
)

// ── Request / response shapes ────────────────────────────────────────────────

type indexerBody struct {
	ID        string          `json:"id"        doc:"Indexer UUID"`
	Name      string          `json:"name"      doc:"Display name"`
	Kind      string          `json:"kind"      doc:"Plugin kind: torznab, newznab"`
	Enabled   bool            `json:"enabled"   doc:"Whether this indexer is active"`
	Priority  int             `json:"priority"  doc:"Search priority; lower = searched first"`
	Settings  json.RawMessage `json:"settings"  doc:"Plugin-specific settings (URL, API key, etc.)"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type indexerInput struct {
	Name     string          `json:"name"               doc:"Display name"`
	Kind     string          `json:"kind"               doc:"Plugin kind: torznab, newznab"`
	Enabled  *bool           `json:"enabled,omitempty"  doc:"Whether this indexer is active (default: true)"`
	Priority *int            `json:"priority,omitempty" doc:"Search priority; lower = searched first (default: 25)"`
	Settings json.RawMessage `json:"settings"           doc:"Plugin-specific settings (URL, API key, etc.)"`
}

type indexerOutput struct {
	Body *indexerBody
}

type indexerListOutput struct {
	Body []*indexerBody
}

type indexerCreateInput struct {
	Body indexerInput
}

type indexerGetInput struct {
	ID string `path:"id"`
}

type indexerUpdateInput struct {
	ID   string `path:"id"`
	Body indexerInput
}

type indexerDeleteInput struct {
	ID string `path:"id"`
}

type indexerDeleteOutput struct{}

type indexerTestInput struct {
	ID string `path:"id"`
}

type indexerTestOutput struct {
	Body struct {
		OK      bool   `json:"ok"`
		Message string `json:"message,omitempty"`
	}
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func indexerToBody(c indexer.Config) *indexerBody {
	return &indexerBody{
		ID:        c.ID,
		Name:      c.Name,
		Kind:      c.Kind,
		Enabled:   c.Enabled,
		Priority:  c.Priority,
		Settings:  registry.Default.SanitizeIndexerSettings(c.Kind, c.Settings),
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

func indexerInputToRequest(in indexerInput) indexer.CreateRequest {
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	priority := 25
	if in.Priority != nil {
		priority = *in.Priority
	}
	return indexer.CreateRequest{
		Name:     in.Name,
		Kind:     in.Kind,
		Enabled:  enabled,
		Priority: priority,
		Settings: in.Settings,
	}
}

// ── Route registration ───────────────────────────────────────────────────────

// RegisterIndexerRoutes registers all /api/v1/indexers endpoints.
func RegisterIndexerRoutes(api huma.API, svc *indexer.Service) {
	// GET /api/v1/indexers
	huma.Register(api, huma.Operation{
		OperationID: "list-indexers",
		Method:      http.MethodGet,
		Path:        "/api/v1/indexers",
		Summary:     "List indexers",
		Tags:        []string{"Indexers"},
	}, func(ctx context.Context, _ *struct{}) (*indexerListOutput, error) {
		configs, err := svc.List(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list indexers", err)
		}
		bodies := make([]*indexerBody, len(configs))
		for i, c := range configs {
			bodies[i] = indexerToBody(c)
		}
		return &indexerListOutput{Body: bodies}, nil
	})

	// POST /api/v1/indexers
	huma.Register(api, huma.Operation{
		OperationID:   "create-indexer",
		Method:        http.MethodPost,
		Path:          "/api/v1/indexers",
		Summary:       "Create an indexer",
		Tags:          []string{"Indexers"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, input *indexerCreateInput) (*indexerOutput, error) {
		cfg, err := svc.Create(ctx, indexerInputToRequest(input.Body))
		if err != nil {
			return nil, huma.NewError(http.StatusBadRequest, "failed to create indexer", err)
		}
		return &indexerOutput{Body: indexerToBody(cfg)}, nil
	})

	// GET /api/v1/indexers/{id}
	huma.Register(api, huma.Operation{
		OperationID: "get-indexer",
		Method:      http.MethodGet,
		Path:        "/api/v1/indexers/{id}",
		Summary:     "Get an indexer",
		Tags:        []string{"Indexers"},
	}, func(ctx context.Context, input *indexerGetInput) (*indexerOutput, error) {
		cfg, err := svc.Get(ctx, input.ID)
		if err != nil {
			if errors.Is(err, indexer.ErrNotFound) {
				return nil, huma.Error404NotFound("indexer not found")
			}
			return nil, huma.NewError(http.StatusInternalServerError, "failed to get indexer", err)
		}
		return &indexerOutput{Body: indexerToBody(cfg)}, nil
	})

	// PUT /api/v1/indexers/{id}
	huma.Register(api, huma.Operation{
		OperationID: "update-indexer",
		Method:      http.MethodPut,
		Path:        "/api/v1/indexers/{id}",
		Summary:     "Update an indexer",
		Tags:        []string{"Indexers"},
	}, func(ctx context.Context, input *indexerUpdateInput) (*indexerOutput, error) {
		cfg, err := svc.Update(ctx, input.ID, indexerInputToRequest(input.Body))
		if err != nil {
			if errors.Is(err, indexer.ErrNotFound) {
				return nil, huma.Error404NotFound("indexer not found")
			}
			return nil, huma.NewError(http.StatusInternalServerError, "failed to update indexer", err)
		}
		return &indexerOutput{Body: indexerToBody(cfg)}, nil
	})

	// DELETE /api/v1/indexers/{id}
	huma.Register(api, huma.Operation{
		OperationID:   "delete-indexer",
		Method:        http.MethodDelete,
		Path:          "/api/v1/indexers/{id}",
		Summary:       "Delete an indexer",
		Tags:          []string{"Indexers"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *indexerDeleteInput) (*indexerDeleteOutput, error) {
		if err := svc.Delete(ctx, input.ID); err != nil {
			if errors.Is(err, indexer.ErrNotFound) {
				return nil, huma.Error404NotFound("indexer not found")
			}
			return nil, huma.NewError(http.StatusInternalServerError, "failed to delete indexer", err)
		}
		return &indexerDeleteOutput{}, nil
	})

	// POST /api/v1/indexers/{id}/test
	huma.Register(api, huma.Operation{
		OperationID: "test-indexer",
		Method:      http.MethodPost,
		Path:        "/api/v1/indexers/{id}/test",
		Summary:     "Test indexer connectivity",
		Tags:        []string{"Indexers"},
	}, func(ctx context.Context, input *indexerTestInput) (*indexerTestOutput, error) {
		err := svc.Test(ctx, input.ID)
		if err != nil {
			if errors.Is(err, indexer.ErrNotFound) {
				return nil, huma.Error404NotFound("indexer not found")
			}
			out := &indexerTestOutput{}
			out.Body.OK = false
			out.Body.Message = err.Error()
			return out, nil
		}
		out := &indexerTestOutput{}
		out.Body.OK = true
		return out, nil
	})
}
