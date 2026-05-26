"use client";

import { memo, useRef, useState, useEffect } from "react";
import { ChevronRight, ChevronLeft } from "lucide-react";
import { Series } from "@/lib/series-api";
import SeriesCard from "@/components/SeriesCard";

interface SeriesCarouselProps {
  series: Series[];
}

function SeriesCarouselImpl({ series }: SeriesCarouselProps) {
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
  }, [series.length]);

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
          <div key={s.id} className="shrink-0 w-[140px] sm:w-[160px]">
            <SeriesCard series={s} />
          </div>
        ))}
      </div>
    </div>
  );
}

const SeriesCarousel = memo(SeriesCarouselImpl);
SeriesCarousel.displayName = "SeriesCarousel";

export default SeriesCarousel;
