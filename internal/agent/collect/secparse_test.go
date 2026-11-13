package collect

import (
	"testing"

	"github.com/uem/uem/internal/protocol"
)

func TestAVProductState(t *testing.T) {
	for _, c := range []struct {
		state  int64
		rt, ok bool
	}{
		{397568, true, true},  // 0x061100: Defender on, up to date
		{393472, false, true}, // 0x060100: Defender off (passive), up to date
		{266240, true, true},  // 0x041000: third-party on, up to date
		{266256, true, false}, // 0x041010: on, definitions out of date
		{262144, false, true}, // 0x040000: off
	} {
		rt, ok := avProductState(c.state)
		if rt != c.rt || ok != c.ok {
			t.Errorf("avProductState(%#x) = %v, %v; want %v, %v", c.state, rt, ok, c.rt, c.ok)
		}
	}
}

func TestParseWindowsSec(t *testing.T) {
	// Defender active, Private profile off, UAC on, user a direct local admin.
	out := `{"fw":[{"name":"Domain","enabled":true},{"name":"Private","enabled":false},{"name":"Public","enabled":true}],
	"av":{"name":"Windows Defender","state":397568},
	"mp":{"rt":true,"age":1,"tamper":true,"mode":"Normal","ver":"1.421.120.0"},
	"uac":1,"user":"ZODIAC\\alberto.espinosa","computer":"CL-ALESPINOSA",
	"admins":["Administrator","ZODIAC\\alberto.espinosa"]}`
	x := parseWindowsSec(out)
	if !x.OK || len(x.FirewallProfiles) != 3 || *x.FirewallProfiles[1].Enabled {
		t.Fatalf("firewall = %+v", x.FirewallProfiles)
	}
	a := x.AV
	if a == nil || a.Name != "Windows Defender" || !*a.Realtime || !*a.UpToDate || *a.SignatureAgeDays != 1 || !*a.TamperProtected || a.Version != "1.421.120.0" {
		t.Fatalf("av = %+v", a)
	}
	if len(x.OSIntegrity) != 1 || x.OSIntegrity[0].Name != "User Account Control" || !*x.OSIntegrity[0].Enabled {
		t.Fatalf("integrity = %+v", x.OSIntegrity)
	}
	if x.User != `ZODIAC\alberto.espinosa` || x.UserAdmin == nil || !*x.UserAdmin {
		t.Fatalf("user = %q %v", x.User, x.UserAdmin)
	}

	// McAfee active and Defender passive: McAfee is reported, without Defender's details.
	m := parseWindowsSec(`{"fw":[],"av":[{"name":"Windows Defender","state":393472},{"name":"McAfee Security","state":266240}],
	"mp":{"rt":false,"age":3},"uac":0,"user":"LAPTOP-1\\arnold","computer":"LAPTOP-1","admins":["Administrator","arnold"]}`)
	if m.AV == nil || m.AV.Name != "McAfee Security" || !*m.AV.Realtime || m.AV.SignatureAgeDays != nil {
		t.Fatalf("mcafee = %+v", m.AV)
	}
	if *m.OSIntegrity[0].Enabled || m.UserAdmin == nil || !*m.UserAdmin {
		t.Fatalf("uac/admin = %+v %v", m.OSIntegrity, m.UserAdmin)
	}

	// Standard user; a domain admins group is in the local group: can't tell.
	if got := windowsAdmin(`CORP\jane`, "PC1", []string{"Administrator", `CORP\Domain Admins`}); got != nil {
		t.Fatalf("via domain group = %v", *got)
	}
	if got := windowsAdmin(`PC1\jane`, "PC1", []string{"Administrator", "itadmin"}); got == nil || *got {
		t.Fatalf("standard = %v", got)
	}
	if parseWindowsSec("garbage").OK {
		t.Fatal("garbage should not be OK")
	}
}

