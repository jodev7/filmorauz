"use client";

import { useState, useEffect, useCallback, useMemo } from "react";
import Link from "next/link";
import { Play } from "lucide-react";
import { Movie } from "@/lib/api";
import MediaImage from "@/components/MediaImage";
import { formatDuration } from "@/lib/movie-utils";
import { normalizeMediaUrl } from "@/lib/image-utils";

interface HeroCarouselProps {
  movies: Movie[];
}

// Auto-advance cadence for the hero slider (ms).
const HERO_AUTOPLAY_MS = 6000;

export default function HeroCarousel({ movies }: HeroCarouselProps) {
  const [currentIndex, setCurrentIndex] = useState(0);
  const [loadedIndexes, setLoadedIndexes] = useState<Set<number>>(() => new Set([0]));
  const [isPaused, setIsPaused] = useState(false);

  const markLoaded = useCallback((idx: number) => {
    setLoadedIndexes((prev) => {
      if (prev.has(idx)) return prev;
      const next = new Set(prev);
      next.add(idx);
      return next;
    });
  }, []);

  const goTo = useCallback((idx: number) => {
    setCurrentIndex(idx);
    markLoaded(idx);
  }, [markLoaded]);

  // After first paint, warm the next slide image so the transition feels instant.
  useEffect(() => {
    if (movies.length <= 1) return;
    const warm = () => markLoaded(1 % movies.length);
    const ric = (window as unknown as {
      requestIdleCallback?: (cb: IdleRequestCallback, opts?: IdleRequestOptions) => number;
      cancelIdleCallback?: (handle: number) => void;
    });
    if (ric.requestIdleCallback) {
      const id = ric.requestIdleCallback(warm, { timeout: 2000 });
      return () => ric.cancelIdleCallback?.(id);
    }
    const id = window.setTimeout(warm, 1500);
    return () => window.clearTimeout(id);
  }, [movies.length, markLoaded]);

  // Auto-advance to the next slide on a timer. The effect re-runs whenever
  // currentIndex changes, so manual navigation naturally resets the countdown.
  // Pauses on hover/focus and respects the user's reduced-motion preference.
  useEffect(() => {
    if (movies.length <= 1 || isPaused) return;
    if (
      typeof window !== "undefined" &&
      window.matchMedia?.("(prefers-reduced-motion: reduce)").matches
    ) {
      return;
    }
    const next = (currentIndex + 1) % movies.length;
    markLoaded(next); // warm the upcoming image before the transition
    const id = window.setTimeout(() => goTo(next), HERO_AUTOPLAY_MS);
    return () => window.clearTimeout(id);
  }, [currentIndex, movies.length, isPaused, goTo, markLoaded]);

  const heroTitle = useMemo(() => movies[0]?.title || "", [movies]);

  if (movies.length === 0) return null;

  return (
    <div
      className="relative h-[58vh] min-h-[420px] max-h-[640px] overflow-hidden rounded-3xl border border-white/10 shadow-[0_24px_70px_rgba(0,0,0,0.5)]"
      aria-roledescription="carousel"
      aria-label={heroTitle ? `Hero: ${heroTitle}` : "Hero"}
      onMouseEnter={() => setIsPaused(true)}
      onMouseLeave={() => setIsPaused(false)}
      onFocusCapture={() => setIsPaused(true)}
      onBlurCapture={() => setIsPaused(false)}
    >
      {/* Slides */}
      <div className="absolute inset-0">
        {movies.map((movie, index) => {
          const isActive = index === currentIndex;
          const shouldRender = loadedIndexes.has(index);
          const backdropSrc = movie.backdrop_url || movie.poster_url;
          return (
            <div
              key={movie.id}
              className={`absolute inset-0 transition-opacity duration-700 ease-in-out ${
                isActive ? "opacity-100 z-10" : "opacity-0 z-0"
              }`}
              aria-hidden={!isActive}
            >
              {/* Background Image with scale effect */}
              <div className="absolute inset-0 overflow-hidden">
                {shouldRender && (
                  <MediaImage
                    src={backdropSrc}
                    alt={movie.title}
                    loading={index === 0 ? "eager" : "lazy"}
                    fetchPriority={index === 0 ? "high" : "auto"}
                    className={`w-full h-full object-cover transition-transform duration-700 ease-in-out ${
                      isActive ? "scale-105" : "scale-100"
                    }`}
                  />
                )}
                {/* Gradient Overlays — deeper, cleaner falloff for a calm,
                    Apple-like base that keeps text crisp. */}
                <div className="absolute inset-0 bg-gradient-to-t from-brand-dark via-brand-dark/70 to-transparent" />
                <div className="absolute inset-0 bg-gradient-to-r from-brand-dark via-brand-dark/40 to-transparent" />
                <div className="absolute inset-0 bg-gradient-to-b from-brand-dark/60 via-transparent to-transparent" />
              </div>

              {/* Content */}
              <div className="relative h-full px-6 sm:px-10 lg:px-12 flex items-end pb-10 sm:pb-14">
                <div className={`max-w-xl sm:max-w-2xl ${isActive ? "fade-up" : ""}`}>
                  {/* Meta chips (year · quality · duration) */}
                  <div className="mb-4 flex items-center gap-2 flex-wrap">
                    {movie.year > 0 && (
                      <span className="glass-pill rounded-full px-3 py-1 text-xs font-medium text-gray-200">
                        {movie.year}
                      </span>
                    )}
                    {movie.quality && (
                      <span className="glass-pill rounded-full px-3 py-1 text-xs font-semibold text-orange-300">
                        {movie.quality}
                      </span>
                    )}
                    {movie.duration > 0 && (
                      <span className="glass-pill rounded-full px-3 py-1 text-xs font-medium text-gray-200">
                        {formatDuration(movie.duration)}
                      </span>
                    )}
                  </div>

                  {/* Title */}
                  <h1 className="text-4xl sm:text-5xl md:text-6xl font-bold tracking-tight text-white leading-[1.05] mb-4 [text-shadow:0_2px_30px_rgba(0,0,0,0.5)]">
                    {movie.title}
                  </h1>

                  {/* Description */}
                  <p className="text-gray-300/90 text-sm sm:text-base leading-relaxed line-clamp-2 mb-6 max-w-lg">
                    {movie.description}
                  </p>

                  {/* Buttons */}
                  <div className="flex items-center gap-3 flex-wrap">
                    <Link
                      href={`/movies/${movie.slug}?play=1`}
                      className="glass-hover flex items-center gap-2 bg-white text-black hover:bg-white font-semibold px-6 py-3 rounded-full text-sm shadow-lg shadow-black/30"
                      aria-label={`Ko'rish: ${movie.title}`}
                    >
                      <Play size={18} fill="currentColor" aria-hidden="true" />
                      Ko&apos;rish
                    </Link>
                    <Link
                      href={`/movies/${movie.slug}`}
                      className="glass glass-hover flex items-center gap-2 text-white font-medium px-6 py-3 rounded-full text-sm"
                      aria-label={`Batafsil: ${movie.title}`}
                    >
                      Batafsil
                    </Link>
                  </div>
                </div>
              </div>
            </div>
          );
        })}
      </div>

      {/* Dots Pagination — floating glass capsule */}
      {movies.length > 1 && (
        <div className="absolute bottom-6 left-1/2 -translate-x-1/2 z-20 glass-pill flex gap-1.5 rounded-full px-3 py-2">
          {movies.map((movie, index) => (
            <button
              key={movie.id || movie.slug || `hero-${index}`}
              type="button"
              onClick={() => goTo(index)}
              onMouseEnter={() => markLoaded(index)}
              onFocus={() => markLoaded(index)}
              className="relative py-1 flex items-center justify-center"
              aria-label={`Slayd ${index + 1}`}
              aria-current={index === currentIndex ? "true" : undefined}
            >
              <span
                className={`block h-1.5 rounded-full transition-all duration-500 ease-out ${
                  index === currentIndex
                    ? "bg-white w-7"
                    : "bg-white/40 hover:bg-white/70 w-1.5"
                }`}
              />
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
