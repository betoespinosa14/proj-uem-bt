import type { NextConfig } from "next";

// All browser requests use relative URLs (/api/...). In production Caddy routes
// /api straight to the Go server; in `next dev` / `next start` these rewrites
// proxy them instead. Note: rewrites do not proxy WebSocket upgrades reliably,
// see README for /api/live in development.
const API_URL = process.env.UEM_API_URL || "http://localhost:8080";

const nextConfig: NextConfig = {
  output: "standalone",
  reactStrictMode: true,
  poweredByHeader: false,
  async rewrites() {
    return [
      { source: "/api/:path*", destination: `${API_URL}/api/:path*` },
      { source: "/downloads/:path*", destination: `${API_URL}/downloads/:path*` },
      { source: "/install.sh", destination: `${API_URL}/install.sh` },
      { source: "/install.ps1", destination: `${API_URL}/install.ps1` },
    ];
  },
};

export default nextConfig;
