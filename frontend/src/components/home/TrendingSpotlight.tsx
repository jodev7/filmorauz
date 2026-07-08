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
    <section className="max-w-7xl mx-auto px-4 py-8">
      <div className="relative overflow-hidden rounded-3xl border border-white/10 shadow-[0_20px_60px_rgba(0,0,0,0.45)]">
        <div className="relative aspect-[16/9] sm:aspect-[21/9]">
          <OptimizedImage
            src={src}
            alt={movie.title}
            aspectRatio="21/9"
            className="h-full w-full"
          />
          {/* Gradients for text legibility */}
          <div className="absolute inset-0 bg-gradient-to-t from-brand-dark via-brand-dark/50 to-transparent" />
          <div className="absolute inset-0 bg-gradient-to-r from-brand-dark via-brand-dark/30 to-transparent" />
        </div>

        <div className="absolute inset-0 flex items-end p-5 sm:p-8 md:p-10">
          <div className="max-w-xl">
            <span className="glass-pill inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-semibold uppercase tracking-wider text-orange-300 mb-3">
              <Flame size={13} aria-hidden="true" />
              Hozir mashhur
            </span>
            <h2 className="text-2xl sm:text-3xl md:text-4xl font-bold tracking-tight text-white leading-tight mb-4 line-clamp-2 [text-shadow:0_2px_20px_rgba(0,0,0,0.5)]">
              {movie.title}
            </h2>
            <div className="flex items-center gap-2 mb-5 flex-wrap">
              {movie.year > 0 && (
                <span className="glass-pill rounded-full px-3 py-1 text-xs font-medium text-gray-200">
                  {movie.year}
                </span>
              )}
              {genre && (
                <span className="glass-pill rounded-full px-3 py-1 text-xs font-medium text-gray-200 capitalize">
                  {genre}
                </span>
              )}
              {movie.views > 0 && (
                <span className="glass-pill inline-flex items-center gap-1 rounded-full px-3 py-1 text-xs font-medium text-gray-200">
                  <Eye size={13} aria-hidden="true" />
                  {movie.views.toLocaleString()}
                </span>
              )}
            </div>
            <div className="flex items-center gap-3">
              <Link
                href={`/movies/${movie.slug}?play=1`}
                className="glass-hover flex items-center gap-2 bg-white text-black font-semibold px-6 py-3 rounded-full text-sm shadow-lg shadow-black/30"
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
    </section>
  );
}
