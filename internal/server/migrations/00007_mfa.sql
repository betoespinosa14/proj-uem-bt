-- +goose Up
-- Two-step sign-in (authenticator-app codes), optional per user.
ALTER TABLE users
    ADD COLUMN totp_secret         text,        -- set once two-step sign-in is on
    ADD COLUMN totp_pending_secret text,        -- set during setup, until the first code confirms it
    ADD COLUMN totp_last_step      bigint NOT NULL DEFAULT 0, -- last accepted code, so it can't be reused
    ADD COLUMN mfa_enabled_at      timestamptz;

-- One-time recovery codes, stored hashed.
CREATE TABLE mfa_recovery_codes (
    user_id   uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash text NOT NULL,
    used_at   timestamptz,
    PRIMARY KEY (user_id, code_hash)
);

-- Browsers where the user ticked "Remember this browser"; id is the sha256 of
-- the cookie token.
CREATE TABLE mfa_trusted_browsers (
    id         text PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    user_agent text
);
CREATE INDEX mfa_trusted_browsers_user_idx ON mfa_trusted_browsers (user_id);

-- +goose Down
DROP TABLE IF EXISTS mfa_trusted_browsers, mfa_recovery_codes;
ALTER TABLE users DROP COLUMN IF EXISTS totp_secret, DROP COLUMN IF EXISTS totp_pending_secret,
    DROP COLUMN IF EXISTS totp_last_step, DROP COLUMN IF EXISTS mfa_enabled_at;
