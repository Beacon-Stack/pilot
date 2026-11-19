-- +goose Up

-- Automatic import retry. 00002 records whether a grab's import succeeded;
-- these columns let a failed import be re-run later without anyone
-- re-importing by hand (e.g. after fixing a library folder that the
-- container user couldn't write to).
--
--   content_path           where the download client put the finished
--                          download; '' = unknown (legacy rows)
--   import_attempts        import attempts so far; hard failures
--                          (unimportable content) jump straight to the cap
--   import_last_attempt_at RFC3339 of the last attempt; drives backoff
ALTER TABLE grab_history ADD COLUMN content_path TEXT NOT NULL DEFAULT '';
ALTER TABLE grab_history ADD COLUMN import_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE grab_history ADD COLUMN import_last_attempt_at TEXT NOT NULL DEFAULT '';

-- Rows imported or failed before this migration had one attempt. They
-- have no content_path, so they are never auto-retried.
UPDATE grab_history SET import_attempts = 1 WHERE import_status != '';

CREATE INDEX idx_grab_history_import_failed
    ON grab_history(import_last_attempt_at)
    WHERE import_status = 'failed';

-- +goose Down
DROP INDEX IF EXISTS idx_grab_history_import_failed;
ALTER TABLE grab_history DROP COLUMN import_last_attempt_at;
ALTER TABLE grab_history DROP COLUMN import_attempts;
ALTER TABLE grab_history DROP COLUMN content_path;
