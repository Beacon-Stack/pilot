-- name: CreateMediaServerConfig :one
INSERT INTO media_server_configs (id, name, kind, enabled, settings, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetMediaServerConfig :one
SELECT * FROM media_server_configs WHERE id = $1;

-- name: ListMediaServerConfigs :many
SELECT * FROM media_server_configs ORDER BY name ASC;

-- name: ListEnabledMediaServers :many
SELECT * FROM media_server_configs WHERE enabled = TRUE ORDER BY name ASC;

-- name: UpdateMediaServerConfig :one
UPDATE media_server_configs SET
    name       = $1,
    kind       = $2,
    enabled    = $3,
    settings   = $4,
    updated_at = $5
WHERE id = $6
RETURNING *;

-- name: DeleteMediaServerConfig :exec
DELETE FROM media_server_configs WHERE id = $1;
