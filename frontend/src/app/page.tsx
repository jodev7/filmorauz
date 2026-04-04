import type { Metadata } from "next";
import Link from "next/link";
import { Play, ChevronRight } from "lucide-react";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import MovieCarousel from "@/components/MovieCarousel";
import SeriesCarousel from "@/components/SeriesCarousel";
import ContinueWatchingSection from "@/components/home/ContinueWatchingSection";
import FeaturedCollectionsSection from "@/components/home/FeaturedCollectionsSection";
import { getMovies, getTrendingMovies, getFeaturedCollections, Movie } from "@/lib/api";
import { getSeries, Series } from "@/lib/series-api";
import { getTranslations } from "@/lib/i18n-server";

export const dynamic = "force-dynamic";

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL || "https://filmorauz.uz";

export async function generateMetadata(): Promise<Metadata> {
  return {
    title: "FILMORAUZ — Onlayn Kino",
    description: "FILMORAUZ — O'zbekistondagi eng yaxshi onlayn kinoteatr. Yangi filmlar, klassika va mashhur filmlarni bepul tomosha qiling.",
    openGraph: {
      title: "FILMORAUZ — Onlayn Kino",
      description: "FILMORAUZ — O'zbekistondagi eng yaxshi onlayn kinoteatr. Yangi filmlar, klassika va mashhur filmlarni bepul tomosha qiling.",
      url: SITE_URL,
      siteName: "FILMORAUZ",
      type: "website",
      locale: "uz_UZ",
      images: [
        {
          url: `${SITE_URL}/og-image.jpg`,
          width: 1200,
          height: 630,
          alt: "FILMORAUZ",
        },
      ],
    },
    twitter: {
      card: "summary_large_image",
      title: "FILMORAUZ — Onlayn Kino",
      description: "FILMORAUZ — O'zbekistondagi eng yaxshi onlayn kinoteatr. Yangi filmlar, klassika va mashhur filmlarni bepul tomosha qiling.",
      images: [`${SITE_URL}/og-image.jpg`],
    },
    alternates: {
      canonical: SITE_URL,
    },
  };
}

