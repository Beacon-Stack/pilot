// Package sonarrimport fetches data from a running Sonarr instance and
// creates matching records in Pilot's database using the existing service
// layer. It is used only for the one-time migration wizard in the UI.
package sonarrimport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/beacon-media/pilot/internal/safedialer"
)

// ── Sonarr API types ─────────────────────────────────────────────────────────

type sonarrStatus struct {
	Version string `json:"version"`
}

type sonarrProfile struct {
	ID             int                 `json:"id"`
	Name           string              `json:"name"`
	UpgradeAllowed bool                `json:"upgradeAllowed"`
	Cutoff         int                 `json:"cutoff"`
	Items          []sonarrProfileItem `json:"items"`
}

type sonarrProfileQuality struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type sonarrProfileItem struct {
	Quality sonarrProfileQuality `json:"quality"`
	Items   []sonarrProfileItem  `json:"items"`
	Allowed bool                 `json:"allowed"`
}

func (item sonarrProfileItem) qualities() []sonarrProfileQuality {
	if len(item.Items) == 0 {
		if item.Quality.ID == 0 {
			return nil
		}
		return []sonarrProfileQuality{item.Quality}
	}
	var out []sonarrProfileQuality
	for _, child := range item.Items {
		out = append(out, child.qualities()...)
	}
	return out
}

type sonarrRootFolder struct {
	Path       string `json:"path"`
	FreeSpace  int64  `json:"freeSpace"`
	Accessible bool   `json:"accessible"`
}

type sonarrField struct {
	Name  string          `json:"name"`
	Value json.RawMessage `json:"value"`
}

func (f sonarrField) stringValue() string {
	var s string
	if err := json.Unmarshal(f.Value, &s); err != nil {
		return ""
	}
	return s
}

func (f sonarrField) intValue() int {
	var n int
	if err := json.Unmarshal(f.Value, &n); err != nil {
		return 0
	}
	return n
}

func (f sonarrField) boolValue() bool {
	var b bool
	if err := json.Unmarshal(f.Value, &b); err != nil {
		return false
	}
	return b
}

type sonarrIndexer struct {
	ID             int           `json:"id"`
	Name           string        `json:"name"`
	ConfigContract string        `json:"configContract"`
	EnableRss      bool          `json:"enableRss"`
	Fields         []sonarrField `json:"fields"`
}

type sonarrDownloadClient struct {
	ID             int           `json:"id"`
	Name           string        `json:"name"`
	ConfigContract string        `json:"configContract"`
	Enable         bool          `json:"enable"`
	Fields         []sonarrField `json:"fields"`
}

type sonarrSeries struct {
	ID               int    `json:"id"`
	TvdbID           int    `json:"tvdbId"`
	TmdbID           int    `json:"tmdbId"`
	Title            string `json:"title"`
	Monitored        bool   `json:"monitored"`
	QualityProfileID int    `json:"qualityProfileId"`
	RootFolderPath   string `json:"rootFolderPath"`
	SeriesType       string `json:"seriesType"` // "standard", "anime", "daily"
}

// ── Client ───────────────────────────────────────────────────────────────────

// Client is an HTTP client for the Sonarr v3 API.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// NewClient creates a new Sonarr API client. baseURL should be the root URL
// of the Sonarr instance, e.g. "http://sonarr.local:8989".
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 30 * time.Second, Transport: safedialer.LANTransport()},
	}
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("X-Api-Key", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to Sonarr: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("Sonarr rejected the API key (401 Unauthorized)")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Sonarr returned HTTP %d for %s", resp.StatusCode, path)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding Sonarr response from %s: %w", path, err)
	}
	return nil
}

func (c *Client) GetStatus(ctx context.Context) (*sonarrStatus, error) {
	var s sonarrStatus
	if err := c.get(ctx, "/api/v3/system/status", &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (c *Client) GetQualityProfiles(ctx context.Context) ([]sonarrProfile, error) {
	var profiles []sonarrProfile
	if err := c.get(ctx, "/api/v3/qualityprofile", &profiles); err != nil {
		return nil, err
	}
	return profiles, nil
}

func (c *Client) GetRootFolders(ctx context.Context) ([]sonarrRootFolder, error) {
	var folders []sonarrRootFolder
	if err := c.get(ctx, "/api/v3/rootfolder", &folders); err != nil {
		return nil, err
	}
	return folders, nil
}

func (c *Client) GetIndexers(ctx context.Context) ([]sonarrIndexer, error) {
	var indexers []sonarrIndexer
	if err := c.get(ctx, "/api/v3/indexer", &indexers); err != nil {
		return nil, err
	}
	return indexers, nil
}

func (c *Client) GetDownloadClients(ctx context.Context) ([]sonarrDownloadClient, error) {
	var clients []sonarrDownloadClient
	if err := c.get(ctx, "/api/v3/downloadclient", &clients); err != nil {
		return nil, err
	}
	return clients, nil
}

func (c *Client) GetSeries(ctx context.Context) ([]sonarrSeries, error) {
	var series []sonarrSeries
	if err := c.get(ctx, "/api/v3/series", &series); err != nil {
		return nil, err
	}
	return series, nil
}

// ── Field helpers ────────────────────────────────────────────────────────────

func fieldString(fields []sonarrField, name string) string {
	for _, f := range fields {
		if f.Name == name {
			return f.stringValue()
		}
	}
	return ""
}

func fieldInt(fields []sonarrField, name string) int {
	for _, f := range fields {
		if f.Name == name {
			return f.intValue()
		}
	}
	return 0
}

func fieldBool(fields []sonarrField, name string) bool {
	for _, f := range fields {
		if f.Name == name {
			return f.boolValue()
		}
	}
	return false
}

func buildURL(host string, port int, useSsl bool, urlBase string) string {
	scheme := "http"
	if useSsl {
		scheme = "https"
	}
	base := fmt.Sprintf("%s://%s:%d", scheme, host, port)
	if urlBase != "" && urlBase != "/" {
		base += "/" + strings.Trim(urlBase, "/")
	}
	return base
}
