package show

import (
	"context"
	"fmt"
	"sort"

	db "github.com/beacon-stack/pilot/internal/db/generated"
)

// Cour is a presentation-layer view of a multi-cour anime "season" —
// what the user expects to see as Season 1/2/3 of Jujutsu Kaisen even
// though TMDB serves all 59 episodes as a single Season 1. The
// underlying episodes table is not reshaped: this is computed at
// read time from the Anime-Lists XML mapping plus the existing
// episodes rows.
type Cour struct {
	// TVDBSeason is the cour identifier (1, 2, 3 …) sourced from
	// Anime-Lists' defaulttvdbseason attribute. The frontend renders
	// this as the season number ("Season 3").
	TVDBSeason int
	// Name is the human-readable cour title from AniDB ("Jujutsu Kaisen
	// Shimetsu Kaiyuu - Zenpen") when available; falls back to a
	// generic "Season N" string in the API layer.
	Name string
	// Monitored reflects either the user's explicit cour-monitor
	// override (anime_cour_monitored row) or the parent TMDB season's
	// monitored bit when no override exists.
	Monitored bool
	// EpisodeCount is the number of episodes in this cour.
	EpisodeCount int64
	// EpisodeFileCount is the number of episodes in this cour with a
	// linked file on disk.
	EpisodeFileCount int64
	// TotalSizeBytes is the sum of episode-file sizes for this cour.
	TotalSizeBytes int64
	// EpisodeIDs lists the show.Episode IDs that belong to this cour,
	// in TMDB-relative episode order. Useful for the Episodes endpoint
	// to filter to a single cour without having to recompute bounds.
	EpisodeIDs []string
}

