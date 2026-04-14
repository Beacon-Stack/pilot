package pulse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/beacon-stack/pilot/internal/core/downloader"
	"github.com/beacon-stack/pilot/internal/core/indexer"
	"github.com/beacon-stack/pilot/internal/core/mediamanagement"
	"github.com/beacon-stack/pilot/internal/core/quality"
	"github.com/beacon-stack/pilot/pkg/plugin"
	"github.com/beacon-stack/pulse/pkg/sdk"
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

// StartSyncLoop runs indexer, download client, quality profile, and shared
// settings sync on a periodic interval. It runs an immediate sync on start,
// then repeats every interval.
func (i *Integration) StartSyncLoop(ctx context.Context, indexerSvc *indexer.Service, dlSvc *downloader.Service, qualitySvc *quality.Service, mmSvc *mediamanagement.Service, interval time.Duration) {
	runOnce := func(prefix string) {
		if err := i.SyncIndexers(ctx, indexerSvc); err != nil {
			i.logger.Warn("pulse: "+prefix+" indexer sync failed", "error", err)
		}
		if err := i.SyncDownloadClients(ctx, dlSvc); err != nil {
			i.logger.Warn("pulse: "+prefix+" download client sync failed", "error", err)
		}
		if err := i.SyncQualityProfiles(ctx, qualitySvc); err != nil {
			i.logger.Warn("pulse: "+prefix+" quality profile sync failed", "error", err)
		}
		if err := i.SyncSharedSettings(ctx, mmSvc); err != nil {
			i.logger.Warn("pulse: "+prefix+" shared settings sync failed", "error", err)
		}
	}

	runOnce("initial")

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runOnce("periodic")
		}
	}
}

// SyncSharedSettings pulls the shared media handling settings from Pulse and
// overlays them on Pilot's local media_management row. Only the 4 shared fields
// are touched — episode naming templates and other per-service settings are untouched.
func (i *Integration) SyncSharedSettings(ctx context.Context, mmSvc *mediamanagement.Service) error {
	remote, err := i.Client.MySharedSettings(ctx)
	if err != nil {
		return fmt.Errorf("fetching shared settings: %w", err)
	}

	local, err := mmSvc.Get(ctx)
	if err != nil {
		return fmt.Errorf("reading local media management: %w", err)
	}

	remoteExts := parseExtensions(remote.ExtraFileExtensions)

	if local.ColonReplacement == remote.ColonReplacement &&
		local.ImportExtraFiles == remote.ImportExtraFiles &&
		local.RenameEpisodes == remote.RenameFiles &&
		extensionsEqual(local.ExtraFileExtensions, remoteExts) {
		return nil
	}

	local.ColonReplacement = remote.ColonReplacement
	local.ImportExtraFiles = remote.ImportExtraFiles
	local.ExtraFileExtensions = remoteExts
	local.RenameEpisodes = remote.RenameFiles // Pulse's rename_files → Pilot's rename_episodes

	if _, err := mmSvc.Update(ctx, local); err != nil {
		return fmt.Errorf("updating local media management: %w", err)
	}
	i.logger.Info("pulse: shared settings synced from control plane",
		"colon_replacement", remote.ColonReplacement,
		"rename_files", remote.RenameFiles)
	return nil
}

