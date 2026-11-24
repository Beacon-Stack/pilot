package v1

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/beacon-stack/pilot/internal/parser"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pilot/internal/core/blocklist"
	"github.com/beacon-stack/pilot/internal/core/decision"
	"github.com/beacon-stack/pilot/internal/core/indexer"
	"github.com/beacon-stack/pilot/internal/core/quality"
	"github.com/beacon-stack/pilot/internal/core/show"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

// buildEpisodeQueries builds the search queries for an episode, a season or
// a whole series. A season search issues two queries, "S01" and
// "Season 1", because many torznab indexers index a release under only
// one of the two forms. The caller dedupes results by GUID.
//
// Anime augmentation: when isAnime is true and absolute > 0, the
// per-episode form additionally emits two absolute-numbering queries
// (`<title> <abs>` and `<title> - <abs>`) because anime fansubs tag
// releases that way rather than S01E48. Without these, episode search
// for shows TMDB structures as one big season — like Jujutsu Kaisen —
// returns zero results because indexers don't tag releases as `S01E48`.
//
// Examples (non-anime):
//
//	["Breaking Bad S01E05"]                      — single episode
//	["Breaking Bad S01", "Breaking Bad Season 1"] — full season
//	["Breaking Bad"]                              — whole series
//
// Examples (anime, absolute=48):
//
//	["Jujutsu Kaisen S01E48", "Jujutsu Kaisen 48", "Jujutsu Kaisen - 48"]
func buildEpisodeQueries(title string, season, episode, absolute int, isAnime bool) []string {
	switch {
	case season > 0 && episode > 0:
		queries := []string{fmt.Sprintf("%s S%02dE%02d", title, season, episode)}
		if isAnime && absolute > 0 {
			queries = append(queries,
				fmt.Sprintf("%s %02d", title, absolute),
				fmt.Sprintf("%s - %02d", title, absolute),
			)
		}
		return queries
	case season > 0:
		return []string{
			fmt.Sprintf("%s S%02d", title, season),
			fmt.Sprintf("%s Season %d", title, season),
		}
	default:
		return []string{title}
	}
}

// applyQualityProfile tags releases whose parsed resolution falls BELOW the
// series' quality profile cutoff resolution. It deliberately gates on
// resolution only (not source/codec/hdr) — a strict by-Score match would
// reject every release whose exact source/codec combo isn't enumerated in
// the profile, which in practice is almost everything returned by public
// indexers and defeats the point of the gate.
//
// Tagging (rather than dropping) preserves the existing grayed-row + override
// UX: users can still see what was rejected and why, and can override a
// single release if the gate is wrong for their situation.
//
// profile may be nil; when nil, no tagging happens.
func applyQualityProfile(results []indexer.SearchResult, profile *quality.Profile) {
	if profile == nil {
		return
	}
	// One rule shared with RSS sync: see decision.MeetsProfileMinimum.
	for i := range results {
		q := results[i].Quality
		if q.Resolution == "" && q.Source == "" {
			q = plugin.ParseQualityFromTitle(results[i].Title)
		}
		if !decision.MeetsProfileMinimum(q, profile) {
			results[i].FilterReasons = append(results[i].FilterReasons, decision.ReasonBelowProfile)
		}
	}
}

// searchPlan turns a series search into the groups every indexer runs.
//
// A season search is ONE group: "Show S01" first, and "Show Season 1"
// only on an indexer whose "S01" results hold no pack for this season
// (Sonarr #3934: indexers tag packs under one naming convention or the
// other). Asking every indexer both, side by side, doubled the scrapes
// of every season search and queued half of them behind the global
// concurrency cap. Episode and anime-absolute variants stay separate
// groups, unioned as before: they find differently named releases of
// the same episode, not the same pack under two names.
//
// Every query carries the series' TVDB, IMDb and TMDB IDs; an indexer
// that lists them in its caps matches the show by ID.
func searchPlan(series show.Series, season, episode, absolute int, isAnime bool) []indexer.SearchGroup {
	queries := buildEpisodeQueries(series.Title, season, episode, absolute, isAnime)
	mk := func(text string) plugin.SearchQuery {
		return plugin.SearchQuery{
			Query:   text,
			TVDBID:  series.TVDBID,
			IMDBID:  series.IMDBID,
			TMDBID:  series.TMDBID,
			Season:  season,
			Episode: episode,
		}
	}
	if season > 0 && episode == 0 {
		group := indexer.SearchGroup{Enough: seasonPackFound(series.Title, series.AlternateTitles, season)}
		for _, q := range queries {
			group.Queries = append(group.Queries, mk(q))
		}
		return []indexer.SearchGroup{group}
	}
	groups := make([]indexer.SearchGroup, len(queries))
	for i, q := range queries {
		groups[i] = indexer.SearchGroup{Queries: []plugin.SearchQuery{mk(q)}}
	}
	return groups
}

