---
title: Webhooks not arriving
section: Troubleshooting
order: 70
summary: How BEACON sends webhooks, and what to check when a SIEM, ITSM tool or Teams channel stops receiving them.
reviewed: 2026-11-20
---

## How webhooks are sent

Webhooks are set up on the **Integrations** page (owners and admins). The code is in `internal/server/notify/`.

- **Events:** `alert.created`, `compliance.violation`, `device.enrolled`, `device.offline`, `command.completed`, and `audit.*` (every audit log entry). A webhook only gets the events ticked for it, and only while it is enabled.
- **Request:** an HTTP `POST` with `Content-Type: application/json`, `User-Agent: uem-webhooks/1`, `X-UEM-Event: <event>` and `X-UEM-Signature: sha256=<hex>`.
- **Formats:**
  - **Generic JSON:** `{"event": …, "org_id": …, "ts": …, "data": …}`. For Splunk, Sentinel, Datadog, Jira and similar.
  - **Microsoft Teams:** an Adaptive Card for a Teams "Send webhook alerts to a channel" workflow, with an "Open in BEACON" button when `UEM_PUBLIC_URL` is set.
- **Timeout:** 10 seconds per attempt. Redirects are not followed, and no HTTP proxy is used.
- **Retries:** up to 3 attempts, waiting 1, then 2 seconds between them. Only network errors and HTTP 5xx answers are retried. A 4xx answer (for example 401, 404) is final.
- **Queue:** events wait in an in-memory queue of 1024, sent by 4 workers. If the queue is full, new events are dropped (logged as `webhooks: queue full, dropping`). Events still queued when the server restarts are lost. There is no later redelivery.

The Integrations page shows each webhook's **last delivery**: the HTTP status (green under 300, red otherwise) and when. `error` there means no HTTP answer at all (connection refused, blocked address, timeout).

## Step 1: send a test

On **Integrations**, click the test button (**Send test event**) on the webhook's row. The server sends a `webhook.test` event at once and shows the result:

- "Test delivered" (Receiver responded with HTTP 2xx): BEACON and the receiver work. The problem is which events are ticked, or nothing has happened that triggers them.
- "Test delivery failed" with an HTTP status: the receiver answered but refused. Check its URL and its authentication or signature settings.
- "Test delivery failed" with an error text: BEACON could not reach the receiver. See the next steps.

The test is sent once, without retries.

## Step 2: private address blocking

By default BEACON refuses to send webhooks to loopback, private (10.x, 172.16–31.x, 192.168.x), link-local (169.254.x, including cloud metadata) and carrier-grade-NAT (100.64.0.0/10) addresses. The check is on the IP address the name resolves to, at connect time. The error reads:

```text
webhook destination 192.168.1.20 is not a public address
```

This protects the office network from webhook URLs typed by console users. If the receiver really is on the internal network (an on-premises SIEM, for example):

1. Add to `~/Library/Application Support/BEACON/server.env`:
   ```bash
   UEM_WEBHOOK_ALLOW_PRIVATE=true
   ```
2. Restart the server:
   ```bash
   sudo launchctl kickstart -k system/com.beacon.server
   ```

This allows private destinations for every webhook. Only owners and admins can create webhooks, so the risk stays limited to them.

## Step 3: signature problems (generic format)

Each generic webhook has a **Signing secret** (shown when editing it). BEACON computes HMAC-SHA256 of the **raw request body** with that secret and sends it as `X-UEM-Signature: sha256=<hex>`.

If the receiver rejects deliveries as unsigned or badly signed:

- It must use the exact raw body bytes, not re-serialized JSON.
- It must strip the `sha256=` prefix before comparing.
- The secret in the receiver must match the one shown in BEACON.

A small check, given the body saved to `body.json` and the secret:

```bash
openssl dgst -sha256 -hmac '<signing secret>' body.json
```

Teams deliveries are signed too, but Teams ignores the signature, and the console does not show a signing secret for Teams webhooks.

## Step 4: Teams-specific problems

- The URL must be the one from the channel's **⋯ → Workflows → "Send webhook alerts to a channel"**.
- If the workflow was deleted or its owner left the company, Teams answers with an error status. Create a new workflow and paste the new URL.
- Without `UEM_PUBLIC_URL` set, cards have no "Open in BEACON" button. That is not an error.

## Step 5: read the server log

Failed deliveries (after all retries) are logged:

```bash
grep 'webhooks:' ~/Library/Logs/BEACON/server.log | tail -n 20
```

Messages: `webhooks: delivery failed` (with the hook ID, event and error), `webhooks: queue full, dropping`, `webhooks: list` and `webhooks: encode` (server-side errors).

Deliveries that got an HTTP answer (even 4xx or 5xx) are not logged; only the last status is kept on the Integrations page.

## Step 6: check the network

From the Mac Mini, check that the receiver is reachable over HTTPS:

```bash
curl -sS -o /dev/null -w '%{http_code}\n' -X POST -H 'Content-Type: application/json' -d '{}' https://<receiver URL>
```

Any HTTP code means the network path works. A timeout or connection error points at the receiver, a firewall, or DNS.

## Related

- [Environment variables](environment.md)
- [Server down](server-down.md)
