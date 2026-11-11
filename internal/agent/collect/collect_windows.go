package collect

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
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
	add := func(sw protocol.Software) {
		k := strings.ToLower(sw.Name + "\x00" + sw.Version)
		if sw.Name == "" || seen[k] {
			return
		}
		seen[k] = true
		out = append(out, sw)
	}
	type root struct {
		key  registry.Key
		path string
	}
	roots := []root{
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`},
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`},
	}
	// Apps installed for one user only (Spotify, Zoom, Discord...) are listed
	// under that user's own hive, loaded while they're signed in.
	for _, sid := range userSIDs() {
		roots = append(roots, root{registry.USERS, sid + `\Software\Microsoft\Windows\CurrentVersion\Uninstall`})
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
			location, _, _ := sk.GetStringValue("InstallLocation")
			sk.Close()
			if sys == 1 {
				continue
			}
			sw := protocol.Software{Name: name, Version: version, Publisher: publisher, Source: "registry", InstallLocation: location}
			if len(date) == 8 {
				d := date[:4] + "-" + date[4:6] + "-" + date[6:]
				sw.InstallDate = &d
			}
			add(sw)
			for _, o := range officePrograms(sw) {
				add(o)
			}
		}
	}
	out = fillFromAppPaths(out, appPaths())
	for _, sw := range storeApps(ctx) {
		add(sw)
	}
	return out
}

// appPaths lists the programs registered under App Paths, machine-wide and
// for each signed-in user, with the product name from each program's version
// info.
func appPaths() []appPath {
	type root struct {
		key  registry.Key
		path string
	}
	roots := []root{
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths`},
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\App Paths`},
	}
	for _, sid := range userSIDs() {
		roots = append(roots, root{registry.USERS, sid + `\Software\Microsoft\Windows\CurrentVersion\App Paths`})
	}
	var out []appPath
	for _, root := range roots {
		k, err := registry.OpenKey(root.key, root.path, registry.ENUMERATE_SUB_KEYS)
		if err != nil {
			continue
		}
		names, _ := k.ReadSubKeyNames(-1)
		k.Close()
		for _, n := range names {
			if !strings.HasSuffix(strings.ToLower(n), ".exe") {
				continue
			}
			sk, err := registry.OpenKey(root.key, root.path+`\`+n, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			exe, _, err := sk.GetStringValue("")
			sk.Close()
			if err != nil {
				continue
			}
			exe = strings.Trim(exe, `" `)
			if x, err := registry.ExpandString(exe); err == nil {
				exe = x
			}
			if !strings.HasSuffix(strings.ToLower(exe), ".exe") {
				continue
			}
			if fi, err := os.Stat(exe); err != nil || fi.IsDir() {
				continue
			}
			if name := productName(exe); name != "" {
				out = append(out, appPath{Exe: exe, ProductName: name})
			}
		}
	}
	return out
}

// productName reads ProductName from a program's version info, in the first
// language it lists, or "".
func productName(exe string) string {
	size, err := windows.GetFileVersionInfoSize(exe, nil)
	if err != nil || size == 0 {
		return ""
	}
	buf := make([]byte, size)
	block := unsafe.Pointer(&buf[0])
	if err := windows.GetFileVersionInfo(exe, 0, size, block); err != nil {
		return ""
	}
	var tr unsafe.Pointer
	var n uint32
	if err := windows.VerQueryValue(block, `\VarFileInfo\Translation`, unsafe.Pointer(&tr), &n); err != nil || n < 4 {
		return ""
	}
	lang, cp := *(*uint16)(tr), *(*uint16)(unsafe.Add(tr, 2))
	var p unsafe.Pointer
	sub := fmt.Sprintf(`\StringFileInfo\%04x%04x\ProductName`, lang, cp)
	if err := windows.VerQueryValue(block, sub, unsafe.Pointer(&p), &n); err != nil || n == 0 {
		return ""
	}
	return strings.TrimSpace(windows.UTF16PtrToString((*uint16)(p)))
}

