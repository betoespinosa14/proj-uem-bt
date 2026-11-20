---
title: Glossary
section: Start here
order: 40
summary: Plain definitions of the words used in BEACON's console, code and these articles.
reviewed: 2026-11-20
---

## Core terms

| Term | Meaning | Where in code |
|---|---|---|
| BEACON | The product name. Older code and paths still say "UEM" or "Sentinel" (for example `uem-server`, `uem-agent`, the Go module `github.com/uem/uem`). | `web/lib/config.ts` (`APP_NAME`) |
| UEM | Unified Endpoint Management: managing computers from one central place. | |
| Org (organization) | A tenant. Every user, device and rule belongs to one org, and every database query is filtered by `org_id`. BEACON in the office uses one org. | `organizations` table |
| Agent | The `uem-agent` program installed as a system service on each device. It reports data and runs commands. | `cmd/agent`, `internal/agent` |
| Device | One enrolled computer. It has its own secret credential; the server stores only its hash. | `devices` table |
| Enrollment token | A code starting with `enr_` that lets a new device join. It can expire, have a use limit, be revoked, and put devices into a group. | `enrollment_tokens` table, [Enrollment](enrollment.md) |
| Group | A set of devices, used as a department. It carries policy such as the app blocklist. A device's name is generated as "group \| employee name". | `device_groups` table |
| Employee name | The person a device belongs to, used to build the device name. | `devices.employee_name` |
| Online / Offline | A device is online while its agent is connected. The server marks it offline after 90 seconds without contact. | `internal/server/scheduler/scheduler.go` (`OfflineAfter`) |
| Heartbeat | A small "I'm alive" message the agent sends every 30 seconds. | `internal/agent/conn/agent.go` |
| Telemetry | CPU, memory, disk and battery figures sent every 60 seconds, kept 30 days. | `device_telemetry` table |
| Inventory | The full snapshot of a device: OS, hardware, network, security, users, software. Sent every 6 hours and on demand. | `device_inventory`, `device_software` |
| Policy | Settings the server pushes to an agent: blocklist, intervals, usage tracking, server address, fallback addresses. | `protocol.Policy` in `internal/protocol/protocol.go` |
| Public server URL | The address agents should use. Changing it in Settings moves agents safely to a new address. | `organizations.server_url`, `UEM_PUBLIC_URL` |
| Fallback address | Another address of the same server (for example its IP). Agents use it only while the main address fails. | `UEM_FALLBACK_URLS`, `internal/agent/conn/fallback.go` |

## Roles

| Role | Can do |
|---|---|
| Owner | Everything, including Settings. Only owners can grant or change the owner role. The last owner can't be removed. |
| Admin | Everything except Settings (`settings.write`). |
| Helpdesk | See devices, run commands, resolve alerts. |
| Auditor | See devices and read the audit log. |

Source: `rolePerms` in `internal/server/auth/auth.go`. See [Roles and permissions](roles.md).

## Management features

