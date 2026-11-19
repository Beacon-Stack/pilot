// Package wanted answers "which episodes still need something": episodes
// whose file is below their quality profile's cutoff, and the best
// quality an episode currently has on disk.
package wanted

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/beacon-stack/pilot/internal/core/quality"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

// ProfileGetter loads a quality profile by ID. quality.Service satisfies it.
type ProfileGetter interface {
	Get(ctx context.Context, id string) (quality.Profile, error)
}

// BestFileQuality returns the best recorded quality among the files that
// belong to episodeID, or nil when the episode has none. A file whose
// quality was never recorded yields an empty Quality (Score 0).
func BestFileQuality(files []db.EpisodeFile, episodeID string) *plugin.Quality {
	var best *plugin.Quality
	for _, f := range files {
		if f.EpisodeID != episodeID {
			continue
		}
		var fq plugin.Quality
		_ = json.Unmarshal([]byte(f.QualityJson), &fq)
		if best == nil || fq.BetterThan(*best) ||
			(fq.Score() == best.Score() && fq.EffectiveRevision() > best.EffectiveRevision()) {
			c := fq
			best = &c
		}
	}
	return best
}

// CutoffEpisode is a monitored episode whose file is below the cutoff.
type CutoffEpisode struct {
	Episode     db.Episode
	SeriesTitle string
	Current     plugin.Quality
	Cutoff      plugin.Quality
}

// CutoffUnmet lists monitored episodes, in monitored series, whose best
// file quality is below the series' quality profile cutoff. Sorted by
// series title, season, episode.
//
// Files of unknown quality (recorded before quality was tracked) are left
// out: automatic upgrades skip them (decision.ReasonCurrentUnknown), so
// listing them here would offer a search that does nothing.
func CutoffUnmet(ctx context.Context, q db.Querier, profiles ProfileGetter) ([]CutoffEpisode, error) {
	series, err := q.ListMonitoredSeries(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing monitored series: %w", err)
	}

	cutoffs := make(map[string]*plugin.Quality)
	var out []CutoffEpisode
	for _, s := range series {
		cutoff, ok := cutoffs[s.QualityProfileID]
		if !ok {
			cutoff = nil
			if p, err := profiles.Get(ctx, s.QualityProfileID); err == nil && p.Cutoff.Score() > 0 {
				c := p.Cutoff
				cutoff = &c
			}
			cutoffs[s.QualityProfileID] = cutoff
		}
		if cutoff == nil {
			continue // no profile, or no meaningful cutoff
		}

		files, err := q.ListEpisodeFilesBySeriesID(ctx, s.ID)
		if err != nil {
			return nil, fmt.Errorf("listing files for series %s: %w", s.ID, err)
		}
		if len(files) == 0 {
			continue
		}
		episodes, err := q.ListEpisodesBySeriesID(ctx, s.ID)
		if err != nil {
			return nil, fmt.Errorf("listing episodes for series %s: %w", s.ID, err)
		}
		for _, ep := range episodes {
			if !ep.Monitored || !ep.HasFile {
				continue
			}
			cur := BestFileQuality(files, ep.ID)
			if cur == nil || cur.Score() == 0 || cur.AtLeast(*cutoff) {
				continue
			}
			out = append(out, CutoffEpisode{Episode: ep, SeriesTitle: s.Title, Current: *cur, Cutoff: *cutoff})
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.SeriesTitle != b.SeriesTitle {
			return a.SeriesTitle < b.SeriesTitle
		}
		if a.Episode.SeasonNumber != b.Episode.SeasonNumber {
			return a.Episode.SeasonNumber < b.Episode.SeasonNumber
		}
		return a.Episode.EpisodeNumber < b.Episode.EpisodeNumber
	})
	return out, nil
}
