-- +goose Up
-- Time an app was in front while the user was interacting with it.
ALTER TABLE app_usage ADD COLUMN active_seconds integer NOT NULL DEFAULT 0
    CHECK (active_seconds BETWEEN 0 AND 86400);
-- Input idle time after which foreground time stops counting as active.
ALTER TABLE organizations ADD COLUMN idle_threshold_sec integer NOT NULL DEFAULT 120
    CHECK (idle_threshold_sec BETWEEN 60 AND 900);

-- +goose Down
ALTER TABLE organizations DROP COLUMN IF EXISTS idle_threshold_sec;
ALTER TABLE app_usage DROP COLUMN IF EXISTS active_seconds;
