"use client";

import { useRef, useState, useEffect } from "react";
import { ChevronRight, ChevronLeft } from "lucide-react";
import { Movie } from "@/lib/api";
import MovieCard from "./MovieCard";

interface MovieCarouselProps {
  movies: Movie[];
}

export default function MovieCarousel({ movies }: MovieCarouselProps) {
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
      scrollElement.addEventListener("scroll", checkScroll);
      checkScroll();
      return () => scrollElement.removeEventListener("scroll", checkScroll);
    }
  }, [movies]);

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
        className={`absolute left-0 top-1/2 -translate-y-1/2 z-10 w-10 h-10 bg-brand-dark/80 hover:bg-brand-red rounded-full flex items-center justify-center transition-all duration-300 opacity-100 hover:scale-110 shadow-lg ${
          canScrollLeft ? "ml-2" : "-ml-12"
        }`}
        aria-label="Scroll left"
      >
        <ChevronLeft size={20} className="text-white" />
      </button>

      <button
        onClick={() => scroll("right")}
        className={`absolute right-0 top-1/2 -translate-y-1/2 z-10 w-10 h-10 bg-brand-dark/80 hover:bg-brand-red rounded-full flex items-center justify-center transition-all duration-300 opacity-100 hover:scale-110 shadow-lg ${
          canScrollRight ? "mr-2" : "-mr-12"
        }`}
        aria-label="Scroll right"
      >
        <ChevronRight size={20} className="text-white" />
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
              className="shrink-0 w-[120px] sm:w-[140px] md:w-[160px] lg:w-[180px] snap-start isolate"
            >
              <MovieCard
                movie={movie}
                priority={index < 3} // Priority load first 3 cards
                showSkeleton={true}
              />
            </div>
          ))}
      </div>
    </div>
  );
}
