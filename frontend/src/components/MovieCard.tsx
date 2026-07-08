"use client";

import { memo } from "react";
import Link from "next/link";
import { Play, Clock, Star } from "lucide-react";
import { Movie } from "@/lib/api";
import { getIsNew, formatRelativeAddedTime, formatDuration } from "@/lib/movie-utils";
import { isMoviePremium, PremiumBadge } from "./PremiumComponents";
import { getLocalizedTitle, getLocalizedGenres } from "@/lib/localization";
import OptimizedImage from "./OptimizedImage";
import MediaTitle from "./MediaTitle";
import { normalizeMediaUrl } from "@/lib/image-utils";

interface Props {
  movie: Partial<Movie> & { id: string; title: string; poster_url: string; slug: string };
  priority?: boolean; // For above-the-fold cards
  showSkeleton?: boolean; // Allow disabling skeleton if needed
}

function areMovieCardPropsEqual(prev: Props, next: Props) {
  return (
    prev.priority === next.priority &&
    prev.showSkeleton === next.showSkeleton &&
    prev.movie.id === next.movie.id &&
    prev.movie.slug === next.movie.slug &&
    prev.movie.title === next.movie.title &&
    prev.movie.poster_url === next.movie.poster_url &&
    prev.movie.poster_thumb_url === next.movie.poster_thumb_url &&
    prev.movie.year === next.movie.year &&
    prev.movie.duration === next.movie.duration &&
    prev.movie.quality === next.movie.quality &&
    prev.movie.code === next.movie.code &&
    prev.movie.created_at === next.movie.created_at &&
    prev.movie.is_premium === next.movie.is_premium &&
    (prev.movie as any).rating_avg === (next.movie as any).rating_avg
  );
}

function MovieCardImpl({ movie, priority = false, showSkeleton = true }: Props) {
  // Get localized metadata based on locale
  const localizedTitle = getLocalizedTitle(movie);
  const localizedGenres = getLocalizedGenres(movie);
  const isEpisode = movie.type === "episode";
  const episodeLinkId = movie.episode_id || movie.target_id || movie.id;
  const href = isEpisode ? `/episode/${episodeLinkId}` : `/movies/${movie.slug}`;
  const cardTitle = isEpisode && movie.series_title ? movie.series_title : localizedTitle;
  const episodeLabel = movie.season_number && movie.episode_number
    ? `S${String(movie.season_number).padStart(2, "0")}E${String(movie.episode_number).padStart(2, "0")}`
    : movie.episode_number
    ? `${movie.episode_number}-qism`
    : "Epizod";
  const episodeSubtitle = isEpisode
    ? `${episodeLabel}${
        (movie.episode_title || localizedTitle) && movie.series_title && (movie.episode_title || localizedTitle) !== movie.series_title
          ? ` • ${movie.episode_title || localizedTitle}`
          : ""
      }`
    : "";

  // Use created_at from backend (ISO 8601 format)
  const createdAt = movie.created_at;
  const isNew = getIsNew(createdAt);
  const relativeTime = formatRelativeAddedTime(createdAt);

  // Rating display - use snake_case from backend
  const ratingValue = (movie as any).rating_avg ?? movie.rating_avg ?? 0;
  const hasRating = typeof ratingValue === 'number' && ratingValue > 0;

  // Check if this is a premium movie
  const premium = isMoviePremium(movie);

  return (
    <Link
      href={href}
      className={`movie-card group relative glass-card rounded-2xl overflow-hidden transition-all duration-300 hover:-translate-y-1 hover:shadow-xl block h-full flex flex-col ${
        premium
          ? "hover:border-yellow-500/40 hover:shadow-yellow-500/10"
          : "hover:border-orange-500/30 hover:shadow-orange-500/10"
      }`}
      data-movie-id={movie.id}
    >
      {/* Poster */}
      <div className="relative aspect-[2/3] overflow-hidden bg-[#1a1a24] shrink-0">
        <OptimizedImage
          src={movie.poster_thumb_url || movie.poster_url}
          alt={localizedTitle}
          className="transition-transform duration-500 group-hover:scale-105"
          priority={priority}
          showSkeleton={showSkeleton}
          aspectRatio="2/3"
        />

        {/* Premium overlay */}
        {premium && (
          <div className="absolute inset-0 bg-gradient-to-t from-black/80 via-black/20 to-transparent opacity-50" />
        )}

        {/* Overlay on hover */}
        <div className="absolute inset-0 bg-black/50 opacity-0 group-hover:opacity-100 transition-opacity duration-300 flex items-center justify-center">
          <div className={`w-12 h-12 sm:w-14 sm:h-14 rounded-full flex items-center justify-center shadow-lg ${
            premium 
              ? "bg-gradient-to-br from-yellow-500 to-amber-600" 
              : "bg-orange-500"
          }`}>
            <Play size={20} className="text-white ml-1" fill="white" />
          </div>
        </div>

        {/* Badges container */}
        <div className="absolute top-2.5 left-2.5 right-2.5 flex justify-between items-start z-10">
          {/* Left badges */}
          <div className="flex flex-col gap-1.5">
            {isNew && (
              <span className="bg-blue-600/90 text-white text-xs font-medium px-2 py-0.5 rounded-md">
                Yangi
              </span>
            )}
            {movie.code && (
              <span className="bg-black/70 text-white text-xs font-mono px-2 py-0.5 rounded-md">
                #{movie.code}
              </span>
            )}
          </div>

          {/* Right badges */}
          <div className="flex flex-col gap-1.5 items-end">
            {premium && (
              <PremiumBadge size="sm" showCrown />
            )}
            {movie.quality && movie.quality !== "" && (
              <span className={`text-xs font-bold px-2 py-0.5 rounded-md ${
                premium 
                  ? "bg-yellow-500/90 text-black" 
                  : "bg-orange-500 text-white"
              }`}>
                {movie.quality}
              </span>
            )}
          </div>
        </div>
      </div>

      {/* Info */}
      <div className="p-3 flex flex-col flex-1 min-h-[80px] min-w-0">
        <h3 className={`font-semibold text-xs sm:text-sm leading-tight line-clamp-2 break-words text-white group-hover:transition-colors ${
          premium ? "group-hover:text-yellow-400" : "group-hover:text-orange-500"
        }`}>
          <MediaTitle title={cardTitle} />
        </h3>
        {episodeSubtitle && (
          <p className="mt-1 text-[11px] leading-tight text-zinc-400 line-clamp-2">
            {episodeSubtitle}
          </p>
        )}
        
        {/* Metadata row with rating and time */}
        <div className="flex items-center gap-2 mt-auto pt-1.5 text-xs text-zinc-500 flex-wrap">
          <span>{movie.year}</span>
          
          {(movie.duration || 0) > 0 && (
            <>
              <span>·</span>
              <Clock size={11} />
              <span>{formatDuration(movie.duration)}</span>
            </>
          )}
          
          {/* Rating */}
          {hasRating && (
            <>
              <span>·</span>
              <span className="flex items-center gap-0.5 text-yellow-400">
                <Star size={11} className="fill-yellow-400" />
                {ratingValue.toFixed(1)}
              </span>
            </>
          )}
        </div>

        {/* Relative time (added) */}
        {relativeTime && (
          <div className="text-xs text-zinc-600 mt-1.5">
            {relativeTime}
          </div>
        )}
      </div>
    </Link>
  );
}

const MovieCard = memo(MovieCardImpl, areMovieCardPropsEqual);
MovieCard.displayName = "MovieCard";

export default MovieCard;
