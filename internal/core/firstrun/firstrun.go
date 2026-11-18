// Package firstrun gives a fresh install working defaults, the way Sonarr
// ships with quality profiles and asks only for a root folder.
//
// On startup, and only while the database has no quality profiles and no
// libraries, it creates:
//
//   - default quality profiles (Any, SD, HD-720p, HD-1080p,
//     HD - 720p/1080p, Ultra-HD), and
//   - a "TV" library at the first usable path from [Options.LibraryPaths]
//     (by default /data/media/tv in the single-/data-mount layout, falling
//     back to /tv for the older three-mount layout).
//
// Anything the user has already set up is left alone: if a profile or a
// library exists, that part is skipped.
package firstrun

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/beacon-stack/pilot/internal/core/library"
	"github.com/beacon-stack/pilot/internal/core/quality"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

// DefaultProfileName is the profile the default library uses.
const DefaultProfileName = "HD-1080p"

// DefaultLibraryPaths is where the default TV library goes, in order of
// preference. A path is used when its parent folder exists (the folder
// itself is created if needed): /data/media/tv for the single /data
// mount, /tv for the older layout.
var DefaultLibraryPaths = []string{"/data/media/tv", "/tv"}

// Options configures Run.
type Options struct {
	// LibraryPaths overrides DefaultLibraryPaths (tests, custom layouts).
	LibraryPaths []string
}

type profileStore interface {
	List(ctx context.Context) ([]quality.Profile, error)
	Create(ctx context.Context, req quality.CreateRequest) (quality.Profile, error)
}

type libraryStore interface {
	List(ctx context.Context) ([]library.Library, error)
	Create(ctx context.Context, req library.CreateRequest) (library.Library, error)
}

// Run applies the first-run defaults. It is safe to call on every start.
func Run(ctx context.Context, profiles profileStore, libraries libraryStore, opts Options, logger *slog.Logger) error {
	existingProfiles, err := profiles.List(ctx)
	if err != nil {
		return fmt.Errorf("listing quality profiles: %w", err)
	}
	existingLibraries, err := libraries.List(ctx)
	if err != nil {
		return fmt.Errorf("listing libraries: %w", err)
	}

	var defaultProfileID string
	if len(existingProfiles) == 0 {
		for _, req := range DefaultProfiles() {
			p, err := profiles.Create(ctx, req)
			if err != nil {
				return fmt.Errorf("creating quality profile %q: %w", req.Name, err)
			}
			if p.Name == DefaultProfileName {
				defaultProfileID = p.ID
			}
		}
		logger.Info("first run: created default quality profiles", "count", len(DefaultProfiles()))
	} else {
		defaultProfileID = existingProfiles[0].ID
		for _, p := range existingProfiles {
			if p.Name == DefaultProfileName {
				defaultProfileID = p.ID
			}
		}
	}

	if len(existingLibraries) > 0 {
		return nil
	}
	paths := opts.LibraryPaths
	if paths == nil {
		paths = DefaultLibraryPaths
	}
	root := pickLibraryPath(paths)
	if root == "" {
		logger.Info("first run: no media folder found, add a library in Settings → Libraries",
			"looked_for", paths)
		return nil
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		logger.Warn("first run: could not create the default library folder; add a library in Settings → Libraries",
			"path", root, "uid", os.Getuid(), "gid", os.Getgid(), "error", err)
		return nil
	}
	if _, err := libraries.Create(ctx, library.CreateRequest{
		Name:                    "TV",
		RootPath:                root,
		DefaultQualityProfileID: defaultProfileID,
	}); err != nil {
		return fmt.Errorf("creating default library: %w", err)
	}
	logger.Info("first run: created default TV library", "path", root)
	return nil
}

// pickLibraryPath returns the first path whose parent directory exists.
func pickLibraryPath(paths []string) string {
	for _, p := range paths {
		if p == "" {
			continue
		}
		if fi, err := os.Stat(filepath.Dir(filepath.Clean(p))); err == nil && fi.IsDir() {
			return p
		}
	}
	return ""
}

