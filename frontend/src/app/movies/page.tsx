import type { Metadata } from "next";
export const dynamic = "force-dynamic";
import { Suspense } from "react";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import MovieCard from "@/components/MovieCard";
import SeriesCard from "@/components/SeriesCard";
import GenreFilter from "@/components/GenreFilter";
import WebsiteAdSlot from "@/components/ads/WebsiteAdSlot";
import { getMovies, searchMovies, Movie } from "@/lib/api";
import { getSeries, type Series } from "@/lib/series-api";
import { getTranslations } from "@/lib/i18n-server";
import { localizeSingleGenre } from "@/lib/localization";
import Link from "next/link";
import { ChevronLeft, ChevronRight } from "lucide-react";

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL || "https://filmorauz.net";

export async function generateMetadata({
  searchParams,
}: {
  searchParams?: { genre?: string };
}): Promise<Metadata> {
  const genre = searchParams?.genre?.trim();
  if (genre) {
    const localized = localizeSingleGenre(genre);
    const cap = localized.charAt(0).toUpperCase() + localized.slice(1);
    const title = `${cap} kinolar o'zbek tilida | FilmoraUz`;
    const description = `${cap} janridagi kinolar va seriallarni o'zbek tilida HD sifatda tomosha qiling.`;
    const canonical = `${SITE_URL}/movies?genre=${encodeURIComponent(genre)}`;
    return {
      title,
      description,
      openGraph: { title, description, url: canonical, siteName: "FILMORAUZ", type: "website", locale: "uz_UZ" },
      twitter: { card: "summary", title, description },
      alternates: { canonical },
    };
  }
  const title = "Barcha kinolar o'zbek tilida | FilmoraUz";
  const description = "FilmoraUz'da barcha kinolarni o'zbek tilida HD sifatda ko'ring. Janr bo'yicha filtr qiling, nom bo'yicha qidiring.";
  return {
    title,
    description,
    openGraph: { title, description, url: `${SITE_URL}/movies`, siteName: "FILMORAUZ", type: "website", locale: "uz_UZ" },
    twitter: { card: "summary", title, description },
    alternates: { canonical: `${SITE_URL}/movies` },
  };
}

interface Props {
  searchParams: { genre?: string; page?: string; search?: string };
}

export default async function MoviesPage({ searchParams }: Props) {
  const { t } = getTranslations("uz");
  const genre = searchParams.genre || "";
  const page = parseInt(searchParams.page || "1");
  const search = searchParams.search || "";
  const limit = 24;

  let movies: Movie[] = [];
  let total = 0;
  // Genre nav links (Dorama, Anime, Multifilmlar…) point here, but those
  // genres span both movies AND series. When browsing by genre (not a free
  // text search) also pull the matching series so the page shows everything
  // under that genre, not just movies. Only the first page of series is
  // shown inline; "Barcha seriallar" links to the full /series list.
  let series: Series[] = [];

  try {
    if (search) {
      movies = await searchMovies(search);
      total = movies.length;
    } else {
      const res = await getMovies({ genre, page, limit });
      movies = res.data || [];
      total = res.total;
    }
  } catch {
    // show empty state
  }

  if (genre && !search && page === 1) {
    try {
      const sres = await getSeries(1, limit, genre);
      series = sres.data || [];
    } catch {
      // non-fatal — just omit the series section
    }
  }

  const totalPages = Math.ceil(total / limit);

  const pageTitle = search
    ? `${t("movies.searchPrefix")} "${search}"`
    : genre
    ? localizeSingleGenre(genre).toUpperCase()
    : "BARCHA FILMLAR";

  return (
    <>
      <Navbar />
      <main className="min-h-screen pt-20 sm:pt-24">
        <div className="max-w-7xl mx-auto px-4">
          <div className="mb-6 sm:mb-8">
            <h1 className="font-display text-3xl sm:text-4xl md:text-5xl text-white tracking-wide mb-2">
              {pageTitle}
            </h1>
            <p className="text-gray-500 text-sm">
              {total} ta film
              {series.length > 0 ? ` · ${series.length} ta serial` : ""} topildi
            </p>
          </div>

          <div className="mb-6">
            <WebsiteAdSlot placement="list_page_banner" variant="banner" />
          </div>

          {!search && (
            <div className="mb-6 sm:mb-8 overflow-x-auto pb-2">
              <Suspense>
                <GenreFilter />
              </Suspense>
            </div>
          )}

          {search && (
            <div className="mb-6">
              <Link href="/movies" className="text-sm text-brand-red hover:underline">
                ← Qidiruvni tozalash
              </Link>
            </div>
          )}

          {/* Seriallar — shown when browsing a genre that also has series. */}
          {series.length > 0 && (
            <section className="mb-10">
              <div className="flex items-center justify-between mb-3 sm:mb-4">
                <h2 className="font-display text-xl sm:text-2xl text-white tracking-wide">
                  Seriallar
                </h2>
                <Link
                  href={`/series?genre=${encodeURIComponent(genre)}`}
                  className="text-sm text-brand-red hover:underline whitespace-nowrap"
                >
                  Barcha seriallar →
                </Link>
              </div>
              <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-3 sm:gap-4 isolate">
                {series.map((s) => (
                  <div key={s.id} className="isolate">
                    <SeriesCard series={s} />
                  </div>
                ))}
              </div>
            </section>
          )}

          {movies.length > 0 ? (
            <section>
              {series.length > 0 && (
                <h2 className="font-display text-xl sm:text-2xl text-white tracking-wide mb-3 sm:mb-4">
                  Kinolar
                </h2>
              )}
              <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-3 sm:gap-4 isolate">
                {movies.map((movie) => (
                  <div key={movie.id} className="isolate">
                    <MovieCard movie={movie} />
                  </div>
                ))}
              </div>
            </section>
          ) : series.length === 0 ? (
            <div className="py-24 text-center text-gray-500">
              <p className="text-lg">{t("movies.noResults")}</p>
              {search && <p className="text-sm mt-2">{t("movies.noResultsHint")}</p>}
            </div>
          ) : null}

          {!search && totalPages > 1 && (
            <div className="flex items-center justify-center gap-4 mt-12 pb-8 flex-wrap">
              {page > 1 && (
                <Link
                  href={`/movies?${new URLSearchParams({ ...(genre && { genre }), page: String(page - 1) })}`}
                  className="flex items-center gap-1 px-4 sm:px-5 py-2 glass-card border border-white/10 rounded-lg text-sm text-gray-300 hover:text-white hover:border-gray-500 transition-colors"
                >
                  <ChevronLeft size={16} />
                  Oldingi
                </Link>
              )}
              <span className="text-sm text-gray-500">Sahifa {page} / {totalPages}</span>
              {page < totalPages && (
                <Link
                  href={`/movies?${new URLSearchParams({ ...(genre && { genre }), page: String(page + 1) })}`}
                  className="flex items-center gap-1 px-4 sm:px-5 py-2 glass-card border border-white/10 rounded-lg text-sm text-gray-300 hover:text-white hover:border-gray-500 transition-colors"
                >
                  Keyingi
                  <ChevronRight size={16} />
                </Link>
              )}
            </div>
          )}
        </div>
      </main>
      <Footer />
    </>
  );
}
