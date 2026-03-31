-- name: CreateSeason :one
INSERT INTO seasons (id, series_id, season_number, monitored)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: GetSeason :one
SELECT * FROM seasons WHERE id = ?;

-- name: ListSeasonsBySeriesID :many
SELECT * FROM seasons WHERE series_id = ? ORDER BY season_number ASC;

-- name: UpdateSeasonMonitored :exec
UPDATE seasons SET monitored = ? WHERE id = ?;

-- name: DeleteSeasonsBySeriesID :exec
DELETE FROM seasons WHERE series_id = ?;
