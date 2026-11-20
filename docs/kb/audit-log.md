---
title: The audit log
section: Security
order: 30
summary: How the hash-chained audit log works, what it records, how to verify and export it, and what to do if the chain is broken.
reviewed: 2026-11-20
---

## What it is

The audit log is an append-only record of who did what in BEACON. Each organization has its own log, stored in the `audit_log` table. Entries are **hash-chained**: each entry's hash covers its own content and the previous entry's hash. Editing or deleting a row in the middle breaks verification of every later row.

Code: `internal/server/audit/audit.go` (hashing and verification) and `internal/server/store/audit.go` (writing and reading).

People with `audit.read` (owners, admins, auditors) see it under **Audit log**. See [Roles and permissions](roles.md).

## How an entry is written

1. The server takes a per-organization database lock (`pg_advisory_xact_lock`), so two writers can't both link to the same predecessor.
2. It reads the newest entry's hash (`prev_hash`; empty for the first entry).
3. It computes `hash = SHA-256(prev_hash, org, timestamp, actor type, actor id, actor email, action, target type, target id, details, ip)`, with a separator byte between fields. `details` is canonical JSON (sorted keys), so it hashes the same after a round trip through Postgres.
4. It inserts the row with both `prev_hash` and `hash`.

If the write fails, the action has already happened. The failure is only logged to the server log ("audit write failed"), not shown to the user.

Every saved entry is also offered to webhooks subscribed to `audit.*` (as event `audit.<action>`). See [FAQ: Integrations](faq-integrations.md).

## What is recorded

Each entry has a time, an actor (`user`, `system` or `device`), an action, a target and JSON details. The main actions:

| Area | Actions |
|---|---|
| Sign-in | `auth.login` (with method: password, authenticator_app, recovery_code, remembered_browser), `auth.login_failed`, `auth.logout`, `auth.mfa_failed`, `auth.mfa_enabled`, `auth.mfa_disabled`, `auth.mfa_recovery_used`, `auth.mfa_recovery_codes_replaced`, `auth.mfa_browsers_forgotten` |
| Users | `user.create`, `user.update` (role from/to, password changed yes/no), `user.delete`, `user.mfa_reset` |
| Organization | `org.create`, `org.update`, `usage.infrastructure.add`, `usage.infrastructure.remove` |
| Devices | `device.enroll`, `device.update`, `device.retire`, `device.compliance.change`, `device.tag.add`, `device.tag.remove`, `agent.updated`, `device.event.<kind>` (for example `device.event.app_blocked`) |
| Commands | `device.command.create` (for scripts: the shell and **full script body**), `device.command.result` (status, exit code) |
| Compliance | `compliance.rule.create/update/delete`, `compliance.evaluate`, `compliance.violation` |
| Other admin | `group.*`, `enrollment_token.create/revoke`, `script.*`, `task.*` (including `task.run`), `alert.resolve`, `webhook.*` |
| Exports | `devices.export`, `device.software.export`, `device.usage.export`, `usage.export`, `audit.export`, `audit.verify` |

Not recorded: page views and reads (except exports), and failed sign-ins for emails that don't exist.

Script **output** isn't in the audit log; it's on the device's Commands tab.

## Using the Audit log page

- Filter by action type (Sign-ins, Devices, Commands, Compliance, Users, Enrollment, Scripts, Tasks, Groups, Webhooks) or by actor email.
- Click a row to see the entry number, IP, target and full JSON details.
- 100 entries load at a time; **Load older entries** pages back.

## Verify integrity

The **Verify integrity** button (API: `GET /api/audit/verify`) recomputes every hash of your organization's chain, oldest first. It returns:

```json
{"valid": true, "checked": 18234, "broken_at_id": null}
```

The check itself is recorded as `audit.verify`. On a large log it reads every row, so it can take a few seconds.

## CSV export

The export button (API: `GET /api/audit/export.csv`) downloads the **whole** log, oldest first, as `audit-log-<UTC time>.csv` with columns:

`id, timestamp, actor_type, actor_id, actor_email, ip, action, target_type, target_id, details, hash`

Cells starting with `=`, `+`, `-`, `@`, tab or carriage return get a leading `'` so spreadsheets don't run them as formulas. Each export is recorded as `audit.export`.

## What a broken chain means

"Tampering detected: the hash chain breaks at entry #N" means entry N doesn't match: either its `prev_hash` isn't the previous row's hash, or its content no longer produces its stored hash. Possible causes:

- someone edited or deleted rows directly in Postgres;
- a partial or mixed database restore;
- a code change to the hash fields or JSON encoding (a bug, not tampering).

The app itself has no way to edit or delete audit rows.

## What to do if it breaks

1. **Don't fix anything yet.** Export the CSV and take a copy of the current database (`pg_dump`) so you keep the evidence.
2. Note the entry number. Look at entries just before and after it in the console.
3. Check whether a restore happened recently ([Restore the database](restore-database.md)) or a release changed `internal/server/audit` ([Releases](releases.md)).
4. Compare with an older backup ([Backups](backups.md)): find the row in the backup and see what changed.
5. If it looks like tampering, treat it as a security incident: rotate secrets ([Secrets](secrets.md)), check who had access to the Mac Mini and the database, and tell the owners.
   **To fill in:** who must be told about a suspected security incident.

The server keeps working with a broken chain. New entries still link to the newest row.

## Limits to be aware of

- **Deleting the newest entries is not detected.** Verification only checks that each row links to the one before. Removing the last rows leaves a shorter but valid chain. Nothing outside the database anchors the latest hash.
- Someone with database access could **rewrite the whole chain** with recomputed hashes. Exported CSVs or old backups are your independent record.
- The audit log is **never purged**. It grows forever (other data has retention; this doesn't).

## Related

- [FAQ: Audit log](faq-audit.md)
- [Known security gaps](known-gaps.md)
