package v1

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pilot/internal/core/seriesbulk"
)

type seriesBulkEditBody struct {
	IDs              []string `json:"ids"                          minItems:"1" doc:"Series to change"`
	Monitored        *bool    `json:"monitored,omitempty"          doc:"Set monitoring; omit to leave as is"`
	QualityProfileID *string  `json:"quality_profile_id,omitempty" doc:"Set the quality profile; omit to leave as is"`
	LibraryID        *string  `json:"library_id,omitempty"         doc:"Re-point to another library (files are not moved); omit to leave as is"`
	SeriesType       *string  `json:"series_type,omitempty"        enum:"standard,anime,daily" doc:"Set the series type; omit to leave as is"`
}

type seriesBulkDeleteBody struct {
	IDs         []string `json:"ids"                    minItems:"1" doc:"Series to delete"`
	DeleteFiles bool     `json:"delete_files,omitempty" doc:"Also remove their episode files from disk (through the recycle bin when one is set)"`
}

type seriesBulkResult struct {
	Body struct {
		Count int `json:"count" doc:"Series changed or deleted"`
	}
}

func seriesBulkError(err error) error {
	var verr *seriesbulk.ValidationError
	if errors.As(err, &verr) {
		return huma.Error422UnprocessableEntity(verr.Msg)
	}
	return huma.NewError(http.StatusInternalServerError, "bulk change failed", err)
}

// RegisterSeriesBulkRoutes registers the series mass editor endpoints.
// Each request changes every listed series or none.
func RegisterSeriesBulkRoutes(api huma.API, svc *seriesbulk.Service) {
	huma.Register(api, huma.Operation{
		OperationID: "bulk-edit-series",
		Method:      http.MethodPut,
		Path:        "/api/v1/series/bulk",
		Summary:     "Change several series at once",
		Description: "Only the fields present change. The whole request is validated first; if anything is invalid nothing changes.",
		Tags:        []string{"Series"},
	}, func(ctx context.Context, input *struct{ Body seriesBulkEditBody }) (*seriesBulkResult, error) {
		n, err := svc.Edit(ctx, seriesbulk.Edit{
			IDs:              input.Body.IDs,
			Monitored:        input.Body.Monitored,
			QualityProfileID: input.Body.QualityProfileID,
			LibraryID:        input.Body.LibraryID,
			SeriesType:       input.Body.SeriesType,
		})
		if err != nil {
			return nil, seriesBulkError(err)
		}
		out := &seriesBulkResult{}
		out.Body.Count = n
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "bulk-delete-series",
		Method:      http.MethodPost,
		Path:        "/api/v1/series/bulk/delete",
		Summary:     "Delete several series at once",
		Tags:        []string{"Series"},
	}, func(ctx context.Context, input *struct{ Body seriesBulkDeleteBody }) (*seriesBulkResult, error) {
		n, err := svc.Delete(ctx, input.Body.IDs, input.Body.DeleteFiles)
		if err != nil {
			return nil, seriesBulkError(err)
		}
		out := &seriesBulkResult{}
		out.Body.Count = n
		return out, nil
	})
}
