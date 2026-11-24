// Package indexer manages indexer configurations and runs release
// searches: it queries every enabled indexer, tags releases that fail a
// filter, and ranks the results (see searchResultLess).
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
	"github.com/beacon-stack/pilot/internal/core/releaseprofile"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/events"
	"github.com/beacon-stack/pilot/internal/parser"
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

// struggleTimeout is how long an interactive search waits on an indexer
// that keeps missing (see indexerHealth) — typically a Cloudflare-walled
// site whose challenge FlareSolverr often can't solve. Its request still
// goes out: Pulse keeps scraping after we stop waiting and caches what it
// finds, and the search modal's automatic re-search picks that up. What
// changes is that everyone else's results don't sit behind it for 25s.
// A var so tests can shorten it.
var struggleTimeout = 6 * time.Second

// indexerHealth remembers each indexer's last few search outcomes.
// An indexer that missed (timed out or failed) struggleMisses of its
// last healthWindow searches is "struggling". A window rather than a
// streak, because a struggling indexer's cached answers come back fast
// and would otherwise reset a streak between every two cold misses.
type indexerHealth struct {
	mu     sync.Mutex
	recent map[string][]bool // true = miss, oldest first
}

const (
	healthWindow   = 3
	struggleMisses = 2
)

func (h *indexerHealth) record(id string, miss bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.recent == nil {
		h.recent = make(map[string][]bool)
	}
	r := append(h.recent[id], miss)
	if len(r) > healthWindow {
		r = r[len(r)-healthWindow:]
	}
	h.recent[id] = r
}

// snapshot copies the recent outcomes, for saving.
func (h *indexerHealth) snapshot() map[string][]bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make(map[string][]bool, len(h.recent))
	for id, r := range h.recent {
		out[id] = append([]bool(nil), r...)
	}
	return out
}

// restore replaces the recent outcomes with saved ones.
func (h *indexerHealth) restore(saved map[string][]bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.recent = make(map[string][]bool, len(saved))
	for id, r := range saved {
		if len(r) > healthWindow {
			r = r[len(r)-healthWindow:]
		}
		h.recent[id] = append([]bool(nil), r...)
	}
}

// healthSettingKey is where indexer health is saved in the settings
// table, so a restart doesn't forget which indexers keep missing and
// give them the full timeout again.
const healthSettingKey = "indexer_health"

// LoadHealth restores indexer health saved by an earlier run. Nothing
// saved, or something unreadable, starts fresh.
func (s *Service) LoadHealth(ctx context.Context) error {
	if s.q == nil {
		return nil
	}
	raw, err := s.q.GetSetting(ctx, healthSettingKey)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("loading indexer health: %w", err)
	}
	var saved map[string][]bool
	if err := json.Unmarshal([]byte(raw), &saved); err != nil {
		return fmt.Errorf("reading saved indexer health: %w", err)
	}
	s.health.restore(saved)
	return nil
}

