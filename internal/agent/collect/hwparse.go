package collect

// Parsers for the hardware details each platform reports (firmware, security
// chip, graphics, memory layout, physical drives). They take the raw command
// output so they can be tested on any OS; the platform files only run the
// commands. Everything is best effort: what can't be read stays empty.

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/uem/uem/internal/protocol"
)

// hwExtras is what a platform adds to protocol.Hardware beyond the basics.
type hwExtras struct {
	Firmware   *protocol.Firmware
	GPUs       []protocol.GPU
	Memory     *protocol.MemoryLayout
	Drives     []protocol.PhysicalDrive
	CycleCount *int
}

func (x hwExtras) apply(hw *protocol.Hardware) {
	hw.Firmware, hw.GPUs, hw.Memory, hw.Drives = x.Firmware, x.GPUs, x.Memory, x.Drives
	if x.CycleCount != nil && *x.CycleCount > 0 && hw.Battery.Present && hw.Battery.CycleCount == nil {
		hw.Battery.CycleCount = x.CycleCount
	}
}

// normHealth maps the many ways platforms say a drive is fine or failing.
func normHealth(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "healthy", "ok", "verified", "passed":
		return "healthy"
	case "warning":
		return "warning"
	case "unhealthy", "failing", "failed", "fail":
		return "failing"
	}
	return ""
}

// smbiosMemoryType maps Win32_PhysicalMemory.SMBIOSMemoryType to a name.
var smbiosMemoryType = map[int]string{
	20: "DDR", 21: "DDR2", 24: "DDR3", 26: "DDR4", 27: "LPDDR", 28: "LPDDR2", 29: "LPDDR3",
	30: "LPDDR4", 34: "DDR5", 35: "LPDDR5",
}

// layout fills the summary fields from the modules. LPDDR memory is always
// soldered to the board.
func layout(m *protocol.MemoryLayout) *protocol.MemoryLayout {
	if m == nil || (len(m.Modules) == 0 && m.Type == "") {
		return nil
	}
	m.SlotsUsed = len(m.Modules)
	if m.SlotsTotal < m.SlotsUsed {
		m.SlotsTotal = m.SlotsUsed
	}
	for _, mod := range m.Modules {
		if m.Type == "" {
			m.Type = mod.Type
		}
		if m.SpeedMTs == 0 {
			m.SpeedMTs = mod.SpeedMTs
		}
	}
	if strings.HasPrefix(m.Type, "LPDDR") {
		m.Soldered = true
	}
	return m
}

// oneOrMany decodes a JSON value that PowerShell may emit as a single object
// or as an array of them.
func oneOrMany[T any](raw json.RawMessage) []T {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var many []T
	if raw[0] == '[' {
		_ = json.Unmarshal(raw, &many)
		return many
	}
	var one T
	if json.Unmarshal(raw, &one) == nil {
		return []T{one}
	}
	return nil
}

// ---- Windows ----

// virtualDisplay matches the software display adapters that remote-desktop and
// screen-sharing tools install (Parsec, RDP, Citrix, Splashtop, spacedesk…).
// They aren't graphics hardware. "Microsoft Basic Display Adapter" is kept: it's
// a real GPU running without its driver, which is worth seeing.
var virtualDisplay = regexp.MustCompile(`(?i)virtual display|remote display|indirect display|parsec|splashtop|spacedesk|teamviewer|citrix|\bIddSample|meta virtual monitor`)

