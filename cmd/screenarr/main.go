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
	_ "github.com/screenarr/screenarr/plugins/indexers/newznab"
	_ "github.com/screenarr/screenarr/plugins/indexers/torznab"

	// Register downloader plugins.
	_ "github.com/screenarr/screenarr/plugins/downloaders/deluge"
	_ "github.com/screenarr/screenarr/plugins/downloaders/nzbget"
	_ "github.com/screenarr/screenarr/plugins/downloaders/qbittorrent"
	_ "github.com/screenarr/screenarr/plugins/downloaders/sabnzbd"
	_ "github.com/screenarr/screenarr/plugins/downloaders/transmission"

	// Register notification plugins.
	_ "github.com/screenarr/screenarr/plugins/notifications/command"
	_ "github.com/screenarr/screenarr/plugins/notifications/discord"
	_ "github.com/screenarr/screenarr/plugins/notifications/email"
	_ "github.com/screenarr/screenarr/plugins/notifications/gotify"
	_ "github.com/screenarr/screenarr/plugins/notifications/ntfy"
	_ "github.com/screenarr/screenarr/plugins/notifications/pushover"
	_ "github.com/screenarr/screenarr/plugins/notifications/slack"
	_ "github.com/screenarr/screenarr/plugins/notifications/telegram"
	_ "github.com/screenarr/screenarr/plugins/notifications/webhook"

	// Register media server plugins.
	_ "github.com/screenarr/screenarr/plugins/mediaservers/emby"
	_ "github.com/screenarr/screenarr/plugins/mediaservers/jellyfin"
	_ "github.com/screenarr/screenarr/plugins/mediaservers/plex"

	// Register import list plugins.
	_ "github.com/screenarr/screenarr/plugins/importlists/custom_list"
	_ "github.com/screenarr/screenarr/plugins/importlists/plex_watchlist_tv"
	_ "github.com/screenarr/screenarr/plugins/importlists/tmdb_popular_tv"
	_ "github.com/screenarr/screenarr/plugins/importlists/tmdb_trending_tv"
	_ "github.com/screenarr/screenarr/plugins/importlists/trakt_list_tv"
	_ "github.com/screenarr/screenarr/plugins/importlists/trakt_popular_tv"
	_ "github.com/screenarr/screenarr/plugins/importlists/trakt_trending_tv"

	"github.com/screenarr/screenarr/internal/api"
	"github.com/screenarr/screenarr/internal/api/ws"
	"github.com/screenarr/screenarr/internal/appinfo"
	"github.com/screenarr/screenarr/internal/config"
	"github.com/screenarr/screenarr/internal/core/activity"
	"github.com/screenarr/screenarr/internal/core/blocklist"
	"github.com/screenarr/screenarr/internal/core/downloader"
	"github.com/screenarr/screenarr/internal/core/importer"
	"github.com/screenarr/screenarr/internal/core/importlist"
	"github.com/screenarr/screenarr/internal/core/indexer"
	"github.com/screenarr/screenarr/internal/core/library"
	"github.com/screenarr/screenarr/internal/core/mediamanagement"
	"github.com/screenarr/screenarr/internal/core/mediaserver"
	"github.com/screenarr/screenarr/internal/core/notification"
	"github.com/screenarr/screenarr/internal/core/quality"
	"github.com/screenarr/screenarr/internal/core/queue"
	"github.com/screenarr/screenarr/internal/core/show"
	"github.com/screenarr/screenarr/internal/core/stats"
	"github.com/screenarr/screenarr/internal/db"
	dbsqlite "github.com/screenarr/screenarr/internal/db/generated/sqlite"
	"github.com/screenarr/screenarr/internal/events"
	"github.com/screenarr/screenarr/internal/logging"
	"github.com/screenarr/screenarr/internal/metadata/tmdbtv"
	"github.com/screenarr/screenarr/internal/ratelimit"
	"github.com/screenarr/screenarr/internal/registry"
	"github.com/screenarr/screenarr/internal/scheduler"
	"github.com/screenarr/screenarr/internal/scheduler/jobs"
	"github.com/screenarr/screenarr/internal/sonarrimport"
	"github.com/screenarr/screenarr/internal/trakt"
	"github.com/screenarr/screenarr/internal/version"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var cfgFile string
	flag.StringVar(&cfgFile, "config", "", "path to config file (default: ~/.config/screenarr/config.yaml)")
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
				"  !!        ~/.config/screenarr/ is writable.\n\n",
				cfg.Auth.APIKey.Value())
			logger.Warn("API key generated but could not be persisted — it will change on next restart",
				"hint", "mount a writable volume at /config (Docker) or ensure ~/.config/screenarr/ is writable",
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

	if database.Driver == "sqlite" {
		logger.Info("database connected", "driver", database.Driver, "path", cfg.Database.Path)
	} else {
		logger.Info("database connected", "driver", database.Driver)
	}

	if err := db.Migrate(database.SQL, database.Driver); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	logger.Info("database migrations up to date")

	// ── Event bus ─────────────────────────────────────────────────────────────
	bus := events.New(logger)

	// ── Services ──────────────────────────────────────────────────────────────
	queries := dbsqlite.New(database.SQL)

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
	})

	// ── Background services ───────────────────────────────────────────────────
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

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
