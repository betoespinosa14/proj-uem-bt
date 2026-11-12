package store

import (
	"context"
	"slices"
	"sort"

	"github.com/jackc/pgx/v5"
)

// RuleTag is a tag a compliance rule put on a device.
type RuleTag struct {
	RuleID string
	Tag    string
}

// SyncRuleTags makes a device's rule-added tags match want (rule ID → tag) for
// the rules it currently fails: wanted tags are added and recorded, and tags a
// rule added earlier come off once no failing rule wants them. A tag that was on
// the device before any rule wanted it was added by a person and is left alone.
// It returns the changes made to the device's tags.
func (s *Store) SyncRuleTags(ctx context.Context, orgID, deviceID string, want map[string]string) (added, removed []RuleTag, err error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)

	var tags []string
	if err := tx.QueryRow(ctx, `SELECT tags FROM devices WHERE org_id = $1 AND id = $2 FOR UPDATE`, orgID, deviceID).Scan(&tags); err != nil {
		return nil, nil, mapErr(err)
	}
	rows, err := tx.Query(ctx, `SELECT rule_id::text, tag FROM device_rule_tags WHERE device_id = $1`, deviceID)
	owned, err := collect(rows, err, func(r pgx.Rows) (RuleTag, error) {
		var t RuleTag
		return t, r.Scan(&t.RuleID, &t.Tag)
	})
	if err != nil {
		return nil, nil, err
	}

	newTags, final, added, removed := reconcileRuleTags(tags, owned, want)
	if len(added) == 0 && len(removed) == 0 && sameRuleTags(owned, final) {
		return nil, nil, nil
	}
	if _, err := tx.Exec(ctx, `DELETE FROM device_rule_tags WHERE device_id = $1`, deviceID); err != nil {
		return nil, nil, err
	}
	for _, t := range final {
		if _, err := tx.Exec(ctx, `INSERT INTO device_rule_tags (device_id, rule_id, tag) VALUES ($1, $2, $3)`, deviceID, t.RuleID, t.Tag); err != nil {
			return nil, nil, err
		}
	}
	if len(added) > 0 || len(removed) > 0 {
		if _, err := tx.Exec(ctx, `UPDATE devices SET tags = $3 WHERE org_id = $1 AND id = $2`, orgID, deviceID, nonNil(newTags)); err != nil {
			return nil, nil, err
		}
	}
	return added, removed, tx.Commit(ctx)
}

// reconcileRuleTags is SyncRuleTags' decision, kept free of the database.
// final is the rule-tag records to keep.
func reconcileRuleTags(tags []string, owned []RuleTag, want map[string]string) (newTags []string, final, added, removed []RuleTag) {
	newTags = slices.Clone(tags)
	has := func(tag string) bool { return slices.Contains(newTags, tag) }
	ownedBy := func(set []RuleTag, tag string) bool {
		return slices.ContainsFunc(set, func(t RuleTag) bool { return t.Tag == tag })
	}

	// Keep the records of rules that still want their tag.
	for _, o := range owned {
		if want[o.RuleID] == o.Tag {
			final = append(final, o)
		}
	}
	ruleIDs := make([]string, 0, len(want))
	for id := range want {
		ruleIDs = append(ruleIDs, id)
	}
	sort.Strings(ruleIDs)
	for _, id := range ruleIDs {
		t := RuleTag{RuleID: id, Tag: want[id]}
		if !slices.Contains(final, t) {
			if has(t.Tag) && !ownedBy(final, t.Tag) {
				continue // put there by a person
			}
			final = append(final, t)
		}
		if !has(t.Tag) {
			newTags = append(newTags, t.Tag)
			added = append(added, t)
		}
	}
	// Take off tags whose rules no longer want them.
	for _, o := range owned {
		if slices.Contains(final, o) || ownedBy(final, o.Tag) || !has(o.Tag) {
			continue
		}
		newTags = slices.DeleteFunc(newTags, func(x string) bool { return x == o.Tag })
		removed = append(removed, o)
	}
	return newTags, final, added, removed
}

func sameRuleTags(a, b []RuleTag) bool {
	if len(a) != len(b) {
		return false
	}
	for _, t := range a {
		if !slices.Contains(b, t) {
			return false
		}
	}
	return true
}

// RuleTagSources maps each tag on a device that a rule added to the names of
// the rules responsible.
func (s *Store) RuleTagSources(ctx context.Context, orgID, deviceID string) (map[string][]string, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT t.tag, coalesce(r.name, 'a deleted rule')
		FROM device_rule_tags t
		JOIN devices d ON d.id = t.device_id AND d.org_id = $1
		LEFT JOIN compliance_rules r ON r.id = t.rule_id
		WHERE t.device_id = $2 AND t.tag = ANY (d.tags)
		ORDER BY t.tag, 2`, orgID, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var tag, rule string
		if err := rows.Scan(&tag, &rule); err != nil {
			return nil, err
		}
		out[tag] = append(out[tag], rule)
	}
	return out, rows.Err()
}
