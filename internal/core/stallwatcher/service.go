// Package stallwatcher is the Pilot-side consumer of Haul's stall detection.
//
// Every 60 seconds it polls Haul's /api/v1/stalls endpoint, correlates each
// stalled torrent with a grab_history row (via info_hash), writes a
// blocklist entry for the release (reason tagged with the stall type), and
// marks the grab as stalled. If the grab was triggered by auto-search, the
// watcher also triggers a re-search for the episode, capped by a circuit
// breaker to avoid loops when every release for an episode is dead.
//
// Design notes in plans/dead-torrent-phase0.md. Specifically: this is
// pull-based (not webhooks), runs entirely in Pilot, and tolerates Haul
// being briefly unreachable without fallout. The watcher does NOT
// blocklist during its own first 2 minutes of uptime — that's to absorb
// restart races where grabs from before the restart look "stalled" because
// we just haven't observed them yet.
//
// ⚠ Before changing anything in this file, run:
//
//	go test ./internal/core/stallwatcher/...
//
// The tests (service_test.go) pin every critical contract:
//   - startup grace suppression
//   - circuit breaker behavior
//   - interactive-vs-auto_search retry gating
//   - unique-info_hash cross-indexer dedup
//   - idempotency on already-stalled grabs
//   - graceful degradation when Haul is down
//
// See pilot/CLAUDE.md for the full regression-guard rationale.
package stallwatcher

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/beacon-stack/pilot/internal/core/blocklist"
	"github.com/beacon-stack/pilot/internal/core/dbutil"
	"github.com/beacon-stack/pilot/internal/core/downloader"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/events"
	"github.com/beacon-stack/pilot/plugins/downloaders/haul"
)

// downloadClientLister is the narrow interface stallwatcher needs from
// the downloader service. Using an interface here rather than the
// concrete *downloader.Service lets tests inject a fake that returns
// whatever download client config they want without needing a real DB.
type downloadClientLister interface {
	List(ctx context.Context) ([]downloader.Config, error)
}

// MaxStallRetriesPerEpisode is the circuit breaker: after this many
// stall-reason blocklist entries for a (series, episode) in the last 24
// hours, auto-re-search stops. Prevents infinite loops when every
// release for an episode happens to be dead.
const MaxStallRetriesPerEpisode = 3

// StaleGrabAge is how long a grab can sit in `downloading` / `queued` /
// `pending` without an info_hash before the sweep marks it failed. The
// info_hash-less branch is invisible to the haul-stall path (haul keys
// stalls by info_hash), so the only signal we have is age. 24h is well
// past any normal "metadata fetch + tracker announce + first peer"
// startup, and it leaves a generous window for genuinely-slow first
// connects on remote VPN endpoints.
const StaleGrabAge = 24 * time.Hour

// startupGrace is how long after the watcher starts before it will
// actually blocklist anything. A Pilot restart races with Haul — a grab
// from 10 minutes ago might already have been archived by Haul and show
// up in /api/v1/stalls, but Pilot has no grab_history correlation built
// yet. Give everything 2 minutes to catch up before acting.
const startupGrace = 2 * time.Minute

// Service polls Haul for stalled torrents and reacts by blocklisting the
// corresponding release.
type Service struct {
	q          db.Querier
	blocklist  *blocklist.Service
	downloader downloadClientLister
	bus        *events.Bus
	logger     *slog.Logger

	startedAt time.Time
	interval  time.Duration
}

// NewService constructs a stallwatcher Service. Call Run in a goroutine.
func NewService(
	q db.Querier,
	blocklist *blocklist.Service,
	downloader *downloader.Service,
	bus *events.Bus,
	logger *slog.Logger,
) *Service {
	return &Service{
		q:          q,
		blocklist:  blocklist,
		downloader: downloader,
		bus:        bus,
		logger:     logger,
		startedAt:  time.Now(),
		interval:   60 * time.Second,
	}
}

// Run is a blocking poll loop. Returns when ctx is canceled.
func (s *Service) Run(ctx context.Context) {
	// Do a first tick fairly quickly so developers see behavior without
	// waiting a full minute, but still well into the startup grace period.
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if err := s.Tick(ctx); err != nil {
				s.logger.Warn("stallwatcher: tick error", "error", err)
			}
			timer.Reset(s.interval)
		}
	}
}

