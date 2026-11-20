---
title: FAQ: Settings
section: FAQ
order: 110
summary: What is on the Settings page, what only owners can change, and which settings live in server.env instead.
reviewed: 2026-11-20
---

### What's on the Settings page?

Three cards:

- **Organization:** organization name and public server URL.
- **Application usage tracking:** on/off and "Count as idle after".
- **Your account:** your name, email, role, member since, two-step sign-in status (with a link to Account security), the organization ID and the latest agent version the server offers.

### Who can change the settings?

Only **owners** (`settings.write`). Everyone else sees the Organization and usage cards read-only, with the note "only owners can change these settings". See [Roles and permissions](roles.md).

### What does "Public server URL" do?

It's the address agents use to reach the server, and the address in install commands. It overrides `UEM_PUBLIC_URL` from `server.env`. Leave it blank to use `UEM_PUBLIC_URL` (and, if that isn't set either, the address you're browsing from).

### What happens when I change the public server URL?

Every connected agent gets the new address right away. Each one first checks that `<new address>/healthz` answers and that it can connect there, and only then saves it. If the new address doesn't work, the agent stays on the old one and tries again after 10 minutes. So a typo can't strand devices. Very old agents (before this feature) keep their address until their install command is re-run. See [Network and DNS](network-dns.md).

### What's the difference between Settings and server.env?

Settings in the console are stored in the database and apply at once. `server.env` on the Mac Mini holds server-level settings that need a restart (`sudo launchctl kickstart -k system/com.beacon.server`):

| Variable | Does |
|---|---|
| `DATABASE_URL` | How to reach Postgres |
| `UEM_PUBLIC_URL` | Default public address (Settings can override it) |
| `UEM_FALLBACK_URLS` | Other addresses agents use while the main one is unreachable |
| `UEM_ALLOW_SIGNUP` | Whether new organizations can sign up (`false` in production) |
| `UEM_SECURE_COOKIES` | Secure cookies (HTTPS only) |
| `UEM_TRUST_PROXY` | Trust Caddy's forwarded client IP |
| `UEM_ALLOWED_ORIGINS` | Extra browser origins for live updates (development) |
| `UEM_AGENT_DIST` | Folder with the agent builds |
| `UEM_WEBHOOK_ALLOW_PRIVATE` | Allow webhooks to private addresses (not set in production) |
| `UEM_LISTEN` | Server listen address (default `:8080`) |

See [Environment variables](environment.md).

### What does turning usage tracking on do?

Agents start recording open and active time per app and user. It's off by default. Inform staff first: it's employee monitoring. Changes are audit-logged. Data is kept for 400 days. See [FAQ: App usage](faq-app-usage.md).

### What does "Count as idle after" do?

Time in front of an app counts as **active** until this long passes without keyboard or mouse input. Choices: 1, 2, 3, 5, 10 or 15 minutes; default 2. It applies from now on and isn't applied to past data.

### Are setting changes logged?

Yes, every change is `org.update` in the audit log, with the new values.

### Can I change how often devices report, or the session length?

Not from Settings. Report intervals (60 s health, 6 h inventory) and the 12-hour session length are fixed in code (`cmd/server/main.go`, `internal/server/api/server.go`).

### Can I change the organization name?

Yes (owners). It shows in the console. The organization ID never changes.

### Is there a setting to require two-step sign-in or set password rules?

No. Two-step sign-in is per person, and the only password rule is a 10-character minimum. See [Known security gaps](known-gaps.md).

### Where do I see the server's agent version?

**Settings → Your account → Latest agent on server.** It's the version of the agent builds the server has. Devices with an older agent show **Update available**. See [Agent updates](agent-updates.md).
