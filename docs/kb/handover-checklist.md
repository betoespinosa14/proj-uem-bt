---
title: Handover checklist
section: Start here
order: 20
summary: The accesses a new maintainer must receive and the checks to run in the first week.
reviewed: 2026-11-20
---

## How to use this list

Work through it with the current maintainer before they leave. Tick each box only when you have tried the access yourself, not when someone says you have it. Never write passwords into this article or the repository: keep them in the password manager.

## 1. Accesses to receive

### Code

- [ ] Read and push access to the GitHub repository.
- [ ] You can clone it and run `make test` (see [Development](development.md)).

**To fill in:** the GitHub repository URL and the organization or account that owns it.

**To fill in:** who can grant GitHub access if the current owner leaves.

### The Mac Mini (production server)

- [ ] Physical access to the Mac Mini in the office.
- [ ] An admin (sudo-capable) macOS account on it. The BEACON daemons run as one user account, from `~/Library/Application Support/BEACON` (see `deploy/macos/install.sh`).
- [ ] Remote access to it (screen sharing or SSH), if used.

**To fill in:** who holds the Mac Mini's admin login.

**To fill in:** which macOS user account the BEACON daemons run as.

**To fill in:** how the Mac Mini is reached remotely (Screen Sharing, SSH, or not at all).

### BEACON itself

- [ ] An **owner** account in the console at https://beacon.itsthev.net. Only owners can change Settings and grant the owner role (`internal/server/auth/auth.go`).
- [ ] At least two owners exist, so one lost phone or departure can't lock everyone out.
- [ ] Two-step sign-in is on for your account, and your recovery codes are stored safely.

**To fill in:** who the current owners are.

### Domain and DNS

- [ ] Access to the registrar account for `itsthev.net`.
- [ ] Access to the DNS host that serves the `beacon` record.

**To fill in:** the domain registrar and who holds that login.

**To fill in:** the DNS host and who holds that login.

**To fill in:** the domain's renewal date and whether auto-renew is on.

### Office network

- [ ] Admin access to the office router or firewall, which forwards TCP 80 and 443 to the Mac Mini (see the README section "Running on a single machine behind office port forwarding").
- [ ] Knowledge of the Mac Mini's reserved LAN IP.

**To fill in:** who administers the office router and where its login is kept.

**To fill in:** the ISP, the office's public IP, and whether it is static.

### Password manager

- [ ] Access to the shared vault that holds the items above.

**To fill in:** which password manager and vault hold the BEACON items.

## 2. Things to verify in week one

### Backups are running

1. On the Mac Mini, list `~/Library/Application Support/BEACON/backups/`. You should see one `uem-YYYY-MM-DD.dump` per day, up to 14 days (`deploy/macos/backup.sh`).
2. Read `~/Library/Logs/BEACON/backup.log` for errors.
3. Check that a copy exists somewhere other than the Mac Mini's disk.
4. Do one test restore into a scratch database. See [Backups and restore](backups.md).

**To fill in:** where off-machine backup copies are kept, and how often they are made.

### The services are healthy

```bash
for l in postgres server web caddy; do sudo launchctl print system/com.beacon.$l | grep -E '^\s*state ='; done
curl -fsS https://beacon.itsthev.net/healthz
```

See [Production server](production-server.md).

### The release flow works

- [ ] `make version` prints the next version.
- [ ] You understand `make release` (tags `vYYYY.MM.N`) and `CHANGELOG.md`. See [Releases](releases.md).

### You can deploy

- [ ] Run `make deploy-mac` then `sudo deploy/macos/install.sh --system` on the Mac Mini, with the current maintainer watching.
- [ ] After the deploy, the console loads and devices come back online.
- [ ] You know how to roll back (the previous console is kept as `web.prev`; see [Releases](releases.md)).

### Agents can update

- [ ] On **Devices**, you can see which devices show **Update available**, and you know how to update one. See [Agent updates](agent-updates.md).

### Security basics

- [ ] Two-step sign-in is on for every owner and admin.
- [ ] `UEM_ALLOW_SIGNUP=false` is set in `server.env`, so strangers can't create an organization.
- [ ] You know the last-resort MFA reset command. See [Locked out](locked-out.md).
- [ ] You have read [Secrets](secrets.md).

### Read these

- [ ] [What BEACON is](overview.md)
- [ ] [Architecture](architecture.md)
- [ ] [Known gaps](known-gaps.md)
- [ ] [Limitations and roadmap](limitations-roadmap.md)

## 3. Sign-off

**To fill in:** handover date, outgoing maintainer, incoming maintainer.

**To fill in:** how long the outgoing maintainer is available for questions afterwards.
