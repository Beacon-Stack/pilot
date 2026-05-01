// Package importer handles completed downloads by linking episode files into the
// library directory tree, creating episode_files records, and marking episodes
// as having a file on disk.
package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/beacon-stack/pilot/internal/core/mediamanagement"
	"github.com/beacon-stack/pilot/internal/core/parser"
	"github.com/beacon-stack/pilot/internal/core/renamer"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/events"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

// epKey is used to index episodes by (season, episode) number.
type epKey struct{ season, episode int }

// videoExtensions is the set of file extensions considered video files.
var videoExtensions = map[string]bool{
	".mkv":  true,
	".mp4":  true,
	".avi":  true,
	".ts":   true,
	".m2ts": true,
	".mov":  true,
	".wmv":  true,
}

// Service subscribes to TypeDownloadDone events and imports completed files
// into the library directory tree.
type Service struct {
	q      db.Querier
	bus    *events.Bus
	logger *slog.Logger
	mm     *mediamanagement.Service
}

// NewService creates a new Service.
func NewService(q db.Querier, bus *events.Bus, logger *slog.Logger, mm *mediamanagement.Service) *Service {
	return &Service{q: q, bus: bus, logger: logger, mm: mm}
}

// Subscribe registers the import handler on the event bus.
// Call once during application startup.
func (s *Service) Subscribe() {
	s.bus.Subscribe(func(ctx context.Context, e events.Event) {
		if e.Type != events.TypeDownloadDone {
			return
		}
		grabID, _ := e.Data["grab_id"].(string)
		contentPath, _ := e.Data["content_path"].(string)
		if grabID == "" {
			s.logger.Warn("import: TypeDownloadDone event missing grab_id")
			return
		}
		if err := s.ImportFile(ctx, grabID, contentPath); err != nil {
			s.logger.Error("import failed",
				"grab_id", grabID,
				"content_path", contentPath,
				"error", err,
			)
			// Best-effort series_title lookup so the activity row
			// renders "Import failed for <Show>" instead of just
			// "Import failed for ".
			data := map[string]any{
				"grab_id": grabID,
				"error":   err.Error(),
				"reason":  err.Error(),
			}
			if grab, gErr := s.q.GetGrabByID(ctx, grabID); gErr == nil {
				if series, sErr := s.q.GetSeries(ctx, grab.SeriesID); sErr == nil {
					data["series_title"] = series.Title
				}
			}
			s.bus.Publish(ctx, events.Event{
				Type:   events.TypeImportFailed,
				ShowID: e.ShowID,
				Data:   data,
			})
		}
	})
}

// ImportFile performs the full import pipeline for a completed download.
// contentPath may be a single video file or a directory (season pack).
func (s *Service) ImportFile(ctx context.Context, grabID, contentPath string) error {
	s.logger.Info("import started", "grab_id", grabID, "content_path", contentPath)

	grab, err := s.q.GetGrabByID(ctx, grabID)
	if err != nil {
		return fmt.Errorf("loading grab %q: %w", grabID, err)
	}

	return s.importIntoSeries(ctx, grab, qualityFromGrab(grab), contentPath, grabID)
}

// ImportFromHaulRecord runs the import pipeline against a Haul history
// record without requiring a pre-existing Pilot grab_history row.
// Used by the "import from Haul" flow on the Activity page and the
// per-episode "Haul has it" badge — surfaces files that landed in
// Haul's downloads directory but never got attached to a Pilot
// episode (because the original grab lost its episode_id, or the
// torrent was added directly in Haul, or the file pre-dates the
// importer fix).
//
// seriesID identifies the target series in Pilot's DB; contentPath is
// the absolute path Haul reports as save_path/name. quality is taken
// as the caller specifies — typically zero plugin.Quality, since the
// caller doesn't know what the file actually is and the parser will
// re-derive what it can from the filename.
func (s *Service) ImportFromHaulRecord(ctx context.Context, seriesID, contentPath string, quality plugin.Quality) error {
	s.logger.Info("import started (from Haul record)", "series_id", seriesID, "content_path", contentPath)

	// Synthesize a minimal grab for downstream call sites that read
	// grab.SeriesID. No grab_history row is created — this import is
	// orthogonal to the grab pipeline.
	grab := db.GrabHistory{SeriesID: seriesID}
	return s.importIntoSeries(ctx, grab, quality, contentPath, "")
}

