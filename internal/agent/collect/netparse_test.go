package collect

import (
	"reflect"
	"testing"

	"github.com/uem/uem/internal/protocol"
)

func TestKindFromName(t *testing.T) {
	for _, c := range []struct{ name, desc, want string }{
		{"Ethernet", "Intel(R) Ethernet Connection (16) I219-LM", "ethernet"},
		{"Wi-Fi", "Intel(R) Wi-Fi 6E AX211 160MHz", "wifi"},
		{"Tailscale", "Tailscale Tunnel", "vpn"},
		{"OpenVPN Data Channel Offload for Surfshark", "", "vpn"},
		{"Local Area Connection", "TAP-Windows Adapter V9", "vpn"},
		{"Local Area Connection* 1", "Microsoft Wi-Fi Direct Virtual Adapter", "virtual"},
		{"Bluetooth Network Connection", "Bluetooth Device (Personal Area Network)", "bluetooth"},
		{"vEthernet (Default Switch)", "Hyper-V Virtual Ethernet Adapter", "virtual"},
		{"eth0", "", "ethernet"},
		{"enp3s0", "", "ethernet"},
		{"wlan0", "", "wifi"},
		{"docker0", "", "virtual"},
		{"tailscale0", "", "vpn"},
		{"utun3", "", "vpn"},
		{"awdl0", "", "virtual"},
		{"Local Area Connection", "", ""},
	} {
		if got := kindFromName(c.name, c.desc); got != c.want {
			t.Errorf("kindFromName(%q, %q) = %q, want %q", c.name, c.desc, got, c.want)
		}
	}
}

func TestParseWindowsNet(t *testing.T) {
	out := `{"domain":"WORKGROUP","joined":false,"workgroup":"WORKGROUP",
	"adapters":[{"name":"Ethernet","desc":"Intel(R) Ethernet Connection (16) I219-LM","speed":1000000000,"virtual":false,"hw":true,"media":"802.3","status":"Up"},
	            {"name":"Wi-Fi","desc":"Intel(R) Wi-Fi 6E AX211 160MHz","speed":0,"virtual":false,"hw":true,"media":"Native 802.11","status":"Disconnected"},
	            {"name":"Local Area Connection* 1","desc":"Microsoft Wi-Fi Direct Virtual Adapter","speed":0,"virtual":true,"hw":false,"media":"Native 802.11","status":"Disconnected"},
	            {"name":"Hidden Thing","desc":"WAN Miniport (IP)","speed":0,"virtual":true,"hw":false,"media":"Unspecified","status":"Up"}],
	"gw":[{"alias":"Ethernet","hop":"192.168.130.1","metric":0},{"alias":"Wi-Fi","hop":"192.168.131.1","metric":25}],
	"dns":[{"alias":"Ethernet","servers":["192.168.130.1","8.8.8.8"]},{"alias":"Wi-Fi","servers":"192.168.131.1"}],
	"ip":[{"alias":"Ethernet","dhcp":"Enabled"},{"alias":"Wi-Fi","dhcp":"Disabled"},{"alias":"Loopback Pseudo-Interface 1","dhcp":"Disabled"}]}`
	x := parseWindowsNet(out)
	if !x.OK || x.Domain != "WORKGROUP" || x.DomainJoined == nil || *x.DomainJoined || x.Gateway != "192.168.130.1" {
		t.Fatalf("network = %+v", x)
	}
	e := x.Ifaces["Ethernet"]
	if e.Kind != "ethernet" || e.SpeedMbps != 1000 || e.Gateway != "192.168.130.1" || e.DHCP == nil || !*e.DHCP {
		t.Fatalf("ethernet = %+v", e)
	}
	w := x.Ifaces["Wi-Fi"]
	if w.Kind != "wifi" || w.SpeedMbps != 0 || w.DHCP == nil || *w.DHCP || w.Gateway != "192.168.131.1" {
		t.Fatalf("wifi = %+v", w)
	}
	if k := x.Ifaces["Local Area Connection* 1"].Kind; k != "virtual" {
		t.Fatalf("wi-fi direct kind = %q", k)
	}
	if k := x.Ifaces["Hidden Thing"].Kind; k != "virtual" {
		t.Fatalf("wan miniport kind = %q", k)
	}
	if !reflect.DeepEqual(x.DNSByIface["Ethernet"], []string{"192.168.130.1", "8.8.8.8"}) ||
		!reflect.DeepEqual(x.DNSByIface["Wi-Fi"], []string{"192.168.131.1"}) {
		t.Fatalf("dns = %+v", x.DNSByIface)
	}
	if bad := parseWindowsNet("not json"); bad.OK {
		t.Fatal("garbage should not be OK")
	}

	// A domain-joined PC reports its domain, not the workgroup.
	j := parseWindowsNet(`{"domain":"corp.example.com","joined":true,"workgroup":null,"adapters":{"name":"Ethernet","desc":"x","speed":2500000000,"hw":true,"media":"802.3","status":"Up"}}`)
	if j.Domain != "corp.example.com" || !*j.DomainJoined || j.Ifaces["Ethernet"].SpeedMbps != 2500 {
		t.Fatalf("joined = %+v", j)
	}
}

