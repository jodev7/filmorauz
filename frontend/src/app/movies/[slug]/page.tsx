import type { Metadata } from "next";
export const dynamic = "force-dynamic";
import dynamicImport from "next/dynamic";
import { notFound } from "next/navigation";
import Script from "next/script";
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
import { getMovie, getRecommendations } from "@/lib/api";
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
const StarRating = dynamicImport(() => import("@/components/StarRating"));
const Comments = dynamicImport(() => import("@/components/Comments"));
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
      <Script
        id="breadcrumb-schema"
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: JSON.stringify(breadcrumbJsonLd) }}
      />
      {/* Movie JSON-LD */}
      <Script
        id="movie-schema"
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: JSON.stringify(movieJsonLd) }}
      />
      {/* VideoObject JSON-LD — drives Google Video Search */}
      <Script
        id="video-schema"
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: JSON.stringify(videoJsonLd) }}
      />
      <Navbar />
      <WatchPlayerProvider initialOpen={autoOpenPlayer}>
      <main className="min-h-screen">
        {/* Backdrop hero */}
        <div className="relative h-[50vh] sm:h-[55vh] min-h-[300px] sm:min-h-[380px]">
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
        <div className="max-w-7xl mx-auto px-4 -mt-24 sm:-mt-32 relative">
          <div className="flex flex-col md:flex-row gap-6 sm:gap-8">
            {/* Poster */}
            <div className="shrink-0">
              <MoviePoster
                src={movie.poster_url}
                alt={localizedTitle}
                className="w-36 sm:w-44 md:w-48 lg:w-56 rounded-xl shadow-2xl border border-white/10"
              />
            </div>

            {/* Details */}
            <div className="flex-1 pt-2 md:pt-6 sm:pt-8">
              <Link
                href="/movies"
                className="inline-flex items-center gap-1 text-sm text-gray-500 hover:text-white mb-3 sm:mb-4 transition-colors"
              >
                <ChevronLeft size={16} />
                {t("common.backToMovies")}
              </Link>

              {/* Movie code with copy button */}
              <div className="mb-3">
                <MovieCode code={movie.code} />
              </div>

              <h1 className="font-display text-3xl sm:text-4xl md:text-5xl lg:text-6xl text-white tracking-wide leading-none mb-3 flex flex-wrap items-center gap-3">
                <MediaTitle title={localizedTitle} />
                {isMoviePremium(movie) && (
                  <PremiumBadge size="default" showCrown />
                )}
              </h1>

              <div className="flex flex-wrap items-center gap-3 sm:gap-4 text-sm text-gray-400 mb-4">
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
                <div className="flex flex-wrap gap-2 mb-5">
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

              <p className="text-gray-300 leading-relaxed max-w-2xl mb-6 sm:mb-8 text-sm sm:text-base">
                {localizedDescription}
              </p>

              <div className="flex flex-wrap items-center gap-4">
                <WatchButton
                  movieSlug={movie.slug}
                  movieTitle={localizedTitle}
                  isPremium={isMoviePremium(movie)}
                />

                <WatchTogetherButton
                  contentType="movie"
                  contentID={movie.id}
                  className="inline-flex items-center gap-1.5 px-4 py-3 glass-card border border-white/10 hover:border-brand-red rounded-xl text-sm text-white transition-colors"
                />

                <ShareButton
                  movieId={movie.id}
                  movieTitle={localizedTitle}
                  movieSlug={movie.slug}
                />
                
                <div className="flex items-center gap-4">
                  <MovieActions movie={movie} />
                </div>
                
                {/* Rating */}
                <div className="flex items-center gap-2 mt-4">
                  <StarRating movieId={movie.id} />
                </div>
              </div>
            </div>
          </div>
        </div>

        {/* Inline player — opens here when "Tomosha qilish" is clicked */}
        <div className="mt-6">
          <MovieWatchSection movie={movie} />
        </div>

        <div className="max-w-7xl mx-auto px-4 mt-8 mb-6">
          <WebsiteAdSlot placement="movie_detail_banner" variant="banner" />
        </div>

        {/* Recommendations Section */}
        {recommendations.length > 0 && (
          <section className="max-w-7xl mx-auto px-4 pb-8">
            <h2 className="font-display text-2xl sm:text-3xl tracking-wide text-white mb-6">
              {t("movie.recommendations")}
            </h2>
            <MovieCarousel movies={recommendations} />
          </section>
        )}

        {/* Comments Section */}
        <section className="max-w-7xl mx-auto px-4 pb-12">
          <Comments movieId={movie.id} />
        </section>
      </main>
      </WatchPlayerProvider>
      <Footer />
    </>
  );
}
