# Changelog

Notable changes to the UEM project, newest first.

Versions use **calendar versioning**: `YYYY.MM.N`, where N counts releases within the month from 0. For
example, 2026.11.0 is the first release of November 2026. Each release is a git tag (`v2026.11.0`), and agents
report this version in the console. A build made between releases reports e.g. `2026.11.0-3-g60f25a5`: 3
commits after 2026.11.0, at commit `60f25a5`.

## Unreleased

### Added
- **Two-step sign-in for the console (optional per user).** Under your name → **Account security**, scan a QR code
  with an authenticator app; from then on sign-in asks for the 6-digit code after the password, with 10 one-time
  recovery codes for a lost phone. "Remember this browser for 30 days" skips the code on a trusted browser. Owners and
  admins can reset it for someone from **Users**, and `uem-server reset-mfa <email>` does it on the server as a last
  resort. Users shows a **2-step** badge for people who have it on. Every change is in the audit log.
- **Nightly database backup on the office Mac Mini.** A new `com.beacon.backup` launchd job saves the whole
  database at 02:30 every night (and once after each deploy) to `~/Library/Application Support/BEACON/backups`,
  keeping 14 days. The README explains how to restore.

### Changed
- **Device page tabs use the same selected colour** as the pickers and sidebar: the current tab (Overview, Hardware,
  Usage…) has accent-coloured bold text and an accent underline.
- **App usage opens on Today** (fleet and per-device), instead of 30 days.
- **The selected option in pill pickers is now easy to see.** App usage's Today / 7 days / 30 days / 90 days / 1 year
  (and the other pill pickers on device, alert, compliance and task pages) mark the chosen option like the sidebar's
  current page: accent-coloured bold text on an accent tint with a thin outline, in light and dark mode.
- **Tests use their own database.** `make test` now runs against `uem_test` instead of the live `uem` database,
  and the integration tests refuse to start unless the database name ends in `_test`. Test runs no longer add
  organisations to production or apply new database changes to it before a deploy.

## 2026.11.14 (2026-11-11): Service accounts out of App usage

### Fixed
- **Service accounts no longer show up in App usage.** On Windows the agent skips processes in session 0, where
  services run and no one signs in, so accounts such as Reckon's `QBDataServiceUser28` stop appearing as users
  (with Reckon open around the clock). Every real sign-in, Remote Desktop included, has its own session and still
  counts. On Linux, shells and the per-user service manager (`bash`, `systemd`, `sudo`, `tmux` and similar) are
  no longer listed as apps; they run for every login or SSH session.

## 2026.11.13 (2026-11-11): Fallback server addresses for agents

### Added
- **Fallback server addresses for agents** (`UEM_FALLBACK_URLS`, comma-separated). The server sends them in
  each device's policy and agents save them. If the main address fails 3 times in a row (about a minute), for
  example because its DNS name stops resolving, the agent connects through the first fallback that answers its
  health check, re-checks the main address every 5 minutes, and goes back to it as soon as it answers. The main
  address is never replaced.

## 2026.11.12 (2026-11-11): Linux agents back within seconds after an update

### Fixed
- **Linux devices showed "offline" for about 2 minutes after a console agent update** (and after any agent
  crash). The systemd unit written by the service library hard-codes `RestartSec=120`. The agent now keeps a
  drop-in, `/etc/systemd/system/uem-agent.service.d/10-beacon-restart.conf`, with `RestartSec=5`, written on
  install and on every start (so existing devices get it with this update) and removed on uninstall. The update
  to this version still takes 2 minutes on Linux; later ones come back in seconds.

## 2026.11.11 (2026-11-11): App usage Today and 7 days, Chrome usage on Windows

### Added
- **"Today" and "7 days" periods in App usage**, on the fleet page and each device's Usage tab, so Top users
  and every other card can show today's or this week's activity. The per-day chart is hidden for Today, and a
  device's weekday chart needs 30 days or more.
- **Employee name on devices.** The "Edit name, group & tags" dialog has a required Employee name field. The
  display name is now generated as `<group> | <employee name>`, or just the employee name without a group, and
  it follows the group: renaming a group renames its devices. Existing devices took their employee name from the
  part after " | " in their old names, and their names were regenerated from their current groups.
