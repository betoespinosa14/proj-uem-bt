// Package compliance evaluates device state against admin-defined rules.
package compliance

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/uem/uem/internal/protocol"
	"github.com/uem/uem/internal/server/store"
)

// Facts is everything a rule can inspect about one device.
type Facts struct {
	Device    store.Device
	Inventory *protocol.Inventory
	Software  []store.Software
	Now       time.Time
}

type FieldType string

const (
	TBool    FieldType = "bool"
	TNumber  FieldType = "number"
	TString  FieldType = "string"
	TVersion FieldType = "version"
	TList    FieldType = "list"
	TFlagged FieldType = "flagged" // apps matching a built-in catalog
)

var opsByType = map[FieldType][]string{
	TBool:    {"is_true", "is_false"},
	TNumber:  {"eq", "neq", "gt", "gte", "lt", "lte"},
	TString:  {"eq", "neq", "contains", "not_contains"},
	TVersion: {"version_gte", "version_lt", "eq", "neq"},
	TList:    {"contains", "not_contains"},
	TFlagged: {"none_found", "any_found"},
}

// Field describes one inspectable attribute; Get returns nil when unknown.
type Field struct {
	Name  string    `json:"field"`
	Label string    `json:"label"`
	Type  FieldType `json:"type"`
	Ops   []string  `json:"ops"`
	Unit  string    `json:"unit,omitempty"`
	Get   func(Facts) any `json:"-"`
}

func boolp(b *bool) any {
	if b == nil {
		return nil
	}
	return *b
}

func inv(f Facts) *protocol.Inventory { return f.Inventory }

// Fields is the registry of rule fields, in display order.
var Fields = []Field{
	{Name: "security.disk_encrypted", Label: "Disk encryption enabled", Type: TBool, Get: func(f Facts) any {
		if inv(f) == nil {
			return nil
		}
		return boolp(f.Inventory.Security.DiskEncrypted)
	}},
	{Name: "security.firewall_enabled", Label: "Firewall enabled", Type: TBool, Get: func(f Facts) any {
		if inv(f) == nil {
			return nil
		}
		return boolp(f.Inventory.Security.FirewallEnabled)
	}},
	{Name: "security.screen_lock_enabled", Label: "Screen lock enabled", Type: TBool, Get: func(f Facts) any {
		if inv(f) == nil {
			return nil
		}
		return boolp(f.Inventory.Security.ScreenLockEnabled)
	}},
	{Name: "security.pending_updates", Label: "Pending OS updates", Type: TNumber, Get: func(f Facts) any {
		if inv(f) == nil || f.Inventory.Security.PendingUpdates == nil {
			return nil
		}
		return float64(*f.Inventory.Security.PendingUpdates)
	}},
	{Name: "os.family", Label: "OS family", Type: TString, Get: func(f Facts) any { return f.Device.OSFamily }},
	{Name: "os.version", Label: "OS version", Type: TVersion, Get: func(f Facts) any { return nonEmpty(f.Device.OSVersion) }},
	{Name: "os.build", Label: "OS build", Type: TVersion, Get: func(f Facts) any { return nonEmpty(f.Device.OSBuild) }},
	{Name: "agent.version", Label: "Agent version", Type: TVersion, Get: func(f Facts) any { return nonEmpty(f.Device.AgentVersion) }},
	{Name: "agent.last_seen_minutes", Label: "Minutes since last check-in", Type: TNumber, Unit: "min", Get: func(f Facts) any {
		if f.Device.LastSeenAt == nil {
			return nil
		}
		return f.Now.Sub(*f.Device.LastSeenAt).Minutes()
	}},
	{Name: "hardware.disk_free_pct", Label: "Free disk space (system volume)", Type: TNumber, Unit: "%", Get: func(f Facts) any {
		if f.Device.DiskPct == nil {
			return nil
		}
		return 100 - *f.Device.DiskPct
	}},
	{Name: "hardware.memory_total_gb", Label: "Installed memory", Type: TNumber, Unit: "GB", Get: func(f Facts) any {
		if inv(f) == nil || f.Inventory.Hardware.MemoryTotalBytes == 0 {
			return nil
		}
		return float64(f.Inventory.Hardware.MemoryTotalBytes) / (1 << 30)
	}},
	{Name: "hardware.battery_health_pct", Label: "Battery health", Type: TNumber, Unit: "%", Get: func(f Facts) any {
		if inv(f) == nil || !f.Inventory.Hardware.Battery.Present || f.Inventory.Hardware.Battery.HealthPct == nil {
			return nil
		}
		return *f.Inventory.Hardware.Battery.HealthPct
	}},
	{Name: "network.wifi_ssid", Label: "Wi-Fi network (SSID)", Type: TString, Get: func(f Facts) any {
		if inv(f) == nil || f.Inventory.Network.WifiSSID == nil {
			return nil
		}
		return *f.Inventory.Network.WifiSSID
	}},
	{Name: "software.installed", Label: "Installed software", Type: TList, Get: func(f Facts) any {
		if inv(f) == nil {
			return nil
		}
		names := make([]string, len(f.Software))
		for i, s := range f.Software {
			names[i] = s.Name
		}
		return names
	}},
	{Name: "software.gambling", Label: "Gambling apps", Type: TFlagged, Get: func(f Facts) any {
		if inv(f) == nil {
			return nil
		}
		return flagged(f, FlagGambling)
	}},
	{Name: "software.games", Label: "Games", Type: TFlagged, Get: func(f Facts) any {
		if inv(f) == nil {
			return nil
		}
		return flagged(f, FlagGame)
	}},
	{Name: "device.tags", Label: "Device tags", Type: TList, Get: func(f Facts) any { return f.Device.Tags }},
}

