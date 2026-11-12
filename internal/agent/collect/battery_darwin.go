package collect

import (
	"os/exec"
	"regexp"
	"strconv"
	"sync"
	"time"

	plist "howett.net/plist"

	"github.com/uem/uem/internal/protocol"
)

// batteryInfo reads the Mac's battery from the AppleSmartBattery registry entry. The keys differ
// between Intel and Apple Silicon and between macOS versions (newer releases drop the AppleRaw*
// keys the battery library depends on), so each value is looked up from several places.
func batteryInfo() protocol.Battery {
	out, err := exec.Command("/usr/sbin/ioreg", "-n", "AppleSmartBattery", "-r", "-a").Output()
	if err != nil {
		return noBattery()
	}
	b, ok := parseSmartBattery(out)
	if !ok {
		return noBattery()
	}
	if b.HealthPct == nil {
		if h, ok := profilerHealth(); ok {
			b.HealthPct = &h
		}
	}
	return b
}

func noBattery() protocol.Battery { return protocol.Battery{Present: false, State: "none"} }

// parseSmartBattery turns `ioreg -n AppleSmartBattery -r -a` output into a battery reading.
func parseSmartBattery(out []byte) (protocol.Battery, bool) {
	var entries []map[string]any
	if len(out) == 0 {
		return protocol.Battery{}, false
	}
	if _, err := plist.Unmarshal(out, &entries); err != nil || len(entries) == 0 {
		return protocol.Battery{}, false
	}
	m := entries[0]
	if installed, ok := m["BatteryInstalled"].(bool); ok && !installed {
		return protocol.Battery{}, false // a desktop Mac reports the entry with no battery in it
	}
	data, _ := m["BatteryData"].(map[string]any)

	// Charge: a matching pair from the same family of keys, so the units agree.
	cur, max := num(m["AppleRawCurrentCapacity"]), num(m["AppleRawMaxCapacity"])
	if cur <= 0 || max <= 0 {
		cur, max = num(m["CurrentCapacity"]), num(m["MaxCapacity"]) // percent on Apple Silicon, mAh on Intel
	}
	if max <= 0 {
		return protocol.Battery{}, false
	}
	pct := cur / max * 100
	if pct > 100 {
		pct = 100
	}
	b := protocol.Battery{Present: true, Percent: round1(pct), State: macBatteryState(m)}

	// Health: full-charge capacity against design capacity, both in mAh.
	design := first(num(m["DesignCapacity"]), num(data["DesignCapacity"]))
	full := first(num(m["NominalChargeCapacity"]), num(data["NominalChargeCapacity"]), num(m["AppleRawMaxCapacity"]))
	if full == 0 && num(m["MaxCapacity"]) > 100 { // Intel: MaxCapacity is already mAh
		full = num(m["MaxCapacity"])
	}
	if design > 0 && full > 0 {
		h := round1(full / design * 100)
		b.HealthPct = &h
	}
	if c := int(first(num(m["CycleCount"]), num(data["CycleCount"]))); c > 0 {
		b.CycleCount = &c
	}
	return b, true
}

func macBatteryState(m map[string]any) string {
	external, _ := m["ExternalConnected"].(bool)
	charging, _ := m["IsCharging"].(bool)
	full, _ := m["FullyCharged"].(bool)
	switch {
	case !external:
		return "discharging"
	case charging:
		return "charging"
	case full:
		return "full"
	default:
		return "idle" // on power, not charging (e.g. macOS holding the charge at 80%)
	}
}

// num reads a plist number. ioreg writes signed values as unsigned 64-bit, so very large values
// are negative numbers; capacities are never negative, so those read as 0.
func num(v any) float64 {
	switch n := v.(type) {
	case int64:
		return float64(n)
	case uint64:
		if n > 1<<62 {
			return 0
		}
		return float64(n)
	case float64:
		return n
	}
	return 0
}

func first(vals ...float64) float64 {
	for _, v := range vals {
		if v > 0 {
			return v
		}
	}
	return 0
}

// profilerHealth is macOS's own "Maximum Capacity" figure, used when the registry has no
// capacities in mAh. system_profiler takes about a second, so the answer is kept for 6 hours.
var profiler struct {
	sync.Mutex
	at  time.Time
	pct float64
	ok  bool
}

var maxCapacityRe = regexp.MustCompile(`Maximum Capacity:\s*(\d+(?:\.\d+)?)\s*%`)

func profilerHealth() (float64, bool) {
	profiler.Lock()
	defer profiler.Unlock()
	if !profiler.at.IsZero() && time.Since(profiler.at) < 6*time.Hour {
		return profiler.pct, profiler.ok
	}
	profiler.at = time.Now()
	out, err := exec.Command("/usr/sbin/system_profiler", "SPPowerDataType").Output()
	profiler.pct, profiler.ok = parseMaxCapacity(string(out))
	if err != nil {
		profiler.ok = false
	}
	return profiler.pct, profiler.ok
}

func parseMaxCapacity(s string) (float64, bool) {
	m := maxCapacityRe.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	f, err := strconv.ParseFloat(m[1], 64)
	return f, err == nil
}
