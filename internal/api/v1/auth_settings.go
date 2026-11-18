package v1

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pilot/internal/api/middleware"
	"github.com/beacon-stack/pilot/internal/config"
)

type authSettingsBody struct {
	Required    string `json:"required"     enum:"disabled_for_local,enabled" doc:"When a login is needed: disabled_for_local (default, like Sonarr) or enabled (everywhere)"`
	Username    string `json:"username"     doc:"Login username; empty when no login is set up"`
	PasswordSet bool   `json:"password_set" doc:"Whether a password is stored"`
}

type authSettingsOutput struct {
	Body *authSettingsBody
}

type updateAuthSettingsInput struct {
	Body struct {
		Required string `json:"required" enum:"disabled_for_local,enabled" doc:"When a login is needed"`
		Username string `json:"username" doc:"Login username. Empty removes the login (only allowed with disabled_for_local)."`
		Password string `json:"password,omitempty" doc:"New password. Omit to keep the current one."`
	}
}

func toAuthSettingsBody(s middleware.AuthSettings) *authSettingsBody {
	return &authSettingsBody{
		Required:    s.Required,
		Username:    s.Username,
		PasswordSet: s.PasswordHash != "",
	}
}

// RegisterAuthSettingsRoutes registers GET/PUT /api/v1/system/auth, which
// read and change the browser login settings. Changes apply immediately
// and are written to config.yaml.
func RegisterAuthSettingsRoutes(api huma.API, auth *middleware.Auth, configFile string) {
	huma.Register(api, huma.Operation{
		OperationID: "get-auth-settings",
		Method:      http.MethodGet,
		Path:        "/api/v1/system/auth",
		Summary:     "Get authentication settings",
		Tags:        []string{"System"},
	}, func(_ context.Context, _ *struct{}) (*authSettingsOutput, error) {
		return &authSettingsOutput{Body: toAuthSettingsBody(auth.Settings())}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-auth-settings",
		Method:      http.MethodPut,
		Path:        "/api/v1/system/auth",
		Summary:     "Update authentication settings",
		Tags:        []string{"System"},
	}, func(_ context.Context, in *updateAuthSettingsInput) (*authSettingsOutput, error) {
		cur := auth.Settings()
		next := middleware.AuthSettings{
			Required:     in.Body.Required,
			Username:     strings.TrimSpace(in.Body.Username),
			PasswordHash: cur.PasswordHash,
		}
		if next.Required == "" {
			next.Required = middleware.AuthDisabledForLocal
		}

		switch {
		case next.Username == "":
			if in.Body.Password != "" {
				return nil, huma.NewError(http.StatusBadRequest, "Enter a username to go with the password.")
			}
			next.PasswordHash = ""
		case in.Body.Password != "":
			if len(in.Body.Password) < 8 {
				return nil, huma.NewError(http.StatusBadRequest, "Password must be at least 8 characters.")
			}
			hash, err := middleware.HashPassword(in.Body.Password)
			if err != nil {
				return nil, huma.NewError(http.StatusInternalServerError, "Could not hash password.", err)
			}
			next.PasswordHash = hash
		case next.PasswordHash == "":
			return nil, huma.NewError(http.StatusBadRequest, "Enter a password.")
		}
		if next.Required == middleware.AuthEnabled && !next.CredentialsConfigured() {
			return nil, huma.NewError(http.StatusBadRequest,
				"Set a username and password before requiring a login everywhere, or you'd be locked out.")
		}

		for _, kv := range [][2]string{
			{"auth.required", next.Required},
			{"auth.username", next.Username},
			{"auth.password_hash", next.PasswordHash},
		} {
			if _, err := config.WriteConfigKey(configFile, kv[0], kv[1]); err != nil {
				return nil, huma.NewError(http.StatusInternalServerError, "Could not save to config.yaml.", err)
			}
		}
		auth.SetSettings(next)
		return &authSettingsOutput{Body: toAuthSettingsBody(next)}, nil
	})
}
