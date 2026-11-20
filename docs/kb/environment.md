---
title: Environment variables
section: Operations
order: 90
summary: Reference of every environment variable BEACON reads, with its default, what it does and where it is set in production.
reviewed: 2026-11-20
---

## Where settings live in production

| Part | Where its settings are | Apply a change with |
|---|---|---|
| API server | `~/Library/Application Support/BEACON/server.env`, loaded by the `com.beacon.server` plist (`set -a; . ./server.env`) | `sudo launchctl kickstart -k system/com.beacon.server` |
| Console | The `EnvironmentVariables` block of `deploy/macos/com.beacon.web.plist` | Edit the repo plist, then `make deploy-mac` and `sudo deploy/macos/install.sh --system` |
| Caddy | The live `Caddyfile` in the BEACON folder (it does not use variables) | `sudo launchctl kickstart -k system/com.beacon.caddy` |
| Docker installs | `deploy/.env` (copied from `deploy/.env.example`), read by `deploy/docker-compose.yml` | `make up` |

`server.env` is created once by `deploy/macos/install.sh` and never overwritten. Its file mode is 600. Copy it before editing (for example `cp server.env server.env.bak-$(date +%F)`).

True/false values accept `1`, `true`, `yes`, `on` and `0`, `false`, `no`, `off`. Anything else means "use the default".

## API server (`cmd/server/main.go`)

| Variable | Default | Meaning | Production |
|---|---|---|---|
| `DATABASE_URL` | the local `uem` database on `localhost:5432`, no SSL | Postgres connection string. Also used by `uem-server reset-mfa`. | `server.env` (local `uem` database) |
| `UEM_LISTEN` | `:8080` | Address and port the server listens on. Also used by `uem-server healthcheck`. | not set (8080) |
| `UEM_PUBLIC_URL` | empty | The address agents and people use. Used in install commands (unless **Settings → Public server URL** is set), sent to agents in their policy, used for agent bootstrap downloads, and for the "Open in BEACON" button on Teams cards. If empty, install commands use the address you are browsing from. | `https://beacon.itsthev.net` |
| `UEM_FALLBACK_URLS` | empty | Comma-separated other addresses of the same server. Agents use them when the main address fails. See [Network and DNS](network-dns.md). | `https://122.49.212.51` |
| `UEM_AGENT_DIST` | `dist` | Folder with the agent builds and `manifest.json`, served at `/downloads/`. Relative to the working directory (the BEACON folder in production). | `dist` |
| `UEM_ALLOWED_ORIGINS` | empty | Comma-separated extra browser origins allowed to open the live-updates WebSocket. Only needed when the console runs on another port, as in development. | `localhost:3000` |
| `UEM_SECURE_COOKIES` | true when `UEM_PUBLIC_URL` starts with `https://`, otherwise false | Marks session cookies `Secure` (sent over HTTPS only). | `true` |
| `UEM_TRUST_PROXY` | `false` | Trust `X-Forwarded-For`, `X-Forwarded-Proto` and `X-Forwarded-Host` from Caddy, so the server sees real client IPs (for login limits and the audit log) and the right scheme and host. Set it only behind a proxy. | `true` |
| `UEM_ALLOW_SIGNUP` | `true` | Allows `/signup`, which creates a new organization. Turn it off once the first owner exists. | `false` |
| `UEM_WEBHOOK_ALLOW_PRIVATE` | `false` | Allows webhooks to loopback, private, link-local and carrier-grade-NAT addresses. See [Webhook failures](webhook-failures.md). | not set (false) |

All server variables are read once at start. Restart the server after a change.

## Caddy and Docker Compose

These are read by `deploy/Caddyfile` and `deploy/docker-compose.yml`. The Mac Mini does not use them: its live Caddyfile has the values written in.

| Variable | Default | Meaning |
|---|---|---|
| `UEM_SITE_ADDRESS` | `:80` | The site Caddy serves. Set it to the domain name (e.g. `uem.example.com`) to get automatic HTTPS. |
| `UEM_SERVER_UPSTREAM` | `server:8080` | Where Caddy sends API traffic. Use `localhost:8080` when Caddy runs on the host. |
| `UEM_WEB_UPSTREAM` | `web:3000` | Where Caddy sends console traffic. Use `localhost:3000` when Caddy runs on the host. |
| `UEM_HTTP_PORT` | `80` | Host port mapped to Caddy's port 80 (Compose only). |
| `UEM_HTTPS_PORT` | `443` | Host port mapped to Caddy's port 443 (Compose only). |
| `POSTGRES_PASSWORD` | none, required | Password of the `uem` database user in the Compose Postgres container. Compose refuses to start without it. Keep it out of git. |

In Compose, the server also gets `UEM_PUBLIC_URL`, `UEM_ALLOW_SIGNUP` (default `true`) and `UEM_SECURE_COOKIES` (default `false`) from `deploy/.env`, and always `UEM_TRUST_PROXY=true`. `deploy/Dockerfile.server` sets `UEM_LISTEN=:8080` and `UEM_AGENT_DIST=/app/dist`.

## Console (`web/`)

| Variable | Default | Meaning | Production |
|---|---|---|---|
| `UEM_API_URL` | `http://localhost:8080` | Where the console's own server proxies `/api`, `/downloads`, `/install.sh` and `/install.ps1` (`web/next.config.ts`). In production Caddy sends those paths straight to the Go server, so this matters mainly in development. | not set |
| `NEXT_PUBLIC_LIVE_URL` | empty (same host as the page) | WebSocket URL for live updates. Development only, because `next dev` cannot proxy WebSockets. Read when the console is built. | not set |
| `BEACON_PUBLIC_URL` | `https://beacon.itsthev.net` | Public address used in link previews when someone shares a BEACON link (`web/lib/share.ts`). Read when the console is built. | not set (default) |
| `PORT`, `HOSTNAME` | | Port and address of the console server. | `3000` and `127.0.0.1`, in `com.beacon.web.plist` |
| `NODE_ENV` | | | `production`, in `com.beacon.web.plist` |

## Agent

| Variable | Default | Meaning |
|---|---|---|
| `UEM_AGENT_CONFIG` | per OS (below) | Path of the agent's config file (`internal/agent/config/config.go`). Installs never set it. |
| `ProgramData` (Windows) | `C:\ProgramData` | Base folder for the Windows config path. |

Default config paths: Windows `C:\ProgramData\UEM Agent\agent.json`, macOS `/Library/Application Support/UEM Agent/agent.json`, Linux `/etc/uem-agent/agent.json`. The `--config` flag on `uem-agent` overrides them too.

## Tests

| Variable | Default | Meaning |
|---|---|---|
| `UEM_TEST_DATABASE_URL` | empty (integration tests are skipped) | Database for the integration tests. Its name must end in `_test`. `make test` sets it from `TEST_DB`. |
| `TEST_DB` (make variable) | the local `uem_test` database | Override with `make test TEST_DB=postgres://…`. |

## launchd plists

Each plist in `deploy/macos/` also sets `HOME` to the BEACON user's home folder. Caddy needs it to find its certificates, and `backup.sh` needs it to find the backups folder. `com.beacon.postgres.plist` sets `LC_ALL=en_US.UTF-8`.

## Related

- [The production server](production-server.md)
- [Development](development.md)
- [Fresh install](fresh-install.md)
