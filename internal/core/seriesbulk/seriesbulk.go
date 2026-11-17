// Package seriesbulk edits or deletes many series at once (Sonarr's mass
// editor). Every change is validated as a whole first and then applied
// in one transaction, so a request either changes every series or none.
package seriesbulk

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/events"
)

// MaxSeries caps one request.
const MaxSeries = 1000

// SeriesTypes are the valid series types.
var SeriesTypes = map[string]bool{"standard": true, "anime": true, "daily": true}

// ValidationError explains why a request was rejected. Nothing changed.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// Edit changes the given fields of every series in IDs. A nil field is
// left as it is.
type Edit struct {
	IDs              []string
	Monitored        *bool
	QualityProfileID *string
	LibraryID        *string
	SeriesType       *string
}

// Recycler removes a file, into the recycle bin when one is set.
type Recycler interface {
	Delete(ctx context.Context, path, libraryRoot string) (string, error)
}

// Publisher announces deleted series.
type Publisher interface {
	Publish(ctx context.Context, e events.Event)
}

// Service applies bulk changes.
type Service struct {
	sqlDB   *sql.DB
	recycle Recycler
	bus     Publisher
	logger  *slog.Logger
	now     func() time.Time
}

// New creates a Service. recycle and bus may be nil.
func New(sqlDB *sql.DB, recycle Recycler, bus Publisher, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Service{sqlDB: sqlDB, recycle: recycle, bus: bus, logger: logger, now: time.Now}
}

func checkIDs(ids []string) error {
	if len(ids) == 0 {
		return &ValidationError{"no series selected"}
	}
	if len(ids) > MaxSeries {
		return &ValidationError{fmt.Sprintf("at most %d series at a time", MaxSeries)}
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			return &ValidationError{"series ids must be non-empty and listed once"}
		}
		seen[id] = true
	}
	return nil
}

// loadAll returns every series, or a ValidationError naming the first
// missing one.
func loadAll(ctx context.Context, q db.Querier, ids []string) ([]db.Series, error) {
	out := make([]db.Series, 0, len(ids))
	for _, id := range ids {
		s, err := q.GetSeries(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &ValidationError{fmt.Sprintf("series %q not found", id)}
		}
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// Edit applies e to every series. It returns how many series changed.
func (s *Service) Edit(ctx context.Context, e Edit) (int, error) {
	if err := checkIDs(e.IDs); err != nil {
		return 0, err
	}
	if e.Monitored == nil && e.QualityProfileID == nil && e.LibraryID == nil && e.SeriesType == nil {
		return 0, &ValidationError{"nothing to change"}
	}

	tx, err := s.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	q := db.New(tx)

	if e.SeriesType != nil && !SeriesTypes[*e.SeriesType] {
		return 0, &ValidationError{"series type must be standard, anime or daily"}
	}
	if e.QualityProfileID != nil {
		if _, err := q.GetQualityProfile(ctx, *e.QualityProfileID); errors.Is(err, sql.ErrNoRows) {
			return 0, &ValidationError{"quality profile not found"}
		} else if err != nil {
			return 0, err
		}
	}
	if e.LibraryID != nil {
		if _, err := q.GetLibrary(ctx, *e.LibraryID); errors.Is(err, sql.ErrNoRows) {
			return 0, &ValidationError{"library not found"}
		} else if err != nil {
			return 0, err
		}
	}
	all, err := loadAll(ctx, q, e.IDs)
	if err != nil {
		return 0, err
	}

	now := s.now().UTC().Format(time.RFC3339)
	for _, cur := range all {
		p := db.UpdateSeriesParams{
			ID:               cur.ID,
			Title:            cur.Title,
			Monitored:        cur.Monitored,
			LibraryID:        cur.LibraryID,
			QualityProfileID: cur.QualityProfileID,
			SeriesType:       cur.SeriesType,
			Path:             cur.Path,
			UpdatedAt:        now,
		}
		if e.Monitored != nil {
			p.Monitored = *e.Monitored
		}
		if e.QualityProfileID != nil {
			p.QualityProfileID = *e.QualityProfileID
		}
		if e.LibraryID != nil {
			p.LibraryID = *e.LibraryID
		}
		if e.SeriesType != nil {
			p.SeriesType = *e.SeriesType
		}
		if _, err := q.UpdateSeries(ctx, p); err != nil {
			return 0, fmt.Errorf("updating series %q: %w", cur.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(all), nil
}

// Delete removes every series in ids. With deleteFiles, their recorded
// episode files are removed from disk too, through the recycle bin when
// one is set. The database change is one transaction; files are removed
// after it commits, and a file that can't be removed is logged, not
// returned (the series is already gone).
func (s *Service) Delete(ctx context.Context, ids []string, deleteFiles bool) (int, error) {
	if err := checkIDs(ids); err != nil {
		return 0, err
	}
	if deleteFiles && s.recycle == nil {
		return 0, errors.New("deleting files is not available")
	}
	tx, err := s.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	q := db.New(tx)

	all, err := loadAll(ctx, q, ids)
	if err != nil {
		return 0, err
	}
	type file struct{ path, root string }
	var files []file
	if deleteFiles {
		roots := map[string]string{}
		for _, sr := range all {
			root, ok := roots[sr.LibraryID]
			if !ok {
				if lib, err := q.GetLibrary(ctx, sr.LibraryID); err == nil {
					root = lib.RootPath
				}
				roots[sr.LibraryID] = root
			}
			efs, err := q.ListEpisodeFilesBySeriesID(ctx, sr.ID)
			if err != nil {
				return 0, fmt.Errorf("listing files of series %q: %w", sr.ID, err)
			}
			for _, ef := range efs {
				files = append(files, file{ef.Path, root})
			}
		}
	}
	for _, sr := range all {
		if err := q.DeleteSeries(ctx, sr.ID); err != nil {
			return 0, fmt.Errorf("deleting series %q: %w", sr.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}

	for _, f := range files {
		if _, err := s.recycle.Delete(ctx, f.path, f.root); err != nil {
			s.logger.Warn("bulk delete: could not remove file", "path", f.path, "error", err)
		}
	}
	if s.bus != nil {
		for _, sr := range all {
			s.bus.Publish(ctx, events.Event{Type: events.TypeShowDeleted, ShowID: sr.ID, Data: map[string]any{"title": sr.Title}})
		}
	}
	return len(all), nil
}
