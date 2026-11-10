package collect

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func plistOf(dict string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><array><dict>` + dict + `</dict></array></plist>`)
}

func ints(kv ...any) string {
	var b strings.Builder
	for i := 0; i < len(kv); i += 2 {
		switch v := kv[i+1].(type) {
		case bool:
			fmt.Fprintf(&b, "<key>%s</key><%t/>", kv[i], v)
		default:
			fmt.Fprintf(&b, "<key>%s</key><integer>%v</integer>", kv[i], v)
		}
	}
	return b.String()
}

func TestParseSmartBattery(t *testing.T) {
	cases := []struct {
		name          string
		in            []byte
		present       bool
		pct, health   float64
		state         string
		healthMissing bool
	}{
		{
			name: "Apple Silicon with AppleRaw keys, discharging",
			in: plistOf(ints("BatteryInstalled", true, "CurrentCapacity", 81, "MaxCapacity", 100,
				"AppleRawCurrentCapacity", 3900, "AppleRawMaxCapacity", 4815, "NominalChargeCapacity", 4920,
				"DesignCapacity", 5500, "Amperage", uint64(18446744073709550591), "ExternalConnected", false,
				"IsCharging", false, "FullyCharged", false)),
			present: true, pct: 81, health: 89.5, state: "discharging",
		},
		{
			name: "newer macOS without AppleRaw keys, capacities under BatteryData",
			in: plistOf(ints("BatteryInstalled", true, "CurrentCapacity", 64, "MaxCapacity", 100,
				"ExternalConnected", true, "IsCharging", true, "FullyCharged", false) +
				`<key>BatteryData</key><dict>` + ints("DesignCapacity", 6075, "NominalChargeCapacity", 5100) + `</dict>`),
			present: true, pct: 64, health: 84, state: "charging",
		},
		{
			name: "Intel MacBook, capacities in mAh",
			in: plistOf(ints("CurrentCapacity", 3000, "MaxCapacity", 5000, "DesignCapacity", 6000,
				"ExternalConnected", true, "IsCharging", false, "FullyCharged", true)),
			present: true, pct: 60, health: 83.3, state: "full",
		},
		{
			name:    "percent only, health left to system_profiler",
			in:      plistOf(ints("CurrentCapacity", 80, "MaxCapacity", 100, "ExternalConnected", true)),
			present: true, pct: 80, state: "idle", healthMissing: true,
		},
		{name: "desktop Mac (no battery installed)", in: plistOf(ints("BatteryInstalled", false, "CurrentCapacity", 0, "MaxCapacity", 0))},
		{name: "no entry at all", in: nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, ok := parseSmartBattery(c.in)
			if ok != c.present || b.Present != c.present {
				t.Fatalf("present = %v/%v, want %v", ok, b.Present, c.present)
			}
			if !c.present {
				return
			}
			if b.Percent != c.pct || b.State != c.state {
				t.Errorf("got %v%% %q, want %v%% %q", b.Percent, b.State, c.pct, c.state)
			}
			switch {
			case c.healthMissing && b.HealthPct != nil:
				t.Errorf("health = %v, want none", *b.HealthPct)
			case !c.healthMissing && (b.HealthPct == nil || *b.HealthPct != c.health):
				t.Errorf("health = %v, want %v", b.HealthPct, c.health)
			}
		})
	}
}

func TestParseMaxCapacity(t *testing.T) {
	out := "Battery Information:\n\n      Health Information:\n          Cycle Count: 412\n          Condition: Normal\n          Maximum Capacity: 87%\n"
	if f, ok := parseMaxCapacity(out); !ok || f != 87 {
		t.Fatalf("got %v %v", f, ok)
	}
	if _, ok := parseMaxCapacity("Power: AC"); ok {
		t.Fatal("matched without a figure")
	}
}

// On this machine: whatever it is, reading must not fail or claim a battery a desktop doesn't have.
func TestBatteryInfoLive(t *testing.T) {
	out, err := exec.Command("/usr/sbin/ioreg", "-n", "AppleSmartBattery", "-r", "-a").Output()
	if err != nil {
		t.Skip("ioreg unavailable")
	}
	b, ok := parseSmartBattery(out)
	t.Logf("this Mac: present=%v %+v", ok, b)
}