// importIntoSeries is the post-grab-loaded portion of ImportFile.
// Extracted so both the regular grab-driven path AND the
// import-from-Haul path can share it. grabID is used only for the
// final TypeImportComplete event Data; pass "" when there's no
// associated grab.
func (s *Service) importIntoSeries(
	ctx context.Context,
	grab db.GrabHistory,
	quality plugin.Quality,
	contentPath string,
	grabID string,
) error {
	episodes, err := s.q.ListEpisodesBySeriesID(ctx, grab.SeriesID)
	if err != nil {
		return fmt.Errorf("loading episodes for series %q: %w", grab.SeriesID, err)
	}

	epIndex := make(map[epKey]db.Episode, len(episodes))
	for _, ep := range episodes {
		epIndex[epKey{int(ep.SeasonNumber), int(ep.EpisodeNumber)}] = ep
	}

	info, statErr := os.Stat(contentPath)
	if statErr != nil {
		return fmt.Errorf("stat content path %q: %w", contentPath, statErr)
	}

	if !info.IsDir() {
		if !videoExtensions[filepath.Ext(contentPath)] {
			return fmt.Errorf("not a recognised video extension: %q", filepath.Ext(contentPath))
		}
		if err := s.importSingleFile(ctx, contentPath, grab, epIndex, quality); err != nil {
			return err
		}
	} else {
		// Directory: season pack — walk and import each video file individually.
		var importErr error
		walkErr := filepath.WalkDir(contentPath, func(path string, d os.DirEntry, werr error) error {
			if werr != nil || d.IsDir() {
				return werr
			}
			if !videoExtensions[filepath.Ext(path)] {
				return nil
			}
			if err := s.importSingleFile(ctx, path, grab, epIndex, quality); err != nil {
				s.logger.Warn("import: failed to import file from season pack",
					"path", path,
					"error", err,
				)
				importErr = err // record last error but continue with remaining files
			}
			return nil
		})
		if walkErr != nil {
			return fmt.Errorf("walking content directory %q: %w", contentPath, walkErr)
		}
		if importErr != nil {
			// At least one file failed; surface it so the caller can log/emit an event.
			return importErr
		}
	}

	// Publish a single TypeImportComplete for the whole grab regardless
	// of single-file vs season-pack — previously only the directory
	// path emitted, leaving the activity log silent for ~80% of grabs
	// (single episodes are far more common than season packs).
	//
	// series_title + quality are read by the activity classifier
	// (internal/core/activity/service.go classify()). The previous emit
	// omitted both, leaving rows rendered as "Imported release —
	// unknown" with no series name.
	seriesTitle := ""
	if series, sErr := s.q.GetSeries(ctx, grab.SeriesID); sErr == nil {
		seriesTitle = series.Title
	}
	s.bus.Publish(ctx, events.Event{
		Type:   events.TypeImportComplete,
		ShowID: grab.SeriesID,
		Data: map[string]any{
			"grab_id":      grabID,
			"content_path": contentPath,
			"series_title": seriesTitle,
			"quality":      qualityLabel(quality),
		},
	})

	s.logger.Info("import complete", "series_id", grab.SeriesID, "content_path", contentPath)
	return nil
}

// qualityLabel renders a plugin.Quality as a short human label
// ("1080p WEBDL", "2160p BluRay", "1080p"). Mirrors what Prism's
// importer emits so the activity classifiers stay aligned.
func qualityLabel(q plugin.Quality) string {
	if q.Source != "" {
		return string(q.Resolution) + " " + string(q.Source)
	}
	return string(q.Resolution)
}

// importSingleFile parses the filename, matches it to an episode, and calls
// AttachFile to do the actual disk transfer and DB write.
func (s *Service) importSingleFile(
	ctx context.Context,
	srcPath string,
	grab db.GrabHistory,
	epIndex map[epKey]db.Episode,
	quality plugin.Quality,
) error {
	parsed := parser.Parse(filepath.Base(srcPath))
	info := parsed.EpisodeInfo

	if len(info.Episodes) == 0 {
		s.logger.Warn("import: could not parse season/episode from filename, skipping",
			"path", srcPath,
		)
		return nil
	}

	// Import each episode number mentioned in the filename (e.g. multi-episode files).
	for _, epNum := range info.Episodes {
		ep, ok := epIndex[epKey{info.Season, epNum}]
		if !ok {
			s.logger.Warn("import: no matching episode in DB, skipping",
				"series_id", grab.SeriesID,
				"season", info.Season,
				"episode", epNum,
				"path", srcPath,
			)
			continue
		}

		info2, statErr := os.Stat(srcPath)
		if statErr != nil {
			return fmt.Errorf("stat source file %q: %w", srcPath, statErr)
		}

		if err := s.AttachFile(ctx, ep.ID, grab.SeriesID, srcPath, info2.Size(), quality); err != nil {
			return fmt.Errorf("attaching file for S%02dE%02d: %w", info.Season, epNum, err)
		}
	}
	return nil
}

