---
title: Roles and permissions
section: Security
order: 20
summary: The four roles, the full permission matrix, what each permission unlocks in the console, and what only owners can do.
reviewed: 2026-11-20
---

## The four roles

Every user has exactly one role. Roles are fixed in code (`rolePerms` in `internal/server/auth/auth.go`); you can't create custom roles from the console.

| Role | Meant for | In one line |
|---|---|---|
| **Owner** | The people responsible for BEACON | Everything, including organization settings and managing other owners. |
| **Admin** | IT staff who run the fleet day to day | Everything except organization settings and touching owners. |
| **Helpdesk** | Support staff | View devices, run commands and scripts, resolve alerts. |
| **Auditor** | Compliance or management reviewers | Read-only devices plus the audit log. |

## Permission matrix

| Permission | Owner | Admin | Helpdesk | Auditor |
|---|:-:|:-:|:-:|:-:|
| `devices.read` | yes | yes | yes | yes |
| `devices.write` | yes | yes | – | – |
| `commands.run` | yes | yes | yes | – |
| `scripts.write` | yes | yes | – | – |
| `compliance.write` | yes | yes | – | – |
| `tasks.write` | yes | yes | – | – |
| `enrollment.write` | yes | yes | – | – |
| `users.write` | yes | yes | – | – |
| `webhooks.write` | yes | yes | – | – |
| `alerts.write` | yes | yes | yes | – |
| `audit.read` | yes | yes | – | yes |
| `settings.write` | yes | – | – | – |
| `kb.read` | yes | yes | – | – |

Owner has `AllPermissions`. Admin has `AllPermissions` minus `settings.write`. `kb.read` (this Knowledge base, added in 2026.11.50) is part of `AllPermissions`, so owners and admins get it automatically and helpdesk and auditors don't.

The server enforces these on every request (`perm` middleware in `internal/server/api/server.go`). The console also hides buttons a user can't use, but the server check is what counts.

## What each permission unlocks in the console

| Permission | Unlocks |
|---|---|
| `devices.read` | Dashboard, Devices list and device pages, Groups, Enrollment, App usage, Compliance rules, Scripts, Scheduled tasks, Alerts, Settings (read-only), CSV exports of devices, software and usage. See the note below. |
| `devices.write` | Edit a device's name, employee, group and tags; **Retire device**; create, edit and delete groups (including the app blocklist); mark or unmark apps as infrastructure in App usage. |
| `commands.run` | Every device action: Run script (library **or ad-hoc**), Send message, Lock screen, Reboot, Shut down, Refresh inventory, Update agent, **Uninstall agent**, and bulk actions on the Devices list. |
| `scripts.write` | Create, edit and delete scripts in the Script library. |
| `compliance.write` | Create, edit, enable/disable and delete compliance rules; **Re-evaluate all**. |
| `tasks.write` | Create, edit, enable/disable, delete and **Run now** scheduled tasks. |
| `enrollment.write` | Create and revoke enrollment tokens. |
| `users.write` | The **Users & roles** page: see users, add, edit, remove, reset two-step sign-in. |
| `webhooks.write` | The **Integrations** page: webhooks, signing secrets, test deliveries. |
| `alerts.write` | The **Resolve** button on Alerts. |
| `audit.read` | The **Audit log** page, **Verify integrity**, and the audit CSV export. |
| `settings.write` | Editing **Settings → Organization** (name, public server URL) and **Application usage tracking** (on/off, idle time). |
| `kb.read` | The **Knowledge base** page. |

**Note on reading:** read-only API calls (GET) mostly don't check a permission at all. Any signed-in user can read devices, groups, scripts, tasks, alerts, compliance rules, App usage and **enrollment tokens** (including the token value and install commands). The exceptions are the audit log (`audit.read`), users (`users.write`), webhooks (`webhooks.write`) and the Knowledge base (`kb.read`). `docs/API.md` states the same rule. See [Known security gaps](known-gaps.md).

**Note on helpdesk:** `commands.run` includes running an **ad-hoc script** typed into the dialog. Scripts run as root on macOS/Linux and SYSTEM on Windows. So helpdesk users have full control of every managed device, even though they can't edit the script library.

## Owner-only actions

1. **Organization settings** (`settings.write`): organization name, public server URL (moves every agent), usage tracking on/off and idle time.
2. **Managing owners** (`canManageRole` in `internal/server/api/admin.go`): only an owner can
   - create a user with the owner role, or give someone the owner role;
   - edit, change the password of, or delete an existing owner;
   - reset an owner's two-step sign-in.

An admin who tries any of these gets "only owners can …". In the console the owner rows have no edit buttons for admins.

## Safety rules built into the code

- An organization must keep **at least one owner**. Demoting the last owner fails with "an organization must keep at least one owner".
- Nobody can delete their own account.
- Nobody can reset their own two-step sign-in from Users; they turn it off on **Account security** instead.
- Changing someone's role signs them out everywhere, so the new permissions apply at once.
- Every user change is audit-logged (`user.create`, `user.update` with `role_from`/`role_to`, `user.delete`, `user.mfa_reset`).

## Related

- [How sign-in works](sign-in.md)
- [FAQ: Users](faq-users.md)
- [Audit log](audit-log.md)
