package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"github.com/beacon-stack/pilot/internal/core/blocklist"
	"github.com/beacon-stack/pilot/internal/core/dbutil"
	"github.com/beacon-stack/pilot/internal/core/decision"
	"github.com/beacon-stack/pilot/internal/core/delay"
	"github.com/beacon-stack/pilot/internal/core/downloader"
	"github.com/beacon-stack/pilot/internal/core/indexer"
	"github.com/beacon-stack/pilot/internal/core/language"
	"github.com/beacon-stack/pilot/internal/core/quality"
	"github.com/beacon-stack/pilot/internal/core/releaseprofile"
	"github.com/beacon-stack/pilot/internal/core/wanted"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/scheduler"
	"github.com/beacon-stack/pilot/pkg/plugin"
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
// the best release each wanted episode's quality profile accepts: missing
// episodes, upgrades below the cutoff, and PROPER/REPACK fixes. Runs every
// 15 minutes.
func RSSSync(
	idxSvc *indexer.Service,
	showQ db.Querier,
	dlSvc *downloader.Service,
	blocklistSvc *blocklist.Service,
	profiles ProfileGetter,
	logger *slog.Logger,
) scheduler.Job {
	return scheduler.Job{
		Name:     "rss_sync",
		Interval: 15 * time.Minute,
		Fn: func(ctx context.Context) {
			logger.Info("task started", "task", "rss_sync")
			start := time.Now()

			grabbed, err := runRSSSync(ctx, idxSvc, showQ, dlSvc, blocklistSvc, profiles, logger)
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
	blocklistSvc *blocklist.Service,
	profiles ProfileGetter,
	logger *slog.Logger,
) (int, error) {
	// 1. Fetch recent releases from all enabled indexers.
	releases, fetchErr := idxSvc.GetRecent(ctx)
	if fetchErr != nil {
		// Non-fatal: partial results from other indexers may still be useful.
		logger.Warn("some indexers failed during RSS fetch", "error", fetchErr)
	}
	// Releases held back by the delay profile on earlier runs compete with
	// this run's releases (see mergePending).
	delayProfile, err := delay.NewStore(q).Get(ctx)
	if err != nil {
		logger.Warn("rss_sync: could not load delay profile; using no delay", "error", err)
		delayProfile = delay.Default()
	}
	now := time.Now().UTC()
	pendingRows, err := q.ListPendingReleases(ctx)
	if err != nil {
		logger.Warn("rss_sync: could not list pending releases", "error", err)
	}
	releases, pendingEligible := mergePending(releases, pendingRows, logger)
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

	// 5. Decide what to grab: per episode, the best release that the
	//    quality profile accepts (including upgrades and PROPERs).
	picks := selectRSSGrabs(ctx, releases, rssEnv{
		seriesByTitle: seriesByTitle,
		episodes:      q.ListEpisodesBySeriesID,
		currentQuality: func(ctx context.Context, episodeID string) (*plugin.Quality, error) {
			return currentFileQuality(ctx, q, episodeID)
		},
		profile:           profileLookup(profiles),
		active:            activeEpisodes,
		recentlyCompleted: recentlyCompletedEpisodes,
		delay:             delayProfile,
		blocked: func(ctx context.Context, guid string) bool {
			// Blocklist guard. Interactive/manual search always filtered on
			// the blocklist; RSS never did — so a blocklisted fake (stalled
			// or unimportable) could still be auto-grabbed forever. Matched
			// by GUID (RSS releases don't carry an info hash pre-grab; the
			// blocklist query's two-keyed match still catches hash-keyed
			// entries whose GUID matches).
			if blocklistSvc == nil {
				return false
			}
			blocked, err := blocklistSvc.IsBlocklistedGUIDOrInfoHash(ctx, guid, "")
			return err == nil && blocked
		},
		logger: logger,
	})

	// 6. Hold picks still inside their delay as pending; grab the rest.
	toGrab, toHold := planRSSGrabs(picks, delayProfile, now, pendingEligible)
	pickedEpisodes := make(map[string]bool, len(picks))
	for _, pick := range picks {
		pickedEpisodes[pick.Episode.ID] = true
	}
	for _, pick := range toHold {
		holdPending(ctx, q, pick, eligibleAt(pick, delayProfile, now, pendingEligible), logger)
	}
	// A pending release whose episode got no pick this run is no longer
	// wanted (file imported, unmonitored, blocklisted, ...).
	for _, row := range pendingRows {
		if !pickedEpisodes[row.EpisodeID] {
			_ = q.DeletePendingRelease(ctx, row.ID)
		}
	}

	var grabbed int
	for _, pick := range toGrab {
		rel, matched, ep := pick.Release, pick.Series, pick.Episode
		sn, epNum := int(ep.SeasonNumber), int(ep.EpisodeNumber)
		// Whatever was pending for this episode is replaced by this grab.
		_ = q.DeletePendingReleaseByEpisode(ctx, ep.ID)

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
			"quality", rel.Quality.Name,
			"upgrade", pick.Upgrade,
		)
		grabbed++
	}

	return grabbed, nil
}

// ProfileGetter loads a quality profile by ID. quality.Service satisfies it.
type ProfileGetter interface {
	Get(ctx context.Context, id string) (quality.Profile, error)
}

func profileLookup(p ProfileGetter) func(context.Context, string) (*quality.Profile, error) {
	return func(ctx context.Context, id string) (*quality.Profile, error) {
		if p == nil || id == "" {
			return nil, nil
		}
		prof, err := p.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		return &prof, nil
	}
}

// rssEnv is everything selectRSSGrabs needs, as plain values and lookup
// functions so the decision logic can be tested without a database.
type rssEnv struct {
	seriesByTitle     map[string]db.Series
	episodes          func(ctx context.Context, seriesID string) ([]db.Episode, error)
	currentQuality    func(ctx context.Context, episodeID string) (*plugin.Quality, error)
	profile           func(ctx context.Context, profileID string) (*quality.Profile, error)
	active            map[string]bool
	recentlyCompleted map[string]bool
	blocked           func(ctx context.Context, guid string) bool
	delay             delay.Profile
	logger            *slog.Logger
}

// rssPick is one release chosen for one episode.
type rssPick struct {
	Release indexer.SearchResult
	Series  db.Series
	Episode db.Episode
	Upgrade bool             // the episode already had a file
	Profile *quality.Profile // the series' quality profile (nil if none)
}

// rssRanksAbove ranks two releases for the same episode: the search
// ranking, except that between releases of equal quality and revision on
// different protocols, the delay profile's preferred protocol wins.
func rssRanksAbove(a, b indexer.SearchResult, d delay.Profile) bool {
	if a.QualityScore == b.QualityScore &&
		a.Quality.EffectiveRevision() == b.Quality.EffectiveRevision() &&
		a.Protocol != b.Protocol {
		return d.Preferred(a.Protocol)
	}
	return indexer.RanksAbove(a, b)
}

// mergePending adds stored pending releases to this run's candidates so
// they compete with new releases, and returns when each may be grabbed
// (by GUID). Their stored eligible_at is used, not a recomputed one: the
// stored release's age is from when it was first seen.
func mergePending(releases []indexer.SearchResult, rows []db.PendingRelease, logger *slog.Logger) ([]indexer.SearchResult, map[string]time.Time) {
	eligible := make(map[string]time.Time, len(rows))
	seen := make(map[string]bool, len(releases))
	for _, r := range releases {
		seen[r.GUID] = true
	}
	for _, row := range rows {
		var rel indexer.SearchResult
		if err := json.Unmarshal([]byte(row.ReleaseJson), &rel); err != nil {
			logger.Warn("rss_sync: unreadable pending release", "id", row.ID, "error", err)
			continue
		}
		at, err := time.Parse(time.RFC3339, row.EligibleAt)
		if err != nil {
			continue
		}
		eligible[rel.GUID] = at
		if !seen[rel.GUID] {
			releases = append(releases, rel)
			seen[rel.GUID] = true
		}
	}
	return releases, eligible
}

func eligibleAt(p rssPick, d delay.Profile, now time.Time, pending map[string]time.Time) time.Time {
	if at, ok := pending[p.Release.GUID]; ok {
		return at
	}
	return d.EligibleAt(p.Release.Protocol, p.Release.AgeDays, now)
}

// planRSSGrabs splits picks into those to grab now and those to hold as
// pending: a pick is grabbed when its delay has passed or it bypasses the
// delay (the best quality its profile allows).
func planRSSGrabs(picks []rssPick, d delay.Profile, now time.Time, pending map[string]time.Time) (grab, hold []rssPick) {
	for _, p := range picks {
		if d.Bypasses(p.Release.Quality, p.Profile) || !eligibleAt(p, d, now, pending).After(now) {
			grab = append(grab, p)
		} else {
			hold = append(hold, p)
		}
	}
	return grab, hold
}

// holdPending stores pick as the episode's pending release, replacing any
// earlier one (it won the ranking against it in selectRSSGrabs).
func holdPending(ctx context.Context, q db.Querier, p rssPick, at time.Time, logger *slog.Logger) {
	b, err := json.Marshal(p.Release)
	if err != nil {
		return
	}
	if err := q.UpsertPendingRelease(ctx, db.UpsertPendingReleaseParams{
		ID:           uuid.New().String(),
		SeriesID:     p.Series.ID,
		EpisodeID:    p.Episode.ID,
		ReleaseTitle: p.Release.Title,
		Protocol:     string(p.Release.Protocol),
		QualityName:  p.Release.Quality.Name,
		ReleaseJson:  string(b),
		EligibleAt:   at.UTC().Format(time.RFC3339),
		AddedAt:      time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		logger.Warn("rss_sync: could not store pending release", "release", p.Release.Title, "error", err)
		return
	}
	logger.Info("rss_sync: holding release for the delay profile",
		"release", p.Release.Title, "episode_id", p.Episode.ID, "eligible_at", at.UTC().Format(time.RFC3339))
}

// selectRSSGrabs decides which RSS releases to grab. For each monitored
// episode it keeps only releases the quality profile accepts (see
// decision.Evaluate: missing episodes, upgrades, PROPERs) and picks the
// best of them with the same ranking search uses. Previously RSS took the
// first matching release in feed order and never checked the profile, so
// a 480p release could be grabbed for a 1080p profile, and episodes with
// a file were never upgraded.
func selectRSSGrabs(ctx context.Context, releases []indexer.SearchResult, env rssEnv) []rssPick {
	best := make(map[string]rssPick)
	var order []string
	episodesBySeries := make(map[string][]db.Episode)
	profiles := make(map[string]*quality.Profile)
	profileFailed := make(map[string]bool)
	current := make(map[string]*plugin.Quality)
	currentLoaded := make(map[string]bool)

	for _, rel := range releases {
		sn, epNum, ok := extractEpisodeNumbers(rel.Title)
		if !ok {
			continue // no S##E## pattern found — skip
		}
		matched, ok := matchSeriesTitle(normalizeRSSTitle(rel.Title), env.seriesByTitle)
		if !ok {
			continue
		}

		eps, loaded := episodesBySeries[matched.ID]
		if !loaded {
			var err error
			eps, err = env.episodes(ctx, matched.ID)
			if err != nil {
				env.logger.Warn("rss_sync: could not list episodes", "series_id", matched.ID, "error", err)
			}
			episodesBySeries[matched.ID] = eps
		}
		ep, found := findEpisode(eps, sn, epNum)
		if !found || !ep.Monitored {
			continue
		}
		// Per-episode guards. See the cohort comment in runRSSSync.
		if env.active[ep.ID] || env.recentlyCompleted[ep.ID] {
			continue
		}
		if env.blocked(ctx, rel.GUID) {
			continue
		}
		if !env.delay.Allows(rel.Protocol) {
			continue // protocol turned off in the delay profile
		}
		if hasFilterReason(rel.FilterReasons, releaseprofile.FilterReason) {
			continue // rejected by a release profile (tagged in GetRecent)
		}
		if hasFilterReason(rel.FilterReasons, language.FilterReason) {
			continue // in no wanted language (tagged in GetRecent)
		}

		var cur *plugin.Quality
		if ep.HasFile {
			if !currentLoaded[ep.ID] {
				cq, err := env.currentQuality(ctx, ep.ID)
				if err != nil {
					env.logger.Warn("rss_sync: could not read current file quality", "episode_id", ep.ID, "error", err)
				}
				current[ep.ID], currentLoaded[ep.ID] = cq, true
			}
			cur = current[ep.ID]
			if cur == nil {
				// has_file is set but no file record is readable: don't
				// guess. The file reconciler will sort the record out.
				continue
			}
		}

		if profileFailed[matched.QualityProfileID] {
			continue
		}
		prof, loaded := profiles[matched.QualityProfileID]
		if !loaded {
			var err error
			prof, err = env.profile(ctx, matched.QualityProfileID)
			if err != nil {
				env.logger.Warn("rss_sync: could not load quality profile; skipping series",
					"series_id", matched.ID, "profile_id", matched.QualityProfileID, "error", err)
				profileFailed[matched.QualityProfileID] = true
				continue
			}
			profiles[matched.QualityProfileID] = prof
		}

		if ok, why := decision.Evaluate(rel.Quality, cur, prof); !ok {
			env.logger.Debug("rss_sync: release rejected",
				"release", rel.Title, "episode_id", ep.ID, "reason", why)
			continue
		}

		pick := rssPick{Release: rel, Series: matched, Episode: ep, Upgrade: cur != nil, Profile: prof}
		prev, seen := best[ep.ID]
		if !seen {
			order = append(order, ep.ID)
		} else if !rssRanksAbove(rel, prev.Release, env.delay) {
			continue
		}
		best[ep.ID] = pick
	}

	out := make([]rssPick, 0, len(order))
	for _, id := range order {
		out = append(out, best[id])
	}
	return out
}

func hasFilterReason(reasons []string, want string) bool {
	for _, r := range reasons {
		if r == want {
			return true
		}
	}
	return false
}

// currentFileQuality returns the best quality among the episode's
// recorded files, or nil when it has none.
func currentFileQuality(ctx context.Context, q db.Querier, episodeID string) (*plugin.Quality, error) {
	files, err := q.ListEpisodeFilesByEpisodeID(ctx, episodeID)
	if err != nil {
		return nil, err
	}
	return wanted.BestFileQuality(files, episodeID), nil
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
	// Limit MUST be set: it binds to a SQL `LIMIT ?`, and the zero value
	// means `LIMIT 0` — zero rows, silently. That exact omission made this
	// entire guard a no-op in production: the map was always empty and RSS
	// re-grabbed the same fake release every 15 minutes, forever.
	rows, err := q.ListGrabHistoryByStatusSince(ctx, db.ListGrabHistoryByStatusSinceParams{
		Status: "completed",
		Since:  since.Format(time.RFC3339),
		Limit:  500,
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
