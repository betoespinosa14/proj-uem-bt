package collect

import (
	"context"
	"time"

	"github.com/uem/uem/internal/protocol"
)

// netDetails reads the default gateway, DNS servers, each port's type and
// name, link speeds and whether active interfaces use DHCP.
func netDetails(ctx context.Context, _ string, ifaces []protocol.Interface) netExtras {
	x := netExtras{OK: true, Ifaces: map[string]ifaceExtra{}}
	if out, err := run(ctx, 5*time.Second, "networksetup", "-listallhardwareports"); err == nil {
		x.Ifaces = parseHardwarePorts(out)
	}
	speeds := map[string]float64{}
	if out, err := run(ctx, 5*time.Second, "ifconfig"); err == nil {
		speeds = parseIfconfigMedia(out)
	}
	if out, err := run(ctx, 5*time.Second, "route", "-n", "get", "default"); err == nil {
		gw, dev := parseRouteGet(out)
		x.Gateway = gw
		if dev != "" && gw != "" {
			e := x.Ifaces[dev]
			e.Gateway = gw
			x.Ifaces[dev] = e
		}
	}
	if out, err := run(ctx, 5*time.Second, "scutil", "--dns"); err == nil {
		x.DNS = parseScutilDNS(out)
	}
	for _, it := range ifaces {
		e := x.Ifaces[it.Name]
		e.SpeedMbps = speeds[it.Name]
		if it.Up && hasIPv4(it.Addrs) {
			// "ipconfig getpacket" prints the DHCP lease, and nothing for a static address.
			out, err := run(ctx, 5*time.Second, "ipconfig", "getpacket", it.Name)
			e.DHCP = ptr(err == nil && out != "")
		}
		x.Ifaces[it.Name] = e
	}
	return x
}
