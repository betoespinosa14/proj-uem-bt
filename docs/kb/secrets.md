---
title: Secrets and how to rotate them
section: Security
order: 40
summary: Every secret BEACON depends on, where it lives, who can read it, and how to rotate it.
reviewed: 2026-11-20
---

This page lists where secrets are, never their values. Don't paste secret values into the Knowledge base, tickets or chat.

## Summary

| Secret | Where it lives | Rotate by |
|---|---|---|
| Server settings, including the database URL | `~/Library/Application Support/BEACON/server.env` on the Mac Mini | Edit the file, restart `com.beacon.server` |
| Postgres access | `DATABASE_URL` in `server.env`; Postgres auth rules in `/opt/homebrew/var/postgresql@16/pg_hba.conf` | Change the role password, update `server.env` (see below) |
| User passwords | `users.password_hash` (argon2id) | Owner/admin sets a new one on Users & roles |
| Sessions | `sessions` table (SHA-256 of the cookie) | Sign out, change password or role, or wait 12 h |
| Authenticator (TOTP) secrets | `users.totp_secret`, **plain text** | User turns two-step sign-in off and on again |
| Recovery codes | `mfa_recovery_codes` (SHA-256) | Account security → New recovery codes |
| Remembered browsers | `mfa_trusted_browsers` (SHA-256 of the cookie) | Account security → Forget remembered browsers |
| Enrollment tokens | `enrollment_tokens.token`, **plain text** | Revoke and create a new token |
| Device secrets (agent credentials) | Server: `devices.secret_hash` (SHA-256). Device: the agent's `agent.json` | Retire and re-enroll the device |
| Webhook signing secrets | `webhooks.secret`, **plain text** | API `PATCH /api/webhooks/{id}` with a new `secret` |
| Teams workflow URLs | `webhooks.url` | Make a new workflow in Teams, edit the webhook |
| HTTPS certificates and keys | Caddy's data folder: `~/Library/Application Support/Caddy` | Automatic (Caddy renews) |
| Database backups | `~/Library/Application Support/BEACON/backups/` | n/a. They contain all of the above that lives in the database |

## server.env on the Mac Mini

`~/Library/Application Support/BEACON/server.env` is read by the `com.beacon.server` daemon at start (`deploy/macos/com.beacon.server.plist`). It's created once by `deploy/macos/install.sh` with mode `600` and never overwritten by later deploys. Today it sets:

`DATABASE_URL`, `UEM_AGENT_DIST`, `UEM_ALLOWED_ORIGINS`, `UEM_PUBLIC_URL`, `UEM_TRUST_PROXY`, `UEM_SECURE_COOKIES`, `UEM_ALLOW_SIGNUP`, `UEM_FALLBACK_URLS`.

Only `DATABASE_URL` is a secret. The others are settings; see [Environment variables](environment.md).

After editing:

```bash
sudo launchctl kickstart -k system/com.beacon.server
```

## Postgres password

- The server connects with `DATABASE_URL` from `server.env`. The template in `install.sh` and the code's fallback (`cmd/server/main.go`) both use the **default development user and password**. Assume production still uses it unless you changed it.
- On the Mac Mini, `pg_hba.conf` uses `trust` for local socket and `127.0.0.1`/`::1` connections. That means **Postgres doesn't check the password at all** for local connections. Any process or user on the Mac Mini can connect as any database role. The nightly `backup.sh` relies on this (`pg_dump -w`, and there is no `~/.pgpass`).
- Postgres listens only on localhost (Homebrew's default; `listen_addresses` isn't set), so it isn't reachable from the network.

