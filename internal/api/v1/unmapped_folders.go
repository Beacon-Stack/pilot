package v1

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pilot/internal/core/show"
	"github.com/beacon-stack/pilot/internal/core/unmapped"
)

type unmappedFolderBody struct {
	ID          string `json:"id"`
	LibraryID   string `json:"library_id"`
	Path        string `json:"path"`
	Name        string `json:"name"`
	GuessTMDBID int    `json:"guess_tmdb_id" doc:"Best TMDB match for the folder name; 0 when none"`
	GuessTitle  string `json:"guess_title"`
	GuessYear   int    `json:"guess_year"`
	PosterPath  string `json:"poster_path"   doc:"TMDB poster path of the guess"`
	Ignored     bool   `json:"ignored"`
	FirstSeenAt string `json:"first_seen_at"`
}

// RegisterUnmappedFolderRoutes registers the library-import review
// endpoints. scan, when not nil, starts a library scan after a folder is
// confirmed so its files are picked up right away.
func RegisterUnmappedFolderRoutes(api huma.API, svc *unmapped.Service, scan func(), logger *slog.Logger) {
	const tag = "Library Import"

	huma.Register(api, huma.Operation{
		OperationID: "list-unmapped-folders",
		Method:      http.MethodGet,
		Path:        "/api/v1/library/unmapped-folders",
		Summary:     "Folders in a library that no series claims",
		Tags:        []string{tag},
	}, func(ctx context.Context, input *struct {
		IncludeIgnored bool `query:"include_ignored" default:"false"`
	}) (*struct{ Body []unmappedFolderBody }, error) {
		fs, err := svc.List(ctx, input.IncludeIgnored)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list unmapped folders", err)
		}
		out := make([]unmappedFolderBody, 0, len(fs))
		for _, f := range fs {
			out = append(out, unmappedFolderBody{
				ID: f.ID, LibraryID: f.LibraryID, Path: f.Path, Name: f.Name,
				GuessTMDBID: f.GuessTMDBID, GuessTitle: f.GuessTitle, GuessYear: f.GuessYear,
				PosterPath: f.GuessPosterPath, Ignored: f.Ignored, FirstSeenAt: f.FirstSeenAt,
			})
		}
		return &struct{ Body []unmappedFolderBody }{Body: out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "add-unmapped-folder",
		Method:      http.MethodPost,
		Path:        "/api/v1/library/unmapped-folders/{id}/add",
		Summary:     "Add the series for a folder",
		Description: "Adds the guessed show, or the given tmdb_id, points it at the folder, and scans the library so its files are imported.",
		Tags:        []string{tag},
	}, func(ctx context.Context, input *struct {
		ID   string `path:"id"`
		Body struct {
			TMDBID           int    `json:"tmdb_id,omitempty"            doc:"Another show; omit to use the guess"`
			QualityProfileID string `json:"quality_profile_id,omitempty" doc:"Omit for the library's default"`
			MonitorType      string `json:"monitor_type,omitempty"       enum:"all,future,missing,none,pilot,first_season,last_season,existing" doc:"Omit for existing"`
		}
	}) (*struct {
		Body struct {
			SeriesID string `json:"series_id"`
		}
	}, error) {
		sr, err := svc.Add(ctx, input.ID, unmapped.AddRequest{
			TMDBID: input.Body.TMDBID, QualityProfileID: input.Body.QualityProfileID, MonitorType: input.Body.MonitorType,
		})
		switch {
		case errors.Is(err, unmapped.ErrNotFound):
			return nil, huma.Error404NotFound("unmapped folder not found")
		case errors.Is(err, show.ErrAlreadyExists):
			return nil, huma.Error409Conflict("that show is already in Pilot")
		case err != nil:
			return nil, huma.Error422UnprocessableEntity(err.Error())
		}
		if scan != nil {
			scan()
		}
		logger.Info("added series from unmapped folder", "series_id", sr.ID, "title", sr.Title)
		out := &struct {
			Body struct {
				SeriesID string `json:"series_id"`
			}
		}{}
		out.Body.SeriesID = sr.ID
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ignore-unmapped-folder",
		Method:      http.MethodPost,
		Path:        "/api/v1/library/unmapped-folders/{id}/ignore",
		Summary:     "Ignore a folder, or stop ignoring it",
		Tags:        []string{tag},
	}, func(ctx context.Context, input *struct {
		ID   string `path:"id"`
		Body struct {
			Ignored bool `json:"ignored"`
		}
	}) (*struct{}, error) {
		if err := svc.SetIgnored(ctx, input.ID, input.Body.Ignored); err != nil {
			if errors.Is(err, unmapped.ErrNotFound) {
				return nil, huma.Error404NotFound("unmapped folder not found")
			}
			return nil, huma.NewError(http.StatusInternalServerError, "failed to update the folder", err)
		}
		return nil, nil
	})
}
