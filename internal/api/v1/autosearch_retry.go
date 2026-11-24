package v1

import (
	"context"
	"log/slog"
	"strconv"
	"sync"

	"github.com/beacon-stack/pilot/internal/core/blocklist"
	"github.com/beacon-stack/pilot/internal/core/downloader"
	"github.com/beacon-stack/pilot/internal/core/indexer"
	"github.com/beacon-stack/pilot/internal/core/quality"
	"github.com/beacon-stack/pilot/internal/core/show"
	"github.com/beacon-stack/pilot/internal/events"
)

// NewAutoSearchRetryHandler returns an event-bus handler that performs the
// re-search requested by TypeAutoSearchRetry. The stall watcher and the
// failed-download handler publish that event after blocklisting a dead
// release; this handler searches again and grabs the next best release.
// The blocklist entry they wrote keeps the dead release out.
func NewAutoSearchRetryHandler(
	showSvc *show.Service,
	indexerSvc *indexer.Service,
	downloaderSvc *downloader.Service,
	blocklistSvc *blocklist.Service,
	qualitySvc *quality.Service,
	logger *slog.Logger,
) events.Handler {
	deps := autoSearchDeps{
		ShowSvc:       showSvc,
		IndexerSvc:    indexerSvc,
		DownloaderSvc: downloaderSvc,
		BlocklistSvc:  blocklistSvc,
		QualitySvc:    qualitySvc,
	}
	r := &autoSearchRetrier{
		logger: logger,
		lookupEpisode: func(ctx context.Context, id string) (int, int, error) {
			ep, err := showSvc.GetEpisode(ctx, id)
			if err != nil {
				return 0, 0, err
			}
			return ep.SeasonNumber, ep.EpisodeNumber, nil
		},
		search: func(ctx context.Context, p autoSearchParams) (*autoSearchResultBody, error) {
			return runAutoSearchAndGrab(ctx, deps, p)
		},
	}
	return r.handle
}

type autoSearchRetrier struct {
	logger        *slog.Logger
	lookupEpisode func(ctx context.Context, episodeID string) (season, episode int, err error)
	search        func(ctx context.Context, p autoSearchParams) (*autoSearchResultBody, error)
	inflight      sync.Map // target key -> struct{}; drops duplicate concurrent retries
}

func (r *autoSearchRetrier) handle(ctx context.Context, e events.Event) {
	if e.Type != events.TypeAutoSearchRetry {
		return
	}
	seriesID := eventString(e.Data, "series_id")
	episodeID := eventString(e.Data, "episode_id")
	if seriesID == "" {
		r.logger.Warn("auto-search retry: event has no series_id")
		return
	}

	p := autoSearchParams{SeriesID: seriesID, EpisodeID: episodeID, Source: "auto_search"}
	switch {
	case episodeID != "":
		season, episode, err := r.lookupEpisode(ctx, episodeID)
		if err != nil {
			r.logger.Warn("auto-search retry: episode lookup failed",
				"series_id", seriesID, "episode_id", episodeID, "error", err)
			return
		}
		p.Season, p.Episode = season, episode
	default:
		season, ok := eventInt(e.Data, "season")
		if !ok {
			// Season-pack grab with no season recorded on the event
			// (older stall-watcher events): nothing safe to search for.
			r.logger.Info("auto-search retry: no episode or season to search", "series_id", seriesID)
			return
		}
		p.Season = season
	}

	key := seriesID + "/" + strconv.Itoa(p.Season) + "/" + strconv.Itoa(p.Episode)
	if _, busy := r.inflight.LoadOrStore(key, struct{}{}); busy {
		return
	}
	defer r.inflight.Delete(key)

	res, err := r.search(ctx, p)
	if err != nil {
		r.logger.Warn("auto-search retry: search failed",
			"series_id", seriesID, "season", p.Season, "episode", p.Episode, "error", err)
		return
	}
	r.logger.Info("auto-search retry finished",
		"series_id", seriesID, "season", p.Season, "episode", p.Episode,
		"result", res.Result, "release", res.ReleaseTitle, "reason", res.Reason)
}

func eventString(data map[string]any, key string) string {
	if s, ok := data[key].(string); ok {
		return s
	}
	return ""
}

// eventInt reads an integer field; bus payloads are built in-process, so
// the value may be any Go integer type (or float64 after a JSON trip).
func eventInt(data map[string]any, key string) (int, bool) {
	switch v := data[key].(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case int32:
		return int(v), true
	case float64:
		return int(v), true
	}
	return 0, false
}