// seasonPackFound reports whether results include a pack of this
// series' season — what a season search is for, and the only reason to
// try a second phrasing on the same indexer.
func seasonPackFound(title string, alternates []string, season int) func([]indexer.SearchResult) bool {
	return func(results []indexer.SearchResult) bool {
		for _, r := range filterByEpisode(results, title, alternates, season, 0, 0, nil) {
			if r.PackType == indexer.PackTypeSeason {
				return true
			}
		}
		return false
	}
}

// interactiveSearchTimeout is the hard cut-off for an interactive search.
// Indexers queue behind the global concurrency limit and Pulse's per-site
// rate limits, so a cold-cache season search can need over 20s; the
// budget is the 25s per-indexer timeout plus queue headroom. The stream
// endpoint shows results long before this.
const interactiveSearchTimeout = 30 * time.Second

// releaseSearch is one interactive search: what it looks for, and what
// turns raw indexer results into release rows. Built once, then applied
// to every partial result set the search produces.
type releaseSearch struct {
	series    show.Series
	season    int
	episode   int
	absolute  int
	tvdbToAbs func(int, int) int
	groups    []indexer.SearchGroup
	profile   *quality.Profile
	grabs     map[string]db.GrabHistory
	blocklist *blocklist.Service
	// blocked remembers blocklist answers by GUID, so the many partial
	// result sets of one stream don't ask the database again for
	// releases they already asked about. One goroutine uses it.
	blocked map[string]bool
}

// episodeContext returns the anime absolute-episode number and the
// TVDB→absolute converter for a search. Only fetched when the series is
// flagged anime AND a specific episode is searched (absolute is
// meaningless for season-pack or whole-series searches); skipping it
// for everything else saves a DB hit on every search.
func episodeContext(ctx context.Context, showSvc *show.Service, series show.Series, season, episode int) (int, func(int, int) int) {
	if series.SeriesType != "anime" || season <= 0 || episode <= 0 {
		return 0, nil
	}
	absolute, _ := showSvc.GetEpisodeAbsoluteNumber(ctx, series.ID, season, episode)
	tmdbID := series.TMDBID
	return absolute, func(s, e int) int {
		abs, _ := showSvc.TVDBSeasonToAbsolute(tmdbID, s, e)
		return abs
	}
}

func newReleaseSearch(ctx context.Context, input *episodeReleasesInput, indexerSvc *indexer.Service, showSvc *show.Service, blocklistSvc *blocklist.Service, qualitySvc *quality.Service) (*releaseSearch, error) {
	series, err := showSvc.Get(ctx, input.SeriesID)
	if err != nil {
		if errors.Is(err, show.ErrNotFound) {
			return nil, huma.Error404NotFound("series not found")
		}
		return nil, huma.NewError(http.StatusInternalServerError, "failed to get series", err)
	}
	rs := &releaseSearch{
		series:    series,
		season:    input.Season,
		episode:   input.Episode,
		blocklist: blocklistSvc,
		blocked:   make(map[string]bool),
	}
	rs.absolute, rs.tvdbToAbs = episodeContext(ctx, showSvc, series, input.Season, input.Episode)
	rs.groups = searchPlan(series, input.Season, input.Episode, rs.absolute, series.SeriesType == "anime")

	if qualitySvc != nil && series.QualityProfileID != "" {
		if profile, err := qualitySvc.Get(ctx, series.QualityProfileID); err == nil {
			rs.profile = &profile
		}
	}
	// guid → most-recent grab, to badge releases the user has already
	// grabbed. The guardrail covers the common "I forgot, did I already
	// grab this?" case; it does not gate the grab — the UI just shows a
	// confirmation prompt.
	if grabRows, gErr := indexerSvc.GrabHistory(ctx, input.SeriesID); gErr == nil {
		rs.grabs = indexLatestGrabsByGUID(grabRows)
	}
	return rs, nil
}

