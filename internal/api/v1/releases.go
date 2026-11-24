package v1

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pilot/internal/core/blocklist"
	"github.com/beacon-stack/pilot/internal/core/dbutil"
	"github.com/beacon-stack/pilot/internal/core/decision"
	"github.com/beacon-stack/pilot/internal/core/downloader"
	"github.com/beacon-stack/pilot/internal/core/indexer"
	"github.com/beacon-stack/pilot/internal/core/parser"
	"github.com/beacon-stack/pilot/internal/core/quality"
	"github.com/beacon-stack/pilot/internal/core/show"
	"github.com/beacon-stack/pilot/internal/core/wanted"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

// ── Request / response shapes ────────────────────────────────────────────────

// releaseBody is the API representation of a single indexer search result.
type releaseBody struct {
	GUID         string         `json:"guid"`
	Title        string         `json:"title"`
	Indexer      string         `json:"indexer"`
	IndexerID    string         `json:"indexer_id"`
	Protocol     string         `json:"protocol"`
	DownloadURL  string         `json:"download_url"`
	InfoURL      string         `json:"info_url,omitempty"`
	Size         int64          `json:"size"`
	Seeds        int            `json:"seeds,omitempty"`
	Peers        int            `json:"peers,omitempty"`
	AgeDays      float64        `json:"age_days,omitempty"`
	Quality      plugin.Quality `json:"quality"`
	QualityScore int            `json:"quality_score"`
	// PackType classifies the release as "season", "multi_episode", "episode",
	// or "" (unknown). Used by the interactive search UI to group, filter,
	// and badge releases. Mirrors indexer.SearchResult.PackType.
	PackType string `json:"pack_type,omitempty"`
	// EpisodeCount is how many episodes the release covers. For season packs
	// this is 0 because the exact count isn't known from the title alone —
	// the frontend should display a "Season Pack" badge instead of a count.
	EpisodeCount  int  `json:"episode_count,omitempty"`
	MultiIndexer  bool `json:"multi_indexer,omitempty"`
	LowConfidence bool `json:"low_confidence,omitempty"`
	// FilterReasons, when non-empty, means the release failed one of the
	// safety filters (min_seeders, stall blocklist, ...). The UI renders
	// these as grayed rows with the reasons shown and an "override and
	// grab anyway" button. The release can still be grabbed via the
	// normal grab endpoint if the user overrides.
	FilterReasons []string `json:"filter_reasons,omitempty"`
	// Languages are the audio languages tagged in the title. Empty when
	// the title has none (treated as English by the language preference).
	Languages []string `json:"languages,omitempty"`
	// AlreadyGrabbedAt, when set, means a grab_history row exists for
	// this release's GUID. The UI badges the row "already grabbed" and
	// asks for confirmation before grabbing again. The other AlreadyGrabbed*
	// fields surface the existing grab's id and status so the frontend
	// can deep-link to it. This is a guardrail, not a hard filter — the
	// user can still override and re-grab.
	AlreadyGrabbedAt     string `json:"already_grabbed_at,omitempty"`
	AlreadyGrabbedID     string `json:"already_grabbed_id,omitempty"`
	AlreadyGrabbedStatus string `json:"already_grabbed_status,omitempty"`
}

// indexerOutcomeBody is the per-indexer search breakdown: how many
// results each enabled indexer contributed, or why it failed.
type indexerOutcomeBody struct {
	IndexerID   string `json:"indexer_id"`
	IndexerName string `json:"indexer_name"`
	Results     int    `json:"results"`
	Error       string `json:"error,omitempty"`
	// Pending: the indexer's scrape is still running server-side and will
	// be cached — re-searching shortly returns its results. Rendered as
	// "still searching", never as a failure.
	Pending bool `json:"pending,omitempty"`
	// Searching: no answer yet. Only in the partial results the stream
	// endpoint sends while a search is running.
	Searching bool `json:"searching,omitempty"`
}

type releaseListBody struct {
	Releases []*releaseBody       `json:"releases"`
	Indexers []indexerOutcomeBody `json:"indexers,omitempty"`
	// Done is false on the partial results the stream endpoint sends
	// while indexers are still answering, and true on the last.
	Done bool `json:"done"`
}

type releaseListOutput struct {
	Body releaseListBody
}

func outcomesToBodies(outcomes []indexer.Outcome) []indexerOutcomeBody {
	out := make([]indexerOutcomeBody, len(outcomes))
	for i, oc := range outcomes {
		out[i] = indexerOutcomeBody{
			IndexerID:   oc.IndexerID,
			IndexerName: oc.IndexerName,
			Results:     oc.Results,
			Error:       oc.Error,
			Pending:     oc.Pending,
			Searching:   oc.Searching,
		}
	}
	return out
}

