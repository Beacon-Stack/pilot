// Package trakt provides a minimal HTTP client for the Trakt API v2.
// Only the TV show endpoints needed for import lists are implemented.
package trakt

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/beacon-media/pilot/internal/safedialer"
)

const (
	baseURL     = "https://api.trakt.tv"
	apiVersion  = "2"
	httpTimeout = 30 * time.Second
)

// Client is a Trakt API v2 HTTP client.
type Client struct {
	clientID string
	baseURL  string // overridable in tests; defaults to baseURL constant
	http     *http.Client
	logger   *slog.Logger
}

// New creates a new Trakt client. clientID is the Trakt API key
// (obtained from trakt.tv/oauth/applications). logger may be nil.
func New(clientID string, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Client{
		clientID: clientID,
		baseURL:  baseURL,
		http: &http.Client{
			Timeout:   httpTimeout,
			Transport: safedialer.Transport(),
		},
		logger: logger,
	}
}

// WithBaseURL overrides the API base URL. Intended for tests only.
func (c *Client) WithBaseURL(u string) *Client {
	c.baseURL = u
	return c
}

// WithHTTPClient overrides the underlying HTTP client. Intended for tests only.
func (c *Client) WithHTTPClient(hc *http.Client) *Client {
	c.http = hc
	return c
}

// Show represents a TV show from the Trakt API.
type Show struct {
	Title string `json:"title"`
	Year  int    `json:"year"`
	IDs   struct {
		Trakt int    `json:"trakt"`
		TMDB  int    `json:"tmdb"`
		IMDB  string `json:"imdb"`
		TVDB  int    `json:"tvdb"`
	} `json:"ids"`
}

// GetPopularShows fetches Trakt's popular TV shows list.
func (c *Client) GetPopularShows(ctx context.Context) ([]Show, error) {
	var shows []Show
	if err := c.get(ctx, "/shows/popular?limit=100", &shows); err != nil {
		return nil, fmt.Errorf("trakt popular shows: %w", err)
	}
	return shows, nil
}

// GetTrendingShows fetches Trakt's trending TV shows (most watched right now).
func (c *Client) GetTrendingShows(ctx context.Context) ([]Show, error) {
	var items []struct {
		Show Show `json:"show"`
	}
	if err := c.get(ctx, "/shows/trending?limit=100", &items); err != nil {
		return nil, fmt.Errorf("trakt trending shows: %w", err)
	}
	shows := make([]Show, len(items))
	for i, item := range items {
		shows[i] = item.Show
	}
	return shows, nil
}

// GetWatchlistShows fetches a user's TV show watchlist.
func (c *Client) GetWatchlistShows(ctx context.Context, username string) ([]Show, error) {
	path := fmt.Sprintf("/users/%s/watchlist/shows", username)
	var items []struct {
		Show Show `json:"show"`
	}
	if err := c.get(ctx, path, &items); err != nil {
		return nil, fmt.Errorf("trakt watchlist shows: %w", err)
	}
	shows := make([]Show, len(items))
	for i, item := range items {
		shows[i] = item.Show
	}
	return shows, nil
}

// GetCustomListShows fetches a user's custom list items (TV shows only).
func (c *Client) GetCustomListShows(ctx context.Context, username, listSlug string) ([]Show, error) {
	path := fmt.Sprintf("/users/%s/lists/%s/items/shows", username, listSlug)
	var items []struct {
		Show Show `json:"show"`
	}
	if err := c.get(ctx, path, &items); err != nil {
		return nil, fmt.Errorf("trakt custom list shows: %w", err)
	}
	shows := make([]Show, len(items))
	for i, item := range items {
		shows[i] = item.Show
	}
	return shows, nil
}

func (c *Client) get(ctx context.Context, path string, dst any) error {
	reqURL := c.baseURL + path

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("trakt-api-version", apiVersion)
	req.Header.Set("trakt-api-key", c.clientID)

	c.logger.InfoContext(ctx, "trakt request",
		slog.String("method", http.MethodGet),
		slog.String("path", path),
	)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	return nil
}
