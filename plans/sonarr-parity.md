# Sonarr Parity Plan

**Status: IN PROGRESS** (started 2026-11-17)

Source: the Pilot vs Sonarr gap review (main @ 78ae5b7). Each section below
is one branch and one PR, merged before the next starts. Order puts
download-correctness bugs first, then quality rules, then operations.

## Phase A: things that are wrong today

| # | Section | Branch | Status |
|---|---------|--------|--------|
| A1 | Hide the Custom Formats page (calls endpoints that don't exist) | `chore/hide-custom-formats` | |
| A2 | Failed-download handling for every client: blocklist + re-search | `feat/failed-download-handling` | |
| A3 | Upgrades replace the old episode file | `fix/upgrade-replaces-old-file` | |
| A4 | Prefer and upgrade to PROPER/REPACK releases | `feat/propers-repacks` | |

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
- Series bulk editor, "search all missing/cutoff unmet"
- More download clients, notification services and import lists

## Rules for every section

- Feature branch off `main`, one logical change per commit.
- `make test`, `go vet`, `golangci-lint run`, `tsc --noEmit` and `vitest` pass before the PR.
- New behavior gets a regression test that fails on the old code.
- Merge the PR once CI is green, then update the status column here.
