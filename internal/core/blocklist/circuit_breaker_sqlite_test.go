package blocklist

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	pilotdb "github.com/beacon-stack/pilot/internal/db"
	db "github.com/beacon-stack/pilot/internal/db/generated"
)

// Runs the real CountRecentStallsForEpisode SQL against a migrated SQLite
// database. The mock-querier tests can't catch a mistake in the query.
// Stalls and download failures share one retry budget per episode; user
// marks and import failures must not count.
func TestCountRecentStallsForEpisode_SQL(t *testing.T) {
	sqlDB, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "pilot.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := pilotdb.Migrate(sqlDB); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Now().UTC()
	recent := now.Add(-time.Hour).Format("2006-01-02T15:04:05Z")
	old := now.Add(-48 * time.Hour).Format("2006-01-02T15:04:05Z")
	rows := []struct{ id, ep, reason, addedAt string }{
		{"1", "ep-1", ReasonStallNoPeersEver, recent},
		{"2", "ep-1", ReasonStallActivityLost, recent},
		{"3", "ep-1", ReasonDownloadFailed, recent},
		{"4", "ep-1", ReasonUserMarked, recent},     // not counted
		{"5", "ep-1", ReasonImportFailed, recent},   // not counted
		{"6", "ep-1", ReasonDownloadFailed, old},    // outside 24h
		{"7", "ep-2", ReasonDownloadFailed, recent}, // other episode
	}
	for _, r := range rows {
		if _, err := sqlDB.Exec(`INSERT INTO blocklist (id, series_id, episode_id, release_guid, release_title, added_at, reason)
			VALUES (?, 'series-1', ?, 'guid-'||?, 'title', ?, ?)`, r.id, r.ep, r.id, r.addedAt, r.reason); err != nil {
			t.Fatalf("insert %s: %v", r.id, err)
		}
	}

	ep1 := "ep-1"
	got, err := db.New(sqlDB).CountRecentStallsForEpisode(context.Background(), db.CountRecentStallsForEpisodeParams{
		SeriesID:  "series-1",
		EpisodeID: &ep1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != 3 {
		t.Fatalf("count = %d, want 3 (two stalls + one download failure)", got)
	}
}
