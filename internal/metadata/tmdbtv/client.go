// Package tmdbtv provides a TMDB API v3 client for TV series data.
package tmdbtv

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/beacon-stack/pilot/internal/appinfo"
)

const (
	defaultBaseURL = "https://api.themoviedb.org/3"
	httpTimeout    = 30 * time.Second
	redactedAPIKey = "***"

	// TMDB allows ~50 req/s; we stay well under it to leave headroom for
	// bursts (Discover browsing, metadata refresh) and avoid 429s.
	rateLimitPerSec = 20
	rateLimitBurst  = 20

	cacheTTL        = 15 * time.Minute
	cacheMaxEntries = 2000
)

// userAgent is the value sent in every outbound request's User-Agent header.
var userAgent = appinfo.AppName + "/0.1.0"

// Client is a TMDB API v3 HTTP client scoped to TV series endpoints.
// All outbound requests are logged. The API key is never logged.
// Outbound calls are rate-limited and GET responses are cached for a short
// TTL. Client is safe for concurrent use.
type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
	logger  *slog.Logger
	limiter *rate.Limiter
	cache   *responseCache
}

// New creates a new tmdbtv Client.
// apiKey must not be empty. logger is used to log outbound requests;
// the API key value is replaced with "***" in logged URLs.
func New(apiKey string, logger *slog.Logger) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		http:    &http.Client{Timeout: httpTimeout},
		logger:  logger,
		limiter: rate.NewLimiter(rate.Limit(rateLimitPerSec), rateLimitBurst),
		cache:   newResponseCache(cacheTTL, cacheMaxEntries),
	}
}

// SearchSeries searches TMDB for TV series matching query.
// If year is non-zero it is sent as the first_air_date_year filter.
func (c *Client) SearchSeries(ctx context.Context, query string, year int) ([]SearchResult, error) {
	params := url.Values{}
	params.Set("query", query)
	if year != 0 {
		params.Set("first_air_date_year", strconv.Itoa(year))
	}

	var envelope struct {
		Results []struct {
			ID           int     `json:"id"`
			Name         string  `json:"name"`
			OriginalName string  `json:"original_name"`
			Overview     string  `json:"overview"`
			FirstAirDate string  `json:"first_air_date"`
			PosterPath   string  `json:"poster_path"`
			BackdropPath string  `json:"backdrop_path"`
			Popularity   float64 `json:"popularity"`
		} `json:"results"`
	}

	if err := c.get(ctx, "/search/tv", params, &envelope); err != nil {
		return nil, fmt.Errorf("tmdbtv search series: %w", err)
	}

	results := make([]SearchResult, 0, len(envelope.Results))
	for _, r := range envelope.Results {
		results = append(results, SearchResult{
			ID:            r.ID,
			Title:         r.Name,
			OriginalTitle: r.OriginalName,
			Overview:      r.Overview,
			FirstAirDate:  r.FirstAirDate,
			Year:          parseYear(r.FirstAirDate),
			PosterPath:    r.PosterPath,
			BackdropPath:  r.BackdropPath,
			Popularity:    r.Popularity,
		})
	}

	return results, nil
}

