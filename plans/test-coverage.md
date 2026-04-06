# Test Coverage — Match Luminarr's Test Suite

## Context

Pilot has ~130 backend tests (all in plugin layer). Luminarr has 550+.
The entire core service layer, API integration, parser, and Phase 8 features
have zero coverage. Frontend has Vitest configured but zero test files.

## Priority 1: Phase 8 Tests

### `internal/core/importlist/service_test.go`

Port from Luminarr's `internal/core/importlist/service_test.go` (14 tests).
Pattern: `testutil.NewTestDBWithSQL(t)` → create quality profile + library
fixtures → test against real in-memory SQLite.

Tests to create:
- `TestCreate` — happy path, verify row returned
- `TestCreate_InvalidKind` — unknown plugin kind returns error
- `TestGet` — fetch by ID
- `TestGet_NotFound` — returns ErrNotFound
- `TestList` — returns all configs ordered by name
- `TestUpdate` — modify fields, verify merge settings
- `TestUpdate_NotFound` — returns ErrNotFound
- `TestDelete` — remove config
- `TestSync_EmptyList` — no enabled lists → zero result
- `TestSync_SkipExisting` — series already in library → skipped
- `TestSync_SkipExcluded` — excluded TMDB ID → skipped
- `TestSync_AddsSeries` — new series added via showSvc.Add
- `TestSync_ZeroTMDBID` — items with TMDbID=0 skipped
- `TestCreateExclusion_Duplicate` — unique constraint error

Requires: mock or stub for `show.Service.Add()` and `registry.Default`.

### `internal/sonarrimport/service_test.go`

Port from Luminarr's `internal/radarrimport/radarr_test.go` (10 tests).
Focus on data mapping logic (no HTTP needed).

Tests to create:
- `TestMapIndexerKind` — NewznabSettings → "newznab", TorznabSettings → "torznab", unknown → ""
- `TestMapClientKind` — QBittorrentSettings → "qbittorrent", DelugeSettings → "deluge", unknown → ""
- `TestMapProfile_LeafQualities` — single quality items extracted correctly
- `TestMapProfile_GroupQualities` — recursive group extraction
- `TestMapProfile_PlaceholderID` — ID=0 entries skipped
- `TestMapSonarrQuality` — known names map to correct resolution/source
- `TestMapSonarrQuality_Unknown` — unknown name maps to ResolutionUnknown
- `TestCutoffName` — finds quality name by cutoff ID
- `TestCutoffName_NotFound` — returns "" for missing ID
- `TestFieldHelpers` — fieldString, fieldInt, fieldBool extract correctly

### `internal/trakt/client_test.go`

Port from Luminarr's `internal/trakt/client_test.go`. Pattern:
`httptest.NewServer` → mock Trakt API responses → verify parsing.

Tests to create:
- `TestGetPopularShows` — returns parsed shows with TMDB IDs
- `TestGetTrendingShows` — unwraps `{show: ...}` envelope
- `TestGetWatchlistShows` — unwraps `{show: ...}` envelope
- `TestGetCustomListShows` — correct path construction with username/slug
- `TestGetPopularShows_Error` — non-200 returns error
- `TestHeaders` — verify trakt-api-version and trakt-api-key headers sent

### `internal/metadata/tmdbtv/client_test.go`

Port from Luminarr's `internal/metadata/tmdb/client_test.go`. Pattern:
`httptest.NewServer` → mock TMDB API responses.

Tests to create:
- `TestSearchSeries` — query and year params, response parsing
- `TestSearchSeries_NoYear` — year param omitted when zero
- `TestGetSeries` — full series detail with seasons
- `TestGetSeasonEpisodes` — episode list for season
- `TestGetPopularTV` — page param, response parsing
- `TestGetTrendingTV` — window and page params
- `TestGetSeries_NotFound` — 404 returns error
- `TestAPIKeyRedacted` — logged URL doesn't contain raw key

### `plugins/importlists/` plugin tests

Create tests for the 3 most complex plugins:

**`plugins/importlists/trakt_list_tv/plugin_test.go`**:
- `TestFactory_Valid` — watchlist config accepted
- `TestFactory_MissingUsername` — error returned
- `TestFactory_CustomMissingSlug` — error returned
- `TestFetch_Watchlist` — mock Trakt → parsed items
- `TestFetch_CustomList` — correct path used
- `TestFetch_SkipsZeroTMDB` — items without TMDB ID filtered

