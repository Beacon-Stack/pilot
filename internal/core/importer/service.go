// Package importer handles completed downloads by linking episode files into the
// library directory tree, creating episode_files records, and marking episodes
// as having a file on disk.
package importer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/beacon-stack/pilot/internal/core/blocklist"
	"github.com/beacon-stack/pilot/internal/core/mediacheck"
	"github.com/beacon-stack/pilot/internal/core/mediamanagement"
	"github.com/beacon-stack/pilot/internal/core/parser"
	"github.com/beacon-stack/pilot/internal/core/recyclebin"
	"github.com/beacon-stack/pilot/internal/core/renamer"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/diskspace"
	"github.com/beacon-stack/pilot/internal/events"
	releaseparser "github.com/beacon-stack/pilot/internal/parser"
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

// AbsoluteEpisodeResolver maps an anime absolute episode number to the
// TMDB (season, episode) tuple Pilot stores in its database. The
// importer uses this to attach files like "Show - 48" — fansub
// releases that don't carry SxxExx — to the right episode row.
//
// Defined as an interface (rather than a concrete *animelist.Service
// dep) so the importer's tests can stub it without dragging in the
// XML loader. Production wiring passes the real service.
type AbsoluteEpisodeResolver interface {
	AbsoluteToTMDBEpisode(tmdbID, abs int) (season, episode int, ok bool)
}

// ErrUnimportableContent wraps import failures that can never succeed on
// retry with the same content — fake uploads delivering a .exe/.scr, or
// a "season pack" containing no video files at all. These are the only
// failures that warrant blocklisting the release; transient errors
// (full disk, DB hiccup) must not.
var ErrUnimportableContent = errors.New("content can never import")

// missingPathHint is added to the error when a download's path doesn't
// exist for Pilot, the usual sign of a client in a container or on
// another machine.
const missingPathHint = "if the download client runs in a container or on another machine, it reports paths as it sees them: add a remote path mapping in Settings > Remote Path Mappings"

// ErrInsufficientSpace means copying a file into the library would leave
// less free space than the library's min_free_space_gb. It is a soft
// failure: the import is retried later, once space may have been freed.
var ErrInsufficientSpace = errors.New("not enough free space in the library")

// freeSpace and linkFile are diskspace.Free and os.Link; tests replace
// them to force the copy path and a full disk.
var (
	freeSpace = diskspace.Free
	linkFile  = os.Link
)

// Service subscribes to TypeDownloadDone events and imports completed files
// into the library directory tree.
type Service struct {
	q         db.Querier
	bus       *events.Bus
	logger    *slog.Logger
	mm        *mediamanagement.Service
	animeR    AbsoluteEpisodeResolver
	blocklist *blocklist.Service // nil-able; hard failures blocklist the release
	recycle   *recyclebin.Service
}

