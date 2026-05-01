package animelist

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// fixtureXML is a tiny slice of the real anime-list-master.xml — three
// representative entries pulled verbatim from the upstream file:
//
//   - Jujutsu Kaisen cours 1, 2, and 3 (all share tmdbtv=95479).
//     Cour 3's entry has tmdboffset="47" — that's what powers the
//     TVDB-S03E01 → absolute-48 conversion the search filter needs.
//   - Cowboy Bebop (defaulttvdbseason="1", a reference well-known case)
//   - 86 (2021) — has an episodeoffset, exercises non-zero offset path
//   - "movie" entry — non-numeric tvdbid, must be skipped without panic
//   - empty-tmdbtv entry — must be skipped (we index by tmdbtv id)
//
// Locked in here rather than fetched at test time so CI doesn't depend
// on GitHub availability.
const fixtureXML = `<?xml version="1.0" encoding="utf-8"?>
<anime-list>
  <anime anidbid="15275" tvdbid="377543" defaulttvdbseason="1" episodeoffset="" tmdbtv="95479" tmdbseason="1" tmdboffset="" tmdbid="" imdbid="">
    <name>Jujutsu Kaisen</name>
  </anime>
  <anime anidbid="17196" tvdbid="377543" defaulttvdbseason="2" episodeoffset="" tmdbtv="95479" tmdbseason="2" tmdboffset="" tmdbid="" imdbid="">
    <name>Jujutsu Kaisen (2023)</name>
  </anime>
  <anime anidbid="18363" tvdbid="377543" defaulttvdbseason="3" episodeoffset="" tmdbtv="95479" tmdbseason="1" tmdboffset="47" tmdbid="" imdbid="">
    <name>Jujutsu Kaisen Shibuya Incident</name>
  </anime>
  <anime anidbid="23" tvdbid="76885" defaulttvdbseason="1" episodeoffset="" tmdbtv="30991" tmdbseason="1" tmdboffset="" tmdbid="" imdbid="">
    <name>Cowboy Bebop</name>
  </anime>
  <anime anidbid="16172" tvdbid="378609" defaulttvdbseason="1" episodeoffset="11" tmdbtv="100565" tmdbseason="1" tmdboffset="11" tmdbid="" imdbid="">
    <name>86 (2021)</name>
  </anime>
  <anime anidbid="16176" tvdbid="movie" defaulttvdbseason="" episodeoffset="" tmdbtv="" tmdbseason="" tmdboffset="" tmdbid="812225" imdbid="tt22868844">
    <name>Some Movie Entry</name>
  </anime>
  <anime anidbid="15282" tvdbid="" defaulttvdbseason="" episodeoffset="" tmdbtv="" tmdbseason="" tmdboffset="" tmdbid="" imdbid="">
    <name>Gaina Tamager</name>
  </anime>
</anime-list>`

// TestParse_HeadlineCase locks down the JJK lookup that motivated this
// whole feature. If this fails, the search-side fix can't possibly work.
func TestLookup_JujutsuKaisen(t *testing.T) {
	s := New("", nil)
	if err := s.loadFromBytes([]byte(fixtureXML)); err != nil {
		t.Fatalf("loadFromBytes: %v", err)
	}
	m, ok := s.Lookup(95479)
	if !ok {
		t.Fatal("expected JJK (tmdbtv=95479) to be found")
	}
	if m.AniDBID != 15275 {
		t.Errorf("expected first matching cour (anidbid=15275), got %d", m.AniDBID)
	}
	if m.TVDBID != 377543 {
		t.Errorf("tvdbid mismatch: %d", m.TVDBID)
	}
	if m.Name != "Jujutsu Kaisen" {
		t.Errorf("name mismatch: %q", m.Name)
	}
}

// IsAnime is the convenience the show service uses; verify it agrees
// with Lookup on the same data.
func TestIsAnime(t *testing.T) {
	s := New("", nil)
	_ = s.loadFromBytes([]byte(fixtureXML))

	if !s.IsAnime(95479) {
		t.Error("JJK should be detected as anime")
	}
	if !s.IsAnime(30991) {
		t.Error("Cowboy Bebop should be detected as anime")
	}
	if s.IsAnime(99999999) {
		t.Error("non-existent tmdb id must NOT be flagged as anime")
	}
	if s.IsAnime(0) {
		t.Error("tmdb id 0 must NOT be flagged as anime")
	}
}

