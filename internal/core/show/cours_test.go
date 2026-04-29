package show

import (
	"database/sql"
	"testing"
	"time"

	db "github.com/beacon-stack/pilot/internal/db/generated"
)

// jjkBounds mirrors the live Anime-Lists XML for Jujutsu Kaisen
// (TMDB id 95479). Cour 1 is episodes 1-24, cour 2 is 25-47, cour 3
// starts at TMDB-relative 48. All three cours map to TMDB Season 1.
var jjkBounds = []CourBound{
	{TVDBSeason: 1, TMDBSeason: 1, TMDBOffset: 0, Name: "Jujutsu Kaisen"},
	{TVDBSeason: 2, TMDBSeason: 1, TMDBOffset: 24, Name: "Jujutsu Kaisen 2"},
	{TVDBSeason: 3, TMDBSeason: 1, TMDBOffset: 47, Name: "Jujutsu Kaisen Shimetsu Kaiyuu"},
}

// makeEpisodes generates n episodes in TMDB Season 1 numbered 1..n.
// Episode IDs are deterministic ("ep1".."epN") so tests can assert
// on EpisodeIDs without caring about UUID generation.
func makeEpisodes(n int, hasFile bool) []db.Episode {
	out := make([]db.Episode, n)
	for i := 0; i < n; i++ {
		out[i] = db.Episode{
			ID:             episodeID(i + 1),
			SeasonNumber:   1,
			EpisodeNumber:  int32(i + 1),
			HasFile:        hasFile,
			AbsoluteNumber: sql.NullInt32{Int32: int32(i + 1), Valid: true},
		}
	}
	return out
}

func episodeID(n int) string {
	if n < 10 {
		return "ep0" + string(rune('0'+n))
	}
	// crude two-digit formatter — fine for tests with <100 episodes.
	tens := n / 10
	ones := n % 10
	return "ep" + string(rune('0'+tens)) + string(rune('0'+ones))
}

// Headline test: JJK with all 59 episodes available → three cours
// with the right windows (24, 23, 12 episodes), each carrying the
// metadata the UI needs for cour-relative numbering and per-cour
// episode fetching.
func TestBuildCours_JujutsuKaisenSplitsCorrectly(t *testing.T) {
	episodes := makeEpisodes(59, false)
	parentMonitored := map[int]bool{1: true}

	cours := buildCours(jjkBounds, episodes, nil, nil, parentMonitored)

	if len(cours) != 3 {
		t.Fatalf("len(cours) = %d, want 3", len(cours))
	}
	wants := []struct {
		tvdb, count, tmdbSeason, offset int
		first, last                     string
	}{
		{1, 24, 1, 0, "ep01", "ep24"},
		{2, 23, 1, 24, "ep25", "ep47"},
		{3, 12, 1, 47, "ep48", "ep59"},
	}
	for i, w := range wants {
		got := cours[i]
		if got.TVDBSeason != w.tvdb {
			t.Errorf("cour %d: TVDBSeason = %d, want %d", i, got.TVDBSeason, w.tvdb)
		}
		if got.TMDBSeason != w.tmdbSeason {
			t.Errorf("cour %d: TMDBSeason = %d, want %d", i, got.TMDBSeason, w.tmdbSeason)
		}
		if got.EpisodeOffset != w.offset {
			t.Errorf("cour %d: EpisodeOffset = %d, want %d", i, got.EpisodeOffset, w.offset)
		}
		if got.EpisodeCount != int64(w.count) {
			t.Errorf("cour %d: EpisodeCount = %d, want %d", i, got.EpisodeCount, w.count)
		}
		if len(got.EpisodeIDs) != w.count {
			t.Errorf("cour %d: len(EpisodeIDs) = %d, want %d", i, len(got.EpisodeIDs), w.count)
		}
		if len(got.EpisodeIDs) > 0 && got.EpisodeIDs[0] != w.first {
			t.Errorf("cour %d: first ep = %q, want %q", i, got.EpisodeIDs[0], w.first)
		}
		if len(got.EpisodeIDs) > 0 && got.EpisodeIDs[len(got.EpisodeIDs)-1] != w.last {
			t.Errorf("cour %d: last ep = %q, want %q", i, got.EpisodeIDs[len(got.EpisodeIDs)-1], w.last)
		}
	}
}

