package v1

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pilot/internal/core/blocklist"
	"github.com/beacon-stack/pilot/internal/core/dbutil"
	"github.com/beacon-stack/pilot/internal/core/downloader"
	"github.com/beacon-stack/pilot/internal/core/indexer"
	"github.com/beacon-stack/pilot/internal/core/quality"
	"github.com/beacon-stack/pilot/internal/core/show"
	"github.com/beacon-stack/pilot/internal/core/wanted"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	releaseparser "github.com/beacon-stack/pilot/internal/parser"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

// ── Request / response shapes ────────────────────────────────────────────────

type wantedEpisodeInput struct {
	Page    int `query:"page"     default:"1"   minimum:"1"`
	PerPage int `query:"per_page" default:"50"  minimum:"1" maximum:"250"`
}

type wantedEpisodeBody struct {
	EpisodeID     string `json:"episode_id"     doc:"Episode UUID"`
	SeriesID      string `json:"series_id"      doc:"Series UUID"`
	SeriesTitle   string `json:"series_title"   doc:"Series title"`
	SeasonNumber  int64  `json:"season_number"  doc:"Season number"`
	EpisodeNumber int64  `json:"episode_number" doc:"Episode number within season"`
	EpisodeTitle  string `json:"episode_title"  doc:"Episode title"`
	AirDate       string `json:"air_date"       doc:"Air date (YYYY-MM-DD)"`
	HasFile       bool   `json:"has_file"       doc:"Whether a file is linked to this episode"`
	Monitored     bool   `json:"monitored"      doc:"Whether this episode is monitored"`
	// Cutoff-unmet list only.
	CurrentQuality string `json:"current_quality,omitempty" doc:"Quality of the file on disk"`
	CutoffQuality  string `json:"cutoff_quality,omitempty"  doc:"The profile's cutoff quality"`
}

type wantedEpisodeListBody struct {
	Episodes []*wantedEpisodeBody `json:"episodes"`
	Total    int64                `json:"total"`
	Page     int                  `json:"page"`
	PerPage  int                  `json:"per_page"`
}

type wantedEpisodeListOutput struct {
	Body *wantedEpisodeListBody
}

// RegisterWantedRoutes registers the wanted/missing and wanted/cutoff endpoints.
func RegisterWantedRoutes(api huma.API, q db.Querier, profiles wanted.ProfileGetter) {
	// GET /api/v1/wanted/missing — monitored episodes with no file, air_date <= today
	huma.Register(api, huma.Operation{
		OperationID: "wanted-missing",
		Method:      http.MethodGet,
		Path:        "/api/v1/wanted/missing",
		Summary:     "List monitored episodes with no file",
		Tags:        []string{"Wanted"},
	}, func(ctx context.Context, input *wantedEpisodeInput) (*wantedEpisodeListOutput, error) {
		total, err := q.CountMissingEpisodes(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to count missing episodes", err)
		}

		offset := int64((input.Page - 1) * input.PerPage)
		rows, err := q.ListMissingEpisodesWithSeries(ctx, db.ListMissingEpisodesWithSeriesParams{
			Limit:  int64(input.PerPage),
			Offset: offset,
		})
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list missing episodes", err)
		}

		bodies := make([]*wantedEpisodeBody, 0, len(rows))
		for _, r := range rows {
			bodies = append(bodies, &wantedEpisodeBody{
				EpisodeID:     r.ID,
				SeriesID:      r.SeriesID,
				SeriesTitle:   r.SeriesTitle,
				SeasonNumber:  r.SeasonNumber,
				EpisodeNumber: r.EpisodeNumber,
				EpisodeTitle:  r.Title,
				AirDate:       dbutil.NullStringValue(r.AirDate),
				HasFile:       r.HasFile,
				Monitored:     r.Monitored,
			})
		}

		return &wantedEpisodeListOutput{Body: &wantedEpisodeListBody{
			Episodes: bodies,
			Total:    total,
			Page:     input.Page,
			PerPage:  input.PerPage,
		}}, nil
	})

	// GET /api/v1/wanted/cutoff — monitored episodes whose file is below
	// the quality profile cutoff.
	huma.Register(api, huma.Operation{
		OperationID: "wanted-cutoff",
		Method:      http.MethodGet,
		Path:        "/api/v1/wanted/cutoff",
		Summary:     "List episodes whose file quality is below the profile cutoff",
		Tags:        []string{"Wanted"},
	}, func(ctx context.Context, input *wantedEpisodeInput) (*wantedEpisodeListOutput, error) {
		var eps []wanted.CutoffEpisode
		var err error
		if profiles != nil {
			eps, err = wanted.CutoffUnmet(ctx, q, profiles)
		}
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list cutoff-unmet episodes", err)
		}
		start := (input.Page - 1) * input.PerPage
		end := start + input.PerPage
		if start > len(eps) {
			start = len(eps)
		}
		if end > len(eps) {
			end = len(eps)
		}
		bodies := make([]*wantedEpisodeBody, 0, end-start)
		for _, ce := range eps[start:end] {
			bodies = append(bodies, &wantedEpisodeBody{
				EpisodeID:      ce.Episode.ID,
				SeriesID:       ce.Episode.SeriesID,
				SeriesTitle:    ce.SeriesTitle,
				SeasonNumber:   ce.Episode.SeasonNumber,
				EpisodeNumber:  ce.Episode.EpisodeNumber,
				EpisodeTitle:   ce.Episode.Title,
				AirDate:        dbutil.NullStringValue(ce.Episode.AirDate),
				HasFile:        ce.Episode.HasFile,
				Monitored:      ce.Episode.Monitored,
				CurrentQuality: qualityDisplayName(ce.Current),
				CutoffQuality:  qualityDisplayName(ce.Cutoff),
			})
		}
		return &wantedEpisodeListOutput{Body: &wantedEpisodeListBody{
			Episodes: bodies,
			Total:    int64(len(eps)),
			Page:     input.Page,
			PerPage:  input.PerPage,
		}}, nil
	})
}

