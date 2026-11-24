package indexer

import (
	"math"

	"github.com/beacon-stack/pilot/internal/parser"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

// PackType classifies a release by what it covers: a full season, multiple
// episodes, a single episode, or unknown. Populated from parser.EpisodeInfo.
type PackType string

const (
	PackTypeUnknown PackType = ""
	PackTypeSeason  PackType = "season"
	PackTypeMulti   PackType = "multi_episode"
	PackTypeEpisode PackType = "episode"
)

// classifyRelease parses a release title into a pack type and episode count.
// Season packs get a sentinel-large effective episode count in the sort
// comparator (see effectiveEpisodeCount) so they beat partial packs and
// individual episodes within the same quality tier, matching Sonarr's
// "Episode Count" ranking criterion.
func classifyRelease(title string) (PackType, int) {
	p := parser.Parse(title)
	if p.EpisodeInfo.IsSeasonPack {
		return PackTypeSeason, 0
	}
	if n := len(p.EpisodeInfo.Episodes); n > 1 {
		return PackTypeMulti, n
	}
	if len(p.EpisodeInfo.Episodes) == 1 {
		return PackTypeEpisode, 1
	}
	return PackTypeUnknown, 0
}

// newSearchResult builds the SearchResult for one release returned by an
// indexer. Indexer plugins don't parse quality, so it is parsed here from
// the title; ranking and grab history both depend on it.
func newSearchResult(r plugin.Release, indexerID, indexerName string) SearchResult {
	if r.Indexer == "" {
		r.Indexer = indexerName
	}
	parsed := parser.Parse(r.Title)
	r.Quality = withParsedQuality(parsed, r.Quality)
	pt, count := classifyRelease(r.Title)
	langs := make([]string, 0, len(parsed.Languages))
	for _, l := range parsed.Languages {
		langs = append(langs, string(l))
	}
	return SearchResult{
		Release:      r,
		IndexerID:    indexerID,
		IndexerName:  indexerName,
		QualityScore: r.Quality.Score(),
		PackType:     pt,
		EpisodeCount: count,
		Languages:    langs,
	}
}

// withParsedQuality fills q from the release title when the indexer gave
// no quality, and always fills the revision (PROPER/REPACK) if missing.
func withParsedQuality(rp parser.ParsedRelease, q plugin.Quality) plugin.Quality {
	parsed := rp.Quality()
	if q.Resolution == "" && q.Source == "" {
		return parsed
	}
	if q.Revision == 0 {
		q.Revision = parsed.Revision
	}
	return q
}

// RanksAbove reports whether a ranks above b under the search ranking.
// RSS sync uses it to pick the best of several releases for one episode.
func RanksAbove(a, b SearchResult) bool { return searchResultLess(a, b) }

// searchResultLess is the ranking comparator for search results. Tiers, in
// order:
//  1. Quality score   (1080p > 720p > 480p). Quality trumps all.
//  2. Revision        (PROPER/REPACK beats the original of the same quality)
//  3. Episode count   (season pack > multi-episode > single episode)
//  4. Seed weight     (age-aware log10 bucket; resists stale seed inflation)
//  5. Age             (newer wins when everything else is equal)
//
// The Episode Count tier matches Sonarr's DownloadDecisionComparer and is
// the reason season packs surface above individual episodes within the
// same quality tier in interactive season searches. Revision sits right
// after quality because Sonarr treats it as part of quality. See
// seedWeight() and effectiveEpisodeCount() for the other tiers.
func searchResultLess(a, b SearchResult) bool {
	if a.QualityScore != b.QualityScore {
		return a.QualityScore > b.QualityScore
	}
	if ra, rb := a.Quality.EffectiveRevision(), b.Quality.EffectiveRevision(); ra != rb {
		return ra > rb
	}
	if ea, eb := effectiveEpisodeCount(a), effectiveEpisodeCount(b); ea != eb {
		return ea > eb
	}
	if wa, wb := seedWeight(a.Seeds, a.AgeDays), seedWeight(b.Seeds, b.AgeDays); wa != wb {
		return wa > wb
	}
	// Final tiebreaker: prefer newer releases. For same-quality same-seed-bucket
	// candidates, newer is more likely to have live peers.
	return a.AgeDays < b.AgeDays
}

// effectiveEpisodeCount returns a comparable number for the sort comparator.
// Season packs return a large sentinel so any full pack outranks a partial
// pack of the same quality. Multi-episode releases return their actual count.
// Single episodes return 1. Unknown returns 0.
func effectiveEpisodeCount(r SearchResult) int {
	switch r.PackType {
	case PackTypeSeason:
		return 999
	case PackTypeMulti:
		if r.EpisodeCount > 0 {
			return r.EpisodeCount
		}
		return 2
	case PackTypeEpisode:
		return 1
	default:
		return 0
	}
}

// seedWeight returns a compressed, age-aware score for the seed count used
// as a ranking tiebreaker after quality.
//
// Public indexers frequently report stale or inflated seeder counts — a
// 5-year-old dead torrent can show "847 seeders" because the number hasn't
// been refreshed since it was healthy. Treating seed count linearly lets
// those inflated numbers dominate rankings. Sonarr's DownloadDecisionComparer
// uses round(log10(seeders)) for the same reason: it compresses the range
// so a claim of 847 lives in the same bucket as a real 30, eliminating the
// incentive for the top-ranked result to win on fake numbers alone.
//
// Buckets:
//
//	0 seeders       → 0
//	1 seeder        → 0
//	2-3 seeders     → 0  (log10 < 0.5 rounds to 0)
//	4-31 seeders    → 1
//	32-316 seeders  → 2
//	317+ seeders    → 3+
//
// For releases older than 1 year, the bucket is capped at 1: an 847-claim
// on a 5-year-old release ranks no higher than a 5-seeder release of the
// same vintage, because both numbers are stale beyond useful.
func seedWeight(seeds int, ageDays float64) int {
	if seeds <= 0 {
		return 0
	}
	bucket := int(math.Round(math.Log10(float64(seeds))))
	if ageDays > 365 && bucket > 1 {
		return 1
	}
	return bucket
}
