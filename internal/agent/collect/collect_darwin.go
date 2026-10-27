package collect

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/uem/uem/internal/protocol"
)

func platformHardware(ctx context.Context) protocol.Hardware {
	hw := protocol.Hardware{Manufacturer: "Apple"}
	out, err := run(ctx, 20*time.Second, "system_profiler", "SPHardwareDataType", "-json")
	if err != nil {
		return hw
	}
	var data struct {
		Items []struct {
			Serial     string `json:"serial_number"`
			ModelName  string `json:"machine_name"`
			ModelID    string `json:"machine_model"`
		} `json:"SPHardwareDataType"`
	}
	if json.Unmarshal([]byte(out), &data) == nil && len(data.Items) > 0 {
		it := data.Items[0]
		hw.Serial = it.Serial
		hw.Model = strings.TrimSpace(it.ModelName + " (" + it.ModelID + ")")
	}
	return hw
}

func osBuild(ctx context.Context, info protocol.OSInfo) string {
	out, _ := run(ctx, 5*time.Second, "sw_vers", "-buildVersion")
	return out
}

func software(ctx context.Context) []protocol.Software {
	out := []protocol.Software{}
	raw, err := run(ctx, 120*time.Second, "system_profiler", "SPApplicationsDataType", "-json", "-detailLevel", "mini")
	if err != nil {
		return out
	}
	var data struct {
		Apps []struct {
			Name         string `json:"_name"`
			Version      string `json:"version"`
			ObtainedFrom string `json:"obtained_from"`
			SignedBy     []string `json:"signed_by"`
			LastModified string `json:"lastModified"`
			Path         string `json:"path"`
		} `json:"SPApplicationsDataType"`
	}
	if json.Unmarshal([]byte(raw), &data) != nil {
		return out
	}
	for _, a := range data.Apps {
		// Skip helper apps buried inside other bundles and system frameworks.
		if strings.Contains(a.Path, ".app/") || strings.HasPrefix(a.Path, "/System/Library/") || strings.HasPrefix(a.Path, "/Library/Apple/") {
			continue
		}
		pub := ""
		if len(a.SignedBy) > 0 {
			pub = strings.TrimPrefix(a.SignedBy[0], "Developer ID Application: ")
		}
		sw := protocol.Software{Name: a.Name, Version: a.Version, Publisher: pub, Source: a.ObtainedFrom}
		if a.LastModified != "" {
			d := a.LastModified
			sw.InstallDate = &d
		}
		out = append(out, sw)
	}
	return out
}

func security(ctx context.Context) protocol.Security {
	var s protocol.Security
	if out, err := run(ctx, 10*time.Second, "fdesetup", "status"); err == nil {
		on := strings.Contains(out, "FileVault is On")
		s.DiskEncrypted = &on
		if on {
			s.EncryptionMethod = ptr("FileVault")
		}
	}
	if out, err := run(ctx, 5*time.Second, "/usr/libexec/ApplicationFirewall/socketfilterfw", "--getglobalstate"); err == nil {
		s.FirewallEnabled = ptr(strings.Contains(strings.ToLower(out), "enabled"))
	}
	// Gatekeeper/XProtect are always present; report XProtect as the built-in AV.
	s.Antivirus = ptr("XProtect")
	return s
}

func wifiSSID(ctx context.Context) string {
	out, err := run(ctx, 5*time.Second, "ipconfig", "getsummary", "en0")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "SSID : "); ok && v != "<redacted>" {
			return v
		}
	}
	return ""
}

func consoleUser(ctx context.Context) string {
	out, err := run(ctx, 5*time.Second, "stat", "-f", "%Su", "/dev/console")
	if err != nil || out == "root" {
		return ""
	}
	return out
}