func qualityDisplayName(q plugin.Quality) string {
	if q.Name != "" {
		return q.Name
	}
	return releaseparser.QualityName(q)
}

// ── Search all ───────────────────────────────────────────────────────────────

// maxBulkSearch caps one "search all" run. Each episode searches every
// indexer, so a very large backlog is better worked through in batches.
const maxBulkSearch = 500

type wantedSearchOutput struct {
	Body struct {
		Queued  int    `json:"queued"  doc:"Number of episodes queued for search"`
		Message string `json:"message" doc:"What happens next"`
	}
}

// RegisterWantedSearchRoutes registers the "search all missing" and
// "search all cutoff unmet" actions. Each queues the matching episodes
// and searches them one at a time in the background, grabbing the best
// release the profile accepts (an upgrade, for cutoff-unmet episodes).
func RegisterWantedSearchRoutes(
	api huma.API,
	q db.Querier,
	showSvc *show.Service,
	indexerSvc *indexer.Service,
	downloaderSvc *downloader.Service,
	blocklistSvc *blocklist.Service,
	qualitySvc *quality.Service,
	logger *slog.Logger,
) {
	deps := autoSearchDeps{
		ShowSvc:       showSvc,
		IndexerSvc:    indexerSvc,
		DownloaderSvc: downloaderSvc,
		BlocklistSvc:  blocklistSvc,
		QualitySvc:    qualitySvc,
	}
	runner := newWantedSearchRunner(func(ctx context.Context, p autoSearchParams) (*autoSearchResultBody, error) {
		return runAutoSearchAndGrab(ctx, deps, p)
	}, logger)

	register := func(kind, path, summary string, targets func(context.Context) ([]autoSearchParams, error)) {
		huma.Register(api, huma.Operation{
			OperationID:   "wanted-" + kind + "-search",
			Method:        http.MethodPost,
			Path:          path,
			Summary:       summary,
			Tags:          []string{"Wanted"},
			DefaultStatus: http.StatusAccepted,
		}, func(ctx context.Context, _ *struct{}) (*wantedSearchOutput, error) {
			ts, err := targets(ctx)
			if err != nil {
				return nil, huma.NewError(http.StatusInternalServerError, "failed to list episodes to search", err)
			}
			if !runner.start(kind, ts) {
				return nil, huma.Error409Conflict("a search of " + kind + " episodes is already running")
			}
			out := &wantedSearchOutput{}
			out.Body.Queued = len(ts)
			out.Body.Message = fmt.Sprintf("Searching %d episodes in the background. Grabs appear in the queue as they're found.", len(ts))
			if len(ts) == 0 {
				out.Body.Message = "Nothing to search."
			}
			return out, nil
		})
	}

	register("missing", "/api/v1/wanted/missing/search", "Search all missing episodes",
		func(ctx context.Context) ([]autoSearchParams, error) { return missingTargets(ctx, q) })
	register("cutoff", "/api/v1/wanted/cutoff/search", "Search upgrades for all cutoff-unmet episodes",
		func(ctx context.Context) ([]autoSearchParams, error) {
			if qualitySvc == nil {
				return nil, nil
			}
			eps, err := wanted.CutoffUnmet(ctx, q, qualitySvc)
			if err != nil {
				return nil, err
			}
			ts := make([]autoSearchParams, 0, len(eps))
			for _, ce := range eps {
				ts = append(ts, episodeTarget(ce.Episode.SeriesID, ce.Episode.ID, ce.Episode.SeasonNumber, ce.Episode.EpisodeNumber))
			}
			return capTargets(ts), nil
		})
}

