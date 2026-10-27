-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE organizations (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text NOT NULL,
    server_url  text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email          text NOT NULL,
    name           text NOT NULL DEFAULT '',
    password_hash  text NOT NULL,
    role           text NOT NULL CHECK (role IN ('owner','admin','helpdesk','auditor')),
    created_at     timestamptz NOT NULL DEFAULT now(),
    last_login_at  timestamptz
);
-- Email is globally unique so login needs no org selector.
CREATE UNIQUE INDEX users_email_uniq ON users (lower(email));
CREATE INDEX users_org_idx ON users (org_id);

CREATE TABLE sessions (
    id          text PRIMARY KEY,           -- sha256 of the opaque session token
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id      uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    csrf_token  text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz NOT NULL,
    ip          text,
    user_agent  text
);
CREATE INDEX sessions_user_idx ON sessions (user_id);

CREATE TABLE device_groups (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name          text NOT NULL,
    description   text NOT NULL DEFAULT '',
    blocked_apps  text[] NOT NULL DEFAULT '{}',
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, name)
);

CREATE TABLE enrollment_tokens (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    token        text NOT NULL UNIQUE,
    description  text NOT NULL DEFAULT '',
    group_id     uuid REFERENCES device_groups(id) ON DELETE SET NULL,
    expires_at   timestamptz,
    max_uses     integer,
    uses         integer NOT NULL DEFAULT 0,
    revoked      boolean NOT NULL DEFAULT false,
    created_by   uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX enrollment_tokens_org_idx ON enrollment_tokens (org_id);

CREATE TABLE devices (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id            uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    group_id          uuid REFERENCES device_groups(id) ON DELETE SET NULL,
    secret_hash       text NOT NULL,
    name              text NOT NULL,
    hostname          text NOT NULL DEFAULT '',
    os_family         text NOT NULL DEFAULT '',
    os_name           text NOT NULL DEFAULT '',
    os_version        text NOT NULL DEFAULT '',
    os_build          text NOT NULL DEFAULT '',
    arch              text NOT NULL DEFAULT '',
    serial            text NOT NULL DEFAULT '',
    model             text NOT NULL DEFAULT '',
    manufacturer      text NOT NULL DEFAULT '',
    agent_version     text NOT NULL DEFAULT '',
    ip_address        text NOT NULL DEFAULT '',
    mac_address       text NOT NULL DEFAULT '',
    logged_in_user    text NOT NULL DEFAULT '',
    status            text NOT NULL DEFAULT 'offline' CHECK (status IN ('online','offline')),
    last_seen_at      timestamptz,
    enrolled_at       timestamptz NOT NULL DEFAULT now(),
    compliance_state  text NOT NULL DEFAULT 'unknown' CHECK (compliance_state IN ('compliant','non_compliant','unknown')),
    tags              text[] NOT NULL DEFAULT '{}',
    cpu_pct           double precision,
    mem_pct           double precision,
    disk_pct          double precision,
    battery_pct       double precision,
    retired_at        timestamptz
);
CREATE INDEX devices_org_idx ON devices (org_id) WHERE retired_at IS NULL;

CREATE TABLE device_inventory (
    device_id     uuid PRIMARY KEY REFERENCES devices(id) ON DELETE CASCADE,
    org_id        uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    collected_at  timestamptz NOT NULL,
    data          jsonb NOT NULL
);

CREATE TABLE device_software (
    device_id     uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    org_id        uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name          text NOT NULL,
    version       text NOT NULL DEFAULT '',
    publisher     text NOT NULL DEFAULT '',
    source        text NOT NULL DEFAULT '',
    install_date  text
);
CREATE INDEX device_software_device_idx ON device_software (device_id);
CREATE INDEX device_software_name_idx ON device_software (org_id, lower(name));

CREATE TABLE device_telemetry (
    device_id    uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    ts           timestamptz NOT NULL,
    cpu_pct      double precision,
    mem_pct      double precision,
    disk_pct     double precision,
    battery_pct  double precision
);
CREATE INDEX device_telemetry_idx ON device_telemetry (device_id, ts DESC);

CREATE TABLE scripts (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id           uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name             text NOT NULL,
    description      text NOT NULL DEFAULT '',
    shell            text NOT NULL CHECK (shell IN ('bash','sh','zsh','powershell','cmd')),
    body             text NOT NULL,
    timeout_seconds  integer NOT NULL DEFAULT 300,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX scripts_org_idx ON scripts (org_id);

CREATE TABLE commands (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    device_id     uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    type          text NOT NULL,
    payload       jsonb NOT NULL DEFAULT '{}',
    status        text NOT NULL DEFAULT 'queued',
    exit_code     integer,
    stdout        text NOT NULL DEFAULT '',
    stderr        text NOT NULL DEFAULT '',
    error         text,
    issued_by     uuid REFERENCES users(id) ON DELETE SET NULL,
    source        text NOT NULL DEFAULT 'user' CHECK (source IN ('user','scheduler','compliance')),
    created_at    timestamptz NOT NULL DEFAULT now(),
    sent_at       timestamptz,
    completed_at  timestamptz
);
CREATE INDEX commands_device_idx ON commands (device_id, created_at DESC);
CREATE INDEX commands_pending_idx ON commands (device_id) WHERE status IN ('queued','sent','running');
CREATE INDEX commands_org_idx ON commands (org_id, created_at DESC);

CREATE TABLE compliance_rules (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name         text NOT NULL,
    description  text NOT NULL DEFAULT '',
    enabled      boolean NOT NULL DEFAULT true,
    severity     text NOT NULL CHECK (severity IN ('low','medium','high','critical')),
    os_families  text[] NOT NULL DEFAULT '{}',
    group_id     uuid REFERENCES device_groups(id) ON DELETE SET NULL,
    match        text NOT NULL DEFAULT 'all' CHECK (match IN ('all','any')),
    conditions   jsonb NOT NULL DEFAULT '[]',
    actions      jsonb NOT NULL DEFAULT '{}',
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX compliance_rules_org_idx ON compliance_rules (org_id);

CREATE TABLE compliance_results (
    device_id     uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    rule_id       uuid NOT NULL REFERENCES compliance_rules(id) ON DELETE CASCADE,
    org_id        uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    passed        boolean NOT NULL,
    details       text NOT NULL DEFAULT '',
    evaluated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (device_id, rule_id)
);
CREATE INDEX compliance_results_rule_idx ON compliance_results (rule_id) WHERE NOT passed;

CREATE TABLE scheduled_tasks (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name         text NOT NULL,
    enabled      boolean NOT NULL DEFAULT true,
    cron         text NOT NULL,
    target       jsonb NOT NULL,
    action       jsonb NOT NULL,
    last_run_at  timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE alerts (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    device_id    uuid REFERENCES devices(id) ON DELETE CASCADE,
    rule_id      uuid REFERENCES compliance_rules(id) ON DELETE SET NULL,
    severity     text NOT NULL,
    title        text NOT NULL,
    message      text NOT NULL DEFAULT '',
    status       text NOT NULL DEFAULT 'open' CHECK (status IN ('open','resolved')),
    created_at   timestamptz NOT NULL DEFAULT now(),
    resolved_at  timestamptz
);
CREATE INDEX alerts_org_idx ON alerts (org_id, created_at DESC);
-- At most one open alert per (device, rule): repeat violations don't spam.
CREATE UNIQUE INDEX alerts_open_uniq ON alerts (device_id, rule_id) WHERE status = 'open' AND rule_id IS NOT NULL;

CREATE TABLE webhooks (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id            uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name              text NOT NULL,
    url               text NOT NULL,
    secret            text NOT NULL,
    events            text[] NOT NULL DEFAULT '{}',
    enabled           boolean NOT NULL DEFAULT true,
    created_at        timestamptz NOT NULL DEFAULT now(),
    last_status       integer,
    last_delivery_at  timestamptz
);

CREATE TABLE audit_log (
    id           bigserial PRIMARY KEY,
    org_id       uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    ts           timestamptz NOT NULL,
    actor_type   text NOT NULL CHECK (actor_type IN ('user','system','device')),
    actor_id     text,
    actor_email  text,
    action       text NOT NULL,
    target_type  text,
    target_id    text,
    details      jsonb NOT NULL DEFAULT '{}',
    ip           text,
    prev_hash    text NOT NULL,
    hash         text NOT NULL
);
CREATE INDEX audit_log_org_idx ON audit_log (org_id, id DESC);

-- The audit log is append-only. Deleting an organization is the only path
-- that may remove rows (via cascade), which runs with the guard disabled.
-- +goose StatementBegin
CREATE FUNCTION audit_log_immutable() RETURNS trigger AS $$
BEGIN
    IF current_setting('uem.allow_audit_delete', true) = 'on' THEN
        RETURN OLD;
    END IF;
    RAISE EXCEPTION 'audit_log is append-only';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER audit_log_no_update BEFORE UPDATE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_immutable();
CREATE TRIGGER audit_log_no_delete BEFORE DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_immutable();

-- +goose Down
DROP TABLE IF EXISTS audit_log, webhooks, alerts, scheduled_tasks, compliance_results,
    compliance_rules, commands, scripts, device_telemetry, device_software,
    device_inventory, devices, enrollment_tokens, device_groups, sessions, users, organizations CASCADE;
DROP FUNCTION IF EXISTS audit_log_immutable();
