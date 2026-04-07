package v1

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-media/pilot/internal/core/show"
	dbsqlite "github.com/beacon-media/pilot/internal/db/generated/sqlite"
	"github.com/beacon-media/pilot/pkg/plugin"
)

// ── Response body shapes ──────────────────────────────────────────────────────

type episodeFileBody struct {
	ID         string         `json:"id"           doc:"Episode file UUID"`
	EpisodeID  string         `json:"episode_id"   doc:"Parent episode UUID"`
	SeriesID   string         `json:"series_id"    doc:"Parent series UUID"`
	Path       string         `json:"path"         doc:"Absolute path on disk"`
	SizeBytes  int64          `json:"size_bytes"   doc:"File size in bytes"`
	Quality    plugin.Quality `json:"quality"      doc:"Quality metadata"`
	ImportedAt string         `json:"imported_at"  doc:"ISO-8601 timestamp of import"`
	IndexedAt  string         `json:"indexed_at"   doc:"ISO-8601 timestamp of last index"`
}

type renamePreviewBody struct {
	FileID  string `json:"file_id"  doc:"Episode file UUID"`
	OldPath string `json:"old_path" doc:"Current path on disk"`
	NewPath string `json:"new_path" doc:"Proposed path after rename"`
}

// ── Input shapes ──────────────────────────────────────────────────────────────

type listEpisodeFilesInput struct {
	ID string `path:"id" doc:"Series UUID"`
}

type deleteEpisodeFileInput struct {
	FileID         string `path:"fileId"          doc:"Episode file UUID"`
	DeleteFromDisk bool   `query:"delete_from_disk" doc:"Also remove the file from disk"`
}

type renameSeriesInput struct {
	ID     string `path:"id"       doc:"Series UUID"`
	DryRun bool   `query:"dry_run" doc:"Preview renames without applying them"`
}

// ── Output shapes ─────────────────────────────────────────────────────────────

type listEpisodeFilesOutput struct {
	Body []*episodeFileBody
}

type renameSeriesOutput struct {
	Body struct {
		DryRun  bool                `json:"dry_run"  doc:"Whether this was a dry run"`
		Renamed []renamePreviewBody `json:"renamed"  doc:"Files that were (or would be) renamed"`
	}
}

// RegisterEpisodeFileRoutes registers file management endpoints for a series.
func RegisterEpisodeFileRoutes(api huma.API, svc *show.Service) {
	// GET /api/v1/series/{id}/files
	huma.Register(api, huma.Operation{
		OperationID: "list-episode-files",
		Method:      http.MethodGet,
		Path:        "/api/v1/series/{id}/files",
		Summary:     "List episode files for a series",
		Tags:        []string{"EpisodeFiles"},
	}, func(ctx context.Context, input *listEpisodeFilesInput) (*listEpisodeFilesOutput, error) {
		if _, err := svc.Get(ctx, input.ID); err != nil {
			if errors.Is(err, show.ErrNotFound) {
				return nil, huma.Error404NotFound("series not found")
			}
			return nil, huma.NewError(http.StatusInternalServerError, "failed to get series", err)
		}
		files, err := svc.ListFiles(ctx, input.ID)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list episode files", err)
		}
		bodies := make([]*episodeFileBody, len(files))
		for i, f := range files {
			bodies[i] = episodeFileToBody(f)
		}
		return &listEpisodeFilesOutput{Body: bodies}, nil
	})

	// DELETE /api/v1/episodefiles/{fileId}
	huma.Register(api, huma.Operation{
		OperationID:   "delete-episode-file",
		Method:        http.MethodDelete,
		Path:          "/api/v1/episodefiles/{fileId}",
		Summary:       "Delete an episode file record, optionally removing it from disk",
		Tags:          []string{"EpisodeFiles"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *deleteEpisodeFileInput) (*struct{}, error) {
		if err := svc.DeleteFile(ctx, input.FileID, input.DeleteFromDisk); err != nil {
			if errors.Is(err, show.ErrFileNotFound) {
				return nil, huma.Error404NotFound("episode file not found")
			}
			return nil, huma.NewError(http.StatusInternalServerError, "failed to delete episode file", err)
		}
		return nil, nil
	})

	// POST /api/v1/series/{id}/rename
	huma.Register(api, huma.Operation{
		OperationID: "rename-series-files",
		Method:      http.MethodPost,
		Path:        "/api/v1/series/{id}/rename",
		Summary:     "Rename episode files for a series to the configured format",
		Tags:        []string{"EpisodeFiles"},
	}, func(ctx context.Context, input *renameSeriesInput) (*renameSeriesOutput, error) {
		if _, err := svc.Get(ctx, input.ID); err != nil {
			if errors.Is(err, show.ErrNotFound) {
				return nil, huma.Error404NotFound("series not found")
			}
			return nil, huma.NewError(http.StatusInternalServerError, "failed to get series", err)
		}
		items, err := svc.RenameFiles(ctx, input.ID, input.DryRun)
		if err != nil {
			return nil, huma.NewError(http.StatusUnprocessableEntity, "rename failed", err)
		}
		out := &renameSeriesOutput{}
		out.Body.DryRun = input.DryRun
		out.Body.Renamed = make([]renamePreviewBody, len(items))
		for i, item := range items {
			out.Body.Renamed[i] = renamePreviewBody{
				FileID:  item.FileID,
				OldPath: item.OldPath,
				NewPath: item.NewPath,
			}
		}
		return out, nil
	})
}

// episodeFileToBody converts a DB model to the API response shape.
func episodeFileToBody(f dbsqlite.EpisodeFile) *episodeFileBody {
	var q plugin.Quality
	_ = json.Unmarshal([]byte(f.QualityJson), &q)

	return &episodeFileBody{
		ID:         f.ID,
		EpisodeID:  f.EpisodeID,
		SeriesID:   f.SeriesID,
		Path:       f.Path,
		SizeBytes:  f.SizeBytes,
		Quality:    q,
		ImportedAt: f.ImportedAt,
		IndexedAt:  f.IndexedAt,
	}
}
