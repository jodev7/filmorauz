import { MetadataRoute } from "next";

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL || "https://filmorauz.net";

export default function robots(): MetadataRoute.Robots {
  const disallowPaths = [
    "/admin",
    "/api",
    "/user",
    "/profile",
    "/login",
    "/watch",
    "/uploads",
    "/notifications",
    "/banned",
  ];

  return {
    rules: [
      { userAgent: "*", allow: "/", disallow: disallowPaths },
      { userAgent: "Googlebot", allow: "/", disallow: disallowPaths },
      { userAgent: "Yandex", allow: "/", disallow: disallowPaths },
    ],
    sitemap: `${SITE_URL}/sitemap.xml`,
    host: SITE_URL,
  };
}