// GetSeries fetches full series details by TMDB ID, including the season list.
func (c *Client) GetSeries(ctx context.Context, tmdbID int) (*SeriesDetail, error) {
	var raw struct {
		ID           int    `json:"id"`
		Name         string `json:"name"`
		OriginalName string `json:"original_name"`
		Overview     string `json:"overview"`
		FirstAirDate string `json:"first_air_date"`
		Status       string `json:"status"`
		Type         string `json:"type"`
		Genres       []struct {
			Name string `json:"name"`
		} `json:"genres"`
		PosterPath     string `json:"poster_path"`
		BackdropPath   string `json:"backdrop_path"`
		EpisodeRunTime []int  `json:"episode_run_time"`
		Networks       []struct {
			Name string `json:"name"`
		} `json:"networks"`
		Seasons []struct {
			SeasonNumber int    `json:"season_number"`
			EpisodeCount int    `json:"episode_count"`
			AirDate      string `json:"air_date"`
		} `json:"seasons"`
	}

	path := fmt.Sprintf("/tv/%d", tmdbID)
	if err := c.get(ctx, path, nil, &raw); err != nil {
		return nil, fmt.Errorf("tmdbtv get series %d: %w", tmdbID, err)
	}

	genres := make([]string, 0, len(raw.Genres))
	for _, g := range raw.Genres {
		genres = append(genres, g.Name)
	}

	var network string
	if len(raw.Networks) > 0 {
		network = raw.Networks[0].Name
	}

	var runtime int
	if len(raw.EpisodeRunTime) > 0 {
		runtime = raw.EpisodeRunTime[0]
	}

	seasons := make([]SeasonSummary, 0, len(raw.Seasons))
	for _, s := range raw.Seasons {
		seasons = append(seasons, SeasonSummary{
			SeasonNumber: s.SeasonNumber,
			EpisodeCount: s.EpisodeCount,
			AirDate:      s.AirDate,
		})
	}

	return &SeriesDetail{
		ID:             raw.ID,
		Title:          raw.Name,
		OriginalTitle:  raw.OriginalName,
		Overview:       raw.Overview,
		FirstAirDate:   raw.FirstAirDate,
		Year:           parseYear(raw.FirstAirDate),
		RuntimeMinutes: runtime,
		Genres:         genres,
		PosterPath:     raw.PosterPath,
		BackdropPath:   raw.BackdropPath,
		Status:         mapStatus(raw.Status),
		Network:        network,
		Seasons:        seasons,
	}, nil
}

