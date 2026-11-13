package collect

// Parsers for the network details each platform reports (gateway, DNS servers,
// domain, adapter type and speed, DHCP, Wi-Fi link). Like hwparse.go they take
// raw command output so they can be tested on any OS. Everything is best
// effort: what can't be read stays empty.

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/uem/uem/internal/protocol"
)

type ifaceExtra struct {
	Kind        string
	Description string
	Gateway     string
	SpeedMbps   float64
	DHCP        *bool
}

// netExtras is what a platform adds to protocol.Network beyond the basics.
type netExtras struct {
	OK           bool // the platform ran its collection (sets Network.Details)
	Gateway      string
	DNS          []string
	DNSByIface   map[string][]string
	Domain       string
	DomainJoined *bool
	Wifi         *protocol.WifiLink
	Ifaces       map[string]ifaceExtra
}

func (x netExtras) apply(n *protocol.Network, primary string) {
	if !x.OK {
		return
	}
	n.Details = true
	for i := range n.Interfaces {
		it := &n.Interfaces[i]
		e := x.Ifaces[it.Name]
		it.Description, it.Gateway, it.DHCP = e.Description, e.Gateway, e.DHCP
		if it.Up {
			it.SpeedMbps = e.SpeedMbps
		}
		it.Kind = e.Kind
		if it.Kind == "" {
			it.Kind = kindFromName(it.Name, it.Description)
		}
	}
	n.Gateway = x.Ifaces[primary].Gateway
	if n.Gateway == "" {
		n.Gateway = x.Gateway
	}
	n.DNS = x.DNSByIface[primary]
	if len(n.DNS) == 0 {
		n.DNS = x.DNS
	}
	n.Domain, n.DomainJoined = x.Domain, x.DomainJoined
	n.Wifi = x.Wifi
	if n.Wifi != nil && n.Wifi.SSID == "" && n.WifiSSID != nil {
		n.Wifi.SSID = *n.WifiSSID
	}
	if n.Wifi == nil && n.WifiSSID != nil {
		n.Wifi = &protocol.WifiLink{SSID: *n.WifiSSID}
	}
}

var (
	kindBluetooth = regexp.MustCompile(`(?i)bluetooth`)
	kindVPN       = regexp.MustCompile(`(?i)tailscale|wireguard|openvpn|\bvpn\b|surfshark|nordvpn|expressvpn|protonvpn|tap-windows|wintun|zerotier|fortinet|forticlient|anyconnect|globalprotect|pangp|\bipsec|^utun\d|^tun\d|^wg\d|^ppp\d`)
	kindVirtual   = regexp.MustCompile(`(?i)wi-fi direct|^local area connection\*|vethernet|hyper-v|virtualbox|vmware|docker|^veth|^br-|^virbr|^bridge\d|^awdl|^llw|^anpi|^ap\d|^gif\d|^stf\d|isatap|teredo|kernel debug|loopback|wan miniport`)
	kindWifi      = regexp.MustCompile(`(?i)wi-?fi|wireless|wlan|802\.11|^wl`)
	kindEthernet  = regexp.MustCompile(`(?i)ethernet|^eth\d|^en[pso]\d|gbe|gigabit|realtek pcie|intel\(r\) ethernet`)
)

