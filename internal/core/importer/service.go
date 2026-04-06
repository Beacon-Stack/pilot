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

	"github.com/beacon-media/pilot/internal/core/mediamanagement"
	"github.com/beacon-media/pilot/internal/core/parser"
	dbsqlite "github.com/beacon-media/pilot/internal/db/generated/sqlite"
	"github.com/beacon-media/pilot/internal/events"
	"github.com/beacon-media/pilot/pkg/plugin"
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
	q      dbsqlite.Querier
	bus    *events.Bus
	logger *slog.Logger
	mm     *mediamanagement.Service
}

// NewService creates a new Service.
func NewService(q dbsqlite.Querier, bus *events.Bus, logger *slog.Logger, mm *mediamanagement.Service) *Service {
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
			s.bus.Publish(ctx, events.Event{
				Type: events.TypeImportFailed,
				Data: map[string]any{
					"grab_id": grabID,
					"error":   err.Error(),
				},
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

	// Load all episodes for the series so we can match by season/episode number.
	episodes, err := s.q.ListEpisodesBySeriesID(ctx, grab.SeriesID)
	if err != nil {
		return fmt.Errorf("loading episodes for series %q: %w", grab.SeriesID, err)
	}

	// Build a (season, episode) → Episode index for fast lookup.
	epIndex := make(map[epKey]dbsqlite.Episode, len(episodes))
	for _, ep := range episodes {
		epIndex[epKey{int(ep.SeasonNumber), int(ep.EpisodeNumber)}] = ep
	}

	quality := qualityFromGrab(grab)

	info, statErr := os.Stat(contentPath)
	if statErr != nil {
		return fmt.Errorf("stat content path %q: %w", contentPath, statErr)
	}

	if !info.IsDir() {
		// Single-file download.
		if !videoExtensions[filepath.Ext(contentPath)] {
			return fmt.Errorf("not a recognised video extension: %q", filepath.Ext(contentPath))
		}
		return s.importSingleFile(ctx, contentPath, grab, epIndex, quality)
	}

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

	s.bus.Publish(ctx, events.Event{
		Type:   events.TypeImportComplete,
		ShowID: grab.SeriesID,
		Data: map[string]any{
			"grab_id":      grabID,
			"content_path": contentPath,
		},
	})

	s.logger.Info("import complete", "series_id", grab.SeriesID, "content_path", contentPath)
	return nil
}

// importSingleFile parses the filename, matches it to an episode, and calls
// AttachFile to do the actual disk transfer and DB write.
func (s *Service) importSingleFile(
	ctx context.Context,
	srcPath string,
	grab dbsqlite.GrabHistory,
	epIndex map[epKey]dbsqlite.Episode,
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

// AttachFile transfers srcPath into the library (or records an existing path),
// creates the episode_files record, and marks the episode as having a file.
//
// The destination path is srcPath as-is when media management renaming is
// disabled; when renaming is enabled the caller is expected to have already
// computed the destination (this method currently records srcPath directly —
// the renamer integration can be layered in later without changing the
// signature).
func (s *Service) AttachFile(ctx context.Context, episodeID, seriesID, filePath string, sizeBytes int64, quality plugin.Quality) error {
	// Destination is currently the source path (rename is a separate step).
	destPath := filePath

	// Load media management settings to honour extra-file copying.
	mm, err := s.mm.Get(ctx)
	if err != nil {
		return fmt.Errorf("loading media management settings: %w", err)
	}

	// If the file is not already in the library root we need to transfer it.
	// Detect by checking whether destPath already exists; if not, perform
	// hardlink-with-copy-fallback.  When srcPath == destPath the file is
	// already in place (e.g. library-scan attach path).
	if _, statErr := os.Stat(destPath); os.IsNotExist(statErr) {
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

	if _, err := s.q.CreateEpisodeFile(ctx, dbsqlite.CreateEpisodeFileParams{
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
	if _, err := s.q.UpdateEpisode(ctx, dbsqlite.UpdateEpisodeParams{
		ID:       ep.ID,
		Title:    ep.Title,
		Overview: ep.Overview,
		AirDate:  ep.AirDate,
		HasFile:  1,
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
func qualityFromGrab(g dbsqlite.GrabHistory) plugin.Quality {
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