// encodePS encodes a multi-line script for -EncodedCommand (UTF-16LE, base64),
// so it reaches PowerShell intact whatever its quotes and line breaks.
func encodePS(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, 2*len(u))
	for i, r := range u {
		b[2*i], b[2*i+1] = byte(r), byte(r>>8)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// windowsHWScript prints one JSON object with everything parseWindowsHW reads.
// It runs as SYSTEM; each part is allowed to fail on its own.
const windowsHWScript = `$ErrorActionPreference='SilentlyContinue'
$b=Get-CimInstance Win32_BIOS
$sb=$null; try { $sb=[bool](Confirm-SecureBootUEFI) } catch {}
$t=Get-CimInstance -Namespace root/cimv2/Security/MicrosoftTpm -ClassName Win32_Tpm | Select-Object -First 1
$cc=Get-CimInstance -Namespace root/wmi -ClassName BatteryCycleCount | Select-Object -First 1
@{
 bios=@{vendor=[string]$b.Manufacturer; version=[string]$b.SMBIOSBIOSVersion; date=$(if($b.ReleaseDate){$b.ReleaseDate.ToString('yyyy-MM-dd')}else{''})}
 secureBoot=$sb
 tpm=$(if($t){@{spec=[string]$t.SpecVersion; enabled=$t.IsEnabled_InitialValue}}else{$null})
 gpus=@(Get-CimInstance Win32_VideoController | ForEach-Object { @{name=[string]$_.Name; driver=[string]$_.DriverVersion} })
 mem=@(Get-CimInstance Win32_PhysicalMemory | ForEach-Object { @{slot=[string]$_.DeviceLocator; bytes=[uint64]$_.Capacity; type=[int]$_.SMBIOSMemoryType; speed=[int]$_.ConfiguredClockSpeed; form=[int]$_.FormFactor} })
 slots=[int](Get-CimInstance Win32_PhysicalMemoryArray | Measure-Object -Property MemoryDevices -Sum).Sum
 disks=@(Get-PhysicalDisk | ForEach-Object { @{model=[string]$_.FriendlyName; media=[string]$_.MediaType; bus=[string]$_.BusType; size=[uint64]$_.Size; health=[string]$_.HealthStatus} })
 cycles=$(if($cc){[int]$cc.CycleCount}else{$null})
} | ConvertTo-Json -Compress -Depth 4`

func parseWindowsHW(out string) hwExtras {
	var v struct {
		BIOS       struct{ Vendor, Version, Date string } `json:"bios"`
		SecureBoot *bool                                  `json:"secureBoot"`
		TPM        *struct {
			Spec    string `json:"spec"`
			Enabled *bool  `json:"enabled"`
		} `json:"tpm"`
		GPUs   json.RawMessage `json:"gpus"`
		Mem    json.RawMessage `json:"mem"`
		Slots  int             `json:"slots"`
		Disks  json.RawMessage `json:"disks"`
		Cycles *int            `json:"cycles"`
	}
	var x hwExtras
	if json.Unmarshal([]byte(strings.TrimSpace(out)), &v) != nil {
		return x
	}
	fw := &protocol.Firmware{Vendor: strings.TrimSpace(v.BIOS.Vendor), Version: strings.TrimSpace(v.BIOS.Version),
		ReleaseDate: v.BIOS.Date, SecureBoot: v.SecureBoot}
	if v.TPM != nil {
		// SpecVersion is like "2.0, 0, 1.38": the first part is the TPM version.
		ver := strings.TrimSpace(strings.Split(v.TPM.Spec, ",")[0])
		fw.SecurityChip = strings.TrimSpace("TPM " + ver)
		fw.TPMEnabled = v.TPM.Enabled
	}
	x.Firmware = fw

	for _, g := range oneOrMany[struct{ Name, Driver string }](v.GPUs) {
		if name := strings.TrimSpace(g.Name); name != "" && !virtualDisplay.MatchString(name) {
			x.GPUs = append(x.GPUs, protocol.GPU{Name: name, DriverVersion: strings.TrimSpace(g.Driver)})
		}
	}

	mem := &protocol.MemoryLayout{SlotsTotal: v.Slots}
	soldered := false
	for _, m := range oneOrMany[struct {
		Slot  string `json:"slot"`
		Bytes uint64 `json:"bytes"`
		Type  int    `json:"type"`
		Speed int    `json:"speed"`
		Form  int    `json:"form"`
	}](v.Mem) {
		if m.Bytes == 0 {
			continue
		}
		mem.Modules = append(mem.Modules, protocol.MemoryModule{Slot: strings.TrimSpace(m.Slot), CapacityBytes: m.Bytes,
			Type: smbiosMemoryType[m.Type], SpeedMTs: m.Speed})
		soldered = soldered || m.Form == 7 // FormFactor 7: "Row of chips", on the board
	}
	if x.Memory = layout(mem); x.Memory != nil && soldered {
		x.Memory.Soldered = true
	}

	for _, d := range oneOrMany[struct {
		Model  string `json:"model"`
		Media  string `json:"media"`
		Bus    string `json:"bus"`
		Size   uint64 `json:"size"`
		Health string `json:"health"`
	}](v.Disks) {
		if strings.TrimSpace(d.Model) == "" {
			continue
		}
		media := d.Media
		if media != "SSD" && media != "HDD" {
			media = ""
		}
		x.Drives = append(x.Drives, protocol.PhysicalDrive{Model: strings.TrimSpace(d.Model), Media: media,
			Bus: busName(d.Bus), SizeBytes: d.Size, Health: normHealth(d.Health)})
	}
	x.CycleCount = v.Cycles
	return x
}

func busName(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "unknown", "unspecified":
		return ""
	case "nvme":
		return "NVMe"
	case "sata", "ata":
		return "SATA"
	case "usb":
		return "USB"
	case "sas":
		return "SAS"
	case "raid":
		return "RAID"
	case "apple fabric":
		return "Apple Fabric"
	case "pci-express", "pcie":
		return "PCIe"
	case "thunderbolt":
		return "Thunderbolt"
	}
	return strings.TrimSpace(s)
}

