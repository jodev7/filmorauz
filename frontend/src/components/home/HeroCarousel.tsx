"use client";

import { useState, useEffect, useCallback } from "react";
import Link from "next/link";
import { Play } from "lucide-react";
import { Movie } from "@/lib/api";
import { localizeSingleGenre } from "@/lib/localization";
import { formatDuration } from "@/lib/movie-utils";

interface HeroCarouselProps {
  movies: Movie[];
}

export default function HeroCarousel({ movies }: HeroCarouselProps) {
  const [currentIndex, setCurrentIndex] = useState(0);
  const [isHovered, setIsHovered] = useState(false);

  const goToNext = useCallback(() => {
    setCurrentIndex((prev) => (prev + 1) % movies.length);
  }, [movies.length]);

  useEffect(() => {
    if (movies.length <= 1 || isHovered) return;
    const interval = setInterval(goToNext, 5000);
    return () => clearInterval(interval);
  }, [movies.length, isHovered, goToNext]);

  if (movies.length === 0) return null;

  return (
    <div
      className="relative h-[60vh] min-h-[450px] max-h-[700px] overflow-hidden"
      onMouseEnter={() => setIsHovered(true)}
      onMouseLeave={() => setIsHovered(false)}
    >
      {/* Slides */}
      <div className="absolute inset-0">
        {movies.map((movie, index) => (
          <div
            key={movie.id}
            className={`absolute inset-0 transition-opacity duration-700 ease-in-out ${
              index === currentIndex ? "opacity-100 z-10" : "opacity-0 z-0"
            }`}
          >
            {/* Background Image with scale effect */}
            <div className="absolute inset-0 overflow-hidden">
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src={movie.backdrop_url || movie.poster_url}
                alt={movie.title}
                loading={index === 0 ? "eager" : "lazy"}
                fetchPriority={index === 0 ? "high" : "auto"}
                decoding="async"
                className={`w-full h-full object-cover transition-transform duration-700 ease-in-out ${
                  index === currentIndex ? "scale-105" : "scale-100"
                }`}
              />
              {/* Gradient Overlays */}
              <div className="absolute inset-0 bg-gradient-to-t from-brand-dark via-brand-dark/60 to-transparent" />
              <div className="absolute inset-0 bg-gradient-to-r from-brand-dark/90 via-brand-dark/30 to-transparent" />
              <div className="absolute inset-0 bg-black/30" />
            </div>

            {/* Content */}
            <div className="relative h-full max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 flex items-end pb-12 sm:pb-16">
              <div className="max-w-xl sm:max-w-2xl">
                {/* Genres */}
                <div className="flex gap-2 mb-3 flex-wrap">
                  {movie.genre?.slice(0, 3).map((g) => (
                    <span
                      key={g}
                      className="text-xs border border-brand-red/60 text-brand-red px-2.5 py-0.5 rounded-full"
                    >
                      {localizeSingleGenre(g)}
                    </span>
                  ))}
                </div>

                {/* Title */}
                <h1 className="font-display text-3xl sm:text-4xl md:text-5xl lg:text-6xl tracking-wide text-white leading-none mb-3">
                  {movie.title}
                </h1>

                {/* Description */}
                <p className="text-gray-300 text-sm sm:text-base leading-relaxed line-clamp-2 mb-5">
                  {movie.description}
                </p>

                {/* Buttons & Meta */}
                <div className="flex items-center gap-3 flex-wrap">
                  <Link
                    href={`/watch/${movie.slug}`}
                    className="flex items-center gap-2 bg-brand-red hover:bg-orange-700 text-white font-semibold px-5 py-2.5 rounded-lg transition-colors text-sm"
                  >
                    <Play size={18} fill="white" />
                    Ko&apos;rish
                  </Link>
                  <Link
                    href={`/movies/${movie.slug}`}
                    className="flex items-center gap-2 bg-white/10 hover:bg-white/20 text-white font-medium px-5 py-2.5 rounded-lg backdrop-blur-sm transition-colors border border-white/10 text-sm"
                  >
                    Batafsil
                  </Link>
                  <div className="flex items-center gap-3 text-sm text-gray-300 ml-2">
                    <span>{movie.year}</span>
                    {movie.quality && (
                      <span className="border border-gray-500 px-2 py-0.5 rounded text-xs font-medium">
                        {movie.quality}
                      </span>
                    )}
                    {movie.duration > 0 && <span>{formatDuration(movie.duration)}</span>}
                  </div>
                </div>
              </div>
            </div>
          </div>
        ))}
      </div>

      {/* Dots Pagination */}
      {movies.length > 1 && (
        <div className="absolute bottom-4 left-1/2 -translate-x-1/2 z-20 flex gap-2">
          {movies.map((_, index) => (
            <button
              key={index}
              onClick={() => setCurrentIndex(index)}
              className={`h-2 rounded-full transition-all duration-300 ${
                index === currentIndex
                  ? "bg-brand-red w-6"
                  : "bg-white/50 hover:bg-white/70 w-2"
              }`}
              aria-label={`Go to slide ${index + 1}`}
            />
          ))}
        </div>
      )}
    </div>
  );
}