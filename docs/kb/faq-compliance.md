---
title: FAQ: Compliance
section: FAQ
order: 30
summary: Compliance rules, when they are checked, what happens on a violation, and how gambling apps, games and system apps are recognised.
reviewed: 2026-11-20
---

### What is a compliance rule?

A named check with one or more conditions, such as "Disk encryption enabled is true" or "Free disk space is at least 10 %". A device either passes or fails each rule that applies to it. Rules are on the **Compliance** page; changing them needs `compliance.write` (owners and admins). Code: `internal/server/compliance/rules.go` and `EvaluateDevice` in `internal/server/core/core.go`.

### What can a rule check?

| Field | Type |
|---|---|
| Disk encryption enabled, Firewall enabled, Screen lock enabled | yes/no |
| Pending OS updates | number |
| OS family | text |
| OS version, OS build, Agent version | version (compared number by number, so 10.0.22631 > 10.0.19045) |
| Minutes since last check-in | number |
| Free disk space (system volume), Installed memory, Battery health | number |
| Wi-Fi network (SSID) | text |
| Installed software | list (contains / doesn't contain) |
| Gambling apps, Games | "none found" / "any found" |
| Device tags | list |

The console's **New rule** offers templates: disk encryption, firewall, checked in within 24 h, at least 10 % free disk, and no gambling apps or games.

### What does "All conditions" vs "Any condition" mean?

"All conditions pass" fails the rule if any one condition fails. "Any condition passes" passes the rule if at least one condition passes.

### What happens when a value is unknown?

The condition **fails**. BEACON doesn't assume a device is compliant when it can't verify it. Common unknowns: encryption in a virtual machine, screen-lock state on macOS and Linux, and Wi-Fi name on recent macOS (Apple hides it from services).

### Which devices does a rule apply to?

Enabled rules apply to every device, narrowed by the rule's operating systems (none ticked = all) and its group (blank = all groups). A disabled rule is ignored and its old results are removed at the next check.

### When are devices checked?

- every time a device sends an inventory (every 6 hours, after connecting, and after **Refresh inventory**);
- in the background after you create, edit or delete a rule;
- after you edit a device (group or tags can change which rules apply);
- when you click **Re-evaluate all**;
- when a scheduled task with the action **Re-evaluate compliance** runs.

### Why does "Checked in within 24 h" not fail for a laptop that's been away for days?

An offline device sends no inventory, so nothing triggers a check. Add a scheduled task that runs **Re-evaluate compliance** every night, so time-based rules are rechecked even for offline devices. See [FAQ: Scheduled tasks](faq-tasks.md).

### What does a device's overall state mean?

- **Compliant:** it passes every rule that applies to it (also when no rules apply).
- **Non-compliant:** it fails at least one.
- **Unknown:** no inventory has arrived yet, so nothing has been verified.

State changes are audit-logged as `device.compliance.change`.

### What happens when a device starts failing a rule?

Only when it **newly** fails (it passed before, or is checked for the first time), the rule's actions run:

- **Open an alert** (on by default) with the rule's severity. See [FAQ: Alerts](faq-alerts.md).
- **Send webhook:** a `compliance.violation` event. See [FAQ: Integrations](faq-integrations.md).
- **Tag the device:** adds a tag while it fails and removes it when it passes again.
- **Remediation:** sends a command: run a library script, refresh inventory, or lock the screen.

The violation itself is always audit-logged as `compliance.violation`. A device that keeps failing doesn't trigger the actions again on every check.

### What happens when it passes again?

Its open alert for that rule is resolved automatically, and the rule's tag comes off.

### How are gambling apps and games recognised?

By name and publisher, using the lists in `internal/server/compliance/catalog.go`:

- **Gambling:** names such as casino, poker, slots, roulette, blackjack, sportsbook, betting, bingo, lottery, sabong, and brands such as PokerStars, bet365, Betway, DraftKings, FanDuel, OKBet, BingoPlus, ArenaPlus; or publishers such as Flutter, Entain, Playtika, DigiPlus.
- **Games:** launchers (Steam, Epic Games, Battle.net, EA app, Ubisoft Connect, Riot Client, Xbox…), popular titles (Minecraft, Roblox, Valorant, Fortnite, League of Legends…), and publishers (Valve, Riot Games, Blizzard, EA, Ubisoft, Activision, miHoYo, Supercell…).

Words must match whole, so "bet" doesn't hit "Alphabet" or "beta".

### Which apps are never flagged as games?

- Apps signed by Apple (for example macOS Chess).
- Microsoft Store apps published by Microsoft (Solitaire, Minesweeper, Mahjong, the Xbox app and Game Bar): they come with Windows.
- Parallels shortcuts on a Mac to apps inside a Windows virtual machine (their version reads "Windows 11").
- Unreal Engine tools shipped by Epic.

### Why does a game's anti-cheat show up after the game was removed?

Anti-cheat drivers (Vanguard, Easy Anti-Cheat, BattlEye, FACEIT…) often stay installed after the game is uninstalled. They're still listed so you can ask about them.

### What's the difference between "System" and "Third-party" apps?

Every installed app is sorted by the source and publisher the agent reports (`Category` in `catalog.go`):

| Counts as **System** | Examples |
|---|---|
| Apple-signed macOS apps | Safari, Calculator |
| Windows apps from Microsoft, except separately sold products | Windows tools; **not** Office, Teams, Visual Studio, SQL Server |
| Hardware drivers and Realtek software on Windows | audio, card reader drivers |
| Linux packages from the distribution | Ubuntu, Debian, Red Hat… packages |
| Parallels shortcuts to Windows apps | Calculator, Command Prompt with version "Windows 11" |

Everything else is **Third-party**: Mac App Store apps, apps from identified developers, other Windows programs, and anything of unknown origin. This split is used on the device's Software tab and in App usage. See [FAQ: App usage](faq-app-usage.md).

### A legitimate app is flagged as a game or gambling. What can I do?

There's no allow-list in the console. Either change the rule (for example, use **Installed software doesn't contain** for specific titles instead of the **Games** check), or change the patterns in `internal/server/compliance/catalog.go`, add a test in `rules_test.go`, and deploy ([Releases](releases.md)).

### Can a remediation script fix the problem automatically?

Yes: pick **Run a library script** as the remediation. It runs as root/SYSTEM when the device newly fails. After it runs, use **Refresh inventory** (or wait for the next inventory) to recheck.
