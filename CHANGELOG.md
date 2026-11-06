# Changelog

Notable changes to the UEM project, newest first.

Versions use **calendar versioning**: `YYYY.MM.N`, where N counts releases within the month from 0. For
example, 2026.11.0 is the first release of November 2026. Each release is a git tag (`v2026.11.0`), and agents
report this version in the console. A build made between releases reports e.g. `2026.11.0-3-g60f25a5`: 3
commits after 2026.11.0, at commit `60f25a5`.

## 2026.11.0 (2026-11-06): Monitoring devices outside the office

Devices now report to Sentinel from anywhere, not just the office network.

### Added
- **Agents move to a new server address on their own** (`60f25a5`).
  - The server announces **Settings → Public server URL** in the agent policy.
  - The agent checks the new address (health check, then a real connection with its credentials) and only
    saves it once that connection works.
  - If the address is wrong or unreachable, the agent keeps its current one and retries after 10 minutes.
  - Future moves, such as to a DNS name or a cloud server, are a Settings change; no reinstall is needed.
- **Calendar versioning** for agent builds (this release), with `make version` and `make release`.

### Changed
- `deploy/Caddyfile` upstreams are configurable (`UEM_SERVER_UPSTREAM`, `UEM_WEB_UPSTREAM`), so Caddy can run
  directly on a host instead of in Docker.

### Deployment (office Mac Mini, not in the repo)
- **Public address `https://122.49.212.51`**, with a FortiGate port forward of 122.49.212.51:443 to the Mac
  Mini (192.168.130.7:443).
  - It uses a separate public IP from the office block because 122.49.212.50 is fully forwarded to the
    media server.
  - **Hairpin NAT** lets office devices use the same address, through one VIP on interface "any" and a
    port1 → port1 policy with NAT.
- **Caddy** serves HTTPS with a short-lived Let's Encrypt certificate for the IP address, renewed
  automatically and validated over port 443 (port 80 is not forwarded).
  - This is temporary: a permanent or cloud setup should use a DNS name.
- The server runs with sign-up disabled, secure cookies, and trusted proxy headers.
- The console runs as a production build behind Caddy.
- **All 4 devices were reinstalled** and connect through the public address.
  - The iMac was tested on a phone hotspot, including a remote lock.

### Known open items
- Windows PowerShell 5.1 needs TLS 1.2 enabled for the HTTPS install command. The generated command doesn't
  do this yet.
- If an agent can't reach its current address, it doesn't try the announced one. This matters only for
  laptops, if the hairpin NAT ever breaks.

## 2026.10.2 (2026-10-29): Application usage tracking

Answers the IT audit question "is a subscribed product actually being used, and by whom?"

### Added
- **Application usage tracking** (`3d7697c`). Opt-in per organization in **Settings**, off by default, and
  audit-logged when changed.
  - Agents record which installed apps each user runs (**open time**), sampled once a minute.
  - New **Usage** tab on the device page, a **Last used** column in the Software tab, and a fleet-wide
    **App usage** page.
  - The **Not used in [period]** filter finds installed third-party apps nobody has used, i.e. licenses
    that could be reclaimed.
  - CSV exports for device and fleet usage. Data is kept for 400 days.
- **Active usage time** (`1221b80`): time an app is in front *and* the user is typing or using the mouse.
  - It is measured by a small helper the agent starts in each user session (the macOS console user;
    Windows console and Remote Desktop sessions). The helper reads only the front app and the input idle
    timer: never keystrokes, window titles or screen contents.
  - The idle threshold is configurable in Settings, from 1 to 15 minutes (default 2). Video playback
    without input stops counting once the threshold passes.
  - A **By app / By app and user** view on both usage pages shows total active hours per app across users,
    and how many devices each app is used on versus installed on.
  - Open time is still shown next to active time.
- **In-place agent upgrades**: re-running the install command on an enrolled device now upgrades the agent
  and keeps its device record. Before, it enrolled a duplicate device and then failed.

### Fixed
- Active usage on **macOS 13 (Ventura)** (`7819c0c`). Its `lsappinfo` prints the front app as
  `"LSBundlePath"="…"`, which wasn't recognised, so no active time was recorded. Found by running the
  probe on the ITSD iMac. The agent now also logs a warning when the probe can't see the front app,
  instead of silently recording zero.

### Changed
- The Export CSV buttons share one `ExportLink` component, and list pages share the `useDebounced` hook.
- README and `docs/API.md` cover the new endpoints, settings and limitations, as well as the
  2026-10-28 features below.

### Verified
- macOS: active time is recording on ITSD iMac (macOS 13, agent `7819c0c`) and ITSD Mac Mini M4 (macOS 27).
- New unit tests cover the macOS probe output (both formats), the idle threshold and sleep gaps, and
  process-to-app mapping. API integration tests cover usage on/off, by-app totals across users,
  installed-on counts, CSV columns and the idle-threshold limits.

### Known open items
- **Windows active time has not been confirmed yet.** Sprout Server Office reports open time but no active
  time so far. Either nobody was using it, or the Windows helper doesn't work yet. It also still runs an
  older agent and needs the installer re-run.
- Windows service accounts (e.g. `QBDataServiceUser28`) are counted as users in open time.
- Agents still don't update themselves: re-run the install command to upgrade.

## 2026.10.1 (2026-10-28): Software insights and reports (`b66694a`)

### Added
- Installed apps labelled **System** or **Third-party**, with a filter in the Software tab.
- **Gambling app and game flagging** from a built-in catalog: badges, a warning banner, and
  `software.gambling` / `software.games` compliance fields with a ready-made rule that raises alerts.
- **CSV reports**: the device list (respects filters) and a device's installed software.

## 2026.10.0 (2026-10-27): First version (`70a1923`)

The Go server, the cross-platform agent (Windows, macOS, Linux) and the web console: inventory, compliance
rules and alerts, remote commands and scripts, groups, scheduled tasks, audit log and webhooks.
