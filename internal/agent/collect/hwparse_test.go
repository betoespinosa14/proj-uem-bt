package collect

import (
	"encoding/json"
	"testing"

	"github.com/uem/uem/internal/protocol"
)

func TestEncodePS(t *testing.T) {
	if got := encodePS("Get-Date"); got != "RwBlAHQALQBEAGEAdABlAA==" {
		t.Fatalf("encodePS = %s", got)
	}
}

func TestParseWindowsHW(t *testing.T) {
	// A laptop: two GPUs, one LPDDR5 module reported, NVMe drive, TPM 2.0.
	out := `{"bios":{"vendor":"LENOVO","version":"R2FET44W (1.24 )","date":"2025-03-18"},"secureBoot":true,
	"tpm":{"spec":"2.0, 0, 1.59","enabled":true},
	"gpus":[{"name":"Intel(R) Iris(R) Xe Graphics","driver":"31.0.101.5382"},{"name":"Microsoft Remote Display Adapter","driver":""}],
	"mem":{"slot":"Controller0-ChannelA-DIMM0","bytes":17179869184,"type":35,"speed":6400,"form":0},
	"slots":8,
	"disks":[{"model":"SKHynix_HFS512GEJ4X112N","media":"SSD","bus":"NVMe","size":512110190592,"health":"Healthy"},
	         {"model":"USB Stick","media":"Unspecified","bus":"USB","size":31029460992,"health":"Warning"}],
	"cycles":142}`
	x := parseWindowsHW(out)
	fw := x.Firmware
	if fw == nil || fw.Vendor != "LENOVO" || fw.Version != "R2FET44W (1.24 )" || fw.ReleaseDate != "2025-03-18" ||
		fw.SecureBoot == nil || !*fw.SecureBoot || fw.SecurityChip != "TPM 2.0" || fw.TPMEnabled == nil || !*fw.TPMEnabled {
		t.Fatalf("firmware = %+v", fw)
	}
	if len(x.GPUs) != 2 || x.GPUs[0].Name != "Intel(R) Iris(R) Xe Graphics" || x.GPUs[0].DriverVersion != "31.0.101.5382" {
		t.Fatalf("gpus = %+v", x.GPUs)
	}
	m := x.Memory // a single module comes as an object, not an array
	if m == nil || m.Type != "LPDDR5" || m.SpeedMTs != 6400 || m.SlotsUsed != 1 || m.SlotsTotal != 8 || !m.Soldered {
		t.Fatalf("memory = %+v", m)
	}
	if len(x.Drives) != 2 || x.Drives[0].Bus != "NVMe" || x.Drives[0].Media != "SSD" || x.Drives[0].Health != "healthy" ||
		x.Drives[1].Media != "" || x.Drives[1].Health != "warning" {
		t.Fatalf("drives = %+v", x.Drives)
	}
	if x.CycleCount == nil || *x.CycleCount != 142 {
		t.Fatalf("cycles = %v", x.CycleCount)
	}

	// A desktop with legacy BIOS, no TPM and two of four DDR4 slots filled.
	x = parseWindowsHW(`{"bios":{"vendor":"Dell Inc.","version":"2.3.1","date":""},"secureBoot":null,"tpm":null,"gpus":[],
	"mem":[{"slot":"DIMM1","bytes":8589934592,"type":26,"speed":2666,"form":8},{"slot":"DIMM2","bytes":8589934592,"type":26,"speed":2666,"form":8}],
	"slots":4,"disks":[],"cycles":null}`)
	if x.Firmware.SecureBoot != nil || x.Firmware.SecurityChip != "" {
		t.Fatalf("firmware = %+v", x.Firmware)
	}
	if m := x.Memory; m == nil || m.Type != "DDR4" || m.SlotsUsed != 2 || m.SlotsTotal != 4 || m.Soldered {
		t.Fatalf("memory = %+v", m)
	}
	if parseWindowsHW("not json").Firmware != nil {
		t.Fatal("garbage should give nothing")
	}
}

