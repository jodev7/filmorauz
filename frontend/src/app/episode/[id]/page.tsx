import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import WatchPageClient from "@/components/watch/WatchPageClient";
import Comments from "@/components/Comments";
import StarRating from "@/components/StarRating";
import { getEpisode, getSeriesBySlug, Episode, EpisodeLink, SeriesWithSeasons } from "@/lib/series-api";
import { normalizeMediaUrl } from "@/lib/image-utils";
import { Metadata } from "next";
import Script from "next/script";
import Link from "next/link";

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL || "https://filmorauz.net";

interface PageProps {
  params: { id: string };
}

export async function generateMetadata({ params }: PageProps): Promise<Metadata> {
  const { id } = params;
  const canonicalUrl = `${SITE_URL}/episode/${id}`;

  try {
    const response = await getEpisode(id);
    const ep = response.episode;
    const seriesTitle = ep.series_title || "Serial";
    let seasonNumber: number | undefined;
    let imageUrl = `${SITE_URL}/og-image.jpg`;

    if (ep.series_slug) {
      try {
        const data = await getSeriesBySlug(ep.series_slug);
        const found = data.seasons.find(s => s.season.id === ep.season_id);
        seasonNumber = found?.season.season_number;
        imageUrl = normalizeMediaUrl(ep.thumbnail_url || data.series.backdrop_url || data.series.poster_url, imageUrl);
      } catch {
        // ignore
      }
    }

    const title = seasonNumber
      ? `${seriesTitle} S${seasonNumber}E${ep.episode_number} o'zbek tilida | FilmoraUz`
      : `${seriesTitle} ${ep.episode_number}-qism o'zbek tilida | FilmoraUz`;
    const description = seasonNumber
      ? `${seriesTitle} ${seasonNumber}-mavsum ${ep.episode_number}-qism o'zbek tilida HD tomosha qiling.`
      : `${seriesTitle} ${ep.episode_number}-qismni o'zbek tilida HD tomosha qiling.`;

    return {
      title,
      description,
      openGraph: {
        title,
        description,
        url: canonicalUrl,
        siteName: "FILMORAUZ",
        type: "video.episode",
        locale: "uz_UZ",
        images: [{ url: imageUrl, width: 1200, height: 630, alt: ep.title }],
      },
      twitter: {
        card: "summary_large_image",
        title,
        description,
        images: [imageUrl],
      },
      alternates: { canonical: canonicalUrl },
      robots: { index: true, follow: true },
    };
  } catch {
    return {
      title: "Epizod topilmadi — FILMORAUZ",
      robots: { index: false, follow: false },
    };
  }
}

export default async function EpisodePage({ params }: PageProps) {
  const { id } = params;
  let episode: Episode | null = null;
  let previousEpisode: EpisodeLink | null = null;
  let nextEpisode: EpisodeLink | null = null;
  let series: SeriesWithSeasons | null = null;
  let seasonNumber: number | undefined;

  try {
    const response = await getEpisode(id);
    episode = response.episode;
    previousEpisode = response.previous_episode || null;
    nextEpisode = response.next_episode || null;
    if (episode?.series_slug) {
      series = await getSeriesBySlug(episode.series_slug);
      seasonNumber = series.seasons.find(s => s.season.id === episode!.season_id)?.season.season_number;
    }
  } catch (error) {
    console.error("Failed to fetch episode:", error);
  }

  const movieData = episode ? {
    id: episode.id,
    title: episode.title,
    description: episode.description || "",
    video_url: episode.video_url || "",
    embed_url: episode.embed_url || "",
    master_playlist_url: episode.video_url || "",
    source_type: episode.source_type || "iframe_embed",
    poster_url: series?.series.poster_url || "",
    backdrop_url: series?.series.backdrop_url || series?.series.poster_url || "",
    slug: episode.id.toString(),
    genre: [],
    country: "",
    duration: episode.duration || 0,
    quality: "",
    code: series?.series.code || "",
    views: episode.views || 0,
    rating_avg: 0,
    rating_count: 0,
    created_at: "",
    updated_at: "",
    type: "episode" as const,
    series_slug: episode.series_slug,
    series_title: series?.series.title,
    episode_number: episode.episode_number,
    thumbnails_base_url: episode.thumbnails_base_url,
    thumbnail_interval: episode.thumbnail_interval,
  } : null;

  const episodeJsonLd = episode ? {
    "@context": "https://schema.org",
    "@type": "TVEpisode",
    name: episode.title,
    episodeNumber: episode.episode_number,
    url: `${SITE_URL}/episode/${episode.id}`,
    image: normalizeMediaUrl(episode.thumbnail_url || series?.series.backdrop_url || series?.series.poster_url, `${SITE_URL}/og-image.jpg`),
    description: episode.description || undefined,
    partOfSeason: seasonNumber ? {
      "@type": "TVSeason",
      seasonNumber,
    } : undefined,
    partOfSeries: series ? {
      "@type": "TVSeries",
      name: series.series.title,
      url: `${SITE_URL}/series/${series.series.slug}`,
    } : undefined,
  } : null;

  return (
    <>
      {episodeJsonLd && (
        <Script
          id="episode-schema"
          type="application/ld+json"
          dangerouslySetInnerHTML={{ __html: JSON.stringify(episodeJsonLd) }}
        />
      )}
      <Navbar />
      {episode ? (
        <>
          <WatchPageClient
            movie={movieData as any}
            episodeNavigation={{
              previousEpisode,
              nextEpisode,
            }}
          />
          <section className="max-w-6xl mx-auto px-3 sm:px-4 pb-4">
            <nav className="flex flex-wrap items-center gap-3 text-sm text-gray-300">
              {series && (
                <Link href={`/series/${series.series.slug}`} className="hover:text-white underline">
                  ← {series.series.title}
                </Link>
              )}
              {previousEpisode && (
                <Link href={`/episode/${previousEpisode.id}`} className="hover:text-white">
                  Oldingi: {previousEpisode.episode_number}-qism
                </Link>
              )}
              {nextEpisode && (
                <Link href={`/episode/${nextEpisode.id}`} className="hover:text-white">
                  Keyingi: {nextEpisode.episode_number}-qism →
                </Link>
              )}
            </nav>
          </section>
          <section className="max-w-6xl mx-auto px-3 sm:px-4 pb-6">
            <div className="mt-4 space-y-4">
              {series && (
                <div>
                  <h3 className="text-white font-display text-lg mb-2">Serial reytingi</h3>
                  <StarRating seriesId={series.series.id} readOnly />
                </div>
              )}
              <div className="flex items-center gap-3">
                <h3 className="text-white font-display text-lg">Bahoyingiz</h3>
                <StarRating episodeId={episode.id} />
              </div>
            </div>
          </section>
          <section className="max-w-6xl mx-auto px-3 sm:px-4 pb-12">
            <Comments targetType="episode" targetId={episode.id} />
          </section>
        </>
      ) : (
        <main className="min-h-screen pt-20 flex items-center justify-center">
          <div className="text-center">
            <h1 className="text-2xl text-white mb-4">Epizod topilmadi</h1>
            <Link href="/series" className="text-brand-red hover:underline">
              Seriallar ro'yxatiga qaytish
            </Link>
          </div>
        </main>
      )}
      <Footer />
    </>
  );
}
