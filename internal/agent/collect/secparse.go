package collect

// Parsers for the security details each platform reports (antivirus health,
// firewall profiles, OS integrity, whether the signed-in user is an admin,
// pending updates on macOS). Like hwparse.go they take raw command output so
// they can be tested on any OS. Everything is best effort.

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/uem/uem/internal/protocol"
)

// ---- Windows ----

// windowsSecScript prints one JSON object with everything parseWindowsSec reads.
// It runs as SYSTEM; each part may fail on its own.
const windowsSecScript = `$ErrorActionPreference='SilentlyContinue'
$adm=(New-Object System.Security.Principal.SecurityIdentifier('S-1-5-32-544')).Translate([System.Security.Principal.NTAccount]).Value.Split('\')[-1]
$m=@(); $raw=net localgroup "$adm" 2>$null; $in=$false
foreach($l in $raw){ if($l -match '^-{5,}'){ $in=$true; continue }; if($in -and $l.Trim() -ne '' -and $l -notmatch 'command completed'){ $m+=$l.Trim() } }
$mp=Get-MpComputerStatus
@{
 fw=@(Get-NetFirewallProfile | ForEach-Object { @{name=[string]$_.Name; enabled=[bool]$_.Enabled} })
 av=@(Get-CimInstance -Namespace root/SecurityCenter2 -ClassName AntiVirusProduct | ForEach-Object { @{name=[string]$_.displayName; state=[int64]$_.productState} })
 mp=$(if($mp){@{rt=[bool]$mp.RealTimeProtectionEnabled; age=[int]$mp.AntivirusSignatureAge; tamper=$mp.IsTamperProtected; mode=[string]$mp.AMRunningMode; ver=[string]$mp.AntivirusSignatureVersion}}else{$null})
 uac=(Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System').EnableLUA
 user=[string](Get-CimInstance Win32_ComputerSystem).UserName
 computer=$env:COMPUTERNAME
 admins=$m
} | ConvertTo-Json -Compress -Depth 4`

// secExtras is what a platform adds to protocol.Security beyond the basics.
type secExtras struct {
	OK                bool
	FirewallProfiles  []protocol.SecuritySetting
	AV                *protocol.AVStatus
	OSIntegrity       []protocol.SecuritySetting
	User              string
	UserAdmin         *bool
	PendingUpdates    *int
	ScreenLockMinutes *int
	EncryptionVolume  string
}

func (x secExtras) apply(s *protocol.Security) {
	if !x.OK {
		return
	}
	s.Details = true
	s.FirewallProfiles, s.OSIntegrity = x.FirewallProfiles, x.OSIntegrity
	s.User, s.UserAdmin = x.User, x.UserAdmin
	s.ScreenLockMinutes, s.EncryptionVolume = x.ScreenLockMinutes, x.EncryptionVolume
	if x.AV != nil {
		s.AV = x.AV
		if x.AV.Name != "" {
			s.Antivirus = ptr(x.AV.Name)
		}
	}
	if x.PendingUpdates != nil && s.PendingUpdates == nil {
		s.PendingUpdates = x.PendingUpdates
	}
	// The overall firewall flag follows the profiles: on only if every one is on.
	if len(x.FirewallProfiles) > 0 {
		all, known := true, false
		for _, p := range x.FirewallProfiles {
			if p.Enabled != nil {
				known = true
				all = all && *p.Enabled
			}
		}
		if known && s.FirewallEnabled == nil {
			s.FirewallEnabled = ptr(all)
		}
	}
}

// avProductState decodes SecurityCenter2's productState: the middle byte says
// whether real-time protection is on (0x10/0x11), the low byte whether the
// definitions are up to date (0x00) or out of date (0x10).
func avProductState(state int64) (realtime, upToDate bool) {
	mid := (state >> 8) & 0xFF
	low := state & 0xFF
	return mid == 0x10 || mid == 0x11, low == 0x00
}

func parseWindowsSec(out string) secExtras {
	var v struct {
		FW json.RawMessage `json:"fw"`
		AV json.RawMessage `json:"av"`
		MP *struct {
			RT     bool   `json:"rt"`
			Age    int    `json:"age"`
			Tamper *bool  `json:"tamper"`
			Mode   string `json:"mode"`
			Ver    string `json:"ver"`
		} `json:"mp"`
		UAC      *int            `json:"uac"`
		User     string          `json:"user"`
		Computer string          `json:"computer"`
		Admins   json.RawMessage `json:"admins"`
	}
	x := secExtras{}
	if json.Unmarshal([]byte(strings.TrimSpace(out)), &v) != nil {
		return x
	}
	x.OK = true
	for _, p := range oneOrMany[struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}](v.FW) {
		if p.Name != "" {
			x.FirewallProfiles = append(x.FirewallProfiles, protocol.SecuritySetting{Name: p.Name, Enabled: ptr(p.Enabled)})
		}
	}

	// Several products can be registered (e.g. McAfee active, Defender passive):
	// report the one with real-time protection on, else the first.
	type product struct {
		Name  string `json:"name"`
		State int64  `json:"state"`
	}
	var chosen *product
	for _, p := range oneOrMany[product](v.AV) {
		p := p
		if strings.TrimSpace(p.Name) == "" {
			continue
		}
		if rt, _ := avProductState(p.State); rt || chosen == nil {
			if chosen == nil || rt {
				chosen = &p
			}
			if rt {
				break
			}
		}
	}
	if chosen != nil {
		rt, ok := avProductState(chosen.State)
		x.AV = &protocol.AVStatus{Name: strings.TrimSpace(chosen.Name), Realtime: ptr(rt), UpToDate: ptr(ok)}
		if strings.Contains(strings.ToLower(chosen.Name), "defender") && v.MP != nil {
			x.AV.Realtime = ptr(v.MP.RT)
			x.AV.SignatureAgeDays = ptr(v.MP.Age)
			x.AV.TamperProtected = v.MP.Tamper
			x.AV.Version = v.MP.Ver
		}
	}

	if v.UAC != nil {
		x.OSIntegrity = append(x.OSIntegrity, protocol.SecuritySetting{Name: "User Account Control", Enabled: ptr(*v.UAC != 0)})
	}
	x.User = strings.TrimSpace(v.User)
	if x.User != "" {
		x.UserAdmin = windowsAdmin(x.User, v.Computer, oneOrMany[string](v.Admins))
	}
	return x
}

