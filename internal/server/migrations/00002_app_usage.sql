-- +goose Up
ALTER TABLE organizations ADD COLUMN usage_tracking boolean NOT NULL DEFAULT false;

-- Minutes each application ran per user per (device-local) day.
CREATE TABLE app_usage (
    device_id   uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    org_id      uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    day         date NOT NULL,
    app         text NOT NULL,
    username    text NOT NULL,
    minutes     integer NOT NULL CHECK (minutes BETWEEN 0 AND 1440),
    last_used   timestamptz NOT NULL,
    PRIMARY KEY (device_id, day, app, username)
);
CREATE INDEX app_usage_org_app_idx ON app_usage (org_id, lower(app));
CREATE INDEX app_usage_day_idx ON app_usage (day);

-- +goose Down
DROP TABLE IF EXISTS app_usage;
ALTER TABLE organizations DROP COLUMN IF EXISTS usage_tracking;
