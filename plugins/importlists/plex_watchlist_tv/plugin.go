// Package plex_watchlist_tv provides a Pilot import list plugin that fetches
// TV shows from a Plex user's watchlist via the Plex metadata API.
package plexwatchlisttv

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/beacon-media/pilot/internal/metadata/tmdbtv"
	"github.com/beacon-media/pilot/internal/registry"
	"github.com/beacon-media/pilot/internal/safedialer"
	"github.com/beacon-media/pilot/pkg/plugin"
)

func init() {
	registry.Default.RegisterImportList("plex_watchlist_tv", func(settings json.RawMessage) (plugin.ImportList, error) {
		var cfg Config
		if err := json.Unmarshal(settings, &cfg); err != nil {
			return nil, fmt.Errorf("plex_watchlist_tv: invalid settings: %w", err)
		}
		if cfg.AccessToken == "" {
			return nil, fmt.Errorf("plex_watchlist_tv: access_token is required")
		}
		return &Plugin{
			cfg: cfg,
			client: &http.Client{
				Timeout:   30 * time.Second,
				Transport: safedialer.Transport(),
			},
		}, nil
	})

	registry.Default.RegisterImportListSanitizer("plex_watchlist_tv", func(settings json.RawMessage) json.RawMessage {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(settings, &m); err != nil {
			return json.RawMessage("{}")
		}
		if _, ok := m["access_token"]; ok {
			m["access_token"] = json.RawMessage(`"***"`)
		}
		out, _ := json.Marshal(m)
		return out
	})
}

// Config holds the user-supplied settings.
type Config struct {
	AccessToken string `json:"access_token"` // Plex account token (not server token)
}

// Plugin implements plugin.ImportList for Plex Watchlist (TV).
type Plugin struct {
	cfg        Config
	client     *http.Client
	tmdbClient *tmdbtv.Client
}

func (p *Plugin) Name() string { return "Plex Watchlist (TV)" }

func (p *Plugin) SetTMDBClient(client any) {
	if c, ok := client.(*tmdbtv.Client); ok {
		p.tmdbClient = c
	}
}

const watchlistURL = "https://metadata.provider.plex.tv/library/sections/watchlist/all?type=2&includeGuids=1"

func (p *Plugin) Fetch(ctx context.Context) ([]plugin.ImportListItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, watchlistURL, nil)
	if err != nil {
		return nil, fmt.Errorf("plex_watchlist_tv: building request: %w", err)
	}
	req.Header.Set("X-Plex-Token", p.cfg.AccessToken)
	req.Header.Set("Accept", "application/xml")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("plex_watchlist_tv: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plex_watchlist_tv: server returned %d", resp.StatusCode)
	}

	var container struct {
		XMLName  xml.Name `xml:"MediaContainer"`
		Metadata []struct {
			Title string `xml:"title,attr"`
			Year  int    `xml:"year,attr"`
			Type  string `xml:"type,attr"`
			Guids []struct {
				ID string `xml:"id,attr"`
			} `xml:"Guid"`
		} `xml:"Video"`
	}
	if err := xml.NewDecoder(resp.Body).Decode(&container); err != nil {
		return nil, fmt.Errorf("plex_watchlist_tv: decoding response: %w", err)
	}

	items := make([]plugin.ImportListItem, 0, len(container.Metadata))
	for _, m := range container.Metadata {
		if m.Type != "" && m.Type != "show" {
			continue
		}
		var tmdbID int
		var imdbID string
		for _, g := range m.Guids {
			if after, ok := strings.CutPrefix(g.ID, "tmdb://"); ok {
				_, _ = fmt.Sscanf(after, "%d", &tmdbID)
			} else if after, ok := strings.CutPrefix(g.ID, "imdb://"); ok {
				imdbID = after
			}
		}
		if tmdbID == 0 {
			continue
		}
		items = append(items, plugin.ImportListItem{
			TMDbID: tmdbID,
			IMDbID: imdbID,
			Title:  m.Title,
			Year:   m.Year,
		})
	}

	return items, nil
}

func (p *Plugin) Test(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, watchlistURL+"&X-Plex-Container-Start=0&X-Plex-Container-Size=1", nil)
	if err != nil {
		return fmt.Errorf("plex_watchlist_tv: building request: %w", err)
	}
	req.Header.Set("X-Plex-Token", p.cfg.AccessToken)
	req.Header.Set("Accept", "application/xml")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("plex_watchlist_tv: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("plex_watchlist_tv: invalid access token")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("plex_watchlist_tv: server returned %d", resp.StatusCode)
	}
	return nil
}
