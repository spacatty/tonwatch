import { readFileSync } from "node:fs";
import type { NextConfig } from "next";

// The Go backend (backend/cmd/server) serves /api; Next proxies it so the browser has one origin.
const API = process.env.FRAGWATCH_API ?? "http://127.0.0.1:8080";

// PUBLIC_URL lives in the root .env next to the backend's settings. When the app is opened through
// that domain (a reverse proxy, the Telegram Mini App), the dev server has to accept its origin.
function publicHost(): string | null {
  let url = process.env.PUBLIC_URL ?? "";
  if (!url) {
    try {
      url = readFileSync("../.env", "utf8").match(/^PUBLIC_URL=["']?([^"'\r\n]+)/m)?.[1] ?? "";
    } catch {}
  }
  try {
    return url ? new URL(url).hostname : null;
  } catch {
    return null;
  }
}
const host = publicHost();

const nextConfig: NextConfig = {
  allowedDevOrigins: host && host !== "localhost" ? [host] : [],
  cacheComponents: true,
  partialPrefetching: true,
  turbopack: {
    rules: {
      "*.css": {
        loaders: ["@tailwindcss/turbopack"],
        as: "*.css",
      },
    },
  },
  rewrites() {
    return [{ source: "/api/:path*", destination: `${API}/api/:path*` }];
  },
};

export default nextConfig;
