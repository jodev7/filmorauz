import type { MetadataRoute } from "next";

// Web app manifest → /manifest.webmanifest. Lets FilmoraUz be installed on
// phones/desktops as a standalone app.
export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "FilmoraUz — Onlayn kinoteatr",
    short_name: "FilmoraUz",
    description: "Kinolar va seriallarni o'zbek tilida HD sifatda tomosha qiling.",
    id: "/",
    start_url: "/?source=pwa",
    scope: "/",
    display: "standalone",
    orientation: "any",
    background_color: "#0A0A0F",
    theme_color: "#0A0A0F",
    lang: "uz",
    categories: ["entertainment", "video"],
    icons: [
      { src: "/icon-192.png", sizes: "192x192", type: "image/png", purpose: "any" },
      { src: "/icon-512.png", sizes: "512x512", type: "image/png", purpose: "any" },
      { src: "/icon-maskable-512.png", sizes: "512x512", type: "image/png", purpose: "maskable" },
    ],
    shortcuts: [
      { name: "Kinolar", url: "/movies", icons: [{ src: "/icon-192.png", sizes: "192x192" }] },
      { name: "Seriallar", url: "/series", icons: [{ src: "/icon-192.png", sizes: "192x192" }] },
      { name: "Profilim", url: "/user", icons: [{ src: "/icon-192.png", sizes: "192x192" }] },
    ],
  };
}
