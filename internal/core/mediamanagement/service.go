// Package mediamanagement provides access to global media management settings
// (episode naming formats, colon replacement, extra file importing, etc.).
package mediamanagement

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	db "github.com/beacon-stack/pilot/internal/db/generated"
)

// Settings is the application-level view of the media_management table.
type Settings struct {
	RenameEpisodes           bool
	StandardEpisodeFormat    string
	DailyEpisodeFormat       string
	AnimeEpisodeFormat       string
	SeriesFolderFormat       string
	SeasonFolderFormat       string
	ColonReplacement         string // "delete" | "dash" | "space-dash" | "smart"
	ImportExtraFiles         bool
	ExtraFileExtensions      []string // parsed from comma-separated DB string
	UnmonitorDeletedEpisodes bool
	// RecycleBinPath is where replaced and deleted episode files go; ""
	// deletes them permanently.
	RecycleBinPath string
	// RecycleBinCleanupDays: files older than this in the recycle bin are
	// deleted; 0 = never.
	RecycleBinCleanupDays int
}

// Service exposes read/write access to the single media_management row.
type Service struct {
	q db.Querier
}

// NewService creates a new Service backed by the given Querier.
func NewService(q db.Querier) *Service {
	return &Service{q: q}
}

// Get returns the current media management settings.
func (s *Service) Get(ctx context.Context) (Settings, error) {
	row, err := s.q.GetMediaManagement(ctx)
	if err != nil {
		return Settings{}, fmt.Errorf("media_management: get: %w", err)
	}
	return fromRow(row), nil
}

// ValidationError explains why settings can't be saved.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// validateRecycleBin checks the recycle bin settings. The folder must be
// absolute and outside every library: inside one, a library scan would
// find recycled files and import them again.
func (s *Service) validateRecycleBin(ctx context.Context, st *Settings) error {
	st.RecycleBinPath = strings.TrimSpace(st.RecycleBinPath)
	if st.RecycleBinCleanupDays < 0 || st.RecycleBinCleanupDays > 3650 {
		return &ValidationError{"recycle bin cleanup must be between 0 (never) and 3650 days"}
	}
	if st.RecycleBinPath == "" {
		return nil
	}
	if !filepath.IsAbs(st.RecycleBinPath) {
		return &ValidationError{"the recycle bin folder must be an absolute path"}
	}
	st.RecycleBinPath = filepath.Clean(st.RecycleBinPath)
	libs, err := s.q.ListLibraries(ctx)
	if err != nil {
		return fmt.Errorf("media_management: listing libraries: %w", err)
	}
	for _, lib := range libs {
		rel, err := filepath.Rel(filepath.Clean(lib.RootPath), st.RecycleBinPath)
		if err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))) {
			return &ValidationError{fmt.Sprintf("the recycle bin can't be inside the library %q: a library scan would import recycled files again", lib.Name)}
		}
	}
	return nil
}

// Update persists new settings and returns the saved values.
func (s *Service) Update(ctx context.Context, settings Settings) (Settings, error) {
	if err := s.validateRecycleBin(ctx, &settings); err != nil {
		return Settings{}, err
	}
	row, err := s.q.UpdateMediaManagement(ctx, db.UpdateMediaManagementParams{
		RenameEpisodes:           settings.RenameEpisodes,
		StandardEpisodeFormat:    settings.StandardEpisodeFormat,
		DailyEpisodeFormat:       settings.DailyEpisodeFormat,
		AnimeEpisodeFormat:       settings.AnimeEpisodeFormat,
		SeriesFolderFormat:       settings.SeriesFolderFormat,
		SeasonFolderFormat:       settings.SeasonFolderFormat,
		ColonReplacement:         settings.ColonReplacement,
		ImportExtraFiles:         settings.ImportExtraFiles,
		ExtraFileExtensions:      strings.Join(settings.ExtraFileExtensions, ","),
		UnmonitorDeletedEpisodes: settings.UnmonitorDeletedEpisodes,
		RecycleBinPath:           settings.RecycleBinPath,
		RecycleBinCleanupDays:    int64(settings.RecycleBinCleanupDays),
	})
	if err != nil {
		return Settings{}, fmt.Errorf("media_management: update: %w", err)
	}
	return fromRow(row), nil
}

// fromRow converts a DB row to a Settings value.
func fromRow(row db.MediaManagement) Settings {
	return Settings{
		RenameEpisodes:           row.RenameEpisodes,
		StandardEpisodeFormat:    row.StandardEpisodeFormat,
		DailyEpisodeFormat:       row.DailyEpisodeFormat,
		AnimeEpisodeFormat:       row.AnimeEpisodeFormat,
		SeriesFolderFormat:       row.SeriesFolderFormat,
		SeasonFolderFormat:       row.SeasonFolderFormat,
		ColonReplacement:         row.ColonReplacement,
		ImportExtraFiles:         row.ImportExtraFiles,
		ExtraFileExtensions:      parseExtensions(row.ExtraFileExtensions),
		UnmonitorDeletedEpisodes: row.UnmonitorDeletedEpisodes,
		RecycleBinPath:           row.RecycleBinPath,
		RecycleBinCleanupDays:    int(row.RecycleBinCleanupDays),
	}
}

// parseExtensions splits a comma-separated extension string, trims whitespace,
// and drops empty tokens.
func parseExtensions(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