// GetSeriesExtended fetches the same /tv/{id} payload as GetSeries but
// also requests credits and recommendations via append_to_response, and
// populates the Rating, Cast, Crew, and Recommendations fields. It is used
// by the Discover detail page; Add/RefreshMetadata use the lighter
// GetSeries instead.
func (c *Client) GetSeriesExtended(ctx context.Context, tmdbID int) (*SeriesDetail, error) {
	var raw struct {
		ID           int     `json:"id"`
		Name         string  `json:"name"`
		OriginalName string  `json:"original_name"`
		Overview     string  `json:"overview"`
		FirstAirDate string  `json:"first_air_date"`
		Status       string  `json:"status"`
		VoteAverage  float64 `json:"vote_average"`
		Genres       []struct {
			Name string `json:"name"`
		} `json:"genres"`
		PosterPath     string `json:"poster_path"`
		BackdropPath   string `json:"backdrop_path"`
		EpisodeRunTime []int  `json:"episode_run_time"`
		Networks       []struct {
			Name string `json:"name"`
		} `json:"networks"`
		Seasons []struct {
			SeasonNumber int    `json:"season_number"`
			EpisodeCount int    `json:"episode_count"`
			AirDate      string `json:"air_date"`
		} `json:"seasons"`
		CreatedBy []struct {
			ID          int    `json:"id"`
			Name        string `json:"name"`
			ProfilePath string `json:"profile_path"`
		} `json:"created_by"`
		Credits struct {
			Cast []struct {
				ID          int    `json:"id"`
				Name        string `json:"name"`
				Character   string `json:"character"`
				ProfilePath string `json:"profile_path"`
				Order       int    `json:"order"`
			} `json:"cast"`
			Crew []struct {
				ID          int    `json:"id"`
				Name        string `json:"name"`
				Job         string `json:"job"`
				Department  string `json:"department"`
				ProfilePath string `json:"profile_path"`
			} `json:"crew"`
		} `json:"credits"`
		Recommendations struct {
			Results []struct {
				ID           int    `json:"id"`
				Name         string `json:"name"`
				FirstAirDate string `json:"first_air_date"`
				PosterPath   string `json:"poster_path"`
			} `json:"results"`
		} `json:"recommendations"`
	}

	params := url.Values{}
	params.Set("append_to_response", "credits,recommendations")

	path := fmt.Sprintf("/tv/%d", tmdbID)
	if err := c.get(ctx, path, params, &raw); err != nil {
		return nil, fmt.Errorf("tmdbtv get series extended %d: %w", tmdbID, err)
	}

	genres := make([]string, 0, len(raw.Genres))
	for _, g := range raw.Genres {
		genres = append(genres, g.Name)
	}

	var network string
	if len(raw.Networks) > 0 {
		network = raw.Networks[0].Name
	}

	var runtime int
	if len(raw.EpisodeRunTime) > 0 {
		runtime = raw.EpisodeRunTime[0]
	}

	seasons := make([]SeasonSummary, 0, len(raw.Seasons))
	for _, s := range raw.Seasons {
		seasons = append(seasons, SeasonSummary{
			SeasonNumber: s.SeasonNumber,
			EpisodeCount: s.EpisodeCount,
			AirDate:      s.AirDate,
		})
	}

	// Top 10 cast by billing order (TMDB returns them pre-sorted).
	maxCast := 10
	if len(raw.Credits.Cast) < maxCast {
		maxCast = len(raw.Credits.Cast)
	}
	cast := make([]CastMember, 0, maxCast)
	for _, c := range raw.Credits.Cast[:maxCast] {
		cast = append(cast, CastMember{
			ID:          c.ID,
			Name:        c.Name,
			Character:   c.Character,
			ProfilePath: c.ProfilePath,
			Order:       c.Order,
		})
	}

	// Key crew. For TV the creators live in a top-level created_by list;
	// surface them as "Creator", then add notable crew jobs.
	crew := make([]CrewMember, 0)
	for _, cr := range raw.CreatedBy {
		crew = append(crew, CrewMember{
			ID:          cr.ID,
			Name:        cr.Name,
			Job:         "Creator",
			Department:  "Production",
			ProfilePath: cr.ProfilePath,
		})
	}
	keyJobs := map[string]bool{
		"Executive Producer": true,
		"Writer":             true,
		"Director":           true,
	}
	for _, cr := range raw.Credits.Crew {
		if keyJobs[cr.Job] {
			crew = append(crew, CrewMember{
				ID:          cr.ID,
				Name:        cr.Name,
				Job:         cr.Job,
				Department:  cr.Department,
				ProfilePath: cr.ProfilePath,
			})
		}
	}

	// Up to 10 recommendations.
	maxRecs := 10
	if len(raw.Recommendations.Results) < maxRecs {
		maxRecs = len(raw.Recommendations.Results)
	}
	recs := make([]SeriesRecommendation, 0, maxRecs)
	for _, r := range raw.Recommendations.Results[:maxRecs] {
		recs = append(recs, SeriesRecommendation{
			TMDBID:     r.ID,
			Title:      r.Name,
			Year:       parseYear(r.FirstAirDate),
			PosterPath: r.PosterPath,
		})
	}

	return &SeriesDetail{
		ID:              raw.ID,
		Title:           raw.Name,
		OriginalTitle:   raw.OriginalName,
		Overview:        raw.Overview,
		FirstAirDate:    raw.FirstAirDate,
		Year:            parseYear(raw.FirstAirDate),
		RuntimeMinutes:  runtime,
		Genres:          genres,
		PosterPath:      raw.PosterPath,
		BackdropPath:    raw.BackdropPath,
		Status:          mapStatus(raw.Status),
		Network:         network,
		Seasons:         seasons,
		Rating:          raw.VoteAverage,
		Cast:            cast,
		Crew:            crew,
		Recommendations: recs,
	}, nil
}

// GetAlternativeTitles fetches the list of alternate titles TMDB knows
// about for the series. Includes per-region marketing names ("Star Wars:
// Andor" for "Andor"), original-language titles, and any user-submitted
// aliases. The canonical title is NOT included — callers prepend it
// themselves so the matcher checks against the full set.
//
// Returns an empty slice (no error) when TMDB has no alternates.
func (c *Client) GetAlternativeTitles(ctx context.Context, tmdbID int) ([]string, error) {
	var raw struct {
		Results []struct {
			Title string `json:"title"`
		} `json:"results"`
	}
	path := fmt.Sprintf("/tv/%d/alternative_titles", tmdbID)
	if err := c.get(ctx, path, nil, &raw); err != nil {
		return nil, fmt.Errorf("tmdbtv get alt titles %d: %w", tmdbID, err)
	}
	titles := make([]string, 0, len(raw.Results))
	seen := make(map[string]struct{}, len(raw.Results))
	for _, r := range raw.Results {
		t := strings.TrimSpace(r.Title)
		if t == "" {
			continue
		}
		key := strings.ToLower(t)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		titles = append(titles, t)
	}
	return titles, nil
}

