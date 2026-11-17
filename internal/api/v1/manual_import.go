package v1

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pilot/internal/core/importer"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

type manualEpisodeBody struct {
	ID            string `json:"id"`
	SeasonNumber  int    `json:"season_number"`
	EpisodeNumber int    `json:"episode_number"`
	Title         string `json:"title"`
}

type manualCandidateBody struct {
	Path        string              `json:"path"`
	Size        int64               `json:"size"`
	SeriesID    string              `json:"series_id"    doc:"Empty when no series matched"`
	SeriesTitle string              `json:"series_title"`
	Season      int                 `json:"season"`
	Episodes    []manualEpisodeBody `json:"episodes"`
	Quality     plugin.Quality      `json:"quality"`
	Languages   []string            `json:"languages"`
	Rejections  []string            `json:"rejections"   doc:"Why this file may not be what you want: sample, unknown_series, unknown_episode, already_imported, existing_file_is_equal_or_better"`
}

type manualScanInput struct {
	Body struct {
		Path     string `json:"path"                minLength:"1" doc:"Absolute path to a folder or a video file on the server"`
		SeriesID string `json:"series_id,omitempty" doc:"Propose every file for this series"`
	}
}

type manualImportItemBody struct {
	Path       string         `json:"path"        minLength:"1"`
	SeriesID   string         `json:"series_id"   minLength:"1"`
	EpisodeIDs []string       `json:"episode_ids" minItems:"1"`
	Quality    plugin.Quality `json:"quality,omitempty" doc:"Defaults to the quality parsed from the file name"`
}

type manualImportInput struct {
	Body struct {
		Items []manualImportItemBody `json:"items" minItems:"1" maxItems:"2000"`
		Mode  string                 `json:"mode"  enum:"copy,move" doc:"copy keeps the source (hardlink or copy); move removes it after importing"`
	}
}

type manualImportResultBody struct {
	Path  string `json:"path"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// RegisterManualImportRoutes registers the manual import endpoints.
func RegisterManualImportRoutes(api huma.API, svc *importer.Service) {
	huma.Register(api, huma.Operation{
		OperationID: "manual-import-scan",
		Method:      http.MethodPost,
		Path:        "/api/v1/manual-import/scan",
		Summary:     "List video files in a folder with a proposed series, episodes and quality",
		Description: "Changes nothing. Review the candidates, then POST the ones to import to /api/v1/manual-import.",
		Tags:        []string{"Manual Import"},
	}, func(ctx context.Context, input *manualScanInput) (*struct {
		Body struct {
			Candidates []manualCandidateBody `json:"candidates"`
		}
	}, error) {
		cs, err := svc.ScanManual(ctx, input.Body.Path, input.Body.SeriesID)
		if err != nil {
			// A bad path or unknown series: the request is wrong, not the server.
			return nil, huma.Error422UnprocessableEntity(err.Error())
		}
		out := &struct {
			Body struct {
				Candidates []manualCandidateBody `json:"candidates"`
			}
		}{}
		out.Body.Candidates = make([]manualCandidateBody, 0, len(cs))
		for _, c := range cs {
			eps := make([]manualEpisodeBody, 0, len(c.Episodes))
			for _, e := range c.Episodes {
				eps = append(eps, manualEpisodeBody{ID: e.ID, SeasonNumber: e.SeasonNumber, EpisodeNumber: e.EpisodeNumber, Title: e.Title})
			}
			langs, rej := c.Languages, c.Rejections
			if langs == nil {
				langs = []string{}
			}
			if rej == nil {
				rej = []string{}
			}
			out.Body.Candidates = append(out.Body.Candidates, manualCandidateBody{
				Path: c.Path, Size: c.Size, SeriesID: c.SeriesID, SeriesTitle: c.SeriesTitle,
				Season: c.Season, Episodes: eps, Quality: c.Quality, Languages: langs, Rejections: rej,
			})
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "manual-import",
		Method:      http.MethodPost,
		Path:        "/api/v1/manual-import",
		Summary:     "Import chosen files into the library",
		Description: "Each item is imported like a download (renaming, free space, recycle bin and replaced files all apply). Results are per item.",
		Tags:        []string{"Manual Import"},
	}, func(ctx context.Context, input *manualImportInput) (*struct {
		Body struct {
			Results []manualImportResultBody `json:"results"`
		}
	}, error) {
		items := make([]importer.ManualItem, 0, len(input.Body.Items))
		for _, it := range input.Body.Items {
			items = append(items, importer.ManualItem{Path: it.Path, SeriesID: it.SeriesID, EpisodeIDs: it.EpisodeIDs, Quality: it.Quality})
		}
		res := svc.ImportManual(ctx, items, input.Body.Mode == "move")
		out := &struct {
			Body struct {
				Results []manualImportResultBody `json:"results"`
			}
		}{}
		out.Body.Results = make([]manualImportResultBody, 0, len(res))
		for _, r := range res {
			out.Body.Results = append(out.Body.Results, manualImportResultBody{Path: r.Path, OK: r.Error == "", Error: r.Error})
		}
		return out, nil
	})
}
