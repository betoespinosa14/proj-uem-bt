---
title: How sign-in works
section: Security
order: 10
summary: Passwords, session cookies, CSRF protection, two-step sign-in, sign-up control and rate limits, as the code implements them.
reviewed: 2026-11-20
---

## The short version

- People sign in with email and password. Passwords are hashed with argon2id.
- A successful sign-in creates a server-side session that lasts **12 hours**. There is no "keep me signed in" beyond that.
- Two-step sign-in (authenticator app codes) is **optional per user**. Nobody is forced to turn it on.
- There is **no SSO and no email**. Nobody can reset their own forgotten password; an owner or admin sets a new one.
- Sign-up of new organizations is controlled by `UEM_ALLOW_SIGNUP`. It is `false` on the production Mac Mini.

The code lives in `internal/server/auth/` (hashing, tokens, TOTP), `internal/server/api/auth.go` (login, sign-up, sessions, rate limiter) and `internal/server/api/mfa.go` (two-step sign-in).

## Passwords

- Hashed with **argon2id**: 2 passes, 19 MiB memory, 1 thread, 32-byte key, random 16-byte salt (`HashPassword` in `internal/server/auth/auth.go`). These are OWASP's minimums.
- Rules: at least **10** characters, at most 256 (`validatePassword` in `internal/server/api/auth.go`). No other complexity rules.
- If the email doesn't exist, the server still runs a dummy argon2 check (`DummyCheck`), so response time doesn't reveal which emails are registered. The error is always "invalid email or password".
- Emails are unique across the whole server, not just per organization.
- Users cannot change their own password from the Account page. An owner or admin changes it on **Users & roles** (edit user, "New password"). See [FAQ: Users](faq-users.md).

## Sessions and cookies

After a correct sign-in, `startSession` in `internal/server/api/auth.go`:

1. Makes a random 32-byte session token. Only its **SHA-256 hash** is stored in the `sessions` table, so a database leak doesn't hand out live sessions.
2. Sets two cookies:

| Cookie | Holds | Flags |
|---|---|---|
| `uem_session` | the session token | `HttpOnly`, `SameSite=Strict`, `Secure` when `UEM_SECURE_COOKIES` is on |
| `uem_csrf` | the CSRF token | readable by the console, `SameSite=Strict`, `Secure` |

- **Lifetime:** 12 hours from sign-in (`SessionTTL` default in `internal/server/api/server.go`). It does not slide with activity, and there's no environment variable to change it. Changing it means editing the code.
- **Expired sessions** are deleted by the hourly retention job (`internal/server/scheduler/scheduler.go`).
- **Sign-out** deletes the session row and clears both cookies.
- **Revocation:** changing a user's role or password signs that user out everywhere (unless you changed your own). Deleting a user deletes their sessions. Resetting someone's two-step sign-in also signs them out.
- The live-update WebSocket (`/api/live`) re-checks the session every minute, so a revoked session also loses live updates.

## CSRF protection

Every state-changing request (anything but GET/HEAD/OPTIONS) must send the header `X-CSRF-Token` equal to the session's CSRF token (`csrf` middleware in `internal/server/api/server.go`). The console reads it from the `uem_csrf` cookie. Together with `SameSite=Strict` cookies this blocks cross-site requests.

## Two-step sign-in (TOTP)

Each person turns it on for themselves under their name → **Account security** (`/account`).

**Setup**
1. The server makes a random 160-bit secret, stores it as *pending*, and shows it once as a QR code (`otpauth://` link, issuer `BEACON`).
2. The user enters a code from their app. Only then does the secret become active.
3. The server shows **10 one-time recovery codes** (format `k7m2q-x9c4d`). They are stored only as SHA-256 hashes.

**Codes:** standard TOTP (SHA-1, 6 digits, 30-second steps), with one step of clock drift allowed either side. A code can't be used twice: the last accepted step is saved (`totp_last_step`).

**Sign-in with it on**
1. Correct password → the server does not create a session yet. It sets a short-lived `uem_mfa` cookie (5 minutes) and asks for a code.
2. The user enters an authenticator code or a recovery code.
3. Limits: **5 wrong codes per sign-in attempt** (then they must enter the password again), and **10 wrong codes per user per 15 minutes**.
4. "Remember this browser" sets a `uem_trusted` cookie for **30 days**. That browser then skips the code. Changing the password, turning two-step sign-in off, or "Forget remembered browsers" on the Account page clears them.

**Turning it off / new recovery codes:** both ask for the password again.

**Lost phone and recovery codes:** see [Locked out](locked-out.md). In short, an owner or admin resets it from **Users & roles** (only an owner can reset another owner), or, as a last resort, run on the server:

```bash
set -a; . "$HOME/Library/Application Support/BEACON/server.env"; set +a
"$HOME/Library/Application Support/BEACON/bin/uem-server" reset-mfa <email>
```

That command (`cmd/server/resetmfa.go`) turns it off, signs the person out everywhere and writes `user.mfa_reset` to the audit log.

The authenticator secret is stored **in plain text** in the `users` table (`totp_secret`). Anyone with the database or a backup can generate codes. See [Secrets](secrets.md).

## Sign-up control

- `POST /api/auth/signup` creates a **new organization** with the caller as its owner. It does not add people to your organization.
- It only works when `UEM_ALLOW_SIGNUP` is true. **The code's default is true** (`cmd/server/main.go`). Production sets `UEM_ALLOW_SIGNUP=false` in `server.env`, so the server answers "sign-up is disabled on this server".
- The login page always shows a "Create an organization" link, even when sign-up is off. Clicking it just ends in that error.
- To add a colleague, use **Users & roles → Add user** instead.

## Rate limits and lockouts

All limiters are **in memory** (`limiter` in `internal/server/api/auth.go`). They reset when the server restarts and would not be shared between several server copies.

| What | Limit | Key |
|---|---|---|
| Password sign-in | 10 attempts per 15 minutes | client IP + email |
| Sign-up | 10 per 15 minutes | client IP |
| Two-step codes | 10 wrong per 15 minutes; 5 per sign-in attempt | user |
| Re-entering password (turn off 2-step, new codes) | 10 per 15 minutes | user |
| Agent enrollment | 30 per minute | client IP |

A successful sign-in clears the counter. There is **no permanent account lockout**: after the window passes, attempts are allowed again. Failed passwords for existing accounts are written to the audit log as `auth.login_failed`, and wrong codes as `auth.mfa_failed`.

The client IP comes from `X-Forwarded-For` only when `UEM_TRUST_PROXY=true` (set in production, because Caddy sits in front).

## Related

- [Roles and permissions](roles.md)
- [Secrets](secrets.md)
- [Locked out](locked-out.md)
- [Known security gaps](known-gaps.md)