export default async function HomePage() {
  const { t } = getTranslations("uz");
  let movies: Movie[] = [];
  let trending: Movie[] = [];
  let featuredCollections: any[] = [];
  let seriesData: Series[] = [];
  
  try {
    const res = await getMovies({ limit: 20 });
    movies = res.data || [];
  } catch {
    // Silently handle — show empty state
  }

  // Fetch trending movies
  try {
    trending = await getTrendingMovies("24h", 12);
  } catch {
    // Silently handle — trending is optional
  }

  // Fetch featured collections
  try {
    featuredCollections = await getFeaturedCollections();
    console.log("Featured collections:", featuredCollections);
  } catch (err) {
    // Log error for debugging
    console.error("Failed to fetch featured collections:", err);
  }

  // Fetch series
  try {
    const seriesRes = await getSeries(1, 20);
    seriesData = seriesRes.data || [];
  } catch {
    // Silently handle — series is optional
  }

  const hero = movies[0];
  const featured = movies.slice(1, 7);
  const recent = movies.slice(0, 12);

  return (
    <>
      <Navbar />
      <main className="min-h-screen">
        {/* ── Hero ──────────────────────────────────────────── */}
        {hero ? (
          <section className="relative h-[85vh] min-h-[500px] flex items-end">
            {/* Backdrop */}
            <div className="absolute inset-0">
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src={hero.backdrop_url || hero.poster_url}
                alt={hero.title}
                className="w-full h-full object-cover"
              />
              {/* Gradient overlay */}
              <div className="absolute inset-0 bg-gradient-to-t from-brand-dark via-brand-dark/60 to-transparent" />
              <div className="absolute inset-0 bg-gradient-to-r from-brand-dark/80 to-transparent" />
            </div>

            {/* Content */}
            <div className="relative max-w-7xl mx-auto px-4 pb-16 sm:pb-20 w-full">
              <div className="max-w-2xl">
                {/* Genres */}
                <div className="flex gap-2 mb-4 flex-wrap">
                  {hero.genre?.slice(0, 3).map((g) => (
                    <span
                      key={g}
                      className="text-xs border border-brand-red/60 text-brand-red px-3 py-1 rounded-full"
                    >
                      {g}
                    </span>
                  ))}
                </div>

                <h1 className="font-display text-4xl sm:text-5xl md:text-7xl tracking-wide text-white leading-none mb-4">
                  {hero.title}
                </h1>

                <p className="text-gray-300 text-sm sm:text-base md:text-lg leading-relaxed line-clamp-3 mb-6 max-w-xl">
                  {hero.description}
                </p>

                <div className="flex items-center gap-3 flex-wrap">
                  <Link
                    href={`/watch/${hero.slug}`}
                    className="flex items-center gap-2 bg-brand-red hover:bg-orange-700 text-white font-semibold px-5 sm:px-7 py-3 rounded-xl transition-colors text-sm sm:text-base"
                  >
                    <Play size={18} fill="white" />
                    {t("common.watchNow")}
                  </Link>
                  <Link
                    href={`/movies/${hero.slug}`}
                    className="flex items-center gap-2 bg-white/10 hover:bg-white/20 text-white font-semibold px-5 sm:px-7 py-3 rounded-xl backdrop-blur-sm transition-colors border border-white/20 text-sm sm:text-base"
                  >
                    {t("home.moreInfo")}
                  </Link>
                  <div className="flex items-center gap-3 text-sm text-gray-400 ml-2">
                    <span>{hero.year}</span>
                    {hero.quality && (
                      <span className="border border-gray-600 px-2 py-0.5 rounded text-xs">
                        {hero.quality}
                      </span>
                    )}
                    {hero.duration > 0 && <span>{hero.duration} min</span>}
                  </div>
                </div>
              </div>
            </div>
          </section>
        ) : (
          /* Empty hero state */
          <section className="h-[60vh] flex items-center justify-center bg-gradient-to-b from-brand-card to-brand-dark">
            <div className="text-center px-4">
              <h1 className="font-display text-5xl sm:text-6xl text-white tracking-wide mb-4">
                FILMORA<span className="text-brand-red">UZ</span>
              </h1>
              <p className="text-gray-400 text-base sm:text-lg mb-6">
                Eng yaxshi filmlar, bitta joyda.
              </p>
              <Link
                href="/movies"
                className="bg-brand-red hover:bg-orange-700 text-white font-semibold px-8 py-3 rounded-xl transition-colors inline-block"
              >
                Kinolarni ko'rish
              </Link>
            </div>
          </section>
        )}

        {/* ── Continue Watching ─────────────────────────────── */}
        <ContinueWatchingSection />

        {/* ── Trending ─────────────────────────────────────── */}
        {trending.length > 0 && (
          <section className="max-w-7xl mx-auto px-4 py-12">
            <div className="flex items-center justify-between mb-6">
              <h2 className="font-display text-2xl sm:text-3xl tracking-wide text-white">
                TRENDING
              </h2>
            </div>
            <MovieCarousel movies={trending} />
          </section>
        )}

        {/* ── Featured Collections ──────────────────────────────────── */}
        <FeaturedCollectionsSection collections={featuredCollections} />

        {/* ── Series Sections ──────────────────────────────────── */}
        {seriesData.length > 0 && (
          <>
            {/* New Series */}
            <section className="max-w-7xl mx-auto px-4 py-12">
              <div className="flex items-center justify-between mb-6">
                <h2 className="font-display text-2xl sm:text-3xl tracking-wide text-white">
                  Yangi seriallar
                </h2>
                <Link
                  href="/series"
                  className="flex items-center gap-1 text-sm text-brand-red hover:text-orange-400 transition-colors"
                >
                  Hammasi <ChevronRight size={16} />
                </Link>
              </div>
              <SeriesCarousel series={seriesData.slice(0, 10)} />
            </section>

            {/* Popular Series (reuse same data for now) */}
            <section className="max-w-7xl mx-auto px-4 pb-12">
              <div className="flex items-center justify-between mb-6">
                <h2 className="font-display text-2xl sm:text-3xl tracking-wide text-white">
                  Mashhur seriallar
                </h2>
                <Link
                  href="/series"
                  className="flex items-center gap-1 text-sm text-brand-red hover:text-orange-400 transition-colors"
                >
                  Hammasi <ChevronRight size={16} />
                </Link>
              </div>
              <SeriesCarousel series={seriesData.slice(0, 10)} />
            </section>
          </>
        )}

        {/* ── Featured row ──────────────────────────────────── */}
        {featured.length > 0 && (
          <section className="max-w-7xl mx-auto px-4 py-12">
            <div className="flex items-center justify-between mb-6">
              <h2 className="font-display text-2xl sm:text-3xl tracking-wide text-white">
                {t("home.featuredMovies")}
              </h2>
              <Link
                href="/movies"
                className="flex items-center gap-1 text-sm text-brand-red hover:text-orange-400 transition-colors"
              >
                {t("common.seeAll")} <ChevronRight size={16} />
              </Link>
            </div>
            <MovieCarousel movies={featured} />
          </section>
        )}

        {/* ── New Releases ──────────────────────────────────── */}
        {recent.length > 0 && (
          <section className="max-w-7xl mx-auto px-4 pb-16">
            <div className="flex items-center justify-between mb-6">
              <h2 className="font-display text-2xl sm:text-3xl tracking-wide text-white">
                {t("home.newReleases")}
              </h2>
              <Link
                href="/movies"
                className="flex items-center gap-1 text-sm text-brand-red hover:text-orange-400 transition-colors"
              >
                Hammasi <ChevronRight size={16} />
              </Link>
            </div>
            <MovieCarousel movies={recent} />
          </section>
        )}

        {/* Empty state */}
        {movies.length === 0 && (
          <section className="max-w-7xl mx-auto px-4 py-24 text-center">
            <p className="text-gray-500 text-lg">
              Hali kinolar yo'q.{" "}
              <Link href="/admin/login" className="text-brand-red hover:underline">
                Admin panelida kino qo'shing.
              </Link>
            </p>
          </section>
        )}
      </main>
      <Footer />
    </>
  );
}