// kindFromName guesses an adapter's type from its name and description when
// the platform didn't say. "" means unknown.
func kindFromName(name, desc string) string {
	s := name + " " + desc
	switch {
	case kindBluetooth.MatchString(s):
		return "bluetooth"
	case kindVPN.MatchString(name) || kindVPN.MatchString(desc):
		return "vpn"
	case kindVirtual.MatchString(name) || kindVirtual.MatchString(desc):
		return "virtual"
	case kindWifi.MatchString(name) || kindWifi.MatchString(desc):
		return "wifi"
	case kindEthernet.MatchString(name) || kindEthernet.MatchString(desc):
		return "ethernet"
	}
	return ""
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// ---- Windows ----

// windowsNetScript prints one JSON object with everything parseWindowsNet reads.
const windowsNetScript = `$ErrorActionPreference='SilentlyContinue'
$cs=Get-CimInstance Win32_ComputerSystem
@{
 domain=[string]$cs.Domain; joined=[bool]$cs.PartOfDomain; workgroup=[string]$cs.Workgroup
 adapters=@(Get-NetAdapter -IncludeHidden | ForEach-Object { @{name=[string]$_.Name; desc=[string]$_.InterfaceDescription; speed=[uint64]$_.Speed; virtual=[bool]$_.Virtual; hw=[bool]$_.HardwareInterface; media=[string]$_.PhysicalMediaType; status=[string]$_.Status} })
 gw=@(Get-NetRoute -DestinationPrefix '0.0.0.0/0' | ForEach-Object { @{alias=[string]$_.InterfaceAlias; hop=[string]$_.NextHop; metric=[int]$_.RouteMetric} })
 dns=@(Get-DnsClientServerAddress -AddressFamily IPv4 | Where-Object { $_.ServerAddresses } | ForEach-Object { @{alias=[string]$_.InterfaceAlias; servers=@($_.ServerAddresses)} })
 ip=@(Get-NetIPInterface -AddressFamily IPv4 | ForEach-Object { @{alias=[string]$_.InterfaceAlias; dhcp=[string]$_.Dhcp} })
} | ConvertTo-Json -Compress -Depth 4`

func parseWindowsNet(out string) netExtras {
	var v struct {
		Domain    string          `json:"domain"`
		Joined    bool            `json:"joined"`
		Workgroup string          `json:"workgroup"`
		Adapters  json.RawMessage `json:"adapters"`
		GW        json.RawMessage `json:"gw"`
		DNS       json.RawMessage `json:"dns"`
		IP        json.RawMessage `json:"ip"`
	}
	x := netExtras{Ifaces: map[string]ifaceExtra{}, DNSByIface: map[string][]string{}}
	if json.Unmarshal([]byte(strings.TrimSpace(out)), &v) != nil {
		return x
	}
	x.OK = true
	joined := v.Joined
	x.DomainJoined = &joined
	if joined {
		x.Domain = strings.TrimSpace(v.Domain)
	} else {
		x.Domain = strings.TrimSpace(v.Workgroup)
	}
	for _, a := range oneOrMany[struct {
		Name    string  `json:"name"`
		Desc    string  `json:"desc"`
		Speed   float64 `json:"speed"`
		Virtual bool    `json:"virtual"`
		HW      bool    `json:"hw"`
		Media   string  `json:"media"`
		Status  string  `json:"status"`
	}](v.Adapters) {
		if a.Name == "" {
			continue
		}
		e := x.Ifaces[a.Name]
		e.Description = strings.TrimSpace(a.Desc)
		if strings.EqualFold(a.Status, "Up") && a.Speed > 0 && a.Speed < 1e12 {
			e.SpeedMbps = a.Speed / 1e6
		}
		e.Kind = kindFromName(a.Name, e.Description)
		if e.Kind == "" {
			media := strings.ToLower(a.Media)
			switch {
			case strings.Contains(media, "802.11"):
				e.Kind = "wifi"
			case strings.Contains(media, "bluetooth"):
				e.Kind = "bluetooth"
			case !a.HW || a.Virtual:
				e.Kind = "virtual"
			case strings.Contains(media, "802.3"):
				e.Kind = "ethernet"
			}
		}
		x.Ifaces[a.Name] = e
	}
	best := map[string]int{}
	bestAll := -1
	for _, g := range oneOrMany[struct {
		Alias  string `json:"alias"`
		Hop    string `json:"hop"`
		Metric int    `json:"metric"`
	}](v.GW) {
		if g.Hop == "" || g.Hop == "0.0.0.0" {
			continue
		}
		if m, ok := best[g.Alias]; !ok || g.Metric < m {
			best[g.Alias] = g.Metric
			e := x.Ifaces[g.Alias]
			e.Gateway = g.Hop
			x.Ifaces[g.Alias] = e
		}
		if bestAll < 0 || g.Metric < bestAll {
			bestAll, x.Gateway = g.Metric, g.Hop
		}
	}
	for _, d := range oneOrMany[struct {
		Alias   string          `json:"alias"`
		Servers json.RawMessage `json:"servers"`
	}](v.DNS) {
		s := uniq(oneOrMany[string](d.Servers))
		if len(s) == 0 {
			continue
		}
		x.DNSByIface[d.Alias] = s
		if len(x.DNS) == 0 {
			x.DNS = s
		}
	}
	for _, i := range oneOrMany[struct {
		Alias string `json:"alias"`
		DHCP  string `json:"dhcp"`
	}](v.IP) {
		e := x.Ifaces[i.Alias]
		switch strings.ToLower(i.DHCP) {
		case "enabled":
			e.DHCP = ptr(true)
		case "disabled":
			e.DHCP = ptr(false)
		default:
			continue
		}
		x.Ifaces[i.Alias] = e
	}
	return x
}

var leadingNum = regexp.MustCompile(`^[\d.]+`)

// parseNetshWlan reads "netsh wlan show interfaces": the first connected
// interface's SSID, signal, band, radio type and receive rate.
func parseNetshWlan(out string) *protocol.WifiLink {
	var cur *protocol.WifiLink
	connected := false
	for _, line := range strings.Split(out, "\n") {
		f := strings.SplitN(line, ":", 2)
		if len(f) != 2 {
			continue
		}
		key, val := strings.TrimSpace(f[0]), strings.TrimSpace(f[1])
		switch key {
		case "Name":
			if cur != nil && connected {
				return cur
			}
			cur, connected = &protocol.WifiLink{}, false
		case "State":
			connected = strings.EqualFold(val, "connected")
		case "SSID":
			if cur != nil {
				cur.SSID = val
			}
		case "Signal":
			if n, err := strconv.Atoi(strings.TrimSuffix(val, "%")); err == nil && cur != nil {
				cur.SignalPct = &n
			}
		case "Band":
			if cur != nil {
				cur.Band = val
			}
		case "Radio type":
			if cur != nil {
				cur.Radio = val
			}
		case "Receive rate (Mbps)":
			if n, err := strconv.ParseFloat(leadingNum.FindString(val), 64); err == nil && cur != nil {
				cur.RxMbps = n
			}
		}
	}
	if cur != nil && connected {
		return cur
	}
	return nil
}

// ---- macOS ----

// parseRouteGet reads "route -n get default": the gateway and its interface.
func parseRouteGet(out string) (gateway, iface string) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "gateway:"); ok {
			gateway = strings.TrimSpace(v)
		} else if v, ok := strings.CutPrefix(line, "interface:"); ok {
			iface = strings.TrimSpace(v)
		}
	}
	return gateway, iface
}

