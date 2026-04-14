-- name: CreateQualityProfile :one
INSERT INTO quality_profiles (
    id, name, cutoff_json, qualities_json,
    upgrade_allowed, upgrade_until_json, created_at, updated_at,
    min_custom_format_score, upgrade_until_cf_score, managed_by_pulse
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7, $8,
    $9, $10, $11
)
RETURNING *;

-- name: GetQualityProfile :one
SELECT * FROM quality_profiles WHERE id = $1;

-- name: ListQualityProfiles :many
SELECT * FROM quality_profiles ORDER BY name ASC;

-- name: ListManagedQualityProfiles :many
SELECT * FROM quality_profiles WHERE managed_by_pulse = TRUE ORDER BY name ASC;

-- name: UpdateQualityProfile :one
UPDATE quality_profiles SET
    name                     = $1,
    cutoff_json              = $2,
    qualities_json           = $3,
    upgrade_allowed          = $4,
    upgrade_until_json       = $5,
    updated_at               = $6,
    min_custom_format_score  = $7,
    upgrade_until_cf_score   = $8
WHERE id = $9
RETURNING *;

-- name: DetachQualityProfileFromPulse :exec
UPDATE quality_profiles SET managed_by_pulse = FALSE WHERE id = $1;

-- name: DeleteQualityProfile :exec
DELETE FROM quality_profiles WHERE id = $1;

-- name: QualityProfileInUse :one
SELECT EXISTS (
    SELECT 1 FROM series WHERE quality_profile_id = $1
    UNION ALL
    SELECT 1 FROM libraries WHERE default_quality_profile_id = $2
) AS in_use;
