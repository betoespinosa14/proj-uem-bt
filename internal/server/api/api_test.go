package api_test

// Integration tests against a real Postgres. Set UEM_TEST_DATABASE_URL to a
// database the tests may migrate and write to (make test uses uem_test); the
// tests skip otherwise. They refuse to run unless the database name ends in
// "_test", so they can never write to the live database.

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
	"path/filepath"
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
	core  *core.Core
	srv   *httptest.Server
	store *store.Store
	audit *audit.Log
	dist  string // agent builds directory (empty until a test writes a manifest)
}

func setup(t *testing.T) *env {
	t.Helper()
	dsn := os.Getenv("UEM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("UEM_TEST_DATABASE_URL not set")
	}
	if err := checkTestDB(dsn); err != nil {
		t.Fatal(err)
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
	dist := t.TempDir()
	c := &core.Core{Store: st, Audit: al, Hub: hub.New(), Hooks: notify.New(st, false),
		TelemetryInterval: time.Minute, InventoryInterval: time.Hour, AgentDist: dist}
	sched := scheduler.New(c)
	srv := httptest.NewServer(api.New(api.Config{AllowSignup: true, AgentDistDir: dist}, c, sched).Handler())
	t.Cleanup(srv.Close)
	return &env{core: c, srv: srv, store: st, audit: al, dist: dist}
}

// checkTestDB refuses any database whose name doesn't end in "_test": the
// tests create organisations and run migrations, which must never happen on
// the live database.
func checkTestDB(dsn string) error {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("UEM_TEST_DATABASE_URL: %w", err)
	}
	if name := cfg.ConnConfig.Database; !strings.HasSuffix(name, "_test") {
		return fmt.Errorf("refusing to run integration tests against database %q: "+
			"use a dedicated database whose name ends in _test (make test uses uem_test)", name)
	}
	return nil
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
	a.do("POST", "/api/enrollment-tokens", map[string]any{"description": "   "}, 400, nil)
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

// Devices remember the token they enrolled with: the token lists its creator
// and last use, and the device list filters by it.
func TestEnrollmentTokenDevices(t *testing.T) {
	e := setup(t)
	a := e.signup(t, "TokenDevices")
	var one, two idResp
	a.do("POST", "/api/enrollment-tokens", map[string]any{"description": "one"}, 201, &one)
	a.do("POST", "/api/enrollment-tokens", map[string]any{"description": "two"}, 201, &two)
	d1 := e.enroll(t, one.Token)
	e.enroll(t, two.Token)

	var devs struct{ Items []struct{ ID string } }
	a.do("GET", "/api/devices?token="+one.ID, nil, 200, &devs)
	if len(devs.Items) != 1 || devs.Items[0].ID != d1.DeviceID {
		t.Fatalf("devices for token one: %+v, want only %s", devs.Items, d1.DeviceID)
	}
	var toks struct {
		Items []struct {
			ID         string
			CreatedBy  *string    `json:"created_by"`
			LastUsedAt *time.Time `json:"last_used_at"`
		}
	}
	a.do("GET", "/api/enrollment-tokens", nil, 200, &toks)
	for _, tk := range toks.Items {
		if tk.CreatedBy == nil || *tk.CreatedBy == "" {
			t.Errorf("token %s: no creator", tk.ID)
		}
		if tk.LastUsedAt == nil {
			t.Errorf("token %s: no last use after an enrollment", tk.ID)
		}
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

	// The knowledge base (server setup and recovery notes) is for owners and admins.
	hd.do("GET", "/api/kb", nil, 403, nil)
	hd.do("GET", "/api/kb/backups", nil, 403, nil)
	var kbList struct {
		Sections []string `json:"sections"`
		Articles []struct {
			Slug, Title, Body string
		} `json:"articles"`
	}
	owner.do("GET", "/api/kb", nil, 200, &kbList)
	if len(kbList.Sections) == 0 || len(kbList.Articles) == 0 || kbList.Articles[0].Body == "" {
		t.Fatalf("knowledge base list: %+v", kbList)
	}
	owner.do("GET", "/api/kb/"+kbList.Articles[0].Slug, nil, 200, nil)
	owner.do("GET", "/api/kb/no-such-article", nil, 404, nil)

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

	// The Windows install command enables TLS 1.2 before it downloads the installer.
	var cmds map[string]string
	a.do("GET", "/api/enrollment-tokens/"+tok.ID+"/install", nil, 200, &cmds)
	win := cmds["windows"]
	tls, irm := strings.Index(win, "SecurityProtocol -bor 3072"), strings.Index(win, "irm ")
	if tls < 0 || irm < 0 || tls > irm || !strings.Contains(win, "/install.ps1") || !strings.Contains(win, tok.Token) {
		t.Fatalf("windows install command = %q", win)
	}

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
	if err != nil || len(rows) != 2 || rows[0][1] != "name" || rows[1][1] != "curl" || rows[1][4] != "Third-party" || rows[1][6] != "false" {
		t.Fatalf("software.csv rows = %v, err = %v", rows, err)
	}
	// Marking the app as infrastructure in App usage flags it on the Software tab (any case).
	a.do("PUT", "/api/usage/infrastructure", map[string]any{"app": "CURL"}, 204, nil)
	var sw struct {
		Items []struct {
			Name           string
			Infrastructure bool
		}
	}
	a.do("GET", "/api/devices/"+dev.DeviceID+"/software", nil, 200, &sw)
	if len(sw.Items) != 1 || !sw.Items[0].Infrastructure {
		t.Fatalf("software = %+v, want curl marked infrastructure", sw.Items)
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
		InstalledOn   int        `json:"installed_on"`
		LastUsed      *time.Time `json:"last_used"`
		ThirdParty    bool       `json:"third_party"`
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
		Groups                        []string
		App, User, Employee           string
		ThirdParty                    bool `json:"third_party"`
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
			Apps          int
			Users         int
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
	if len(sum.Daily) != 1 || sum.Daily[0].ActiveSeconds != 4800 || sum.Daily[0].Minutes != 180 || sum.Daily[0].Devices != 1 || sum.Daily[0].Apps != 1 || sum.Daily[0].Users != 2 ||
		len(sum.TopApps) != 1 || sum.TopApps[0].App != "Slack" || sum.TopApps[0].Users != 2 ||
		len(sum.IdleApps) != 1 || sum.IdleApps[0].Minutes != 180 ||
		len(sum.TopUsers) != 2 || sum.TopUsers[0].User != "ana" || sum.TopUsers[0].ActiveSeconds != 3600 || sum.TopUsers[0].Apps != 1 ||
		len(sum.TopUsers[0].DeviceNames) != 1 ||
		len(sum.Background) != 2 || sum.Background[0].App != "Dropbox" || sum.Background[0].User != "carl" || sum.Background[0].Minutes != 60 ||
		!sum.Background[0].ThirdParty || sum.Background[1].App != "Contacts" || sum.Background[1].ThirdParty {
		t.Fatalf("summary = %+v", sum)
	}
	if !sum.IdleApps[0].ThirdParty || len(sum.TopUsers[0].Groups) != 0 {
		t.Fatalf("idle third_party / top user groups = %+v / %+v", sum.IdleApps, sum.TopUsers)
	}
	if sum.TopUsers[0].Employee != "" {
		t.Fatalf("employee before one is set = %q", sum.TopUsers[0].Employee)
	}
	// Top users carry the employee name of their device.
	a.do("PATCH", "/api/devices/"+dev.DeviceID, map[string]any{"employee_name": "Ana Cruz"}, 200, nil)
	a.do("GET", "/api/usage/summary", nil, 200, &sum)
	if len(sum.TopUsers) != 2 || sum.TopUsers[0].Employee != "Ana Cruz" || sum.TopUsers[0].User != "ana" {
		t.Fatalf("top users with an employee = %+v", sum.TopUsers)
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

	// Device filter: the test device sees its own usage; another device ID sees nothing.
	a.do("GET", "/api/usage/summary?device_id="+dev.DeviceID, nil, 200, &sum)
	if sum.Totals.ActiveSeconds != 4800 || sum.Totals.Devices != 1 || len(sum.TopApps) != 1 || sum.Totals.UnusedApps != 1 || len(sum.Background) != 2 {
		t.Fatalf("summary for the test device = %+v", sum)
	}
	a.do("GET", "/api/usage?group=app&device_id="+dev.DeviceID, nil, 200, &usage)
	// Dropbox and Contacts only ran in the background, so they aren't "used".
	if len(usage.Items) != 1 || usage.Items[0].App != "Slack" {
		t.Fatalf("usage by app for the test device = %+v", usage.Items)
	}
	other := "?device_id=00000000-0000-4000-8000-000000000000"
	a.do("GET", "/api/usage/summary"+other, nil, 200, &sum)
	if sum.Totals.ActiveSeconds != 0 || len(sum.TopApps) != 0 || sum.Totals.UnusedApps != 0 || len(sum.Background) != 0 {
		t.Fatalf("summary for another device = %+v", sum)
	}
	a.do("GET", "/api/usage"+other, nil, 200, &usage)
	if len(usage.Items) != 0 {
		t.Fatalf("usage for another device = %+v", usage.Items)
	}
	resp, err = a.http.Get(a.base + "/api/usage/reclaim.csv" + other)
	if err != nil {
		t.Fatal(err)
	}
	rows, err = csv.NewReader(resp.Body).ReadAll()
	resp.Body.Close()
	if err != nil || len(rows) != 1 {
		t.Fatalf("reclaim csv for another device = %v, err = %v", rows, err)
	}
	a.do("GET", "/api/usage/summary?device_id=nope", nil, 400, nil)

	// Short periods: use from 3 days ago counts in 7 days but not today.
	threeDaysAgo := time.Now().AddDate(0, 0, -3)
	send(protocol.TypeUsage, protocol.Usage{Entries: []protocol.UsageEntry{
		{Date: threeDaysAgo.Format(time.DateOnly), App: "Slack", User: "dora", Minutes: 30, ActiveSeconds: 900, LastSeen: threeDaysAgo},
	}})
	hasDora := func() bool {
		for _, u := range sum.TopUsers {
			if u.User == "dora" {
				return true
			}
		}
		return false
	}
	// Usage is stored asynchronously and the summary reads it in several
	// queries, so wait until every part of one response includes it.
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline) && !(hasDora() && len(sum.Daily) == 2); time.Sleep(50 * time.Millisecond) {
		a.do("GET", "/api/usage/summary?days=7", nil, 200, &sum)
	}
	if !hasDora() || len(sum.Daily) != 2 {
		t.Fatalf("summary?days=7 = %+v", sum)
	}
	a.do("GET", "/api/usage/summary?days=1", nil, 200, &sum)
	if hasDora() || len(sum.Daily) != 1 || len(sum.TopUsers) != 2 {
		t.Fatalf("summary?days=1 = %+v", sum)
	}

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
	// A macOS built-in left open after a few seconds of use is ranked with
	// third_party false, so the console can hide it; top users carry the
	// group of their device.
	send(protocol.TypeUsage, protocol.Usage{Entries: []protocol.UsageEntry{
		{Date: today, App: "Contacts", User: "dan", Minutes: 300, ActiveSeconds: 5, LastSeen: time.Now()},
	}})
	var sales idResp
	a.do("POST", "/api/groups", map[string]any{"name": "Sales"}, 201, &sales)
	a.do("PATCH", "/api/devices/"+dev.DeviceID, map[string]any{"group_id": sales.ID}, 200, nil)
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		a.do("GET", "/api/usage/summary", nil, 200, &sum)
		if len(sum.IdleApps) == 2 {
			break
		}
	}
	if len(sum.IdleApps) != 2 || sum.IdleApps[0].App != "Contacts" || sum.IdleApps[0].ThirdParty || !sum.IdleApps[1].ThirdParty ||
		len(sum.TopUsers) == 0 || len(sum.TopUsers[0].Groups) != 1 || sum.TopUsers[0].Groups[0] != "Sales" {
		t.Fatalf("summary with a system app and a group = %+v / %+v", sum.IdleApps, sum.TopUsers)
	}

	// An app that is a Windows built-in on one install and user-installed on
	// another (Edge) is third-party in the idle ranking, as in the background
	// card. A Realtek driver is a system app.
	send(protocol.TypeInventory, protocol.Inventory{OS: protocol.OSInfo{Family: "macos"}, Software: []protocol.Software{
		{Name: "Slack", Source: "mac_app_store"}, {Name: "Zoom", Source: "identified_developer"}, {Name: "Contacts", Source: "apple"},
		{Name: "Microsoft Edge", Publisher: "Microsoft Corporation", Source: "registry"},
		{Name: "Microsoft Edge", Publisher: "Microsoft Corporation (UBF8T346G9)", Source: "identified_developer"},
		{Name: "Realtek Audio Driver", Publisher: "Realtek Semiconductor Corp.", Source: "registry"},
	}})
	send(protocol.TypeUsage, protocol.Usage{Entries: []protocol.UsageEntry{
		{Date: today, App: "Microsoft Edge", User: "dan", Minutes: 600, ActiveSeconds: 60, LastSeen: time.Now()},
		{Date: today, App: "Realtek Audio Driver", User: "dan", Minutes: 500, ActiveSeconds: 1, LastSeen: time.Now()},
	}})
	idle := map[string]bool{}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		a.do("GET", "/api/usage/summary", nil, 200, &sum)
		idle = map[string]bool{}
		for _, x := range sum.IdleApps {
			idle[x.App] = x.ThirdParty
		}
		if len(idle) == 4 {
			break
		}
	}
	if third, ok := idle["Microsoft Edge"]; !ok || !third {
		t.Fatalf("Edge in idle apps = %+v", sum.IdleApps)
	}
	if third, ok := idle["Realtek Audio Driver"]; !ok || third {
		t.Fatalf("Realtek driver in idle apps = %+v", sum.IdleApps)
	}

	// Yesterday: that day only (not today), with no comparison. Today carries
	// the last working day before it (30+ minutes of combined use): yesterday
	// (10 minutes), two days ago (2) and three days ago (dora, 15) are
	// skipped; four days ago (40) counts.
	yesterday := time.Now().AddDate(0, 0, -1)
	twoDaysAgo, fourDaysAgo := time.Now().AddDate(0, 0, -2), time.Now().AddDate(0, 0, -4)
	send(protocol.TypeUsage, protocol.Usage{Entries: []protocol.UsageEntry{
		{Date: yesterday.Format(time.DateOnly), App: "Slack", User: "eve", Minutes: 20, ActiveSeconds: 600, LastSeen: yesterday},
		{Date: twoDaysAgo.Format(time.DateOnly), App: "Slack", User: "gus", Minutes: 5, ActiveSeconds: 120, LastSeen: twoDaysAgo},
		{Date: fourDaysAgo.Format(time.DateOnly), App: "Slack", User: "hal", Minutes: 60, ActiveSeconds: 2400, LastSeen: fourDaysAgo},
	}})
	var ysum struct {
		Totals struct {
			ActiveSeconds int `json:"active_seconds"`
		}
		Daily   []struct{ Day string }
		TopApps []stat `json:"top_apps"`
		Compare *struct {
			Day           string
			ActiveSeconds int `json:"active_seconds"`
			ActiveUsers   int `json:"active_users"`
			Apps, Users   map[string]int
		}
	}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		a.do("GET", "/api/usage/summary?days=1&yesterday=true", nil, 200, &ysum)
		if ysum.Totals.ActiveSeconds > 0 {
			break
		}
	}
	if ysum.Totals.ActiveSeconds != 600 || len(ysum.Daily) != 1 || ysum.Daily[0].Day != yesterday.Format(time.DateOnly) ||
		len(ysum.TopApps) != 1 || ysum.TopApps[0].ActiveSeconds != 600 || ysum.Compare != nil {
		t.Fatalf("summary?yesterday = %+v", ysum)
	}
	a.do("GET", "/api/usage/summary?days=1", nil, 200, &ysum)
	prev := ysum.Compare
	if prev == nil || prev.Day != fourDaysAgo.Format(time.DateOnly) || prev.ActiveSeconds != 2400 || prev.ActiveUsers != 1 ||
		prev.Apps["slack"] != 2400 || prev.Users["hal"] != 2400 {
		t.Fatalf("summary?days=1 compare = %+v", prev)
	}
	a.do("GET", "/api/usage?group=app&yesterday=true", nil, 200, &usage)
	if len(usage.Items) != 1 || usage.Items[0].App != "Slack" || usage.Items[0].ActiveSeconds != 600 {
		t.Fatalf("usage?yesterday = %+v", usage.Items)
	}
	// Other periods carry no comparison.
	ysum.Compare = nil
	a.do("GET", "/api/usage/summary?days=7", nil, 200, &ysum)
	if ysum.Compare != nil {
		t.Fatalf("summary?days=7 compare = %+v", ysum.Compare)
	}

	// The table counts active use only, like the dashboard: fay had Slack
	// open without using it, so Slack keeps 2 users today; Zoom only ran in
	// the background, so it is still "not used", with no last active time.
	send(protocol.TypeUsage, protocol.Usage{Entries: []protocol.UsageEntry{
		{Date: today, App: "Slack", User: "fay", Minutes: 45, LastSeen: time.Now()},
		{Date: today, App: "Zoom", User: "fay", Minutes: 45, LastSeen: time.Now()},
	}})
	hasFay := func() bool {
		for _, b := range sum.Background {
			if b.User == "fay" {
				return true
			}
		}
		return false
	}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline) && !hasFay(); time.Sleep(50 * time.Millisecond) {
		a.do("GET", "/api/usage/summary?days=1", nil, 200, &sum)
	}
	if !hasFay() || sum.Totals.UnusedApps != 1 {
		t.Fatalf("summary with background-only use = %+v", sum)
	}
	a.do("GET", "/api/usage?group=app&days=1", nil, 200, &usage)
	byName := map[string]row{}
	for _, u := range usage.Items {
		byName[u.App] = u
	}
	if sl, ok := byName["Slack"]; !ok || sl.Users != 2 || sl.LastUsed == nil || !sl.ThirdParty {
		t.Fatalf("Slack by app = %+v", usage.Items)
	}
	if _, ok := byName["Zoom"]; ok {
		t.Fatalf("background-only Zoom counted as used: %+v", usage.Items)
	}
	if c, ok := byName["Contacts"]; !ok || c.ThirdParty {
		t.Fatalf("Contacts by app = %+v", usage.Items)
	}
	a.do("GET", "/api/usage?group=app&days=1&third_party=true", nil, 200, &usage)
	for _, u := range usage.Items {
		if !u.ThirdParty || u.App == "Contacts" {
			t.Fatalf("third_party=true = %+v", usage.Items)
		}
	}
	a.do("GET", "/api/usage?group=app&days=1&unused=true", nil, 200, &usage)
	if len(usage.Items) != 1 || usage.Items[0].App != "Zoom" || usage.Items[0].LastUsed != nil || !usage.Items[0].ThirdParty {
		t.Fatalf("unused with background-only Zoom = %+v", usage.Items)
	}
	a.do("GET", "/api/usage?days=1", nil, 200, &usage)
	for _, u := range usage.Items {
		if u.User == "fay" {
			t.Fatalf("fay's open-only rows in the per-user table = %+v", usage.Items)
		}
	}

	// Linux reports no keyboard or mouse activity, so open time counts as use there.
	send(protocol.TypeInventory, protocol.Inventory{OS: protocol.OSInfo{Family: "linux"}, Software: []protocol.Software{
		{Name: "Slack", Source: "mac_app_store"}, {Name: "Zoom", Source: "identified_developer"}, {Name: "Contacts", Source: "apple"},
	}})
	poll("/api/usage?group=app&days=1&app=zoom", func() bool {
		return len(usage.Items) == 1 && usage.Items[0].Users == 1 && usage.Items[0].LastUsed != nil
	})

	// Helper components (an updater) are left out of "Not used", its count
	// and the reclaim list, and counted apart; a real app (Figma) stays.
	send(protocol.TypeInventory, protocol.Inventory{OS: protocol.OSInfo{Family: "linux"}, Software: []protocol.Software{
		{Name: "Slack", Source: "mac_app_store"}, {Name: "Zoom", Source: "identified_developer"}, {Name: "Contacts", Source: "apple"},
		{Name: "Adobe Update Helper", Source: "identified_developer"}, {Name: "Figma", Source: "identified_developer"},
	}})
	poll("/api/usage?group=app&days=1&unused=true", func() bool {
		return len(usage.Items) == 1 && usage.Items[0].App == "Figma"
	})
	var comp struct {
		Totals struct {
			UnusedApps       int `json:"unused_apps"`
			UnusedComponents int `json:"unused_components"`
		}
	}
	a.do("GET", "/api/usage/summary?days=1", nil, 200, &comp)
	if comp.Totals.UnusedApps != 1 || comp.Totals.UnusedComponents != 1 {
		t.Fatalf("summary with a helper component = %+v", comp.Totals)
	}
	resp, err = a.http.Get(a.base + "/api/usage/reclaim.csv?days=1")
	if err != nil {
		t.Fatal(err)
	}
	rows, err = csv.NewReader(resp.Body).ReadAll()
	resp.Body.Close()
	if err != nil || len(rows) != 2 || rows[1][0] != "Figma" {
		t.Fatalf("reclaim csv with a helper component = %v, err = %v", rows, err)
	}

	// Parallels' shortcuts to Windows apps (source unknown, version
	// "Windows 11") are Windows built-ins: out of "Not used" in both views,
	// even next to the Mac's own app of the same name. Unused per-user rows
	// carry the account signed in on the device.
	send(protocol.TypeInventory, protocol.Inventory{OS: protocol.OSInfo{Family: "macos"}, Users: protocol.Users{LoggedIn: []string{"zed"}}, Software: []protocol.Software{
		{Name: "Slack", Source: "mac_app_store"}, {Name: "Zoom", Source: "identified_developer"}, {Name: "Figma", Source: "identified_developer"},
		{Name: "Calculator", Source: "apple"}, {Name: "Calculator", Version: "Windows 11", Source: "unknown"},
		{Name: "Command Prompt", Version: "Windows 11", Source: "unknown"},
	}})
	poll("/api/usage?group=app&days=1&app=calculator", func() bool { return len(usage.Items) == 1 })
	a.do("GET", "/api/usage?group=app&days=1&unused=true", nil, 200, &usage)
	byApp := map[string]bool{}
	for _, u := range usage.Items {
		byApp[u.App] = true
	}
	var perUser struct {
		Items []struct {
			App        string
			DeviceUser string `json:"device_user"`
		}
	}
	a.do("GET", "/api/usage?days=1&unused=true", nil, 200, &perUser)
	byUser := map[string]bool{}
	for _, u := range perUser.Items {
		byUser[u.App] = true
		if u.DeviceUser != "zed" {
			t.Fatalf("unused row without the device's user = %+v", perUser.Items)
		}
	}
	if byApp["Calculator"] || byApp["Command Prompt"] || byUser["Calculator"] || byUser["Command Prompt"] || !byApp["Figma"] || len(byApp) != len(byUser) {
		t.Fatalf("unused with Parallels shortcuts: by app %v, by user %v", byApp, byUser)
	}

	// The reclaim list flags infrastructure apps.
	a.do("PUT", "/api/usage/infrastructure", map[string]any{"app": "Figma"}, 204, nil)
	resp, err = a.http.Get(a.base + "/api/usage/reclaim.csv?days=1")
	if err != nil {
		t.Fatal(err)
	}
	rows, err = csv.NewReader(resp.Body).ReadAll()
	resp.Body.Close()
	if err != nil || len(rows) < 2 || rows[0][9] != "infrastructure" || rows[1][0] != "Figma" || rows[1][9] != "yes" {
		t.Fatalf("reclaim csv with an infrastructure app = %v, err = %v", rows, err)
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

func TestAgentUpdate(t *testing.T) {
	e := setup(t)
	a := e.signup(t, "Updates")
	var tok idResp
	a.do("POST", "/api/enrollment-tokens", map[string]any{"description": "t"}, 201, &tok)
	enroll := func(os, arch, version string) protocol.EnrollResponse {
		b, _ := json.Marshal(protocol.EnrollRequest{Token: tok.Token, Facts: protocol.Facts{Hostname: "h-" + version, OSFamily: os, Arch: arch, AgentVersion: version}})
		resp, err := http.Post(e.srv.URL+"/api/agent/enroll", "application/json", bytes.NewReader(b))
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("enroll: %v %v", err, resp.Status)
		}
		defer resp.Body.Close()
		var out protocol.EnrollResponse
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return out
	}
	type cmdResp struct {
		Type    string
		Status  string
		Payload map[string]any
	}
	update := func(id string, want int) cmdResp {
		var c cmdResp
		a.do("POST", "/api/devices/"+id+"/commands", map[string]any{"type": "update_agent"}, want, &c)
		return c
	}

	// No builds on the server yet.
	var latest struct{ Version *string }
	a.do("GET", "/api/agents/latest", nil, 200, &latest)
	if latest.Version != nil {
		t.Fatalf("latest = %v with no manifest", *latest.Version)
	}
	current := enroll("linux", "amd64", "2026.11.7")
	update(current.DeviceID, 400)

	os.WriteFile(filepath.Join(e.dist, "manifest.json"), []byte(`{"version":"2026.11.9","files":{"uem-agent-linux-amd64":{"sha256":"`+strings.Repeat("ab", 32)+`","size":42}}}`), 0o644)
	a.do("GET", "/api/agents/latest", nil, 200, &latest)
	if latest.Version == nil || *latest.Version != "2026.11.9" {
		t.Fatalf("latest = %v", latest.Version)
	}
	resp, err := http.Get(e.srv.URL + "/downloads/manifest.json")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("manifest download: %v %v", err, resp.Status)
	}
	resp.Body.Close()

	// An agent that can update itself gets update_agent with the build's checksum (queued while offline).
	c := update(current.DeviceID, 201)
	if c.Type != "update_agent" || c.Status != "queued" || c.Payload["file"] != "uem-agent-linux-amd64" ||
		c.Payload["version"] != "2026.11.9" || c.Payload["sha256"] != strings.Repeat("ab", 32) || c.Payload["size"] != float64(42) {
		t.Fatalf("update_agent command = %+v", c)
	}

	// An older agent gets the one-time replacement script, which needs the public server address.
	old := enroll("linux", "amd64", "2026.11.3-dirty")
	update(old.DeviceID, 400)
	a.do("PATCH", "/api/org", map[string]any{"server_url": "https://beacon.example"}, 200, nil)
	c = update(old.DeviceID, 201)
	script, _ := c.Payload["script"].(string)
	if c.Type != "run_script" || c.Payload["shell"] != "sh" || !strings.Contains(script, "https://beacon.example/downloads/uem-agent-linux-amd64") ||
		!strings.Contains(script, strings.Repeat("ab", 32)) || !strings.Contains(script, "kickstart") {
		t.Fatalf("bootstrap command = %+v", c)
	}

	// Bulk: each device gets the command its agent understands; current devices are skipped.
	var bulk struct{ Items []cmdResp }
	a.do("POST", "/api/commands/bulk", map[string]any{"type": "update_agent", "device_ids": []string{current.DeviceID, old.DeviceID}}, 201, &bulk)
	if len(bulk.Items) != 2 || bulk.Items[0].Type == bulk.Items[1].Type {
		t.Fatalf("bulk update = %+v", bulk.Items)
	}
	upToDate := enroll("linux", "amd64", "2026.11.9")
	a.do("POST", "/api/commands/bulk", map[string]any{"type": "update_agent", "device_ids": []string{upToDate.DeviceID}}, 400, nil)

	// Already current, or no build for the platform: refused.
	update(upToDate.DeviceID, 400)
	update(enroll("windows", "arm64", "2026.11.7").DeviceID, 400)

	// When the device reconnects on the new version, the change is audited.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(e.srv.URL, "http")+"/api/agent/ws",
		&websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Device " + current.DeviceID + ":" + current.DeviceSecret}}})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()
	f, _ := protocol.Encode(protocol.TypeHello, protocol.Hello{ProtocolVersion: protocol.Version, AgentVersion: "2026.11.9"})
	if err := ws.Write(ctx, websocket.MessageText, f); err != nil {
		t.Fatal(err)
	}
	var log struct {
		Items []struct {
			Action  string
			Details map[string]any
		}
	}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		a.do("GET", "/api/audit?action=agent.updated", nil, 200, &log)
		if len(log.Items) > 0 {
			break
		}
	}
	if len(log.Items) != 1 || log.Items[0].Details["from"] != "2026.11.7" || log.Items[0].Details["to"] != "2026.11.9" {
		t.Fatalf("agent.updated audit = %+v", log.Items)
	}
}

