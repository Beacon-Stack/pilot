package delay

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/beacon-stack/pilot/internal/config"
	"github.com/beacon-stack/pilot/internal/core/quality"
	dbpkg "github.com/beacon-stack/pilot/internal/db"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

func TestValidate(t *testing.T) {
	ok := Default()
	if err := ok.Validate(); err != nil {
		t.Fatalf("default profile invalid: %v", err)
	}
	bad := map[string]Profile{
		"no protocol":   {PreferredProtocol: "usenet"},
		"bad preferred": {UsenetEnabled: true, PreferredProtocol: "ftp"},
		"negative":      {UsenetEnabled: true, PreferredProtocol: "usenet", UsenetDelayMinutes: -1},
		"too long":      {UsenetEnabled: true, PreferredProtocol: "usenet", TorrentDelayMinutes: MaxDelayMinutes + 1},
	}
	for name, p := range bad {
		var verr *ValidationError
		if err := p.Validate(); !errors.As(err, &verr) {
			t.Errorf("%s: err = %v, want ValidationError", name, err)
		}
	}
}

func TestProtocolRules(t *testing.T) {
	p := Profile{UsenetEnabled: true, TorrentEnabled: false, PreferredProtocol: "usenet", UsenetDelayMinutes: 30, TorrentDelayMinutes: 120}
	if !p.Allows(plugin.ProtocolNZB) || p.Allows(plugin.ProtocolTorrent) {
		t.Fatal("Allows ignores the enabled flags")
	}
	if p.Delay(plugin.ProtocolNZB) != 30*time.Minute || p.Delay(plugin.ProtocolTorrent) != 2*time.Hour {
		t.Fatal("Delay uses the wrong protocol's setting")
	}
	if !p.Preferred(plugin.ProtocolNZB) || p.Preferred(plugin.ProtocolTorrent) {
		t.Fatal("Preferred is wrong")
	}
}

func TestEligibleAt(t *testing.T) {
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	p := Profile{TorrentEnabled: true, PreferredProtocol: "torrent", TorrentDelayMinutes: 60}
	// Published 15 minutes ago, 60 minute delay: eligible in 45 minutes.
	got := p.EligibleAt(plugin.ProtocolTorrent, 15.0/(24*60), now)
	if want := now.Add(45 * time.Minute); got.Sub(want).Abs() > time.Second {
		t.Fatalf("EligibleAt = %v, want %v", got, want)
	}
}

func TestBypasses(t *testing.T) {
	web := plugin.Quality{Resolution: plugin.Resolution1080p, Source: plugin.SourceWEBDL}
	bd := plugin.Quality{Resolution: plugin.Resolution1080p, Source: plugin.SourceBluRay}
	prof := &quality.Profile{Cutoff: web, Qualities: []plugin.Quality{web, bd}}
	p := Default()
	if !p.Bypasses(plugin.Quality{Resolution: plugin.Resolution1080p, Source: plugin.SourceBluRay, Codec: "x265"}, prof) {
		t.Fatal("the best allowed quality (any codec) should bypass")
	}
	if p.Bypasses(web, prof) {
		t.Fatal("a lower allowed quality must not bypass")
	}
	p.BypassIfHighestQuality = false
	if p.Bypasses(bd, prof) {
		t.Fatal("bypass is off")
	}
	if Default().Bypasses(bd, nil) {
		t.Fatal("no profile: no bypass")
	}
}

func TestStore_DefaultAndRoundTrip(t *testing.T) {
	d, err := dbpkg.Open(config.DatabaseConfig{Path: filepath.Join(t.TempDir(), "pilot.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := dbpkg.Migrate(d.SQL); err != nil {
		t.Fatal(err)
	}
	s := NewStore(db.New(d.SQL))
	ctx := context.Background()

	got, err := s.Get(ctx)
	if err != nil || got != Default() {
		t.Fatalf("unsaved profile = %+v, %v; want the default", got, err)
	}
	want := Profile{UsenetEnabled: true, TorrentEnabled: true, PreferredProtocol: "torrent", UsenetDelayMinutes: 10, TorrentDelayMinutes: 90}
	if err := s.Set(ctx, want); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.Get(ctx); got != want {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
	if err := s.Set(ctx, Profile{}); err == nil {
		t.Fatal("an invalid profile was saved")
	}
}
