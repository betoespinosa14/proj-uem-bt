package api_test

// Integration tests against a real Postgres. Set UEM_TEST_DATABASE_URL to a
// database the tests may migrate and write to (they use unique emails per
// run, so a shared dev database works); the tests skip otherwise.

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/uem/uem/internal/protocol"
	"github.com/uem/uem/internal/server/api"
	"github.com/uem/uem/internal/server/audit"
	"github.com/uem/uem/internal/server/core"
	"github.com/uem/uem/internal/server/hub"
	"github.com/uem/uem/internal/server/migrations"
	"github.com/uem/uem/internal/server/notify"
	"github.com/uem/uem/internal/server/scheduler"
	"github.com/uem/uem/internal/server/store"
)

type env struct {
	srv   *httptest.Server
	store *store.Store
	audit *audit.Log
}

func setup(t *testing.T) *env {
	t.Helper()
	dsn := os.Getenv("UEM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("UEM_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := migrations.Up(ctx, stdlib.OpenDBFromPool(pool)); err != nil {
		t.Fatal(err)
	}
	st := store.New(pool)
	al := audit.New(st)
	c := &core.Core{Store: st, Audit: al, Hub: hub.New(), Hooks: notify.New(st, false),
		TelemetryInterval: time.Minute, InventoryInterval: time.Hour}
	sched := scheduler.New(c)
	srv := httptest.NewServer(api.New(api.Config{AllowSignup: true}, c, sched).Handler())
	t.Cleanup(srv.Close)
	return &env{srv: srv, store: st, audit: al}
}

// client is a logged-in browser session.
type client struct {
	t     *testing.T
	base  string
	http  *http.Client
	csrf  string
	orgID string
}

func (e *env) signup(t *testing.T, org string) *client {
	jar, _ := cookiejar.New(nil)
	c := &client{t: t, base: e.srv.URL, http: &http.Client{Jar: jar}}
	email := fmt.Sprintf("%s-%d@test.example", strings.ToLower(org), time.Now().UnixNano())
	var me struct {
		Org struct{ ID string } `json:"org"`
	}
	c.do("POST", "/api/auth/signup", map[string]string{"org_name": org, "name": "Admin", "email": email, "password": "correct-horse-1"}, 200, &me)
	c.orgID = me.Org.ID
	u, _ := url.Parse(e.srv.URL)
	for _, ck := range jar.Cookies(u) {
		if ck.Name == "uem_csrf" {
			c.csrf = ck.Value
		}
	}
	return c
}

func (c *client) do(method, path string, body any, wantStatus int, out any) {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, r)
	req.Header.Set("Content-Type", "application/json")
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		c.t.Fatalf("%s %s: status %d, want %d: %s", method, path, resp.StatusCode, wantStatus, data)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			c.t.Fatalf("%s %s: decode: %v: %s", method, path, err, data)
		}
	}
}

type idResp struct {
	ID    string `json:"id"`
	Token string `json:"token"`
}

