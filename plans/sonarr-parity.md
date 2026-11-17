# Sonarr Parity Plan

**Status: IN PROGRESS** (started 2026-11-17)

Source: the Pilot vs Sonarr gap review (main @ 78ae5b7). Each section below
is one branch and one PR, merged before the next starts. Order puts
download-correctness bugs first, then quality rules, then operations.

## Phase A: things that are wrong today

| # | Section | Branch | Status |
|---|---------|--------|--------|
| A1 | Hide the Custom Formats page (calls endpoints that don't exist) | `chore/hide-custom-formats` | Done, #39 |
| A2 | Failed-download handling for every client: blocklist + re-search | `feat/failed-download-handling` | Done, #40 |
| A3 | Upgrades replace the old episode file | `fix/upgrade-replaces-old-file` | Done, #41 |
| A4a | Parse release quality at ingestion; rank PROPER/REPACK above originals | `fix/release-quality-parsing` | Done, #42 |
| A4b | RSS sync applies the quality profile, picks the best release, upgrades and PROPERs | `feat/rss-quality-decisions` | Done, #43 |
| A5 | "Search all missing" / "Search all cutoff unmet" on the Wanted page (RSS only sees new releases; Sonarr also has no scheduled backlog search) | `feat/wanted-search-all` | |

### Found while doing Phase A

- **A2:** the stall watcher's `TypeAutoSearchRetry` event had no subscriber, so Haul stall re-search never ran. Fixed in A2.
- **A3:** `qualityFromGrab` never set `Quality.Name`, so `{Quality Full}` rendered empty in every imported filename. Fixed in A3.
- **A4a:** indexer plugins return no quality, so every search result scored 0 and ranking fell through to seeds. Fixed in A4a.
- **A4b:** RSS sync never checked the quality profile and never upgraded. Fixed in A4b.
- Still open:
  - `quality.Profile.WantRelease`/`RejectReason` are unused, use the strict quality match, and accept an *equal* quality below the cutoff (a re-grab loop if ever wired up). Replace or delete them in B1.
  - The stall watcher's retry event carries no season, so stalled **season packs** aren't re-searched (failed-download retries do carry it).
  - With renaming off, `AttachFile` records the download path as the library file; nothing is moved into the library.
  - Notifications and media servers don't preserve masked `"***"` secrets on update (indexers, clients and import lists do).
  - `internal/dbutil` is an unused duplicate of `internal/core/dbutil`.

## Phase B: release and quality rules

| # | Section | Branch | Status |
|---|---------|--------|--------|
| B1 | Quality groups in profiles (replace the resolution-only gate) | `feat/quality-groups` | |
| B2 | Release profiles: required / ignored terms | `feat/release-profiles` | |
| B3 | Delay profiles: preferred protocol + grab delay | `feat/delay-profiles` | |
| B4 | Language preference | `feat/language-preference` | |

## Phase C: operations

| # | Section | Branch | Status |
|---|---------|--------|--------|
| C1 | Health checks (disk, indexers, clients) + enforce `min_free_space_gb` | `feat/health-checks` | |
| C2 | Remote path mappings | `feat/remote-path-mappings` | |
| C3 | Scheduled backups with retention | `feat/scheduled-backups` | |
| C4 | Recycle bin for replaced/deleted files | `feat/recycle-bin` | |

## Phase D: larger features (separate design pass first)

- Scene numbering / alternate episode orders (TheXEM)
- Manual import from any folder; reviewable library import
- Custom formats backend (then restore the page from A1)
- Series bulk editor
- More download clients, notification services and import lists

## Rules for every section

- Feature branch off `main`, one logical change per commit.
- `make test`, `go vet`, `golangci-lint run`, `tsc --noEmit` and `vitest` pass before the PR.
- New behavior gets a regression test that fails on the old code.
- Merge the PR once CI is green, then update the status column here.
