---
title: Console or API not loading
section: Troubleshooting
order: 30
summary: A step-by-step triage when BEACON's console or API does not load, from Caddy and the server down to Postgres, disk space and a bad deploy.
reviewed: 2026-11-20
---

## Work from the outside in

A request goes DNS → router → Caddy (443) → server (8080) or console (3000) → Postgres (5432). Find the first step that fails. Run the commands on the Mac Mini unless noted.

## 1. Is it just you?

From a phone on mobile data (outside the office network), open `https://beacon.itsthev.net/healthz`.

- `ok`: the server works from outside. The problem is your network or browser, or the console only (go to step 5).
- Nothing loads at all: carry on.

## 2. Are the launchd jobs running?

```bash
for l in postgres server web caddy; do
  printf '%-20s %s\n' "com.beacon.$l" "$(sudo launchctl print system/com.beacon.$l 2>/dev/null | awk '/^\tstate =/{print $3}')"
done
```

All four should say `running`. An empty state means the job is not loaded. Start it again:

```bash
sudo launchctl bootstrap system /Library/LaunchDaemons/com.beacon.server.plist   # change the name as needed
```

A job that keeps restarting shows `running` with a changing PID and a non-zero `last exit code` in `sudo launchctl print system/com.beacon.<name>`. Its log says why (step 6).

## 3. Does the server answer locally?

```bash
curl -fsS http://127.0.0.1:8080/healthz; echo
"$HOME/Library/Application Support/BEACON/bin/uem-server" healthcheck; echo "exit $?"
```

- `ok` and `exit 0`: the server and database are fine. Look at Caddy (step 4).
- `db unavailable`: the server runs but cannot reach Postgres (step 7).
- Connection refused and `exit 1`: the server is not running or not listening (step 6).

`uem-server healthcheck` prints nothing. It calls `http://127.0.0.1:8080/healthz` (or the port in `UEM_LISTEN`) and exits 0 when that answers `ok`.

## 4. Is Caddy serving HTTPS?

```bash
curl -fsS --resolve beacon.itsthev.net:443:127.0.0.1 https://beacon.itsthev.net/healthz; echo
tail -n 30 ~/Library/Logs/BEACON/caddy.log
```

- Works locally but not from outside: the router, port forward or DNS (see [Network and DNS](network-dns.md)).
- Certificate errors: see [HTTPS certificates](https-certificates.md).
- Caddy not running and its log says port 443 is in use: find the process with `sudo lsof -nP -iTCP:443 -sTCP:LISTEN`.

## 5. Does the console answer?

```bash
curl -sS -o /dev/null -w '%{http_code}\n' http://127.0.0.1:3000/login
tail -n 30 ~/Library/Logs/BEACON/web.log
```

Expect `200`. If `/healthz` works but pages are blank or broken, the console is the problem. Restart it: `sudo launchctl kickstart -k system/com.beacon.web`. If it broke right after a deploy, see step 9.

## 6. Read the server log

```bash
tail -n 50 ~/Library/Logs/BEACON/server.log
grep '"level":"ERROR"' ~/Library/Logs/BEACON/server.log | tail -n 20
```

Common causes:

- `server exited` with a migration error: a deploy brought a migration that failed. Roll back (step 9) and fix the migration.
- `waiting for database` repeated: Postgres is down (step 7).
- `address already in use`: something else holds port 8080. `sudo lsof -nP -iTCP:8080 -sTCP:LISTEN` shows what. A server started by hand with `go run` is a common culprit.
- A broken `server.env` line (the server starts through `/bin/sh`, so a syntax error there stops it). Compare with the latest `server.env.bak-…`.

## 7. Is Postgres up?

```bash
/opt/homebrew/opt/postgresql@16/bin/pg_isready -h localhost
tail -n 30 ~/Library/Logs/BEACON/postgres.log
```

If it is not accepting connections, restart it and watch its log:

```bash
sudo launchctl kickstart -k system/com.beacon.postgres
```

If the log mentions "No space left on device", go to step 8.

## 8. Is the disk full?

```bash
df -h /
du -sh ~/Library/Logs/BEACON ~/Library/Logs/Sentinel ~/Library/Application\ Support/BEACON/backups
```

If the disk is full, free space by truncating a big log (`: > ~/Library/Logs/BEACON/postgres.log`) or moving old dumps elsewhere. Then restart Postgres and the server. Never delete files in `/opt/homebrew/var/postgresql@16`.

## 9. Did it break after a deploy?

If the trouble started right after `sudo deploy/macos/install.sh --system`:

- Console only: put the previous console back (`web.prev`), as described in [Releases and deploying](releases.md).
- Server: deploy the previous git tag, as in [Releases and deploying](releases.md). If the database is damaged, see [Restoring the database](restore-database.md).

## 10. Last resort: reboot

All jobs start at boot. Reboot the Mac Mini if nothing else helps, then repeat step 2.

## After it is back

- Agents reconnect within about a minute, and devices turn online again.
- Check that last night's backup ran (`tail ~/Library/Logs/BEACON/backup.log`).
- Write down what happened and what fixed it.

**To fill in:** who to tell when BEACON is down, and where incidents are recorded.

## Related

- [The production server](production-server.md)
- [Network and DNS](network-dns.md)
- [HTTPS certificates](https-certificates.md)
