"use client";

import Link from "next/link";
import { Play, Clock, Star } from "lucide-react";
import { Movie } from "@/lib/api";
import { getIsNew, formatRelativeAddedTime } from "@/lib/movie-utils";
import { isMoviePremium, PremiumBadge } from "./PremiumComponents";
import { getLocalizedTitle, getLocalizedGenres } from "@/lib/localization";

interface Props {
  movie: Partial<Movie> & { id: string; title: string; poster_url: string; slug: string };
}

export default function MovieCard({ movie }: Props) {
  // Get localized metadata based on locale
  const localizedTitle = getLocalizedTitle(movie);
  const localizedGenres = getLocalizedGenres(movie);

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
      href={`/movies/${movie.slug}`}
      className={`movie-card group relative bg-brand-card rounded-xl overflow-hidden border transition-all duration-300 hover:scale-[1.02] hover:shadow-2xl block ${
        premium 
          ? "border-yellow-500/30 hover:border-yellow-500/50 hover:shadow-yellow-500/20" 
          : "border-brand-border hover:shadow-brand-red/10"
      }`}
      data-movie-id={movie.id}
    >
      {/* Poster */}
      <div className="relative aspect-[2/3] overflow-hidden bg-brand-border">
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img
          src={movie.poster_url}
          alt={localizedTitle}
          className="w-full h-full object-cover transition-transform duration-500 group-hover:scale-105"
          loading="lazy"
          onError={(e) => {
            (e.target as HTMLImageElement).src = "/placeholder-poster.jpg";
          }}
        />

        {/* Premium overlay */}
        {premium && (
          <div className="absolute inset-0 bg-gradient-to-t from-black/80 via-black/20 to-transparent opacity-60" />
        )}

        {/* Overlay on hover */}
        <div className="absolute inset-0 bg-black/60 opacity-0 group-hover:opacity-100 transition-opacity duration-300 flex items-center justify-center">
          <div className={`w-12 h-12 sm:w-14 sm:h-14 rounded-full flex items-center justify-center shadow-lg ${
            premium 
              ? "bg-gradient-to-br from-yellow-500 to-amber-600" 
              : "bg-brand-red"
          }`}>
            <Play size={20} className="text-white ml-1" fill="white" />
          </div>
        </div>

        {/* Badges container */}
        <div className="absolute top-2 left-2 right-2 flex justify-between items-start z-10">
          {/* Left badges */}
          <div className="flex flex-col gap-1">
            {isNew && (
              <span className="bg-blue-600/80 text-white text-xs px-1.5 py-0.5 rounded">
                New
              </span>
            )}
            {/* Movie code badge - below New badge */}
            {movie.code && (
              <span className="bg-black/60 text-white text-xs px-1.5 py-0.5 rounded">
                #{movie.code}
              </span>
            )}
          </div>

          {/* Right badges */}
          <div className="flex flex-col gap-1 items-end">
            {/* Premium badge */}
            {premium && (
              <PremiumBadge size="sm" showCrown />
            )}
            {/* Quality badge */}
            {movie.quality && movie.quality !== "" && (
              <span className={`text-xs font-bold px-2 py-0.5 rounded ${
                premium 
                  ? "bg-yellow-500/80 text-black" 
                  : "bg-brand-red text-white"
              }`}>
                {movie.quality}
              </span>
            )}
          </div>
        </div>
      </div>

      {/* Info */}
      <div className="p-2 sm:p-3">
        <h3 className={`font-semibold text-xs sm:text-sm leading-tight line-clamp-2 group-hover:transition-colors ${
          premium ? "group-hover:text-yellow-400" : "group-hover:text-brand-red"
        }`}>
          {localizedTitle}
        </h3>
        
        {/* Metadata row with rating and time */}
        <div className="flex items-center gap-2 mt-1.5 text-xs text-gray-500 flex-wrap">
          <span>{movie.year}</span>
          
          {(movie.duration || 0) > 0 && (
            <>
              <span>·</span>
              <Clock size={11} />
              <span>{movie.duration}m</span>
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
          <div className="text-xs text-gray-500 mt-1">
            {relativeTime}
          </div>
        )}

        {(localizedGenres?.length || 0) > 0 && (
          <div className="flex flex-wrap gap-1 mt-2">
            {(localizedGenres || []).slice(0, 2).map((g) => (
              <span
                key={g}
                className="text-xs bg-brand-border text-gray-400 px-1.5 sm:px-2 py-0.5 rounded-full"
              >
                {g}
              </span>
            ))}
          </div>
        )}
      </div>
    </Link>
  );
}
