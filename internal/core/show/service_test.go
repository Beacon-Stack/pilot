package show

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/metadata/tmdbtv"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

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

	svc := NewService(mock, nil, nil, nil, nil)
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
	svc := NewService(mock, nil, nil, nil, nil)
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

// rowToSeries: alternate_titles JSON parsing

func TestRowToSeries_AlternateTitlesEmptyArray(t *testing.T) {
	row := mkSeriesRow("s1", `[]`)
	got, err := rowToSeries(row)
	if err != nil {
		t.Fatalf("rowToSeries: %v", err)
	}
	if len(got.AlternateTitles) != 0 {
		t.Errorf("AlternateTitles = %v, want empty", got.AlternateTitles)
	}
}

func TestRowToSeries_AlternateTitlesNullJSON(t *testing.T) {
	// Defensive: a row written by an older code path could carry
	// literal JSON null. Should produce empty slice, not crash.
	row := mkSeriesRow("s1", `null`)
	got, err := rowToSeries(row)
	if err != nil {
		t.Fatalf("rowToSeries on null: %v", err)
	}
	if len(got.AlternateTitles) != 0 {
		t.Errorf("AlternateTitles = %v, want empty", got.AlternateTitles)
	}
}

func TestRowToSeries_AlternateTitlesEmptyBytes(t *testing.T) {
	// Defensive: rare DB driver edge cases hand back zero-length JSON.
	row := mkSeriesRow("s1", ``)
	got, err := rowToSeries(row)
	if err != nil {
		t.Fatalf("rowToSeries on empty bytes: %v", err)
	}
	if len(got.AlternateTitles) != 0 {
		t.Errorf("AlternateTitles = %v, want empty", got.AlternateTitles)
	}
}

func TestRowToSeries_AlternateTitlesMalformedJSON(t *testing.T) {
	// Bad JSON in the column shouldn't fail series fetch — we
	// gracefully degrade to empty alternates so the strict-title
	// fallback still works.
	row := mkSeriesRow("s1", `{not json`)
	got, err := rowToSeries(row)
	if err != nil {
		t.Fatalf("rowToSeries should tolerate bad JSON, got error: %v", err)
	}
	if len(got.AlternateTitles) != 0 {
		t.Errorf("AlternateTitles on bad JSON = %v, want empty", got.AlternateTitles)
	}
}

func TestRowToSeries_AlternateTitlesPopulated(t *testing.T) {
	row := mkSeriesRow("s1", `["Star Wars: Andor","Andor: A Star Wars Story"]`)
	got, err := rowToSeries(row)
	if err != nil {
		t.Fatalf("rowToSeries: %v", err)
	}
	want := []string{"Star Wars: Andor", "Andor: A Star Wars Story"}
	if len(got.AlternateTitles) != len(want) {
		t.Fatalf("AlternateTitles len = %d, want %d", len(got.AlternateTitles), len(want))
	}
	for i := range want {
		if got.AlternateTitles[i] != want[i] {
			t.Errorf("AlternateTitles[%d] = %q, want %q", i, got.AlternateTitles[i], want[i])
		}
	}
}

// mkSeriesRow returns a minimally-valid db.Series row with the supplied
// alternate_titles JSON. All other fields are filled with safe defaults.
func mkSeriesRow(id string, altTitles string) db.Series {
	now := "2025-01-01T00:00:00Z"
	return db.Series{
		ID:              id,
		TmdbID:          1,
		Title:           "Test",
		SortTitle:       "test",
		Year:            2024,
		GenresJson:      "[]",
		SeriesType:      "standard",
		AddedAt:         now,
		UpdatedAt:       now,
		AlternateTitles: altTitles,
	}
}

// fakeAnimeLookup is a minimal AnimeLookup for tests — IsAnime returns
// true exactly when the queried tmdb id is in the configured set.
type fakeAnimeLookup struct {
	hits map[int]bool
}

func (f fakeAnimeLookup) IsAnime(tmdbID int) bool { return f.hits[tmdbID] }

func (f fakeAnimeLookup) TVDBSeasonToAbsolute(_, _, _ int) (int, bool) {
	return 0, false
}

func (f fakeAnimeLookup) CourBounds(_ int) []CourBound { return nil }

// RefreshMetadata

