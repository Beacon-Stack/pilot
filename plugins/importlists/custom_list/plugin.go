// Package custom_list provides a Screenarr import list plugin that fetches
// series from a user-provided JSON URL.
package customlist

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/screenarr/screenarr/internal/registry"
	"github.com/screenarr/screenarr/internal/safedialer"
	"github.com/screenarr/screenarr/pkg/plugin"
)

const maxResponseSize = 10 << 20 // 10 MiB

func init() {
	registry.Default.RegisterImportList("custom_list", func(settings json.RawMessage) (plugin.ImportList, error) {
		var cfg Config
		if err := json.Unmarshal(settings, &cfg); err != nil {
			return nil, fmt.Errorf("custom_list: invalid settings: %w", err)
		}
		if cfg.URL == "" {
			return nil, fmt.Errorf("custom_list: url is required")
		}
		return &Plugin{
			cfg: cfg,
			client: &http.Client{
				Timeout:   30 * time.Second,
				Transport: safedialer.Transport(),
			},
		}, nil
	})
}

// Config holds the user-supplied settings.
type Config struct {
	URL string `json:"url"` // URL returning a JSON array of series
}

// Plugin implements plugin.ImportList for Custom JSON List.
type Plugin struct {
	cfg    Config
	client *http.Client
}

func (p *Plugin) Name() string { return "Custom JSON List" }

// rawItem is a flexible struct that accepts common field naming conventions.
type rawItem struct {
	TMDB   int    `json:"tmdb"`
	TMDBID int    `json:"tmdb_id"`
	IMDB   string `json:"imdb"`
	IMDBID string `json:"imdb_id"`
	Title  string `json:"title"`
	Year   int    `json:"year"`
}

func (r rawItem) tmdbID() int {
	if r.TMDBID != 0 {
		return r.TMDBID
	}
	return r.TMDB
}

func (r rawItem) imdbID() string {
	if r.IMDBID != "" {
		return r.IMDBID
	}
	return r.IMDB
}

func (p *Plugin) Fetch(ctx context.Context) ([]plugin.ImportListItem, error) {
	body, err := p.fetch(ctx)
	if err != nil {
		return nil, err
	}

	var rawItems []rawItem
	if err := json.Unmarshal(body, &rawItems); err != nil {
		return nil, fmt.Errorf("custom_list: parsing JSON: %w", err)
	}

	items := make([]plugin.ImportListItem, 0, len(rawItems))
	for _, r := range rawItems {
		tmdbID := r.tmdbID()
		if tmdbID == 0 {
			continue
		}
		items = append(items, plugin.ImportListItem{
			TMDbID: tmdbID,
			IMDbID: r.imdbID(),
			Title:  r.Title,
			Year:   r.Year,
		})
	}
	return items, nil
}

func (p *Plugin) Test(ctx context.Context) error {
	body, err := p.fetch(ctx)
	if err != nil {
		return err
	}
	var rawItems []rawItem
	if err := json.Unmarshal(body, &rawItems); err != nil {
		return fmt.Errorf("custom_list: response is not a valid JSON array: %w", err)
	}
	if len(rawItems) > 0 && rawItems[0].tmdbID() == 0 {
		return fmt.Errorf("custom_list: first item has no tmdb_id — check the JSON format")
	}
	return nil
}

func (p *Plugin) fetch(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("custom_list: building request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("custom_list: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("custom_list: server returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("custom_list: reading response: %w", err)
	}
	return body, nil
}
