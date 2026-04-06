# Feature: Command Palette (Cmd/Ctrl+K)

**Status: DONE** (completed 2026-03-30)

## Context

Luminarr has a full command palette (~1,340 lines across 6 files) for
keyboard-driven navigation, series search, and action triggering. Pilot
has no equivalent. This is a high-visibility UX feature.

## Files to Create

### `web/ui/src/components/command-palette/useCommandPalette.ts` (~22 lines)

Simple hook managing open/close state:
```typescript
export function useCommandPalette() {
  const [open, setOpen] = useState(false);
  return { open, setOpen };
}
```

Exported via context or prop drilling from Shell.

### `web/ui/src/components/command-palette/commands.ts` (~145 lines)

Defines command types and navigation entries. Adapt from Luminarr:

**Types**:
```typescript
interface Command {
  id: string;
  label: string;
  group: "page" | "series" | "action";
  icon?: ComponentType;
  action: (navigate: NavigateFunction) => void;
}
```

**Navigation commands** (~20, adapted for TV):
- Dashboard, Activity, Calendar, Wanted, Statistics, Queue, History
- Settings: Libraries, Media Management, Quality Profiles, Quality Definitions,
  Indexers, Download Clients, Notifications, Media Servers, Import Lists,
  Blocklist, Import, System, App Settings

**Action commands** (3):
- Run RSS Sync → `POST /api/v1/tasks/rss_sync/run`
- Scan All Libraries → `POST /api/v1/tasks/library_scan/run`
- Refresh All Metadata → `POST /api/v1/tasks/refresh_metadata/run`

**Fuzzy match scoring** (port from Luminarr):
- Consecutive character matches get bonus
- Word-boundary matches get +10 bonus
- Case-insensitive comparison

### `web/ui/src/components/command-palette/CommandPalette.tsx` (~700 lines)

The main modal component. Port from Luminarr with these adaptations:

**Layout**:
- Fixed overlay with centered modal (max-width ~600px)
- Search input at top with magnifying glass icon
- Scrollable results list grouped by category
- Keyboard navigation (arrow keys, Enter, Escape)

**Series Search** (replacing movie search):
- Debounced 300ms, triggers for queries ≥ 2 chars
- Calls `useLookupSeries()` → `POST /api/v1/series/lookup`
- Shows poster thumbnail, title, year, "In Library" badge
- Clicking navigates to series detail or adds series

**Groups display**: "Pages", "Series", "Actions" with uppercase headers

**Styling**: Use same CSS variable vocabulary as rest of app.

### `web/ui/src/components/command-palette/index.ts`

Re-export CommandPalette and useCommandPalette.

## Files to Modify

### `web/ui/src/layouts/Shell.tsx`

- Add `useCommandPalette()` state
- Render `<CommandPalette open={open} onClose={...} />`
- Add global keydown listener for Cmd+K / Ctrl+K

### `web/ui/src/api/series.ts` (if not already present)

Ensure `useLookupSeries()` hook exists for TMDB search.

## Tests to Create

### `web/ui/src/components/command-palette/commands.test.ts` (~137 lines)

Port from Luminarr:
- All navigation commands have unique IDs
- All commands have non-empty labels
- Fuzzy match: exact match scores highest
- Fuzzy match: partial match scores lower
- Fuzzy match: no match returns 0

### `web/ui/src/components/command-palette/useCommandPalette.test.ts` (~65 lines)

- Default state is closed
- setOpen(true) opens
- setOpen(false) closes

### `web/ui/src/components/command-palette/CommandPalette.test.tsx` (~210 lines)

- Renders when open
- Doesn't render when closed
- Escape closes
- Input focuses on open
- Keyboard navigation works

## Verification

1. `cd web/ui && npx tsc --noEmit`
2. `cd web/ui && npm test`
3. Visual: press Cmd+K → palette opens, type "cal" → Calendar page highlighted,
   press Enter → navigates to Calendar
