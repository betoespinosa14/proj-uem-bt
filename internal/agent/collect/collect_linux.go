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

func readSys(name string) string {
	b, err := os.ReadFile("/sys/class/dmi/id/" + name)
	if err != nil {
		return ""
	}
	v := strings.TrimSpace(string(b))
	switch strings.ToLower(v) {
	case "", "to be filled by o.e.m.", "default string", "system serial number", "not specified", "none":
		return ""
	}
	return v
}

func platformHardware(ctx context.Context) protocol.Hardware {
	return protocol.Hardware{
		Manufacturer: readSys("sys_vendor"),
		Model:        strings.TrimSpace(readSys("product_name") + " " + readSys("product_version")),
		Serial:       readSys("product_serial"), // readable by root only
	}
}

func osBuild(ctx context.Context, info protocol.OSInfo) string { return info.Kernel }

func software(ctx context.Context) []protocol.Software {
	out := []protocol.Software{}
	if _, err := exec.LookPath("dpkg-query"); err == nil {
		s, err := run(ctx, 60*time.Second, "dpkg-query", "-W", "-f", `${db:Status-Abbrev}\t${Package}\t${Version}\t${Maintainer}\n`)
		if err == nil {
			for _, line := range strings.Split(s, "\n") {
				f := strings.SplitN(line, "\t", 4)
				if len(f) == 4 && strings.HasPrefix(f[0], "ii") {
					out = append(out, protocol.Software{Name: f[1], Version: f[2], Publisher: f[3], Source: "dpkg"})
				}
			}
			return out
		}
	}
	if _, err := exec.LookPath("rpm"); err == nil {
		s, err := run(ctx, 60*time.Second, "rpm", "-qa", "--qf", `%{NAME}\t%{VERSION}-%{RELEASE}\t%{VENDOR}\n`)
		if err == nil {
			for _, line := range strings.Split(s, "\n") {
				f := strings.SplitN(line, "\t", 3)
				if len(f) == 3 && f[0] != "gpg-pubkey" {
					out = append(out, protocol.Software{Name: f[0], Version: f[1], Publisher: strings.TrimSpace(strings.ReplaceAll(f[2], "(none)", "")), Source: "rpm"})
				}
			}
		}
	}
	if _, err := exec.LookPath("snap"); err == nil {
		if s, err := run(ctx, 20*time.Second, "snap", "list"); err == nil {
			for i, line := range strings.Split(s, "\n") {
				f := strings.Fields(line)
				if i > 0 && len(f) >= 5 {
					out = append(out, protocol.Software{Name: f[0], Version: f[1], Publisher: f[4], Source: "snap"})
				}
			}
		}
	}
	return out
}

func security(ctx context.Context) protocol.Security {
	var s protocol.Security
	s.DiskEncrypted, s.EncryptionMethod = rootEncryption(ctx)
	s.FirewallEnabled = firewall(ctx)
	s.PendingUpdates = pendingUpdates(ctx)
	return s
}

// rootEncryption reports whether the root filesystem sits on a dm-crypt (LUKS) device.
func rootEncryption(ctx context.Context) (*bool, *string) {
	src, err := run(ctx, 5*time.Second, "findmnt", "-no", "SOURCE", "/")
	if err != nil || !strings.HasPrefix(src, "/dev/") {
		return nil, nil // overlay/container root: cannot determine
	}
	src = strings.SplitN(src, "[", 2)[0] // btrfs subvolume suffix
	types, err := run(ctx, 5*time.Second, "lsblk", "-s", "-n", "-o", "TYPE", src)
	if err != nil {
		return nil, nil
	}
	for _, t := range strings.Fields(types) {
		if t == "crypt" {
			return ptr(true), ptr("LUKS")
		}
	}
	return ptr(false), nil
}

func firewall(ctx context.Context) *bool {
	if _, err := exec.LookPath("ufw"); err == nil {
		if out, err := run(ctx, 5*time.Second, "ufw", "status"); err == nil {
			return ptr(strings.Contains(out, "Status: active"))
		}
	}
	if _, err := exec.LookPath("firewall-cmd"); err == nil {
		out, _ := run(ctx, 5*time.Second, "firewall-cmd", "--state")
		return ptr(out == "running")
	}
	if _, err := exec.LookPath("nft"); err == nil {
		if out, err := run(ctx, 5*time.Second, "nft", "list", "ruleset"); err == nil {
			return ptr(strings.Contains(out, "hook input") && strings.Contains(out, "policy drop"))
		}
	}
	return nil
}

func pendingUpdates(ctx context.Context) *int {
	if _, err := exec.LookPath("apt-get"); err == nil {
		out, err := run(ctx, 60*time.Second, "apt-get", "-s", "-o", "Debug::NoLocking=1", "upgrade")
		if err != nil {
			return nil
		}
		n := 0
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, "Inst ") {
				n++
			}
		}
		return &n
	}
	if _, err := exec.LookPath("dnf"); err == nil {
		// Exit code 100 means updates are available; output lists one per line.
		out, _ := run(ctx, 120*time.Second, "dnf", "-q", "--cacheonly", "check-update")
		n := 0
		for _, line := range strings.Split(out, "\n") {
			if f := strings.Fields(line); len(f) == 3 && strings.Contains(f[0], ".") {
				n++
			}
		}
		return &n
	}
	return nil
}

func wifiSSID(ctx context.Context) string {
	if _, err := exec.LookPath("nmcli"); err == nil {
		if out, err := run(ctx, 5*time.Second, "nmcli", "-t", "-f", "active,ssid", "dev", "wifi"); err == nil {
			for _, line := range strings.Split(out, "\n") {
				if ssid, ok := strings.CutPrefix(line, "yes:"); ok {
					return ssid
				}
			}
		}
	}
	if out, err := run(ctx, 5*time.Second, "iwgetid", "-r"); err == nil {
		return out
	}
	return ""
}

func consoleUser(ctx context.Context) string {
	out, err := run(ctx, 5*time.Second, "loginctl", "list-sessions", "--no-legend")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) >= 3 {
			if uid, err := strconv.Atoi(f[1]); err == nil && uid >= 1000 {
				return f[2]
			}
		}
	}
	return ""
}
