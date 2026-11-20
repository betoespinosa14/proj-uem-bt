---
title: FAQ: App usage
section: FAQ
order: 60
summary: What Active, Open, Users, Not used and Last active mean in App usage, how the agent measures them, and how the reclaim CSV works.
reviewed: 2026-11-21
---

### What is App usage for?

License and subscription reviews: which installed apps people actually use, and which paid apps sit unused and could be reclaimed. It's the **App usage** page, and the same view for one device on each device page. Code: `internal/agent/usage/` (measuring), `internal/server/store/usage.go` and `internal/server/api/usage.go` (totals and exports), `web/components/usage-explorer.tsx` and `usage-dashboard.tsx` (console).

### Is it on?

Only if an owner turned it on: **Settings → Application usage tracking**. It's **off by default**, because it is employee monitoring. Inform staff before turning it on. Turning it on or off is audit-logged (`org.update`). When it's off, agents stop collecting and the server drops any reports still in flight; existing data stays.

### What exactly does the agent record?

Two things, per day, per app, per user account:

1. **Open time:** once a minute the agent lists running programs, maps each to an installed app, and adds one minute to every app that's running for that user.
2. **Active time:** a small helper in each signed-in session reads which app is in front and how long since the last keyboard or mouse input. Time counts as active while that's under the idle threshold (Settings → "Count as idle after", 1 to 15 minutes, default 2).

It never records keystrokes, window titles or screen contents. Processes owned by system and service accounts (root, SYSTEM, `_`-accounts on macOS, Windows services) are ignored.

### What do the columns mean?

| Column | Meaning |
|---|---|
| **Active** | Time the app was in the foreground while the user was using the keyboard or mouse. |
| **Open** | Time the app was running, whether or not anyone used it. |
| **Users** | People (accounts) who **actively** used it in the period. Apps that only ran in the background don't count. |
| **Days used** | Days with keyboard or mouse use in the period. |
| **Last active** | When someone last used it with keyboard or mouse. "Never" if no one has since tracking began. |
| **Used on / installed on** | Devices where someone used it in the period, out of the devices it's installed on. |
| **Installed** | Whether the app is in that device's installed software list. |

### Why is Active so much lower than Open?

Apps stay open for hours while people work in something else. Active is the better measure of real use. On macOS, menu-bar and login-item apps count as open while they run, which inflates Open further.

### What counts as "used"?

In the Table, an app counts as used by a person on a device only if it had **active** time. An app that only ran in the background isn't "used".

**Linux exception:** Linux has no active-time helper, so there **any open time counts as use**, and Last active is the last time it was open.

### What is "Not used"?

Turn on **Not used** in the Table. It lists installed apps with **no active use in the period** you picked (Today, Yesterday, 7, 30, 90 days or 1 year). It's always:

- **third-party only** (the "Third-party only" switch is locked on), because OS built-ins aren't licensed products;
- **without helper components**: updaters, crash reporters, installers, runtimes and redistributables, WebView2, licensing helpers, and items named "… helper", "… service" or "… agent" (`IsComponent` in `internal/server/compliance/catalog.go`). Nobody opens those, so "not used" means nothing for them;
- sorted by **Last active**, newest first;
- limited to devices that aren't retired.

The dashboard's unused count follows the same rule.

### What does the "Third-party only" switch do?

It hides apps that come with the operating system and hardware drivers: Apple-signed macOS apps, Microsoft's own Windows apps (but not Office, Teams and other separately sold products), Linux distribution packages, and Realtek/driver software. An app counts as third-party if **any** install of it is (Edge is built into Windows but hand-installed on Macs), or if it isn't in the inventory at all. See [FAQ: Compliance](faq-compliance.md) for the full rule.

### What does the "Not installed" badge mean?

The app was used on that device but isn't in the device's installed software list. Usually it was uninstalled after use, runs without an installer (a portable app), or was installed only for one Windows user in a way the inventory doesn't see. It is also counted as third-party, since BEACON can't tell where it came from.

Safari used to show this badge on every Mac: macOS 13 and later keep Safari in a separate system folder (`/System/Cryptexes/App/System/Applications`) that `system_profiler` doesn't list. From agent 2026.11.51 the Mac agent reads that folder too, so Safari is listed as an Apple app, has no badge and isn't third-party. Macs on older agents still show the badge until their agent is updated.

### Why don't Windows apps from Parallels show up on Macs?

A Mac running Windows in Parallels lists shortcuts to the Windows apps (Calculator, Command Prompt, Paint…) with version "Windows 11". BEACON treats those as **system apps**, like the Windows originals, so they stay out of Not used. When a Mac has both its own app and a shortcut with the same name, the real install decides.

### What are infrastructure apps?

Apps that are meant to stay open with nobody using them, such as remote-access hosts, or that are needed even though nobody opens them, such as a monitoring agent or server packages (Zabbix). Mark one with the button next to it on the dashboard, or, for an app nobody opens, on its row in the **Not used** table (By app); both need `devices.write`. The same place has **Unmark**. For marked apps:

- active time still counts;
- open time is left out of open totals, the idle ranking and the background list;
- they're listed in their own section;
- the reclaim CSV says `infrastructure = yes`, so they aren't reclaimed by mistake.

Marking and unmarking are audit-logged (`usage.infrastructure.add` / `.remove`).

### What is "Running in the background, never used"?

Apps that ran in the period but nobody was ever active in (for that device and user): system processes, helpers, and apps left running under another account. They're listed separately and not counted as use. Worth a look when it's remote-access software or a paid app.

### What is the reclaim CSV?

With **Not used** on, **Export reclaim list** downloads `license-reclaim-<time>.csv`: one row per unused third-party app **per device**, for procurement. Columns:

| Column | Content |
|---|---|
| `app` | App name |
| `publisher` | Publisher from the inventory |
| `version` | Installed version |
| `device` | Device name |
| `device_user` | Account signed in on the device (whom to ask) |
| `device_last_seen` | When the device last reported |
| `last_used` | Last active use ever, or `never` |
| `installed_on` | Install date, if the OS reports it |
| `period_days` | Length of the period checked |
| `infrastructure` | `yes` if marked as infrastructure |

Helper components are left out, as on screen. Each export is audit-logged as `usage.export` with kind `reclaim`.

### What other exports are there?

The normal export button downloads what the Table shows: **By app** (`app, users, devices_used_on, installed_on, active_hours, open_hours, last_used, period_days`) or **By app and user** (`app, device, user, installed, active_hours, open_hours, days_used, last_used, period_days`). A device page shows the same App usage view for that one device, with the same exports.

### How fresh is the data?

The agent samples every minute and sends a report every **5 samples** (about every 5 minutes). If the device is offline, it keeps up to 5,000 entries and sends them after reconnecting. So new usage appears within a few minutes.

### What does Today compare against?

Today is shown against the **last working day** in full: the most recent earlier day with at least 30 minutes of active use in total. Days nobody worked, usually weekends and holidays, are skipped automatically. Yesterday stands alone.

### Why is there no active time for a device?

Either it runs Linux (not measured there), or no one was signed in at the screen, or the agent is too old to have the active-time helper. On Windows, active time covers the console and Remote Desktop sessions; on macOS, the console user.

### How long is usage kept?

400 days. Older rows are deleted by the hourly retention job.

### Why is the Table cut off at 10,000 rows?

To keep the page fast. Search an app name to narrow it down.

### An app is missing or has an odd name. Why?

Running programs are matched to the device's installed software list, which is rebuilt from each inventory, so names match the device's Software tab. A program not linked to an installed app may be missing or show under its package name.
