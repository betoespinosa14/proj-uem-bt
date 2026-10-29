# Sentinel UEM

A self-hostable, multi-tenant **Unified Endpoint Management** platform modeled on the capabilities of Scalefusion. It has three parts:

- **Web console** (Next.js) to monitor and manage the fleet.
- **Control-plane API** (Go + PostgreSQL) with live updates over WebSockets.
- **Lightweight agent** (a single static Go binary) for **Windows, macOS and Linux**.

Milestone 1 (this repository) delivers enrollment, inventory, telemetry, remote commands and scripts, a compliance engine, scheduled tasks, an app blocklist, alerts, webhooks (SIEM/ITSM), RBAC and a tamper-evident audit log.

```
 ┌──────────────┐  HTTPS   ┌──────────┐  /api, /downloads   ┌──────────────────────┐    ┌────────────┐
 │  Browser     │─────────▶│  Caddy   │────────────────────▶│  uem-server (Go)     │───▶│ PostgreSQL │
 │  (console)   │◀─ WS ────│  (TLS)   │──────▶ Next.js web  │  REST · live WS ·    │    └────────────┘
 └──────────────┘          └──────────┘                     │  agent gateway ·     │
                                ▲                           │  scheduler · audit   │──▶ webhooks
 ┌──────────────┐  WSS (device secret)                      └──────────────────────┘    (SIEM/ITSM)
 │  uem-agent   │───────────────┘
 │ win/mac/linux│  heartbeat 30s · telemetry 60s · inventory 6h · commands ⇄ results
 └──────────────┘
```

## Quick start (Docker)

```sh
cp deploy/.env.example deploy/.env      # set POSTGRES_PASSWORD, UEM_PUBLIC_URL
make up                                  # docker compose: postgres, server, web, caddy
open http://localhost                    # sign up → creates your organization
```

Enroll a device:

1. In the console, open **Enrollment**, create a token, and copy the install command for the OS.
2. Run the command on the endpoint with administrator rights, for example:
   ```sh
   curl -fsSL https://uem.example.com/install.sh | sudo sh -s -- --server https://uem.example.com --token enr_…
   ```
   On Windows, use the PowerShell one-liner from the console instead.
3. The installer downloads the agent, enrolls it, and installs it as a system service (systemd, launchd or a Windows service). The device appears online within seconds.

For production, set `UEM_SITE_ADDRESS=uem.example.com`, `UEM_PUBLIC_URL=https://uem.example.com` and `UEM_SECURE_COOKIES=true`. Caddy then obtains a TLS certificate automatically. After creating your organization, set `UEM_ALLOW_SIGNUP=false`.

## Development

Requirements: Go 1.26+, Node 22+, PostgreSQL 16.

```sh
# API server (migrations run automatically on start)
DATABASE_URL=postgres://uem:uem@localhost:5432/uem?sslmode=disable UEM_AGENT_DIST=dist go run ./cmd/server

# Web console on :3000; /api is proxied to :8080. `next dev` can't proxy
# WebSockets, so point the live feed straight at the server and allow the origin:
#   server: UEM_ALLOWED_ORIGINS=localhost:3000
cd web && npm install && NEXT_PUBLIC_LIVE_URL=ws://localhost:8080/api/live npm run dev

# Agent in the foreground against the local server
make build-agents
go run ./cmd/agent enroll --server http://localhost:8080 --token enr_… --config ./agent.json
go run ./cmd/agent run --config ./agent.json
go run ./cmd/agent inventory          # print what the agent collects, as JSON
```

Tests:

```sh
make test     # go vet, then unit and integration tests with -race (integration tests need Postgres; set TEST_DB=…)
```

The integration tests (`internal/server/api/api_test.go`) start the real HTTP server against Postgres. They cover:
- cross-tenant isolation on every resource type;
- RBAC and CSRF enforcement;
- enrollment-token limits and revocation;
- a full agent WebSocket round-trip: enroll → inventory → compliance violation → alert → command → result;
- audit-chain verification, plus the database-level block on editing audit rows.

### Layout

| Path | What |
|---|---|
| `cmd/server`, `cmd/agent` | Entry points |
| `internal/protocol` | Agent ⇄ server wire types (JSON over WebSocket) |
| `internal/server/api` | REST handlers, auth/CSRF/RBAC middleware, agent gateway, live WS |
| `internal/server/core` | Command dispatch, result ingestion, compliance evaluation, broadcasting |
| `internal/server/compliance` | Rule field registry and pure evaluator |
| `internal/server/audit` | Hash-chained audit writer and verifier |
| `internal/server/store` | PostgreSQL queries (every tenant query takes `orgID`) |
| `internal/server/migrations` | SQL schema (goose, embedded) |
| `internal/server/scheduler` | Cron tasks, offline detection, command timeouts, retention |
| `internal/server/notify` | Signed outbound webhooks |
| `internal/agent/collect` | Inventory collectors (`*_linux.go`, `*_darwin.go`, `*_windows.go`) |
| `internal/agent/exec` | Script runner and device actions |
| `internal/agent/conn` | Connection loop, command execution, blocklist enforcement |
| `web/` | Next.js console (App Router, TanStack Query, live WebSocket cache sync) |
| `deploy/` | Dockerfile, docker-compose, Caddyfile |
| `docs/API.md` | REST/WebSocket API contract |

## Features (Milestone 1)

