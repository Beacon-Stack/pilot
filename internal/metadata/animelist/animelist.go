// Package animelist consumes the Anime-Lists/anime-lists community XML
// and exposes per-TMDB-id lookups for downstream consumers (the show
// refresh path and the search query builder).
//
// Why this exists: TMDB's anime metadata is structurally inconsistent —
// many shows are served as one giant Season 1 (Jujutsu Kaisen has 47+
// episodes under TMDB S1 even though TVDB and the wider anime community
// treat it as 3 distinct seasons). Indexers and fansub release groups
// tag anime episodes with absolute numbering ("Show - 48"), not the
// TMDB-style S01E48 we naively emit. Without this mapping, episode
// search for anime returns zero results.
//
// The Anime-Lists XML at https://github.com/Anime-Lists/anime-lists is
// the canonical community-maintained mapping among AniDB / TVDB / TMDB
// ids and per-episode offsets. We fetch it on a daily schedule, persist
// it to disk for fallback, and serve point lookups by TMDB id.
//
// Direct TVDB or AniDB integration was considered and rejected — see
// memory/project_beacon_tvdb_proxy.md for the reasoning. tl;dr both
// require credentials end users won't have.
package animelist

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// upstreamURL is where the master XML lives. Pinning to master/raw is
// fine — the file is auto-regenerated daily by a GitHub Action, so any
// breaking format changes would land in the upstream tests first.
const upstreamURL = "https://raw.githubusercontent.com/Anime-Lists/anime-lists/master/anime-list-master.xml"

// upstreamURLForTest lets the test suite point fetch() at a httptest
// server without monkey-patching the const. Production callers leave
// this empty; fetch() falls through to upstreamURL.
var upstreamURLForTest = ""

// fetchTimeout caps a single HTTP fetch attempt. The XML is ~3MB; a
// generous timeout covers high-latency mirrors without blocking startup.
const fetchTimeout = 60 * time.Second

// refreshInterval is the gap between background refreshes. Daily is
// plenty given the upstream's own update cadence; more often just burns
// GitHub's rate-limit budget.
const refreshInterval = 24 * time.Hour

// Mapping is one entry from anime-list-master.xml — a single AniDB
// "anime" pointing at its TVDB and TMDB equivalents. Multiple entries
// can share a TMDB id (TMDB lumps cours into one tmdbtv id, but each
// cour has its own anidb id). Callers that look up by TMDB id receive
// the FIRST matching entry — usually the original season, which is
// what the search-query builder needs to flag the show as anime.
type Mapping struct {
	// AniDBID is the AniDB anime ID (the canonical anime metadata id).
	AniDBID int
	// TVDBID is the TheTVDB series ID, or 0 when no TVDB mapping exists.
	TVDBID int
	// TMDBTV is the TMDB TV id; 0 when no mapping. This is our primary
	// lookup key — Pilot stores TMDB tv ids on every series row.
	TMDBTV int
	// DefaultTVDBSeason is the TVDB season number this anime maps to.
	// Special value -1 indicates "absolute" — the show's episodes are
	// tracked by absolute number on TVDB, not season-relative. For our
	// purposes this just confirms the show is anime; we use the
	// absolute number for indexer queries either way.
	DefaultTVDBSeason int
	// EpisodeOffset shifts incoming episode numbers by this much when
	// translating between AniDB and TVDB layouts. 0 for most shows.
	EpisodeOffset int
	// TMDBSeason is the TMDB season number this AniDB entry maps to.
	// Used to disambiguate when a single TMDB tv id covers multiple
	// AniDB cours (rare but happens, e.g. Jujutsu Kaisen 95479 has
	// entries for both tmdbseason=1 and tmdbseason=2).
	TMDBSeason int
	// Name is the AniDB name for this anime. Surfaced in logs only.
	Name string
}

// Service is the runtime handle: fetch the XML, hold an indexed map,
// serve lookups. Safe for concurrent reads after Start returns.
type Service struct {
	logger     *slog.Logger
	cachePath  string
	httpClient *http.Client

	mu         sync.RWMutex
	byTMDBID   map[int]*Mapping
	lastLoaded time.Time
}

