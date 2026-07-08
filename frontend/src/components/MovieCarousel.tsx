"use client";

import { memo, useRef, useState, useEffect } from "react";
import { ChevronRight, ChevronLeft } from "lucide-react";
import { Movie } from "@/lib/api";
import MovieCard from "./MovieCard";

interface MovieCarouselProps {
  movies: Movie[];
  /**
   * Number of leading cards to mark as priority (eager + fetchPriority=high).
   * Only use this for the first above-the-fold carousel on a page. Defaults
   * to 0 so below-the-fold carousels lazy-load every poster.
   */
  priorityCount?: number;
}

function MovieCarouselImpl({ movies, priorityCount = 0 }: MovieCarouselProps) {
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
      window.addEventListener("resize", checkScroll, { passive: true });
      checkScroll();
      return () => {
        scrollElement.removeEventListener("scroll", checkScroll);
        window.removeEventListener("resize", checkScroll);
      };
    }
  }, [movies.length]);

  const scroll = (direction: "left" | "right") => {
    if (!scrollRef.current) return;
    const scrollAmount = scrollRef.current.clientWidth * 0.8;
    scrollRef.current.scrollBy({
      left: direction === "left" ? -scrollAmount : scrollAmount,
      behavior: "smooth",
    });
  };

  if (movies.length === 0) return null;

  return (
    <div className="relative">
      {/* Navigation Buttons */}
      <button
        onClick={() => scroll("left")}
        className={`absolute left-0 top-1/2 -translate-y-1/2 z-10 w-10 h-10 glass-strong hover:bg-brand-red rounded-full hidden sm:flex items-center justify-center transition-all duration-300 hover:scale-110 shadow-lg ${
          canScrollLeft ? "ml-2 opacity-100" : "-ml-12 opacity-0 pointer-events-none"
        }`}
        aria-label="Chapga surish"
      >
        <ChevronLeft size={20} className="text-white" aria-hidden="true" />
      </button>

      <button
        onClick={() => scroll("right")}
        className={`absolute right-0 top-1/2 -translate-y-1/2 z-10 w-10 h-10 glass-strong hover:bg-brand-red rounded-full hidden sm:flex items-center justify-center transition-all duration-300 hover:scale-110 shadow-lg ${
          canScrollRight ? "mr-2 opacity-100" : "-mr-12 opacity-0 pointer-events-none"
        }`}
        aria-label="O'ngga surish"
      >
        <ChevronRight size={20} className="text-white" aria-hidden="true" />
      </button>

      {/* Carousel Container */}
      <div
        ref={scrollRef}
        className="flex gap-3 overflow-x-auto scrollbar-hide pb-2 snap-x snap-mandatory isolate"
        style={{
          scrollbarWidth: "none",
          msOverflowStyle: "none",
        }}
      >
        {movies.map((movie, index) => (
          <div
            key={movie.id}
            className="shrink-0 w-[145px] sm:w-[168px] md:w-[188px] lg:w-[210px] snap-start isolate"
          >
            <MovieCard
              movie={movie}
              priority={index < priorityCount}
              showSkeleton={true}
            />
          </div>
        ))}
      </div>
    </div>
  );
}

const MovieCarousel = memo(MovieCarouselImpl);
MovieCarousel.displayName = "MovieCarousel";

export default MovieCarousel;