| Area | What's included |
|---|---|
| **Enrollment** | Expiring and limited-use tokens, assignable to a group. One-line installers for Linux, macOS and Windows. Each device gets its own 256-bit credential; only its hash is stored. |
| **Inventory** | OS, version and build, kernel, CPU, RAM, disks, battery health, serial, model, network interfaces, IP and MAC, Wi-Fi SSID, logged-in users. Installed software: dpkg, rpm and snap on Linux; the application list on macOS; the Uninstall registry keys on Windows. |
| **Security posture** | Disk encryption (BitLocker, FileVault, LUKS), firewall, antivirus (Windows Security Center), screen-lock policy, and pending OS updates. |
| **Telemetry** | CPU, memory, disk and battery every 60 s, kept for 30 days and charted in the console. |
| **Remote commands** | Run scripts (bash, sh, zsh, PowerShell, cmd) with timeouts, exit codes and captured output. Also reboot, shutdown, lock, show a message, refresh inventory and uninstall the agent. Commands queue while a device is offline and are delivered when it reconnects, and redelivery never re-runs a command. Bulk actions are supported. |
| **Software insights** | Installed apps are labelled System or Third-party. Gambling apps and games are flagged from a built-in catalog, and a ready-made compliance rule can raise alerts for them. |
| **App usage tracking** | Opt-in per organization, for license and subscription audits. Records **active time** (the app is in front and the user is typing or using the mouse, within a configurable idle threshold of 1–15 minutes) and **open time** (the app is running), per user and app. Shown per device (Usage tab) and fleet-wide (App usage page), totalled per app across users or broken down by user, with how many devices each app is used on versus installed on. Kept for 400 days. |
| **Reports** | CSV exports of the device list, a device's installed software, a device's app usage, and fleet app usage. Each one respects the current filters and is audit-logged. |
| **Script library** | Reusable scripts. A command records the exact script body that ran. |
| **Compliance engine** | Rules built from 17 fields (encryption, firewall, OS/agent version, check-in age, free disk, RAM, battery health, SSID, installed software, gambling apps, games, tags) combined with all/any. Rules can be scoped by OS and group. When a check can't be verified, it counts as failed. |
| **Violation actions** | Alert (deduplicated, auto-resolves when the device passes again), webhook, tag the device, or run a remediation command. |
| **Scheduled tasks** | Cron-scheduled scripts, commands or compliance checks targeting all devices, a group, or chosen devices. |
| **App blocklist** | Per-group list of process names that the agent terminates. Each termination is audit-logged. |
| **RBAC** | Owner, Admin, Helpdesk and Auditor roles (see `docs/API.md`). Only owners can grant or modify the owner role, and the last owner is protected. |
| **Audit log** | Every admin action, device command and result, compliance change and agent event. The log is append-only (enforced by a database trigger) and hash-chained per organization. It can be verified from the console and exported to CSV (with spreadsheet-formula escaping). |
| **Integrations** | Signed (HMAC-SHA256) webhooks for `alert.created`, `compliance.violation`, `device.enrolled`, `device.offline`, `command.completed` and `audit.*`. These cover Splunk, Sentinel, Datadog, Jira and similar tools via their HTTP intake. |
| **Live console** | A WebSocket pushes device status, telemetry, command progress and alerts. |

### Security notes

- Sessions are opaque random tokens; only their SHA-256 hash is stored, which makes them revocable. Cookies are httpOnly and SameSite=Strict, CSRF uses a double-submit token, and passwords are hashed with argon2id.
- Logins are rate-limited per IP and email. The login response takes the same time whether or not the account exists.
- Webhooks refuse loopback, private and link-local destinations, which blocks SSRF to cloud metadata endpoints. Set `UEM_WEBHOOK_ALLOW_PRIVATE=true` for on-premises receivers.
- The agent config containing the device secret is mode `0600` on Linux and macOS. On Windows the installer restricts it to SYSTEM and Administrators.
- App usage tracking monitors employee activity, so it is **off by default**. Turning it on or off is audit-logged. Inform staff before enabling it.
- Remote **wipe is intentionally not implemented** until an approval workflow exists (see the roadmap).

## Known limitations

- The Windows and macOS collectors are cross-compiled and reviewed, but in this milestone they have only been *executed* on Linux. Validate them on real Windows and macOS hosts before relying on them.
- Some posture data can't be read by a system service: screen-lock state on macOS and Linux, and Wi-Fi SSID on recent macOS (redacted). These fields report unknown.
- Login rate limits and the live-update hub are in-memory, so run a single server replica (or add a shared store) for now.
- Active time comes from a small probe (`uem-agent session-probe`) that the agent starts in each interactive session: the console user on macOS, and the console and Remote Desktop sessions on Windows. It reads only the front app and the time since the last input. It is not available on Linux.
- Open time counts the time an app is *running*. On macOS only an app's main process counts; widgets and background helpers don't. Menu-bar and login-item apps still count while they run, so active time is the better measure of real use.
- To upgrade an agent, re-run its install command. An enrolled device keeps its identity. Agents don't update themselves yet.
- **Lock** on macOS sleeps the display, which locks only if "require password after sleep" is enabled. On Windows it disconnects the console session.

## Roadmap

- **Phase 2:** OIDC SSO (Entra ID, Okta, Google Workspace), MFA, PDF reports and scheduled email reports, native Jira and Freshservice connectors.
- **Phase 3:** OS and third-party patch management, and software deployment (.msi, .pkg, .deb). Also BitLocker and FileVault key escrow.
- **Phase 4:** Android Enterprise (Android Management API), kiosk mode, geofencing.
- **Phase 5:** Apple MDM (APNs, ABM/ADE, VPP, DDM), native Windows MDM (OMA-DM), SCEP/PKI.
- **Phase 6:** Remote screen view and control (WebRTC), and remote wipe with an approval workflow.
