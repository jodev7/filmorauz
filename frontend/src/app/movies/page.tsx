import type { Metadata } from "next";
export const dynamic = "force-dynamic";
import { Suspense } from "react";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import MovieCard from "@/components/MovieCard";
import GenreFilter from "@/components/GenreFilter";
import WebsiteAdSlot from "@/components/ads/WebsiteAdSlot";
import { getMovies, searchMovies, Movie } from "@/lib/api";
import { getTranslations } from "@/lib/i18n-server";
import Link from "next/link";
import { ChevronLeft, ChevronRight } from "lucide-react";

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL || "https://filmorauz.uz";

export async function generateMetadata(): Promise<Metadata> {
  return {
    title: "Barcha Filmlar — FILMORAUZ",
    description: "FILMORAUZ'da barcha filmlarni ko'ring. Janr bo'yicha filtr qiling, nom bo'yicha qidiring.",
    openGraph: {
      title: "Barcha Filmlar",
      description: "FILMORAUZ'da barcha filmlarni ko'ring. Janr bo'yicha filtr qiling, nom bo'yicha qidiring.",
      url: `${SITE_URL}/movies`,
      siteName: "FILMORAUZ",
      type: "website",
      locale: "uz_UZ",
    },
    twitter: {
      card: "summary",
      title: "Barcha Filmlar",
      description: "FILMORAUZ'da barcha filmlarni ko'ring. Janr bo'yicha filtr qiling, nom bo'yicha qidiring.",
    },
    alternates: {
      canonical: `${SITE_URL}/movies`,
    },
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

  const totalPages = Math.ceil(total / limit);

  const pageTitle = search
    ? `${t("movies.searchPrefix")} "${search}"`
    : genre
    ? genre.toUpperCase()
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
              {total} ta film topildi
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

          {movies.length > 0 ? (
            <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-3 sm:gap-4 isolate">
              {movies.map((movie) => (
                <div key={movie.id} className="isolate">
                  <MovieCard movie={movie} />
                </div>
              ))}
            </div>
          ) : (
            <div className="py-24 text-center text-gray-500">
              <p className="text-lg">{t("movies.noResults")}</p>
              {search && <p className="text-sm mt-2">{t("movies.noResultsHint")}</p>}
            </div>
          )}

          {!search && totalPages > 1 && (
            <div className="flex items-center justify-center gap-4 mt-12 pb-8 flex-wrap">
              {page > 1 && (
                <Link
                  href={`/movies?${new URLSearchParams({ ...(genre && { genre }), page: String(page - 1) })}`}
                  className="flex items-center gap-1 px-4 sm:px-5 py-2 bg-brand-card border border-brand-border rounded-lg text-sm text-gray-300 hover:text-white hover:border-gray-500 transition-colors"
                >
                  <ChevronLeft size={16} />
                  Oldingi
                </Link>
              )}
              <span className="text-sm text-gray-500">Sahifa {page} / {totalPages}</span>
              {page < totalPages && (
                <Link
                  href={`/movies?${new URLSearchParams({ ...(genre && { genre }), page: String(page + 1) })}`}
                  className="flex items-center gap-1 px-4 sm:px-5 py-2 bg-brand-card border border-brand-border rounded-lg text-sm text-gray-300 hover:text-white hover:border-gray-500 transition-colors"
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
