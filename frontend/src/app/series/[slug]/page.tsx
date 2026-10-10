import type { Metadata } from "next";
export const revalidate = 300;
import dynamicImport from "next/dynamic";
import { notFound } from "next/navigation";
import Link from "next/link";
import { ChevronLeft, Calendar, Globe, Play } from "lucide-react";
import MediaTitle from "@/components/MediaTitle";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import MediaImage from "@/components/MediaImage";
import MovieCode from "@/components/MovieCode";
import SeasonList from "@/components/SeasonList";
import CastRow from "@/components/CastRow";
import WatchTogetherButton from "@/components/WatchTogetherButton";
import { getSeriesBySlug, getSeriesRecommendations } from "@/lib/series-api";
import { localizeSingleGenre } from "@/lib/localization";
import { DEFAULT_POSTER_PLACEHOLDER, normalizeMediaUrl } from "@/lib/image-utils";
import { buildSeriesUrl } from "@/lib/content-routes";
import { buildContentDescription, buildContentKeywords, buildContentTitle, pickSeoImage } from "@/lib/seo";
import JsonLd from "@/components/JsonLd";
import { getTopReviewsForSeo, reviewsToJsonLd } from "@/lib/api";

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL || "https://filmorauz.net";
const WebsiteAdSlot = dynamicImport(() => import("@/components/ads/WebsiteAdSlot"));
const StarRating = dynamicImport(() => import("@/components/StarRating"));
const ShareSheet = dynamicImport(() => import("@/components/share/ShareSheet"));
const ContentDiscovery = dynamicImport(() => import("@/components/discovery/ContentDiscovery"));
const LibraryButtons = dynamicImport(() => import("@/components/LibraryButtons"));
const Comments = dynamicImport(() => import("@/components/Comments"));
const SeriesResumeButton = dynamicImport(() => import("@/components/SeriesResumeButton"));

interface Props {
  params: { slug: string };
}

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { slug } = params;
  const canonicalUrl = buildSeriesUrl(slug);

  try {
    const data = await getSeriesBySlug(slug);
    const s = data.series;
    const title = buildContentTitle(s.title);
    const description = buildContentDescription(s.title, s.year);
    const imageUrl = pickSeoImage(s.backdrop_url, s.poster_url);

    return {
      title,
      description,
      keywords: buildContentKeywords({
        title: s.title,
        uzbekTitle: s.title,
        genres: s.genre,
        extra: [s.year?.toString() || "", "serial"],
      }),
      openGraph: {
        title,
        description,
        url: canonicalUrl,
        siteName: "FILMORAUZ",
        type: "video.tv_show",
        locale: "uz_UZ",
        images: [{ url: imageUrl, width: 1200, height: 630, alt: s.title }],
      },
      twitter: {
        card: "summary_large_image",
        title,
        description,
        images: [imageUrl],
      },
      alternates: { canonical: canonicalUrl, types: { "text/markdown": `${SITE_URL}/llms/series/${encodeURIComponent(slug)}.md` } },
      robots: { index: true, follow: true },
    };
  } catch {
    return {
      title: "Serial topilmadi — FILMORAUZ",
      robots: { index: false, follow: false },
    };
  }
}

