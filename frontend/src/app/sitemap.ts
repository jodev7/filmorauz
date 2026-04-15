import { MetadataRoute } from "next";
import { getMovies, getCollections } from "@/lib/api";

export const dynamic = "force-dynamic";

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL || "https://filmorauz.net";

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const locales = ["uz", "ru", "en"] as const;
  const baseUrl = SITE_URL;

  // Static pages for all locales
  const staticPages: { path: string; priority: number; changeFrequency: "daily" | "weekly" | "monthly" }[] = [
    { path: "", priority: 1.0, changeFrequency: "daily" },
    { path: "/movies", priority: 0.9, changeFrequency: "daily" },
    { path: "/collections", priority: 0.8, changeFrequency: "weekly" },
  ];

  const sitemapEntries: MetadataRoute.Sitemap = [];

  // Add static pages for all locales
  for (const locale of locales) {
    for (const page of staticPages) {
      sitemapEntries.push({
        url: `${baseUrl}/${locale}${page.path}`,
        lastModified: new Date(),
        changeFrequency: page.changeFrequency,
        priority: page.priority,
      });
    }
  }

  // Fetch movies from API
  try {
    const response = await getMovies({ page: 1, limit: 100 });
    const movies = response.data;
    
    if (movies && movies.length > 0) {
      for (const movie of movies) {
        for (const locale of locales) {
          sitemapEntries.push({
            url: `${baseUrl}/${locale}/movies/${movie.slug}`,
            lastModified: movie.updated_at ? new Date(movie.updated_at) : new Date(),
            changeFrequency: "weekly" as const,
            priority: 0.7,
          });
        }
      }
    }
  } catch (error) {
    console.error("Failed to fetch movies for sitemap:", error);
  }

  // Fetch collections from API
  try {
    const collections = await getCollections();
    
    if (collections && collections.length > 0) {
      for (const collection of collections) {
        for (const locale of locales) {
          sitemapEntries.push({
            url: `${baseUrl}/${locale}/collections/${collection.slug}`,
            lastModified: new Date(),
            changeFrequency: "weekly" as const,
            priority: 0.6,
          });
        }
      }
    }
  } catch (error) {
    console.error("Failed to fetch collections for sitemap:", error);
  }

  return sitemapEntries;
}
