package indexer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/pkg/plugin"
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
