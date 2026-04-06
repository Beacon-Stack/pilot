// Package tmdbtv provides a TMDB API v3 client for TV series data.
package tmdbtv

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/beacon-media/pilot/internal/appinfo"
)

const (
	defaultBaseURL = "https://api.themoviedb.org/3"
	httpTimeout    = 30 * time.Second
	redactedAPIKey = "***"
)

// userAgent is the value sent in every outbound request's User-Agent header.
var userAgent = appinfo.AppName + "/0.1.0"

// Client is a TMDB API v3 HTTP client scoped to TV series endpoints.
// All outbound requests are logged. The API key is never logged.
// Client is safe for concurrent use.
type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
	logger  *slog.Logger
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
		} `json:"episodes"`
	}

	path := fmt.Sprintf("/tv/%d/season/%d", tmdbID, seasonNum)
	if err := c.get(ctx, path, nil, &raw); err != nil {
		return nil, fmt.Errorf("tmdbtv get season episodes %d s%d: %w", tmdbID, seasonNum, err)
	}

	episodes := make([]EpisodeDetail, 0, len(raw.Episodes))
	for _, e := range raw.Episodes {
		episodes = append(episodes, EpisodeDetail{
			ID:            e.ID,
			SeasonNumber:  e.SeasonNumber,
			EpisodeNumber: e.EpisodeNumber,
			Title:         e.Name,
			Overview:      e.Overview,
			AirDate:       e.AirDate,
		})
	}

	return episodes, nil
}

// GetPopularTV returns the current most popular TV series from TMDB.
func (c *Client) GetPopularTV(ctx context.Context, page int) ([]SearchResult, error) {
	params := url.Values{}
	if page > 0 {
		params.Set("page", strconv.Itoa(page))
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

	if err := c.get(ctx, "/tv/popular", params, &envelope); err != nil {
		return nil, fmt.Errorf("tmdbtv popular tv: %w", err)
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

// GetTrendingTV returns trending TV series from TMDB.
// window must be "day" or "week".
func (c *Client) GetTrendingTV(ctx context.Context, window string, page int) ([]SearchResult, error) {
	if window == "" {
		window = "week"
	}

	params := url.Values{}
	if page > 0 {
		params.Set("page", strconv.Itoa(page))
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

	path := fmt.Sprintf("/trending/tv/%s", window)
	if err := c.get(ctx, path, params, &envelope); err != nil {
		return nil, fmt.Errorf("tmdbtv trending tv: %w", err)
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

// get performs a GET against the TMDB API, decodes the JSON body into dst,
// and returns a structured error on non-200 responses.
func (c *Client) get(ctx context.Context, path string, params url.Values, dst any) error {
	if params == nil {
		params = url.Values{}
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

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Try to extract the TMDB error message for context.
		var apiErr struct {
			StatusMessage string `json:"status_message"`
			StatusCode    int    `json:"status_code"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		if apiErr.StatusMessage != "" {
			return fmt.Errorf("http %d: %s", resp.StatusCode, apiErr.StatusMessage)
		}
		return fmt.Errorf("http %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("decoding response: %w", err)
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
