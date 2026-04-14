-- name: CreateNotificationConfig :one
INSERT INTO notification_configs (id, name, kind, enabled, settings, on_events, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetNotificationConfig :one
SELECT * FROM notification_configs WHERE id = $1;

-- name: ListNotificationConfigs :many
SELECT * FROM notification_configs ORDER BY name ASC;

-- name: ListEnabledNotifications :many
SELECT * FROM notification_configs WHERE enabled = TRUE ORDER BY name ASC;

-- name: UpdateNotificationConfig :one
UPDATE notification_configs SET
    name       = $1,
    kind       = $2,
    enabled    = $3,
    settings   = $4,
    on_events  = $5,
    updated_at = $6
WHERE id = $7
RETURNING *;

-- name: DeleteNotificationConfig :exec
DELETE FROM notification_configs WHERE id = $1;
