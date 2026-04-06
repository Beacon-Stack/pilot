# Fix Branding — Remove All "Luminarr" References

**Status: DONE** (completed 2026-03-30)

## Context

47 occurrences of "Luminarr" remain in Pilot source code. These appear in
user-facing notification messages, plugin package docs, test assertions, and
stray comments. Every occurrence must say "Pilot".

## Files to Change

### Notification Plugins — User-Facing Strings (16 files)

**`plugins/notifications/discord/plugin.go`**
- Line ~59: `cfg.Username = "Luminarr"` → `"Pilot"`
- Line ~146: `Message: "Luminarr Discord test …"` → `"Pilot Discord test …"`
- Line 1-2: Package doc `// Package discord implements a Luminarr …` → `Pilot`

**`plugins/notifications/discord/plugin_test.go`**
- Line ~77: assertion `want Luminarr` → `want Pilot`

**`plugins/notifications/slack/plugin.go`**
- Line ~59: `cfg.Username = "Luminarr"` → `"Pilot"`
- Line ~116: `Title: fmt.Sprintf("[Luminarr] …")` → `[Pilot]`
- Line ~117: `Footer: "Luminarr"` → `"Pilot"`
- Line ~149: `Message: "Luminarr Slack test …"` → `"Pilot Slack test …"`
- Line 1-2: Package doc → `Pilot`

**`plugins/notifications/slack/plugin_test.go`**
- Line ~31: `Username: "LuminarrBot"` → `"PilotBot"`
- Line ~45: assertion `want LuminarrBot` → `want PilotBot`
- Line ~76: assertion `want Luminarr` → `want Pilot`

**`plugins/notifications/telegram/plugin.go`**
- Line ~95: `fmt.Sprintf("<b>[Luminarr] …")` → `[Pilot]`
- Line 1-2: Package doc → `Pilot`

**`plugins/notifications/pushover/plugin.go`**
- Line ~88: `Title: "Luminarr — "` → `"Pilot — "`
- Line 1-2: Package doc → `Pilot`

**`plugins/notifications/ntfy/plugin.go`**
- Line ~88: `"Luminarr — " + string(event.Type)` → `"Pilot — "`
- Line ~95: duplicate title reference → `"Pilot — "`
- Line ~119: `Message: "Luminarr ntfy test …"` → `"Pilot ntfy test …"`

**`plugins/notifications/gotify/plugin.go`**
- Line ~79: `Title: "Luminarr — "` → `"Pilot — "`
- Line ~103: `Title: "Luminarr Gotify test"` → `"Pilot Gotify test"`
- Line ~104: `Message: "Luminarr Gotify test …"` → `"Pilot Gotify test …"`

**`plugins/notifications/email/plugin.go`**
- Line ~95: `Subject: "[Luminarr] "` → `"[Pilot] "`
- Line ~120: `Message: "Luminarr email test …"` → `"Pilot email test …"`

**`plugins/notifications/webhook/plugin.go`**
- Line ~89: `Message: "Luminarr webhook test …"` → `"Pilot webhook test …"`
- Line 1-2: Package doc → `Pilot`

**`plugins/notifications/command/plugin.go`**
- Line 1-2: Package doc `// Package command implements a Luminarr …` → `Pilot`

### Media Server Plugins — Package Docs

**`plugins/mediaservers/plex/plugin.go`** — Package doc → `Pilot`
**`plugins/mediaservers/jellyfin/plugin.go`** — Package doc → `Pilot`
**`plugins/mediaservers/emby/plugin.go`** — Package doc → `Pilot`

### Downloader Plugins — Comments

**`plugins/downloaders/qbittorrent/qbittorrent.go`** — Comment mentioning "Luminarr's own HTTP client" → `Pilot`
**`plugins/downloaders/deluge/deluge.go`** — Comment mentioning Luminarr encoding → `Pilot`

### Other

**`internal/db/migrations/00005_remaining_configs.sql`** — Migration comment → `Pilot`
**`web/ui/src/App.tsx`** — Comment `(matches Luminarr)` → remove or update

## Approach

Use `replace_all` edits. For each file:
1. Read the file
2. Replace all "Luminarr" → "Pilot" (case-sensitive)
3. Replace "LuminarrBot" → "PilotBot" in slack test

## Verification

```bash
# Zero results expected:
grep -r "Luminarr" --include="*.go" --include="*.tsx" --include="*.ts" --include="*.sql" \
  --exclude-dir=node_modules --exclude-dir=.git .
make check
```
