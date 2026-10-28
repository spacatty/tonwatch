import type { MetadataRoute } from "next";

export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "TON Watch",
    short_name: "TON Watch",
    description: "Every Telegram username for sale, from every marketplace, in one place",
    start_url: "/",
    display: "standalone",
    background_color: "#06080c",
    theme_color: "#06080c",
    icons: [
      { src: "/icon-192.png", sizes: "192x192", type: "image/png" },
      { src: "/icon-512.png", sizes: "512x512", type: "image/png" },
    ],
  };
}
