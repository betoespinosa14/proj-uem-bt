---
title: Limitations and roadmap
section: Start here
order: 50
summary: What BEACON can't do today, the weak spots visible in the code, and the planned phases.
reviewed: 2026-11-20
---

## Why read this

So you can answer "can BEACON do X?" honestly, and know which weak spots are by design and which are just not built yet. For day-to-day open issues see [Known gaps](known-gaps.md).

## Known limitations (from the README)

| Area | Limitation |
|---|---|
| Tested platforms | Collectors are used in production on Windows 10/11, macOS and Ubuntu. Other Linux distributions and older Windows or macOS versions are untested. |
| Posture data | A system service can't read some settings: screen-lock state on macOS and Linux, and Wi-Fi SSID on recent macOS (redacted). These show as unknown. |
| Single server | Login rate limits and the live-update hub are in memory, so only one server process may run. Two replicas would split live updates and rate limits. |
| Active time | Comes from the session probe on macOS (console user) and Windows (console and Remote Desktop sessions). Not available on Linux, where open time counts as use instead. |
| Open time | Counts time an app is running. On macOS only the main process counts; widgets and helpers don't. Menu-bar and login-item apps still count while they run, so active time is the better measure. |
| Agent updates | Agents update when you click **Update agents** or Update on one device. Only tagged releases are offered. If an update fails, retry it or re-run the install command (the device keeps its identity). |
| Lock | On macOS, Lock sleeps the display (`pmset displaysleepnow`); it only locks if "require password after sleep" is on. On Windows it disconnects the console session (`tsdiscon`). On Linux it runs `loginctl lock-sessions`. |
| Remote wipe | Intentionally not built until an approval workflow exists. |

## Weak spots visible in the code

These are not bugs, but you should know about them.

| Weak spot | Detail | Code |
|---|---|---|
| One machine | Everything (database, server, console, Caddy, backups) runs on one Mac Mini. If it is off, no device can report and nobody can sign in. Agents reconnect on their own afterwards. | `deploy/macos/` |
| Backups on the same disk | Nightly dumps are kept on the Mac Mini for 14 days. Without a manual copy elsewhere, a disk failure loses both. | `deploy/macos/backup.sh` |
| Webhook delivery | Deliveries go through an in-memory queue of 1024 with 4 workers. Each is tried 3 times (waits of 1, 2, 4 seconds). If the queue is full, or the server restarts, pending deliveries are lost. Only the last status is stored. | `internal/server/notify/webhooks.go` |
| Private webhook targets | Webhooks to private, loopback, link-local or CGNAT addresses are refused unless `UEM_WEBHOOK_ALLOW_PRIVATE=true`. | `notify.isPublic` |
| Lost commands | A command still running after 75 minutes is marked timed out. | `scheduler.expireCommands` |
| Agent output | Script stdout and stderr are capped at 1 MiB each. | `protocol.MaxOutputBytes` |
| Cron in UTC | Scheduled tasks use 5-field cron in UTC, not office time. | `internal/server/scheduler/scheduler.go` |
| MFA secret | The authenticator secret is stored in the database (recovery codes are hashed). Protect the database and its backups. | `00007_mfa.sql` |
| Machine config outside git | The Mac Mini's `Caddyfile` and `server.env` are created once and not tracked in the repository. | `deploy/macos/install.sh` |
| Names say "UEM" | The agent service is still called `uem-agent` / "UEM Agent", and webhook headers are `X-UEM-*`. Renaming them would break installed agents and receivers. | `cmd/agent/main.go`, `notify.post` |

**To fill in:** any other limitation the current maintainer knows about from experience.

## Keeping docs current

When a release changes how something works, update the matching article here and its `reviewed:` date in the same commit. The README and `docs/API.md` cover the same ground in less detail; fix them too.

## Roadmap (from the README)

| Phase | Planned |
|---|---|
| Milestone 1 | Done: enrollment, inventory, telemetry, commands and scripts, compliance, scheduled tasks, app blocklist, alerts, webhooks, RBAC, audit log, app usage, two-step sign-in. |
| Phase 2 | Single sign-on with OIDC (Entra ID, Okta, Google Workspace). Requiring two-step sign-in for a whole organization. PDF and scheduled email reports. Native Jira and Freshservice connectors. |
| Phase 3 | OS and third-party patch management. Software deployment (.msi, .pkg, .deb). BitLocker and FileVault key escrow. |
| Phase 4 | Android Enterprise, kiosk mode, geofencing. |
| Phase 5 | Apple MDM (APNs, ABM/ADE, VPP, DDM), native Windows MDM (OMA-DM), SCEP/PKI. |
| Phase 6 | Remote screen view and control (WebRTC). Remote wipe with an approval workflow. |

**To fill in:** which roadmap items management has actually approved, and their priority.

## Hosting plans

The office setup reaches the server through port forwarding to the Mac Mini. Moving to a cloud host is possible with the Docker files in `deploy/` (`docker-compose.yml`, `Dockerfile.server`, `web/Dockerfile`, `Caddyfile`). Any move must keep a company DNS name, because agents follow the **Public server URL** to a new address. See [Network and DNS](network-dns.md) and [Fresh install](fresh-install.md).

**To fill in:** whether a cloud move is planned, and who decides.
