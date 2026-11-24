# Pilot: notes for Claude Code

Human contributors: see `.github/CONTRIBUTING.md`. This file is the short
version of the same rules, plus the two areas where a careless change has
caused real bugs.

## Working here

- Work on a branch and merge through a pull request. Never commit to `main`.
- `gh` commands target `Beacon-Stack/pilot`.
- `make check` before pushing: lint, dead-code checks, type-check and
  frontend tests. `make test` runs the Go tests.
- One logical change per commit. Comments say what the code does and why,
  not how it got that way; history belongs in the commit message.
- `web/ui/src/shared/` is a copy of the `web-shared` repo. Change it
  there, not here.
- SQL lives in `internal/db/queries/sqlite/`; run `make generate` after
  changing it. Don't edit `internal/db/generated/`.
- The Docker image builds with the Go version in `docker/Dockerfile`.
  After `go get`, check the `go` line in `go.mod` didn't move past it.

## Release ranking and stalled downloads

Search results are ranked by quality, then revision, then seed count in
log buckets (so inflated seed counts can't win), then age. Releases
below an indexer's minimum seeders are tagged, not dropped. A grab that
never finds peers is blocklisted by the stall watcher and searched
again, up to three times per episode per day.

The tests in `internal/core/indexer`, `internal/core/stallwatcher` and
`internal/core/blocklist` pin this behaviour. If one fails, fix the
code, not the test. The shape of Haul's `/api/v1/stalls` response is
shared with Haul (`plugins/downloaders/haul/haul.go`); change both sides
together.

## Authentication

`internal/api/middleware/auth.go` is the only place requests are
authenticated: API key, login session, or a local address under a host
name a web page can't own.

- `Sec-Fetch-Site` is a cross-site check, never a credential.
- New routes go on `protected` in `internal/api/router.go` (the huma API
  is mounted on it). The only open routes are `/health`, the three
  `/api/v1/auth/*` routes and the static UI.
- Add any new protected surface (binary endpoint, hook, stream) to
  `protectedEndpoints` in `internal/api/router_auth_test.go`.
