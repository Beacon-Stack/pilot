// Package trakt_popular_tv provides a Pilot import list plugin that fetches
// Trakt's most popular TV shows.
package traktpopulartv

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/beacon-media/pilot/internal/registry"
	"github.com/beacon-media/pilot/internal/trakt"
	"github.com/beacon-media/pilot/pkg/plugin"
)

func init() {
	registry.Default.RegisterImportList("trakt_popular_tv", func(_ json.RawMessage) (plugin.ImportList, error) {
		return &Plugin{}, nil
	})
}

// Plugin implements plugin.ImportList for Trakt Popular TV.
type Plugin struct {
	client *trakt.Client
}

func (p *Plugin) Name() string { return "Trakt Popular TV" }

func (p *Plugin) SetTraktClient(client any) {
	if c, ok := client.(*trakt.Client); ok {
		p.client = c
	}
}

func (p *Plugin) Fetch(ctx context.Context) ([]plugin.ImportListItem, error) {
	if p.client == nil {
		return nil, fmt.Errorf("trakt_popular_tv: Trakt client not configured")
	}
	shows, err := p.client.GetPopularShows(ctx)
	if err != nil {
		return nil, fmt.Errorf("trakt_popular_tv: %w", err)
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
		return fmt.Errorf("trakt_popular_tv: Trakt client not configured")
	}
	_, err := p.client.GetPopularShows(ctx)
	return err
}
