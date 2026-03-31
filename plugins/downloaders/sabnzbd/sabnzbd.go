// Package sabnzbd implements the plugin.DownloadClient interface for
// SABnzbd's REST API. Tested against SABnzbd 4.x.
package sabnzbd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/screenarr/screenarr/internal/registry"
	"github.com/screenarr/screenarr/internal/safedialer"
	"github.com/screenarr/screenarr/pkg/plugin"
)

func init() {
	registry.Default.RegisterDownloader("sabnzbd", func(s json.RawMessage) (plugin.DownloadClient, error) {
		var cfg Config
		if err := json.Unmarshal(s, &cfg); err != nil {
			return nil, fmt.Errorf("sabnzbd: invalid settings: %w", err)
		}
		if cfg.URL == "" {
			return nil, errors.New("sabnzbd: url is required")
		}
		if cfg.APIKey == "" {
			return nil, errors.New("sabnzbd: api_key is required")
		}
		return New(cfg), nil
	})
	registry.Default.RegisterDownloaderSanitizer("sabnzbd", func(settings json.RawMessage) json.RawMessage {
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

// Config holds the connection settings for a SABnzbd instance.
type Config struct {
	URL      string `json:"url"`                // e.g. "http://localhost:8080"
	APIKey   string `json:"api_key"`            // full API key
	Category string `json:"category,omitempty"` // category applied to added NZBs
}

// Client implements plugin.DownloadClient against the SABnzbd API.
type Client struct {
	cfg  Config
	http *http.Client
	base string // precomputed base: {url}/sabnzbd/api?output=json&apikey={key}
}

// New creates a new SABnzbd client.
func New(cfg Config) *Client {
	base := strings.TrimRight(cfg.URL, "/") + "/sabnzbd/api?output=json&apikey=" + url.QueryEscape(cfg.APIKey)
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: 30 * time.Second, Transport: safedialer.LANTransport()},
		base: base,
	}
}

// NewWithHTTPClient creates a Client with a caller-supplied http.Client.
// Intended for unit tests that need to bypass the safe dialer.
func NewWithHTTPClient(cfg Config, client *http.Client) *Client {
	base := strings.TrimRight(cfg.URL, "/") + "/sabnzbd/api?output=json&apikey=" + url.QueryEscape(cfg.APIKey)
	return &Client{cfg: cfg, http: client, base: base}
}

func (c *Client) Name() string              { return "SABnzbd" }
func (c *Client) Protocol() plugin.Protocol { return plugin.ProtocolNZB }

// Test verifies connectivity by fetching the version, then validates the
// API key by requesting a queue summary.
func (c *Client) Test(ctx context.Context) error {
	// version does not require an API key — verifies base connectivity.
	versionURL := strings.TrimRight(c.cfg.URL, "/") + "/sabnzbd/api?output=json&mode=version"
	var ver versionResponse
	if err := c.getJSON(ctx, versionURL, &ver); err != nil {
		return fmt.Errorf("sabnzbd: connectivity check failed: %w", err)
	}
	if ver.Version == "" {
		return errors.New("sabnzbd: version endpoint returned empty version")
	}

	// Validate API key by fetching queue.
	var q queueWrapper
	if err := c.getJSON(ctx, c.base+"&mode=queue&limit=0", &q); err != nil {
		return fmt.Errorf("sabnzbd: API key check failed: %w", err)
	}
	return nil
}

// Add submits an NZB by URL to SABnzbd. Returns the nzo_id as client item ID.
func (c *Client) Add(ctx context.Context, r plugin.Release) (string, error) {
	u := c.base + "&mode=addurl&name=" + url.QueryEscape(r.DownloadURL)
	if r.Title != "" {
		u += "&nzbname=" + url.QueryEscape(r.Title)
	}
	if c.cfg.Category != "" {
		u += "&cat=" + url.QueryEscape(c.cfg.Category)
	}

	var resp addResponse
	if err := c.getJSON(ctx, u, &resp); err != nil {
		return "", fmt.Errorf("sabnzbd: addurl: %w", err)
	}
	if !resp.Status {
		return "", fmt.Errorf("sabnzbd: addurl failed: %s", resp.Error)
	}
	if len(resp.NzoIDs) == 0 {
		return "", errors.New("sabnzbd: addurl returned no nzo_id")
	}
	return resp.NzoIDs[0], nil
}

// Status returns the state of a single item by nzo_id.
// Checks the active queue first, then history.
func (c *Client) Status(ctx context.Context, clientItemID string) (plugin.QueueItem, error) {
	// Check active queue.
	queue, err := c.fetchQueue(ctx)
	if err != nil {
		return plugin.QueueItem{}, err
	}
	for _, slot := range queue {
		if slot.NzoID == clientItemID {
			return slot.toQueueItem(), nil
		}
	}

	// Check history.
	history, err := c.fetchHistory(ctx, 200)
	if err != nil {
		return plugin.QueueItem{}, err
	}
	for _, slot := range history {
		if slot.NzoID == clientItemID {
			return slot.toQueueItem(), nil
		}
	}

	return plugin.QueueItem{}, fmt.Errorf("sabnzbd: item %q not found in queue or history", clientItemID)
}

// GetQueue returns all items from the active queue plus recent history.
func (c *Client) GetQueue(ctx context.Context) ([]plugin.QueueItem, error) {
	queue, err := c.fetchQueue(ctx)
	if err != nil {
		return nil, err
	}
	history, err := c.fetchHistory(ctx, 50)
	if err != nil {
		return nil, err
	}

	items := make([]plugin.QueueItem, 0, len(queue)+len(history))
	for _, s := range queue {
		items = append(items, s.toQueueItem())
	}
	for _, s := range history {
		items = append(items, s.toQueueItem())
	}
	return items, nil
}

// Remove deletes an item from SABnzbd. Tries queue first, then history.
func (c *Client) Remove(ctx context.Context, clientItemID string, deleteFiles bool) error {
	delFiles := "0"
	if deleteFiles {
		delFiles = "1"
	}

	// Try queue deletion.
	queueURL := c.base + "&mode=queue&name=delete&value=" + url.QueryEscape(clientItemID) + "&del_files=" + delFiles
	var qResp deleteResponse
	if err := c.getJSON(ctx, queueURL, &qResp); err != nil {
		return fmt.Errorf("sabnzbd: queue delete: %w", err)
	}
	if qResp.Status {
		return nil
	}

	// Try history deletion.
	histURL := c.base + "&mode=history&name=delete&value=" + url.QueryEscape(clientItemID) + "&del_files=" + delFiles
	var hResp deleteResponse
	if err := c.getJSON(ctx, histURL, &hResp); err != nil {
		return fmt.Errorf("sabnzbd: history delete: %w", err)
	}
	if !hResp.Status {
		return fmt.Errorf("sabnzbd: item %q not found for deletion", clientItemID)
	}
	return nil
}

// ── API helpers ──────────────────────────────────────────────────────────────

func (c *Client) getJSON(ctx context.Context, rawURL string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) fetchQueue(ctx context.Context) ([]queueSlot, error) {
	var q queueWrapper
	if err := c.getJSON(ctx, c.base+"&mode=queue&limit=0", &q); err != nil {
		return nil, fmt.Errorf("sabnzbd: fetch queue: %w", err)
	}
	return q.Queue.Slots, nil
}

func (c *Client) fetchHistory(ctx context.Context, limit int) ([]historySlot, error) {
	u := c.base + "&mode=history&limit=" + strconv.Itoa(limit)
	var h historyWrapper
	if err := c.getJSON(ctx, u, &h); err != nil {
		return nil, fmt.Errorf("sabnzbd: fetch history: %w", err)
	}
	return h.History.Slots, nil
}

// ── Response types ───────────────────────────────────────────────────────────

type versionResponse struct {
	Version string `json:"version"`
}

type addResponse struct {
	Status bool     `json:"status"`
	NzoIDs []string `json:"nzo_ids"`
	Error  string   `json:"error"`
}

type deleteResponse struct {
	Status bool `json:"status"`
}

type queueWrapper struct {
	Queue struct {
		Slots []queueSlot `json:"slots"`
	} `json:"queue"`
}

type queueSlot struct {
	NzoID      string `json:"nzo_id"`
	Filename   string `json:"filename"`
	Status     string `json:"status"`
	MB         string `json:"mb"`
	MBLeft     string `json:"mbleft"`
	Percentage string `json:"percentage"`
}

func (s queueSlot) toQueueItem() plugin.QueueItem {
	totalMB, _ := strconv.ParseFloat(s.MB, 64)
	leftMB, _ := strconv.ParseFloat(s.MBLeft, 64)
	totalBytes := int64(totalMB * 1024 * 1024)
	downloadedBytes := int64((totalMB - leftMB) * 1024 * 1024)

	return plugin.QueueItem{
		ClientItemID: s.NzoID,
		Title:        s.Filename,
		Status:       mapQueueStatus(s.Status),
		Size:         totalBytes,
		Downloaded:   downloadedBytes,
	}
}

type historyWrapper struct {
	History struct {
		Slots []historySlot `json:"slots"`
	} `json:"history"`
}

type historySlot struct {
	NzoID       string `json:"nzo_id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Bytes       int64  `json:"bytes"`
	Downloaded  int64  `json:"downloaded"`
	Storage     string `json:"storage"`
	FailMessage string `json:"fail_message"`
	Completed   int64  `json:"completed"`
}

func (s historySlot) toQueueItem() plugin.QueueItem {
	return plugin.QueueItem{
		ClientItemID: s.NzoID,
		Title:        s.Name,
		Status:       mapHistoryStatus(s.Status),
		Size:         s.Bytes,
		Downloaded:   s.Downloaded,
		Error:        s.FailMessage,
		ContentPath:  s.Storage,
		AddedAt:      s.Completed,
	}
}

// ── Status mapping ───────────────────────────────────────────────────────────

func mapQueueStatus(status string) plugin.DownloadStatus {
	switch status {
	case "Downloading":
		return plugin.StatusDownloading
	case "Paused":
		return plugin.StatusPaused
	case "Queued", "Fetching", "Grabbing", "Propagating", "Checking":
		return plugin.StatusQueued
	default:
		return plugin.StatusQueued
	}
}

func mapHistoryStatus(status string) plugin.DownloadStatus {
	if strings.HasPrefix(status, "Failed") {
		return plugin.StatusFailed
	}
	if strings.HasPrefix(status, "Completed") {
		return plugin.StatusCompleted
	}
	// Post-processing states.
	switch status {
	case "Extracting", "Verifying", "Repairing", "Moving", "Running":
		return plugin.StatusDownloading
	default:
		return plugin.StatusQueued
	}
}
