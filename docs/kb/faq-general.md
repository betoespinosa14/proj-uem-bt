---
title: FAQ: General
section: FAQ
order: 10
summary: Quick answers for whoever takes over BEACON - signing in, where things are, deploying a fix, and what to do when the server fails.
reviewed: 2026-11-20
---

### What is BEACON, in one paragraph?

A self-hosted device management (UEM) system. A small agent on each Windows, Mac and Linux computer reports inventory, health, compliance and app usage to a Go server with a Postgres database. IT staff use a web console to see devices, run scripts and actions, and get alerts. See [Overview](overview.md) and [Architecture](architecture.md).

### Where does it run?

On one office Mac Mini, as five launchd jobs: `com.beacon.postgres`, `com.beacon.server`, `com.beacon.web`, `com.beacon.caddy` and the nightly `com.beacon.backup`. The live files are in `~/Library/Application Support/BEACON` and logs in `~/Library/Logs/BEACON`. On 2026-11-20 its `server.env` sets the public address to `https://beacon.itsthev.net`, with `https://122.49.212.51` as a fallback for agents. See [Production server](production-server.md) and [Network and DNS](network-dns.md).

**To fill in:** the Mac Mini's physical location and who can get to it.

### How do I sign in as the owner?

Open the console address, enter the owner's email and password, then the 6-digit code if two-step sign-in is on.

**To fill in:** which password manager holds the BEACON owner login.

If nobody can sign in, see [Locked out](locked-out.md).

### Where is the code?

In a git repository: `https://github.com/betoespinosa14/proj-uem-bt` (the `origin` remote). The Go server is in `cmd/server` and `internal/server`, the agent in `cmd/agent` and `internal/agent`, the console in `web/`, deployment files in `deploy/`. See [Code layout](code-layout.md).

**To fill in:** who owns the GitHub repository and how a new maintainer gets access.

### Where is the API documented?

In `docs/API.md` in the repository. It lists every endpoint, the request and response shapes, and the permission rules.

### How do I deploy a fix?

On the Mac Mini, from the repository folder:

```bash
make deploy-mac                         # build server, agents and console, and stage them (no sudo)
sudo deploy/macos/install.sh --system   # swap in the new build and restart the services
```

The first step builds into `~/Library/Application Support/BEACON`. The second restarts server, console and Caddy; Postgres keeps running. Open browser tabs keep working. See [Releases](releases.md) for tagging versions.

### Do I need to restart anything after editing settings?

Yes. After editing `server.env`: `sudo launchctl kickstart -k system/com.beacon.server`. After editing the Caddyfile: the same with `com.beacon.caddy`. Settings changed in the console (**Settings** page) apply at once. See [Environment variables](environment.md).

### How do I check that everything is running?

```bash
sudo launchctl print system/com.beacon.server | grep state
curl -s https://beacon.itsthev.net/healthz      # prints "ok" when the server and database are up
```

`/healthz` returns "db unavailable" (HTTP 503) when Postgres is down. See [Server down](server-down.md).

### What happens to devices while the server is down?

They keep working normally; they just can't report. Each agent retries with a growing delay (at most about a minute and a half between tries) and reconnects by itself when the server is back. Commands sent while a device is offline wait in a queue and go out when it reconnects.

### What if the Mac Mini dies?

1. Get a replacement machine running ([Fresh install](fresh-install.md)).
2. Restore the newest database backup ([Restore the database](restore-database.md)). Backups live **on the Mac Mini's own disk**, so you need a copy taken off the machine.
3. Point the DNS name (and router port forward) to the new machine ([Network and DNS](network-dns.md)).

If the database is restored, agents reconnect with their existing credentials and nothing else is needed. If there's **no** backup, every device must be re-enrolled: delete the agent's `agent.json` on each device and run a new install command. See [Backups](backups.md) and [Known security gaps](known-gaps.md).

### How much does it cost to run?

The code has no paid services built in: no cloud, no licences, no email provider. Costs are outside the code.

**To fill in:** yearly cost of the domain, DNS, internet line and any hardware or support contracts.

### What should I do in my first week?

1. Work through the [Handover checklist](handover-checklist.md).
2. Make sure you can sign in as an owner and turn on two-step sign-in.
3. Copy the newest backup off the Mac Mini and test a restore somewhere safe.
4. Read [Known security gaps](known-gaps.md).
5. Check the **Audit log → Verify integrity** button reports an intact chain.
6. Fill in every "To fill in" line in [Contacts](contacts.md) and this Knowledge base.

### Who do I ask about BEACON?

See [Contacts](contacts.md).

**To fill in:** the previous maintainer's contact details and until when they're available.

### Which versions are running?

Settings → **Your account** shows "Latest agent on server". Each device's agent version is on the Devices list. Versions are calendar based (`2026.11.49` = 50th release of November 2026); see `CHANGELOG.md` and [Releases](releases.md).

### Can I run a copy for testing?

Yes. [Development](development.md) explains running Postgres, the server, the console and an agent locally. Don't point test agents at the production server.

### Where do I look when something breaks?

Start in the Troubleshooting section: [Device offline](device-offline.md), [Server down](server-down.md), [Agent not updating](agent-not-updating.md), [Webhook failures](webhook-failures.md), [HTTPS certificates](https-certificates.md). The server log is `~/Library/Logs/BEACON/server.log` (JSON lines).

### Is there a glossary?

Yes: [Glossary](glossary.md).
