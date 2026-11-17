package mediaserver

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/beacon-stack/pilot/internal/config"
	dbpkg "github.com/beacon-stack/pilot/internal/db"
	db "github.com/beacon-stack/pilot/internal/db/generated"
)

// The API masks secrets as "***". Saving a media server as the UI sends
// it back (masked, or with the key left out) must keep the real secret;
// it used to be overwritten with the literal "***".
func TestUpdate_KeepsMaskedSecrets(t *testing.T) {
	d, err := dbpkg.Open(config.DatabaseConfig{Path: filepath.Join(t.TempDir(), "pilot.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := dbpkg.Migrate(d.SQL); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SQL.Exec(`INSERT INTO media_server_configs (id, name, kind, enabled, settings, created_at, updated_at)
		VALUES ('n1', 'Plex', 'plex', TRUE, '{"webhook_url":"https://discord.example/secret","username":"pilot"}', 'x', 'x')`); err != nil {
		t.Fatal(err)
	}
	svc := NewService(db.New(d.SQL), nil)
	ctx := context.Background()

	_, err = svc.Update(ctx, "n1", UpdateRequest{Name: "Renamed", Kind: "plex", Enabled: true,
		Settings: json.RawMessage(`{"webhook_url":"***","username":"bot"}`)})
	if err != nil {
		t.Fatal(err)
	}
	row, _ := db.New(d.SQL).GetMediaServerConfig(ctx, "n1")
	var got map[string]string
	_ = json.Unmarshal([]byte(row.Settings), &got)
	if got["webhook_url"] != "https://discord.example/secret" || got["username"] != "bot" || row.Name != "Renamed" {
		t.Fatalf("after masked update: name=%q settings=%s", row.Name, row.Settings)
	}

	// A new value replaces the secret.
	_, _ = svc.Update(ctx, "n1", UpdateRequest{Name: "Renamed", Kind: "plex", Enabled: true,
		Settings: json.RawMessage(`{"webhook_url":"https://discord.example/new"}`)})
	row, _ = db.New(d.SQL).GetMediaServerConfig(ctx, "n1")
	_ = json.Unmarshal([]byte(row.Settings), &got)
	if got["webhook_url"] != "https://discord.example/new" {
		t.Fatalf("new secret not saved: %s", row.Settings)
	}
}