func TestEmployeeName(t *testing.T) {
	e := setup(t)
	a := e.signup(t, "Employee")
	var tok, grp idResp
	a.do("POST", "/api/enrollment-tokens", map[string]any{"description": "t"}, 201, &tok)
	dev := e.enroll(t, tok.Token)
	a.do("POST", "/api/groups", map[string]any{"name": "Finance", "blocked_apps": []string{}}, 201, &grp)
	path := "/api/devices/" + dev.DeviceID
	type device struct {
		Name     string
		Employee string `json:"employee_name"`
	}
	var d device
	// No employee yet: the name is free text.
	a.do("PATCH", path, map[string]any{"name": "Spare laptop"}, 200, &d)
	if d.Name != "Spare laptop" || d.Employee != "" {
		t.Fatalf("free name = %+v", d)
	}
	a.do("PATCH", path, map[string]any{"employee_name": "  "}, 400, nil)
	// With an employee, the name is generated and a typed name is ignored.
	a.do("PATCH", path, map[string]any{"employee_name": " Ana Cruz ", "group_id": grp.ID, "name": "ignored"}, 200, &d)
	if d.Name != "Finance | Ana Cruz" || d.Employee != "Ana Cruz" {
		t.Fatalf("generated name = %+v", d)
	}
	a.do("PATCH", path, map[string]any{"name": "ignored again"}, 200, &d)
	if d.Name != "Finance | Ana Cruz" {
		t.Fatalf("name after a typed name = %q", d.Name)
	}
	// Renaming the group renames its devices.
	a.do("PATCH", "/api/groups/"+grp.ID, map[string]any{"name": "FINANCE"}, 200, nil)
	a.do("GET", path, nil, 200, &d)
	if d.Name != "FINANCE | Ana Cruz" {
		t.Fatalf("name after group rename = %q", d.Name)
	}
	// Search finds the employee.
	var list struct{ Items []device }
	a.do("GET", "/api/devices?q=ana", nil, 200, &list)
	if len(list.Items) != 1 {
		t.Fatalf("search by employee = %+v", list.Items)
	}
	// Leaving the group, or deleting it, drops the prefix.
	a.do("PATCH", path, map[string]any{"group_id": nil}, 200, &d)
	if d.Name != "Ana Cruz" {
		t.Fatalf("name without a group = %q", d.Name)
	}
	a.do("PATCH", path, map[string]any{"group_id": grp.ID}, 200, &d)
	a.do("DELETE", "/api/groups/"+grp.ID, nil, 200, nil)
	a.do("GET", path, nil, 200, &d)
	if d.Name != "Ana Cruz" {
		t.Fatalf("name after deleting the group = %q", d.Name)
	}
}

