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

type statsQualityOutput struct {
	Body []stats.QualityBucket
}

type statsQualitySeriesInput struct {
	Resolution string `query:"resolution" doc:"Filter by resolution (e.g. 1080p, 2160p)"`
	Source     string `query:"source" doc:"Filter by source (e.g. Bluray, WEBDL, Remux)"`
}

type statsQualitySeriesOutput struct {
	Body []string
}

type statsGrowthOutput struct {
	Body []stats.GrowthPoint
}

type statsDecadesOutput struct {
	Body []stats.DecadeBucket
}

type statsGenresOutput struct {
	Body []stats.GenreBucket
}

type statsGrabsBody struct {
	TotalGrabs  int64               `json:"total_grabs"`
	Successful  int64               `json:"successful"`
	Failed      int64               `json:"failed"`
	SuccessRate float64             `json:"success_rate"`
	TopIndexers []stats.IndexerStat `json:"top_indexers"`
}

type statsGrabsOutput struct {
	Body statsGrabsBody
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

	// GET /api/v1/stats/quality-tiers
	huma.Register(humaAPI, huma.Operation{
		OperationID: "get-stats-quality-tiers",
		Method:      http.MethodGet,
		Path:        "/api/v1/stats/quality-tiers",
		Summary:     "Quality distribution grouped by resolution+source with deduplicated series counts",
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

	// GET /api/v1/stats/quality
	huma.Register(humaAPI, huma.Operation{
		OperationID: "get-stats-quality",
		Method:      http.MethodGet,
		Path:        "/api/v1/stats/quality",
		Summary:     "Quality buckets grouped by resolution+source+codec+hdr",
		Tags:        []string{"Statistics"},
	}, func(ctx context.Context, _ *struct{}) (*statsQualityOutput, error) {
		buckets, err := svc.Quality(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to get quality buckets", err)
		}
		if buckets == nil {
			buckets = []stats.QualityBucket{}
		}
		return &statsQualityOutput{Body: buckets}, nil
	})

	// GET /api/v1/stats/quality-series — list series IDs matching a quality tier
	huma.Register(humaAPI, huma.Operation{
		OperationID: "get-stats-quality-series",
		Method:      http.MethodGet,
		Path:        "/api/v1/stats/quality-series",
		Summary:     "List series IDs matching a quality tier (resolution and/or source filter)",
		Tags:        []string{"Statistics"},
	}, func(ctx context.Context, input *statsQualitySeriesInput) (*statsQualitySeriesOutput, error) {
		ids, err := svc.SeriesIDsByQualityTier(ctx, input.Resolution, input.Source)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to get series for quality tier", err)
		}
		if ids == nil {
			ids = []string{}
		}
		return &statsQualitySeriesOutput{Body: ids}, nil
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

	// GET /api/v1/stats/decades
	huma.Register(humaAPI, huma.Operation{
		OperationID: "get-stats-decades",
		Method:      http.MethodGet,
		Path:        "/api/v1/stats/decades",
		Summary:     "Series counts grouped by decade",
		Tags:        []string{"Statistics"},
	}, func(ctx context.Context, _ *struct{}) (*statsDecadesOutput, error) {
		buckets, err := svc.DecadeDistribution(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to get decade distribution", err)
		}
		if buckets == nil {
			buckets = []stats.DecadeBucket{}
		}
		return &statsDecadesOutput{Body: buckets}, nil
	})

	// GET /api/v1/stats/genres
	huma.Register(humaAPI, huma.Operation{
		OperationID: "get-stats-genres",
		Method:      http.MethodGet,
		Path:        "/api/v1/stats/genres",
		Summary:     "Top 15 genres by series count",
		Tags:        []string{"Statistics"},
	}, func(ctx context.Context, _ *struct{}) (*statsGenresOutput, error) {
		buckets, err := svc.GenreDistribution(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to get genre distribution", err)
		}
		if buckets == nil {
			buckets = []stats.GenreBucket{}
		}
		return &statsGenresOutput{Body: buckets}, nil
	})

	// GET /api/v1/stats/grabs
	huma.Register(humaAPI, huma.Operation{
		OperationID: "get-stats-grabs",
		Method:      http.MethodGet,
		Path:        "/api/v1/stats/grabs",
		Summary:     "Overall grab counts plus top indexers by volume",
		Tags:        []string{"Statistics"},
	}, func(ctx context.Context, _ *struct{}) (*statsGrabsOutput, error) {
		overall, indexers, err := svc.GrabPerformance(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to get grab stats", err)
		}
		if indexers == nil {
			indexers = []stats.IndexerStat{}
		}
		return &statsGrabsOutput{Body: statsGrabsBody{
			TotalGrabs:  overall.TotalGrabs,
			Successful:  overall.Successful,
			Failed:      overall.Failed,
			SuccessRate: overall.SuccessRate,
			TopIndexers: indexers,
		}}, nil
	})
}
