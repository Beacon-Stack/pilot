# Plan: Season & Episode Display Redesign

## Context

Pilot has a series detail page with basic season tabs and episode tables, but it's minimal — no quality info shown, no status indicators, no filtering, limited actions. Sonarr's approach works but has well-documented pain points: confusing monitor cascade, unclear status colors, no episode filtering, and limited bulk actions.

This plan designs the season/episode experience from scratch, keeping what works and fixing what doesn't.

## Sonarr's Key Problems We're Solving

1. **Monitor cascade destroys state** — toggling a season off then back on re-monitors ALL episodes, losing per-episode customization
2. **Status colors are cryptic** — dark vs light quality badges, progress bars that show nothing when 0%, no text labels
3. **No episode filtering** — can't show "just missing" or "just downloaded" within a season
4. **No real bulk select** — only shift-click for monitor toggles, no checkbox multi-select
5. **Everything on one scroll** — 10 seasons × 24 episodes = massive page with no way to focus

## Design Principles

- **Text over color** — every status has a readable label, not just a color
- **Filter, don't scroll** — filter chips at the season level to show exactly what you need
- **Non-destructive monitoring** — season monitor toggle doesn't reset per-episode choices
- **Progressive disclosure** — show the essential info by default, expand for details
- **Actions where you need them** — episode actions inline, not buried in menus

## Layout

### Series Header (existing — minor enhancements)
Keep the current hero banner with poster, title, metadata. Add:
- Episode progress summary: "142/156 episodes · 312 GB" with a thin progress bar
- Quality breakdown mini-badge: "84 HD · 52 FHD · 6 UHD"

### Season Navigation: Horizontal scrolling pills (not tabs)

```
[All Seasons] [Season 1 ✓] [Season 2 ●] [Season 3 ○] [Season 4 ◐] [Specials]
```

Each pill shows a status dot:
- ✓ Green = all episodes downloaded
- ● Blue = downloading
- ◐ Yellow = partially downloaded
- ○ Gray = nothing downloaded
- No dot = unaired season

Clicking a season shows its episode list below. "All Seasons" shows a collapsed overview of every season.

### Season Header (when a season is selected)

```
Season 3 · 12 episodes · 8 downloaded · 24.6 GB
[Monitored ✓] [Search Missing] [Filter: All ▾]
```

- Progress bar: visual fill showing download completion
- Monitor toggle: affects only the season flag, NOT individual episodes
- Search Missing: one-click search for all monitored+missing episodes in this season
- Filter dropdown: All | Downloaded | Missing | Unaired | Unmonitored

### Episode List

Not a table — a **card list** with progressive disclosure. Each episode is a compact row that expands on click.

**Collapsed row (default):**
```
┌─────────────────────────────────────────────────────────────────────┐
│ ☑ │ 3x05 │ The Door │ Mar 22, 2024 │ ██ 1080p HEVC · 1.4 GB │ ⋯ │
└─────────────────────────────────────────────────────────────────────┘
```

- Checkbox (for bulk select)
- Episode number (SxxExx format)
- Title
- Air date (relative for recent: "2 days ago", absolute for older)
- Quality badge + size (if downloaded) OR status label:
  - `Missing` (red text) — aired but no file
  - `Unaired · Apr 15` (muted) — future episode with date
  - `Unaired · TBA` (muted) — no air date
  - `Downloading 45%` (blue) — in queue
  - `1080p HEVC · 1.4 GB` (green-ish) — downloaded with quality info
- Three-dot menu (⋯) for episode actions

**Expanded row (click to expand):**
```
┌─────────────────────────────────────────────────────────────────────┐
│ ☑ │ 3x05 │ The Door │ Mar 22, 2024 │ ██ 1080p HEVC · 1.4 GB │ ⋯ │
├─────────────────────────────────────────────────────────────────────┤
│ Cersei makes a move. Arya faces challenges. Jon and Sansa         │
│ prepare. Bran discovers the truth.                                 │
│                                                                     │
│ File: /tv/Game.of.Thrones.S03E05.1080p.BluRay.x265-GROUP.mkv     │
│ Quality: Bluray-1080p · HEVC x265 · 1.4 GB                        │
│ Imported: Jan 15, 2024                                              │
│                                                                     │
│ [Search] [Manual Search] [Delete File]                              │
└─────────────────────────────────────────────────────────────────────┘
```

Shows: overview text, file path, quality details (source+resolution+codec), import date, and action buttons.

### Bulk Actions Bar

