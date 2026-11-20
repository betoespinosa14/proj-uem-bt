---
title: Known security and operations gaps
section: Security
order: 50
summary: An honest list of weak spots in BEACON today, the risk each one carries, and a suggested fix.
reviewed: 2026-11-20
---

These are known, not hidden. Each item says where it comes from in the code, what could go wrong, and what to do about it. Roughly most important first.

## 1. Backups are on the same disk as the database

- **Where:** `deploy/macos/backup.sh` writes to `~/Library/Application Support/BEACON/backups/` on the Mac Mini. The database is in `/opt/homebrew/var/postgresql@16` on the same disk. 14 days are kept.
- **Risk:** a disk failure, theft, ransomware or a wiped Mac takes the database **and** every backup. All devices would have to be re-enrolled by hand.
- **Fix:** copy the newest dump off the machine every day (encrypted cloud storage, NAS, or Time Machine to an external drive). Test a restore now and then. See [Backups](backups.md) and [Restore the database](restore-database.md).

**To fill in:** where off-machine backup copies are kept, if anywhere.

## 2. One machine runs everything

- **Where:** Postgres, server, console and Caddy all run as launchd daemons on one office Mac Mini (`deploy/macos/`). Login limits and the live hub are in memory, so only one server copy can run.
- **Risk:** if the Mac Mini, its power, the office internet or the router port forward is down, no device can report and nobody can use the console. Agents reconnect by themselves afterwards.
- **Fix:** a UPS and auto-restart after power loss for now. Longer term, move to a cloud host with a company DNS name. See [Production server](production-server.md) and [Limitations and roadmap](limitations-roadmap.md).

## 3. Agents aren't code-signed

- **Where:** `Makefile` (`build-agents`) builds plain Go binaries. There's no `codesign`, notarization or `signtool` anywhere in the repo.
- **Risk:** Windows SmartScreen and antivirus may warn about or block the agent, and nothing proves a binary came from you. Agent self-updates are checked against a SHA-256 from the server's own manifest, which protects against corrupted downloads but not against a compromised server.
- **Fix:** buy an Apple Developer ID and a Windows code-signing certificate. Sign (and notarize on macOS) in the build step, and have the agent verify the signature before it installs an update.

## 4. Postgres trusts every local connection

- **Where:** `/opt/homebrew/var/postgresql@16/pg_hba.conf` uses `trust` for local connections. The database user still has the default development password from the `install.sh` template.
- **Risk:** any process or person with a login on the Mac Mini can read or change the whole database, including the audit log, without a password.
- **Fix:** set a strong password, switch `pg_hba.conf` to `scram-sha-256`, add a `~/.pgpass` for `backup.sh`. Steps in [Secrets](secrets.md).

## 5. Secrets stored in plain text in the database

- **Where:** `users.totp_secret`, `webhooks.secret`, `enrollment_tokens.token` (see the migrations in `internal/server/migrations/`).
- **Risk:** anyone with the database or a backup can generate two-step codes for every user, forge webhook signatures, and enroll devices.
- **Fix:** encrypt these columns with a key kept outside the database (for example in `server.env`), and protect backups. Short term: limit who can log in to the Mac Mini.

## 6. Read access is wider than the roles suggest

- **Where:** in `internal/server/api/server.go` most GET routes have no permission check.
- **Risk:** helpdesk and auditor users can read **enrollment tokens** (and enroll devices with them), every script body, and every command's output. Auditors are supposed to be read-only, but a token lets them add devices.
- **Fix:** require `enrollment.write` for `GET /api/enrollment-tokens` and its install commands, and consider `devices.read` checks on the rest.

## 7. Helpdesk has root on every device

- **Where:** `commands.run` allows ad-hoc scripts (`internal/server/core/core.go`, `ResolvePayload`). Scripts run as root/SYSTEM.
- **Risk:** a helpdesk account (or its stolen password) has full control of the fleet. Every script body is audit-logged, but nothing stops it.
- **Fix:** split "run library scripts" from "run ad-hoc scripts" into separate permissions, or require two-step sign-in for anyone with `commands.run`.

## 8. Two-step sign-in is optional, and there's no SSO or email

