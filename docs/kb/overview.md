---
title: What BEACON is
section: Start here
order: 10
summary: BEACON is the company's own device management system; this page explains what it does, who uses it and where it runs.
reviewed: 2026-11-20
---

## In one paragraph

BEACON (Business Endpoint Authorization, Configuration and Oversight Network, formerly "Sentinel UEM") is a self-hosted Unified Endpoint Management (UEM) system. A small agent runs on every company computer (Windows, macOS and Linux). It reports inventory, health and security status to a central server, and runs commands the IT team sends from a web console. Everything is built in this repository: there is no vendor to call.

## The three parts

| Part | What it is | Code |
|---|---|---|
| Web console | The website IT staff use. Next.js app. | `web/` |
| Server | The API, live updates, agent gateway, scheduler and audit log. Go program `uem-server`, data in PostgreSQL. | `cmd/server`, `internal/server` |
| Agent | One Go binary, `uem-agent`, installed as a system service on each device. | `cmd/agent`, `internal/agent` |

See [Architecture](architecture.md) for how they fit together.

## Where it runs

- Public address: **https://beacon.itsthev.net**. Devices and browsers both use it.
- The server runs on a **Mac Mini in the office**, as launchd daemons (`com.beacon.postgres`, `com.beacon.server`, `com.beacon.web`, `com.beacon.caddy`, plus the nightly `com.beacon.backup`). See [Production server](production-server.md).
- Caddy on the Mac Mini terminates HTTPS and splits traffic between the server and the console.
- Agents only make outgoing connections, so laptops outside the office keep reporting as long as the address is reachable from the internet. See [Network and DNS](network-dns.md).

**To fill in:** the Mac Mini's exact location in the office and its asset tag.

## Who uses it

| Role in BEACON | Typical person | What they can do |
|---|---|---|
| Owner | IT manager | Everything, including organization settings |
| Admin | IT staff | Everything except organization settings |
| Helpdesk | Support staff | See devices, run commands, resolve alerts |
| Auditor | Compliance / management | See devices and the audit log, read-only |

The exact permissions are in `internal/server/auth/auth.go` (`rolePerms`). See [Roles and permissions](roles.md).

**To fill in:** the departments and people who currently have BEACON accounts.

## The modules

The left-hand menu is defined in `web/components/app-shell.tsx` (`NAV`). Each page lives in `web/app/(console)/<name>/page.tsx`.

| Module | Path | What it does |
|---|---|---|
| Dashboard | `/` | Fleet health at a glance: headline figures with 30-day trends, Needs attention, device health, security posture, OS mix, recent activity. Filter by department (group). |
| Devices | `/devices` | List of every enrolled computer with status, compliance and health. Each device page has inventory, software, telemetry charts, compliance, commands and app usage. Bulk actions and agent updates start here. |
| Enrollment | `/enrollment` | Create enrollment tokens and copy the one-line install command for Linux, macOS or Windows. |
| Groups | `/groups` | Organize devices (used as departments). A group also carries policy such as the app blocklist. Device names are generated as "group \| employee name". |
| App usage | `/usage` | Opt-in tracking of how much each installed app is actually used, for license reviews. Off by default. |
| Compliance | `/compliance` | Rules (for example "disk is encrypted", "agent version at least X") checked on every inventory report. Failing devices can raise alerts, send webhooks, get a tag or run a fix script. |
| Scripts | `/scripts` | Library of reusable bash, sh, zsh, PowerShell and cmd scripts to run on devices. |
| Scheduled tasks | `/tasks` | Cron schedules that run a script, a command or a compliance check on all devices, a group or chosen devices. |
| Alerts | `/alerts` | Alerts raised by compliance rules. They close by themselves when the device passes again. |
| Audit log | `/audit` | Append-only, hash-chained record of every admin action, command and compliance change. Can be verified and exported. |
| Users & roles | `/users` | Invite people, set their role, reset their two-step sign-in. |
| Integrations | `/integrations` | Signed webhooks to SIEM/ITSM tools, or Microsoft Teams cards. |
| Settings | `/settings` | Organization name, Public server URL (moves agents to a new address), app usage tracking switch and idle threshold. Only owners can change them. |
| Knowledge base | `/knowledge` | These articles. Owners and admins only (`kb.read`). |

Each person's own two-step sign-in is under **Account security** (`/account`).

## What it does not do (yet)

No remote wipe, no software deployment or patching, no Apple MDM or Android management, no single sign-on. See [Limitations and roadmap](limitations-roadmap.md).

## Where to go next

Start with these, in order:

1. [Handover checklist](handover-checklist.md) — what to receive and check in week one.
2. [Contacts](contacts.md) — people and vendors.
3. [Glossary](glossary.md) — the words used across BEACON.
4. [Architecture](architecture.md) and [Tech stack](tech-stack.md).
5. [Production server](production-server.md) and [Backups and restore](backups.md).
6. [Releases](releases.md) and [Agent updates](agent-updates.md).

When something breaks:

- [Server down](server-down.md)
- [Device offline](device-offline.md)
- [Locked out](locked-out.md)
- [Restore the database](restore-database.md)
- [HTTPS certificates](https-certificates.md)

For code work: [Code layout](code-layout.md), [Development](development.md), [Database](database.md), [Agent](agent.md), [Live updates](live-updates.md).
