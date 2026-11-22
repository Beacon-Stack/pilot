package v1

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pilot/internal/core/show"
	"github.com/beacon-stack/pilot/internal/metadata/tmdbtv"
)

// ── Response body shapes ─────────────────────────────────────────────────────

type discoverCastBody struct {
	ID          int    `json:"id"                     doc:"TMDB person ID"`
	Name        string `json:"name"                   doc:"Actor name"`
	Character   string `json:"character,omitempty"    doc:"Character played"`
	ProfilePath string `json:"profile_path,omitempty" doc:"TMDB profile image path"`
}

type discoverCrewBody struct {
	ID          int    `json:"id"                     doc:"TMDB person ID"`
	Name        string `json:"name"                   doc:"Crew member name"`
	Job         string `json:"job"                    doc:"Job title (e.g. Creator, Writer)"`
	ProfilePath string `json:"profile_path,omitempty" doc:"TMDB profile image path"`
}

type discoverRecommendationBody struct {
	TMDBID     int    `json:"tmdb_id"               doc:"TMDB series ID"`
	Title      string `json:"title"                 doc:"Series title"`
	Year       int    `json:"year,omitempty"        doc:"First air year"`
	PosterPath string `json:"poster_path,omitempty" doc:"TMDB poster path"`
}

type discoverSeasonBody struct {
	SeasonNumber int    `json:"season_number"      doc:"Season number (0 = specials)"`
	EpisodeCount int    `json:"episode_count"      doc:"Episodes in the season"`
	AirDate      string `json:"air_date,omitempty" doc:"Season premiere date"`
}

type discoverSeriesDetailBody struct {
	TMDBID          int                          `json:"tmdb_id"                   doc:"TMDB series ID"`
	Title           string                       `json:"title"                     doc:"Series title"`
	OriginalTitle   string                       `json:"original_title,omitempty"  doc:"Original-language title"`
	Year            int                          `json:"year,omitempty"            doc:"First air year"`
	Overview        string                       `json:"overview,omitempty"        doc:"Plot summary"`
	FirstAirDate    string                       `json:"first_air_date,omitempty"  doc:"First air date"`
	Rating          float64                      `json:"rating,omitempty"          doc:"TMDB vote average"`
	Status          string                       `json:"status,omitempty"          doc:"continuing | ended | upcoming"`
	Network         string                       `json:"network,omitempty"         doc:"Primary network"`
	Genres          []string                     `json:"genres,omitempty"          doc:"Genre names"`
	PosterPath      string                       `json:"poster_path,omitempty"     doc:"TMDB poster path"`
	BackdropPath    string                       `json:"backdrop_path,omitempty"   doc:"TMDB backdrop path"`
	Seasons         []discoverSeasonBody         `json:"seasons,omitempty"         doc:"Season summaries"`
	Cast            []discoverCastBody           `json:"cast,omitempty"            doc:"Top billed cast"`
	Crew            []discoverCrewBody           `json:"crew,omitempty"            doc:"Key crew members"`
	Recommendations []discoverRecommendationBody `json:"recommendations,omitempty" doc:"Recommended series"`
	InLibrary       bool                         `json:"in_library"                doc:"Whether the series is already in the library"`
	LibrarySeriesID string                       `json:"library_series_id,omitempty" doc:"Library series UUID, when in_library"`
}

type discoverSeriesInput struct {
	TMDBID int `path:"tmdbId" doc:"TMDB series ID"`
}

type discoverSeriesDetailOutput struct {
	Body *discoverSeriesDetailBody
}

// ── Browse list shapes ────────────────────────────────────────────────────────

type discoverListItemBody struct {
	TMDBID     int     `json:"tmdb_id"               doc:"TMDB series ID"`
	Title      string  `json:"title"                 doc:"Series title"`
	Year       int     `json:"year,omitempty"        doc:"First air year"`
	Overview   string  `json:"overview,omitempty"    doc:"Plot summary"`
	PosterPath string  `json:"poster_path,omitempty" doc:"TMDB poster path"`
	Rating     float64 `json:"rating,omitempty"      doc:"TMDB vote average"`
}

type discoverListBody struct {
	Results    []discoverListItemBody `json:"results"     doc:"Series in this page"`
	Page       int                    `json:"page"        doc:"Current page"`
	TotalPages int                    `json:"total_pages" doc:"Total pages available"`
}

type discoverListOutput struct {
	Body *discoverListBody
}

type discoverPageInput struct {
	Page int `query:"page" doc:"1-based page number" default:"1"`
}

type discoverBrowseInput struct {
	Category  string  `query:"category" default:"trending" enum:"trending,popular,top-rated,new,coming-soon" doc:"Which list"`
	Page      int     `query:"page" default:"1" minimum:"1" maximum:"500" doc:"1-based page number"`
	Genre     int     `query:"genre" doc:"TMDB genre ID; 0 for any"`
	Provider  int     `query:"provider" doc:"TMDB streaming service ID; 0 for any"`
	MinRating float64 `query:"min_rating" minimum:"0" maximum:"10" doc:"Minimum TMDB rating (0-10)"`
	Region    string  `query:"region" default:"US" doc:"Country for streaming availability (ISO 3166-1)"`
}

