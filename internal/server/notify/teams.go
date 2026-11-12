package notify

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Webhook body formats.
const (
	FormatGeneric = "generic" // {event, org_id, ts, data} JSON for SIEM/ITSM tools
	FormatTeams   = "teams"   // an Adaptive Card for a Microsoft Teams Workflows webhook
)

var Formats = []string{FormatGeneric, FormatTeams}

// maxText keeps long messages and script errors from swamping a chat channel.
const maxText = 500

// teamsCard renders an event as the message a Teams "Send webhook alerts to a
// channel" workflow posts: {"type":"message","attachments":[Adaptive Card]}.
// The workflow accepts any body, but only posts one in this shape, so every
// event (including unknown ones) gets a card. consoleURL, when set, adds an
// "Open in BEACON" button.
func teamsCard(event string, data any, consoleURL string, now time.Time) ([]byte, error) {
	// Read the payload through its JSON form so values and pointers of the store
	// types, and plain maps, are handled the same way.
	var m map[string]any
	if raw, err := json.Marshal(data); err == nil {
		_ = json.Unmarshal(raw, &m)
	}
	c := describe(event, m, now)
	body := []any{
		map[string]any{"type": "TextBlock", "text": c.title, "weight": "Bolder", "size": "Medium", "wrap": true, "color": c.color},
	}
	if c.text != "" {
		body = append(body, map[string]any{"type": "TextBlock", "text": clip(c.text), "wrap": true, "spacing": "Small"})
	}
	var facts []any
	for _, f := range c.facts {
		if f[1] != "" {
			facts = append(facts, map[string]any{"title": f[0], "value": clip(f[1])})
		}
	}
	if len(facts) > 0 {
		body = append(body, map[string]any{"type": "FactSet", "facts": facts})
	}
	card := map[string]any{
		"type":    "AdaptiveCard",
		"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
		"version": "1.4",
		"msteams": map[string]any{"width": "Full"},
		"body":    body,
	}
	if base := strings.TrimRight(consoleURL, "/"); base != "" && c.link != "" {
		card["actions"] = []any{map[string]any{"type": "Action.OpenUrl", "title": "Open in BEACON", "url": base + c.link}}
	}
	return json.Marshal(map[string]any{
		"type": "message",
		"attachments": []any{map[string]any{
			"contentType": "application/vnd.microsoft.card.adaptive",
			"contentUrl":  nil,
			"content":     card,
		}},
	})
}

type cardText struct {
	title, color, text, link string
	facts                    [][2]string
}

func describe(event string, m map[string]any, now time.Time) cardText {
	device := func(d map[string]any) string { return first(str(d, "name"), str(d, "hostname"), str(d, "device_name")) }
	deviceLink := func(id string) string {
		if id == "" {
			return ""
		}
		return "/devices/" + id
	}
	switch {
	case event == "webhook.test":
		return cardText{title: "BEACON test message", color: "Accent",
			text: "If you can read this, Teams is connected to BEACON.", facts: [][2]string{{"Sent", when(now)}}}

	case event == "alert.created":
		sev := str(m, "severity")
		return cardText{title: icon(sev) + " " + first(str(m, "title"), "New alert"), color: severityColor(sev),
			text: str(m, "message"), link: deviceLink(str(m, "device_id")),
			facts: [][2]string{{"Device", str(m, "device_name")}, {"Severity", title(sev)}, {"Raised", whenStr(str(m, "created_at"))}}}

	case event == "compliance.violation":
		d, _ := m["device"].(map[string]any)
		r, _ := m["rule"].(map[string]any)
		sev := str(r, "severity")
		return cardText{title: icon(sev) + " Compliance violation: " + first(str(r, "name"), "rule"), color: severityColor(sev),
			text: str(m, "details"), link: deviceLink(str(d, "id")),
			facts: [][2]string{{"Device", device(d)}, {"Department", str(d, "group_name")}, {"Employee", str(d, "employee_name")}, {"Severity", title(sev)}}}

	case event == "device.enrolled":
		return cardText{title: "🆕 New device enrolled: " + first(device(m), "unknown"), color: "Good", link: deviceLink(str(m, "id")),
			facts: [][2]string{{"Hostname", str(m, "hostname")}, {"System", strings.TrimSpace(str(m, "os_name") + " " + str(m, "os_version"))},
				{"Employee", str(m, "employee_name")}, {"Department", str(m, "group_name")}}}

	case event == "device.offline":
		return cardText{title: "⚪ Device offline: " + first(device(m), "unknown"), color: "Warning", link: deviceLink(str(m, "id")),
			facts: [][2]string{{"Department", str(m, "group_name")}, {"Employee", str(m, "employee_name")}, {"Last seen", whenStr(str(m, "last_seen_at"))}}}

	case event == "command.completed":
		status := str(m, "status")
		color, mark := "Good", "✅"
		if status != "succeeded" {
			color, mark = "Attention", "❌"
		}
		exit := ""
		if v, ok := m["exit_code"].(float64); ok {
			exit = fmt.Sprintf("%d", int(v))
		}
		return cardText{title: fmt.Sprintf("%s %s %s on %s", mark, humanize(str(m, "type")), status, first(str(m, "device_name"), "a device")),
			color: color, text: str(m, "error"), link: deviceLink(str(m, "device_id")),
			facts: [][2]string{{"Exit code", exit}, {"Issued by", first(str(m, "issued_by_email"), str(m, "source"))}, {"Finished", whenStr(str(m, "completed_at"))}}}

	case strings.HasPrefix(event, "audit."):
		actor := first(str(m, "actor_email"), str(m, "actor_type"))
		target := strings.TrimSpace(str(m, "target_type") + " " + str(m, "target_id"))
		return cardText{title: "📝 Audit: " + first(str(m, "action"), strings.TrimPrefix(event, "audit.")), color: "Default", link: "/audit",
			facts: [][2]string{{"By", actor}, {"Target", target}, {"When", whenStr(str(m, "ts"))}}}
	}
	return cardText{title: "BEACON event: " + event, color: "Default", facts: [][2]string{{"When", when(now)}}}
}

func str(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	switch v := m[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return fmt.Sprintf("%g", v)
	case bool:
		return fmt.Sprintf("%t", v)
	}
	return ""
}

func first(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func clip(s string) string {
	if r := []rune(s); len(r) > maxText {
		return string(r[:maxText]) + "…"
	}
	return s
}

// when shows a time in the server's local zone, e.g. "12 Nov 2026, 14:02".
func when(t time.Time) string { return t.Local().Format("2 Jan 2006, 15:04") }

func whenStr(s string) string {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return ""
	}
	return when(t)
}

func title(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// humanize turns a command type like "run_script" into "Run script".
func humanize(s string) string { return title(strings.ReplaceAll(first(s, "command"), "_", " ")) }

func icon(severity string) string {
	switch severity {
	case "critical", "high":
		return "🔴"
	case "medium":
		return "🟠"
	}
	return "🔵"
}

func severityColor(severity string) string {
	switch severity {
	case "critical", "high":
		return "Attention"
	case "medium":
		return "Warning"
	}
	return "Accent"
}
