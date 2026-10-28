package compliance

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/uem/uem/internal/protocol"
	"github.com/uem/uem/internal/server/store"
)

func ptr[T any](v T) *T { return &v }

func cond(field, op string, value any) store.Condition {
	raw, _ := json.Marshal(value)
	return store.Condition{Field: field, Op: op, Value: raw}
}

func sampleFacts() Facts {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	seen := now.Add(-30 * time.Minute)
	return Facts{
		Now: now,
		Device: store.Device{
			OSFamily: "windows", OSVersion: "10.0.22631", AgentVersion: "0.1.0",
			LastSeenAt: &seen, DiskPct: ptr(85.0), Tags: []string{"finance"},
		},
		Inventory: &protocol.Inventory{
			Security: protocol.Security{DiskEncrypted: ptr(false), FirewallEnabled: ptr(true)},
			Hardware: protocol.Hardware{MemoryTotalBytes: 16 << 30},
		},
		Software: []store.Software{{Name: "Google Chrome"}, {Name: "Zoom Workplace"}, {Name: "uTorrent"}, {Name: "Steam", Publisher: "Valve"}},
	}
}

func TestEvalCondition(t *testing.T) {
	f := sampleFacts()
	cases := []struct {
		name string
		c    store.Condition
		want bool
	}{
		{"encryption required fails", cond("security.disk_encrypted", "is_true", nil), false},
		{"firewall on", cond("security.firewall_enabled", "is_true", nil), true},
		{"unknown screen lock fails closed", cond("security.screen_lock_enabled", "is_true", nil), false},
		{"os version gte", cond("os.version", "version_gte", "10.0.19045"), true},
		{"os version gte numeric segments", cond("os.version", "version_gte", "10.0.100000"), false},
		{"os version lt", cond("os.version", "version_lt", "11"), true},
		{"disk free gte 10", cond("hardware.disk_free_pct", "gte", 10), true},
		{"disk free gte 20", cond("hardware.disk_free_pct", "gte", 20), false},
		{"number given as string", cond("hardware.memory_total_gb", "gte", "8"), true},
		{"last seen within 60m", cond("agent.last_seen_minutes", "lte", 60), true},
		{"blocked software present", cond("software.installed", "not_contains", "torrent"), false},
		{"required software present", cond("software.installed", "contains", "chrome"), true},
		{"no gambling apps", cond("software.gambling", "none_found", nil), true},
		{"games present", cond("software.games", "none_found", nil), false},
		{"games present any_found", cond("software.games", "any_found", nil), true},
		{"tag present", cond("device.tags", "contains", "finance"), true},
		{"os family case-insensitive", cond("os.family", "eq", "Windows"), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := EvalCondition(c.c, f); got.Passed != c.want {
				t.Fatalf("got %v (%s), want %v", got.Passed, got.Detail, c.want)
			}
		})
	}
}

func TestEvaluateMatchModes(t *testing.T) {
	f := sampleFacts()
	conds := []store.Condition{
		cond("security.disk_encrypted", "is_true", nil),
		cond("security.firewall_enabled", "is_true", nil),
	}
	if ok, _ := Evaluate(store.Rule{Match: "all", Conditions: conds}, f); ok {
		t.Error("all: expected failure")
	}
	if ok, _ := Evaluate(store.Rule{Match: "any", Conditions: conds}, f); !ok {
		t.Error("any: expected pass")
	}
}

func TestNoInventoryFailsClosed(t *testing.T) {
	f := sampleFacts()
	f.Inventory = nil
	if res := EvalCondition(cond("software.installed", "not_contains", "torrent"), f); res.Passed {
		t.Fatal("expected unknown inventory to fail")
	}
}

func TestApplies(t *testing.T) {
	g := "g1"
	d := store.Device{OSFamily: "linux", GroupID: &g}
	if !Applies(store.Rule{Enabled: true}, d) {
		t.Error("unscoped rule should apply")
	}
	if Applies(store.Rule{Enabled: false}, d) {
		t.Error("disabled rule should not apply")
	}
	if Applies(store.Rule{Enabled: true, OSFamilies: []string{"windows"}}, d) {
		t.Error("OS filter should exclude")
	}
	other := "g2"
	if Applies(store.Rule{Enabled: true, GroupID: &other}, d) {
		t.Error("group scope should exclude")
	}
}