**`plugins/importlists/plex_watchlist_tv/plugin_test.go`**:
- `TestFactory_Valid` — access token accepted
- `TestFactory_MissingToken` — error returned
- `TestFetch_ParsesXML` — mock Plex metadata API → items
- `TestFetch_FiltersTVOnly` — type != "show" excluded
- `TestFetch_ParsesTMDBGuid` — `tmdb://123` → 123
- `TestTest_Unauthorized` — 401 → descriptive error

**`plugins/importlists/custom_list/plugin_test.go`**:
- `TestFactory_Valid` — URL accepted
- `TestFactory_MissingURL` — error returned
- `TestFetch_ParsesJSON` — mock JSON array → items
- `TestFetch_FlexibleFields` — accepts both `tmdb` and `tmdb_id`
- `TestTest_InvalidJSON` — non-array response → error
- `TestTest_NoTMDBID` — first item missing tmdb_id → error

### `internal/scheduler/jobs/import_list_sync_test.go`

Port from Luminarr's (2 tests):
- `TestImportListSync_JobMetadata` — verify Name = "import_list_sync", Interval = 6h
- `TestImportListSync_DoesNotPanic` — call Fn(ctx) with nil-safe service

## Priority 2: API Integration Tests

### `internal/api/integration_test.go`

Port from Luminarr's integration_test.go (85 tests). This is the most
impactful single test file — it tests the full HTTP stack with real SQLite.

Pattern:
```go
func newTestRouter(t *testing.T) (*httptest.Server, dbsqlite.Querier) {
    db := testutil.NewTestDB(t)
    q := dbsqlite.New(db)
    // Wire all real services with test plugins
    router := api.NewRouter(api.RouterConfig{...})
    return httptest.NewServer(router), q
}
```

Key endpoint groups to test:
- Series CRUD (POST/GET/PUT/DELETE /api/v1/series)
- Quality profiles CRUD
- Libraries CRUD
- Indexer CRUD + test
- Download client CRUD + test
- Import list CRUD + sync + preview + exclusions
- Sonarr import preview + execute
- Queue, calendar, wanted, history
- System status + health

Requires: `internal/testutil/` package with `NewTestDB(t)` helper.
Check if Pilot already has this — if not, create it (in-memory SQLite
with migrations applied).

## Priority 3: Frontend Tests

### Setup

Pilot has Vitest in package.json but may need:
- `web/ui/vitest.config.ts` or vitest section in `vite.config.ts`
- `web/ui/src/test/setup.ts` — global test setup
- `web/ui/src/test/helpers.tsx` — `renderWithProviders()` wrapper

### API Hook Tests

Create tests for the most critical hooks following Luminarr's MSW pattern:

**`web/ui/src/api/importlists.test.tsx`**:
- useImportLists returns data
- useCreateImportList invalidates cache
- useSyncAllImportLists returns result

**`web/ui/src/api/series.test.tsx`** (if hooks exist):
- useSeries, useCreateSeries, useDeleteSeries

### Component/Page Tests

**`web/ui/src/theme.test.ts`**:
- Port from Luminarr: preset validation, mode switching, localStorage
- Add: tooltip preference functions

**`web/ui/src/pages/dashboard/Dashboard.test.tsx`**:
- Loading state (skeleton), empty state, data rendered

## Priority 4: Core Service Tests

Port from Luminarr for remaining services:
- `internal/core/show/service_test.go` — CRUD, metadata refresh
- `internal/core/indexer/service_test.go` — CRUD, search
- `internal/core/downloader/service_test.go` — CRUD, test, add release
- `internal/core/notification/service_test.go` — CRUD, test, send
- `internal/core/library/service_test.go` — CRUD, stats
- `internal/core/quality/service_test.go` — CRUD, profile evaluation
- `internal/core/blocklist/service_test.go` — CRUD
- `internal/core/activity/service_test.go` — logging, filtering
- `internal/core/stats/service_test.go` — snapshot, aggregation

## Verification

```bash
go test ./... -count=1
cd web/ui && npm test
make check
```
