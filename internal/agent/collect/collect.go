// Package collect gathers device inventory and telemetry. Cross-platform
// facts come from gopsutil; OS-specific details (serial, installed software,
// security posture) live in the *_linux.go, *_darwin.go and *_windows.go files.
package collect

import (
	"bytes"
	"context"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/distatus/battery"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	psnet "github.com/shirou/gopsutil/v4/net"

	"github.com/uem/uem/internal/protocol"
)

// OSFamily maps GOOS to the console's family names.
func OSFamily() string {
	switch runtime.GOOS {
	case "darwin":
		return "macos"
	default:
		return runtime.GOOS
	}
}

// Facts returns the minimal identity sent at enrollment and on connect.
func Facts(ctx context.Context, agentVersion string) protocol.Facts {
	f := protocol.Facts{OSFamily: OSFamily(), Arch: runtime.GOARCH, AgentVersion: agentVersion}
	f.Hostname, _ = os.Hostname()
	if hi, err := host.InfoWithContext(ctx); err == nil {
		f.OSName = osName(hi)
		f.OSVersion = hi.PlatformVersion
	}
	f.Serial = platformHardware(ctx).Serial
	return f
}

func osName(hi *host.InfoStat) string {
	name := hi.Platform
	switch runtime.GOOS {
	case "darwin":
		return "macOS"
	case "windows":
		return strings.TrimPrefix(name, "Microsoft ")
	}
	if name == "" {
		return "Linux"
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// Inventory collects a full snapshot. Individual collectors fail soft:
// unknown values are left empty/nil rather than aborting the snapshot.
func Inventory(ctx context.Context) protocol.Inventory {
	inv := protocol.Inventory{CollectedAt: time.Now().UTC()}

	if hi, err := host.InfoWithContext(ctx); err == nil {
		inv.OS = protocol.OSInfo{
			Family: OSFamily(), Name: osName(hi), Version: hi.PlatformVersion, Kernel: hi.KernelVersion,
			Arch: runtime.GOARCH, UptimeSeconds: hi.Uptime, BootTime: time.Unix(int64(hi.BootTime), 0).UTC(),
		}
		inv.Network.Hostname = hi.Hostname
	}
	inv.OS.Build = osBuild(ctx, inv.OS)

	inv.Hardware = platformHardware(ctx)
	if infos, err := cpu.InfoWithContext(ctx); err == nil && len(infos) > 0 {
		inv.Hardware.CPUModel = strings.TrimSpace(infos[0].ModelName)
	}
	inv.Hardware.CPUCores, _ = cpu.CountsWithContext(ctx, false)
	inv.Hardware.CPUThreads, _ = cpu.CountsWithContext(ctx, true)
	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		inv.Hardware.MemoryTotalBytes, inv.Hardware.MemoryUsedBytes = vm.Total, vm.Used
	}
	inv.Hardware.Disks = disks(ctx)
	inv.Hardware.Battery = batteryInfo()

	inv.Network = network(ctx, inv.Network.Hostname)
	inv.Security = security(ctx)
	inv.Users.LoggedIn = loggedInUsers(ctx)
	inv.Software = software(ctx)
	sort.Slice(inv.Software, func(i, j int) bool {
		return strings.ToLower(inv.Software[i].Name) < strings.ToLower(inv.Software[j].Name)
	})
	return inv
}

// Telemetry samples utilization; it blocks ~1s to measure CPU.
func Telemetry(ctx context.Context) protocol.Telemetry {
	t := protocol.Telemetry{TS: time.Now().UTC()}
	if pct, err := cpu.PercentWithContext(ctx, time.Second, false); err == nil && len(pct) > 0 {
		t.CPUPct = round1(pct[0])
	}
	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		t.MemPct = round1(vm.UsedPercent)
	}
	if u, err := disk.UsageWithContext(ctx, systemMount()); err == nil {
		t.DiskPct = round1(u.UsedPercent)
	}
	if b := batteryInfo(); b.Present {
		p := round1(b.Percent)
		t.BatteryPct = &p
	}
	t.PrimaryIP, _ = primaryAddr(ctx)
	if users := loggedInUsers(ctx); len(users) > 0 {
		t.LoggedIn = users[0]
	}
	return t
}

func round1(f float64) float64 { return float64(int64(f*10+0.5)) / 10 }

