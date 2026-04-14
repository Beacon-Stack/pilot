-- name: CreateIndexerConfig :one
INSERT INTO indexer_configs (id, name, kind, enabled, priority, settings, min_seeders, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetIndexerConfig :one
SELECT * FROM indexer_configs WHERE id = $1;

-- name: ListIndexerConfigs :many
SELECT * FROM indexer_configs ORDER BY priority ASC, name ASC;

-- name: ListEnabledIndexers :many
SELECT * FROM indexer_configs WHERE enabled = TRUE ORDER BY priority ASC, name ASC;

-- name: UpdateIndexerConfig :one
UPDATE indexer_configs SET
    name        = $1,
    kind        = $2,
    enabled     = $3,
    priority    = $4,
    settings    = $5,
    min_seeders = $6,
    updated_at  = $7
WHERE id = $8
RETURNING *;

-- name: DeleteIndexerConfig :exec
DELETE FROM indexer_configs WHERE id = $1;