// GetSeasonEpisodes fetches the episode list for a single season.
func (c *Client) GetSeasonEpisodes(ctx context.Context, tmdbID int, seasonNum int) ([]EpisodeDetail, error) {
	var raw struct {
		Episodes []struct {
			ID            int    `json:"id"`
			Name          string `json:"name"`
			Overview      string `json:"overview"`
			AirDate       string `json:"air_date"`
			SeasonNumber  int    `json:"season_number"`
			EpisodeNumber int    `json:"episode_number"`
			StillPath     string `json:"still_path"`
			Runtime       int    `json:"runtime"`
		} `json:"episodes"`
	}

	path := fmt.Sprintf("/tv/%d/season/%d", tmdbID, seasonNum)
	if err := c.get(ctx, path, nil, &raw); err != nil {
		return nil, fmt.Errorf("tmdbtv get season episodes %d s%d: %w", tmdbID, seasonNum, err)
	}

	episodes := make([]EpisodeDetail, 0, len(raw.Episodes))
	for _, e := range raw.Episodes {
		episodes = append(episodes, EpisodeDetail{
			ID:             e.ID,
			SeasonNumber:   e.SeasonNumber,
			EpisodeNumber:  e.EpisodeNumber,
			Title:          e.Name,
			Overview:       e.Overview,
			AirDate:        e.AirDate,
			StillPath:      e.StillPath,
			RuntimeMinutes: e.Runtime,
		})
	}

	return episodes, nil
}

// GetPopularTV returns the current most popular TV series from TMDB.
// pageParams builds query params carrying a 1-based page (page<=0 omitted,
// so TMDB defaults to page 1).
func pageParams(page int) url.Values {
	params := url.Values{}
	if page > 0 {
		params.Set("page", strconv.Itoa(page))
	}
	return params
}

// fetchTVList fetches a paginated list of TV series from a TMDB list
// endpoint (popular, top_rated, on_the_air, trending, discover) and maps the
// common envelope (results + page + total_pages) to PaginatedSeries.
func (c *Client) fetchTVList(ctx context.Context, path string, params url.Values) (*PaginatedSeries, error) {
	var envelope struct {
		Page       int `json:"page"`
		TotalPages int `json:"total_pages"`
		Results    []struct {
			ID           int     `json:"id"`
			Name         string  `json:"name"`
			OriginalName string  `json:"original_name"`
			Overview     string  `json:"overview"`
			FirstAirDate string  `json:"first_air_date"`
			PosterPath   string  `json:"poster_path"`
			BackdropPath string  `json:"backdrop_path"`
			Popularity   float64 `json:"popularity"`
			VoteAverage  float64 `json:"vote_average"`
		} `json:"results"`
	}
	if err := c.get(ctx, path, params, &envelope); err != nil {
		return nil, err
	}
	results := make([]SearchResult, 0, len(envelope.Results))
	for _, r := range envelope.Results {
		results = append(results, SearchResult{
			ID:            r.ID,
			Title:         r.Name,
			OriginalTitle: r.OriginalName,
			Overview:      r.Overview,
			FirstAirDate:  r.FirstAirDate,
			Year:          parseYear(r.FirstAirDate),
			PosterPath:    r.PosterPath,
			BackdropPath:  r.BackdropPath,
			Popularity:    r.Popularity,
			VoteAverage:   r.VoteAverage,
		})
	}
	return &PaginatedSeries{Results: results, Page: envelope.Page, TotalPages: envelope.TotalPages}, nil
}

func (c *Client) GetPopularTV(ctx context.Context, page int) (*PaginatedSeries, error) {
	pr, err := c.fetchTVList(ctx, "/tv/popular", pageParams(page))
	if err != nil {
		return nil, fmt.Errorf("tmdbtv popular tv: %w", err)
	}
	return pr, nil
}

