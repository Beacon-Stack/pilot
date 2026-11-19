// Package indexer manages indexer configurations and orchestrates release searches.
//
// ⚠ Before changing anything in this file — especially `seedWeight`,
// `applyMinSeedersFilter`, or the `Search()` sort/filter closure — run:
//
//	go test ./internal/core/indexer/...
//
// The tests in service_test.go lock down the dead-torrent regression fix:
// the 847-seeders-on-a-5-year-old-release bug would have been caught by
// `TestSearchSort_ReproducesIncident` if the suite had existed at the time.
// See pilot/CLAUDE.md "Regression guard: dead-torrent release search" for
// the full list of guarded files and the rationale.
package indexer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/beacon-stack/pilot/internal/core/dbutil"
	"github.com/beacon-stack/pilot/internal/core/language"
	"github.com/beacon-stack/pilot/internal/core/parser"
	"github.com/beacon-stack/pilot/internal/core/releaseprofile"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/events"
	releaseparser "github.com/beacon-stack/pilot/internal/parser"
	"github.com/beacon-stack/pilot/internal/ratelimit"
	"github.com/beacon-stack/pilot/internal/registry"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

// ErrNotFound is returned when an indexer config does not exist.
var ErrNotFound = errors.New("indexer not found")

// Config is the domain representation of a stored indexer configuration.
type Config struct {
	ID         string
	Name       string
	Kind       string // "torznab", "newznab"
	Enabled    bool
	Priority   int
	MinSeeders int // releases below this threshold get tagged with a filter reason
	Settings   json.RawMessage
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// CreateRequest carries the fields needed to create an indexer config.
type CreateRequest struct {
	Name       string
	Kind       string
	Enabled    bool
	Priority   int
	MinSeeders int // 0 means "use the sensible default (5)"
	Settings   json.RawMessage
}

// UpdateRequest carries the fields needed to update an indexer config.
type UpdateRequest = CreateRequest

// PackType classifies a release by what it covers: a full season, multiple
// episodes, a single episode, or unknown. Populated from parser.EpisodeInfo.
type PackType string

const (
	PackTypeUnknown PackType = ""
	PackTypeSeason  PackType = "season"
	PackTypeMulti   PackType = "multi_episode"
	PackTypeEpisode PackType = "episode"
)

// SearchResult pairs a plugin release with its source indexer identity.
type SearchResult struct {
	plugin.Release
	// IndexerID is the DB UUID of the indexer that returned this release.
	IndexerID    string
	IndexerName  string
	QualityScore int
	// PackType classifies the release as a season pack, multi-episode pack,
	// single episode, or unknown. Parsed from the release title once at
	// ingestion so the sort comparator and API response can share it.
	PackType PackType
	// EpisodeCount is how many discrete episodes the release covers. For
	// season packs it is sentinel-large (see effectiveEpisodeCount) so the
	// ranking comparator prefers full packs over partial packs within the
	// same quality tier.
	EpisodeCount int
	// Languages are the audio languages tagged in the title (parser
	// names, e.g. "french"). Empty when the title has no language tag.
	Languages []string
	// FilterReasons lists all the reasons this release was filtered out
	// of the "active" set. If non-empty, the UI should render the row
	// grayed with the reasons shown and an "override" button. Empty means
	// the release is in the active/top list. We return filtered rows
	// intentionally so users can see what was dropped and why — hiding
	// them creates "content loss traps" when false-positive blocklists
	// or overly-aggressive min_seeders thresholds silently eat content.
	FilterReasons []string
}

// GrabRequest carries the fields needed to record a grab history entry.
type GrabRequest struct {
	SeriesID     string
	EpisodeID    string // optional; empty for season-pack grabs
	SeasonNumber int    // optional; 0 when not known
	Release      plugin.Release
	IndexerID    string
	// Source is "interactive" (user-initiated via UI) or "auto_search"
	// (triggered by scheduled episode search). Controls whether the stall
	// watcher triggers automatic re-search on failure.
	Source string
}

// Service manages indexer configuration and search orchestration.
// maxConcurrentIndexerSearches bounds the number of in-flight indexer HTTP
// searches across the whole service. A season search issues several query
// variations, each fanning out to every enabled indexer, so without a global
// cap the outbound concurrency was queries×indexers — a thundering herd
// against providers and the local FD table. The semaphore is shared so the
// bound holds across concurrent Search/GetRecent calls, not just within one.
// 16 slots: a season search issues 2 query variants × ~10 indexers = 20
// tasks; at 8 slots the tail waited a full scrape-round before starting
// and blew the interactive budget. Pulse rate-limits per site anyway, so
// the extra concurrency doesn't hammer providers.
const maxConcurrentIndexerSearches = 16

// errSearchTimedOut replaces a raw context.DeadlineExceeded in
// user-facing per-indexer outcomes. Slow (Cloudflare-protected)
// indexers finish their scrape server-side after we disconnect and the
// result is cached — retrying is the fix, and the message should say so.
var errSearchTimedOut = errors.New("timed out — slow (Cloudflare-protected) indexers finish in the background; search again shortly for cached results")

// perIndexerSearchTimeout bounds how long a single indexer call may run (and
// thus hold a semaphore slot). It inherits any earlier deadline on the parent
// context, so a tight per-request budget still wins; this only stops one hung
// indexer from starving the pool when the caller passes a long-lived context.
const perIndexerSearchTimeout = 25 * time.Second

type Service struct {
	q      db.Querier
	reg    *registry.Registry
	bus    *events.Bus
	rl     *ratelimit.Registry
	logger *slog.Logger
	cache  sync.Map      // config ID → plugin.Indexer
	sem    chan struct{} // bounds concurrent indexer searches
}

// NewService creates a new Service. A nil logger falls back to slog.Default()
// so existing callers continue to work without migration.
func NewService(q db.Querier, reg *registry.Registry, bus *events.Bus, rl *ratelimit.Registry, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		q:      q,
		reg:    reg,
		bus:    bus,
		rl:     rl,
		logger: logger,
		sem:    make(chan struct{}, maxConcurrentIndexerSearches),
	}
}

