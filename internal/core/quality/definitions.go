package quality

import (
	"context"
	"fmt"

	dbsqlite "github.com/beacon-stack/pilot/internal/db/generated/sqlite"
)

// Definition describes a known quality level and the acceptable file-size
// range for releases of that quality, expressed in MB per minute of runtime.
type Definition struct {
	ID            string // stable slug, e.g. "1080p-bluray-x265-none"
	Name          string // human-readable label, e.g. "1080p Bluray"
	Resolution    string
	Source        string
	Codec         string
	HDR           string
	MinSize       float64 // MB per minute (0 = no minimum)
	MaxSize       float64 // MB per minute (0 = no limit)
	PreferredSize float64 // MB per minute target within [min, max] (0 = same as max)
	SortOrder     int
}

// DefinitionSizeUpdate carries the new size values for a single definition.
type DefinitionSizeUpdate struct {
	ID            string
	MinSize       float64
	MaxSize       float64
	PreferredSize float64
}

// DefinitionService manages quality definitions.
type DefinitionService struct {
	q dbsqlite.Querier
}

// NewDefinitionService returns a new DefinitionService backed by the querier.
func NewDefinitionService(q dbsqlite.Querier) *DefinitionService {
	return &DefinitionService{q: q}
}

// List returns all quality definitions ordered by sort_order.
func (s *DefinitionService) List(ctx context.Context) ([]Definition, error) {
	rows, err := s.q.ListQualityDefinitions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list quality definitions: %w", err)
	}
	out := make([]Definition, len(rows))
	for i, r := range rows {
		out[i] = rowToDefinition(r)
	}
	return out, nil
}

// BulkUpdate applies size updates for all provided definitions.
// Unknown IDs are silently ignored (exec does nothing for missing rows).
func (s *DefinitionService) BulkUpdate(ctx context.Context, updates []DefinitionSizeUpdate) error {
	for _, u := range updates {
		if err := s.q.UpdateQualityDefinitionSizes(ctx, dbsqlite.UpdateQualityDefinitionSizesParams{
			MinSize:       u.MinSize,
			MaxSize:       u.MaxSize,
			PreferredSize: u.PreferredSize,
			ID:            u.ID,
		}); err != nil {
			return fmt.Errorf("update quality definition %q: %w", u.ID, err)
		}
	}
	return nil
}

func rowToDefinition(r dbsqlite.QualityDefinition) Definition {
	return Definition{
		ID:            r.ID,
		Name:          r.Name,
		Resolution:    r.Resolution,
		Source:        r.Source,
		Codec:         r.Codec,
		HDR:           r.Hdr,
		MinSize:       r.MinSize,
		MaxSize:       r.MaxSize,
		PreferredSize: r.PreferredSize,
		SortOrder:     int(r.SortOrder),
	}
}
