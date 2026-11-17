-- +goose Up

-- Release profiles (Sonarr's "Must contain" / "Must not contain").
--
--   required_json  JSON array of terms; a release must match at least one
--                  (empty array = no requirement)
--   ignored_json   JSON array of terms; a release must match none
--   indexer_id     limit the profile to one indexer; NULL = all indexers
--
-- A term is a case-insensitive substring, or a regular expression when
-- written as /pattern/. Every enabled profile that applies to a release's
-- indexer must accept it.
CREATE TABLE release_profiles (
    id            TEXT NOT NULL PRIMARY KEY,
    name          TEXT NOT NULL,
    enabled       BOOLEAN NOT NULL DEFAULT TRUE,
    required_json TEXT NOT NULL DEFAULT '[]',
    ignored_json  TEXT NOT NULL DEFAULT '[]',
    indexer_id    TEXT REFERENCES indexer_configs(id) ON DELETE CASCADE,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS release_profiles;