func TestParseMacProfiler(t *testing.T) {
	// Apple silicon (from a Mac mini M4).
	apple := `{"SPHardwareDataType":[{"boot_rom_version":"20457.41.3","chip_type":"Apple M4"}],
	"SPDisplaysDataType":[{"sppci_model":"Apple M4","sppci_cores":"10"}],
	"SPMemoryDataType":[{"SPMemoryDataType":"16 GB","dimm_manufacturer":"Hynix","dimm_type":"LPDDR5"}],
	"SPStorageDataType":[
	 {"_name":"Data","size_in_bytes":494384795648,"physical_drive":{"device_name":"APPLE SSD AP0512Z","medium_type":"ssd","protocol":"Apple Fabric","smart_status":"Verified"}},
	 {"_name":"Macintosh HD","size_in_bytes":494384795648,"physical_drive":{"device_name":"APPLE SSD AP0512Z","medium_type":"ssd","protocol":"Apple Fabric","smart_status":"Verified"}},
	 {"_name":"Backup","size_in_bytes":2000398934016,"physical_drive":{"device_name":"WD Elements","medium_type":"rotational","protocol":"USB","smart_status":"Not Supported"}}]}`
	x := parseMacProfiler([]byte(apple))
	if x.Firmware == nil || x.Firmware.Version != "20457.41.3" || x.Firmware.SecurityChip != "Apple Secure Enclave" || x.Firmware.SecureBoot != nil {
		t.Fatalf("firmware = %+v", x.Firmware)
	}
	if len(x.GPUs) != 1 || x.GPUs[0].Name != "Apple M4" || x.GPUs[0].Cores != 10 {
		t.Fatalf("gpus = %+v", x.GPUs)
	}
	if m := x.Memory; m == nil || m.Type != "LPDDR5" || !m.Soldered || m.SlotsTotal != 0 || len(m.Modules) != 1 || m.Modules[0].CapacityBytes != 16<<30 {
		t.Fatalf("memory = %+v", m)
	}
	if len(x.Drives) != 2 || x.Drives[0].Model != "APPLE SSD AP0512Z" || x.Drives[0].Health != "healthy" ||
		x.Drives[1].Media != "HDD" || x.Drives[1].Bus != "USB" || x.Drives[1].Health != "" {
		t.Fatalf("drives = %+v", x.Drives)
	}

	// An Intel iMac with two of four slots used.
	intel := `{"SPHardwareDataType":[{"boot_rom_version":"1916.40.8.0.0"}],
	"SPMemoryDataType":[{"is_memory_upgradeable":"Yes","_items":[
	 {"_name":"ChannelA-DIMM0","dimm_size":"8 GB","dimm_speed":"2667 MHz","dimm_type":"DDR4"},
	 {"_name":"ChannelA-DIMM1","dimm_size":"empty"},
	 {"_name":"ChannelB-DIMM0","dimm_size":"8 GB","dimm_speed":"2667 MHz","dimm_type":"DDR4"},
	 {"_name":"ChannelB-DIMM1","dimm_size":"empty"}]}]}`
	x = parseMacProfiler([]byte(intel))
	if x.Firmware.SecurityChip != "" {
		t.Fatalf("Intel Mac security chip = %q", x.Firmware.SecurityChip)
	}
	if m := x.Memory; m == nil || m.Type != "DDR4" || m.SpeedMTs != 2667 || m.SlotsUsed != 2 || m.SlotsTotal != 4 || m.Soldered {
		t.Fatalf("memory = %+v", m)
	}
}

