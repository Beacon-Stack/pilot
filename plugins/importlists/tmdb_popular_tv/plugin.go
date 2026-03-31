// Package tmdb_popular_tv provides a Screenarr import list plugin that fetches
// the current most popular TV series from TMDB.
package tmdbpopulartv

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/screenarr/screenarr/internal/metadata/tmdbtv"
	"github.com/screenarr/screenarr/internal/registry"
	"github.com/screenarr/screenarr/pkg/plugin"
)

func init() {
	registry.Default.RegisterImportList("tmdb_popular_tv", func(_ json.RawMessage) (plugin.ImportList, error) {
		return &Plugin{}, nil
	})
}

// Plugin implements plugin.ImportList for TMDB Popular TV.
type Plugin struct {
	client *tmdbtv.Client
}

func (p *Plugin) Name() string { return "TMDb Popular TV" }

func (p *Plugin) SetTMDBClient(client any) {
	if c, ok := client.(*tmdbtv.Client); ok {
		p.client = c
	}
}

func (p *Plugin) Fetch(ctx context.Context) ([]plugin.ImportListItem, error) {
	if p.client == nil {
		return nil, fmt.Errorf("tmdb_popular_tv: TMDB client not configured")
	}
	results, err := p.client.GetPopularTV(ctx, 1)
	if err != nil {
		return nil, fmt.Errorf("tmdb_popular_tv: %w", err)
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
		return fmt.Errorf("tmdb_popular_tv: TMDB client not configured")
	}
	_, err := p.client.GetPopularTV(ctx, 1)
	return err
}
