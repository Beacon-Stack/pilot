package v1

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pilot/internal/core/importlist"
)

// ── Request / response shapes ────────────────────────────────────────────────

type importListBody struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	Kind             string          `json:"kind"               doc:"Plugin kind: tmdb_popular_tv, trakt_list_tv, etc."`
	Enabled          bool            `json:"enabled"`
	Settings         json.RawMessage `json:"settings"           doc:"Plugin-specific settings as JSON"`
	SearchOnAdd      bool            `json:"search_on_add"      doc:"Auto-search when a series is added"`
	Monitor          bool            `json:"monitor"            doc:"Set added series as monitored"`
	MonitorType      string          `json:"monitor_type"       doc:"Episode monitoring type: all, future, missing, none, pilot, first_season, last_season"`
	QualityProfileID string          `json:"quality_profile_id" doc:"Quality profile for added series"`
	LibraryID        string          `json:"library_id"         doc:"Target library for added series"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

type importListListOutput struct {
	Body []*importListBody
}

type importListGetOutput struct {
	Body *importListBody
}

type importListIDInput struct {
	ID string `path:"id"`
}

type importListCreateBody struct {
	Name             string          `json:"name"               minLength:"1"`
	Kind             string          `json:"kind"               minLength:"1"`
	Enabled          bool            `json:"enabled"`
	Settings         json.RawMessage `json:"settings,omitempty"`
	SearchOnAdd      bool            `json:"search_on_add"`
	Monitor          bool            `json:"monitor"`
	MonitorType      string          `json:"monitor_type"`
	QualityProfileID string          `json:"quality_profile_id"`
	LibraryID        string          `json:"library_id"`
}

type importListCreateInput struct {
	Body importListCreateBody
}

type importListUpdateInput struct {
	ID   string `path:"id"`
	Body importListCreateBody
}

type importListDeleteOutput struct{}

type importListTestOutput struct{}

type importListSyncOutput struct {
	Body *importlist.SyncResult
}

type importListPreviewBody struct {
	Kind     string          `json:"kind"     minLength:"1"`
	Settings json.RawMessage `json:"settings,omitempty"`
}

type importListPreviewInput struct {
	Body importListPreviewBody
}

type importListPreviewOutput struct {
	Body []importlist.PreviewItem
}

// ── Exclusion shapes ─────────────────────────────────────────────────────────

type exclusionBody struct {
	ID        string    `json:"id"`
	TMDbID    int       `json:"tmdb_id"`
	Title     string    `json:"title"`
	Year      int       `json:"year"`
	CreatedAt time.Time `json:"created_at"`
}

type exclusionListOutput struct {
	Body []*exclusionBody
}

type exclusionCreateBody struct {
	TMDbID int    `json:"tmdb_id"`
	Title  string `json:"title"`
	Year   int    `json:"year"`
}

type exclusionCreateInput struct {
	Body exclusionCreateBody
}

type exclusionCreateOutput struct {
	Body *exclusionBody
}

type exclusionDeleteInput struct {
	ID string `path:"id"`
}

type exclusionDeleteOutput struct{}

// ── Registration ─────────────────────────────────────────────────────────────

func RegisterImportListRoutes(api huma.API, svc *importlist.Service) {
	tags := []string{"Import Lists"}

	// GET /api/v1/importlists
	huma.Register(api, huma.Operation{
		OperationID: "list-import-lists",
		Method:      http.MethodGet,
		Path:        "/api/v1/importlists",
		Summary:     "List import lists",
		Tags:        tags,
	}, func(ctx context.Context, _ *struct{}) (*importListListOutput, error) {
		cfgs, err := svc.List(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list import lists", err)
		}
		bodies := make([]*importListBody, len(cfgs))
		for i, c := range cfgs {
			bodies[i] = importListToBody(c)
		}
		return &importListListOutput{Body: bodies}, nil
	})

	// POST /api/v1/importlists
	huma.Register(api, huma.Operation{
		OperationID:   "create-import-list",
		Method:        http.MethodPost,
		Path:          "/api/v1/importlists",
		Summary:       "Create an import list",
		Tags:          tags,
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, input *importListCreateInput) (*importListGetOutput, error) {
		cfg, err := svc.Create(ctx, importListInputToRequest(input.Body))
		if err != nil {
			return nil, huma.NewError(http.StatusBadRequest, "failed to create import list", err)
		}
		return &importListGetOutput{Body: importListToBody(cfg)}, nil
	})

	// GET /api/v1/importlists/{id}
	huma.Register(api, huma.Operation{
		OperationID: "get-import-list",
		Method:      http.MethodGet,
		Path:        "/api/v1/importlists/{id}",
		Summary:     "Get an import list",
		Tags:        tags,
	}, func(ctx context.Context, input *importListIDInput) (*importListGetOutput, error) {
		cfg, err := svc.Get(ctx, input.ID)
		if err != nil {
			if errors.Is(err, importlist.ErrNotFound) {
				return nil, huma.Error404NotFound("import list not found")
			}
			return nil, huma.NewError(http.StatusInternalServerError, "failed to get import list", err)
		}
		return &importListGetOutput{Body: importListToBody(cfg)}, nil
	})

	// PUT /api/v1/importlists/{id}
	huma.Register(api, huma.Operation{
		OperationID: "update-import-list",
		Method:      http.MethodPut,
		Path:        "/api/v1/importlists/{id}",
		Summary:     "Update an import list",
		Tags:        tags,
	}, func(ctx context.Context, input *importListUpdateInput) (*importListGetOutput, error) {
		cfg, err := svc.Update(ctx, input.ID, importListInputToRequest(input.Body))
		if err != nil {
			if errors.Is(err, importlist.ErrNotFound) {
				return nil, huma.Error404NotFound("import list not found")
			}
			return nil, huma.NewError(http.StatusBadRequest, "failed to update import list", err)
		}
		return &importListGetOutput{Body: importListToBody(cfg)}, nil
	})

	// DELETE /api/v1/importlists/{id}
	huma.Register(api, huma.Operation{
		OperationID:   "delete-import-list",
		Method:        http.MethodDelete,
		Path:          "/api/v1/importlists/{id}",
		Summary:       "Delete an import list",
		Tags:          tags,
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *importListIDInput) (*importListDeleteOutput, error) {
		if err := svc.Delete(ctx, input.ID); err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to delete import list", err)
		}
		return nil, nil
	})

	// POST /api/v1/importlists/{id}/test
	huma.Register(api, huma.Operation{
		OperationID:   "test-import-list",
		Method:        http.MethodPost,
		Path:          "/api/v1/importlists/{id}/test",
		Summary:       "Test import list connection",
		Tags:          tags,
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *importListIDInput) (*importListTestOutput, error) {
		if err := svc.Test(ctx, input.ID); err != nil {
			return nil, huma.NewError(http.StatusBadRequest, "test failed", err)
		}
		return nil, nil
	})

	// POST /api/v1/importlists/sync
	huma.Register(api, huma.Operation{
		OperationID:   "sync-all-import-lists",
		Method:        http.MethodPost,
		Path:          "/api/v1/importlists/sync",
		Summary:       "Sync all enabled import lists",
		Tags:          tags,
		DefaultStatus: http.StatusAccepted,
	}, func(ctx context.Context, _ *struct{}) (*importListSyncOutput, error) {
		result := svc.Sync(ctx)
		return &importListSyncOutput{Body: &result}, nil
	})

	// POST /api/v1/importlists/{id}/sync
	huma.Register(api, huma.Operation{
		OperationID:   "sync-import-list",
		Method:        http.MethodPost,
		Path:          "/api/v1/importlists/{id}/sync",
		Summary:       "Sync a single import list",
		Tags:          tags,
		DefaultStatus: http.StatusAccepted,
	}, func(ctx context.Context, input *importListIDInput) (*importListSyncOutput, error) {
		result := svc.SyncOne(ctx, input.ID)
		return &importListSyncOutput{Body: &result}, nil
	})

	// POST /api/v1/importlists/preview
	huma.Register(api, huma.Operation{
		OperationID: "preview-import-list",
		Method:      http.MethodPost,
		Path:        "/api/v1/importlists/preview",
		Summary:     "Preview items from an import list source",
		Tags:        tags,
	}, func(ctx context.Context, input *importListPreviewInput) (*importListPreviewOutput, error) {
		items, err := svc.Preview(ctx, input.Body.Kind, input.Body.Settings)
		if err != nil {
			return nil, huma.NewError(http.StatusBadRequest, "preview failed", err)
		}
		return &importListPreviewOutput{Body: items}, nil
	})

	// ── Exclusions ────────────────────────────────────────────────────────────

	exclTags := []string{"Import Exclusions"}

	// GET /api/v1/importlists/exclusions
	huma.Register(api, huma.Operation{
		OperationID: "list-import-exclusions",
		Method:      http.MethodGet,
		Path:        "/api/v1/importlists/exclusions",
		Summary:     "List import exclusions",
		Tags:        exclTags,
	}, func(ctx context.Context, _ *struct{}) (*exclusionListOutput, error) {
		excls, err := svc.ListExclusions(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list exclusions", err)
		}
		bodies := make([]*exclusionBody, len(excls))
		for i, e := range excls {
			bodies[i] = &exclusionBody{
				ID:        e.ID,
				TMDbID:    e.TMDbID,
				Title:     e.Title,
				Year:      e.Year,
				CreatedAt: e.CreatedAt,
			}
		}
		return &exclusionListOutput{Body: bodies}, nil
	})

	// POST /api/v1/importlists/exclusions
	huma.Register(api, huma.Operation{
		OperationID:   "create-import-exclusion",
		Method:        http.MethodPost,
		Path:          "/api/v1/importlists/exclusions",
		Summary:       "Add series to import exclusion list",
		Tags:          exclTags,
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, input *exclusionCreateInput) (*exclusionCreateOutput, error) {
		e, err := svc.CreateExclusion(ctx, input.Body.TMDbID, input.Body.Title, input.Body.Year)
		if err != nil {
			return nil, huma.NewError(http.StatusBadRequest, "failed to create exclusion", err)
		}
		return &exclusionCreateOutput{Body: &exclusionBody{
			ID:        e.ID,
			TMDbID:    e.TMDbID,
			Title:     e.Title,
			Year:      e.Year,
			CreatedAt: e.CreatedAt,
		}}, nil
	})

	// DELETE /api/v1/importlists/exclusions/{id}
	huma.Register(api, huma.Operation{
		OperationID:   "delete-import-exclusion",
		Method:        http.MethodDelete,
		Path:          "/api/v1/importlists/exclusions/{id}",
		Summary:       "Remove import exclusion",
		Tags:          exclTags,
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *exclusionDeleteInput) (*exclusionDeleteOutput, error) {
		if err := svc.DeleteExclusion(ctx, input.ID); err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to delete exclusion", err)
		}
		return nil, nil
	})
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func importListToBody(c importlist.Config) *importListBody {
	return &importListBody{
		ID:               c.ID,
		Name:             c.Name,
		Kind:             c.Kind,
		Enabled:          c.Enabled,
		Settings:         c.Settings,
		SearchOnAdd:      c.SearchOnAdd,
		Monitor:          c.Monitor,
		MonitorType:      c.MonitorType,
		QualityProfileID: c.QualityProfileID,
		LibraryID:        c.LibraryID,
		CreatedAt:        c.CreatedAt,
		UpdatedAt:        c.UpdatedAt,
	}
}

func importListInputToRequest(b importListCreateBody) importlist.CreateRequest {
	return importlist.CreateRequest{
		Name:             b.Name,
		Kind:             b.Kind,
		Enabled:          b.Enabled,
		Settings:         b.Settings,
		SearchOnAdd:      b.SearchOnAdd,
		Monitor:          b.Monitor,
		MonitorType:      b.MonitorType,
		QualityProfileID: b.QualityProfileID,
		LibraryID:        b.LibraryID,
	}
}
