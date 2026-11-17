-- name: UpsertPendingRelease :exec
INSERT INTO pending_releases (id, series_id, episode_id, release_title, protocol, quality_name, release_json, eligible_at, added_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(episode_id) DO UPDATE SET
    release_title = excluded.release_title,
    protocol      = excluded.protocol,
    quality_name  = excluded.quality_name,
    release_json  = excluded.release_json,
    eligible_at   = excluded.eligible_at;

-- name: ListPendingReleases :many
SELECT * FROM pending_releases ORDER BY eligible_at ASC;

-- name: ListPendingReleasesWithSeries :many
SELECT p.id, p.series_id, p.episode_id, p.release_title, p.protocol, p.quality_name, p.eligible_at, p.added_at,
       s.title AS series_title, e.season_number, e.episode_number
FROM pending_releases p
JOIN series s ON s.id = p.series_id
JOIN episodes e ON e.id = p.episode_id
ORDER BY p.eligible_at ASC;

-- name: DeletePendingRelease :exec
DELETE FROM pending_releases WHERE id = ?;

-- name: DeletePendingReleaseByEpisode :exec
DELETE FROM pending_releases WHERE episode_id = ?;
