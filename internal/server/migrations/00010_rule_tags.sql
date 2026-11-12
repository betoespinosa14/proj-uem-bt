-- +goose Up
-- Tags a compliance rule put on a device, so BEACON can take them off again
-- when the device passes the rule. Tags people add by hand have no row here
-- and are never removed automatically.
CREATE TABLE device_rule_tags (
    device_id  uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    rule_id    uuid NOT NULL, -- no foreign key: a deleted rule's tags must still come off
    tag        text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (device_id, rule_id, tag)
);

-- Existing tags: a device currently failing a rule and carrying that rule's tag
-- got the tag from the rule.
INSERT INTO device_rule_tags (device_id, rule_id, tag)
SELECT r.device_id, r.rule_id, btrim(cr.actions->>'tag')
FROM compliance_results r
JOIN compliance_rules cr ON cr.id = r.rule_id
JOIN devices d ON d.id = r.device_id
WHERE NOT r.passed
  AND btrim(coalesce(cr.actions->>'tag', '')) <> ''
  AND btrim(cr.actions->>'tag') = ANY (d.tags)
ON CONFLICT DO NOTHING;

-- +goose Down
DROP TABLE device_rule_tags;
