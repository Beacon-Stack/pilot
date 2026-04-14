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

// handleStall correlates a single stalled torrent with grab history and,
// if a match is found, blocklists the release and marks the grab stalled.
func (s *Service) handleStall(ctx context.Context, st haul.StalledTorrent, inGrace bool) error {
	// Find the grab_history row for this info_hash.
	grab, err := s.q.GetGrabByInfoHash(ctx, sql.NullString{String: st.InfoHash, Valid: true})
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
		EpisodeID:    grab.EpisodeID.String,
		ReleaseGUID:  grab.ReleaseGuid,
		ReleaseTitle: grab.ReleaseTitle,
		IndexerID:    grab.IndexerID.String,
		Protocol:     grab.Protocol,
		Size:         int64(grab.Size),
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
		recentStalls, err := s.blocklist.CountRecentStalls(ctx, grab.SeriesID, grab.EpisodeID.String)
		if err != nil {
			s.logger.Warn("stallwatcher: count recent stalls failed", "error", err)
			return nil
		}
		if recentStalls >= MaxStallRetriesPerEpisode {
			s.logger.Info("stallwatcher: circuit breaker tripped, skipping re-search",
				"series_id", grab.SeriesID, "episode_id", grab.EpisodeID.String, "stall_count", recentStalls)
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
				"episode_id":  grab.EpisodeID.String,
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
