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

// ── rowToSeries: alternate_titles JSON parsing ───────────────────────────────

func TestRowToSeries_AlternateTitlesEmptyArray(t *testing.T) {
	row := mkSeriesRow("s1", []byte(`[]`))
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
	row := mkSeriesRow("s1", []byte(`null`))
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
	row := mkSeriesRow("s1", []byte(``))
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
	row := mkSeriesRow("s1", []byte(`{not json`))
	got, err := rowToSeries(row)
	if err != nil {
		t.Fatalf("rowToSeries should tolerate bad JSON, got error: %v", err)
	}
	if len(got.AlternateTitles) != 0 {
		t.Errorf("AlternateTitles on bad JSON = %v, want empty", got.AlternateTitles)
	}
}

func TestRowToSeries_AlternateTitlesPopulated(t *testing.T) {
	row := mkSeriesRow("s1", []byte(`["Star Wars: Andor","Andor: A Star Wars Story"]`))
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
func mkSeriesRow(id string, altTitles []byte) db.Series {
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

// ── RefreshMetadata ──────────────────────────────────────────────────────────

// refreshMetadataMock implements just the queries RefreshMetadata calls.
type refreshMetadataMock struct {
	db.Querier

	getSeriesRow db.Series
	getSeriesErr error

	updatedRow         db.Series
	updateErr          error
	updateCalled       bool
	gotUpdateAlternate []byte

	// Anime backfill capture: records calls so tests can assert that
	// RefreshMetadata triggers the upgrade exactly once when warranted
	// and never when it shouldn't. The series_type write goes through
	// the existing UpdateSeries query (which sets several fields at
	// once) rather than a dedicated UpdateSeriesType — see
	// BackfillAnimeIfNeeded.
	gotSeriesUpdates   []db.UpdateSeriesParams
	episodes           []db.Episode // returned by ListEpisodesBySeriesID
	gotAbsoluteUpdates []db.UpdateEpisodeAbsoluteNumberParams
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
}

func (s *stubMeta) SearchSeries(context.Context, string, int) ([]tmdbtv.SearchResult, error) {
	return nil, nil
}
func (s *stubMeta) GetSeries(context.Context, int) (*tmdbtv.SeriesDetail, error) {
	return s.getSeriesResult, s.getSeriesErr
}
func (s *stubMeta) GetSeasonEpisodes(context.Context, int, int) ([]tmdbtv.EpisodeDetail, error) {
	return nil, nil
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
	mock := &refreshMetadataMock{getSeriesRow: mkSeriesRow("s1", []byte(`[]`))}
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
	row := mkSeriesRow("s1", []byte(`[]`))
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
	row := mkSeriesRow("s1", []byte(`[]`))
	updatedRow := row
	updatedRow.AlternateTitles = []byte(`["Star Wars: Andor"]`)
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
	row := mkSeriesRow("s1", []byte(`[]`))
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

// ── Anime detection / absolute_number backfill (refresh path) ────────────────

// Headline regression test for the Jujutsu Kaisen incident: a series
// added before anime detection shipped (series_type=standard, episodes
// have absolute_number=NULL). On refresh, the AnimeLookup says yes →
// series_type flips to anime AND absolute_number gets populated 1..N
// across non-special seasons.
func TestRefreshMetadata_AnimeBackfill_FlipsTypeAndPopulatesAbsolute(t *testing.T) {
	row := mkSeriesRow("s1", []byte(`[]`))
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
		want := int32(i + 1)
		if !u.AbsoluteNumber.Valid || u.AbsoluteNumber.Int32 != want {
			t.Errorf("update[%d]: got %v, want %d", i, u.AbsoluteNumber, want)
		}
	}
}

// Don't downgrade or re-write when the user already explicitly marked
// the series as anime — backfill should be additive, not destructive,
// and the series_type write is gated on row.SeriesType == "standard".
func TestRefreshMetadata_AnimeBackfill_SkipsAlreadyFlaggedSeries(t *testing.T) {
	row := mkSeriesRow("s1", []byte(`[]`))
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
	row := mkSeriesRow("s1", []byte(`[]`))
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
	row := mkSeriesRow("s1", []byte(`[]`))
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
	row := mkSeriesRow("s1", []byte(`[]`))
	row.SeriesType = "anime" // already anime; the populate call still runs IF triggered.
	row.TmdbID = 95479
	mock := &refreshMetadataMock{
		getSeriesRow: row,
		updatedRow:   row,
		episodes: []db.Episode{
			{ID: "e1", SeasonNumber: 1, EpisodeNumber: 1, AbsoluteNumber: sql.NullInt32{Int32: 1, Valid: true}},
			{ID: "e2", SeasonNumber: 1, EpisodeNumber: 2, AbsoluteNumber: sql.NullInt32{Int32: 2, Valid: true}},
		},
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

// silence unused-import alarm for time when only used elsewhere
var _ = time.Now
