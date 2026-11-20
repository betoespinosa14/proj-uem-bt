---
title: Locked out of the console
section: Troubleshooting
order: 20
summary: What to do when someone lost their two-step sign-in phone, forgot their password or is blocked by too many attempts, including the break-glass command on the server.
reviewed: 2026-11-20
---

## Which case is it?

| Symptom | Section below |
|---|---|
| "too many failed attempts, try again later" | Too many attempts |
| Lost phone, no recovery codes | Lost two-step sign-in |
| Forgot password | Forgot password |
| Nobody who could help can sign in | Break-glass on the server |

## Too many attempts

The server counts sign-in attempts per IP address and email: 10 in 15 minutes. Two-step codes are limited the same way per user, and to 5 wrong codes per sign-in attempt. A correct sign-in resets the count.

Wait 15 minutes and try again. The counters are kept in memory only, so restarting the server also clears them:

```bash
sudo launchctl kickstart -k system/com.beacon.server
```

Failed passwords are recorded in the audit log as `auth.login_failed`.

## Lost two-step sign-in

First, the person should try one of their 10 one-time recovery codes in place of the 6-digit code.

If they have none left, another user resets it from the console:

1. Sign in as an owner or admin.
2. Open **Users & roles**.
3. Click the reset button on the person's row (**Reset two-step sign-in**) and confirm.

Effects: two-step sign-in is turned off for that person and they are signed out everywhere. Their password alone then signs them in. They should turn it on again under their name → **Account security** right away. The reset is recorded as `user.mfa_reset` in the audit log.

Rules (`internal/server/api/mfa.go`):

- Only an **owner** can reset another **owner**. An admin can reset admins, helpdesk and auditors.
- Nobody can reset their own two-step sign-in this way. (Signed-in users turn it off themselves under Account security.)

## Forgot password

BEACON has no "forgot password" link and sends no email. Passwords are reset by another user:

1. An owner or admin opens **Users & roles** and edits the person.
2. They type a **New password** (at least 10 characters) and save.
3. They give the new password to the person through a safe channel.

Changing a password signs that user out everywhere, and remembered browsers ask for a two-step code again. As with two-step resets, only an owner can change an owner's password.

There is **no command-line password reset**. If the only owner forgets their password and no other owner exists, nobody can reset it from the console. Keep at least two owners so this never happens.

**To fill in:** who the owners are, and the agreed procedure if every owner is locked out at once.

## Break-glass on the server

When nobody who could reset someone's two-step sign-in can sign in (for example the only owner lost their phone and recovery codes), use the `reset-mfa` command on the Mac Mini. It is in `cmd/server/resetmfa.go`.

1. Sign in to the Mac Mini as the BEACON user (the account that owns `~/Library/Application Support/BEACON`).
2. Run, replacing the email address:
   ```bash
   set -a; . "$HOME/Library/Application Support/BEACON/server.env"; set +a
   "$HOME/Library/Application Support/BEACON/bin/uem-server" reset-mfa person@example.com
   ```
   The first line loads `DATABASE_URL` from `server.env`, so the command works on the live database. Without it the command falls back to the default local database URL.
3. Expected output:
   - `Two-step sign-in is now off for … and they were signed out.` Done.
   - `… doesn't have two-step sign-in on; nothing to do.` The problem is something else (password or rate limit).
   - `no user with email "…"` Check the spelling; it must match the sign-in email.

The command does the same as the console reset: turns two-step sign-in off, deletes the user's sessions, and writes `user.mfa_reset` to the audit log with actor `uem-server reset-mfa` and `via: command line`. The server does not need to be stopped.

On a Docker install, run the same subcommand inside the server container: `docker compose -f deploy/docker-compose.yml exec server /usr/local/bin/uem-server reset-mfa person@example.com`.

## Related

- [Server down](server-down.md)
- [The production server](production-server.md)