// Specials must carry TMDBSeason=0 and EpisodeOffset=0 so the UI
// fetches /seasons/0/episodes (where they actually live) rather than
// /seasons/1/episodes (which would return 0 matches).
func TestBuildCours_SpecialsCarryTMDBSeasonZero(t *testing.T) {
	episodes := makeEpisodes(59, false)
	episodes = append(episodes,
		db.Episode{ID: "sp01", SeasonNumber: 0, EpisodeNumber: 1},
	)
	cours := buildCours(jjkBounds, episodes, nil, nil, map[int]bool{0: true, 1: true})

	if cours[0].TVDBSeason != 0 {
		t.Fatalf("expected specials first; got TVDBSeason=%d", cours[0].TVDBSeason)
	}
	if cours[0].TMDBSeason != 0 {
		t.Errorf("specials TMDBSeason = %d, want 0", cours[0].TMDBSeason)
	}
	if cours[0].EpisodeOffset != 0 {
		t.Errorf("specials EpisodeOffset = %d, want 0", cours[0].EpisodeOffset)
	}
}

// Specials (TMDB Season 0) must be returned as their own bucket
// before the cours, regardless of cour structure.
func TestBuildCours_SpecialsLeadAsSeparateBucket(t *testing.T) {
	episodes := makeEpisodes(59, false)
	episodes = append(episodes,
		db.Episode{ID: "sp01", SeasonNumber: 0, EpisodeNumber: 1},
		db.Episode{ID: "sp02", SeasonNumber: 0, EpisodeNumber: 2},
	)
	cours := buildCours(jjkBounds, episodes, nil, nil, map[int]bool{0: false, 1: true})

	if len(cours) != 4 {
		t.Fatalf("len(cours) = %d, want 4 (specials + 3 cours)", len(cours))
	}
	if cours[0].TVDBSeason != 0 {
		t.Errorf("cours[0].TVDBSeason = %d, want 0 (specials)", cours[0].TVDBSeason)
	}
	if cours[0].EpisodeCount != 2 {
		t.Errorf("specials EpisodeCount = %d, want 2", cours[0].EpisodeCount)
	}
	if cours[0].Monitored {
		t.Errorf("specials should inherit parent monitored=false; got true")
	}
}

// Mid-season state: only cours 1 and 2 have all episodes; cour 3 has
// only 6 of its 12 yet (the show is still airing). Cour 3's count
// should reflect only the present episodes, not the announced total.
func TestBuildCours_MidAiringPartialLastCour(t *testing.T) {
	episodes := makeEpisodes(53, false) // 24 + 23 + 6 of cour 3
	cours := buildCours(jjkBounds, episodes, nil, nil, map[int]bool{1: true})

	if len(cours) != 3 {
		t.Fatalf("len(cours) = %d, want 3", len(cours))
	}
	wants := []int64{24, 23, 6}
	for i, w := range wants {
		if cours[i].EpisodeCount != w {
			t.Errorf("cour %d: EpisodeCount = %d, want %d", i, cours[i].EpisodeCount, w)
		}
	}
}

// File-size accounting: total per-cour bytes should sum the file rows
// keyed by EpisodeID. Verify across cour boundaries.
func TestBuildCours_TotalSizeBytesAccumulatesPerCour(t *testing.T) {
	episodes := makeEpisodes(59, true)
	sizes := map[string]int64{}
	for _, ep := range episodes {
		sizes[ep.ID] = 100_000_000 // 100MB each → cour 1 = 2.4GB, cour 2 = 2.3GB, cour 3 = 1.2GB
	}
	cours := buildCours(jjkBounds, episodes, sizes, nil, map[int]bool{1: true})

	wants := []int64{2_400_000_000, 2_300_000_000, 1_200_000_000}
	for i, w := range wants {
		if cours[i].TotalSizeBytes != w {
			t.Errorf("cour %d: TotalSizeBytes = %d, want %d", i, cours[i].TotalSizeBytes, w)
		}
		if cours[i].EpisodeFileCount != cours[i].EpisodeCount {
			t.Errorf("cour %d: EpisodeFileCount = %d, want %d (HasFile=true on all)",
				i, cours[i].EpisodeFileCount, cours[i].EpisodeCount)
		}
	}
}

