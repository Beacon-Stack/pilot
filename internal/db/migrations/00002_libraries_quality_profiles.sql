-- +goose Up

CREATE TABLE quality_profiles (
    id                      TEXT NOT NULL PRIMARY KEY,
    name                    TEXT NOT NULL,
    cutoff_json             TEXT NOT NULL DEFAULT '{}',
    qualities_json          TEXT NOT NULL DEFAULT '[]',
    upgrade_allowed         INTEGER NOT NULL DEFAULT 1,
    upgrade_until_json      TEXT,
    min_custom_format_score INTEGER NOT NULL DEFAULT 0,
    upgrade_until_cf_score  INTEGER NOT NULL DEFAULT 0,
    created_at              TEXT NOT NULL,
    updated_at              TEXT NOT NULL
);

CREATE TABLE libraries (
    id                          TEXT NOT NULL PRIMARY KEY,
    name                        TEXT NOT NULL,
    root_path                   TEXT NOT NULL,
    default_quality_profile_id  TEXT NOT NULL REFERENCES quality_profiles(id),
    naming_format               TEXT,
    folder_format               TEXT,
    min_free_space_gb           INTEGER NOT NULL DEFAULT 5,
    tags_json                   TEXT NOT NULL DEFAULT '[]',
    created_at                  TEXT NOT NULL,
    updated_at                  TEXT NOT NULL
);

-- +goose Down

DROP TABLE IF EXISTS libraries;
DROP TABLE IF EXISTS quality_profiles;
