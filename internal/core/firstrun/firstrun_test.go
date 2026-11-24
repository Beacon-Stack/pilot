package firstrun

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/beacon-stack/pilot/internal/config"
	"github.com/beacon-stack/pilot/internal/core/library"
	"github.com/beacon-stack/pilot/internal/core/quality"
	dbpkg "github.com/beacon-stack/pilot/internal/db"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/events"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

type env struct {
	profiles  *quality.Service
	libraries *library.Service
	dir       string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	d, err := dbpkg.Open(config.DatabaseConfig{Path: filepath.Join(dir, "pilot.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := dbpkg.Migrate(d.SQL); err != nil {
		t.Fatal(err)
	}
	q := db.New(d.SQL)
	bus := events.New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	return &env{profiles: quality.NewService(q, bus), libraries: library.NewService(q, bus), dir: dir}
}

func (e *env) run(t *testing.T, paths ...string) {
	t.Helper()
	if err := Run(context.Background(), e.profiles, e.libraries, Options{LibraryPaths: paths},
		slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
}

func TestRun_FreshInstallGetsProfilesAndLibrary(t *testing.T) {
	e := newEnv(t)
	data := filepath.Join(e.dir, "data")
	if err := os.MkdirAll(filepath.Join(data, "media"), 0o755); err != nil {
		t.Fatal(err)
	}
	tvRoot := filepath.Join(data, "media", "tv")
	e.run(t, tvRoot, filepath.Join(e.dir, "tv"))

	profiles, _ := e.profiles.List(context.Background())
	if len(profiles) != len(DefaultProfiles()) {
		t.Fatalf("profiles = %d, want %d", len(profiles), len(DefaultProfiles()))
	}
	libs, _ := e.libraries.List(context.Background())
	if len(libs) != 1 || libs[0].RootPath != tvRoot {
		t.Fatalf("libraries = %+v, want one at %s", libs, tvRoot)
	}
	if fi, err := os.Stat(tvRoot); err != nil || !fi.IsDir() {
		t.Fatal("library folder was not created")
	}
	var defaultID string
	for _, p := range profiles {
		if p.Name == DefaultProfileName {
			defaultID = p.ID
		}
	}
	if libs[0].DefaultQualityProfileID != defaultID {
		t.Fatalf("library profile = %s, want %s (%s)", libs[0].DefaultQualityProfileID, defaultID, DefaultProfileName)
	}
}

// The documented setup is one empty folder mounted at /data: the library
// folder and its parent are both created in it.
func TestRun_EmptyDataFolderGetsLibrary(t *testing.T) {
	e := newEnv(t)
	data := filepath.Join(e.dir, "data")
	if err := os.Mkdir(data, 0o755); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(data, "media", "tv")
	e.run(t, root, filepath.Join(e.dir, "nomount", "tv"))

	libs, _ := e.libraries.List(context.Background())
	if len(libs) != 1 || libs[0].RootPath != root {
		t.Fatalf("libraries = %+v, want one at %s", libs, root)
	}
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		t.Fatalf("library folder not created: %v", err)
	}
}

func TestRun_IsIdempotent(t *testing.T) {
	e := newEnv(t)
	tvRoot := filepath.Join(e.dir, "tv")
	e.run(t, tvRoot)
	e.run(t, tvRoot)
	profiles, _ := e.profiles.List(context.Background())
	libs, _ := e.libraries.List(context.Background())
	if len(profiles) != len(DefaultProfiles()) || len(libs) != 1 {
		t.Fatalf("second run duplicated defaults: %d profiles, %d libraries", len(profiles), len(libs))
	}
}

func TestRun_LeavesExistingSetupAlone(t *testing.T) {
	e := newEnv(t)
	mine, err := e.profiles.Create(context.Background(), quality.CreateRequest{
		Name:   "Mine",
		Cutoff: plugin.Quality{Resolution: plugin.Resolution1080p, Source: plugin.SourceWEBDL},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Existing profile, no library: no new profiles, library uses mine.
	tvRoot := filepath.Join(e.dir, "tv")
	e.run(t, tvRoot)
	profiles, _ := e.profiles.List(context.Background())
	if len(profiles) != 1 {
		t.Fatalf("profiles = %d, want only the user's", len(profiles))
	}
	libs, _ := e.libraries.List(context.Background())
	if len(libs) != 1 || libs[0].DefaultQualityProfileID != mine.ID {
		t.Fatalf("library = %+v, want one using the existing profile", libs)
	}

	// Existing library: nothing more is created, even at a new path.
	other := filepath.Join(e.dir, "other")
	e.run(t, other)
	libs, _ = e.libraries.List(context.Background())
	if len(libs) != 1 {
		t.Fatalf("libraries = %d, want 1", len(libs))
	}
}

func TestRun_NoMediaFolderSkipsLibrary(t *testing.T) {
	e := newEnv(t)
	e.run(t, filepath.Join(e.dir, "missing", "media", "tv"))
	libs, _ := e.libraries.List(context.Background())
	if len(libs) != 0 {
		t.Fatalf("created a library with no media folder: %+v", libs)
	}
	profiles, _ := e.profiles.List(context.Background())
	if len(profiles) == 0 {
		t.Fatal("profiles should still be created")
	}
}

func TestDefaultProfiles_CutoffIsAllowed(t *testing.T) {
	for _, req := range DefaultProfiles() {
		p := quality.Profile{Qualities: req.Qualities}
		if !p.Allows(req.Cutoff) {
			t.Errorf("%s: cutoff %s is not in its own quality list", req.Name, req.Cutoff.Name)
		}
		if req.UpgradeUntil != nil && !p.Allows(*req.UpgradeUntil) {
			t.Errorf("%s: upgrade-until %s is not in its own quality list", req.Name, req.UpgradeUntil.Name)
		}
	}
}
