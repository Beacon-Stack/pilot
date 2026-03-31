# Fix Branding — Remove All "Luminarr" References

**Status: DONE** (completed 2026-03-30)

## Context

47 occurrences of "Luminarr" remain in Screenarr source code. These appear in
user-facing notification messages, plugin package docs, test assertions, and
stray comments. Every occurrence must say "Screenarr".

## Files to Change

### Notification Plugins — User-Facing Strings (16 files)

**`plugins/notifications/discord/plugin.go`**
- Line ~59: `cfg.Username = "Luminarr"` → `"Screenarr"`
- Line ~146: `Message: "Luminarr Discord test …"` → `"Screenarr Discord test …"`
- Line 1-2: Package doc `// Package discord implements a Luminarr …` → `Screenarr`

**`plugins/notifications/discord/plugin_test.go`**
- Line ~77: assertion `want Luminarr` → `want Screenarr`

**`plugins/notifications/slack/plugin.go`**
- Line ~59: `cfg.Username = "Luminarr"` → `"Screenarr"`
- Line ~116: `Title: fmt.Sprintf("[Luminarr] …")` → `[Screenarr]`
- Line ~117: `Footer: "Luminarr"` → `"Screenarr"`
- Line ~149: `Message: "Luminarr Slack test …"` → `"Screenarr Slack test …"`
- Line 1-2: Package doc → `Screenarr`

**`plugins/notifications/slack/plugin_test.go`**
- Line ~31: `Username: "LuminarrBot"` → `"ScreenarrBot"`
- Line ~45: assertion `want LuminarrBot` → `want ScreenarrBot`
- Line ~76: assertion `want Luminarr` → `want Screenarr`

**`plugins/notifications/telegram/plugin.go`**
- Line ~95: `fmt.Sprintf("<b>[Luminarr] …")` → `[Screenarr]`
- Line 1-2: Package doc → `Screenarr`

**`plugins/notifications/pushover/plugin.go`**
- Line ~88: `Title: "Luminarr — "` → `"Screenarr — "`
- Line 1-2: Package doc → `Screenarr`

**`plugins/notifications/ntfy/plugin.go`**
- Line ~88: `"Luminarr — " + string(event.Type)` → `"Screenarr — "`
- Line ~95: duplicate title reference → `"Screenarr — "`
- Line ~119: `Message: "Luminarr ntfy test …"` → `"Screenarr ntfy test …"`

**`plugins/notifications/gotify/plugin.go`**
- Line ~79: `Title: "Luminarr — "` → `"Screenarr — "`
- Line ~103: `Title: "Luminarr Gotify test"` → `"Screenarr Gotify test"`
- Line ~104: `Message: "Luminarr Gotify test …"` → `"Screenarr Gotify test …"`

**`plugins/notifications/email/plugin.go`**
- Line ~95: `Subject: "[Luminarr] "` → `"[Screenarr] "`
- Line ~120: `Message: "Luminarr email test …"` → `"Screenarr email test …"`

**`plugins/notifications/webhook/plugin.go`**
- Line ~89: `Message: "Luminarr webhook test …"` → `"Screenarr webhook test …"`
- Line 1-2: Package doc → `Screenarr`

**`plugins/notifications/command/plugin.go`**
- Line 1-2: Package doc `// Package command implements a Luminarr …` → `Screenarr`

### Media Server Plugins — Package Docs

**`plugins/mediaservers/plex/plugin.go`** — Package doc → `Screenarr`
**`plugins/mediaservers/jellyfin/plugin.go`** — Package doc → `Screenarr`
**`plugins/mediaservers/emby/plugin.go`** — Package doc → `Screenarr`

### Downloader Plugins — Comments

**`plugins/downloaders/qbittorrent/qbittorrent.go`** — Comment mentioning "Luminarr's own HTTP client" → `Screenarr`
**`plugins/downloaders/deluge/deluge.go`** — Comment mentioning Luminarr encoding → `Screenarr`

### Other

**`internal/db/migrations/00005_remaining_configs.sql`** — Migration comment → `Screenarr`
**`web/ui/src/App.tsx`** — Comment `(matches Luminarr)` → remove or update

## Approach

Use `replace_all` edits. For each file:
1. Read the file
2. Replace all "Luminarr" → "Screenarr" (case-sensitive)
3. Replace "LuminarrBot" → "ScreenarrBot" in slack test

## Verification

```bash
# Zero results expected:
grep -r "Luminarr" --include="*.go" --include="*.tsx" --include="*.ts" --include="*.sql" \
  --exclude-dir=node_modules --exclude-dir=.git .
make check
```