func TestTelemetryClampAndBuckets(t *testing.T) {
	e := setup(t)
	a := e.signup(t, "Telemetry")
	var tok idResp
	a.do("POST", "/api/enrollment-tokens", map[string]any{"description": "t"}, 201, &tok)
	dev := e.enroll(t, tok.Token)
	ctx := context.Background()

	// Rows stored before the battery cap (101%) must still read back as 100%.
	start := time.Now().Add(-2 * time.Hour).Truncate(time.Hour)
	for i := range 10 {
		if _, err := e.store.Pool.Exec(ctx, `
			INSERT INTO device_telemetry (device_id, ts, cpu_pct, mem_pct, disk_pct, battery_pct)
			VALUES ($1, $2, $3, 50, 40, 101)`, dev.DeviceID, start.Add(time.Duration(i)*time.Minute), float64(i*10)); err != nil {
			t.Fatal(err)
		}
	}
	// New samples are capped on the way in.
	over := 130.0
	if err := e.store.RecordTelemetry(ctx, dev.DeviceID, protocol.Telemetry{TS: start.Add(30 * time.Minute), CPUPct: 120, MemPct: 50, DiskPct: 40, BatteryPct: &over}); err != nil {
		t.Fatal(err)
	}

	// A device without a battery keeps reporting none (not 100%).
	if err := e.store.RecordTelemetry(ctx, dev.DeviceID, protocol.Telemetry{TS: start.Add(40 * time.Minute), CPUPct: 5, MemPct: 50, DiskPct: 40}); err != nil {
		t.Fatal(err)
	}

	type point struct {
		CPU     float64  `json:"cpu_pct"`
		Battery *float64 `json:"battery_pct"`
	}
	var raw struct {
		Items    []point `json:"items"`
		Interval int     `json:"interval_sec"`
	}
	a.do("GET", "/api/devices/"+dev.DeviceID+"/telemetry?hours=24", nil, 200, &raw)
	if raw.Interval != 60 || len(raw.Items) != 12 {
		t.Fatalf("24h: interval %d, %d points; want 60, 12", raw.Interval, len(raw.Items))
	}
	for _, p := range raw.Items[:11] {
		if p.CPU > 100 || p.Battery == nil || *p.Battery > 100 {
			t.Fatalf("24h point over 100%% or missing battery: %+v", p)
		}
	}
	if raw.Items[11].Battery != nil {
		t.Fatalf("no-battery sample came back as %v, want null", *raw.Items[11].Battery)
	}

	var weekly struct {
		Items    []point `json:"items"`
		Interval int     `json:"interval_sec"`
	}
	a.do("GET", "/api/devices/"+dev.DeviceID+"/telemetry?hours=168", nil, 200, &weekly)
	// Minutes 0-9 fall in two 5-minute buckets (bin origin is "now - 7d", so any
	// alignment gives 2 or 3 buckets), the capped sample at minute 30 and the
	// no-battery one at minute 40.
	if weekly.Interval != 300 || len(weekly.Items) < 4 || len(weekly.Items) > 5 {
		t.Fatalf("7d: interval %d, %d points; want 300, 4-5", weekly.Interval, len(weekly.Items))
	}
	capped := weekly.Items[len(weekly.Items)-2]
	if capped.CPU != 100 || capped.Battery == nil || *capped.Battery != 100 {
		t.Fatalf("7d capped sample = %+v, want 100/100", capped)
	}
	if weekly.Items[len(weekly.Items)-1].Battery != nil {
		t.Fatal("7d no-battery bucket should have a null battery")
	}
}
