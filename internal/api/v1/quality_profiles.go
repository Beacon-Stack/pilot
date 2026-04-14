package v1

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pilot/internal/core/quality"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

// ── Request / response shapes ─────────────────────────────────────────────────

type qualityProfileBody struct {
	ID                   string           `json:"id"                        doc:"Profile UUID"`
	Name                 string           `json:"name"                      doc:"Human-readable profile name"`
	Cutoff               plugin.Quality   `json:"cutoff"                    doc:"Minimum acceptable quality"`
	Qualities            []plugin.Quality `json:"qualities"                 doc:"Ordered list of accepted qualities"`
	UpgradeAllowed       bool             `json:"upgrade_allowed"           doc:"Whether upgrades are permitted"`
	UpgradeUntil         *plugin.Quality  `json:"upgrade_until,omitempty"   doc:"Quality ceiling for upgrades"`
	MinCustomFormatScore int              `json:"min_custom_format_score"   doc:"Minimum CF score to accept a release"`
	UpgradeUntilCFScore  int              `json:"upgrade_until_cf_score"    doc:"CF score ceiling for upgrades"`
	ManagedByPulse       bool             `json:"managed_by_pulse"          doc:"True if synced from Pulse"`
}

type qualityProfileInput struct {
	Name                 string           `json:"name"                             doc:"Human-readable profile name"`
	Cutoff               plugin.Quality   `json:"cutoff"                           doc:"Minimum acceptable quality"`
	Qualities            []plugin.Quality `json:"qualities"                        doc:"Ordered list of accepted qualities"`
	UpgradeAllowed       bool             `json:"upgrade_allowed"                  doc:"Whether upgrades are permitted"`
	UpgradeUntil         *plugin.Quality  `json:"upgrade_until,omitempty"          doc:"Quality ceiling for upgrades"`
	MinCustomFormatScore *int             `json:"min_custom_format_score,omitempty" doc:"Minimum CF score (default: 0)"`
	UpgradeUntilCFScore  *int             `json:"upgrade_until_cf_score,omitempty"  doc:"CF score ceiling (default: 0)"`
}

// ── Output wrappers ───────────────────────────────────────────────────────────

type qualityProfileOutput struct {
	Body *qualityProfileBody
}

type qualityProfileListOutput struct {
	Body []*qualityProfileBody
}

type qualityProfileDeleteOutput struct{}

// ── Input wrappers ────────────────────────────────────────────────────────────

type qualityProfileCreateInput struct {
	Body qualityProfileInput
}

type qualityProfileUpdateInput struct {
	ID   string `path:"id"`
	Body qualityProfileInput
}

type qualityProfileGetInput struct {
	ID string `path:"id"`
}

