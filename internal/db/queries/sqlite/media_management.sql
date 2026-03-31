-- name: GetMediaManagement :one
SELECT * FROM media_management WHERE id = 1;

-- name: UpdateMediaManagement :one
UPDATE media_management
SET rename_episodes            = ?,
    standard_episode_format    = ?,
    daily_episode_format       = ?,
    anime_episode_format       = ?,
    series_folder_format       = ?,
    season_folder_format       = ?,
    colon_replacement          = ?,
    import_extra_files         = ?,
    extra_file_extensions      = ?,
    unmonitor_deleted_episodes = ?
WHERE id = 1
RETURNING *;
