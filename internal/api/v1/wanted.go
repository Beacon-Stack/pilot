package v1

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	dbsqlite "github.com/beacon-stack/pilot/internal/db/generated/sqlite"
)

// ── Request / response shapes ────────────────────────────────────────────────

type wantedEpisodeInput struct {
	Page    int `query:"page"     default:"1"   minimum:"1"`
	PerPage int `query:"per_page" default:"50"  minimum:"1" maximum:"250"`
}

type wantedEpisodeBody struct {
	EpisodeID     string `json:"episode_id"     doc:"Episode UUID"`
	SeriesID      string `json:"series_id"      doc:"Series UUID"`
	SeriesTitle   string `json:"series_title"   doc:"Series title"`
	SeasonNumber  int64  `json:"season_number"  doc:"Season number"`
	EpisodeNumber int64  `json:"episode_number" doc:"Episode number within season"`
	EpisodeTitle  string `json:"episode_title"  doc:"Episode title"`
	AirDate       string `json:"air_date"       doc:"Air date (YYYY-MM-DD)"`
	HasFile       bool   `json:"has_file"       doc:"Whether a file is linked to this episode"`
	Monitored     bool   `json:"monitored"      doc:"Whether this episode is monitored"`
}

type wantedEpisodeListBody struct {
	Episodes []*wantedEpisodeBody `json:"episodes"`
	Total    int64                `json:"total"`
	Page     int                  `json:"page"`
	PerPage  int                  `json:"per_page"`
}

type wantedEpisodeListOutput struct {
	Body *wantedEpisodeListBody
}

// RegisterWantedRoutes registers the wanted/missing and wanted/cutoff endpoints.
func RegisterWantedRoutes(api huma.API, q dbsqlite.Querier) {
	// GET /api/v1/wanted/missing — monitored episodes with no file, air_date <= today
	huma.Register(api, huma.Operation{
		OperationID: "wanted-missing",
		Method:      http.MethodGet,
		Path:        "/api/v1/wanted/missing",
		Summary:     "List monitored episodes with no file",
		Tags:        []string{"Wanted"},
	}, func(ctx context.Context, input *wantedEpisodeInput) (*wantedEpisodeListOutput, error) {
		total, err := q.CountMissingEpisodes(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to count missing episodes", err)
		}

		offset := int64((input.Page - 1) * input.PerPage)
		rows, err := q.ListMissingEpisodesWithSeries(ctx, dbsqlite.ListMissingEpisodesWithSeriesParams{
			Limit:  int64(input.PerPage),
			Offset: offset,
		})
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list missing episodes", err)
		}

		bodies := make([]*wantedEpisodeBody, 0, len(rows))
		for _, r := range rows {
			airDate := ""
			if r.AirDate != nil {
				airDate = *r.AirDate
			}
			bodies = append(bodies, &wantedEpisodeBody{
				EpisodeID:     r.ID,
				SeriesID:      r.SeriesID,
				SeriesTitle:   r.SeriesTitle,
				SeasonNumber:  r.SeasonNumber,
				EpisodeNumber: r.EpisodeNumber,
				EpisodeTitle:  r.Title,
				AirDate:       airDate,
				HasFile:       r.HasFile != 0,
				Monitored:     r.Monitored != 0,
			})
		}

		return &wantedEpisodeListOutput{Body: &wantedEpisodeListBody{
			Episodes: bodies,
			Total:    total,
			Page:     input.Page,
			PerPage:  input.PerPage,
		}}, nil
	})

	// GET /api/v1/wanted/cutoff — episodes below quality cutoff (stub)
	huma.Register(api, huma.Operation{
		OperationID: "wanted-cutoff",
		Method:      http.MethodGet,
		Path:        "/api/v1/wanted/cutoff",
		Summary:     "List episodes whose file quality is below the profile cutoff",
		Tags:        []string{"Wanted"},
	}, func(ctx context.Context, _ *struct{}) (*wantedEpisodeListOutput, error) {
		// Cutoff logic is not yet implemented — quality profile cutoff evaluation
		// requires linking episode files to quality profiles. Return empty for now.
		return &wantedEpisodeListOutput{Body: &wantedEpisodeListBody{
			Episodes: []*wantedEpisodeBody{},
			Total:    0,
			Page:     1,
			PerPage:  50,
		}}, nil
	})
}
