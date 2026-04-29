package v1

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pilot/internal/core/provider"
)

// Providers API — rotates the baked-in TMDB / Trakt keys without
// rebuilding the image. Read endpoints return a redacted preview and
// the source ("default" | "override"); the actual key value is never
// sent back through any endpoint.
//
// Changes take effect on the next Pilot restart — the metadata client
// is constructed once at startup. Documented in the UI copy.

type providerStatusInput struct {
	Name string `path:"name" doc:"Provider name: tmdb | trakt"`
}

type providerStatusOutput struct {
	Body *providerStatusBody
}

type providerStatusBody struct {
	Name        string `json:"name"`
	Source      string `json:"source"  doc:"Where the effective key came from: default (baked) or override (UI-set)"`
	Preview     string `json:"preview" doc:"Redacted key (last 3 characters visible) or empty string if unconfigured"`
	HasDefault  bool   `json:"hasDefault" doc:"True if the image was built with a baked default for this provider"`
	HasOverride bool   `json:"hasOverride" doc:"True if a Settings-UI override is currently stored"`
}

type setProviderOverrideInput struct {
	Name string `path:"name"`
	Body struct {
		Value string `json:"value" doc:"Plaintext API key to store as an override. Empty to clear (also see DELETE)."`
	}
}

// RegisterProviderRoutes wires the /api/v1/settings/providers/{name}
// endpoints onto the given Huma API.
func RegisterProviderRoutes(api huma.API, r *provider.Resolver) {
	huma.Register(api, huma.Operation{
		OperationID: "get-provider",
		Method:      http.MethodGet,
		Path:        "/api/v1/settings/providers/{name}",
		Summary:     "Get the status of a third-party provider key",
		Description: "Returns where the effective key came from (baked default or UI override) plus a redacted preview. The full key value is never returned.",
		Tags:        []string{"Settings"},
	}, func(ctx context.Context, input *providerStatusInput) (*providerStatusOutput, error) {
		return buildStatus(ctx, r, input.Name)
	})

	huma.Register(api, huma.Operation{
		OperationID: "set-provider-override",
		Method:      http.MethodPut,
		Path:        "/api/v1/settings/providers/{name}",
		Summary:     "Set a Settings-UI override for a third-party provider key",
		Description: "Stores the supplied value as an override, replacing the baked default at the next Pilot restart.",
		Tags:        []string{"Settings"},
	}, func(ctx context.Context, input *setProviderOverrideInput) (*providerStatusOutput, error) {
		if err := r.SetOverride(ctx, input.Name, input.Body.Value); err != nil {
			return nil, huma.NewError(http.StatusBadRequest, err.Error())
		}
		return buildStatus(ctx, r, input.Name)
	})

	huma.Register(api, huma.Operation{
		OperationID: "clear-provider-override",
		Method:      http.MethodDelete,
		Path:        "/api/v1/settings/providers/{name}",
		Summary:     "Clear the Settings-UI override for a third-party provider key",
		Description: "Reverts to the baked default at the next Pilot restart.",
		Tags:        []string{"Settings"},
	}, func(ctx context.Context, input *providerStatusInput) (*providerStatusOutput, error) {
		if err := r.ClearOverride(ctx, input.Name); err != nil {
			return nil, huma.NewError(http.StatusBadRequest, err.Error())
		}
		return buildStatus(ctx, r, input.Name)
	})
}

func buildStatus(ctx context.Context, r *provider.Resolver, name string) (*providerStatusOutput, error) {
	hasOverride, err := r.HasOverride(ctx, name)
	if err != nil {
		if errors.Is(err, errUnknownProvider(name)) {
			return nil, huma.NewError(http.StatusNotFound, "unknown provider: "+name)
		}
		return nil, huma.NewError(http.StatusInternalServerError, "reading provider status", err)
	}
	preview, source, err := r.Preview(ctx, name)
	if err != nil {
		return nil, huma.NewError(http.StatusInternalServerError, "previewing provider key", err)
	}
	// HasDefault flag: compare the default path directly to know if the
	// image shipped with a baked key. This doesn't leak the key itself.
	effective, _, err := r.EffectiveKey(ctx, name)
	if err != nil {
		return nil, huma.NewError(http.StatusInternalServerError, "resolving effective key", err)
	}
	hasDefault := !hasOverride && effective != ""
	return &providerStatusOutput{Body: &providerStatusBody{
		Name:        name,
		Source:      string(source),
		Preview:     preview,
		HasDefault:  hasDefault,
		HasOverride: hasOverride,
	}}, nil
}

// errUnknownProvider produces a sentinel we can match against the
// Resolver's error strings. Simpler than exporting a new error type
// from the provider package.
func errUnknownProvider(name string) error {
	return errors.New("unknown provider \"" + name + "\"")
}
