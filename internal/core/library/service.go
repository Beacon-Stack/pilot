// Package library manages Pilot library records and their series counts.
package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/events"
)

// ErrNotFound is returned when a library does not exist.
var ErrNotFound = errors.New("library not found")

// CreateRequest carries the fields needed to create a library.
type CreateRequest struct {
	Name                    string
	RootPath                string
	DefaultQualityProfileID string
	NamingFormat            *string
	FolderFormat            *string
	MinFreeSpaceGB          int
	Tags                    []string
}

// UpdateRequest carries the fields needed to update a library.
// It is identical in shape to CreateRequest.
type UpdateRequest = CreateRequest

// Library is the domain representation of a library record.
type Library struct {
	ID                      string
	Name                    string
	RootPath                string
	DefaultQualityProfileID string
	NamingFormat            *string
	FolderFormat            *string
	MinFreeSpaceGB          int
	Tags                    []string
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

// Stats holds runtime metrics about a library.
type Stats struct {
	SeriesCount int64
}

// Service manages library records.
type Service struct {
	q   db.Querier
	bus *events.Bus
}

// NewService creates a new Service backed by the given querier and event bus.
func NewService(q db.Querier, bus *events.Bus) *Service {
	return &Service{q: q, bus: bus}
}

// Create inserts a new library and returns the persisted domain type.
func (s *Service) Create(ctx context.Context, req CreateRequest) (Library, error) {
	tagsJSON, err := marshalTags(req.Tags)
	if err != nil {
		return Library{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	row, err := s.q.CreateLibrary(ctx, db.CreateLibraryParams{
		ID:                      uuid.New().String(),
		Name:                    req.Name,
		RootPath:                req.RootPath,
		DefaultQualityProfileID: req.DefaultQualityProfileID,
		NamingFormat:            ptrToNullString(req.NamingFormat),
		FolderFormat:            ptrToNullString(req.FolderFormat),
		MinFreeSpaceGb:          int32(req.MinFreeSpaceGB),
		TagsJson:                tagsJSON,
		CreatedAt:               now,
		UpdatedAt:               now,
	})
	if err != nil {
		return Library{}, fmt.Errorf("inserting library: %w", err)
	}

	return rowToLibrary(row)
}

// Get returns a library by ID.
// Returns ErrNotFound if no library with that ID exists.
func (s *Service) Get(ctx context.Context, id string) (Library, error) {
	row, err := s.q.GetLibrary(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Library{}, ErrNotFound
		}
		return Library{}, fmt.Errorf("fetching library %q: %w", id, err)
	}
	return rowToLibrary(row)
}

// List returns all libraries ordered by name.
func (s *Service) List(ctx context.Context) ([]Library, error) {
	rows, err := s.q.ListLibraries(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing libraries: %w", err)
	}

	libs := make([]Library, 0, len(rows))
	for _, row := range rows {
		lib, err := rowToLibrary(row)
		if err != nil {
			return nil, err
		}
		libs = append(libs, lib)
	}
	return libs, nil
}

// Update replaces the mutable fields of an existing library.
// Returns ErrNotFound if the library does not exist.
func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (Library, error) {
	// Confirm existence before update so we can surface ErrNotFound clearly.
	if _, err := s.q.GetLibrary(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Library{}, ErrNotFound
		}
		return Library{}, fmt.Errorf("fetching library %q for update: %w", id, err)
	}

	tagsJSON, err := marshalTags(req.Tags)
	if err != nil {
		return Library{}, err
	}

	row, err := s.q.UpdateLibrary(ctx, db.UpdateLibraryParams{
		ID:                      id,
		Name:                    req.Name,
		RootPath:                req.RootPath,
		DefaultQualityProfileID: req.DefaultQualityProfileID,
		NamingFormat:            ptrToNullString(req.NamingFormat),
		FolderFormat:            ptrToNullString(req.FolderFormat),
		MinFreeSpaceGb:          int32(req.MinFreeSpaceGB),
		TagsJson:                tagsJSON,
		UpdatedAt:               time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return Library{}, fmt.Errorf("updating library %q: %w", id, err)
	}

	return rowToLibrary(row)
}

// Delete removes a library by ID.
// Returns ErrNotFound if the library does not exist.
func (s *Service) Delete(ctx context.Context, id string) error {
	if _, err := s.q.GetLibrary(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("fetching library %q for delete: %w", id, err)
	}

	if err := s.q.DeleteLibrary(ctx, id); err != nil {
		return fmt.Errorf("deleting library %q: %w", id, err)
	}
	return nil
}

// Stats returns runtime metrics for the library.
func (s *Service) Stats(ctx context.Context, id string) (Stats, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return Stats{}, err
	}

	seriesCount, err := s.q.CountSeriesInLibrary(ctx, id)
	if err != nil {
		return Stats{}, fmt.Errorf("counting series in library %q: %w", id, err)
	}

	return Stats{SeriesCount: seriesCount}, nil
}

// rowToLibrary converts a DB row into the domain Library type.
func rowToLibrary(row db.Library) (Library, error) {
	var tags []string
	if err := json.Unmarshal([]byte(row.TagsJson), &tags); err != nil {
		return Library{}, fmt.Errorf("unmarshaling tags for library %q: %w", row.ID, err)
	}

	createdAt, err := time.Parse(time.RFC3339, row.CreatedAt)
	if err != nil {
		return Library{}, fmt.Errorf("parsing created_at for library %q: %w", row.ID, err)
	}

	updatedAt, err := time.Parse(time.RFC3339, row.UpdatedAt)
	if err != nil {
		return Library{}, fmt.Errorf("parsing updated_at for library %q: %w", row.ID, err)
	}

	return Library{
		ID:                      row.ID,
		Name:                    row.Name,
		RootPath:                row.RootPath,
		DefaultQualityProfileID: row.DefaultQualityProfileID,
		NamingFormat:            nullStringToPtr(row.NamingFormat),
		FolderFormat:            nullStringToPtr(row.FolderFormat),
		MinFreeSpaceGB:          int(row.MinFreeSpaceGb),
		Tags:                    tags,
		CreatedAt:               createdAt,
		UpdatedAt:               updatedAt,
	}, nil
}

func ptrToNullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

func nullStringToPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	return &ns.String
}

// marshalTags serializes a tags slice to JSON. A nil or empty slice becomes "[]".
func marshalTags(tags []string) (string, error) {
	if tags == nil {
		tags = []string{}
	}
	b, err := json.Marshal(tags)
	if err != nil {
		return "", fmt.Errorf("marshaling tags: %w", err)
	}
	return string(b), nil
}
