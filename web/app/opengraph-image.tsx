import { ImageResponse } from "next/og";
import { APP_NAME } from "@/lib/config";
import { PUBLIC_HOST, SHARE_TAGLINE } from "@/lib/share";

// The card chat and email apps show when a BEACON link is shared. Built once at build time.
// It shows only the name and logo; apps fetch it without signing in.
export const alt = `${APP_NAME} · ${SHARE_TAGLINE}`;
export const size = { width: 1200, height: 630 };
export const contentType = "image/png";

// The console's font, as TTF (the image renderer can't read WOFF2). If the download fails the
// card still builds, in the renderer's default sans.
async function font(weight: number): Promise<ArrayBuffer | null> {
  try {
    const css = await (
      await fetch(`https://fonts.googleapis.com/css2?family=Plus+Jakarta+Sans:wght@${weight}`, {
        headers: { "User-Agent": "Mozilla/4.0" }, // an old agent gets TTF instead of WOFF2
      })
    ).text();
    const url = css.match(/src: url\((.+?)\) format\('truetype'\)/)?.[1];
    return url ? await (await fetch(url)).arrayBuffer() : null;
  } catch {
    return null;
  }
}

export default async function OpenGraphImage() {
  const [bold, medium] = await Promise.all([font(700), font(500)]);
  const fonts = [
    ...(bold ? [{ name: "Jakarta", data: bold, weight: 700 as const, style: "normal" as const }] : []),
    ...(medium ? [{ name: "Jakarta", data: medium, weight: 500 as const, style: "normal" as const }] : []),
  ];
  return new ImageResponse(
    (
      <div
        style={{
          width: "100%",
          height: "100%",
          display: "flex",
          alignItems: "center",
          padding: "0 110px",
          gap: 60,
          backgroundColor: "#0a2a33",
          backgroundImage:
            "radial-gradient(circle at 12% 8%, rgba(0,164,204,0.30), rgba(0,164,204,0) 60%), radial-gradient(circle at 92% 96%, rgba(131,253,128,0.16), rgba(131,253,128,0) 50%)",
          fontFamily: fonts.length ? "Jakarta" : undefined,
          color: "#e6f1f3",
        }}
      >
        <svg width="320" height="320" viewBox="0 0 64 64">
          <path d="M56 9 L54.5 33 L45 31.5 L46 17 Z" fill="#00a4cc" />
          <path d="M54.5 33 L36.5 55.4 L32 47 L45 31.5 Z" fill="#00889c" />
          <path d="M39.65 51.48 L36.5 55.4 L32 47 Z" fill="#005870" />
          <path d="M19 31.5 L9.5 33 L32 61 L36.5 55.4 L32 47 Z" fill="#00ccec" />
          <path d="M9.5 33 L8 9 L18 17 L19 31.5 Z" fill="#00a4cc" />
          <path d="M8 9 L32 3 L56 9 L46 17 L32 13.5 L18 17 Z" fill="#00889c" />
          <path d="M8 9 L18 17 L13 20 Z" fill="#004858" />
          <path d="M56 9 L51.5 13.5 L46 17 Z" fill="#005870" />
          <circle cx="32" cy="27.5" r="5" fill="#83fd80" />
        </svg>
        <div style={{ display: "flex", flexDirection: "column" }}>
          <div style={{ display: "flex", alignItems: "center", gap: 28 }}>
            <span style={{ fontSize: 104, fontWeight: 700, letterSpacing: 16, lineHeight: 1 }}>{APP_NAME}</span>
            <span style={{ width: 24, height: 24, borderRadius: 12, backgroundColor: "#83fd80" }} />
          </div>
          <span style={{ marginTop: 26, fontSize: 34, fontWeight: 500, letterSpacing: 7, opacity: 0.72 }}>
            {SHARE_TAGLINE.toUpperCase()}
          </span>
          <span style={{ marginTop: 84, fontSize: 24, opacity: 0.45 }}>{PUBLIC_HOST}</span>
        </div>
      </div>
    ),
    { ...size, fonts },
  );
}