// Tick runs one poll cycle. Exposed for tests and manual invocation.
func (s *Service) Tick(ctx context.Context) error {
	// Startup grace — don't blocklist in the first 2 minutes of watcher life.
	// We still poll and log, but skip the side effects.
	inGrace := time.Since(s.startedAt) < startupGrace

	// First: sweep stuck grabs that have no info_hash. Those never make
	// it into haul's stall reporting (haul keys by info_hash), so the
	// info_hash-keyed path below is blind to them. They sit in
	// `downloading` indefinitely and the UI keeps rendering them as
	// active. Cheap query — runs every tick regardless of haul state.
	if !inGrace {
		if err := s.sweepStaleGrabs(ctx); err != nil {
			s.logger.Warn("stallwatcher: stale-grab sweep", "error", err)
		}
	}

	client, err := s.resolveHaulClient(ctx)
	if err != nil {
		if errors.Is(err, errNoHaulConfigured) {
			// No haul client means nothing for us to watch. Not an error.
			return nil
		}
		return fmt.Errorf("resolving haul client: %w", err)
	}
	if client == nil {
		return nil
	}

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	stalled, err := client.ListStalled(reqCtx)
	if err != nil {
		return fmt.Errorf("haul list stalled: %w", err)
	}

	if len(stalled) == 0 {
		return nil
	}

	for _, st := range stalled {
		if err := s.handleStall(ctx, st, inGrace); err != nil {
			s.logger.Warn("stallwatcher: handle stall",
				"info_hash", st.InfoHash, "reason", st.Reason, "error", err)
		}
	}
	return nil
}

// sweepStaleGrabs marks grab_history rows as `failed` when they're
// stuck in a non-terminal state (`downloading`, `queued`, `pending`)
// AND have no info_hash AND are older than StaleGrabAge.
//
// Why this is needed: a grab that gets recorded but never has its
// info_hash populated (download client never responded, or the grab
// raced with a haul restart that lost the in-memory torrent before the
// hash plumbed back to pilot) is invisible to the haul-stall path.
// The haul-stall path queries grab_history by info_hash; without one,
// there's no row to update. So these grabs sit in `downloading`
// forever and the UI shows them as active.
//
// 24h is intentionally conservative — the operator's expectation is
// "this should have completed by now." If we see >0 sweeps, there's
// either a flaky download client, a race in the grab handler, or a
// haul restart that lost state. Any of those is worth knowing about.
func (s *Service) sweepStaleGrabs(ctx context.Context) error {
	// grab_history.grabbed_at is RFC3339 text; lexicographic ordering
	// is timestamp-correct. Compute the cutoff in the same format.
	cutoff := time.Now().UTC().Add(-StaleGrabAge).Format(time.RFC3339)

	stale, err := s.q.ListStaleActiveGrabs(ctx, cutoff)
	if err != nil {
		return fmt.Errorf("listing stale grabs: %w", err)
	}
	if len(stale) == 0 {
		return nil
	}

	// Mark each as failed. We don't blocklist (no info_hash, no release
	// to ban — and the user may legitimately re-grab the same release
	// later through a different path that does record the hash).
	for _, g := range stale {
		if err := s.q.UpdateGrabStatus(ctx, db.UpdateGrabStatusParams{
			DownloadStatus:  "failed",
			DownloadedBytes: g.DownloadedBytes,
			ID:              g.ID,
		}); err != nil {
			s.logger.Warn("stallwatcher: failed to mark stale grab failed",
				"grab_id", g.ID, "error", err)
			continue
		}
		s.logger.Info("stallwatcher: expired stuck-active grab",
			"grab_id", g.ID,
			"release_title", g.ReleaseTitle,
			"prior_status", g.DownloadStatus,
			"grabbed_at", g.GrabbedAt,
			"reason", "no info_hash + age > "+StaleGrabAge.String())
	}
	s.logger.Info("stallwatcher: stale-grab sweep complete",
		"expired", len(stale), "cutoff", cutoff)
	return nil
}