func TestParseNetshWlan(t *testing.T) {
	out := `
There is 1 interface on the system:

    Name                   : Wi-Fi
    Description            : Intel(R) Wi-Fi 6E AX211 160MHz
    GUID                   : 1b2c
    Physical address       : e0:c2:64:dd:53:06
    Interface type         : Primary
    State                  : connected
    SSID                   : V-Office
    AP BSSID               : 70:4c:a5:11:22:33
    Band                   : 5 GHz
    Channel                : 44
    Network type           : Infrastructure
    Radio type             : 802.11ax
    Authentication         : WPA2-Personal
    Receive rate (Mbps)    : 1201
    Transmit rate (Mbps)   : 960
    Signal                 : 87%
    Profile                : V-Office
`
	w := parseNetshWlan(out)
	if w == nil || w.SSID != "V-Office" || w.Band != "5 GHz" || w.Radio != "802.11ax" || w.RxMbps != 1201 || w.SignalPct == nil || *w.SignalPct != 87 {
		t.Fatalf("wlan = %+v", w)
	}
	if parseNetshWlan("    Name : Wi-Fi\n    State : disconnected\n") != nil {
		t.Fatal("disconnected Wi-Fi should be nil")
	}
}

func TestMacParsers(t *testing.T) {
	gw, dev := parseRouteGet(`   route to: default
destination: default
       mask: default
    gateway: 192.168.130.1
  interface: en0
      flags: <UP,GATEWAY,DONE,STATIC,PRCLONING,GLOBAL>`)
	if gw != "192.168.130.1" || dev != "en0" {
		t.Fatalf("route = %q %q", gw, dev)
	}
	dns := parseScutilDNS(`DNS configuration

resolver #1
  search domain[0] : lan
  nameserver[0] : 192.168.130.1
  nameserver[1] : 1.1.1.1
  if_index : 15 (en0)
  reach    : 0x00020002 (Reachable,Directly Reachable Address)

resolver #2
  domain   : local
  nameserver[0] : 224.0.0.251

DNS configuration (for scoped queries)

resolver #1
  nameserver[0] : 192.168.130.1`)
	if !reflect.DeepEqual(dns, []string{"192.168.130.1", "1.1.1.1"}) {
		t.Fatalf("dns = %v", dns)
	}
	ports := parseHardwarePorts(`
Hardware Port: Ethernet
Device: en0
Ethernet Address: 1c:f6:4c:aa:bb:cc

Hardware Port: Wi-Fi
Device: en1
Ethernet Address: 1c:f6:4c:aa:bb:cd

Hardware Port: Thunderbolt Bridge
Device: bridge0
Ethernet Address: 36:aa:bb:cc:dd:00

VLAN Configurations
===================`)
	if ports["en0"].Kind != "ethernet" || ports["en1"].Kind != "wifi" || ports["en1"].Description != "Wi-Fi" || ports["bridge0"].Kind != "virtual" {
		t.Fatalf("ports = %+v", ports)
	}
	speeds := parseIfconfigMedia(`en0: flags=8863<UP,BROADCAST,SMART,RUNNING,SIMPLEX,MULTICAST> mtu 1500
	ether 1c:f6:4c:aa:bb:cc
	media: autoselect (1000baseT <full-duplex>)
	status: active
en1: flags=8863<UP,BROADCAST,SMART,RUNNING,SIMPLEX,MULTICAST> mtu 1500
	media: autoselect
	status: inactive
en5: flags=8863<UP> mtu 1500
	media: autoselect (10Gbase-T <full-duplex>)`)
	if speeds["en0"] != 1000 || speeds["en1"] != 0 || speeds["en5"] != 10000 {
		t.Fatalf("speeds = %v", speeds)
	}
}

