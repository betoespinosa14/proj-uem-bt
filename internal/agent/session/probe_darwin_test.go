package session

import (
	"testing"
	"time"
)

func TestParseDarwin(t *testing.T) {
	if got := parseFront("ASN:0x0-0x8d28d2:\n"); got != "ASN:0x0-0x8d28d2:" {
		t.Errorf("parseFront = %q", got)
	}
	info := `[ NULL ]  ASN:0x0-0x8d28d2: (in front) 
    bundleID=[ NULL ] 
    bundle path="/Applications/Visual Studio Code.app"
    executable path=[ NULL ] 
`
	if got := parseBundlePath(info); got != "/Applications/Visual Studio Code.app" {
		t.Errorf("parseBundlePath = %q", got)
	}
	if got := parseBundlePath("[ NULL ] bundle path=[ NULL ]"); got != "" {
		t.Errorf("parseBundlePath(null) = %q", got)
	}
	d, err := parseHIDIdle(`  |   "HIDIdleTime" = 44578440416` + "\n")
	if err != nil || d != 44578440416*time.Nanosecond {
		t.Errorf("parseHIDIdle = %v, %v", d, err)
	}
	if _, err := parseHIDIdle("nothing"); err == nil {
		t.Error("parseHIDIdle accepted garbage")
	}
}
