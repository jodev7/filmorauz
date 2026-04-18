import type { Metadata } from "next";
import Link from "next/link";
import { Play, ChevronRight, Flame, Clapperboard, Sparkles, Film } from "lucide-react";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import MovieCarousel from "@/components/MovieCarousel";
import SeriesCarousel from "@/components/SeriesCarousel";
import ContinueWatchingSection from "@/components/home/ContinueWatchingSection";
import FeaturedCollectionsSection from "@/components/home/FeaturedCollectionsSection";
import HeroCarousel from "@/components/home/HeroCarousel";
import WebsiteAdSlot from "@/components/ads/WebsiteAdSlot";
import { getMovies, getTrendingMovies, getFeaturedCollections, Movie } from "@/lib/api";
import { getSeries, Series } from "@/lib/series-api";
import { getTranslations } from "@/lib/i18n-server";

const GENRE_CHIPS = [
  { label: "Jangari", slug: "action" },
  { label: "Drama", slug: "drama" },
  { label: "Komediya", slug: "comedy" },
  { label: "Fantastika", slug: "sci-fi" },
  { label: "Triller", slug: "thriller" },
  { label: "Multfilm", slug: "animation" },
  { label: "Romantika", slug: "romance" },
  { label: "Daxvo", slug: "horror" },
  { label: "Klassika", slug: "classic" },
];

