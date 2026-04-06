package pulse

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/beacon-media/pulse/pkg/sdk"
	"github.com/beacon-media/pilot/internal/core/downloader"
	"github.com/beacon-media/pilot/internal/core/indexer"
)

// SyncIndexers pulls indexers assigned to this service from Pulse
// and ensures they exist in Prism's local indexer database.
// New indexers are created; existing ones (matched by name) are left as-is.
func (i *Integration) SyncIndexers(ctx context.Context, indexerSvc *indexer.Service) error {
	remote, err := i.Client.MyIndexers(ctx)
	if err != nil {
		return fmt.Errorf("fetching assigned indexers: %w", err)
	}

	// Note: remote can be empty — that means all indexers were unassigned.
	// We still need to run the delete logic below to clean up local copies.

	// Get existing local indexers.
	local, err := indexerSvc.List(ctx)
	if err != nil {
		return fmt.Errorf("listing local indexers: %w", err)
	}
	localByName := make(map[string]indexer.Config, len(local))
	for _, l := range local {
		localByName[l.Name] = l
	}

	var created, updated, skipped int
	for _, r := range remote {
		settings := buildSettings(r)

		if existing, ok := localByName[r.Name]; ok {
			// Check if the URL changed (e.g., rewritten to proxy URL).
			if string(existing.Settings) != string(settings) {
				_, err := indexerSvc.Update(ctx, existing.ID, indexer.UpdateRequest{
					Name:     r.Name,
					Kind:     r.Kind,
					Enabled:  r.Enabled,
					Priority: r.Priority,
					Settings: settings,
				})
				if err != nil {
					i.logger.Warn("pulse: failed to update indexer",
						"indexer", r.Name, "error", err)
				} else {
					updated++
					i.logger.Info("pulse: updated indexer from control plane",
						"indexer", r.Name)
				}
			} else {
				skipped++
			}
			continue
		}

		_, err := indexerSvc.Create(ctx, indexer.CreateRequest{
			Name:     r.Name,
			Kind:     r.Kind,
			Enabled:  r.Enabled,
			Priority: r.Priority,
			Settings: settings,
		})
		if err != nil {
			i.logger.Warn("pulse: failed to sync indexer",
				"indexer", r.Name, "error", err)
			continue
		}
		created++
		i.logger.Info("pulse: synced indexer from control plane",
			"indexer", r.Name, "kind", r.Kind)
	}

	// Remove local indexers that are no longer assigned in Pulse.
	// Only delete ones that were synced from Pulse (proxy URLs).
	remoteNames := make(map[string]bool, len(remote))
	for _, r := range remote {
		remoteNames[r.Name] = true
	}

	var deleted int
	for _, l := range local {
		if remoteNames[l.Name] {
			continue
		}
		// Only delete if it looks like a Pulse-synced indexer (has proxy URL).
		settings := string(l.Settings)
		if !strings.Contains(settings, "/api/v1/torznab/") {
			continue
		}
		if err := indexerSvc.Delete(ctx, l.ID); err != nil {
			i.logger.Warn("pulse: failed to delete removed indexer",
				"indexer", l.Name, "error", err)
			continue
		}
		deleted++
		i.logger.Info("pulse: removed indexer (no longer assigned by control plane)",
			"indexer", l.Name)
	}

	i.logger.Info("pulse: indexer sync complete",
		"remote", len(remote), "created", created, "updated", updated, "skipped", skipped, "deleted", deleted)
	return nil
}

// StartSyncLoop runs indexer and download client sync on a periodic interval.
// It runs an immediate sync on start, then repeats every interval.
func (i *Integration) StartSyncLoop(ctx context.Context, indexerSvc *indexer.Service, dlSvc *downloader.Service, interval time.Duration) {
	// Immediate sync on startup.
	if err := i.SyncIndexers(ctx, indexerSvc); err != nil {
		i.logger.Warn("pulse: initial indexer sync failed", "error", err)
	}
	if err := i.SyncDownloadClients(ctx, dlSvc); err != nil {
		i.logger.Warn("pulse: initial download client sync failed", "error", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := i.SyncIndexers(ctx, indexerSvc); err != nil {
				i.logger.Warn("pulse: periodic indexer sync failed", "error", err)
			}
			if err := i.SyncDownloadClients(ctx, dlSvc); err != nil {
				i.logger.Warn("pulse: periodic download client sync failed", "error", err)
			}
		}
	}
}

