---
title: Database
section: Architecture
order: 90
summary: The PostgreSQL database, its main tables, how migrations run, and how to connect on the Mac Mini.
reviewed: 2026-11-20
---

## What and why

All of BEACON's lasting data is in one PostgreSQL 16 database named `uem`, owned by the user `uem`. The server is the only program that writes to it. Postgres was chosen for reliable transactions, JSON columns (inventory, rule conditions) and triggers (the audit log guard).

The server connects with the `DATABASE_URL` environment variable (`cmd/server/main.go`). On the Mac Mini it is set in `~/Library/Application Support/BEACON/server.env`. Never paste that value into tickets or chat: it may contain a password. See [Secrets](secrets.md).

## Main tables

Defined in `internal/server/migrations/*.sql`. Every tenant table has an `org_id`.

| Table | Holds |
|---|---|
| `organizations` | The org: name, `server_url` (Public server URL), `usage_tracking`, `idle_threshold_sec` |
| `users` | Console users: email (unique, case-insensitive), argon2id password hash, role, TOTP fields |
| `sessions` | Signed-in sessions; `id` is the SHA-256 of the cookie token, plus the CSRF token |
| `mfa_recovery_codes`, `mfa_trusted_browsers` | Hashed recovery codes; browsers that ticked "Remember this browser" |
| `device_groups` | Groups (departments) and their `blocked_apps` |
| `enrollment_tokens` | Tokens, expiry, use limits, revoked flag, target group |
| `devices` | One row per device: hashed secret, OS, hardware summary, status, last seen, compliance state, tags, latest CPU/memory/disk/battery, `employee_name`, `remote_ip`, `enrollment_token_id`, `retired_at` |
| `device_inventory` | The latest full inventory per device as JSON (`data jsonb`) |
| `device_software` | Installed software rows per device |
| `device_telemetry` | Time series of CPU/memory/disk/battery (kept 30 days) |
| `scripts` | The script library |
| `commands` | Every command sent to a device, with status, exit code, output |
| `compliance_rules` | Rules: conditions and actions as JSON, severity, OS and group scope |
| `compliance_results` | Latest pass/fail per device and rule |
| `device_rule_tags` | Tags put on devices by rules, so they can be removed again |
| `scheduled_tasks` | Cron tasks: target and action as JSON |
| `alerts` | Alerts; a unique index allows one open alert per device and rule |
| `webhooks` | Webhook URL, secret, events, format (`generic` / `teams`), last status |
| `audit_log` | The append-only, hash-chained audit log |
| `app_usage` | Per device, day, app and user: open `minutes`, `active_seconds`, `last_used`, `last_active` (kept 400 days) |
| `usage_infrastructure_apps` | Apps marked as infrastructure |
| `fleet_snapshots` | Daily Dashboard figures per org and department (kept 400 days) |
| `goose_db_version` | goose's record of which migrations have run |

### The audit log guard

Triggers `audit_log_no_update` and `audit_log_no_delete` (in `00001_init.sql`) raise an error on any UPDATE or DELETE of `audit_log`. Only a session that sets `uem.allow_audit_delete = on` may delete rows, which is used when deleting a whole organization. Don't work around this. See [Audit log](audit-log.md).

## How migrations work

1. Migration files live in `internal/server/migrations/`, named `000NN_description.sql`. There are 13 today (`00001_init.sql` to `00013_app_usage_last_active.sql`).
2. They are embedded into the server binary (`//go:embed *.sql` in `migrations.go`), so there are no loose files to deploy.
3. Every time the server starts, `cmd/server/main.go` waits for the database, then calls `migrations.Up`, which runs goose and applies any migration not yet listed in `goose_db_version`.
4. If a migration fails, the server exits with an error and launchd restarts it every 10 seconds (`ThrottleInterval`). Read `~/Library/Logs/BEACON/server.log`.

There is no command to roll migrations back. The `-- +goose Down` sections exist but nothing runs them. To undo, restore a backup ([Restore the database](restore-database.md)) or write a new forward migration.

## How to add a migration

1. Create the next file, for example `internal/server/migrations/00014_device_notes.sql`.
2. Write it in goose format:

   ```sql
   -- +goose Up
   -- Why this column exists, in one sentence.
   ALTER TABLE devices ADD COLUMN notes text NOT NULL DEFAULT '';

   -- +goose Down
   ALTER TABLE devices DROP COLUMN notes;
   ```

3. Wrap functions or anything with `;` inside a statement in `-- +goose StatementBegin` / `-- +goose StatementEnd`.
4. Update the Go types in `internal/server/store/models.go` and the queries.
5. Run `make test` against the test database.
6. Deploy. The server applies it on start. Take a manual backup first if the change is risky (`sudo launchctl kickstart system/com.beacon.backup`).

Rules:

- Never edit or rename a migration that has already run in production. Add a new one.
- Keep new columns `NOT NULL DEFAULT ...` where possible so old rows stay valid.
- Large data rewrites run during startup, while devices wait. Keep them quick.

## Connecting with psql on the Mac Mini

Postgres comes from Homebrew (`postgresql@16`), runs as the `com.beacon.postgres` daemon, and keeps its data in `/opt/homebrew/var/postgresql@16` (`deploy/macos/com.beacon.postgres.plist`).

```bash
/opt/homebrew/opt/postgresql@16/bin/psql -h localhost -U uem uem
```

Useful read-only checks:

```sql
SELECT version_id, is_applied, tstamp FROM goose_db_version ORDER BY id DESC LIMIT 5;
SELECT status, count(*) FROM devices WHERE retired_at IS NULL GROUP BY status;
SELECT pg_size_pretty(pg_database_size('uem'));
```

The nightly backup connects as `uem` with `-w` (never prompt for a password), so local connections from the BEACON user must already work without typing one.

**To fill in:** how Postgres authenticates local connections on the Mac Mini (trust in `pg_hba.conf`, or a `~/.pgpass` file) and where that password is kept.

Be careful: you are on the live database. Prefer `SELECT`. For changes, take a backup first. See [Backups and restore](backups.md).

## Test database

The integration tests (`internal/server/api/api_test.go`) need a separate database:

```bash
createdb -h localhost -U uem uem_test   # once
make test                               # uses TEST_DB, default .../uem_test
```

`make test` passes `TEST_DB` as `UEM_TEST_DATABASE_URL`. The tests refuse any database whose name doesn't end in `_test`, so they can't touch the live one. If the variable is unset, those tests are skipped. See [Development](development.md).

## Docker setup

In `deploy/docker-compose.yml` the database is the `postgres:16-alpine` service with user and database `uem`, password from `POSTGRES_PASSWORD` in `deploy/.env`, and data in the `pgdata` volume. That setup is not used on the Mac Mini.
