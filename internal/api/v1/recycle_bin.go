package v1

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pilot/internal/core/recyclebin"
)

// RegisterRecycleBinRoutes registers the recycle bin endpoints. The bin's
// folder and cleanup period are media management settings.
func RegisterRecycleBinRoutes(api huma.API, svc *recyclebin.Service) {
	huma.Register(api, huma.Operation{
		OperationID: "empty-recycle-bin",
		Method:      http.MethodPost,
		Path:        "/api/v1/recycle-bin/empty",
		Summary:     "Delete everything in the recycle bin",
		Tags:        []string{"Media Management"},
	}, func(ctx context.Context, _ *struct{}) (*struct {
		Body struct {
			Deleted int `json:"deleted" doc:"Files deleted"`
		}
	}, error) {
		n, err := svc.Empty(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to empty the recycle bin", err)
		}
		out := &struct {
			Body struct {
				Deleted int `json:"deleted" doc:"Files deleted"`
			}
		}{}
		out.Body.Deleted = n
		return out, nil
	})
}
