package jobs

// Regression tests for RSS decisions. Before selectRSSGrabs, RSS sync took
// the first matching release in feed order, never checked the quality
// profile (a 480p release could be grabbed for a 1080p profile), and
// skipped every episode that had a file, so nothing was ever upgraded.

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/beacon-stack/pilot/internal/core/indexer"
	"github.com/beacon-stack/pilot/internal/core/quality"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	releaseparser "github.com/beacon-stack/pilot/internal/parser"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

// rssRelease builds a release the way GetRecent does: quality parsed
// from the title.
func rssRelease(title, guid string, seeds int) indexer.SearchResult {
	q := releaseparser.Parse(title).Quality()
	return indexer.SearchResult{
		Release:      plugin.Release{Title: title, GUID: guid, Seeds: seeds, Quality: q},
		QualityScore: q.Score(),
	}
}

type rssFixture struct {
	series   db.Series
	episodes []db.Episode
	current  map[string]*plugin.Quality // episode ID -> current file quality
	profile  *quality.Profile
	profErr  error
	active   map[string]bool
	recent   map[string]bool
	blocked  map[string]bool
}

func newRSSFixture() *rssFixture {
	return &rssFixture{
		series: db.Series{ID: "s1", Title: "Show", QualityProfileID: "qp"},
		episodes: []db.Episode{
			{ID: "e1", SeriesID: "s1", SeasonNumber: 1, EpisodeNumber: 1, Monitored: true},
			{ID: "e2", SeriesID: "s1", SeasonNumber: 1, EpisodeNumber: 2, Monitored: true},
		},
		current: map[string]*plugin.Quality{},
		profile: &quality.Profile{
			Cutoff: plugin.Quality{Resolution: plugin.Resolution1080p, Source: plugin.SourceWEBDL},
		},
		active:  map[string]bool{},
		recent:  map[string]bool{},
		blocked: map[string]bool{},
	}
}

func (f *rssFixture) withFile(epID string, q plugin.Quality) {
	for i := range f.episodes {
		if f.episodes[i].ID == epID {
			f.episodes[i].HasFile = true
		}
	}
	f.current[epID] = &q
}

func (f *rssFixture) run(releases ...indexer.SearchResult) []rssPick {
	return selectRSSGrabs(context.Background(), releases, rssEnv{
		seriesByTitle: map[string]db.Series{normalizeRSSTitle(f.series.Title): f.series},
		episodes: func(context.Context, string) ([]db.Episode, error) {
			return f.episodes, nil
		},
		currentQuality: func(_ context.Context, id string) (*plugin.Quality, error) {
			return f.current[id], nil
		},
		profile: func(context.Context, string) (*quality.Profile, error) {
			return f.profile, f.profErr
		},
		active:            f.active,
		recentlyCompleted: f.recent,
		blocked:           func(_ context.Context, guid string) bool { return f.blocked[guid] },
		logger:            slog.New(slog.DiscardHandler),
	})
}

func titles(picks []rssPick) []string {
	out := make([]string, len(picks))
	for i, p := range picks {
		out[i] = p.Release.Title
	}
	return out
}

// Headline: a release below the profile is never grabbed.
func TestRSS_RejectsReleaseBelowProfile(t *testing.T) {
	f := newRSSFixture()
	picks := f.run(rssRelease("Show.S01E01.480p.HDTV.x264-GRP", "g1", 900))
	if len(picks) != 0 {
		t.Fatalf("grabbed below-profile release: %v", titles(picks))
	}
}

// Headline: the best acceptable release wins, not the first in the feed.
func TestRSS_PicksBestReleaseNotFirst(t *testing.T) {
	f := newRSSFixture()
	picks := f.run(
		rssRelease("Show.S01E01.1080p.WEB-DL.x264-GRP", "g1", 50),
		rssRelease("Show.S01E01.1080p.BluRay.x264-GRP", "g2", 5),
		rssRelease("Show.S01E01.2160p.WEB-DL.x265-GRP", "g3", 5),
	)
	if len(picks) != 1 || picks[0].Release.GUID != "g3" {
		t.Fatalf("picks = %v, want the 2160p release only", titles(picks))
	}
	if picks[0].Upgrade {
		t.Fatal("a missing episode is not an upgrade")
	}
}

func TestRSS_OnePickPerEpisode(t *testing.T) {
	f := newRSSFixture()
	picks := f.run(
		rssRelease("Show.S01E01.1080p.WEB-DL.x264-GRP", "g1", 50),
		rssRelease("Show.S01E02.1080p.WEB-DL.x264-GRP", "g2", 50),
		rssRelease("Show.S01E01.1080p.WEB-DL.x264-OTHER", "g3", 10),
	)
	if len(picks) != 2 {
		t.Fatalf("picks = %v, want one per episode", titles(picks))
	}
}

