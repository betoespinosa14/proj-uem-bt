package collect

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows/registry"

	"github.com/uem/uem/internal/protocol"
)

// powershell runs a script with a JSON-producing pipeline.
func powershell(ctx context.Context, timeout time.Duration, script string) (string, error) {
	return run(ctx, timeout, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
}

func platformHardware(ctx context.Context) protocol.Hardware {
	var hw protocol.Hardware
	out, err := powershell(ctx, 30*time.Second,
		`$b=Get-CimInstance Win32_BIOS; $c=Get-CimInstance Win32_ComputerSystem; `+
			`@{serial=$b.SerialNumber; manufacturer=$c.Manufacturer; model=$c.Model} | ConvertTo-Json -Compress`)
	if err != nil {
		return hw
	}
	var v struct{ Serial, Manufacturer, Model string }
	if json.Unmarshal([]byte(out), &v) == nil {
		hw.Serial, hw.Manufacturer, hw.Model = strings.TrimSpace(v.Serial), strings.TrimSpace(v.Manufacturer), strings.TrimSpace(v.Model)
	}
	return hw
}

func osBuild(ctx context.Context, info protocol.OSInfo) string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return info.Kernel
	}
	defer k.Close()
	build, _, _ := k.GetStringValue("CurrentBuildNumber")
	ubr, _, err := k.GetIntegerValue("UBR")
	if build != "" && err == nil {
		return build + "." + strconv.FormatUint(ubr, 10)
	}
	if build != "" {
		return build
	}
	return info.Kernel
}

func software(ctx context.Context) []protocol.Software {
	out := []protocol.Software{}
	seen := map[string]bool{}
	roots := []struct {
		key  registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`},
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`},
	}
	for _, root := range roots {
		k, err := registry.OpenKey(root.key, root.path, registry.ENUMERATE_SUB_KEYS)
		if err != nil {
			continue
		}
		names, _ := k.ReadSubKeyNames(-1)
		k.Close()
		for _, n := range names {
			sk, err := registry.OpenKey(root.key, root.path+`\`+n, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			name, _, _ := sk.GetStringValue("DisplayName")
			sys, _, _ := sk.GetIntegerValue("SystemComponent")
			version, _, _ := sk.GetStringValue("DisplayVersion")
			publisher, _, _ := sk.GetStringValue("Publisher")
			date, _, _ := sk.GetStringValue("InstallDate")
			sk.Close()
			if name == "" || sys == 1 || seen[name+version] {
				continue
			}
			seen[name+version] = true
			sw := protocol.Software{Name: name, Version: version, Publisher: publisher, Source: "registry"}
			if len(date) == 8 {
				d := date[:4] + "-" + date[4:6] + "-" + date[6:]
				sw.InstallDate = &d
			}
			out = append(out, sw)
		}
	}
	return out
}

var protectionOn = regexp.MustCompile(`(?i)Protection Status:\s+Protection On`)

func security(ctx context.Context) protocol.Security {
	var s protocol.Security
	drive := systemMount()[:2]
	if out, err := run(ctx, 20*time.Second, "manage-bde.exe", "-status", drive); err == nil || out != "" {
		on := protectionOn.MatchString(out)
		s.DiskEncrypted = &on
		if on {
			s.EncryptionMethod = ptr("BitLocker")
		}
	}
	if out, err := powershell(ctx, 20*time.Second,
		`@(Get-NetFirewallProfile | Where-Object { -not $_.Enabled }).Count`); err == nil {
		s.FirewallEnabled = ptr(strings.TrimSpace(out) == "0")
	}
	if out, err := powershell(ctx, 20*time.Second,
		`(Get-CimInstance -Namespace root/SecurityCenter2 -ClassName AntiVirusProduct | Select-Object -First 1).displayName`); err == nil && out != "" {
		s.Antivirus = ptr(out)
	}
	if v := screenLockPolicy(); v != nil {
		s.ScreenLockEnabled = v
	}
	if out, err := powershell(ctx, 180*time.Second,
		`(New-Object -ComObject Microsoft.Update.Session).CreateUpdateSearcher().Search("IsInstalled=0 and IsHidden=0 and Type='Software'").Updates.Count`); err == nil {
		var n int
		if json.Unmarshal([]byte(strings.TrimSpace(out)), &n) == nil {
			s.PendingUpdates = &n
		}
	}
	return s
}

// screenLockPolicy reads the machine inactivity-lock policy, if configured.
func screenLockPolicy() *bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System`, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("InactivityTimeoutSecs")
	if err != nil {
		return nil
	}
	return ptr(v > 0)
}

func wifiSSID(ctx context.Context) string {
	out, err := run(ctx, 5*time.Second, "netsh", "wlan", "show", "interfaces")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(out, "\n") {
		f := strings.SplitN(line, ":", 2)
		if len(f) == 2 && strings.TrimSpace(f[0]) == "SSID" {
			return strings.TrimSpace(f[1])
		}
	}
	return ""
}

func consoleUser(ctx context.Context) string {
	out, err := powershell(ctx, 15*time.Second, `(Get-CimInstance Win32_ComputerSystem).UserName`)
	if err != nil {
		return ""
	}
	return out
}