// checkBlocklist looks up, in one query, the GUIDs this search hasn't
// asked about yet, and remembers the answers for its later snapshots.
func (rs *releaseSearch) checkBlocklist(ctx context.Context, results []indexer.SearchResult) {
	if rs.blocklist == nil {
		return
	}
	var unknown []string
	for _, r := range results {
		if _, ok := rs.blocked[r.GUID]; !ok {
			unknown = append(unknown, r.GUID)
		}
	}
	if len(unknown) == 0 {
		return
	}
	hits, err := rs.blocklist.BlocklistedGUIDs(ctx, unknown)
	if err != nil {
		return // not remembered: asked again with the next snapshot
	}
	for _, g := range unknown {
		rs.blocked[g] = hits[g]
	}
}

// list turns a (partial or final) search snapshot into the response body.
func (rs *releaseSearch) list(ctx context.Context, snap indexer.Snapshot) releaseListBody {
	// Only the requested series and season/episode.
	results := filterByEpisode(snap.Results, rs.series.Title, rs.series.AlternateTitles, rs.season, rs.episode, rs.absolute, rs.tvdbToAbs)

	// Tag releases outside the series' quality profile so the UI greys
	// them with an "override and grab anyway" button (same UX as the
	// blocklist and min_seeders filters).
	applyQualityProfile(results, rs.profile)
	rs.checkBlocklist(ctx, results)

	// Count how many indexers each title appears on.
	titleIndexers := make(map[string]map[string]bool)
	for _, r := range results {
		if titleIndexers[r.Title] == nil {
			titleIndexers[r.Title] = make(map[string]bool)
		}
		titleIndexers[r.Title][r.IndexerName] = true
	}

	bodies := make([]*releaseBody, len(results))
	for i, r := range results {
		b := indexerResultToReleaseBody(r)
		// Mark multi-indexer results (found on 2+ indexers with high seeds).
		if len(titleIndexers[r.Title]) > 1 && r.Seeds >= 5 {
			b.MultiIndexer = true
		}
		// Mark low-confidence results (< 5 seeds — indexer count may be stale).
		if r.Seeds < 5 {
			b.LowConfidence = true
		}
		// Blocklisted releases are tagged, not dropped, so the UI can show
		// them greyed out with an override button.
		if rs.blocked[r.GUID] {
			b.FilterReasons = append(b.FilterReasons, "previously stalled (on blocklist)")
		}
		if prior, ok := rs.grabs[r.GUID]; ok {
			b.AlreadyGrabbedAt = prior.GrabbedAt
			b.AlreadyGrabbedID = prior.ID
			b.AlreadyGrabbedStatus = prior.DownloadStatus
		}
		bodies[i] = b
	}

	// Indexer failures don't fail the request or vanish: the per-indexer
	// breakdown ships in the envelope so the UI can render "3 of 10
	// indexers failed" instead of a bare "No releases found."
	return releaseListBody{
		Releases: bodies,
		Indexers: outcomesToBodies(snap.Outcomes),
		Done:     snap.Done,
	}
}

