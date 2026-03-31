-- name: CreateNotificationConfig :one
INSERT INTO notification_configs (id, name, kind, enabled, settings, on_events, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetNotificationConfig :one
SELECT * FROM notification_configs WHERE id = ?;

-- name: ListNotificationConfigs :many
SELECT * FROM notification_configs ORDER BY name ASC;

-- name: ListEnabledNotifications :many
SELECT * FROM notification_configs WHERE enabled = 1 ORDER BY name ASC;

-- name: UpdateNotificationConfig :one
UPDATE notification_configs SET
    name       = ?,
    kind       = ?,
    enabled    = ?,
    settings   = ?,
    on_events  = ?,
    updated_at = ?
WHERE id = ?
RETURNING *;

-- name: DeleteNotificationConfig :exec
DELETE FROM notification_configs WHERE id = ?;
