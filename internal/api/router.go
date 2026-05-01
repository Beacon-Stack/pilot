package api

import (
	"crypto/subtle"
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/beacon-stack/pilot/internal/api/middleware"
	v1 "github.com/beacon-stack/pilot/internal/api/v1"
	"github.com/beacon-stack/pilot/internal/api/ws"
	"github.com/beacon-stack/pilot/internal/appinfo"
	"github.com/beacon-stack/pilot/internal/config"
	"github.com/beacon-stack/pilot/internal/core/activity"
	"github.com/beacon-stack/pilot/internal/core/blocklist"
	"github.com/beacon-stack/pilot/internal/core/downloader"
	"github.com/beacon-stack/pilot/internal/core/importer"
	"github.com/beacon-stack/pilot/internal/core/importlist"
	"github.com/beacon-stack/pilot/internal/core/indexer"
	"github.com/beacon-stack/pilot/internal/core/library"
	"github.com/beacon-stack/pilot/internal/core/mediamanagement"
	"github.com/beacon-stack/pilot/internal/core/mediaserver"
	"github.com/beacon-stack/pilot/internal/core/notification"
	"github.com/beacon-stack/pilot/internal/core/provider"
	"github.com/beacon-stack/pilot/internal/core/quality"
	"github.com/beacon-stack/pilot/internal/core/queue"
	"github.com/beacon-stack/pilot/internal/core/show"
	"github.com/beacon-stack/pilot/internal/core/stats"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/logging"
	"github.com/beacon-stack/pilot/internal/scheduler"
	"github.com/beacon-stack/pilot/internal/sonarrimport"
	"github.com/beacon-stack/pilot/internal/version"
	"github.com/beacon-stack/pilot/web"
)

// RouterConfig holds everything the router needs to function.
type RouterConfig struct {
	Auth                   config.Secret
	Logger                 *slog.Logger
	StartTime              time.Time
	DB                     *sql.DB
	DBType                 string
	DBPath                 string
	ConfigFile             string
	TMDBKeyConfigured      bool
	TMDBKeyIsDefault       bool
	LogBuffer              *logging.RingBuffer
	Queries                db.Querier
	ShowService            *show.Service
	QualityService         *quality.Service
	LibraryService         *library.Service
	IndexerService         *indexer.Service
	DownloaderService      *downloader.Service
	NotificationService    *notification.Service
	MediaServerService     *mediaserver.Service
	BlocklistService       *blocklist.Service
	MediaManagementService *mediamanagement.Service
	QualityDefService      *quality.DefinitionService
	QueueService           *queue.Service
	ImporterService        *importer.Service
	ActivityService        *activity.Service
	StatsService           *stats.Service
	ImportListService      *importlist.Service
	SonarrImportService    *sonarrimport.Service
	ProviderResolver       *provider.Resolver
	WSHub                  *ws.Hub
	Scheduler              *scheduler.Scheduler
	PulseSyncHandler       http.HandlerFunc
}