// streamReleases runs the search and writes one JSON line per update:
// everything found so far, then a last line with done=true. Updates
// that arrive while a line is being written are coalesced into the next.
func streamReleases(hctx huma.Context, indexerSvc *indexer.Service, rs *releaseSearch) {
	ctx := hctx.Context()
	hctx.SetHeader("Content-Type", "application/x-ndjson")
	hctx.SetHeader("Cache-Control", "no-store")
	hctx.SetHeader("X-Accel-Buffering", "no") // don't let a proxy hold the lines back
	w := hctx.BodyWriter()
	enc := json.NewEncoder(w)
	send := func(snap indexer.Snapshot) error {
		if err := enc.Encode(rs.list(ctx, snap)); err != nil {
			return err
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		return nil
	}

	searchCtx, cancel := context.WithTimeout(ctx, interactiveSearchTimeout)
	defer cancel()

	var (
		mu      sync.Mutex
		latest  indexer.Snapshot
		updates = make(chan struct{}, 1)
		done    = make(chan indexer.Snapshot, 1)
	)
	go func() {
		snap, _ := indexerSvc.SearchInteractive(searchCtx, rs.groups, func(s indexer.Snapshot) {
			mu.Lock()
			latest = s
			mu.Unlock()
			select {
			case updates <- struct{}{}:
			default: // one already waiting; it will carry this snapshot
			}
		})
		done <- snap
	}()

	for {
		select {
		case <-updates:
			mu.Lock()
			snap := latest
			mu.Unlock()
			if err := send(snap); err != nil {
				cancel() // the client has gone; stop asking indexers
				<-done
				return
			}
		case snap := <-done:
			_ = send(snap)
			return
		}
	}
}

// filterByEpisode removes results that don't match the requested series and
// season/episode. If season is 0, only the series title is checked
// (whole-series search). Season packs are kept when searching for a
// specific season.
//
// The title check matters because many torznab indexers match loosely on
// the query string and return unrelated shows that share common words.
// Each result's title is re-parsed and dropped unless its show title
// matches the series or one of its alternate titles.
//
// For anime, pass the absolute episode number and a TVDB→absolute
// converter. When absolute > 0:
//
//   - releases tagged with a matching absolute number (fansub
//     "Show - 48" form) pass regardless of season;
//   - releases tagged TVDB-style ("Show S03E01") pass when
//     tvdbToAbsolute(season, episode) gives the requested absolute.
//
// Pass absolute=0 and tvdbToAbsolute=nil for strict season/episode
// matching.
func filterByEpisode(results []indexer.SearchResult, seriesTitle string, alternateTitles []string, season, episode, absolute int, tvdbToAbsolute func(int, int) int) []indexer.SearchResult {
	// Build the candidate-title set once: canonical title + any TMDB
	// alternates. The series row's AlternateTitles list does NOT contain
	// the canonical title — prepend it explicitly so single-title series
	// behave identically to before.
	candidates := make([]string, 0, 1+len(alternateTitles))
	candidates = append(candidates, seriesTitle)
	candidates = append(candidates, alternateTitles...)

	filtered := make([]indexer.SearchResult, 0, len(results))
	for _, r := range results {
		parsed := parser.Parse(r.Title)

		// Series-title gate — strict equality (after normalization) against
		// any of the canonical/alternate titles. See parser.TitleMatches
		// and TitleMatchesAny for semantics.
		if !parser.TitleMatchesAny(candidates, parsed.Title) {
			continue
		}

		if season == 0 {
			filtered = append(filtered, r)
			continue
		}

		ep := parsed.EpisodeInfo

		// Anime absolute-episode acceptance: when the caller knows the
		// requested episode's absolute number AND the parsed release
		// has a matching absolute, accept it regardless of parsed
		// season. This is what makes "Jujutsu Kaisen - 48" findable
		// when the user asked for S01E48.
		if absolute > 0 && ep.AbsoluteEpisode == absolute {
			filtered = append(filtered, r)
			continue
		}

		// TVDB-tagged anime acceptance: a release tagged S03E01 for a
		// show TMDB serves as a single Season 1 corresponds to absolute
		// 48 (or whatever the Anime-Lists XML says). Run the parsed
		// (season, episode) through the converter; on a match against
		// the requested absolute, accept the release. Without this,
		// every TVDB-tagged anime release gets dropped as wrong-season
		// because TMDB and TVDB disagree about season layout.
		if absolute > 0 && tvdbToAbsolute != nil && ep.Season > 0 && len(ep.Episodes) == 1 {
			if tvdbToAbsolute(ep.Season, ep.Episodes[0]) == absolute {
				filtered = append(filtered, r)
				continue
			}
		}

		// Wrong season — skip.
		if ep.Season != season {
			continue
		}

		// Season pack — keep if we're searching for the season.
		if ep.IsSeasonPack {
			filtered = append(filtered, r)
			continue
		}

		// No specific episode requested — keep all from this season.
		if episode == 0 {
			filtered = append(filtered, r)
			continue
		}

		// Check if the requested episode is in the parsed episodes list.
		for _, e := range ep.Episodes {
			if e == episode {
				filtered = append(filtered, r)
				break
			}
		}
	}
	return filtered
}