- **The Devices list shows the employee** in place of the signed-in account (hover to see that account). The
  device page shows both, device search matches employee names, and the devices CSV has an `employee_name`
  column.
- **App usage summary on a device's Overview:** active hours per day and the top 5 apps for the last 30
  days, with a link to the Usage tab. It's hidden when tracking is off, the device has no recorded use, or it
  runs Linux.
- **A device's Usage dashboard shows active hours by weekday** in place of Top users, which mostly repeated the
  employee's name: the average active time for each day of the week, from the device's first report in the
  period. The App usage page keeps Top users.
- **App usage "Top users" shows employee names** instead of signed-in accounts: each account is shown as the
  employee of the device it was most active on, with the account on hover. Accounts without an employee name
  show as before.

### Fixed
- **Google Chrome usage on Windows wasn't counted when Chrome was installed with Google's enterprise MSI.** That
  installer leaves Chrome's uninstall entry without an install folder, so its running programs couldn't be matched.
  The agent now takes a missing folder from the Windows "App Paths" registry when the registered program's product
  name equals the app's name. Usage is counted from the agent update on.

## 2026.11.10 (2026-11-10): Per-device App usage, Windows Store and Office apps

### Added
- **Each device's Usage tab has the full App usage features,** for that device only: the dashboard (headline
  figures, active hours per day, top apps and users, active vs open, background-only processes), app search,
  the "Not used" list with its reclaim export, and the Infrastructure badges and marking. Marking an app as
  infrastructure from a device page still applies to every device. The App usage endpoints accept
  `device_id=` for this.
- **Windows: app usage and installed software now include Microsoft Store apps, apps installed for one user,
  and each Office program.** Usage used to count only apps in Windows' machine-wide installed list, matched
  by folder. Store apps (the new Teams, WhatsApp, Spotify) and per-user installs were never counted, and Word,
  Excel, PowerPoint and Outlook all counted as "Microsoft 365 Apps for enterprise". Each now has its own name.
  Store apps keep matching after they update themselves. They show "Microsoft Store" as their source, and
  Microsoft's own Store apps (Calculator, Photos) count as part of Windows. Needs the agent update.

### Changed
- **Games that come with Windows aren't flagged.** Microsoft's own Store apps, such as Solitaire & Casual
  Games, Minesweeper, Mahjong, the Xbox app and Game Bar, are exempt from the "No gambling apps or games" check,
  like the built-in Mac apps. Games people install themselves are still flagged, from the Store or otherwise.

## 2026.11.9 (2026-11-10): Phone layout, battery display, Windows install TLS

### Fixed
- **The console fits phone screens.** Card rows on most pages (Dashboard, device page, App usage, Enrollment,
  Settings, Users, Scheduled tasks, Compliance, Scripts) could grow to fit one long line, such as an alert
  listing several games. That made the page wider than the screen, and the phone shrank the whole page. Long
  lines now end in "…". `npm run lint` checks that new layouts keep this rule.
- **Devices list on phones:** status, compliance and last seen appear under each device's name instead of in
  columns cut off at the screen edge. On a device's page, the system and last-seen line wraps instead of running
  off the screen.
- **Battery shows at most 100%,** and its bar reads like a charge: green when full, amber at 40% or below, red at
  20% or below. It used to be red when full, like a usage bar. Windows agents also cap a reading above 100%.
- **The Windows install command works on older Windows.** Built-in PowerShell on older Windows 10 and Windows
  Server doesn't offer TLS 1.2, so the command failed with "Could not create SSL/TLS secure channel" before
  anything installed. The command on the Enrollment page now enables TLS 1.2 first. The installer adds TLS 1.2
  to the allowed protocols instead of replacing them, so TLS 1.3 stays available.

## 2026.11.8 (2026-11-10): Agent update fixes

### Fixed
- **The one-time agent update now restarts laptops that are on battery.** Windows scheduled tasks start only on
  mains power by default, so a laptop on battery got the new agent file but kept running the old agent until it
  was plugged in. The restart task is now allowed to run on battery.
