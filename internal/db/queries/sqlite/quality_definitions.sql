-- name: ListQualityDefinitions :many
SELECT * FROM quality_definitions ORDER BY sort_order ASC;

-- name: UpdateQualityDefinitionSizes :exec
UPDATE quality_definitions
SET min_size = ?, max_size = ?, preferred_size = ?
WHERE id = ?;
