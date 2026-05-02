import { MetadataRoute } from "next";
import { getMovies } from "@/lib/api";
import { getSeries } from "@/lib/series-api";
import { getPublicGenres } from "@/lib/genres";
import {
  buildBestEpisodeUrl,
  buildMovieUrl,
  buildSeasonUrl,
  buildSeriesUrl,
  SITE_URL,
} from "@/lib/content-routes";

export const dynamic = "force-dynamic";

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const now = new Date();
  const entries: MetadataRoute.Sitemap = [];
  const seenUrls = new Set<string>();

  const pushEntry = (entry: MetadataRoute.Sitemap[number]) => {
    if (seenUrls.has(entry.url)) return;
    seenUrls.add(entry.url);
    entries.push(entry);
  };

  pushEntry({ url: `${SITE_URL}/`, lastModified: now, changeFrequency: "daily", priority: 1.0 });
  pushEntry({ url: `${SITE_URL}/movies`, lastModified: now, changeFrequency: "daily", priority: 0.8 });
  pushEntry({ url: `${SITE_URL}/series`, lastModified: now, changeFrequency: "daily", priority: 0.8 });
  pushEntry({ url: `${SITE_URL}/genres`, lastModified: now, changeFrequency: "weekly", priority: 0.7 });

  try {
    const genres = await getPublicGenres();
    for (const genre of genres) {
      pushEntry({
        url: `${SITE_URL}/genres/${encodeURIComponent(genre)}`,
        lastModified: now,
        changeFrequency: "weekly",
        priority: 0.7,
      });
    }
  } catch {
    // Ignore genre sitemap failures.
  }

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
        if (m.approval_status && m.approval_status !== "approved") continue;
        if (m.is_published === false && m.approval_status) continue;
        seen.add(m.slug);
        pushEntry({
          url: buildMovieUrl(m.slug),
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

  // Series + seasons + SEO episode URLs
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
        pushEntry({
          url: buildSeriesUrl(s.slug),
          lastModified: s.updated_at ? new Date(s.updated_at) : now,
          changeFrequency: "weekly",
          priority: 0.8,
        });

        // Episodes
        try {
          const detail = await getSeriesBySlugSafe(s.slug);
          for (const season of detail?.seasons || []) {
            pushEntry({
              url: buildSeasonUrl(s.slug, season.season.season_number),
              lastModified: season.season.updated_at ? new Date(season.season.updated_at) : now,
              changeFrequency: "weekly",
              priority: 0.7,
            });
            for (const ep of season.episodes || []) {
              if (!ep.id) continue;
              pushEntry({
                url: buildBestEpisodeUrl({
                  episodeId: ep.id,
                  seriesSlug: s.slug,
                  seasonNumber: season.season.season_number,
                  episodeNumber: ep.episode_number,
                }),
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
