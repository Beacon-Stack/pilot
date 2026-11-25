package availability

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/beacon-stack/pilot/internal/core/queue"
	"github.com/beacon-stack/pilot/internal/core/show"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

func TestDecide(t *testing.T) {
	cases := []struct {
		name string
		f    Facts
		want State
	}{
		{"in the media server", Facts{HasFile: true, Aired: true, InMediaServer: true}, Available},
		{"in the media server, no file Pilot can see", Facts{Aired: true, Monitored: true, InMediaServer: true}, Available},
		{"downloading", Facts{Aired: true, Monitored: true, Downloading: true}, Downloading},
		{"on disk, not in the media server yet", Facts{HasFile: true, Aired: true}, Downloaded},
		{"not aired", Facts{Monitored: true}, Upcoming},
		{"aired, nothing yet", Facts{Aired: true, Monitored: true}, Wanted},
		{"aired, not monitored", Facts{Aired: true}, Unmonitored},
	}
	for _, c := range cases {
		if got := Decide(c.f); got != c.want {
			t.Errorf("%s: Decide = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestSummarise(t *testing.T) {
	cases := []struct {
		name   string
		counts map[State]int
		want   State
	}{
		{"everything aired is in the media server", map[State]int{Available: 20, Upcoming: 3, Unmonitored: 2}, Available},
		{"one episode downloading", map[State]int{Available: 20, Downloaded: 2, Downloading: 1, Wanted: 1}, Downloading},
		{"some on disk, not in the media server", map[State]int{Available: 20, Downloaded: 2, Wanted: 1}, Downloaded},
		{"some still wanted", map[State]int{Available: 20, Wanted: 1}, Wanted},
		{"nothing aired yet", map[State]int{Upcoming: 8}, Upcoming},
		{"nothing monitored", map[State]int{Unmonitored: 8}, Unmonitored},
	}
	for _, c := range cases {
		if got := Summarise(c.counts); got != c.want {
			t.Errorf("%s: Summarise = %q, want %q", c.name, got, c.want)
		}
	}
}

type fakeLister struct {
	items []plugin.ShowItem
	err   error
}

func (f *fakeLister) ListShowItems(context.Context) ([]plugin.ShowItem, error) { return f.items, f.err }

type fakeServers struct{ listers []plugin.ShowLister }

func (f *fakeServers) ShowListers(context.Context) ([]plugin.ShowLister, error) {
	return f.listers, nil
}

type fakeQueue struct{ items []queue.Item }

func (f *fakeQueue) GetQueue(context.Context) ([]queue.Item, error) { return f.items, nil }

type fakeEpisodes map[string][]show.Episode

func (f fakeEpisodes) EpisodesOfSeries(_ context.Context, id string) ([]show.Episode, error) {
	return f[id], nil
}

func ep(series, id string, season, number int, air string, hasFile bool) show.Episode {
	return show.Episode{ID: id, SeriesID: series, SeasonNumber: season, EpisodeNumber: number, AirDate: air, Monitored: true, HasFile: hasFile}
}

func newTestService(plex *fakeLister, q []queue.Item, eps fakeEpisodes) *Service {
	svc := NewService(&fakeServers{listers: []plugin.ShowLister{plex}}, &fakeQueue{items: q}, eps, nil)
	svc.now = func() time.Time { return time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC) }
	return svc
}

func TestForEpisodesAndSeries(t *testing.T) {
	plex := &fakeLister{items: []plugin.ShowItem{{
		TVDBID: 500, Title: "Rings", Year: 2022,
		Episodes: []plugin.EpisodeNumber{{Season: 3, Episode: 1}, {Season: 3, Episode: 2}},
	}}}
	episodes := []show.Episode{
		ep("s1", "e1", 3, 1, "2026-11-11", true),  // in Plex
		ep("s1", "e2", 3, 2, "2026-11-11", false), // in Plex, no file Pilot sees
		ep("s1", "e3", 3, 3, "2026-11-11", true),  // on disk only
		ep("s1", "e4", 3, 4, "2026-11-18", false), // downloading
		ep("s1", "e5", 3, 5, "2026-11-18", false), // wanted
		ep("s1", "e6", 3, 6, "2026-12-02", false), // not aired
	}
	q := []queue.Item{{SeriesID: "s1", EpisodeID: "e4", SeasonNumber: -1, Status: "downloading", Size: 100, DownloadedBytes: 40}}
	svc := newTestService(plex, q, fakeEpisodes{"s1": episodes})
	if err := svc.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	// Pilot knows the show by TMDB and TVDB ID; Plex only gave a TVDB ID.
	series := show.Series{ID: "s1", TMDBID: 84773, TVDBID: 500, Title: "The Rings", Year: 2022}

	got := svc.ForEpisodes(context.Background(), series, episodes)
	want := map[string]State{"e1": Available, "e2": Available, "e3": Downloaded, "e4": Downloading, "e5": Wanted, "e6": Upcoming}
	for id, state := range want {
		if got[id].State != state {
			t.Errorf("%s: state = %q, want %q", id, got[id].State, state)
		}
	}
	if got["e4"].Progress != 0.4 {
		t.Errorf("download progress = %v, want 0.4", got["e4"].Progress)
	}

	sum := svc.ForSeries(context.Background(), []show.Series{series})["s1"]
	if sum.State != Downloading || sum.Available != 2 || sum.Progress != 0.4 {
		t.Errorf("series = %+v, want downloading, 2 available, progress 0.4", sum)
	}
}

// A season pack covers the episodes of its season that have no file yet,
// and nothing in other seasons.
func TestSeasonPackCoversItsSeason(t *testing.T) {
	episodes := []show.Episode{
		ep("s1", "a", 1, 1, "2020-01-01", false),
		ep("s1", "b", 2, 1, "2021-01-01", false),
		ep("s1", "c", 2, 2, "2021-01-08", true),
	}
	q := []queue.Item{{SeriesID: "s1", SeasonNumber: 2, Status: "downloading", Size: 100, DownloadedBytes: 10}}
	svc := newTestService(&fakeLister{}, q, fakeEpisodes{"s1": episodes})

	got := svc.ForEpisodes(context.Background(), show.Series{ID: "s1"}, episodes)
	want := map[string]State{"a": Wanted, "b": Downloading, "c": Downloaded}
	for id, state := range want {
		if got[id].State != state {
			t.Errorf("%s: state = %q, want %q", id, got[id].State, state)
		}
	}
}

// With no media server set up, a file on disk is as far as an episode goes.
func TestNoMediaServer(t *testing.T) {
	episodes := []show.Episode{ep("s1", "a", 1, 1, "2020-01-01", true)}
	svc := NewService(&fakeServers{}, nil, fakeEpisodes{"s1": episodes}, nil)
	if err := svc.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if got := svc.ForSeries(context.Background(), []show.Series{{ID: "s1"}})["s1"]; got.State != Downloaded || got.Available != 0 {
		t.Errorf("series = %+v, want downloaded with 0 available", got)
	}
}

// A media server that is briefly unreachable must not turn every episode
// back to "downloaded".
func TestRefresh_KeepsLastKnownContentsOnFailure(t *testing.T) {
	plex := &fakeLister{items: []plugin.ShowItem{{TMDBID: 7, Episodes: []plugin.EpisodeNumber{{Season: 1, Episode: 1}}}}}
	episodes := []show.Episode{ep("s1", "a", 1, 1, "2020-01-01", true)}
	svc := newTestService(plex, nil, fakeEpisodes{"s1": episodes})
	if err := svc.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	plex.items, plex.err = nil, errors.New("connection refused")
	if err := svc.Refresh(context.Background()); err == nil {
		t.Fatal("Refresh with a failing server = nil, want an error")
	}
	got := svc.ForEpisodes(context.Background(), show.Series{ID: "s1", TMDBID: 7}, episodes)
	if got["a"].State != Available {
		t.Errorf("state after a failed refresh = %q, want available", got["a"].State)
	}
}

func TestNilService(t *testing.T) {
	var svc *Service
	if got := svc.ForSeries(context.Background(), []show.Series{{ID: "a"}}); len(got) != 0 {
		t.Errorf("ForSeries on a nil Service = %v, want nothing", got)
	}
	if got := svc.ForEpisodes(context.Background(), show.Series{}, []show.Episode{{ID: "a"}}); len(got) != 0 {
		t.Errorf("ForEpisodes on a nil Service = %v, want nothing", got)
	}
}