type discoverRegionInput struct {
	Region string `query:"region" default:"US" doc:"Country (ISO 3166-1)"`
}

type discoverSearchInput struct {
	Query string `query:"q" maxLength:"200" doc:"Title to search for"`
	Page  int    `query:"page" default:"1" minimum:"1" maximum:"500" doc:"1-based page number"`
}

type providerBody struct {
	ID       int    `json:"id"        doc:"TMDB watch provider ID"`
	Name     string `json:"name"      doc:"Service name"`
	LogoPath string `json:"logo_path" doc:"TMDB logo path"`
}

type providerListOutput struct {
	Body []providerBody
}

type discoverGenreInput struct {
	ID   int `path:"id" doc:"TMDB genre ID"`
	Page int `query:"page" doc:"1-based page number" default:"1"`
}

type genreBody struct {
	ID   int    `json:"id"   doc:"TMDB genre ID"`
	Name string `json:"name" doc:"Genre name"`
}

type genreListOutput struct {
	Body []genreBody
}

func paginatedToBody(pr *tmdbtv.PaginatedSeries) *discoverListBody {
	items := make([]discoverListItemBody, 0, len(pr.Results))
	for _, r := range pr.Results {
		title, ok := displayTitle(r.Title, r.OriginalTitle)
		if !ok {
			continue
		}
		items = append(items, discoverListItemBody{
			TMDBID:     r.ID,
			Title:      title,
			Year:       r.Year,
			Overview:   r.Overview,
			PosterPath: r.PosterPath,
			Rating:     r.VoteAverage,
		})
	}
	page := pr.Page
	if page < 1 {
		page = 1
	}
	return &discoverListBody{Results: items, Page: page, TotalPages: pr.TotalPages}
}

// ── Route registration ────────────────────────────────────────────────────────

// RegisterDiscoverRoutes registers the /api/v1/discover/* endpoints used by
// the Discover detail page.
func RegisterDiscoverRoutes(api huma.API, showSvc *show.Service) {
	// Browse with filters: one endpoint for every category, so genre,
	// streaming service and rating can be combined with any of them.
	huma.Register(api, huma.Operation{
		OperationID: "discover-browse",
		Method:      http.MethodGet,
		Path:        "/api/v1/discover/browse",
		Summary:     "Browse TV series by category with optional filters",
		Description: "Categories: trending, popular, top-rated, new (series that premiered on a streaming service in the last 90 days), coming-soon (premiering in the next 90 days). Filters are ignored for trending.",
		Tags:        []string{"Discover"},
	}, func(ctx context.Context, input *discoverBrowseInput) (*discoverListOutput, error) {
		pr, err := showSvc.DiscoverBrowse(ctx, tmdbtv.BrowseFilter{
			Category:   input.Category,
			Page:       input.Page,
			GenreID:    input.Genre,
			ProviderID: input.Provider,
			MinRating:  input.MinRating,
			Region:     input.Region,
			Today:      time.Now(),
		})
		if err != nil {
			return nil, mapShowError(err, "series")
		}
		return &discoverListOutput{Body: paginatedToBody(pr)}, nil
	})

	// GET /api/v1/discover/providers — streaming services for the filter.
	huma.Register(api, huma.Operation{
		OperationID: "discover-providers",
		Method:      http.MethodGet,
		Path:        "/api/v1/discover/providers",
		Summary:     "List streaming services in a country",
		Tags:        []string{"Discover"},
	}, func(ctx context.Context, input *discoverRegionInput) (*providerListOutput, error) {
		ps, err := showSvc.DiscoverProviders(ctx, input.Region)
		if err != nil {
			return nil, mapShowError(err, "series")
		}
		out := make([]providerBody, 0, len(ps))
		for _, p := range ps {
			out = append(out, providerBody{ID: p.ID, Name: p.Name, LogoPath: p.LogoPath})
		}
		return &providerListOutput{Body: out}, nil
	})

	// GET /api/v1/discover/search — title search, a page at a time.
	huma.Register(api, huma.Operation{
		OperationID: "discover-search",
		Method:      http.MethodGet,
		Path:        "/api/v1/discover/search",
		Summary:     "Search TV series by title",
		Tags:        []string{"Discover"},
	}, func(ctx context.Context, input *discoverSearchInput) (*discoverListOutput, error) {
		q := strings.TrimSpace(input.Query)
		if q == "" {
			return &discoverListOutput{Body: &discoverListBody{Results: []discoverListItemBody{}, Page: 1}}, nil
		}
		pr, err := showSvc.DiscoverSearch(ctx, q, input.Page)
		if err != nil {
			return nil, mapShowError(err, "series")
		}
		return &discoverListOutput{Body: paginatedToBody(pr)}, nil
	})

	// GET /api/v1/discover/{tmdbId}
	huma.Register(api, huma.Operation{
		OperationID: "discover-series",
		Method:      http.MethodGet,
		Path:        "/api/v1/discover/{tmdbId}",
		Summary:     "Get full series details from TMDB by ID",
		Tags:        []string{"Discover"},
	}, func(ctx context.Context, input *discoverSeriesInput) (*discoverSeriesDetailOutput, error) {
		detail, inLibrary, librarySeriesID, err := showSvc.LookupDetail(ctx, input.TMDBID)
		if err != nil {
			return nil, mapShowError(err, "series")
		}
		return &discoverSeriesDetailOutput{Body: discoverDetailToBody(detail, inLibrary, librarySeriesID)}, nil
	})

	// Browse category lists. Static segments take priority over the
	// {tmdbId} wildcard above, so these coexist with /discover/{tmdbId}.
	registerList := func(opID, path, summary, category string) {
		huma.Register(api, huma.Operation{
			OperationID: opID,
			Method:      http.MethodGet,
			Path:        path,
			Summary:     summary,
			Tags:        []string{"Discover"},
		}, func(ctx context.Context, input *discoverPageInput) (*discoverListOutput, error) {
			pr, err := showSvc.DiscoverList(ctx, category, input.Page)
			if err != nil {
				return nil, mapShowError(err, "series")
			}
			return &discoverListOutput{Body: paginatedToBody(pr)}, nil
		})
	}
	registerList("discover-trending", "/api/v1/discover/trending", "Trending TV series this week", "trending")
	registerList("discover-popular", "/api/v1/discover/popular", "Popular TV series", "popular")
	registerList("discover-top-rated", "/api/v1/discover/top-rated", "Top rated TV series", "top-rated")
	registerList("discover-on-the-air", "/api/v1/discover/on-the-air", "TV series airing soon", "on-the-air")

	// GET /api/v1/discover/genre/{id}
	huma.Register(api, huma.Operation{
		OperationID: "discover-genre",
		Method:      http.MethodGet,
		Path:        "/api/v1/discover/genre/{id}",
		Summary:     "Discover TV series by genre",
		Tags:        []string{"Discover"},
	}, func(ctx context.Context, input *discoverGenreInput) (*discoverListOutput, error) {
		pr, err := showSvc.DiscoverByGenre(ctx, input.ID, input.Page)
		if err != nil {
			return nil, mapShowError(err, "series")
		}
		return &discoverListOutput{Body: paginatedToBody(pr)}, nil
	})

	// GET /api/v1/discover/genres
	huma.Register(api, huma.Operation{
		OperationID: "discover-genres",
		Method:      http.MethodGet,
		Path:        "/api/v1/discover/genres",
		Summary:     "List TMDB TV genres",
		Tags:        []string{"Discover"},
	}, func(ctx context.Context, _ *struct{}) (*genreListOutput, error) {
		genres, err := showSvc.DiscoverGenres(ctx)
		if err != nil {
			return nil, mapShowError(err, "series")
		}
		bodies := make([]genreBody, 0, len(genres))
		for _, g := range genres {
			bodies = append(bodies, genreBody{ID: g.ID, Name: g.Name})
		}
		return &genreListOutput{Body: bodies}, nil
	})
}

