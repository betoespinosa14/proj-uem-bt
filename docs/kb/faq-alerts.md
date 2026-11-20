---
title: FAQ: Alerts
section: FAQ
order: 70
summary: Where alerts come from, how they open and close, and how to get notified about them.
reviewed: 2026-11-20
---

### Where do alerts come from?

Only from **compliance rules**. When a device newly fails a rule that has "Open an alert" switched on (the default), BEACON opens an alert with the rule's severity, titled `<device name>: <rule name>`, with the failing details as its message (`onViolation` in `internal/server/core/core.go`). See [FAQ: Compliance](faq-compliance.md).

### Does a device going offline raise an alert?

No. Offline devices show on the Devices list and Dashboard, and can send a `device.offline` webhook, but they don't open an alert. If you want an alert, make a compliance rule on **Minutes since last check-in** and schedule a nightly **Re-evaluate compliance** task.

### Can one device have several alerts for the same rule?

No. There's at most one **open** alert per device and rule. A new one opens only after the device has passed the rule again and then fails it again.

### How does an alert close?

- **Automatically**, when the device passes the rule again (on its next check).
- **By hand**, with **Resolve** (needs `alerts.write`: owners, admins and helpdesk).

### If I resolve an alert but the device still fails, does it come back?

Not by itself. The rule's actions only run when the device **newly** fails. It stays failing and non-compliant without an open alert until it passes and fails again. So resolve by hand only when you've dealt with the cause, or when the device is fixed and you don't want to wait for its next inventory. To force a recheck, use **Refresh inventory** on the device.

### Who can see alerts?

Every signed-in user. Only owners, admins and helpdesk can resolve them.

### What do the severities mean?

They come from the rule: low, medium, high or critical. BEACON doesn't treat them differently beyond showing them; use them to sort what to look at first.

### How do I get notified when an alert opens?

Create a webhook on **Integrations** subscribed to `alert.created`. A **Microsoft Teams** webhook posts a card to a channel with an "Open in BEACON" button. There's no email. See [FAQ: Integrations](faq-integrations.md).

### Where are alerts shown?

The **Alerts** page (Open / Resolved) and the **Open alerts** figure on the Dashboard.

### How many alerts does the page show?

The newest 500 for the chosen status.

### Are alerts audit-logged?

The violation is (`compliance.violation`), and resolving by hand is (`alert.resolve`). Automatic closing isn't a separate entry. If the device becomes fully compliant, that change is logged as `device.compliance.change`.

### How do I stop an alert for a rule that's too noisy?

Edit the rule and switch off **Open an alert**, narrow it to certain operating systems or a group, or disable the rule. Existing open alerts stay until you resolve them. Deleting a rule keeps its alerts but unlinks them from the rule.
