-- +goose Up
-- The employee a device belongs to. Once set, the device's display name is
-- generated as "<group name> | <employee name>" (or just the employee name
-- when the device has no group).
ALTER TABLE devices ADD COLUMN employee_name text NOT NULL DEFAULT '';

-- Existing devices: take the employee from today's hand-typed names
-- ("ADMIN | Arnold Nacario" -> "Arnold Nacario"; no " | " -> the whole name),
-- then generate their names.
UPDATE devices SET employee_name = btrim(CASE WHEN position(' | ' IN name) > 0
        THEN substr(name, position(' | ' IN name) + 3) ELSE name END)
WHERE retired_at IS NULL;

UPDATE devices d SET name = COALESCE((SELECT g.name || ' | ' FROM device_groups g WHERE g.id = d.group_id), '') || d.employee_name
WHERE d.retired_at IS NULL AND d.employee_name <> '';

-- +goose Down
ALTER TABLE devices DROP COLUMN IF EXISTS employee_name;