// saveHealth writes the current indexer health. Called in the background
// after each search; saves are serialized so an older snapshot can't
// land after a newer one.
func (s *Service) saveHealth() {
	if s.q == nil {
		return
	}
	s.healthSaveMu.Lock()
	defer s.healthSaveMu.Unlock()
	b, err := json.Marshal(s.health.snapshot())
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.q.SetSetting(ctx, db.SetSettingParams{
		Key:       healthSettingKey,
		Value:     string(b),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil && s.logger != nil {
		s.logger.Warn("could not save indexer health", "error", err)
	}
}

func (h *indexerHealth) struggling(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	misses := 0
	for _, miss := range h.recent[id] {
		if miss {
			misses++
		}
	}
	return misses >= struggleMisses
}

type Service struct {
	q      db.Querier
	reg    *registry.Registry
	bus    *events.Bus
	rl     *ratelimit.Registry
	logger *slog.Logger
	cache  sync.Map      // config ID → plugin.Indexer
	sem    chan struct{} // bounds concurrent indexer searches
	health indexerHealth // recent outcomes per indexer; see struggleTimeout
	// healthSaveMu serializes saveHealth.
	healthSaveMu sync.Mutex
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
		langs := make([]parser.Language, 0, len(results[i].Languages))
		for _, l := range results[i].Languages {
			langs = append(langs, parser.Language(l))
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

// Outcome summarizes one indexer's contribution to a search. The UI shows
// it per indexer, so "half the indexers failed" looks different from
// "nothing matched".
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
	// Searching means the indexer hasn't answered yet. Only seen in the
	// snapshots of a search still running (see SearchStream).
	Searching bool
}

// SearchGroup is one search every indexer runs. Its Queries are tried in
// order on each indexer, a later one only while the results so far fall
// short of Enough — so a second phrasing ("Show Season 1" after "Show
// S01") costs a request only on the indexers where the first didn't
// find what was wanted. Results from every query tried are kept.
type SearchGroup struct {
	Queries []plugin.SearchQuery
	// Enough reports whether one indexer's results so far suffice. Nil
	// means any result is enough.
	Enough func([]SearchResult) bool
}

// Snapshot is a search's results so far: ranked and tagged exactly as
// the finished search will be, with where each indexer stands.
type Snapshot struct {
	Results  []SearchResult
	Outcomes []Outcome
	Done     bool
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
	snap, err := s.SearchStream(ctx, []SearchGroup{{Queries: []plugin.SearchQuery{q}}}, nil)
	return snap.Results, snap.Outcomes, err
}

// SearchStream runs every group on every enabled indexer and returns the
// merged, ranked, tagged results. When onUpdate is set it is called each
// time an indexer finishes a group, with everything found so far, so a
// caller can show results while slower indexers are still working.
// onUpdate calls are serialized and arrive in order; keep them quick.
//
// Every indexer gets the full per-indexer timeout. Use SearchInteractive
// when a person is waiting.
func (s *Service) SearchStream(ctx context.Context, groups []SearchGroup, onUpdate func(Snapshot)) (Snapshot, error) {
	return s.search(ctx, groups, onUpdate, false)
}

// SearchInteractive is SearchStream for a person watching the results:
// an indexer that keeps missing is waited on for struggleTimeout rather
// than the full timeout, and reported pending if it hasn't answered.
// Automatic searches keep the full wait, so they don't lose a slow
// indexer's results — nobody re-searches for them a minute later.
func (s *Service) SearchInteractive(ctx context.Context, groups []SearchGroup, onUpdate func(Snapshot)) (Snapshot, error) {
	return s.search(ctx, groups, onUpdate, true)
}

func (s *Service) search(ctx context.Context, groups []SearchGroup, onUpdate func(Snapshot), interactive bool) (Snapshot, error) {
	rows, err := s.q.ListEnabledIndexers(ctx)
	if err != nil {
		return Snapshot{Done: true}, fmt.Errorf("listing enabled indexers: %w", err)
	}
	if len(rows) == 0 || len(groups) == 0 {
		return Snapshot{Done: true}, nil
	}

	// Loaded once for the whole search, not once per snapshot or query.
	fin := s.newFinisher(ctx, rows)

	type tally struct {
		outcome   Outcome
		remaining int
	}
	var (
		mu      sync.Mutex
		raw     []SearchResult
		seen    = make(map[string]struct{})
		tallies = make([]*tally, len(rows))
		byID    = make(map[string]*tally, len(rows))
		errs    []error
	)
	for i, row := range rows {
		tallies[i] = &tally{
			outcome:   Outcome{IndexerID: row.ID, IndexerName: row.Name, Searching: true},
			remaining: len(groups),
		}
		byID[row.ID] = tallies[i]
	}

	snapshot := func(done bool) Snapshot {
		results := make([]SearchResult, len(raw))
		for i, r := range raw {
			r.FilterReasons = append([]string(nil), r.FilterReasons...)
			results[i] = r
		}
		fin.apply(results)
		outcomes := make([]Outcome, len(tallies))
		for i, t := range tallies {
			outcomes[i] = t.outcome
		}
		return Snapshot{Results: results, Outcomes: outcomes, Done: done}
	}

	// First update before anyone answers: which indexers are searching.
	if onUpdate != nil {
		onUpdate(snapshot(false))
	}

	var wg sync.WaitGroup
	for _, row := range rows {
		cfg, _ := rowToConfig(row)
		for _, g := range groups {
			wg.Add(1)
			go func(cfg Config, g SearchGroup) {
				defer wg.Done()
				found, err := s.runGroup(ctx, cfg, g, interactive)

				mu.Lock()
				defer mu.Unlock()
				t := byID[cfg.ID]
				t.remaining--
				t.outcome.Searching = t.remaining > 0
				switch {
				case err == nil:
					for _, r := range found {
						key := r.IndexerID + "\x00" + r.GUID
						if _, dup := seen[key]; dup {
							continue
						}
						seen[key] = struct{}{}
						raw = append(raw, r)
						t.outcome.Results++
					}
				case errors.Is(err, errSearchTimedOut):
					// Deadline hits are "pending", not failures: the scrape
					// continues server-side and its result will be cached.
					t.outcome.Pending = true
				default:
					// An error in any group marks the indexer failed; the
					// first error's message wins.
					errs = append(errs, fmt.Errorf("indexer %q: %w", cfg.Name, err))
					if t.outcome.Error == "" {
						t.outcome.Error = err.Error()
					}
				}
				if onUpdate != nil {
					onUpdate(snapshot(false))
				}
			}(cfg, g)
		}
	}
	wg.Wait()
	go s.saveHealth()

	mu.Lock()
	defer mu.Unlock()
	final := snapshot(true)
	var combinedErr error
	if len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, e := range errs {
			msgs[i] = e.Error()
		}
		combinedErr = fmt.Errorf("%d indexer(s) failed: %v", len(errs), msgs)
	}
	return final, combinedErr
}

// runGroup runs one group's queries on one indexer, in order, until the
// results are enough. Each query takes a concurrency slot for its own
// request only (see searchSlotted). The error is nil whenever anything
// was found.
func (s *Service) runGroup(ctx context.Context, cfg Config, g SearchGroup, interactive bool) ([]SearchResult, error) {
	idx, err := s.cachedIndexer(cfg.Kind, cfg.ID, cfg.Settings)
	if err != nil {
		return nil, err
	}

	var found []SearchResult
	var lastErr error
	for _, q := range g.Queries {
		if ctx.Err() != nil {
			break
		}
		releases, err := s.searchSlotted(ctx, cfg, idx, q, interactive)
		if err != nil {
			lastErr = err
			// A timeout means this indexer is still scraping (Pulse
			// finishes and caches it). Another phrasing now would only
			// start a second slow scrape it has no time left to finish.
			if errors.Is(err, errSearchTimedOut) {
				break
			}
		}
		for _, r := range releases {
			found = append(found, newSearchResult(r, cfg.ID, cfg.Name))
		}
		enough := len(found) > 0
		if g.Enough != nil {
			enough = g.Enough(found)
		}
		if enough {
			break
		}
	}
	if len(found) > 0 {
		return found, nil
	}
	if lastErr == nil && ctx.Err() != nil {
		lastErr = ctx.Err()
		if errors.Is(lastErr, context.DeadlineExceeded) {
			lastErr = errSearchTimedOut
		}
	}
	return nil, lastErr
}

// searchSlotted waits for the indexer's rate limit, then takes one of the
// global concurrency slots for the request alone. The wait comes first
// and holds nothing, so an indexer waiting out its own rate limit doesn't
// keep faster indexers queued behind it.
func (s *Service) searchSlotted(ctx context.Context, cfg Config, idx plugin.Indexer, q plugin.SearchQuery, interactive bool) ([]plugin.Release, error) {
	if err := s.rl.Wait(ctx, cfg.ID, extractRateLimit(cfg.Settings)); err != nil {
		return nil, fmt.Errorf("waiting for this indexer's rate limit: %w", err)
	}
	// Bound global concurrency so queries×indexers can't all hit the
	// network at once.
	if s.sem != nil {
		select {
		case s.sem <- struct{}{}:
			defer func() { <-s.sem }()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return s.searchOne(ctx, cfg, idx, q, interactive)
}

// searchOne sends one query to one indexer under the per-indexer
// timeout, logs how long it took, and records the outcome in the
// indexer's health. The caller has already waited for the rate limit.
func (s *Service) searchOne(ctx context.Context, cfg Config, idx plugin.Indexer, q plugin.SearchQuery, interactive bool) ([]plugin.Release, error) {
	// Per-indexer timeout so one hung provider can't hold its slot for
	// the whole parent budget. Inherits any earlier parent deadline.
	timeout := perIndexerSearchTimeout
	if interactive && s.health.struggling(cfg.ID) {
		timeout = struggleTimeout
	}
	sctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	started := time.Now()
	releases, err := idx.Search(sctx, q)
	elapsed := time.Since(started)
	// A search the caller ended (modal closed, request budget spent) says
	// nothing about this indexer; anything else does.
	if ctx.Err() == nil {
		s.health.record(cfg.ID, err != nil)
	}
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
			"timeout_ms", timeout.Milliseconds(),
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
	return releases, err
}

// finisher ranks and tags a result set: the ordering and the min-seeders,
// release-profile and language tags every search applies. Its inputs are
// loaded once per search.
type finisher struct {
	minByIndexer map[string]int
	profiles     *releaseprofile.Matcher
	language     *language.Preference
}

func (s *Service) newFinisher(ctx context.Context, rows []db.IndexerConfig) finisher {
	f := finisher{minByIndexer: make(map[string]int, len(rows))}
	for _, r := range rows {
		f.minByIndexer[r.ID] = int(r.MinSeeders)
	}
	if s.q == nil {
		return f
	}
	// A load failure tags nothing: a database hiccup must not empty
	// every search.
	if m, err := releaseprofile.LoadMatcher(ctx, s.q); err == nil {
		f.profiles = m
	} else if s.logger != nil {
		s.logger.Warn("could not load release profiles; not applying them", "error", err)
	}
	if pref, err := language.NewStore(s.q).Get(ctx); err == nil {
		f.language = &pref
	} else if s.logger != nil {
		s.logger.Warn("could not load the language preference; not applying it", "error", err)
	}
	return f
}

func (f finisher) apply(results []SearchResult) {
	sort.Slice(results, func(i, j int) bool {
		return searchResultLess(results[i], results[j])
	})
	applyMinSeedersFilter(results, f.minByIndexer)
	if f.profiles != nil {
		applyReleaseProfiles(results, f.profiles)
	}
	if f.language != nil {
		applyLanguagePreference(results, *f.language)
	}
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
