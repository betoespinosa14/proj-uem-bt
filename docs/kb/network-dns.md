---
title: Network and DNS
section: Operations
order: 20
summary: How a browser or agent reaches BEACON, from the DNS name through the office router to Caddy on the Mac Mini, plus fallback addresses for agents.
reviewed: 2026-11-20
---

## The path of a request

```text
browser or agent
  → DNS: beacon.itsthev.net  (A record → office public IP)
  → office router: port forward TCP 443 → Mac Mini LAN IP
  → Caddy on the Mac Mini (port 443, HTTPS)
      → /api/*, /downloads/*, /install.sh, /install.ps1, /healthz → uem-server on localhost:8080
      → everything else                                        → console (Next.js) on localhost:3000
```

Agents only make outgoing connections (HTTPS and a WebSocket to `/api/agent/ws`). Laptops outside the office need no VPN and no firewall changes, as long as the public address is reachable from the internet.

## What is known about the live setup

These facts come from comments in the live Caddyfile (`~/Library/Application Support/BEACON/Caddyfile`) and from `server.env`, as of 2026-11-20:

| Item | Value |
|---|---|
| Public name | `beacon.itsthev.net` (also `UEM_PUBLIC_URL` in `server.env`) |
| Office public IP | `122.49.212.51` |
| Router | a FortiGate, forwarding `122.49.212.51:443` → `192.168.130.7:443` |
| Mac Mini LAN IP | `192.168.130.7` |
| Port 80 | **not** forwarded to the Mac Mini (that is why the Caddyfile validates certificates over 443) |
| Agent fallback address | `UEM_FALLBACK_URLS=https://122.49.212.51` |

Confirm these against the router before relying on them.

**To fill in:** the registrar of `itsthev.net`, the account that owns it, and when the domain renews.

**To fill in:** which service hosts the DNS zone for `itsthev.net` and who can edit the `beacon` A record.

**To fill in:** the FortiGate model, its admin address, who administers it, and the exact port-forward (VIP) rules for the Mac Mini.

**To fill in:** whether the office public IP is static, and the internet provider's support contact.

**To fill in:** whether the Mac Mini's LAN IP `192.168.130.7` is a DHCP reservation or set by hand on the Mac.

## Caddy routing

The live Caddyfile has three sites. All of them use the same routing block (named `sentinel`, from the product's old name):

1. `beacon.itsthev.net`: the main address, with a normal Let's Encrypt certificate.
2. `https://122.49.212.51`: the old bare-IP address, kept so agents and bookmarks that still use it keep working. It uses Let's Encrypt's short-lived IP certificate profile.
3. `:443`: anything else on port 443, for example `https://192.168.130.7` typed inside the office. It has no certificate of its own, so browsers show a name warning.

The routing block sends `/api/*`, `/downloads/*`, `/install.sh`, `/install.ps1` and `/healthz` to `localhost:8080`, and everything else to `localhost:3000`. It compresses responses (`encode zstd gzip`) and writes an access log to `~/Library/Logs/Sentinel/caddy-access.log`.

The repo's `deploy/Caddyfile` has the same routing but is the template used by Docker. It reads `UEM_SITE_ADDRESS`, `UEM_SERVER_UPSTREAM` and `UEM_WEB_UPSTREAM`. On the Mac Mini, edit the live Caddyfile in the BEACON folder, then restart Caddy:

```bash
sudo launchctl kickstart -k system/com.beacon.caddy
tail -n 30 ~/Library/Logs/BEACON/caddy.log
```

The live Caddyfile turns off Caddy's admin API (`admin off`), so `caddy reload` does not work. Restart the job instead.

## HTTPS certificates

Caddy gets and renews certificates by itself. See [HTTPS certificates](https-certificates.md) for how, where they are stored and what breaks them.

## Inside the office (NAT hairpin)

From inside the office, `beacon.itsthev.net` still resolves to the public IP. That only works if the router sends the traffic back in (NAT loopback, also called hairpin), or if an internal DNS record points the name at `192.168.130.7`. The README lists this as a requirement. The server log shows office agents connecting from `192.168.130.1`, which suggests the FortiGate does the hairpin.

**To fill in:** how the name resolves inside the office (router hairpin or internal DNS record), and where that is configured.

## Fallback addresses for agents

`UEM_FALLBACK_URLS` in `server.env` lists other addresses of the same server, comma-separated. The server sends the list to agents in their policy, and agents save it in their config file (`fallback_urls`). The logic is in `internal/agent/conn/fallback.go`:

- After 3 failed connections in a row to the main address (about a minute), the agent tries each fallback in order and connects through the first one whose `/healthz` answers.
- It never saves a fallback as its main address. While on a fallback, it re-checks the main address every 5 minutes and goes back as soon as it answers.
- This helps when DNS fails (provider outage, lapsed domain, a network that cannot resolve the name). It does not help when the server itself is down.
- Each fallback needs a certificate the agent trusts. That is why the live Caddyfile keeps a real certificate for `https://122.49.212.51`.

After changing `UEM_FALLBACK_URLS`, restart the server: `sudo launchctl kickstart -k system/com.beacon.server`.

## Moving agents to a new address

Set **Settings → Public server URL** in the console (or `UEM_PUBLIC_URL`). Each connected agent checks `<new address>/healthz`, connects to it, and only saves it once that works. If it fails, the agent stays where it is and tries again after 10 minutes. Keep the old address working until every device has moved. The same URL is used in the install commands on the Enrollment page.

## Quick checks

```bash
dig +short beacon.itsthev.net                       # should print the office public IP
curl -fsS https://beacon.itsthev.net/healthz; echo  # from outside the office: prints ok
curl -fsS https://122.49.212.51/healthz; echo       # the fallback address
```

## Related

- [The production server](production-server.md)
- [Device offline](device-offline.md)
- [Server down](server-down.md)
