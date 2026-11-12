-- +goose Up
-- How a webhook's body is written: 'generic' JSON for SIEM/ITSM tools, or a
-- Microsoft Teams message card for a Teams Workflows webhook.
ALTER TABLE webhooks
    ADD COLUMN format text NOT NULL DEFAULT 'generic' CHECK (format IN ('generic', 'teams'));

-- +goose Down
ALTER TABLE webhooks DROP COLUMN format;