func discoverDetailToBody(d *tmdbtv.SeriesDetail, inLibrary bool, librarySeriesID string) *discoverSeriesDetailBody {
	seasons := make([]discoverSeasonBody, 0, len(d.Seasons))
	for _, s := range d.Seasons {
		seasons = append(seasons, discoverSeasonBody{
			SeasonNumber: s.SeasonNumber,
			EpisodeCount: s.EpisodeCount,
			AirDate:      s.AirDate,
		})
	}

	cast := make([]discoverCastBody, 0, len(d.Cast))
	for _, c := range d.Cast {
		cast = append(cast, discoverCastBody{
			ID:          c.ID,
			Name:        c.Name,
			Character:   c.Character,
			ProfilePath: c.ProfilePath,
		})
	}

	crew := make([]discoverCrewBody, 0, len(d.Crew))
	for _, c := range d.Crew {
		crew = append(crew, discoverCrewBody{
			ID:          c.ID,
			Name:        c.Name,
			Job:         c.Job,
			ProfilePath: c.ProfilePath,
		})
	}

	recs := make([]discoverRecommendationBody, 0, len(d.Recommendations))
	for _, r := range d.Recommendations {
		recs = append(recs, discoverRecommendationBody{
			TMDBID:     r.TMDBID,
			Title:      r.Title,
			Year:       r.Year,
			PosterPath: r.PosterPath,
		})
	}

	return &discoverSeriesDetailBody{
		TMDBID:          d.ID,
		Title:           d.Title,
		OriginalTitle:   d.OriginalTitle,
		Year:            d.Year,
		Overview:        d.Overview,
		FirstAirDate:    d.FirstAirDate,
		Rating:          d.Rating,
		Status:          d.Status,
		Network:         d.Network,
		Genres:          d.Genres,
		PosterPath:      d.PosterPath,
		BackdropPath:    d.BackdropPath,
		Seasons:         seasons,
		Cast:            cast,
		Crew:            crew,
		Recommendations: recs,
		InLibrary:       inLibrary,
		LibrarySeriesID: librarySeriesID,
	}
}