// GetTopRatedTV returns the highest-rated TV series from TMDB.
func (c *Client) GetTopRatedTV(ctx context.Context, page int) (*PaginatedSeries, error) {
	pr, err := c.fetchTVList(ctx, "/tv/top_rated", pageParams(page))
	if err != nil {
		return nil, fmt.Errorf("tmdbtv top rated tv: %w", err)
	}
	return pr, nil
}

// GetOnTheAirTV returns series with episodes airing in the next ~7 days.
// This is the TV analog to a movies "upcoming" list (TMDB has no /tv/upcoming).
func (c *Client) GetOnTheAirTV(ctx context.Context, page int) (*PaginatedSeries, error) {
	pr, err := c.fetchTVList(ctx, "/tv/on_the_air", pageParams(page))
	if err != nil {
		return nil, fmt.Errorf("tmdbtv on the air tv: %w", err)
	}
	return pr, nil
}

// DiscoverTVByGenre returns popular series in the given TMDB genre.
func (c *Client) DiscoverTVByGenre(ctx context.Context, genreID, page int) (*PaginatedSeries, error) {
	params := pageParams(page)
	params.Set("with_genres", strconv.Itoa(genreID))
	params.Set("sort_by", "popularity.desc")
	pr, err := c.fetchTVList(ctx, "/discover/tv", params)
	if err != nil {
		return nil, fmt.Errorf("tmdbtv discover by genre: %w", err)
	}
	return pr, nil
}

