-- name: SetSetting :exec
INSERT INTO settings (key, value, updated_at)
VALUES ($1, $2, $3)
ON CONFLICT (key) DO UPDATE SET
    value      = excluded.value,
    updated_at = excluded.updated_at;

-- name: GetSetting :one
SELECT value FROM settings WHERE key = $1;

-- name: DeleteSetting :exec
DELETE FROM settings WHERE key = $1;
