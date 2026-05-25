package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"

	"github.com/beacon-stack/pilot/internal/core/dbutil"
	"github.com/beacon-stack/pilot/internal/core/downloader"
	"github.com/beacon-stack/pilot/internal/core/indexer"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/scheduler"
)

// completedSkipWindow is how long a `completed` grab for an episode
// suppresses fresh RSS grabs. The original Maul S01E07 failure mode:
// a grab completed but the importer didn't flip has_file=true, and
// every 15-minute RSS tick re-grabbed the same release for ~3 hours,
// leaving 11 duplicate grab_history rows. 6 hours is generous enough
// for any real importer to finish or surface a failure, while short
// enough that a genuinely-failed release can be retried via RSS the
// same day.
const completedSkipWindow = 6 * time.Hour

// RSSSync returns a Job that polls all enabled indexers for recent releases,
// matches them against monitored series/episodes, and automatically grabs
// wanted episodes. Runs every 15 minutes.
func RSSSync(
	idxSvc *indexer.Service,
	showQ db.Querier,
	dlSvc *downloader.Service,
	logger *slog.Logger,
) scheduler.Job {
	return scheduler.Job{
		Name:     "rss_sync",
		Interval: 15 * time.Minute,
		Fn: func(ctx context.Context) {
			logger.Info("task started", "task", "rss_sync")
			start := time.Now()

			grabbed, err := runRSSSync(ctx, idxSvc, showQ, dlSvc, logger)
			if err != nil {
				logger.Warn("task failed",
					"task", "rss_sync",
					"error", err,
					"duration_ms", time.Since(start).Milliseconds(),
				)
				return
			}

			logger.Info("task finished",
				"task", "rss_sync",
				"grabbed", grabbed,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		},
	}
}

func runRSSSync(
	ctx context.Context,
	idxSvc *indexer.Service,
	q db.Querier,
	dlSvc *downloader.Service,
	logger *slog.Logger,
) (int, error) {
	// 1. Fetch recent releases from all enabled indexers.
	releases, fetchErr := idxSvc.GetRecent(ctx)
	if fetchErr != nil {
		// Non-fatal: partial results from other indexers may still be useful.
		logger.Warn("some indexers failed during RSS fetch", "error", fetchErr)
	}
	if len(releases) == 0 {
		return 0, nil
	}

	// 2. List all monitored series.
	series, err := q.ListMonitoredSeries(ctx)
	if err != nil {
		return 0, fmt.Errorf("listing monitored series: %w", err)
	}
	if len(series) == 0 {
		return 0, nil
	}

	// 3. Build a map of series ID → row for quick lookup after title matching.
	seriesByTitle := make(map[string]db.Series, len(series))
	for _, s := range series {
		seriesByTitle[normalizeRSSTitle(s.Title)] = s
	}

	// 4. Build per-episode dedup sets so we don't queue duplicate downloads.
	//
	// Two cohorts skip a release:
	//
	//  a. activeEpisodes — an in-flight grab already exists for this
	//     specific episode. Was previously keyed by series_id, which
	//     blocked legitimate parallel grabs of different episodes in
	//     the same series. More importantly, the inverse hole let RSS
	//     re-grab the SAME episode after a grab terminated as
	//     "completed" but the importer didn't flip has_file=true (the
	//     11-dupe Maul S01E07 failure mode — see plans/lifecycle-trust.md).
	//
	//  b. recentlyCompletedEpisodes — the same episode had a `completed`
	//     grab within the last completedSkipWindow. Even if has_file is
	//     false, we don't fire a fresh grab — the importer can be
	//     stuck or the file may be inflight in haul. Without this guard,
	//     RSS hammers a release every 15 minutes generating duplicate
	//     grab_history rows. The window is intentionally short relative
	//     to typical RSS refresh — the assumption is that 6h is plenty
	//     for a real importer to finish or surface a failure.
	activeGrabs, err := q.ListActiveGrabs(ctx)
	if err != nil {
		return 0, fmt.Errorf("listing active grabs: %w", err)
	}
	activeEpisodes := make(map[string]bool, len(activeGrabs))
	for _, g := range activeGrabs {
		if g.EpisodeID != nil && *g.EpisodeID != "" {
			activeEpisodes[*g.EpisodeID] = true
		}
	}
	recentlyCompletedEpisodes, err := buildRecentlyCompletedEpisodes(ctx, q, time.Now().UTC().Add(-completedSkipWindow))
	if err != nil {
		// Non-fatal: log and continue with active-only dedup. Worst case
		// we miss the second-line guardrail and rely on the unique index
		// (QW4) to backstop.
		logger.Warn("rss_sync: could not build recently-completed set", "error", err)
		recentlyCompletedEpisodes = map[string]bool{}
	}

	// 5. Process each release.
	var grabbed int
	for _, rel := range releases {
		// Parse the release title to extract show name and season/episode.
		norm := normalizeRSSTitle(rel.Title)
		sn, epNum, ok := extractEpisodeNumbers(rel.Title)
		if !ok {
			continue // no S##E## pattern found — skip
		}

		// Try to match the normalised show name prefix against a monitored series.
		matched, ok := matchSeriesTitle(norm, seriesByTitle)
		if !ok {
			continue
		}

		// Load all episodes for the series and find the specific one.
		// (Per-episode dedup needs the episode ID — series-level skip is gone.)
		episodes, err := q.ListEpisodesBySeriesID(ctx, matched.ID)
		if err != nil {
			logger.Warn("rss_sync: could not list episodes",
				"series_id", matched.ID,
				"error", err,
			)
			continue
		}

		ep, found := findEpisode(episodes, sn, epNum)
		if !found {
			continue
		}

		// Only grab if episode is monitored and has no file.
		if !ep.Monitored || ep.HasFile {
			continue
		}

		// Per-episode guards (the dedup change). See cohort comment above.
		if activeEpisodes[ep.ID] {
			continue // already in flight for this exact episode
		}
		if recentlyCompletedEpisodes[ep.ID] {
			continue // recently completed; importer is the right place to recover, not RSS
		}

		// Submit to a download client.
		dcID, itemID, err := dlSvc.Add(ctx, rel.Release, nil)
		if err != nil {
			if errors.Is(err, downloader.ErrNoCompatibleClient) {
				logger.Warn("rss_sync: no download client configured for protocol",
					"series_id", matched.ID,
					"protocol", rel.Protocol,
				)
			} else {
				logger.Warn("rss_sync: could not submit release to download client",
					"series_id", matched.ID,
					"release", rel.Title,
					"error", err,
				)
			}
			continue
		}

		// Record the grab in history. Source is "auto_search" so stall
		// detection is allowed to auto-re-search on failure.
		grab, grabErr := idxSvc.CreateGrab(ctx, indexer.GrabRequest{
			SeriesID:     matched.ID,
			EpisodeID:    ep.ID,
			SeasonNumber: sn,
			Release:      rel.Release,
			IndexerID:    rel.IndexerID,
			Source:       "auto_search",
		})
		if grabErr != nil {
			logger.Warn("rss_sync: could not record grab history",
				"series_id", matched.ID,
				"release", rel.Title,
				"error", grabErr,
			)
			// Don't skip the count — we submitted it successfully.
		}

		// Update grab with the download client assignment if we have a grab row.
		if grabErr == nil && (dcID != "" || itemID != "") {
			if err := idxSvc.UpdateGrabDownloadClient(ctx, db.UpdateGrabDownloadClientParams{
				DownloadClientID: dbutil.NullableString(dcID),
				ClientItemID:     dbutil.NullableString(itemID),
				ID:               grab.ID,
			}); err != nil {
				logger.Warn("rss_sync: could not update grab download client",
					"grab_id", grab.ID,
					"error", err,
				)
			}
		}

		logger.Info("rss_sync: auto-grabbed episode",
			"series_id", matched.ID,
			"series_title", matched.Title,
			"season", sn,
			"episode", epNum,
			"release", rel.Title,
		)
		grabbed++
		// Mark this episode as in-flight so a later release in the same
		// RSS batch doesn't double-grab.
		activeEpisodes[ep.ID] = true
	}

	return grabbed, nil
}

// matchSeriesTitle finds the series whose normalised title appears as a
// word-aligned prefix of the normalised release title.
func matchSeriesTitle(normRelease string, seriesByTitle map[string]db.Series) (db.Series, bool) {
	for normTitle, s := range seriesByTitle {
		if normTitle == "" {
			continue
		}
		if strings.HasPrefix(normRelease, normTitle) {
			rest := normRelease[len(normTitle):]
			// The remaining text must start with a space (or be empty) so
			// "breaking bad" doesn't match "breaking badly".
			if rest == "" || rest[0] == ' ' {
				return s, true
			}
		}
	}
	return db.Series{}, false
}

// findEpisode returns the first episode matching the given season and episode number.
func findEpisode(episodes []db.Episode, season, episode int) (db.Episode, bool) {
	for _, ep := range episodes {
		if int(ep.SeasonNumber) == season && int(ep.EpisodeNumber) == episode {
			return ep, true
		}
	}
	return db.Episode{}, false
}

// extractEpisodeNumbers parses the first S##E## pattern from a release title.
// Returns season number, episode number, and whether the pattern was found.
func extractEpisodeNumbers(title string) (season, episode int, ok bool) {
	// Walk the string looking for 'S' followed by digits, 'E', digits.
	s := strings.ToUpper(title)
	for i := 0; i < len(s)-4; i++ {
		if s[i] != 'S' {
			continue
		}
		// Read season digits.
		j := i + 1
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		if j == i+1 || j >= len(s) || s[j] != 'E' {
			continue
		}
		sn := 0
		for _, c := range s[i+1 : j] {
			sn = sn*10 + int(c-'0')
		}
		// Read episode digits.
		k := j + 1
		for k < len(s) && s[k] >= '0' && s[k] <= '9' {
			k++
		}
		if k == j+1 {
			continue
		}
		en := 0
		for _, c := range s[j+1 : k] {
			en = en*10 + int(c-'0')
		}
		return sn, en, true
	}
	return 0, 0, false
}

// buildRecentlyCompletedEpisodes queries grab_history for `completed`
// rows newer than the cutoff and returns a set of their episode IDs.
// Used by RSS sync to skip re-grabbing episodes whose previous attempt
// is recently completed (regardless of has_file) — preventing the
// "importer hasn't run / file in flight" race that produced 11
// duplicate grab_history rows for one release in production.
func buildRecentlyCompletedEpisodes(ctx context.Context, q db.Querier, since time.Time) (map[string]bool, error) {
	rows, err := q.ListGrabHistoryByStatusSince(ctx, db.ListGrabHistoryByStatusSinceParams{
		Status: "completed",
		Since:  since.Format(time.RFC3339),
	})
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(rows))
	for _, g := range rows {
		if g.EpisodeID != nil && *g.EpisodeID != "" {
			out[*g.EpisodeID] = true
		}
	}
	return out, nil
}

// normalizeRSSTitle lowercases a string, converts common separators to spaces,
// strips other non-alphanumeric characters, and collapses whitespace. The
// result is safe to use as a map key or for prefix comparisons.
func normalizeRSSTitle(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '.' || r == '_' || r == '-':
			b.WriteRune(' ')
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ':
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
