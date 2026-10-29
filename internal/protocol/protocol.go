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
)

// CommandTypes lists every command type a user may issue.
var CommandTypes = []string{
	CmdRunScript, CmdReboot, CmdShutdown, CmdLock, CmdDisplayMessage,
	CmdRefreshInventory, CmdUninstallAgent,
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
}

type Network struct {
	Hostname   string      `json:"hostname"`
	PrimaryIP  string      `json:"primary_ip"`
	PrimaryMAC string      `json:"primary_mac"`
	WifiSSID   *string     `json:"wifi_ssid"`
	Interfaces []Interface `json:"interfaces"`
}

type Interface struct {
	Name  string   `json:"name"`
	MAC   string   `json:"mac"`
	Addrs []string `json:"addrs"`
	Up    bool     `json:"up"`
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
