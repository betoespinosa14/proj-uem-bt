---
title: Releases and deploying
section: Operations
order: 30
summary: How to version, test, tag and deploy a new BEACON release to the Mac Mini, and how to roll back.
reviewed: 2026-11-20
---

## Versions

BEACON uses calendar versions: `YYYY.MM.N`, where `N` counts releases within the month from 0. `2026.11.0` is the first release of November 2026. Each release is an annotated git tag such as `v2026.11.49`.

The version comes from `git describe` (see the top of the `Makefile`):

- A build of a tag reports e.g. `2026.11.49`.
- A build made after a tag reports e.g. `2026.11.49-3-g60f25a5` (3 commits later, at commit `60f25a5`), plus `-dirty` if there were uncommitted changes.

```bash
make version         # version the next server/console build reports
make agent-version   # version the next agent build reports
git tag --sort=-v:refname | head -5   # latest releases
```

### The agent version only changes when agent code changes

The `Makefile` keeps the agent on the last shipped version while nothing it is built from has changed. `AGENT_SRC` lists those paths: `cmd/agent`, `internal/agent`, `internal/protocol`, `internal/agentdist`, `go.mod` and `go.sum`. If `dist/manifest.json` already holds a version whose tag matches the current agent code (`SHIPPED_AGENT`), that version is reused. So a console-only release does not relabel identical agents, and devices do not all show "Update available".

The server binary is built with `-X main.Version=…`, but `cmd/server` has no `Version` variable, so the server does not report its own version anywhere. Use the git tag and `CHANGELOG.md` to know what is deployed.

## Changelog convention

`CHANGELOG.md` lists releases newest first. Each entry looks like this:

```markdown
## 2026.11.49 (2026-11-19): Short headline of the release

Server and console; agents stay on 2026.11.29.

### Fixed
- **Bold one-line summary.** Plain explanation for an IT reader.

### Changed
- ...

### Added
- ...
```

The line under the heading says which parts changed ("Console only", "Server and console", or a new agent version) and which agent version devices stay on. Mention anything the server does to the database on start-up (for example "The server adds a column to app usage on start-up.").

## Checklist before a release

1. Run the Go tests. They need a local `uem_test` database (see [Development](development.md)):
   ```bash
   make test
   ```
2. Type-check and lint the console:
   ```bash
   cd web && npx tsc --noEmit -p . && npm run lint && cd ..
   ```
   `npm run lint` also runs `scripts/check-grids.mjs`, which catches grids that break the phone layout.
3. Look at the change in a browser against a copy of the live data (see [Development](development.md)).
4. Write the `CHANGELOG.md` entry. The next number is the latest tag's `N + 1` in the same month, or `0` in a new month.
5. Commit everything. `make release` refuses to run with uncommitted changes.

## Release and deploy

Run these on the Mac Mini, in the repo folder.

1. Tag the release:
   ```bash
   make release
   ```
   It prints e.g. `Tagged v2026.11.50`. (It also says "Next: make build-agents"; `make deploy-mac` does that for you.)
2. Build and stage, as your normal user, without `sudo`:
   ```bash
   make deploy-mac
   ```
   This runs `deploy/macos/install.sh`. It builds the server and all six agent builds, runs `npm run build` in `web/`, and copies everything into `~/Library/Application Support/BEACON`. It ends with `Staged version 2026.11.50 (agents 2026.11.29)`. Nothing running has changed yet.
3. Install and restart:
   ```bash
   sudo deploy/macos/install.sh --system
   ```
   It swaps in the new console, restarts `server`, `web`, `caddy` and `backup`, and prints the state of each job. Postgres is left running.
4. Check:
   ```bash
   curl -fsS https://beacon.itsthev.net/healthz; echo
   curl -fsS https://beacon.itsthev.net/downloads/manifest.json | head -3
   tail -n 20 ~/Library/Logs/BEACON/server.log
   ```
   Open the console and check the pages you changed.
5. Push the commit and the tag to the remote:
   ```bash
   git push && git push --tags
   ```

The remote (`origin`) is a GitHub repository; `git remote -v` shows its address.

**To fill in:** which GitHub account owns the repository, and who else has access.

`make deploy-mac` needs `cd web && npm ci` to have been run at least once, because it only runs `npm run build`.

## What a deploy rebuilds and restarts

| Part | What happens |
|---|---|
| Server | Rebuilt. The new binary is renamed into place, and the job restarts. Database migrations run automatically on start. All agents disconnect and reconnect within about a minute. |
| Console | Rebuilt and swapped in. The previous console is kept in `web.prev/`. |
| Agents | All six builds and `dist/manifest.json` are rebuilt. The server reads the manifest on each request, so a new agent version is offered at once. Devices are **not** updated automatically; see below. |
| Caddy | Restarted with the same Caddyfile. |
| Backup | Started once, so a fresh dump is taken right after the deploy. |
| Postgres | Not restarted. |

If a plist in `deploy/macos/` changed, `--system` replaces the installed one and reloads that job instead of just restarting it.

## When agents update

When the new manifest has a newer agent version, devices on an older agent are marked **Update** on the Devices list and **Update available** on their page. An owner, admin or helpdesk user (anyone with the `commands.run` permission) starts the update with **Update agents (N)** on the Devices list, or **⋯ → Update agent to …** on one device. See [Agent updates](agent-updates.md).

## Open browser tabs

A tab opened before the deploy keeps working. The new console also carries the previous builds' script and style files for 14 days (`KEEP_OLD_STATIC_DAYS` in `deploy/macos/install.sh`), so old tabs can still load pages until they are reloaded.

## Rolling back

`install.sh` keeps only the previous console (`web.prev/`). It overwrites the server binary and the agent builds. So the normal rollback is to deploy the previous tag:

1. Check out the previous release:
   ```bash
   git checkout v2026.11.49    # the tag you want to go back to
   ```
2. Deploy it as usual:
   ```bash
   make deploy-mac
   sudo deploy/macos/install.sh --system
   ```
3. Go back to the main branch afterwards: `git checkout main`.

Notes:

- Migrations do not run backwards. Most migrations only add columns or tables, which an older server ignores. If a release changed the database in a way the old server cannot use, restore a backup from before the deploy ([Restoring the database](restore-database.md)).
- The post-deploy backup overwrites that day's dump file (`uem-YYYY-MM-DD.dump`), so the newest pre-deploy copy is usually the previous day's file. For a risky release, take a manual dump first (see [Backups](backups.md)).
- Agents are never downgraded. A rollback build with an older agent version simply offers no update.

For a console-only problem you can also put the previous console back by hand:

```bash
cd ~/Library/Application\ Support/BEACON
mv web web.bad && mv web.prev web
sudo launchctl kickstart -k system/com.beacon.web
```

## Related

- [The production server](production-server.md)
- [Development](development.md)
- [Agent updates](agent-updates.md)