func (e *env) enroll(t *testing.T, token string) protocol.EnrollResponse {
	b, _ := json.Marshal(protocol.EnrollRequest{Token: token, Facts: protocol.Facts{Hostname: "test-host", OSFamily: "linux", OSName: "Ubuntu", OSVersion: "24.04"}})
	resp, err := http.Post(e.srv.URL+"/api/agent/enroll", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out protocol.EnrollResponse
	if resp.StatusCode != 200 {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("enroll: %d %s", resp.StatusCode, data)
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out
}

func TestTenantIsolation(t *testing.T) {
	e := setup(t)
	a, b := e.signup(t, "OrgA"), e.signup(t, "OrgB")

	var group, script, rule, tok idResp
	a.do("POST", "/api/groups", map[string]any{"name": "G", "blocked_apps": []string{}}, 201, &group)
	a.do("POST", "/api/scripts", map[string]any{"name": "S", "shell": "sh", "body": "id"}, 201, &script)
	a.do("POST", "/api/compliance/rules", map[string]any{"name": "R", "severity": "low",
		"conditions": []map[string]any{{"field": "security.firewall_enabled", "op": "is_true", "value": nil}}}, 201, &rule)
	a.do("POST", "/api/enrollment-tokens", map[string]any{"description": "t"}, 201, &tok)
	dev := e.enroll(t, tok.Token)
	var cmd idResp
	a.do("POST", "/api/devices/"+dev.DeviceID+"/commands", map[string]any{"type": "lock", "payload": map[string]any{}}, 201, &cmd)

	// Org B can see none of it, and cannot act on it.
	for _, p := range []string{
		"/api/devices/" + dev.DeviceID, "/api/commands/" + cmd.ID, "/api/scripts/" + script.ID,
		"/api/compliance/rules/" + rule.ID, "/api/enrollment-tokens/" + tok.ID + "/install",
	} {
		b.do("GET", p, nil, 404, nil)
	}
	b.do("POST", "/api/devices/"+dev.DeviceID+"/commands", map[string]any{"type": "lock", "payload": map[string]any{}}, 404, nil)
	b.do("PATCH", "/api/devices/"+dev.DeviceID, map[string]any{"name": "pwned"}, 404, nil)
	b.do("DELETE", "/api/devices/"+dev.DeviceID, nil, 404, nil)
	b.do("PATCH", "/api/groups/"+group.ID, map[string]any{"name": "pwned"}, 404, nil)
	b.do("DELETE", "/api/scripts/"+script.ID, nil, 404, nil)
	b.do("DELETE", "/api/enrollment-tokens/"+tok.ID, nil, 404, nil)
	// B cannot run A's library script on its own devices, nor move devices into A's group.
	var tokB idResp
	b.do("POST", "/api/enrollment-tokens", map[string]any{"description": "b"}, 201, &tokB)
	devB := e.enroll(t, tokB.Token)
	b.do("POST", "/api/devices/"+devB.DeviceID+"/commands", map[string]any{"type": "run_script", "payload": map[string]any{"script_id": script.ID}}, 400, nil)
	b.do("PATCH", "/api/devices/"+devB.DeviceID, map[string]any{"group_id": group.ID}, 400, nil)
	var bulk struct{ Items []any }
	b.do("POST", "/api/commands/bulk", map[string]any{"device_ids": []string{dev.DeviceID}, "type": "lock", "payload": map[string]any{}}, 201, &bulk)
	if len(bulk.Items) != 0 {
		t.Fatal("bulk command reached another org's device")
	}

	var list struct{ Items []struct{ ID string } }
	b.do("GET", "/api/devices", nil, 200, &list)
	if len(list.Items) != 1 || list.Items[0].ID != devB.DeviceID {
		t.Fatalf("org B device list leaked or wrong: %+v", list.Items)
	}
	a.do("GET", "/api/devices/"+dev.DeviceID, nil, 200, nil) // still intact for A
}

func TestEnrollmentTokenLimits(t *testing.T) {
	e := setup(t)
	a := e.signup(t, "Limits")
	var tok idResp
	a.do("POST", "/api/enrollment-tokens", map[string]any{"description": "one", "max_uses": 1}, 201, &tok)
	e.enroll(t, tok.Token)
	b, _ := json.Marshal(protocol.EnrollRequest{Token: tok.Token})
	resp, _ := http.Post(e.srv.URL+"/api/agent/enroll", "application/json", bytes.NewReader(b))
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("exhausted token: got %d, want 401", resp.StatusCode)
	}
	var tok2 idResp
	a.do("POST", "/api/enrollment-tokens", map[string]any{"description": "two"}, 201, &tok2)
	a.do("DELETE", "/api/enrollment-tokens/"+tok2.ID, nil, 200, nil)
	b, _ = json.Marshal(protocol.EnrollRequest{Token: tok2.Token})
	resp, _ = http.Post(e.srv.URL+"/api/agent/enroll", "application/json", bytes.NewReader(b))
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("revoked token: got %d, want 401", resp.StatusCode)
	}
}