func TestLinuxParsers(t *testing.T) {
	best, by := parseIPRouteDefault(`default via 192.168.130.1 dev eno1 proto dhcp src 192.168.130.20 metric 100
default via 10.0.0.1 dev wlan0 proto dhcp metric 600`)
	if best != "192.168.130.1" || by["eno1"] != "192.168.130.1" || by["wlan0"] != "10.0.0.1" {
		t.Fatalf("routes = %q %v", best, by)
	}
	global, links := parseResolvectl(`Global:
Link 2 (eno1): 192.168.130.1 8.8.8.8
Link 3 (docker0):`)
	if len(global) != 0 || !reflect.DeepEqual(links["eno1"], []string{"192.168.130.1", "8.8.8.8"}) || links["docker0"] != nil {
		t.Fatalf("resolvectl = %v %v", global, links)
	}
	if got := parseResolvConf("# generated\nnameserver 127.0.0.53\noptions edns0\nnameserver 192.168.130.1\n"); !reflect.DeepEqual(got, []string{"192.168.130.1"}) {
		t.Fatalf("resolv.conf = %v", got)
	}
	dyn := parseIPAddrDynamic(`2: eno1    inet 192.168.130.20/24 brd 192.168.130.255 scope global dynamic noprefixroute eno1\       valid_lft 80000sec preferred_lft 80000sec
3: docker0    inet 172.17.0.1/16 brd 172.17.255.255 scope global docker0\       valid_lft forever preferred_lft forever`)
	if !dyn["eno1"] || dyn["docker0"] {
		t.Fatalf("dynamic = %v", dyn)
	}
}

func TestNetExtrasApply(t *testing.T) {
	ssid := "V-Office"
	n := protocol.Network{PrimaryIP: "192.168.130.69", WifiSSID: &ssid, Interfaces: []protocol.Interface{
		{Name: "Ethernet", Up: true}, {Name: "Wi-Fi", Up: false}, {Name: "Tailscale", Up: true},
	}}
	x := netExtras{OK: true, Gateway: "10.0.0.1", DNS: []string{"1.1.1.1"},
		DNSByIface: map[string][]string{"Ethernet": {"192.168.130.1"}},
		Ifaces: map[string]ifaceExtra{
			"Ethernet": {Kind: "ethernet", SpeedMbps: 1000, Gateway: "192.168.130.1"},
			"Wi-Fi":    {Kind: "wifi", SpeedMbps: 866},
		},
		Wifi: &protocol.WifiLink{Band: "5 GHz"},
	}
	x.apply(&n, "Ethernet")
	if !n.Details || n.Gateway != "192.168.130.1" || !reflect.DeepEqual(n.DNS, []string{"192.168.130.1"}) {
		t.Fatalf("network = %+v", n)
	}
	if n.Interfaces[0].SpeedMbps != 1000 || n.Interfaces[1].SpeedMbps != 0 || n.Interfaces[2].Kind != "vpn" {
		t.Fatalf("interfaces = %+v", n.Interfaces)
	}
	if n.Wifi == nil || n.Wifi.SSID != "V-Office" || n.Wifi.Band != "5 GHz" {
		t.Fatalf("wifi = %+v", n.Wifi)
	}
	// A platform that collected nothing leaves the report as before.
	m := protocol.Network{Interfaces: []protocol.Interface{{Name: "eth0"}}}
	netExtras{}.apply(&m, "eth0")
	if m.Details || m.Interfaces[0].Kind != "" {
		t.Fatalf("untouched = %+v", m)
	}
}

func TestParseAirport(t *testing.T) {
	// macOS 15+: the name is redacted, the link details aren't.
	out := `{"SPAirPortDataType":[{"spairport_airport_interfaces":[
	  {"_name":"en0","spairport_status_information":"spairport_status_connected",
	   "spairport_current_network_information":{"_name":"<redacted>","spairport_network_channel":"44 (5GHz, 80MHz)",
	     "spairport_network_phymode":"802.11ax","spairport_network_rate":573,"spairport_signal_noise":"-64 dBm / -94 dBm"}},
	  {"_name":"awdl0"}]}]}`
	w := parseAirport([]byte(out))
	if w == nil || w.SSID != "" || w.Band != "5 GHz" || w.Radio != "802.11ax" || w.RxMbps != 573 || w.SignalPct == nil || *w.SignalPct != 72 {
		t.Fatalf("airport = %+v", w)
	}
	// Older macOS still gives the name; 2.4 GHz channels read "(2GHz".
	old := `{"SPAirPortDataType":[{"spairport_airport_interfaces":[{"_name":"en0","spairport_status_information":"spairport_status_connected",
	  "spairport_current_network_information":{"_name":"V-Office","spairport_network_channel":"6 (2GHz, 20MHz)","spairport_signal_noise":"-40 dBm / -90 dBm"}}]}]}`
	if w := parseAirport([]byte(old)); w == nil || w.SSID != "V-Office" || w.Band != "2.4 GHz" || *w.SignalPct != 100 {
		t.Fatalf("old = %+v", w)
	}
	// Wi-Fi switched off (this Mac Mini): nothing.
	if parseAirport([]byte(`{"SPAirPortDataType":[{"spairport_airport_interfaces":[{"_name":"en1","spairport_status_information":"spairport_status_off"}]}]}`)) != nil {
		t.Fatal("off should be nil")
	}
}
