// Package trakt_trending_tv provides a Screenarr import list plugin that fetches
// Trakt's trending TV shows (most watched right now).
package trakttrendingtv

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/screenarr/screenarr/internal/registry"
	"github.com/screenarr/screenarr/internal/trakt"
	"github.com/screenarr/screenarr/pkg/plugin"
)

func init() {
	registry.Default.RegisterImportList("trakt_trending_tv", func(_ json.RawMessage) (plugin.ImportList, error) {
		return &Plugin{}, nil
	})
}

// Plugin implements plugin.ImportList for Trakt Trending TV.
type Plugin struct {
	client *trakt.Client
}

func (p *Plugin) Name() string { return "Trakt Trending TV" }

func (p *Plugin) SetTraktClient(client any) {
	if c, ok := client.(*trakt.Client); ok {
		p.client = c
	}
}

func (p *Plugin) Fetch(ctx context.Context) ([]plugin.ImportListItem, error) {
	if p.client == nil {
		return nil, fmt.Errorf("trakt_trending_tv: Trakt client not configured")
	}
	shows, err := p.client.GetTrendingShows(ctx)
	if err != nil {
		return nil, fmt.Errorf("trakt_trending_tv: %w", err)
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
		return fmt.Errorf("trakt_trending_tv: Trakt client not configured")
	}
	_, err := p.client.GetTrendingShows(ctx)
	return err
}
