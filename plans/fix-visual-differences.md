# Fix Visual Differences — AppSettings, Themes, Mode Picker

**Status: DONE** (completed 2026-03-30)

## Context

Pilot's AppSettings page and theme system are simplified compared to
Luminarr. The color mode picker is a plain `<select>` instead of icon buttons,
5 theme presets are missing, dark/light preset tracking is absent, and the
tooltip toggle was removed.

## Changes

### 1. Port AppSettings Page from Luminarr

**File**: `web/ui/src/pages/settings/app/AppSettings.tsx` (currently 193 lines → ~500+ lines)

Replace with Luminarr's `AppSettingsPage.tsx` layout, adapted for Pilot:

**Color Mode Picker** — 3 icon buttons instead of `<select>`:
- Monitor icon → "System"
- Moon icon → "Dark"
- Sun icon → "Light"
- Active state: accent border + muted accent background
- Use `lucide-react` icons: `Monitor`, `Moon`, `Sun`

**Theme Preset Grid**:
- Show only presets matching current resolved mode (dark presets when dark, light when light)
- Each swatch: ~80px wide, shows 3-color strip (bg, accent, border from preset)
- Label below each swatch
- Selected state: accent ring + checkmark overlay
- Grid: `display: grid; grid-template-columns: repeat(auto-fill, minmax(80px, 1fr))`

**UI Preferences Section** (new):
- "Show Tooltips" toggle switch
- Uses `getTooltipsEnabled()` / `setTooltipsEnabled()` from theme.ts

**API Key Section** (new):
- Display the Pilot API key with show/hide toggle
- Copy-to-clipboard button
- Read from `/api/v1/system/status` (already available)

**Key references**:
- Luminarr source: `/home/davidfic/dev/luminarr/luminarr/web/ui/src/pages/settings/app/AppSettingsPage.tsx` (891 lines)
- Pilot current: `web/ui/src/pages/settings/app/AppSettings.tsx` (193 lines)

### 2. Add Missing Theme Presets

**File**: `web/ui/src/theme.ts`

Add these 5 presets (copy color values from Luminarr's theme.ts):

**Dark presets** (add to `darkPresets` array):
- `gruvbox-dark` — Gruvbox Dark (warm retro)
- `one-dark` — One Dark (Atom editor)
- `rose-pine` — Rosé Pine (muted pastels)
- `kanagawa` — Kanagawa (Japanese ink)

**Light presets** (add to `lightPresets` array):
- `gruvbox-light` — Gruvbox Light

Each preset is an object with `id`, `name`, and 16 CSS custom property values.
Copy exact hex values from Luminarr's theme.ts.

### 3. Add Separate Dark/Light Preset Tracking

**File**: `web/ui/src/theme.ts`

Pilot already has `pilot-theme-dark` and `pilot-theme-light` storage
keys. Verify that `setThemePreset(resolvedMode, presetId)` writes to the
correct mode-specific key and that `getStoredPreset(mode)` reads back the
right one. The AppSettings page should pass the resolved mode when calling
`setThemePreset`.

### 4. Add Tooltip Preference Functions

**File**: `web/ui/src/theme.ts`

Add at the end:
```typescript
const TOOLTIPS_KEY = "pilot-ui-tooltips";

export function getTooltipsEnabled(): boolean {
  return localStorage.getItem(TOOLTIPS_KEY) !== "false";
}

export function setTooltipsEnabled(enabled: boolean): void {
  localStorage.setItem(TOOLTIPS_KEY, String(enabled));
}
```

## Verification

1. `cd web/ui && npx tsc --noEmit` — no type errors
2. `cd web/ui && npm test` — if tests exist, they pass
3. Visual: open Settings → App, verify:
   - 3 icon buttons for dark/light/system
   - Preset grid shows all presets for current mode
   - Switching mode remembers separate preset per mode
   - Tooltip toggle works
   - API key section shows key with copy button
