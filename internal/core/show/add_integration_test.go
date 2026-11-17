package show

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	pilotdb "github.com/beacon-stack/pilot/internal/db"
	db "github.com/beacon-stack/pilot/internal/db/generated"
)

// openTestDB opens a real, migrated SQLite database with foreign-key
// enforcement ON — the same pragma the production Open() sets. The mock-based
// unit tests can't catch FK violations because they never touch SQLite; this
// harness exists specifically to reproduce the "foreign key failure" the
// Discover Add flow hit when quality_profile_id was empty.
func openTestDB(t *testing.T) *db.Queries {
	t.Helper()

	path := filepath.Join(t.TempDir(), "test.db")
	dsn := "file:" + path + "?_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	sqlDB.SetMaxOpenConns(1)

	if err := pilotdb.Migrate(sqlDB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db.New(sqlDB)
}

// seedLibraryWithProfile creates a quality profile and a library that points
// at it, returning the library ID and the profile ID. This mirrors the
// minimal setup a real install has before a user adds their first series.
func seedLibraryWithProfile(t *testing.T, q *db.Queries) (libraryID, profileID string) {
	t.Helper()
	ctx := context.Background()
	const now = "2025-01-01T00:00:00Z"

	profileID = "qp-1"
	if _, err := q.CreateQualityProfile(ctx, db.CreateQualityProfileParams{
		ID:            profileID,
		Name:          "HD-1080p",
		CutoffJson:    "{}",
		QualitiesJson: "[]",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("create quality profile: %v", err)
	}

	libraryID = "lib-1"
	if _, err := q.CreateLibrary(ctx, db.CreateLibraryParams{
		ID:                      libraryID,
		Name:                    "TV",
		RootPath:                "/tv",
		DefaultQualityProfileID: profileID,
		TagsJson:                "[]",
		CreatedAt:               now,
		UpdatedAt:               now,
	}); err != nil {
		t.Fatalf("create library: %v", err)
	}
	return libraryID, profileID
}

// TestAddIntegration_EmptyQualityProfileDoesNotViolateFK is the headline
// regression for the Discover "Add" foreign-key popup. Against a real SQLite
// DB with foreign_keys=ON, adding a series with an empty quality_profile_id
// (the "Any Quality" option) must succeed by falling back to the library's
// default profile — NOT raise a FOREIGN KEY constraint failed error.
func TestAddIntegration_EmptyQualityProfileDoesNotViolateFK(t *testing.T) {
	q := openTestDB(t)
	libraryID, profileID := seedLibraryWithProfile(t, q)

	svc := NewService(q, &addStubMeta{}, nil, nil, discardLogger())

	got, err := svc.Add(context.Background(), AddRequest{
		TMDBID:           42,
		LibraryID:        libraryID,
		QualityProfileID: "", // "Any Quality" from the Discover panel
		Monitored:        true,
		MonitorType:      "all",
	})
	if err != nil {
		t.Fatalf("Add with empty quality profile failed (the original bug): %v", err)
	}
	if got.QualityProfileID != profileID {
		t.Errorf("quality_profile_id = %q, want library default %q", got.QualityProfileID, profileID)
	}

	// Sanity-check that the row actually persisted with a valid FK.
	row, err := q.GetSeriesByTMDBID(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetSeriesByTMDBID after add: %v", err)
	}
	if row.QualityProfileID != profileID {
		t.Errorf("persisted quality_profile_id = %q, want %q", row.QualityProfileID, profileID)
	}
}

// TestAddIntegration_EmptyQualityProfileIsAFKViolationWithoutFallback locks in
// WHY the fallback is required: inserting an empty quality_profile_id directly
// (bypassing the service fallback) really does trip the foreign-key
// constraint. If this ever stops failing, the schema's FK guarantee has
// regressed and the fallback is no longer load-bearing.
func TestAddIntegration_EmptyQualityProfileIsAFKViolationWithoutFallback(t *testing.T) {
	q := openTestDB(t)
	libraryID, _ := seedLibraryWithProfile(t, q)
	const now = "2025-01-01T00:00:00Z"

	_, err := q.CreateSeries(context.Background(), db.CreateSeriesParams{
		ID:               "series-x",
		TmdbID:           99,
		Title:            "Direct Insert",
		SortTitle:        "direct insert",
		GenresJson:       "[]",
		Status:           "continuing",
		SeriesType:       "standard",
		MonitorType:      "all",
		LibraryID:        libraryID,
		QualityProfileID: "", // no such quality_profiles row → FK violation
		AddedAt:          now,
		UpdatedAt:        now,
		AlternateTitles:  "[]",
	})
	if err == nil {
		t.Fatal("expected a foreign-key violation inserting empty quality_profile_id, got nil")
	}
}