// FIRST-wins rule — when the XML has multiple entries sharing one
// tmdbtv id (Jujutsu Kaisen has cours 1, 2, and 3), Lookup returns
// the one indexed first. Verifies we don't accidentally swap to
// last-write-wins.
func TestLookup_FirstWinsWhenMultipleEntriesShareTMDBID(t *testing.T) {
	s := New("", nil)
	_ = s.loadFromBytes([]byte(fixtureXML))

	m, _ := s.Lookup(95479)
	if m.TMDBSeason != 1 {
		t.Errorf("expected first (tmdbseason=1) entry to win, got %d", m.TMDBSeason)
	}
	if m.DefaultTVDBSeason != 1 {
		t.Errorf("expected first cour (DefaultTVDBSeason=1), got %d", m.DefaultTVDBSeason)
	}
}

// ── TVDBSeasonToAbsolute (the JJK incident path) ───────────────────────────

// Headline: Jujutsu Kaisen tagged in TVDB-style as S03E01 should
// resolve to absolute episode 48 — that's the user's TMDB-relative
// S01E48. Without this, fansub releases tagged TVDB-style get
// dropped by filterByEpisode as wrong-season.
func TestTVDBSeasonToAbsolute_JujutsuKaisenS03E01Resolves48(t *testing.T) {
	s := New("", nil)
	_ = s.loadFromBytes([]byte(fixtureXML))

	abs, ok := s.TVDBSeasonToAbsolute(95479, 3, 1)
	if !ok {
		t.Fatal("expected TVDB S03E01 → absolute conversion to succeed for JJK")
	}
	if abs != 48 {
		t.Errorf("absolute: got %d, want 48 (1 + offset 47)", abs)
	}
}

// Multi-episode arithmetic — TVDB S03E05 → absolute 52.
func TestTVDBSeasonToAbsolute_OffsetAddsCorrectly(t *testing.T) {
	s := New("", nil)
	_ = s.loadFromBytes([]byte(fixtureXML))
	abs, _ := s.TVDBSeasonToAbsolute(95479, 3, 5)
	if abs != 52 {
		t.Errorf("S03E05: got %d, want 52", abs)
	}
}

// Cour 1 of JJK has tmdboffset="" (empty/0) → S01E05 still resolves
// because offset 0 means TVDB and TMDB agree on numbering. Confirms
// the empty-offset case doesn't get rejected as "no mapping."
func TestTVDBSeasonToAbsolute_ZeroOffsetStillWorks(t *testing.T) {
	s := New("", nil)
	_ = s.loadFromBytes([]byte(fixtureXML))
	abs, ok := s.TVDBSeasonToAbsolute(95479, 1, 5)
	if !ok {
		t.Error("expected zero-offset cour 1 to still resolve")
	}
	if abs != 5 {
		t.Errorf("zero offset: got %d, want 5", abs)
	}
}

// JJK cour 2 has tmdbseason=2 — TMDB has split it into a real S2 in
// the XML's view (even if pilot's actual TMDB DB collapsed it). This
// case is intentionally OUT of scope for v1 — we only handle the
// "TMDB collapsed everything into season 1" case. Verify we cleanly
// return (0, false) rather than silently producing wrong numbers.
func TestTVDBSeasonToAbsolute_TMDBSplitSeasonReturnsFalse(t *testing.T) {
	s := New("", nil)
	_ = s.loadFromBytes([]byte(fixtureXML))
	_, ok := s.TVDBSeasonToAbsolute(95479, 2, 1) // tmdbseason=2 entry
	if ok {
		t.Error("TMDB-split-into-S2 case should NOT resolve (out of scope for v1)")
	}
}

// Unknown TVDB season → (0, false), not a panic and not a guess.
func TestTVDBSeasonToAbsolute_UnknownSeasonReturnsFalse(t *testing.T) {
	s := New("", nil)
	_ = s.loadFromBytes([]byte(fixtureXML))
	if _, ok := s.TVDBSeasonToAbsolute(95479, 99, 1); ok {
		t.Error("unknown TVDB season must return (_, false)")
	}
}

// Non-anime tmdb id → (0, false). Avoids accidentally feeding fake
// absolutes into the search-filter for non-anime series.
func TestTVDBSeasonToAbsolute_NonAnimeReturnsFalse(t *testing.T) {
	s := New("", nil)
	_ = s.loadFromBytes([]byte(fixtureXML))
	if _, ok := s.TVDBSeasonToAbsolute(99999999, 1, 1); ok {
		t.Error("non-anime tmdb id must return (_, false)")
	}
}

