-- +goose Up

-- Tighten activity_log.category from a free-form TEXT to a small set of
-- explicit, success/failure-split values. The frontend's Activity-page
-- "Needs attention" rail relies on these strings to surface failures
-- without re-parsing the type field; without an enum-like contract the
-- next refactor of an emit site silently breaks the page.
--
-- The new vocabulary (kept in sync with internal/core/activity/categories.go):
--
--   grab_succeeded     — TypeEpisodeGrabbed
--   grab_failed        — TypeGrabFailed (and stallwatcher gave-up case)
--   import_succeeded   — TypeImportComplete
--   import_failed      — TypeImportFailed
--   stalled            — TypeGrabStalled
--   show               — show added/deleted/updated (kept; not used for alerts)
--   task               — task started/finished (kept; reserved for future task tracker)
--   health             — health checks (kept; reserved for future health emitters)
--
-- Existing rows: only `grab`, `import`, and `show` were ever written
-- (events code never published task/health). The grab and import buckets
-- are split by the `type` column, which records the originating event
-- name verbatim — a clean, lossless rewrite.

-- grab → grab_succeeded / grab_failed
UPDATE activity_log SET category = 'grab_succeeded'
 WHERE category = 'grab' AND type = 'episode_grabbed';
UPDATE activity_log SET category = 'grab_failed'
 WHERE category = 'grab' AND type IN ('grab_failed', 'grab_stalled_gave_up');
UPDATE activity_log SET category = 'stalled'
 WHERE category = 'grab' AND type = 'grab_stalled';

-- import → import_succeeded / import_failed
-- (download_done is treated as the start of the import pipeline; classify
-- it as succeeded for backfill purposes — older rows pre-date the
-- import_complete/import_failed split.)
UPDATE activity_log SET category = 'import_succeeded'
 WHERE category = 'import' AND type IN ('import_complete', 'download_done');
UPDATE activity_log SET category = 'import_failed'
 WHERE category = 'import' AND type = 'import_failed';

-- Any leftover `grab` / `import` rows whose type didn't match above are
-- left as-is. The frontend treats unknown categories as "other" and the
-- API's category validator accepts the legacy values, so nothing breaks.

-- +goose Down

-- Coalesce the split categories back to the original three values.
UPDATE activity_log SET category = 'grab'
 WHERE category IN ('grab_succeeded', 'grab_failed', 'stalled');
UPDATE activity_log SET category = 'import'
 WHERE category IN ('import_succeeded', 'import_failed');
