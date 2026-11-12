package store

import (
	"slices"
	"testing"
)

func TestReconcileRuleTags(t *testing.T) {
	games := RuleTag{"r1", "game"}
	cases := []struct {
		name                string
		tags                []string
		owned               []RuleTag
		want                map[string]string
		wantTags            []string
		wantFinal           []RuleTag
		wantAdded, wantGone int
	}{
		{"failing adds and records", []string{"vip"}, nil, map[string]string{"r1": "game"},
			[]string{"vip", "game"}, []RuleTag{games}, 1, 0},
		{"passing removes", []string{"vip", "game"}, []RuleTag{games}, nil,
			[]string{"vip"}, nil, 0, 1},
		{"manual tag with the same name is left alone", []string{"game"}, nil, map[string]string{"r1": "game"},
			[]string{"game"}, nil, 0, 0},
		{"manual tag survives the rule passing", []string{"game"}, nil, nil,
			[]string{"game"}, nil, 0, 0},
		{"removed by hand while failing comes back", []string{}, []RuleTag{games}, map[string]string{"r1": "game"},
			[]string{"game"}, []RuleTag{games}, 1, 0},
		{"shared tag stays while another rule fails", []string{"bad"}, []RuleTag{{"r1", "bad"}, {"r2", "bad"}}, map[string]string{"r2": "bad"},
			[]string{"bad"}, []RuleTag{{"r2", "bad"}}, 0, 0},
		{"second rule joins an owned tag", []string{"bad"}, []RuleTag{{"r1", "bad"}}, map[string]string{"r1": "bad", "r2": "bad"},
			[]string{"bad"}, []RuleTag{{"r1", "bad"}, {"r2", "bad"}}, 0, 0},
		{"renamed rule tag swaps", []string{"old"}, []RuleTag{{"r1", "old"}}, map[string]string{"r1": "new"},
			[]string{"new"}, []RuleTag{{"r1", "new"}}, 1, 1},
		{"already removed by hand: nothing to do", []string{}, []RuleTag{games}, nil,
			[]string{}, nil, 0, 0},
	}
	for _, c := range cases {
		tags, final, added, removed := reconcileRuleTags(c.tags, c.owned, c.want)
		if !slices.Equal(nonNil(tags), nonNil(c.wantTags)) || !sameRuleTags(final, c.wantFinal) ||
			len(added) != c.wantAdded || len(removed) != c.wantGone {
			t.Errorf("%s: tags=%v final=%v added=%v removed=%v", c.name, tags, final, added, removed)
		}
	}
}