// episodeReleasesInput describes the path and optional query parameters for
// GET /api/v1/series/{id}/releases.
type episodeReleasesInput struct {
	SeriesID string `path:"id"               doc:"Series UUID"`
	Season   int    `query:"season"          doc:"Season number (required when episode is set)"`
	Episode  int    `query:"episode"         doc:"Episode number; omit to search for all season releases"`
}

// grabHistoryBody is a summary of one recorded grab.
type grabHistoryBody struct {
	ID             string    `json:"id"`
	SeriesID       string    `json:"series_id"`
	EpisodeID      *string   `json:"episode_id,omitempty"`
	SeasonNumber   *int64    `json:"season_number,omitempty"`
	IndexerID      *string   `json:"indexer_id,omitempty"`
	ReleaseGUID    string    `json:"release_guid"`
	ReleaseTitle   string    `json:"release_title"`
	Protocol       string    `json:"protocol"`
	Size           int64     `json:"size"`
	DownloadStatus string    `json:"download_status"`
	GrabbedAt      time.Time `json:"grabbed_at"`
}

type grabHistoryListOutput struct {
	Body []*grabHistoryBody
}

type seriesGrabHistoryInput struct {
	SeriesID string `path:"id" doc:"Series UUID"`
}

// grabInput carries the release the client wants to grab.
type grabInput struct {
	SeriesID string `path:"id"`
	Body     grabReleaseBody
}

type grabReleaseBody struct {
	GUID         string         `json:"guid"`
	Title        string         `json:"title"`
	IndexerID    string         `json:"indexer_id,omitempty"`
	Protocol     string         `json:"protocol"`
	DownloadURL  string         `json:"download_url"`
	Size         int64          `json:"size"`
	EpisodeID    string         `json:"episode_id,omitempty"`
	SeasonNumber int            `json:"season_number,omitempty"`
	Quality      plugin.Quality `json:"quality,omitempty"`
	// Override, when true, removes the release from the blocklist
	// before grabbing. Used by the grayed-row "override and grab anyway"
	// button in the interactive search UI.
	Override bool `json:"override,omitempty"`
}

