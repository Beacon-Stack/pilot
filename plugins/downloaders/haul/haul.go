// Package haul implements the plugin.DownloadClient interface for Beacon Haul,
// the native Beacon torrent download client.
package haul

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/beacon-stack/pilot/internal/registry"
	"github.com/beacon-stack/pilot/internal/safedialer"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

func init() {
	registry.Default.RegisterDownloader("haul", func(s json.RawMessage) (plugin.DownloadClient, error) {
		var cfg Config
		if err := json.Unmarshal(s, &cfg); err != nil {
			return nil, fmt.Errorf("haul: invalid settings: %w", err)
		}
		if cfg.URL == "" {
			return nil, errors.New("haul: url is required")
		}
		return New(cfg), nil
	})
	registry.Default.RegisterDownloaderSanitizer("haul", func(settings json.RawMessage) json.RawMessage {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(settings, &m); err != nil {
			return json.RawMessage("{}")
		}
		if _, ok := m["api_key"]; ok {
			m["api_key"] = json.RawMessage(`"***"`)
		}
		out, _ := json.Marshal(m)
		return out
	})
}

// Config holds the connection settings for a Haul instance.
type Config struct {
	URL      string `json:"url"`                 // e.g. "http://localhost:8484"
	APIKey   string `json:"api_key,omitempty"`   // Haul API key
	Category string `json:"category,omitempty"`  // category assigned to added torrents
	SavePath string `json:"save_path,omitempty"` // override default save path
}

// Client implements plugin.DownloadClient against the Haul REST API.
type Client struct {
	cfg  Config
	http *http.Client
}

// New creates a new Haul client.
func New(cfg Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: 30 * time.Second, Transport: safedialer.LANTransport()},
	}
}

func (c *Client) Name() string              { return "Haul" }
func (c *Client) Protocol() plugin.Protocol { return plugin.ProtocolTorrent }

