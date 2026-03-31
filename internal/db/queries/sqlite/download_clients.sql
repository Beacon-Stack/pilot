-- name: CreateDownloadClientConfig :one
INSERT INTO download_client_configs (id, name, kind, enabled, priority, settings, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetDownloadClientConfig :one
SELECT * FROM download_client_configs WHERE id = ?;

-- name: ListDownloadClientConfigs :many
SELECT * FROM download_client_configs ORDER BY priority ASC, name ASC;

-- name: ListEnabledDownloadClients :many
SELECT * FROM download_client_configs WHERE enabled = 1 ORDER BY priority ASC, name ASC;

-- name: UpdateDownloadClientConfig :one
UPDATE download_client_configs SET
    name       = ?,
    kind       = ?,
    enabled    = ?,
    priority   = ?,
    settings   = ?,
    updated_at = ?
WHERE id = ?
RETURNING *;

-- name: DeleteDownloadClientConfig :exec
DELETE FROM download_client_configs WHERE id = ?;