type grabOutput struct {
	Body *grabHistoryBody
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// isGrabbedTerminalFailure returns true for download statuses where the
// grab effectively didn't deliver a usable file — failed, stalled
// (timed out before a peer ever appeared), or removed (user purged).
// The search-time "already grabbed" guardrail skips these so a user
// who got a dud release still sees a clean Grab button on retry,
// not a misleading "Already grabbed" pill that suggests the work is
// done.
func isGrabbedTerminalFailure(status string) bool {
	switch status {
	case "failed", "stalled", "removed":
		return true
	}
	return false
}

// indexLatestGrabsByGUID returns a map of release GUID → most-recent
// grab row for that GUID. The search handler uses this to badge
// releases the user has already grabbed (see "Already grabbed" guardrail).
//
// Failed-class grabs (failed/stalled/removed) are excluded — they
// represent attempts the user almost certainly wants to retry, not
// suppress. If every grab for a GUID is failed-class, the map will
// have no entry for that GUID and no badge will render.
//
// "Most recent" is determined by GrabbedAt string comparison. The
// column stores RFC3339 UTC timestamps which sort lexically the same
// as chronologically, so a string compare is the right tiebreaker.
func indexLatestGrabsByGUID(rows []db.GrabHistory) map[string]db.GrabHistory {
	out := make(map[string]db.GrabHistory, len(rows))
	for _, gr := range rows {
		if isGrabbedTerminalFailure(gr.DownloadStatus) {
			continue
		}
		if existing, ok := out[gr.ReleaseGuid]; !ok || gr.GrabbedAt > existing.GrabbedAt {
			out[gr.ReleaseGuid] = gr
		}
	}
	return out
}

func indexerResultToReleaseBody(r indexer.SearchResult) *releaseBody {
	q := r.Quality
	if q.Resolution == "" && q.Source == "" {
		q = plugin.ParseQualityFromTitle(r.Title)
	}
	return &releaseBody{
		GUID:          r.GUID,
		Title:         r.Title,
		Indexer:       r.Indexer,
		IndexerID:     r.IndexerID,
		Protocol:      string(r.Protocol),
		DownloadURL:   r.DownloadURL,
		InfoURL:       r.InfoURL,
		Size:          r.Size,
		Seeds:         r.Seeds,
		Peers:         r.Peers,
		AgeDays:       r.AgeDays,
		Quality:       q,
		QualityScore:  q.Score(),
		PackType:      string(r.PackType),
		EpisodeCount:  r.EpisodeCount,
		FilterReasons: append([]string(nil), r.FilterReasons...),
		Languages:     r.Languages,
	}
}

// buildEpisodeQueries builds the list of Sonarr-style search queries to issue
// for a specific episode or season. Season-level searches issue TWO queries —
// one using the canonical "S01" form and one using the "Season 1" form —
// because many torznab indexers index releases under exactly one of the two
// naming conventions and will silently miss the other. Dedupe happens at the
// caller by GUID. See Sonarr issue #3934 for the same bug in Sonarr proper.
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

// interactiveSearchTimeout is the hard interactive-search cut-off. This
// was 12s when half the fleet failed instantly (masked HTTP 400s) and
// dead indexers were the thing to kill quickly. With the 2026-08-17
// engine fixes all indexers genuinely scrape and queue behind the
// global semaphore and Pulse's per-site rate limiting, so a cold-cache
// season search legitimately needs 20s+. Failures are now fast and
// honest (torznab <error> docs, no hangs), so the budget matches the
// 25s per-indexer timeout plus queue headroom. Stragglers show up in
// pilot logs as `indexer search failed duration_ms=30000+ error=context
// deadline exceeded`. The stream endpoint shows results long before.
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
		// Blocklist filter: any release that's been previously stalled
		// gets tagged with a filter reason so the UI can render it
		// grayed with an override button.
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

// ── Route registration ───────────────────────────────────────────────────────

// RegisterReleaseRoutes registers the release search and grab history endpoints.
// qualitySvc may be nil; when nil, the quality-profile gate is skipped and all
// releases are considered acceptable regardless of the series profile.
func RegisterReleaseRoutes(api huma.API, indexerSvc *indexer.Service, showSvc *show.Service, downloaderSvc *downloader.Service, blocklistSvc *blocklist.Service, qualitySvc *quality.Service) {
	// GET /api/v1/series/{id}/releases?season=1&episode=5
	huma.Register(api, huma.Operation{
		OperationID: "search-series-releases",
		Method:      http.MethodGet,
		Path:        "/api/v1/series/{id}/releases",
		Summary:     "Search for releases for a series episode across all enabled indexers",
		Tags:        []string{"Releases"},
	}, func(ctx context.Context, input *episodeReleasesInput) (*releaseListOutput, error) {
		rs, err := newReleaseSearch(ctx, input, indexerSvc, showSvc, blocklistSvc, qualitySvc)
		if err != nil {
			return nil, err
		}
		searchCtx, cancel := context.WithTimeout(ctx, interactiveSearchTimeout)
		defer cancel()
		// Indexer errors are in the outcomes; the combined error is redundant.
		snap, _ := indexerSvc.SearchInteractive(searchCtx, rs.groups, nil)
		return &releaseListOutput{Body: rs.list(ctx, snap)}, nil
	})

	// GET /api/v1/series/{id}/releases/stream?season=1&episode=5
	//
	// The same search, sent as it happens: newline-delimited JSON, one
	// releaseListBody per line, each holding everything found so far.
	// Fast indexers' results show in a second or two instead of waiting
	// out the slowest one. The last line has done=true.
	huma.Register(api, huma.Operation{
		OperationID: "stream-series-releases",
		Method:      http.MethodGet,
		Path:        "/api/v1/series/{id}/releases/stream",
		Summary:     "Search for releases, streaming results as each indexer answers (NDJSON)",
		Tags:        []string{"Releases"},
	}, func(ctx context.Context, input *episodeReleasesInput) (*huma.StreamResponse, error) {
		rs, err := newReleaseSearch(ctx, input, indexerSvc, showSvc, blocklistSvc, qualitySvc)
		if err != nil {
			return nil, err
		}
		return &huma.StreamResponse{Body: func(hctx huma.Context) {
			streamReleases(hctx, indexerSvc, rs)
		}}, nil
	})

	// GET /api/v1/series/{id}/grab-history
	huma.Register(api, huma.Operation{
		OperationID: "list-series-grab-history",
		Method:      http.MethodGet,
		Path:        "/api/v1/series/{id}/grab-history",
		Summary:     "List grab history for a series",
		Tags:        []string{"Releases"},
	}, func(ctx context.Context, input *seriesGrabHistoryInput) (*grabHistoryListOutput, error) {
		if _, err := showSvc.Get(ctx, input.SeriesID); err != nil {
			if errors.Is(err, show.ErrNotFound) {
				return nil, huma.Error404NotFound("series not found")
			}
			return nil, huma.NewError(http.StatusInternalServerError, "failed to get series", err)
		}

		rows, err := indexerSvc.GrabHistory(ctx, input.SeriesID)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list grab history", err)
		}

		bodies := make([]*grabHistoryBody, 0, len(rows))
		for _, row := range rows {
			bodies = append(bodies, grabToBody(row))
		}

		return &grabHistoryListOutput{Body: bodies}, nil
	})

	// POST /api/v1/series/{id}/releases/grab
	huma.Register(api, huma.Operation{
		OperationID:   "grab-series-release",
		Method:        http.MethodPost,
		Path:          "/api/v1/series/{id}/releases/grab",
		Summary:       "Grab a release: submit to download client and record in history",
		Tags:          []string{"Releases"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, input *grabInput) (*grabOutput, error) {
		series, err := showSvc.Get(ctx, input.SeriesID)
		if err != nil {
			if errors.Is(err, show.ErrNotFound) {
				return nil, huma.Error404NotFound("series not found")
			}
			return nil, huma.NewError(http.StatusInternalServerError, "failed to get series", err)
		}

		release := plugin.Release{
			GUID:         input.Body.GUID,
			Title:        input.Body.Title,
			Protocol:     plugin.Protocol(input.Body.Protocol),
			DownloadURL:  input.Body.DownloadURL,
			Size:         input.Body.Size,
			Quality:      input.Body.Quality,
			MediaType:    "tv",
			MediaTitle:   series.Title,
			MediaYear:    series.Year,
			SeasonNumber: input.Body.SeasonNumber,
			// Forward arr-side identity to history-aware download
			// clients (Haul) so the next "have I downloaded this?"
			// lookup can match by Pilot's own UUIDs.
			TMDBID:    int(series.TMDBID),
			SeriesID:  input.SeriesID,
			EpisodeID: input.Body.EpisodeID,
		}

		// Override flow: if the user clicked "grab anyway" on a grayed
		// (filtered) row, remove the release from the blocklist first so
		// the normal grab flow proceeds without being immediately re-filtered
		// on the next search.
		if input.Body.Override && blocklistSvc != nil {
			if err := blocklistSvc.RemoveByGUID(ctx, input.Body.GUID); err != nil {
				// Not fatal — just log. The grab may proceed.
				_ = err
			}
		}

		// Instrumentation: log indexer-claimed peer counts so we can join
		// them offline against Haul's real peer counts per grab. This is
		// the measurement pipeline from plans/dead-torrent-phase0.md Step 0.
		// After a week we grep for grab.metrics and decide whether the
		// post-hoc stall detection is proportionate to the actual failure
		// rate, or whether pre-flight BEP 48 scrape is warranted.
		slog.Default().Info("grab.metrics",
			"release_guid", release.GUID,
			"title", release.Title,
			"indexer", input.Body.IndexerID,
			"indexer_seeds", input.Body.Size, // seeds not in body yet — Phase 1 adds it; indexer_id is enough for now
			"override", input.Body.Override,
			"grab_source", "interactive",
		)

		// Record the grab in history first with status "queued". Source is
		// "interactive" so the stall watcher will NOT auto-re-search on
		// failure — the user picked this release deliberately, they own
		// the decision if it turns out to be dead.
		row, err := indexerSvc.CreateGrab(ctx, indexer.GrabRequest{
			SeriesID:     input.SeriesID,
			EpisodeID:    input.Body.EpisodeID,
			SeasonNumber: input.Body.SeasonNumber,
			Release:      release,
			IndexerID:    input.Body.IndexerID,
			Source:       "interactive",
		})
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to record grab", err)
		}

		// Submit to a download client and update the grab record with client IDs.
		if downloaderSvc != nil {
			clientID, itemID, addErr := downloaderSvc.Add(ctx, release, nil)
			if addErr != nil {
				// Mark the grab as failed so the search-time guardrail
				// doesn't keep surfacing "already grabbed" on a release
				// that never actually made it to the download client.
				_ = indexerSvc.UpdateGrabStatus(ctx, row.ID, "failed")
				return nil, huma.NewError(http.StatusBadGateway, "failed to send to download client: "+addErr.Error())
			}
			_ = indexerSvc.UpdateGrabDownloadClient(ctx, db.UpdateGrabDownloadClientParams{
				ID:               row.ID,
				DownloadClientID: dbutil.NullableString(clientID),
				ClientItemID:     dbutil.NullableString(itemID),
			})
			// For torrent clients (Haul), itemID is the info_hash.
			// Record it on the grab so the stall watcher can correlate
			// Haul's reports back to this grab.
			if itemID != "" && release.Protocol == plugin.ProtocolTorrent {
				_ = indexerSvc.UpdateGrabInfoHash(ctx, row.ID, itemID)
				row.InfoHash = &itemID
			}
			row.DownloadClientID = dbutil.NullableString(clientID)
			row.ClientItemID = dbutil.NullableString(itemID)
		}

		return &grabOutput{Body: grabToBody(row)}, nil
	})

	// POST /api/v1/series/{id}/auto-search — search and automatically grab the best result
	type autoSearchInput struct {
		SeriesID string `path:"id"`
		Body     struct {
			Season    int    `json:"season"            doc:"Season number"`
			Episode   int    `json:"episode,omitempty"  required:"false" doc:"Episode number (omit for full season)"`
			EpisodeID string `json:"episode_id,omitempty" required:"false" doc:"Episode UUID for grab history"`
		}
	}

	type autoSearchOutput struct {
		Body *autoSearchResultBody
	}

	huma.Register(api, huma.Operation{
		OperationID: "auto-search-episode",
		Method:      http.MethodPost,
		Path:        "/api/v1/series/{id}/auto-search",
		Summary:     "Search and automatically grab the best matching release",
		Description: "Searches all enabled indexers and grabs the highest-scored result. Uses quality profile scoring to pick the best match.",
		Tags:        []string{"Releases"},
	}, func(ctx context.Context, input *autoSearchInput) (*autoSearchOutput, error) {
		body, err := runAutoSearchAndGrab(ctx, autoSearchDeps{
			ShowSvc:       showSvc,
			IndexerSvc:    indexerSvc,
			DownloaderSvc: downloaderSvc,
			BlocklistSvc:  blocklistSvc,
			QualitySvc:    qualitySvc,
		}, autoSearchParams{
			SeriesID:  input.SeriesID,
			Season:    input.Body.Season,
			Episode:   input.Body.Episode,
			EpisodeID: input.Body.EpisodeID,
			Source:    "auto_search",
		})
		if err != nil {
			return nil, err
		}
		return &autoSearchOutput{Body: body}, nil
	})

	registerResearchEndpoint(api, indexerSvc, showSvc, downloaderSvc, blocklistSvc, qualitySvc)
}

