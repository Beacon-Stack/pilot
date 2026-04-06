// Package tmdb_trending_tv provides a Pilot import list plugin that fetches
// trending TV series from TMDB.
package tmdbtrendingtv

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/beacon-media/pilot/internal/metadata/tmdbtv"
	"github.com/beacon-media/pilot/internal/registry"
	"github.com/beacon-media/pilot/pkg/plugin"
)

func init() {
	registry.Default.RegisterImportList("tmdb_trending_tv", func(settings json.RawMessage) (plugin.ImportList, error) {
		var cfg Config
		if len(settings) > 0 {
			if err := json.Unmarshal(settings, &cfg); err != nil {
				return nil, fmt.Errorf("tmdb_trending_tv: invalid settings: %w", err)
			}
		}
		if cfg.Window == "" {
			cfg.Window = "week"
		}
		return &Plugin{cfg: cfg}, nil
	})
}

// Config holds the optional settings.
type Config struct {
	Window string `json:"window"` // "day" or "week" (default: "week")
}

// Plugin implements plugin.ImportList for TMDB Trending TV.
type Plugin struct {
	cfg    Config
	client *tmdbtv.Client
}

func (p *Plugin) Name() string { return "TMDb Trending TV" }

func (p *Plugin) SetTMDBClient(client any) {
	if c, ok := client.(*tmdbtv.Client); ok {
		p.client = c
	}
}

func (p *Plugin) Fetch(ctx context.Context) ([]plugin.ImportListItem, error) {
	if p.client == nil {
		return nil, fmt.Errorf("tmdb_trending_tv: TMDB client not configured")
	}
	results, err := p.client.GetTrendingTV(ctx, p.cfg.Window, 1)
	if err != nil {
		return nil, fmt.Errorf("tmdb_trending_tv: %w", err)
	}
	items := make([]plugin.ImportListItem, 0, len(results))
	for _, r := range results {
		items = append(items, plugin.ImportListItem{
			TMDbID:     r.ID,
			Title:      r.Title,
			Year:       r.Year,
			PosterPath: r.PosterPath,
		})
	}
	return items, nil
}

func (p *Plugin) Test(ctx context.Context) error {
	if p.client == nil {
		return fmt.Errorf("tmdb_trending_tv: TMDB client not configured")
	}
	_, err := p.client.GetTrendingTV(ctx, p.cfg.Window, 1)
	return err
}