// Monitor override semantics:
//   - explicit override wins over parent
//   - missing override falls back to parent
//   - missing parent falls back to default (true)
func TestBuildCours_MonitorOverrideWinsOverParent(t *testing.T) {
	episodes := makeEpisodes(59, false)
	parent := map[int]bool{1: true}
	overrides := map[int]bool{2: false} // user paused cour 2

	cours := buildCours(jjkBounds, episodes, nil, overrides, parent)

	if !cours[0].Monitored {
		t.Errorf("cour 1 inherits parent=true; got %v", cours[0].Monitored)
	}
	if cours[1].Monitored {
		t.Errorf("cour 2 has override=false; got monitored=true")
	}
	if !cours[2].Monitored {
		t.Errorf("cour 3 inherits parent=true; got %v", cours[2].Monitored)
	}
}

func TestBuildCours_NoParentNoOverrideDefaultsTrue(t *testing.T) {
	episodes := makeEpisodes(24, false)
	bounds := []CourBound{{TVDBSeason: 1, TMDBSeason: 1, TMDBOffset: 0}}

	cours := buildCours(bounds, episodes, nil, nil, nil)
	if !cours[0].Monitored {
		t.Errorf("default monitored should be true; got false")
	}
}

// JJK headline regression with realistic air dates. Cour 2 declares
// tmdbseason=2 but TMDB has no Season 2; the algorithm must fold cour
// 2 into TMDB Season 1, then use the air-date gap between ep 24
// (Mar 2021) and ep 25 (Jul 2023) to split cours 1 and 2.
func TestBuildCours_AirDateGapSplitsFoldedJJKCours(t *testing.T) {
	bounds := []CourBound{
		{TVDBSeason: 1, TMDBSeason: 1, TMDBOffset: 0, Name: "Jujutsu Kaisen"},
		{TVDBSeason: 2, TMDBSeason: 2, TMDBOffset: 0, Name: "Jujutsu Kaisen 2"},
		{TVDBSeason: 3, TMDBSeason: 1, TMDBOffset: 47, Name: "JJK Shimetsu Kaiyuu"},
	}
	episodes := makeJJKEpisodes()
	cours := buildCours(bounds, episodes, nil, nil, map[int]bool{1: true})

	if len(cours) != 3 {
		t.Fatalf("len(cours) = %d, want 3 (cour 2 folded, all three surface)", len(cours))
	}
	wants := []struct {
		tvdb, count, offset int
		firstEp, lastEp     int
	}{
		{1, 24, 0, 1, 24},
		{2, 23, 24, 25, 47},
		{3, 12, 47, 48, 59},
	}
	for i, w := range wants {
		got := cours[i]
		if got.TVDBSeason != w.tvdb {
			t.Errorf("cour %d: TVDBSeason = %d, want %d", i, got.TVDBSeason, w.tvdb)
		}
		if got.EpisodeCount != int64(w.count) {
			t.Errorf("cour %d: EpisodeCount = %d, want %d", i, got.EpisodeCount, w.count)
		}
		if got.EpisodeOffset != w.offset {
			t.Errorf("cour %d: EpisodeOffset = %d, want %d", i, got.EpisodeOffset, w.offset)
		}
		if got.TMDBSeason != 1 {
			t.Errorf("cour %d: TMDBSeason = %d, want 1 (folded into S1)", i, got.TMDBSeason)
		}
	}
}

