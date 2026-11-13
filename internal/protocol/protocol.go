// Package protocol defines the messages exchanged between the agent and the
// server over the agent WebSocket. Every frame is a JSON-encoded Envelope.
package protocol

import (
	"encoding/json"
	"fmt"
	"time"
)

// Version is bumped when the wire format changes incompatibly.
const Version = 1

// Message types sent by the agent.
const (
	TypeHello         = "hello"
	TypeHeartbeat     = "heartbeat"
	TypeTelemetry     = "telemetry"
	TypeInventory     = "inventory"
	TypeCommandResult = "command_result"
	TypeEvent         = "event"
	TypeUsage         = "usage"
)

// Message types sent by the server.
const (
	TypeCommand = "command"
	TypePolicy  = "policy"
)

// Command types understood by the agent.
const (
	CmdRunScript        = "run_script"
	CmdReboot           = "reboot"
	CmdShutdown         = "shutdown"
	CmdLock             = "lock"
	CmdDisplayMessage   = "display_message"
	CmdRefreshInventory = "refresh_inventory"
	CmdUninstallAgent   = "uninstall_agent"
	CmdUpdateAgent      = "update_agent"
)

// CommandTypes lists every command type a user may issue.
var CommandTypes = []string{
	CmdRunScript, CmdReboot, CmdShutdown, CmdLock, CmdDisplayMessage,
	CmdRefreshInventory, CmdUninstallAgent, CmdUpdateAgent,
}

// Command status values.
const (
	StatusQueued    = "queued"
	StatusSent      = "sent"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusTimeout   = "timeout"
	StatusCancelled = "cancelled"
)

// MaxOutputBytes caps stdout and stderr captured from a script.
const MaxOutputBytes = 1 << 20

// Envelope wraps every frame on the wire.
type Envelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// Encode marshals a typed payload into an Envelope frame.
func Encode(typ string, v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return json.Marshal(Envelope{Type: typ, Data: data})
}

// Decode parses a frame into its envelope; callers unmarshal Data by Type.
func Decode(b []byte) (Envelope, error) {
	var e Envelope
	if err := json.Unmarshal(b, &e); err != nil {
		return e, err
	}
	if e.Type == "" {
		return e, fmt.Errorf("protocol: missing message type")
	}
	return e, nil
}

// EnrollRequest is POSTed to /api/agent/enroll.
type EnrollRequest struct {
	Token string `json:"token"`
	Facts Facts  `json:"facts"`
}

// Facts are the minimal host facts known at enrollment.
type Facts struct {
	Hostname     string `json:"hostname"`
	OSFamily     string `json:"os_family"`
	OSName       string `json:"os_name"`
	OSVersion    string `json:"os_version"`
	Arch         string `json:"arch"`
	Serial       string `json:"serial"`
	AgentVersion string `json:"agent_version"`
}

// EnrollResponse returns the device's long-lived credential.
type EnrollResponse struct {
	DeviceID     string `json:"device_id"`
	DeviceSecret string `json:"device_secret"`
}

// Hello is the first frame the agent sends after connecting.
type Hello struct {
	ProtocolVersion int    `json:"protocol_version"`
	AgentVersion    string `json:"agent_version"`
	Facts           Facts  `json:"facts"`
}

// Telemetry is a lightweight periodic sample.
type Telemetry struct {
	TS         time.Time `json:"ts"`
	CPUPct     float64   `json:"cpu_pct"`
	MemPct     float64   `json:"mem_pct"`
	DiskPct    float64   `json:"disk_pct"`
	BatteryPct *float64  `json:"battery_pct"`
	PrimaryIP  string    `json:"primary_ip"`
	LoggedIn   string    `json:"logged_in_user"`
}

// Inventory is the full device snapshot.
type Inventory struct {
	CollectedAt time.Time  `json:"collected_at"`
	OS          OSInfo     `json:"os"`
	Hardware    Hardware   `json:"hardware"`
	Network     Network    `json:"network"`
	Security    Security   `json:"security"`
	Users       Users      `json:"users"`
	Software    []Software `json:"software"`
}

type OSInfo struct {
	Family        string    `json:"family"`
	Name          string    `json:"name"`
	Version       string    `json:"version"`
	Build         string    `json:"build"`
	Kernel        string    `json:"kernel"`
	Arch          string    `json:"arch"`
	UptimeSeconds uint64    `json:"uptime_seconds"`
	BootTime      time.Time `json:"boot_time"`
}

