# Feature: Missing UI Components

**Status: DONE** (completed 2026-03-30)

## Context

Luminarr has 7 shared UI components that Pilot is missing. Some are used
across multiple pages (ErrorBoundary, QualityBadge, StatusBadge). Others
enhance specific features (DirPicker for library paths, ScoreChip for
custom formats).

## Components to Port

### 1. ErrorBoundary (~90 lines) — HIGH PRIORITY

React class component that catches render errors and shows a fallback UI
instead of crashing the entire app.

**File**: `web/ui/src/components/ErrorBoundary.tsx`

Port from Luminarr:
- `getDerivedStateFromError()` → set `hasError: true`
- `componentDidCatch(error, errorInfo)` → capture component stack
- Fallback UI: error message + "Try again" button that resets state
- Props: `children`, optional `fallback` render prop, `resetKey` for
  external reset trigger

**Integration**: Wrap main routes in `App.tsx`:
```tsx
<ErrorBoundary>
  <Routes>...</Routes>
</ErrorBoundary>
```

**Test**: `ErrorBoundary.test.tsx` — renders children normally, catches
thrown error, shows fallback, resetKey triggers recovery.

### 2. QualityBadge (~36 lines) — MEDIUM PRIORITY

Inline badge showing quality info like "1080p BluRay".

**File**: `web/ui/src/components/QualityBadge.tsx`

Port from Luminarr:
- Props: `quality` object (from release/file), or separate `source`/`resolution` strings
- Renders: `"{resolution} {source}"` in a styled `<span>`
- Accent background color, small rounded badge
- Fallback: em dash "—" if no data

Used in: episode file rows, manual search results.

### 3. StatusBadge (~60 lines) — MEDIUM PRIORITY

Color-coded badge for download/queue status.

**File**: `web/ui/src/components/StatusBadge.tsx`

Port from Luminarr:
- Maps status to color:
  - `downloading` → accent (blue)
  - `queued` → warning (yellow)
  - `completed` → success (green)
  - `paused` → muted
  - `failed` → danger (red)
  - `removed` → muted
- Capitalized label text
- Unknown statuses get fallback muted style

Used in: queue page, history page.

### 4. IndexerPill (~29 lines) — LOW PRIORITY

Small colored pill showing indexer name with deterministic color.

**File**: `web/ui/src/components/IndexerPill.tsx`

Port from Luminarr:
- Hash-based hue from indexer name (deterministic color per indexer)
- HSL: `hsl(${hue}, 60%, 55%)` text, `hsl(${hue}, 60%, 55%, 0.15)` background
- Compact inline display

Used in: manual search results modal.

### 5. ScoreChip (~106 lines) — LOW PRIORITY

Custom format score display with hover breakdown tooltip.

**File**: `web/ui/src/components/ScoreChip.tsx`

Port from Luminarr:
- Props: `ScoreBreakdown` with dimensions array
- Main display: `score/maxScore` in bordered chip
- Hover tooltip: table showing each dimension's score, max, "got" vs "want"
- Color thresholds: ≥80% green, ≥50% yellow, else red
- Absolute positioning for tooltip

Used in: manual search results, when custom formats are configured.

### 6. DirPicker (~267 lines) — MEDIUM PRIORITY

Modal directory browser for selecting library root paths.

**File**: `web/ui/src/components/DirPicker.tsx`

Port from Luminarr:
- Modal overlay with breadcrumb path display
- Lists directories from backend via `useFsBrowse(path)` hook
- Parent directory row ("..") for navigation
- "Select This Folder" and "Cancel" buttons
- Loading skeleton, error handling

**Requires API**: `GET /api/v1/system/fs?path=/some/dir` endpoint that lists
subdirectories. Check if this exists in Pilot — if not, add it.

**Hook**: `useFsBrowse(path)` in `web/ui/src/api/system.ts`

Used in: library creation/edit modal.

### 7. Poster improvements

Pilot likely has a Poster component already. Compare with Luminarr's
version (126 lines) and ensure it has:
- Image with fallback to placeholder on error
- `PosterPlaceholder` with hash-based color gradient
- Film icon and title in placeholder
- Year display option
- 2:3 aspect ratio

## Files to Create

- `web/ui/src/components/ErrorBoundary.tsx`
- `web/ui/src/components/QualityBadge.tsx`
- `web/ui/src/components/StatusBadge.tsx`
- `web/ui/src/components/IndexerPill.tsx`
- `web/ui/src/components/ScoreChip.tsx`
- `web/ui/src/components/DirPicker.tsx`

## Tests to Create (matching Luminarr)

- `web/ui/src/components/ErrorBoundary.test.tsx`
- `web/ui/src/components/IndexerPill.test.tsx`
- `web/ui/src/components/ScoreChip.test.tsx`
- `web/ui/src/components/Poster.test.tsx`

## Verification

1. `cd web/ui && npx tsc --noEmit`
2. `cd web/ui && npm test`
3. Visual spot-check each component in context