// GetTVGenres returns the list of TMDB TV genres.
func (c *Client) GetTVGenres(ctx context.Context) ([]Genre, error) {
	var envelope struct {
		Genres []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"genres"`
	}
	if err := c.get(ctx, "/genre/tv/list", nil, &envelope); err != nil {
		return nil, fmt.Errorf("tmdbtv tv genres: %w", err)
	}
	genres := make([]Genre, 0, len(envelope.Genres))
	for _, g := range envelope.Genres {
		genres = append(genres, Genre{ID: g.ID, Name: g.Name})
	}
	return genres, nil
}

// GetTrendingTV returns trending TV series from TMDB.
// window must be "day" or "week" (defaults to "week").
func (c *Client) GetTrendingTV(ctx context.Context, window string, page int) (*PaginatedSeries, error) {
	if window == "" {
		window = "week"
	}
	pr, err := c.fetchTVList(ctx, fmt.Sprintf("/trending/tv/%s", window), pageParams(page))
	if err != nil {
		return nil, fmt.Errorf("tmdbtv trending tv: %w", err)
	}
	return pr, nil
}

// GetPerson fetches basic info for a TMDB person.
func (c *Client) GetPerson(ctx context.Context, personID int) (*Person, error) {
	var raw struct {
		ID                 int    `json:"id"`
		Name               string `json:"name"`
		ProfilePath        string `json:"profile_path"`
		KnownForDepartment string `json:"known_for_department"`
	}
	path := fmt.Sprintf("/person/%d", personID)
	if err := c.get(ctx, path, nil, &raw); err != nil {
		return nil, fmt.Errorf("tmdbtv get person %d: %w", personID, err)
	}
	return &Person{
		ID:                 raw.ID,
		Name:               raw.Name,
		ProfilePath:        raw.ProfilePath,
		KnownForDepartment: raw.KnownForDepartment,
	}, nil
}

// GetPersonTVCredits returns the TV series a person is credited on. For
// personType "director" it returns crew credits with job "Director"; for
// anything else (actors) it returns top-billed cast credits (order < 5),
// mirroring Prism's movie filmography behaviour.
func (c *Client) GetPersonTVCredits(ctx context.Context, personID int, personType string) ([]PersonCredit, error) {
	var envelope struct {
		Cast []struct {
			ID           int    `json:"id"`
			Name         string `json:"name"`
			FirstAirDate string `json:"first_air_date"`
			PosterPath   string `json:"poster_path"`
			Order        int    `json:"order"`
			EpisodeCount int    `json:"episode_count"`
		} `json:"cast"`
		Crew []struct {
			ID           int    `json:"id"`
			Name         string `json:"name"`
			FirstAirDate string `json:"first_air_date"`
			PosterPath   string `json:"poster_path"`
			Job          string `json:"job"`
			EpisodeCount int    `json:"episode_count"`
		} `json:"crew"`
	}
	path := fmt.Sprintf("/person/%d/tv_credits", personID)
	if err := c.get(ctx, path, nil, &envelope); err != nil {
		return nil, fmt.Errorf("tmdbtv get person tv credits %d: %w", personID, err)
	}

	// Note: ranking/capping by episode count happens in the service layer —
	// the cast `order` field is unreliable for TV (talk-show and self
	// appearances are order 0), so we return all credits and let the caller
	// rank them by screen time.
	var items []PersonCredit
	if personType == "director" {
		for _, cr := range envelope.Crew {
			if cr.Job != "Director" {
				continue
			}
			items = append(items, PersonCredit{
				TMDBID:       cr.ID,
				Title:        cr.Name,
				Year:         parseYear(cr.FirstAirDate),
				PosterPath:   cr.PosterPath,
				EpisodeCount: cr.EpisodeCount,
			})
		}
	} else {
		for _, cr := range envelope.Cast {
			items = append(items, PersonCredit{
				TMDBID:       cr.ID,
				Title:        cr.Name,
				Year:         parseYear(cr.FirstAirDate),
				PosterPath:   cr.PosterPath,
				Order:        cr.Order,
				EpisodeCount: cr.EpisodeCount,
			})
		}
	}
	return items, nil
}

// get performs a GET against the TMDB API, decodes the JSON body into dst,
// and returns a structured error on non-200 responses.
func (c *Client) get(ctx context.Context, path string, params url.Values, dst any) error {
	if params == nil {
		params = url.Values{}
	}

	// Cache key is the path+query without the API key, so it's stable across
	// key rotations and never embeds the secret.
	cacheKey := path + "?" + params.Encode()
	if c.cache != nil {
		if body, ok := c.cache.get(cacheKey); ok {
			if err := json.Unmarshal(body, dst); err != nil {
				return fmt.Errorf("decoding cached response: %w", err)
			}
			return nil
		}
	}

	params.Set("api_key", c.apiKey)
	rawURL := c.baseURL + path + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	// Log the URL with the API key redacted.
	c.logger.InfoContext(ctx, "tmdbtv request",
		slog.String("method", http.MethodGet),
		slog.String("url", redactAPIKey(rawURL, c.apiKey)),
	)

	if c.limiter != nil {
		if err := c.limiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limiter: %w", err)
		}
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Try to extract the TMDB error message for context.
		var apiErr struct {
			StatusMessage string `json:"status_message"`
			StatusCode    int    `json:"status_code"`
		}
		_ = json.Unmarshal(body, &apiErr)
		if apiErr.StatusMessage != "" {
			return fmt.Errorf("http %d: %s", resp.StatusCode, apiErr.StatusMessage)
		}
		return fmt.Errorf("http %d", resp.StatusCode)
	}

	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	if c.cache != nil {
		c.cache.set(cacheKey, body)
	}

	return nil
}

// parseYear extracts the four-digit year from a "YYYY-MM-DD" date string.
// Returns 0 if the string is empty or malformed.
func parseYear(date string) int {
	if date == "" {
		return 0
	}
	parts := strings.SplitN(date, "-", 2)
	y, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0
	}
	return y
}

// mapStatus converts a TMDB series status string to the internal vocabulary.
//
//   - "Returning Series"      → "continuing"
//   - "Ended" / "Canceled"   → "ended"
//   - anything else           → "upcoming"
func mapStatus(tmdbStatus string) string {
	switch tmdbStatus {
	case "Returning Series":
		return "continuing"
	case "Ended", "Canceled":
		return "ended"
	default:
		return "upcoming"
	}
}

// redactAPIKey replaces the api_key query parameter value in a URL string with "***".
// Best-effort: returns the original string if nothing to replace.
func redactAPIKey(rawURL, apiKey string) string {
	if apiKey == "" {
		return rawURL
	}
	return strings.ReplaceAll(rawURL, "api_key="+apiKey, "api_key="+redactedAPIKey)
}
