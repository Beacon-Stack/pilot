-- +goose Up

-- Import outcome tracking on grabs. Previously import success lived only
-- in episode_files and failure was recorded nowhere — a grab whose file
-- failed to import (fake .exe uploads) stayed 'completed' forever and
-- looked healthy. import_status: '' (not attempted), 'imported', 'failed'.
ALTER TABLE grab_history ADD COLUMN import_status TEXT NOT NULL DEFAULT '';
ALTER TABLE grab_history ADD COLUMN imported_at TEXT NOT NULL DEFAULT '';
ALTER TABLE grab_history ADD COLUMN import_error TEXT NOT NULL DEFAULT '';

-- Backfill: completed grabs whose episode has a file were imported.
-- imported_at comes from the episode file record where available.
UPDATE grab_history SET
    import_status = 'imported',
    imported_at = COALESCE(
        (SELECT ef.imported_at FROM episode_files ef
         WHERE ef.episode_id = grab_history.episode_id
         ORDER BY ef.imported_at DESC LIMIT 1),
        grab_history.grabbed_at
    )
WHERE download_status = 'completed'
  AND episode_id IS NOT NULL
  AND episode_id IN (SELECT episode_id FROM episode_files);

-- +goose Down
ALTER TABLE grab_history DROP COLUMN import_status;
ALTER TABLE grab_history DROP COLUMN imported_at;
ALTER TABLE grab_history DROP COLUMN import_error;
