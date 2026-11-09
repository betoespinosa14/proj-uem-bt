-- +goose Up
-- Apps an admin marked as infrastructure (e.g. a remote-access host): open by
-- design, so they are left out of idle/open-time rankings on App usage.
CREATE TABLE usage_infrastructure_apps (
    org_id     uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    app        text NOT NULL CHECK (length(app) BETWEEN 1 AND 256),
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX usage_infrastructure_apps_org_app_idx ON usage_infrastructure_apps (org_id, lower(app));

-- +goose Down
DROP TABLE IF EXISTS usage_infrastructure_apps;