// cachedIndexer returns a cached or freshly-created indexer for the given config.
func (s *Service) cachedIndexer(kind, id string, settings json.RawMessage) (plugin.Indexer, error) {
	if v, ok := s.cache.Load(id); ok {
		return v.(plugin.Indexer), nil
	}
	idx, err := s.reg.NewIndexer(kind, settings)
	if err != nil {
		return nil, err
	}
	actual, _ := s.cache.LoadOrStore(id, idx)
	return actual.(plugin.Indexer), nil
}

// evictIndexer removes a cached indexer instance.
func (s *Service) evictIndexer(id string) {
	s.cache.Delete(id)
}

// Create persists a new indexer configuration.
func (s *Service) Create(ctx context.Context, req CreateRequest) (Config, error) {
	settings := req.Settings
	if len(settings) == 0 {
		settings = json.RawMessage("{}")
	}
	// Validate that the kind is registered.
	if _, err := s.reg.NewIndexer(req.Kind, settings); err != nil {
		return Config{}, fmt.Errorf("invalid indexer kind or settings: %w", err)
	}

	priority := req.Priority
	if priority <= 0 {
		priority = 25
	}

	minSeeders := req.MinSeeders
	if minSeeders <= 0 {
		minSeeders = 5 // sensible default for public trackers
	}

	now := time.Now().UTC()
	row, err := s.q.CreateIndexerConfig(ctx, db.CreateIndexerConfigParams{
		ID:         uuid.New().String(),
		Name:       req.Name,
		Kind:       req.Kind,
		Enabled:    req.Enabled,
		Priority:   int64(priority),
		Settings:   string(settings),
		MinSeeders: int64(minSeeders),
		CreatedAt:  now.Format(time.RFC3339),
		UpdatedAt:  now.Format(time.RFC3339),
	})
	if err != nil {
		return Config{}, fmt.Errorf("inserting indexer config: %w", err)
	}

	return rowToConfig(row)
}