// ---- macOS ----

// parseMacProfiler reads `system_profiler -json SPHardwareDataType
// SPDisplaysDataType SPMemoryDataType SPStorageDataType`.
func parseMacProfiler(out []byte) hwExtras {
	var v struct {
		Hardware []struct {
			BootROM string `json:"boot_rom_version"`
			Chip    string `json:"chip_type"` // Apple silicon only
		} `json:"SPHardwareDataType"`
		Displays []struct {
			Model string `json:"sppci_model"`
			Cores string `json:"sppci_cores"`
		} `json:"SPDisplaysDataType"`
		Memory []struct {
			Size       string `json:"SPMemoryDataType"` // Apple silicon: "16 GB"
			Type       string `json:"dimm_type"`
			Upgradable string `json:"is_memory_upgradeable"`
			Items      []struct {
				Name  string `json:"_name"`
				Size  string `json:"dimm_size"`
				Speed string `json:"dimm_speed"`
				Type  string `json:"dimm_type"`
			} `json:"_items"`
		} `json:"SPMemoryDataType"`
		Storage []struct {
			Size  uint64 `json:"size_in_bytes"`
			Drive struct {
				Name     string `json:"device_name"`
				Medium   string `json:"medium_type"`
				Protocol string `json:"protocol"`
				SMART    string `json:"smart_status"`
			} `json:"physical_drive"`
		} `json:"SPStorageDataType"`
	}
	var x hwExtras
	if json.Unmarshal(out, &v) != nil {
		return x
	}
	if len(v.Hardware) > 0 {
		h := v.Hardware[0]
		fw := &protocol.Firmware{Vendor: "Apple", Version: strings.TrimSpace(h.BootROM)}
		if h.Chip != "" { // Apple silicon: the Secure Enclave plays the TPM's part
			fw.SecurityChip = "Apple Secure Enclave"
		}
		x.Firmware = fw
	}
	for _, d := range v.Displays {
		if d.Model == "" {
			continue
		}
		cores, _ := strconv.Atoi(strings.TrimSpace(d.Cores))
		x.GPUs = append(x.GPUs, protocol.GPU{Name: d.Model, Cores: cores})
	}
	if len(v.Memory) > 0 {
		m := v.Memory[0]
		mem := &protocol.MemoryLayout{}
		if len(m.Items) > 0 { // Intel: one entry per slot
			mem.SlotsTotal = len(m.Items)
			for _, it := range m.Items {
				b := parseSize(it.Size)
				if b == 0 {
					continue // "empty"
				}
				mem.Modules = append(mem.Modules, protocol.MemoryModule{Slot: it.Name, CapacityBytes: b, Type: it.Type,
					SpeedMTs: leadingInt(it.Speed)})
			}
			x.Memory = layout(mem)
			if x.Memory != nil && strings.EqualFold(m.Upgradable, "No") {
				x.Memory.Soldered = true
			}
		} else if m.Type != "" { // Apple silicon: unified memory in the chip package
			mem.Type = m.Type
			if b := parseSize(m.Size); b > 0 {
				mem.Modules = []protocol.MemoryModule{{Slot: "Unified", CapacityBytes: b, Type: m.Type}}
			}
			x.Memory = layout(mem)
			if x.Memory != nil {
				x.Memory.Soldered, x.Memory.SlotsTotal, x.Memory.SlotsUsed = true, 0, 0
			}
		}
	}
	// Volumes share a physical drive: list each drive once, sized by its largest volume.
	byName := map[string]*protocol.PhysicalDrive{}
	var order []string
	for _, s := range v.Storage {
		name := strings.TrimSpace(s.Drive.Name)
		if name == "" {
			continue
		}
		d, ok := byName[name]
		if !ok {
			media := ""
			switch strings.ToLower(s.Drive.Medium) {
			case "ssd":
				media = "SSD"
			case "rotational", "hdd":
				media = "HDD"
			}
			d = &protocol.PhysicalDrive{Model: name, Media: media, Bus: busName(s.Drive.Protocol), Health: normHealth(s.Drive.SMART)}
			byName[name] = d
			order = append(order, name)
		}
		if s.Size > d.SizeBytes {
			d.SizeBytes = s.Size
		}
	}
	for _, n := range order {
		x.Drives = append(x.Drives, *byName[n])
	}
	return x
}

