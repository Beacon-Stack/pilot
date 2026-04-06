package v1

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-media/pilot/internal/core/show"
	"github.com/beacon-media/pilot/internal/metadata/tmdbtv"
)

// ── Response body shapes ─────────────────────────────────────────────────────

type seriesBody struct {
	ID                  string     `json:"id"                               doc:"Series UUID"`
	TMDBID              int        `json:"tmdb_id"                          doc:"TMDB series ID"`
	IMDBID              string     `json:"imdb_id,omitempty"                doc:"IMDB series ID"`
	Title               string     `json:"title"                            doc:"Series title"`
	SortTitle           string     `json:"sort_title"                       doc:"Normalised sort key"`
	Year                int        `json:"year"                             doc:"First air year"`
	Overview            string     `json:"overview"                         doc:"Plot summary"`
	RuntimeMinutes      int        `json:"runtime_minutes"                  doc:"Episode runtime in minutes"`
	Genres              []string   `json:"genres"                           doc:"Genre list"`
	PosterURL           string     `json:"poster_url,omitempty"             doc:"TMDB poster path"`
	FanartURL           string     `json:"fanart_url,omitempty"             doc:"TMDB backdrop path"`
	Status              string     `json:"status"                           doc:"Series status (continuing/ended/upcoming)"`
	SeriesType          string     `json:"series_type"                      doc:"standard, anime, or daily"`
	MonitorType         string     `json:"monitor_type"                     doc:"Monitor strategy (all, future, missing, none, …)"`
	Network             string     `json:"network,omitempty"                doc:"Broadcast network"`
	AirTime             string     `json:"air_time,omitempty"               doc:"Scheduled air time"`
	Certification       string     `json:"certification,omitempty"          doc:"Age rating"`
	Monitored           bool       `json:"monitored"                        doc:"Whether the series is monitored for downloads"`
	LibraryID           string     `json:"library_id"                       doc:"Parent library UUID"`
	QualityProfileID    string     `json:"quality_profile_id"               doc:"Quality profile UUID"`
	Path                string     `json:"path,omitempty"                   doc:"Absolute path on disk"`
	EpisodeCount        int64      `json:"episode_count"                    doc:"Total episode count"`
	EpisodeFileCount    int64      `json:"episode_file_count"               doc:"Episodes with a file on disk"`
	AddedAt             time.Time  `json:"added_at"                         doc:"When the series was added"`
	UpdatedAt           time.Time  `json:"updated_at"                       doc:"When the record was last changed"`
	MetadataRefreshedAt *time.Time `json:"metadata_refreshed_at,omitempty"  doc:"When metadata was last refreshed"`
}

type seasonBody struct {
	ID           string `json:"id"             doc:"Season UUID"`
	SeriesID     string `json:"series_id"      doc:"Parent series UUID"`
	SeasonNumber int    `json:"season_number"  doc:"Season number (0 = specials)"`
	Monitored    bool   `json:"monitored"      doc:"Whether this season is monitored"`
}

type episodeBody struct {
	ID             string `json:"id"                         doc:"Episode UUID"`
	SeriesID       string `json:"series_id"                  doc:"Parent series UUID"`
	SeasonID       string `json:"season_id"                  doc:"Parent season UUID"`
	SeasonNumber   int    `json:"season_number"              doc:"Season number"`
	EpisodeNumber  int    `json:"episode_number"             doc:"Episode number"`
	AbsoluteNumber *int   `json:"absolute_number,omitempty"  doc:"Absolute episode number (anime)"`
	AirDate        string `json:"air_date,omitempty"         doc:"Air date (YYYY-MM-DD)"`
	Title          string `json:"title"                      doc:"Episode title"`
	Overview       string `json:"overview"                   doc:"Episode synopsis"`
	Monitored      bool   `json:"monitored"                  doc:"Whether this episode is monitored"`
	HasFile        bool   `json:"has_file"                   doc:"Whether a file is linked to this episode"`
}

