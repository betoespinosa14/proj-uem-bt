package auth

import "testing"

func TestPasswordRoundTrip(t *testing.T) {
	h, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckPassword(h, "correct horse battery staple"); err != nil {
		t.Fatalf("expected match: %v", err)
	}
	if err := CheckPassword(h, "wrong"); err == nil {
		t.Fatal("expected mismatch")
	}
	if err := CheckPassword("garbage", "x"); err == nil {
		t.Fatal("expected mismatch on malformed hash")
	}
}

func TestRolePermissionMatrix(t *testing.T) {
	cases := []struct {
		role, perm string
		want       bool
	}{
		{"owner", PermSettingsWrite, true},
		{"admin", PermSettingsWrite, false},
		{"admin", PermUsersWrite, true},
		{"helpdesk", PermCommandsRun, true},
		{"helpdesk", PermComplianceWrite, false},
		{"helpdesk", PermAuditRead, false},
		{"auditor", PermAuditRead, true},
		{"auditor", PermCommandsRun, false},
		{"auditor", PermDevicesWrite, false},
		{"nobody", PermDevicesRead, false},
	}
	for _, c := range cases {
		if got := Can(c.role, c.perm); got != c.want {
			t.Errorf("Can(%q, %q) = %v, want %v", c.role, c.perm, got, c.want)
		}
	}
}
