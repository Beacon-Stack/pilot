package show

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/beacon-stack/pilot/internal/core/renamer"
	db "github.com/beacon-stack/pilot/internal/db/generated"
)

// RenameSettings holds the format configuration for renaming files.
type RenameSettings struct {
	EpisodeFormat      string
	SeriesFolderFormat string
	SeasonFolderFormat string
	ColonReplacement   renamer.ColonReplacement
}

// RenamePreview describes a single file rename operation (or proposed rename
// when dry_run=true).
type RenamePreview struct {
	FileID  string
	OldPath string
	NewPath string
	// Extras are subtitle files that move with the episode file.
	Extras []RenameMove
}

// RenameMove is one extra file (a subtitle) moved alongside a rename.
type RenameMove struct {
	OldPath string
	NewPath string
}

// Recycler removes a file, into the recycle bin when one is set.
// Implemented by recyclebin.Service.
type Recycler interface {
	Delete(ctx context.Context, path, libraryRoot string) (recycledTo string, err error)
}

// SetRecycler installs the recycle bin used when a file is deleted from
// disk. Without one, files are deleted permanently.
func (s *Service) SetRecycler(r Recycler) { s.recycle = r }

// libraryRootFor returns the root folder of the series' library, or "".
func (s *Service) libraryRootFor(ctx context.Context, seriesID string) string {
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

// ListFiles returns all episode files associated with the given series ID.
func (s *Service) ListFiles(ctx context.Context, seriesID string) ([]db.EpisodeFile, error) {
	files, err := s.q.ListEpisodeFilesBySeriesID(ctx, seriesID)
	if err != nil {
		return nil, fmt.Errorf("list episode files for series %q: %w", seriesID, err)
	}
	return files, nil
}

// DeleteFile removes an episode_file record.  When deleteFromDisk is true it
// also removes the underlying file from the filesystem and marks the episode as
// no longer having a file.
func (s *Service) DeleteFile(ctx context.Context, fileID string, deleteFromDisk bool) error {
	f, err := s.q.GetEpisodeFile(ctx, fileID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrFileNotFound
		}
		return fmt.Errorf("get episode file %q: %w", fileID, err)
	}

	if deleteFromDisk {
		if s.recycle != nil {
			if _, rmErr := s.recycle.Delete(ctx, f.Path, s.libraryRootFor(ctx, f.SeriesID)); rmErr != nil {
				return fmt.Errorf("remove file from disk %q: %w", f.Path, rmErr)
			}
		} else if rmErr := os.Remove(f.Path); rmErr != nil && !os.IsNotExist(rmErr) {
			return fmt.Errorf("remove file from disk %q: %w", f.Path, rmErr)
		}
	}

	if err := s.q.DeleteEpisodeFile(ctx, fileID); err != nil {
		return fmt.Errorf("delete episode_file record %q: %w", fileID, err)
	}

	// Clear has_file on the associated episode.
	ep, err := s.q.GetEpisode(ctx, f.EpisodeID)
	if err == nil {
		_, _ = s.q.UpdateEpisode(ctx, db.UpdateEpisodeParams{
			ID:             ep.ID,
			Title:          ep.Title,
			Overview:       ep.Overview,
			AirDate:        ep.AirDate,
			HasFile:        false,
			StillPath:      ep.StillPath,
			RuntimeMinutes: ep.RuntimeMinutes,
		})
	}

	return nil
}