| Term | Meaning | Where in code |
|---|---|---|
| Command | One action sent to one device: run script, reboot, shutdown, lock, display message, refresh inventory, uninstall agent, update agent. It moves through queued → sent → running → succeeded / failed / timeout / cancelled. | `internal/protocol/protocol.go`, `commands` table |
| Script | A reusable script in the library (bash, sh, zsh, PowerShell, cmd) with a timeout. When run, the exact body is copied into the command. | `scripts` table |
| Compliance rule | A check made of conditions (fields such as encryption, firewall, agent version, free disk) joined by "all" or "any", optionally limited by OS and group, with actions on failure. | `compliance_rules`, `internal/server/compliance` |
| Finding (result) | The outcome of one rule on one device: passed or failed, with details. A check that can't be verified counts as failed. | `compliance_results` table |
| Compliant / Non-compliant / Unknown | A device's overall compliance state. | `devices.compliance_state` |
| Violation action | What happens when a rule fails: alert, webhook, tag the device, or run a fix command. | `core.onViolation` in `internal/server/core/core.go` |
| Rule tag | A tag a rule put on a device; it is removed again when the device passes. Hand-added tags are never removed automatically. | `device_rule_tags` table |
| Scheduled task | A cron schedule (5 fields, UTC) that runs a script, a command or a compliance check on all devices, a group, or chosen devices. | `scheduled_tasks`, `internal/server/scheduler` |
| Alert | A notice raised by a failing rule. Only one open alert per device and rule. It resolves itself when the device passes. | `alerts` table |
| App blocklist | Per-group list of process names the agent kills (checked every 15 seconds). Each kill is audit-logged. | `device_groups.blocked_apps` |
| Webhook | A signed HTTP POST BEACON sends to another tool when an event happens. Format "generic" (JSON) or "teams" (Teams card). | `internal/server/notify`, [Webhook failures](webhook-failures.md) |
| Signature | The `X-UEM-Signature: sha256=<hex>` header: HMAC-SHA256 of the body with the webhook's secret, so the receiver can verify it. | `notify.Sign` |
| Audit log | The record of every action. Rows can't be edited or deleted (database trigger). | `audit_log` table, [Audit log](audit-log.md) |
| Audit chain | Each audit row's hash covers its content and the previous row's hash, per org. Changing any row breaks verification of every later row. | `internal/server/audit` |
| Snapshot | One row per org, department and day with the Dashboard figures, refreshed hourly and kept 400 days, used for trends. | `fleet_snapshots` table |

## App usage terms

App usage is off by default and turned on per org in Settings. The agent samples running apps every minute and reports every 5 minutes.

| Term | Meaning |
|---|---|
| Active | Time the app was in front **and** the user was typing or using the mouse, within the idle threshold (1–15 minutes, default 2). Measured by the session probe. |
| Open | Time the app was running, whether or not anyone used it. |
| Users | People who used the app with keyboard or mouse in the period. Apps that only ran in the background don't count. |
| Not used | Installed third-party apps with no active use in the chosen period. Helper components are left out. |
| Last active | When someone last used the app with keyboard or mouse; "Never" if no one has since tracking began. |
| Linux rule | Linux reports no keyboard or mouse activity, so on Linux any open time counts as use and "last active" is the last time it ran. |
| Idle threshold | How long without input before front-app time stops counting as active. |
| Session probe | A small helper (`uem-agent session-probe`) the agent starts inside each signed-in user session on macOS and Windows. It reports the front app and idle time every 5 seconds. |
| Third-party app | Anything not shipped by the OS vendor or a hardware driver vendor. Decided from the software's source and publisher. |
| System app | Apple-signed apps, Microsoft and driver packages on Windows, distro packages on Linux. |
| Helper component | An installed item that isn't an app people open (updaters, crash reporters, runtimes, redistributables, WebView2…). Left out of "Not used". |
| Infrastructure app | An app an admin marked as open by design (for example a remote-access host), so its open time isn't ranked as idle. |

Sources: `internal/server/store/usage.go` (`activeRow`, `lastActive`), `web/components/usage-explorer.tsx` (column hints), `compliance.IsComponent` and `compliance.Category` in `internal/server/compliance/catalog.go`, `usage_infrastructure_apps` table.

## Release and operations terms

| Term | Meaning |
|---|---|
| Calendar version | `YYYY.MM.N`, where N counts releases within the month from 0 (2026.11.0 is the first of November 2026). Releases are git tags like `v2026.11.0`. |
| Between-release build | Reports e.g. `2026.11.0-3-g60f25a5` (3 commits after the tag), plus `-dirty` with uncommitted changes. |
| Agent version | Agents keep the last release's version while their code hasn't changed, so server-only releases don't show "Update available" everywhere. See `Makefile` (`AGENT_VERSION`). |
| Manifest | `dist/manifest.json`: the agent version and the size and SHA-256 of each agent build. Agents check these before updating. |
| launchd daemon | A macOS background service. BEACON's are `com.beacon.*` in `/Library/LaunchDaemons`. |
| Caddy | The web server in front of BEACON that handles HTTPS. |

See also [Releases](releases.md) and [Agent updates](agent-updates.md).
