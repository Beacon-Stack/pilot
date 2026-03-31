# Screenarr — Full Feature Implementation Plan

## Context

Screenarr is a Sonarr clone — a TV series manager that automates episode acquisition, quality management, and library organization. It's the sibling project to Luminarr (Radarr clone for movies). The project skeleton already exists at `~/dev/screenarr` with config, db, events, logging, API router, health/status endpoints, and a React shell. Now we need the actual features.

**Goal**: Clone Sonarr's feature set using Luminarr's architecture patterns and UI design language. No improvements yet — feature parity first.

**Key architectural difference from Luminarr**: The data model is hierarchical (Series > Season > Episode > EpisodeFile) instead of flat (Movie > MovieFile). Everything flows from this.

## Architectural Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Metadata provider | **TMDB TV API** (not TVDB) | Free, no subscription, Luminarr already has TMDB key, endpoints exist (`/search/tv`, `/tv/{id}`, `/tv/{id}/season/{n}`) |
| Episode monitoring | Per-episode granularity | Sonarr model: monitor all, future, specific seasons, or individual episodes |
| Series types | Standard, Daily, Anime | Affects search queries, filename parsing, and naming templates |
| Season packs | Supported | Parser detects "S01" without episode number; importer maps files inside to episodes |
| Plugins | Copy from Luminarr | Indexers, downloaders, notifications, media servers are protocol-level — TV categories passed at search time |
| Sonarr v3 compat | Phase 8 stretch goal | Like Luminarr's Radarr v3 layer for Overseerr/Homepage integration |

## Reuse Summary

**Copy as-is from Luminarr** (module path changes only):
- `pkg/plugin/` — all plugin interfaces
- `plugins/` — all indexer, downloader, notification, media server plugins
- `internal/registry/` — plugin factory registry
- `internal/ratelimit/` — rate limiting
- `internal/safedialer/` — secure HTTP dialer
- `internal/scheduler/` — job scheduler framework
- `internal/parser/` — release title parsing (already handles S01E01)
- `internal/core/quality/` — quality profiles + definitions (media-agnostic)
- `internal/core/tag/` — tag system (change movie_tags → series_tags)
- `internal/core/customformat/` — custom format specs + scoring
- `internal/core/downloader/` — download client orchestration
- `internal/core/health/` — health checks
- `internal/core/notification/`, `internal/notifications/` — notification dispatch
- `internal/core/mediaserver/`, `internal/mediaservers/` — media server dispatch
- `internal/api/ws/` — WebSocket hub
- Frontend: theme system, Shell layout, all reusable components (PageHeader, Modal, Pill, Poster, QualityBadge, StatusBadge, RangeSlider, DirPicker, ErrorBoundary, CommandPalette)

**Write new / heavily adapt**:
- `internal/metadata/tmdb_tv/` — TMDB TV API client (new)
- `internal/core/show/` — series/season/episode domain service (new, replaces movie/)
- `internal/core/autosearch/` — episode-level search (adapted)
- `internal/core/importer/` — season pack handling (adapted)
- `internal/core/renamer/` — TV naming templates (adapted)
- `internal/core/stats/` — episode-based statistics (adapted)
- `internal/core/library/` — show-based path structure (adapted)
- Database schema — entirely new (series/seasons/episodes)
- Frontend pages — all new/adapted for TV

---

## Phase 1: Data Model, Metadata, and Basic CRUD

**Adds**: Series/season/episode entities, TMDB TV metadata, libraries, quality profiles, add/list/view/edit/delete shows.

### Database Migrations

**`00002_libraries_quality_profiles.sql`** — Copy from Luminarr:
- `libraries` (id, name, root_path, default_quality_profile_id, naming_format, min_free_space_gb, tags_json)
- `quality_profiles` (id, name, cutoff_json, qualities_json, upgrade_allowed, upgrade_until_json, min_custom_format_score, upgrade_until_cf_score)

