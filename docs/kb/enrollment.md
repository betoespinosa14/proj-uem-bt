---
title: Enrolling devices
section: Operations
order: 50
summary: How to put the BEACON agent on a Windows, macOS or Linux computer, where it installs, and how to uninstall or re-enroll it.
reviewed: 2026-11-20
---

## How enrollment works

1. An owner or admin creates an **enrollment token** on the **Enrollment** page (`web/app/(console)/enrollment/page.tsx`).
2. The console shows a one-line install command per OS. The command downloads an installer script from the server (`/install.sh` or `/install.ps1`, embedded in the server from `internal/server/installers/`).
3. The script downloads the agent binary from `/downloads/`, runs `uem-agent enroll`, then installs and starts the agent as a system service.
4. On enroll, the server checks the token and gives the device its own random credential. Only its hash is stored. The device appears in the console within seconds.

## Enrollment tokens

When creating a token you set:

| Field | Meaning |
|---|---|
| Description | Required. Names the token in the list and in the audit log. |
| Enroll into group | Optional. New devices join this group. |
| Expires | In 1 hour, 24 hours, 7 days, 30 days, or Never. |
| Max devices | How many devices may use it. Blank means unlimited. |

A token stops working when it is revoked, expired or used up. Click **Install commands** next to an active token to see the commands again. The token is a secret: anyone who has it can enroll a device into the organization until it expires or is revoked. Prefer short expiries and revoke tokens you no longer need.

The server also limits enroll attempts to 30 per minute per IP address.

The address in the commands is **Settings → Public server URL**, or `UEM_PUBLIC_URL` if that is empty. In production it is `https://beacon.itsthev.net`.

## Install on Windows

Open PowerShell **as Administrator** and paste the Windows command from the console. It looks like this:

```powershell
powershell -ExecutionPolicy Bypass -Command "[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor 3072; & ([scriptblock]::Create((irm https://beacon.itsthev.net/install.ps1))) -Server 'https://beacon.itsthev.net' -Token 'enr_…'"
```

The first part turns on TLS 1.2 for older Windows PowerShell. The script picks `amd64` or `arm64` itself.

## Install on macOS or Linux

Open a terminal and paste the command from the console:

```bash
curl -fsSL https://beacon.itsthev.net/install.sh | sudo sh -s -- --server https://beacon.itsthev.net --token enr_…
```

It asks for an admin password (on macOS) and must run as root. It supports `x86_64` and `arm64`/`aarch64`. On Linux it works with systemd distributions such as Debian/Ubuntu and RHEL/Fedora.

## Where the agent lives

| | Windows | macOS | Linux |
|---|---|---|---|
| Binary | `C:\Program Files\UEM Agent\uem-agent.exe` | `/usr/local/bin/uem-agent` | `/usr/local/bin/uem-agent` |
| Config (holds the device secret) | `C:\ProgramData\UEM Agent\agent.json` (SYSTEM and Administrators only) | `/Library/Application Support/UEM Agent/agent.json` (mode 0600) | `/etc/uem-agent/agent.json` (mode 0600) |
| Service | Windows service `uem-agent` ("UEM Agent") | launchd daemon `uem-agent` (`/Library/LaunchDaemons/uem-agent.plist`) | systemd unit `uem-agent` (`/etc/systemd/system/uem-agent.service`) |
| Logs | no log file | `/var/log/uem-agent.err.log` | `journalctl -u uem-agent` |

The agent still uses the old name "UEM Agent" on disk. `UEM_AGENT_CONFIG` can point the agent at a different config file; installs never set it.

On Linux the agent also writes `/etc/systemd/system/uem-agent.service.d/10-beacon-restart.conf`, so systemd restarts it within 5 seconds instead of 2 minutes.

Useful agent commands (run as root / Administrator):

```bash
uem-agent version      # agent version
uem-agent status       # running or stopped
uem-agent inventory    # print what the agent collects, as JSON
```

## Re-running the installer

If the config file already exists, the installer keeps the device's identity: it replaces the binary, re-registers the service and starts it, without using the token. This is how you repair or manually upgrade an agent. The command still needs a `--token` value, but it is not checked, so any token's command works.

## Uninstall

From the console: open the device, **⋯ → Uninstall agent…**. The agent deletes its config, removes its service and stops. The binary file stays on disk. The device record stays in the console (offline) until you retire it.

To retire a device: **⋯ → Retire device…**. Its credential is revoked at once and it disappears from the fleet. History stays in the audit log. A retired device whose agent is still installed keeps trying to connect and is rejected (every 5 minutes).

By hand on the device:

```bash
sudo uem-agent stop
sudo uem-agent uninstall
sudo rm -f /usr/local/bin/uem-agent
sudo rm -rf "/Library/Application Support/UEM Agent"   # macOS
sudo rm -rf /etc/uem-agent                              # Linux
```

```powershell
& "C:\Program Files\UEM Agent\uem-agent.exe" stop
& "C:\Program Files\UEM Agent\uem-agent.exe" uninstall
Remove-Item -Recurse -Force "C:\Program Files\UEM Agent", "C:\ProgramData\UEM Agent"
```

## Re-enroll a device

Needed when a device was retired, when its config file was lost, or after a database restore that predates the device.

1. Remove the old config file on the device (path in the table above). Without this step, the installer keeps the old, rejected identity.
2. Run the install command from a valid token.
3. Enrolling always creates a **new** device record. Retire the old, offline record in the console.

## Related

- [Device offline](device-offline.md)
- [Agent updates](agent-updates.md)
- [Network and DNS](network-dns.md)
