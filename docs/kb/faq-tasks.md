---
title: FAQ: Scheduled tasks
section: FAQ
order: 50
summary: Scheduled tasks, cron expressions, why times are in UTC, holidays, targets and what happens to offline devices.
reviewed: 2026-11-20
---

### What is a scheduled task?

An action BEACON runs on a set of devices on a timetable (**Scheduled tasks** page). Creating, editing, enabling, deleting and **Run now** need `tasks.write` (owners and admins). Code: `internal/server/scheduler/scheduler.go`.

### What can a task do?

- **Run a library script**
- **Re-evaluate compliance** (rechecks rules on the server; nothing is sent to devices)
- **Refresh inventory**
- **Reboot**
- **Lock screen**

Shut down, Send message, Update agent and Uninstall agent aren't offered as task actions in the console.

### Which devices does it run on?

All devices, one group, or a hand-picked list. The list is worked out **each time the task runs**, so devices that join a group later are included, and retired devices are left out.

### How do I write the schedule?

A standard 5-field cron expression: `minute hour day-of-month month day-of-week`. The console offers presets:

| Preset | Cron |
|---|---|
| Every hour | `0 * * * *` |
| Daily at 02:00 UTC | `0 2 * * *` |
| Weekdays at 09:00 UTC | `0 9 * * 1-5` |
| Sundays at 03:00 UTC | `0 3 * * 0` |
| Monthly on the 1st | `0 0 1 * *` |

Shortcuts like `@daily` and `@hourly` are accepted too. An invalid expression is rejected when you save.

### What time zone are schedules in?

**UTC**, always (`cron.WithLocation(time.UTC)`). Convert from local time yourself. For example, at UTC+8, 22:00 local time is `0 14 * * *`. There's no per-task time zone setting, and daylight saving time doesn't shift UTC schedules, so in places that use it the local run time moves by an hour twice a year.

The "Next" time in the table is shown in your browser's local time.

### Does it skip weekends and holidays?

Weekends, yes, if you use a day-of-week range like `1-5` (Monday to Friday). Holidays, no: there's no holiday calendar for tasks. Disable the task for that day, or accept that it runs.

(App usage's "last working day" comparison does skip holidays by itself, but that's a different feature. See [FAQ: App usage](faq-app-usage.md).)

### What happens to devices that are offline when the task runs?

Commands to them are **queued** and run when they reconnect. So a nightly reboot task makes a laptop that was off all night reboot when it's next switched on and connects. Keep that in mind for disruptive actions. **Re-evaluate compliance** runs on the server and covers offline devices too.

### Can I run a task right away?

Yes, **Run now** (the play button). It runs on the current targets and shows how many devices it went to. The run is recorded as `task.run` with you as the actor; scheduled runs show the `system` actor.

### Does a task use the latest version of its script?

Yes. Tasks store the script's ID, so edits apply from the next run. If the script is deleted, the task fails until you choose another script.

### Do tasks run if the server was down at the scheduled time?

No. Missed runs aren't made up. The next run happens at the next scheduled time after the server is back.

### Is a disabled task kept?

Yes. Disabled tasks keep their settings and are skipped. The switch in the Enabled column turns them on and off.

### How do I see what a task did?

The table shows the last and next run. Each run is in the audit log as `task.run` with the number of devices. Each command it sent is on the device's Commands tab, showing **Scheduler** where a person's email would be.

### What's a good task to start with?

A nightly **Re-evaluate compliance** on all devices, so time-based rules like "checked in within 24 h" are rechecked even for devices that are off. See [FAQ: Compliance](faq-compliance.md).
