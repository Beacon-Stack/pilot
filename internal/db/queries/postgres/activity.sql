-- name: InsertActivity :exec
INSERT INTO activity_log (id, type, category, series_id, title, detail, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListActivities :many
-- The ::text casts are required for Postgres to plan a parameterised
-- "($1 IS NULL OR col = $1)" — without them the planner can't infer the
-- type from `$1 IS NULL` alone and fails with SQLSTATE 42P08.
SELECT * FROM activity_log
WHERE (sqlc.narg('category')::text IS NULL OR category = sqlc.narg('category')::text)
  AND (sqlc.narg('since')::text    IS NULL OR created_at > sqlc.narg('since')::text)
ORDER BY created_at DESC
LIMIT sqlc.arg('limit');

-- name: CountActivities :one
SELECT COUNT(*) FROM activity_log
WHERE (sqlc.narg('category')::text IS NULL OR category = sqlc.narg('category')::text)
  AND (sqlc.narg('since')::text    IS NULL OR created_at > sqlc.narg('since')::text);

-- name: PruneActivities :exec
DELETE FROM activity_log WHERE created_at < $1;