var userSID = regexp.MustCompile(`^S-1-5-21-[0-9-]+$`)

// userSIDs lists the loaded hives of real user accounts.
func userSIDs() []string {
	k, err := registry.OpenKey(registry.USERS, "", registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil
	}
	defer k.Close()
	names, _ := k.ReadSubKeyNames(-1)
	var out []string
	for _, n := range names {
		if userSID.MatchString(n) {
			out = append(out, n)
		}
	}
	return out
}

// officePrograms lists Word, Excel and the other Office programs found in an
// Office suite's install folder. Their InstallLocation is the program itself,
// so usage is counted per program rather than for the whole suite.
func officePrograms(suite protocol.Software) []protocol.Software {
	loc := strings.TrimRight(strings.Trim(suite.InstallLocation, `" `), `\/`)
	if loc == "" || !strings.Contains(strings.ToLower(loc), "microsoft office") {
		return nil
	}
	var out []protocol.Software
	for _, dir := range officeDirs {
		for _, a := range officeApps {
			exe := filepath.Join(loc, dir, a.exe)
			if _, err := os.Stat(exe); err == nil {
				out = append(out, protocol.Software{Name: a.name, Version: suite.Version, Publisher: suite.Publisher,
					Source: "registry", InstallDate: suite.InstallDate, InstallLocation: exe})
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return out
}

// storeApps lists Microsoft Store (MSIX/AppX) apps installed for any user,
// such as the new Teams, WhatsApp and Spotify. Parts of Windows itself
// (system-signed or non-removable packages) and frameworks are left out.
func storeApps(ctx context.Context) []protocol.Software {
	out, err := powershell(ctx, 90*time.Second,
		`$p = @(Get-AppxPackage -AllUsers | Where-Object { -not $_.IsFramework -and -not $_.IsResourcePackage -and -not $_.NonRemovable -and `+
			`$_.SignatureKind -ne 'System' -and $_.InstallLocation } | Select-Object Name, PackageFullName, Version, InstallLocation); `+
			`ConvertTo-Json -Compress -InputObject $p`)
	if err != nil {
		return nil
	}
	var pkgs []struct{ Name, PackageFullName, Version, InstallLocation string }
	if json.Unmarshal([]byte(strings.TrimSpace(out)), &pkgs) != nil {
		return nil
	}
	var list []protocol.Software
	done := map[string]bool{}
	for _, p := range pkgs {
		if done[p.PackageFullName] {
			continue
		}
		done[p.PackageFullName] = true
		data, err := os.ReadFile(filepath.Join(p.InstallLocation, "AppxManifest.xml"))
		if err != nil {
			continue
		}
		app, ok := parseAppxManifest(data)
		if !ok {
			continue
		}
		name := app.Name
		if isResourceRef(name) {
			name = resolveResource(p.PackageFullName, p.Name, name)
			if name == "" {
				name = app.Fallback
			}
		}
		list = append(list, protocol.Software{Name: name, Version: p.Version, Publisher: app.Publisher,
			Source: "msstore", InstallLocation: p.InstallLocation})
	}
	return list
}

var procSHLoadIndirectString = windows.NewLazySystemDLL("shlwapi.dll").NewProc("SHLoadIndirectString")

// resolveResource looks up a manifest "ms-resource:" display name in the
// package's own resources; "" if Windows can't resolve it.
func resolveResource(fullName, packageName, ref string) string {
	if procSHLoadIndirectString.Find() != nil {
		return ""
	}
	for _, uri := range resourceURIs(ref, packageName) {
		src, err := windows.UTF16PtrFromString("@{" + fullName + "?" + uri + "}")
		if err != nil {
			continue
		}
		buf := make([]uint16, 512)
		hr, _, _ := procSHLoadIndirectString.Call(uintptr(unsafe.Pointer(src)), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0)
		if hr == 0 {
			if s := strings.TrimSpace(windows.UTF16ToString(buf)); s != "" && !isResourceRef(s) {
				return s
			}
		}
	}
	return ""
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
