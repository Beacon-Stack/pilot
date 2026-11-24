package v1

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pilot/internal/core/blocklist"
	"github.com/beacon-stack/pilot/internal/core/dbutil"
	"github.com/beacon-stack/pilot/internal/core/decision"
	"github.com/beacon-stack/pilot/internal/core/downloader"
	"github.com/beacon-stack/pilot/internal/core/indexer"
	"github.com/beacon-stack/pilot/internal/core/quality"
	"github.com/beacon-stack/pilot/internal/core/show"
	"github.com/beacon-stack/pilot/internal/core/wanted"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

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
// ranking order. Among releases of the top quality and revision only, it
// prefers one listed by several indexers: more confidence that its seed
// count is real.
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
