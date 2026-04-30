import type { Metadata } from "next";
import dynamic from "next/dynamic";
import Link from "next/link";
import { ChevronRight, Flame, Clapperboard, Sparkles, Film } from "lucide-react";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import MovieCarousel from "@/components/MovieCarousel";
import SeriesCarousel from "@/components/SeriesCarousel";
import HeroCarousel from "@/components/home/HeroCarousel";
import { getHomepageData } from "@/lib/api";
import { getTranslations } from "@/lib/i18n-server";

const FeaturedCollectionsSection = dynamic(() => import("@/components/home/FeaturedCollectionsSection"));
const WebsiteAdSlot = dynamic(() => import("@/components/ads/WebsiteAdSlot"));

// ISR: regenerate the homepage shell every 60s. User-specific sections (ads)
// are client components and still fetch per-session on their own.
export const revalidate = 60;

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL || "https://filmorauz.net";

export async function generateMetadata(): Promise<Metadata> {
  return {
    title: "FilmoraUz — Kinolar va seriallar o'zbek tilida",
    description: "FilmoraUz'da kinolar va seriallarni o'zbek tilida HD sifatda tomosha qiling.",
    openGraph: {
      title: "FilmoraUz — Kinolar va seriallar o'zbek tilida",
      description: "FilmoraUz'da kinolar va seriallarni o'zbek tilida HD sifatda tomosha qiling.",
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
      title: "FilmoraUz — Kinolar va seriallar o'zbek tilida",
      description: "FilmoraUz'da kinolar va seriallarni o'zbek tilida HD sifatda tomosha qiling.",
      images: [`${SITE_URL}/og-image.jpg`],
    },
    alternates: {
      canonical: SITE_URL,
    },
  };
}

export default async function HomePage() {
  const { t } = getTranslations("uz");
  const homepage = await getHomepageData();
  const featured = homepage.featured_movies || [];
  const recent = homepage.new_movies || [];
  const latestMovies = homepage.hero || [];
  const trending = homepage.trending || [];
  const featuredCollections = homepage.featured_collections || [];
  const seriesData = homepage.series || [];
  const genreChips = homepage.genres || [];

  return (
    <>
      <Navbar />
      <main className="min-h-screen">
        {/* ── Hero Carousel (Latest Movies) ─────────────────────── */}
        <HeroCarousel movies={latestMovies} />

        {/* ── Genre Chips ──────────────────────────────────────────── */}
        <section className="max-w-7xl mx-auto px-4 mt-6 mb-4">
          <div className="flex gap-2.5 overflow-x-auto pb-2 scrollbar-hide">
            {genreChips.map((genre) => (
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

        {/* ── Homepage Top Banner Ad — lazy; shared prefetch serves it ─ */}
        <div className="max-w-7xl mx-auto px-4 mt-8 mb-6">
          <WebsiteAdSlot placement="homepage_top_banner" variant="banner" lazy />
        </div>

        {/* ── New Movies — only carousel with priority posters (above-fold) ── */}
        {recent.length > 0 && (
          <section className="max-w-7xl mx-auto px-4 py-6">
            <div className="flex items-center justify-between mb-4">
              <h2 className="font-display text-xl sm:text-2xl tracking-wide text-white flex items-center gap-2">
                <Clapperboard size={20} className="text-orange-500" aria-hidden="true" />
                Yangi filmlar
              </h2>
              <Link
                href="/movies"
                className="flex items-center gap-1 text-xs text-orange-500 hover:text-orange-400 transition-colors"
              >
                Hammasi <ChevronRight size={14} aria-hidden="true" />
              </Link>
            </div>
            <MovieCarousel movies={recent} priorityCount={4} />
          </section>
        )}

        {/* ── Trending ─────────────────────────────────────── */}
        {trending.length > 0 && (
          <section className="max-w-7xl mx-auto px-4 py-6">
            <div className="flex items-center justify-between mb-4">
              <h2 className="font-display text-xl sm:text-2xl tracking-wide text-white flex items-center gap-2">
                <Flame size={20} className="text-orange-500" aria-hidden="true" />
                Mashhur
              </h2>
            </div>
            <MovieCarousel movies={trending} />
          </section>
        )}

        {/* ── Premium ─────────────────────────────────────── */}
        {(homepage.premium_movies || []).length > 0 && (
          <section className="max-w-7xl mx-auto px-4 py-6">
            <div className="flex items-center justify-between mb-4">
              <h2 className="font-display text-xl sm:text-2xl tracking-wide text-white flex items-center gap-2">
                <Sparkles size={20} className="text-yellow-500" aria-hidden="true" />
                Premium
              </h2>
              <Link
                href="/movies?premium=true"
                className="flex items-center gap-1 text-xs text-yellow-500 hover:text-yellow-400 transition-colors"
              >
                Hammasi <ChevronRight size={14} aria-hidden="true" />
              </Link>
            </div>
            <MovieCarousel movies={homepage.premium_movies || []} />
          </section>
        )}

        {/* ── Featured Collections ──────────────────────────────────── */}
        <FeaturedCollectionsSection collections={featuredCollections} />

        {/* ── New Series ──────────────────────────────────── */}
        {seriesData.length > 0 && (
          <section className="max-w-7xl mx-auto px-4 py-6">
            <div className="flex items-center justify-between mb-4">
              <h2 className="font-display text-xl sm:text-2xl tracking-wide text-white flex items-center gap-2">
                <Film size={20} className="text-orange-500" aria-hidden="true" />
                Yangi seriallar
              </h2>
              <Link
                href="/series"
                className="flex items-center gap-1 text-xs text-orange-500 hover:text-orange-400 transition-colors"
              >
                Hammasi <ChevronRight size={14} aria-hidden="true" />
              </Link>
            </div>
            <SeriesCarousel series={seriesData.slice(0, 10)} />
          </section>
        )}

        {/* ── Homepage Inline Block ────────────────────────── */}
        <div className="max-w-7xl mx-auto px-4 mt-10 mb-8">
          <WebsiteAdSlot placement="homepage_inline_block_1" variant="inline" lazy />
        </div>

        {/* ── Featured row ──────────────────────────────────── */}
        {featured.length > 0 && (
          <section className="max-w-7xl mx-auto px-4 py-6">
            <div className="flex items-center justify-between mb-4">
              <h2 className="font-display text-xl sm:text-2xl tracking-wide text-white flex items-center gap-2">
                <Clapperboard size={20} className="text-orange-500" aria-hidden="true" />
                {t("home.featuredMovies")}
              </h2>
              <Link
                href="/movies"
                className="flex items-center gap-1 text-xs text-orange-500 hover:text-orange-400 transition-colors"
              >
                {t("common.seeAll")} <ChevronRight size={14} aria-hidden="true" />
              </Link>
            </div>
            <MovieCarousel movies={featured} />
          </section>
        )}

        {/* Empty state */}
        {recent.length === 0 && (
          <section className="max-w-7xl mx-auto px-4 py-24 text-center">
            <p className="text-gray-400 text-lg">
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
