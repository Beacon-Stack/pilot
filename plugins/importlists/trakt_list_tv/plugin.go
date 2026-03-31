// Package trakt_list_tv provides a Screenarr import list plugin that fetches
// TV shows from a Trakt user's watchlist or custom list.
package traktlisttv

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/screenarr/screenarr/internal/registry"
	"github.com/screenarr/screenarr/internal/trakt"
	"github.com/screenarr/screenarr/pkg/plugin"
)

func init() {
	registry.Default.RegisterImportList("trakt_list_tv", func(settings json.RawMessage) (plugin.ImportList, error) {
		var cfg Config
		if err := json.Unmarshal(settings, &cfg); err != nil {
			return nil, fmt.Errorf("trakt_list_tv: invalid settings: %w", err)
		}
		if cfg.Username == "" {
			return nil, fmt.Errorf("trakt_list_tv: username is required")
		}
		if cfg.ListType == "" {
			cfg.ListType = "watchlist"
		}
		if cfg.ListType == "custom" && cfg.ListSlug == "" {
			return nil, fmt.Errorf("trakt_list_tv: list_slug is required for custom lists")
		}
		return &Plugin{cfg: cfg}, nil
	})
}

// Config holds the user-supplied settings.
type Config struct {
	Username string `json:"username"`
	ListType string `json:"list_type"` // "watchlist" (default) or "custom"
	ListSlug string `json:"list_slug"` // required when list_type = "custom"
}

// Plugin implements plugin.ImportList for Trakt User List (TV).
type Plugin struct {
	cfg    Config
	client *trakt.Client
}

func (p *Plugin) Name() string { return "Trakt User List (TV)" }

func (p *Plugin) SetTraktClient(client any) {
	if c, ok := client.(*trakt.Client); ok {
		p.client = c
	}
}

func (p *Plugin) Fetch(ctx context.Context) ([]plugin.ImportListItem, error) {
	if p.client == nil {
		return nil, fmt.Errorf("trakt_list_tv: Trakt client not configured")
	}

	var shows []trakt.Show
	var err error

	switch p.cfg.ListType {
	case "custom":
		shows, err = p.client.GetCustomListShows(ctx, p.cfg.Username, p.cfg.ListSlug)
	default:
		shows, err = p.client.GetWatchlistShows(ctx, p.cfg.Username)
	}
	if err != nil {
		return nil, fmt.Errorf("trakt_list_tv: %w", err)
	}

	items := make([]plugin.ImportListItem, 0, len(shows))
	for _, s := range shows {
		if s.IDs.TMDB == 0 {
			continue
		}
		items = append(items, plugin.ImportListItem{
			TMDbID: s.IDs.TMDB,
			IMDbID: s.IDs.IMDB,
			Title:  s.Title,
			Year:   s.Year,
		})
	}
	return items, nil
}

func (p *Plugin) Test(ctx context.Context) error {
	if p.client == nil {
		return fmt.Errorf("trakt_list_tv: Trakt client not configured")
	}
	_, err := p.client.GetWatchlistShows(ctx, p.cfg.Username)
	return err
}