**`00003_series.sql`** — New core schema:
- `series` (id, tvdb_id→tmdb_id, imdb_id, title, sort_title, year, overview, runtime_minutes, genres_json, poster_url, fanart_url, status [continuing/ended/upcoming], series_type [standard/daily/anime], monitor_type [all/future/missing/existing/pilot/first_season/last_season/none], network, air_time, certification, monitored, library_id FK, quality_profile_id FK, path, added_at, updated_at, metadata_refreshed_at)
- `seasons` (id, series_id FK CASCADE, season_number, monitored, UNIQUE(series_id, season_number))
- `episodes` (id, series_id FK CASCADE, season_id FK CASCADE, season_number, episode_number, absolute_number, air_date, title, overview, monitored, has_file, UNIQUE(series_id, season_number, episode_number))

### Go Packages

- **`internal/metadata/tmdb_tv/`** — New. Client for TMDB TV endpoints:
  - `SearchSeries(ctx, query, year)` → `[]SeriesSearchResult`
  - `GetSeries(ctx, tmdbID)` → `SeriesDetail` (with seasons)
  - `GetSeasonEpisodes(ctx, tmdbID, seasonNum)` → `[]EpisodeDetail`
  - Types: `SeriesSearchResult`, `SeriesDetail`, `SeasonDetail`, `EpisodeDetail`

- **`internal/core/show/`** — New. Domain service (follows movie.Service pattern):
  - `Service` struct with querier + MetadataProvider interface + event bus + logger
  - Methods: `Add`, `Get`, `List` (paginated), `Update`, `Delete`, `Lookup`, `RefreshMetadata`
  - `Add` fetches metadata → creates series row → creates season rows → creates episode rows → applies monitor_type logic
  - `MetadataProvider` interface so provider can be swapped

- **`internal/core/quality/`** — Copy from Luminarr (profiles + definitions are media-agnostic)
- **`internal/core/library/`** — Copy from Luminarr, adapt stats (series/episode counts instead of movie counts)

### sqlc Queries
- `series.sql` — CRUD + ListMonitored + GetByTMDBID
- `seasons.sql` — Create, List by series, UpdateMonitored
- `episodes.sql` — Create, List by season/series, Update, UpdateMonitored, CountMissing, ListMissing
- `libraries.sql` — Copy from Luminarr
- `quality_profiles.sql` — Copy from Luminarr

### API Endpoints
- `POST /api/v1/series/lookup` — search TMDB TV
- `POST /api/v1/series` — add series
- `GET /api/v1/series` — list (paginated, filterable)
- `GET /api/v1/series/{id}` — get with season/episode counts
- `PUT /api/v1/series/{id}` — update
- `DELETE /api/v1/series/{id}` — delete (cascade)
- `GET /api/v1/series/{id}/seasons` — list seasons
- `GET /api/v1/series/{id}/seasons/{num}/episodes` — list episodes
- `PUT /api/v1/episodes/{id}` — update (toggle monitored)
- `PUT /api/v1/seasons/{id}` — update (toggle monitored, cascades)
- Libraries + quality profiles: copy from Luminarr

### Frontend
- Copy from Luminarr: theme.ts, Shell layout (adapt nav: Series instead of Movies), all shared components
- `types/index.ts` — Series, Season, Episode, Library, QualityProfile types
- `api/series.ts` — hooks for all series/episode endpoints
- `pages/dashboard/Dashboard.tsx` — Series poster grid with episode count badges ("12/24 episodes")
- `pages/series/SeriesDetail.tsx` — Banner/poster, series info, season tabs/accordion with episode grid. Each episode row: number, title, air date, monitored toggle, file status badge.
- Settings: library list, quality profile list — copy from Luminarr

### Verification
- `make check` passes
- Add a series via API, see it listed with correct season/episode data
- Frontend shows series grid, clicking opens detail with season/episode table

---

## Phase 2: Indexers, Search, and Grab History

