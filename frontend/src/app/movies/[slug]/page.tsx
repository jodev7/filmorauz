import type { Metadata } from "next";
export const dynamic = "force-dynamic";
import dynamicImport from "next/dynamic";
import { notFound } from "next/navigation";
import Link from "next/link";
import { Clock, Calendar, Globe, ChevronLeft } from "lucide-react";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import MediaImage from "@/components/MediaImage";
import MoviePoster from "@/components/MoviePoster";
import MovieCode from "@/components/MovieCode";
import MovieCarousel from "@/components/MovieCarousel";
import WatchButton from "@/components/WatchButton";
import WatchTogetherButton from "@/components/WatchTogetherButton";
import MovieWatchSection from "@/components/MovieWatchSection";
import MediaTitle from "@/components/MediaTitle";
import { WatchPlayerProvider } from "@/lib/watch-player-context";
import { isMoviePremium, PremiumBadge } from "@/components/PremiumComponents";
import { getMovie, getRecommendations, getTopReviewsForSeo, reviewsToJsonLd, personPath } from "@/lib/api";
import JsonLd from "@/components/JsonLd";
import PersonChip from "@/components/PersonChip";
import { getTranslations } from "@/lib/i18n-server";
import { formatDuration } from "@/lib/movie-utils";
import { normalizeMediaUrl } from "@/lib/image-utils";
import { buildMovieUrl } from "@/lib/content-routes";
import { buildContentDescription, buildContentKeywords, buildContentTitle, pickSeoImage } from "@/lib/seo";
import {
	getLocalizedTitle,
	getLocalizedDescription,
	getLocalizedGenres,
	getLocalizedCountry,
	localizeSingleGenre,
} from "@/lib/localization";

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL || "https://filmorauz.net";
const MovieActions = dynamicImport(() => import("@/components/MovieActions"));
const LibraryButtons = dynamicImport(() => import("@/components/LibraryButtons"));
const StarRating = dynamicImport(() => import("@/components/StarRating"));
const Comments = dynamicImport(() => import("@/components/Comments"));
const Reviews = dynamicImport(() => import("@/components/Reviews"));
const ShareButton = dynamicImport(() => import("@/components/ShareButton"));
const WebsiteAdSlot = dynamicImport(() => import("@/components/ads/WebsiteAdSlot"));

interface Props {
  params: { slug: string };
  searchParams?: { play?: string };
}

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { slug } = params;
  const canonicalUrl = buildMovieUrl(slug);
  
  try {
    const movie = await getMovie(slug);
    const imageUrl = pickSeoImage(movie.backdrop_url, movie.poster_url);
    
    const localizedTitle = getLocalizedTitle(movie);
    const seoTitle = buildContentTitle(localizedTitle);
    const description = buildContentDescription(localizedTitle, movie.year);

    return {
      title: seoTitle,
      description,
      keywords: buildContentKeywords({
        title: localizedTitle,
        originalTitle: movie.original_title,
        uzbekTitle: movie.title_uz || localizedTitle,
        genres: movie.genre,
        extra: [movie.year?.toString() || "", "kino"],
      }),
      authors: [{ name: "FILMORAUZ" }],
      publisher: "FILMORAUZ",
      openGraph: {
        title: seoTitle,
        description,
        url: canonicalUrl,
        siteName: "FILMORAUZ",
        type: "video.movie",
        locale: "uz_UZ",
        images: [
          {
            url: imageUrl,
            width: 1200,
            height: 630,
            alt: localizedTitle,
          },
        ],
      },
      twitter: {
        card: "summary_large_image",
        title: seoTitle,
        description: description,
        images: [imageUrl],
      },
      alternates: {
        canonical: canonicalUrl,
      },
      robots: {
        index: true,
        follow: true,
      },
    };
  } catch {
    return {
      title: "Kino topilmadi — FILMORAUZ",
      robots: { index: false, follow: false },
    };
  }
}

