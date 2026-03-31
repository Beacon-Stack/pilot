package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"

	"github.com/screenarr/screenarr/internal/core/downloader"
	"github.com/screenarr/screenarr/internal/core/indexer"
	dbsqlite "github.com/screenarr/screenarr/internal/db/generated/sqlite"
	"github.com/screenarr/screenarr/internal/scheduler"
)

// RSSSync returns a Job that polls all enabled indexers for recent releases,
// matches them against monitored series/episodes, and automatically grabs
// wanted episodes. Runs every 15 minutes.
func RSSSync(
	idxSvc *indexer.Service,
	showQ dbsqlite.Querier,
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
	q dbsqlite.Querier,
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
	seriesByTitle := make(map[string]dbsqlite.Series, len(series))
	for _, s := range series {
		seriesByTitle[normalizeRSSTitle(s.Title)] = s
	}

	// 4. Build a set of series IDs that already have an active grab so we
	//    don't queue duplicate downloads.
	activeGrabs, err := q.ListActiveGrabs(ctx)
	if err != nil {
		return 0, fmt.Errorf("listing active grabs: %w", err)
	}
	activeSeries := make(map[string]bool, len(activeGrabs))
	for _, g := range activeGrabs {
		activeSeries[g.SeriesID] = true
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

		if activeSeries[matched.ID] {
			continue // already downloading something for this series
		}

		// Load all episodes for the series and find the specific one.
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
		if ep.Monitored == 0 || ep.HasFile != 0 {
			continue
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

		// Record the grab in history.
		grab, grabErr := idxSvc.CreateGrab(ctx, indexer.GrabRequest{
			SeriesID:     matched.ID,
			EpisodeID:    ep.ID,
			SeasonNumber: sn,
			Release:      rel.Release,
			IndexerID:    rel.IndexerID,
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
			var dcIDPtr, itemIDPtr *string
			if dcID != "" {
				dcIDPtr = &dcID
			}
			if itemID != "" {
				itemIDPtr = &itemID
			}
			if err := idxSvc.UpdateGrabDownloadClient(ctx, dbsqlite.UpdateGrabDownloadClientParams{
				DownloadClientID: dcIDPtr,
				ClientItemID:     itemIDPtr,
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
		activeSeries[matched.ID] = true
	}

	return grabbed, nil
}

// matchSeriesTitle finds the series whose normalised title appears as a
// word-aligned prefix of the normalised release title.
func matchSeriesTitle(normRelease string, seriesByTitle map[string]dbsqlite.Series) (dbsqlite.Series, bool) {
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
	return dbsqlite.Series{}, false
}

// findEpisode returns the first episode matching the given season and episode number.
func findEpisode(episodes []dbsqlite.Episode, season, episode int) (dbsqlite.Episode, bool) {
	for _, ep := range episodes {
		if int(ep.SeasonNumber) == season && int(ep.EpisodeNumber) == episode {
			return ep, true
		}
	}
	return dbsqlite.Episode{}, false
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