// autoSearchDeps gathers the services runAutoSearchAndGrab needs. Lifted
// out of the handler closure so the same flow can be reused by the
// /grabs/by-hash/{hash}/research endpoint, which calls it after
// blocklisting the dead release.
type autoSearchDeps struct {
	ShowSvc       *show.Service
	IndexerSvc    *indexer.Service
	DownloaderSvc *downloader.Service
	BlocklistSvc  *blocklist.Service
	QualitySvc    *quality.Service
}

// autoSearchParams scopes one auto-search call to a single (series,
// season[, episode]) target. Source is recorded on the resulting
// grab row — "auto_search" makes the stall watcher eligible to
// auto-re-search; "manual_research" stamps user-driven retries so the
// circuit breaker treats them as off-band.
type autoSearchParams struct {
	SeriesID  string
	Season    int
	Episode   int
	EpisodeID string
	Source    string
}

// pickBestViable returns the release to grab from viable, which is in
// ranking order. Among releases of the top quality and revision, it
// prefers one listed by several indexers (more confidence its seed count
// is real). That preference used to look across all qualities, so a 720p
// on two indexers could beat a 2160p on one.
func pickBestViable(viable []indexer.SearchResult) indexer.SearchResult {
	best := viable[0]
	sameTier := func(r indexer.SearchResult) bool {
		return r.QualityScore == best.QualityScore && r.Quality.EffectiveRevision() == best.Quality.EffectiveRevision()
	}
	titleCounts := make(map[string]int)
	for _, r := range viable {
		if sameTier(r) {
			titleCounts[r.Title]++
		}
	}
	for _, r := range viable {
		if !sameTier(r) {
			break // viable is ranked; nothing later is in the top tier
		}
		if titleCounts[r.Title] > 1 && titleCounts[r.Title] > titleCounts[best.Title] {
			return r
		}
	}
	return best
}

