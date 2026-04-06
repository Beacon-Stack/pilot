// Package blocklist manages the release blocklist used to skip known-bad releases.
package blocklist

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/beacon-media/pilot/internal/core/dbutil"
	dbsqlite "github.com/beacon-media/pilot/internal/db/generated/sqlite"
)

// ErrAlreadyBlocklisted is returned when adding a GUID that is already on the blocklist.
var ErrAlreadyBlocklisted = errors.New("release already blocklisted")

// Entry is the domain representation of a blocklist record.
type Entry struct {
	ID           string
	SeriesID     string
	SeriesTitle  string
	EpisodeID    string
	ReleaseGUID  string
	ReleaseTitle string
	IndexerID    string
	Protocol     string
	Size         int64
	AddedAt      time.Time
	Notes        string
}

// Service manages the release blocklist.
type Service struct {
	q dbsqlite.Querier
}

// NewService creates a new Service.
func NewService(q dbsqlite.Querier) *Service {
	return &Service{q: q}
}

// Add inserts a new blocklist entry. Returns ErrAlreadyBlocklisted if the GUID
// is already present (the unique index on release_guid enforces this).
func (s *Service) Add(ctx context.Context, seriesID, episodeID, releaseGUID, releaseTitle, indexerID, protocol string, size int64, notes string) error {
	var epID *string
	if episodeID != "" {
		epID = &episodeID
	}
	var idxID *string
	if indexerID != "" {
		idxID = &indexerID
	}
	_, err := s.q.CreateBlocklistEntry(ctx, dbsqlite.CreateBlocklistEntryParams{
		ID:           uuid.New().String(),
		SeriesID:     seriesID,
		EpisodeID:    epID,
		ReleaseGuid:  releaseGUID,
		ReleaseTitle: releaseTitle,
		IndexerID:    idxID,
		Protocol:     protocol,
		Size:         size,
		AddedAt:      time.Now().UTC(),
		Notes:        notes,
	})
	if err != nil {
		if dbutil.IsUniqueViolation(err) {
			return ErrAlreadyBlocklisted
		}
		return fmt.Errorf("inserting blocklist entry: %w", err)
	}
	return nil
}

// IsBlocklisted reports whether a release GUID is on the blocklist.
func (s *Service) IsBlocklisted(ctx context.Context, releaseGUID string) (bool, error) {
	count, err := s.q.IsBlocklisted(ctx, releaseGUID)
	if err != nil {
		return false, fmt.Errorf("checking blocklist: %w", err)
	}
	return count > 0, nil
}

// IsBlocklistedByTitle reports whether a release title is on the blocklist.
func (s *Service) IsBlocklistedByTitle(ctx context.Context, releaseTitle string) (bool, error) {
	count, err := s.q.IsBlocklistedByTitle(ctx, releaseTitle)
	if err != nil {
		return false, fmt.Errorf("checking blocklist by title: %w", err)
	}
	return count > 0, nil
}

// List returns a paginated list of blocklist entries, newest first.
func (s *Service) List(ctx context.Context, page, perPage int) ([]Entry, int64, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 50
	}
	offset := int64((page - 1) * perPage)

	total, err := s.q.CountBlocklist(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("counting blocklist: %w", err)
	}

	rows, err := s.q.ListBlocklist(ctx, dbsqlite.ListBlocklistParams{
		Limit:  int64(perPage),
		Offset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing blocklist: %w", err)
	}

	entries := make([]Entry, len(rows))
	for i, r := range rows {
		epID := ""
		if r.EpisodeID != nil {
			epID = *r.EpisodeID
		}
		idxID := ""
		if r.IndexerID != nil {
			idxID = *r.IndexerID
		}
		entries[i] = Entry{
			ID:           r.ID,
			SeriesID:     r.SeriesID,
			SeriesTitle:  r.SeriesTitle,
			EpisodeID:    epID,
			ReleaseGUID:  r.ReleaseGuid,
			ReleaseTitle: r.ReleaseTitle,
			IndexerID:    idxID,
			Protocol:     r.Protocol,
			Size:         r.Size,
			AddedAt:      r.AddedAt,
			Notes:        r.Notes,
		}
	}
	return entries, total, nil
}

// Delete removes a single blocklist entry by ID.
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := s.q.DeleteBlocklistEntry(ctx, id); err != nil {
		return fmt.Errorf("deleting blocklist entry %q: %w", id, err)
	}
	return nil
}

// Clear removes all blocklist entries.
func (s *Service) Clear(ctx context.Context) error {
	if err := s.q.ClearBlocklist(ctx); err != nil {
		return fmt.Errorf("clearing blocklist: %w", err)
	}
	return nil
}