- **Retrying an agent update after a missed restart works.** If the new agent is already in place, the script
  just restarts the service. On Windows, an older copy that is still running is moved aside under another name
  instead of failing with "Cannot create a file when that file already exists". The agent removes these copies
  when it starts.

## 2026.11.7 (2026-11-10): Update agents from the console

### Added
- **Update agents from the console.** No more rerunning the install command on each device.
  - Devices on an older agent show **Update available** on their page, and an **Update** marker in the new
    Agent column of the Devices list.
  - Update one device with **⋯ → Update agent**, a selection with **Actions → Update agent**, or every outdated
    device with **Update agents (N)**.
  - **How the agent updates itself:** it downloads the new build from BEACON, checks its size and sha256 against
    `dist/manifest.json` (now written by `make build-agents`), runs it once to confirm it works, swaps it in and
    restarts. A failed check leaves the current agent untouched.
  - **The audit log** records each version change as `agent.updated`.
  - **Agents older than this release** are replaced once by a script sent through Run script, and keep their
    enrollment.
  - **Settings** shows the latest agent on the server.

### Fixed
- **Browser tabs left open through an update keep working.** Before, the update removed the old console's
  script files, so a tab opened earlier could end up with sidebar links that did nothing until it was closed
  and reopened. The deploy now also serves the previous versions' static files for 14 days
  (`KEEP_OLD_STATIC_DAYS` in `deploy/macos/install.sh`).

## 2026.11.6 (2026-11-10): New logo, link previews, MacBook battery

### Added
- **Link previews.** Sharing https://beacon.itsthev.net in a chat or email app shows a card with the BEACON
  logo, the name and "Endpoint oversight". It shows no device or company data. The address comes from
  `BEACON_PUBLIC_URL` at build time (default https://beacon.itsthev.net).

### Fixed
- **MacBooks report their battery again.** The agent read the Mac battery through a library that depends on
  registry fields newer macOS versions no longer provide, so MacBooks showed no battery at all. The agent now
  reads the battery itself: the charge, plus health as full-charge capacity against design capacity. When the
  registry has no capacities in mAh, it falls back to macOS's own "Maximum Capacity" from System Information.
  It handles both Apple Silicon and Intel. On Windows and Linux, a partial battery reading now still reports
  the charge instead of no battery. MacBooks need the 2026.11.6 agent.

### Changed
- **BEACON has its own logo:** a teal ribbon folded into a shield, a nod to The V's folded-ribbon logo, with
  the signal light in the middle. It's used in the sidebar, the login page, the browser tab and the phone
  home-screen icon. It's a flat vector in the company teals, and the dot follows the theme (dark green on light,
  neon on dark). The artwork is in `web/public/logo.svg` and `logo-dark.svg`.

## 2026.11.5 (2026-11-09): Display polish

### Changed
- **Offline devices stand out on the Dashboard.** In "Devices by operating system", the offline part of each bar
  and its legend dot are now amber (the same color as the Offline headline number) instead of grey, and a
  non-zero offline count is shown in bold.
- **Devices list: CPU, Mem and Disk each have their own header label**, lined up over their columns, instead of
  one "CPU / Mem / Disk" title that didn't line up with the numbers.
- **Sidebar:**
  - Fleet is ordered Dashboard, Devices, Enrollment, Groups, App usage, following the order you work in.
  - Audit log has a history icon, so it no longer looks like Scripts.
  - In the collapsed sidebar, the lines between Fleet, Policy and Administration are easier to see, and each
    icon's tooltip shows its section above the page name.
  - The green marker for the current page sits 4px left of the highlighted item, in both collapsed and expanded
    mode, instead of on the screen edge.
  - **Light mode has a light sidebar.** The sidebar and the frame around the page are a soft grey-blue with dark
    slate text, instead of the dark teal slab. The current page is a white highlight with dark green text and a
    dark green marker (neon is unreadable on light colors). Dark mode is unchanged, and the login page stays dark.
  - The section headings and the "Endpoint oversight" line are brighter in both themes, so they meet the 4.5:1
    contrast minimum.

## 2026.11.4 (2026-11-09): App usage dashboard

