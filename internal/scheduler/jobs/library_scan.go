package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/beacon-media/pilot/internal/core/library"
	"github.com/beacon-media/pilot/internal/core/parser"
	"github.com/beacon-media/pilot/internal/core/show"
	dbsqlite "github.com/beacon-media/pilot/internal/db/generated/sqlite"
	"github.com/beacon-media/pilot/internal/scheduler"
	"github.com/beacon-media/pilot/pkg/plugin"
)

// libraryScanInterval is how often the library scan job runs.
const libraryScanInterval = 6 * time.Hour

// videoExtensions is the set of file extensions the scanner considers video files.
var libraryScanVideoExtensions = map[string]bool{
	".mkv":  true,
	".mp4":  true,
	".avi":  true,
	".ts":   true,
	".m2ts": true,
	".mov":  true,
	".wmv":  true,
}

// yearSuffixRe strips a trailing (YYYY) or .YYYY from folder names.
var yearSuffixRe = regexp.MustCompile(`[\s.(]+(?:19|20)\d{2}[\s.)]*$`)

// releaseTagsRe strips common release tags from folder names.
var releaseTagsRe = regexp.MustCompile(`(?i)[\s.\[(-]*(720p|1080p|2160p|4k|bluray|brrip|webrip|web-dl|hdtv|dvdrip|x264|x265|h\.?264|h\.?265|aac|dts|atmos|proper|repack|complete|season|s\d{1,2}).*$`)

// LibraryScan returns a Job that walks all library root directories, discovers
// new shows from folder names (auto-adding via TMDB), then matches video files
// to known series/episodes and creates episode_file records.
func LibraryScan(libSvc *library.Service, showSvc *show.Service, q dbsqlite.Querier, logger *slog.Logger) scheduler.Job {
	return scheduler.Job{
		Name:     "library_scan",
		Interval: libraryScanInterval,
		Fn: func(ctx context.Context) {
			logger.Info("task started", "task", "library_scan")
			start := time.Now()

			if err := runLibraryScan(ctx, libSvc, showSvc, q, logger); err != nil {
				logger.Warn("task failed",
					"task", "library_scan",
					"error", err,
					"duration_ms", time.Since(start).Milliseconds(),
				)
				return
			}

			logger.Info("task finished",
				"task", "library_scan",
				"duration_ms", time.Since(start).Milliseconds(),
			)
		},
	}
}

// runLibraryScan performs the actual scan work.
func runLibraryScan(ctx context.Context, libSvc *library.Service, showSvc *show.Service, q dbsqlite.Querier, logger *slog.Logger) error {
	libs, err := libSvc.List(ctx)
	if err != nil {
		return err
	}

	// Phase 1: Discover new shows from folder names and auto-add via TMDB.
	for _, lib := range libs {
		discoverShows(ctx, lib, showSvc, logger)
	}

	// Phase 2: Match video files to known series/episodes.
	knownPaths, err := q.ListAllEpisodeFilePaths(ctx)
	if err != nil {
		return err
	}
	known := make(map[string]bool, len(knownPaths))
	for _, p := range knownPaths {
		known[p] = true
	}

	for _, lib := range libs {
		if err := scanLibraryFiles(ctx, lib, showSvc, q, known, logger); err != nil {
			logger.Warn("library scan error",
				"library_id", lib.ID,
				"library_name", lib.Name,
				"error", err,
			)
		}
	}
	return nil
}

