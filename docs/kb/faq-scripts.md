---
title: FAQ: Scripts
section: FAQ
order: 40
summary: The script library, running scripts on devices, which shells work on which OS, timeouts and output.
reviewed: 2026-11-20
---

### What is the Script library?

A list of saved scripts (**Scripts** page) you can run on one device, many devices, from a scheduled task, or as a compliance remediation. Creating, editing and deleting them needs `scripts.write` (owners and admins). Anyone signed in can open and read them.

### Which shells can I use?

`bash`, `sh`, `zsh`, `powershell` and `cmd`. What works depends on the device (`internal/agent/exec/exec.go`):

| Shell | Windows | macOS | Linux |
|---|:-:|:-:|:-:|
| `powershell` | yes (Windows PowerShell) | only if `pwsh` is installed | only if `pwsh` is installed |
| `cmd` | yes | – | – |
| `bash`, `sh`, `zsh` | – | yes | yes (`zsh` only if installed) |

If the shell isn't available, the command fails with a clear error such as "bash is not available on Windows". When all selected devices share one OS, the run dialog offers only the shells that work there.

### What account does a script run as?

The agent's service account: **root** on macOS and Linux, **SYSTEM** on Windows. It doesn't run as the signed-in user, so anything that depends on the user's session (their desktop, their keychain or profile) won't behave as it would for them. To tell the user something, use **Send message**.

### How do I run a script?

On a device page or with devices selected on the Devices list, choose **Run script…**. Pick **From library** or **Ad-hoc script** (type it in). This needs `commands.run`, which helpdesk users also have. See [FAQ: Devices](faq-devices.md).

### How does the agent run it?

It writes the script to a new temporary folder (the file has mode 700), runs it with the chosen shell, and deletes the folder afterwards. If the shell program isn't found, the command fails with "… not found on this device". Details:

- PowerShell runs with `-NoProfile -NonInteractive -ExecutionPolicy Bypass`, and BEACON adds `$ErrorActionPreference = 'Stop'` at the top so errors give a non-zero exit code.
- `cmd` scripts run as a `.bat` file with `/Q /C`.
- Line endings are converted to Windows style for PowerShell and cmd.

### How is success decided?

Exit code 0 = **succeeded**; anything else = **failed**. If it runs past its timeout it's stopped and marked **timeout**.

### What's the timeout?

Set per script, from 1 to 3,600 seconds (1 hour). The default is 300 seconds (5 minutes).

### Where do I see the output?

On the device's **Commands** tab: status, exit code, standard output and standard error. Each stream keeps the first **1 MiB**; the rest is cut off. Output updates live.

### Is the output in the audit log?

No. The audit log records who ran what (`device.command.create`, including the shell and the **full script body**) and the result (`device.command.result` with status and exit code), but not the output.

### If I edit a library script, what happens to past and scheduled runs?

- **Past runs** keep the exact script that ran: the body is copied into the command when it's sent.
- **Scheduled tasks** refer to the script by ID, so they use the edited version from their next run.
- **Deleting** a script makes tasks that use it fail until you update them. Past command history is kept.

### What if the device is offline?

The command waits in the queue and runs when the device reconnects. See [FAQ: Devices](faq-devices.md).

### What if a script never reports back?

If no result arrives within 75 minutes of sending, the server marks it **timeout** with "no result received from agent". That covers scripts lost when a device sleeps or reboots mid-run.

### How big can a script be?

Up to 256 KiB.

### Can I pass parameters or secrets to a script?

There are no parameters or secret variables. Everything is in the script body, which is stored in the database and copied into the audit log. **Don't put passwords or keys in scripts.**

### Can I stop a running script?

Not from the console. It ends at its timeout.

### Can I run a script on a schedule?

Yes, with a scheduled task: action **Run a library script**. See [FAQ: Scheduled tasks](faq-tasks.md).
