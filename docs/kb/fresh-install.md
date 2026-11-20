---
title: Fresh install
section: Operations
order: 70
summary: Installing BEACON from scratch on a new Mac with Homebrew and launchd, or anywhere with Docker Compose, and bringing the data over from a backup.
reviewed: 2026-11-20
---

## Which way

- **(a) A new Mac with Homebrew and launchd.** The same setup as today's Mac Mini. Use this to replace the Mac Mini.
- **(b) Docker Compose** (`deploy/docker-compose.yml`). Any machine with Docker. Simpler, but it differs from production (see the notes at the end).

Either way, the network side stays the same: a DNS name pointing at a public IP, and the router forwarding TCP 443 (and ideally 80) to the machine. See [Network and DNS](network-dns.md).

## (a) New Mac with Homebrew and launchd

`deploy/macos/install.sh` and the plists hard-code `/opt/homebrew`, so this needs an Apple Silicon Mac.

### 1. Prepare the Mac

1. Create the macOS user that will run BEACON, and sign in as that user.
2. In System Settings, turn system sleep off and turn on "Start up automatically after a power failure". Leave FileVault off, or the Mac waits at the unlock screen after every reboot and nothing starts.
3. Reserve a fixed LAN IP for the Mac on the router.

### 2. Install the tools

```bash
brew install go node postgresql@16 caddy rsync
```

The repo needs Go 1.26 or newer (`go.mod`) and Node 22 or newer.

### 3. Create the database

1. Start Postgres once with Homebrew (the launchd job replaces this later):
   ```bash
   brew services start postgresql@16
   ```
2. Create the `uem` role and database. Use the password you will put in `DATABASE_URL`:
   ```bash
   /opt/homebrew/opt/postgresql@16/bin/psql -d postgres -c "CREATE ROLE uem LOGIN PASSWORD '<choose one>'"
   /opt/homebrew/opt/postgresql@16/bin/createdb -O uem uem
   ```
   The server creates all tables itself on first start (migrations in `internal/server/migrations/`).

Homebrew's Postgres trusts local connections by default, which is what `backup.sh` relies on (`pg_dump -w`, no password).

### 4. Get the code

```bash
git clone <repository URL> proj-uem-bt
cd proj-uem-bt/web && npm ci && cd ..
```

The clone can live anywhere (today it is under `~/Desktop`). The daemons never run from it; they run from the copy in `~/Library/Application Support/BEACON`.

**To fill in:** the repository URL and the account used to clone it.

### 5. Put the Caddyfile in place first

On its first run, `install.sh` copies the Caddyfile from `~/Library/Application Support/Sentinel/Caddyfile`, a folder from the old setup. On a new Mac that file does not exist and the script stops. Create the BEACON folder and its Caddyfile yourself before running it:

```bash
mkdir -p ~/Library/Application\ Support/BEACON
cp <old Mac's Caddyfile> ~/Library/Application\ Support/BEACON/Caddyfile
```

Copy the live Caddyfile from the old Mac if you have it. Otherwise start from `deploy/Caddyfile`: replace `{$UEM_SITE_ADDRESS::80}` with the domain name, and the two upstreams with `localhost:8080` and `localhost:3000`. If port 80 is not forwarded, add the `tls { issuer acme { disable_http_challenge } }` block the live file uses (see [HTTPS certificates](https-certificates.md)).

### 6. Build, stage and install

```bash
make deploy-mac
```

This creates `~/Library/Application Support/BEACON/server.env` from a template. **Edit it before going on.** The template has `UEM_PUBLIC_URL=http://192.168.130.7:8080`, which is wrong for production. Set at least:

```bash
DATABASE_URL=postgres://uem:<password>@localhost:5432/uem?sslmode=disable
UEM_PUBLIC_URL=https://beacon.itsthev.net
UEM_ALLOW_SIGNUP=true          # only until the first owner exists, see below
```

Keep `UEM_AGENT_DIST=dist`, `UEM_TRUST_PROXY=true` and `UEM_SECURE_COOKIES=true`. Add `UEM_FALLBACK_URLS` if you use one. All variables are in [Environment variables](environment.md).

Then install the launchd jobs:

```bash
sudo deploy/macos/install.sh --system
```

On the first run it stops Homebrew's own Postgres service and starts `com.beacon.postgres` on the same data folder. It ends by printing the state of the five jobs.

### 7. Check

```bash
curl -fsS http://127.0.0.1:8080/healthz; echo
curl -fsS https://beacon.itsthev.net/healthz; echo
tail -n 30 ~/Library/Logs/BEACON/caddy.log
```

## (b) Docker Compose

1. Install Docker with the Compose plugin.
2. Create the settings file:
   ```bash
   cp deploy/.env.example deploy/.env
   ```
   Edit `deploy/.env`: set `POSTGRES_PASSWORD` to a long random string, `UEM_SITE_ADDRESS` to the domain name, `UEM_PUBLIC_URL=https://<domain>` and `UEM_SECURE_COOKIES=true`.
3. Start everything:
   ```bash
   make up          # docker compose -f deploy/docker-compose.yml up -d --build
   make logs        # follow the server log
   ```
   This starts `postgres`, `server`, `web` and `caddy`. Caddy listens on ports 80 and 443 and keeps its certificates in the `caddy_data` volume. The database lives in the `pgdata` volume.

Differences from the Mac Mini:

- There is no backup job. Set one up yourself, for example a nightly `docker compose -f deploy/docker-compose.yml exec -T postgres pg_dump -U uem -Fc uem > uem-$(date +%F).dump`.
- `Dockerfile.server` builds the agents with version `0.0.0-dev` and writes no `manifest.json`, so the console offers no agent updates. Fix the build before relying on it.
- `UEM_FALLBACK_URLS` and `UEM_WEBHOOK_ALLOW_PRIVATE` are not passed through; add them to the `server` service's `environment` if needed.

## The first owner account

Sign-up creates a new organization and makes that person its owner. It is allowed while `UEM_ALLOW_SIGNUP` is true (the server's default, and the default in `docker-compose.yml`).

1. Open `https://<domain>/signup` and create the organization and owner.
2. Set `UEM_ALLOW_SIGNUP=false` (in `server.env`, or `deploy/.env`) and restart the server, so nobody else can create an organization.

Skip this when you restore a backup: the restored database already has the organization and users.

## Bringing data over from the old machine

1. On the new machine, finish the install and check that `/healthz` answers.
2. Copy the latest dump from the old machine (`~/Library/Application Support/BEACON/backups/uem-YYYY-MM-DD.dump`), or from the off-machine copy.
3. Restore it as in [Restoring the database](restore-database.md). With Docker, stop the server (`docker compose -f deploy/docker-compose.yml stop server`) and run:
   ```bash
   docker compose -f deploy/docker-compose.yml exec -T postgres \
     pg_restore -U uem --clean --if-exists -d uem < uem-YYYY-MM-DD.dump
   docker compose -f deploy/docker-compose.yml start server
   ```
4. Point DNS (or the router's port forward) at the new machine.
5. Devices reconnect by themselves: their identity is in the database. If the public address changed, keep the old address working until agents have moved (see [Network and DNS](network-dns.md)).

## Related

- [The production server](production-server.md)
- [Environment variables](environment.md)
- [Backups](backups.md)