var scutilNameserver = regexp.MustCompile(`^\s*nameserver\[\d+\]\s*:\s*(\S+)`)

// parseScutilDNS reads "scutil --dns": the name servers of the first
// (default) resolver.
func parseScutilDNS(out string) []string {
	var servers []string
	inFirst := false
	for _, line := range strings.Split(out, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "resolver #") {
			if inFirst {
				break
			}
			inFirst = t == "resolver #1"
			continue
		}
		if t == "DNS configuration (for scoped queries)" {
			break
		}
		if m := scutilNameserver.FindStringSubmatch(line); m != nil && inFirst {
			servers = append(servers, m[1])
		}
	}
	return uniq(servers)
}

// parseHardwarePorts reads "networksetup -listallhardwareports": each device's
// port name (its description) and type.
func parseHardwarePorts(out string) map[string]ifaceExtra {
	m := map[string]ifaceExtra{}
	port := ""
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "Hardware Port:"); ok {
			port = strings.TrimSpace(v)
		} else if v, ok := strings.CutPrefix(line, "Device:"); ok && port != "" {
			dev := strings.TrimSpace(v)
			kind := ""
			p := strings.ToLower(port)
			switch {
			case strings.Contains(p, "wi-fi") || strings.Contains(p, "airport"):
				kind = "wifi"
			case strings.Contains(p, "bluetooth"):
				kind = "bluetooth"
			case strings.Contains(p, "bridge"):
				kind = "virtual"
			case strings.Contains(p, "ethernet") || strings.Contains(p, "lan") || strings.Contains(p, "usb"):
				kind = "ethernet"
			}
			m[dev] = ifaceExtra{Kind: kind, Description: port}
			port = ""
		}
	}
	return m
}

