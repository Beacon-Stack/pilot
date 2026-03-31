# Contributing to Screenarr

Thanks for your interest. Here's what you need to know before opening a PR.

## Before you start

- For bug fixes or small improvements, open an issue first so we can discuss whether the fix is in scope and how it should work.
- For new features, open a feature request issue and wait for a response before writing code. We don't want you to spend time on something that won't be merged.
- Check existing issues and PRs. Something might already be in progress.

## Development setup

The short version:

```bash
git clone https://github.com/screenarr/screenarr
cd screenarr
go build ./...              # confirm Go build is clean
cd web/ui && npm ci && npm run build  # confirm frontend build is clean
```

For hot reload during development:

```bash
# Backend (requires air: go install github.com/air-verse/air@latest)
make dev

# Frontend
cd web/ui && npm run dev
```

## Code standards

**Go:**
- `go build ./...` must pass with zero errors
- `go test ./...` must pass (run with `-race` for concurrency-sensitive code)
- Follow existing patterns. Read similar files before writing new ones
- New service methods need corresponding tests
- Security-sensitive code (auth, credentials, external HTTP) warrants extra attention and a comment

**TypeScript / React:**
- `npx tsc --noEmit` must pass with zero TypeScript errors
- `npm test` must pass
- No `any` types without a comment explaining why
- All inline styles use CSS variables (`var(--color-*)`). No hardcoded colours
- Hover effects via `onMouseEnter`/`onMouseLeave`, matching the existing Shell.tsx pattern
- Every loading state gets a skeleton, not a spinner
- Every error state is handled explicitly. Never silently swallowed

**General:**
- Keep changes scoped. Fix the stated problem; don't refactor surrounding code unless it's directly related.
- No feature flags or backwards-compatibility shims. Just change the code.
- Add comments where logic isn't self-evident. Skip comments that restate what the code says.

## Pull requests

- Branch from `main`
- One logical change per PR
- Include a clear description of what changed and why
- Reference the issue number if there is one (`Fixes #123`)
- Make sure both `go build ./...` and `cd web/ui && npx tsc --noEmit` pass before submitting

## Questions

Open a discussion or an issue. We're happy to help you understand the codebase.
