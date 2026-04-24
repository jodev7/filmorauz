import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import WatchPageClient from "@/components/watch/WatchPageClient";
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
    source_type: episode.source_type || "iframe_embed",
    poster_url: series?.series.poster_url || "",
    backdrop_url: series?.series.backdrop_url || series?.series.poster_url || "",
    slug: episode.id.toString(),
    genre: [],
    country: "",
    duration: episode.duration || 0,
    quality: "",
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
          <WatchPageClient movie={movieData as any} />
          {(previousEpisode || nextEpisode) && (
            <section className="px-4 pb-8">
              <div className="mx-auto max-w-7xl">
                <div className="rounded-2xl border border-brand-border bg-brand-card/70 p-4">
                  <div className="flex flex-col gap-3 sm:flex-row sm:justify-between">
                    {previousEpisode ? (
                      <Link
                        href={`/episode/${previousEpisode.id}`}
                        className="inline-flex items-center justify-center rounded-xl border border-brand-border bg-brand-dark px-4 py-3 text-sm font-medium text-white transition-colors hover:border-brand-red hover:text-brand-red sm:min-w-[180px]"
                      >
                        Oldingi qism
                      </Link>
                    ) : <div />}
                    {nextEpisode ? (
                      <Link
                        href={`/episode/${nextEpisode.id}`}
                        className="inline-flex items-center justify-center rounded-xl border border-brand-border bg-brand-dark px-4 py-3 text-sm font-medium text-white transition-colors hover:border-brand-red hover:text-brand-red sm:min-w-[180px]"
                      >
                        Keyingi qism
                      </Link>
                    ) : <div />}
                  </div>
                </div>
              </div>
            </section>
          )}
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
