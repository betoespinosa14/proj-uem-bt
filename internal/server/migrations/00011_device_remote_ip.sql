-- +goose Up
-- The address a device's agent last connected from, as the server saw it: the
-- office firewall for devices in the office, the home or mobile network's
-- public address for devices outside.
ALTER TABLE devices ADD COLUMN remote_ip text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE devices DROP COLUMN remote_ip;
