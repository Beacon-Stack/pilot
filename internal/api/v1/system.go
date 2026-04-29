package v1

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pilot/internal/appinfo"
	"github.com/beacon-stack/pilot/internal/config"
	"github.com/beacon-stack/pilot/internal/version"
)

// systemStatus holds the shape of the status response body.
type systemStatus struct {
	AppName       string `json:"app_name"               doc:"Application name"`
	Version       string `json:"version"                doc:"Build version"`
	BuildTime     string `json:"build_time"             doc:"UTC build timestamp"`
	GoVersion     string `json:"go_version"             doc:"Go runtime version"`
	DBType        string `json:"db_type"                doc:"Active database driver"`
	DBPath        string `json:"db_path,omitempty"      doc:"SQLite database file path (sqlite only)"`
	UptimeSeconds int64  `json:"uptime_seconds"         doc:"Seconds since startup"`
	StartTime     string `json:"start_time"             doc:"UTC server start time"`
}

type systemStatusOutput struct {
	Body *systemStatus
}

// systemConfigBody is the response body for GET /api/v1/system/config.
type systemConfigBody struct {
	APIKey        string `json:"api_key"                  doc:"Masked API key"`
	ConfigFile    string `json:"config_file,omitempty"    doc:"Path of the loaded config file, if any"`
	TMDBKeySource string `json:"tmdb_key_source"          doc:"Source of the TMDB key: 'default' (baked-in), 'custom' (user override), or 'none' (unset)"`
}

type systemConfigOutput struct {
	Body *systemConfigBody
}

// updateConfigInput is the request body for PUT /api/v1/system/config.
type updateConfigInput struct {
	Body struct {
		TMDBAPIKey string `json:"tmdb_api_key,omitempty" doc:"TMDB API key to persist"`
	}
}

// updateConfigResult is the response body for PUT /api/v1/system/config.
type updateConfigResult struct {
	Saved      bool   `json:"saved"`
	ConfigFile string `json:"config_file"`
}

type updateConfigOutput struct {
	Body *updateConfigResult
}

// tmdbKeySource collapses the (configured, isDefault) pair into a single
// string so the UI doesn't have to encode the precedence rules itself.
//
//	"none"    → no key set (lookup will 503)
//	"default" → using the build-time baked-in key
//	"custom"  → user provided their own override
func tmdbKeySource(configured, isDefault bool) string {
	if !configured {
		return "none"
	}
	if isDefault {
		return "default"
	}
	return "custom"
}

// RegisterSystemRoutes registers the /api/v1/system/* endpoints.
func RegisterSystemRoutes(api huma.API, startTime time.Time, dbType, dbPath, configFile, apiKey string, tmdbKeyConfigured, tmdbKeyIsDefault bool, logger *slog.Logger) {
	huma.Register(api, huma.Operation{
		OperationID: "get-system-status",
		Method:      "GET",
		Path:        "/api/v1/system/status",
		Summary:     "Get system status",
		Description: "Returns runtime information about the server.",
		Tags:        []string{"System"},
	}, func(ctx context.Context, _ *struct{}) (*systemStatusOutput, error) {
		return &systemStatusOutput{
			Body: &systemStatus{
				AppName:       appinfo.AppName,
				Version:       version.Version,
				BuildTime:     version.BuildTime,
				GoVersion:     version.GoVersion(),
				DBType:        dbType,
				DBPath:        dbPath,
				UptimeSeconds: int64(time.Since(startTime).Seconds()),
				StartTime:     startTime.UTC().Format(time.RFC3339),
			},
		}, nil
	})

	// GET /api/v1/system/config — surface what is and isn't configured.
	huma.Register(api, huma.Operation{
		OperationID: "get-system-config",
		Method:      http.MethodGet,
		Path:        "/api/v1/system/config",
		Summary:     "Get system configuration status",
		Tags:        []string{"System"},
	}, func(ctx context.Context, _ *struct{}) (*systemConfigOutput, error) {
		maskedKey := apiKey
		if len(apiKey) > 4 {
			maskedKey = apiKey[:4] + "****"
		}
		return &systemConfigOutput{Body: &systemConfigBody{
			APIKey:        maskedKey,
			ConfigFile:    configFile,
			TMDBKeySource: tmdbKeySource(tmdbKeyConfigured, tmdbKeyIsDefault),
		}}, nil
	})

	// GET /api/v1/system/config/apikey — reveal the full API key on explicit request.
	huma.Register(api, huma.Operation{
		OperationID: "get-api-key",
		Method:      http.MethodGet,
		Path:        "/api/v1/system/config/apikey",
		Summary:     "Reveal the full API key",
		Tags:        []string{"System"},
	}, func(_ context.Context, _ *struct{}) (*struct {
		Body struct {
			APIKey string `json:"api_key"`
		}
	}, error) {
		return &struct {
			Body struct {
				APIKey string `json:"api_key"`
			}
		}{
			Body: struct {
				APIKey string `json:"api_key"`
			}{APIKey: apiKey},
		}, nil
	})

	// PUT /api/v1/system/config — update config values.
	huma.Register(api, huma.Operation{
		OperationID: "update-system-config",
		Method:      http.MethodPut,
		Path:        "/api/v1/system/config",
		Summary:     "Update system configuration",
		Tags:        []string{"System"},
	}, func(ctx context.Context, input *updateConfigInput) (*updateConfigOutput, error) {
		if input.Body.TMDBAPIKey == "" {
			return nil, huma.NewError(http.StatusBadRequest, "no config values provided")
		}

		writePath, err := config.WriteConfigKey(configFile, "tmdb.api_key", input.Body.TMDBAPIKey)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to write TMDB config", err)
		}

		return &updateConfigOutput{Body: &updateConfigResult{
			Saved:      true,
			ConfigFile: writePath,
		}}, nil
	})
}
