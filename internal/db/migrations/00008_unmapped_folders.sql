-- +goose Up

-- Folders in a library root that no series in Pilot claims. The library
-- scan used to auto-add a series for each one from its best TMDB search
-- hit, without asking; a wrong hit added the wrong show. Now the scan
-- records the folder with its best guess, and the user confirms, picks
-- another show, or ignores it.
CREATE TABLE unmapped_folders (
    id                TEXT NOT NULL PRIMARY KEY,
    library_id        TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    path              TEXT NOT NULL UNIQUE,
    name              TEXT NOT NULL,
    guess_tmdb_id     INTEGER NOT NULL DEFAULT 0,
    guess_title       TEXT NOT NULL DEFAULT '',
    guess_year        INTEGER NOT NULL DEFAULT 0,
    guess_poster_path TEXT NOT NULL DEFAULT '',
    ignored           BOOLEAN NOT NULL DEFAULT FALSE,
    first_seen_at     TEXT NOT NULL
);

CREATE INDEX idx_unmapped_folders_library_id ON unmapped_folders(library_id);

-- +goose Down
DROP TABLE IF EXISTS unmapped_folders;
