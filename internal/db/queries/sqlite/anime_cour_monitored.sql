-- name: ListAnimeCourMonitored :many
SELECT * FROM anime_cour_monitored
WHERE series_id = ?
ORDER BY tvdb_season ASC;

-- name: UpsertAnimeCourMonitored :exec
INSERT INTO anime_cour_monitored (series_id, tvdb_season, monitored, updated_at)
VALUES (?, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
ON CONFLICT (series_id, tvdb_season) DO UPDATE
    SET monitored = excluded.monitored,
        updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now');

-- name: DeleteAnimeCourMonitoredBySeriesID :exec
DELETE FROM anime_cour_monitored WHERE series_id = ?;
