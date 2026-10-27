# UEM REST API contract (v1)

All endpoints are JSON under `/api`. The browser authenticates with an httpOnly cookie, `uem_session`. Every mutating request (POST/PATCH/PUT/DELETE) must send the header `X-CSRF-Token` with the same value as the readable cookie `uem_csrf`. The server sets that cookie on login/signup.

Errors are returned as `{"error": "message"}` with an appropriate 4xx/5xx status. A 401 means the user is not logged in, and a 403 means they lack the permission.
List endpoints return `{"items": [...]}`. Timestamps are RFC3339 strings, IDs are UUID strings, and a nullable field is `null` when unset.

## Permissions
`devices.read, devices.write, commands.run, scripts.write, compliance.write, tasks.write, enrollment.write, users.write, webhooks.write, alerts.write, audit.read, settings.write`

| role     | permissions |
|----------|-------------|
| owner    | all |
| admin    | all except `settings.write` |
| helpdesk | devices.read, commands.run, alerts.write |
| auditor  | devices.read, audit.read |

Every authenticated user can perform the read (GET) endpoints below, except the audit endpoints, which need `audit.read`. `GET /api/users` and `GET /api/webhooks` need `users.write` and `webhooks.write` respectively.

## Auth
- `POST /api/auth/signup` `{org_name, name, email, password}` → `Me`. This creates a new organization with the caller as its owner.
- `POST /api/auth/login` `{email, password}` → `Me`
- `POST /api/auth/logout` → `{ok:true}`
- `GET /api/auth/me` → `Me`

```ts
type Me = { user: User; org: Org; permissions: string[] }
type User = { id; email; name; role: "owner"|"admin"|"helpdesk"|"auditor"; created_at; last_login_at: string|null }
type Org = { id; name; server_url: string; created_at }
```

## Dashboard
`GET /api/dashboard` →
```ts
{ devices_total, online, offline, compliant, non_compliant, unknown, open_alerts: number,
  by_os: {os_family: string, count: number}[],
  recent_alerts: Alert[], recent_commands: Command[] }
```

## Devices
```ts
type Device = {
  id; name; hostname; os_family: "windows"|"macos"|"linux"|string; os_name; os_version; os_build; arch;
  serial; model; manufacturer; agent_version; ip_address; mac_address; logged_in_user;
  status: "online"|"offline"; last_seen_at: string|null; enrolled_at;
  compliance_state: "compliant"|"non_compliant"|"unknown";
  group_id: string|null; group_name: string|null; tags: string[];
  cpu_pct: number|null; mem_pct: number|null; disk_pct: number|null; battery_pct: number|null  // latest telemetry
}
type DeviceDetail = Device & { inventory: Inventory|null }
type Inventory = {
  collected_at: string;
  os: { family, name, version, build, kernel, arch, uptime_seconds: number, boot_time: string };
  hardware: { manufacturer, model, serial, cpu_model, cpu_cores: number, cpu_threads: number,
              memory_total_bytes: number, memory_used_bytes: number,
              disks: {mount, fstype, total_bytes, used_bytes, free_bytes}[],
              battery: {present: boolean, percent: number, state: string, health_pct: number|null} };
  network: { hostname, primary_ip, primary_mac, wifi_ssid: string|null,
             interfaces: {name, mac, addrs: string[], up: boolean}[] };
  security: { disk_encrypted: boolean|null, encryption_method: string|null, firewall_enabled: boolean|null,
              screen_lock_enabled: boolean|null, pending_updates: number|null, antivirus: string|null };
  users: { logged_in: string[] };
}
```
- `GET /api/devices?q=&status=&os=&compliance=&group_id=` → `{items: Device[]}`
- `GET /api/devices/{id}` → `DeviceDetail`
- `PATCH /api/devices/{id}` `{name?, group_id?, tags?}` → `Device` (devices.write)
- `DELETE /api/devices/{id}` → `{ok:true}` (devices.write). This retires the device.
- `GET /api/devices/{id}/software` → `{items: {name, version, publisher, source, install_date: string|null}[]}`
- `GET /api/devices/{id}/telemetry?hours=24` → `{items: {ts, cpu_pct, mem_pct, disk_pct, battery_pct: number|null}[]}`
- `GET /api/devices/{id}/compliance` → `{items: ComplianceResult[]}`
- `GET /api/devices/{id}/commands` → `{items: Command[]}`
- `POST /api/devices/{id}/commands` `{type, payload}` → `Command` (commands.run)
- `POST /api/commands/bulk` `{device_ids: string[], type, payload}` → `{items: Command[]}` (commands.run)
- `GET /api/commands/{id}` → `Command`

```ts
type CommandType = "run_script"|"reboot"|"shutdown"|"lock"|"display_message"|"refresh_inventory"|"uninstall_agent"
// payloads:
//   run_script: {script_id} OR {shell: "bash"|"sh"|"zsh"|"powershell"|"cmd", script: string, timeout_seconds?: number}
//   reboot/shutdown: {delay_seconds?: number}
//   display_message: {title: string, message: string}
//   others: {}
type Command = { id; device_id; device_name; type: CommandType; payload: object;
  status: "queued"|"sent"|"running"|"succeeded"|"failed"|"timeout"|"cancelled";
  exit_code: number|null; stdout: string; stderr: string; error: string|null;
  issued_by_email: string|null;  // null when issued by system (scheduler/compliance)
  source: "user"|"scheduler"|"compliance";
  created_at; sent_at: string|null; completed_at: string|null }
```

