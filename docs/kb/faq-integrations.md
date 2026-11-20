---
title: FAQ: Integrations
section: FAQ
order: 80
summary: Webhooks in generic JSON and Microsoft Teams formats, the events, HMAC signing, the test button, retries and private addresses.
reviewed: 2026-11-20
---

### What integrations does BEACON have?

Outgoing **webhooks** only (**Integrations** page; needs `webhooks.write`, so owners and admins). BEACON POSTs an event to a URL you give it. There are no built-in connectors, no email and no incoming API keys. Code: `internal/server/notify/webhooks.go` and `teams.go`.

### Which formats are there?

| Format | Use for | Body |
|---|---|---|
| **Generic JSON** | SIEM and ITSM tools (Splunk, Microsoft Sentinel, Datadog, Jira…) | `{"event": …, "org_id": …, "ts": …, "data": {…}}` |
| **Microsoft Teams** | A Teams channel | An Adaptive Card wrapped as `{"type": "message", "attachments": [...]}` |

### Which events can I subscribe to?

| Event | Sent when |
|---|---|
| `alert.created` | A compliance alert opens |
| `compliance.violation` | A device newly fails a rule that has "Send webhook" on |
| `device.enrolled` | A new device enrolls |
| `device.offline` | The server's 90-second check finds a device has stopped reporting |
| `command.completed` | A command or script finishes (any final status) |
| `audit.*` | Every audit log entry, as `audit.<action>` |

A webhook must subscribe to at least one event. New Teams webhooks start with `alert.created` and `device.enrolled`, because the others are noisy in a chat.

### Why don't I get `device.offline` for every device that goes offline?

It's sent only by the server's periodic check, when a device still marked online hasn't been heard from for 90 seconds (a computer that loses power or network). When the agent's connection closes cleanly, the device goes offline at once and no `device.offline` event is sent. The console still warns that it can be noisy in a chat channel, since PCs that shut down at night may fire it.

### How do I connect a Teams channel?

1. In Teams, open the channel, **⋯ → Workflows → "Send webhook alerts to a channel"**, and copy the URL it gives you.
2. In BEACON, **Integrations → New webhook**, choose **Microsoft Teams**, paste the URL, pick events, save.
3. Click the send icon (**Send test event**). A "BEACON test message" card should appear in the channel.

Cards have a title, details and an **Open in BEACON** button linking to the device (or the Alerts or Audit page). The button needs `UEM_PUBLIC_URL` to be set on the server; without it the button is left out. Keep the workflow URL private: anyone with it can post to the channel.

### How do receivers check that a delivery is really from BEACON?

Every delivery (both formats) carries:

- `X-UEM-Signature: sha256=<hex>`: HMAC-SHA256 of the **raw request body**, keyed with the webhook's signing secret;
- `X-UEM-Event: <event name>`;
- `Content-Type: application/json` and `User-Agent: uem-webhooks/1`.

The receiver recomputes the HMAC with the same secret and compares. Example:

```bash
printf '%s' "$BODY" | openssl dgst -sha256 -hmac "$SECRET" -hex
```

The secret (`whsec_…`) is created automatically and shown in the edit dialog of a generic webhook. Teams doesn't check signatures, so the dialog doesn't show it there.

### How do I change a signing secret?

The console has no button for it. Use the API (`PATCH /api/webhooks/{id}` with a new `secret`) or delete and recreate the webhook. See [Secrets](secrets.md).

### What does the test button do?

It sends one `webhook.test` event right away and waits for the answer. The console says "Test delivered" with the HTTP status, or "Test delivery failed" with the error or status. The result also updates **Last delivery**.

### What does "Last delivery" show?

The HTTP status of the most recent delivery attempt (green below 300, red otherwise) and when it happened. "error" means there was no HTTP answer at all (DNS failure, refused connection, timeout, blocked address).

### What happens if the receiver is down?

Each delivery tries up to **3 times**, waiting 1 s and then 2 s, but only when there's no answer or the receiver returns a 5xx error. A 4xx answer isn't retried. Each try times out after 10 seconds. Redirects aren't followed. After the last try the event is dropped; there's no delivery history beyond Last delivery. See [Webhook failures](webhook-failures.md).

### Can events be lost?

Yes. Events wait in an in-memory queue of 1,024 that four workers send from. If the queue is full, new events are dropped with a "queue full" warning in the server log. Events still queued when the server restarts are lost too.

### Why is my webhook to an internal server refused?

By default BEACON refuses to deliver to loopback, private (10.x, 172.16–31.x, 192.168.x), link-local (including 169.254.169.254 cloud metadata) and carrier-grade NAT (100.64.0.0/10) addresses. This stops webhook URLs being used to probe the server's own network. The error reads "webhook destination … is not a public address". To allow on-premises receivers, set `UEM_WEBHOOK_ALLOW_PRIVATE=true` in `server.env` and restart the server. Proxy settings are ignored for webhooks.

### Can I use plain http:// URLs?

Yes, `http` and `https` are both accepted, but use HTTPS: the body contains device details.

### Can I pause a webhook without deleting it?

Yes, switch off **Enabled** in its edit dialog. A disabled webhook gets no events.

### Are webhook changes audit-logged?

Yes: `webhook.create`, `webhook.update` and `webhook.delete`. Test deliveries aren't.

### What's the difference between `compliance.violation` and `alert.created`?

`alert.created` fires when a rule with "Open an alert" opens one. `compliance.violation` fires only for rules with "Send webhook" switched on, and carries the device, the rule and the failure details. A rule can do both.