func TestRBACAndCSRF(t *testing.T) {
	e := setup(t)
	owner := e.signup(t, "Rbac")
	email := fmt.Sprintf("helpdesk-%d@test.example", time.Now().UnixNano())
	owner.do("POST", "/api/users", map[string]any{"email": email, "name": "H", "role": "helpdesk", "password": "correct-horse-1"}, 201, nil)

	jar, _ := cookiejar.New(nil)
	hd := &client{t: t, base: e.srv.URL, http: &http.Client{Jar: jar}}
	hd.do("POST", "/api/auth/login", map[string]string{"email": email, "password": "correct-horse-1"}, 200, nil)
	u, _ := url.Parse(e.srv.URL)
	for _, ck := range jar.Cookies(u) {
		if ck.Name == "uem_csrf" {
			hd.csrf = ck.Value
		}
	}
	hd.do("GET", "/api/devices", nil, 200, nil)
	hd.do("POST", "/api/scripts", map[string]any{"name": "S", "shell": "sh", "body": "id"}, 403, nil)
	hd.do("GET", "/api/audit", nil, 403, nil)
	hd.do("PUT", "/api/usage/infrastructure", map[string]any{"app": "Parsec"}, 403, nil)
	hd.do("POST", "/api/users", map[string]any{"email": "x@y.z", "role": "owner", "password": "correct-horse-1"}, 403, nil)

	// Missing CSRF token is rejected even with a valid session.
	saved := owner.csrf
	owner.csrf = ""
	owner.do("POST", "/api/groups", map[string]any{"name": "nope"}, 403, nil)
	owner.csrf = saved
}