// Episodes with a file used to be skipped outright.
func TestRSS_UpgradesBelowCutoff(t *testing.T) {
	f := newRSSFixture()
	f.profile.Cutoff = plugin.Quality{Resolution: plugin.Resolution1080p, Source: plugin.SourceBluRay}
	f.withFile("e1", plugin.Quality{Resolution: plugin.Resolution1080p, Source: plugin.SourceWEBDL, Revision: 1})
	picks := f.run(rssRelease("Show.S01E01.1080p.BluRay.x264-GRP", "g1", 20))
	if len(picks) != 1 || !picks[0].Upgrade {
		t.Fatalf("picks = %v, want one upgrade", titles(picks))
	}
}

func TestRSS_NoUpgradeWhenCutoffMetAndUpgradesOff(t *testing.T) {
	f := newRSSFixture()
	f.withFile("e1", plugin.Quality{Resolution: plugin.Resolution1080p, Source: plugin.SourceWEBDL, Revision: 1})
	picks := f.run(rssRelease("Show.S01E01.1080p.BluRay.x264-GRP", "g1", 20))
	if len(picks) != 0 {
		t.Fatalf("upgraded past the cutoff with upgrades off: %v", titles(picks))
	}
}

func TestRSS_ProperOfCurrentQualityIsGrabbed(t *testing.T) {
	f := newRSSFixture()
	f.withFile("e1", plugin.Quality{Resolution: plugin.Resolution1080p, Source: plugin.SourceWEBDL, Revision: 1})
	picks := f.run(rssRelease("Show.S01E01.PROPER.1080p.WEB-DL.x264-GRP", "g1", 20))
	if len(picks) != 1 {
		t.Fatalf("PROPER of the current quality not grabbed: %v", titles(picks))
	}
}

// The PROPER already on disk must not be grabbed again every 15 minutes.
func TestRSS_ProperAlreadyOnDiskIsNotRegrabbed(t *testing.T) {
	f := newRSSFixture()
	f.withFile("e1", plugin.Quality{Resolution: plugin.Resolution1080p, Source: plugin.SourceWEBDL, Revision: 2})
	picks := f.run(rssRelease("Show.S01E01.PROPER.1080p.WEB-DL.x264-GRP", "g1", 20))
	if len(picks) != 0 {
		t.Fatalf("re-grabbed the PROPER already on disk: %v", titles(picks))
	}
}

func TestRSS_UnknownCurrentQualityIsNotUpgraded(t *testing.T) {
	f := newRSSFixture()
	f.withFile("e1", plugin.Quality{})
	picks := f.run(rssRelease("Show.S01E01.2160p.WEB-DL.x265-GRP", "g1", 20))
	if len(picks) != 0 {
		t.Fatalf("upgraded a file of unknown quality: %v", titles(picks))
	}
}

func TestRSS_HasFileWithoutRecordIsSkipped(t *testing.T) {
	f := newRSSFixture()
	f.episodes[0].HasFile = true // no entry in f.current
	picks := f.run(rssRelease("Show.S01E01.2160p.WEB-DL.x265-GRP", "g1", 20))
	if len(picks) != 0 {
		t.Fatalf("grabbed for an episode whose file record is missing: %v", titles(picks))
	}
}

func TestRSS_Guards(t *testing.T) {
	cases := map[string]func(f *rssFixture){
		"unmonitored":        func(f *rssFixture) { f.episodes[0].Monitored = false },
		"active grab":        func(f *rssFixture) { f.active["e1"] = true },
		"recently completed": func(f *rssFixture) { f.recent["e1"] = true },
		"blocklisted":        func(f *rssFixture) { f.blocked["g1"] = true },
		"profile load error": func(f *rssFixture) { f.profErr = errors.New("db down") },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := newRSSFixture()
			mutate(f)
			if picks := f.run(rssRelease("Show.S01E01.1080p.WEB-DL.x264-GRP", "g1", 20)); len(picks) != 0 {
				t.Fatalf("grabbed despite guard: %v", titles(picks))
			}
		})
	}
}

// A blocklisted best release falls back to the next acceptable one.
func TestRSS_BlocklistedBestFallsBackToNext(t *testing.T) {
	f := newRSSFixture()
	f.blocked["g2"] = true
	picks := f.run(
		rssRelease("Show.S01E01.1080p.WEB-DL.x264-GRP", "g1", 50),
		rssRelease("Show.S01E01.2160p.WEB-DL.x265-GRP", "g2", 50),
	)
	if len(picks) != 1 || picks[0].Release.GUID != "g1" {
		t.Fatalf("picks = %v, want the non-blocklisted 1080p", titles(picks))
	}
}
