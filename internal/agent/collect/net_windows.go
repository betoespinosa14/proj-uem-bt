package collect

import (
	"context"
	"time"

	"github.com/uem/uem/internal/protocol"
)

// netDetails reads adapter types and speeds, gateways, DNS servers, DHCP and
// the domain in one PowerShell call (see windowsNetScript), plus the Wi-Fi link.
func netDetails(ctx context.Context, _ string, _ []protocol.Interface) netExtras {
	out, err := run(ctx, 45*time.Second, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-EncodedCommand", encodePS(windowsNetScript))
	if err != nil && out == "" {
		return netExtras{}
	}
	x := parseWindowsNet(out)
	if w, err := run(ctx, 5*time.Second, "netsh", "wlan", "show", "interfaces"); err == nil {
		x.Wifi = parseNetshWlan(w)
	}
	return x
}