// When a cour has no sibling at all (all cours' declared TMDB seasons
// are empty), there's nowhere to fold to. The cour is dropped — the
// caller produces no phantom card.
func TestBuildCours_OrphanCourIsDropped(t *testing.T) {
	bounds := []CourBound{
		{TVDBSeason: 1, TMDBSeason: 99, TMDBOffset: 0, Name: "Orphan"},
	}
	cours := buildCours(bounds, makeEpisodes(10, false), nil, nil, nil)
	if len(cours) != 0 {
		t.Errorf("orphan cour should be dropped; got %d cours", len(cours))
	}
}

// When air dates are missing or parse-broken, fall back to even
// spacing so we still produce N+1 cours (better than dropping).
func TestBuildCours_FallsBackToEvenSplitWhenNoAirDates(t *testing.T) {
	bounds := []CourBound{
		{TVDBSeason: 1, TMDBSeason: 1, TMDBOffset: 0},
		{TVDBSeason: 2, TMDBSeason: 2, TMDBOffset: 0}, // forces fold
	}
	// 24 episodes, no air dates → no gap detection possible.
	episodes := makeEpisodes(24, false)
	cours := buildCours(bounds, episodes, nil, nil, map[int]bool{1: true})
	if len(cours) != 2 {
		t.Fatalf("len(cours) = %d, want 2", len(cours))
	}
	// Even split: roughly 12/12.
	if cours[0].EpisodeCount+cours[1].EpisodeCount != 24 {
		t.Errorf("even-split total = %d, want 24",
			cours[0].EpisodeCount+cours[1].EpisodeCount)
	}
	if cours[0].EpisodeCount == 0 || cours[1].EpisodeCount == 0 {
		t.Errorf("even split must produce two non-empty cours; got %d/%d",
			cours[0].EpisodeCount, cours[1].EpisodeCount)
	}
}

// makeJJKEpisodes produces 59 episodes in TMDB Season 1 with realistic
// air dates: cour 1 = weekly Oct 2020–Mar 2021, cour 2 = weekly
// Jul 2023–Dec 2023, cour 3 = weekly Jan 2026+. The gap between ep 24
// and ep 25 spans ~28 months, the gap between 47 and 48 spans ~25
// months — both far exceed any in-cour weekly cadence.
func makeJJKEpisodes() []db.Episode {
	base := []time.Time{
		// cour 1: 24 weekly episodes starting 2020-10-03
		time.Date(2020, 10, 3, 0, 0, 0, 0, time.UTC),
		// cour 2: 23 weekly episodes starting 2023-07-06
		time.Date(2023, 7, 6, 0, 0, 0, 0, time.UTC),
		// cour 3: 12 weekly episodes starting 2026-01-08
		time.Date(2026, 1, 8, 0, 0, 0, 0, time.UTC),
	}
	courLengths := []int{24, 23, 12}
	out := make([]db.Episode, 0, 59)
	epNum := 1
	for ci, courLen := range courLengths {
		for i := 0; i < courLen; i++ {
			d := base[ci].AddDate(0, 0, 7*i)
			out = append(out, db.Episode{
				ID:             episodeID(epNum),
				SeasonNumber:   1,
				EpisodeNumber:  int32(epNum),
				AirDate:        sql.NullString{String: d.Format("2006-01-02"), Valid: true},
				AbsoluteNumber: sql.NullInt32{Int32: int32(epNum), Valid: true},
			})
			epNum++
		}
	}
	return out
}

// Single-cour anime: bounds has one entry. Behaviour should reduce to
// "all episodes go in one cour", which is identical to the current
// non-cour view — no regression for "normal" anime.
func TestBuildCours_SingleCourEqualsAllEpisodes(t *testing.T) {
	episodes := makeEpisodes(12, false)
	bounds := []CourBound{{TVDBSeason: 1, TMDBSeason: 1, TMDBOffset: 0, Name: "Solo Show"}}
	cours := buildCours(bounds, episodes, nil, nil, map[int]bool{1: true})

	if len(cours) != 1 {
		t.Fatalf("single-cour len = %d, want 1", len(cours))
	}
	if cours[0].EpisodeCount != 12 {
		t.Errorf("single-cour EpisodeCount = %d, want 12", cours[0].EpisodeCount)
	}
}
