package quality

import (
	"context"
	"fmt"

	db "github.com/beacon-stack/pilot/internal/db/generated"
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
	q db.Querier
}

// NewDefinitionService returns a new DefinitionService backed by the querier.
func NewDefinitionService(q db.Querier) *DefinitionService {
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
		if err := s.q.UpdateQualityDefinitionSizes(ctx, db.UpdateQualityDefinitionSizesParams{
			MinSize:       float32(u.MinSize),
			MaxSize:       float32(u.MaxSize),
			PreferredSize: float32(u.PreferredSize),
			ID:            u.ID,
		}); err != nil {
			return fmt.Errorf("update quality definition %q: %w", u.ID, err)
		}
	}
	return nil
}

func rowToDefinition(r db.QualityDefinition) Definition {
	return Definition{
		ID:            r.ID,
		Name:          r.Name,
		Resolution:    r.Resolution,
		Source:        r.Source,
		Codec:         r.Codec,
		HDR:           r.Hdr,
		MinSize:       float64(r.MinSize),
		MaxSize:       float64(r.MaxSize),
		PreferredSize: float64(r.PreferredSize),
		SortOrder:     int(r.SortOrder),
	}
}
