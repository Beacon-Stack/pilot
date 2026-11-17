package v1

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pilot/internal/core/releaseprofile"
)

type releaseProfileBody struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Enabled   bool     `json:"enabled"`
	Required  []string `json:"required"   doc:"Must contain: a release must match at least one (empty = no requirement)"`
	Ignored   []string `json:"ignored"    doc:"Must not contain: a release must match none"`
	IndexerID string   `json:"indexer_id" doc:"Limit to one indexer; empty = all indexers"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

type releaseProfileInput struct {
	Name      string   `json:"name"                 minLength:"1"`
	Enabled   *bool    `json:"enabled,omitempty"    doc:"Default true"`
	Required  []string `json:"required,omitempty"   doc:"Terms; /pattern/ for a regular expression"`
	Ignored   []string `json:"ignored,omitempty"    doc:"Terms; /pattern/ for a regular expression"`
	IndexerID string   `json:"indexer_id,omitempty" doc:"Limit to one indexer; empty = all indexers"`
}

func (in releaseProfileInput) toRequest() releaseprofile.Request {
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	return releaseprofile.Request{
		Name:      in.Name,
		Enabled:   enabled,
		Required:  in.Required,
		Ignored:   in.Ignored,
		IndexerID: in.IndexerID,
	}
}

func releaseProfileToBody(p releaseprofile.Profile) *releaseProfileBody {
	return &releaseProfileBody{
		ID:        p.ID,
		Name:      p.Name,
		Enabled:   p.Enabled,
		Required:  p.Required,
		Ignored:   p.Ignored,
		IndexerID: p.IndexerID,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
}

type releaseProfileOutput struct{ Body *releaseProfileBody }

type releaseProfileIDInput struct {
	ID string `path:"id"`
}

// releaseProfileError maps service errors to HTTP errors.
func releaseProfileError(action string, err error) error {
	var verr *releaseprofile.ValidationError
	switch {
	case errors.Is(err, releaseprofile.ErrNotFound):
		return huma.Error404NotFound("release profile not found")
	case errors.As(err, &verr):
		return huma.Error422UnprocessableEntity(verr.Msg)
	default:
		return huma.NewError(http.StatusInternalServerError, "failed to "+action+" release profile", err)
	}
}

// RegisterReleaseProfileRoutes registers CRUD for release profiles.
func RegisterReleaseProfileRoutes(api huma.API, svc *releaseprofile.Service) {
	huma.Register(api, huma.Operation{
		OperationID: "list-release-profiles",
		Method:      http.MethodGet,
		Path:        "/api/v1/release-profiles",
		Summary:     "List release profiles",
		Tags:        []string{"Release Profiles"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []*releaseProfileBody }, error) {
		profiles, err := svc.List(ctx)
		if err != nil {
			return nil, releaseProfileError("list", err)
		}
		out := make([]*releaseProfileBody, len(profiles))
		for i, p := range profiles {
			out[i] = releaseProfileToBody(p)
		}
		return &struct{ Body []*releaseProfileBody }{Body: out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "create-release-profile",
		Method:        http.MethodPost,
		Path:          "/api/v1/release-profiles",
		Summary:       "Create a release profile",
		Tags:          []string{"Release Profiles"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, input *struct{ Body releaseProfileInput }) (*releaseProfileOutput, error) {
		p, err := svc.Create(ctx, input.Body.toRequest())
		if err != nil {
			return nil, releaseProfileError("create", err)
		}
		return &releaseProfileOutput{Body: releaseProfileToBody(p)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-release-profile",
		Method:      http.MethodGet,
		Path:        "/api/v1/release-profiles/{id}",
		Summary:     "Get a release profile",
		Tags:        []string{"Release Profiles"},
	}, func(ctx context.Context, input *releaseProfileIDInput) (*releaseProfileOutput, error) {
		p, err := svc.Get(ctx, input.ID)
		if err != nil {
			return nil, releaseProfileError("get", err)
		}
		return &releaseProfileOutput{Body: releaseProfileToBody(p)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-release-profile",
		Method:      http.MethodPut,
		Path:        "/api/v1/release-profiles/{id}",
		Summary:     "Update a release profile",
		Tags:        []string{"Release Profiles"},
	}, func(ctx context.Context, input *struct {
		ID   string `path:"id"`
		Body releaseProfileInput
	}) (*releaseProfileOutput, error) {
		p, err := svc.Update(ctx, input.ID, input.Body.toRequest())
		if err != nil {
			return nil, releaseProfileError("update", err)
		}
		return &releaseProfileOutput{Body: releaseProfileToBody(p)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "delete-release-profile",
		Method:        http.MethodDelete,
		Path:          "/api/v1/release-profiles/{id}",
		Summary:       "Delete a release profile",
		Tags:          []string{"Release Profiles"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *releaseProfileIDInput) (*struct{}, error) {
		if err := svc.Delete(ctx, input.ID); err != nil {
			return nil, releaseProfileError("delete", err)
		}
		return nil, nil
	})
}