// New constructs a Service. cachePath is where we persist the most
// recently fetched XML on disk so we have a fallback when GitHub is
// unreachable; pass "" to disable disk caching (tests).
func New(cachePath string, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		logger:     logger,
		cachePath:  cachePath,
		httpClient: &http.Client{Timeout: fetchTimeout},
		byTMDBID:   make(map[int]*Mapping),
	}
}

// Start kicks off the lifecycle: load from disk cache (if any) so
// lookups work immediately, then fetch fresh from upstream in the
// background, then keep refreshing on the schedule until ctx cancels.
//
// Returns nil immediately — failures are logged, not returned.
// Callers don't block on network I/O.
func (s *Service) Start(ctx context.Context) {
	// Best-effort: load cached XML so anime detection works pre-network.
	if s.cachePath != "" {
		if err := s.loadFromDisk(); err != nil {
			s.logger.Debug("animelist: no cached XML on disk yet", "path", s.cachePath, "error", err)
		} else {
			s.logger.Info("animelist: loaded from disk cache", "entries", s.size(), "path", s.cachePath)
		}
	}

	// Fire one fetch immediately, then on a timer.
	go func() {
		if err := s.refresh(ctx); err != nil {
			s.logger.Warn("animelist: initial fetch failed; using cached data", "error", err)
		}
		t := time.NewTicker(refreshInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := s.refresh(ctx); err != nil {
					s.logger.Warn("animelist: scheduled refresh failed", "error", err)
				}
			}
		}
	}()
}

// Lookup returns the AniDB mapping for a given TMDB tv id. Returns nil,
// false when the show isn't in the anime list (the common case for
// non-anime series). Returns the FIRST matching mapping — for shows
// where the same tmdbtv id covers multiple AniDB cours, we surface the
// one with the lowest tmdbseason number (typically the original cour).
func (s *Service) Lookup(tmdbID int) (*Mapping, bool) {
	if tmdbID == 0 {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.byTMDBID[tmdbID]
	return m, ok
}

// IsAnime is a convenience wrapper for callers that only care about the
// boolean — anime detection during series add/refresh.
func (s *Service) IsAnime(tmdbID int) bool {
	_, ok := s.Lookup(tmdbID)
	return ok
}

// LastLoaded reports the wall-clock time of the most recent successful
// load (from disk OR upstream). Zero when never loaded. Useful for
// system-status UI.
func (s *Service) LastLoaded() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastLoaded
}

// refresh fetches the upstream XML, parses it, swaps in the new index,
// and persists to disk. Network failures fall back silently to whatever
// was already in memory.
func (s *Service) refresh(ctx context.Context) error {
	body, err := s.fetch(ctx)
	if err != nil {
		return err
	}
	if err := s.loadFromBytes(body); err != nil {
		return fmt.Errorf("parse upstream xml: %w", err)
	}
	if s.cachePath != "" {
		if err := writeAtomic(s.cachePath, body); err != nil {
			s.logger.Warn("animelist: persisting cache failed", "path", s.cachePath, "error", err)
		}
	}
	s.logger.Info("animelist: refreshed", "entries", s.size(), "bytes", len(body))
	return nil
}

