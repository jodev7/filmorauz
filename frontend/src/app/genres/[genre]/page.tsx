import type { Metadata } from "next";
import Link from "next/link";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import MovieCard from "@/components/MovieCard";
import SeriesCarousel from "@/components/SeriesCarousel";
import { getMovies } from "@/lib/api";
import { getSeries } from "@/lib/series-api";
import { SITE_URL } from "@/lib/content-routes";
import { localizeSingleGenre } from "@/lib/localization";

export const dynamic = "force-dynamic";

interface Props {
  params: { genre: string };
}

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const genre = params.genre.toLowerCase();
  const label = localizeSingleGenre(genre);
  const canonical = `${SITE_URL}/genres/${encodeURIComponent(genre)}`;
  const title = `${label} janridagi kinolar va seriallar — FilmoraUz`;
  const description = `${label} janridagi kinolar va seriallarni o'zbek tilida online tomosha qiling. FilmoraUz.net`;

  return {
    title,
    description,
    alternates: { canonical },
    openGraph: {
      title,
      description,
      url: canonical,
      siteName: "FILMORAUZ",
      type: "website",
      locale: "uz_UZ",
    },
    twitter: {
      card: "summary",
      title,
      description,
    },
    robots: { index: true, follow: true },
  };
}

export default async function GenreDetailPage({ params }: Props) {
  const genre = params.genre.toLowerCase();
  const label = localizeSingleGenre(genre);

  const [moviesRes, seriesRes] = await Promise.allSettled([
    getMovies({ genre, page: 1, limit: 24 }),
    getSeries(1, 30, genre),
  ]);

  const movies = moviesRes.status === "fulfilled" ? moviesRes.value.data || [] : [];
  const series = seriesRes.status === "fulfilled" ? seriesRes.value.data || [] : [];

  return (
    <>
      <Navbar />
      <main className="min-h-screen pt-20 sm:pt-24">
        <div className="max-w-7xl mx-auto px-4 pb-12">
          <div className="mb-8">
            <Link href="/genres" className="text-sm text-brand-red hover:underline">
              ← Janrlar
            </Link>
            <h1 className="font-display text-3xl sm:text-4xl md:text-5xl text-white tracking-wide mt-3 mb-2">
              {label.toUpperCase()}
            </h1>
            <p className="text-gray-400">
              {movies.length + series.length} ta natija
            </p>
          </div>

          {movies.length > 0 && (
            <section className="mb-12">
              <h2 className="font-display text-2xl text-white tracking-wide mb-4">KINOLAR</h2>
              <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-3 sm:gap-4 isolate">
                {movies.map((movie) => (
                  <div key={movie.id} className="isolate">
                    <MovieCard movie={movie} />
                  </div>
                ))}
              </div>
            </section>
          )}

          {series.length > 0 && (
            <section>
              <h2 className="font-display text-2xl text-white tracking-wide mb-4">SERIALLAR</h2>
              <SeriesCarousel series={series} />
            </section>
          )}

          {movies.length === 0 && series.length === 0 && (
            <div className="py-24 text-center text-gray-500">
              <p className="text-lg">Bu janr uchun hozircha kontent topilmadi.</p>
            </div>
          )}
        </div>
      </main>
      <Footer />
    </>
  );
}
