-- +goose Up
-- When an app was last used with keyboard or mouse, as opposed to last_used,
-- which moves whenever the app is seen running (also in the background).
-- Set from usage reports that carry active time, so it is accurate to the
-- agent's report interval (about 5 minutes). Earlier rows with active time
-- take their last_used as the best available estimate.
ALTER TABLE app_usage ADD COLUMN last_active timestamptz;
UPDATE app_usage SET last_active = last_used WHERE active_seconds > 0;

-- +goose Down
ALTER TABLE app_usage DROP COLUMN last_active;