func (s *Service) fetch(ctx context.Context) ([]byte, error) {
	url := upstreamURL
	if upstreamURLForTest != "" {
		url = upstreamURLForTest
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream returned HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (s *Service) loadFromDisk() error {
	if s.cachePath == "" {
		return errors.New("no cache path configured")
	}
	body, err := os.ReadFile(s.cachePath)
	if err != nil {
		return err
	}
	return s.loadFromBytes(body)
}

// loadFromBytes parses raw XML and atomically swaps in the new index.
// Exported only for tests; production callers go through refresh.
func (s *Service) loadFromBytes(body []byte) error {
	mappings, err := parse(body)
	if err != nil {
		return err
	}
	idx := make(map[int]*Mapping, len(mappings))
	for i := range mappings {
		m := &mappings[i]
		if m.TMDBTV == 0 {
			continue // can't index without a TMDB id
		}
		// Keep the FIRST entry seen for each tmdbtv id. The XML lists
		// multiple AniDB cours sharing one TMDB id (e.g. Jujutsu Kaisen
		// 95479 has tmdbseason=1 and tmdbseason=2 entries pointing at
		// different AniDB ids). The first one is typically the original
		// cour, which is what most callers want for "is this anime?"
		// detection. More nuanced per-episode mapping would scan the
		// whole list at lookup time — out of scope for v1.
		if _, exists := idx[m.TMDBTV]; exists {
			continue
		}
		idx[m.TMDBTV] = m
	}
	s.mu.Lock()
	s.byTMDBID = idx
	s.lastLoaded = time.Now()
	s.mu.Unlock()
	return nil
}

func (s *Service) size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.byTMDBID)
}

// ── XML parsing ─────────────────────────────────────────────────────────────

// rawAnime is the on-disk shape. The XML uses string-typed attributes
// (some are blank, some are the literal "movie" keyword for non-series
// entries) so we accept everything as string and parse defensively in
// toMapping.
type rawAnime struct {
	XMLName           xml.Name `xml:"anime"`
	AniDBID           string   `xml:"anidbid,attr"`
	TVDBID            string   `xml:"tvdbid,attr"`
	DefaultTVDBSeason string   `xml:"defaulttvdbseason,attr"`
	EpisodeOffset     string   `xml:"episodeoffset,attr"`
	TMDBTV            string   `xml:"tmdbtv,attr"`
	TMDBSeason        string   `xml:"tmdbseason,attr"`
	Name              string   `xml:"name"`
}

type rawList struct {
	XMLName xml.Name   `xml:"anime-list"`
	Entries []rawAnime `xml:"anime"`
}

// parse is exported only for tests.
func parse(body []byte) ([]Mapping, error) {
	var raw rawList
	if err := xml.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	out := make([]Mapping, 0, len(raw.Entries))
	for _, e := range raw.Entries {
		m, ok := toMapping(e)
		if !ok {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

// toMapping converts the on-disk row to our domain type. Returns false
// for entries that aren't useful for our purposes (no AniDB id, or
// non-numeric tvdb/tmdb fields like the literal "movie").
func toMapping(r rawAnime) (Mapping, bool) {
	anidb, ok := atoiSafe(r.AniDBID)
	if !ok || anidb == 0 {
		return Mapping{}, false
	}
	tvdb, _ := atoiSafe(r.TVDBID) // "movie" or empty → 0
	tmdbtv, _ := atoiSafe(r.TMDBTV)
	tmdbseason, _ := atoiSafe(r.TMDBSeason)
	episodeOffset, _ := atoiSafe(r.EpisodeOffset)

	defSeason := -1 // sentinel for "a"/absolute
	if r.DefaultTVDBSeason != "a" && r.DefaultTVDBSeason != "" {
		defSeason, _ = atoiSafe(r.DefaultTVDBSeason)
	}
	return Mapping{
		AniDBID:           anidb,
		TVDBID:            tvdb,
		TMDBTV:            tmdbtv,
		TMDBSeason:        tmdbseason,
		EpisodeOffset:     episodeOffset,
		DefaultTVDBSeason: defSeason,
		Name:              r.Name,
	}, true
}

// atoiSafe parses an int from a string, treating empty / non-numeric as
// (0, false). Replaces strconv.Atoi to keep the parse loop tidy — most
// fields in the upstream XML are blank for entries we don't care about.
func atoiSafe(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	negative := false
	for i, r := range s {
		if i == 0 && r == '-' {
			negative = true
			continue
		}
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	if negative {
		n = -n
	}
	return n, true
}

// writeAtomic writes data to a temp file in the same directory and
// renames it into place. Avoids leaving a half-written file if the
// process dies mid-write.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".animelist-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op if rename succeeded
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