// AttachFile transfers srcPath into the library, applying rename if configured.
// Creates the episode_files record and marks the episode as having a file.
func (s *Service) AttachFile(ctx context.Context, episodeID, seriesID, filePath string, sizeBytes int64, quality plugin.Quality) error {
	// Load media management settings.
	mm, err := s.mm.Get(ctx)
	if err != nil {
		return fmt.Errorf("loading media management settings: %w", err)
	}

	// Compute destination path.
	destPath := filePath
	if mm.RenameEpisodes {
		// Load series and episode info for the renamer.
		series, seriesErr := s.q.GetSeries(ctx, seriesID)
		if seriesErr != nil {
			return fmt.Errorf("loading series for rename: %w", seriesErr)
		}
		ep, epErr := s.q.GetEpisode(ctx, episodeID)
		if epErr != nil {
			return fmt.Errorf("loading episode for rename: %w", epErr)
		}
		lib, libErr := s.q.GetLibrary(ctx, series.LibraryID)
		if libErr != nil {
			return fmt.Errorf("loading library for rename: %w", libErr)
		}

		// Use library-level format overrides if set, else global settings.
		episodeFormat := mm.StandardEpisodeFormat
		if lib.NamingFormat.Valid && lib.NamingFormat.String != "" {
			episodeFormat = lib.NamingFormat.String
		}
		seriesFolderFormat := mm.SeriesFolderFormat
		seasonFolderFormat := mm.SeasonFolderFormat
		if lib.FolderFormat.Valid && lib.FolderFormat.String != "" {
			seriesFolderFormat = lib.FolderFormat.String
		}

		colon := renamer.ColonReplacement(mm.ColonReplacement)

		destPath = renamer.DestPath(
			lib.RootPath,
			episodeFormat,
			seriesFolderFormat,
			seasonFolderFormat,
			renamer.Series{Title: series.Title, Year: int(series.Year)},
			renamer.Episode{
				SeasonNumber:  int(ep.SeasonNumber),
				EpisodeNumber: int(ep.EpisodeNumber),
				Title:         ep.Title,
				AirDate:       ep.AirDate.String,
			},
			quality, colon,
			filepath.Ext(filePath),
		)

		s.logger.Info("rename computed",
			"src", filePath,
			"dest", destPath,
		)
	}

	// Create destination directory.
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("creating destination directory: %w", err)
	}

	// Transfer: hardlink with copy fallback.
	if destPath != filePath {
		if err := transferFile(filePath, destPath); err != nil {
			return fmt.Errorf("transferring %q → %q: %w", filePath, destPath, err)
		}
	}

	// Copy extra files (subtitles, NFOs, etc.) when configured.
	if mm.ImportExtraFiles && len(mm.ExtraFileExtensions) > 0 {
		copyExtraFiles(s.logger, filepath.Dir(filePath), filepath.Dir(destPath), mm.ExtraFileExtensions)
	}

	qualityJSON, err := json.Marshal(quality)
	if err != nil {
		return fmt.Errorf("marshaling quality: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	if _, err := s.q.CreateEpisodeFile(ctx, db.CreateEpisodeFileParams{
		ID:          uuid.New().String(),
		EpisodeID:   episodeID,
		SeriesID:    seriesID,
		Path:        destPath,
		SizeBytes:   sizeBytes,
		QualityJson: string(qualityJSON),
		ImportedAt:  now,
		IndexedAt:   now,
	}); err != nil {
		return fmt.Errorf("creating episode_file record: %w", err)
	}

	// Mark the episode as having a file.  Fetch current values first so we
	// don't clobber title/overview/air_date.
	ep, err := s.q.GetEpisode(ctx, episodeID)
	if err != nil {
		return fmt.Errorf("fetching episode %q to update has_file: %w", episodeID, err)
	}
	if _, err := s.q.UpdateEpisode(ctx, db.UpdateEpisodeParams{
		ID:             ep.ID,
		Title:          ep.Title,
		Overview:       ep.Overview,
		AirDate:        ep.AirDate,
		HasFile:        true,
		StillPath:      ep.StillPath,
		RuntimeMinutes: ep.RuntimeMinutes,
	}); err != nil {
		return fmt.Errorf("marking episode %q as has_file: %w", episodeID, err)
	}

	s.logger.Info("episode file attached",
		"episode_id", episodeID,
		"series_id", seriesID,
		"path", destPath,
	)
	return nil
}

// qualityFromGrab reconstructs a plugin.Quality from the denormalized fields
// stored in grab_history.
func qualityFromGrab(g db.GrabHistory) plugin.Quality {
	return plugin.Quality{
		Resolution: plugin.Resolution(g.ReleaseResolution),
		Source:     plugin.Source(g.ReleaseSource),
		Codec:      plugin.Codec(g.ReleaseCodec),
		HDR:        plugin.HDRFormat(g.ReleaseHdr),
	}
}

// transferFile copies src to dst using hardlink first, falling back to
// io.Copy. The source is intentionally NOT deleted — the download client's
// seed lifecycle handles cleanup.
func transferFile(src, dst string) error {
	if err := os.Link(src, dst); err == nil {
		return nil
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}

// copyExtraFiles walks srcDir and transfers any file whose extension matches
// one of exts into destDir. Errors are logged but do not abort the import.
func copyExtraFiles(logger *slog.Logger, srcDir, destDir string, exts []string) {
	extSet := make(map[string]bool, len(exts))
	for _, e := range exts {
		e = strings.TrimSpace(e)
		if e != "" && e[0] != '.' {
			e = "." + e
		}
		extSet[strings.ToLower(e)] = true
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		logger.Warn("extra files: cannot read source dir", "dir", srcDir, "error", err)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !extSet[strings.ToLower(filepath.Ext(name))] {
			continue
		}
		src := filepath.Join(srcDir, name)
		dst := filepath.Join(destDir, name)
		if err := transferFile(src, dst); err != nil {
			logger.Warn("extra files: transfer failed", "src", src, "dst", dst, "error", err)
		}
	}
}