func TestMacSecParsers(t *testing.T) {
	if v := parseCsrutil("System Integrity Protection status: enabled."); v == nil || !*v {
		t.Fatal("csrutil enabled")
	}
	if v := parseCsrutil("System Integrity Protection status: disabled."); v == nil || *v {
		t.Fatal("csrutil disabled")
	}
	if v := parseSpctl("assessments enabled"); v == nil || !*v {
		t.Fatal("spctl")
	}
	if v := parseSpctl("assessments disabled"); v == nil || *v {
		t.Fatal("spctl off")
	}
	for out, want := range map[string]bool{
		"Firewall is disabled. (State = 0)":                                        false,
		"Firewall is enabled. (State = 1)":                                         true,
		"Firewall stealth mode is off":                                             false,
		"Firewall stealth mode is on":                                              true,
		"Firewall is blocking all non-essential incoming connections. (State = 2)": true,
	} {
		if v := parseSocketfilterOn(out); v == nil || *v != want {
			t.Errorf("socketfilterfw %q = %v", out, v)
		}
	}
	if v := parseDseditgroup("yes albertoespinosa is a member of admin"); v == nil || !*v {
		t.Fatal("dseditgroup yes")
	}
	if v := parseDseditgroup("no mark is NOT a member of admin"); v == nil || *v {
		t.Fatal("dseditgroup no")
	}
	if v := parseSoftwareUpdate("Software Update Tool\n\nFinding available software\nNo new software available."); v == nil || *v != 0 {
		t.Fatal("no updates")
	}
	two := `Software Update Tool

Finding available software
Software Update found the following new or updated software:
* Label: macOS Tahoe 26.1-25B78
	Title: macOS Tahoe 26.1, Version: 26.1, Size: 1562148KiB, Recommended: YES, Action: restart,
* Label: Safari26.1-26.1
	Title: Safari, Version: 26.1, Size: 140223KiB, Recommended: YES,`
	if v := parseSoftwareUpdate(two); v == nil || *v != 2 {
		t.Fatalf("two updates = %v", v)
	}
	if parseSoftwareUpdate("Software Update Tool\n\nFinding available software\n") != nil {
		t.Fatal("no answer should be unknown")
	}
}

func TestLinuxSecParsers(t *testing.T) {
	if s := parseGetenforce("Enforcing\n"); s == nil || !*s.Enabled {
		t.Fatal("enforcing")
	}
	if s := parseGetenforce("Permissive"); s == nil || *s.Enabled || s.Detail == "" {
		t.Fatal("permissive")
	}
	if v := parseIDGroups("itadmin adm cdrom sudo dip plugdev"); v == nil || !*v {
		t.Fatal("sudo")
	}
	if v := parseIDGroups("jane users"); v == nil || *v {
		t.Fatal("standard")
	}
}

func TestSecExtrasApply(t *testing.T) {
	s := protocol.Security{}
	off, on := false, true
	secExtras{OK: true, FirewallProfiles: []protocol.SecuritySetting{{Name: "Domain", Enabled: &on}, {Name: "Private", Enabled: &off}},
		AV: &protocol.AVStatus{Name: "McAfee Security"}, PendingUpdates: ptr(2), EncryptionVolume: "C:"}.apply(&s)
	if !s.Details || s.FirewallEnabled == nil || *s.FirewallEnabled || *s.Antivirus != "McAfee Security" || *s.PendingUpdates != 2 || s.EncryptionVolume != "C:" {
		t.Fatalf("security = %+v", s)
	}
	// An already-read value wins over the extras' copy.
	s2 := protocol.Security{FirewallEnabled: &on, PendingUpdates: ptr(0)}
	secExtras{OK: true, FirewallProfiles: []protocol.SecuritySetting{{Name: "x", Enabled: &off}}, PendingUpdates: ptr(5)}.apply(&s2)
	if !*s2.FirewallEnabled || *s2.PendingUpdates != 0 {
		t.Fatalf("kept = %+v", s2)
	}
	s3 := protocol.Security{}
	secExtras{}.apply(&s3)
	if s3.Details {
		t.Fatal("not collected")
	}
}