func q(res plugin.Resolution, src plugin.Source, name string) plugin.Quality {
	return plugin.Quality{Resolution: res, Source: src, Name: name}
}

// DefaultProfiles returns the profiles a fresh install starts with,
// modelled on Sonarr's. Matching uses resolution + source only, so codec
// and HDR are left empty. Qualities are ordered best first.
func DefaultProfiles() []quality.CreateRequest {
	var (
		sdtv       = q(plugin.ResolutionSD, plugin.SourceHDTV, "SDTV")
		sdDVD      = q(plugin.ResolutionSD, plugin.SourceDVD, "DVD")
		sdWEBDL    = q(plugin.ResolutionSD, plugin.SourceWEBDL, "WEBDL-480p")
		sdWEBRip   = q(plugin.ResolutionSD, plugin.SourceWEBRip, "WEBRip-480p")
		hdtv720    = q(plugin.Resolution720p, plugin.SourceHDTV, "HDTV-720p")
		webdl720   = q(plugin.Resolution720p, plugin.SourceWEBDL, "WEBDL-720p")
		webrip720  = q(plugin.Resolution720p, plugin.SourceWEBRip, "WEBRip-720p")
		bluray720  = q(plugin.Resolution720p, plugin.SourceBluRay, "Bluray-720p")
		hdtv1080   = q(plugin.Resolution1080p, plugin.SourceHDTV, "HDTV-1080p")
		webdl1080  = q(plugin.Resolution1080p, plugin.SourceWEBDL, "WEBDL-1080p")
		webrip1080 = q(plugin.Resolution1080p, plugin.SourceWEBRip, "WEBRip-1080p")
		bluray1080 = q(plugin.Resolution1080p, plugin.SourceBluRay, "Bluray-1080p")
		hdtv2160   = q(plugin.Resolution2160p, plugin.SourceHDTV, "HDTV-2160p")
		webdl2160  = q(plugin.Resolution2160p, plugin.SourceWEBDL, "WEBDL-2160p")
		webrip2160 = q(plugin.Resolution2160p, plugin.SourceWEBRip, "WEBRip-2160p")
		bluray2160 = q(plugin.Resolution2160p, plugin.SourceBluRay, "Bluray-2160p")
	)
	hd720 := []plugin.Quality{bluray720, webdl720, webrip720, hdtv720}
	hd1080 := []plugin.Quality{bluray1080, webdl1080, webrip1080, hdtv1080}
	uhd := []plugin.Quality{bluray2160, webdl2160, webrip2160, hdtv2160}
	sd := []plugin.Quality{sdWEBDL, sdWEBRip, sdDVD, sdtv}

	cat := func(groups ...[]plugin.Quality) []plugin.Quality {
		var out []plugin.Quality
		for _, g := range groups {
			out = append(out, g...)
		}
		return out
	}
	return []quality.CreateRequest{
		{Name: "Any", Cutoff: sdtv, Qualities: cat(uhd, hd1080, hd720, sd), UpgradeAllowed: false},
		{Name: "SD", Cutoff: sdtv, Qualities: sd, UpgradeAllowed: false},
		{Name: "HD-720p", Cutoff: hdtv720, Qualities: hd720, UpgradeAllowed: false},
		{Name: DefaultProfileName, Cutoff: webdl1080, Qualities: hd1080, UpgradeAllowed: true, UpgradeUntil: &webdl1080},
		{Name: "HD - 720p/1080p", Cutoff: hdtv720, Qualities: cat(hd1080, hd720), UpgradeAllowed: true, UpgradeUntil: &webdl1080},
		{Name: "Ultra-HD", Cutoff: webdl2160, Qualities: uhd, UpgradeAllowed: true, UpgradeUntil: &webdl2160},
	}
}
