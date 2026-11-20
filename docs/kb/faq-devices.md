---
title: FAQ: Devices
section: FAQ
order: 20
summary: The Devices list and device page, online and offline, commands and remote actions, groups, and retiring a device.
reviewed: 2026-11-20
---

### What does "online" mean?

The device's agent has an open connection to the server and sent a heartbeat recently. Agents send a heartbeat every 30 seconds. A device goes **offline** when its connection closes, or when the server's check (every 15 seconds) finds no heartbeat for **90 seconds** (`internal/server/scheduler/scheduler.go`). When the server restarts, every device starts as offline until its agent reconnects, usually within seconds.

### A device shows offline but the computer is on. Why?

The agent can't reach the server: the agent service is stopped, the network blocks outgoing HTTPS, the server address changed, or the device was retired. See [Device offline](device-offline.md).

### How often does a device report?

- Heartbeat: every 30 seconds.
- Health figures (CPU, memory, disk, battery): every 60 seconds.
- Full inventory (hardware, software, security settings): every 6 hours, and shortly after connecting. **Refresh inventory** asks for one now.

These intervals are set by the server in the device's policy (`TelemetryInterval` and `InventoryInterval` in `cmd/server/main.go`).

### What can I search and filter on?

The search box matches name, hostname, serial number, IP address, signed-in user, employee name and tags. Filters: online/offline, operating system, compliance state, group and enrollment token. The Dashboard's posture links add filters for "not encrypted", "firewall off", "no antivirus" and "updates pending"; a device that hasn't reported counts as missing the setting.

### Can I export the device list?

Yes, the export button gives a CSV of the **filtered** list, including OS, hardware, signed-in user, employee, group, tags, agent version, health figures, software count, and any gambling apps and games found. Every export is audit-logged as `devices.export`.

### What remote actions are there?

From the device page (**⋯** menu) or by selecting devices on the list. All need `commands.run` (owners, admins, helpdesk):

| Action | What it does |
|---|---|
| Run script | A library script or an ad-hoc one, as root/SYSTEM. See [FAQ: Scripts](faq-scripts.md). |
| Send message | Shows a window with a title (default "Message from IT") and text. |
| Lock screen | Windows: disconnects the console session. macOS: sleeps the display, which locks only if "require password after sleep" is on. |
| Reboot / Shut down | Optional delay from 0 to 86,400 seconds. |
| Refresh inventory | Asks for a full inventory now. |
| Update agent | Installs the newest agent build the server has. See [Agent updates](agent-updates.md). |
| Uninstall agent | Removes the agent from the device. |

There is no remote wipe; it was left out on purpose.

### What happens if I send a command to an offline device?

It's saved as **queued** and sent the moment the device reconnects. Queued commands don't expire. The dialog tells you how many targets are offline.

### What do the command statuses mean?

`queued` (waiting for the device), `sent` (delivered), `running`, then `succeeded`, `failed`, `timeout` or `cancelled`. If a sent command gets no result within **75 minutes**, the server marks it `timeout` with "no result received from agent". Results and output show on the device's Commands tab, live.

### Can I send one action to many devices?

Yes. Select devices on the Devices list and pick an action. A bulk request takes up to 1,000 devices. For reboot, shut down, lock, run script and uninstall agent on **more than 10 devices**, the dialog makes you type the number of devices first, so a slip can't reboot the whole office.

### How do I rename a device?

**⋯ → Edit name, group & tags** (needs `devices.write`). If you set an **employee name**, the name is generated for you as `<group> | <employee>` (or just the employee without a group) and kept up to date when the group changes or is renamed.

### What are groups for?

Groups (Groups page) organise devices and let you:
- target scheduled tasks and compliance rules at one group;
- enroll new devices straight into a group (set on the enrollment token);
- block applications on every device in the group.

A device is in at most one group. Tags are free text and a device can have many.

### How does the app blocklist work?

On a group, list **process names** (for example `chrome`, not "Google Chrome"; not case sensitive; `.exe` optional). Every 15 seconds the agent on each device in the group closes any matching process and reports it. Each block is audit-logged as `device.event.app_blocked`. Changes reach online devices at once.

### What happens when I delete a group?

Its devices stay but become ungrouped, and its blocklist stops applying. Devices with an employee name get their name regenerated without the group.

### How do I delete a device?

**⋯ → Retire device** (needs `devices.write`). Retiring:
- revokes the device's credential immediately and disconnects it;
- hides it from the Devices list, App usage and reports;
- keeps its history in the audit log (the database row stays, marked retired).

Retiring does **not** remove the agent from the computer. The agent keeps trying to connect every 5 minutes and is refused. So for a computer you still have, run **Uninstall agent** first and retire it after. There's no "un-retire" in the console.

### How do I re-enroll a retired or broken device?

On the device, delete the agent's `agent.json` (Windows `C:\ProgramData\UEM Agent\`, macOS `/Library/Application Support/UEM Agent/`, Linux `/etc/uem-agent/`) and run a fresh install command from Enrollment. Running the installer while `agent.json` exists only upgrades the agent and keeps the old identity. See [Enrollment](enrollment.md).

### How far back do the health charts go?

Up to 30 days. The last 24 hours show one point per minute; longer ranges show 5-minute averages. Health samples older than 30 days are deleted.

### What does "Update available" next to a device mean?

The device runs an older agent than the newest build on the server. Update it from the device page or all at once with **Update agents** on the Devices list. See [Agent updates](agent-updates.md) and [Agent not updating](agent-not-updating.md).

### Why is the compliance state "Unknown"?

The device hasn't sent an inventory yet, so nothing can be checked. It changes after the first inventory. See [FAQ: Compliance](faq-compliance.md).

### Which IP address is shown?

Two: the address the device reports for itself (often a private LAN address), and the address its agent connected from as seen by the server (the public address when it's outside the office).
