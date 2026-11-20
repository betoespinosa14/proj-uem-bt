---
title: The agent
section: Architecture
order: 100
summary: What uem-agent does on each device, how it connects, what it collects and runs, and where it is installed.
reviewed: 2026-11-20
---

## What it is

`uem-agent` is one static Go binary built for Linux, macOS and Windows on amd64 and arm64 (`AGENT_TARGETS` in the `Makefile`). It runs as a system service, connects out to the BEACON server, reports what the device is and how it is doing, and carries out commands. It never listens on a port, so devices need no firewall changes.

Code: `cmd/agent/main.go` (entry point) and `internal/agent/*`.

## Install locations and service names

| | Linux | macOS | Windows |
|---|---|---|---|
| Binary | `/usr/local/bin/uem-agent` | `/usr/local/bin/uem-agent` | `C:\Program Files\UEM Agent\uem-agent.exe` |
| Config file | `/etc/uem-agent/agent.json` | `/Library/Application Support/UEM Agent/agent.json` | `C:\ProgramData\UEM Agent\agent.json` |
| Config protection | mode 0600 | mode 0600 | Folder limited to SYSTEM and Administrators (`icacls`) |
| Service manager | systemd (unit `uem-agent`) | launchd (label `uem-agent`) | Windows service `uem-agent`, display name "UEM Agent" |
| Restart on crash | `Restart=always`, plus drop-in `/etc/systemd/system/uem-agent.service.d/10-beacon-restart.conf` setting `RestartSec=5` | `KeepAlive` | restart on failure |

Sources: `internal/server/installers/install.sh`, `install.ps1`, `internal/agent/config/config.go`, `cmd/agent/main.go`, `cmd/agent/restart.go`. The service is created by the kardianos/service library, which decides the exact unit/plist file locations. `UEM_AGENT_CONFIG` overrides the config path.

The config file holds `server_url`, `device_id`, `device_secret` and `fallback_urls`. The secret is the device's password: treat the file as sensitive. See [Secrets](secrets.md).

## Commands on the device

Run as root / Administrator:

| Command | Does |
|---|---|
| `uem-agent status` | Prints running or stopped |
| `uem-agent stop` / `start` / `restart` | Controls the service |
| `uem-agent version` | Prints the agent version |
| `uem-agent inventory` | Prints what the agent collects, as JSON (handy for debugging) |
| `uem-agent enroll --server URL --token enr_…` | Enrolls and writes the config |
| `uem-agent install` / `uninstall` | Registers or removes the service |
| `uem-agent run` | Runs in the foreground (what the service runs) |

See [Enrollment](enrollment.md) for the install one-liners and [Device offline](device-offline.md) for troubleshooting.

## Connection

`internal/agent/conn/agent.go`:

1. Connects to `wss://<server_url>/api/agent/ws` with header `Authorization: Device <id>:<secret>`.
2. Sends `hello` with its version and basic facts. The server replies with the **policy**.
3. Sends a heartbeat every 30 s and pings every 30 s (drops the link if no pong within 10 s), so a dead network is noticed quickly.
4. Sends telemetry every 60 s and full inventory every 6 h (intervals come from the policy), and on a refresh command.
5. If disconnected, it retries with backoff from 1 s up to 1 minute, with jitter. If the server rejects its credentials (retired device), it retries only every 5 minutes.

### Moving to a new address

When the policy's `server_url` differs from the agent's, the agent checks `<new>/healthz`, reconnects there, and saves the new address only after that connection works. If it fails, it returns to the old address and doesn't retry the new one for 10 minutes (`internal/agent/conn/move.go`).

### Fallback addresses

The policy can carry `fallback_urls` (from `UEM_FALLBACK_URLS` on the server). After 3 failed connections in a row to the main address, the agent uses the first fallback whose `/healthz` answers. It never saves a fallback as its address, re-checks the main address every 5 minutes, and goes back as soon as it answers (`internal/agent/conn/fallback.go`). This helps with DNS problems, not with the server being down.

**To fill in:** the fallback address(es) currently set in `server.env` on the Mac Mini, and the certificate they use.

## What it collects

`internal/agent/collect`, one file per OS. Common data comes from gopsutil (CPU, memory, disks, network, processes).

