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
    const res = await getMovies({ limit: 30 });
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
  const latestMovies = movies.slice(0, 6); // For carousel - newest 6 movies

  return (
    <>
      <Navbar />
      <main className="min-h-screen">
        {/* ── Hero Carousel (Latest Movies) ─────────────────────── */}
        <HeroCarousel movies={latestMovies} />

        {/* ── Genre Chips ──────────────────────────────────────────── */}
        <section className="max-w-7xl mx-auto px-4 mt-4 mb-2">
          <div className="flex gap-2 overflow-x-auto pb-2 scrollbar-hide">
            {GENRE_CHIPS.map((genre) => (
              <Link
                key={genre.slug}
                href={`/movies?genre=${genre.slug}`}
                className="shrink-0 px-4 py-1.5 bg-brand-card border border-brand-border text-gray-300 text-sm rounded-full hover:border-brand-red hover:text-brand-red transition-colors"
              >
                {genre.label}
              </Link>
            ))}
          </div>
        </section>

        {/* ── Homepage Banner Ad ────────────────────────────── */}
        <div className="max-w-7xl mx-auto px-4 mt-4">
          <WebsiteAdSlot placement="homepage_banner" />
        </div>

        {/* ── Continue Watching ─────────────────────────────── */}
        <ContinueWatchingSection />

        {/* ── New Movies ─────────────────────────────────────── */}
        {recent.length > 0 && (
          <section className="max-w-7xl mx-auto px-4 py-8">
            <div className="flex items-center justify-between mb-5">
              <h2 className="font-display text-xl sm:text-2xl tracking-wide text-white flex items-center gap-2">
                <Clapperboard size={20} className="text-brand-red" />
                Yangi filmlar
              </h2>
              <Link
                href="/movies"
                className="flex items-center gap-1 text-xs text-brand-red hover:text-orange-400 transition-colors"
              >
                Hammasi <ChevronRight size={14} />
              </Link>
            </div>
            <MovieCarousel movies={recent} />
          </section>
        )}

        {/* ── Trending ─────────────────────────────────────── */}
        {trending.length > 0 && (
          <section className="max-w-7xl mx-auto px-4 py-8">
            <div className="flex items-center justify-between mb-5">
              <h2 className="font-display text-xl sm:text-2xl tracking-wide text-white flex items-center gap-2">
                <Flame size={20} className="text-brand-red" />
                Trending
              </h2>
            </div>
            <MovieCarousel movies={trending} />
          </section>
        )}

        {/* ── Premium ─────────────────────────────────────── */}
        {movies.filter(m => m.is_premium).length > 0 && (
          <section className="max-w-7xl mx-auto px-4 py-8">
            <div className="flex items-center justify-between mb-5">
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
          <section className="max-w-7xl mx-auto px-4 py-8">
            <div className="flex items-center justify-between mb-5">
              <h2 className="font-display text-xl sm:text-2xl tracking-wide text-white flex items-center gap-2">
                <Film size={20} className="text-brand-red" />
                Yangi seriallar
              </h2>
              <Link
                href="/series"
                className="flex items-center gap-1 text-xs text-brand-red hover:text-orange-400 transition-colors"
              >
                Hammasi <ChevronRight size={14} />
              </Link>
            </div>
            <SeriesCarousel series={seriesData.slice(0, 10)} />
          </section>
        )}

        {/* ── Featured row ──────────────────────────────────── */}
        {featured.length > 0 && (
          <section className="max-w-7xl mx-auto px-4 py-8">
            <div className="flex items-center justify-between mb-5">
              <h2 className="font-display text-xl sm:text-2xl tracking-wide text-white flex items-center gap-2">
                <Clapperboard size={20} className="text-brand-red" />
                {t("home.featuredMovies")}
              </h2>
              <Link
                href="/movies"
                className="flex items-center gap-1 text-xs text-brand-red hover:text-orange-400 transition-colors"
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
    </>
  );
}
