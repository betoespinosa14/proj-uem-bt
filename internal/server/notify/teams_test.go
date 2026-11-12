package notify

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/uem/uem/internal/server/store"
)

type teamsMsg struct {
	Type        string `json:"type"`
	Attachments []struct {
		ContentType string `json:"contentType"`
		Content     struct {
			Type    string `json:"type"`
			Version string `json:"version"`
			Body    []struct {
				Type  string `json:"type"`
				Text  string `json:"text"`
				Color string `json:"color"`
				Facts []struct{ Title, Value string }
			} `json:"body"`
			Actions []struct{ Type, Title, URL string } `json:"actions"`
		} `json:"content"`
	} `json:"attachments"`
}

func decodeCard(t *testing.T, event string, data any, consoleURL string) teamsMsg {
	t.Helper()
	raw, err := teamsCard(event, data, consoleURL, time.Date(2026, 11, 12, 6, 2, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	var m teamsMsg
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("%s: invalid JSON: %v", event, err)
	}
	if m.Type != "message" || len(m.Attachments) != 1 ||
		m.Attachments[0].ContentType != "application/vnd.microsoft.card.adaptive" ||
		m.Attachments[0].Content.Type != "AdaptiveCard" || len(m.Attachments[0].Content.Body) == 0 {
		t.Fatalf("%s: not a Teams card envelope: %s", event, raw)
	}
	return m
}

func (m teamsMsg) title() string { return m.Attachments[0].Content.Body[0].Text }

func (m teamsMsg) fact(name string) string {
	for _, b := range m.Attachments[0].Content.Body {
		for _, f := range b.Facts {
			if f.Title == name {
				return f.Value
			}
		}
	}
	return ""
}

func (m teamsMsg) button() string {
	if a := m.Attachments[0].Content.Actions; len(a) == 1 {
		return a[0].URL
	}
	return ""
}

func ptr[T any](v T) *T { return &v }

func TestTeamsCards(t *testing.T) {
	const console = "https://beacon.example.com/"
	at := time.Date(2026, 11, 12, 6, 0, 0, 0, time.UTC)
	dev := store.Device{ID: "d1", Name: "FIN-PC-02", Hostname: "fin-pc-02", OSName: "Windows 11", OSVersion: "24H2",
		EmployeeName: "Ana", GroupName: ptr("Finance"), LastSeenAt: &at}

	cases := []struct {
		event, title, fact, value, button string
		data                              any
	}{
		{"alert.created", "🔴 FIN-PC-02: Firewall on", "Device", "FIN-PC-02", console + "devices/d1",
			store.Alert{DeviceID: ptr("d1"), DeviceName: ptr("FIN-PC-02"), Severity: "high", Title: "FIN-PC-02: Firewall on", Message: "firewall off", CreatedAt: at}},
		{"compliance.violation", "🟠 Compliance violation: Disk encrypted", "Department", "Finance", console + "devices/d1",
			map[string]any{"device": dev, "rule": map[string]any{"name": "Disk encrypted", "severity": "medium"}, "details": "not encrypted"}},
		{"device.enrolled", "🆕 New device enrolled: FIN-PC-02", "System", "Windows 11 24H2", console + "devices/d1", dev},
		{"device.offline", "⚪ Device offline: FIN-PC-02", "Last seen", when(at), console + "devices/d1", &dev},
		{"command.completed", "❌ Run script failed on FIN-PC-02", "Exit code", "1", console + "devices/d1",
			&store.Command{DeviceID: "d1", DeviceName: "FIN-PC-02", Type: "run_script", Status: "failed", ExitCode: ptr(1), Source: "console"}},
		{"audit.device.retire", "📝 Audit: device.retire", "By", "it@example.com", console + "audit",
			store.AuditEntry{Action: "device.retire", ActorEmail: ptr("it@example.com"), TS: at}},
		{"webhook.test", "BEACON test message", "Sent", when(time.Date(2026, 11, 12, 6, 2, 0, 0, time.UTC)), "",
			map[string]string{"message": "Test delivery from BEACON"}},
		{"something.new", "BEACON event: something.new", "", "", "", 42},
	}
	for _, c := range cases {
		m := decodeCard(t, c.event, c.data, console)
		if m.title() != c.title {
			t.Errorf("%s: title = %q, want %q", c.event, m.title(), c.title)
		}
		if c.fact != "" && m.fact(c.fact) != c.value {
			t.Errorf("%s: fact %s = %q, want %q", c.event, c.fact, m.fact(c.fact), c.value)
		}
		if m.button() != c.button {
			t.Errorf("%s: button = %q, want %q", c.event, m.button(), c.button)
		}
	}
}

func TestTeamsCardNoConsoleURLAndClipping(t *testing.T) {
	long := strings.Repeat("x", 2000)
	m := decodeCard(t, "alert.created", store.Alert{DeviceID: ptr("d1"), Severity: "low", Title: "t", Message: long}, "")
	if m.button() != "" {
		t.Errorf("button without a console URL: %q", m.button())
	}
	if got := len([]rune(m.Attachments[0].Content.Body[1].Text)); got != maxText+1 {
		t.Errorf("message length = %d, want %d (clipped plus ellipsis)", got, maxText+1)
	}
}

func TestBodyFormats(t *testing.T) {
	w := New(nil, false)
	now := time.Now()
	generic, err := w.body(FormatGeneric, "org", "device.enrolled", map[string]string{"id": "d1"}, now)
	if err != nil {
		t.Fatal(err)
	}
	var g map[string]any
	if json.Unmarshal(generic, &g) != nil || g["event"] != "device.enrolled" || g["org_id"] != "org" {
		t.Fatalf("generic body = %s", generic)
	}
	teams, err := w.body(FormatTeams, "org", "device.enrolled", map[string]string{"id": "d1"}, now)
	if err != nil || !strings.Contains(string(teams), "application/vnd.microsoft.card.adaptive") {
		t.Fatalf("teams body = %s (%v)", teams, err)
	}
}