var sizeRe = regexp.MustCompile(`(?i)^\s*([\d.]+)\s*(TB|GB|MB)\b`)

// parseSize reads "16 GB", "8192 MB" or "1 TB" (binary units, as the tools mean them).
func parseSize(s string) uint64 {
	m := sizeRe.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	n, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0
	}
	mult := map[string]float64{"MB": 1 << 20, "GB": 1 << 30, "TB": 1 << 40}[strings.ToUpper(m[2])]
	return uint64(n * mult)
}

func leadingInt(s string) int {
	f := strings.Fields(s)
	if len(f) == 0 {
		return 0
	}
	n, _ := strconv.Atoi(f[0])
	return n
}

// ---- Linux ----

// parseLsblk reads `lsblk -d -b -J -o NAME,MODEL,SIZE,ROTA,TRAN,TYPE` and also
// returns each drive's device name (sda, nvme0n1). Older util-linux prints
// numbers and booleans as strings, so both are accepted.
func parseLsblk(out []byte) (drives []protocol.PhysicalDrive, names []string) {
	var v struct {
		Devices []map[string]any `json:"blockdevices"`
	}
	if json.Unmarshal(out, &v) != nil {
		return nil, nil
	}
	for _, d := range v.Devices {
		name, _ := d["name"].(string)
		if t, _ := d["type"].(string); t != "disk" || strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "zram") || strings.HasPrefix(name, "ram") {
			continue
		}
		size := anyUint(d["size"])
		if size == 0 {
			continue
		}
		model, _ := d["model"].(string)
		model = strings.TrimSpace(model)
		if model == "" {
			model = name // virtual disks often have no model
		}
		media := "SSD"
		if anyBool(d["rota"]) {
			media = "HDD"
		}
		tran, _ := d["tran"].(string)
		bus := busName(tran)
		if strings.HasPrefix(name, "nvme") {
			bus = "NVMe"
		}
		drives = append(drives, protocol.PhysicalDrive{Model: model, Media: media, Bus: bus, SizeBytes: size})
		names = append(names, name)
	}
	return drives, names
}

