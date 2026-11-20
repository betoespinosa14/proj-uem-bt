---
title: Restoring the database
section: Troubleshooting
order: 40
summary: Step-by-step restore of the BEACON database from a nightly dump, with the server stopped first and checks afterwards.
reviewed: 2026-11-20
---

## Before you start

A restore replaces **all** data in the live `uem` database with the contents of the dump. Anything that happened after the dump was taken is lost: new devices, commands, audit entries, user changes. Devices enrolled after the dump will be rejected by the server and need to be enrolled again (see [Enrolling devices](enrollment.md)).

Restore only when the data is damaged or lost, or after a bad release that changed the database. For anything else, try [Server down](server-down.md) first.

These steps follow the header of `deploy/macos/backup.sh` and the README. Run them on the Mac Mini, as the BEACON user, with `sudo` where shown.

## Steps

1. Pick the dump to restore and check it is readable:
   ```bash
   ls -lh ~/Library/Application\ Support/BEACON/backups/
   F=~/Library/Application\ Support/BEACON/backups/uem-2026-11-19.dump   # change the date
   /opt/homebrew/opt/postgresql@16/bin/pg_restore --list "$F" | head
   ```
   Remember that a deploy replaces that day's file with a dump taken **after** the deploy. To undo a release, take the file from the day before, or the manual dump you made before deploying.
2. Save the current state, so you can go back if the restore was a mistake:
   ```bash
   /opt/homebrew/opt/postgresql@16/bin/pg_dump -h localhost -U uem -Fc \
     -f ~/Library/Application\ Support/BEACON/backups/before-restore-$(date +%F-%H%M).dump uem
   ```
3. Stop the server, so nothing writes to the database during the restore:
   ```bash
   sudo launchctl bootout system/com.beacon.server
   ```
   Leave Postgres running.
4. Restore:
   ```bash
   /opt/homebrew/opt/postgresql@16/bin/pg_restore -h localhost -U uem \
     --clean --if-exists -d uem "$F"
   ```
   `--clean --if-exists` drops each object before recreating it. A few warnings can appear; errors that stop the restore will be obvious. Read the output before going on.
5. Start the server again:
   ```bash
   sudo launchctl bootstrap system /Library/LaunchDaemons/com.beacon.server.plist
   ```
   On start the server runs any pending migrations, so an older dump is brought up to the current schema automatically.

## Check the result

1. The server is up and can reach the database:
   ```bash
   curl -fsS http://127.0.0.1:8080/healthz; echo        # prints ok
   tail -n 20 ~/Library/Logs/BEACON/server.log
   ```
2. Sign in to the console. Check that the Devices list and recent audit entries look as expected for the date of the dump.
3. Open **Audit log** and click **Verify integrity**. The audit log is hash-chained per organization, so a clean restore should still verify.
4. Wait a minute or two and check that devices come back online as their agents reconnect.

## If something went wrong

- **The restore failed halfway:** the server is still stopped. Restore the `before-restore-…` dump from step 2 the same way, then start the server.
- **The server does not start:** read `server.log`. A migration error usually means the dump came from a newer BEACON version than the running one. Deploy the matching version ([Releases and deploying](releases.md)) and start again.
- **`pg_restore: connection refused`:** Postgres is not running. Check `sudo launchctl print system/com.beacon.postgres` and `~/Library/Logs/BEACON/postgres.log`.

## Restoring onto a new machine

If the Mac Mini itself is lost, install BEACON on a new machine first, then restore the dump into its empty database. See [Fresh install](fresh-install.md).

## Related

- [Backups](backups.md)
- [Server down](server-down.md)
