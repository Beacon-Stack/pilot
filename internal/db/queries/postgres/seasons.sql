-- name: CreateSeason :one
INSERT INTO seasons (id, series_id, season_number, monitored)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetSeason :one
SELECT * FROM seasons WHERE id = $1;

-- name: ListSeasonsBySeriesID :many
SELECT * FROM seasons WHERE series_id = $1 ORDER BY season_number ASC;

-- name: UpdateSeasonMonitored :exec
UPDATE seasons SET monitored = $1 WHERE id = $2;

-- name: DeleteSeasonsBySeriesID :exec
DELETE FROM seasons WHERE series_id = $1;