func TestAgentCommandRoundTripAndAudit(t *testing.T) {
	e := setup(t)
	a := e.signup(t, "Agent")
	var tok idResp
	a.do("POST", "/api/enrollment-tokens", map[string]any{"description": "t"}, 201, &tok)
	dev := e.enroll(t, tok.Token)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(e.srv.URL, "http") + "/api/agent/ws"

	// Wrong secret is refused.
	_, resp, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Device " + dev.DeviceID + ":wrong"}}})
	if err == nil || resp == nil || resp.StatusCode != 401 {
		t.Fatalf("expected 401 for bad secret, got err=%v", err)
	}

	c, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Device " + dev.DeviceID + ":" + dev.DeviceSecret}}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	send := func(typ string, v any) {
		f, _ := protocol.Encode(typ, v)
		if err := c.Write(ctx, websocket.MessageText, f); err != nil {
			t.Fatal(err)
		}
	}
	send(protocol.TypeHello, protocol.Hello{ProtocolVersion: protocol.Version, AgentVersion: "test"})
	fw := false
	send(protocol.TypeInventory, protocol.Inventory{OS: protocol.OSInfo{Family: "linux", Version: "24.04"},
		Security: protocol.Security{FirewallEnabled: &fw}, Software: []protocol.Software{{Name: "curl", Version: "8"}}})

	// A rule that fails must flip the device to non_compliant and open an alert.
	a.do("POST", "/api/compliance/rules", map[string]any{"name": "FW", "severity": "high",
		"conditions": []map[string]any{{"field": "security.firewall_enabled", "op": "is_true", "value": nil}},
		"actions":    map[string]any{"alert": true, "webhook": false, "tag": nil, "command": nil}}, 201, nil)
	// Inventory is ingested asynchronously; poll until it lands and is evaluated.
	var d struct {
		ComplianceState string `json:"compliance_state"`
	}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		a.do("POST", "/api/compliance/evaluate", nil, 200, nil)
		a.do("GET", "/api/devices/"+dev.DeviceID, nil, 200, &d)
		if d.ComplianceState == "non_compliant" {
			break
		}
	}
	if d.ComplianceState != "non_compliant" {
		t.Fatalf("compliance_state = %q", d.ComplianceState)
	}
	resp, err = a.http.Get(a.base + "/api/devices/" + dev.DeviceID + "/software.csv")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(resp.Body).ReadAll()
	resp.Body.Close()
	if err != nil || len(rows) != 2 || rows[0][1] != "name" || rows[1][1] != "curl" || rows[1][4] != "Third-party" {
		t.Fatalf("software.csv rows = %v, err = %v", rows, err)
	}

	// The dashboard lists the failing rule and counts the device as online for its OS.
	var dash struct {
		ByOS []struct {
			OSFamily      string `json:"os_family"`
			Count, Online int
		} `json:"by_os"`
		FailingRules []struct {
			Name, Severity string
			Devices        int
		} `json:"failing_rules"`
	}
	a.do("GET", "/api/dashboard", nil, 200, &dash)
	if len(dash.FailingRules) != 1 || dash.FailingRules[0].Name != "FW" || dash.FailingRules[0].Severity != "high" || dash.FailingRules[0].Devices != 1 {
		t.Fatalf("dashboard failing_rules = %+v", dash.FailingRules)
	}
	if len(dash.ByOS) != 1 || dash.ByOS[0].OSFamily != "linux" || dash.ByOS[0].Count != 1 || dash.ByOS[0].Online != 1 {
		t.Fatalf("dashboard by_os = %+v", dash.ByOS)
	}

	var alerts struct{ Items []struct{ Severity string } }
	a.do("GET", "/api/alerts?status=open", nil, 200, &alerts)
	if len(alerts.Items) != 1 || alerts.Items[0].Severity != "high" {
		t.Fatalf("alerts = %+v", alerts.Items)
	}

	var cmd idResp
	a.do("POST", "/api/devices/"+dev.DeviceID+"/commands", map[string]any{"type": "run_script",
		"payload": map[string]any{"shell": "sh", "script": "echo hi"}}, 201, &cmd)
	// Read frames until the command arrives (policy comes first).
	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		env, _ := protocol.Decode(data)
		if env.Type != protocol.TypeCommand {
			continue
		}
		var got protocol.Command
		_ = json.Unmarshal(env.Data, &got)
		if got.ID != cmd.ID {
			t.Fatalf("got command %s, want %s", got.ID, cmd.ID)
		}
		break
	}
	zero := 0
	send(protocol.TypeCommandResult, protocol.CommandResult{ID: cmd.ID, Status: protocol.StatusSucceeded, ExitCode: &zero, Stdout: "hi\n"})
	var got struct{ Status, Stdout string }
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		a.do("GET", "/api/commands/"+cmd.ID, nil, 200, &got)
		if got.Status == protocol.StatusSucceeded {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got.Status != protocol.StatusSucceeded || got.Stdout != "hi\n" {
		t.Fatalf("command = %+v", got)
	}

	var v audit.VerifyResult
	a.do("GET", "/api/audit/verify", nil, 200, &v)
	if !v.Valid || v.Checked < 5 {
		t.Fatalf("audit verify = %+v", v)
	}
	// The DB itself refuses to rewrite history.
	if _, err := e.store.Pool.Exec(ctx, `UPDATE audit_log SET action = 'x' WHERE org_id = $1`, a.orgID); err == nil {
		t.Fatal("audit_log update was not blocked")
	}
}

