package v1

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pilot/internal/core/health"
)

type healthReportBody struct {
	Issues []health.Issue `json:"issues"`
	// ConnectivityCheckedAt is when download clients and indexers were
	// last tested (RFC3339). Library checks are always current.
	ConnectivityCheckedAt string `json:"connectivity_checked_at"`
}

func healthResponse(r health.Report) *struct{ Body healthReportBody } {
	issues := r.Issues
	if issues == nil {
		issues = []health.Issue{}
	}
	at := ""
	if !r.ConnectivityCheckedAt.IsZero() {
		at = r.ConnectivityCheckedAt.UTC().Format(time.RFC3339)
	}
	return &struct{ Body healthReportBody }{Body: healthReportBody{Issues: issues, ConnectivityCheckedAt: at}}
}

// RegisterHealthRoutes registers the health check endpoints.
func RegisterHealthRoutes(api huma.API, svc *health.Service) {
	huma.Register(api, huma.Operation{
		OperationID: "get-system-health",
		Method:      http.MethodGet,
		Path:        "/api/v1/system/health",
		Summary:     "Current health issues",
		Description: "Library folders and free space are checked on each call. Download clients and indexers are tested hourly; the last result is returned.",
		Tags:        []string{"System"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body healthReportBody }, error) {
		return healthResponse(svc.Report(ctx)), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "check-system-health",
		Method:      http.MethodPost,
		Path:        "/api/v1/system/health/check",
		Summary:     "Test download clients and indexers now",
		Tags:        []string{"System"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body healthReportBody }, error) {
		svc.CheckConnectivity(ctx)
		return healthResponse(svc.Report(ctx)), nil
	})
}