type Hardware struct {
	Manufacturer     string  `json:"manufacturer"`
	Model            string  `json:"model"`
	Serial           string  `json:"serial"`
	CPUModel         string  `json:"cpu_model"`
	CPUCores         int     `json:"cpu_cores"`
	CPUThreads       int     `json:"cpu_threads"`
	MemoryTotalBytes uint64  `json:"memory_total_bytes"`
	MemoryUsedBytes  uint64  `json:"memory_used_bytes"`
	Disks            []Disk  `json:"disks"`
	Battery          Battery `json:"battery"`

	// Hardware inventory v2 (agents from 2026.11.15). All optional: nil or
	// empty means the agent couldn't determine it, or is older.
	Firmware *Firmware       `json:"firmware,omitempty"`
	GPUs     []GPU           `json:"gpus,omitempty"`
	Memory   *MemoryLayout   `json:"memory,omitempty"`
	Drives   []PhysicalDrive `json:"drives,omitempty"`
}

// Firmware is the BIOS/UEFI and the hardware security chip.
type Firmware struct {
	Vendor      string `json:"vendor,omitempty"`
	Version     string `json:"version,omitempty"`
	ReleaseDate string `json:"release_date,omitempty"` // YYYY-MM-DD when known
	SecureBoot  *bool  `json:"secure_boot,omitempty"`
	// SecurityChip is e.g. "TPM 2.0" or "Apple Secure Enclave"; "" if none was found.
	SecurityChip string `json:"security_chip,omitempty"`
	TPMEnabled   *bool  `json:"tpm_enabled,omitempty"`
}

type GPU struct {
	Name          string `json:"name"`
	DriverVersion string `json:"driver_version,omitempty"`
	Cores         int    `json:"cores,omitempty"`
}

// MemoryLayout describes the installed RAM modules and free slots.
type MemoryLayout struct {
	Type       string         `json:"type,omitempty"` // DDR4, DDR5, LPDDR5, …
	SpeedMTs   int            `json:"speed_mts,omitempty"`
	SlotsTotal int            `json:"slots_total,omitempty"`
	SlotsUsed  int            `json:"slots_used,omitempty"`
	Soldered   bool           `json:"soldered,omitempty"` // on the board, not upgradable
	Modules    []MemoryModule `json:"modules,omitempty"`
}

type MemoryModule struct {
	Slot          string `json:"slot,omitempty"`
	CapacityBytes uint64 `json:"capacity_bytes"`
	Type          string `json:"type,omitempty"`
	SpeedMTs      int    `json:"speed_mts,omitempty"`
}

// PhysicalDrive is a disk device (as opposed to a volume in Disks).
type PhysicalDrive struct {
	Model     string `json:"model"`
	Media     string `json:"media,omitempty"` // SSD, HDD
	Bus       string `json:"bus,omitempty"`   // NVMe, SATA, USB, RAID, …
	SizeBytes uint64 `json:"size_bytes,omitempty"`
	Health    string `json:"health,omitempty"` // healthy, warning, failing; "" unknown
}

type Disk struct {
	Mount      string `json:"mount"`
	FSType     string `json:"fstype"`
	TotalBytes uint64 `json:"total_bytes"`
	UsedBytes  uint64 `json:"used_bytes"`
	FreeBytes  uint64 `json:"free_bytes"`
}

type Battery struct {
	Present   bool     `json:"present"`
	Percent   float64  `json:"percent"`
	State     string   `json:"state"`
	HealthPct *float64 `json:"health_pct"`
	// From 2026.11.15: charge cycles, and capacity now vs. when new (mWh).
	CycleCount *int    `json:"cycle_count,omitempty"`
	DesignMWh  float64 `json:"design_mwh,omitempty"`
	FullMWh    float64 `json:"full_mwh,omitempty"`
}

type Network struct {
	Hostname   string      `json:"hostname"`
	PrimaryIP  string      `json:"primary_ip"`
	PrimaryMAC string      `json:"primary_mac"`
	WifiSSID   *string     `json:"wifi_ssid"`
	Interfaces []Interface `json:"interfaces"`
	// From 2026.11.18. Details is set by agents that collect the fields below,
	// so the console can tell "none" (e.g. no gateway) from "not reported".
	Details      bool      `json:"details,omitempty"`
	Gateway      string    `json:"gateway,omitempty"`       // default gateway of the primary interface
	DNS          []string  `json:"dns,omitempty"`           // DNS servers in use
	Domain       string    `json:"domain,omitempty"`        // AD domain when joined, else the workgroup (Windows)
	DomainJoined *bool     `json:"domain_joined,omitempty"` // nil where it isn't known
	Wifi         *WifiLink `json:"wifi,omitempty"`          // the current Wi-Fi connection, if any
}