// NewService creates a new Service. Pass nil for animelist when the
// caller doesn't have one wired up; the importer just won't resolve
// anime absolute-numbered files in that case (same as before this
// dependency existed). blocklistSvc may be nil (tests); hard import
// failures then simply skip the blocklist step.
func NewService(q db.Querier, bus *events.Bus, logger *slog.Logger, mm *mediamanagement.Service, animeR AbsoluteEpisodeResolver, blocklistSvc *blocklist.Service) *Service {
	return &Service{q: q, bus: bus, logger: logger, mm: mm, animeR: animeR, blocklist: blocklistSvc, recycle: recyclebin.New(mm, logger)}
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
		err := s.ImportFile(ctx, grabID, contentPath)
		s.recordImportOutcome(ctx, grabID, contentPath, err)
		if err != nil {
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

// recordImportOutcome persists the import result on the grab row, and on
// a HARD failure (content that can never import — fake uploads)
// blocklists the release so search and RSS never grab it again.
//
// It also notes the attempt (content path, attempt count, time) so
// RetryFailedImports can re-run soft failures later. Hard failures spend
// the whole retry budget at once: retrying a .exe never helps.
func (s *Service) recordImportOutcome(ctx context.Context, grabID, contentPath string, importErr error) {
	now := time.Now().UTC().Format(time.RFC3339)
	minAttempts := int64(0)
	if errors.Is(importErr, ErrUnimportableContent) {
		minAttempts = MaxImportAttempts
	}
	if err := s.q.NoteGrabImportAttempt(ctx, db.NoteGrabImportAttemptParams{
		ContentPath: contentPath,
		MinAttempts: minAttempts,
		AttemptedAt: now,
		ID:          grabID,
	}); err != nil {
		s.logger.Warn("import: failed to note import attempt", "grab_id", grabID, "error", err)
	}
	if importErr == nil {
		if err := s.q.SetGrabImportState(ctx, db.SetGrabImportStateParams{
			ImportStatus: "imported",
			ImportedAt:   now,
			ImportError:  "",
			ID:           grabID,
		}); err != nil {
			s.logger.Warn("import: failed to record import success", "grab_id", grabID, "error", err)
		}
		return
	}

	if err := s.q.SetGrabImportState(ctx, db.SetGrabImportStateParams{
		ImportStatus: "failed",
		ImportedAt:   "",
		ImportError:  importErr.Error(),
		ID:           grabID,
	}); err != nil {
		s.logger.Warn("import: failed to record import failure", "grab_id", grabID, "error", err)
	}

	if !errors.Is(importErr, ErrUnimportableContent) || s.blocklist == nil {
		return
	}
	grab, err := s.q.GetGrabByID(ctx, grabID)
	if err != nil {
		s.logger.Warn("import: cannot blocklist, grab lookup failed", "grab_id", grabID, "error", err)
		return
	}
	entry := blocklist.StallEntry{
		SeriesID:     grab.SeriesID,
		ReleaseGUID:  grab.ReleaseGuid,
		ReleaseTitle: grab.ReleaseTitle,
		Protocol:     grab.Protocol,
		Size:         grab.Size,
		Notes:        "import failed: " + importErr.Error(),
	}
	if grab.EpisodeID != nil {
		entry.EpisodeID = *grab.EpisodeID
	}
	if grab.IndexerID != nil {
		entry.IndexerID = *grab.IndexerID
	}
	if grab.InfoHash != nil {
		entry.InfoHash = *grab.InfoHash
	}
	if err := s.blocklist.AddFromImportFailure(ctx, entry); err != nil && !errors.Is(err, blocklist.ErrAlreadyBlocklisted) {
		s.logger.Warn("import: failed to blocklist unimportable release",
			"grab_id", grabID, "release", grab.ReleaseTitle, "error", err)
		return
	}
	s.logger.Info("import: blocklisted unimportable release",
		"release", grab.ReleaseTitle, "reason", importErr.Error())
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
		if errors.Is(statErr, fs.ErrNotExist) {
			return fmt.Errorf("stat content path %q: %w (%s)", contentPath, statErr, missingPathHint)
		}
		return fmt.Errorf("stat content path %q: %w", contentPath, statErr)
	}

	if !info.IsDir() {
		if !videoExtensions[filepath.Ext(contentPath)] {
			return fmt.Errorf("%w: not a recognised video extension: %q", ErrUnimportableContent, filepath.Ext(contentPath))
		}
		if err := s.importSingleFile(ctx, contentPath, grab, epIndex, quality); err != nil {
			return err
		}
	} else {
		// Directory: season pack — walk and import each video file individually.
		var importErr error
		videoCount := 0
		walkErr := filepath.WalkDir(contentPath, func(path string, d os.DirEntry, werr error) error {
			if werr != nil || d.IsDir() {
				return werr
			}
			if !videoExtensions[filepath.Ext(path)] {
				return nil
			}
			videoCount++
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
		if videoCount == 0 {
			// A "season pack" with zero video files is a fake upload —
			// same class as the single-file .exe case.
			return fmt.Errorf("%w: directory contains no video files", ErrUnimportableContent)
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

// resolveEpisodeInfo parses srcPath's filename and applies the anime
// absolute-episode fallback when SxxExx isn't present.
//
// Headline regression case: "[SubsPlease] Jujutsu Kaisen - 48 [1080p].mkv"
// has no SxxExx but does have absolute=48. Without the fallback, the
// importer skipped the file silently. With the fallback, the animelist
// service maps absolute → TMDB (season, episode) using the show's
// tmdbID, and the importer attaches the file to the right row.
//
// Pure of filesystem and disk I/O — only DB lookup (GetSeries) and the
// resolver call. Tested via TestResolveEpisodeInfo_*.
func (s *Service) resolveEpisodeInfo(
	ctx context.Context,
	srcPath string,
	grab db.GrabHistory,
) parser.EpisodeInfo {
	parsed := parser.Parse(filepath.Base(srcPath))
	info := parsed.EpisodeInfo

	// Anime absolute fallback: when the filename doesn't have SxxExx
	// but the parser extracted an absolute episode number AND a
	// resolver is wired up, look up the show's anime mapping to
	// recover the TMDB (season, episode) tuple.
	if len(info.Episodes) == 0 && info.AbsoluteEpisode > 0 && s.animeR != nil {
		series, sErr := s.q.GetSeries(ctx, grab.SeriesID)
		if sErr == nil {
			if season, ep, ok := s.animeR.AbsoluteToTMDBEpisode(int(series.TmdbID), info.AbsoluteEpisode); ok {
				s.logger.Info("import: resolved anime absolute episode via animelist",
					"path", srcPath,
					"absolute", info.AbsoluteEpisode,
					"season", season,
					"episode", ep,
				)
				info.Season = season
				info.Episodes = []int{ep}
			}
		}
	}

	return info
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
	info := s.resolveEpisodeInfo(ctx, srcPath, grab)

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
	// A file with no data (an incomplete, preallocated download) would mark
	// the episode as having a file, and it would never be downloaded again.
	if err := mediacheck.Check(filePath); err != nil {
		return fmt.Errorf("not importing: %w", err)
	}

	// Load media management settings.
	mm, err := s.mm.Get(ctx)
	if err != nil {
		return fmt.Errorf("loading media management settings: %w", err)
	}

	// Compute the destination: the library's series and season folders,
	// always. With renaming on, the file is named by the episode format;
	// with it off, it keeps its original name (as in Sonarr). Before, with
	// renaming off nothing was moved into the library at all: the
	// download path itself was recorded as the library file.
	var destPath string
	var minFree uint64
	{
		series, seriesErr := s.q.GetSeries(ctx, seriesID)
		if seriesErr != nil {
			return fmt.Errorf("loading series for import: %w", seriesErr)
		}
		ep, epErr := s.q.GetEpisode(ctx, episodeID)
		if epErr != nil {
			return fmt.Errorf("loading episode for import: %w", epErr)
		}
		lib, libErr := s.q.GetLibrary(ctx, series.LibraryID)
		if libErr != nil {
			return fmt.Errorf("loading library for import: %w", libErr)
		}

		// Use library-level format overrides if set, else global settings.
		episodeFormat := mm.StandardEpisodeFormat
		if lib.NamingFormat != nil && *lib.NamingFormat != "" {
			episodeFormat = *lib.NamingFormat
		}
		seriesFolderFormat := mm.SeriesFolderFormat
		seasonFolderFormat := mm.SeasonFolderFormat
		if lib.FolderFormat != nil && *lib.FolderFormat != "" {
			seriesFolderFormat = *lib.FolderFormat
		}
		if lib.MinFreeSpaceGb > 0 {
			minFree = uint64(lib.MinFreeSpaceGb) * diskspace.GB
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
				AirDate: func() string {
					if ep.AirDate == nil {
						return ""
					}
					return *ep.AirDate
				}(),
			},
			quality, colon,
			filepath.Ext(filePath),
		)
		if !mm.RenameEpisodes {
			destPath = filepath.Join(filepath.Dir(destPath), filepath.Base(filePath))
		}

		s.logger.Info("import destination computed",
			"src", filePath,
			"dest", destPath,
			"renamed", mm.RenameEpisodes,
		)
	}

	// A retried (or manually re-run) import of a season pack that partly
	// succeeded earlier reaches files that are already recorded at
	// destPath. episode_files.path is UNIQUE, so re-inserting would fail
	// the whole retry, and re-copying would waste time on a cross-device
	// library. An existing row whose file on disk IS this source file
	// (same inode, or same size for a cross-device copy) means this file
	// is done.
	//
	// Anything else at destPath is an older file with the same name, e.g.
	// an upgrade when the naming format has no quality token. Treating
	// that as "done" silently kept the old file, so drop the row and let
	// the transfer replace the file.
	alreadyRecorded := false
	existing, err := s.q.GetEpisodeFileByPath(ctx, destPath)
	switch {
	case err == nil && existing.EpisodeID != episodeID:
		return fmt.Errorf("destination %q is already recorded for a different episode (%s)", destPath, existing.EpisodeID)
	case err == nil:
		if sameImportedFile(filePath, destPath, sizeBytes) {
			alreadyRecorded = true
		} else {
			if dErr := s.q.DeleteEpisodeFile(ctx, existing.ID); dErr != nil {
				return fmt.Errorf("removing stale episode_file record: %w", dErr)
			}
		}
	case errors.Is(err, sql.ErrNoRows):
	default:
		return fmt.Errorf("checking existing episode_file record: %w", err)
	}

	if !alreadyRecorded {
		// Create destination directory.
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			if errors.Is(err, fs.ErrPermission) {
				return fmt.Errorf("creating destination directory: %w (the library folder is not writable by Pilot's user, uid %d; fix its ownership and the import will be retried automatically)", err, os.Getuid())
			}
			return fmt.Errorf("creating destination directory: %w", err)
		}

		// Transfer: hardlink with copy fallback.
		if destPath != filePath {
			// A different file already at destPath (an upgrade whose
			// filename didn't change) is about to be overwritten. Once the
			// new file is fully staged, move the old one to the recycle
			// bin (if set); if that fails, the old file stays.
			beforeReplace := func() error {
				if _, statErr := os.Lstat(destPath); errors.Is(statErr, fs.ErrNotExist) || sameImportedFile(filePath, destPath, sizeBytes) {
					return nil // nothing there, or it is this same file
				}
				if _, err := s.recycle.Preserve(ctx, destPath, s.libraryRoot(ctx, seriesID)); err != nil {
					return fmt.Errorf("recycling the file being replaced: %w", err)
				}
				return nil
			}
			if err := transferFileReplacing(filePath, destPath, minFree, beforeReplace); err != nil {
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

		// The new file is in place and recorded: retire any other file
		// this episode had (an upgrade to a different filename).
		s.removeReplacedFiles(ctx, episodeID, seriesID, destPath)
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

// sameImportedFile reports whether dest already holds the file being
// imported: the same inode (linked there by an earlier attempt), or, for a
// cross-device copy, the same size.
func sameImportedFile(src, dest string, srcSize int64) bool {
	di, err := os.Stat(dest)
	if err != nil {
		return false
	}
	if si, err := os.Stat(src); err == nil && os.SameFile(si, di) {
		return true
	}
	return di.Size() == srcSize
}

// removeReplacedFiles deletes the episode's other files after a new one
// was imported at keepPath. Only files inside the series' library root
// are removed from disk; a recorded file anywhere else (a download
// folder, another mount) keeps its bytes and only loses its record.
// Failures are logged, not returned: the new file is already imported.
func (s *Service) removeReplacedFiles(ctx context.Context, episodeID, seriesID, keepPath string) {
	files, err := s.q.ListEpisodeFilesByEpisodeID(ctx, episodeID)
	if err != nil {
		s.logger.Warn("import: listing episode files for replacement", "episode_id", episodeID, "error", err)
		return
	}
	root := s.libraryRoot(ctx, seriesID)
	for _, f := range files {
		if f.Path == keepPath {
			continue
		}
		if root != "" && pathWithin(root, f.Path) {
			recycledTo, err := s.recycle.Delete(ctx, f.Path, root)
			if err != nil {
				s.logger.Warn("import: could not delete replaced file; keeping its record",
					"episode_id", episodeID, "path", f.Path, "error", err)
				continue
			}
			s.logger.Info("import: removed replaced file", "episode_id", episodeID, "path", f.Path,
				"replaced_by", keepPath, "recycled_to", recycledTo)
		} else {
			s.logger.Info("import: replaced file is outside the library; leaving it on disk",
				"episode_id", episodeID, "path", f.Path)
		}
		if err := s.q.DeleteEpisodeFile(ctx, f.ID); err != nil {
			s.logger.Warn("import: removing replaced file record", "episode_file_id", f.ID, "error", err)
		}
	}
}

// libraryRoot returns the root folder of the series' library, or "" when
// it can't be determined (then nothing is deleted from disk).
func (s *Service) libraryRoot(ctx context.Context, seriesID string) string {
	series, err := s.q.GetSeries(ctx, seriesID)
	if err != nil {
		return ""
	}
	lib, err := s.q.GetLibrary(ctx, series.LibraryID)
	if err != nil {
		return ""
	}
	return lib.RootPath
}

// pathWithin reports whether p is strictly inside root.
func pathWithin(root, p string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(p))
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func checkSpaceForCopy(src, dst string, minFree uint64) error {
	if minFree == 0 {
		return nil
	}
	si, err := os.Stat(src)
	if err != nil {
		return err
	}
	avail, err := freeSpace(filepath.Dir(dst))
	if err != nil {
		return nil //nolint:nilerr // unknown free space: don't block the import
	}
	need := uint64(si.Size()) + minFree
	if avail < need {
		return fmt.Errorf("%w: copying %.1f GB would leave %.1f GB free, below the library minimum of %.0f GB",
			ErrInsufficientSpace, float64(si.Size())/diskspace.GB,
			(float64(avail)-float64(si.Size()))/diskspace.GB, float64(minFree)/diskspace.GB)
	}
	return nil
}

// qualityFromGrab reconstructs a plugin.Quality from the denormalized fields
// stored in grab_history. Name must be filled in too: the renamer's
// {Quality Full} token renders Name, and without it every imported
// filename lost its quality (and upgrades collided on one filename).
func qualityFromGrab(g db.GrabHistory) plugin.Quality {
	q := plugin.Quality{
		Resolution: plugin.Resolution(g.ReleaseResolution),
		Source:     plugin.Source(g.ReleaseSource),
		Codec:      plugin.Codec(g.ReleaseCodec),
		HDR:        plugin.HDRFormat(g.ReleaseHdr),
	}
	q.Name = releaseparser.QualityName(q)
	// The revision (PROPER/REPACK) isn't a grab_history column; recover it
	// from the release title. Without it an imported PROPER looks like an
	// original, and RSS sync would grab the same PROPER again as an upgrade.
	q.Revision = releaseparser.Parse(g.ReleaseTitle).Revision.Version
	return q
}

// transferFile copies src to dst using hardlink first, falling back to
// io.Copy. The source is intentionally NOT deleted — the download client's
// seed lifecycle handles cleanup.
//
// The new file is staged at a temp path and renamed over dst, never
// written into dst in place. That matters on retries: if dst is already a
// hardlink to src (an earlier attempt linked it), opening dst with
// O_TRUNC would truncate the shared inode and destroy the download.
//
// A hardlink takes no space. Before falling back to a copy, the copy is
// refused with ErrInsufficientSpace if it would leave less than minFree
// bytes free where dst lives (0 = no minimum). If free space can't be
// read, the copy goes ahead: an unknown must not block every import.
func transferFile(src, dst string, minFree uint64) error {
	return transferFileReplacing(src, dst, minFree, nil)
}

// transferFileReplacing is transferFile, calling beforeReplace (when not
// nil) after the new file is staged and just before it is renamed over
// dst. An error from beforeReplace aborts the transfer and leaves dst as
// it was.
func transferFileReplacing(src, dst string, minFree uint64, beforeReplace func() error) error {
	if di, err := os.Stat(dst); err == nil {
		if si, err := os.Stat(src); err == nil && os.SameFile(si, di) {
			return nil // already linked by an earlier attempt
		}
	}

	tmp := dst + ".pilot-tmp"
	_ = os.Remove(tmp) // leftover from an interrupted attempt

	if err := linkFile(src, tmp); err == nil {
		if beforeReplace != nil {
			if err := beforeReplace(); err != nil {
				os.Remove(tmp)
				return err
			}
		}
		if err := os.Rename(tmp, dst); err != nil {
			os.Remove(tmp)
			return err
		}
		return nil
	}

	if err := checkSpaceForCopy(src, dst, minFree); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(tmp)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if beforeReplace != nil {
		if err := beforeReplace(); err != nil {
			os.Remove(tmp)
			return err
		}
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
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
		if err := transferFile(src, dst, 0); err != nil {
			logger.Warn("extra files: transfer failed", "src", src, "dst", dst, "error", err)
		}
	}
}