// discoverShows reads top-level directories in a library root, extracts show
// names, searches TMDB, and auto-adds any that aren't already in the database.
func discoverShows(ctx context.Context, lib library.Library, showSvc *show.Service, logger *slog.Logger) {
	entries, err := os.ReadDir(lib.RootPath)
	if err != nil {
		logger.Warn("library scan: cannot read library root",
			"library_id", lib.ID,
			"root_path", lib.RootPath,
			"error", err,
		)
		return
	}

	// Build set of titles already in this library for quick skip.
	existing, err := showSvc.List(ctx, show.ListRequest{LibraryID: lib.ID, Page: 1, PerPage: 10000})
	if err != nil {
		logger.Warn("library scan: cannot list existing series", "error", err)
		return
	}
	existingTitles := make(map[string]bool, len(existing.Series))
	for _, s := range existing.Series {
		existingTitles[strings.ToLower(s.Title)] = true
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		showTitle := cleanFolderName(entry.Name())
		if showTitle == "" {
			continue
		}

		// Skip if we already have this show (case-insensitive).
		if existingTitles[strings.ToLower(showTitle)] {
			continue
		}

		// Search TMDB for this show title.
		results, err := showSvc.Lookup(ctx, show.LookupRequest{Query: showTitle})
		if err != nil {
			logger.Warn("library scan: TMDB lookup failed",
				"title", showTitle,
				"folder", entry.Name(),
				"error", err,
			)
			continue
		}
		if len(results) == 0 {
			logger.Info("library scan: no TMDB match for folder",
				"title", showTitle,
				"folder", entry.Name(),
			)
			continue
		}

		// Use the top result.
		best := results[0]
		logger.Info("library scan: auto-adding series from folder",
			"folder", entry.Name(),
			"matched_title", best.Title,
			"tmdb_id", best.ID,
		)

		_, addErr := showSvc.Add(ctx, show.AddRequest{
			TMDBID:      best.ID,
			LibraryID:   lib.ID,
			Monitored:   true,
			MonitorType: "existing",
			SeriesType:  "standard",
		})
		if addErr != nil {
			if errors.Is(addErr, show.ErrAlreadyExists) {
				continue
			}
			logger.Warn("library scan: failed to auto-add series",
				"title", best.Title,
				"tmdb_id", best.ID,
				"error", addErr,
			)
			continue
		}

		// Track it so we don't try again for a duplicate folder variant.
		existingTitles[strings.ToLower(best.Title)] = true
	}
}

// cleanFolderName normalizes a directory name into a human-readable show title
// suitable for a TMDB search. It strips release tags, year suffixes,
// and replaces separators with spaces.
func cleanFolderName(name string) string {
	// Replace common separators with spaces.
	name = strings.NewReplacer(
		".", " ",
		"_", " ",
		"-", " ",
	).Replace(name)

	// Strip release tags (720p, x264, BRRip, etc.).
	name = releaseTagsRe.ReplaceAllString(name, "")

	// Strip trailing year like (2019) or .2019
	name = yearSuffixRe.ReplaceAllString(name, "")

	// Strip content inside square brackets (release groups like [TGx]).
	name = regexp.MustCompile(`\[.*?\]`).ReplaceAllString(name, "")

	// Collapse whitespace and trim.
	for strings.Contains(name, "  ") {
		name = strings.ReplaceAll(name, "  ", " ")
	}
	name = strings.TrimSpace(name)

	// Skip names that are too short or look like just episode fragments.
	if len(name) < 2 {
		return ""
	}
	return name
}