// buildSettings creates the JSON settings blob that Prism's torznab/newznab
// plugins expect: {"url": "...", "api_key": "..."}.
func buildSettings(r sdk.Indexer) json.RawMessage {
	// The Pulse indexer stores the URL in the top-level field and
	// extra settings (including api_key) in the settings JSON blob.
	// Merge them into the format Prism plugins expect.
	m := map[string]string{"url": r.URL}

	// Try to extract api_key from the settings JSON.
	var extra map[string]string
	if err := json.Unmarshal([]byte(r.Settings), &extra); err == nil {
		for k, v := range extra {
			m[k] = v
		}
	}

	out, _ := json.Marshal(m)
	return out
}

// ── Download Client Sync ─────────────────────────────────────────────────────

// SyncDownloadClients pulls download clients from Pulse and syncs them
// into Prism's local database. Same pattern as indexer sync.
func (i *Integration) SyncDownloadClients(ctx context.Context, dlSvc *downloader.Service) error {
	remote, err := i.Client.MyDownloadClients(ctx)
	if err != nil {
		return fmt.Errorf("fetching download clients: %w", err)
	}

	local, err := dlSvc.List(ctx)
	if err != nil {
		return fmt.Errorf("listing local download clients: %w", err)
	}
	localByName := make(map[string]downloader.Config, len(local))
	for _, l := range local {
		localByName[l.Name] = l
	}

	var created, updated, skipped int
	for _, r := range remote {
		if !r.Enabled {
			continue
		}

		settings := buildDownloadClientSettings(r)

		if existing, ok := localByName[r.Name]; ok {
			if string(existing.Settings) != string(settings) {
				_, err := dlSvc.Update(ctx, existing.ID, downloader.UpdateRequest{
					Name:     r.Name,
					Kind:     r.Kind,
					Enabled:  r.Enabled,
					Priority: r.Priority,
					Settings: settings,
				})
				if err != nil {
					i.logger.Warn("pulse: failed to update download client",
						"client", r.Name, "error", err)
				} else {
					updated++
				}
			} else {
				skipped++
			}
			continue
		}

		_, err := dlSvc.Create(ctx, downloader.CreateRequest{
			Name:     r.Name,
			Kind:     r.Kind,
			Enabled:  r.Enabled,
			Priority: r.Priority,
			Settings: settings,
		})
		if err != nil {
			i.logger.Warn("pulse: failed to sync download client",
				"client", r.Name, "error", err)
			continue
		}
		created++
		i.logger.Info("pulse: synced download client from control plane",
			"client", r.Name, "kind", r.Kind)
	}

	// Delete local clients that were synced from Pulse but no longer exist.
	remoteNames := make(map[string]bool, len(remote))
	for _, r := range remote {
		remoteNames[r.Name] = true
	}
	var deleted int
	for _, l := range local {
		if remoteNames[l.Name] {
			continue
		}
		// Only delete if it looks like a Pulse-synced client (has pulse marker).
		settings := string(l.Settings)
		if !strings.Contains(settings, "pulse") {
			continue
		}
		if err := dlSvc.Delete(ctx, l.ID); err != nil {
			continue
		}
		deleted++
		i.logger.Info("pulse: removed download client (no longer in control plane)",
			"client", l.Name)
	}

	i.logger.Info("pulse: download client sync complete",
		"remote", len(remote), "created", created, "updated", updated, "skipped", skipped, "deleted", deleted)
	return nil
}

// buildDownloadClientSettings converts a Pulse download client to the
// JSON settings format Prism's downloader plugins expect.
func buildDownloadClientSettings(r sdk.DownloadClient) json.RawMessage {
	scheme := "http"
	if r.UseSSL {
		scheme = "https"
	}

	m := map[string]interface{}{
		"url":          fmt.Sprintf("%s://%s:%d", scheme, r.Host, r.Port),
		"username":     r.Username,
		"pulse":  true, // marker so we know this was synced
	}

	if r.Category != "" {
		m["category"] = r.Category
	}
	if r.Directory != "" {
		m["save_path"] = r.Directory
	}

	// Merge any extra settings from Pulse
	var extra map[string]interface{}
	if err := json.Unmarshal([]byte(r.Settings), &extra); err == nil {
		for k, v := range extra {
			if k != "url" && k != "username" && k != "pulse" {
				m[k] = v
			}
		}
	}

	out, _ := json.Marshal(m)
	return out
}

// logField returns a slog attribute for debug logging.
func logField(key, value string) slog.Attr {
	return slog.String(key, value)
}