var ifconfigHeader = regexp.MustCompile(`^([a-zA-Z0-9]+): flags=`)
var mediaSpeed = regexp.MustCompile(`(?i)\((\d+)(g?)base`)

// parseIfconfigMedia reads macOS "ifconfig": each interface's link speed in
// Mbps from its media line, e.g. "media: autoselect (1000baseT <full-duplex>)".
func parseIfconfigMedia(out string) map[string]float64 {
	m := map[string]float64{}
	cur := ""
	for _, line := range strings.Split(out, "\n") {
		if h := ifconfigHeader.FindStringSubmatch(line); h != nil {
			cur = h[1]
			continue
		}
		t := strings.TrimSpace(line)
		if !strings.HasPrefix(t, "media:") || cur == "" {
			continue
		}
		if s := mediaSpeed.FindStringSubmatch(t); s != nil {
			n, _ := strconv.ParseFloat(s[1], 64)
			if s[2] != "" {
				n *= 1000
			}
			m[cur] = n
		}
	}
	return m
}

// ---- Linux ----

// parseIPRouteDefault reads "ip route show default": the gateway per
// interface, and the one with the lowest metric.
func parseIPRouteDefault(out string) (best string, byIface map[string]string) {
	byIface = map[string]string{}
	bestMetric := -1
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || f[0] != "default" {
			continue
		}
		var gw, dev string
		metric := 0
		for i := 0; i+1 < len(f); i++ {
			switch f[i] {
			case "via":
				gw = f[i+1]
			case "dev":
				dev = f[i+1]
			case "metric":
				metric, _ = strconv.Atoi(f[i+1])
			}
		}
		if gw == "" {
			continue
		}
		if _, ok := byIface[dev]; !ok {
			byIface[dev] = gw
		}
		if bestMetric < 0 || metric < bestMetric {
			bestMetric, best = metric, gw
		}
	}
	return best, byIface
}

var resolvectlLink = regexp.MustCompile(`^Link \d+ \(([^)]+)\):\s*(.*)$`)

// parseResolvectl reads "resolvectl dns": the global servers and each link's.
func parseResolvectl(out string) (global []string, byIface map[string][]string) {
	byIface = map[string][]string{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "Global:"); ok {
			global = uniq(strings.Fields(v))
		} else if m := resolvectlLink.FindStringSubmatch(line); m != nil {
			if s := uniq(strings.Fields(m[2])); len(s) > 0 {
				byIface[m[1]] = s
			}
		}
	}
	return global, byIface
}

// parseResolvConf reads /etc/resolv.conf. systemd-resolved's local stub
// (127.0.0.53) isn't a real server, so it's skipped.
func parseResolvConf(out string) []string {
	var s []string
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == "nameserver" && f[1] != "127.0.0.53" && f[1] != "127.0.0.54" {
			s = append(s, f[1])
		}
	}
	return uniq(s)
}

// parseIPAddrDynamic reads "ip -o -4 addr show": whether each interface's
// address came from DHCP (marked "dynamic").
func parseIPAddrDynamic(out string) map[string]bool {
	m := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 || f[2] != "inet" {
			continue
		}
		dev := strings.TrimSuffix(f[1], ":")
		dyn := false
		for _, w := range f[4:] {
			if w == "dynamic" {
				dyn = true
			}
		}
		m[dev] = m[dev] || dyn
	}
	return m
}