When checkboxes are selected, a floating bar appears at the bottom:

```
┌─────────────────────────────────────────────────────────────────────┐
│ 5 episodes selected │ [Monitor] [Unmonitor] [Search] [Delete Files] │ Clear │
└─────────────────────────────────────────────────────────────────────┘
```

### "All Seasons" View

When "All Seasons" pill is selected, show a compact overview:

```
Season 5 · 10 episodes · 10/10 · 28.4 GB     [Monitored ✓]
Season 4 · 10 episodes · 8/10 · 22.1 GB      [Monitored ✓]
Season 3 · 12 episodes · 0/12                 [Monitored ○]
Season 2 · 10 episodes · 10/10 · 18.9 GB     [Monitored ✓]
Season 1 · 8 episodes · 8/8 · 14.2 GB        [Monitored ✓]
Specials · 3 episodes · 1/3 · 2.1 GB         [Monitored ○]
```

Click any season row to navigate to it. This replaces Sonarr's accordion pattern — no more scrolling through collapsed sections.

## Monitoring Model: Non-Destructive

**The key difference from Sonarr:**

- Season monitor toggle: sets a flag on the season. Does NOT change individual episode monitor states.
- An episode is considered "effectively monitored" if BOTH the season AND the episode are monitored.
- Toggling the season off = all episodes in that season stop being searched, but their individual flags are preserved.
- Toggling the season back on = episodes return to their previous per-episode state.

This eliminates Sonarr's #1 pain point.

## Filter System

Filter chips above the episode list:

```
[All] [Downloaded] [Missing] [Unaired] [Unmonitored]
```

- **All**: show everything (default)
- **Downloaded**: only episodes with files
- **Missing**: aired + monitored + no file (the actionable ones)
- **Unaired**: future episodes
- **Unmonitored**: episodes where monitored=false

Filters are per-season and remembered during the session.

## Implementation

### Phase 1: Season pills + episode card list with status

**Modified files:**
| File | Change |
|------|--------|
| `web/ui/src/pages/series/SeriesDetail.tsx` | Replace season tabs with pills, rebuild episode display |

**New components to extract:**
| File | Purpose |
|------|---------|
| `web/ui/src/pages/series/SeasonPills.tsx` | Horizontal scrolling season selector with status dots |
| `web/ui/src/pages/series/EpisodeRow.tsx` | Collapsed/expandable episode row with quality badge |
| `web/ui/src/pages/series/SeasonHeader.tsx` | Season stats bar with monitor toggle + search + filter |
| `web/ui/src/pages/series/AllSeasonsView.tsx` | Compact season overview grid |

### Phase 2: Episode filtering + bulk actions

**New components:**
| File | Purpose |
|------|---------|
| `web/ui/src/pages/series/EpisodeFilter.tsx` | Filter chips component |
| `web/ui/src/pages/series/BulkActionBar.tsx` | Floating bottom bar for bulk operations |

### Phase 3: Non-destructive monitoring

**Backend change:**
| File | Change |
|------|--------|
| `internal/core/show/service.go` | Season monitor toggle no longer cascades to episodes |

### No new API endpoints needed

All data (episodes, files, quality) is already available. The redesign is primarily frontend.

## Status Badge Design

| State | Badge | Color |
|-------|-------|-------|
| Downloaded, cutoff met | `1080p HEVC · 1.4 GB` | Green bg |
| Downloaded, upgradeable | `720p x264 · 890 MB ↑` | Yellow bg, upgrade arrow |
| Downloading | `Downloading 45%` with progress bar | Blue |
| Missing (monitored) | `Missing` | Red text |
| Unaired with date | `Apr 15, 2025` | Muted text |
| Unaired TBA | `TBA` | Muted text |
| Unmonitored, no file | `Unmonitored` | Gray text |
| Unmonitored, has file | `1080p · Unmonitored` | Gray bg |

Every state has a text label. No relying on color alone.

## Verification

1. Open a series with multiple seasons → season pills show correct status dots
2. Click Season 3 → episode list loads with quality badges
3. Click "Missing" filter → only missing episodes shown
4. Select 3 episodes via checkboxes → bulk action bar appears
5. Click "Search" in bulk bar → search triggered for selected episodes
6. Expand an episode → overview, file path, quality details visible
7. Toggle season monitor off → episodes stop searching but keep their individual flags
8. Toggle season monitor back on → same episodes are monitored as before (non-destructive)
9. Click "All Seasons" → compact overview of every season with progress
