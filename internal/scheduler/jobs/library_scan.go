package jobs

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/screenarr/screenarr/internal/core/library"
	"github.com/screenarr/screenarr/internal/core/parser"
	"github.com/screenarr/screenarr/internal/core/show"
	dbsqlite "github.com/screenarr/screenarr/internal/db/generated/sqlite"
	"github.com/screenarr/screenarr/internal/scheduler"
	"github.com/screenarr/screenarr/pkg/plugin"
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

// LibraryScan returns a Job that walks all library root directories, matches
// video files to known series/episodes, and creates episode_file records for
// any newly discovered files.
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
	// Build a set of already-known file paths so we don't re-import them.
	knownPaths, err := q.ListAllEpisodeFilePaths(ctx)
	if err != nil {
		return err
	}
	known := make(map[string]bool, len(knownPaths))
	for _, p := range knownPaths {
		known[p] = true
	}

	libs, err := libSvc.List(ctx)
	if err != nil {
		return err
	}

	for _, lib := range libs {
		if err := scanLibrary(ctx, lib, showSvc, q, known, logger); err != nil {
			// Log per-library errors but continue with remaining libraries.
			logger.Warn("library scan error",
				"library_id", lib.ID,
				"library_name", lib.Name,
				"error", err,
			)
		}
	}
	return nil
}

// scanLibrary walks a single library root directory and imports new files.
func scanLibrary(
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

	// Match show title against known series (case-insensitive).
	series, ok := seriesByTitle[strings.ToLower(parsed.ShowTitle)]
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
			ID:       ep.ID,
			Title:    ep.Title,
			Overview: ep.Overview,
			AirDate:  ep.AirDate,
			HasFile:  1,
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
