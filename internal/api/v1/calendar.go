package v1

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	dbsqlite "github.com/beacon-stack/pilot/internal/db/generated/sqlite"
)

// ── Request / response shapes ────────────────────────────────────────────────

type calendarInput struct {
	Start string `query:"start" doc:"Start date inclusive (YYYY-MM-DD)"`
	End   string `query:"end"   doc:"End date inclusive (YYYY-MM-DD)"`
}

type calendarEpisodeBody struct {
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

type calendarOutput struct {
	Body []*calendarEpisodeBody
}

// RegisterCalendarRoutes registers the calendar endpoint.
func RegisterCalendarRoutes(api huma.API, q dbsqlite.Querier) {
	huma.Register(api, huma.Operation{
		OperationID: "calendar-list",
		Method:      http.MethodGet,
		Path:        "/api/v1/calendar",
		Summary:     "List episodes airing in a date range",
		Tags:        []string{"Calendar"},
	}, func(ctx context.Context, input *calendarInput) (*calendarOutput, error) {
		rows, err := q.ListEpisodesByAirDateRange(ctx, dbsqlite.ListEpisodesByAirDateRangeParams{
			AirDate:   &input.Start,
			AirDate_2: &input.End,
		})
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list calendar episodes", err)
		}

		bodies := make([]*calendarEpisodeBody, 0, len(rows))
		for _, r := range rows {
			airDate := ""
			if r.AirDate != nil {
				airDate = *r.AirDate
			}
			bodies = append(bodies, &calendarEpisodeBody{
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

		return &calendarOutput{Body: bodies}, nil
	})
}