type lookupResultBody struct {
	TMDBID        int     `json:"tmdb_id"        doc:"TMDB series ID"`
	Title         string  `json:"title"          doc:"Series title"`
	OriginalTitle string  `json:"original_title" doc:"Original-language title"`
	Overview      string  `json:"overview"       doc:"Plot summary"`
	FirstAirDate  string  `json:"first_air_date" doc:"First air date"`
	Year          int     `json:"year"           doc:"First air year"`
	PosterPath    string  `json:"poster_path"    doc:"TMDB poster path"`
	BackdropPath  string  `json:"backdrop_path"  doc:"TMDB backdrop path"`
	Popularity    float64 `json:"popularity"     doc:"TMDB popularity score"`
}

// ── Output wrappers ───────────────────────────────────────────────────────────

type seriesOutput struct {
	Body *seriesBody
}

type seriesListOutput struct {
	Body *seriesListBody
}

type seriesListBody struct {
	Series  []*seriesBody `json:"series"`
	Total   int64         `json:"total"`
	Page    int           `json:"page"`
	PerPage int           `json:"per_page"`
}

type seriesLookupOutput struct {
	Body []*lookupResultBody
}

type seriesDeleteOutput struct{}

type seasonListOutput struct {
	Body []*seasonBody
}

type episodeListOutput struct {
	Body []*episodeBody
}

type episodeOutput struct {
	Body *episodeBody
}

type seasonOutput struct {
	Body *seasonBody
}

// ── Input wrappers ────────────────────────────────────────────────────────────

type lookupSeriesInput struct {
	Body struct {
		Query  string `json:"query,omitempty"   doc:"Search string (used when tmdb_id is 0)"`
		TMDBID int    `json:"tmdb_id,omitempty" doc:"Fetch a specific series by TMDB ID"`
		Year   int    `json:"year,omitempty"    doc:"Optional year filter"`
	}
}

type addSeriesInput struct {
	Body struct {
		TMDBID           int    `json:"tmdb_id"                        doc:"TMDB series ID to add"`
		LibraryID        string `json:"library_id"                     doc:"Library UUID to place the series in"`
		QualityProfileID string `json:"quality_profile_id"             doc:"Quality profile UUID"`
		Monitored        bool   `json:"monitored"                      doc:"Whether to monitor the series for downloads"`
		MonitorType      string `json:"monitor_type,omitempty"         doc:"Monitor strategy: all, future, missing, none, pilot, first_season, last_season, existing (default: all)"`
		SeriesType       string `json:"series_type,omitempty"          doc:"standard, anime, or daily (default: standard)"`
	}
}

type listSeriesInput struct {
	LibraryID string `query:"library_id"`
	Page      int    `query:"page"     default:"1"`
	PerPage   int    `query:"per_page" default:"50"`
}

type getSeriesInput struct {
	ID string `path:"id"`
}

type updateSeriesInput struct {
	ID   string `path:"id"`
	Body struct {
		Title            string `json:"title,omitempty"                doc:"Updated title"`
		Monitored        bool   `json:"monitored"                      doc:"Whether to monitor the series"`
		LibraryID        string `json:"library_id,omitempty"           doc:"Library UUID"`
		QualityProfileID string `json:"quality_profile_id,omitempty"   doc:"Quality profile UUID"`
		SeriesType       string `json:"series_type,omitempty"          doc:"standard, anime, or daily"`
		Path             string `json:"path,omitempty"                 doc:"Absolute path on disk"`
	}
}

type deleteSeriesInput struct {
	ID string `path:"id"`
}

type getSeasonListInput struct {
	ID string `path:"id"`
}

type getEpisodeListInput struct {
	ID           string `path:"id"`
	SeasonNumber int    `path:"seasonNumber"`
}

type updateEpisodeInput struct {
	ID   string `path:"id"`
	Body struct {
		Monitored bool `json:"monitored" doc:"Whether this episode is monitored"`
	}
}

