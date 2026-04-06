# Feature: Backup & Restore

**Status: DONE** (completed 2026-03-30)

## Context

Luminarr has backup/restore functionality (218 lines backend + frontend
section). Users can download the SQLite database as a backup file and upload
a previously saved backup to restore. Pilot has none of this — users have
no way to protect their data.

## Backend

### `internal/api/v1/backup.go` (~130 lines)

Port from Luminarr's backup.go:

**`GET /api/v1/system/backup`** — BackupHandler:
- Execute `VACUUM INTO ?` with a temp file path to create a consistent,
  defragmented copy of the SQLite database
- Stream the file as `application/octet-stream`
- Set `Content-Disposition: attachment; filename="pilot-backup-YYYY-MM-DD.db"`
- Clean up temp file after streaming
- Requires `dbPath` to be passed in (from RouterConfig)

**`POST /api/v1/system/restore`** — RestoreHandler:
- Accept raw binary upload (`application/octet-stream`), max 500 MiB
- Validate SQLite magic header (first 16 bytes = `SQLite format 3\000`)
- Write to staging path `{dbPath}.restore`
- On next app startup, `cmd/pilot/main.go` already has the staging
  swap logic (check `{dbPath}.restore` exists → rename over `{dbPath}`)
- Return JSON: `{"message":"Restore file saved. Restart Pilot to complete the restore."}`

### `internal/api/v1/backup_test.go` (~87 lines)

Port from Luminarr:
- `TestRestore_ValidDB` — valid SQLite header accepted, staging file created
- `TestRestore_InvalidHeader` — random bytes → 400 error
- `TestRestore_TooSmall` — file < 16 bytes → 400 error
- `TestBackup_ReturnsDB` — GET returns valid SQLite file

### Router Registration

**`internal/api/router.go`** — Add backup/restore routes:
```go
if cfg.DBPath != "" && cfg.DBType == "sqlite" {
    v1.RegisterBackupRoutes(humaAPI, cfg.DBPath, database.SQL)
}
```

Note: These routes must be registered outside the huma JSON middleware since
they deal with raw binary streams. Register them directly on the chi router,
similar to how the WebSocket endpoint is registered.

## Frontend

### `web/ui/src/pages/settings/app/AppSettings.tsx`

Add a "Backup & Restore" section at the bottom of the page:

**Download Backup** button:
```typescript
<button onClick={() => window.location.href = "/api/v1/system/backup"}>
  Download Backup
</button>
```

**Restore** — hidden file input (accept=".db"):
- On file select, upload via `fetch("/api/v1/system/restore", { method: "POST", body: file })`
- Show success message: "Restore staged — restart Pilot to apply the backup."
- Show error on failure

Style to match existing settings sections.

## Files to Create

- `internal/api/v1/backup.go`
- `internal/api/v1/backup_test.go`

## Files to Modify

- `internal/api/router.go` — add RegisterBackupRoutes call
- `web/ui/src/pages/settings/app/AppSettings.tsx` — add Backup & Restore section

## Verification

1. `make check` passes
2. `go test ./internal/api/v1/ -run TestBackup -v` passes
3. Manual: Settings → App → Download Backup → file downloads
4. Manual: Upload the downloaded file → "Restart to apply" message