## Groups
`Group = {id, name, description, blocked_apps: string[], device_count: number, created_at}`
- `GET /api/groups`, `POST /api/groups` `{name, description, blocked_apps}`, `PATCH /api/groups/{id}`, `DELETE /api/groups/{id}` (devices.write). `blocked_apps` holds process names that the agent terminates.

## Enrollment
`EnrollmentToken = {id, token, description, group_id: string|null, group_name: string|null, expires_at: string|null, max_uses: number|null, uses: number, revoked: boolean, created_at}`
- `GET /api/enrollment-tokens`, `POST /api/enrollment-tokens` `{description, group_id?, expires_in_hours?, max_uses?}`, `DELETE /api/enrollment-tokens/{id}` (revoke) (enrollment.write)
- `GET /api/enrollment-tokens/{id}/install` → `{linux: string, macos: string, windows: string}`. These are ready-to-paste one-line install commands.

## Scripts
`Script = {id, name, description, shell, body, timeout_seconds, created_at, updated_at}`
- `GET/POST /api/scripts`, `GET/PATCH/DELETE /api/scripts/{id}` (scripts.write)

## Compliance
```ts
type Condition = { field: string; op: "eq"|"neq"|"gt"|"gte"|"lt"|"lte"|"version_gte"|"version_lt"|"contains"|"not_contains"|"is_true"|"is_false"; value: any }
type Rule = { id; name; description; enabled: boolean; severity: "low"|"medium"|"high"|"critical";
  os_families: string[];            // empty = all
  group_id: string|null;            // null = all groups
  match: "all"|"any"; conditions: Condition[];
  actions: { alert: boolean; webhook: boolean; tag: string|null; command: {type, payload}|null };
  created_at; updated_at; violating_devices: number }
type ComplianceResult = { rule_id; rule_name; severity; passed: boolean; details: string; evaluated_at }
type Field = { field: string; label: string; type: "bool"|"number"|"string"|"version"|"list"; ops: string[]; unit?: string }
```
- `GET /api/compliance/fields` → `{items: Field[]}`
- `GET/POST /api/compliance/rules`, `GET/PATCH/DELETE /api/compliance/rules/{id}` (compliance.write)
- `POST /api/compliance/evaluate` → `{evaluated: number}`. This re-evaluates every device (compliance.write).

## Scheduled tasks
`Task = {id, name, enabled, cron: string, target: {group_id: string|null, device_ids: string[], all: boolean}, action: {type: CommandType|"compliance_check", payload}, last_run_at: string|null, next_run_at: string|null, created_at}`
- `GET/POST /api/tasks`, `PATCH/DELETE /api/tasks/{id}`, `POST /api/tasks/{id}/run` (tasks.write). Cron uses the standard 5-field format in UTC.

## Alerts
`Alert = {id, device_id: string|null, device_name: string|null, rule_id: string|null, severity, title, message, status: "open"|"resolved", created_at, resolved_at: string|null}`
- `GET /api/alerts?status=open|resolved` → `{items}`
- `POST /api/alerts/{id}/resolve` (alerts.write)

## Audit log (audit.read)
`AuditEntry = {id: number, ts, actor_type: "user"|"system"|"device", actor_id: string|null, actor_email: string|null, action: string, target_type: string|null, target_id: string|null, details: object, ip: string|null, hash: string}`
- `GET /api/audit?action=&actor=&target_id=&limit=100&before_id=` → `{items, next_before_id: number|null}`
- `GET /api/audit/export.csv` → CSV download
- `GET /api/audit/verify` → `{valid: boolean, checked: number, broken_at_id: number|null}`

## Users (users.write)
- `GET /api/users`, `POST /api/users` `{email, name, role, password}`, `PATCH /api/users/{id}` `{name?, role?, password?}`, `DELETE /api/users/{id}`

## Webhooks (webhooks.write)
`Webhook = {id, name, url, secret, events: string[], enabled, created_at, last_status: number|null, last_delivery_at: string|null}`. The events are `alert.created`, `device.enrolled`, `device.offline`, `command.completed`, `audit.*`. Payloads are POSTed as JSON and signed with the header `X-UEM-Signature: sha256=<hex hmac of body with secret>`.
- `GET/POST /api/webhooks`, `PATCH/DELETE /api/webhooks/{id}`, `POST /api/webhooks/{id}/test`

## Org settings
- `GET /api/org` → `Org`; `PATCH /api/org` `{name?, server_url?}` (settings.write)

## Live updates
`GET /api/live` is a WebSocket on the same origin that authenticates with the session cookie. The server pushes events as `{type, data}` messages:
- `device.updated` → `Device`. This is sent on status changes, new telemetry or inventory, and enrollment.
- `command.updated` → `Command`
- `alert.created` → `Alert`

## Agent endpoints (used only by the agent)
- `POST /api/agent/enroll` `{token, facts: {hostname, os_family, os_name, os_version, arch, serial, agent_version}}` → `{device_id, device_secret}`
- `GET /api/agent/ws` with the header `Authorization: Device <device_id>:<secret>`. Message types are defined in `internal/protocol`.
- `GET /downloads/uem-agent-<os>-<arch>[.exe]`, `GET /install.sh`, `GET /install.ps1`