func anyUint(v any) uint64 {
	switch t := v.(type) {
	case float64:
		return uint64(t)
	case string:
		n, _ := strconv.ParseUint(strings.TrimSpace(t), 10, 64)
		return n
	}
	return 0
}

func anyBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "1" || strings.EqualFold(t, "true")
	case float64:
		return t != 0
	}
	return false
}

// parseSmartctl reads `smartctl -H` output.
func parseSmartctl(out string) string {
	switch {
	case strings.Contains(out, "test result: PASSED"), strings.Contains(out, "SMART Health Status: OK"):
		return "healthy"
	case strings.Contains(out, "test result: FAILED"), strings.Contains(out, "FAILING_NOW"):
		return "failing"
	}
	return ""
}

var lspciField = regexp.MustCompile(`"([^"]*)"`)

// parseLspci reads `lspci -mm` and returns the display controllers.
func parseLspci(out string) []protocol.GPU {
	var gpus []protocol.GPU
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		f := lspciField.FindAllStringSubmatch(sc.Text(), -1)
		if len(f) < 3 {
			continue
		}
		class := f[0][1]
		if !strings.Contains(class, "VGA") && !strings.Contains(class, "3D controller") && !strings.Contains(class, "Display controller") {
			continue
		}
		vendor := strings.NewReplacer(" Corporation", "", " Corp.", "", ", Inc.", "", " Inc.", "").Replace(f[1][1])
		gpus = append(gpus, protocol.GPU{Name: strings.TrimSpace(vendor + " " + f[2][1])})
	}
	return gpus
}

// parseDmidecodeMemory reads `dmidecode -t memory`.
func parseDmidecodeMemory(out string) *protocol.MemoryLayout {
	mem := &protocol.MemoryLayout{}
	devices := 0
	soldered := false
	var cur map[string]string
	flush := func() {
		if cur == nil {
			return
		}
		devices++
		if b := parseSize(cur["Size"]); b > 0 {
			speed := leadingInt(cur["Configured Memory Speed"])
			if speed == 0 {
				speed = leadingInt(cur["Speed"])
			}
			typ := cur["Type"]
			if typ == "Unknown" || typ == "Other" {
				typ = ""
			}
			mem.Modules = append(mem.Modules, protocol.MemoryModule{Slot: cur["Locator"], CapacityBytes: b, Type: typ, SpeedMTs: speed})
			soldered = soldered || strings.EqualFold(cur["Form Factor"], "Row Of Chips")
		}
		cur = nil
	}
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		trim := strings.TrimSpace(line)
		switch {
		case trim == "Memory Device":
			flush()
			cur = map[string]string{}
		case !strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, " "):
			flush()
		case strings.HasPrefix(trim, "Number Of Devices:"):
			mem.SlotsTotal = leadingInt(strings.TrimPrefix(trim, "Number Of Devices:"))
		case cur != nil:
			if k, v, ok := strings.Cut(trim, ":"); ok {
				cur[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
	}
	flush()
	if mem.SlotsTotal == 0 {
		mem.SlotsTotal = devices
	}
	out2 := layout(mem)
	if out2 != nil && soldered {
		out2.Soldered = true
	}
	return out2
}

// biosDate turns the DMI "MM/DD/YYYY" into "YYYY-MM-DD".
func biosDate(s string) string {
	p := strings.Split(strings.TrimSpace(s), "/")
	if len(p) != 3 || len(p[2]) != 4 {
		return strings.TrimSpace(s)
	}
	return p[2] + "-" + p[0] + "-" + p[1]
}

// sortDrives keeps internal-looking drives first and the order stable.
func sortDrives(d []protocol.PhysicalDrive) {
	sort.SliceStable(d, func(i, j int) bool { return d[i].Bus != "USB" && d[j].Bus == "USB" })
}
