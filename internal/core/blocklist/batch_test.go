package blocklist

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/beacon-stack/pilot/internal/config"
	dbpkg "github.com/beacon-stack/pilot/internal/db"
	db "github.com/beacon-stack/pilot/internal/db/generated"
)

// Against a real, migrated SQLite database: the generated IN (...) query
// expands its slice at run time, which only a real database exercises.
func TestBlocklistedGUIDs_RealDB(t *testing.T) {
	d, err := dbpkg.Open(config.DatabaseConfig{Path: filepath.Join(t.TempDir(), "pilot.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := dbpkg.Migrate(d.SQL); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	conn, err := d.SQL.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// The table references series; these rows don't need a real one.
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatal(err)
	}
	// 1,203 entries so the lookup crosses two batch boundaries.
	for i := 0; i < 1203; i++ {
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO blocklist (id, series_id, release_guid, release_title, added_at) VALUES (?, 's', ?, 't', 'now')`,
			fmt.Sprint("id", i), fmt.Sprint("blocked-", i)); err != nil {
			t.Fatal(err)
		}
	}
	conn.Close()

	svc := NewService(db.New(d.SQL))
	guids := []string{"blocked-0", "clean-1", "blocked-600", "clean-2", "blocked-1202"}
	for i := 0; i < 1100; i++ {
		guids = append(guids, fmt.Sprint("clean-x", i))
	}
	got, err := svc.BlocklistedGUIDs(ctx, guids)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || !got["blocked-0"] || !got["blocked-600"] || !got["blocked-1202"] {
		t.Errorf("blocklisted = %v, want exactly blocked-0, blocked-600, blocked-1202", got)
	}
	empty, err := svc.BlocklistedGUIDs(ctx, nil)
	if err != nil || len(empty) != 0 {
		t.Errorf("no GUIDs: %v, %v", empty, err)
	}
}
