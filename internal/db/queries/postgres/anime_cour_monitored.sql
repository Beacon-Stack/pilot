-- name: ListAnimeCourMonitored :many
SELECT * FROM anime_cour_monitored
WHERE series_id = $1
ORDER BY tvdb_season ASC;

-- name: UpsertAnimeCourMonitored :exec
INSERT INTO anime_cour_monitored (series_id, tvdb_season, monitored, updated_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (series_id, tvdb_season) DO UPDATE
    SET monitored = EXCLUDED.monitored,
        updated_at = now();

-- name: DeleteAnimeCourMonitoredBySeriesID :exec
DELETE FROM anime_cour_monitored WHERE series_id = $1;