| OS | Main sources |
|---|---|
| macOS | `sw_vers`, `system_profiler`, `fdesetup` (FileVault), `socketfilterfw` (firewall, stealth mode), `spctl` (Gatekeeper), `csrutil` (SIP), `softwareupdate` (pending updates), the application list, battery via `ioreg` and `system_profiler SPPowerDataType` |
| Linux | `dpkg-query`, `rpm`, `snap` (software), `lsblk` and dm-crypt check (LUKS), `ufw` / `firewall-cmd` / `nft` (firewall), `apt-get` / `dnf` (pending updates), `dmidecode`, `smartctl`, `nmcli` / `iwgetid` (Wi-Fi) |
| Windows | Registry Uninstall keys (machine and per-user) and Store apps (`Get-AppxPackage`), `manage-bde` (BitLocker), `Get-NetFirewallProfile`, Security Center (antivirus), CIM/WMI for BIOS, hardware and the signed-in user |

Run `uem-agent inventory` on a device to see the exact output.

## Commands it runs

`execute` in `internal/agent/conn/agent.go`, helpers in `internal/agent/exec`. Up to 4 commands run at once. A command ID already seen is never run again; the agent just re-reports its result.

| Command | What happens |
|---|---|
| `run_script` | Writes the script to a private temp file and runs it with the shell: `bash`/`sh`/`zsh` (not on Windows), `powershell` (`powershell.exe -ExecutionPolicy Bypass` on Windows, `pwsh` elsewhere if installed), `cmd` (Windows only). Timeout enforced; stdout/stderr capped at 1 MiB each. |
| `reboot`, `shutdown` | Reports success first, then schedules it (`shutdown.exe` on Windows, `shutdown` elsewhere) after the requested delay. |
| `lock` | Windows `tsdiscon console`; macOS `pmset displaysleepnow`; Linux `loginctl lock-sessions`. |
| `display_message` | Windows message box to active sessions; macOS dialog via `osascript` for the console user; Linux `wall`. |
| `refresh_inventory` | Sends a fresh inventory. |
| `uninstall_agent` | Deletes the config, removes the service and exits. |
| `update_agent` | Self-update, below. |

It also enforces the group's **app blocklist**: every 15 s it kills matching processes and reports an event that is audit-logged.

## User sessions and active time

A system service can't see which window is in front. So while usage tracking is on, the agent starts `uem-agent session-probe` inside each interactive session (macOS console user; Windows console and Remote Desktop sessions). The probe reports the front app and time since last input every 5 seconds over a private pipe (`internal/agent/session`). Linux has no probe.

## Usage sampling

`internal/agent/usage` and `trackUsage` in `conn/agent.go`:

- Only while the org has app usage tracking on (from the policy).
- Every minute, list running processes and map each to an installed app and user: that is **open** time.
- Probe samples add **active** seconds when the app was in front and input was within the idle threshold.
- Every 5 samples (about 5 minutes) send a `usage` report. If sending fails, entries are kept and sent later (up to 5000 entries).
- Only processes owned by a person count: system and service accounts, and Windows processes in session 0, are skipped (`usage.IsHuman`).

See [FAQ: App usage](faq-app-usage.md).

## Self-update

`internal/agent/update/update.go`:

1. The server sends `update_agent` with version, file name, SHA-256 and size from `dist/manifest.json`.
2. The agent downloads `<server>/downloads/<file>` next to itself as `uem-agent.new`, checks size and hash, and runs it once with `version` to prove it works.
3. It swaps the file in (on Windows the running exe is renamed aside to `.old`), reports success, and exits with code 3 so the service manager starts the new binary.
4. Any failure leaves the current agent untouched. Leftovers are cleaned on next start.

Agents older than 2026.11.7 can't do this; the server sends them a one-time bootstrap script instead (`core/agentupdate.go`). Updates are refused when the agent runs in the foreground. See [Agent updates](agent-updates.md) and [Agent not updating](agent-not-updating.md).

## Logs

The agent logs to standard error as text. Where that ends up depends on the service manager: `journalctl -u uem-agent` on Linux with systemd. On macOS and Windows no log file is configured in the code.

**To fill in:** where agent logs have been found in practice on macOS and Windows devices.
