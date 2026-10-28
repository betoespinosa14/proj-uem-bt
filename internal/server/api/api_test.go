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
