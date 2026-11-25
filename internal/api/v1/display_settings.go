package v1

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pilot/internal/availability"
)

type displaySettingsBody struct {
	CountSpecials bool `json:"count_specials" doc:"Count specials (season 0) in a series' episode totals and status"`
}

// RegisterDisplaySettingsRoutes registers the settings for how series
// progress is shown.
func RegisterDisplaySettingsRoutes(api huma.API, store *availability.DisplaySettings) {
	huma.Register(api, huma.Operation{
		OperationID: "get-display-settings",
		Method:      http.MethodGet,
		Path:        "/api/v1/display-settings",
		Summary:     "Get the display settings",
		Tags:        []string{"Settings"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body displaySettingsBody }, error) {
		d, err := store.Get(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to load the display settings", err)
		}
		return &struct{ Body displaySettingsBody }{Body: displaySettingsBody(d)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-display-settings",
		Method:      http.MethodPut,
		Path:        "/api/v1/display-settings",
		Summary:     "Update the display settings",
		Tags:        []string{"Settings"},
	}, func(ctx context.Context, input *struct{ Body displaySettingsBody }) (*struct{ Body displaySettingsBody }, error) {
		if err := store.Set(ctx, availability.Display(input.Body)); err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to save the display settings", err)
		}
		return &struct{ Body displaySettingsBody }{Body: input.Body}, nil
	})
}