// Defensive: zero/negative episode numbers fail without panic.
func TestTVDBSeasonToAbsolute_RejectsZeroEpisode(t *testing.T) {
	s := New("", nil)
	_ = s.loadFromBytes([]byte(fixtureXML))
	if _, ok := s.TVDBSeasonToAbsolute(95479, 1, 0); ok {
		t.Error("episode=0 must return (_, false)")
	}
	if _, ok := s.TVDBSeasonToAbsolute(95479, 1, -1); ok {
		t.Error("episode=-1 must return (_, false)")
	}
}

// ── AbsoluteToTMDBEpisode (inverse) ─────────────────────────────────────────
//
// These tests pin the import-side mapping that turns a fansub
// absolute number ("Show - 48") back into the TMDB (season, episode)
// tuple the importer needs to attach the file. Without this lookup,
// every anime grab fails silently with "could not parse season/
// episode from filename, skipping" — the JJK incident.

// Headline: JJK absolute 48 → TMDB S01E48. Locks the JJK regression
// from the import side. If this fails, the production bug is back.
func TestAbsoluteToTMDBEpisode_JujutsuKaisenAbs48(t *testing.T) {
	s := New("", nil)
	_ = s.loadFromBytes([]byte(fixtureXML))

	season, ep, ok := s.AbsoluteToTMDBEpisode(95479, 48)
	if !ok {
		t.Fatal("expected absolute=48 → TMDB conversion to succeed for JJK")
	}
	if season != 1 || ep != 48 {
		t.Errorf("got (S%dE%d), want (S01E48)", season, ep)
	}
}

// Within-cour absolute (ep 5 in JJK season 1) also resolves. Confirms
// the function isn't accidentally requiring a TMDB offset.
func TestAbsoluteToTMDBEpisode_JujutsuKaisenAbs5(t *testing.T) {
	s := New("", nil)
	_ = s.loadFromBytes([]byte(fixtureXML))
	season, ep, ok := s.AbsoluteToTMDBEpisode(95479, 5)
	if !ok || season != 1 || ep != 5 {
		t.Errorf("abs=5 → got (S%dE%d ok=%v), want (S01E05 ok=true)",
			season, ep, ok)
	}
}

// Cowboy Bebop has a single mapping with TMDBSeason=1 — single-cour
// shows must work the same as multi-cour ones.
func TestAbsoluteToTMDBEpisode_SingleCourAnime(t *testing.T) {
	s := New("", nil)
	_ = s.loadFromBytes([]byte(fixtureXML))
	// Cowboy Bebop tmdbtv=30991
	season, ep, ok := s.AbsoluteToTMDBEpisode(30991, 12)
	if !ok || season != 1 || ep != 12 {
		t.Errorf("Bebop abs=12 → got (S%dE%d ok=%v), want (S01E12 ok=true)",
			season, ep, ok)
	}
}

// Unknown TMDB ID returns (0, 0, false). Matches the contract Lookup
// uses elsewhere — caller falls back to the parser's natural output.
func TestAbsoluteToTMDBEpisode_UnknownTMDBIDReturnsFalse(t *testing.T) {
	s := New("", nil)
	_ = s.loadFromBytes([]byte(fixtureXML))
	season, ep, ok := s.AbsoluteToTMDBEpisode(123456, 5)
	if ok || season != 0 || ep != 0 {
		t.Errorf("unknown tmdbID → got (S%dE%d ok=%v), want (0,0,false)",
			season, ep, ok)
	}
}

// Defensive: zero / negative absolute fails without panic.
func TestAbsoluteToTMDBEpisode_RejectsZeroAndNegative(t *testing.T) {
	s := New("", nil)
	_ = s.loadFromBytes([]byte(fixtureXML))
	if _, _, ok := s.AbsoluteToTMDBEpisode(95479, 0); ok {
		t.Error("abs=0 must return ok=false")
	}
	if _, _, ok := s.AbsoluteToTMDBEpisode(95479, -1); ok {
		t.Error("abs=-1 must return ok=false")
	}
	if _, _, ok := s.AbsoluteToTMDBEpisode(0, 5); ok {
		t.Error("tmdbID=0 must return ok=false")
	}
}

// "movie" tvdbid + empty tmdbtv must NOT crash the parser. The XML
// has both these shapes scattered through it.
func TestParse_SkipsNonIndexableEntries(t *testing.T) {
	s := New("", nil)
	if err := s.loadFromBytes([]byte(fixtureXML)); err != nil {
		t.Fatalf("loadFromBytes: %v", err)
	}
	// 4 indexable entries (JJK x2, Cowboy Bebop, 86) → 3 distinct tmdb ids
	// after the first-wins dedup.
	if s.size() != 3 {
		t.Errorf("expected 3 indexed entries, got %d", s.size())
	}
}

