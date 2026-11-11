-- +goose Up
-- One row per organization, department and day with the Dashboard's figures,
-- refreshed hourly, so the Dashboard can show trends. group_key '' = all departments.
CREATE TABLE fleet_snapshots (
    org_id          uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    group_key       text NOT NULL DEFAULT '',
    day             date NOT NULL,
    devices         int  NOT NULL DEFAULT 0,
    online          int  NOT NULL DEFAULT 0,
    offline         int  NOT NULL DEFAULT 0,
    compliant       int  NOT NULL DEFAULT 0,
    non_compliant   int  NOT NULL DEFAULT 0,
    open_alerts     int  NOT NULL DEFAULT 0,
    reported        int  NOT NULL DEFAULT 0,
    encrypted       int  NOT NULL DEFAULT 0,
    firewall        int  NOT NULL DEFAULT 0,
    antivirus       int  NOT NULL DEFAULT 0,
    updates_pending int  NOT NULL DEFAULT 0,
    agent_current   int  NOT NULL DEFAULT 0,
    health_issues   int  NOT NULL DEFAULT 0,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, group_key, day)
);

-- +goose Down
DROP TABLE IF EXISTS fleet_snapshots;
