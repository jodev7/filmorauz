import type { Metadata } from "next";
import dynamic from "next/dynamic";
import Link from "next/link";
import { Flame, Clapperboard, Sparkles, Film, Star, Layers } from "lucide-react";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import MovieCarousel from "@/components/MovieCarousel";
import SeriesCarousel from "@/components/SeriesCarousel";
import HeroCarousel from "@/components/home/HeroCarousel";
import QuickActionsBar from "@/components/home/QuickActionsBar";
import TrendingSpotlight from "@/components/home/TrendingSpotlight";
import SectionHeader from "@/components/home/SectionHeader";
import WeeklyTop10 from "@/components/home/WeeklyTop10";
import type { Movie } from "@/lib/api";
import { getHomepageData } from "@/lib/api";
import { getTranslations } from "@/lib/i18n-server";

const FeaturedCollectionsSection = dynamic(() => import("@/components/home/FeaturedCollectionsSection"));
const ContinueWatchingRow = dynamic(() => import("@/components/home/ContinueWatchingRow"));
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
  const topRated = homepage.top_rated || [];
  const recent = homepage.new_movies || [];
  const latestMovies = homepage.hero || [];
  const trending = homepage.trending || [];
  const featuredCollections = homepage.featured_collections || [];
  const seriesData = homepage.series || [];
  const genreChips = homepage.genres || [];
  const genreRows = homepage.genre_rows || [];

  // Weekly Top 10 — pool every movie the homepage already loaded, de-dupe by
  // id, and rank by view count. Uses existing payload (no extra request).
  const weeklyTop10 = (() => {
    const pool: Movie[] = [
      ...trending,
      ...recent,
      ...topRated,
      ...(homepage.premium_movies || []),
      ...genreRows.flatMap((r) => r.movies || []),
    ];
    const seen = new Set<string>();
    const unique: Movie[] = [];
    for (const m of pool) {
      if (!m?.id || seen.has(m.id)) continue;
      seen.add(m.id);
      unique.push(m);
    }
    return unique
      .sort((a, b) => (b.views || 0) - (a.views || 0))
      .slice(0, 10);
  })();

  return (
    <>
      <Navbar />
      <main className="min-h-screen">
        {/* ── Hero Carousel (Latest Movies) ─────────────────────────
            Inset rounded card that sits *below* the floating navbar island
            so the backdrop is fully visible (nothing overlaps its top). */}
        <div className="max-w-7xl mx-auto px-2 sm:px-4 pt-[calc(env(safe-area-inset-top)_+_92px)] sm:pt-[calc(env(safe-area-inset-top)_+_104px)]">
          <HeroCarousel movies={latestMovies} />
        </div>

        {/* ── Quick section entry points (under hero) ──────────────── */}
        <QuickActionsBar />

        {/* ── Genre Chips ──────────────────────────────────────────── */}
        <section className="max-w-7xl mx-auto px-4 mt-6 mb-4">
          <div className="flex gap-2 overflow-x-auto pb-2 scrollbar-hide">
            {genreChips.map((genre) => (
              <Link
                key={genre.slug}
                href={`/movies?genre=${genre.slug}`}
                className="glass-pill glass-hover shrink-0 rounded-full px-4 py-2 text-sm font-medium text-gray-300 hover:text-white"
              >
                {genre.label}
              </Link>
            ))}
          </div>
        </section>

        {/* ── Weekly Top 10 (medal podium; ranked by views) ─────────── */}
        <WeeklyTop10 movies={weeklyTop10} />

        {/* ── Continue Watching (logged-in users; self-hides when empty) ── */}
        <ContinueWatchingRow />

        {/* ── Homepage Top Banner Ad — lazy; shared prefetch serves it ─ */}
        <div className="max-w-7xl mx-auto px-4 mt-8 mb-6">
          <WebsiteAdSlot placement="homepage_top_banner" variant="banner" lazy />
        </div>

        {/* ── New Movies — only carousel with priority posters (above-fold) ── */}
        {recent.length > 0 && (
          <section className="max-w-7xl mx-auto px-4 py-6">
            <SectionHeader title="Yangi filmlar" icon={Clapperboard} href="/movies" />
            <MovieCarousel movies={recent} priorityCount={4} />
          </section>
        )}

        {/* ── Trending spotlight (#1) — landscape rhythm break ───── */}
        {trending.length > 0 && <TrendingSpotlight movie={trending[0]} />}

        {/* ── Trending (rest) ──────────────────────────────── */}
        {trending.length > 1 && (
          <section className="max-w-7xl mx-auto px-4 py-6">
            <SectionHeader title="Mashhur" icon={Flame} />
            <MovieCarousel movies={trending.slice(1)} />
          </section>
        )}

        {/* ── Premium ─────────────────────────────────────── */}
        {(homepage.premium_movies || []).length > 0 && (
          <section className="max-w-7xl mx-auto px-4 py-6">
            <SectionHeader
              title="Premium"
              icon={Sparkles}
              href="/movies?premium=true"
              accent="yellow"
            />
            <MovieCarousel movies={homepage.premium_movies || []} />
          </section>
        )}

        {/* ── Featured Collections ──────────────────────────────────── */}
        <FeaturedCollectionsSection collections={featuredCollections} />

        {/* ── New Series ──────────────────────────────────── */}
        {seriesData.length > 0 && (
          <section className="max-w-7xl mx-auto px-4 py-6">
            <SectionHeader title="Yangi seriallar" icon={Film} href="/series" />
            <SeriesCarousel series={seriesData.slice(0, 10)} />
          </section>
        )}

        {/* ── Homepage Inline Block ────────────────────────── */}
        <div className="max-w-7xl mx-auto px-4 mt-10 mb-8">
          <WebsiteAdSlot placement="homepage_inline_block_1" variant="inline" lazy />
        </div>

        {/* ── Top Rated ─────────────────────────────────────── */}
        {topRated.length > 0 && (
          <section className="max-w-7xl mx-auto px-4 py-6">
            <SectionHeader
              title="Eng yuqori baholangan"
              icon={Star}
              href="/movies"
              linkLabel={t("common.seeAll")}
              accent="yellow"
            />
            <MovieCarousel movies={topRated} />
          </section>
        )}

        {/* ── Genre discovery rows ──────────────────────────── */}
        {genreRows.map((row) => (
          <section key={row.slug} className="max-w-7xl mx-auto px-4 py-6">
            <SectionHeader
              title={row.label}
              icon={Layers}
              href={`/movies?genre=${row.slug}`}
              linkLabel={t("common.seeAll")}
            />
            <MovieCarousel movies={row.movies} />
          </section>
        ))}

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