// Get returns an indexer config by ID. Returns ErrNotFound if absent.
func (s *Service) Get(ctx context.Context, id string) (Config, error) {
	row, err := s.q.GetIndexerConfig(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Config{}, ErrNotFound
		}
		return Config{}, fmt.Errorf("fetching indexer %q: %w", id, err)
	}
	return rowToConfig(row)
}

// List returns all indexer configs, ordered by priority then name.
func (s *Service) List(ctx context.Context) ([]Config, error) {
	rows, err := s.q.ListIndexerConfigs(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing indexer configs: %w", err)
	}
	configs := make([]Config, 0, len(rows))
	for _, row := range rows {
		cfg, err := rowToConfig(row)
		if err != nil {
			return nil, err
		}
		configs = append(configs, cfg)
	}
	return configs, nil
}

// Update replaces the mutable fields of an indexer config.
// Returns ErrNotFound if the indexer does not exist.
func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (Config, error) {
	existing, err := s.q.GetIndexerConfig(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Config{}, ErrNotFound
		}
		return Config{}, fmt.Errorf("fetching indexer %q for update: %w", id, err)
	}

	// Merge: keys absent from req.Settings are preserved from existing settings.
	// This ensures secret fields (API keys) are not erased when omitted by the client.
	settings := dbutil.MergeSettings(json.RawMessage(existing.Settings), req.Settings)
	if len(settings) == 0 {
		settings = json.RawMessage("{}")
	}

	priority := req.Priority
	if priority <= 0 {
		priority = 25
	}

	minSeeders := req.MinSeeders
	if minSeeders <= 0 {
		// Preserve existing value if the request didn't set one.
		minSeeders = int(existing.MinSeeders)
		if minSeeders <= 0 {
			minSeeders = 5
		}
	}

	row, err := s.q.UpdateIndexerConfig(ctx, db.UpdateIndexerConfigParams{
		ID:         id,
		Name:       req.Name,
		Kind:       req.Kind,
		Enabled:    req.Enabled,
		Priority:   int64(priority),
		Settings:   string(settings),
		MinSeeders: int64(minSeeders),
		UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return Config{}, fmt.Errorf("updating indexer %q: %w", id, err)
	}
	s.evictIndexer(id)
	return rowToConfig(row)
}

// Delete removes an indexer config. Returns ErrNotFound if absent.
func (s *Service) Delete(ctx context.Context, id string) error {
	if _, err := s.q.GetIndexerConfig(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("fetching indexer %q for delete: %w", id, err)
	}
	if err := s.q.DeleteIndexerConfig(ctx, id); err != nil {
		return fmt.Errorf("deleting indexer %q: %w", id, err)
	}
	s.rl.Remove(id)
	s.evictIndexer(id)
	return nil
}

// Test instantiates the indexer plugin and verifies connectivity.
func (s *Service) Test(ctx context.Context, id string) error {
	cfg, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.rl.Wait(ctx, cfg.ID, extractRateLimit(cfg.Settings)); err != nil {
		return fmt.Errorf("rate limit wait: %w", err)
	}
	idx, err := s.reg.NewIndexer(cfg.Kind, cfg.Settings)
	if err != nil {
		return fmt.Errorf("instantiating indexer plugin: %w", err)
	}
	return idx.Test(ctx)
}

