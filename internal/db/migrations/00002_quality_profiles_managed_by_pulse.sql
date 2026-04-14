-- +goose Up

-- Flag distinguishing locally-managed profiles from ones synced from Pulse.
-- When TRUE, the sync loop will update/delete this row to match Pulse.
-- When FALSE (default), the row is a local profile that sync never touches.
-- Setting a previously managed profile to FALSE turns it into a local shadow.
ALTER TABLE quality_profiles
    ADD COLUMN managed_by_pulse BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX idx_quality_profiles_managed_by_pulse
    ON quality_profiles(managed_by_pulse)
    WHERE managed_by_pulse = TRUE;

-- +goose Down

DROP INDEX IF EXISTS idx_quality_profiles_managed_by_pulse;
ALTER TABLE quality_profiles DROP COLUMN IF EXISTS managed_by_pulse;
