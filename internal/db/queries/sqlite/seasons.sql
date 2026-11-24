-- name: CreateSeason :one
INSERT INTO seasons (id, series_id, season_number, monitored)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: ListSeasonsBySeriesID :many
SELECT * FROM seasons WHERE series_id = ? ORDER BY season_number ASC;

-- name: UpdateSeasonMonitored :exec
UPDATE seasons SET monitored = ? WHERE id = ?;

