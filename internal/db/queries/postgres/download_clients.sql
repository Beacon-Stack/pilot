-- name: CreateDownloadClientConfig :one
INSERT INTO download_client_configs (id, name, kind, enabled, priority, settings, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetDownloadClientConfig :one
SELECT * FROM download_client_configs WHERE id = $1;

-- name: ListDownloadClientConfigs :many
SELECT * FROM download_client_configs ORDER BY priority ASC, name ASC;

-- name: ListEnabledDownloadClients :many
SELECT * FROM download_client_configs WHERE enabled = TRUE ORDER BY priority ASC, name ASC;

-- name: UpdateDownloadClientConfig :one
UPDATE download_client_configs SET
    name       = $1,
    kind       = $2,
    enabled    = $3,
    priority   = $4,
    settings   = $5,
    updated_at = $6
WHERE id = $7
RETURNING *;

-- name: DeleteDownloadClientConfig :exec
DELETE FROM download_client_configs WHERE id = $1;
