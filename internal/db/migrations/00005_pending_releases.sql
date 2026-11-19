-- +goose Up

-- Releases RSS sync found but is holding back because of the delay
-- profile (Sonarr's "pending" releases). At most one per episode: the best
-- seen so far. When eligible_at passes, the next RSS sync grabs it if it
-- is still the best wanted release for the episode.
--
--   release_json  the indexer.SearchResult as JSON
--   eligible_at   RFC3339 UTC; the release's publish time + the delay for
--                 its protocol
CREATE TABLE pending_releases (
    id            TEXT NOT NULL PRIMARY KEY,
    series_id     TEXT NOT NULL REFERENCES series(id) ON DELETE CASCADE,
    episode_id    TEXT NOT NULL UNIQUE REFERENCES episodes(id) ON DELETE CASCADE,
    release_title TEXT NOT NULL,
    protocol      TEXT NOT NULL,
    quality_name  TEXT NOT NULL DEFAULT '',
    release_json  TEXT NOT NULL,
    eligible_at   TEXT NOT NULL,
    added_at      TEXT NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS pending_releases;
