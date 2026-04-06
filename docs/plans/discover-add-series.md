# Plan: Discover & Add Series Page for Beacon Pilot

## Context

Pilot's backend supports searching TMDB and adding TV series, but there's no UI for it. Users need a Discover page to search, preview, and add TV shows.

## Pre-implementation Fixes Required

### Fix 1: API method mismatch
The `useLookupSeries` hook sends a GET request, but the backend expects POST with a JSON body. **Fix the hook** to send POST with `{ "query": "..." }`.

### Fix 2: Lookup TypeScript types
The hook types its return as `Series[]`, but lookup results are a different shape (`LookupResult` with tmdb_id, title, year, overview, poster_path, popularity — no id, library_id, genres, network, status, etc.). **Add a `LookupResult` interface** and update the hook return type.

### Fix 3: Poster URL construction
Lookup returns raw TMDB paths like `/abc123.jpg`. **Add a helper** `tmdbPosterURL(path: string)` that prefixes `https://image.tmdb.org/t/p/w500`.

### Fix 4: Debounce threshold
Change from 2 chars to 3 chars to reduce noisy results and TMDB API quota burn.

## Design

### Page: `/discover`

**Layout:**
1. **Search bar** — prominent, auto-focused, real-time results as you type (3+ chars)
2. **Results grid** — poster cards showing only what the lookup API returns: title, year, overview, poster, popularity
3. **Already-added indicator** — dim cards for series in the library
4. **Click → Add drawer** — slide-in drawer with available metadata + add form

### Drawer content (designed around actual lookup data):
- Poster + title + year
- Overview text
- Popularity score
- **Add to Library** form:
  - Library selector (default: first library)
  - Quality profile selector (default: first profile)
  - Monitor type: All (default), Future, Missing, None, Pilot, First Season, Last Season
  - Series type: Standard (default), Daily, Anime
  - Monitored toggle (default: on)
- Add button with loading/error states

Note: genres, network, status, season count are NOT shown — the lookup API doesn't return them. They appear after adding when the full TMDB detail fetch runs.

### Already-added detection
Fetch all library TMDB IDs via a dedicated query (not paginated series list). **Add a lightweight backend endpoint** `GET /api/v1/series/tmdb-ids` that returns just the array of TMDB IDs in the library. This is O(1) on the frontend regardless of library size.

### Empty state
- **No query:** Simple prompt "Search for a TV series by name" — no trending/popular (would require new backend endpoints, out of scope)
- **No results:** "No series found for [query]"
- **No libraries:** Warning with link to Settings → Libraries
- **No quality profiles:** Warning with link to Settings → Quality Profiles

## Implementation

### New files

| File | Purpose |
|------|---------|
| `web/ui/src/pages/discover/DiscoverPage.tsx` | Page: search bar + results grid |
| `web/ui/src/pages/discover/DiscoverCard.tsx` | Result card component |
| `web/ui/src/pages/discover/AddSeriesDrawer.tsx` | Slide-in drawer with add form |
| `web/ui/src/components/Drawer.tsx` | Reusable drawer component (extracted for consistency) |

### Modified files

| File | Change |
|------|--------|
| `web/ui/src/api/series.ts` | Fix lookup hook: GET→POST, add LookupResult type, 3-char debounce |
| `web/ui/src/App.tsx` | Add `/discover` route |
| `web/ui/src/layouts/Shell.tsx` | Add "Discover" to sidebar nav |
| `web/ui/src/pages/dashboard/Dashboard.tsx` | Add "+ Add Series" button in header |
| `web/ui/src/components/command-palette/commands.ts` | Add "Discover" nav command |

### Backend changes (minimal)

| File | Change |
|------|--------|
| `internal/api/v1/series.go` | Add `GET /api/v1/series/tmdb-ids` endpoint returning `[]int` |

## Form Defaults

| Field | Default |
|-------|---------|
| Library | First available library |
| Quality Profile | First available profile |
| Monitor Type | `all` (All Episodes) |
| Series Type | `standard` |
| Monitored | `true` |

## Error Handling

- **409 Conflict** (already exists): toast "This series is already in your library" + close drawer
- **503** (TMDB not configured): toast "TMDB API key not configured" 
- **Other errors**: toast with error message from API

## Verification

1. Navigate to `/discover` or click "Discover" in sidebar
2. Type "breaking bad" (3+ chars) — results appear with posters
3. Already-added series show "In Library" badge, dimmed
4. Click a result — drawer slides in from right with overview + add form
5. Select library, quality profile, monitor type → click Add Series
6. Toast: "Breaking Bad added to library" → card shows "In Library"
7. Dashboard shows the new series
8. Command palette: type "discover" → navigates to the page
