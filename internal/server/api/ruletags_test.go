package api_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/uem/uem/internal/protocol"
)

func TestRuleTagsComeOffWhenDevicePasses(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	a := e.signup(t, "Tags")
	var tok idResp
	a.do("POST", "/api/enrollment-tokens", map[string]any{"description": "t"}, 201, &tok)
	dev := e.enroll(t, tok.Token).DeviceID
	other := e.enroll(t, tok.Token).DeviceID

	// "other" already carries the tag by hand before the rule exists.
	a.do("PATCH", "/api/devices/"+other, map[string]any{"tags": []string{"no-firewall", "loaner"}}, 200, nil)
	a.do("PATCH", "/api/devices/"+dev, map[string]any{"tags": []string{"loaner"}}, 200, nil)
	var rule idResp
	a.do("POST", "/api/compliance/rules", map[string]any{"name": "Firewall on", "severity": "high",
		"conditions": []map[string]any{{"field": "security.firewall_enabled", "op": "is_true", "value": nil}},
		"actions":    map[string]any{"alert": true, "webhook": false, "tag": " no-firewall ", "command": nil}}, 201, &rule)

	firewall := func(id string, on bool) {
		t.Helper()
		if err := e.store.SaveInventory(ctx, a.orgID, id, protocol.Inventory{CollectedAt: time.Now(),
			OS: protocol.OSInfo{Family: "linux"}, Security: protocol.Security{FirewallEnabled: &on}}); err != nil {
			t.Fatal(err)
		}
		if err := e.core.EvaluateDevice(ctx, a.orgID, id); err != nil {
			t.Fatal(err)
		}
	}
	type detail struct {
		Tags            []string
		ComplianceState string              `json:"compliance_state"`
		RuleTags        map[string][]string `json:"rule_tags"`
	}
	get := func(id string) detail {
		var d detail
		a.do("GET", "/api/devices/"+id, nil, 200, &d)
		return d
	}

	firewall(dev, false)
	firewall(other, false)
	d := get(dev)
	if d.ComplianceState != "non_compliant" || !slices.Equal(d.Tags, []string{"loaner", "no-firewall"}) {
		t.Fatalf("failing device: %+v", d)
	}
	if got := d.RuleTags["no-firewall"]; !slices.Equal(got, []string{"Firewall on"}) || d.RuleTags["loaner"] != nil {
		t.Fatalf("rule_tags = %v", d.RuleTags)
	}

	// Removing the tag by hand while the device still fails: it comes back.
	a.do("PATCH", "/api/devices/"+dev, map[string]any{"tags": []string{"loaner"}}, 200, nil)
	if d = get(dev); !slices.Contains(d.Tags, "no-firewall") {
		t.Fatalf("rule tag should return while failing: %v", d.Tags)
	}

	// Passing again takes the rule's tag off and leaves the hand-made one.
	firewall(dev, true)
	firewall(other, true)
	if d = get(dev); d.ComplianceState != "compliant" || !slices.Equal(d.Tags, []string{"loaner"}) || len(d.RuleTags) != 0 {
		t.Fatalf("passing device: %+v", d)
	}
	if o := get(other); !slices.Equal(o.Tags, []string{"no-firewall", "loaner"}) {
		t.Fatalf("a tag added by hand must stay: %v", o.Tags)
	}

	// Each automatic change is audit-logged.
	var audit struct {
		Items []struct{ Action string }
	}
	a.do("GET", "/api/audit?limit=200", nil, 200, &audit)
	adds, removes := 0, 0
	for _, it := range audit.Items {
		switch it.Action {
		case "device.tag.add":
			adds++
		case "device.tag.remove":
			removes++
		}
	}
	if adds != 2 || removes != 1 {
		t.Fatalf("audit: %d tag adds, %d removes; want 2 and 1", adds, removes)
	}

	// Deleting the rule while a device fails it also takes its tag off.
	firewall(dev, false)
	a.do("DELETE", "/api/compliance/rules/"+rule.ID, nil, 200, nil)
	if err := e.core.EvaluateDevice(ctx, a.orgID, dev); err != nil {
		t.Fatal(err)
	}
	if d = get(dev); slices.Contains(d.Tags, "no-firewall") {
		t.Fatalf("deleted rule's tag should come off: %v", d.Tags)
	}
}
