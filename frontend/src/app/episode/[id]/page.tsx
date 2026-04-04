import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import WatchPageClient from "@/components/watch/WatchPageClient";
import { getEpisode, getSeriesBySlug, Episode, SeriesWithSeasons } from "@/lib/series-api";
import { Metadata } from "next";
import Link from "next/link";

interface PageProps {
  params: { id: string };
}

export async function generateMetadata({ params }: PageProps): Promise<Metadata> {
  const { id } = params;
  
  try {
    const episode = await getEpisode(id);
    return {
      title: `${episode.title} — FILMORAUZ`,
      description: `Epizod: ${episode.title}`,
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
  let series: SeriesWithSeasons | null = null;
  
  try {
    episode = await getEpisode(id);
    if (episode?.series_slug) {
      series = await getSeriesBySlug(episode.series_slug);
    }
  } catch (error) {
    console.error("Failed to fetch episode:", error);
  }

  const videoUrl = episode?.video_url;
  const movieData = episode ? {
    id: episode.id,
    title: episode.title,
    description: episode.description || "",
    poster_url: series?.series.poster_url || "",
    slug: episode.id.toString(),
    type: "episode" as const,
    series_title: series?.series.title,
    episode_number: episode.episode_number,
  } : null;

  return (
    <>
      <Navbar />
      {episode ? (
        <WatchPageClient movie={movieData as any} />
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
