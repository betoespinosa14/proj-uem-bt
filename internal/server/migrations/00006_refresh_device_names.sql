-- +goose Up
-- Regenerate device names once: groups renamed by a server that predates
-- generated names (00005) left the old group text in device names.
UPDATE devices d SET name = COALESCE((SELECT g.name || ' | ' FROM device_groups g WHERE g.id = d.group_id), '') || d.employee_name
WHERE d.retired_at IS NULL AND d.employee_name <> '';

-- +goose Down
SELECT 1;
