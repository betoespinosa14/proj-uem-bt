-- +goose Up
-- The enrollment token each device enrolled with, so a token can list its
-- devices and show when it was last used. Devices enrolled before this column
-- are linked only where their organization has exactly one token (then it must
-- be the one they used); elsewhere they stay unlinked.
ALTER TABLE devices ADD COLUMN enrollment_token_id uuid REFERENCES enrollment_tokens(id) ON DELETE SET NULL;
CREATE INDEX devices_enrollment_token_idx ON devices (enrollment_token_id);

UPDATE devices d SET enrollment_token_id = t.id
FROM enrollment_tokens t
WHERE t.org_id = d.org_id
  AND t.created_at <= d.enrolled_at
  AND (SELECT count(*) FROM enrollment_tokens x WHERE x.org_id = d.org_id) = 1;

-- +goose Down
ALTER TABLE devices DROP COLUMN enrollment_token_id;
