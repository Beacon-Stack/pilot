-- name: CreateQualityProfile :one
INSERT INTO quality_profiles (
    id, name, cutoff_json, qualities_json,
    upgrade_allowed, upgrade_until_json, created_at, updated_at,
    min_custom_format_score, upgrade_until_cf_score, managed_by_pulse
) VALUES (
    ?, ?, ?, ?,
    ?, ?, ?, ?,
    ?, ?, ?
)
RETURNING *;

-- name: GetQualityProfile :one
SELECT * FROM quality_profiles WHERE id = ?;

-- name: ListQualityProfiles :many
SELECT * FROM quality_profiles ORDER BY name ASC;

-- name: ListManagedQualityProfiles :many
SELECT * FROM quality_profiles WHERE managed_by_pulse = TRUE ORDER BY name ASC;

-- name: UpdateQualityProfile :one
UPDATE quality_profiles SET
    name                     = ?,
    cutoff_json              = ?,
    qualities_json           = ?,
    upgrade_allowed          = ?,
    upgrade_until_json       = ?,
    updated_at               = ?,
    min_custom_format_score  = ?,
    upgrade_until_cf_score   = ?
WHERE id = ?
RETURNING *;

-- name: DetachQualityProfileFromPulse :exec
UPDATE quality_profiles SET managed_by_pulse = FALSE WHERE id = ?;

-- name: DeleteQualityProfile :exec
DELETE FROM quality_profiles WHERE id = ?;

-- name: QualityProfileInUse :one
SELECT EXISTS (
    SELECT 1 FROM series WHERE quality_profile_id = ?
    UNION ALL
    SELECT 1 FROM libraries WHERE default_quality_profile_id = ?
) AS in_use;
