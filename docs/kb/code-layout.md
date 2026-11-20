---
title: Code layout
section: Architecture
order: 80
summary: A tour of the repository's folders and a table of where to change common things.
reviewed: 2026-11-20
---

## Top level

| Path | What |
|---|---|
| `cmd/server` | Entry point of `uem-server`. `main.go` reads environment variables, runs migrations and starts everything. `resetmfa.go` is the `reset-mfa <email>` command. |
| `cmd/agent` | Entry point of `uem-agent`: subcommands `enroll`, `run`, `install`, `uninstall`, `start`, `stop`, `restart`, `status`, `inventory`, `version`, and the internal `session-probe`. `restart*.go` writes the Linux systemd drop-in for fast restarts. |
| `cmd/distmanifest` | Writes `dist/manifest.json` (agent version, sizes, SHA-256). Run by `make build-agents`. |
| `internal/protocol` | The agent ⇄ server message types (JSON over WebSocket). Shared by both sides. |
| `internal/agentdist` | Reads the manifest; decides whether an agent is outdated and whether it can self-update. |
| `internal/server/...` | The server packages (below). |
| `internal/agent/...` | The agent packages (below). |
| `web/` | The Next.js console. |
| `deploy/` | Docker files, `Caddyfile`, and `deploy/macos/` for the Mac Mini. |
| `docs/API.md` | The REST and WebSocket API contract. |
| `docs/kb/` | These articles, embedded into the server by `docs/kb/kb.go`. |
| `Makefile` | Build, test, version, release and deploy targets. |
| `CHANGELOG.md` | What each release contains. |
| `bin/`, `dist/` | Build output (server binary, agent builds). Not source. |

## Server packages (`internal/server`)

| Package | What it does |
|---|---|
| `api` | HTTP handlers, the router (`server.go`), auth/CSRF/RBAC middleware, agent gateway (`agent.go`), live WebSocket, downloads and install scripts, MFA, usage endpoints. Integration tests in `api_test.go`. |
| `core` | The business logic that ties things together: issue commands, handle results, evaluate compliance, push policy, publish live events, agent updates (`agentupdate.go`), dashboard snapshots. |
| `compliance` | The rule field registry and a pure evaluator (`rules.go`), plus the software catalog (`catalog.go`): gambling/game flags, System vs Third-party, helper components. |
| `store` | All SQL. Every tenant query takes `orgID`. One file per area: `devices.go`, `commands.go`, `policy.go`, `usage.go`, `users.go`, `mfa.go`, `audit.go`, `snapshots.go`, `ruletags.go`. |
| `migrations` | SQL schema files, embedded and applied with goose at startup. See [Database](database.md). |
| `audit` | Writes and verifies the hash-chained audit log. |
| `auth` | Password hashing (argon2id), tokens, TOTP, and the role → permission table. |
| `hub` | In-memory registry of live connections (agents and browser tabs). |
| `scheduler` | Cron tasks and housekeeping loops. |
| `notify` | Outbound webhooks, including the Teams card format (`teams.go`). |
| `installers` | `install.sh` and `install.ps1`, embedded and served at `/install.sh` and `/install.ps1`. |

## Agent packages (`internal/agent`)

| Package | What it does |
|---|---|
| `config` | Reads and writes `agent.json` (device ID, secret, server URL, fallbacks). |
| `conn` | The main loop: connect, send hello/heartbeat/telemetry/inventory, receive commands and policy, blocklist, usage, address moves (`move.go`) and fallbacks (`fallback.go`). |
| `collect` | Inventory collectors, one file per OS: `*_darwin.go`, `*_linux.go`, `*_windows.go`. Parsers are in separate files with tests. |
| `exec` | Runs scripts and device actions (reboot, shutdown, lock, message). |
| `session` | The session probe and its supervisor, for active app time on macOS and Windows. |
| `usage` | Counts per-app minutes per user per day between reports. |
| `update` | Downloads, verifies and swaps in a new agent binary. |

See [Agent](agent.md).

## Console (`web/`)

| Path | What |
|---|---|
| `web/app/(console)/<module>/page.tsx` | One folder per menu item: `devices` (and `devices/[id]`), `enrollment`, `groups`, `usage`, `compliance`, `scripts`, `tasks`, `alerts`, `audit`, `users`, `integrations`, `settings`, `account`. `page.tsx` at the root is the Dashboard. |
| `web/app/(auth)/` | Login and signup pages. |
| `web/components/app-shell.tsx` | The sidebar (`NAV`), top bar and live indicator. |
| `web/components/ui/` | Small reusable widgets: button, card, dialog, table, tabs, etc. |
| `web/components/` | Larger shared pieces: commands, usage dashboard/explorer, knowledge base, Markdown renderer, toasts. |
| `web/lib/api.ts` | API types, fetch helpers and React Query keys (`qk`). |
| `web/lib/live.ts` | The live WebSocket and cache updates. |
| `web/lib/config.ts` | Product name and the live URL. |
| `web/lib/session.ts`, `hooks.ts`, `utils.ts`, `share.ts` | Session/permission helpers, shared hooks and utilities. |
| `web/next.config.ts` | Standalone build and `/api` rewrites. |
| `web/scripts/check-grids.mjs` | Extra lint check run by `npm run lint`. |

## Where do I change X?

| I want to… | Change |
|---|---|
| Add or rename a menu item | `NAV` in `web/components/app-shell.tsx`, plus a page folder in `web/app/(console)/` |
| Add an API endpoint | Route in `internal/server/api/server.go`, handler in the matching `api/*.go`, SQL in `store/`, document in `docs/API.md` |
| Change what a role may do | `rolePerms` in `internal/server/auth/auth.go` (and check `web/lib/session.ts` for the UI) |
| Add a database column or table | New file in `internal/server/migrations/` (see [Database](database.md)), then `store/models.go` and queries |
| Add a compliance field | `internal/server/compliance/rules.go` (field registry and evaluator) and tests in `rules_test.go` |
| Change System / Third-party or helper-component rules | `internal/server/compliance/catalog.go` |
| Collect a new piece of inventory | Type in `internal/protocol/protocol.go`, collectors in `internal/agent/collect/collect_<os>.go` |
| Add a new command type | `internal/protocol/protocol.go`, `core.ResolvePayload`, the agent's `execute` in `internal/agent/conn/agent.go`, console UI in `web/components/commands.tsx` |
| Change timing (heartbeat, ping, usage) | Constants at the top of `internal/agent/conn/agent.go` |
| Change offline detection or retention | `internal/server/scheduler/scheduler.go` |
| Add a webhook event | `notify.Events` and an `Emit` call where it happens |
| Change the Teams card | `internal/server/notify/teams.go` |
| Change the install commands | `internal/server/installers/install.sh` / `install.ps1` |
| Change Mac Mini service setup | `deploy/macos/install.sh` and `deploy/macos/com.beacon.*.plist` |
| Change backup schedule or retention | `deploy/macos/com.beacon.backup.plist`, `deploy/macos/backup.sh` (`KEEP_DAYS`) |
| Change the product name in the UI | `web/lib/config.ts` |
| Add a Knowledge base article | A new `.md` file in `docs/kb/` with the header described in `docs/kb/kb.go` |

See [Development](development.md) for how to run and test changes.