export default async function SeriesDetailPage({ params }: Props) {
  const { slug } = params;
  let seriesData;
  let relatedSeries: any[] = [];
  
  try {
    seriesData = await getSeriesBySlug(slug);
  } catch {
    notFound();
  }

  // Fetch content-similar series (by genre / country / year), seeded from this
  // series — the "Sizga yoqishi mumkin" row.
  try {
    relatedSeries = await getSeriesRecommendations(seriesData.series.id, 12);
  } catch {
    // Silently handle — the row just won't render.
  }

  const { series, seasons } = seriesData;
  const canonicalUrl = buildSeriesUrl(slug);

  // First episode (lowest season, lowest episode) for the "start watching" CTA.
  const firstEpisode = (() => {
    const sorted = [...(seasons || [])].sort((a, b) => a.season.season_number - b.season.season_number);
    for (const s of sorted) {
      const eps = [...(s.episodes || [])].sort((a, b) => a.episode_number - b.episode_number);
      if (eps[0]) return { id: eps[0].id, seasonNumber: s.season.season_number, episodeNumber: eps[0].episode_number };
    }
    return null;
  })();

  const seriesJsonLd: Record<string, any> = {
    "@context": "https://schema.org",
    "@type": "TVSeries",
    name: series.title,
    description: series.description || buildContentDescription(series.title, series.year),
    image: [pickSeoImage(series.poster_url, series.backdrop_url)],
    genre: series.genre || [],
    url: canonicalUrl,
    datePublished: series.year?.toString(),
    countryOfOrigin: series.country,
    numberOfSeasons: seasons?.length || undefined,
  };
  if (series.rating_count && series.rating_count > 0 && series.rating_avg) {
    seriesJsonLd.aggregateRating = {
      "@type": "AggregateRating",
      ratingValue: series.rating_avg,
      ratingCount: series.rating_count,
      bestRating: 5,
      worstRating: 1,
    };
  }

  // Top written reviews (if any) — eligible for review snippets.
  const topReviews = await getTopReviewsForSeo("series", series.id);
  if (topReviews.length > 0) {
    seriesJsonLd.review = reviewsToJsonLd(topReviews);
  }

  const breadcrumbJsonLd = {
    "@context": "https://schema.org",
    "@type": "BreadcrumbList",
    itemListElement: [
      { "@type": "ListItem", position: 1, name: "Bosh sahifa", item: SITE_URL },
      { "@type": "ListItem", position: 2, name: "Seriallar", item: `${SITE_URL}/series` },
      { "@type": "ListItem", position: 3, name: series.title, item: canonicalUrl },
    ],
  };

  return (
    <>
      <JsonLd data={seriesJsonLd} />
      <JsonLd data={breadcrumbJsonLd} />
      <Navbar />
      <main className="min-h-screen">
        {/* Backdrop hero */}
        <div className="relative h-[40vh] sm:h-[45vh] min-h-[250px]">
          {series.backdrop_url && (
            <>
              <MediaImage
                src={series.backdrop_url}
                alt={series.title}
                loading="eager"
                fetchPriority="high"
                className="absolute inset-0 h-full w-full object-cover"
              />
              <div className="absolute inset-0 bg-gradient-to-t from-brand-dark via-brand-dark/50 to-black/30" />
            </>
          )}
        </div>

        {/* Main content */}
        <div className="max-w-7xl mx-auto px-4 -mt-20 relative">
          <div className="flex flex-col md:flex-row gap-6 sm:gap-8">
            {/* Poster */}
            <div className="shrink-0">
              {series.poster_url && (
                <div className="relative w-36 sm:w-44 md:w-48 lg:w-56 aspect-[2/3] rounded-xl shadow-2xl border border-white/10 overflow-hidden">
                  <MediaImage
                    src={series.poster_url}
                    alt={series.title}
                    fallbackSrc={DEFAULT_POSTER_PLACEHOLDER}
                    className="absolute inset-0 h-full w-full object-cover"
                  />
                </div>
              )}
            </div>

            {/* Details */}
            <div className="flex-1 pt-2 md:pt-4">
              <Link
                href="/series"
                className="inline-flex items-center gap-1 text-sm text-gray-500 hover:text-white mb-3 sm:mb-4 transition-colors"
              >
                <ChevronLeft size={16} />
                Seriallarga qaytish
              </Link>

              {series.code && (
                <div className="mb-3">
                  <MovieCode code={series.code} label="🎟 Serial kodi:" />
                </div>
              )}

              <h1 className="font-display text-3xl sm:text-4xl md:text-5xl text-white tracking-wide leading-none mb-3 flex flex-wrap items-center gap-3">
                <MediaTitle title={series.title} />
                {series.is_premium && (
                  <span className="inline-flex items-center gap-1 text-xs bg-gradient-to-r from-yellow-500 to-amber-600 text-black px-2 py-1 rounded-full font-bold shadow-[0_0_10px_rgba(234,179,8,0.3)]">
                    PREMIUM
                  </span>
                )}
              </h1>

              <div className="flex flex-wrap items-center gap-3 sm:gap-4 text-sm text-gray-400 mb-4">
                <span className="flex items-center gap-1">
                  <Calendar size={14} />
                  {series.year}
                </span>
                {series.country && (
                  <span className="flex items-center gap-1">
                    <Globe size={14} />
                    {series.country}
                  </span>
                )}
                {series.quality && (
                  <span className="border border-brand-red text-brand-red text-xs font-bold px-2 py-0.5 rounded">
                    {series.quality}
                  </span>
                )}
              </div>

              {series.genre && series.genre.length > 0 && (
                <div className="flex flex-wrap gap-2 mb-5">
                  {series.genre.map((g: string) => (
                    <Link
                      key={g}
                      href={`/series?genre=${encodeURIComponent(g.toLowerCase())}`}
                      className="text-xs sm:text-sm glass-card border border-white/10 text-gray-300 px-3 py-1 rounded-full hover:border-brand-red hover:text-brand-red transition-colors"
                    >
                      {localizeSingleGenre(g)}
                    </Link>
                  ))}
                </div>
              )}

              <p className="text-gray-300 leading-relaxed max-w-2xl mb-6 text-sm sm:text-base">
                {series.description}
              </p>

              {firstEpisode && (
                <div className="mb-4">
                  <SeriesResumeButton seriesId={series.id} seriesSlug={series.slug} firstEpisode={firstEpisode} />
                </div>
              )}

              <div className="mb-4 flex flex-wrap items-start gap-2">
                <WatchTogetherButton contentType="series" contentID={series.id} />
                <ShareSheet
                  kind="series"
                  id={series.id}
                  title={series.title}
                  path={`/series/${series.slug}`}
                  posterUrl={series.poster_url}
                  subtitle={seasons?.length ? `${seasons.length} fasl` : undefined}
                />
              </div>

              <div className="mb-4">
                <LibraryButtons targetType="series" targetId={series.id} title={series.title} />
              </div>

              <div className="mb-6">
                <StarRating seriesId={series.id} readOnly />
              </div>
            </div>
          </div>

          <div className="mb-6">
            <WebsiteAdSlot placement="series_detail_page_banner" variant="banner" bannerPlace="movie" />
          </div>

          <CastRow
            director={series.director}
            directorPhoto={series.director_profile_url}
            directorRole="Yaratuvchi"
            cast={series.cast}
            castDetails={series.cast_details}
          />

          {/* Seasons */}
          {seasons && seasons.length > 0 && (
            <section className="mt-8 pb-12">
              <h2 className="font-display text-2xl sm:text-3xl tracking-wide text-white mb-6">
                FASLLAR
              </h2>
              <SeasonList
                seasons={seasons}
                seriesBackdropUrl={series.backdrop_url}
                seriesPosterUrl={series.poster_url}
                seriesSlug={series.slug}
                seriesId={series.id}
              />
            </section>
          )}

          {/* Comments (star-rated reviews) */}
          <Comments commentsEnabled={false} reviewTarget={{ type: "series", id: series.id }} />

          {/* Similar series + recommended / random movies */}
          <section className="pb-12">
            <ContentDiscovery similarSeries={relatedSeries} similarTitle="O'xshash seriallar" />
          </section>
        </div>
      </main>
      <Footer />
    </>
  );
}
