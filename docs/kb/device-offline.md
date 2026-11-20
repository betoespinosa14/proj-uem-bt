---
title: Device shows offline
section: Troubleshooting
order: 10
summary: How BEACON decides a device is online or offline, and what to check on the device, the network and the server when it stays offline.
reviewed: 2026-11-20
---

## How online and offline are decided

- The agent keeps one WebSocket open to `<server>/api/agent/ws`. When it connects and says hello, the server marks the device **online**.
- The agent sends a heartbeat every 30 seconds (`internal/agent/conn/agent.go`). Each heartbeat, telemetry or inventory message updates `last_seen_at`.
- Every 15 seconds the server marks devices offline whose last message is older than 90 seconds (`OfflineAfter` in `internal/server/scheduler/scheduler.go`). It also marks a device offline the moment its connection closes.
- When the server starts, it marks **every** device offline. They turn online again as agents reconnect, usually within a minute.
- Marking a device offline sends the `device.offline` webhook.

So "offline" means the server has not heard from the agent for 90 seconds. Commands sent meanwhile are queued and delivered when it reconnects.

## First: is it one device or all of them?

- **Most or all devices offline:** the problem is the server or the network, not the devices. Go to [Server down](server-down.md), and check DNS and the router ([Network and DNS](network-dns.md)).
- **One device:** carry on below.

## 1. Is the device on and online?

A laptop that is asleep, shut down or without internet shows offline. That is normal. Look at the device's "last seen" time on its page. The Dashboard's **Device health** card lists devices offline for over a day.

## 2. Is the agent running?

| OS | Check status | Restart |
|---|---|---|
| Windows (PowerShell as Administrator) | `Get-Service uem-agent` | `Restart-Service uem-agent` |
| macOS | `sudo launchctl print system/uem-agent \| grep state` | `sudo launchctl kickstart -k system/uem-agent` |
| Linux | `systemctl status uem-agent` | `sudo systemctl restart uem-agent` |

On any OS, `uem-agent status` prints `running` or `stopped` (`C:\Program Files\UEM Agent\uem-agent.exe status` on Windows).

If the service does not exist, the agent was uninstalled. Reinstall it (step 6).

## 3. Read the agent's log

| OS | Where |
|---|---|
| macOS | `sudo tail -n 50 /var/log/uem-agent.err.log` |
| Linux | `sudo journalctl -u uem-agent -n 50` |
| Windows | The agent writes no log file. Check **Event Viewer → Windows Logs → System** for the `uem-agent` service starting or stopping. |

What the messages mean:

| Message | Meaning |
|---|---|
| `connected to server` … `fallback=false` | Connected normally. |
| `connected to server` … `fallback=true` | Connected through a fallback address; the main address is failing. |
| `disconnected from server` with a DNS, timeout or certificate error | Network, DNS or HTTPS problem between device and server. |
| `server rejected device credentials (device may have been retired)` | The server does not know this device's credential: it was retired, or the database was restored to before it enrolled. The agent retries every 5 minutes. Re-enroll it (step 6). |
| `agent is not enrolled` | The config file is missing or incomplete. Re-enroll. |

## 4. Check the agent's config

| OS | Config file |
|---|---|
| Windows | `C:\ProgramData\UEM Agent\agent.json` |
| macOS | `/Library/Application Support/UEM Agent/agent.json` |
| Linux | `/etc/uem-agent/agent.json` |

Read it as root/Administrator. Check `server_url`: it should be `https://beacon.itsthev.net` (or an address that still works). `fallback_urls` lists the fallback addresses the server sent. Do not share the file: `device_secret` is the device's password.

## 5. Can the device reach the server?

From the device:

```bash
curl -fsS https://beacon.itsthev.net/healthz; echo     # prints ok
```

```powershell
(Invoke-WebRequest -UseBasicParsing https://beacon.itsthev.net/healthz).Content
```

- **Name does not resolve:** a DNS problem on that network. Inside the office, see the NAT hairpin note in [Network and DNS](network-dns.md). If `UEM_FALLBACK_URLS` is set, the agent switches to the fallback after about a minute (3 failed attempts) and comes back by itself later.
- **Timeout:** a firewall, captive portal or proxy blocks outgoing HTTPS. Agents need outgoing TCP 443 only.
- **Certificate error:** the device's clock may be wrong, or the server's certificate has a problem ([HTTPS certificates](https-certificates.md)).
- **`ok` but still offline:** check the server log for that device: `grep <device id> ~/Library/Logs/BEACON/server.log | tail`. The device ID is in the console's URL for the device.

## 6. Reinstall or re-enroll

- **Agent broken or missing, identity still good:** run the install command again from **Enrollment → Install commands**. If the config file exists, the installer keeps the device's identity.
- **Credentials rejected, or config lost:** delete the config file first, then run the install command. This creates a new device record; retire the old one.

Details are in [Enrolling devices](enrollment.md).

## Related

- [Server down](server-down.md)
- [Network and DNS](network-dns.md)
- [Enrolling devices](enrollment.md)