// autoSearchResultBody is shared between the public auto-search
// handler and the research handler.
type autoSearchResultBody struct {
	Result       string `json:"result"` // "grabbed", "no_match"
	ReleaseTitle string `json:"release_title,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

// runAutoSearchAndGrab executes the full search → filter → grab → submit
// flow. Returns a structured result body for "no_match" cases and a
// huma error for fatal ones (series not found, DB errors). Caller wraps
// the body in whatever output struct it needs.
func runAutoSearchAndGrab(ctx context.Context, deps autoSearchDeps, p autoSearchParams) (*autoSearchResultBody, error) {
	showSvc := deps.ShowSvc
	indexerSvc := deps.IndexerSvc
	downloaderSvc := deps.DownloaderSvc
	blocklistSvc := deps.BlocklistSvc
	qualitySvc := deps.QualitySvc

	source := p.Source
	if source == "" {
		source = "auto_search"
	}

	series, err := showSvc.Get(ctx, p.SeriesID)
	if err != nil {
		if errors.Is(err, show.ErrNotFound) {
			return nil, huma.Error404NotFound("series not found")
		}
		return nil, huma.NewError(http.StatusInternalServerError, "failed to get series", err)
	}

	absoluteEp, tvdbToAbs := episodeContext(ctx, showSvc, series, p.Season, p.Episode)
	groups := searchPlan(series, p.Season, p.Episode, absoluteEp, series.SeriesType == "anime")
	snap, searchErr := indexerSvc.SearchStream(ctx, groups, nil)

	// Filter results to only include the requested series and season/episode.
	results := filterByEpisode(snap.Results, series.Title, series.AlternateTitles, p.Season, p.Episode, absoluteEp, tvdbToAbs)

	// Tag releases outside the series' quality profile. For auto-search
	// these tags also gate the viable set further down, so the auto-grab
	// never picks a release rejected by the profile.
	var profile *quality.Profile
	if qualitySvc != nil && series.QualityProfileID != "" {
		if prof, err := qualitySvc.Get(ctx, series.QualityProfileID); err == nil {
			profile = &prof
			applyQualityProfile(results, profile)
		}
	}

	// When the episode already has a file of known quality, only real
	// upgrades (or a PROPER of the same quality) are viable. Without this
	// an auto-search for an episode on disk grabbed the top result even if
	// it was worse than the file. A file of unknown quality doesn't gate:
	// the user asked for this search explicitly.
	var current *plugin.Quality
	if p.EpisodeID != "" && p.Episode > 0 {
		if files, err := showSvc.ListFiles(ctx, p.SeriesID); err == nil {
			if cur := wanted.BestFileQuality(files, p.EpisodeID); cur != nil && cur.Score() > 0 {
				current = cur
			}
		}
	}

	if len(results) == 0 {
		reason := "no results from any indexer"
		if searchErr != nil {
			reason = searchErr.Error()
		}
		return &autoSearchResultBody{
			Result: "no_match",
			Reason: reason,
		}, nil
	}

	// Auto-search honors the per-indexer min_seeders filter and the
	// blocklist filter. Search() already populates FilterReasons for
	// min_seeders violations; we additionally check blocklist here.
	// A release is "viable" for auto-grab if it has no filter reasons.
	// One blocklist query for all of them, not one per release.
	var blocked map[string]bool
	if blocklistSvc != nil {
		guids := make([]string, len(results))
		for i, r := range results {
			guids[i] = r.GUID
		}
		blocked, _ = blocklistSvc.BlocklistedGUIDs(ctx, guids)
	}
	var viable []indexer.SearchResult
	for _, r := range results {
		if len(r.FilterReasons) > 0 {
			continue
		}
		if current != nil {
			if ok, _ := decision.Evaluate(r.Quality, current, profile); !ok {
				continue
			}
		}
		if blocked[r.GUID] {
			continue
		}
		viable = append(viable, r)
	}

	if len(viable) == 0 {
		reason := fmt.Sprintf("found %d results but all were filtered (below min_seeders or on blocklist)", len(results))
		if current != nil {
			reason = fmt.Sprintf("found %d results but none is an upgrade over the current %s file", len(results), current.Name)
		}
		return &autoSearchResultBody{
			Result: "no_match",
			Reason: reason,
		}, nil
	}

	best := pickBestViable(viable)

	// Look up series for TMDB id + title (used by Haul's history
	// index and rename-on-complete). Best-effort; fall back to
	// whatever we already had if the lookup fails.
	var seriesTitle string
	var seriesYear int
	var seriesTMDBID int
	if s, sErr := showSvc.Get(ctx, p.SeriesID); sErr == nil {
		seriesTitle = s.Title
		seriesYear = s.Year
		seriesTMDBID = int(s.TMDBID)
	}

	release := plugin.Release{
		GUID:         best.GUID,
		Title:        best.Title,
		Protocol:     best.Protocol,
		DownloadURL:  best.DownloadURL,
		Size:         best.Size,
		Quality:      best.Quality,
		MediaType:    "tv",
		MediaTitle:   seriesTitle,
		MediaYear:    seriesYear,
		SeasonNumber: p.Season,
		// Arr-side identity for Haul's history lookup.
		TMDBID:    seriesTMDBID,
		SeriesID:  p.SeriesID,
		EpisodeID: p.EpisodeID,
	}

	// Record the grab. Source is whatever the caller said — stall
	// watcher uses this to decide whether to auto-re-search on later
	// failure ("auto_search" yes; "manual_research" or anything else no).
	row, err := indexerSvc.CreateGrab(ctx, indexer.GrabRequest{
		SeriesID:     p.SeriesID,
		EpisodeID:    p.EpisodeID,
		SeasonNumber: p.Season,
		Release:      release,
		IndexerID:    best.IndexerID,
		Source:       source,
	})
	if err != nil {
		return nil, huma.NewError(http.StatusInternalServerError, "failed to record grab", err)
	}

	// Submit to download client.
	if downloaderSvc != nil {
		clientID, itemID, addErr := downloaderSvc.Add(ctx, release, nil)
		if addErr != nil {
			// Mark the grab as failed so the search-time guardrail
			// doesn't keep surfacing "already grabbed" on a release
			// that never actually made it to the download client.
			_ = indexerSvc.UpdateGrabStatus(ctx, row.ID, "failed")
			// nilerr: we intentionally return the rejection as a
			// structured "no_match" body instead of a 500 so the UI
			// can show the user why the grab failed.
			return &autoSearchResultBody{ //nolint:nilerr
				Result:       "no_match",
				ReleaseTitle: best.Title,
				Reason:       "download client rejected: " + addErr.Error(),
			}, nil
		}
		_ = indexerSvc.UpdateGrabDownloadClient(ctx, db.UpdateGrabDownloadClientParams{
			ID:               row.ID,
			DownloadClientID: dbutil.NullableString(clientID),
			ClientItemID:     dbutil.NullableString(itemID),
		})
		if itemID != "" && release.Protocol == plugin.ProtocolTorrent {
			_ = indexerSvc.UpdateGrabInfoHash(ctx, row.ID, itemID)
		}
	}

	return &autoSearchResultBody{
		Result:       "grabbed",
		ReleaseTitle: best.Title,
	}, nil
}

// filterByEpisode removes results that don't match the requested series and
// season/episode. If season is 0, only the series-title gate is applied
// (whole-series search). Season packs are kept when searching for a specific
// season.
//
// The series-title gate is the regression fix for the wrong-torrent bug: many
// torznab indexers do loose matching on the raw query string and return
// unrelated shows whose names happen to share common words. We re-parse each
// result's title and drop anything whose parsed show title doesn't match the
// series we're actually searching for.
// filterByEpisode (extended) accepts an absolute episode number and
// an optional TVDB→absolute converter. When `absolute > 0`:
//
//   - releases the parser tagged with a matching AbsoluteEpisode
//     (fansub "Show - 48" form) pass through regardless of season;
//   - releases parsed as TVDB-style "Show S03E01" run through
//     `tvdbToAbsolute(parsed_season, parsed_episode)`; if that
//     yields the requested absolute, they also pass.
//
// Pass `absolute=0` and `tvdbToAbsolute=nil` for the non-anime
// behavior (no augmentation, strict season/episode matching only).
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
		if !parser.TitleMatchesAny(candidates, parsed.ShowTitle) {
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

func grabToBody(row db.GrabHistory) *grabHistoryBody {
	grabbedAt, _ := time.Parse(time.RFC3339, row.GrabbedAt)
	return &grabHistoryBody{
		ID:             row.ID,
		SeriesID:       row.SeriesID,
		EpisodeID:      row.EpisodeID,
		SeasonNumber:   row.SeasonNumber,
		IndexerID:      row.IndexerID,
		ReleaseGUID:    row.ReleaseGuid,
		ReleaseTitle:   row.ReleaseTitle,
		Protocol:       row.Protocol,
		Size:           row.Size,
		DownloadStatus: row.DownloadStatus,
		GrabbedAt:      grabbedAt,
	}
}

// registerResearchEndpoint wires POST /api/v1/grabs/by-hash/{hash}/research.
//
// Story: a torrent has gone dead (paused-and-stalled by Haul) and the
// user wants Pilot to pick a different release for the same episode.
// The existing auto-search-episode endpoint can do the search itself,
// but the caller (Haul UI) only knows the info_hash — so this endpoint
// looks the grab up, blocklists the dead release, and forwards into
// the same flow with source="manual_research" so the stall watcher's
// circuit breaker treats it as user-initiated rather than counting it
// as another auto-retry.
func registerResearchEndpoint(
	api huma.API,
	indexerSvc *indexer.Service,
	showSvc *show.Service,
	downloaderSvc *downloader.Service,
	blocklistSvc *blocklist.Service,
	qualitySvc *quality.Service,
) {
	type input struct {
		Hash string `path:"hash" doc:"BitTorrent info hash of the dead grab"`
	}
	type output struct {
		Body *autoSearchResultBody
	}

	huma.Register(api, huma.Operation{
		OperationID: "research-by-info-hash",
		Method:      http.MethodPost,
		Path:        "/api/v1/grabs/by-hash/{hash}/research",
		Summary:     "Re-search and grab an alternative release for a stalled torrent",
		Description: "Looks up the grab by info_hash, blocklists the dead release, then runs the auto-search flow to pick a different release for the same episode. Bypasses the stall-watcher circuit breaker since the request is user-initiated.",
		Tags:        []string{"Releases"},
	}, func(ctx context.Context, in *input) (*output, error) {
		// 1. Look up the grab.
		grab, err := indexerSvc.GetGrabByInfoHash(ctx, in.Hash)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, huma.Error404NotFound("no grab found for info_hash")
			}
			return nil, huma.NewError(http.StatusInternalServerError, "lookup grab failed", err)
		}

		// 2. Blocklist the dead release. user_marked semantics — the
		// user explicitly said "this one's bad". Idempotent: if the
		// stall watcher already added it, swallow the duplicate.
		if blocklistSvc != nil {
			addErr := blocklistSvc.Add(
				ctx,
				grab.SeriesID,
				dbutil.NullStringValue(grab.EpisodeID),
				grab.ReleaseGuid,
				grab.ReleaseTitle,
				dbutil.NullStringValue(grab.IndexerID),
				grab.Protocol,
				grab.Size,
				"user-initiated research from Haul stalled list",
			)
			if addErr != nil && !errors.Is(addErr, blocklist.ErrAlreadyBlocklisted) {
				return nil, huma.NewError(http.StatusInternalServerError, "failed to blocklist dead release", addErr)
			}
		}

		// 3. Remove the original torrent from the download client so the
		// user doesn't end up with the dead release AND the new one both
		// running. Best-effort: if the download client is gone or the
		// torrent has already been removed, log and continue — losing
		// the alternative grab because the dead one's gone is worse.
		if downloaderSvc != nil && grab.DownloadClientID != nil && grab.ClientItemID != nil {
			client, cErr := downloaderSvc.ClientFor(ctx, *grab.DownloadClientID)
			if cErr == nil {
				if rErr := client.Remove(ctx, *grab.ClientItemID, false); rErr != nil {
					// keep_files=false would also delete files on disk
					// — but the user might want to manually rescue
					// partial data, so we err on the safe side and only
					// remove from the engine.
					_ = rErr
				}
			}
		}

		// 4. Mark the original grab as failed so it stops surfacing as
		// "currently downloading" anywhere. Best-effort.
		_ = indexerSvc.UpdateGrabStatus(ctx, grab.ID, "failed")

		// 5. Run the same flow as the auto-search endpoint.
		body, err := runAutoSearchAndGrab(ctx, autoSearchDeps{
			ShowSvc:       showSvc,
			IndexerSvc:    indexerSvc,
			DownloaderSvc: downloaderSvc,
			BlocklistSvc:  blocklistSvc,
			QualitySvc:    qualitySvc,
		}, autoSearchParams{
			SeriesID: grab.SeriesID,
			Season: func() int {
				if grab.SeasonNumber == nil {
					return 0
				}
				return int(*grab.SeasonNumber)
			}(),
			Episode:   0, // grab_history doesn't carry episode-number; let auto-search find any episode in the season
			EpisodeID: dbutil.NullStringValue(grab.EpisodeID),
			Source:    "manual_research",
		})
		if err != nil {
			return nil, err
		}
		return &output{Body: body}, nil
	})
}