// applyMinSeedersFilter tags each result whose seed count falls below the
// configured per-indexer minimum with a FilterReason string. Results are
// NOT dropped — the UI renders filtered rows grayed with an "override and
// grab anyway" button so users can recover false positives. Hiding rows
// creates content-loss traps.
//
// Freshness exception: releases less than 12 hours old AND confirmed on
// 2+ distinct indexers bypass the filter. Rationale: public tracker
// aggregators lag behind real seeder counts by minutes-to-hours on brand-
// new releases, so a just-uploaded hot torrent may legitimately show 0
// seeders on its first indexer hit. Requiring a second-indexer confirmation
// guards against a single bad source lying about the release being fresh.
//
// minByIndexer is the per-indexer threshold map. Missing / zero entries
// fall back to a sensible default of 5 — this matters for edge cases where
// an indexer exists in search results but not in the rows list (shouldn't
// happen in production, but tests may construct such cases).
// applyReleaseProfiles tags results that an enabled release profile
// rejects (a must-not-contain term, or none of the must-contain terms).
// Tagged rather than dropped, like min_seeders: interactive search shows
// them grayed with an override, auto-search and RSS skip them. If the
// profiles can't be loaded, nothing is tagged: a database hiccup must
// not empty every search.
func (s *Service) applyReleaseProfiles(ctx context.Context, results []SearchResult) {
	if s.q == nil || len(results) == 0 {
		return
	}
	m, err := releaseprofile.LoadMatcher(ctx, s.q)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("could not load release profiles; not applying them", "error", err)
		}
		return
	}
	applyReleaseProfiles(results, m)
}

// applyLanguagePreference tags results in no wanted language (see
// internal/core/language). Like release profiles: tagged, not dropped,
// and a load failure tags nothing.
func (s *Service) applyLanguagePreference(ctx context.Context, results []SearchResult) {
	if s.q == nil || len(results) == 0 {
		return
	}
	pref, err := language.NewStore(s.q).Get(ctx)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("could not load the language preference; not applying it", "error", err)
		}
		return
	}
	applyLanguagePreference(results, pref)
}

func applyLanguagePreference(results []SearchResult, pref language.Preference) {
	if len(pref.Languages) == 0 {
		return
	}
	for i := range results {
		langs := make([]releaseparser.Language, 0, len(results[i].Languages))
		for _, l := range results[i].Languages {
			langs = append(langs, releaseparser.Language(l))
		}
		if !pref.Accepts(langs) {
			results[i].FilterReasons = append(results[i].FilterReasons, language.FilterReason)
		}
	}
}

func applyReleaseProfiles(results []SearchResult, m *releaseprofile.Matcher) {
	for i := range results {
		if ok, _ := m.Check(results[i].Title, results[i].IndexerID); !ok {
			results[i].FilterReasons = append(results[i].FilterReasons, releaseprofile.FilterReason)
		}
	}
}

func applyMinSeedersFilter(results []SearchResult, minByIndexer map[string]int) {
	titleIndexers := make(map[string]map[string]bool)
	for _, r := range results {
		if titleIndexers[r.Title] == nil {
			titleIndexers[r.Title] = make(map[string]bool)
		}
		titleIndexers[r.Title][r.IndexerID] = true
	}

	for i := range results {
		r := &results[i]
		minSeeds := minByIndexer[r.IndexerID]
		if minSeeds <= 0 {
			minSeeds = 5
		}
		if r.Seeds < minSeeds {
			fresh := r.AgeDays < 0.5 && len(titleIndexers[r.Title]) >= 2
			if !fresh {
				r.FilterReasons = append(r.FilterReasons,
					fmt.Sprintf("below minimum seeders (%d < %d)", r.Seeds, minSeeds))
			}
		}
	}
}