- **Where:** MFA is per user (`internal/server/api/mfa.go`). There's no OIDC/SAML and no email sending anywhere in the server.
- **Risk:** an account with only a password is one leak away from fleet control. Nobody can reset a forgotten password themselves, and there are no email alerts or invitations; new users get a temporary password passed on by hand.
- **Fix:** check the **2-step** badge on Users & roles and ask everyone to turn it on. Roadmap: SSO (Entra ID, Okta, Google) and an "MFA required" switch (see `README.md`, Roadmap).

## 9. People can't change their own password

- **Where:** the Account page has no password field. Only `PATCH /api/users/{id}` changes passwords, and it needs `users.write`.
- **Risk:** helpdesk and auditor users keep the temporary password an admin gave them, which someone else knows.
- **Fix:** add a "change my password" form (current + new password) to Account security.

## 10. Sign-up is on by default in code

- **Where:** `UEM_ALLOW_SIGNUP` defaults to `true` in `cmd/server/main.go`. Production sets `false` in `server.env`.
- **Risk:** on a fresh install, or if `server.env` loses that line, anyone who reaches the login page can create their own organization on your server.
- **Fix:** default it to `false` in code. Hide the "Create an organization" link on the login page when sign-up is off.

## 11. The install.sh template hardcodes a LAN address

- **Where:** the `server.env` template in `deploy/macos/install.sh` sets `UEM_PUBLIC_URL=http://192.168.130.7:8080`.
- **Risk:** on a new machine, install commands and agent policy would point to that old LAN IP over plain HTTP until someone edits `server.env`. Production already uses `https://beacon.itsthev.net`.
- **Fix:** make the template require the address (or read it from an argument), and default to HTTPS. See [Fresh install](fresh-install.md).

## 12. Leftover "Sentinel" names

- **Where:**
  - `web/package.json` is named `sentinel-uem-web`.
  - `deploy/macos/install.sh`, on a first install, copies the Caddyfile from `~/Library/Application Support/Sentinel/Caddyfile`. On a fresh machine that folder doesn't exist, so the build step **fails** (`set -e`).
  - The production Caddyfile on the Mac Mini (`~/Library/Application Support/BEACON/Caddyfile`, copied from that folder) still uses a snippet named `(sentinel)` and writes its access log to `~/Library/Logs/Sentinel/caddy-access.log`, not to `~/Library/Logs/BEACON/`.
  - `README.md` mentions the old name ("formerly Sentinel UEM").
- **Risk:** confusion (Caddy's access log isn't where the other logs are), and a broken first install on a new Mac.
- **Fix:** rename the package, and make `install.sh` copy `deploy/Caddyfile` (or write a template) when no Caddyfile exists. Rename the snippet and log path in the production Caddyfile, then restart `com.beacon.caddy`. Note that "Sentinel" also appears in the Integrations page and README as Microsoft Sentinel, the SIEM. That one is correct.

## 13. The audit log can be trimmed without detection

- **Where:** `internal/server/audit/audit.go` verifies each link but nothing anchors the newest hash.
- **Risk:** someone with database access can delete the newest entries, or rewrite the whole chain, and verification still passes.
- **Fix:** stream `audit.*` to an outside system with a webhook, or periodically export the CSV / record the latest hash somewhere else. See [The audit log](audit-log.md).

## 14. Smaller items

| Gap | Risk | Suggested fix |
|---|---|---|
| No account lockout; rate limits are in memory | A server restart clears the counters | Store attempts in Postgres if this matters |
| Session length fixed at 12 h in code | Can't shorten it for stricter policies | Read it from an environment variable |
| No `Content-Security-Policy` or `Strict-Transport-Security` header (`securityHeaders` in `server.go`; the production Caddyfile adds none and sets `auto_https disable_redirects`, so plain HTTP isn't redirected) | Weaker defence against injected scripts and HTTPS downgrade | Add both headers in the Caddyfile |
| Webhook queue holds 1024 events and drops the rest when full (`internal/server/notify/webhooks.go`) | Lost events during a flood (for example `audit.*` plus a big bulk action) | Persist deliveries in the database |
| Audit log never purged | The table grows forever | Fine for years at this size; archive old rows only with a plan for the hash chain |
| Remote wipe intentionally not built | Lost laptops can only be locked | See the roadmap in `README.md` |

## Related

- [Secrets](secrets.md)
- [Roles and permissions](roles.md)
- [Limitations and roadmap](limitations-roadmap.md)
