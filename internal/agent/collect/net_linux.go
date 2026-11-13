package collect

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/uem/uem/internal/protocol"
)

// netDetails reads gateways, DNS servers, DHCP and each interface's type and
// speed from ip, resolvectl or resolv.conf, and sysfs.
func netDetails(ctx context.Context, _ string, ifaces []protocol.Interface) netExtras {
	x := netExtras{OK: true, Ifaces: map[string]ifaceExtra{}, DNSByIface: map[string][]string{}}
	gws := map[string]string{}
	if out, err := run(ctx, 5*time.Second, "ip", "route", "show", "default"); err == nil {
		x.Gateway, gws = parseIPRouteDefault(out)
	}
	dyn := map[string]bool{}
	if out, err := run(ctx, 5*time.Second, "ip", "-o", "-4", "addr", "show"); err == nil {
		dyn = parseIPAddrDynamic(out)
	}
	if _, err := exec.LookPath("resolvectl"); err == nil {
		if out, err := run(ctx, 5*time.Second, "resolvectl", "dns"); err == nil {
			x.DNS, x.DNSByIface = parseResolvectl(out)
		}
	}
	if len(x.DNS) == 0 {
		if b, err := os.ReadFile("/etc/resolv.conf"); err == nil {
			x.DNS = parseResolvConf(string(b))
		}
	}
	for _, it := range ifaces {
		base := "/sys/class/net/" + it.Name
		e := ifaceExtra{Gateway: gws[it.Name]}
		if d, ok := dyn[it.Name]; ok {
			e.DHCP = ptr(d)
		}
		if b, err := os.ReadFile(base + "/speed"); err == nil {
			if n, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64); err == nil && n > 0 {
				e.SpeedMbps = n
			}
		}
		e.Kind = kindFromName(it.Name, "")
		if e.Kind == "" {
			_, wireless := os.Stat(base + "/wireless")
			_, device := os.Stat(base + "/device")
			switch {
			case wireless == nil:
				e.Kind = "wifi"
			case device != nil: // no hardware behind it
				e.Kind = "virtual"
			default:
				e.Kind = "ethernet"
			}
		}
		x.Ifaces[it.Name] = e
	}
	return x
}