// seedWeight returns a compressed, age-aware score for the seed count used
// as a ranking tiebreaker after quality.
//
// Public indexers frequently report stale or inflated seeder counts — a
// 5-year-old dead torrent can show "847 seeders" because the number hasn't
// been refreshed since it was healthy. Treating seed count linearly lets
// those inflated numbers dominate rankings, which is exactly the bug this
// function exists to defend against. Sonarr's DownloadDecisionComparer
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
// the title. Before this, every release kept an empty Quality: the
// "quality first" sort tier compared zeros, ranking fell through to seeds
// and age, and grabs recorded no quality.
func newSearchResult(r plugin.Release, indexerID, indexerName string) SearchResult {
	if r.Indexer == "" {
		r.Indexer = indexerName
	}
	parsed := releaseparser.Parse(r.Title)
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
func withParsedQuality(rp releaseparser.ParsedRelease, q plugin.Quality) plugin.Quality {
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

// Outcome summarizes one indexer's contribution to a search —
// the raw material for the per-indexer breakdown the UI shows instead of
// a bare "No releases found." This data was previously computed and
// thrown away, which made "half the fleet is broken" indistinguishable
// from "nothing matched".
type Outcome struct {
	IndexerID   string
	IndexerName string
	Results     int
	Error       string // empty on success
	// Pending means the indexer hit our deadline but its scrape continues
	// server-side (Pulse detaches Cloudflare-walled scrapes and caches
	// the result) — a retry shortly will be served from cache. NOT a
	// failure; the UI shows "still searching", not an error.
	Pending bool
}

// Search queries all enabled indexers with the given search query and returns
// results sorted by quality score, then an age-aware log10 seed weight, then
// release age. Errors from individual indexers are collected; a combined error
// is returned alongside any results that were gathered.
func (s *Service) Search(ctx context.Context, q plugin.SearchQuery) ([]SearchResult, error) {
	results, _, err := s.SearchWithOutcomes(ctx, q)
	return results, err
}

// SearchWithOutcomes is Search plus the per-indexer outcome list.
func (s *Service) SearchWithOutcomes(ctx context.Context, q plugin.SearchQuery) ([]SearchResult, []Outcome, error) {
	rows, err := s.q.ListEnabledIndexers(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("listing enabled indexers: %w", err)
	}

	if len(rows) == 0 {
		return nil, nil, nil
	}

	type indexerResult struct {
		indexerID   string
		indexerName string
		releases    []plugin.Release
		err         error
	}

	resultsCh := make(chan indexerResult, len(rows))
	var wg sync.WaitGroup

	for _, row := range rows {
		wg.Add(1)
		go func(row db.IndexerConfig) {
			defer wg.Done()
			cfg, _ := rowToConfig(row)

			// Bound global concurrency so queries×indexers can't all hit the
			// network at once; release the slot when this indexer finishes.
			if s.sem != nil {
				select {
				case s.sem <- struct{}{}:
					defer func() { <-s.sem }()
				case <-ctx.Done():
					resultsCh <- indexerResult{indexerID: cfg.ID, indexerName: cfg.Name, err: ctx.Err()}
					return
				}
			}

			// Per-indexer timeout so one hung provider can't hold its slot for
			// the whole parent budget. Inherits any earlier parent deadline.
			sctx, cancel := context.WithTimeout(ctx, perIndexerSearchTimeout)
			defer cancel()

			started := time.Now()
			if err := s.rl.Wait(sctx, cfg.ID, extractRateLimit(cfg.Settings)); err != nil {
				resultsCh <- indexerResult{indexerID: cfg.ID, indexerName: cfg.Name, err: err}
				return
			}
			idx, err := s.cachedIndexer(cfg.Kind, cfg.ID, cfg.Settings)
			if err != nil {
				resultsCh <- indexerResult{indexerID: cfg.ID, indexerName: cfg.Name, err: err}
				return
			}
			releases, err := idx.Search(sctx, q)
			elapsed := time.Since(started)
			// Per-indexer timing on every search so a slow indexer is
			// visible in `scripts/logs pilot | grep indexer.search`. Errors
			// log at warn level (typical: timeout, Cloudflare 403, dead host);
			// success at info.
			if err != nil {
				s.logger.Warn("indexer search failed",
					"indexer", cfg.Name,
					"kind", cfg.Kind,
					"query", q.Query,
					"duration_ms", elapsed.Milliseconds(),
					"error", err)
			} else {
				s.logger.Info("indexer search ok",
					"indexer", cfg.Name,
					"kind", cfg.Kind,
					"query", q.Query,
					"duration_ms", elapsed.Milliseconds(),
					"results", len(releases))
			}
			// A deadline hit is almost always a Cloudflare-walled indexer
			// mid-solve: Pulse keeps scraping after we hang up and caches
			// the result, so a retry shortly succeeds instantly. Say that
			// instead of a bare "context deadline exceeded".
			if err != nil && errors.Is(err, context.DeadlineExceeded) {
				err = errSearchTimedOut
			}
			resultsCh <- indexerResult{
				indexerID:   cfg.ID,
				indexerName: cfg.Name,
				releases:    releases,
				err:         err,
			}
		}(row)
	}

	wg.Wait()
	close(resultsCh)

	var allResults []SearchResult
	var errs []error
	var outcomes []Outcome

	for res := range resultsCh {
		if res.err != nil {
			// Deadline hits are "pending", not failures: the scrape
			// continues server-side and its result will be cached. Only
			// genuine errors (torznab <error> docs, connection failures)
			// count as failed.
			if errors.Is(res.err, errSearchTimedOut) {
				outcomes = append(outcomes, Outcome{
					IndexerID:   res.indexerID,
					IndexerName: res.indexerName,
					Pending:     true,
				})
				continue
			}
			errs = append(errs, fmt.Errorf("indexer %q: %w", res.indexerName, res.err))
			outcomes = append(outcomes, Outcome{
				IndexerID:   res.indexerID,
				IndexerName: res.indexerName,
				Error:       res.err.Error(),
			})
			continue
		}
		outcomes = append(outcomes, Outcome{
			IndexerID:   res.indexerID,
			IndexerName: res.indexerName,
			Results:     len(res.releases),
		})
		for _, r := range res.releases {
			allResults = append(allResults, newSearchResult(r, res.indexerID, res.indexerName))
		}
	}

	sort.Slice(allResults, func(i, j int) bool {
		return searchResultLess(allResults[i], allResults[j])
	})

	// Build per-indexer min_seeders lookup from the rows we already fetched.
	minByIndexer := make(map[string]int, len(rows))
	for _, r := range rows {
		minByIndexer[r.ID] = int(r.MinSeeders)
	}
	applyMinSeedersFilter(allResults, minByIndexer)
	s.applyReleaseProfiles(ctx, allResults)
	s.applyLanguagePreference(ctx, allResults)

	var combinedErr error
	if len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, e := range errs {
			msgs[i] = e.Error()
		}
		combinedErr = fmt.Errorf("%d indexer(s) failed: %v", len(errs), msgs)
	}

	return allResults, outcomes, combinedErr
}

// GetRecent fetches the most recent releases from all enabled indexers via
// their RSS feeds. Results from all indexers are merged and returned together.
// Errors from individual indexers are collected; a combined error is returned
// alongside any results that were gathered.
func (s *Service) GetRecent(ctx context.Context) ([]SearchResult, error) {
	rows, err := s.q.ListEnabledIndexers(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing enabled indexers: %w", err)
	}

	if len(rows) == 0 {
		return nil, nil
	}

	type indexerResult struct {
		indexerID   string
		indexerName string
		releases    []plugin.Release
		err         error
	}

	resultsCh := make(chan indexerResult, len(rows))
	var wg sync.WaitGroup

	for _, row := range rows {
		wg.Add(1)
		go func(row db.IndexerConfig) {
			defer wg.Done()
			cfg, _ := rowToConfig(row)

			// Same global concurrency bound as Search (see the per-indexer
			// goroutine there for rationale).
			if s.sem != nil {
				select {
				case s.sem <- struct{}{}:
					defer func() { <-s.sem }()
				case <-ctx.Done():
					resultsCh <- indexerResult{indexerID: cfg.ID, indexerName: cfg.Name, err: ctx.Err()}
					return
				}
			}

			sctx, cancel := context.WithTimeout(ctx, perIndexerSearchTimeout)
			defer cancel()

			if err := s.rl.Wait(sctx, cfg.ID, extractRateLimit(cfg.Settings)); err != nil {
				resultsCh <- indexerResult{indexerID: cfg.ID, indexerName: cfg.Name, err: err}
				return
			}
			idx, err := s.cachedIndexer(cfg.Kind, cfg.ID, cfg.Settings)
			if err != nil {
				resultsCh <- indexerResult{indexerID: cfg.ID, indexerName: cfg.Name, err: err}
				return
			}
			releases, err := idx.GetRecent(sctx)
			// A deadline hit is almost always a Cloudflare-walled indexer
			// mid-solve: Pulse keeps scraping after we hang up and caches
			// the result, so a retry shortly succeeds instantly. Say that
			// instead of a bare "context deadline exceeded".
			if err != nil && errors.Is(err, context.DeadlineExceeded) {
				err = errSearchTimedOut
			}
			resultsCh <- indexerResult{
				indexerID:   cfg.ID,
				indexerName: cfg.Name,
				releases:    releases,
				err:         err,
			}
		}(row)
	}

	wg.Wait()
	close(resultsCh)

	var allResults []SearchResult
	var errs []error

	for res := range resultsCh {
		if res.err != nil {
			errs = append(errs, fmt.Errorf("indexer %q: %w", res.indexerName, res.err))
			continue
		}
		for _, r := range res.releases {
			allResults = append(allResults, newSearchResult(r, res.indexerID, res.indexerName))
		}
	}

	var combinedErr error
	if len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, e := range errs {
			msgs[i] = e.Error()
		}
		combinedErr = fmt.Errorf("%d indexer(s) failed: %v", len(errs), msgs)
	}

	s.applyReleaseProfiles(ctx, allResults)
	s.applyLanguagePreference(ctx, allResults)

	return allResults, combinedErr
}