// WifiLink describes a connected Wi-Fi network. Empty fields weren't readable.
type WifiLink struct {
	SSID      string  `json:"ssid,omitempty"`
	SignalPct *int    `json:"signal_pct,omitempty"`
	Band      string  `json:"band,omitempty"`  // "2.4 GHz", "5 GHz", "6 GHz"
	Radio     string  `json:"radio,omitempty"` // "802.11ax"
	RxMbps    float64 `json:"rx_mbps,omitempty"`
}

type Interface struct {
	Name  string   `json:"name"`
	MAC   string   `json:"mac"`
	Addrs []string `json:"addrs"`
	Up    bool     `json:"up"`
	// From 2026.11.18.
	Kind        string  `json:"kind,omitempty"` // ethernet | wifi | vpn | virtual | bluetooth
	Description string  `json:"description,omitempty"`
	SpeedMbps   float64 `json:"speed_mbps,omitempty"`
	DHCP        *bool   `json:"dhcp,omitempty"`
	Gateway     string  `json:"gateway,omitempty"`
}

// Security fields are pointers because nil means "could not determine".
type Security struct {
	DiskEncrypted     *bool   `json:"disk_encrypted"`
	EncryptionMethod  *string `json:"encryption_method"`
	FirewallEnabled   *bool   `json:"firewall_enabled"`
	ScreenLockEnabled *bool   `json:"screen_lock_enabled"`
	PendingUpdates    *int    `json:"pending_updates"`
	Antivirus         *string `json:"antivirus"`
}

type Users struct {
	LoggedIn []string `json:"logged_in"`
}

type Software struct {
	Name        string  `json:"name"`
	Version     string  `json:"version"`
	Publisher   string  `json:"publisher"`
	Source      string  `json:"source"`
	InstallDate *string `json:"install_date"`
	// InstallLocation is used by the agent to attribute running processes
	// to an app; the server doesn't store it.
	InstallLocation string `json:"install_location,omitempty"`
}

// Command is pushed from server to agent.
type Command struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// RunScriptPayload is the resolved payload for run_script; the server
// inlines library scripts so the agent never sees a script_id.
type RunScriptPayload struct {
	Shell          string `json:"shell"`
	Script         string `json:"script"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

// UpdatePayload tells the agent which build to install. File is downloaded from
// <server>/downloads/<file> and must match SHA256 and Size before it replaces the agent.
type UpdatePayload struct {
	Version string `json:"version"`
	File    string `json:"file"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
}

type DelayPayload struct {
	DelaySeconds int `json:"delay_seconds"`
}

type MessagePayload struct {
	Title   string `json:"title"`
	Message string `json:"message"`
}

// CommandResult reports progress or completion of a command. The agent
// sends status=running when it starts and a terminal status at the end.
type CommandResult struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	ExitCode *int   `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Error    string `json:"error,omitempty"`
}

// Policy is pushed on connect and whenever the device's effective policy changes.
type Policy struct {
	BlockedApps          []string `json:"blocked_apps"`
	TelemetryIntervalSec int      `json:"telemetry_interval_sec"`
	InventoryIntervalSec int      `json:"inventory_interval_sec"`
	UsageTracking        bool     `json:"usage_tracking"`
	IdleThresholdSec     int      `json:"idle_threshold_sec"`
	// ServerURL is the address agents should use. When it differs from
	// theirs, agents check it is reachable and switch to it.
	ServerURL string `json:"server_url,omitempty"`
	// FallbackURLs are other addresses of the same server. Agents keep them
	// and connect through one while ServerURL is unreachable (e.g. its DNS
	// name fails), returning to ServerURL once it answers again.
	FallbackURLs []string `json:"fallback_urls,omitempty"`
}

// Usage reports application running time accumulated since the last report.
type Usage struct {
	Entries []UsageEntry `json:"entries"`
}

// UsageEntry is the minutes an app ran for one user on one (device-local) day.
type UsageEntry struct {
	Date    string `json:"date"` // YYYY-MM-DD
	App     string `json:"app"`
	User    string `json:"user"`
	Minutes int    `json:"minutes"`
	// ActiveSeconds is time the app was in front while the user was
	// interacting (input within the org's idle threshold).
	ActiveSeconds int       `json:"active_seconds,omitempty"`
	LastSeen      time.Time `json:"last_seen"`
}

// Event is an agent-originated notable occurrence (e.g. a blocked app was killed).
type Event struct {
	Kind    string         `json:"kind"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// IsTerminal reports whether a command status is final.
func IsTerminal(status string) bool {
	switch status {
	case StatusSucceeded, StatusFailed, StatusTimeout, StatusCancelled:
		return true
	}
	return false
}
