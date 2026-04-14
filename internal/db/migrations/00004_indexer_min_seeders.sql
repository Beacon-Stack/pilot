-- +goose Up

-- Per-indexer minimum seeders threshold. Default 5 is a reasonable
-- starting point for public trackers (the 0-2 "fuzzed by indexer" band
-- is dropped, but legitimate low-seed releases are kept). TRaSH Guides
-- recommends 20+ for public trackers; users can bump per-indexer via
-- the settings UI.
--
-- Note: this is Pilot-local for Phase 0. We may centralize it in Pulse
-- in Phase 1 after measurement data shapes the decision.
ALTER TABLE indexer_configs
    ADD COLUMN min_seeders INTEGER NOT NULL DEFAULT 5;

-- +goose Down

ALTER TABLE indexer_configs DROP COLUMN IF EXISTS min_seeders;