// ISR: regenerate the homepage shell every 60s. User-specific sections
// (ContinueWatchingSection, ads) are client components and still fetch
// per-session on their own.
export const revalidate = 60;

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL || "https://filmorauz.net";

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

  // Run all four public fetches in parallel. Each has ISR caching (revalidate=60)
  // so Next.js dedupes them across renders. allSettled keeps the page rendering
  // even if one endpoint is down.
  const fetchStartedAt = Date.now();
  const [moviesRes, trendingRes, collectionsRes, seriesRes] = await Promise.allSettled([
    getMovies({ limit: 30 }),
    getTrendingMovies("24h", 12),
    getFeaturedCollections(),
    getSeries(1, 20),
  ]);
  console.log(`[HomePage] parallel fetches done in ${Date.now() - fetchStartedAt}ms`, {
    movies: moviesRes.status,
    trending: trendingRes.status,
    collections: collectionsRes.status,
    series: seriesRes.status,
  });

  if (moviesRes.status === "fulfilled") movies = moviesRes.value.data || [];
  if (trendingRes.status === "fulfilled") trending = trendingRes.value;
  if (collectionsRes.status === "fulfilled") featuredCollections = collectionsRes.value;
  else console.error("[HomePage] featured collections failed:", collectionsRes.reason);
  if (seriesRes.status === "fulfilled") seriesData = seriesRes.value.data || [];

  const hero = movies[0];
  const featured = movies.slice(1, 7);
  const recent = movies.slice(0, 12);
  const latestMovies = movies.slice(0, 6); // For carousel - newest 6 movies

  return (
    <>
      <Navbar />
      <main className="min-h-screen">
        {/* ── Hero Carousel (Latest Movies) ─────────────────────── */}
        <HeroCarousel movies={latestMovies} />

        {/* ── Genre Chips ──────────────────────────────────────────── */}
        <section className="max-w-7xl mx-auto px-4 mt-6 mb-4">
          <div className="flex gap-2.5 overflow-x-auto pb-2 scrollbar-hide">
            {GENRE_CHIPS.map((genre) => (
              <Link
                key={genre.slug}
                href={`/movies?genre=${genre.slug}`}
                className="shrink-0 px-4 py-2 bg-[#12121a] border border-[#1e1e2e] text-gray-400 text-sm rounded-full hover:border-orange-500 hover:text-orange-500 transition-colors"
              >
                {genre.label}
              </Link>
            ))}
          </div>
        </section>

        {/* ── Homepage Top Banner Ad ────────────────────────── */}
        <div className="max-w-7xl mx-auto px-4 mt-8 mb-6">
          <WebsiteAdSlot placement="homepage_top_banner" variant="banner" />
        </div>

        {/* ── Continue Watching ─────────────────────────────── */}
        <ContinueWatchingSection />

        {/* ── New Movies ─────────────────────────────────────── */}
        {recent.length > 0 && (
          <section className="max-w-7xl mx-auto px-4 py-6">
            <div className="flex items-center justify-between mb-4">
              <h2 className="font-display text-xl sm:text-2xl tracking-wide text-white flex items-center gap-2">
                <Clapperboard size={20} className="text-orange-500" />
                Yangi filmlar
              </h2>
              <Link
                href="/movies"
                className="flex items-center gap-1 text-xs text-orange-500 hover:text-orange-400 transition-colors"
              >
                Hammasi <ChevronRight size={14} />
              </Link>
            </div>
            <MovieCarousel movies={recent} />
          </section>
        )}

        {/* ── Trending ─────────────────────────────────────── */}
        {trending.length > 0 && (
          <section className="max-w-7xl mx-auto px-4 py-6">
            <div className="flex items-center justify-between mb-4">
              <h2 className="font-display text-xl sm:text-2xl tracking-wide text-white flex items-center gap-2">
                <Flame size={20} className="text-orange-500" />
                Trending
              </h2>
            </div>
            <MovieCarousel movies={trending} />
          </section>
        )}

        {/* ── Premium ─────────────────────────────────────── */}
        {movies.filter(m => m.is_premium).length > 0 && (
          <section className="max-w-7xl mx-auto px-4 py-6">
            <div className="flex items-center justify-between mb-4">
              <h2 className="font-display text-xl sm:text-2xl tracking-wide text-white flex items-center gap-2">
                <Sparkles size={20} className="text-yellow-500" />
                Premium
              </h2>
              <Link
                href="/movies?premium=true"
                className="flex items-center gap-1 text-xs text-yellow-500 hover:text-yellow-400 transition-colors"
              >
                Hammasi <ChevronRight size={14} />
              </Link>
            </div>
            <MovieCarousel movies={movies.filter(m => m.is_premium).slice(0, 12)} />
          </section>
        )}

        {/* ── Featured Collections ──────────────────────────────────── */}
        <FeaturedCollectionsSection collections={featuredCollections} />

        {/* ── New Series ──────────────────────────────────── */}
        {seriesData.length > 0 && (
          <section className="max-w-7xl mx-auto px-4 py-6">
            <div className="flex items-center justify-between mb-4">
              <h2 className="font-display text-xl sm:text-2xl tracking-wide text-white flex items-center gap-2">
                <Film size={20} className="text-orange-500" />
                Yangi seriallar
              </h2>
              <Link
                href="/series"
                className="flex items-center gap-1 text-xs text-orange-500 hover:text-orange-400 transition-colors"
              >
                Hammasi <ChevronRight size={14} />
              </Link>
            </div>
            <SeriesCarousel series={seriesData.slice(0, 10)} />
          </section>
        )}

        {/* ── Homepage Inline Block ────────────────────────── */}
        <div className="max-w-7xl mx-auto px-4 mt-10 mb-8">
          <WebsiteAdSlot placement="homepage_inline_block_1" variant="inline" />
        </div>

        {/* ── Featured row ──────────────────────────────────── */}
        {featured.length > 0 && (
          <section className="max-w-7xl mx-auto px-4 py-6">
            <div className="flex items-center justify-between mb-4">
              <h2 className="font-display text-xl sm:text-2xl tracking-wide text-white flex items-center gap-2">
                <Clapperboard size={20} className="text-orange-500" />
                {t("home.featuredMovies")}
              </h2>
              <Link
                href="/movies"
                className="flex items-center gap-1 text-xs text-orange-500 hover:text-orange-400 transition-colors"
              >
                {t("common.seeAll")} <ChevronRight size={14} />
              </Link>
            </div>
            <MovieCarousel movies={featured} />
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
      {/* Popup ad — renders as modal overlay when an active ad is available */}
      <WebsiteAdSlot placement="homepage_popup" popup />
    </>
  );
}