**Adds**: Indexer plugins, episode release search, grab history tracking.

### DB Migrations
- **`00004_indexers.sql`** — Copy indexer_configs from Luminarr
- **`00005_grab_history.sql`** — Adapted: series_id, episode_id (nullable for season packs), season_number, release info columns, grabbed_at

### Go Packages
- **`internal/core/indexer/`** — Copy from Luminarr. Pass TV categories (5000-5999) in search queries.
- **`pkg/plugin/`** — Copy from Luminarr. Add `Season`, `Episode`, `TVDBID` fields to `SearchQuery`.
- **`plugins/indexers/`** — Copy newznab + torznab from Luminarr.
- **`internal/registry/`** — Copy from Luminarr.
- **Episode search logic** in show service or new `internal/core/autosearch/`:
  - Build query string: "Series Title S01E05" (standard), "Series Title 2024-01-15" (daily)
  - Parse results: match S01E05 in release titles to episode records
  - Score against quality profile
  - Season pack detection: release has "S01" but no episode number

### API Endpoints
- Indexer CRUD + test: copy from Luminarr
- `GET /api/v1/series/{id}/releases?season=1&episode=5` — search releases for episode
- `POST /api/v1/episodes/{id}/search` — auto-search for one episode
- `POST /api/v1/series/{id}/search` — auto-search all missing monitored episodes

### Frontend
- Settings: indexer list — copy from Luminarr
- `ManualSearchModal.tsx` — adapted: release results for episode, season pack badge
- Search button per episode row in SeriesDetail

### Verification
- Add indexer, test connectivity
- Search for an episode, see quality-scored results
- Season packs detected and labeled

---

## Phase 3: Download Clients and Queue

**Adds**: Download client plugins, grab-to-client submission, queue tracking.

### DB Migrations
- **`00006_download_clients.sql`** — Copy from Luminarr
- Add `download_status`, `downloaded_bytes`, `download_client_id`, `client_item_id` columns to grab_history

### Go Packages
- **`internal/core/downloader/`** — Copy from Luminarr (media-agnostic)
- **`internal/core/queue/`** — Copy from Luminarr, adapt Item to include series_id + episode info
- **`plugins/downloaders/`** — Copy all 5 from Luminarr
- **`internal/core/downloadhandling/`** — Copy settings table from Luminarr (check interval, remove completed, etc.)

### API Endpoints
- Download client CRUD + test: copy from Luminarr
- `GET /api/v1/queue` — active downloads with series/episode info
- `DELETE /api/v1/queue/{id}` — remove from queue
- `POST /api/v1/queue/{id}/blocklist` — blocklist and retry

### Frontend
- Settings: download client list — copy from Luminarr
- `pages/queue/Queue.tsx` — adapted: series title + S01E05 instead of movie title

### Verification
- Add download client, test it
- Grab a release → submitted to client
- Queue page shows progress

---

## Phase 4: File Import, Renaming, and Media Management

**Adds**: Post-download import, season pack extraction, TV naming templates, library scanning, notifications.

### DB Migrations
- **`00007_episode_files.sql`**: episode_files (id, episode_id FK, series_id FK, path UNIQUE, size_bytes, quality_json, imported_at, indexed_at)
- **`00008_media_management.sql`**: media_management singleton with: rename_episodes, standard_episode_format, daily_episode_format, anime_episode_format, series_folder_format, season_folder_format, colon_replacement, import_extra_files, extra_file_extensions, unmonitor_deleted_episodes
- **`00009_download_handling.sql`** — Copy from Luminarr
- **`00010_notifications.sql`** — Copy from Luminarr