// Test validates connectivity to the Haul instance.
func (c *Client) Test(ctx context.Context) error {
	resp, err := c.get(ctx, "/health")
	if err != nil {
		return fmt.Errorf("haul: connectivity check failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("haul: health check returned status %d", resp.StatusCode)
	}
	return nil
}

// Add submits a torrent to Haul via its REST API.
// For magnet links, sends the URI directly. For HTTP URLs (torznab proxy),
// downloads the .torrent file first and sends as a magnet or resolves the
// redirect — this avoids Haul needing network access to Pulse.
func (c *Client) Add(ctx context.Context, r plugin.Release) (string, error) {
	// Reject empty (or whitespace-only) download URL with a clear,
	// actionable error before we ship the request off to Haul. This
	// happens when the indexer's torznab response is missing the
	// <enclosure> tag (some scrapers like Pulse's Pirate Bay produce
	// this) — and occasionally when a scraper emits a single space or
	// a tab. Without the early check, Haul returns a confusing "either
	// uri or file must be provided" 422 that makes the operator chase
	// the wrong layer.
	uri := strings.TrimSpace(r.DownloadURL)
	if uri == "" {
		return "", fmt.Errorf("haul: release %q from indexer %q has no download URL — the torznab response is missing an enclosure tag; disable that indexer or fix its scraper definition",
			r.Title, r.Indexer)
	}

	// If the download URL is an HTTP(S) URL (not a magnet), resolve it locally
	// before sending to Haul. This handles torznab proxy URLs that may redirect
	// to magnets, and ensures Haul doesn't need to reach Pulse/indexer proxies.
	if strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
		resolved, err := c.resolveDownloadURL(ctx, uri)
		if err != nil {
			return "", fmt.Errorf("haul: resolving download URL: %w", err)
		}
		if resolved == "" {
			return "", fmt.Errorf("haul: download URL %q resolved to empty (torznab proxy returned no torrent or magnet) — try a different release", r.DownloadURL)
		}
		uri = resolved
	}

	body := map[string]any{
		"uri":      uri,
		"category": c.cfg.Category,
	}
	if c.cfg.SavePath != "" {
		body["save_path"] = c.cfg.SavePath
	}

	// Send media metadata so Haul can rename files on completion.
	if r.MediaTitle != "" {
		body["metadata"] = map[string]any{
			"requester":      "pilot",
			"media_type":     r.MediaType,
			"title":          r.MediaTitle,
			"year":           r.MediaYear,
			"season_number":  r.SeasonNumber,
			"episode_number": r.EpisodeNumber,
			"episode_title":  r.EpisodeTitle,
			"quality":        r.Quality.Name,
		}
	}

	data, _ := json.Marshal(body)
	resp, err := c.postJSON(ctx, "/api/v1/torrents", data)
	if err != nil {
		return "", fmt.Errorf("haul: add torrent failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
		return "", fmt.Errorf("haul: add returned status %d: %s", resp.StatusCode, string(errBody))
	}

	var result struct {
		InfoHash string `json:"info_hash"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("haul: decoding add response: %w", err)
	}
	return result.InfoHash, nil
}

// resolveDownloadURL fetches a torznab/indexer download URL from Pilot's network
// (which can reach Pulse) and returns either:
// - A magnet URI (if the URL redirects to one)
// - The original URL (if it serves a .torrent file — Haul will need to fetch it)
func (c *Client) resolveDownloadURL(ctx context.Context, downloadURL string) (string, error) {
	// Use a client that doesn't follow redirects so we can intercept magnet redirects.
	noRedirect := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := noRedirect.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Check for redirect to magnet.
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		location := resp.Header.Get("Location")
		if strings.HasPrefix(location, "magnet:") {
			return location, nil
		}
		// Follow non-magnet redirect and check again.
		return c.resolveDownloadURL(ctx, location)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, downloadURL)
	}

	// Read the response body — it's either a .torrent file or a magnet link.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20))
	if err != nil {
		return "", err
	}

	// Check if it's a magnet link in the body.
	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, "magnet:") {
		return trimmed, nil
	}

	// It's a .torrent file. We can't easily send raw bytes through the JSON API,
	// so we'll pass the original URL — but Haul can't reach it.
	// Instead, encode as a data URI or find the magnet from the torrent metadata.
	// For now, try to extract the info hash from the .torrent and build a magnet.
	// The torrent file contains a bencoded info dict with trackers.
	return extractMagnetFromTorrent(body)
}

// extractMagnetFromTorrent extracts the info hash from .torrent file bytes
// and constructs a magnet URI with tracker announces.
func extractMagnetFromTorrent(torrentData []byte) (string, error) {
	// Simple bencode parser: find "info" dict, SHA1 hash it for info_hash.
	// For robustness, we use a minimal approach.
	// Look for the info_hash by finding d8:announce and computing hash.
	// This is complex — instead, let's base64-encode and send as a data URI
	// that Haul can decode.

	// Actually, the simplest approach: encode the .torrent as base64 and
	// send it in the request body as a file upload. But our API uses JSON.
	// Let's add base64 torrent support to Haul's add endpoint.

	encoded := base64.StdEncoding.EncodeToString(torrentData)
	return "data:application/x-bittorrent;base64," + encoded, nil
}

// Status returns the current state of a torrent by its info hash.
func (c *Client) Status(ctx context.Context, clientItemID string) (plugin.QueueItem, error) {
	resp, err := c.get(ctx, "/api/v1/torrents/"+clientItemID)
	if err != nil {
		return plugin.QueueItem{}, fmt.Errorf("haul: status request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return plugin.QueueItem{}, fmt.Errorf("torrent %q not found in Haul", clientItemID)
	}

	var t torrentInfo
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return plugin.QueueItem{}, fmt.Errorf("haul: decoding status: %w", err)
	}
	return t.toQueueItem(), nil
}

// GetQueue returns all torrents tracked by Haul.
func (c *Client) GetQueue(ctx context.Context) ([]plugin.QueueItem, error) {
	resp, err := c.get(ctx, "/api/v1/torrents")
	if err != nil {
		return nil, fmt.Errorf("haul: queue request failed: %w", err)
	}
	defer resp.Body.Close()

	var torrents []torrentInfo
	if err := json.NewDecoder(resp.Body).Decode(&torrents); err != nil {
		return nil, fmt.Errorf("haul: decoding queue: %w", err)
	}

	items := make([]plugin.QueueItem, len(torrents))
	for i, t := range torrents {
		items[i] = t.toQueueItem()
	}
	return items, nil
}

// Remove deletes a torrent from Haul.
func (c *Client) Remove(ctx context.Context, clientItemID string, deleteFiles bool) error {
	url := fmt.Sprintf("/api/v1/torrents/%s?delete_files=%t", clientItemID, deleteFiles)
	resp, err := c.delete(ctx, url)
	if err != nil {
		return fmt.Errorf("haul: remove failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("haul: remove returned status %d", resp.StatusCode)
	}
	return nil
}

// SetSeedLimits implements plugin.SeedLimiter.
func (c *Client) SetSeedLimits(ctx context.Context, clientItemID string, ratioLimit float64, seedTimeSecs int) error {
	body := map[string]any{
		"ratio_limit":     ratioLimit,
		"time_limit_secs": seedTimeSecs,
	}
	data, _ := json.Marshal(body)
	resp, err := c.putJSON(ctx, "/api/v1/torrents/"+clientItemID+"/seed-limits", data)
	if err != nil {
		return fmt.Errorf("haul: set seed limits failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("haul: set seed limits returned status %d", resp.StatusCode)
	}
	return nil
}

// StalledTorrent is the shape Haul returns from GET /api/v1/stalls.
// Fields mirror haul/internal/core/torrent/stall.go's StalledTorrent —
// we keep them in sync informally. If Haul changes the shape this
// decode step fails loudly and the stall watcher returns an error.
type StalledTorrent struct {
	InfoHash     string    `json:"info_hash"`
	Name         string    `json:"name"`
	Level        int       `json:"level"`
	Reason       string    `json:"reason"`
	InactiveSecs int64     `json:"inactive_secs"`
	AddedAt      time.Time `json:"added_at"`
}

// ListStalled fetches the current stall list from Haul. Returns an empty
// slice (never nil) if Haul is reachable and nothing is stalled. Returns
// an error if Haul is unreachable or returns a non-200 — the caller
// decides whether to swallow or propagate.
func (c *Client) ListStalled(ctx context.Context) ([]StalledTorrent, error) {
	resp, err := c.get(ctx, "/api/v1/stalls")
	if err != nil {
		return nil, fmt.Errorf("haul: list stalled: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
		return nil, fmt.Errorf("haul: list stalled returned %d: %s", resp.StatusCode, string(errBody))
	}
	var out []StalledTorrent
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("haul: decoding stalled list: %w", err)
	}
	if out == nil {
		out = []StalledTorrent{}
	}
	return out, nil
}

// ── HTTP helpers ─────────────────────────────────────────────────────────────

func (c *Client) get(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.URL+path, nil)
	if err != nil {
		return nil, err
	}
	if c.cfg.APIKey != "" {
		req.Header.Set("X-Api-Key", c.cfg.APIKey)
	}
	return c.http.Do(req)
}

func (c *Client) postJSON(ctx context.Context, path string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.URL+path, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.APIKey != "" {
		req.Header.Set("X-Api-Key", c.cfg.APIKey)
	}
	return c.http.Do(req)
}

func (c *Client) putJSON(ctx context.Context, path string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.cfg.URL+path, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.APIKey != "" {
		req.Header.Set("X-Api-Key", c.cfg.APIKey)
	}
	return c.http.Do(req)
}

func (c *Client) delete(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.cfg.URL+path, nil)
	if err != nil {
		return nil, err
	}
	if c.cfg.APIKey != "" {
		req.Header.Set("X-Api-Key", c.cfg.APIKey)
	}
	return c.http.Do(req)
}

// ── Response types ───────────────────────────────────────────────────────────

type torrentInfo struct {
	InfoHash    string  `json:"info_hash"`
	Name        string  `json:"name"`
	Status      string  `json:"status"`
	Size        int64   `json:"size"`
	Downloaded  int64   `json:"downloaded"`
	SeedRatio   float64 `json:"seed_ratio"`
	ContentPath string  `json:"content_path"`
	AddedAt     string  `json:"added_at"`
}

func (t torrentInfo) toQueueItem() plugin.QueueItem {
	status := plugin.StatusDownloading
	switch t.Status {
	case "downloading":
		status = plugin.StatusDownloading
	case "seeding", "completed":
		status = plugin.StatusCompleted
	case "paused":
		status = plugin.StatusPaused
	case "queued":
		status = plugin.StatusQueued
	case "failed":
		status = plugin.StatusFailed
	}

	var addedAt int64
	if ts, err := time.Parse(time.RFC3339Nano, t.AddedAt); err == nil {
		addedAt = ts.Unix()
	}

	return plugin.QueueItem{
		ClientItemID: t.InfoHash,
		Title:        t.Name,
		Status:       status,
		Size:         t.Size,
		Downloaded:   t.Downloaded,
		SeedRatio:    t.SeedRatio,
		ContentPath:  t.ContentPath,
		AddedAt:      addedAt,
	}
}
