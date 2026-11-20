---
title: Backups
section: Operations
order: 40
summary: The nightly database dump on the Mac Mini, where it goes, how long it is kept, how to check it ran and how to copy it off the machine.
reviewed: 2026-11-20
---

## What is backed up

The `com.beacon.backup` launchd job runs `deploy/macos/backup.sh` (installed as `~/Library/Application Support/BEACON/bin/backup.sh`). It dumps the whole `uem` database with `pg_dump` in custom format (`-Fc`).

The database holds everything that matters: organizations, users, devices, inventory, telemetry, commands, compliance rules, scripts, tasks, webhooks and the audit log. It also holds secrets: device credential hashes, webhook signing secrets and users' two-step sign-in secrets. Treat every dump as confidential.

Not in the dump, and not backed up by BEACON:

- `server.env` and the live `Caddyfile` in `~/Library/Application Support/BEACON/` (small; copy them by hand).
- Caddy's certificates in `~/Library/Application Support/Caddy/` (Caddy gets new ones by itself).
- The code: that is in git.

## When it runs

- Every night at 02:30 (`StartCalendarInterval` in `deploy/macos/com.beacon.backup.plist`).
- Once after each deploy, because `sudo deploy/macos/install.sh --system` restarts the job.
- Whenever you start it by hand:
  ```bash
  sudo launchctl kickstart system/com.beacon.backup
  ```

If the Mac is off at 02:30, that night's run is skipped.

## Where the dumps go and how long they stay

- Folder: `~/Library/Application Support/BEACON/backups/`
- File name: `uem-YYYY-MM-DD.dump`, one per day. A second run on the same day (for example after a deploy) **replaces** that day's file.
- Retention: dumps older than 14 days are deleted at the end of each run (`KEEP_DAYS=14` in `backup.sh`).
- The folder and files are readable only by the BEACON user (`umask 077`).

The script writes to `.uem-in-progress.dump` first, checks the dump with `pg_restore --list`, and only then renames it. A run that was cut short never leaves a file that looks good.

A dump of the live database was about 1.2 MB in November 2026.

## Check that it ran

1. Look at the log:
   ```bash
   tail -n 6 ~/Library/Logs/BEACON/backup.log
   ```
   A good run shows `backup starting` and then `backup written: …/uem-2026-11-20.dump (1.2M)`. Old files show as `removed old backup: …`.
2. List the dumps, newest last:
   ```bash
   ls -lh ~/Library/Application\ Support/BEACON/backups/
   ```
   Today's file should be there after 02:30.
3. Check that a dump is readable:
   ```bash
   /opt/homebrew/opt/postgresql@16/bin/pg_restore --list \
     ~/Library/Application\ Support/BEACON/backups/uem-$(date +%F).dump | head
   ```

If a run failed, the error is in `backup.log`. The usual cause is that Postgres was not running (see [Server down](server-down.md)).

## Take a manual dump before something risky

The automatic files are named by date, so a manual run on the same day overwrites them. For a dump you want to keep separately (for example before a risky release), write it under another name:

```bash
/opt/homebrew/opt/postgresql@16/bin/pg_dump -h localhost -U uem -Fc \
  -f ~/Library/Application\ Support/BEACON/backups/manual-$(date +%F-%H%M).dump uem
```

Files that do not match `uem-*.dump` are not removed by the 14-day cleanup. Delete them yourself when no longer needed.

## Copy backups off the machine

The dumps are on the same disk as the database. If the Mac Mini's disk dies, is stolen or is wiped, the backups go with it. Copy a recent dump somewhere else regularly, for example:

```bash
cp ~/Library/Application\ Support/BEACON/backups/uem-$(date +%F).dump /Volumes/<backup drive>/
```

Keep `server.env` and the `Caddyfile` with it, because a new machine needs them too. Store the copy encrypted, since the dump contains secrets.

**To fill in:** where off-machine copies are kept (drive, NAS or cloud), how often they are made, and who does it.

**To fill in:** whether Time Machine or any other backup covers the Mac Mini.

## Restore

In short: stop the server, run `pg_restore --clean --if-exists` into the `uem` database, start the server again. The full steps, including checks, are in [Restoring the database](restore-database.md). To move BEACON to a new machine, see [Fresh install](fresh-install.md).

## Related

- [The production server](production-server.md)
- [Restoring the database](restore-database.md)
- [Releases and deploying](releases.md)
