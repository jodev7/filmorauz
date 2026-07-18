import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import WatchPageClient from "@/components/watch/WatchPageClient";
import Comments from "@/components/Comments";
import StarRating from "@/components/StarRating";
import SeriesCarousel from "@/components/SeriesCarousel";
import type { EpisodePageData } from "@/lib/episode-page-data";
import Link from "next/link";
import Script from "next/script";
import { buildEpisodeJsonLd } from "@/lib/episode-page-data";
import { buildSeriesPath } from "@/lib/content-routes";
import { getSeriesRecommendations } from "@/lib/series-api";

interface EpisodePageViewProps {
  data: EpisodePageData;
}

export default async function EpisodePageView({ data }: EpisodePageViewProps) {
  const { episode, series, previousEpisode, nextEpisode } = data;

  // Content-similar series (by genre / country / year), seeded from the parent
  // series — the "Sizga yoqishi mumkin" row for episode pages.
  let relatedSeries: Awaited<ReturnType<typeof getSeriesRecommendations>> = [];
  try {
    relatedSeries = await getSeriesRecommendations(series.series.id, 12);
  } catch {
    // Silently handle — the row just won't render.
  }

  const movieData = {
    id: episode.id,
    _id: episode.id,
    title: episode.title,
    description: episode.description || "",
    video_url: episode.video_url || "",
    embed_url: episode.embed_url || "",
    master_playlist_url: episode.video_url || "",
    source_type: episode.source_type || "iframe_embed",
    poster_url: series.series.poster_url || "",
    backdrop_url: series.series.backdrop_url || series.series.poster_url || "",
    slug: episode.id.toString(),
    genre: [],
    country: "",
    duration: episode.duration || 0,
    quality: "",
    code: series.series.code || "",
    views: episode.views || 0,
    rating_avg: 0,
    rating_count: 0,
    created_at: episode.created_at || "",
    updated_at: episode.updated_at || "",
    type: "episode" as const,
    series_slug: series.series.slug,
    series_title: series.series.title,
    episode_number: episode.episode_number,
    thumbnails_base_url: episode.thumbnails_base_url,
    thumbnail_interval: episode.thumbnail_interval,
  };

  return (
    <>
      <Script
        id="episode-schema"
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: JSON.stringify(buildEpisodeJsonLd(data)) }}
      />
      <Navbar />
      <WatchPageClient
        movie={movieData as any}
        progressTargetId={episode.id}
        seriesSeasons={series.seasons}
        seriesSlugForModal={series.series.slug}
        episodeNavigation={{
          previousEpisode: previousEpisode
            ? {
                id: previousEpisode.id,
                href: previousEpisode.href,
                title: previousEpisode.title,
                episode_number: previousEpisode.episode_number,
              }
            : null,
          nextEpisode: nextEpisode
            ? {
                id: nextEpisode.id,
                href: nextEpisode.href,
                title: nextEpisode.title,
                episode_number: nextEpisode.episode_number,
              }
            : null,
        }}
      />
      <section className="max-w-6xl mx-auto px-3 sm:px-4 pb-4">
        <nav className="flex flex-wrap items-center gap-3 text-sm text-gray-300">
          <Link href={buildSeriesPath(series.series.slug)} className="hover:text-white underline">
            ← {series.series.title}
          </Link>
          {previousEpisode && (
            <Link href={previousEpisode.href} className="hover:text-white">
              Oldingi: {previousEpisode.episode_number}-qism
            </Link>
          )}
          {nextEpisode && (
            <Link href={nextEpisode.href} className="hover:text-white">
              Keyingi: {nextEpisode.episode_number}-qism →
            </Link>
          )}
        </nav>
      </section>
      <section className="max-w-6xl mx-auto px-3 sm:px-4 pb-6">
        <div className="mt-4 space-y-4">
          <div>
            <h3 className="text-white font-display text-lg mb-2">Serial reytingi</h3>
            <StarRating seriesId={series.series.id} readOnly />
          </div>
          <div className="flex items-center gap-3">
            <h3 className="text-white font-display text-lg">Bahoyingiz</h3>
            <StarRating episodeId={episode.id} />
          </div>
        </div>
      </section>
      {relatedSeries.length > 0 && (
        <section className="max-w-6xl mx-auto px-3 sm:px-4 pb-8">
          <h2 className="font-display text-xl sm:text-2xl tracking-wide text-white mb-4">
            SIZGA YOQISHI MUMKIN
          </h2>
          <SeriesCarousel series={relatedSeries} />
        </section>
      )}
      <section className="max-w-6xl mx-auto px-3 sm:px-4 pb-12">
        <Comments targetType="episode" targetId={episode.id} />
      </section>
      <Footer />
    </>
  );
}
