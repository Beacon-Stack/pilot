package v1

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-media/pilot/internal/core/mediamanagement"
)

// ── Response / request shapes ─────────────────────────────────────────────────

type mediaManagementBody struct {
	RenameEpisodes           bool   `json:"rename_episodes"            doc:"Rename episode files on import"`
	StandardEpisodeFormat    string `json:"standard_episode_format"    doc:"Naming format for standard episodes"`
	DailyEpisodeFormat       string `json:"daily_episode_format"       doc:"Naming format for daily/date-based episodes"`
	AnimeEpisodeFormat       string `json:"anime_episode_format"       doc:"Naming format for anime episodes"`
	SeriesFolderFormat       string `json:"series_folder_format"       doc:"Folder naming format for series"`
	SeasonFolderFormat       string `json:"season_folder_format"       doc:"Folder naming format for seasons"`
	ColonReplacement         string `json:"colon_replacement"          doc:"How to handle colons in titles: delete, dash, space-dash, smart"`
	ImportExtraFiles         bool   `json:"import_extra_files"         doc:"Copy extra files (subtitles, NFOs) alongside the video"`
	ExtraFileExtensions      string `json:"extra_file_extensions"      doc:"Comma-separated list of extra file extensions to import"`
	UnmonitorDeletedEpisodes bool   `json:"unmonitor_deleted_episodes" doc:"Unmonitor episodes whose files are deleted from disk"`
}

type mediaManagementOutput struct {
	Body *mediaManagementBody
}

type mediaManagementInput struct {
	Body *mediaManagementBody
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func settingsToBody(s mediamanagement.Settings) *mediaManagementBody {
	return &mediaManagementBody{
		RenameEpisodes:           s.RenameEpisodes,
		StandardEpisodeFormat:    s.StandardEpisodeFormat,
		DailyEpisodeFormat:       s.DailyEpisodeFormat,
		AnimeEpisodeFormat:       s.AnimeEpisodeFormat,
		SeriesFolderFormat:       s.SeriesFolderFormat,
		SeasonFolderFormat:       s.SeasonFolderFormat,
		ColonReplacement:         s.ColonReplacement,
		ImportExtraFiles:         s.ImportExtraFiles,
		ExtraFileExtensions:      strings.Join(s.ExtraFileExtensions, ","),
		UnmonitorDeletedEpisodes: s.UnmonitorDeletedEpisodes,
	}
}

func bodyToSettings(b *mediaManagementBody) mediamanagement.Settings {
	exts := []string{}
	for _, e := range strings.Split(b.ExtraFileExtensions, ",") {
		e = strings.TrimSpace(e)
		if e != "" {
			exts = append(exts, e)
		}
	}
	return mediamanagement.Settings{
		RenameEpisodes:           b.RenameEpisodes,
		StandardEpisodeFormat:    b.StandardEpisodeFormat,
		DailyEpisodeFormat:       b.DailyEpisodeFormat,
		AnimeEpisodeFormat:       b.AnimeEpisodeFormat,
		SeriesFolderFormat:       b.SeriesFolderFormat,
		SeasonFolderFormat:       b.SeasonFolderFormat,
		ColonReplacement:         b.ColonReplacement,
		ImportExtraFiles:         b.ImportExtraFiles,
		ExtraFileExtensions:      exts,
		UnmonitorDeletedEpisodes: b.UnmonitorDeletedEpisodes,
	}
}

// ── Route registration ────────────────────────────────────────────────────────

// RegisterMediaManagementRoutes registers /api/v1/media-management endpoints.
func RegisterMediaManagementRoutes(api huma.API, svc *mediamanagement.Service) {
	// GET /api/v1/media-management
	huma.Register(api, huma.Operation{
		OperationID: "get-media-management",
		Method:      http.MethodGet,
		Path:        "/api/v1/media-management",
		Summary:     "Get media management settings",
		Tags:        []string{"Media Management"},
	}, func(ctx context.Context, _ *struct{}) (*mediaManagementOutput, error) {
		s, err := svc.Get(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to get media management settings", err)
		}
		return &mediaManagementOutput{Body: settingsToBody(s)}, nil
	})

	// PUT /api/v1/media-management
	huma.Register(api, huma.Operation{
		OperationID: "update-media-management",
		Method:      http.MethodPut,
		Path:        "/api/v1/media-management",
		Summary:     "Update media management settings",
		Tags:        []string{"Media Management"},
	}, func(ctx context.Context, input *mediaManagementInput) (*mediaManagementOutput, error) {
		updated, err := svc.Update(ctx, bodyToSettings(input.Body))
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to update media management settings", err)
		}
		return &mediaManagementOutput{Body: settingsToBody(updated)}, nil
	})
}
