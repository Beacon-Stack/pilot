package v1

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pilot/internal/core/blocklist"
	"github.com/beacon-stack/pilot/internal/core/dbutil"
	"github.com/beacon-stack/pilot/internal/core/downloader"
	"github.com/beacon-stack/pilot/internal/core/indexer"
	"github.com/beacon-stack/pilot/internal/core/quality"
	"github.com/beacon-stack/pilot/internal/core/show"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

// Request / response shapes

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

// Helpers

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
