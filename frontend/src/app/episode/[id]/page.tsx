import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import WatchPageClient from "@/components/watch/WatchPageClient";
import Comments from "@/components/Comments";
import StarRating from "@/components/StarRating";
import { getEpisode, getSeriesBySlug, Episode, EpisodeLink, SeriesWithSeasons } from "@/lib/series-api";
import { Metadata } from "next";
import Link from "next/link";

interface PageProps {
  params: { id: string };
}

export async function generateMetadata({ params }: PageProps): Promise<Metadata> {
  const { id } = params;
  
  try {
    const response = await getEpisode(id);
    return {
      title: `${response.episode.title} — FILMORAUZ`,
      description: `Epizod: ${response.episode.title}`,
    };
  } catch {
    return {
      title: "Epizod topilmadi — FILMORAUZ",
    };
  }
}

export default async function EpisodePage({ params }: PageProps) {
  const { id } = params;
  let episode: Episode | null = null;
  let previousEpisode: EpisodeLink | null = null;
  let nextEpisode: EpisodeLink | null = null;
  let series: SeriesWithSeasons | null = null;
  
  try {
    const response = await getEpisode(id);
    episode = response.episode;
    previousEpisode = response.previous_episode || null;
    nextEpisode = response.next_episode || null;
    if (episode?.series_slug) {
      series = await getSeriesBySlug(episode.series_slug);
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
    views: 0,
    rating_avg: 0,
    rating_count: 0,
    created_at: "",
    updated_at: "",
    type: "episode" as const,
    series_slug: episode.series_slug,
    series_title: series?.series.title,
    episode_number: episode.episode_number,
  } : null;

  return (
    <>
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
          <section className="max-w-6xl mx-auto px-3 sm:px-4 pb-6">
            <div className="flex items-center gap-3 mt-4">
              <h3 className="text-white font-display text-lg">Bahoyingiz</h3>
              <StarRating episodeId={episode.id} />
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
