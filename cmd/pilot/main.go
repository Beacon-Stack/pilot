package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	// Import pgx stdlib for database/sql compatibility (Postgres support).
	_ "github.com/jackc/pgx/v5/stdlib"

	// Register indexer plugins so they self-register with registry.Default.
	_ "github.com/beacon-stack/pilot/plugins/indexers/newznab"
	_ "github.com/beacon-stack/pilot/plugins/indexers/torznab"

	// Register downloader plugins.
	_ "github.com/beacon-stack/pilot/plugins/downloaders/deluge"
	_ "github.com/beacon-stack/pilot/plugins/downloaders/haul"
	_ "github.com/beacon-stack/pilot/plugins/downloaders/nzbget"
	_ "github.com/beacon-stack/pilot/plugins/downloaders/qbittorrent"
	_ "github.com/beacon-stack/pilot/plugins/downloaders/sabnzbd"
	_ "github.com/beacon-stack/pilot/plugins/downloaders/transmission"

	// Register notification plugins.
	_ "github.com/beacon-stack/pilot/plugins/notifications/command"
	_ "github.com/beacon-stack/pilot/plugins/notifications/discord"
	_ "github.com/beacon-stack/pilot/plugins/notifications/email"
	_ "github.com/beacon-stack/pilot/plugins/notifications/gotify"
	_ "github.com/beacon-stack/pilot/plugins/notifications/ntfy"
	_ "github.com/beacon-stack/pilot/plugins/notifications/pushover"
	_ "github.com/beacon-stack/pilot/plugins/notifications/slack"
	_ "github.com/beacon-stack/pilot/plugins/notifications/telegram"
	_ "github.com/beacon-stack/pilot/plugins/notifications/webhook"

	// Register media server plugins.
	_ "github.com/beacon-stack/pilot/plugins/mediaservers/emby"
	_ "github.com/beacon-stack/pilot/plugins/mediaservers/jellyfin"
	_ "github.com/beacon-stack/pilot/plugins/mediaservers/plex"

	// Register import list plugins.
	_ "github.com/beacon-stack/pilot/plugins/importlists/custom_list"
	_ "github.com/beacon-stack/pilot/plugins/importlists/plex_watchlist_tv"
	_ "github.com/beacon-stack/pilot/plugins/importlists/tmdb_popular_tv"
	_ "github.com/beacon-stack/pilot/plugins/importlists/tmdb_trending_tv"
	_ "github.com/beacon-stack/pilot/plugins/importlists/trakt_list_tv"
	_ "github.com/beacon-stack/pilot/plugins/importlists/trakt_popular_tv"
	_ "github.com/beacon-stack/pilot/plugins/importlists/trakt_trending_tv"

	"github.com/beacon-stack/pilot/internal/api"
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
	"github.com/beacon-stack/pilot/internal/core/quality"
	"github.com/beacon-stack/pilot/internal/core/queue"
	"github.com/beacon-stack/pilot/internal/core/show"
	"github.com/beacon-stack/pilot/internal/core/stallwatcher"
	"github.com/beacon-stack/pilot/internal/core/stats"
	"github.com/beacon-stack/pilot/internal/db"
	dbgen "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/events"
	"github.com/beacon-stack/pilot/internal/logging"
	"github.com/beacon-stack/pilot/internal/metadata/tmdbtv"
	pulseint "github.com/beacon-stack/pilot/internal/pulse"
	"github.com/beacon-stack/pilot/internal/ratelimit"
	"github.com/beacon-stack/pilot/internal/registry"
	"github.com/beacon-stack/pilot/internal/scheduler"
	"github.com/beacon-stack/pilot/internal/scheduler/jobs"
	"github.com/beacon-stack/pilot/internal/sonarrimport"
	"github.com/beacon-stack/pilot/internal/trakt"
	"github.com/beacon-stack/pilot/internal/version"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var cfgFile string
	flag.StringVar(&cfgFile, "config", "", "path to config file (default: ~/.config/pilot/config.yaml)")
	flag.Parse()

	// ── Config ────────────────────────────────────────────────────────────────
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// ── Logger ────────────────────────────────────────────────────────────────
	logger, logBuffer := logging.New(cfg.Log.Level, cfg.Log.Format)
	slog.SetDefault(logger)

	// Advisory config file permission check.
	checkPath := cfg.ConfigFile
	if checkPath == "" {
		checkPath = cfgFile
	}
	if checkPath != "" {
		if info, statErr := os.Stat(checkPath); statErr == nil {
			if info.Mode()&0o044 != 0 {
				if chmodErr := os.Chmod(checkPath, 0o600); chmodErr != nil {
					logger.Warn("config file is group- or world-readable and chmod failed — please run: chmod 600 "+checkPath,
						"path", checkPath,
						"error", chmodErr,
					)
				} else {
					logger.Info("fixed config file permissions to 0600", "path", checkPath)
				}
			}
		}
	}

	// ── API Key ───────────────────────────────────────────────────────────────
	generated, err := config.EnsureAPIKey(cfg)
	if err != nil {
		return fmt.Errorf("ensuring API key: %w", err)
	}
	if generated {
		if _, persistErr := config.WriteConfigKey(cfg.ConfigFile, "auth.api_key", cfg.Auth.APIKey.Value()); persistErr != nil {
			fmt.Fprintf(os.Stderr, "\n  !! API key generated but could not be saved to disk.\n"+
				"  !! It will change on next restart. Set it now in your client:\n"+
				"  !!\n"+
				"  !!   API key: %s\n"+
				"  !!\n"+
				"  !! Hint: mount a writable volume at /config (Docker) or ensure\n"+
				"  !!        ~/.config/pilot/ is writable.\n\n",
				cfg.Auth.APIKey.Value())
			logger.Warn("API key generated but could not be persisted — it will change on next restart",
				"hint", "mount a writable volume at /config (Docker) or ensure ~/.config/pilot/ is writable",
				"error", persistErr,
			)
		} else {
			logger.Info("API key generated and saved to config — stable across restarts")
		}
	} else {
		key := cfg.Auth.APIKey.Value()
		masked := key
		if len(key) > 4 {
			masked = key[:4] + "****"
		}
		logger.Info("API key loaded", "key_prefix", masked, "source", "config/env")
	}

	// ── Startup banner ────────────────────────────────────────────────────────
	configFile := cfg.ConfigFile
	if configFile == "" {
		configFile = "(none — using defaults/env)"
	}
	logger.Info(appinfo.AppName+" starting",
		"version", version.Version,
		"build_time", version.BuildTime,
		"go", version.GoVersion(),
		"db", cfg.Database.Driver,
		"config_file", configFile,
	)

	// ── Database restore staging check ────────────────────────────────────────
	if cfg.Database.Path != "" {
		stagingPath := cfg.Database.Path + ".restore"
		if _, statErr := os.Stat(stagingPath); statErr == nil {
			if renameErr := os.Rename(stagingPath, cfg.Database.Path); renameErr == nil {
				logger.Info("database restored from backup")
			} else {
				logger.Warn("failed to swap restore file into place", "error", renameErr)
			}
		}
	}

	// ── Database ──────────────────────────────────────────────────────────────
	database, err := db.Open(cfg.Database)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer database.Close()

	logger.Info("database connected", "driver", database.Driver)

	if err := db.Migrate(database.SQL, database.Driver); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	logger.Info("database migrations up to date")

	// ── Event bus ─────────────────────────────────────────────────────────────
	bus := events.New(logger)

	// ── Services ──────────────────────────────────────────────────────────────
	queries := dbgen.New(database.SQL)

	qualitySvc := quality.NewService(queries, bus)
	librarySvc := library.NewService(queries, bus)

	var tmdbClient *tmdbtv.Client
	if !cfg.TVDB.APIKey.IsEmpty() {
		tmdbClient = tmdbtv.New(cfg.TVDB.APIKey.Value(), logger)
	}
	var showMeta show.MetadataProvider
	if tmdbClient != nil {
		showMeta = tmdbClient
	}
	showSvc := show.NewService(queries, showMeta, bus, logger)

	indexerRL := ratelimit.New()
	indexerSvc := indexer.NewService(queries, registry.Default, bus, indexerRL)

	downloaderSvc := downloader.NewService(queries, registry.Default, bus)
	notificationSvc := notification.NewService(queries, registry.Default)
	mediaServerSvc := mediaserver.NewService(queries, registry.Default)
	blocklistSvc := blocklist.NewService(queries)
	mediaManagementSvc := mediamanagement.NewService(queries)
	qualityDefSvc := quality.NewDefinitionService(queries)
	queueSvc := queue.NewService(queries, downloaderSvc, bus, logger)
	stallWatcher := stallwatcher.NewService(queries, blocklistSvc, downloaderSvc, bus, logger)

	importerSvc := importer.NewService(queries, bus, logger, mediaManagementSvc)
	importerSvc.Subscribe()

	activitySvc := activity.NewService(queries, logger)
	activitySvc.Subscribe(bus)

	statsSvc := stats.NewService(queries)

	var traktClient *trakt.Client
	if !cfg.Trakt.ClientID.IsEmpty() {
		traktClient = trakt.New(cfg.Trakt.ClientID.Value(), logger)
	}

	importListSvc := importlist.NewService(queries, registry.Default, showSvc, tmdbClient, traktClient, logger)

	sonarrImportSvc := sonarrimport.NewService(showSvc, qualitySvc, librarySvc, indexerSvc, downloaderSvc)

	wsHub := ws.NewHub(logger, []byte(cfg.Auth.APIKey.Value()))
	bus.Subscribe(wsHub.HandleEvent)

	// ── Scheduler ─────────────────────────────────────────────────────────────
	sched := scheduler.New(logger)
	sched.Add(jobs.QueuePoll(queueSvc, 30*time.Second, logger))
	sched.Add(jobs.LibraryScan(librarySvc, showSvc, queries, logger))
	sched.Add(jobs.RSSSync(indexerSvc, queries, downloaderSvc, logger))
	sched.Add(jobs.RefreshMetadata(showSvc, logger))
	sched.Add(jobs.StatsSnapshot(statsSvc, logger))
	sched.Add(jobs.ImportListSync(importListSvc, logger))
	sched.Add(jobs.ActivityPrune(activitySvc, logger))

	// ── Pulse integration (optional) ──────────────────────────────────────────
	pulseIntegration, err := pulseint.New(cfg.Pulse, cfg.Server.Host, cfg.Server.Port, logger)
	if err != nil {
		logger.Warn("pulse integration failed — continuing without it", "error", err)
	}
	if pulseIntegration != nil {
		defer pulseIntegration.Close()
	}

	// ── HTTP router ───────────────────────────────────────────────────────────
	startTime := time.Now()
	router := api.NewRouter(api.RouterConfig{
		Auth:                   cfg.Auth.APIKey,
		Logger:                 logger,
		StartTime:              startTime,
		DB:                     database.SQL,
		DBType:                 database.Driver,
		DBPath:                 cfg.Database.Path,
		ConfigFile:             cfg.ConfigFile,
		LogBuffer:              logBuffer,
		Queries:                queries,
		ShowService:            showSvc,
		QualityService:         qualitySvc,
		LibraryService:         librarySvc,
		IndexerService:         indexerSvc,
		DownloaderService:      downloaderSvc,
		NotificationService:    notificationSvc,
		MediaServerService:     mediaServerSvc,
		BlocklistService:       blocklistSvc,
		MediaManagementService: mediaManagementSvc,
		QualityDefService:      qualityDefSvc,
		QueueService:           queueSvc,
		ImporterService:        importerSvc,
		ActivityService:        activitySvc,
		StatsService:           statsSvc,
		ImportListService:      importListSvc,
		SonarrImportService:    sonarrImportSvc,
		WSHub:                  wsHub,
		Scheduler:              sched,
		PulseSyncHandler:       pulseSyncHandler(pulseIntegration, indexerSvc, downloaderSvc),
	})

	// ── Background services ───────────────────────────────────────────────────
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	// Pulse sync — pull indexers, download clients, quality profiles, and shared settings from the control plane.
	if pulseIntegration != nil {
		go pulseIntegration.StartSyncLoop(appCtx, indexerSvc, downloaderSvc, qualitySvc, mediaManagementSvc, 30*time.Second)
	}

	// Stall watcher — poll Haul every 60s for dead torrents and blocklist
	// the corresponding releases. See plans/dead-torrent-phase0.md.
	go stallWatcher.Run(appCtx)

	go sched.Start(appCtx)

	// ── HTTP server ───────────────────────────────────────────────────────────
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("HTTP server listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// ── Graceful shutdown ─────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	case sig := <-quit:
		logger.Info("shutdown signal received", "signal", sig)
	}

	// Stop the scheduler and background goroutines.
	appCancel()

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutCancel()

	if err := srv.Shutdown(shutCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}

	logger.Info("server stopped cleanly")
	return nil
}

// pulseSyncHandler returns the Pulse sync webhook handler, or nil if disabled.
func pulseSyncHandler(integration *pulseint.Integration, indexerSvc *indexer.Service, dlSvc *downloader.Service) http.HandlerFunc {
	if integration == nil {
		return nil
	}
	return integration.SyncHandler(indexerSvc, dlSvc)
}
