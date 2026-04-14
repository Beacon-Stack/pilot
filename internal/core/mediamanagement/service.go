// Package mediamanagement provides access to global media management settings
// (episode naming formats, colon replacement, extra file importing, etc.).
package mediamanagement

import (
	"context"
	"fmt"
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

// Update persists new settings and returns the saved values.
func (s *Service) Update(ctx context.Context, settings Settings) (Settings, error) {
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
