package api

import (
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
	"github.com/beacon-stack/pilot/internal/core/health"
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
	"github.com/beacon-stack/pilot/internal/core/releaseprofile"
	"github.com/beacon-stack/pilot/internal/core/remotepath"
	"github.com/beacon-stack/pilot/internal/core/show"
	"github.com/beacon-stack/pilot/internal/core/stats"
	"github.com/beacon-stack/pilot/internal/core/wanted"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/scheduler"
	"github.com/beacon-stack/pilot/internal/sonarrimport"
	"github.com/beacon-stack/pilot/internal/version"
	"github.com/beacon-stack/pilot/web"
	beaconlog "github.com/beacon-stack/pulse/pkg/log"
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
	LogSystem              *beaconlog.System
	DockerLogs             *beaconlog.DockerLogsReader
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
	ReleaseProfileService  *releaseprofile.Service
	WSHub                  *ws.Hub
	Scheduler              *scheduler.Scheduler
	HealthService          *health.Service
	RemotePathService      *remotepath.Service
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

	// Browser login/logout — the only unauthenticated /api routes.
	auth := middleware.NewAuth(cfg.Auth.Value())
	r.Post("/api/v1/auth/login", auth.LoginHandler())
	r.Post("/api/v1/auth/logout", auth.LogoutHandler())

	// Every route registered on `protected` (including the huma API and
	// its docs) requires an X-Api-Key header or a session cookie. Auth is
	// attached per route rather than by path prefix so nothing can route
	// around it.
	protected := r.With(auth.Middleware)

	// Pulse sync webhook — called by Beacon Pulse when configs change.
	if cfg.PulseSyncHandler != nil {
		protected.Post("/api/v1/hooks/pulse/sync", cfg.PulseSyncHandler)
	}

	// WebSocket endpoint — registered directly on chi so the upgrade
	// bypasses the huma JSON middleware stack.
	if cfg.WSHub != nil {
		protected.Get("/api/v1/ws", cfg.WSHub.ServeHTTP)
	}

	// Backup / restore — registered directly on chi (binary body/response, not JSON).
	if cfg.DB != nil && cfg.DBPath != "" {
		protected.Get("/api/v1/system/backup", v1.BackupHandler(cfg.DB, cfg.DBPath, cfg.Logger))
		protected.Post("/api/v1/system/restore", v1.RestoreHandler(cfg.DBPath, cfg.Logger))
	}

	humaConfig := huma.DefaultConfig(appinfo.AppName+" API", version.Version)
	humaConfig.DocsPath = "/api/docs"
	humaConfig.OpenAPIPath = "/api/openapi"
	humaConfig.SchemasPath = "/api/schemas"
	humaConfig.Info.Description = appinfo.AppName + " TV series manager API. " +
		"Browsers authenticate with a session cookie from POST /api/v1/auth/login; " +
		"other clients must provide X-Api-Key."

	humaAPI := humachi.New(protected, humaConfig)

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

	v1.RegisterSystemRoutes(humaAPI, cfg.StartTime, cfg.DBType, cfg.DBPath, cfg.ConfigFile, cfg.Auth.Value(), cfg.TMDBKeyConfigured, cfg.TMDBKeyIsDefault, cfg.Logger)
	v1.RegisterRuntimeRoutes(humaAPI, cfg.StartTime)
	v1.RegisterEnvRoutes(humaAPI)
	v1.RegisterFilesystemRoutes(humaAPI)

	if cfg.LogSystem != nil {
		beaconlog.RegisterRoutesWithDocker(humaAPI, cfg.LogSystem, cfg.DockerLogs)
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

	// Haul history endpoints — needs querier (for grab_history
	// lookups), downloader (to find a Haul client), and importer
	// (to run the import pipeline).
	if cfg.Queries != nil && cfg.DownloaderService != nil && cfg.ImporterService != nil {
		v1.RegisterHaulHistoryRoutes(humaAPI, cfg.Queries, cfg.DownloaderService, cfg.ImporterService, cfg.RemotePathService)
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

	if cfg.RemotePathService != nil {
		v1.RegisterRemotePathMappingRoutes(humaAPI, cfg.RemotePathService)
	}
	if cfg.HealthService != nil {
		v1.RegisterHealthRoutes(humaAPI, cfg.HealthService)
	}
	if cfg.Scheduler != nil {
		v1.RegisterTaskRoutes(humaAPI, cfg.Scheduler)
	}

	if cfg.Queries != nil {
		v1.RegisterCalendarRoutes(humaAPI, cfg.Queries)
		// Pass the profile getter only when set: a nil *quality.Service in
		// a non-nil interface would panic on use.
		var profiles wanted.ProfileGetter
		if cfg.QualityService != nil {
			profiles = cfg.QualityService
		}
		v1.RegisterWantedRoutes(humaAPI, cfg.Queries, profiles)
		if cfg.ShowService != nil && cfg.IndexerService != nil && cfg.DownloaderService != nil {
			v1.RegisterWantedSearchRoutes(humaAPI, cfg.Queries, cfg.ShowService, cfg.IndexerService,
				cfg.DownloaderService, cfg.BlocklistService, cfg.QualityService, cfg.Logger)
		}
		v1.RegisterHistoryRoutes(humaAPI, cfg.Queries)
		v1.RegisterDelayRoutes(humaAPI, cfg.Queries)
		v1.RegisterLanguageRoutes(humaAPI, cfg.Queries)
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

	if cfg.ReleaseProfileService != nil {
		v1.RegisterReleaseProfileRoutes(humaAPI, cfg.ReleaseProfileService)
	}

	if cfg.ProviderResolver != nil {
		v1.RegisterProviderRoutes(humaAPI, cfg.ProviderResolver)
	}

	// Serve the embedded React SPA — must come after all API routes.
	r.Handle("/*", web.ServeStatic())

	return r
}
