# Pilot — Claude Code Rules

## App Name

The display name **"Pilot"** is a working title. It is centralised in
`internal/appinfo/appinfo.go` — change it there and everything (startup
banner, API responses, docs) updates automatically.

**Rename checklist** (when the name changes):
1. `internal/appinfo/appinfo.go` — `const AppName`
2. `web/ui/index.html` — `<title>` tag
3. Structural: Go module path (`go.mod`), env prefix (`PILOT_`),
   binary name (`cmd/pilot`), config dirs (`~/.config/pilot/`),
   DB filename (`pilot.db`), Makefile vars, Docker image name.

## GitHub

All `gh` commands MUST target `pilot/pilot`:

```sh
gh <command> --repo pilot/pilot
```

## Branching

**Always work on a feature branch** — never commit directly to `main`.

```sh
git checkout -b feat/my-feature
```

Merge to `main` via PR or fast-forward after work is complete and tests pass.

## Code Quality

- Run `make check` before every push (golangci-lint + tsc --noEmit).
- One logical unit per commit.
- Frontend tests: `cd web/ui && npm test` must pass before pushing frontend changes.

## Regression guard: dead-torrent release search

The "top result is a dead torrent" bug recurred enough times that we
built a full regression suite around the fix (see
`plans/dead-torrent-phase0.md` in the beacon repo root). **Run `make
test` before touching any of the files below** — the whole suite runs
in under 2 seconds and has no external dependencies:

- `internal/core/indexer/service.go` (`Search`, `seedWeight`, `applyMinSeedersFilter`)
- `internal/core/stallwatcher/service.go` (the entire file)
- `internal/core/blocklist/service.go` (any method, especially `AddFromStall`)
- `internal/api/v1/releases.go` (grab handlers, `releaseBody.FilterReasons`)
- `internal/db/queries/postgres/blocklist.sql` / `grab_history.sql` / `indexer_configs.sql`
- `internal/db/migrations/00003_blocklist_stall_columns.sql` and `00004_indexer_min_seeders.sql`
- `plugins/downloaders/haul/haul.go` (the `ListStalled` method + `StalledTorrent` struct — must match Haul's endpoint shape)
- `web/ui/src/components/ManualSearchModal.tsx` (grayed-row rendering + override)

### The dead-torrent regression suite

Every test below is in `go test ./...` (default `make test`). No build
tags, no `-short` gating.

**Ranking + filter pass** (`internal/core/indexer/service_test.go`):
- `TestSeedWeight_BucketsAndAgeCap` — locks down the exact
  `round(log10(seeds))` buckets + the 1-year age cap. Pin the contract
  so nobody accidentally reverts to a linear tiebreaker and lets fake
  seed counts dominate again.
- `TestSearchSort_ReproducesIncident` — the 847-seeders-5-years-old
  dead TGx release MUST rank below a fresh 20-seed release of equal
  quality. This is the headline regression test. If it fails, the
  original bug is back.
- `TestSearchSort_QualityPrimacyPreserved` — asserts quality still
  dominates seeds. Prevents the opposite over-correction where a
  400-seed 720p outranks a 3-seed 1080p Bluray.
- `TestSearchSort_SameQualityMoreSeedsWins` — within the same quality
  tier, more seeds still wins.
- `TestSearchSort_AgeTiebreakerOnTies` — the final tiebreaker on
  quality + seed bucket is release age (newer wins).
- `TestFilterPass_BelowMinSeedersGetsTagged` / `AboveMinSeedersUntagged`
  / `FreshMultiIndexerBypass` / `OldReleaseNoBypass` /
  `SingleIndexerFreshNoBypass` / `DefaultMinSeeders` /
  `PerIndexerDifferentThresholds` / `DoesNotCatchInflatedSeeders` —
  comprehensive coverage of the per-indexer min_seeders filter pass
  including the freshness exception (AgeDays < 0.5 && 2+ indexers).

**Stallwatcher** (`internal/core/stallwatcher/service_test.go`):
- `TestTick_BlocklistsStalledGrab` — headline: a fake Haul returning a
  stalled torrent causes the watcher to blocklist the corresponding
  grab with `reason=stall_no_peers_ever` and update grab status to
  `stalled`. End-to-end via mock querier + httptest fake Haul.
- `TestTick_SkipsNonMatchingInfoHash` — stalls for torrents not in
  grab_history are silently ignored (test torrents, direct Haul uploads).
- `TestTick_RespectsStartupGrace` — no blocklist during the first 2
  minutes of watcher uptime (absorbs Pilot-restart races).
- `TestTick_SkipsAlreadyTerminalGrabs` — idempotency: grabs already in
  a terminal state are not re-blocklisted.
- `TestTick_CircuitBreaker` — auto-re-search stops after 3 stall
  blocklist entries per (series, episode) in 24 hours. Prevents infinite
  retry loops when every release for an episode is dead.
- `TestTick_AutoSearchTriggersRetry` — the positive case: below the
  circuit breaker threshold, `TypeAutoSearchRetry` fires.
- `TestTick_InteractiveDoesNotAutoRetry` — interactive grabs only toast
  on stall, never auto-re-search. User owns the decision.
- `TestTick_NoHaulConfigured` / `TestTick_HaulUnreachable` — graceful
  degradation: no panic, no blocklist side effects.
- `TestMapStallReason` — reason-string translation is exhaustive and
  unknown reasons fall back to `stall_activity_lost` (never crash).

**Blocklist** (`internal/core/blocklist/service_test.go`):
- `TestAddFromStall_RoundTrip` — full write→read, including the
  two-keyed (guid OR info_hash) lookup for cross-indexer dedup.
- `TestAddFromStall_DuplicateIsIdempotent` — second insert returns
  `ErrAlreadyBlocklisted`, which callers treat as success.
- `TestRemoveByGUID` — the override flow removes the entry.
- `TestCountRecentStalls_PerEpisode` — the circuit breaker query
  correctly scopes by (series, episode).
- `TestAddFromStall_SetsReasonExplicitly` /
  `TestAdd_SetsUserMarkedReason` — stall entries get
  `stall_*` reasons; user entries get `user_marked`.

### Rules

- If a test fails, **fix the code, not the test**. The failure message
  names the specific file and line to check.
- Never gate these behind build tags or `-short`.
- When adding a new failure mode (e.g. a new stall reason, a new
  blocklist key, a new filter rule), extend the suite with a subtest
  in the same file. Reuse the mock querier pattern.
- The Haul side of this system has its own regression suite — see
  `haul/CLAUDE.md` for `TestSessionIntegration_DownloadFromPeer`,
  `TestCheckStalls_*`, and `TestListStalled_*`. Any change to Haul's
  `/api/v1/stalls` endpoint shape must be coordinated with Pilot's
  `stallwatcher` and `plugins/downloaders/haul/haul.go` `StalledTorrent`
  struct — the shape is an inter-service contract, not a local type.