**To rotate** (plan a short outage):
1. Pick a new password and store it in the password manager.
2. `psql -h localhost -U uem -d uem -c "ALTER ROLE uem PASSWORD '…'"` (type it; don't leave it in shell history).
3. Update `DATABASE_URL` in `server.env` and restart `com.beacon.server`.
4. If you also switch `pg_hba.conf` from `trust` to `scram-sha-256`, create a `~/.pgpass` (mode 600) so `backup.sh` still works, and run a backup by hand to check: `sudo launchctl kickstart system/com.beacon.backup`.

For Docker installs the password is `POSTGRES_PASSWORD` in `deploy/.env` (see `deploy/docker-compose.yml`).

## Human passwords

BEACON stores only argon2id hashes. Nobody, including the server, can read a password back.

**To fill in:** which password manager holds the BEACON owner login.

**To fill in:** where the Mac Mini's macOS admin account password is kept (needed for `sudo` on every deploy).

**To fill in:** who holds the GitHub account for the repository and how access is granted.

**To fill in:** where the DNS provider and router admin logins are kept.

To rotate a user's password: **Users & roles → edit → New password**. That signs them out everywhere and forgets their remembered browsers. Only owners can change an owner's password.

## Authenticator secrets and recovery codes

- TOTP secrets are stored **unencrypted** in `users.totp_secret`. Anyone with the database or a backup can produce valid codes for every user with two-step sign-in on.
- Recovery codes are stored as SHA-256 hashes, so they can't be read back.
- To rotate a person's authenticator secret: they turn two-step sign-in **off** and **on** again on Account security (new QR code). An owner/admin reset does the "off" part.
- If the database or a backup leaks, ask everyone to do this.

## Enrollment tokens

- Format `enr_…`. Stored in plain text and shown in the console, because install commands include them.
- **Any signed-in user can read them**, not only those with `enrollment.write` (the list endpoint has no permission check).
- Options when creating: group, expiry (up to 8760 hours) and max devices.
- **To rotate:** Enrollment → revoke the old token, create a new one. Already-enrolled devices are not affected: tokens are only used once, at enrollment.
- Good habit: give tokens an expiry and a max-devices count.

## Device secrets (how agents authenticate)

1. At enrollment the agent sends the token to `POST /api/agent/enroll`.
2. The server creates the device and a random 32-byte **device secret**. It stores only the SHA-256 hash (`devices.secret_hash`) and returns the secret once.
3. The agent saves server URL, device ID and secret in `agent.json`:
   - Windows: `C:\ProgramData\UEM Agent\agent.json` (the installer limits it to SYSTEM and Administrators)
   - macOS: `/Library/Application Support/UEM Agent/agent.json` (mode 600)
   - Linux: `/etc/uem-agent/agent.json` (mode 600)
4. Every connection to `/api/agent/ws` sends `Authorization: Device <id>:<secret>`.

There's no built-in rotation. **To rotate** one device's secret: on the device page, **Retire device** (this blanks the stored hash and disconnects it), then on the device delete `agent.json` and run a fresh install command. The device comes back as a new device. Re-running the installer while `agent.json` exists keeps the old identity and doesn't re-enroll. See [Enrollment](enrollment.md).

## Webhook signing secrets

- Each webhook gets a random secret `whsec_…` when it's created. Deliveries carry `X-UEM-Signature: sha256=<HMAC-SHA256 of the body>`.
- Stored in plain text and shown in the edit dialog (generic webhooks only) to anyone with `webhooks.write`.
- **The console has no "rotate secret" button.** To rotate, call the API as an owner/admin: `PATCH /api/webhooks/{id}` with `{"secret": "<new value>"}` (and the `X-CSRF-Token` header), then update the receiver. Or delete the webhook and create a new one.
- A Teams workflow URL is itself a secret: anyone with it can post to the channel. To rotate, create a new workflow in Teams and paste the new URL into the webhook.

## HTTPS certificates

Caddy gets and renews certificates for the site address automatically and keeps them (with private keys) under `~/Library/Application Support/Caddy`. See [HTTPS certificates](https-certificates.md).

## Backups contain secrets

The nightly dumps hold password hashes, TOTP secrets, enrollment tokens, webhook secrets and device secret hashes. They're written with mode 600 into the same folder tree. Treat any copy you take off the machine as sensitive. See [Backups](backups.md).

## Related

- [Known security gaps](known-gaps.md)
- [How sign-in works](sign-in.md)
