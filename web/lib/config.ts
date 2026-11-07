/** Product branding. Change it here and it updates everywhere in the UI. */
export const APP_NAME = "BEACON";
export const APP_SHORT_NAME = "BEACON";
export const APP_DESCRIPTION = "Business Endpoint Authorization, Configuration and Oversight Network";

/**
 * WebSocket URL for live updates. In production the console and API share an
 * origin (Caddy routes /api to the Go server), so we use ws(s)://<host>/api/live.
 * NEXT_PUBLIC_LIVE_URL can override it in development, e.g. ws://localhost:8080/api/live.
 */
export function liveUrl(): string {
  const override = process.env.NEXT_PUBLIC_LIVE_URL;
  if (override) return override;
  const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${window.location.host}/api/live`;
}
