import Link from "next/link";
import { Play, Flame, Eye } from "lucide-react";
import { Movie } from "@/lib/api";
import OptimizedImage from "@/components/OptimizedImage";

interface TrendingSpotlightProps {
  movie: Movie;
}

// A wide landscape "spotlight" banner for the single hottest title. Breaks up
// the rhythm of identical 2/3 poster carousels with a large backdrop block.
export default function TrendingSpotlight({ movie }: TrendingSpotlightProps) {
  const src = movie.backdrop_url || movie.poster_url;
  const genre = (movie.genre || [])[0];

  return (
    <section className="max-w-7xl mx-auto px-4 py-6">
      <div className="relative overflow-hidden rounded-2xl border border-[#1e1e2e]">
        <div className="relative aspect-[16/9] sm:aspect-[21/9]">
          <OptimizedImage
            src={src}
            alt={movie.title}
            aspectRatio="21/9"
            className="h-full w-full"
          />
          {/* Gradients for text legibility */}
          <div className="absolute inset-0 bg-gradient-to-t from-brand-dark via-brand-dark/40 to-transparent" />
          <div className="absolute inset-0 bg-gradient-to-r from-brand-dark/90 via-brand-dark/20 to-transparent" />
        </div>

        <div className="absolute inset-0 flex items-end p-5 sm:p-8">
          <div className="max-w-xl">
            <span className="inline-flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-orange-400 mb-2">
              <Flame size={14} aria-hidden="true" />
              Hozir mashhur
            </span>
            <h2 className="font-display text-2xl sm:text-3xl md:text-4xl tracking-wide text-white leading-tight mb-3 line-clamp-2">
              {movie.title}
            </h2>
            <div className="flex items-center gap-3 text-sm text-gray-300 mb-4 flex-wrap">
              {movie.year > 0 && <span>{movie.year}</span>}
              {genre && (
                <span className="border border-gray-500 px-2 py-0.5 rounded text-xs font-medium capitalize">
                  {genre}
                </span>
              )}
              {movie.views > 0 && (
                <span className="flex items-center gap-1">
                  <Eye size={14} aria-hidden="true" />
                  {movie.views.toLocaleString()}
                </span>
              )}
            </div>
            <div className="flex items-center gap-3">
              <Link
                href={`/movies/${movie.slug}?play=1`}
                className="flex items-center gap-2 bg-brand-red hover:bg-orange-700 text-white font-semibold px-5 py-2.5 rounded-lg transition-colors text-sm"
                aria-label={`Ko'rish: ${movie.title}`}
              >
                <Play size={18} fill="white" aria-hidden="true" />
                Ko&apos;rish
              </Link>
              <Link
                href={`/movies/${movie.slug}`}
                className="flex items-center gap-2 bg-white/10 hover:bg-white/20 text-white font-medium px-5 py-2.5 rounded-lg backdrop-blur-sm transition-colors border border-white/10 text-sm"
                aria-label={`Batafsil: ${movie.title}`}
              >
                Batafsil
              </Link>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