// missingTargets lists aired, monitored, file-less episodes of monitored
// series. (The missing-episodes query doesn't check the series' own
// monitored flag, so it is applied here.)
func missingTargets(ctx context.Context, q db.Querier) ([]autoSearchParams, error) {
	series, err := q.ListMonitoredSeries(ctx)
	if err != nil {
		return nil, err
	}
	monitored := make(map[string]bool, len(series))
	for _, s := range series {
		monitored[s.ID] = true
	}
	rows, err := q.ListMissingEpisodesWithSeries(ctx, db.ListMissingEpisodesWithSeriesParams{Limit: maxBulkSearch * 4, Offset: 0})
	if err != nil {
		return nil, err
	}
	ts := make([]autoSearchParams, 0, len(rows))
	for _, r := range rows {
		if monitored[r.SeriesID] {
			ts = append(ts, episodeTarget(r.SeriesID, r.ID, r.SeasonNumber, r.EpisodeNumber))
		}
	}
	return capTargets(ts), nil
}

func episodeTarget(seriesID, episodeID string, season, episode int64) autoSearchParams {
	return autoSearchParams{
		SeriesID:  seriesID,
		EpisodeID: episodeID,
		Season:    int(season),
		Episode:   int(episode),
		Source:    "auto_search",
	}
}

func capTargets(ts []autoSearchParams) []autoSearchParams {
	if len(ts) > maxBulkSearch {
		return ts[:maxBulkSearch]
	}
	return ts
}

// wantedSearchRunner runs at most one bulk search per kind at a time.
type wantedSearchRunner struct {
	mu      sync.Mutex
	running map[string]bool
	search  func(ctx context.Context, p autoSearchParams) (*autoSearchResultBody, error)
	logger  *slog.Logger
	done    func(kind string) // test hook; nil in production
}

func newWantedSearchRunner(search func(context.Context, autoSearchParams) (*autoSearchResultBody, error), logger *slog.Logger) *wantedSearchRunner {
	return &wantedSearchRunner{running: make(map[string]bool), search: search, logger: logger}
}

// start launches a background search over targets. It returns false if a
// search of the same kind is still running.
func (r *wantedSearchRunner) start(kind string, targets []autoSearchParams) bool {
	r.mu.Lock()
	if r.running[kind] {
		r.mu.Unlock()
		return false
	}
	r.running[kind] = true
	r.mu.Unlock()

	go func() {
		defer func() {
			r.mu.Lock()
			delete(r.running, kind)
			r.mu.Unlock()
			if r.done != nil {
				r.done(kind)
			}
		}()
		ctx := context.Background()
		grabbed, failed := 0, 0
		r.logger.Info("wanted search started", "kind", kind, "episodes", len(targets))
		for _, t := range targets {
			res, err := r.search(ctx, t)
			switch {
			case err != nil:
				failed++
				r.logger.Warn("wanted search: episode search failed",
					"kind", kind, "series_id", t.SeriesID, "season", t.Season, "episode", t.Episode, "error", err)
			case res != nil && res.Result == "grabbed":
				grabbed++
			}
		}
		r.logger.Info("wanted search finished", "kind", kind, "episodes", len(targets), "grabbed", grabbed, "failed", failed)
	}()
	return true
}