// refreshMetadataMock implements just the queries RefreshMetadata calls.
type refreshMetadataMock struct {
	db.Querier

	getSeriesRow db.Series
	getSeriesErr error

	updatedRow         db.Series
	updateErr          error
	updateCalled       bool
	gotUpdateAlternate string

	// Anime backfill capture: records calls so tests can assert that
	// RefreshMetadata triggers the upgrade exactly once when warranted
	// and never when it shouldn't. The series_type write goes through
	// the existing UpdateSeries query (which sets several fields at
	// once) rather than a dedicated UpdateSeriesType — see
	// BackfillAnimeIfNeeded.
	gotSeriesUpdates   []db.UpdateSeriesParams
	episodes           []db.Episode // returned by ListEpisodesBySeriesID
	gotAbsoluteUpdates []db.UpdateEpisodeAbsoluteNumberParams
	gotEpisodeUpdates  []db.UpdateEpisodeParams
}

func (m *refreshMetadataMock) UpdateEpisode(_ context.Context, p db.UpdateEpisodeParams) (db.Episode, error) {
	m.gotEpisodeUpdates = append(m.gotEpisodeUpdates, p)
	return db.Episode{}, nil
}

func (m *refreshMetadataMock) GetSeries(_ context.Context, _ string) (db.Series, error) {
	return m.getSeriesRow, m.getSeriesErr
}

func (m *refreshMetadataMock) UpdateSeriesMetadata(_ context.Context, p db.UpdateSeriesMetadataParams) (db.Series, error) {
	m.updateCalled = true
	m.gotUpdateAlternate = p.AlternateTitles
	if m.updateErr != nil {
		return db.Series{}, m.updateErr
	}
	return m.updatedRow, nil
}

func (m *refreshMetadataMock) UpdateSeries(_ context.Context, p db.UpdateSeriesParams) (db.Series, error) {
	m.gotSeriesUpdates = append(m.gotSeriesUpdates, p)
	// Reflect the change back so the caller sees the upgraded type.
	out := m.getSeriesRow
	out.SeriesType = p.SeriesType
	return out, nil
}

func (m *refreshMetadataMock) ListEpisodesBySeriesID(_ context.Context, _ string) ([]db.Episode, error) {
	return m.episodes, nil
}

func (m *refreshMetadataMock) UpdateEpisodeAbsoluteNumber(_ context.Context, p db.UpdateEpisodeAbsoluteNumberParams) error {
	m.gotAbsoluteUpdates = append(m.gotAbsoluteUpdates, p)
	return nil
}

// stubMeta is a controllable MetadataProvider for refresh tests. Each
// hook returns the supplied result/error verbatim.
type stubMeta struct {
	getSeriesResult *tmdbtv.SeriesDetail
	getSeriesErr    error
	altTitles       []string
	altErr          error
	seasonEpisodes  []tmdbtv.EpisodeDetail
}

func (s *stubMeta) SearchSeries(context.Context, string, int) ([]tmdbtv.SearchResult, error) {
	return nil, nil
}
func (s *stubMeta) GetSeries(context.Context, int) (*tmdbtv.SeriesDetail, error) {
	return s.getSeriesResult, s.getSeriesErr
}
func (s *stubMeta) GetSeriesExtended(context.Context, int) (*tmdbtv.SeriesDetail, error) {
	return s.getSeriesResult, s.getSeriesErr
}
func (s *stubMeta) GetPerson(context.Context, int) (*tmdbtv.Person, error) {
	return nil, nil
}
func (s *stubMeta) GetPersonTVCredits(context.Context, int, string) ([]tmdbtv.PersonCredit, error) {
	return nil, nil
}
func (s *stubMeta) DiscoverTVByGenre(context.Context, int, int) (*tmdbtv.PaginatedSeries, error) {
	return &tmdbtv.PaginatedSeries{}, nil
}
func (s *stubMeta) GetTVGenres(context.Context) ([]tmdbtv.Genre, error) {
	return nil, nil
}
func (s *stubMeta) Browse(context.Context, tmdbtv.BrowseFilter) (*tmdbtv.PaginatedSeries, error) {
	return &tmdbtv.PaginatedSeries{}, nil
}
func (s *stubMeta) WatchProviders(context.Context, string) ([]tmdbtv.WatchProvider, error) {
	return nil, nil
}
func (s *stubMeta) SearchPaginated(context.Context, string, int) (*tmdbtv.PaginatedSeries, error) {
	return &tmdbtv.PaginatedSeries{}, nil
}
func (s *stubMeta) GetSeasonEpisodes(context.Context, int, int) ([]tmdbtv.EpisodeDetail, error) {
	return s.seasonEpisodes, nil
}
func (s *stubMeta) GetAlternativeTitles(context.Context, int) ([]string, error) {
	return s.altTitles, s.altErr
}

