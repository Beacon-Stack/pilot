-- +goose Up

-- Extend the existing blocklist table with fields needed for reason-aware
-- automated blocklisting (stall detection) and cross-GUID dedup by info
-- hash. See plans/dead-torrent-phase0.md for the rationale — the short
-- version is that the existing blocklist was built for user-marked entries
-- post-grab, and now the stall watcher needs to populate it automatically
-- with structured reason data.
ALTER TABLE blocklist
    ADD COLUMN reason    TEXT NOT NULL DEFAULT 'user_marked',
    ADD COLUMN info_hash TEXT;

-- Non-unique index: different-GUID-same-content releases should both dedup
-- through info_hash, but we don't require the hash to be present (user-
-- marked entries may be added before any download exposes the hash).
CREATE INDEX idx_blocklist_info_hash
    ON blocklist(info_hash)
    WHERE info_hash IS NOT NULL;

-- Extend grab_history with the minimum fields the stall watcher needs:
-- `source` (interactive vs auto_search — controls whether a stall triggers
-- automatic re-search) and `info_hash` (populated by the stall watcher
-- once Haul has observed it, used to correlate Haul state to Pilot grabs).
ALTER TABLE grab_history
    ADD COLUMN source    TEXT NOT NULL DEFAULT 'interactive',
    ADD COLUMN info_hash TEXT;

CREATE INDEX idx_grab_history_info_hash
    ON grab_history(info_hash)
    WHERE info_hash IS NOT NULL;

-- +goose Down

DROP INDEX IF EXISTS idx_grab_history_info_hash;
ALTER TABLE grab_history
    DROP COLUMN IF EXISTS info_hash,
    DROP COLUMN IF EXISTS source;

DROP INDEX IF EXISTS idx_blocklist_info_hash;
ALTER TABLE blocklist
    DROP COLUMN IF EXISTS info_hash,
    DROP COLUMN IF EXISTS reason;
