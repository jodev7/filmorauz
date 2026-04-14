import { MetadataRoute } from "next";

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL || "https://filmorauz.net";

export default function robots(): MetadataRoute.Robots {
  // Pages that should NOT be indexed
  const disallowPaths = [
    "/admin",
    "/user",
    "/watch",
    "/api",
    "/uploads",
    "/favorites",
    "/history",
  ];

  return {
    rules: [
      {
        userAgent: "*",
        allow: "/",
        disallow: disallowPaths,
      },
      {
        // Specific rules for Googlebot to ensure proper indexing
        userAgent: "Googlebot",
        allow: "/",
        disallow: disallowPaths,
      },
      {
        // Yandex bot
        userAgent: "Yandex",
        allow: "/",
        disallow: disallowPaths,
      },
    ],
    sitemap: `${SITE_URL}/sitemap.xml`,
    host: SITE_URL,
  };
}