// handleStall correlates a single stalled torrent with grab history and,
// if a match is found, blocklists the release and marks the grab stalled.
func (s *Service) handleStall(ctx context.Context, st haul.StalledTorrent, inGrace bool) error {
	// Find the grab_history row for this info_hash.
	grab, err := s.q.GetGrabByInfoHash(ctx, &st.InfoHash)
	if err != nil {
		// If no grab matches this info_hash, it's a torrent Pilot didn't
		// initiate (maybe added via Haul UI directly, or a test torrent).
		// Nothing to do.
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("lookup grab by info_hash: %w", err)
	}

	// Skip already-terminal grabs — no point blocklisting a finished
	// download. Possible if Haul took longer than our poll interval to
	// archive a stalled torrent that had briefly received data.
	switch grab.DownloadStatus {
	case "completed", "failed", "removed", "stalled":
		return nil
	}

	reason := mapStallReason(st.Reason)
	if inGrace {
		s.logger.Info("stallwatcher: stall detected during startup grace, skipping blocklist",
			"info_hash", st.InfoHash, "reason", st.Reason, "grab_id", grab.ID)
		return nil
	}

	s.logger.Warn("stallwatcher: blocklisting stalled release",
		"info_hash", st.InfoHash,
		"release", grab.ReleaseTitle,
		"reason", reason,
		"inactive_secs", st.InactiveSecs,
		"grab_id", grab.ID,
		"source", grab.Source,
	)

	// Blocklist the release. Idempotent via ErrAlreadyBlocklisted — we
	// just want the entry to exist.
	err = s.blocklist.AddFromStall(ctx, blocklist.StallEntry{
		SeriesID:     grab.SeriesID,
		EpisodeID:    dbutil.NullStringValue(grab.EpisodeID),
		ReleaseGUID:  grab.ReleaseGuid,
		ReleaseTitle: grab.ReleaseTitle,
		IndexerID:    dbutil.NullStringValue(grab.IndexerID),
		Protocol:     grab.Protocol,
		Size:         grab.Size,
		Notes: fmt.Sprintf("auto-blocklisted by stall watcher after %d seconds (%s)",
			st.InactiveSecs, st.Reason),
		Reason:   reason,
		InfoHash: st.InfoHash,
	})
	if err != nil && !errors.Is(err, blocklist.ErrAlreadyBlocklisted) {
		return fmt.Errorf("blocklist add: %w", err)
	}

	// Mark the grab as stalled.
	if err := s.q.UpdateGrabStatus(ctx, db.UpdateGrabStatusParams{
		DownloadStatus:  "stalled",
		DownloadedBytes: 0,
		ID:              grab.ID,
	}); err != nil {
		return fmt.Errorf("updating grab status: %w", err)
	}

	// Publish a bus event so the WS layer can toast the UI.
	s.bus.Publish(ctx, events.Event{
		Type: events.TypeGrabStalled,
		Data: map[string]any{
			"grab_id":       grab.ID,
			"series_id":     grab.SeriesID,
			"release_title": grab.ReleaseTitle,
			"reason":        reason,
			"info_hash":     st.InfoHash,
			"source":        grab.Source,
		},
	})

	// If the grab came from auto-search, trigger a re-search under the
	// circuit breaker. Interactive grabs only toast; the user decides.
	if grab.Source == "auto_search" {
		recentStalls, err := s.blocklist.CountRecentStalls(ctx, grab.SeriesID, dbutil.NullStringValue(grab.EpisodeID))
		if err != nil {
			s.logger.Warn("stallwatcher: count recent stalls failed", "error", err)
			return nil
		}
		if recentStalls >= MaxStallRetriesPerEpisode {
			s.logger.Info("stallwatcher: circuit breaker tripped, skipping re-search",
				"series_id", grab.SeriesID, "episode_id", dbutil.NullStringValue(grab.EpisodeID), "stall_count", recentStalls)
			s.bus.Publish(ctx, events.Event{
				Type: events.TypeGrabStalledGaveUp,
				Data: map[string]any{
					"grab_id":     grab.ID,
					"series_id":   grab.SeriesID,
					"stall_count": recentStalls,
				},
			})
			return nil
		}
		// NB: the actual re-search is triggered by publishing a specific
		// event that the scheduler subscribes to. Plumbing that live is
		// part of Step 5 (scheduler wiring). For now, we publish the
		// event and trust the subscriber will exist.
		s.bus.Publish(ctx, events.Event{
			Type: events.TypeAutoSearchRetry,
			Data: map[string]any{
				"series_id":   grab.SeriesID,
				"episode_id":  dbutil.NullStringValue(grab.EpisodeID),
				"retry_count": recentStalls + 1,
			},
		})
	}

	return nil
}

// resolveHaulClient finds an enabled haul download client in the
// downloader registry and returns a concrete plugin client. Returns
// errNoHaulConfigured if the user hasn't set one up — Phase 0 only
// watches Haul, so other clients are silently ignored.
func (s *Service) resolveHaulClient(ctx context.Context) (*haul.Client, error) {
	clients, err := s.downloader.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range clients {
		if !c.Enabled {
			continue
		}
		if !strings.EqualFold(c.Kind, "haul") {
			continue
		}
		var cfg haul.Config
		if err := json.Unmarshal(c.Settings, &cfg); err != nil {
			s.logger.Warn("stallwatcher: cannot parse haul settings", "id", c.ID, "error", err)
			continue
		}
		if cfg.URL == "" {
			continue
		}
		return haul.New(cfg), nil
	}
	return nil, errNoHaulConfigured
}

var errNoHaulConfigured = errors.New("no haul download client configured")

// mapStallReason converts Haul's stall reason string into Pilot's blocklist
// reason constant. Unknown reasons fall back to a generic stall category.
func mapStallReason(haulReason string) string {
	switch haulReason {
	case "no_peers_ever":
		return blocklist.ReasonStallNoPeersEver
	case "no_peers", "no_seeders", "no_data_received":
		return blocklist.ReasonStallActivityLost
	default:
		return blocklist.ReasonStallActivityLost
	}
}
