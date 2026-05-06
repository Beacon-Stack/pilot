-- +goose Up

-- Lifecycle Trust QW4: prevent duplicate active grabs for the same
-- (episode, release) at the database level. Backstops any code path
-- that bypasses pilot's RSS / search dedup logic.
--
-- Why a partial index instead of a plain UNIQUE constraint:
-- terminal grabs (failed / stalled / removed / completed) MUST stay in
-- grab_history for the activity timeline + library badges. Only the
-- non-terminal states should be dedup-protected, so the user can
-- always re-grab a release after a previous attempt for the same
-- episode terminated.
--
-- Why the dedup step first: QW1 fixes the bug that creates dupes, but
-- pre-existing rows in production must be cleaned up before the index
-- can be applied — Postgres rejects index creation if duplicates
-- already exist in the keyed columns. We keep the most-recent grab
-- per (episode_id, release_guid) (it's the one most likely to reflect
-- current intent) and delete older dupes.
--
-- NULL episode_id (season-pack grabs that don't bind to a single
-- episode) are skipped by the partial index — Postgres treats NULL
-- in unique indexes as distinct, and we don't want to dedup season
-- packs by release_guid alone.

-- Dedup: keep the row with the latest grabbed_at per (episode_id,
-- release_guid); delete the rest. The terminal_filter mirrors the
-- index predicate below — terminal rows are exempt from dedup.
DELETE FROM grab_history
WHERE id IN (
    SELECT id FROM (
        SELECT id,
               ROW_NUMBER() OVER (
                   PARTITION BY episode_id, release_guid
                   ORDER BY grabbed_at DESC, id DESC
               ) AS rn
        FROM grab_history
        WHERE episode_id IS NOT NULL
          AND download_status NOT IN ('failed', 'stalled', 'removed', 'completed')
    ) ranked
    WHERE rn > 1
);

-- Partial unique index: at most one active grab per (episode, release).
CREATE UNIQUE INDEX idx_grab_history_active_episode_release
    ON grab_history (episode_id, release_guid)
    WHERE download_status NOT IN ('failed', 'stalled', 'removed', 'completed');

-- +goose Down

DROP INDEX IF EXISTS idx_grab_history_active_episode_release;
