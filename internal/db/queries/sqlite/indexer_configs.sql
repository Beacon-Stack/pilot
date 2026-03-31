-- name: CreateIndexerConfig :one
INSERT INTO indexer_configs (id, name, kind, enabled, priority, settings, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetIndexerConfig :one
SELECT * FROM indexer_configs WHERE id = ?;

-- name: ListIndexerConfigs :many
SELECT * FROM indexer_configs ORDER BY priority ASC, name ASC;

-- name: ListEnabledIndexers :many
SELECT * FROM indexer_configs WHERE enabled = 1 ORDER BY priority ASC, name ASC;

-- name: UpdateIndexerConfig :one
UPDATE indexer_configs SET
    name       = ?,
    kind       = ?,
    enabled    = ?,
    priority   = ?,
    settings   = ?,
    updated_at = ?
WHERE id = ?
RETURNING *;

-- name: DeleteIndexerConfig :exec
DELETE FROM indexer_configs WHERE id = ?;