// scanLibraryFiles walks a single library root directory and imports new files.
func scanLibraryFiles(
	ctx context.Context,
	lib library.Library,
	showSvc *show.Service,
	q dbsqlite.Querier,
	known map[string]bool,
	logger *slog.Logger,
) error {
	// Load all series in this library so we can match by title.
	result, err := showSvc.List(ctx, show.ListRequest{LibraryID: lib.ID, Page: 1, PerPage: 10000})
	if err != nil {
		return err
	}

	// Build a lowercase-title → Series map for fuzzy matching.
	seriesByTitle := make(map[string]show.Series, len(result.Series))
	for _, s := range result.Series {
		seriesByTitle[strings.ToLower(s.Title)] = s
	}

	return filepath.WalkDir(lib.RootPath, func(path string, d os.DirEntry, werr error) error {
		if werr != nil {
			logger.Warn("library scan: walk error", "path", path, "error", werr)
			return nil // keep walking
		}
		if d.IsDir() {
			return nil
		}
		if !libraryScanVideoExtensions[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		if known[path] {
			return nil // already tracked
		}

		if err := importScannedFile(ctx, path, lib, seriesByTitle, showSvc, q, logger); err != nil {
			logger.Warn("library scan: import failed", "path", path, "error", err)
		}
		return nil
	})
}

// importScannedFile tries to match a discovered file to an existing
// series/episode and creates an episode_file record if successful.
func importScannedFile(
	ctx context.Context,
	path string,
	lib library.Library,
	seriesByTitle map[string]show.Series,
	showSvc *show.Service,
	q dbsqlite.Querier,
	logger *slog.Logger,
) error {
	parsed := parser.Parse(filepath.Base(path))
	info := parsed.EpisodeInfo

	if len(info.Episodes) == 0 {
		logger.Debug("library scan: no episode info in filename, skipping", "path", path)
		return nil
	}

	// Try matching parsed show title first.
	series, ok := seriesByTitle[strings.ToLower(parsed.ShowTitle)]
	if !ok {
		// Fallback: try matching against the top-level folder name within the library root.
		rel, err := filepath.Rel(lib.RootPath, path)
		if err == nil {
			topDir := strings.SplitN(rel, string(filepath.Separator), 2)[0]
			folderTitle := cleanFolderName(topDir)
			series, ok = seriesByTitle[strings.ToLower(folderTitle)]
		}
	}
	if !ok {
		logger.Debug("library scan: no matching series for title",
			"title", parsed.ShowTitle,
			"path", path,
		)
		return nil
	}

	// Load all episodes for the series.
	episodes, err := q.ListEpisodesBySeriesID(ctx, series.ID)
	if err != nil {
		return err
	}

	type epKey struct{ season, episode int }
	epIndex := make(map[epKey]dbsqlite.Episode, len(episodes))
	for _, ep := range episodes {
		epIndex[epKey{int(ep.SeasonNumber), int(ep.EpisodeNumber)}] = ep
	}

	fi, err := os.Stat(path)
	if err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)

	for _, epNum := range info.Episodes {
		ep, ok := epIndex[epKey{info.Season, epNum}]
		if !ok {
			logger.Debug("library scan: no matching episode",
				"series", series.Title,
				"season", info.Season,
				"episode", epNum,
				"path", path,
			)
			continue
		}

		qualityJSON, _ := json.Marshal(plugin.Quality{})

		if _, err := q.CreateEpisodeFile(ctx, dbsqlite.CreateEpisodeFileParams{
			ID:          uuid.New().String(),
			EpisodeID:   ep.ID,
			SeriesID:    series.ID,
			Path:        path,
			SizeBytes:   fi.Size(),
			QualityJson: string(qualityJSON),
			ImportedAt:  now,
			IndexedAt:   now,
		}); err != nil {
			logger.Warn("library scan: failed to create episode_file record",
				"path", path,
				"episode_id", ep.ID,
				"error", err,
			)
			continue
		}

		// Mark episode as having a file.
		if _, err := q.UpdateEpisode(ctx, dbsqlite.UpdateEpisodeParams{
			ID:             ep.ID,
			Title:          ep.Title,
			Overview:       ep.Overview,
			AirDate:        ep.AirDate,
			HasFile:        1,
			StillPath:      ep.StillPath,
			RuntimeMinutes: ep.RuntimeMinutes,
		}); err != nil {
			logger.Warn("library scan: failed to mark episode has_file",
				"episode_id", ep.ID,
				"error", err,
			)
		}

		logger.Info("library scan: indexed episode file",
			"series", series.Title,
			"season", info.Season,
			"episode", epNum,
			"path", path,
		)
	}

	return nil
}
