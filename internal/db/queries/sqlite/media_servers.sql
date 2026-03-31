-- name: CreateMediaServerConfig :one
INSERT INTO media_server_configs (id, name, kind, enabled, settings, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetMediaServerConfig :one
SELECT * FROM media_server_configs WHERE id = ?;

-- name: ListMediaServerConfigs :many
SELECT * FROM media_server_configs ORDER BY name ASC;

-- name: ListEnabledMediaServers :many
SELECT * FROM media_server_configs WHERE enabled = 1 ORDER BY name ASC;

-- name: UpdateMediaServerConfig :one
UPDATE media_server_configs SET
    name       = ?,
    kind       = ?,
    enabled    = ?,
    settings   = ?,
    updated_at = ?
WHERE id = ?
RETURNING *;

-- name: DeleteMediaServerConfig :exec
DELETE FROM media_server_configs WHERE id = ?;
