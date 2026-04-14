// Package blocklist manages the release blocklist used to skip known-bad releases.
//
// ⚠ Before changing anything in this file, run:
//
//	go test ./internal/core/blocklist/...
//
// The tests (service_test.go) pin the stall-watcher's write path:
// AddFromStall, two-keyed dedup (guid OR info_hash), idempotency on
// duplicate insert, RemoveByGUID for the grab override flow, and the
// CountRecentStalls query used by the circuit breaker. Breaking any of
// these silently breaks automated dead-torrent blocklisting.
//
// See pilot/CLAUDE.md "Regression guard: dead-torrent release search"
// for the full context.
package blocklist

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/beacon-stack/pilot/internal/core/dbutil"
	db "github.com/beacon-stack/pilot/internal/db/generated"
)

// ErrAlreadyBlocklisted is returned when adding a GUID that is already on the blocklist.
var ErrAlreadyBlocklisted = errors.New("release already blocklisted")

// Reason classifies why an entry is on the blocklist. The stall watcher
// uses stall_* reasons, users marking via UI use UserMarked. New reasons
// that start with "stall_" count toward the circuit breaker.
const (
	ReasonUserMarked        = "user_marked"
	ReasonStallNoPeersEver  = "stall_no_peers_ever"
	ReasonStallActivityLost = "stall_activity_lost"
)

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
	Reason       string
	InfoHash     string
}

// Service manages the release blocklist.
type Service struct {
	q db.Querier
}

// NewService creates a new Service.
func NewService(q db.Querier) *Service {
	return &Service{q: q}
}

// Add inserts a new user-marked blocklist entry. Returns ErrAlreadyBlocklisted
// if the GUID is already present (the unique index on release_guid enforces
// this). For automated stall-detected entries, use AddFromStall instead so
// the reason field is set correctly.
func (s *Service) Add(ctx context.Context, seriesID, episodeID, releaseGUID, releaseTitle, indexerID, protocol string, size int64, notes string) error {
	return s.insert(ctx, insertParams{
		SeriesID:     seriesID,
		EpisodeID:    episodeID,
		ReleaseGUID:  releaseGUID,
		ReleaseTitle: releaseTitle,
		IndexerID:    indexerID,
		Protocol:     protocol,
		Size:         size,
		Notes:        notes,
		Reason:       ReasonUserMarked,
	})
}

// AddFromStall inserts a blocklist entry triggered by Haul's stall
// detection. reason must be one of the stall_* constants; info_hash may
// be empty if unavailable. Idempotent — double-fires are swallowed via
// ErrAlreadyBlocklisted (callers usually treat that as success).
func (s *Service) AddFromStall(ctx context.Context, p StallEntry) error {
	return s.insert(ctx, insertParams(p))
}

// StallEntry is the shape of a stall-triggered blocklist insert.
type StallEntry struct {
	SeriesID     string
	EpisodeID    string
	ReleaseGUID  string
	ReleaseTitle string
	IndexerID    string
	Protocol     string
	Size         int64
	Notes        string
	Reason       string // must be a stall_* constant
	InfoHash     string
}

type insertParams struct {
	SeriesID     string
	EpisodeID    string
	ReleaseGUID  string
	ReleaseTitle string
	IndexerID    string
	Protocol     string
	Size         int64
	Notes        string
	Reason       string
	InfoHash     string
}

func (s *Service) insert(ctx context.Context, p insertParams) error {
	epID := sql.NullString{}
	if p.EpisodeID != "" {
		epID = sql.NullString{String: p.EpisodeID, Valid: true}
	}
	idxID := sql.NullString{}
	if p.IndexerID != "" {
		idxID = sql.NullString{String: p.IndexerID, Valid: true}
	}
	ih := sql.NullString{}
	if p.InfoHash != "" {
		ih = sql.NullString{String: p.InfoHash, Valid: true}
	}
	_, err := s.q.CreateBlocklistEntry(ctx, db.CreateBlocklistEntryParams{
		ID:           uuid.New().String(),
		SeriesID:     p.SeriesID,
		EpisodeID:    epID,
		ReleaseGuid:  p.ReleaseGUID,
		ReleaseTitle: p.ReleaseTitle,
		IndexerID:    idxID,
		Protocol:     p.Protocol,
		Size:         int32(p.Size),
		AddedAt:      time.Now().UTC(),
		Notes:        p.Notes,
		Reason:       p.Reason,
		InfoHash:     ih,
	})
	if err != nil {
		if dbutil.IsUniqueViolation(err) {
			return ErrAlreadyBlocklisted
		}
		return fmt.Errorf("inserting blocklist entry: %w", err)
	}
	return nil
}

// IsBlocklistedGUIDOrInfoHash reports whether a release is on the blocklist
// under either its original GUID or its info hash. This is the two-keyed
// dedup the search filter uses so a release re-surfaced from a different
// indexer (different GUID, same content) still gets filtered.
func (s *Service) IsBlocklistedGUIDOrInfoHash(ctx context.Context, releaseGUID, infoHash string) (bool, error) {
	ih := sql.NullString{}
	if infoHash != "" {
		ih = sql.NullString{String: infoHash, Valid: true}
	}
	count, err := s.q.IsBlocklistedByGuidOrInfoHash(ctx, db.IsBlocklistedByGuidOrInfoHashParams{
		ReleaseGuid: releaseGUID,
		InfoHash:    ih,
	})
	if err != nil {
		return false, fmt.Errorf("checking blocklist by guid/info_hash: %w", err)
	}
	return count > 0, nil
}

// RemoveByGUID removes a blocklist entry by its release GUID. Used by the
// grab-override flow so a user can force-grab a previously-blocklisted
// release and have it disappear from the blocklist in one click.
func (s *Service) RemoveByGUID(ctx context.Context, releaseGUID string) error {
	if err := s.q.DeleteBlocklistEntryByGUID(ctx, releaseGUID); err != nil {
		return fmt.Errorf("deleting blocklist entry by guid: %w", err)
	}
	return nil
}

// CountRecentStalls counts stall-reason blocklist entries for a given
// (series, episode) within the last 24 hours. Used by the auto-re-search
// circuit breaker to stop after a configurable number of stall retries.
func (s *Service) CountRecentStalls(ctx context.Context, seriesID, episodeID string) (int64, error) {
	epID := sql.NullString{}
	if episodeID != "" {
		epID = sql.NullString{String: episodeID, Valid: true}
	}
	return s.q.CountRecentStallsForEpisode(ctx, db.CountRecentStallsForEpisodeParams{
		SeriesID:  seriesID,
		EpisodeID: epID,
	})
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
	offset := int32((page - 1) * perPage)

	total, err := s.q.CountBlocklist(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("counting blocklist: %w", err)
	}

	rows, err := s.q.ListBlocklist(ctx, db.ListBlocklistParams{
		Limit:  int32(perPage),
		Offset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing blocklist: %w", err)
	}

	entries := make([]Entry, len(rows))
	for i, r := range rows {
		entries[i] = Entry{
			ID:           r.ID,
			SeriesID:     r.SeriesID,
			SeriesTitle:  r.SeriesTitle,
			EpisodeID:    r.EpisodeID.String,
			ReleaseGUID:  r.ReleaseGuid,
			ReleaseTitle: r.ReleaseTitle,
			IndexerID:    r.IndexerID.String,
			Protocol:     r.Protocol,
			Size:         int64(r.Size),
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
