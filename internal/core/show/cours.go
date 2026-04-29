package show

import (
	"context"
	"fmt"
	"sort"
	"time"

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
	// TMDBSeason is the underlying TMDB season this cour pulls episodes
	// from. Almost always 1 for multi-cour anime; 0 for the specials
	// bucket. The UI uses this to fetch the correct season's episode
	// list before filtering down to the cour's window.
	TMDBSeason int
	// EpisodeOffset is the count of TMDB episodes that sit before this
	// cour within the same TMDBSeason. The UI subtracts this from each
	// TMDB-relative episode number to display cour-relative numbers
	// ("3x01" instead of "3x48"). Always 0 for specials and for cour 1.
	EpisodeOffset int
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
	// TMDBSeason=0, EpisodeOffset=0 — specials use their own number
	// space starting at 1.
	if specials, ok := bySeason[0]; ok && len(specials) > 0 {
		c := courFromEpisodes(0, "Specials", specials, sizeByEpisode, overrideByCour, parentMonitored)
		c.TMDBSeason = 0
		c.EpisodeOffset = 0
		out = append(out, c)
	}

	// Compute each cour's effective TMDB section. Some cours declare a
	// TMDB season that doesn't exist in our data (e.g. Jujutsu Kaisen's
	// cour 2 declares tmdbseason=2 but TMDB collapses every cour into
	// Season 1) — those get folded into a sibling cour's TMDB season,
	// with the boundary inferred from large air-date gaps inside the
	// shared section. See resolveCourBounds for the full algorithm.
	resolved := resolveCourBounds(bounds, bySeason)
	for _, r := range resolved {
		c := courFromEpisodes(r.Bound.TVDBSeason, r.Bound.Name, r.Episodes, sizeByEpisode, overrideByCour, parentMonitored)
		c.TVDBSeason = r.Bound.TVDBSeason // keep the cour identifier even when slice is empty
		c.TMDBSeason = r.TMDBSeason
		c.EpisodeOffset = r.EpisodeOffset
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

// resolvedCour is a CourBound that has been mapped to its actual
// position in the TMDB layout — useful when the declared TMDB season
// disagrees with what TMDB serves (the JJK cour-2 case).
type resolvedCour struct {
	Bound         CourBound
	TMDBSeason    int          // effective TMDB season (may differ from declared)
	EpisodeOffset int          // count of preceding TMDB episodes within TMDBSeason
	Episodes      []db.Episode // episodes that belong to this cour, in order
}

// resolveCourBounds maps each declared cour onto its actual TMDB
// position. Cours whose declared TMDB season has episodes pass
// through unchanged; cours whose season is empty (Anime-Lists XML
// disagrees with what TMDB serves) get folded into a neighbouring
// cour's TMDB season, with the boundary inferred from the largest
// air-date gap inside the shared section.
//
// JJK case:
//   - cour 1: declared (s=1, off=0), passes through
//   - cour 2: declared (s=2, off=0), TMDB has no S2 → folded into S1
//   - cour 3: declared (s=1, off=47), passes through
//
// Within TMDB Season 1 we now have 3 cours with one explicit boundary
// at episode 47. The region [1..47] holds 2 cours (1 and 2) so we
// look for one air-date gap inside it. JJK eps 24→25 span Mar 2021
// to Jul 2023 — that gap becomes the cour-1 / cour-2 boundary.
//
// Cours that can't be folded (no sibling has episodes) are dropped
// so the caller produces no phantom "0 episodes" cards.
func resolveCourBounds(bounds []CourBound, bySeason map[int][]db.Episode) []resolvedCour {
	if len(bounds) == 0 {
		return nil
	}

	// Step 1 — pick an effective TMDB season per cour. The declared
	// season wins when TMDB has episodes there; otherwise borrow the
	// nearest sibling's season (preferring the previous cour, then
	// the next cour, in defaulttvdbseason order).
	effSeason := make([]int, len(bounds))
	for i, b := range bounds {
		if len(bySeason[b.TMDBSeason]) > 0 {
			effSeason[i] = b.TMDBSeason
		} else {
			effSeason[i] = -1 // marker: needs a home
		}
	}
	for i := range bounds {
		if effSeason[i] != -1 {
			continue
		}
		donor := -1
		for j := i - 1; j >= 0; j-- {
			if effSeason[j] >= 0 {
				donor = effSeason[j]
				break
			}
		}
		if donor == -1 {
			for j := i + 1; j < len(bounds); j++ {
				if effSeason[j] >= 0 {
					donor = effSeason[j]
					break
				}
			}
		}
		effSeason[i] = donor // -1 stays for orphans
	}

	// Step 2 — group cour indices by effective TMDB season, in input
	// order (which is defaulttvdbseason-ascending). Drop orphans.
	type group struct {
		season  int
		courIxs []int
	}
	groups := make(map[int]*group)
	var groupOrder []int
	for i, s := range effSeason {
		if s < 0 {
			continue
		}
		g, ok := groups[s]
		if !ok {
			g = &group{season: s}
			groups[s] = g
			groupOrder = append(groupOrder, s)
		}
		g.courIxs = append(g.courIxs, i)
	}

	out := make([]resolvedCour, 0, len(bounds))
	// Stable iteration: groups in the order their first cour appeared,
	// which matches input order (= defaulttvdbseason ascending).
	for _, season := range groupOrder {
		g := groups[season]
		eps := bySeason[season]
		windows := computeCourWindows(g.courIxs, bounds, eps)
		for k, ix := range g.courIxs {
			b := bounds[ix]
			startEp, endEp := windows[k][0], windows[k][1]
			var slice []db.Episode
			for _, ep := range eps {
				n := int(ep.EpisodeNumber)
				if n >= startEp && n <= endEp {
					slice = append(slice, ep)
				}
			}
			out = append(out, resolvedCour{
				Bound:         b,
				TMDBSeason:    season,
				EpisodeOffset: startEp - 1,
				Episodes:      slice,
			})
		}
	}
	return out
}

// computeCourWindows returns each cour's [startEp, endEp] window
// (1-based, inclusive of both endpoints) within the TMDB season
// represented by `eps`. `courIxs` are indices into `bounds` for the
// cours that share this TMDB season, in defaulttvdbseason order.
//
// Algorithm:
//   - A cour is "anchored" when its declared TMDB season equals the
//     season we're laying out — its declared offset is then a known
//     boundary. The first cour in the group is implicitly anchored
//     at offset 0 even when it wasn't declared in this season.
//   - Folded cours (declared season != current season) have no
//     explicit anchor. Their boundaries fall inside the segment
//     between two anchored cours.
//   - For each segment with K cours and 2 outer anchors, we need
//     K-1 implicit boundaries — find them as the largest air-date
//     gaps among the segment's episodes.
func computeCourWindows(courIxs []int, bounds []CourBound, eps []db.Episode) [][2]int {
	n := len(courIxs)
	windows := make([][2]int, n)
	if n == 0 {
		return windows
	}

	groupSeason := 0
	if len(eps) > 0 {
		groupSeason = int(eps[0].SeasonNumber)
	}

	hasAnchor := make([]bool, n)
	anchorOffset := make([]int, n)
	for k, ix := range courIxs {
		b := bounds[ix]
		if b.TMDBSeason == groupSeason {
			hasAnchor[k] = true
			anchorOffset[k] = b.TMDBOffset
		}
	}
	// First cour always anchors at offset 0 (start of the season).
	if !hasAnchor[0] {
		hasAnchor[0] = true
		anchorOffset[0] = 0
	}

	endOfSeason := len(eps)

	var anchors []int
	for k := 0; k < n; k++ {
		if hasAnchor[k] {
			anchors = append(anchors, k)
		}
	}

	for a := 0; a < len(anchors); a++ {
		startK := anchors[a]
		startOff := anchorOffset[startK]
		endK := n
		endOff := endOfSeason
		if a+1 < len(anchors) {
			endK = anchors[a+1]
			endOff = anchorOffset[endK]
		}
		segCount := endK - startK
		if segCount <= 0 {
			continue
		}
		// Cours [startK, endK) share episodes (startOff, endOff].
		var segEps []db.Episode
		for _, ep := range eps {
			n2 := int(ep.EpisodeNumber)
			if n2 > startOff && n2 <= endOff {
				segEps = append(segEps, ep)
			}
		}
		boundaries := largestGapBoundaries(segEps, segCount-1)
		// Combine outer offsets with the implicit gap boundaries to
		// build segCount+1 partition points.
		bnds := make([]int, 0, segCount+1)
		bnds = append(bnds, startOff)
		bnds = append(bnds, boundaries...)
		bnds = append(bnds, endOff)
		for k := 0; k < segCount; k++ {
			windows[startK+k] = [2]int{bnds[k] + 1, bnds[k+1]}
		}
	}

	return windows
}

// largestGapBoundaries returns up to n boundary episode-numbers, one
// per "gap" between consecutive episodes' air dates. Sorted ascending
// in episode-number order, so callers get a partition-friendly list.
//
// boundary[i] = the LAST episode number before the i-th boundary
// (i.e., the gap is between boundary[i] and boundary[i]+1).
//
// When the episode list has fewer parseable air-date gaps than n
// (e.g., back-to-back airing, or missing AirDate values), we fall
// back to evenly-spaced boundaries so the caller still produces n+1
// non-empty cours.
func largestGapBoundaries(episodes []db.Episode, n int) []int {
	if n <= 0 || len(episodes) <= 1 {
		return nil
	}

	type gap struct {
		days     float64
		beforeEp int
	}
	var gaps []gap
	for i := 1; i < len(episodes); i++ {
		prev, curr := episodes[i-1], episodes[i]
		if !prev.AirDate.Valid || !curr.AirDate.Valid {
			continue
		}
		prevDate, errP := time.Parse("2006-01-02", prev.AirDate.String)
		currDate, errC := time.Parse("2006-01-02", curr.AirDate.String)
		if errP != nil || errC != nil {
			continue
		}
		gaps = append(gaps, gap{
			days:     currDate.Sub(prevDate).Hours() / 24,
			beforeEp: int(prev.EpisodeNumber),
		})
	}

	if len(gaps) == 0 {
		// No air dates available — fall back to evenly-spaced
		// boundaries within the episode-number range.
		return evenBoundaries(episodes, n)
	}

	// Take the n largest gaps.
	sort.Slice(gaps, func(i, j int) bool { return gaps[i].days > gaps[j].days })
	if len(gaps) > n {
		gaps = gaps[:n]
	}

	// If we found fewer gaps than requested, top up with even spacing
	// so the partition still produces n+1 cours.
	if len(gaps) < n {
		extra := evenBoundaries(episodes, n-len(gaps))
		for _, b := range extra {
			gaps = append(gaps, gap{beforeEp: b})
		}
	}

	out := make([]int, len(gaps))
	for i, g := range gaps {
		out[i] = g.beforeEp
	}
	sort.Ints(out)
	return out
}

// evenBoundaries returns n boundary episode-numbers evenly spaced
// across `episodes`. Used as the fallback when air-date data is
// unavailable for gap detection.
func evenBoundaries(episodes []db.Episode, n int) []int {
	if n <= 0 || len(episodes) <= 1 {
		return nil
	}
	out := make([]int, 0, n)
	step := float64(len(episodes)) / float64(n+1)
	for i := 1; i <= n; i++ {
		idx := int(float64(i) * step)
		if idx >= len(episodes) {
			idx = len(episodes) - 1
		}
		out = append(out, int(episodes[idx-1].EpisodeNumber))
	}
	return out
}