// GrabHistory returns the grab history for a series, newest first.
func (s *Service) GrabHistory(ctx context.Context, seriesID string) ([]db.GrabHistory, error) {
	return s.q.ListGrabHistoryBySeries(ctx, seriesID)
}

// CreateGrab records a grab history entry linking the release to the series
// and optionally to a specific episode. Pass empty strings for fields that are
// not applicable (e.g. season-pack grabs have no episodeID).
func (s *Service) CreateGrab(ctx context.Context, req GrabRequest) (db.GrabHistory, error) {
	var seasonNumber *int64
	if req.SeasonNumber > 0 {
		sn := int64(req.SeasonNumber)
		seasonNumber = &sn
	}

	source := req.Source
	if source == "" {
		source = "interactive"
	}

	now := time.Now().UTC().Format(time.RFC3339)
	row, err := s.q.CreateGrabHistory(ctx, db.CreateGrabHistoryParams{
		ID:                uuid.New().String(),
		SeriesID:          req.SeriesID,
		EpisodeID:         dbutil.NullableString(req.EpisodeID),
		SeasonNumber:      seasonNumber,
		IndexerID:         dbutil.NullableString(req.IndexerID),
		ReleaseGuid:       req.Release.GUID,
		ReleaseTitle:      req.Release.Title,
		ReleaseSource:     string(req.Release.Quality.Source),
		ReleaseResolution: string(req.Release.Quality.Resolution),
		ReleaseCodec:      string(req.Release.Quality.Codec),
		ReleaseHdr:        string(req.Release.Quality.HDR),
		Protocol:          string(req.Release.Protocol),
		Size:              req.Release.Size,
		DownloadClientID:  nil,
		ClientItemID:      nil,
		DownloadStatus:    "queued",
		ScoreBreakdown:    nil,
		GrabbedAt:         now,
		Source:            source,
		InfoHash:          nil, // populated later when Haul reports it via UpdateGrabInfoHash
	})
	if err != nil {
		return db.GrabHistory{}, fmt.Errorf("recording grab history: %w", err)
	}

	if s.bus != nil {
		s.bus.Publish(ctx, events.Event{
			Type:   events.TypeEpisodeGrabbed,
			ShowID: req.SeriesID,
			Data:   map[string]any{"title": req.Release.Title},
		})
	}

	return row, nil
}