func TestRefreshMetadata_NoMetadataProvider(t *testing.T) {
	svc := NewService(&refreshMetadataMock{}, nil, nil, nil, discardLogger())
	_, err := svc.RefreshMetadata(context.Background(), "s1")
	if !errors.Is(err, ErrMetadataNotConfigured) {
		t.Errorf("err = %v, want ErrMetadataNotConfigured", err)
	}
}

func TestRefreshMetadata_SeriesNotFound(t *testing.T) {
	mock := &refreshMetadataMock{getSeriesErr: sql.ErrNoRows}
	meta := &stubMeta{getSeriesResult: testSeriesDetail(), altTitles: []string{}}
	svc := NewService(mock, meta, nil, nil, discardLogger())

	_, err := svc.RefreshMetadata(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
	if mock.updateCalled {
		t.Error("UpdateSeriesMetadata should not be called when GetSeries fails")
	}
}

func TestRefreshMetadata_GetSeriesDBFailure(t *testing.T) {
	mock := &refreshMetadataMock{getSeriesErr: errors.New("connection lost")}
	meta := &stubMeta{}
	svc := NewService(mock, meta, nil, nil, discardLogger())

	_, err := svc.RefreshMetadata(context.Background(), "s1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errors.Is(err, ErrNotFound) {
		t.Error("non-ErrNoRows DB error should not map to ErrNotFound")
	}
}

func TestRefreshMetadata_TMDBSeriesFetchFails(t *testing.T) {
	mock := &refreshMetadataMock{getSeriesRow: mkSeriesRow("s1", `[]`)}
	meta := &stubMeta{getSeriesErr: errors.New("tmdb 503")}
	svc := NewService(mock, meta, nil, nil, discardLogger())

	_, err := svc.RefreshMetadata(context.Background(), "s1")
	if err == nil {
		t.Fatal("expected error when GetSeries metadata fails, got nil")
	}
	if mock.updateCalled {
		t.Error("UpdateSeriesMetadata should not be called when metadata fetch fails")
	}
}

func TestRefreshMetadata_AlternateTitlesFetchFails_StillUpdates(t *testing.T) {
	// Per design: alt-titles fetch failure is non-fatal. The series row
	// is still refreshed (with empty alternates). The user can retry
	// later and pick up alternates if TMDB is healthy.
	row := mkSeriesRow("s1", `[]`)
	mock := &refreshMetadataMock{
		getSeriesRow: row,
		updatedRow:   row,
	}
	meta := &stubMeta{
		getSeriesResult: testSeriesDetail(),
		altErr:          errors.New("tmdb alt 500"),
	}
	svc := NewService(mock, meta, nil, nil, discardLogger())

	got, err := svc.RefreshMetadata(context.Background(), "s1")
	if err != nil {
		t.Fatalf("expected RefreshMetadata to swallow alt-fetch error, got: %v", err)
	}
	if !mock.updateCalled {
		t.Fatal("UpdateSeriesMetadata should still be called when only alt-fetch fails")
	}
	if string(mock.gotUpdateAlternate) != `[]` {
		t.Errorf("UpdateSeriesMetadata alt JSON = %q, want %q", mock.gotUpdateAlternate, `[]`)
	}
	if len(got.AlternateTitles) != 0 {
		t.Errorf("returned series alt titles = %v, want empty", got.AlternateTitles)
	}
}

func TestRefreshMetadata_HappyPath_StoresAlternates(t *testing.T) {
	row := mkSeriesRow("s1", `[]`)
	updatedRow := row
	updatedRow.AlternateTitles = `["Star Wars: Andor"]`
	mock := &refreshMetadataMock{
		getSeriesRow: row,
		updatedRow:   updatedRow,
	}
	meta := &stubMeta{
		getSeriesResult: testSeriesDetail(),
		altTitles:       []string{"Star Wars: Andor", "Andor: A Star Wars Story"},
	}
	svc := NewService(mock, meta, nil, nil, discardLogger())

	got, err := svc.RefreshMetadata(context.Background(), "s1")
	if err != nil {
		t.Fatalf("RefreshMetadata: %v", err)
	}
	// Verify the JSON marshalled into UpdateSeriesMetadata contains both.
	want := `["Star Wars: Andor","Andor: A Star Wars Story"]`
	if string(mock.gotUpdateAlternate) != want {
		t.Errorf("UpdateSeriesMetadata alt JSON = %q, want %q", mock.gotUpdateAlternate, want)
	}
	// Returned Series came from the mock's updated row, which has 1 alt.
	// We're verifying the round-trip path works end-to-end.
	if len(got.AlternateTitles) != 1 || got.AlternateTitles[0] != "Star Wars: Andor" {
		t.Errorf("returned series AlternateTitles = %v", got.AlternateTitles)
	}
}

func TestRefreshMetadata_NilAlternatesMarshalsAsEmptyArray(t *testing.T) {
	// Defensive: TMDB returns nil slice (not empty). We must persist
	// "[]" not "null" so the JSONB column stays valid for queries.
	row := mkSeriesRow("s1", `[]`)
	mock := &refreshMetadataMock{getSeriesRow: row, updatedRow: row}
	meta := &stubMeta{getSeriesResult: testSeriesDetail(), altTitles: nil}
	svc := NewService(mock, meta, nil, nil, discardLogger())

	if _, err := svc.RefreshMetadata(context.Background(), "s1"); err != nil {
		t.Fatalf("RefreshMetadata: %v", err)
	}
	if string(mock.gotUpdateAlternate) != `[]` {
		t.Errorf("nil alternates marshalled as %q, want \"[]\"", mock.gotUpdateAlternate)
	}
}

// testSeriesDetail returns a minimally-valid SeriesDetail that
// passes through RefreshMetadata's marshalling without surprises.
func testSeriesDetail() *tmdbtv.SeriesDetail {
	return &tmdbtv.SeriesDetail{
		ID:     1,
		Title:  "Test Series",
		Year:   2024,
		Status: "continuing",
		Genres: []string{"Drama"},
	}
}

// Anime detection / absolute_number backfill (refresh path)

// Headline regression test for the Jujutsu Kaisen incident: a series
// added before anime detection shipped (series_type=standard, episodes
// have absolute_number=NULL). On refresh, the AnimeLookup says yes →
// series_type flips to anime AND absolute_number gets populated 1..N
// across non-special seasons.
func TestRefreshMetadata_AnimeBackfill_FlipsTypeAndPopulatesAbsolute(t *testing.T) {
	row := mkSeriesRow("s1", `[]`)
	row.TmdbID = 95479 // Jujutsu Kaisen
	row.SeriesType = "standard"
	mock := &refreshMetadataMock{
		getSeriesRow: row,
		updatedRow:   row, // refresh returns the original row; anime upgrade happens after.
		episodes: []db.Episode{
			{ID: "e0", SeasonNumber: 0, EpisodeNumber: 1},  // special
			{ID: "e0b", SeasonNumber: 0, EpisodeNumber: 2}, // special
			{ID: "e1", SeasonNumber: 1, EpisodeNumber: 1},
			{ID: "e2", SeasonNumber: 1, EpisodeNumber: 2},
			{ID: "e3", SeasonNumber: 1, EpisodeNumber: 3},
		},
	}
	meta := &stubMeta{getSeriesResult: testSeriesDetail(), altTitles: []string{}}
	anime := fakeAnimeLookup{hits: map[int]bool{95479: true}}
	svc := NewService(mock, meta, anime, nil, discardLogger())

	res, err := svc.RefreshMetadata(context.Background(), "s1")
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if res.SeriesType != "anime" {
		t.Errorf("series_type after refresh = %q, want \"anime\"", res.SeriesType)
	}
	if len(mock.gotSeriesUpdates) != 1 || mock.gotSeriesUpdates[0].SeriesType != "anime" {
		t.Errorf("UpdateSeries: got %+v, want exactly one call setting type=anime", mock.gotSeriesUpdates)
	}
	// 3 non-special episodes → 3 absolute updates with values 1, 2, 3.
	if len(mock.gotAbsoluteUpdates) != 3 {
		t.Fatalf("expected 3 absolute updates, got %d: %+v", len(mock.gotAbsoluteUpdates), mock.gotAbsoluteUpdates)
	}
	for i, u := range mock.gotAbsoluteUpdates {
		want := int64(i + 1)
		if u.AbsoluteNumber == nil || *u.AbsoluteNumber != want {
			t.Errorf("update[%d]: got %v, want %d", i, u.AbsoluteNumber, want)
		}
	}
}

// Don't downgrade or re-write when the user already explicitly marked
// the series as anime — backfill should be additive, not destructive,
// and the series_type write is gated on row.SeriesType == "standard".
func TestRefreshMetadata_AnimeBackfill_SkipsAlreadyFlaggedSeries(t *testing.T) {
	row := mkSeriesRow("s1", `[]`)
	row.SeriesType = "anime" // already anime — should not re-fire the upgrade
	mock := &refreshMetadataMock{getSeriesRow: row, updatedRow: row}
	meta := &stubMeta{getSeriesResult: testSeriesDetail(), altTitles: []string{}}
	anime := fakeAnimeLookup{hits: map[int]bool{1: true}}
	svc := NewService(mock, meta, anime, nil, discardLogger())

	if _, err := svc.RefreshMetadata(context.Background(), "s1"); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if len(mock.gotSeriesUpdates) != 0 {
		t.Errorf("series_type should not be re-written when already anime; got %+v", mock.gotSeriesUpdates)
	}
	if len(mock.gotAbsoluteUpdates) != 0 {
		t.Errorf("backfill should not run when row was already anime; got %d absolute updates", len(mock.gotAbsoluteUpdates))
	}
}

// Don't touch series that aren't in the anime list — upgrades should be
// triggered by the lookup, not blanket-applied to every refresh.
func TestRefreshMetadata_AnimeBackfill_SkipsWhenNotAnime(t *testing.T) {
	row := mkSeriesRow("s1", `[]`)
	row.SeriesType = "standard"
	mock := &refreshMetadataMock{getSeriesRow: row, updatedRow: row}
	meta := &stubMeta{getSeriesResult: testSeriesDetail(), altTitles: []string{}}
	anime := fakeAnimeLookup{hits: map[int]bool{99999: true}} // not row.TmdbID (=1)
	svc := NewService(mock, meta, anime, nil, discardLogger())

	if _, err := svc.RefreshMetadata(context.Background(), "s1"); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if len(mock.gotSeriesUpdates) != 0 {
		t.Errorf("non-anime series must not be upgraded; got %+v", mock.gotSeriesUpdates)
	}
}

// AnimeLookup is allowed to be nil (deployments that don't run the
// fetcher) — refresh must still work end-to-end without panicking.
func TestRefreshMetadata_AnimeBackfill_NilLookupIsSafe(t *testing.T) {
	row := mkSeriesRow("s1", `[]`)
	row.SeriesType = "standard"
	mock := &refreshMetadataMock{getSeriesRow: row, updatedRow: row}
	meta := &stubMeta{getSeriesResult: testSeriesDetail(), altTitles: []string{}}
	svc := NewService(mock, meta, nil, nil, discardLogger())

	if _, err := svc.RefreshMetadata(context.Background(), "s1"); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if len(mock.gotSeriesUpdates) != 0 || len(mock.gotAbsoluteUpdates) != 0 {
		t.Errorf("nil lookup must not trigger any backfill writes")
	}
}

// Absolute backfill is idempotent — running it twice over a series with
// already-correct values produces no extra writes.
func TestRefreshMetadata_AnimeBackfill_IsIdempotent(t *testing.T) {
	row := mkSeriesRow("s1", `[]`)
	row.SeriesType = "anime" // already anime; the populate call still runs IF triggered.
	row.TmdbID = 95479
	mock := &refreshMetadataMock{
		getSeriesRow: row,
		updatedRow:   row,
		episodes: func() []db.Episode {
			v1, v2 := int64(1), int64(2)
			return []db.Episode{
				{ID: "e1", SeasonNumber: 1, EpisodeNumber: 1, AbsoluteNumber: &v1},
				{ID: "e2", SeasonNumber: 1, EpisodeNumber: 2, AbsoluteNumber: &v2},
			}
		}(),
	}
	meta := &stubMeta{getSeriesResult: testSeriesDetail(), altTitles: []string{}}
	anime := fakeAnimeLookup{hits: map[int]bool{95479: true}}
	svc := NewService(mock, meta, anime, nil, discardLogger())

	// Direct call to populateAbsoluteNumbers — refresh's gate would skip
	// this (already anime), but this exercises the helper's no-op-when-
	// already-correct invariant directly.
	if err := svc.populateAbsoluteNumbers(context.Background(), "s1"); err != nil {
		t.Fatalf("populateAbsoluteNumbers: %v", err)
	}
	if len(mock.gotAbsoluteUpdates) != 0 {
		t.Errorf("expected zero writes when absolute_number already correct; got %d", len(mock.gotAbsoluteUpdates))
	}
}

// Add: quality-profile FK resolution

// addMock implements just the queries Service.Add touches. It records the
// CreateSeries params so tests can assert the quality_profile_id that gets
// written, and lets tests control what GetLibrary returns.
type addMock struct {
	db.Querier

	library     db.Library
	libraryErr  error
	getLibCalls int

	createSeriesParams *db.CreateSeriesParams
}

func (m *addMock) GetSeriesByTMDBID(_ context.Context, _ int64) (db.Series, error) {
	// No existing series — Add proceeds past the duplicate check.
	return db.Series{}, sql.ErrNoRows
}

func (m *addMock) GetLibrary(_ context.Context, _ string) (db.Library, error) {
	m.getLibCalls++
	return m.library, m.libraryErr
}

func (m *addMock) CreateSeries(_ context.Context, p db.CreateSeriesParams) (db.Series, error) {
	m.createSeriesParams = &p
	// Echo the params back as a row so buildSeries has something to work with.
	return db.Series{
		ID:               p.ID,
		TmdbID:           p.TmdbID,
		Title:            p.Title,
		SortTitle:        p.SortTitle,
		Year:             p.Year,
		GenresJson:       p.GenresJson,
		Status:           p.Status,
		SeriesType:       p.SeriesType,
		MonitorType:      p.MonitorType,
		Monitored:        p.Monitored,
		LibraryID:        p.LibraryID,
		QualityProfileID: p.QualityProfileID,
		AddedAt:          p.AddedAt,
		UpdatedAt:        p.UpdatedAt,
		AlternateTitles:  p.AlternateTitles,
	}, nil
}

func (m *addMock) CreateSeason(_ context.Context, p db.CreateSeasonParams) (db.Season, error) {
	return db.Season(p), nil
}

func (m *addMock) CreateEpisode(_ context.Context, p db.CreateEpisodeParams) (db.Episode, error) {
	return db.Episode{ID: p.ID, SeriesID: p.SeriesID, SeasonID: p.SeasonID, SeasonNumber: p.SeasonNumber, EpisodeNumber: p.EpisodeNumber}, nil
}

func (m *addMock) UpdateEpisodeMonitored(_ context.Context, _ db.UpdateEpisodeMonitoredParams) error {
	return nil
}

func (m *addMock) UpdateSeasonMonitored(_ context.Context, _ db.UpdateSeasonMonitoredParams) error {
	return nil
}

func (m *addMock) CountEpisodesBySeriesID(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

func (m *addMock) CountEpisodesWithFileBySeriesID(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

// addStubMeta is a MetadataProvider that returns a single season's worth of
// metadata for the Add flow. Embeds stubMeta so only the Add-relevant hooks
// need overriding.
type addStubMeta struct {
	stubMeta
}

func (m *addStubMeta) GetSeries(context.Context, int) (*tmdbtv.SeriesDetail, error) {
	return &tmdbtv.SeriesDetail{
		ID:     42,
		Title:  "Test Series",
		Year:   2024,
		Status: "continuing",
		Genres: []string{"Drama"},
		Seasons: []tmdbtv.SeasonSummary{
			{SeasonNumber: 1, EpisodeCount: 1},
		},
	}, nil
}

func (m *addStubMeta) GetAlternativeTitles(context.Context, int) ([]string, error) {
	return []string{}, nil
}

func (m *addStubMeta) GetSeasonEpisodes(context.Context, int, int) ([]tmdbtv.EpisodeDetail, error) {
	return []tmdbtv.EpisodeDetail{
		{SeasonNumber: 1, EpisodeNumber: 1, Title: "Pilot"},
	}, nil
}

// Regression: adding a series with an empty quality_profile_id (the Discover
// panel's "Any Quality" option) must NOT write "" into the NOT NULL FK
// column. Instead it falls back to the library's default profile. Before the
// fix this surfaced as a foreign-key failure popup in the UI.
func TestAdd_EmptyQualityProfileFallsBackToLibraryDefault(t *testing.T) {
	mock := &addMock{
		library: db.Library{ID: "lib-1", DefaultQualityProfileID: "qp-default"},
	}
	svc := NewService(mock, &addStubMeta{}, nil, nil, discardLogger())

	got, err := svc.Add(context.Background(), AddRequest{
		TMDBID:           42,
		LibraryID:        "lib-1",
		QualityProfileID: "", // "Any Quality"
		Monitored:        true,
		MonitorType:      "all",
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if mock.createSeriesParams == nil {
		t.Fatal("CreateSeries was never called")
	}
	if mock.createSeriesParams.QualityProfileID != "qp-default" {
		t.Errorf("quality_profile_id written = %q, want %q (library default)",
			mock.createSeriesParams.QualityProfileID, "qp-default")
	}
	if got.QualityProfileID != "qp-default" {
		t.Errorf("returned series quality_profile_id = %q, want %q", got.QualityProfileID, "qp-default")
	}
}

// When the caller supplies an explicit quality profile, it must be used
// verbatim and the library default must NOT be consulted.
func TestAdd_ExplicitQualityProfileIsRespected(t *testing.T) {
	mock := &addMock{
		library: db.Library{ID: "lib-1", DefaultQualityProfileID: "qp-default"},
	}
	svc := NewService(mock, &addStubMeta{}, nil, nil, discardLogger())

	_, err := svc.Add(context.Background(), AddRequest{
		TMDBID:           42,
		LibraryID:        "lib-1",
		QualityProfileID: "qp-chosen",
		Monitored:        true,
		MonitorType:      "all",
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if mock.createSeriesParams.QualityProfileID != "qp-chosen" {
		t.Errorf("quality_profile_id written = %q, want %q (explicit)",
			mock.createSeriesParams.QualityProfileID, "qp-chosen")
	}
	if mock.getLibCalls != 0 {
		t.Errorf("GetLibrary called %d times; should not be consulted when profile is explicit", mock.getLibCalls)
	}
}

// If the library lookup fails while resolving the default profile, Add must
// surface the error rather than fall through and attempt an invalid insert.
func TestAdd_EmptyQualityProfileLibraryLookupError(t *testing.T) {
	mock := &addMock{
		libraryErr: errors.New("library gone"),
	}
	svc := NewService(mock, &addStubMeta{}, nil, nil, discardLogger())

	_, err := svc.Add(context.Background(), AddRequest{
		TMDBID:      42,
		LibraryID:   "lib-1",
		MonitorType: "all",
	})
	if err == nil {
		t.Fatal("expected error when library lookup fails, got nil")
	}
	if mock.createSeriesParams != nil {
		t.Error("CreateSeries must not be called when the default-profile lookup fails")
	}
}

// TVDBSeasonToAbsolute pass-through

// recordingAnimeLookup records the args TVDBSeasonToAbsolute was called
// with so we can assert the show service forwards them verbatim.
type recordingAnimeLookup struct {
	gotTMDB, gotSeason, gotEpisode int
	returnAbs                      int
	returnOk                       bool
}

func (r *recordingAnimeLookup) IsAnime(int) bool           { return false }
func (r *recordingAnimeLookup) CourBounds(int) []CourBound { return nil }
func (r *recordingAnimeLookup) TVDBSeasonToAbsolute(tmdbID, season, episode int) (int, bool) {
	r.gotTMDB, r.gotSeason, r.gotEpisode = tmdbID, season, episode
	return r.returnAbs, r.returnOk
}

func TestService_TVDBSeasonToAbsolute_DelegatesToLookup(t *testing.T) {
	rec := &recordingAnimeLookup{returnAbs: 48, returnOk: true}
	svc := NewService(nil, nil, rec, nil, discardLogger())

	abs, ok := svc.TVDBSeasonToAbsolute(95479, 3, 1)
	if !ok || abs != 48 {
		t.Fatalf("got (%d, %v), want (48, true)", abs, ok)
	}
	if rec.gotTMDB != 95479 || rec.gotSeason != 3 || rec.gotEpisode != 1 {
		t.Errorf("forwarded args wrong: tmdb=%d season=%d episode=%d",
			rec.gotTMDB, rec.gotSeason, rec.gotEpisode)
	}
}

// Deployments that don't run the Anime-Lists fetcher pass nil for the
// AnimeLookup. The pass-through must return (0, false) without panic so
// the search filter falls back to strict season/episode matching.
func TestService_TVDBSeasonToAbsolute_NilLookupIsSafe(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, discardLogger())

	abs, ok := svc.TVDBSeasonToAbsolute(95479, 3, 1)
	if ok || abs != 0 {
		t.Errorf("nil lookup must return (0, false); got (%d, %v)", abs, ok)
	}
}

// silence unused-import alarm for time when only used elsewhere
var _ = time.Now

func TestRefreshEpisodeMetadata_ReplacesPlaceholderTitles(t *testing.T) {
	aired := "2026-07-04"
	mock := &refreshMetadataMock{episodes: []db.Episode{
		// Added before it aired: placeholder title, no overview or date.
		{ID: "e1", SeasonNumber: 3, EpisodeNumber: 1, Title: "Episode 1", HasFile: true},
		// Already up to date: must not be written.
		{ID: "e2", SeasonNumber: 3, EpisodeNumber: 2, Title: "It's All Good", Overview: "o2", AirDate: &aired, StillPath: "/s2.jpg", RuntimeMinutes: 50},
	}}
	meta := &stubMeta{seasonEpisodes: []tmdbtv.EpisodeDetail{
		{SeasonNumber: 3, EpisodeNumber: 1, Title: "The Engineer", Overview: "o1", AirDate: "2026-07-04", StillPath: "/s1.jpg", RuntimeMinutes: 52},
		{SeasonNumber: 3, EpisodeNumber: 2, Title: "It's All Good", Overview: "o2", AirDate: "2026-07-04", StillPath: "/s2.jpg", RuntimeMinutes: 50},
	}}
	svc := NewService(mock, meta, nil, nil, discardLogger())

	if err := svc.RefreshEpisodeMetadata(context.Background(), "s1", 125988); err != nil {
		t.Fatal(err)
	}
	if len(mock.gotEpisodeUpdates) != 1 {
		t.Fatalf("got %d episode updates, want 1 (only the stale episode)", len(mock.gotEpisodeUpdates))
	}
	got := mock.gotEpisodeUpdates[0]
	if got.ID != "e1" || got.Title != "The Engineer" || got.Overview != "o1" ||
		got.AirDate == nil || *got.AirDate != "2026-07-04" || got.StillPath != "/s1.jpg" || got.RuntimeMinutes != 52 {
		t.Errorf("update = %+v, want TMDB's title, overview, air date, still and runtime", got)
	}
	if !got.HasFile {
		t.Error("refresh must keep has_file as stored")
	}
}

func TestRefreshedEpisode_EmptyTMDBFieldsKeepStoredValues(t *testing.T) {
	aired := "2023-05-05"
	ep := db.Episode{ID: "e1", Title: "Freedom Day", Overview: "stored", AirDate: &aired, StillPath: "/a.jpg", RuntimeMinutes: 60}
	p, changed := refreshedEpisode(ep, tmdbtv.EpisodeDetail{EpisodeNumber: 1})
	if changed {
		t.Errorf("empty TMDB detail reported a change: %+v", p)
	}
	if p.Title != "Freedom Day" || p.Overview != "stored" || p.AirDate == nil || *p.AirDate != aired || p.StillPath != "/a.jpg" || p.RuntimeMinutes != 60 {
		t.Errorf("stored values were blanked: %+v", p)
	}
}