// windowsAdmin reports whether user ("DOMAIN\name") is a direct member of the
// local Administrators group, as "net localgroup" lists it (local accounts
// without the computer name, others as DOMAIN\name). nil when the group also
// holds domain groups the user might belong to, which can't be resolved here.
func windowsAdmin(user, computer string, members []string) *bool {
	dom, name, ok := strings.Cut(user, `\`)
	if !ok {
		name, dom = user, ""
	}
	local := strings.EqualFold(dom, computer) || dom == ""
	viaGroup := false
	for _, m := range members {
		m = strings.TrimSpace(m)
		if strings.EqualFold(m, user) || (local && strings.EqualFold(m, name)) {
			return ptr(true)
		}
		if md, mn, ok := strings.Cut(m, `\`); ok && !strings.EqualFold(md, computer) && !strings.EqualFold(md, "NT AUTHORITY") {
			if strings.Contains(strings.ToLower(mn), "admins") || strings.Contains(strings.ToLower(mn), "administrators") {
				viaGroup = true
			}
		}
	}
	if viaGroup {
		return nil
	}
	return ptr(false)
}

// ---- macOS ----

// parseCsrutil reads "csrutil status": System Integrity Protection.
func parseCsrutil(out string) *bool {
	o := strings.ToLower(out)
	switch {
	case strings.Contains(o, "status: enabled"):
		return ptr(true)
	case strings.Contains(o, "status: disabled"):
		return ptr(false)
	}
	return nil
}

// parseSpctl reads "spctl --status": Gatekeeper.
func parseSpctl(out string) *bool {
	o := strings.ToLower(strings.TrimSpace(out))
	switch {
	case strings.Contains(o, "assessments enabled"):
		return ptr(true)
	case strings.Contains(o, "assessments disabled"):
		return ptr(false)
	}
	return nil
}

// parseSocketfilterOn reads socketfilterfw's "--getglobalstate" or
// "--getstealthmode": "is enabled"/"is on" vs "is disabled"/"is off".
func parseSocketfilterOn(out string) *bool {
	o := strings.ToLower(out)
	switch {
	case strings.Contains(o, "is enabled") || strings.Contains(o, "is on") || strings.Contains(o, "state = 1") || strings.Contains(o, "state = 2"):
		return ptr(true)
	case strings.Contains(o, "is disabled") || strings.Contains(o, "is off") || strings.Contains(o, "state = 0"):
		return ptr(false)
	}
	return nil
}

// parseDseditgroup reads "dseditgroup -o checkmember -m <user> admin".
func parseDseditgroup(out string) *bool {
	o := strings.ToLower(strings.TrimSpace(out))
	switch {
	case strings.HasPrefix(o, "yes"):
		return ptr(true)
	case strings.HasPrefix(o, "no"):
		return ptr(false)
	}
	return nil
}

// parseSoftwareUpdate counts the updates "softwareupdate -l" lists.
func parseSoftwareUpdate(out string) *int {
	if strings.Contains(out, "No new software available") {
		return ptr(0)
	}
	n, seen := 0, false
	for _, line := range strings.Split(out, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "* Label:") {
			n++
		}
		if strings.Contains(t, "found the following") {
			seen = true
		}
	}
	if n == 0 && !seen {
		return nil // an error or no network: unknown
	}
	return &n
}

// ---- Linux ----

// parseGetenforce reads "getenforce": SELinux.
func parseGetenforce(out string) *protocol.SecuritySetting {
	switch strings.TrimSpace(out) {
	case "Enforcing":
		return &protocol.SecuritySetting{Name: "SELinux", Enabled: ptr(true), Detail: "Enforcing"}
	case "Permissive":
		return &protocol.SecuritySetting{Name: "SELinux", Enabled: ptr(false), Detail: "Permissive (logs only)"}
	case "Disabled":
		return &protocol.SecuritySetting{Name: "SELinux", Enabled: ptr(false), Detail: "Disabled"}
	}
	return nil
}

var adminGroups = regexp.MustCompile(`^(sudo|wheel|admin)$`)

// parseIDGroups reads "id -nG <user>": admin when in sudo, wheel or admin.
func parseIDGroups(out string) *bool {
	f := strings.Fields(out)
	if len(f) == 0 {
		return nil
	}
	for _, g := range f {
		if adminGroups.MatchString(g) {
			return ptr(true)
		}
	}
	return ptr(false)
}

// screenLockMinutes turns InactivityTimeoutSecs into whole minutes (at least 1).
func screenLockMinutes(secs uint64) *int {
	if secs == 0 {
		return nil
	}
	m := int((secs + 59) / 60)
	return &m
}

