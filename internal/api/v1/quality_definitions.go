package v1

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/screenarr/screenarr/internal/core/quality"
)

// ── Response / request shapes ─────────────────────────────────────────────────

type qualityDefinitionBody struct {
	ID            string  `json:"id"             doc:"Stable slug identifier"`
	Name          string  `json:"name"           doc:"Human-readable quality name"`
	Resolution    string  `json:"resolution"     doc:"Video resolution"`
	Source        string  `json:"source"         doc:"Release source"`
	Codec         string  `json:"codec"          doc:"Video codec"`
	HDR           string  `json:"hdr"            doc:"HDR format"`
	MinSize       float64 `json:"min_size"       doc:"Minimum file size in MB per minute (0 = no minimum)"`
	MaxSize       float64 `json:"max_size"       doc:"Maximum file size in MB per minute (0 = no limit)"`
	PreferredSize float64 `json:"preferred_size" doc:"Preferred file size in MB per minute within [min, max] (0 = same as max)"`
	SortOrder     int     `json:"sort_order"     doc:"Display sort order"`
}

type qualityDefinitionListOutput struct {
	Body []*qualityDefinitionBody
}

type qualityDefinitionUpdateItem struct {
	ID            string  `json:"id"             doc:"Definition ID to update"`
	MinSize       float64 `json:"min_size"       doc:"Minimum file size in MB per minute"`
	MaxSize       float64 `json:"max_size"       doc:"Maximum file size in MB per minute"`
	PreferredSize float64 `json:"preferred_size" doc:"Preferred file size in MB per minute"`
}

type qualityDefinitionBulkUpdateInput struct {
	Body []qualityDefinitionUpdateItem
}

type qualityDefinitionBulkUpdateOutput struct{}

// ── Helpers ───────────────────────────────────────────────────────────────────

func definitionToBody(d quality.Definition) *qualityDefinitionBody {
	return &qualityDefinitionBody{
		ID:            d.ID,
		Name:          d.Name,
		Resolution:    d.Resolution,
		Source:        d.Source,
		Codec:         d.Codec,
		HDR:           d.HDR,
		MinSize:       d.MinSize,
		MaxSize:       d.MaxSize,
		PreferredSize: d.PreferredSize,
		SortOrder:     d.SortOrder,
	}
}

// ── Route registration ────────────────────────────────────────────────────────

// RegisterQualityDefinitionRoutes registers /api/v1/quality-definitions endpoints.
func RegisterQualityDefinitionRoutes(api huma.API, svc *quality.DefinitionService) {
	// GET /api/v1/quality-definitions
	huma.Register(api, huma.Operation{
		OperationID: "list-quality-definitions",
		Method:      http.MethodGet,
		Path:        "/api/v1/quality-definitions",
		Summary:     "List all quality definitions with their size constraints",
		Tags:        []string{"Quality Definitions"},
	}, func(ctx context.Context, _ *struct{}) (*qualityDefinitionListOutput, error) {
		defs, err := svc.List(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list quality definitions", err)
		}
		bodies := make([]*qualityDefinitionBody, len(defs))
		for i, d := range defs {
			bodies[i] = definitionToBody(d)
		}
		return &qualityDefinitionListOutput{Body: bodies}, nil
	})

	// PUT /api/v1/quality-definitions
	huma.Register(api, huma.Operation{
		OperationID: "update-quality-definitions",
		Method:      http.MethodPut,
		Path:        "/api/v1/quality-definitions",
		Summary:     "Update size constraints for quality definitions",
		Tags:        []string{"Quality Definitions"},
	}, func(ctx context.Context, input *qualityDefinitionBulkUpdateInput) (*qualityDefinitionBulkUpdateOutput, error) {
		updates := make([]quality.DefinitionSizeUpdate, len(input.Body))
		for i, item := range input.Body {
			updates[i] = quality.DefinitionSizeUpdate{
				ID:            item.ID,
				MinSize:       item.MinSize,
				MaxSize:       item.MaxSize,
				PreferredSize: item.PreferredSize,
			}
		}
		if err := svc.BulkUpdate(ctx, updates); err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to update quality definitions", err)
		}
		return &qualityDefinitionBulkUpdateOutput{}, nil
	})
}