func systemMount() string {
	if runtime.GOOS == "windows" {
		if d := os.Getenv("SystemDrive"); d != "" {
			return d + `\`
		}
		return `C:\`
	}
	return "/"
}

var pseudoFS = map[string]bool{
	"tmpfs": true, "devtmpfs": true, "overlay": true, "squashfs": true, "proc": true, "sysfs": true,
	"devfs": true, "autofs": true, "nullfs": true, "fuse.snapfuse": true, "efivarfs": true,
}

func disks(ctx context.Context) []protocol.Disk {
	parts, err := disk.PartitionsWithContext(ctx, false)
	out := []protocol.Disk{}
	seen := map[string]bool{}
	if err == nil {
		for _, p := range parts {
			if pseudoFS[p.Fstype] || seen[p.Device] || strings.HasPrefix(p.Mountpoint, "/System/Volumes/") && p.Mountpoint != "/System/Volumes/Data" {
				continue
			}
			u, err := disk.UsageWithContext(ctx, p.Mountpoint)
			if err != nil || u.Total == 0 {
				continue
			}
			seen[p.Device] = true
			out = append(out, protocol.Disk{Mount: p.Mountpoint, FSType: p.Fstype, TotalBytes: u.Total, UsedBytes: u.Used, FreeBytes: u.Free})
		}
	}
	// Containers often expose only an overlay root; still report it.
	if len(out) == 0 {
		if u, err := disk.UsageWithContext(ctx, systemMount()); err == nil {
			out = append(out, protocol.Disk{Mount: systemMount(), FSType: u.Fstype, TotalBytes: u.Total, UsedBytes: u.Used, FreeBytes: u.Free})
		}
	}
	return out
}

func batteryInfo() protocol.Battery {
	bats, err := battery.GetAll()
	if err != nil || len(bats) == 0 || bats[0] == nil || bats[0].Full == 0 {
		return protocol.Battery{Present: false, State: "none"}
	}
	b := bats[0]
	out := protocol.Battery{Present: true, Percent: round1(b.Current / b.Full * 100), State: strings.ToLower(b.State.String())}
	if b.Design > 0 {
		h := round1(b.Full / b.Design * 100)
		out.HealthPct = &h
	}
	return out
}

// primaryAddr finds the interface used for the default route by asking the
// kernel which source address it would use (no packets are sent).
func primaryAddr(ctx context.Context) (ip, iface string) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "udp", "8.8.8.8:80")
	if err != nil {
		return "", ""
	}
	defer conn.Close()
	local := conn.LocalAddr().(*net.UDPAddr).IP
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.Equal(local) {
				return local.String(), i.Name
			}
		}
	}
	return local.String(), ""
}

func network(ctx context.Context, hostname string) protocol.Network {
	n := protocol.Network{Hostname: hostname, Interfaces: []protocol.Interface{}}
	if n.Hostname == "" {
		n.Hostname, _ = os.Hostname()
	}
	ip, primary := primaryAddr(ctx)
	n.PrimaryIP = ip
	ifs, err := psnet.InterfacesWithContext(ctx)
	if err == nil {
		for _, i := range ifs {
			loopback := false
			up := false
			for _, f := range i.Flags {
				switch f {
				case "loopback":
					loopback = true
				case "up":
					up = true
				}
			}
			if loopback {
				continue
			}
			addrs := []string{}
			for _, a := range i.Addrs {
				addrs = append(addrs, a.Addr)
			}
			n.Interfaces = append(n.Interfaces, protocol.Interface{Name: i.Name, MAC: i.HardwareAddr, Addrs: addrs, Up: up})
			if i.Name == primary {
				n.PrimaryMAC = i.HardwareAddr
			}
		}
	}
	if ssid := wifiSSID(ctx); ssid != "" {
		n.WifiSSID = &ssid
	}
	return n
}

func loggedInUsers(ctx context.Context) []string {
	out := []string{}
	if users, err := host.UsersWithContext(ctx); err == nil {
		seen := map[string]bool{}
		for _, u := range users {
			if u.User != "" && !seen[u.User] {
				seen[u.User] = true
				out = append(out, u.User)
			}
		}
	}
	if len(out) == 0 {
		if u := consoleUser(ctx); u != "" {
			out = append(out, u)
		}
	}
	return out
}

// run executes a command with a timeout and returns trimmed stdout.
func run(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

func ptr[T any](v T) *T { return &v }
