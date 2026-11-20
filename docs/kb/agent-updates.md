---
title: Agent updates
section: Operations
order: 60
summary: How a new agent build gets from the repo to every device, how the self-update works, the one-time bootstrap for old agents, and how to see who runs which version.
reviewed: 2026-11-20
---

## The short version

1. A deploy (`make deploy-mac`) builds six agent binaries and `dist/manifest.json`, and stages them in `~/Library/Application Support/BEACON/dist/`.
2. The server reads the manifest on every request. If the manifest's version is newer than a device's agent, the console marks the device with **Update** (Devices list) or **Update available** (device page).
3. Someone with the `commands.run` permission (owner, admin or helpdesk) clicks **Update agents (N)** on the Devices list, or **⋯ → Update agent to …** on a device.
4. The server sends each device an `update_agent` command. The agent downloads its build, checks it, swaps it in and restarts.

Updates are not pushed automatically: nothing happens on devices until someone clicks Update. A new agent version only exists when agent code changed (see [Releases and deploying](releases.md)).

## The pieces

| Piece | File | Job |
|---|---|---|
| Build | `Makefile` target `build-agents` | Builds `uem-agent-{linux,darwin,windows}-{amd64,arm64}` (Windows with `.exe`) into `dist/`, stamped with `AGENT_VERSION`. |
| Manifest | `cmd/distmanifest`, `internal/agentdist` | Writes `dist/manifest.json`: the version, plus SHA-256 and size of each binary. |
| Download | `internal/server/api/downloads.go` | Serves `/downloads/<file>` and `/downloads/manifest.json` from `UEM_AGENT_DIST` (`dist` in production, relative to the BEACON folder). |
| Decision | `internal/server/core/agentupdate.go` | Builds the command for one device: `update_agent` for new agents, a bootstrap script for old ones. |
| Install | `internal/agent/update/update.go` | Downloads, verifies, test-runs and swaps the binary on the device. |
| Restart | `cmd/agent/main.go` | The agent exits with code 3; launchd, systemd or the Windows service manager starts the new binary. |

## What the server checks

`agentUpdate` in `internal/server/core/agentupdate.go` refuses the update when:

- there is no manifest ("no agent build is available on the server");
- the manifest has no build for the device's OS and architecture;
- the device is not behind (`agentdist.Outdated`). A between-release build such as `2026.11.6-3-gabc` counts as its release. A `-dirty` build counts as behind the same clean release.

**Update agents (N)** sends the command to every outdated device at once (up to 1000). Offline devices get it when they reconnect.

## What the agent does

In `internal/agent/update/update.go`:

1. Downloads `<server>/downloads/<file>` next to itself as `uem-agent.new` (`uem-agent.new.exe` on Windows), from its main server address (`server_url` in its config), even while connected through a fallback.
2. Checks the size and SHA-256 against the values the server sent. A mismatch stops the update.
3. Runs the new file with `version` and checks it prints the expected version. A failure stops the update.
4. Swaps it in. On Windows the running `.exe` is first renamed to `uem-agent.old.exe`.
5. Reports success ("Updated from X to Y; restarting"), waits 3 seconds, and exits so the service manager starts the new binary.

Any failure leaves the current agent untouched and shows as a failed command on the device's page. Leftover `.new` and `.old` files are cleaned up at the next start.

When the agent reconnects with a new version, the server records an `agent.updated` entry in the audit log (from and to).

## Old agents: the one-time bootstrap

Agents older than `2026.11.7` (`SelfUpdateSince` in `internal/agentdist/agentdist.go`) do not understand `update_agent`. For them the server sends a `run_script` command instead (`BootstrapScript`):

- macOS/Linux (`sh`): downloads the build into `/usr/local/bin`, checks the SHA-256 and that it runs, replaces `/usr/local/bin/uem-agent`, then restarts the service in the background (`launchctl kickstart -k system/uem-agent` or `systemctl restart uem-agent`).
- Windows (PowerShell): the same checks, then a one-off SYSTEM scheduled task named "BEACON agent update" restarts the `uem-agent` service.

The bootstrap download URL uses **Settings → Public server URL**, or `UEM_PUBLIC_URL`. If both are empty the server refuses ("set the public server URL in Settings first"). After this one update, the device updates itself like any other.

## Who runs which version

- **Dashboard → Security posture → Latest agent:** how many devices run the latest agent version.
- **Devices list:** the agent version column, with an "Update" marker next to outdated devices.
- **Device page:** the agent version with an "Update available" badge, and past updates in the device's command history.
- **The offered version:**
  ```bash
  curl -fsS https://beacon.itsthev.net/downloads/manifest.json | head -3
  ```
- **On the Mac Mini**, from the database:
  ```bash
  /opt/homebrew/opt/postgresql@16/bin/psql -h localhost -U uem uem -c \
    "SELECT agent_version, count(*) FROM devices WHERE retired_at IS NULL GROUP BY 1 ORDER BY 1"
  ```

Compliance rules can also require a minimum agent version, so outdated devices raise alerts.

## Manual update

Re-running the install command on a device always installs the server's current build and keeps the device's identity. See [Enrolling devices](enrollment.md). This also works for agents too broken to take a command.

## Related

- [Agent not updating](agent-not-updating.md)
- [Releases and deploying](releases.md)
- [Enrolling devices](enrollment.md)