func TestParseLinux(t *testing.T) {
	// lsblk: new style (numbers, booleans) and old style (strings).
	drives, names := parseLsblk([]byte(`{"blockdevices":[
	 {"name":"nvme0n1","model":"Samsung SSD 980 1TB","size":1000204886016,"rota":false,"tran":"nvme","type":"disk"},
	 {"name":"sda","model":"ST2000DM008-2FR102","size":"2000398934016","rota":"1","tran":"sata","type":"disk"},
	 {"name":"vda","model":null,"size":42949672960,"rota":true,"tran":null,"type":"disk"},
	 {"name":"loop0","model":null,"size":4096,"rota":false,"tran":null,"type":"loop"},
	 {"name":"sr0","model":"DVD","size":1073741312,"rota":true,"tran":"sata","type":"rom"}]}`))
	if len(drives) != 3 || names[1] != "sda" {
		t.Fatalf("drives = %+v names = %v", drives, names)
	}
	if drives[0].Bus != "NVMe" || drives[0].Media != "SSD" || drives[1].Media != "HDD" || drives[1].SizeBytes != 2000398934016 || drives[2].Model != "vda" {
		t.Fatalf("drives = %+v", drives)
	}

	gpus := parseLspci(`00:02.0 "VGA compatible controller" "Intel Corporation" "UHD Graphics 630" -r00 "Dell" "Device 0869"
00:1f.3 "Audio device" "Intel Corporation" "Cannon Lake PCH cAVS" -r10 "Dell" "Device 0869"
01:00.0 "3D controller" "NVIDIA Corporation" "GA107M [GeForce RTX 3050 Mobile]" -ra1 "Dell" "Device 0a62"`)
	if len(gpus) != 2 || gpus[0].Name != "Intel UHD Graphics 630" || gpus[1].Name != "NVIDIA GA107M [GeForce RTX 3050 Mobile]" {
		t.Fatalf("gpus = %+v", gpus)
	}

	mem := parseDmidecodeMemory(`# dmidecode 3.3
Handle 0x0010, DMI type 16, 23 bytes
Physical Memory Array
	Location: System Board Or Motherboard
	Number Of Devices: 4

Handle 0x0011, DMI type 17, 84 bytes
Memory Device
	Size: 16 GB
	Form Factor: DIMM
	Locator: DIMM_A1
	Type: DDR4
	Speed: 3200 MT/s
	Configured Memory Speed: 2933 MT/s

Handle 0x0012, DMI type 17, 84 bytes
Memory Device
	Size: No Module Installed
	Locator: DIMM_A2
	Type: Unknown

Handle 0x0013, DMI type 17, 84 bytes
Memory Device
	Size: 16384 MB
	Form Factor: DIMM
	Locator: DIMM_B1
	Type: DDR4
	Speed: 3200 MT/s
`)
	if mem == nil || mem.Type != "DDR4" || mem.SpeedMTs != 2933 || mem.SlotsUsed != 2 || mem.SlotsTotal != 4 || mem.Soldered {
		t.Fatalf("memory = %+v", mem)
	}
	if mem.Modules[1].CapacityBytes != 16<<30 {
		t.Fatalf("16384 MB = %d", mem.Modules[1].CapacityBytes)
	}

	if parseSmartctl("SMART overall-health self-assessment test result: PASSED") != "healthy" ||
		parseSmartctl("SMART Health Status: OK") != "healthy" ||
		parseSmartctl("SMART overall-health self-assessment test result: FAILED!") != "failing" ||
		parseSmartctl("Unable to detect device type") != "" {
		t.Fatal("smartctl mapping")
	}
	if biosDate("03/18/2025") != "2025-03-18" || biosDate("") != "" {
		t.Fatal("bios date")
	}
}

func TestHardwareExtrasApply(t *testing.T) {
	n := 87
	hw := protocol.Hardware{Battery: protocol.Battery{Present: true}}
	hwExtras{CycleCount: &n, Drives: []protocol.PhysicalDrive{{Model: "x"}}}.apply(&hw)
	if hw.Battery.CycleCount == nil || *hw.Battery.CycleCount != 87 || len(hw.Drives) != 1 {
		t.Fatalf("hw = %+v", hw)
	}
	// No battery: the cycle count is ignored. Old fields stay omitted from JSON.
	desk := protocol.Hardware{}
	hwExtras{CycleCount: &n}.apply(&desk)
	b, _ := json.Marshal(desk)
	for _, k := range []string{"firmware", "gpus", "drives", "cycle_count"} {
		if json.Valid(b) && contains(string(b), `"`+k+`"`) {
			t.Fatalf("%s should be omitted: %s", k, b)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