func init() {
	for i := range Fields {
		Fields[i].Ops = opsByType[Fields[i].Type]
	}
}

// noValue reports whether an operator takes no operand.
func noValue(op string) bool {
	return op == "is_true" || op == "is_false" || op == "none_found" || op == "any_found"
}

func nonEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func FieldByName(name string) (Field, bool) {
	for _, f := range Fields {
		if f.Name == name {
			return f, true
		}
	}
	return Field{}, false
}

// Validate checks a rule's conditions against the field registry.
func Validate(conds []store.Condition) error {
	if len(conds) == 0 {
		return fmt.Errorf("at least one condition is required")
	}
	for _, c := range conds {
		f, ok := FieldByName(c.Field)
		if !ok {
			return fmt.Errorf("unknown field %q", c.Field)
		}
		if !contains(f.Ops, c.Op) {
			return fmt.Errorf("operator %q is not valid for field %q", c.Op, c.Field)
		}
		if noValue(c.Op) {
			continue
		}
		if _, err := operand(f.Type, c.Value); err != nil {
			return fmt.Errorf("field %q: %v", c.Field, err)
		}
	}
	return nil
}

// operand decodes a condition value for the field type.
func operand(t FieldType, raw json.RawMessage) (any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, fmt.Errorf("value is required")
	}
	switch t {
	case TNumber:
		var n float64
		if err := json.Unmarshal(raw, &n); err == nil {
			return n, nil
		}
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			if n, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
				return n, nil
			}
		}
		return nil, fmt.Errorf("value must be a number")
	case TBool:
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return nil, fmt.Errorf("value must be true or false")
		}
		return b, nil
	default:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			// Accept bare numbers for version/string fields, e.g. 14 → "14".
			var n json.Number
			if err2 := json.Unmarshal(raw, &n); err2 != nil {
				return nil, fmt.Errorf("value must be a string")
			}
			s = n.String()
		}
		if strings.TrimSpace(s) == "" {
			return nil, fmt.Errorf("value is required")
		}
		return s, nil
	}
}

// ConditionResult explains one condition's outcome.
type ConditionResult struct {
	Passed bool
	Detail string
}

