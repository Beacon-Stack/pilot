-- +goose Up
ALTER TABLE episodes ADD COLUMN still_path TEXT NOT NULL DEFAULT '';
ALTER TABLE episodes ADD COLUMN runtime_minutes INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE episodes DROP COLUMN still_path;
ALTER TABLE episodes DROP COLUMN runtime_minutes;