// UpdateGrabDownloadClient records the download client and client-assigned item ID
// on an existing grab history entry. Called after a release is successfully submitted
// to a download client.
func (s *Service) UpdateGrabDownloadClient(ctx context.Context, arg db.UpdateGrabDownloadClientParams) error {
	if err := s.q.UpdateGrabDownloadClient(ctx, arg); err != nil {
		return fmt.Errorf("updating grab download client: %w", err)
	}
	return nil
}

// UpdateGrabStatus sets the download_status on a grab row. Used when
// the grab handler can't make forward progress (e.g. download client
// rejects the release because the indexer response had no download
// URL) — without this, the row stays at "queued" forever and shows
// up in the search guardrail as "already grabbed", suppressing the
// retry the user wants.
func (s *Service) UpdateGrabStatus(ctx context.Context, grabID, status string) error {
	if err := s.q.UpdateGrabStatus(ctx, db.UpdateGrabStatusParams{
		DownloadStatus:  status,
		DownloadedBytes: 0,
		ID:              grabID,
	}); err != nil {
		return fmt.Errorf("updating grab status: %w", err)
	}
	return nil
}

// UpdateGrabInfoHash records the BitTorrent info_hash on a grab row. The
// stall watcher uses this to correlate Haul's stall events back to the
// grab that initiated them.
func (s *Service) UpdateGrabInfoHash(ctx context.Context, grabID, infoHash string) error {
	if err := s.q.UpdateGrabInfoHash(ctx, db.UpdateGrabInfoHashParams{
		InfoHash: dbutil.NullableString(infoHash),
		ID:       grabID,
	}); err != nil {
		return fmt.Errorf("updating grab info_hash: %w", err)
	}
	return nil
}

