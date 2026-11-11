package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/uem/uem/internal/protocol"
)

// connectAgent opens an agent session for an enrolled device and returns a sender.
func (e *env) connectAgent(t *testing.T, dev protocol.EnrollResponse) func(typ string, v any) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	wsURL := "ws" + strings.TrimPrefix(e.srv.URL, "http") + "/api/agent/ws"
	c, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Device " + dev.DeviceID + ":" + dev.DeviceSecret}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.CloseNow() })
	send := func(typ string, v any) {
		f, _ := protocol.Encode(typ, v)
		if err := c.Write(ctx, websocket.MessageText, f); err != nil {
			t.Fatal(err)
		}
	}
	send(protocol.TypeHello, protocol.Hello{ProtocolVersion: protocol.Version, AgentVersion: "test"})
	return send
}

func TestDashboardHealthPostureAndGroupFilter(t *testing.T) {
	e := setup(t)
	a := e.signup(t, "Dash")
	var tok idResp
	a.do("POST", "/api/enrollment-tokens", map[string]any{"description": "t"}, 201, &tok)
	full := e.enroll(t, tok.Token)
	other := e.enroll(t, tok.Token)

	var finance, hr idResp
	a.do("POST", "/api/groups", map[string]any{"name": "Finance"}, 201, &finance)
	a.do("POST", "/api/groups", map[string]any{"name": "HR"}, 201, &hr)
	a.do("PATCH", "/api/devices/"+full.DeviceID, map[string]any{"group_id": finance.ID}, 200, nil)
	a.do("PATCH", "/api/devices/"+other.DeviceID, map[string]any{"group_id": hr.ID}, 200, nil)

	yes := true
	send := e.connectAgent(t, full)
	send(protocol.TypeInventory, protocol.Inventory{OS: protocol.OSInfo{Family: "windows"},
		Security: protocol.Security{DiskEncrypted: &yes, FirewallEnabled: &yes}})
	send(protocol.TypeTelemetry, protocol.Telemetry{TS: time.Now(), CPUPct: 10, MemPct: 95, DiskPct: 96})

	type dash struct {
		DevicesTotal int `json:"devices_total"`
		Health       []struct {
			DeviceID string `json:"device_id"`
			Kind     string
			Value    float64
		}
		Posture struct {
			Reported, Encrypted, Firewall, Antivirus int
		}
		AgentVersions []struct {
			Version string
			Count   int
		} `json:"agent_versions"`
	}
	var all dash
	waitUntil(t, 10*time.Second, "telemetry and inventory on the dashboard", func() bool {
		a.do("GET", "/api/dashboard", nil, 200, &all)
		return len(all.Health) == 2 && all.Posture.Reported == 1
	})
	if all.DevicesTotal != 2 {
		t.Fatalf("devices_total = %d, want 2", all.DevicesTotal)
	}
	kinds := map[string]float64{}
	for _, h := range all.Health {
		if h.DeviceID != full.DeviceID {
			t.Fatalf("health lists the wrong device: %+v", h)
		}
		kinds[h.Kind] = h.Value
	}
	if kinds["disk"] != 96 || kinds["memory"] != 95 {
		t.Fatalf("health = %+v, want disk 96 and memory 95", all.Health)
	}
	if all.Posture.Encrypted != 1 || all.Posture.Firewall != 1 || all.Posture.Antivirus != 0 {
		t.Fatalf("posture = %+v", all.Posture)
	}

	// One department narrows every figure.
	var hrOnly dash
	a.do("GET", "/api/dashboard?group_id="+hr.ID, nil, 200, &hrOnly)
	if hrOnly.DevicesTotal != 1 || len(hrOnly.Health) != 0 || hrOnly.Posture.Reported != 0 {
		t.Fatalf("HR dashboard = %+v, want 1 device and nothing else", hrOnly)
	}
	// The posture meters link to the devices missing a setting; no report counts as missing.
	var unencrypted struct{ Items []struct{ ID string } }
	a.do("GET", "/api/devices?posture=unencrypted", nil, 200, &unencrypted)
	if len(unencrypted.Items) != 1 || unencrypted.Items[0].ID != other.DeviceID {
		t.Fatalf("posture=unencrypted = %+v, want only the device without a report", unencrypted.Items)
	}
	// A group ID that isn't one of ours shows nothing rather than failing.
	var none dash
	a.do("GET", "/api/dashboard?group_id=not-a-group", nil, 200, &none)
	if none.DevicesTotal != 0 {
		t.Fatalf("unknown group: %d devices", none.DevicesTotal)
	}
}

func waitUntil(t *testing.T, limit time.Duration, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