func parseExtensions(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func extensionsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
		"url":      fmt.Sprintf("%s://%s:%d", scheme, r.Host, r.Port),
		"username": r.Username,
		"pulse":    true, // marker so we know this was synced
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

// ── Quality Profile Sync ─────────────────────────────────────────────────────

// SyncQualityProfiles reconciles Pulse-managed quality profiles with the
// local database. Same pattern as Prism's implementation:
//   - match by ID (Pulse UUID is canonical)
//   - create missing, update changed, delete absent
//   - never touch managed_by_pulse=false (local shadows)
func (i *Integration) SyncQualityProfiles(ctx context.Context, qualitySvc *quality.Service) error {
	remote, err := i.Client.MyQualityProfiles(ctx)
	if err != nil {
		return fmt.Errorf("fetching quality profiles: %w", err)
	}

	managed, err := qualitySvc.ListManaged(ctx)
	if err != nil {
		return fmt.Errorf("listing managed quality profiles: %w", err)
	}

	managedByID := make(map[string]quality.Profile, len(managed))
	for _, p := range managed {
		managedByID[p.ID] = p
	}

	allLocal, err := qualitySvc.List(ctx)
	if err != nil {
		return fmt.Errorf("listing all quality profiles: %w", err)
	}
	allLocalIDs := make(map[string]bool, len(allLocal))
	for _, p := range allLocal {
		allLocalIDs[p.ID] = true
	}

	var created, updated, deleted, skipped, shadowed int
	remoteIDs := make(map[string]bool, len(remote))

	for _, r := range remote {
		remoteIDs[r.ID] = true

		req, err := remoteToCreateRequest(r)
		if err != nil {
			i.logger.Warn("pulse: failed to parse remote profile",
				"id", r.ID, "name", r.Name, "error", err)
			continue
		}

		if existing, ok := managedByID[r.ID]; ok {
			if profileNeedsUpdate(existing, req) {
				if _, err := qualitySvc.Update(ctx, r.ID, req); err != nil {
					i.logger.Warn("pulse: failed to update managed quality profile",
						"id", r.ID, "name", r.Name, "error", err)
					continue
				}
				updated++
			} else {
				skipped++
			}
		} else if allLocalIDs[r.ID] {
			// Local shadow — detached, do not touch.
			shadowed++
		} else {
			if _, err := qualitySvc.CreateManaged(ctx, r.ID, req); err != nil {
				i.logger.Warn("pulse: failed to create managed quality profile",
					"id", r.ID, "name", r.Name, "error", err)
				continue
			}
			created++
		}
	}

	for _, l := range managed {
		if remoteIDs[l.ID] {
			continue
		}
		if err := qualitySvc.Delete(ctx, l.ID); err != nil {
			if errors.Is(err, quality.ErrInUse) {
				i.logger.Info("pulse: managed quality profile in use, skipping delete",
					"id", l.ID, "name", l.Name)
				continue
			}
			i.logger.Warn("pulse: failed to delete managed quality profile",
				"id", l.ID, "name", l.Name, "error", err)
			continue
		}
		deleted++
	}

	i.logger.Info("pulse: quality profile sync complete",
		"remote", len(remote), "created", created, "updated", updated,
		"skipped", skipped, "shadowed", shadowed, "deleted", deleted)
	return nil
}

func remoteToCreateRequest(r sdk.QualityProfile) (quality.CreateRequest, error) {
	var cutoff plugin.Quality
	if r.CutoffJSON != "" && r.CutoffJSON != "{}" {
		if err := json.Unmarshal([]byte(r.CutoffJSON), &cutoff); err != nil {
			return quality.CreateRequest{}, fmt.Errorf("unmarshaling cutoff: %w", err)
		}
	}

	var qualities []plugin.Quality
	if r.QualitiesJSON != "" && r.QualitiesJSON != "[]" {
		if err := json.Unmarshal([]byte(r.QualitiesJSON), &qualities); err != nil {
			return quality.CreateRequest{}, fmt.Errorf("unmarshaling qualities: %w", err)
		}
	}

	var upgradeUntil *plugin.Quality
	if r.UpgradeUntilJSON != nil && *r.UpgradeUntilJSON != "" && *r.UpgradeUntilJSON != "null" {
		var q plugin.Quality
		if err := json.Unmarshal([]byte(*r.UpgradeUntilJSON), &q); err != nil {
			return quality.CreateRequest{}, fmt.Errorf("unmarshaling upgrade_until: %w", err)
		}
		upgradeUntil = &q
	}

	return quality.CreateRequest{
		Name:                 r.Name,
		Cutoff:               cutoff,
		Qualities:            qualities,
		UpgradeAllowed:       r.UpgradeAllowed,
		UpgradeUntil:         upgradeUntil,
		MinCustomFormatScore: r.MinCustomFormatScore,
		UpgradeUntilCFScore:  r.UpgradeUntilCFScore,
	}, nil
}

func profileNeedsUpdate(local quality.Profile, remote quality.CreateRequest) bool {
	if local.Name != remote.Name ||
		local.UpgradeAllowed != remote.UpgradeAllowed ||
		local.MinCustomFormatScore != remote.MinCustomFormatScore ||
		local.UpgradeUntilCFScore != remote.UpgradeUntilCFScore {
		return true
	}
	if !qualityEqual(local.Cutoff, remote.Cutoff) {
		return true
	}
	if len(local.Qualities) != len(remote.Qualities) {
		return true
	}
	for i := range local.Qualities {
		if !qualityEqual(local.Qualities[i], remote.Qualities[i]) {
			return true
		}
	}
	if (local.UpgradeUntil == nil) != (remote.UpgradeUntil == nil) {
		return true
	}
	if local.UpgradeUntil != nil && !qualityEqual(*local.UpgradeUntil, *remote.UpgradeUntil) {
		return true
	}
	return false
}

func qualityEqual(a, b plugin.Quality) bool {
	return a.Resolution == b.Resolution &&
		a.Source == b.Source &&
		a.Codec == b.Codec &&
		a.HDR == b.HDR &&
		a.AudioCodec == b.AudioCodec &&
		a.AudioChannels == b.AudioChannels &&
		a.Name == b.Name
}