### Go Packages
- **`internal/core/importer/`** — New (modeled on Luminarr's). Key difference: **season pack handling**. When download has multiple video files: parse each for S01E05, match to episode, import individually, set `episodes.has_file = 1`.
- **`internal/core/renamer/`** — New. Three format templates: standard (`{Series Title} - S{Season:00}E{Episode:00} - {Episode Title} {Quality Full}`), daily (`{Series Title} - {Air Date} - {Episode Title}`), anime (includes `{Absolute Episode:000}`).
- **`internal/core/mediamanagement/`** — Copy from Luminarr, adapt settings struct for TV fields.
- **`internal/parser/`** — Copy from Luminarr. Add episode extraction: `Season`, `Episode`, `AbsoluteEpisode`, `IsSeasonPack`, `AirDate` fields to `ParsedRelease`.
- **`internal/core/notification/`** + **`internal/notifications/`** — Copy from Luminarr.
- **`plugins/notifications/`** — Copy all 9 from Luminarr.
- Library scan job: walk filesystem, parse S01E01, match to episodes.

### API Endpoints
- Media management settings: adapted for TV fields
- Download handling settings: copy from Luminarr
- Notification CRUD + test: copy from Luminarr
- `GET /api/v1/series/{id}/files` — list episode files
- `DELETE /api/v1/episodefiles/{id}` — delete file
- `POST /api/v1/series/{id}/rename` — preview/execute rename

### Frontend
- Settings: media management (3 naming format fields), notifications, download handling — adapted/copied from Luminarr
- File info per episode row in SeriesDetail (path, size, quality badge)

### Verification
- Download completes → file imported with correct naming → episode marked has_file
- Season pack (10 episodes) creates 10 episode_file records
- Library scan finds existing files
- Rename preview shows correct paths
- Notification fires on import

---

## Phase 5: RSS Sync, Calendar, and Wanted

**Adds**: Automated RSS monitoring, calendar view, missing/cutoff wanted lists.

### DB Migrations
- **`00011_quality_definitions.sql`** — Copy from Luminarr

### Go Packages
- **`internal/scheduler/`** — Copy framework from Luminarr
- **`internal/scheduler/jobs/`** — New/adapted:
  - `rss_sync.go` — Parse releases for series+S01E05, match to monitored episodes, score, grab
  - `queue_poll.go` — Copy from Luminarr
  - `library_scan.go` — Adapted for TV directory structure
  - `refresh_metadata.go` — Refresh series/episodes from TMDB TV
- **`internal/core/quality/definition.go`** — Copy from Luminarr

### API Endpoints
- `GET /api/v1/calendar?start=...&end=...` — episodes by air date range
- `GET /api/v1/wanted/missing` — monitored episodes without files (paginated)
- `GET /api/v1/wanted/cutoff` — episodes below quality cutoff
- Quality definitions: copy from Luminarr
- Tasks: copy from Luminarr
- Parse: copy from Luminarr

### Frontend
- `pages/calendar/CalendarPage.tsx` — Month/week view, episodes by air date, color-coded (green=has file, red=missing+monitored, gray=unmonitored)
- `pages/wanted/WantedPage.tsx` — Two tabs: Missing + Cutoff Unmet. Paginated table, bulk search button.
- Settings: quality definitions — copy from Luminarr

### Verification
- RSS sync grabs new episodes automatically
- Calendar shows correct air dates
- Wanted page lists accurate missing/cutoff episodes

---

## Phase 6: Blocklist, Tags, Custom Formats, and Media Servers

**Adds**: Release blocklisting, tag-based routing, custom format scoring, media server integration.

### DB Migrations
- **`00012_blocklist.sql`** — Adapted: series_id + episode_id instead of movie_id
- **`00013_tags.sql`** — Adapted: series_tags instead of movie_tags
- **`00014_custom_formats.sql`** — Copy from Luminarr
- **`00015_media_servers.sql`** — Copy from Luminarr

### Go Packages
- **`internal/core/blocklist/`** — Adapted for series/episode
- **`internal/core/tag/`** — Adapted (series_tags)
- **`internal/core/customformat/`** — Copy from Luminarr
- **`internal/core/mediaserver/`** + **`internal/mediaservers/`** — Copy from Luminarr
- **`plugins/mediaservers/`** — Copy plex, jellyfin, emby from Luminarr

### API Endpoints
- Blocklist, tags, custom formats, media servers: CRUD + test — copied/adapted from Luminarr

### Frontend
- Settings pages for each: copied/adapted from Luminarr
- Tag management in series edit modal

### Verification
- Blocklisted release excluded from future grabs
- Tags route series to specific indexers/clients
- Custom formats score correctly
- Media server notified on import

---

## Phase 7: History, Activity, Statistics, and WebSocket

**Adds**: Full observability — grab history, activity log, stats dashboard, real-time events.

### DB Migrations
- **`00016_activity_log.sql`** — Copy from Luminarr
- **`00017_stats_snapshots.sql`** — Adapted for episode-based counts

### Go Packages
- **`internal/core/stats/`** — New. Episode-based: total_series, total_episodes, monitored, with_file, missing, quality distribution, storage trends, grabs per day
- **`internal/core/activity/`** — Copy from Luminarr
- **`internal/api/ws/`** — Copy WebSocket hub from Luminarr (events carry series_id/episode_id)
- **`internal/core/health/`** — Copy from Luminarr

### API Endpoints
- History, activity, stats, health, WebSocket — adapted/copied from Luminarr

### Frontend
- `pages/history/HistoryPage.tsx` — Grab history with series/episode context
- `pages/activity/ActivityPage.tsx` — Copy from Luminarr
- `pages/stats/StatsPage.tsx` — Adapted: series/episode counts, quality pie chart, storage trends
- Enhanced dashboard: stats cards (series count, episodes on disk, missing, queue)
- `pages/settings/system/SystemPage.tsx` — Copy from Luminarr (health, tasks, logs, backup)
- `components/command-palette/` — Copy from Luminarr, adapt for series navigation
- WebSocket client for real-time UI updates

### Verification
- History shows all past grabs
- WebSocket pushes real-time events
- Stats accurate, dashboard shows at-a-glance info

---

## Phase 8: Import Lists, Sonarr Import, and Polish

**Adds**: Automated series discovery, migration from Sonarr, app settings, final polish.

### DB Migrations
- **`00018_import_lists.sql`** — Adapted for TV series import lists
- **`00019_import_exclusions.sql`** — Series-specific exclusions

### Go Packages
- **`internal/core/importlist/`** — Adapted. TV sources: Trakt trending/popular TV, TMDB popular TV, Plex watchlist (TV), custom list
- **`plugins/importlists/`** — Adapted from Luminarr (trakt_trending, trakt_popular, plex_watchlist, custom_list → TV variants)
- **`internal/sonarrimport/`** — New. Connect to Sonarr API, preview import, migrate series + monitoring state + profiles + indexers + clients

### API Endpoints
- Import list CRUD + sync + test
- `POST /api/v1/system/sonarr/preview` + `POST /api/v1/system/sonarr/import`
- App settings (theme, UI preferences)

### Frontend
- Settings: import lists, Sonarr import wizard, app settings — adapted/copied from Luminarr
- Final polish: responsive layout, loading states, error states, empty states

### Verification
- Import lists add series on schedule
- Sonarr import migrates a full library
- End-to-end: add series → RSS grabs episode → download completes → file imported → media server notified → shows in history

---

## Phase Dependencies

```
Phase 1 (Data Model + CRUD)
  └── Phase 2 (Indexers + Search)
        └── Phase 3 (Download Clients + Queue)
              └── Phase 4 (Import + Rename + Notifications)
                    ├── Phase 5 (RSS + Calendar + Wanted)  ← can parallel with 6
                    └── Phase 6 (Blocklist + Tags + CF + Media Servers)
                          └── Phase 7 (History + Activity + Stats + WS)
                                └── Phase 8 (Import Lists + Sonarr Import + Polish)
```

Phases 5 and 6 can be developed in parallel after Phase 4.