func TestValidate(t *testing.T) {
	if err := Validate(nil); err == nil {
		t.Error("empty conditions should fail")
	}
	if err := Validate([]store.Condition{cond("nope", "eq", 1)}); err == nil {
		t.Error("unknown field should fail")
	}
	if err := Validate([]store.Condition{cond("os.version", "gt", "1")}); err == nil {
		t.Error("wrong operator should fail")
	}
	if err := Validate([]store.Condition{cond("hardware.disk_free_pct", "gte", "abc")}); err == nil {
		t.Error("non-numeric value should fail")
	}
	if err := Validate([]store.Condition{cond("security.disk_encrypted", "is_true", nil)}); err != nil {
		t.Errorf("valid rule rejected: %v", err)
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"10.0.22631", "10.0.19045", 1},
		{"14.1", "14.1.0", 0},
		{"v1.2.3", "1.2.10", -1},
		{"22.04", "22.04", 0},
		{"6.8.0-45-generic", "6.8.0-40-generic", 1},
	}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		sw   store.Software
		want string
	}{
		{store.Software{Name: "PokerStars"}, FlagGambling},
		{store.Software{Name: "bet365"}, FlagGambling},
		{store.Software{Name: "Some Casino Client"}, FlagGambling},
		{store.Software{Name: "Steam"}, FlagGame},
		{store.Software{Name: "Epic Games Launcher"}, FlagGame},
		{store.Software{Name: "League of Legends", Publisher: "Riot Games, Inc"}, FlagGame},
		{store.Software{Name: "Diablo IV", Publisher: "Blizzard Entertainment"}, FlagGame},
		{store.Software{Name: "aisleriot", Source: "dpkg"}, FlagGame},
		{store.Software{Name: "Chess", Source: "apple"}, ""},
		{store.Software{Name: "Alphabet Toolkit"}, ""},
		{store.Software{Name: "Microsoft Visual C++ 2015-2022 Redistributable", Publisher: "Microsoft Corporation"}, ""},
		{store.Software{Name: "Adobe Photoshop 2024"}, ""},
		{store.Software{Name: "betaflight-configurator"}, ""},
	}
	for _, c := range cases {
		if got := Classify(c.sw); got != c.want {
			t.Errorf("Classify(%q) = %q, want %q", c.sw.Name, got, c.want)
		}
	}
}

func TestCategory(t *testing.T) {
	cases := []struct {
		sw   store.Software
		want string
	}{
		{store.Software{Name: "Contacts", Source: "apple"}, CategorySystem},
		{store.Software{Name: "Slack", Source: "mac_app_store"}, CategoryThirdParty},
		{store.Software{Name: "Adobe Photoshop", Source: "identified_developer"}, CategoryThirdParty},
		{store.Software{Name: "Microsoft Visual C++ 2015-2022 Redistributable (x64)", Publisher: "Microsoft Corporation", Source: "registry"}, CategorySystem},
		{store.Software{Name: "Microsoft 365 Apps for enterprise - en-us", Publisher: "Microsoft Corporation", Source: "registry"}, CategoryThirdParty},
		{store.Software{Name: "HP Support Assistant", Publisher: "HP Inc.", Source: "registry"}, CategoryThirdParty},
		{store.Software{Name: "procps", Publisher: "Ubuntu Developers <ubuntu-devel-discuss@lists.ubuntu.com>", Source: "dpkg"}, CategorySystem},
		{store.Software{Name: "zabbix-agent", Publisher: "Zabbix Packager <info@zabbix.com>", Source: "dpkg"}, CategoryThirdParty},
		{store.Software{Name: "NewApp", Source: "unknown"}, CategoryThirdParty},
	}
	for _, c := range cases {
		if got := Category(c.sw); got != c.want {
			t.Errorf("Category(%q) = %q, want %q", c.sw.Name, got, c.want)
		}
	}
}
