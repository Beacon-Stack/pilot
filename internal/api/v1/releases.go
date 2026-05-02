package v1

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pilot/internal/core/blocklist"
	"github.com/beacon-stack/pilot/internal/core/downloader"
	"github.com/beacon-stack/pilot/internal/core/indexer"
	"github.com/beacon-stack/pilot/internal/core/parser"
	"github.com/beacon-stack/pilot/internal/core/quality"
	"github.com/beacon-stack/pilot/internal/core/show"
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

type releaseListOutput struct {
	Body []*releaseBody
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
	// Score formula is resolution*100 + source*10 + codec, so integer-
	// dividing by 100 isolates the resolution score. Anything below the
	// cutoff's resolution digit is stamped as below_quality_profile.
	cutoffResScore := profile.Cutoff.Score() / 100
	if cutoffResScore == 0 {
		return // cutoff not meaningfully set
	}
	for i := range results {
		q := results[i].Quality
		if q.Resolution == "" && q.Source == "" {
			q = plugin.ParseQualityFromTitle(results[i].Title)
		}
		if q.Score()/100 < cutoffResScore {
			results[i].FilterReasons = append(results[i].FilterReasons, "below_quality_profile")
		}
	}
}

// searchAllQueries fans out each query to indexerSvc in parallel and
// merges results, deduping by GUID. The first occurrence of a GUID wins;
// query order is preserved by walking `queries` in input order during
// the merge. Errors from individual searches are collected into a
// combined error; partial results are still returned alongside.
//
// Parallel because each query already calls every indexer in parallel
// internally — running queries serially multiplied total wait time by
// the number of query variations. Season searches typically issue 3+
// variations ("Show", "Show S01", "Show Season 1"), so the serial
// loop turned a 30s worst case into 90s+.
func searchAllQueries(ctx context.Context, indexerSvc *indexer.Service, queries []string, season, episode int) ([]indexer.SearchResult, error) {
	type queryResult struct {
		results []indexer.SearchResult
		err     error
	}
	out := make([]queryResult, len(queries))
	var wg sync.WaitGroup
	for i, query := range queries {
		i, query := i, query
		wg.Add(1)
		go func() {
			defer wg.Done()
			sq := plugin.SearchQuery{Query: query, Season: season, Episode: episode}
			res, err := indexerSvc.Search(ctx, sq)
			out[i] = queryResult{results: res, err: err}
		}()
	}
	wg.Wait()

	seen := make(map[string]struct{})
	var merged []indexer.SearchResult
	var errs []string
	for _, qr := range out {
		if qr.err != nil {
			errs = append(errs, qr.err.Error())
		}
		for _, r := range qr.results {
			if _, ok := seen[r.GUID]; ok {
				continue
			}
			seen[r.GUID] = struct{}{}
			merged = append(merged, r)
		}
	}
	var combined error
	if len(errs) > 0 {
		combined = fmt.Errorf("%d search(es) failed: %v", len(errs), errs)
	}
	return merged, combined
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
		series, err := showSvc.Get(ctx, input.SeriesID)
		if err != nil {
			if errors.Is(err, show.ErrNotFound) {
				return nil, huma.Error404NotFound("series not found")
			}
			return nil, huma.NewError(http.StatusInternalServerError, "failed to get series", err)
		}

		// Anime context: only fetch the absolute episode number when the
		// series is flagged anime AND we're searching for a specific
		// episode (absolute is meaningless for season-pack or whole-series
		// searches). Skipping this lookup for non-anime series saves a DB
		// hit on every manual search.
		var absoluteEp int
		var tvdbToAbs func(int, int) int
		isAnime := series.SeriesType == "anime"
		if isAnime && input.Season > 0 && input.Episode > 0 {
			absoluteEp, _ = showSvc.GetEpisodeAbsoluteNumber(ctx, input.SeriesID, input.Season, input.Episode)
			tmdbID := series.TMDBID
			tvdbToAbs = func(s, e int) int {
				abs, _ := showSvc.TVDBSeasonToAbsolute(tmdbID, s, e)
				return abs
			}
		}
		queries := buildEpisodeQueries(series.Title, input.Season, input.Episode, absoluteEp, isAnime)

		// Hard interactive-search cut-off. A single dead/walled indexer
		// can otherwise hold the search at the per-indexer client timeout
		// (~30s). Empirically healthy indexers come back in 0.2–10s
		// (Cloudflare-walled torznab providers tend to be the slow tail);
		// 12s leaves room for those while killing genuinely dead ones
		// quickly. Stragglers show up in pilot logs as `indexer search
		// failed duration_ms=12000+ error=context deadline exceeded`.
		searchCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		defer cancel()

		results, searchErr := searchAllQueries(searchCtx, indexerSvc, queries, input.Season, input.Episode)

		// Filter results to only include the requested series and season/episode.
		results = filterByEpisode(results, series.Title, series.AlternateTitles, input.Season, input.Episode, absoluteEp, tvdbToAbs)

		// Tag releases outside the series' quality profile so the UI greys
		// them with an "override and grab anyway" button (same UX as the
		// blocklist and min_seeders filters).
		if qualitySvc != nil && series.QualityProfileID != "" {
			if profile, err := qualitySvc.Get(ctx, series.QualityProfileID); err == nil {
				applyQualityProfile(results, &profile)
			}
		}

		// Count how many indexers each title appears on.
		titleIndexers := make(map[string]map[string]bool)
		for _, r := range results {
			if titleIndexers[r.Title] == nil {
				titleIndexers[r.Title] = make(map[string]bool)
			}
			titleIndexers[r.Title][r.IndexerName] = true
		}

		// Build a guid → most-recent grab map so we can badge releases
		// the user has already grabbed before. Cheap: one query per
		// search, hash-map lookup per release. The guardrail covers the
		// common "I forgot, did I already grab this?" case; it does not
		// gate the grab — the UI just shows a confirmation prompt.
		var grabsByGUID map[string]db.GrabHistory
		if grabRows, gErr := indexerSvc.GrabHistory(ctx, input.SeriesID); gErr == nil {
			grabsByGUID = indexLatestGrabsByGUID(grabRows)
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
			// grayed with an override button. Two-keyed dedup: check by
			// both release GUID and info_hash.
			if blocklistSvc != nil {
				ok, bErr := blocklistSvc.IsBlocklistedGUIDOrInfoHash(ctx, r.GUID, "")
				if bErr == nil && ok {
					b.FilterReasons = append(b.FilterReasons, "previously stalled (on blocklist)")
				}
			}
			if prior, ok := grabsByGUID[r.GUID]; ok {
				b.AlreadyGrabbedAt = prior.GrabbedAt
				b.AlreadyGrabbedID = prior.ID
				b.AlreadyGrabbedStatus = prior.DownloadStatus
			}
			bodies[i] = b
		}

		if len(bodies) == 0 && searchErr != nil {
			return nil, huma.NewError(http.StatusBadGateway, searchErr.Error())
		}

		return &releaseListOutput{Body: bodies}, nil
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
				DownloadClientID: sql.NullString{String: clientID, Valid: clientID != ""},
				ClientItemID:     sql.NullString{String: itemID, Valid: itemID != ""},
			})
			// For torrent clients (Haul), itemID is the info_hash.
			// Record it on the grab so the stall watcher can correlate
			// Haul's reports back to this grab.
			if itemID != "" && release.Protocol == plugin.ProtocolTorrent {
				_ = indexerSvc.UpdateGrabInfoHash(ctx, row.ID, itemID)
				row.InfoHash = sql.NullString{String: itemID, Valid: true}
			}
			row.DownloadClientID = sql.NullString{String: clientID, Valid: clientID != ""}
			row.ClientItemID = sql.NullString{String: itemID, Valid: itemID != ""}
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

	type autoSearchResultBody struct {
		Result       string `json:"result"` // "grabbed", "no_match"
		ReleaseTitle string `json:"release_title,omitempty"`
		Reason       string `json:"reason,omitempty"`
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
		series, err := showSvc.Get(ctx, input.SeriesID)
		if err != nil {
			if errors.Is(err, show.ErrNotFound) {
				return nil, huma.Error404NotFound("series not found")
			}
			return nil, huma.NewError(http.StatusInternalServerError, "failed to get series", err)
		}

		// Anime context: same rationale as the manual-search path above —
		// only fetch absolute number when the series is anime AND a
		// specific episode is being searched.
		var absoluteEp int
		var tvdbToAbs func(int, int) int
		isAnime := series.SeriesType == "anime"
		if isAnime && input.Body.Season > 0 && input.Body.Episode > 0 {
			absoluteEp, _ = showSvc.GetEpisodeAbsoluteNumber(ctx, input.SeriesID, input.Body.Season, input.Body.Episode)
			tmdbID := series.TMDBID
			tvdbToAbs = func(s, e int) int {
				abs, _ := showSvc.TVDBSeasonToAbsolute(tmdbID, s, e)
				return abs
			}
		}
		queries := buildEpisodeQueries(series.Title, input.Body.Season, input.Body.Episode, absoluteEp, isAnime)
		results, searchErr := searchAllQueries(ctx, indexerSvc, queries, input.Body.Season, input.Body.Episode)

		// Filter results to only include the requested series and season/episode.
		results = filterByEpisode(results, series.Title, series.AlternateTitles, input.Body.Season, input.Body.Episode, absoluteEp, tvdbToAbs)

		// Tag releases outside the series' quality profile. For auto-search
		// these tags also gate the viable set further down, so the auto-grab
		// never picks a release rejected by the profile.
		if qualitySvc != nil && series.QualityProfileID != "" {
			if profile, err := qualitySvc.Get(ctx, series.QualityProfileID); err == nil {
				applyQualityProfile(results, &profile)
			}
		}

		if len(results) == 0 {
			reason := "no results from any indexer"
			if searchErr != nil {
				reason = searchErr.Error()
			}
			return &autoSearchOutput{Body: &autoSearchResultBody{
				Result: "no_match",
				Reason: reason,
			}}, nil
		}

		// Auto-search honors the per-indexer min_seeders filter and the
		// blocklist filter. Search() already populates FilterReasons for
		// min_seeders violations; we additionally check blocklist here.
		// A release is "viable" for auto-grab if it has no filter reasons.
		var viable []indexer.SearchResult
		for _, r := range results {
			if len(r.FilterReasons) > 0 {
				continue
			}
			if blocklistSvc != nil {
				ok, bErr := blocklistSvc.IsBlocklistedGUIDOrInfoHash(ctx, r.GUID, "")
				if bErr == nil && ok {
					continue
				}
			}
			viable = append(viable, r)
		}

		if len(viable) == 0 {
			return &autoSearchOutput{Body: &autoSearchResultBody{
				Result: "no_match",
				Reason: fmt.Sprintf("found %d results but all were filtered (below min_seeders or on blocklist)", len(results)),
			}}, nil
		}

		// Prefer results that appear on multiple indexers (higher confidence
		// that the seed count is accurate).
		best := viable[0]
		if len(viable) > 1 {
			titleCounts := make(map[string]int)
			for _, r := range viable {
				titleCounts[r.Title]++
			}
			for _, r := range viable {
				if titleCounts[r.Title] > 1 && titleCounts[r.Title] > titleCounts[best.Title] {
					best = r
					break
				}
			}
		}

		// Look up series for TMDB id + title (used by Haul's history
		// index and rename-on-complete). Best-effort; fall back to
		// whatever we already had if the lookup fails.
		var seriesTitle string
		var seriesYear int
		var seriesTMDBID int
		if s, sErr := showSvc.Get(ctx, input.SeriesID); sErr == nil {
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
			SeasonNumber: input.Body.Season,
			// Arr-side identity for Haul's history lookup.
			TMDBID:    seriesTMDBID,
			SeriesID:  input.SeriesID,
			EpisodeID: input.Body.EpisodeID,
		}

		// Record the grab. Source is "auto_search" so stall watcher is
		// allowed to auto-re-search on failure (with circuit breaker).
		seasonNum := input.Body.Season
		row, err := indexerSvc.CreateGrab(ctx, indexer.GrabRequest{
			SeriesID:     input.SeriesID,
			EpisodeID:    input.Body.EpisodeID,
			SeasonNumber: seasonNum,
			Release:      release,
			IndexerID:    best.IndexerID,
			Source:       "auto_search",
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
				return &autoSearchOutput{Body: &autoSearchResultBody{ //nolint:nilerr
					Result:       "no_match",
					ReleaseTitle: best.Title,
					Reason:       "download client rejected: " + addErr.Error(),
				}}, nil
			}
			_ = indexerSvc.UpdateGrabDownloadClient(ctx, db.UpdateGrabDownloadClientParams{
				ID:               row.ID,
				DownloadClientID: sql.NullString{String: clientID, Valid: clientID != ""},
				ClientItemID:     sql.NullString{String: itemID, Valid: itemID != ""},
			})
			if itemID != "" && release.Protocol == plugin.ProtocolTorrent {
				_ = indexerSvc.UpdateGrabInfoHash(ctx, row.ID, itemID)
			}
		}

		return &autoSearchOutput{Body: &autoSearchResultBody{
			Result:       "grabbed",
			ReleaseTitle: best.Title,
		}}, nil
	})
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
	var epID *string
	if row.EpisodeID.Valid {
		epID = &row.EpisodeID.String
	}
	var sn *int64
	if row.SeasonNumber.Valid {
		v := int64(row.SeasonNumber.Int32)
		sn = &v
	}
	var idxID *string
	if row.IndexerID.Valid {
		idxID = &row.IndexerID.String
	}
	grabbedAt, _ := time.Parse(time.RFC3339, row.GrabbedAt)
	return &grabHistoryBody{
		ID:             row.ID,
		SeriesID:       row.SeriesID,
		EpisodeID:      epID,
		SeasonNumber:   sn,
		IndexerID:      idxID,
		ReleaseGUID:    row.ReleaseGuid,
		ReleaseTitle:   row.ReleaseTitle,
		Protocol:       row.Protocol,
		Size:           int64(row.Size),
		DownloadStatus: row.DownloadStatus,
		GrabbedAt:      grabbedAt,
	}
}
