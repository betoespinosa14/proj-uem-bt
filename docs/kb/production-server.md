---
title: The production server (Mac Mini)
section: Operations
order: 10
summary: How BEACON runs on the office Mac Mini as five launchd jobs, where its files and logs are, and how to check, stop and restart it.
reviewed: 2026-11-20
---

## What runs where

BEACON in production runs on one Mac Mini in the office. There is no Docker on it. Five launchd jobs, all defined in `deploy/macos/`, run the whole system:

| launchd label | What it runs | Port | Log file |
|---|---|---|---|
| `com.beacon.postgres` | PostgreSQL 16 from Homebrew: `/opt/homebrew/opt/postgresql@16/bin/postgres -D /opt/homebrew/var/postgresql@16` | 5432 | `postgres.log` |
| `com.beacon.server` | The Go API server `bin/uem-server`, after loading `server.env` | 8080 | `server.log` |
| `com.beacon.web` | The Next.js console: `/opt/homebrew/bin/node server.js` in the `web` folder, on 127.0.0.1 | 3000 | `web.log` |
| `com.beacon.caddy` | Caddy: `/opt/homebrew/bin/caddy run --config <BEACON folder>/Caddyfile` | 443 | `caddy.log` |
| `com.beacon.backup` | `bin/backup.sh`, a nightly database dump at 02:30 | none | `backup.log` |

The plist files are installed in `/Library/LaunchDaemons/` (for example `/Library/LaunchDaemons/com.beacon.server.plist`). They are LaunchDaemons, so they start at boot without anyone logging in. Each one has a `UserName` key, so the processes run as the Mac user who installed BEACON, not as root.

`postgres`, `server`, `web` and `caddy` have `RunAtLoad` and `KeepAlive`. launchd starts them at boot and restarts them within 10 seconds (`ThrottleInterval`) if they crash. `backup` has only a calendar time, so it shows as "not running" between runs. That is normal.

For how a request reaches these services, see [Network and DNS](network-dns.md).

## Where the files are

`~` below means the home folder of the Mac user who runs BEACON.

| Path | What it is |
|---|---|
| `~/Library/Application Support/BEACON/` | The BEACON folder. The daemons run from here, not from the git repo, because macOS privacy protection blocks background processes from reading `~/Desktop`. |
| `…/BEACON/bin/uem-server` | The server binary. |
| `…/BEACON/bin/backup.sh` | Copy of `deploy/macos/backup.sh`. |
| `…/BEACON/server.env` | Server settings (database URL, public URL and so on). Created once by `install.sh`, never overwritten. See [Environment variables](environment.md). |
| `…/BEACON/Caddyfile` | The live Caddy config. Created once, never overwritten. It is **not** the same as `deploy/Caddyfile` in the repo. |
| `…/BEACON/web/` | The running console. `web.prev/` is the previous one. |
| `…/BEACON/dist/` | Agent downloads and `manifest.json`. See [Agent updates](agent-updates.md). |
| `…/BEACON/backups/` | Nightly database dumps. See [Backups](backups.md). |
| `…/BEACON/launchd/` | The rendered plists that `install.sh --system` copies into `/Library/LaunchDaemons/`. |
| `/opt/homebrew/var/postgresql@16/` | The database files. |
| `~/Library/Application Support/Caddy/` | Caddy's certificates and ACME account. See [HTTPS certificates](https-certificates.md). |
| `~/Library/Logs/BEACON/` | Logs of all five jobs (`postgres.log`, `server.log`, `web.log`, `caddy.log`, `backup.log`). |
| `~/Library/Logs/Sentinel/caddy-access.log` | Caddy's access log (one line per request). The path is set in the live Caddyfile and still uses the old "Sentinel" name. |

`server.env` and `Caddyfile` are machine configuration. Dated copies such as `server.env.bak-2026-11-10` in the same folder are manual backups made before edits.

**To fill in:** the Mac Mini's macOS user name, its admin account, and where its password is kept.

## Check status

1. See all five jobs at once:
   ```bash
   for l in postgres server web caddy backup; do
     printf '%-20s %s\n' "com.beacon.$l" "$(sudo launchctl print system/com.beacon.$l 2>/dev/null | awk '/^\tstate =/{print $3}')"
   done
   ```
   Expect `running` for the first four. `backup` shows `not running` except at 02:30.
2. Details for one job (PID, last exit code, number of runs):
   ```bash
   sudo launchctl print system/com.beacon.server
   ```
3. Check the server answers and can reach the database:
   ```bash
   curl -fsS http://127.0.0.1:8080/healthz; echo
   ```
   It prints `ok`. A `db unavailable` answer means Postgres is down.
4. Check Postgres directly:
   ```bash
   /opt/homebrew/opt/postgresql@16/bin/pg_isready -h localhost
   ```

## Start, stop and restart

| Task | Command |
|---|---|
| Restart one job (for example after editing `server.env`) | `sudo launchctl kickstart -k system/com.beacon.server` |
| Stop a job (it stays stopped until the next boot) | `sudo launchctl bootout system/com.beacon.caddy` |
| Start a stopped job again | `sudo launchctl bootstrap system /Library/LaunchDaemons/com.beacon.caddy.plist` |
| Run a backup now | `sudo launchctl kickstart system/com.beacon.backup` |

Do not use `kill` on these processes: launchd restarts them immediately because of `KeepAlive`. To stop a job for good, `bootout` it and also delete its plist from `/Library/LaunchDaemons/`.

Restarting `com.beacon.server` disconnects every agent. They reconnect on their own within about a minute. Restarting Postgres also stops the server from working until Postgres is back.

## Read the logs

```bash
cd ~/Library/Logs/BEACON
tail -f server.log            # follow the API server
tail -n 100 caddy.log         # recent Caddy messages (certificates, errors)
grep '"level":"ERROR"' server.log | tail -n 20
```

`server.log` is JSON, one object per line, with `time`, `level` and `msg`. Agent connects and disconnects appear as `agent connected` / `agent disconnected` with the device ID.

The launchd logs are plain files that nothing rotates. They grow forever, so check their size now and then (below). Caddy rolls its own access log by itself.

## Disk space

```bash
df -h /
du -sh ~/Library/Logs/BEACON ~/Library/Logs/Sentinel \
  ~/Library/Application\ Support/BEACON/backups /opt/homebrew/var/postgresql@16
```

If the disk fills up, Postgres stops writing and the server fails. To free space, truncate a large log (for example `: > ~/Library/Logs/BEACON/postgres.log`) or move old backups off the machine. Do not delete anything under `/opt/homebrew/var/postgresql@16`.

## What happens on reboot or power loss

- All four services start at boot, before anyone logs in, in no fixed order. The server waits up to about a minute for the database. If Postgres is still not up, the server exits and launchd starts it again 10 seconds later.
- When the server starts, it marks every device offline. Devices turn online again as their agents reconnect, usually within a minute.
- If the Mac is off at 02:30, that night's backup does not run.
- As checked on 2026-11-20 (`pmset -g`, `fdesetup status`): system sleep is off, "restart automatically after a power failure" (`autorestart`) is on, and FileVault is off. FileVault matters: with it on, a rebooted Mac waits at the unlock screen and nothing starts. Keep these settings.

While the Mac Mini is down, no device can report and the console is unreachable. Agents reconnect automatically afterwards.

**To fill in:** where the Mac Mini physically sits, whether it is on a UPS, and who has physical access.

## Related

- [Releases and deploying](releases.md)
- [Server down](server-down.md)
- [Environment variables](environment.md)