// GetGrabByInfoHash returns the most-recent grab for the given
// info_hash. Returns sql.ErrNoRows when none matches — callers map
// that to 404 at the API edge. Used by the /grabs/by-hash/{hash}/research
// endpoint to find the original grab so it can blocklist the dead
// release and retry.
func (s *Service) GetGrabByInfoHash(ctx context.Context, infoHash string) (db.GrabHistory, error) {
	return s.q.GetGrabByInfoHash(ctx, dbutil.NullableString(infoHash))
}

func rowToConfig(row db.IndexerConfig) (Config, error) {
	createdAt, err := time.Parse(time.RFC3339, row.CreatedAt)
	if err != nil {
		createdAt = time.Time{}
	}
	updatedAt, err := time.Parse(time.RFC3339, row.UpdatedAt)
	if err != nil {
		updatedAt = time.Time{}
	}
	return Config{
		ID:         row.ID,
		Name:       row.Name,
		Kind:       row.Kind,
		Enabled:    row.Enabled,
		Priority:   int(row.Priority),
		MinSeeders: int(row.MinSeeders),
		Settings:   json.RawMessage(row.Settings),
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	}, nil
}

// extractRateLimit reads the rate_limit field from an indexer's settings JSON.
// Returns 0 (unlimited) if the field is absent or unparseable.
func extractRateLimit(settings json.RawMessage) int {
	var s struct {
		RateLimit int `json:"rate_limit"`
	}
	_ = json.Unmarshal(settings, &s)
	return s.RateLimit
}
