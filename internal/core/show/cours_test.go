package show

import (
	"database/sql"
	"testing"

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
// with the right windows (24, 23, 12 episodes).
func TestBuildCours_JujutsuKaisenSplitsCorrectly(t *testing.T) {
	episodes := makeEpisodes(59, false)
	parentMonitored := map[int]bool{1: true}

	cours := buildCours(jjkBounds, episodes, nil, nil, parentMonitored)

	if len(cours) != 3 {
		t.Fatalf("len(cours) = %d, want 3", len(cours))
	}
	wants := []struct {
		tvdb, count int
		first, last string
	}{
		{1, 24, "ep01", "ep24"},
		{2, 23, "ep25", "ep47"},
		{3, 12, "ep48", "ep59"},
	}
	for i, w := range wants {
		got := cours[i]
		if got.TVDBSeason != w.tvdb {
			t.Errorf("cour %d: TVDBSeason = %d, want %d", i, got.TVDBSeason, w.tvdb)
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

// A cour whose declared TMDBSeason has no episodes in our DB is
// hidden — the alternative ("0 episodes" phantom card) is worse UX.
// The Jujutsu Kaisen wire case: cour 2 declares tmdbseason=2 but TMDB
// itself has no Season 2 (cours 1+2 are folded into Season 1).
func TestBuildCours_DropsCourWhenTMDBSeasonAbsent(t *testing.T) {
	// Same bounds as JJK, but only TMDB Season 1 has episodes — cour 2
	// (declared tmdbseason=2) finds no episodes and must be dropped.
	bounds := []CourBound{
		{TVDBSeason: 1, TMDBSeason: 1, TMDBOffset: 0},
		{TVDBSeason: 2, TMDBSeason: 2, TMDBOffset: 0}, // missing season
		{TVDBSeason: 3, TMDBSeason: 1, TMDBOffset: 47},
	}
	episodes := makeEpisodes(59, false)
	cours := buildCours(bounds, episodes, nil, nil, map[int]bool{1: true})

	if len(cours) != 2 {
		t.Fatalf("len(cours) = %d, want 2 (cour 2 dropped, cours 1+3 kept)", len(cours))
	}
	if cours[0].TVDBSeason != 1 || cours[1].TVDBSeason != 3 {
		t.Errorf("got cours [%d, %d], want [1, 3]", cours[0].TVDBSeason, cours[1].TVDBSeason)
	}
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