func TestDeviceExportCSV(t *testing.T) {
	e := setup(t)
	a, b := e.signup(t, "ExportA"), e.signup(t, "ExportB")
	var tok idResp
	a.do("POST", "/api/enrollment-tokens", map[string]any{"description": "t"}, 201, &tok)
	e.enroll(t, tok.Token)
	b.do("POST", "/api/enrollment-tokens", map[string]any{"description": "t"}, 201, &tok)
	e.enroll(t, tok.Token)

	resp, err := a.http.Get(a.base + "/api/devices/export.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/csv") {
		t.Fatalf("status %d, content-type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	rows, err := csv.NewReader(resp.Body).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0][0] != "name" || rows[1][1] != "test-host" {
		t.Fatalf("rows = %v", rows) // header + only org A's device
	}
}

func TestUsageTracking(t *testing.T) {
	e := setup(t)
	a := e.signup(t, "Usage")
	var tok idResp
	a.do("POST", "/api/enrollment-tokens", map[string]any{"description": "t"}, 201, &tok)
	dev := e.enroll(t, tok.Token)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(e.srv.URL, "http") + "/api/agent/ws"
	c, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Device " + dev.DeviceID + ":" + dev.DeviceSecret}}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	send := func(typ string, v any) {
		f, _ := protocol.Encode(typ, v)
		if err := c.Write(ctx, websocket.MessageText, f); err != nil {
			t.Fatal(err)
		}
	}
	send(protocol.TypeHello, protocol.Hello{ProtocolVersion: protocol.Version, AgentVersion: "test"})
	send(protocol.TypeInventory, protocol.Inventory{OS: protocol.OSInfo{Family: "macos"}, Software: []protocol.Software{
		{Name: "Slack", Source: "mac_app_store"}, {Name: "Zoom", Source: "identified_developer"}, {Name: "Contacts", Source: "apple"},
	}})
	today := time.Now().Format(time.DateOnly)
	report := protocol.Usage{Entries: []protocol.UsageEntry{
		{Date: today, App: "Slack", User: "ana", Minutes: 90, ActiveSeconds: 1800, LastSeen: time.Now()},
		{Date: today, App: "Slack", User: "ben", ActiveSeconds: 600, LastSeen: time.Now()}, // active only: probe saw it, sampler didn't
	}}

	type row struct {
		App, User     string
		DaysUsed      int `json:"days_used"`
		Minutes       int
		ActiveSeconds int `json:"active_seconds"`
		Installed     bool
		Users         int
		Devices       int
		InstalledOn   int `json:"installed_on"`
	}
	var usage struct{ Items []row }
	poll := func(path string, want func() bool) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
			a.do("GET", path, nil, 200, &usage)
			if want() {
				return
			}
		}
		t.Fatalf("%s: %+v", path, usage.Items)
	}

	// Tracking is off by default: reports are dropped.
	send(protocol.TypeUsage, report)
	time.Sleep(300 * time.Millisecond)
	a.do("GET", "/api/devices/"+dev.DeviceID+"/usage", nil, 200, &usage)
	if len(usage.Items) != 0 {
		t.Fatalf("usage recorded while tracking off: %+v", usage.Items)
	}

	var org struct {
		UsageTracking bool `json:"usage_tracking"`
	}
	a.do("PATCH", "/api/org", map[string]any{"usage_tracking": true}, 200, &org)
	if !org.UsageTracking {
		t.Fatal("usage_tracking not enabled")
	}
	send(protocol.TypeUsage, report)
	send(protocol.TypeUsage, report) // accumulates into the same day
	poll("/api/devices/"+dev.DeviceID+"/usage", func() bool {
		return len(usage.Items) == 2 && usage.Items[0].User == "ana" && usage.Items[0].Minutes == 180 &&
			usage.Items[0].ActiveSeconds == 3600 && usage.Items[0].DaysUsed == 1 && usage.Items[1].ActiveSeconds == 1200
	})
	// Totals per app across users.
	poll("/api/devices/"+dev.DeviceID+"/usage?group=app", func() bool {
		return len(usage.Items) == 1 && usage.Items[0].User == "" && usage.Items[0].Users == 2 && usage.Items[0].ActiveSeconds == 4800
	})
	poll("/api/usage?group=app&app=slack", func() bool {
		u := usage.Items
		return len(u) == 1 && u[0].Users == 2 && u[0].Devices == 1 && u[0].InstalledOn == 1 && u[0].ActiveSeconds == 4800 && u[0].Minutes == 180
	})
	poll("/api/usage?group=app&unused=true", func() bool {
		u := usage.Items
		return len(u) == 1 && u[0].App == "Zoom" && u[0].InstalledOn == 1 && u[0].Devices == 0
	})

	// Fleet: Slack is used; Zoom is installed but unused; Contacts is a system app.
	poll("/api/usage?app=slack", func() bool {
		return len(usage.Items) == 2 && usage.Items[0].User == "ana" && usage.Items[0].Installed
	})
	poll("/api/usage?unused=true", func() bool {
		return len(usage.Items) == 1 && usage.Items[0].App == "Zoom" && usage.Items[0].User == ""
	})

	resp, err := a.http.Get(a.base + "/api/usage/export.csv?app=slack")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(resp.Body).ReadAll()
	resp.Body.Close()
	// app, device, user, installed, active_hours, open_hours, ...
	if err != nil || len(rows) != 3 || rows[0][4] != "active_hours" || rows[1][2] != "ana" || rows[1][4] != "1.00" || rows[1][5] != "3.0" {
		t.Fatalf("usage csv = %v, err = %v", rows, err)
	}

	// Dashboard summary: same data, totalled. A process that only ever ran in
	// the background (carl's Dropbox: open, never active) is reported apart
	// from foreground use instead of inflating open time.
	send(protocol.TypeUsage, protocol.Usage{Entries: []protocol.UsageEntry{
		{Date: today, App: "Dropbox", User: "carl", Minutes: 60, LastSeen: time.Now()},
		{Date: today, App: "Contacts", User: "carl", Minutes: 30, LastSeen: time.Now()}, // installed macOS built-in
	}})
	type stat struct {
		DeviceNames                   []string `json:"device_names"`
		App, User                     string
		ActiveSeconds                 int `json:"active_seconds"`
		Minutes, Users, Apps, Devices int
	}
	var sum struct {
		Totals struct {
			ActiveSeconds int `json:"active_seconds"`
			OpenMinutes   int `json:"open_minutes"`
			AppsUsed      int `json:"apps_used"`
			ActiveUsers   int `json:"active_users"`
			Devices       int
			UnusedApps    int `json:"unused_apps"`
			BgMinutes     int `json:"background_minutes"`
			BgApps        int `json:"background_apps"`
			BgThirdParty  int `json:"background_third_party"`
		}
		Daily []struct {
			Day           string
			ActiveSeconds int `json:"active_seconds"`
			Minutes       int
			Devices       int
		}
		TopApps    []stat `json:"top_apps"`
		TopUsers   []stat `json:"top_users"`
		IdleApps   []stat `json:"idle_apps"`
		Background []struct {
			App, User  string
			Minutes    int
			ThirdParty bool `json:"third_party"`
		}
	}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		a.do("GET", "/api/usage/summary", nil, 200, &sum)
		if len(sum.Background) == 2 && sum.Totals.BgApps == 2 { // usage is stored asynchronously
			break
		}
	}
	tt := sum.Totals
	if tt.ActiveSeconds != 4800 || tt.OpenMinutes != 180 || tt.AppsUsed != 1 || tt.ActiveUsers != 2 || tt.Devices != 1 ||
		tt.UnusedApps != 1 || tt.BgMinutes != 90 || tt.BgApps != 2 || tt.BgThirdParty != 1 {
		t.Fatalf("summary totals = %+v", tt)
	}
	if len(sum.Daily) != 1 || sum.Daily[0].ActiveSeconds != 4800 || sum.Daily[0].Minutes != 180 || sum.Daily[0].Devices != 1 ||
		len(sum.TopApps) != 1 || sum.TopApps[0].App != "Slack" || sum.TopApps[0].Users != 2 ||
		len(sum.IdleApps) != 1 || sum.IdleApps[0].Minutes != 180 ||
		len(sum.TopUsers) != 2 || sum.TopUsers[0].User != "ana" || sum.TopUsers[0].ActiveSeconds != 3600 || sum.TopUsers[0].Apps != 1 ||
		len(sum.TopUsers[0].DeviceNames) != 1 ||
		len(sum.Background) != 2 || sum.Background[0].App != "Dropbox" || sum.Background[0].User != "carl" || sum.Background[0].Minutes != 60 ||
		!sum.Background[0].ThirdParty || sum.Background[1].App != "Contacts" || sum.Background[1].ThirdParty {
		t.Fatalf("summary = %+v", sum)
	}
	// The app search narrows it: Zoom was never used.
	a.do("GET", "/api/usage/summary?app=zoom", nil, 200, &sum)
	if sum.Totals.ActiveSeconds != 0 || len(sum.Daily) != 0 || len(sum.TopApps) != 0 || sum.Totals.UnusedApps != 1 {
		t.Fatalf("summary?app=zoom = %+v", sum)
	}

	// Marking an app as infrastructure takes its open time out of the idle
	// ranking and open totals; its active time still counts.
	a.do("PUT", "/api/usage/infrastructure", map[string]any{"app": "slack"}, 204, nil)
	a.do("PUT", "/api/usage/infrastructure", map[string]any{"app": "Slack"}, 204, nil) // idempotent, any case
	var infra struct{ Items []struct{ App string } }
	a.do("GET", "/api/usage/infrastructure", nil, 200, &infra)
	if len(infra.Items) != 1 {
		t.Fatalf("infrastructure = %+v", infra.Items)
	}
	var withInfra struct {
		Totals struct {
			ActiveSeconds int `json:"active_seconds"`
			OpenMinutes   int `json:"open_minutes"`
		}
		TopApps        []stat `json:"top_apps"`
		IdleApps       []stat `json:"idle_apps"`
		Infrastructure []stat `json:"infrastructure"`
	}
	a.do("GET", "/api/usage/summary", nil, 200, &withInfra)
	if withInfra.Totals.ActiveSeconds != 4800 || withInfra.Totals.OpenMinutes != 0 || len(withInfra.IdleApps) != 0 ||
		len(withInfra.TopApps) != 1 || len(withInfra.Infrastructure) != 1 || withInfra.Infrastructure[0].Minutes != 180 {
		t.Fatalf("summary with Slack as infrastructure = %+v", withInfra)
	}
	a.do("DELETE", "/api/usage/infrastructure?app=SLACK", nil, 204, nil)
	a.do("GET", "/api/usage/summary", nil, 200, &withInfra)
	if withInfra.Totals.OpenMinutes != 180 || len(withInfra.IdleApps) != 1 || len(withInfra.Infrastructure) != 0 {
		t.Fatalf("summary after restoring Slack = %+v", withInfra)
	}

	// Reclaim list: installed third-party apps unused in the period, per
	// device. Slack is used; Contacts is a system app; Zoom is the one.
	resp, err = a.http.Get(a.base + "/api/usage/reclaim.csv")
	if err != nil {
		t.Fatal(err)
	}
	rows, err = csv.NewReader(resp.Body).ReadAll()
	resp.Body.Close()
	if err != nil || len(rows) != 2 || rows[0][0] != "app" || rows[0][4] != "device_user" || rows[1][0] != "Zoom" || rows[1][6] != "never" {
		t.Fatalf("reclaim csv = %v, err = %v", rows, err)
	}

	// Group filter: the test device isn't in this group, so everything is empty.
	var grp idResp
	a.do("POST", "/api/groups", map[string]any{"name": "Accounting"}, 201, &grp)
	q := "?group_id=" + grp.ID
	a.do("GET", "/api/usage/summary"+q, nil, 200, &sum)
	if sum.Totals.ActiveSeconds != 0 || len(sum.TopApps) != 0 || sum.Totals.UnusedApps != 0 || len(sum.Background) != 0 {
		t.Fatalf("summary for an empty group = %+v", sum)
	}
	a.do("GET", "/api/usage"+q, nil, 200, &usage)
	if len(usage.Items) != 0 {
		t.Fatalf("usage for an empty group = %+v", usage.Items)
	}
	resp, err = a.http.Get(a.base + "/api/usage/reclaim.csv" + q)
	if err != nil {
		t.Fatal(err)
	}
	rows, err = csv.NewReader(resp.Body).ReadAll()
	resp.Body.Close()
	if err != nil || len(rows) != 1 {
		t.Fatalf("reclaim csv for an empty group = %v, err = %v", rows, err)
	}
	a.do("GET", "/api/usage/summary?group_id=nope", nil, 400, nil)

	// Idle threshold: 1–15 minutes.
	a.do("PATCH", "/api/org", map[string]any{"idle_threshold_sec": 30}, 400, nil)
	a.do("PATCH", "/api/org", map[string]any{"idle_threshold_sec": 1000}, 400, nil)
	var org2 struct {
		IdleThresholdSec int `json:"idle_threshold_sec"`
	}
	a.do("PATCH", "/api/org", map[string]any{"idle_threshold_sec": 300}, 200, &org2)
	if org2.IdleThresholdSec != 300 {
		t.Fatalf("idle_threshold_sec = %d", org2.IdleThresholdSec)
	}
}