// GetCours returns a cour-shaped projection of the series, suitable for
// display when series_type is anime. Returns (nil, nil) for non-anime
// series and for anime series with no Anime-Lists mapping — callers
// should fall back to GetSeasons in those cases.
//
// Specials (TMDB Season 0) are always returned as their own bucket
// with TVDBSeason=0, regardless of cour structure, because they don't
// participate in the cour layout.
//
// Implementation: bucket episodes by (tmdb_season, episode_number)
// against the cour bounds derived from the Anime-Lists XML. The bound
// for cour N within a TMDB season is `[TMDBOffset[N]+1, TMDBOffset[N+1]]`;
// the last cour's upper bound is the season's episode count.
func (s *Service) GetCours(ctx context.Context, seriesID string) ([]Cour, error) {
	series, err := s.Get(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	if series.SeriesType != "anime" || s.anime == nil {
		return nil, nil
	}

	bounds := s.anime.CourBounds(series.TMDBID)
	if len(bounds) == 0 {
		return nil, nil
	}

	episodes, err := s.q.ListEpisodesBySeriesID(ctx, seriesID)
	if err != nil {
		return nil, fmt.Errorf("list episodes: %w", err)
	}
	files, err := s.q.ListEpisodeFilesBySeriesID(ctx, seriesID)
	if err != nil {
		return nil, fmt.Errorf("list episode files: %w", err)
	}
	sizeByEpisode := make(map[string]int64, len(files))
	for _, f := range files {
		sizeByEpisode[f.EpisodeID] += f.SizeBytes
	}

	overrides, err := s.q.ListAnimeCourMonitored(ctx, seriesID)
	if err != nil {
		return nil, fmt.Errorf("list cour monitor overrides: %w", err)
	}
	overrideByCour := make(map[int]bool, len(overrides))
	for _, o := range overrides {
		overrideByCour[int(o.TvdbSeason)] = o.Monitored
	}

	parentMonitored, err := s.parentSeasonMonitoredByNumber(ctx, seriesID)
	if err != nil {
		return nil, err
	}

	cours := buildCours(bounds, episodes, sizeByEpisode, overrideByCour, parentMonitored)
	return cours, nil
}

// parentSeasonMonitoredByNumber maps season_number → monitored for the
// underlying TMDB seasons. Cours inherit the parent TMDB season's
// monitored bit when the user hasn't set an explicit override.
func (s *Service) parentSeasonMonitoredByNumber(ctx context.Context, seriesID string) (map[int]bool, error) {
	rows, err := s.q.ListSeasonsBySeriesID(ctx, seriesID)
	if err != nil {
		return nil, fmt.Errorf("list seasons: %w", err)
	}
	out := make(map[int]bool, len(rows))
	for _, r := range rows {
		out[int(r.SeasonNumber)] = r.Monitored
	}
	return out, nil
}

// buildCours is the pure function that turns (bounds, episodes, sizes,
// overrides, parent-monitored) into a cour list. Pulled out from the
// service method for unit-testability — no DB, no context.
//
// `bounds` must be sorted ascending by TVDBSeason (the Service's
// CourBounds contract guarantees this).
func buildCours(
	bounds []CourBound,
	episodes []db.Episode,
	sizeByEpisode map[string]int64,
	overrideByCour map[int]bool,
	parentMonitored map[int]bool,
) []Cour {
	// Group episodes by TMDB season number, sorted ascending by
	// episode number within each season. This is the canonical layout
	// the cour bounds reference.
	bySeason := make(map[int][]db.Episode)
	for _, ep := range episodes {
		bySeason[int(ep.SeasonNumber)] = append(bySeason[int(ep.SeasonNumber)], ep)
	}
	for season, eps := range bySeason {
		sort.Slice(eps, func(i, j int) bool {
			return eps[i].EpisodeNumber < eps[j].EpisodeNumber
		})
		bySeason[season] = eps
	}

	out := make([]Cour, 0, len(bounds)+1)

	// Specials always come first (TVDBSeason=0). They don't participate
	// in cour layout — surface them as their own bucket if present.
	if specials, ok := bySeason[0]; ok && len(specials) > 0 {
		out = append(out, courFromEpisodes(0, "Specials", specials, sizeByEpisode, overrideByCour, parentMonitored))
	}

	// For each cour, compute its episode-number window within the
	// cour's TMDB season and slice the season's episode list.
	for i, b := range bounds {
		seasonEps := bySeason[b.TMDBSeason]
		if len(seasonEps) == 0 {
			// The cour's declared TMDB season has no episodes at all
			// in our DB. Two ways this happens:
			//   - The TMDB record genuinely has no such season (the
			//     XML disagrees with TMDB's layout — Jujutsu Kaisen
			//     cour 2 declares tmdbseason=2, but TMDB folds all
			//     cours into Season 1).
			//   - We haven't refreshed the series yet to pull that
			//     season's episodes.
			// In either case, surfacing an empty cour gives the user
			// a phantom "0 episodes" card, which is worse UX than
			// just hiding it. Skip.
			continue
		}
		// Window: [TMDBOffset+1, next-bound's TMDBOffset OR end-of-season]
		startEp := b.TMDBOffset + 1
		endEp := len(seasonEps)
		// Look ahead for the next cour in the SAME TMDB season —
		// that's where this cour ends.
		for j := i + 1; j < len(bounds); j++ {
			if bounds[j].TMDBSeason == b.TMDBSeason {
				endEp = bounds[j].TMDBOffset
				break
			}
		}
		var slice []db.Episode
		for _, ep := range seasonEps {
			n := int(ep.EpisodeNumber)
			if n >= startEp && n <= endEp {
				slice = append(slice, ep)
			}
		}
		c := courFromEpisodes(b.TVDBSeason, b.Name, slice, sizeByEpisode, overrideByCour, parentMonitored)
		c.TVDBSeason = b.TVDBSeason // keep the cour identifier even when slice is empty
		out = append(out, c)
	}

	return out
}

// courFromEpisodes assembles a Cour from a sliced episode list plus
// the size/monitor lookups. Pulled out so the caller doesn't repeat
// the same accounting twice.
func courFromEpisodes(
	tvdbSeason int,
	name string,
	episodes []db.Episode,
	sizeByEpisode map[string]int64,
	overrideByCour map[int]bool,
	parentMonitored map[int]bool,
) Cour {
	c := Cour{
		TVDBSeason: tvdbSeason,
		Name:       name,
		EpisodeIDs: make([]string, 0, len(episodes)),
	}
	tmdbSeason := -1
	for _, ep := range episodes {
		c.EpisodeCount++
		if ep.HasFile {
			c.EpisodeFileCount++
		}
		c.TotalSizeBytes += sizeByEpisode[ep.ID]
		c.EpisodeIDs = append(c.EpisodeIDs, ep.ID)
		if tmdbSeason == -1 {
			tmdbSeason = int(ep.SeasonNumber)
		}
	}
	if tmdbSeason == -1 {
		tmdbSeason = 0 // empty cour fallback; resolveMonitored treats unknown as default
	}
	c.Monitored = resolveMonitored(tvdbSeason, tmdbSeason, overrideByCour, parentMonitored)
	return c
}

// resolveMonitored picks the cour's monitored bit. Explicit override
// wins; otherwise inherit the parent TMDB season's monitored bit;
// otherwise default to true (unknown season → monitored, matching the
// new-series default elsewhere in the codebase).
func resolveMonitored(tvdbSeason, tmdbSeason int, overrideByCour map[int]bool, parentMonitored map[int]bool) bool {
	if v, ok := overrideByCour[tvdbSeason]; ok {
		return v
	}
	if v, ok := parentMonitored[tmdbSeason]; ok {
		return v
	}
	return true
}

// SetCourMonitored upserts the user's explicit cour-monitor override.
// The DB row exists from now on, so subsequent GetCours calls return
// `monitored` regardless of what the parent season is set to.
func (s *Service) SetCourMonitored(ctx context.Context, seriesID string, tvdbSeason int, monitored bool) error {
	if err := s.q.UpsertAnimeCourMonitored(ctx, db.UpsertAnimeCourMonitoredParams{
		SeriesID:   seriesID,
		TvdbSeason: int32(tvdbSeason),
		Monitored:  monitored,
	}); err != nil {
		return fmt.Errorf("upsert anime cour monitored: %w", err)
	}
	return nil
}
