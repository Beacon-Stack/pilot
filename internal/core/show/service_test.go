package show

import (
	"context"
	"testing"

	db "github.com/beacon-stack/pilot/internal/db/generated"
)

// getSeasonsMock embeds db.Querier so only the methods GetSeasons touches
// need to be implemented.
type getSeasonsMock struct {
	db.Querier

	seasons  []db.Season
	episodes []db.Episode
	files    []db.EpisodeFile
}

func (m *getSeasonsMock) ListSeasonsBySeriesID(_ context.Context, _ string) ([]db.Season, error) {
	return m.seasons, nil
}

func (m *getSeasonsMock) ListEpisodesBySeriesID(_ context.Context, _ string) ([]db.Episode, error) {
	return m.episodes, nil
}

func (m *getSeasonsMock) ListEpisodeFilesBySeriesID(_ context.Context, _ string) ([]db.EpisodeFile, error) {
	return m.files, nil
}

func TestGetSeasons_PopulatesEpisodeCounts(t *testing.T) {
	const seriesID = "series-1"
	s1 := db.Season{ID: "s1", SeriesID: seriesID, SeasonNumber: 1, Monitored: true}
	s2 := db.Season{ID: "s2", SeriesID: seriesID, SeasonNumber: 2, Monitored: true}
	s0 := db.Season{ID: "s0", SeriesID: seriesID, SeasonNumber: 0, Monitored: false}

	mock := &getSeasonsMock{
		seasons: []db.Season{s1, s2, s0},
		episodes: []db.Episode{
			// Season 1: 10 episodes, 2 with file
			{ID: "e1", SeasonID: "s1", HasFile: true},
			{ID: "e2", SeasonID: "s1", HasFile: true},
			{ID: "e3", SeasonID: "s1", HasFile: false},
			{ID: "e4", SeasonID: "s1", HasFile: false},
			{ID: "e5", SeasonID: "s1", HasFile: false},
			{ID: "e6", SeasonID: "s1", HasFile: false},
			{ID: "e7", SeasonID: "s1", HasFile: false},
			{ID: "e8", SeasonID: "s1", HasFile: false},
			{ID: "e9", SeasonID: "s1", HasFile: false},
			{ID: "e10", SeasonID: "s1", HasFile: false},

			// Season 2: 8 episodes, 0 with file
			{ID: "e11", SeasonID: "s2", HasFile: false},
			{ID: "e12", SeasonID: "s2", HasFile: false},
			{ID: "e13", SeasonID: "s2", HasFile: false},
			{ID: "e14", SeasonID: "s2", HasFile: false},
			{ID: "e15", SeasonID: "s2", HasFile: false},
			{ID: "e16", SeasonID: "s2", HasFile: false},
			{ID: "e17", SeasonID: "s2", HasFile: false},
			{ID: "e18", SeasonID: "s2", HasFile: false},

			// Specials: 3 episodes, 1 with file
			{ID: "e19", SeasonID: "s0", HasFile: true},
			{ID: "e20", SeasonID: "s0", HasFile: false},
			{ID: "e21", SeasonID: "s0", HasFile: false},
		},
		files: []db.EpisodeFile{
			// Season 1 files: 1GB + 1.5GB = 2.5GB
			{EpisodeID: "e1", SizeBytes: 1_000_000_000},
			{EpisodeID: "e2", SizeBytes: 1_500_000_000},
			// Specials file: 500MB
			{EpisodeID: "e19", SizeBytes: 500_000_000},
		},
	}

	svc := NewService(mock, nil, nil, nil)
	got, err := svc.GetSeasons(context.Background(), seriesID)
	if err != nil {
		t.Fatalf("GetSeasons: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 seasons, got %d", len(got))
	}

	want := map[string]struct {
		total    int64
		withFile int64
		size     int64
	}{
		"s1": {10, 2, 2_500_000_000},
		"s2": {8, 0, 0},
		"s0": {3, 1, 500_000_000},
	}
	for _, season := range got {
		w, ok := want[season.ID]
		if !ok {
			t.Errorf("unexpected season %s", season.ID)
			continue
		}
		if season.EpisodeCount != w.total {
			t.Errorf("season %s: EpisodeCount = %d, want %d", season.ID, season.EpisodeCount, w.total)
		}
		if season.EpisodeFileCount != w.withFile {
			t.Errorf("season %s: EpisodeFileCount = %d, want %d", season.ID, season.EpisodeFileCount, w.withFile)
		}
		if season.TotalSizeBytes != w.size {
			t.Errorf("season %s: TotalSizeBytes = %d, want %d", season.ID, season.TotalSizeBytes, w.size)
		}
	}
}

func TestGetSeasons_EmptySeries(t *testing.T) {
	mock := &getSeasonsMock{
		seasons: []db.Season{{ID: "s1", SeriesID: "series-1", SeasonNumber: 1}},
	}
	svc := NewService(mock, nil, nil, nil)
	got, err := svc.GetSeasons(context.Background(), "series-1")
	if err != nil {
		t.Fatalf("GetSeasons: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 season, got %d", len(got))
	}
	if got[0].EpisodeCount != 0 || got[0].EpisodeFileCount != 0 {
		t.Errorf("empty series should have 0/0 counts, got %d/%d",
			got[0].EpisodeFileCount, got[0].EpisodeCount)
	}
}
