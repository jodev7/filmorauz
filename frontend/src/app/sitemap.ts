import { MetadataRoute } from "next";
import { getMovies } from "@/lib/api";
import { getSeries } from "@/lib/series-api";

export const dynamic = "force-dynamic";

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL || "https://filmorauz.net";

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const now = new Date();

  const entries: MetadataRoute.Sitemap = [
    { url: `${SITE_URL}/`, lastModified: now, changeFrequency: "daily", priority: 1.0 },
    { url: `${SITE_URL}/movies`, lastModified: now, changeFrequency: "daily", priority: 0.9 },
    { url: `${SITE_URL}/series`, lastModified: now, changeFrequency: "daily", priority: 0.9 },
    { url: `${SITE_URL}/collections`, lastModified: now, changeFrequency: "weekly", priority: 0.6 },
  ];

  // Movies
  try {
    const seen = new Set<string>();
    let page = 1;
    while (page <= 20) {
      const res = await getMovies({ page, limit: 100 });
      const data = res.data || [];
      if (data.length === 0) break;
      for (const m of data) {
        if (!m.slug || seen.has(m.slug)) continue;
        seen.add(m.slug);
        entries.push({
          url: `${SITE_URL}/movies/${m.slug}`,
          lastModified: m.updated_at ? new Date(m.updated_at) : (m.created_at ? new Date(m.created_at) : now),
          changeFrequency: "weekly",
          priority: 0.8,
        });
      }
      if (data.length < 100) break;
      page++;
    }
  } catch (e) {
    console.error("sitemap: failed to fetch movies", e);
  }

  // Genre pages from movie genres
  try {
    const res = await getMovies({ page: 1, limit: 100 });
    const genres = new Set<string>();
    for (const m of res.data || []) {
      for (const g of m.genre || []) {
        if (g) genres.add(g.toLowerCase());
      }
    }
    for (const g of Array.from(genres)) {
      entries.push({
        url: `${SITE_URL}/movies?genre=${encodeURIComponent(g)}`,
        lastModified: now,
        changeFrequency: "weekly",
        priority: 0.5,
      });
    }
  } catch {
    // ignore
  }

  // Series + episodes
  try {
    const seenSeries = new Set<string>();
    let page = 1;
    while (page <= 20) {
      const res = await getSeries(page, 100);
      const data = res.data || [];
      if (data.length === 0) break;
      for (const s of data) {
        if (!s.slug || seenSeries.has(s.slug)) continue;
        seenSeries.add(s.slug);
        entries.push({
          url: `${SITE_URL}/series/${s.slug}`,
          lastModified: s.updated_at ? new Date(s.updated_at) : now,
          changeFrequency: "weekly",
          priority: 0.8,
        });

        // Episodes
        try {
          const detail = await getSeriesBySlugSafe(s.slug);
          for (const season of detail?.seasons || []) {
            for (const ep of season.episodes || []) {
              if (!ep.id) continue;
              entries.push({
                url: `${SITE_URL}/episode/${ep.id}`,
                lastModified: ep.updated_at ? new Date(ep.updated_at) : now,
                changeFrequency: "weekly",
                priority: 0.6,
              });
            }
          }
        } catch {
          // skip series detail failures
        }
      }
      if (data.length < 100) break;
      page++;
    }
  } catch (e) {
    console.error("sitemap: failed to fetch series", e);
  }

  return entries;
}

async function getSeriesBySlugSafe(slug: string) {
  const { getSeriesBySlug } = await import("@/lib/series-api");
  try {
    return await getSeriesBySlug(slug);
  } catch {
    return null;
  }
}