func TestPolicyAnnouncesServerURL(t *testing.T) {
	e := setup(t)
	a := e.signup(t, "Move")
	var tok idResp
	a.do("POST", "/api/enrollment-tokens", map[string]any{"description": "t"}, 201, &tok)
	dev := e.enroll(t, tok.Token)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(e.srv.URL, "http") + "/api/agent/ws"
	c, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Device " + dev.DeviceID + ":" + dev.DeviceSecret}}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	f, _ := protocol.Encode(protocol.TypeHello, protocol.Hello{ProtocolVersion: protocol.Version, AgentVersion: "test"})
	if err := c.Write(ctx, websocket.MessageText, f); err != nil {
		t.Fatal(err)
	}
	// nextPolicy returns the server_url of the next policy frame.
	nextPolicy := func() string {
		t.Helper()
		for {
			_, data, err := c.Read(ctx)
			if err != nil {
				t.Fatal(err)
			}
			env, err := protocol.Decode(data)
			if err != nil || env.Type != protocol.TypePolicy {
				continue
			}
			var p protocol.Policy
			if err := json.Unmarshal(env.Data, &p); err != nil {
				t.Fatal(err)
			}
			return p.ServerURL
		}
	}
	if got := nextPolicy(); got != "" {
		t.Fatalf("initial server_url = %q, want empty (no org or public URL set)", got)
	}
	// Setting the org's public server URL re-pushes policy to connected agents.
	a.do("PATCH", "/api/org", map[string]any{"server_url": "https://uem.example.com/"}, 200, nil)
	if got := nextPolicy(); got != "https://uem.example.com" {
		t.Fatalf("server_url after update = %q", got)
	}
}