### Added
- **A Dashboard view on App usage**, next to the existing table. It opens first and remembers your last choice.
  - **Headline numbers:**
    - active time;
    - share of open time actually used;
    - apps used;
    - active users;
    - **Installed, not used**, with a "Review list" action.
  - **Active hours per day:** days with no data are shaded, and a "First report" marker shows where reporting began.
  - **Top apps and top users** by active time. Users show which machine they used; there's no percentage score next to people. Clicking an app opens the table filtered to it.
  - **Active vs open:** apps people used, sorted by idle hours.
  - **Running in the background, never used:** processes nobody touched (daemons, helpers, remote-access hosts, service accounts), with a "Third-party only" switch.
- **Background-only processes are kept out of usage numbers.** A (device, app, user) with no active time in the period is listed separately instead of inflating open time. This applies to bash and systemd on Linux, Adobe helpers, drivers, and apps under service accounts.
- **Mark as infrastructure:** admins can mark apps that are open by design, such as Parsec, so their open time isn't ranked as idle. Restore puts them back.
- **License reclaim export** (`/api/usage/reclaim.csv`): one row per unused third-party app per device, with publisher, version, device user and last use.
- **Group filter on App usage:** it narrows the dashboard, the table and every export to one device group, e.g. a single department.
- An **Infrastructure** badge marks those apps in the App usage table.
- API: `/api/usage/summary` and `/api/usage/infrastructure`. Migration `00004` adds the infrastructure list.

### Fixed
- **Deploying no longer breaks the running console.** The new console is staged beside the live one and swapped in at the restart. Before, the pages lost their styling until the restart.

## 2026.11.3 (2026-11-09): Commands survive a network switch

### Fixed
- **A command sent while a device was switching networks stayed "Sent" forever.**
  - If the device reconnected before the server noticed its old connection was gone, a command sent on the old connection was never delivered again. Seen with a Refresh sent while the iMac moved from Wi-Fi to cable.
  - The server now re-sends any command still marked sent whenever an agent connects.
  - The agent recognises commands it already has, so nothing runs twice.
  - Server-only fix: agents don't need reinstalling.

## 2026.11.2 (2026-11-09): Starts at boot; agents recover quickly after switching networks

### Added
- **BEACON starts at boot on the office Mac Mini** (`deploy/macos`, `make deploy-mac`). Postgres, the API server, the console and Caddy run as launchd daemons that start without a login and restart if they crash. This also keeps the IP certificate renewing.

### Fixed
- **Agents reconnect within about 40 seconds after a device changes networks** (cable to Wi-Fi, office to home).
  - The agent now pings the server every 30 seconds and drops a connection that doesn't answer within 10 seconds.
  - Before, a connection that died silently was only noticed when the operating system gave up on it, which takes minutes. The device showed Offline until then.
  - Only agents installed from this release have the fix.

### Deployment (office FortiGate, not in the repo)
- **Office Wi-Fi (192.168.131.x) can reach `https://122.49.212.51`.**
  - It needs a Wi-Fi → port1 policy with destination VIP `UEM-HTTPS-ALL` and NAT, like the wired LAN's policy 39.
  - Without it, devices on Wi-Fi could open `https://192.168.130.7` but their agents couldn't connect, so they showed Offline.

## 2026.11.1 (2026-11-07): BEACON rebrand

The console is renamed **BEACON** (Business Endpoint Authorization, Configuration and Oversight Network) and
restyled in the company colors.

### Changed
- **New look:**
  - the company logo and teal palette;
  - Neon Jade (#83FD80) used as a signal color;
  - the Plus Jakarta Sans font;
  - a dark icon rail with a collapse control at the bottom;
  - a rounded frame with a top bar, user menu and live indicator;
  - outlined cards, with a 1px #2A3B43 border in dark mode;
  - small transitions between pages.
- **Dashboard:**
  - headline numbers beside a device search box;
  - devices per OS shown as horizontal online/offline bars, each linking to the filtered device list;
  - a "Failing compliance checks" list replaces the compliance donut.
- **Tables** are about 20% denser across the console.
- The devices page accepts `?q=` and `?os=` in the URL.

### Added
- `/api/dashboard` returns `by_os[].online` and `failing_rules` (see `docs/API.md`).

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
