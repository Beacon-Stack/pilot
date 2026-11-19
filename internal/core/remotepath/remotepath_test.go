package remotepath

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/beacon-stack/pilot/internal/config"
	dbpkg "github.com/beacon-stack/pilot/internal/db"
	db "github.com/beacon-stack/pilot/internal/db/generated"
)

func TestApply(t *testing.T) {
	ms := []Mapping{
		{RemotePath: "/downloads", LocalPath: "/mnt/media/downloads"},
		{RemotePath: "/downloads/tv/", LocalPath: "/mnt/tv-dl"},
		{DownloadClientID: "sab", RemotePath: "/downloads", LocalPath: "/mnt/usenet"},
		{RemotePath: `C:\Downloads`, LocalPath: "/mnt/win"},
		{DownloadClientID: "other", RemotePath: "/data", LocalPath: "/never"},
	}
	cases := []struct {
		name, client, in, want string
	}{
		{"prefix", "qbit", "/downloads/Show.S01E01/ep.mkv", "/mnt/media/downloads/Show.S01E01/ep.mkv"},
		{"exact folder", "qbit", "/downloads", "/mnt/media/downloads"},
		{"trailing slash", "qbit", "/downloads/", "/mnt/media/downloads"},
		{"longest prefix wins", "qbit", "/downloads/tv/Show.S01E01", "/mnt/tv-dl/Show.S01E01"},
		{"client-specific beats any", "sab", "/downloads/Show.S01E01", "/mnt/usenet/Show.S01E01"},
		{"longer any-client still beats shorter specific", "sab", "/downloads/tv/x", "/mnt/tv-dl/x"},
		{"component boundary", "qbit", "/downloads2/Show", "/downloads2/Show"},
		{"other client's mapping ignored", "qbit", "/data/Show", "/data/Show"},
		{"no match", "qbit", "/srv/Show", "/srv/Show"},
		{"windows remote, case-insensitive", "qbit", `c:\downloads\Show.S01E01\Ep.mkv`, "/mnt/win/Show.S01E01/Ep.mkv"},
		{"empty", "qbit", "", ""},
	}
	for _, c := range cases {
		if got := Apply(ms, c.client, c.in); got != filepath.FromSlash(c.want) {
			t.Errorf("%s: Apply(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func newService(t *testing.T) (*Service, func(string)) {
	t.Helper()
	d, err := dbpkg.Open(config.DatabaseConfig{Path: filepath.Join(t.TempDir(), "pilot.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := dbpkg.Migrate(d.SQL); err != nil {
		t.Fatal(err)
	}
	addClient := func(id string) {
		now := time.Now().UTC().Format(time.RFC3339)
		if _, err := d.SQL.Exec(`INSERT INTO download_client_configs (id, name, kind, enabled, priority, settings, created_at, updated_at)
			VALUES (?, ?, 'qbittorrent', TRUE, 1, '{}', ?, ?)`, id, id, now, now); err != nil {
			t.Fatal(err)
		}
	}
	return NewService(db.New(d.SQL)), addClient
}

func TestService_CRUDAndMap(t *testing.T) {
	s, addClient := newService(t)
	ctx := context.Background()
	addClient("qbit")

	if s.Map(ctx, "qbit", "/downloads/x") != "/downloads/x" {
		t.Fatal("no mappings should leave the path alone")
	}
	m, err := s.Create(ctx, Request{DownloadClientID: "qbit", RemotePath: " /downloads ", LocalPath: "/mnt/dl/"})
	if err != nil {
		t.Fatal(err)
	}
	if m.LocalPath != "/mnt/dl" || m.RemotePath != "/downloads" {
		t.Fatalf("not normalized: %+v", m)
	}
	if got := s.Map(ctx, "qbit", "/downloads/x"); got != filepath.FromSlash("/mnt/dl/x") {
		t.Fatalf("Map = %q", got)
	}
	if got := s.Map(ctx, "sab", "/downloads/x"); got != "/downloads/x" {
		t.Fatalf("mapping for qbit applied to sab: %q", got)
	}

	if _, err := s.Update(ctx, m.ID, Request{RemotePath: "/downloads", LocalPath: "/mnt/any"}); err != nil {
		t.Fatal(err)
	}
	if got := s.Map(ctx, "sab", "/downloads/x"); got != filepath.FromSlash("/mnt/any/x") {
		t.Fatalf("any-client mapping not applied: %q", got)
	}
	if _, err := s.Update(ctx, "nope", Request{RemotePath: "/a", LocalPath: "/b"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing: %v", err)
	}

	if err := s.Delete(ctx, m.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, m.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
}

func TestService_Validation(t *testing.T) {
	s, _ := newService(t)
	for name, req := range map[string]Request{
		"empty remote":      {LocalPath: "/mnt"},
		"empty local":       {RemotePath: "/downloads"},
		"relative remote":   {RemotePath: "downloads", LocalPath: "/mnt"},
		"relative local":    {RemotePath: "/downloads", LocalPath: "mnt"},
		"unknown client id": {DownloadClientID: "ghost", RemotePath: "/downloads", LocalPath: "/mnt"},
	} {
		var verr *ValidationError
		if _, err := s.Create(context.Background(), req); !errors.As(err, &verr) {
			t.Errorf("%s: err = %v, want ValidationError", name, err)
		}
	}
	if _, err := s.Create(context.Background(), Request{RemotePath: `D:\Torrents`, LocalPath: "/mnt/t"}); err != nil {
		t.Errorf("windows remote path rejected: %v", err)
	}
}
