---
title: FAQ: Audit log
section: FAQ
order: 90
summary: Short answers about who can see the audit log, finding entries, verifying, exporting and keeping it.
reviewed: 2026-11-20
---

For how the hash chain works, see [The audit log](audit-log.md).

### Who can see the audit log?

People with `audit.read`: owners, admins and auditors. Helpdesk users can't. See [Roles and permissions](roles.md).

### Can anyone edit or delete entries?

Not through BEACON: there's no edit or delete anywhere in the app or API. Someone with direct database access could, and **Verify integrity** would then report a broken chain (except for removal of the very newest entries; see [The audit log](audit-log.md)).

### How do I find what a person did?

Type part of their email into **Filter by actor email**. Combine it with the action filter (Sign-ins, Devices, Commands, Compliance, Users, Enrollment, Scripts, Tasks, Groups, Webhooks).

### How do I see exactly which script someone ran?

Filter by **Commands** and open the `device.command.create` entry. Its details include the shell and the full script body. The output is on the device's Commands tab, not in the audit log.

### What do the actor types mean?

- **User:** a person signed in to the console (with email and IP).
- **System:** BEACON itself, for example compliance changes, scheduled tasks, or the `reset-mfa` command line.
- **Device:** an agent, for example enrollment, agent updates and blocked apps.

### Why are there `auth.login_failed` entries?

Someone entered the wrong password for an existing account. Many in a row from one IP can mean password guessing; that IP and email are limited to 10 tries per 15 minutes. Wrong two-step codes show as `auth.mfa_failed`. Failed attempts for emails that don't exist aren't logged.

### How do I check nobody tampered with it?

Click **Verify integrity**. A green bar ("Integrity verified: N entries, hash chain intact") is good. A red bar names the first broken entry; follow "What to do if it breaks" in [The audit log](audit-log.md).

### How often should I verify?

The code doesn't do it on a schedule. A monthly check by hand is a reasonable habit.

### How do I export it?

The export button on the Audit log page downloads the **whole** log (not just the filtered view) as CSV, oldest first, with the hash of every entry. Each export is itself logged as `audit.export`.

### Can I send the audit log to another system?

Yes: a generic webhook subscribed to `audit.*` receives every new entry as it's written. That also gives you a copy outside the Mac Mini. See [FAQ: Integrations](faq-integrations.md).

### How long are entries kept?

Forever. Telemetry, usage and sessions have automatic clean-up; the audit log doesn't.

### Why does my action not appear?

Reads and page views aren't logged, only changes, sign-ins and exports. If a change is missing, check the server log (`~/Library/Logs/BEACON/server.log`) for "audit write failed".

### What time zone are the times in?

They're stored in UTC and shown in your browser's local time. The CSV uses UTC.