// A device that switches networks reconnects before the server notices its old
// connection is gone. A command sent on the old connection must be delivered
// again on the new one instead of staying "sent" forever.
func TestCommandOnReplacedConnectionIsRedelivered(t *testing.T) {
	e := setup(t)
	a := e.signup(t, "Roaming")
	var tok idResp
	a.do("POST", "/api/enrollment-tokens", map[string]any{"description": "t"}, 201, &tok)
	dev := e.enroll(t, tok.Token)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(e.srv.URL, "http") + "/api/agent/ws"
	dial := func() *websocket.Conn {
		c, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Device " + dev.DeviceID + ":" + dev.DeviceSecret}}})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.CloseNow() })
		f, _ := protocol.Encode(protocol.TypeHello, protocol.Hello{ProtocolVersion: protocol.Version, AgentVersion: "test"})
		if err := c.Write(ctx, websocket.MessageText, f); err != nil {
			t.Fatal(err)
		}
		return c
	}
	waitCommand := func(c *websocket.Conn, id string) {
		for {
			_, data, err := c.Read(ctx)
			if err != nil {
				t.Fatalf("waiting for command %s: %v", id, err)
			}
			env, _ := protocol.Decode(data)
			if env.Type != protocol.TypeCommand {
				continue
			}
			var got protocol.Command
			_ = json.Unmarshal(env.Data, &got)
			if got.ID == id {
				return
			}
		}
	}

	old := dial()
	var cmd idResp
	a.do("POST", "/api/devices/"+dev.DeviceID+"/commands", map[string]any{"type": "refresh_inventory"}, 201, &cmd)
	waitCommand(old, cmd.ID) // sent on the old connection, never answered

	// The new connection replaces the old one before the old one is torn down.
	waitCommand(dial(), cmd.ID)
}
