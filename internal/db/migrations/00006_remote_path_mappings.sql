-- +goose Up

-- Remote path mappings (as in Sonarr): a download client that runs in a
-- container or on another machine reports paths as it sees them, e.g.
-- /downloads/Show.S01E01. A mapping rewrites the remote_path prefix to
-- local_path, the same folder as Pilot sees it.
--
--   download_client_id  limit the mapping to one client; NULL = any client
CREATE TABLE remote_path_mappings (
    id                 TEXT NOT NULL PRIMARY KEY,
    download_client_id TEXT REFERENCES download_client_configs(id) ON DELETE CASCADE,
    remote_path        TEXT NOT NULL,
    local_path         TEXT NOT NULL,
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS remote_path_mappings;
