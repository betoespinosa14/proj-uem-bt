---
title: Tech stack
section: Architecture
order: 70
summary: Every technology BEACON uses, with its version and what it is for.
reviewed: 2026-11-20
---

## Where versions come from

| Part | File that pins the versions |
|---|---|
| Go server and agent | `go.mod` (and `go.sum`) |
| Console | `web/package.json` (exact versions in `web/package-lock.json`) |
| Docker images | `deploy/Dockerfile.server`, `web/Dockerfile`, `deploy/docker-compose.yml` |
| Mac Mini | Homebrew packages referenced in `deploy/macos/*.plist` and `install.sh` |

A `^` in `package.json` means "this version or a newer compatible one"; the lock file records what is actually installed. Update this page when you upgrade something.

## Server and agent (Go)

| Technology | Version | What it does here |
|---|---|---|
| Go | 1.26 (`go 1.26.0` in `go.mod`; `golang:1.26-alpine` image) | Language for the server and agent. Builds one static binary per platform with no runtime to install. |
| chi | `github.com/go-chi/chi/v5` v5.5.0 | HTTP router for the server's REST API (`internal/server/api/server.go`). |
| pgx | `github.com/jackc/pgx/v5` v5.11.0 | PostgreSQL driver and connection pool used by `internal/server/store`. |
| goose | `github.com/pressly/goose/v3` v3.29.0 | Applies the SQL migrations in `internal/server/migrations` when the server starts. |
| coder/websocket | `github.com/coder/websocket` v1.8.15 | WebSockets: the agent connection, and live updates to browsers. |
| robfig/cron | `github.com/robfig/cron/v3` v3.0.1 | Runs scheduled tasks from cron expressions (`internal/server/scheduler`). |
| kardianos/service | `github.com/kardianos/service` v1.3.0 | Installs and runs the agent as a Windows service, launchd daemon or systemd unit (`cmd/agent/main.go`). |
| gopsutil | `github.com/shirou/gopsutil/v4` v4.26.9 | Reads CPU, memory, disk, network and process data on the device (`internal/agent/collect`, `internal/agent/conn`). |
| battery | `github.com/distatus/battery` v0.11.0 | Reads battery level on Windows and Linux (`battery_other.go`). |
| plist | `howett.net/plist` v1.0.1 | Parses macOS property lists, used to read battery details on macOS (`battery_darwin.go`). |
| x/crypto | `golang.org/x/crypto` v0.58.0 | argon2id password hashing (`internal/server/auth/auth.go`). |
| x/sys | `golang.org/x/sys` v0.49.0 | Low-level Windows calls in the agent: registry, user sessions, message boxes. |

Two-step sign-in codes (TOTP) are implemented in the project itself (`internal/server/auth/totp.go`), not by a library.

## Console (web)

| Technology | Version | What it does here |
|---|---|---|
| Next.js | 15.5.28 | Web framework for the console (App Router), built as a standalone Node server. |
| React | ^19.2.0 (and react-dom) | The UI library the pages are written in. |
| TanStack React Query | ^5.105.0 | Fetches API data, caches it and keeps it fresh; live events patch this cache (`web/lib/live.ts`). |
| Tailwind CSS | ^3.4.19 (with PostCSS ^8.5.6, autoprefixer ^10.4.21) | Utility CSS classes for all styling. |
| Recharts | ^3.10.1 | Charts: telemetry, Dashboard trends, app usage. |
| lucide-react | ^1.62.0 | The icon set. |
| clsx, tailwind-merge | ^2.1.1, ^3.7.0 | Small helpers to combine CSS class names. |
| qrcode | 1.5.4 | Draws the QR code when a user sets up two-step sign-in (`web/app/(console)/account/page.tsx`). |
| TypeScript | ^5.9.3 | Typed JavaScript; `npm run typecheck` checks it. |
| ESLint | ^9.39.0 with eslint-config-next 15.5.28 | Code checks; `npm run lint` also runs `web/scripts/check-grids.mjs`. |
| react-markdown, remark-gfm | ^10.1 / ^4.0 | Render these Markdown articles (tables, task lists) in the console. |

## Infrastructure

| Technology | Version | What it does here |
|---|---|---|
| PostgreSQL | 16 (`postgres:16-alpine` in Docker; Homebrew `postgresql@16` on the Mac Mini) | Stores everything. See [Database](database.md). |
| Caddy | 2 (`caddy:2-alpine` in Docker; Homebrew `caddy` on the Mac Mini, version not pinned) | HTTPS with automatic certificates, and routing to the server and console. See [HTTPS certificates](https-certificates.md). |
| Node.js | 22 (`node:22-alpine` image; README requires Node 22+; `/opt/homebrew/bin/node` on the Mac Mini, version not pinned) | Runs the console's production server and the build. |
| Docker Compose | Compose file with services `postgres`, `server`, `web`, `caddy` | The containerized way to run BEACON (`make up`). Not used on the Mac Mini. |
| Distroless | `gcr.io/distroless/static-debian12:nonroot` | Minimal base image for the server container: no shell, non-root. |
| launchd | macOS built-in | Starts BEACON's daemons at boot on the Mac Mini and restarts them if they crash. |
| Homebrew | Installed at `/opt/homebrew` | Provides Postgres, Caddy and Node on the Mac Mini. |
| systemd / Windows services / launchd | OS built-ins | Run the agent on devices. |
| make | – | Build, test, release and deploy commands (`Makefile`). |
| git tags | – | Releases are tags `vYYYY.MM.N`. See [Releases](releases.md). |

**To fill in:** the exact Homebrew versions of Caddy, Node and PostgreSQL on the Mac Mini (`brew list --versions caddy node postgresql@16`).

## Upgrading safely

1. Change one thing at a time.
2. Go: `go get <module>@<version>`, then `go mod tidy` and `make test`.
3. Console: `npm install <package>@<version>` in `web/`, then `npm run lint`, `npm run typecheck`, `npm run build`.
4. Postgres major versions (16 → 17) need a dump and restore or `pg_upgrade`, and a change to the paths in `deploy/macos/*.plist` and `backup.sh`. Plan it; don't let Homebrew do it by surprise.
5. Note the change in `CHANGELOG.md`.

See [Development](development.md) and [Releases](releases.md).
