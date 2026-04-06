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