type qualityProfileDeleteInput struct {
	ID string `path:"id"`
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func profileToBody(p quality.Profile) *qualityProfileBody {
	return &qualityProfileBody{
		ID:                   p.ID,
		Name:                 p.Name,
		Cutoff:               p.Cutoff,
		Qualities:            p.Qualities,
		UpgradeAllowed:       p.UpgradeAllowed,
		UpgradeUntil:         p.UpgradeUntil,
		MinCustomFormatScore: p.MinCustomFormatScore,
		UpgradeUntilCFScore:  p.UpgradeUntilCFScore,
		ManagedByPulse:       p.ManagedByPulse,
	}
}

func inputToQualityCreateRequest(in qualityProfileInput) quality.CreateRequest {
	minScore := 0
	if in.MinCustomFormatScore != nil {
		minScore = *in.MinCustomFormatScore
	}
	cfScore := 0
	if in.UpgradeUntilCFScore != nil {
		cfScore = *in.UpgradeUntilCFScore
	}
	return quality.CreateRequest{
		Name:                 in.Name,
		Cutoff:               in.Cutoff,
		Qualities:            in.Qualities,
		UpgradeAllowed:       in.UpgradeAllowed,
		UpgradeUntil:         in.UpgradeUntil,
		MinCustomFormatScore: minScore,
		UpgradeUntilCFScore:  cfScore,
	}
}

// ── Route registration ────────────────────────────────────────────────────────

// RegisterQualityProfileRoutes registers all /api/v1/quality-profiles endpoints.
func RegisterQualityProfileRoutes(api huma.API, qualitySvc *quality.Service) {
	// GET /api/v1/quality-profiles
	huma.Register(api, huma.Operation{
		OperationID: "list-quality-profiles",
		Method:      http.MethodGet,
		Path:        "/api/v1/quality-profiles",
		Summary:     "List quality profiles",
		Tags:        []string{"Quality Profiles"},
	}, func(ctx context.Context, _ *struct{}) (*qualityProfileListOutput, error) {
		profiles, err := qualitySvc.List(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list quality profiles", err)
		}
		bodies := make([]*qualityProfileBody, len(profiles))
		for i, p := range profiles {
			bodies[i] = profileToBody(p)
		}
		return &qualityProfileListOutput{Body: bodies}, nil
	})

	// POST /api/v1/quality-profiles
	huma.Register(api, huma.Operation{
		OperationID:   "create-quality-profile",
		Method:        http.MethodPost,
		Path:          "/api/v1/quality-profiles",
		Summary:       "Create a quality profile",
		Tags:          []string{"Quality Profiles"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, input *qualityProfileCreateInput) (*qualityProfileOutput, error) {
		p, err := qualitySvc.Create(ctx, inputToQualityCreateRequest(input.Body))
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to create quality profile", err)
		}
		return &qualityProfileOutput{Body: profileToBody(p)}, nil
	})

	// GET /api/v1/quality-profiles/{id}
	huma.Register(api, huma.Operation{
		OperationID: "get-quality-profile",
		Method:      http.MethodGet,
		Path:        "/api/v1/quality-profiles/{id}",
		Summary:     "Get a quality profile",
		Tags:        []string{"Quality Profiles"},
	}, func(ctx context.Context, input *qualityProfileGetInput) (*qualityProfileOutput, error) {
		p, err := qualitySvc.Get(ctx, input.ID)
		if err != nil {
			if errors.Is(err, quality.ErrNotFound) {
				return nil, huma.Error404NotFound("quality profile not found")
			}
			return nil, huma.NewError(http.StatusInternalServerError, "failed to get quality profile", err)
		}
		return &qualityProfileOutput{Body: profileToBody(p)}, nil
	})

	// PUT /api/v1/quality-profiles/{id}
	// If the profile is managed_by_pulse, this auto-detaches it first so the
	// local edit becomes a shadow. Future Pulse sync runs won't overwrite it.
	huma.Register(api, huma.Operation{
		OperationID: "update-quality-profile",
		Method:      http.MethodPut,
		Path:        "/api/v1/quality-profiles/{id}",
		Summary:     "Update a quality profile (auto-detaches from Pulse if managed)",
		Tags:        []string{"Quality Profiles"},
	}, func(ctx context.Context, input *qualityProfileUpdateInput) (*qualityProfileOutput, error) {
		existing, getErr := qualitySvc.Get(ctx, input.ID)
		if getErr == nil && existing.ManagedByPulse {
			if err := qualitySvc.DetachFromPulse(ctx, input.ID); err != nil {
				return nil, huma.NewError(http.StatusInternalServerError, "failed to detach from Pulse", err)
			}
		}

		p, err := qualitySvc.Update(ctx, input.ID, inputToQualityCreateRequest(input.Body))
		if err != nil {
			if errors.Is(err, quality.ErrNotFound) {
				return nil, huma.Error404NotFound("quality profile not found")
			}
			return nil, huma.NewError(http.StatusInternalServerError, "failed to update quality profile", err)
		}
		return &qualityProfileOutput{Body: profileToBody(p)}, nil
	})

	// POST /api/v1/quality-profiles/{id}/detach
	huma.Register(api, huma.Operation{
		OperationID:   "detach-quality-profile",
		Method:        http.MethodPost,
		Path:          "/api/v1/quality-profiles/{id}/detach",
		Summary:       "Detach a quality profile from Pulse management",
		Tags:          []string{"Quality Profiles"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *qualityProfileGetInput) (*qualityProfileDeleteOutput, error) {
		if err := qualitySvc.DetachFromPulse(ctx, input.ID); err != nil {
			if errors.Is(err, quality.ErrNotFound) {
				return nil, huma.Error404NotFound("quality profile not found")
			}
			return nil, huma.NewError(http.StatusInternalServerError, "failed to detach quality profile", err)
		}
		return &qualityProfileDeleteOutput{}, nil
	})

	// DELETE /api/v1/quality-profiles/{id}
	huma.Register(api, huma.Operation{
		OperationID:   "delete-quality-profile",
		Method:        http.MethodDelete,
		Path:          "/api/v1/quality-profiles/{id}",
		Summary:       "Delete a quality profile",
		Tags:          []string{"Quality Profiles"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *qualityProfileDeleteInput) (*qualityProfileDeleteOutput, error) {
		if err := qualitySvc.Delete(ctx, input.ID); err != nil {
			if errors.Is(err, quality.ErrNotFound) {
				return nil, huma.Error404NotFound("quality profile not found")
			}
			if errors.Is(err, quality.ErrInUse) {
				return nil, huma.Error409Conflict("quality profile is in use by one or more series or libraries")
			}
			return nil, huma.NewError(http.StatusInternalServerError, "failed to delete quality profile", err)
		}
		return &qualityProfileDeleteOutput{}, nil
	})
}