type updateSeasonInput struct {
	ID   string `path:"id"`
	Body struct {
		Monitored bool `json:"monitored" doc:"Whether this season (and all its episodes) is monitored"`
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func seriesToBody(s show.Series) *seriesBody {
	return &seriesBody{
		ID:                  s.ID,
		TMDBID:              s.TMDBID,
		IMDBID:              s.IMDBID,
		Title:               s.Title,
		SortTitle:           s.SortTitle,
		Year:                s.Year,
		Overview:            s.Overview,
		RuntimeMinutes:      s.RuntimeMinutes,
		Genres:              s.Genres,
		PosterURL:           s.PosterURL,
		FanartURL:           s.FanartURL,
		Status:              s.Status,
		SeriesType:          s.SeriesType,
		MonitorType:         s.MonitorType,
		Network:             s.Network,
		AirTime:             s.AirTime,
		Certification:       s.Certification,
		Monitored:           s.Monitored,
		LibraryID:           s.LibraryID,
		QualityProfileID:    s.QualityProfileID,
		Path:                s.Path,
		EpisodeCount:        s.EpisodeCount,
		EpisodeFileCount:    s.EpisodeFileCount,
		AddedAt:             s.AddedAt,
		UpdatedAt:           s.UpdatedAt,
		MetadataRefreshedAt: s.MetadataRefreshedAt,
	}
}

func seasonToBody(s show.Season) *seasonBody {
	return &seasonBody{
		ID:           s.ID,
		SeriesID:     s.SeriesID,
		SeasonNumber: s.SeasonNumber,
		Monitored:    s.Monitored,
	}
}

func episodeToBody(e show.Episode) *episodeBody {
	return &episodeBody{
		ID:             e.ID,
		SeriesID:       e.SeriesID,
		SeasonID:       e.SeasonID,
		SeasonNumber:   e.SeasonNumber,
		EpisodeNumber:  e.EpisodeNumber,
		AbsoluteNumber: e.AbsoluteNumber,
		AirDate:        e.AirDate,
		Title:          e.Title,
		Overview:       e.Overview,
		Monitored:      e.Monitored,
		HasFile:        e.HasFile,
	}
}

func lookupResultToBody(r tmdbtv.SearchResult) *lookupResultBody {
	return &lookupResultBody{
		TMDBID:        r.ID,
		Title:         r.Title,
		OriginalTitle: r.OriginalTitle,
		Overview:      r.Overview,
		FirstAirDate:  r.FirstAirDate,
		Year:          r.Year,
		PosterPath:    r.PosterPath,
		BackdropPath:  r.BackdropPath,
		Popularity:    r.Popularity,
	}
}

// mapShowError converts sentinel errors from the show service to Huma HTTP errors.
func mapShowError(err error, entityName string) error {
	switch {
	case errors.Is(err, show.ErrNotFound):
		return huma.Error404NotFound(entityName + " not found")
	case errors.Is(err, show.ErrAlreadyExists):
		return huma.Error409Conflict(entityName + " already in library")
	case errors.Is(err, show.ErrMetadataNotConfigured):
		return huma.NewError(http.StatusServiceUnavailable, "metadata provider not configured — set tvdb.api_key in config")
	default:
		return huma.NewError(http.StatusInternalServerError, "unexpected error", err)
	}
}

// ── Route registration ────────────────────────────────────────────────────────

// RegisterSeriesRoutes registers all /api/v1/series, /api/v1/episodes, and
// /api/v1/seasons endpoints.
func RegisterSeriesRoutes(api huma.API, showSvc *show.Service) {
	// POST /api/v1/series/lookup
	huma.Register(api, huma.Operation{
		OperationID:   "lookup-series",
		Method:        http.MethodPost,
		Path:          "/api/v1/series/lookup",
		Summary:       "Look up series metadata without adding to library",
		Tags:          []string{"Series"},
		DefaultStatus: http.StatusOK,
	}, func(ctx context.Context, input *lookupSeriesInput) (*seriesLookupOutput, error) {
		results, err := showSvc.Lookup(ctx, show.LookupRequest{
			Query:  input.Body.Query,
			TMDBID: input.Body.TMDBID,
			Year:   input.Body.Year,
		})
		if err != nil {
			return nil, mapShowError(err, "series")
		}
		bodies := make([]*lookupResultBody, len(results))
		for i, r := range results {
			bodies[i] = lookupResultToBody(r)
		}
		return &seriesLookupOutput{Body: bodies}, nil
	})

	// POST /api/v1/series
	huma.Register(api, huma.Operation{
		OperationID:   "add-series",
		Method:        http.MethodPost,
		Path:          "/api/v1/series",
		Summary:       "Add a series to the library",
		Tags:          []string{"Series"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, input *addSeriesInput) (*seriesOutput, error) {
		s, err := showSvc.Add(ctx, show.AddRequest{
			TMDBID:           input.Body.TMDBID,
			LibraryID:        input.Body.LibraryID,
			QualityProfileID: input.Body.QualityProfileID,
			Monitored:        input.Body.Monitored,
			MonitorType:      input.Body.MonitorType,
			SeriesType:       input.Body.SeriesType,
		})
		if err != nil {
			return nil, mapShowError(err, "series")
		}
		return &seriesOutput{Body: seriesToBody(s)}, nil
	})

	// GET /api/v1/series/tmdb-ids — lightweight list for "already added" detection
	huma.Register(api, huma.Operation{
		OperationID: "list-series-tmdb-ids",
		Method:      http.MethodGet,
		Path:        "/api/v1/series/tmdb-ids",
		Summary:     "List all TMDB IDs in the library",
		Tags:        []string{"Series"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []int64 }, error) {
		ids, err := showSvc.ListAllTMDBIDs(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list TMDB IDs", err)
		}
		if ids == nil {
			ids = []int64{}
		}
		return &struct{ Body []int64 }{Body: ids}, nil
	})

	// GET /api/v1/series
	huma.Register(api, huma.Operation{
		OperationID: "list-series",
		Method:      http.MethodGet,
		Path:        "/api/v1/series",
		Summary:     "List series in the library",
		Tags:        []string{"Series"},
	}, func(ctx context.Context, input *listSeriesInput) (*seriesListOutput, error) {
		result, err := showSvc.List(ctx, show.ListRequest{
			LibraryID: input.LibraryID,
			Page:      input.Page,
			PerPage:   input.PerPage,
		})
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list series", err)
		}
		bodies := make([]*seriesBody, len(result.Series))
		for i, s := range result.Series {
			bodies[i] = seriesToBody(s)
		}
		return &seriesListOutput{Body: &seriesListBody{
			Series:  bodies,
			Total:   result.Total,
			Page:    result.Page,
			PerPage: result.PerPage,
		}}, nil
	})

	// GET /api/v1/series/{id}
	huma.Register(api, huma.Operation{
		OperationID: "get-series",
		Method:      http.MethodGet,
		Path:        "/api/v1/series/{id}",
		Summary:     "Get a series by ID",
		Tags:        []string{"Series"},
	}, func(ctx context.Context, input *getSeriesInput) (*seriesOutput, error) {
		s, err := showSvc.Get(ctx, input.ID)
		if err != nil {
			return nil, mapShowError(err, "series")
		}
		return &seriesOutput{Body: seriesToBody(s)}, nil
	})

	// PUT /api/v1/series/{id}
	huma.Register(api, huma.Operation{
		OperationID: "update-series",
		Method:      http.MethodPut,
		Path:        "/api/v1/series/{id}",
		Summary:     "Update a series",
		Tags:        []string{"Series"},
	}, func(ctx context.Context, input *updateSeriesInput) (*seriesOutput, error) {
		s, err := showSvc.Update(ctx, input.ID, show.UpdateRequest{
			Title:            input.Body.Title,
			Monitored:        input.Body.Monitored,
			LibraryID:        input.Body.LibraryID,
			QualityProfileID: input.Body.QualityProfileID,
			SeriesType:       input.Body.SeriesType,
			Path:             input.Body.Path,
		})
		if err != nil {
			return nil, mapShowError(err, "series")
		}
		return &seriesOutput{Body: seriesToBody(s)}, nil
	})

	// DELETE /api/v1/series/{id}
	huma.Register(api, huma.Operation{
		OperationID:   "delete-series",
		Method:        http.MethodDelete,
		Path:          "/api/v1/series/{id}",
		Summary:       "Delete a series from the library",
		Tags:          []string{"Series"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *deleteSeriesInput) (*seriesDeleteOutput, error) {
		if err := showSvc.Delete(ctx, input.ID); err != nil {
			return nil, mapShowError(err, "series")
		}
		return &seriesDeleteOutput{}, nil
	})

	// GET /api/v1/series/{id}/seasons
	huma.Register(api, huma.Operation{
		OperationID: "list-seasons",
		Method:      http.MethodGet,
		Path:        "/api/v1/series/{id}/seasons",
		Summary:     "List seasons for a series",
		Tags:        []string{"Series"},
	}, func(ctx context.Context, input *getSeasonListInput) (*seasonListOutput, error) {
		seasons, err := showSvc.GetSeasons(ctx, input.ID)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list seasons", err)
		}
		bodies := make([]*seasonBody, len(seasons))
		for i, s := range seasons {
			bodies[i] = seasonToBody(s)
		}
		return &seasonListOutput{Body: bodies}, nil
	})

	// GET /api/v1/series/{id}/seasons/{seasonNumber}/episodes
	huma.Register(api, huma.Operation{
		OperationID: "list-episodes",
		Method:      http.MethodGet,
		Path:        "/api/v1/series/{id}/seasons/{seasonNumber}/episodes",
		Summary:     "List episodes in a season",
		Tags:        []string{"Series"},
	}, func(ctx context.Context, input *getEpisodeListInput) (*episodeListOutput, error) {
		// Resolve the season ID by matching season_number within the series.
		seasons, err := showSvc.GetSeasons(ctx, input.ID)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to fetch seasons", err)
		}
		seasonID := ""
		for _, s := range seasons {
			if s.SeasonNumber == input.SeasonNumber {
				seasonID = s.ID
				break
			}
		}
		if seasonID == "" {
			return nil, huma.Error404NotFound("season not found")
		}

		episodes, err := showSvc.GetEpisodes(ctx, seasonID)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list episodes", err)
		}
		bodies := make([]*episodeBody, len(episodes))
		for i, e := range episodes {
			bodies[i] = episodeToBody(e)
		}
		return &episodeListOutput{Body: bodies}, nil
	})

	// PUT /api/v1/episodes/{id}
	huma.Register(api, huma.Operation{
		OperationID: "update-episode",
		Method:      http.MethodPut,
		Path:        "/api/v1/episodes/{id}",
		Summary:     "Update episode monitored state",
		Tags:        []string{"Episodes"},
	}, func(ctx context.Context, input *updateEpisodeInput) (*episodeOutput, error) {
		if err := showSvc.UpdateEpisodeMonitored(ctx, input.ID, input.Body.Monitored); err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to update episode", err)
		}
		// Huma requires a Body; return a minimal acknowledgement with the ID.
		return &episodeOutput{Body: &episodeBody{ID: input.ID, Monitored: input.Body.Monitored}}, nil
	})

	// PUT /api/v1/seasons/{id}
	huma.Register(api, huma.Operation{
		OperationID: "update-season",
		Method:      http.MethodPut,
		Path:        "/api/v1/seasons/{id}",
		Summary:     "Update season monitored state (cascades to episodes)",
		Tags:        []string{"Seasons"},
	}, func(ctx context.Context, input *updateSeasonInput) (*seasonOutput, error) {
		if err := showSvc.UpdateSeasonMonitored(ctx, input.ID, input.Body.Monitored); err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to update season", err)
		}
		return &seasonOutput{Body: &seasonBody{ID: input.ID, Monitored: input.Body.Monitored}}, nil
	})
}
