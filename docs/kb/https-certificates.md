---
title: HTTPS certificates
section: Troubleshooting
order: 50
summary: How Caddy gets and renews BEACON's HTTPS certificates by itself, which ports it needs, where certificates are stored, and how to fix common failures.
reviewed: 2026-11-20
---

## How it works

Caddy (`com.beacon.caddy`) gets certificates from Let's Encrypt automatically, using the ACME protocol. Nobody buys or installs certificates by hand. Caddy renews them well before they expire, and Let's Encrypt can also tell it when to renew early (Caddy logs this as "ARI window").

The live Caddyfile (`~/Library/Application Support/BEACON/Caddyfile`) has two certificate sites:

| Site | Certificate |
|---|---|
| `beacon.itsthev.net` | A normal Let's Encrypt certificate for the name. |
| `https://122.49.212.51` | A Let's Encrypt certificate for the bare IP, with the `shortlived` profile (IP certificates are only issued short-lived, so it renews often). It exists so agents using the IP as a fallback can verify it. |

Requests to any other address on 443 (for example `https://192.168.130.7` inside the office) get no matching certificate, and browsers show a name warning. That is expected.

## Ports and validation

Let's Encrypt must reach the Mac Mini from the internet to prove control of the name. There are two ways:

- **HTTP challenge** on port 80.
- **TLS-ALPN challenge** on port 443.

Port 80 is **not** forwarded to the Mac Mini. So both sites in the live Caddyfile have:

```text
tls {
	issuer acme {
		disable_http_challenge
	}
}
```

and validation happens over port 443 only. Port 443 must therefore stay forwarded from the office public IP to the Mac Mini at all times, or renewals fail.

The repo's `deploy/Caddyfile` (used by Docker) has no such block. With it, Caddy may use port 80 or 443; Docker Compose publishes both.

The live Caddyfile also sets `auto_https disable_redirects` (no automatic HTTP→HTTPS redirect, since port 80 is not reachable) and `skip_install_trust` (Caddy does not try to add its own local CA to the Mac's keychain).

## Where certificates are stored

```text
~/Library/Application Support/Caddy/
  certificates/acme-v02.api.letsencrypt.org-directory/beacon.itsthev.net/
  certificates/acme-v02.api.letsencrypt.org-directory/122.49.212.51/
  certificates/local/…     (Caddy's internal CA, from earlier setups; not used by the live sites)
  acme/                    (the Let's Encrypt account)
```

Caddy finds this folder through `HOME`, which `com.beacon.caddy.plist` sets to the BEACON user's home. If `HOME` were wrong, Caddy would start with an empty store and request everything again.

Do not delete this folder to "fix" something. Caddy would request new certificates for every site at once, which can run into Let's Encrypt rate limits.

On Docker, the same data is in the `caddy_data` volume.

## How to check

1. See what the server presents and when it expires:
   ```bash
   echo | openssl s_client -connect beacon.itsthev.net:443 -servername beacon.itsthev.net 2>/dev/null \
     | openssl x509 -noout -subject -issuer -dates
   ```
2. Look at Caddy's recent certificate activity:
   ```bash
   grep -E 'certificate (obtained|renewed)|could not get certificate|error' ~/Library/Logs/BEACON/caddy.log | tail -n 20
   ```
   Healthy renewals show `certificate renewed successfully`.
3. Check from outside the office too (a phone on mobile data): open the site and look at the padlock.

## Common failures

| Symptom | Likely cause | Fix |
|---|---|---|
| Caddy log shows ACME errors, renewals fail | Port 443 not forwarded to the Mac Mini, or forwarded to another machine | Check the router's port forward ([Network and DNS](network-dns.md)). |
| Validation fails with a wrong-IP or connection error | The DNS A record does not point at the office public IP (or the public IP changed) | Fix the A record. `dig +short beacon.itsthev.net` must print the office public IP. |
| Errors mention "rate limit" or "too many certificates" | Too many requests to Let's Encrypt, often after deleting the Caddy folder or many restarts with a broken config | Wait (limits reset over hours to days), do not keep restarting. |
| Browsers say the certificate is for another name | Someone used a bare IP or a LAN address | Use `https://beacon.itsthev.net`. |
| Certificate error on one device only | The device's clock is wrong, or a network proxy inspects HTTPS | Fix the clock; exempt BEACON from the proxy. |
| Caddy does not start after editing the Caddyfile | Syntax error | Read `caddy.log`. Validate with `/opt/homebrew/bin/caddy validate --config ~/Library/Application\ Support/BEACON/Caddyfile --adapter caddyfile`. |

After fixing the cause, restart Caddy so it tries again at once:

```bash
sudo launchctl kickstart -k system/com.beacon.caddy
tail -f ~/Library/Logs/BEACON/caddy.log
```

The live Caddyfile sets no ACME email address, and nothing else on the Mac Mini watches certificate expiry. Checking `caddy.log` now and then is the only early warning.

**To fill in:** whether anyone monitors certificate expiry for `beacon.itsthev.net` from outside (an uptime or certificate monitor).

## Related

- [Network and DNS](network-dns.md)
- [Server down](server-down.md)
