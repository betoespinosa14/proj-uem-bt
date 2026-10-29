package store

import (
	"encoding/json"
	"time"
)

// The JSON shapes here are the API contract documented in docs/API.md.

type Org struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	ServerURL        string    `json:"server_url"`
	UsageTracking    bool      `json:"usage_tracking"`
	IdleThresholdSec int       `json:"idle_threshold_sec"`
	CreatedAt        time.Time `json:"created_at"`
}

type User struct {
	ID           string     `json:"id"`
	OrgID        string     `json:"-"`
	Email        string     `json:"email"`
	Name         string     `json:"name"`
	Role         string     `json:"role"`
	PasswordHash string     `json:"-"`
	CreatedAt    time.Time  `json:"created_at"`
	LastLoginAt  *time.Time `json:"last_login_at"`
}

type Session struct {
	ID        string
	UserID    string
	OrgID     string
	CSRFToken string
	ExpiresAt time.Time
}

type Group struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	BlockedApps []string  `json:"blocked_apps"`
	DeviceCount int       `json:"device_count"`
	CreatedAt   time.Time `json:"created_at"`
}

type EnrollmentToken struct {
	ID          string     `json:"id"`
	OrgID       string     `json:"-"`
	Token       string     `json:"token"`
	Description string     `json:"description"`
	GroupID     *string    `json:"group_id"`
	GroupName   *string    `json:"group_name"`
	ExpiresAt   *time.Time `json:"expires_at"`
	MaxUses     *int       `json:"max_uses"`
	Uses        int        `json:"uses"`
	Revoked     bool       `json:"revoked"`
	CreatedAt   time.Time  `json:"created_at"`
}

type Device struct {
	ID              string     `json:"id"`
	OrgID           string     `json:"-"`
	Name            string     `json:"name"`
	Hostname        string     `json:"hostname"`
	OSFamily        string     `json:"os_family"`
	OSName          string     `json:"os_name"`
	OSVersion       string     `json:"os_version"`
	OSBuild         string     `json:"os_build"`
	Arch            string     `json:"arch"`
	Serial          string     `json:"serial"`
	Model           string     `json:"model"`
	Manufacturer    string     `json:"manufacturer"`
	AgentVersion    string     `json:"agent_version"`
	IPAddress       string     `json:"ip_address"`
	MACAddress      string     `json:"mac_address"`
	LoggedInUser    string     `json:"logged_in_user"`
	Status          string     `json:"status"`
	LastSeenAt      *time.Time `json:"last_seen_at"`
	EnrolledAt      time.Time  `json:"enrolled_at"`
	ComplianceState string     `json:"compliance_state"`
	GroupID         *string    `json:"group_id"`
	GroupName       *string    `json:"group_name"`
	Tags            []string   `json:"tags"`
	CPUPct          *float64   `json:"cpu_pct"`
	MemPct          *float64   `json:"mem_pct"`
	DiskPct         *float64   `json:"disk_pct"`
	BatteryPct      *float64   `json:"battery_pct"`
}

type DeviceDetail struct {
	Device
	Inventory json.RawMessage `json:"inventory"`
}

type Software struct {
	Name        string     `json:"name"`
	Version     string     `json:"version"`
	Publisher   string     `json:"publisher"`
	Source      string     `json:"source"`
	InstallDate *string    `json:"install_date"`
	LastUsed    *time.Time `json:"last_used"`      // most recent tracked use by any user
	Category    string     `json:"category"`       // set by the API from the compliance catalog
	Flag        string     `json:"flag,omitempty"` // set by the API from the compliance catalog
}

type TelemetryPoint struct {
	TS         time.Time `json:"ts"`
	CPUPct     *float64  `json:"cpu_pct"`
	MemPct     *float64  `json:"mem_pct"`
	DiskPct    *float64  `json:"disk_pct"`
	BatteryPct *float64  `json:"battery_pct"`
}

type Command struct {
	ID            string          `json:"id"`
	OrgID         string          `json:"-"`
	DeviceID      string          `json:"device_id"`
	DeviceName    string          `json:"device_name"`
	Type          string          `json:"type"`
	Payload       json.RawMessage `json:"payload"`
	Status        string          `json:"status"`
	ExitCode      *int            `json:"exit_code"`
	Stdout        string          `json:"stdout"`
	Stderr        string          `json:"stderr"`
	Error         *string         `json:"error"`
	IssuedByEmail *string         `json:"issued_by_email"`
	Source        string          `json:"source"`
	CreatedAt     time.Time       `json:"created_at"`
	SentAt        *time.Time      `json:"sent_at"`
	CompletedAt   *time.Time      `json:"completed_at"`
}