// EvalCondition evaluates one condition. Unknown device data fails the
// condition: compliance should not be assumed when it can't be verified.
func EvalCondition(c store.Condition, facts Facts) ConditionResult {
	f, ok := FieldByName(c.Field)
	if !ok {
		return ConditionResult{false, fmt.Sprintf("unknown field %s", c.Field)}
	}
	actual := f.Get(facts)
	if actual == nil {
		return ConditionResult{false, fmt.Sprintf("%s: unknown", f.Label)}
	}
	desc := func(pass bool) ConditionResult {
		return ConditionResult{pass, fmt.Sprintf("%s is %s", f.Label, display(actual, f.Unit))}
	}
	switch c.Op {
	case "is_true":
		return desc(actual == true)
	case "is_false":
		return desc(actual == false)
	case "none_found", "any_found":
		found := actual.([]string)
		detail := fmt.Sprintf("%s: none found", f.Label)
		if len(found) > 0 {
			quoted := make([]string, len(found))
			for i, n := range found {
				quoted[i] = strconv.Quote(n)
			}
			detail = fmt.Sprintf("%s: found %s", f.Label, strings.Join(quoted, ", "))
		}
		return ConditionResult{(len(found) == 0) == (c.Op == "none_found"), detail}
	}
	want, err := operand(f.Type, c.Value)
	if err != nil {
		return ConditionResult{false, fmt.Sprintf("%s: invalid rule value", f.Label)}
	}
	switch f.Type {
	case TNumber:
		a, w := actual.(float64), want.(float64)
		switch c.Op {
		case "eq":
			return desc(a == w)
		case "neq":
			return desc(a != w)
		case "gt":
			return desc(a > w)
		case "gte":
			return desc(a >= w)
		case "lt":
			return desc(a < w)
		case "lte":
			return desc(a <= w)
		}
	case TVersion:
		cmp := CompareVersions(actual.(string), want.(string))
		switch c.Op {
		case "version_gte":
			return desc(cmp >= 0)
		case "version_lt":
			return desc(cmp < 0)
		case "eq":
			return desc(cmp == 0)
		case "neq":
			return desc(cmp != 0)
		}
	case TString:
		a, w := strings.ToLower(actual.(string)), strings.ToLower(want.(string))
		switch c.Op {
		case "eq":
			return desc(a == w)
		case "neq":
			return desc(a != w)
		case "contains":
			return desc(strings.Contains(a, w))
		case "not_contains":
			return desc(!strings.Contains(a, w))
		}
	case TList:
		w := strings.ToLower(want.(string))
		var match string
		for _, item := range actual.([]string) {
			if strings.Contains(strings.ToLower(item), w) {
				match = item
				break
			}
		}
		found := match != ""
		detail := fmt.Sprintf("%s: no entry matching %q", f.Label, want)
		if found {
			detail = fmt.Sprintf("%s: found %q", f.Label, match)
		}
		switch c.Op {
		case "contains":
			return ConditionResult{found, detail}
		case "not_contains":
			return ConditionResult{!found, detail}
		}
	}
	return ConditionResult{false, fmt.Sprintf("unsupported operator %s", c.Op)}
}

func display(v any, unit string) string {
	switch x := v.(type) {
	case float64:
		s := strconv.FormatFloat(x, 'f', 1, 64)
		s = strings.TrimSuffix(s, ".0")
		if unit == "%" {
			return s + "%"
		}
		if unit != "" {
			return s + " " + unit
		}
		return s
	case bool:
		if x {
			return "yes"
		}
		return "no"
	case string:
		return strconv.Quote(x)
	}
	return fmt.Sprint(v)
}

// Applies reports whether a rule targets the device at all.
func Applies(r store.Rule, d store.Device) bool {
	if !r.Enabled {
		return false
	}
	if len(r.OSFamilies) > 0 && !contains(r.OSFamilies, d.OSFamily) {
		return false
	}
	if r.GroupID != nil && (d.GroupID == nil || *d.GroupID != *r.GroupID) {
		return false
	}
	return true
}

// Evaluate runs all conditions of a rule against the facts.
func Evaluate(r store.Rule, facts Facts) (bool, string) {
	any := r.Match == "any"
	passed := !any
	var failing, passing []string
	for _, c := range r.Conditions {
		res := EvalCondition(c, facts)
		if res.Passed {
			passing = append(passing, res.Detail)
		} else {
			failing = append(failing, res.Detail)
		}
		if any {
			passed = passed || res.Passed
		} else {
			passed = passed && res.Passed
		}
	}
	if passed {
		return true, strings.Join(passing, "; ")
	}
	return false, strings.Join(failing, "; ")
}

// CompareVersions compares dotted versions numerically segment by segment
// ("10.0.22631" vs "10.0.19045"); non-numeric runs compare lexically.
func CompareVersions(a, b string) int {
	as, bs := splitVersion(a), splitVersion(b)
	for i := 0; i < len(as) || i < len(bs); i++ {
		var x, y string
		if i < len(as) {
			x = as[i]
		}
		if i < len(bs) {
			y = bs[i]
		}
		xn, xerr := strconv.Atoi(x)
		yn, yerr := strconv.Atoi(y)
		if x == "" {
			xn, xerr = 0, nil
		}
		if y == "" {
			yn, yerr = 0, nil
		}
		if xerr == nil && yerr == nil {
			if xn != yn {
				if xn < yn {
					return -1
				}
				return 1
			}
			continue
		}
		if c := strings.Compare(x, y); c != 0 {
			return c
		}
	}
	return 0
}

func splitVersion(v string) []string {
	return strings.FieldsFunc(strings.TrimPrefix(strings.TrimSpace(v), "v"), func(r rune) bool {
		return r == '.' || r == '-' || r == '_' || r == ' ' || r == '+'
	})
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
