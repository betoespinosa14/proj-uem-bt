---
title: Live updates
section: Architecture
order: 110
summary: How the console updates itself without reloading, and how the agent's WebSocket protocol works.
reviewed: 2026-11-20
---

## Two WebSockets

BEACON uses two separate WebSocket connections. Both go through Caddy on the same address as everything else.

| Socket | Who connects | Authenticates with | Direction | Code |
|---|---|---|---|---|
| `/api/live` | Each open console tab | Session cookie | Server → browser only | `live` in `internal/server/api/agent.go`, `web/lib/live.ts` |
| `/api/agent/ws` | Each device's agent | `Authorization: Device <id>:<secret>` | Both ways | `agentWS` in `internal/server/api/agent.go`, `internal/agent/conn/agent.go` |

Both are tracked by the in-memory **hub** (`internal/server/hub/hub.go`): one connection per device, and a set of browser connections per organization.

## Console live updates (`/api/live`)

### What the server sends

Every message is JSON `{"type": ..., "data": ...}` (`hub.Publish`). There are three types:

| Type | Data | Sent when |
|---|---|---|
| `device.updated` | A `Device` | Status change, new telemetry or inventory, enrollment, going offline |
| `command.updated` | A `Command` | A command is queued, sent, running, finished or timed out |
| `alert.created` | An `Alert` | A compliance rule raises a new alert |

Events only go to tabs of the same organization.

### How the console uses them

`useLive` in `web/lib/live.ts` is started once by the app shell:

1. Opens `wss://<host>/api/live` (or `NEXT_PUBLIC_LIVE_URL` in development; see `web/lib/config.ts`).
2. On `device.updated` it patches the device in every cached device list and the device page. If the device now matches or no longer matches a list's filter, that list is refetched. Refetches are batched every 2 seconds so a burst of telemetry doesn't flood the API.
3. On `command.updated` it updates the command, the device's command list and the Dashboard's recent activity.
4. On `alert.created` it adds the alert, refreshes alerts, Dashboard and compliance data, and shows a toast.
5. The top bar shows **Live**, **Connecting** or **Offline** (`useLiveState`).

### Reconnecting

- If the socket closes, the console retries with exponential backoff from 1 s up to 30 s, with jitter.
- When the tab becomes visible again it reconnects at once.
- After a reconnect it refetches everything, because events sent while disconnected are not replayed.

### Server side safeguards

- The server pings each browser every 30 s; a failed ping closes the socket.
- Every minute it re-checks the session; signing out or revoking a session ends the stream.
- Each connection has a send buffer (512 frames for browsers, 256 for agents). A client too slow to keep up is disconnected and resyncs on reconnect.
- The browser's `Origin` must match the host or a pattern in `UEM_ALLOWED_ORIGINS`.

### In development

`next dev` can't proxy WebSockets. Point the console straight at the server and allow its origin:

```bash
# server
UEM_ALLOWED_ORIGINS=localhost:3000 go run ./cmd/server
# console
cd web && NEXT_PUBLIC_LIVE_URL=ws://localhost:8080/api/live npm run dev
```

See [Development](development.md).

## Agent protocol (`/api/agent/ws`)

Defined in `internal/protocol/protocol.go` (protocol version 1). Every frame is a JSON envelope `{"type": ..., "data": ...}`.

### Enrollment (before the socket)

`POST /api/agent/enroll` with the token and basic facts returns `device_id` and `device_secret`. The server stores only a hash of the secret and fires the `device.enrolled` webhook. Enrollment is rate-limited per IP.

### Agent → server

| Type | When | Server does |
|---|---|---|
| `hello` | First frame on every connection | Marks the device online, audits an agent version change, sends policy, resends unconfirmed commands, delivers queued commands |
| `heartbeat` | Every 30 s | Updates last seen |
| `telemetry` | Every 60 s | Stores CPU/memory/disk/battery, IP and logged-in user |
| `inventory` | Every 6 h and on request | Stores it, evaluates compliance, publishes `device.updated` |
| `usage` | About every 5 min, while tracking is on | Adds to `app_usage` (dropped if tracking is off) |
| `command_result` | `running`, then a final status | Stores it, publishes `command.updated`, fires `command.completed` |
| `event` | E.g. a blocked app was killed | Writes an audit entry `device.event.<kind>` |

### Server → agent

| Type | When | Carries |
|---|---|---|
| `policy` | On connect and when settings change (org, group) | Blocklist, telemetry and inventory intervals, usage tracking, idle threshold, server URL, fallback URLs |
| `command` | When issued, or when the device reconnects | Command ID, type and payload (library scripts are inlined) |

### Timeouts and reliability

| Rule | Value | Where |
|---|---|---|
| Server read timeout | 90 s without a frame closes the socket | `agentReadTimeout` |
| Max frame size | 16 MiB (big software lists) | `agentMaxMessage` |
| Marked offline | 90 s after last contact (checked every 15 s) | `scheduler.markOffline` |
| Agent ping | Every 30 s, 10 s to answer | `pingInterval`, `pingTimeout` |
| Duplicate commands | Never re-run; result re-reported | `handleCommand` |
| Commands on disconnect | Those marked sent go back to queued | `store.RequeueSent` |
| Stuck commands | Timed out after 75 min | `scheduler.expireCommands` |

If a device reconnects while its old socket is still open, the hub closes the old one.

## Limits

- The hub is in memory: run one server process only. A second replica would not see the other's connections.
- Webhooks are separate from live updates; see [Architecture](architecture.md) and [Webhook failures](webhook-failures.md).
- If live updates stop in production, check that Caddy passes WebSocket upgrades on `/api/*` and that the browser's address matches the server's allowed origins. See [Server down](server-down.md).