type Script struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	Shell          string    `json:"shell"`
	Body           string    `json:"body"`
	TimeoutSeconds int       `json:"timeout_seconds"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Condition struct {
	Field string          `json:"field"`
	Op    string          `json:"op"`
	Value json.RawMessage `json:"value"`
}

type CommandSpec struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type RuleActions struct {
	Alert   bool         `json:"alert"`
	Webhook bool         `json:"webhook"`
	Tag     *string      `json:"tag"`
	Command *CommandSpec `json:"command"`
}

type Rule struct {
	ID               string      `json:"id"`
	OrgID            string      `json:"-"`
	Name             string      `json:"name"`
	Description      string      `json:"description"`
	Enabled          bool        `json:"enabled"`
	Severity         string      `json:"severity"`
	OSFamilies       []string    `json:"os_families"`
	GroupID          *string     `json:"group_id"`
	Match            string      `json:"match"`
	Conditions       []Condition `json:"conditions"`
	Actions          RuleActions `json:"actions"`
	CreatedAt        time.Time   `json:"created_at"`
	UpdatedAt        time.Time   `json:"updated_at"`
	ViolatingDevices int         `json:"violating_devices"`
}

type ComplianceResult struct {
	RuleID      string    `json:"rule_id"`
	RuleName    string    `json:"rule_name"`
	Severity    string    `json:"severity"`
	Passed      bool      `json:"passed"`
	Details     string    `json:"details"`
	EvaluatedAt time.Time `json:"evaluated_at"`
}

type TaskTarget struct {
	GroupID   *string  `json:"group_id"`
	DeviceIDs []string `json:"device_ids"`
	All       bool     `json:"all"`
}

type TaskAction struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type Task struct {
	ID        string     `json:"id"`
	OrgID     string     `json:"-"`
	Name      string     `json:"name"`
	Enabled   bool       `json:"enabled"`
	Cron      string     `json:"cron"`
	Target    TaskTarget `json:"target"`
	Action    TaskAction `json:"action"`
	LastRunAt *time.Time `json:"last_run_at"`
	NextRunAt *time.Time `json:"next_run_at"`
	CreatedAt time.Time  `json:"created_at"`
}

type Alert struct {
	ID         string     `json:"id"`
	OrgID      string     `json:"-"`
	DeviceID   *string    `json:"device_id"`
	DeviceName *string    `json:"device_name"`
	RuleID     *string    `json:"rule_id"`
	Severity   string     `json:"severity"`
	Title      string     `json:"title"`
	Message    string     `json:"message"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	ResolvedAt *time.Time `json:"resolved_at"`
}

type Webhook struct {
	ID             string     `json:"id"`
	OrgID          string     `json:"-"`
	Name           string     `json:"name"`
	URL            string     `json:"url"`
	Secret         string     `json:"secret"`
	Events         []string   `json:"events"`
	Enabled        bool       `json:"enabled"`
	CreatedAt      time.Time  `json:"created_at"`
	LastStatus     *int       `json:"last_status"`
	LastDeliveryAt *time.Time `json:"last_delivery_at"`
}

type AuditEntry struct {
	ID         int64           `json:"id"`
	OrgID      string          `json:"-"`
	TS         time.Time       `json:"ts"`
	ActorType  string          `json:"actor_type"`
	ActorID    *string         `json:"actor_id"`
	ActorEmail *string         `json:"actor_email"`
	Action     string          `json:"action"`
	TargetType *string         `json:"target_type"`
	TargetID   *string         `json:"target_id"`
	Details    json.RawMessage `json:"details"`
	IP         *string         `json:"ip"`
	PrevHash   string          `json:"-"`
	Hash       string          `json:"hash"`
}

type OSCount struct {
	OSFamily string `json:"os_family"`
	Count    int    `json:"count"`
}

type Dashboard struct {
	DevicesTotal   int       `json:"devices_total"`
	Online         int       `json:"online"`
	Offline        int       `json:"offline"`
	Compliant      int       `json:"compliant"`
	NonCompliant   int       `json:"non_compliant"`
	Unknown        int       `json:"unknown"`
	OpenAlerts     int       `json:"open_alerts"`
	ByOS           []OSCount `json:"by_os"`
	RecentAlerts   []Alert   `json:"recent_alerts"`
	RecentCommands []Command `json:"recent_commands"`
}
