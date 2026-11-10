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
type Org = { id; name; server_url: string; usage_tracking: boolean; idle_threshold_sec: number; created_at }
```

## Dashboard
`GET /api/dashboard` →
```ts
{ devices_total, online, offline, compliant, non_compliant, unknown, open_alerts: number,
  by_os: {os_family: string, count: number, online: number}[],
  failing_rules: {rule_id: string, name: string, severity: Severity, devices: number}[],
  recent_alerts: Alert[], recent_commands: Command[] }
```
`failing_rules` lists up to 6 enabled compliance rules that active devices currently fail, most severe first,
then by number of failing devices.

## Devices
```ts
type Device = {
  id; name; hostname; os_family: "windows"|"macos"|"linux"|string; os_name; os_version; os_build; arch;
  serial; model; manufacturer; agent_version; ip_address; mac_address; logged_in_user; employee_name;
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
- `GET /api/devices/export.csv?q=&status=&os=&compliance=&group_id=` → CSV report of the matching devices, including software count and flagged gambling apps/games. Audit-logged as `devices.export`.
- `GET /api/devices/{id}` → `DeviceDetail`
- `PATCH /api/devices/{id}` `{name?, employee_name?, group_id?, tags?}` → `Device` (devices.write)
  - Once a device has an `employee_name`, its name is generated as `<group name> | <employee name>`, or the employee name alone without a group, and `name` is ignored. `name` only applies to devices without one.
  - `employee_name` can't be empty (400) and is at most 128 characters.
  - Renaming or deleting a group regenerates the names of its devices. The device search (`q`) also matches `employee_name`, and the CSV export has an `employee_name` column.
- `DELETE /api/devices/{id}` → `{ok:true}` (devices.write). This retires the device.
- `GET /api/devices/{id}/software` → `{items: {name, version, publisher, source, install_date: string|null, last_used: string|null, category: "system"|"third_party", flag?: "gambling"|"game"}[]}`. `category` and `flag` are computed by the server (`internal/server/compliance/catalog.go`); `last_used` comes from usage tracking.
- `GET /api/devices/{id}/software.csv?q=&category=system|third_party|flagged` → CSV of the device's installed software. Audit-logged as `device.software.export`.
- `GET /api/devices/{id}/usage?days=30|90|365&group=app|app_user` → `{items: UsageRow[]}`, most active first. `group=app` (totals per app across users) leaves `user` empty and sets `users`. The default is `app_user`.
- `GET /api/devices/{id}/usage.csv?days=&group=` → CSV of the same. Audit-logged as `device.usage.export`.
- `GET /api/devices/{id}/telemetry?hours=24` → `{items: {ts, cpu_pct, mem_pct, disk_pct, battery_pct: number|null}[]}`
- `GET /api/devices/{id}/compliance` → `{items: ComplianceResult[]}`
- `GET /api/devices/{id}/commands` → `{items: Command[]}`
- `POST /api/devices/{id}/commands` `{type, payload}` → `Command` (commands.run)
- `POST /api/commands/bulk` `{device_ids: string[], type, payload}` → `{items: Command[]}` (commands.run)
- `GET /api/commands/{id}` → `Command`

```ts
type CommandType = "run_script"|"reboot"|"shutdown"|"lock"|"display_message"|"refresh_inventory"|"uninstall_agent"|"update_agent"
// payloads:
//   run_script: {script_id} OR {shell: "bash"|"sh"|"zsh"|"powershell"|"cmd", script: string, timeout_seconds?: number}
//   reboot/shutdown: {delay_seconds?: number}
//   display_message: {title: string, message: string}
//   update_agent: send {} — the server fills {version, file, sha256, size} from the device's platform build.
//     Agents older than 2026.11.7 get a one-time run_script (shell "sh"/"powershell") that replaces them instead.
//     Refused (400) when the device already runs the latest agent or no build exists for its platform; the bulk
//     endpoint skips such devices and returns 400 only if none needs an update.
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
type Condition = { field: string; op: "eq"|"neq"|"gt"|"gte"|"lt"|"lte"|"version_gte"|"version_lt"|"contains"|"not_contains"|"is_true"|"is_false"|"none_found"|"any_found"; value: any }
type Rule = { id; name; description; enabled: boolean; severity: "low"|"medium"|"high"|"critical";
  os_families: string[];            // empty = all
  group_id: string|null;            // null = all groups
  match: "all"|"any"; conditions: Condition[];
  actions: { alert: boolean; webhook: boolean; tag: string|null; command: {type, payload}|null };
  created_at; updated_at; violating_devices: number }
type ComplianceResult = { rule_id; rule_name; severity; passed: boolean; details: string; evaluated_at }
type Field = { field: string; label: string; type: "bool"|"number"|"string"|"version"|"list"|"flagged"; ops: string[]; unit?: string }
// "flagged" fields (software.gambling, software.games) take no value: none_found passes when no installed
// app matches the built-in catalog, any_found when at least one does. Details list the matching apps.
```
- `GET /api/compliance/fields` → `{items: Field[]}`
- `GET/POST /api/compliance/rules`, `GET/PATCH/DELETE /api/compliance/rules/{id}` (compliance.write)
- `POST /api/compliance/evaluate` → `{evaluated: number}`. This re-evaluates every device (compliance.write).

## Scheduled tasks
`Task = {id, name, enabled, cron: string, target: {group_id: string|null, device_ids: string[], all: boolean}, action: {type: CommandType|"compliance_check", payload}, last_run_at: string|null, next_run_at: string|null, created_at}`
- `GET/POST /api/tasks`, `PATCH/DELETE /api/tasks/{id}`, `POST /api/tasks/{id}/run` (tasks.write). Cron uses the standard 5-field format in UTC.

## App usage
Recorded only while the org's `usage_tracking` is on, per user, per device-local day, and kept for 400 days.
- **Open time** (`minutes`): the agent checks running apps once a minute.
- **Active time** (`active_seconds`): a probe in each user session reports the front app and the input idle time every 5 s. Time counts as active while idle time is below the org's `idle_threshold_sec`, and gaps between samples are capped at 10 s. macOS and Windows only.
- Agents report every 5 minutes.
```ts
type UsageRow = { app: string; user: string; users: number; last_used: string; days_used: number;
  minutes: number; active_seconds: number }
type FleetUsageRow = { device_id; device_name; app: string;
  user: string;               // "" for an installed app with no use in the period
  installed: boolean;         // false if used but no longer in the installed list
  last_used: string|null;     // for unused rows: last use ever recorded (null = never)
  days_used: number; minutes: number; active_seconds: number }
type FleetAppRow = { app: string; users: number;
  devices: number;            // devices it was used on in the period
  installed_on: number;       // devices it is installed on now
  last_used: string|null; minutes: number; active_seconds: number }
```
- `GET /api/usage?app=&days=30|90|365&unused=true&group=app|app_user&group_id=&device_id=` → `{items}` (up to 10,000 rows). `group=app` returns `FleetAppRow[]`, most active first. The default `app_user` returns `FleetUsageRow[]`.
  - With no `app`, only used rows are returned. With `app`, devices where the app is installed but unused are included too.
  - `unused=true` returns only installed third-party apps with no use in the period.
  - `group_id` limits every App usage endpoint (list, export, summary and reclaim list) to devices in that group. A value that isn't a group ID gets a 400.
  - `device_id` limits them all to one device; the device page's Usage tab uses it. A value that isn't a device ID gets a 400.
- `GET /api/usage/export.csv` (same params) → CSV with `active_hours` and `open_hours` columns. Audit-logged as `usage.export`.
- `GET /api/usage/summary?app=&days=30|90|365&group_id=&device_id=` → the App usage dashboard. It uses the same period and app search as `/api/usage`, and only counts devices that aren't retired.
  - Use is split per (device, app, user):
    - **foreground** if anyone was active in it at least once in the period;
    - **background-only** otherwise, such as system daemons, helpers, remote-access hosts and apps under service accounts.
  - Open time, the app lists and the user lists count foreground use only. Background-only use is totalled and listed separately.
  ```
  { totals: { active_seconds, open_minutes, apps_used, active_users, devices,
              background_minutes, background_apps, background_third_party, unused_apps },
    daily: [{ day: "YYYY-MM-DD", active_seconds, minutes, devices }],  // days anyone reported; a missing day = no data
    top_apps: [{ app, active_seconds, minutes, users, devices }],       // top 10 by active time
    idle_apps: [ same shape ],                                          // top 10 by idle time (open minus active)
    top_users: [{ user, employee, active_seconds, open_minutes, apps, devices, device_names }], // top 10 by active time; employee = employee name of the device the account was most active on, or ""
    background: [{ app, device_id, device_name, user, minutes, third_party }],        // top 40 background-only, by open time
    infrastructure: [{ app, active_seconds, minutes, users, devices }] }               // apps marked as infrastructure
  ```
  - `unused_apps` counts the same apps as `unused=true`: installed third-party apps with no use in the period.
  - **Infrastructure apps** are open by design, such as remote-access hosts. Their active time still counts, including in `top_apps` and `top_users`. Their open time is left out of `open_minutes`, `daily.minutes`, `idle_apps` and `background`, and they're listed under `infrastructure`.
  - `third_party` uses the same system vs third-party rule: OS built-ins and distro packages (bash, systemd) are `false`, and vendor software is `true`. `background_third_party` counts the `true` rows.
- `GET /api/usage/infrastructure` → `{items: [{app, created_at}]}`: the apps marked as infrastructure.
- `PUT /api/usage/infrastructure` `{app}` → 204. Marks an app as infrastructure. Case-insensitive; marking it again does nothing. Needs `devices.write`. Audit-logged as `usage.infrastructure.add`.
- `DELETE /api/usage/infrastructure?app=` → 204. Puts the app back in the rankings. Needs `devices.write`. Audit-logged as `usage.infrastructure.remove`.
- `GET /api/usage/reclaim.csv?app=&days=&group_id=&device_id=` → the license-reclamation list: one row per installed third-party app per device with no use in the period.
  - Columns: `app, publisher, version, device, device_user, device_last_seen, last_used, installed_on, period_days`. `last_used` is the most recent use ever, or `never`.
  - Audit-logged as `usage.export` with `kind: reclaim`.
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

## Agent builds
- `GET /api/agents/latest` → `{version: string|null, self_update_since: string}`: the agent version in the server's `dist/manifest.json` (null when none is published).
- When a device reconnects on a different agent version, the audit log records `agent.updated` with `{from, to}`.

## Org settings
- `GET /api/org` → `Org`; `PATCH /api/org` `{name?, server_url?, usage_tracking?, idle_threshold_sec? (60–900)}` (settings.write). Changing `usage_tracking` or `idle_threshold_sec` re-pushes policy to online agents.

## Live updates
`GET /api/live` is a WebSocket on the same origin that authenticates with the session cookie. The server pushes events as `{type, data}` messages:
- `device.updated` → `Device`. This is sent on status changes, new telemetry or inventory, and enrollment.
- `command.updated` → `Command`
- `alert.created` → `Alert`

## Agent endpoints (used only by the agent)
- `POST /api/agent/enroll` `{token, facts: {hostname, os_family, os_name, os_version, arch, serial, agent_version}}` → `{device_id, device_secret}`
- `GET /api/agent/ws` with the header `Authorization: Device <device_id>:<secret>`. Message types are defined in `internal/protocol`. The `policy` message carries `usage_tracking`, `idle_threshold_sec` and `server_url` (the org's Public server URL, else `UEM_PUBLIC_URL`). When `server_url` differs from the agent's address, the agent checks `/healthz`, reconnects there, and saves it only after that connection succeeds; otherwise it keeps its address and retries after 10 minutes. The agent sends `usage` messages (`{entries: {date, app, user, minutes, active_seconds, last_seen}[]}`), which the server drops while tracking is off.
- `GET /downloads/uem-agent-<os>-<arch>[.exe]`, `GET /install.sh`, `GET /install.ps1`
- `GET /downloads/manifest.json` → `{version, files: {<file>: {sha256, size}}}`, written by `make build-agents`. On `update_agent` the agent downloads `<server>/downloads/<file>`, checks size and sha256, runs it once (`version`), swaps it in (renaming the running exe aside on Windows) and exits so its service manager starts the new binary; any failure leaves the current agent in place.
