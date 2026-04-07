package v1

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pilot/internal/core/stats"
)

// ── Output wrappers ───────────────────────────────────────────────────────────

type statsCollectionOutput struct {
	Body stats.CollectionStats
}

type statsStorageOutput struct {
	Body stats.StorageStat
}

type statsQualityTiersOutput struct {
	Body []stats.QualityTier
}

type statsGrowthOutput struct {
	Body []stats.GrowthPoint
}

// RegisterStatsRoutes registers the /api/v1/stats/* endpoints.
func RegisterStatsRoutes(humaAPI huma.API, svc *stats.Service) {
	// GET /api/v1/stats/collection
	huma.Register(humaAPI, huma.Operation{
		OperationID: "get-stats-collection",
		Method:      http.MethodGet,
		Path:        "/api/v1/stats/collection",
		Summary:     "Collection overview statistics",
		Tags:        []string{"Statistics"},
	}, func(ctx context.Context, _ *struct{}) (*statsCollectionOutput, error) {
		c, err := svc.Collection(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to get collection stats", err)
		}
		return &statsCollectionOutput{Body: c}, nil
	})

	// GET /api/v1/stats/storage
	huma.Register(humaAPI, huma.Operation{
		OperationID: "get-stats-storage",
		Method:      http.MethodGet,
		Path:        "/api/v1/stats/storage",
		Summary:     "Storage usage by episode files",
		Tags:        []string{"Statistics"},
	}, func(ctx context.Context, _ *struct{}) (*statsStorageOutput, error) {
		s, err := svc.Storage(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to get storage stats", err)
		}
		return &statsStorageOutput{Body: s}, nil
	})

	// GET /api/v1/stats/quality/tiers
	huma.Register(humaAPI, huma.Operation{
		OperationID: "get-stats-quality-tiers",
		Method:      http.MethodGet,
		Path:        "/api/v1/stats/quality/tiers",
		Summary:     "Quality distribution grouped by resolution+source",
		Tags:        []string{"Statistics"},
	}, func(ctx context.Context, _ *struct{}) (*statsQualityTiersOutput, error) {
		tiers, err := svc.QualityTiers(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to get quality tiers", err)
		}
		if tiers == nil {
			tiers = []stats.QualityTier{}
		}
		return &statsQualityTiersOutput{Body: tiers}, nil
	})

	// GET /api/v1/stats/growth
	huma.Register(humaAPI, huma.Operation{
		OperationID: "get-stats-growth",
		Method:      http.MethodGet,
		Path:        "/api/v1/stats/growth",
		Summary:     "Historical stats snapshots for trend charts",
		Tags:        []string{"Statistics"},
	}, func(ctx context.Context, _ *struct{}) (*statsGrowthOutput, error) {
		points, err := svc.Growth(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to get growth data", err)
		}
		if points == nil {
			points = []stats.GrowthPoint{}
		}
		return &statsGrowthOutput{Body: points}, nil
	})
}
