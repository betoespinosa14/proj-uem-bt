/**
 * How BEACON appears when its link is shared (link previews in chat and email apps).
 * The public address can be overridden at build time with BEACON_PUBLIC_URL.
 */
export const PUBLIC_URL = process.env.BEACON_PUBLIC_URL || "https://beacon.itsthev.net";
export const PUBLIC_HOST = new URL(PUBLIC_URL).host;
export const SHARE_TAGLINE = "Endpoint oversight";