// RenameFiles computes (and optionally applies) renames for all episode files
// belonging to a series based on the naming format settings.
func (s *Service) RenameFiles(ctx context.Context, seriesID string, settings RenameSettings, dryRun bool) ([]RenamePreview, error) {
	series, err := s.Get(ctx, seriesID)
	if err != nil {
		return nil, err
	}

	lib, err := s.q.GetLibrary(ctx, series.LibraryID)
	if err != nil {
		return nil, fmt.Errorf("loading library: %w", err)
	}

	files, err := s.q.ListEpisodeFilesBySeriesID(ctx, seriesID)
	if err != nil {
		return nil, fmt.Errorf("listing episode files: %w", err)
	}

	if len(files) == 0 {
		return nil, nil
	}

	// Library-level format overrides win, as they do on import; otherwise
	// a rename would undo the names the importer just gave.
	if lib.NamingFormat != nil && *lib.NamingFormat != "" {
		settings.EpisodeFormat = *lib.NamingFormat
	}
	if lib.FolderFormat != nil && *lib.FolderFormat != "" {
		settings.SeriesFolderFormat = *lib.FolderFormat
	}

	rSeries := renamer.Series{Title: series.Title, Year: int(series.Year)}
	colon := settings.ColonReplacement

	var previews []RenamePreview
	var renameErrors []string

	for _, f := range files {
		// Look up the episode for this file.
		ep, epErr := s.q.GetEpisode(ctx, f.EpisodeID)
		if epErr != nil {
			s.logger.Warn("rename: episode not found for file", "file_id", f.ID, "episode_id", f.EpisodeID)
			continue
		}

		ext := filepath.Ext(f.Path)
		airDate := ""
		if ep.AirDate != nil {
			airDate = *ep.AirDate
		}

		newPath := renamer.DestPath(
			lib.RootPath,
			settings.EpisodeFormat,
			settings.SeriesFolderFormat,
			settings.SeasonFolderFormat,
			rSeries,
			renamer.Episode{
				SeasonNumber:  int(ep.SeasonNumber),
				EpisodeNumber: int(ep.EpisodeNumber),
				Title:         ep.Title,
				AirDate:       airDate,
			},
			renameQuality(f),
			colon,
			ext,
		)

		if newPath == f.Path {
			continue // already correct
		}

		extras := subtitleMoves(f.Path, newPath)
		previews = append(previews, RenamePreview{
			FileID:  f.ID,
			OldPath: f.Path,
			NewPath: newPath,
			Extras:  extras,
		})

		if !dryRun {
			// Create destination directory.
			if mkErr := os.MkdirAll(filepath.Dir(newPath), 0o755); mkErr != nil {
				renameErrors = append(renameErrors, fmt.Sprintf("mkdir %s: %v", filepath.Dir(newPath), mkErr))
				continue
			}

			// Check target doesn't exist.
			if _, statErr := os.Stat(newPath); statErr == nil {
				renameErrors = append(renameErrors, fmt.Sprintf("target already exists: %s", newPath))
				continue
			}

			if renameErr := os.Rename(f.Path, newPath); renameErr != nil {
				renameErrors = append(renameErrors, fmt.Sprintf("rename %s → %s: %v", f.Path, newPath, renameErr))
				continue
			}

			// Update path in DB.
			if dbErr := s.q.UpdateEpisodeFilePath(ctx, db.UpdateEpisodeFilePathParams{
				Path: newPath,
				ID:   f.ID,
			}); dbErr != nil {
				renameErrors = append(renameErrors, fmt.Sprintf("db update %s: %v", f.ID, dbErr))
			}

			s.logger.Info("renamed episode file", "old", f.Path, "new", newPath)

			// Subtitles follow their episode. Never overwrite: an existing
			// target is reported and the subtitle stays where it is.
			for _, m := range extras {
				if _, statErr := os.Lstat(m.NewPath); statErr == nil {
					renameErrors = append(renameErrors, fmt.Sprintf("subtitle target already exists: %s", m.NewPath))
					continue
				}
				if mvErr := os.Rename(m.OldPath, m.NewPath); mvErr != nil {
					renameErrors = append(renameErrors, fmt.Sprintf("rename %s → %s: %v", m.OldPath, m.NewPath, mvErr))
					continue
				}
				s.logger.Info("moved subtitle with episode", "old", m.OldPath, "new", m.NewPath)
			}
		}
	}

	if len(renameErrors) > 0 {
		return previews, fmt.Errorf("rename errors: %s", strings.Join(renameErrors, "; "))
	}
	return previews, nil
}
