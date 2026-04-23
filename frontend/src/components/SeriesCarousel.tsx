"use client";

import { useRef, useState, useEffect } from "react";
import { ChevronRight, ChevronLeft, Play, Star, Tv } from "lucide-react";
import { Series } from "@/lib/series-api";
import Link from "next/link";
import { localizeSingleGenre } from "@/lib/localization";

interface SeriesCarouselProps {
  series: Series[];
}

export default function SeriesCarousel({ series }: SeriesCarouselProps) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const [canScrollLeft, setCanScrollLeft] = useState(false);
  const [canScrollRight, setCanScrollRight] = useState(true);

  const checkScroll = () => {
    if (!scrollRef.current) return;
    const { scrollLeft, scrollWidth, clientWidth } = scrollRef.current;
    setCanScrollLeft(scrollLeft > 0);
    setCanScrollRight(scrollLeft < scrollWidth - clientWidth - 10);
  };

  useEffect(() => {
    const scrollElement = scrollRef.current;
    if (scrollElement) {
      scrollElement.addEventListener("scroll", checkScroll, { passive: true });
      checkScroll();
      return () => scrollElement.removeEventListener("scroll", checkScroll);
    }
  }, [series]);

  const scroll = (direction: "left" | "right") => {
    if (!scrollRef.current) return;
    const scrollAmount = scrollRef.current.clientWidth * 0.8;
    scrollRef.current.scrollBy({
      left: direction === "left" ? -scrollAmount : scrollAmount,
      behavior: "smooth",
    });
  };

  if (series.length === 0) return null;

  return (
    <div className="relative">
      {/* Navigation Buttons */}
      <button
        onClick={() => scroll("left")}
        className={`absolute left-0 top-1/2 -translate-y-1/2 z-10 w-10 h-10 bg-brand-dark/80 hover:bg-brand-red rounded-full flex items-center justify-center transition-all duration-300 opacity-100 hover:scale-110 shadow-lg ${
          canScrollLeft ? "ml-2" : "-ml-12"
        }`}
        aria-label="Chapga surish"
      >
        <ChevronLeft size={20} className="text-white" aria-hidden="true" />
      </button>

      <button
        onClick={() => scroll("right")}
        className={`absolute right-0 top-1/2 -translate-y-1/2 z-10 w-10 h-10 bg-brand-dark/80 hover:bg-brand-red rounded-full flex items-center justify-center transition-all duration-300 opacity-100 hover:scale-110 shadow-lg ${
          canScrollRight ? "mr-2" : "-mr-12"
        }`}
        aria-label="O'ngga surish"
      >
        <ChevronRight size={20} className="text-white" aria-hidden="true" />
      </button>

      {/* Cards Container */}
      <div
        ref={scrollRef}
        className="flex gap-4 overflow-x-auto scrollbar-hide pb-2 px-2"
        style={{ scrollbarWidth: "none", msOverflowStyle: "none" }}
      >
        {series.map((s) => (
          <Link
            key={s.id}
            href={`/series/${s.slug}`}
            className="series-card group relative bg-brand-card rounded-xl overflow-hidden border border-brand-border transition-all duration-300 hover:scale-[1.02] hover:shadow-2xl block shrink-0 w-[140px] sm:w-[160px]"
          >
            {/* Poster */}
            <div className="relative aspect-[2/3] overflow-hidden bg-brand-border">
              <img
                src={s.poster_url || "/placeholder-poster.jpg"}
                alt={s.title}
                width={240}
                height={360}
                className="w-full h-full object-cover transition-transform duration-500 group-hover:scale-105"
                loading="lazy"
                decoding="async"
                onError={(e) => {
                  (e.target as HTMLImageElement).src = "/placeholder-poster.jpg";
                }}
              />

              {/* Premium overlay */}
              {s.is_premium && (
                <div className="absolute inset-0 bg-gradient-to-t from-black/80 via-black/20 to-transparent opacity-60" />
              )}

              {/* Overlay on hover */}
              <div className="absolute inset-0 bg-black/60 opacity-0 group-hover:opacity-100 transition-opacity duration-300 flex items-center justify-center">
                <div className={`w-12 h-12 sm:w-14 sm:h-14 rounded-full flex items-center justify-center shadow-lg ${
                  s.is_premium 
                    ? "bg-gradient-to-br from-yellow-500 to-amber-600" 
                    : "bg-brand-red"
                }`}>
                  <Play size={20} className="text-white ml-1" fill="white" />
                </div>
              </div>

              {/* Badges */}
              <div className="absolute top-2 left-2 right-2 flex justify-between items-start z-10">
                {/* Series badge */}
                <span className="bg-brand-red/80 text-white text-xs px-1.5 py-0.5 rounded flex items-center gap-1">
                  <Tv className="w-3 h-3" />
                  Serial
                </span>
              </div>

              {/* Premium badge */}
              {s.is_premium && (
                <div className="absolute top-2 right-2">
                  <span className="bg-yellow-500/80 text-white text-xs px-1.5 py-0.5 rounded flex items-center gap-1">
                    <Star className="w-3 h-3 fill-white" />
                  </span>
                </div>
              )}
            </div>

            {/* Info */}
            <div className="p-3">
              <h3 className="font-medium text-white text-sm truncate group-hover:text-brand-red transition-colors">
                {s.title}
              </h3>
              <div className="flex items-center gap-2 mt-1 text-xs text-gray-400 flex-wrap">
                <span>{s.year}</span>
                {s.genre && s.genre.length > 0 && (
                  <>
                    <span>•</span>
                    <span className="truncate">{localizeSingleGenre(s.genre[0])}</span>
                  </>
                )}
                {/* Rating */}
                {s.rating_avg > 0 && (
                  <>
                    <span>•</span>
                    <span className="flex items-center gap-0.5 text-yellow-400">
                      <Star size={11} className="fill-yellow-400" />
                      {s.rating_avg.toFixed(1)}
                    </span>
                  </>
                )}
              </div>
            </div>
          </Link>
        ))}
      </div>
    </div>
  );
}
