"use client";

import { memo, useRef, useState, useEffect } from "react";
import { ChevronRight, ChevronLeft } from "lucide-react";
import { Series } from "@/lib/series-api";
import SeriesCard from "@/components/SeriesCard";
import { useDragScroll } from "@/lib/use-drag-scroll";

interface SeriesCarouselProps {
  series: Series[];
}

function SeriesCarouselImpl({ series }: SeriesCarouselProps) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const dragHandlers = useDragScroll(scrollRef);
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

      {/* Cards Container */}
      <div
        ref={scrollRef}
        {...dragHandlers}
        className="flex gap-4 overflow-x-auto scrollbar-hide pb-2 px-2 sm:cursor-grab sm:active:cursor-grabbing"
        style={{ scrollbarWidth: "none", msOverflowStyle: "none" }}
      >
        {series.map((s) => (
          <div key={s.id} className="shrink-0 w-[165px] sm:w-[188px] md:w-[208px]">
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