export default async function MovieDetailPage({ params, searchParams }: Props) {
  const { slug } = params;
  const autoOpenPlayer = searchParams?.play === "1";
  const { t } = getTranslations("uz");

  let movie;
  try {
    movie = await getMovie(slug);
  } catch {
    notFound();
  }

  if (!movie) {
    notFound();
  }

  let recommendations: any[] = [];

  // Fetch recommendations
  try {
    recommendations = await getRecommendations(movie.id, 12);
  } catch {
    // Silently handle - recommendations are optional
  }

  // Get localized metadata based on locale
  const localizedTitle = getLocalizedTitle(movie);
  const localizedDescription = getLocalizedDescription(movie);
  const localizedGenres = getLocalizedGenres(movie);
  const localizedCountry = getLocalizedCountry(movie);
  const movieGenres = Array.isArray(movie.genre) ? movie.genre : [];
  const movieUrl = buildMovieUrl(slug);

  // JSON-LD structured data for SEO - Breadcrumbs
  const breadcrumbJsonLd = {
    "@context": "https://schema.org",
    "@type": "BreadcrumbList",
    itemListElement: [
      {
        "@type": "ListItem",
        position: 1,
        name: "Bosh sahifa",
        item: `${SITE_URL}`,
      },
      {
        "@type": "ListItem",
        position: 2,
        name: "Kinolar",
        item: `${SITE_URL}/movies`,
      },
      {
        "@type": "ListItem",
        position: 3,
        name: localizedTitle,
        item: movieUrl,
      },
    ],
  };

  // JSON-LD structured data for SEO - Movie
  const movieJsonLd: Record<string, any> = {
    "@context": "https://schema.org",
    "@type": "Movie",
    name: localizedTitle,
    alternateName: movie.original_title || undefined,
    description: localizedDescription || buildContentDescription(localizedTitle, movie.year),
    image: [pickSeoImage(movie.poster_url, movie.backdrop_url)],
    datePublished: movie.year?.toString(),
    genre: localizedGenres,
    countryOfOrigin: localizedCountry,
    url: movieUrl,
    duration: movie.duration ? `PT${movie.duration}M` : undefined,
    quality: movie.quality,
    potentialAction: {
      "@type": "WatchAction",
      target: `${movieUrl}?play=1`,
    },
  };
  // Top written reviews (if any) — eligible for review snippets.
  const topReviews = await getTopReviewsForSeo("movie", movie.id);
  if (topReviews.length > 0) {
    movieJsonLd.review = reviewsToJsonLd(topReviews);
  }
  if (movie.cast && movie.cast.length > 0) {
    movieJsonLd.actor = movie.cast.slice(0, 10).map((name) => ({ "@type": "Person", name, url: `${SITE_URL}${personPath(name)}` }));
  }
  if (movie.director) {
    movieJsonLd.director = { "@type": "Person", name: movie.director };
  }
  if (movie.rating_count && movie.rating_count > 0 && movie.rating_avg) {
    movieJsonLd.aggregateRating = {
      "@type": "AggregateRating",
      ratingValue: movie.rating_avg,
      ratingCount: movie.rating_count,
      bestRating: 5,
      worstRating: 1,
    };
  }

  // VideoObject — required for Google Video Search results. Carries the
  // poster, page URL as embedUrl, and the watch URL as contentUrl.
  const videoJsonLd: Record<string, any> = {
    "@context": "https://schema.org",
    "@type": "VideoObject",
    name: localizedTitle,
    description: localizedDescription || buildContentDescription(localizedTitle, movie.year),
    thumbnailUrl: [pickSeoImage(movie.poster_url, movie.backdrop_url)],
    uploadDate: movie.created_at || (movie.year ? `${movie.year}-01-01` : undefined),
    // contentUrl must point at the actual media file (HLS/CDN), not the HTML
    // landing page — otherwise Google reports "Video isn't on a watch page".
    contentUrl: movie.master_playlist_url || movie.video_url || undefined,
    embedUrl: `${movieUrl}?play=1`,
    duration: movie.duration ? `PT${movie.duration}M` : undefined,
    inLanguage: "uz",
    isFamilyFriendly: true,
    publisher: { "@type": "Organization", name: "FILMORAUZ", logo: { "@type": "ImageObject", url: `${SITE_URL}/favicon.png` } },
  };

  return (
    <>
      {/* Breadcrumbs JSON-LD */}
      <JsonLd data={breadcrumbJsonLd} />
      {/* Movie JSON-LD */}
      <JsonLd data={movieJsonLd} />
      {/* VideoObject JSON-LD — drives Google Video Search */}
      <JsonLd data={videoJsonLd} />
      <Navbar />
      <WatchPlayerProvider initialOpen={autoOpenPlayer}>
      <main className="min-h-screen">
        {/* Backdrop hero */}
        <div className="relative h-[36vh] sm:h-[55vh] min-h-[240px] sm:min-h-[380px]">
          <MediaImage
            src={movie.backdrop_url || movie.poster_url}
            alt={movie.title}
            loading="eager"
            fetchPriority="high"
            className="absolute inset-0 h-full w-full object-cover"
          />
          <div className="absolute inset-0 bg-gradient-to-t from-brand-dark via-brand-dark/50 to-black/30" />
        </div>

        {/* Main content */}
        <div className="max-w-7xl mx-auto px-4 -mt-28 sm:-mt-36 relative">
          {/* Header: poster beside title on every screen size */}
          <div className="flex gap-4 sm:gap-6 md:gap-8">
            <div className="shrink-0">
              <MoviePoster
                src={movie.poster_url}
                alt={localizedTitle}
                className="w-28 sm:w-40 md:w-48 lg:w-56 rounded-xl shadow-2xl border border-white/10"
              />
            </div>

            <div className="min-w-0 flex-1 pt-10 sm:pt-16 md:pt-24">
              <Link
                href="/movies"
                className="hidden sm:inline-flex items-center gap-1 text-sm text-gray-400 hover:text-white mb-3 transition-colors"
              >
                <ChevronLeft size={16} />
                {t("common.backToMovies")}
              </Link>

              <h1 className="font-display text-2xl sm:text-4xl md:text-5xl lg:text-6xl text-white tracking-wide leading-none mb-3 flex flex-wrap items-center gap-2 sm:gap-3">
                <MediaTitle title={localizedTitle} />
                {isMoviePremium(movie) && (
                  <PremiumBadge size="default" showCrown />
                )}
              </h1>

              <div className="flex flex-wrap items-center gap-3 sm:gap-4 text-xs sm:text-sm text-gray-400 mb-3 sm:mb-4">
                <span className="flex items-center gap-1">
                  <Calendar size={14} />
                  {movie.year}
                </span>
                {movie.duration > 0 && (
                  <span className="flex items-center gap-1">
                    <Clock size={14} />
                    {formatDuration(movie.duration)}
                  </span>
                )}
                {localizedCountry && (
                  <span className="flex items-center gap-1">
                    <Globe size={14} />
                    {localizedCountry}
                  </span>
                )}
                {movie.quality && (
                  <span className="border border-brand-red text-brand-red text-xs font-bold px-2 py-0.5 rounded">
                    {movie.quality}
                  </span>
                )}
              </div>

              {movieGenres.length > 0 && (
                <div className="flex flex-wrap gap-1.5 sm:gap-2 mb-3">
                  {movieGenres.map((g) => (
                    <Link
                      key={g}
                      href={`/movies?genre=${encodeURIComponent(g.toLowerCase())}`}
                      className="text-xs sm:text-sm glass-card border border-white/10 text-gray-300 px-3 py-1 rounded-full hover:border-brand-red hover:text-brand-red transition-colors"
                    >
                      {localizeSingleGenre(g)}
                    </Link>
                  ))}
                </div>
              )}


              <div className="hidden sm:block">
                <MovieCode code={movie.code} />
              </div>
            </div>
          </div>

          {/* Body: full width on phones, aligned with the title column on desktop */}
          <div className="mt-5 md:ml-[14rem] lg:ml-[16rem]">
            <div className="sm:hidden mb-4">
              <MovieCode code={movie.code} />
            </div>

            {/* Primary actions */}
            <div className="flex flex-wrap items-center gap-3 [&>*:first-child]:w-full sm:[&>*:first-child]:w-auto">
              <WatchButton
                movieSlug={movie.slug}
                movieTitle={localizedTitle}
                isPremium={isMoviePremium(movie)}
              />
              <LibraryButtons targetType="movie" targetId={movie.id} title={localizedTitle} />
            </div>

            {/* Secondary actions */}
            <div className="mt-3 flex flex-wrap items-center gap-2 sm:gap-3">
              <WatchTogetherButton
                contentType="movie"
                contentID={movie.id}
                className="inline-flex items-center gap-1.5 px-4 py-2.5 glass-card border border-white/10 hover:border-brand-red rounded-xl text-sm text-white transition-colors"
              />
              <ShareButton
                movieId={movie.id}
                movieTitle={localizedTitle}
                movieSlug={movie.slug}
              />
              <MovieActions movie={movie} />
            </div>

            <p className="mt-6 text-gray-300 leading-relaxed max-w-3xl text-sm sm:text-base">
              {localizedDescription}
            </p>

            <div className="mt-4">
              <StarRating movieId={movie.id} />
            </div>
          </div>

          {/* Cast & crew */}
          {(movie.director || (movie.cast && movie.cast.length > 0)) && (
            <section className="mt-8" aria-labelledby="cast-title">
              <h2 id="cast-title" className="font-display text-xl sm:text-2xl tracking-wide text-white mb-4">
                ROLLARDA VA IJODKORLAR
              </h2>
              <ul className="scrollbar-hide -mx-4 flex gap-3 overflow-x-auto px-4 pb-2">
                {movie.director && (
                  <li className="shrink-0">
                    <PersonChip name={movie.director} role="Rejissyor" />
                  </li>
                )}
                {(movie.cast || []).slice(0, 15).map((name) => (
                  <li key={name} className="shrink-0">
                    <PersonChip name={name} role="Aktyor" />
                  </li>
                ))}
              </ul>
            </section>
          )}
        </div>

        {/* Inline player — opens here when "Tomosha qilish" is clicked */}
        <div className="mt-6">
          <MovieWatchSection movie={movie} />
        </div>

        <div className="max-w-7xl mx-auto px-4 mt-8 mb-6">
          <WebsiteAdSlot placement="movie_detail_banner" variant="banner" />
        </div>

        {/* Similar titles */}
        {recommendations.length > 0 && (
          <section className="max-w-7xl mx-auto px-4 pb-8">
            <h2 className="font-display text-2xl sm:text-3xl tracking-wide text-white mb-6">
              O&apos;XSHASH KINOLAR
            </h2>
            <MovieCarousel movies={recommendations} />
          </section>
        )}

        {/* Reviews + Comments */}
        <section className="max-w-7xl mx-auto px-4 pb-12">
          <Reviews targetType="movie" targetId={movie.id} />
          <Comments movieId={movie.id} />
        </section>
      </main>
      </WatchPlayerProvider>
      <Footer />
    </>
  );
}
