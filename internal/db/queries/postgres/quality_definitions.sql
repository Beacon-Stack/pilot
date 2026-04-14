-- name: ListQualityDefinitions :many
SELECT * FROM quality_definitions ORDER BY sort_order ASC;

-- name: UpdateQualityDefinitionSizes :exec
UPDATE quality_definitions
SET min_size = $1, max_size = $2, preferred_size = $3
WHERE id = $4;
