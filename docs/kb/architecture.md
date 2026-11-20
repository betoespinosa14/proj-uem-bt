---
title: Architecture
section: Architecture
order: 60
summary: How devices, Caddy, the Go server, the Next.js console and Postgres connect, and where each piece of state lives.
reviewed: 2026-11-20
---

## The big picture

BEACON has one public entry point (Caddy) and three programs behind it: the Go server, the Next.js console and PostgreSQL. Agents on devices and browsers both talk to Caddy over HTTPS.

```text
  Devices (uem-agent)                         Browsers (IT staff)
  Windows / macOS / Linux                     console users
        |                                            |
        | WSS /api/agent/ws  (device secret)         | HTTPS pages + /api/* (session cookie)
        | HTTPS /downloads, /healthz                 | WSS /api/live
        v                                            v
  +-------------------------------------------------------------+
  |  Caddy  (TLS for beacon.itsthev.net, ports 80/443)          |
  |   /api/*  /downloads/*  /install.sh  /install.ps1  /healthz |---+
  |   everything else                                           |-+ |
  +-------------------------------------------------------------+ | |
                                                                  | |
        +---------------------------------------------------------+ |
        v                                                           v
  +---------------------------+     /api rewrite      +---------------------------+
  | Next.js console  :3000    | ------------------->  | uem-server (Go)   :8080   |
  | pages, React, React Query |  (only when Caddy is  | REST API, live hub,       |
  +---------------------------+   not in front)       | agent gateway, scheduler, |
                                                      | webhooks, audit           |
                                                      +-------------+-------------+
                                                                    |  pgx pool
                                                                    v
                                                      +---------------------------+     signed POST
                                                      | PostgreSQL 16  (db "uem") |     ----------> SIEM / ITSM /
                                                      +---------------------------+   (from server)  Teams webhooks
```

Routing is defined in `deploy/Caddyfile`. Paths `/api/*`, `/downloads/*`, `/install.sh`, `/install.ps1` and `/healthz` go to the server; all other paths go to the console. The console also has rewrites for the same paths (`web/next.config.ts`), used in development and whenever the console is reached directly. On the Mac Mini the console listens only on `127.0.0.1:3000` (`deploy/macos/com.beacon.web.plist`).

## The pieces

| Piece | Runs as (Mac Mini) | Listens on | Job |
|---|---|---|---|
| Caddy | `com.beacon.caddy` | 80, 443 | HTTPS certificates, compression, routing |
| Console | `com.beacon.web` (`node server.js`) | 127.0.0.1:3000 | Serves the web pages |
| Server | `com.beacon.server` (`bin/uem-server`) | :8080 (`UEM_LISTEN`) | Everything else |
| Database | `com.beacon.postgres` | 5432 | All persistent data |
| Backup | `com.beacon.backup` | – | Nightly `pg_dump` at 02:30 |

See [Production server](production-server.md) for how they are installed.

## Request flow: a person clicks something

1. The browser loads a page from the console via Caddy.
2. The page calls `/api/...` with relative URLs. Caddy sends those to the server.
3. The server checks the session cookie, the CSRF header (`X-CSRF-Token`) on changes, and the user's role permission (`internal/server/api/server.go`).
4. The handler reads or writes Postgres through `internal/server/store`. Every query includes the org ID, so one org never sees another's data.
5. Important actions are written to the audit log (`internal/server/audit`), which may also trigger `audit.*` webhooks.
6. The browser caches results with React Query and refreshes them when live events arrive.

## Request flow: a command to a device

1. An admin clicks, say, **Run script**. The server validates it, stores a `commands` row with status `queued`, and audits it (`core.IssueCommand` in `internal/server/core/core.go`).
2. If the device's agent is connected, the server pushes it over the agent's WebSocket at once (status `sent`). If not, it waits.
3. The agent replies `running`, does the work, then sends the final result.
4. The server stores the result, publishes `command.updated` to browsers, and fires the `command.completed` webhook.
5. When an offline device reconnects, queued commands are delivered (`core.FlushPending`). The agent never runs the same command ID twice.

## Request flow: a device reports in

1. The agent connects to `wss://<server>/api/agent/ws` with `Authorization: Device <id>:<secret>`.
2. It sends `hello`; the server marks it online, sends its policy, and delivers waiting commands.
3. Heartbeats every 30 s, telemetry every 60 s, inventory every 6 h (intervals from `cmd/server/main.go`, sent in the policy).
4. Each inventory is saved, then every compliance rule is evaluated for that device (`core.EvaluateDevice`). Failures may raise alerts, tags, webhooks or fix commands.

See [Agent](agent.md) and [Live updates](live-updates.md).

## Live updates

The server keeps an in-memory **hub** (`internal/server/hub/hub.go`) with one connection per connected agent and every open console tab per org. When something changes, it publishes `device.updated`, `command.updated` or `alert.created` to that org's tabs over `/api/live`. Because the hub is in memory, only one server process can run. Details in [Live updates](live-updates.md).

## Background jobs (scheduler)

Started in `cmd/server/main.go`, defined in `internal/server/scheduler/scheduler.go`:

| Job | How often | What it does |
|---|---|---|
| Admin scheduled tasks | Their cron (UTC) | Run scripts, commands or compliance checks on targets |
| Mark offline | Every 15 s | Devices silent for 90 s become offline; `device.offline` webhook |
| Expire commands | Every minute | Commands stuck for 75 min become timed out |
| Retention | Every hour | Deletes telemetry over 30 days, app usage over 400 days, snapshots over 400 days, expired sessions and remembered browsers |
| Dashboard snapshot | Hourly, first after 5 min | Saves today's Dashboard figures for trends |

## Webhooks worker

`internal/server/notify/webhooks.go`. When an event happens, matching enabled webhooks are put on an in-memory queue (1024 slots). Four workers send them with an HMAC-SHA256 signature, trying up to 3 times. The last HTTP status is saved on the webhook. Pending deliveries are lost on restart. See [Webhook failures](webhook-failures.md).

## Where state lives

| State | Where | Survives restart? |
|---|---|---|
| Users, devices, inventory, rules, results, commands, alerts, audit log, usage, snapshots | Postgres (data dir `/opt/homebrew/var/postgresql@16` on the Mac Mini) | Yes |
| Sessions, MFA remembered browsers | Postgres (only hashes) | Yes |
| Live connections, login rate limits, webhook queue | Server memory | No |
| Agent builds and `manifest.json` | `dist/` (staged to `~/Library/Application Support/BEACON/dist`) | Yes |
| Server settings | `server.env` in the BEACON folder | Yes |
| HTTPS certificates | Caddy's data folder | Yes |
| Agent identity (device ID, secret, server URL, fallbacks) | `agent.json` on each device | Yes |
| Backups | `~/Library/Application Support/BEACON/backups/` | Yes |

On startup the server waits for the database (up to 30 tries, 2 s apart), runs migrations, and marks every device offline until its agent reconnects (`store.MarkAllOffline`).

**To fill in:** where Caddy keeps its data (certificates) on the Mac Mini, if not the default for the user account.

## Related

- [Tech stack](tech-stack.md)
- [Code layout](code-layout.md)
- [Database](database.md)
- [Environment variables](environment.md)