// EpisodeOffset is a non-trivial field (used by future per-episode
// mapping); make sure it survives the parse round trip.
func TestParse_PreservesEpisodeOffset(t *testing.T) {
	s := New("", nil)
	_ = s.loadFromBytes([]byte(fixtureXML))

	m, ok := s.Lookup(100565) // 86 (2021)
	if !ok {
		t.Fatal("86 (2021) not found")
	}
	if m.EpisodeOffset != 11 {
		t.Errorf("episode offset: got %d, want 11", m.EpisodeOffset)
	}
}

// Empty / malformed XML must produce an error, not panic.
func TestParse_RejectsGarbage(t *testing.T) {
	s := New("", nil)
	if err := s.loadFromBytes([]byte("not xml at all")); err == nil {
		t.Error("expected error parsing non-XML; got nil")
	}
	// Empty document parses to zero entries — that's fine, not an error.
	if err := s.loadFromBytes([]byte(`<?xml version="1.0"?><anime-list></anime-list>`)); err != nil {
		t.Errorf("empty list should parse: %v", err)
	}
	if s.size() != 0 {
		t.Errorf("empty list size: %d, want 0", s.size())
	}
}

// LastLoaded must move forward after a successful load.
func TestLastLoaded_UpdatesOnLoad(t *testing.T) {
	s := New("", nil)
	if !s.LastLoaded().IsZero() {
		t.Fatal("expected zero LastLoaded before any load")
	}
	_ = s.loadFromBytes([]byte(fixtureXML))
	if s.LastLoaded().IsZero() {
		t.Error("LastLoaded should be set after a successful load")
	}
}

// End-to-end: serve XML from a local httptest server, hit refresh,
// confirm we ingested it AND wrote it to the disk cache.
func TestRefresh_FetchesAndPersists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(fixtureXML))
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	cachePath := filepath.Join(cacheDir, "anime-list-master.xml")
	s := New(cachePath, nil)
	// Override URL via local helper — keeps prod URL out of test runtime.
	origURL := upstreamURLForTest
	upstreamURLForTest = srv.URL
	defer func() { upstreamURLForTest = origURL }()

	if err := s.refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if !s.IsAnime(95479) {
		t.Error("JJK should be loaded after refresh")
	}
	stat, err := os.Stat(cachePath)
	if err != nil {
		t.Fatalf("cache file should exist: %v", err)
	}
	if stat.Size() == 0 {
		t.Error("cache file is empty")
	}
}

// loadFromDisk on a fresh path returns an error (no file yet) — that's
// the path Start uses to log "no cache yet" without aborting.
func TestLoadFromDisk_MissingFileIsAnError(t *testing.T) {
	s := New("/tmp/doesnt-exist-animelist-zfgh.xml", nil)
	if err := s.loadFromDisk(); err == nil {
		t.Error("expected error reading missing cache; got nil")
	}
}

// loadFromDisk on a present file restores the index.
func TestLoadFromDisk_RestoresIndex(t *testing.T) {
	cacheDir := t.TempDir()
	cachePath := filepath.Join(cacheDir, "cache.xml")
	if err := os.WriteFile(cachePath, []byte(fixtureXML), 0o644); err != nil {
		t.Fatalf("write seed cache: %v", err)
	}
	s := New(cachePath, nil)
	if err := s.loadFromDisk(); err != nil {
		t.Fatalf("loadFromDisk: %v", err)
	}
	if !s.IsAnime(95479) {
		t.Error("disk-loaded XML should yield the same lookups as in-memory")
	}
}

// atoiSafe is the only foot-shooter in the parser — guard it explicitly.
func TestAtoiSafe(t *testing.T) {
	cases := map[string]struct {
		in   string
		want int
		ok   bool
	}{
		"empty":         {"", 0, false},
		"basic":         {"42", 42, true},
		"zero":          {"0", 0, true},
		"negative":      {"-5", -5, true},
		"movie keyword": {"movie", 0, false},
		"alpha":         {"abc", 0, false},
		"trailing junk": {"42abc", 0, false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got, ok := atoiSafe(c.in)
			if got != c.want || ok != c.ok {
				t.Errorf("atoiSafe(%q) = (%d, %v), want (%d, %v)", c.in, got, ok, c.want, c.ok)
			}
		})
	}
}
