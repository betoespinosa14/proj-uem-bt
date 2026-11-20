---
title: Agent not updating
section: Troubleshooting
order: 60
summary: What to check when devices stay on an old agent version, from the server's agent builds to the update command's result on the device, and how to update by hand.
reviewed: 2026-11-20
---

Read [Agent updates](agent-updates.md) first for how updates work. Remember that updates are not automatic: someone must click **Update agents (N)** on the Devices list, or **⋯ → Update agent to …** on a device.

## 1. Is a newer agent actually offered?

The server offers whatever is in `~/Library/Application Support/BEACON/dist/manifest.json`.

```bash
curl -fsS https://beacon.itsthev.net/downloads/manifest.json | head -3
ls -l ~/Library/Application\ Support/BEACON/dist/
```

- The `version` must be newer than the device's agent version. If it is the same, there is nothing to update.
- A release that changed no agent code keeps the previous agent version on purpose (see [Releases and deploying](releases.md)). Check what the next build would report with `make agent-version` in the repo.
- If the manifest is missing, the console shows "No agent build published" on the Dashboard, and an update attempt fails with "no agent build is available on the server". Run `make deploy-mac` and `sudo deploy/macos/install.sh --system`.
- There must be a file for the device's platform, e.g. `uem-agent-darwin-arm64` or `uem-agent-windows-amd64.exe`. Otherwise the error is "no agent build for" followed by the OS and architecture.

## 2. What did the console say when you clicked Update?

| Message | Meaning |
|---|---|
| "… already runs agent …" | The device is not behind the offered version. |
| "none of the selected devices needs an agent update" | Same, for the bulk button. |
| "set the public server URL in Settings first" | An old agent needs the bootstrap script, which downloads from **Settings → Public server URL** or `UEM_PUBLIC_URL`. Both are empty. |

## 3. Is the device online?

The update is a command. If the device is offline, it waits in the queue and runs when the agent reconnects. Check the device's status and see [Device shows offline](device-offline.md).

## 4. Read the command's result

Open the device and look at its command history. The update command shows the agent's answer:

| Result | Meaning and fix |
|---|---|
| "Updated from X to Y; restarting" | It worked. The agent restarts about 3 seconds later. Give it a minute to reconnect. |
| "Already running Y" | Nothing to do. |
| "download failed: …" | The device could not download from `<its server address>/downloads/<file>`. The agent downloads from its main address, even while connected through a fallback. Check that address works from the device (`curl -fsS <address>/healthz`). |
| "download is damaged (size or checksum mismatch)" | The file changed between the manifest and the download (for example a deploy ran in between), or something on the network altered it. Retry. If it repeats, redeploy so the manifest and files match. |
| "the new agent did not run (…)" | The downloaded build failed to start on this machine (wrong architecture, blocked by security software). The current agent is untouched. Look at the text in brackets. |
| "could not replace the agent: …" | The file could not be swapped (permissions, disk full, on Windows a locked file). Retry; if it repeats, update by hand. |
| "updates are not supported when running in the foreground" | The agent was started by hand (`uem-agent run`), not as a service. |
| Command timed out or stays "sent" | The agent never answered. Check it is running and connected. |

Agents older than `2026.11.7` get a **Run script** command instead (the one-time bootstrap). Its output is shown like any script result. Typical failures: "Download is damaged (checksum mismatch)", "The new agent did not run", or `curl` missing on a Linux machine.

## 5. Did the agent restart onto the new version?

After a successful swap, the agent exits and the service manager starts the new file. If the console still shows the old version a few minutes later:

| OS | Check |
|---|---|
| macOS | `sudo /usr/local/bin/uem-agent version` and `sudo launchctl print system/uem-agent \| grep state` |
| Linux | `sudo /usr/local/bin/uem-agent version` and `systemctl status uem-agent` |
| Windows | `& "C:\Program Files\UEM Agent\uem-agent.exe" version` and `Get-Service uem-agent` |

If the file on disk is new but the service runs the old one, restart the service (`sudo launchctl kickstart -k system/uem-agent`, `sudo systemctl restart uem-agent`, or `Restart-Service uem-agent`).

On Linux, agents before the restart drop-in (`/etc/systemd/system/uem-agent.service.d/10-beacon-restart.conf`) wait 2 minutes before systemd restarts them. That is slow but normal.

Agent logs: macOS `/var/log/uem-agent.err.log`, Linux `journalctl -u uem-agent`. Look for `restarting on the updated agent`. Windows agents write no log file.

When the new version connects, the audit log gets an `agent.updated` entry with the old and new versions.

## 6. Update by hand

Re-run the install command on the device (**Enrollment → Install commands**, any active token). On an enrolled device the installer downloads the server's current build, keeps the device's identity, and restarts the service. This works even when the agent is too old or too broken to take commands. See [Enrolling devices](enrollment.md).

## Related

- [Agent updates](agent-updates.md)
- [Device shows offline](device-offline.md)
- [Releases and deploying](releases.md)
