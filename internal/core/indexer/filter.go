package indexer

import (
	"context"
	"fmt"
	"sort"

	"github.com/beacon-stack/pilot/internal/core/language"
	"github.com/beacon-stack/pilot/internal/core/releaseprofile"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/parser"
)

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