// NewRouter builds and returns the application HTTP handler.
func NewRouter(cfg RouterConfig) http.Handler {
	r := chi.NewRouter()

	// Global middleware — applied to every request including /health.
	r.Use(chimiddleware.RequestID)
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.MaxRequestBodySize(1 << 20)) // 1 MiB
	r.Use(middleware.RequestLogger(cfg.Logger))
	r.Use(middleware.Recovery(cfg.Logger))

	// Unauthenticated health check for load balancers / container probes.
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// Pulse sync webhook — called by Beacon Pulse when configs change.
	if cfg.PulseSyncHandler != nil {
		r.Post("/api/v1/hooks/pulse/sync", cfg.PulseSyncHandler)
	}

	// WebSocket endpoint — registered directly on chi so the upgrade
	// bypasses the huma JSON middleware stack.
	if cfg.WSHub != nil {
		r.Get("/api/v1/ws", cfg.WSHub.ServeHTTP)
	}

	// Backup / restore — registered directly on chi (binary body/response, not JSON).
	if cfg.DB != nil && cfg.DBPath != "" && cfg.DBType == "sqlite" {
		authKey := []byte(cfg.Auth.Value())
		withAuth := func(next http.HandlerFunc) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Sec-Fetch-Site") == "same-origin" {
					next(w, r)
					return
				}
				if len(authKey) > 0 && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Api-Key")), authKey) == 1 {
					next(w, r)
					return
				}
				http.Error(w, `{"status":401,"title":"Unauthorized"}`, http.StatusUnauthorized)
			}
		}
		r.Get("/api/v1/system/backup", withAuth(v1.BackupHandler(cfg.DB, cfg.DBPath, cfg.Logger)))
		r.Post("/api/v1/system/restore", withAuth(v1.RestoreHandler(cfg.DBPath, cfg.Logger)))
	}

	humaConfig := huma.DefaultConfig(appinfo.AppName+" API", version.Version)
	humaConfig.DocsPath = "/api/docs"
	humaConfig.OpenAPIPath = "/api/openapi"
	humaConfig.SchemasPath = "/api/schemas"
	humaConfig.Info.Description = appinfo.AppName + " TV series manager API. " +
		"Browser requests are authenticated via Sec-Fetch-Site; external clients must provide X-Api-Key."

	humaAPI := humachi.New(r, humaConfig)

	// Register X-Api-Key security scheme for the docs UI.
	oapi := humaAPI.OpenAPI()
	if oapi.Components == nil {
		oapi.Components = &huma.Components{}
	}
	if oapi.Components.SecuritySchemes == nil {
		oapi.Components.SecuritySchemes = map[string]*huma.SecurityScheme{}
	}
	oapi.Components.SecuritySchemes["ApiKeyAuth"] = &huma.SecurityScheme{
		Type: "apiKey",
		In:   "header",
		Name: "X-Api-Key",
	}
	oapi.Security = []map[string][]string{{"ApiKeyAuth": {}}}

	// Auth middleware: same-origin browser requests are trusted; external
	// consumers must provide a valid X-Api-Key header.
	apiKeyBytes := []byte(cfg.Auth.Value())
	humaAPI.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		if ctx.Header("Sec-Fetch-Site") == "same-origin" {
			next(ctx)
			return
		}
		if len(apiKeyBytes) > 0 && subtle.ConstantTimeCompare([]byte(ctx.Header("X-Api-Key")), apiKeyBytes) == 1 {
			next(ctx)
			return
		}
		_ = huma.WriteErr(humaAPI, ctx, http.StatusUnauthorized, "A valid X-Api-Key header is required.")
	})

	v1.RegisterSystemRoutes(humaAPI, cfg.StartTime, cfg.DBType, cfg.DBPath, cfg.ConfigFile, cfg.Auth.Value(), cfg.TMDBKeyConfigured, cfg.TMDBKeyIsDefault, cfg.Logger)
	v1.RegisterRuntimeRoutes(humaAPI, cfg.StartTime)
	v1.RegisterEnvRoutes(humaAPI)
	v1.RegisterFilesystemRoutes(humaAPI)

	if cfg.LogBuffer != nil {
		v1.RegisterLogRoutes(humaAPI, cfg.LogBuffer)
	}

	if cfg.ShowService != nil {
		v1.RegisterSeriesRoutes(humaAPI, cfg.ShowService)
		v1.RegisterEpisodeFileRoutes(humaAPI, cfg.ShowService, cfg.MediaManagementService)
	}

	if cfg.QualityService != nil {
		v1.RegisterQualityProfileRoutes(humaAPI, cfg.QualityService)
	}

	if cfg.LibraryService != nil {
		v1.RegisterLibraryRoutes(humaAPI, cfg.LibraryService)
	}

	if cfg.IndexerService != nil {
		v1.RegisterIndexerRoutes(humaAPI, cfg.IndexerService)
		v1.RegisterReleaseRoutes(humaAPI, cfg.IndexerService, cfg.ShowService, cfg.DownloaderService, cfg.BlocklistService, cfg.QualityService)
	}

	if cfg.DownloaderService != nil {
		v1.RegisterDownloadClientRoutes(humaAPI, cfg.DownloaderService)
	}

	// Haul history endpoints — needs both downloader (to find a
	// Haul client) and importer (to run the import pipeline).
	if cfg.DownloaderService != nil && cfg.ImporterService != nil {
		v1.RegisterHaulHistoryRoutes(humaAPI, cfg.DownloaderService, cfg.ImporterService)
	}

	if cfg.NotificationService != nil {
		v1.RegisterNotificationRoutes(humaAPI, cfg.NotificationService)
	}

	if cfg.MediaServerService != nil {
		v1.RegisterMediaServerRoutes(humaAPI, cfg.MediaServerService)
	}

	if cfg.BlocklistService != nil {
		v1.RegisterBlocklistRoutes(humaAPI, cfg.BlocklistService)
	}

	if cfg.MediaManagementService != nil {
		v1.RegisterMediaManagementRoutes(humaAPI, cfg.MediaManagementService)
	}

	if cfg.QualityDefService != nil {
		v1.RegisterQualityDefinitionRoutes(humaAPI, cfg.QualityDefService)
	}

	if cfg.QueueService != nil {
		v1.RegisterQueueRoutes(humaAPI, cfg.QueueService, cfg.BlocklistService)
	}

	if cfg.Scheduler != nil {
		v1.RegisterTaskRoutes(humaAPI, cfg.Scheduler)
	}

	if cfg.Queries != nil {
		v1.RegisterCalendarRoutes(humaAPI, cfg.Queries)
		v1.RegisterWantedRoutes(humaAPI, cfg.Queries)
		v1.RegisterHistoryRoutes(humaAPI, cfg.Queries)
	}

	if cfg.ActivityService != nil {
		v1.RegisterActivityRoutes(humaAPI, cfg.ActivityService)
	}

	if cfg.StatsService != nil {
		v1.RegisterStatsRoutes(humaAPI, cfg.StatsService)
	}

	if cfg.ImportListService != nil {
		v1.RegisterImportListRoutes(humaAPI, cfg.ImportListService)
	}

	if cfg.SonarrImportService != nil {
		v1.RegisterImportRoutes(humaAPI, cfg.SonarrImportService)
	}

	if cfg.ProviderResolver != nil {
		v1.RegisterProviderRoutes(humaAPI, cfg.ProviderResolver)
	}

	// Serve the embedded React SPA — must come after all API routes.
	r.Handle("/*", web.ServeStatic())

	return r
}
