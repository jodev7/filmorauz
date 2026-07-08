"use client";

import { memo, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { ChevronLeft, ChevronRight, Play, History } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { getContinueWatching, ContinueWatchingItem } from "@/lib/api";
import OptimizedImage from "@/components/OptimizedImage";

function resumeHref(item: ContinueWatchingItem): string {
  if (item.type === "episode" || item.target_type === "episode") {
    const id = item.episode_id || item.target_id;
    return id ? `/episode/${id}` : "#";
  }
  return item.slug ? `/movies/${item.slug}?play=1` : "#";
}

function itemLabel(item: ContinueWatchingItem): string {
  if (item.series_title) {
    const code =
      item.season_number && item.episode_number
        ? `S${String(item.season_number).padStart(2, "0")}E${String(item.episode_number).padStart(2, "0")}`
        : item.episode_number
        ? `${item.episode_number}-qism`
        : "";
    return code ? `${item.series_title} · ${code}` : item.series_title;
  }
  return item.title;
}

function ContinueWatchingRowImpl() {
  const { token, isLoading } = useAuth();
  const [items, setItems] = useState<ContinueWatchingItem[]>([]);
  const [loaded, setLoaded] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (isLoading) return;
    if (!token) {
      setItems([]);
      setLoaded(true);
      return;
    }
    let active = true;
    getContinueWatching(token).then((data) => {
      if (active) {
        setItems(data.filter((i) => i.progress_percent < 95));
        setLoaded(true);
      }
    });
    return () => {
      active = false;
    };
  }, [token, isLoading]);

  const scroll = (dir: "left" | "right") => {
    if (!scrollRef.current) return;
    const amount = scrollRef.current.clientWidth * 0.8;
    scrollRef.current.scrollBy({ left: dir === "left" ? -amount : amount, behavior: "smooth" });
  };

  // Render nothing until we know there is something to show — avoids a layout
  // flash for logged-out users or empty histories.
  if (!loaded || items.length === 0) return null;

  return (
    <section className="max-w-7xl mx-auto px-4 py-6">
      <div className="flex items-center justify-between mb-4">
        <h2 className="font-display text-xl sm:text-2xl tracking-wide text-white flex items-center gap-2">
          <History size={20} className="text-orange-500" aria-hidden="true" />
          Tomosha qilishda davom eting
        </h2>
      </div>

      <div className="relative">
        <button
          onClick={() => scroll("left")}
          className="absolute left-0 top-1/2 -translate-y-1/2 z-10 w-10 h-10 bg-brand-dark/80 hover:bg-brand-red rounded-full flex items-center justify-center transition-all duration-300 hover:scale-110 shadow-lg ml-2"
          aria-label="Chapga surish"
        >
          <ChevronLeft size={20} className="text-white" aria-hidden="true" />
        </button>
        <button
          onClick={() => scroll("right")}
          className="absolute right-0 top-1/2 -translate-y-1/2 z-10 w-10 h-10 bg-brand-dark/80 hover:bg-brand-red rounded-full flex items-center justify-center transition-all duration-300 hover:scale-110 shadow-lg mr-2"
          aria-label="O'ngga surish"
        >
          <ChevronRight size={20} className="text-white" aria-hidden="true" />
        </button>

        <div
          ref={scrollRef}
          className="flex gap-3 overflow-x-auto scrollbar-hide pb-2 snap-x"
          style={{ scrollbarWidth: "none", msOverflowStyle: "none" }}
        >
          {items.map((item) => (
            <Link
              key={`${item.target_type}-${item.target_id}-${item.episode_id || ""}`}
              href={resumeHref(item)}
              className="group shrink-0 w-[220px] sm:w-[260px] snap-start"
            >
              <div className="relative aspect-video overflow-hidden rounded-xl bg-[#1a1a24] border border-white/10 group-hover:border-orange-500/40 transition-colors">
                <OptimizedImage
                  src={item.poster_url}
                  alt={item.title}
                  aspectRatio="16/9"
                  className="transition-transform duration-500 group-hover:scale-105"
                />
                <div className="absolute inset-0 bg-black/40 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center">
                  <div className="w-12 h-12 rounded-full bg-orange-500 flex items-center justify-center shadow-lg">
                    <Play size={20} className="text-white ml-1" fill="white" />
                  </div>
                </div>
                {/* Progress bar */}
                <div className="absolute bottom-0 left-0 right-0 h-1 bg-black/50">
                  <div
                    className="h-full bg-orange-500"
                    style={{ width: `${Math.min(100, Math.max(2, item.progress_percent))}%` }}
                  />
                </div>
              </div>
              <p className="mt-2 text-sm font-medium text-white line-clamp-1 group-hover:text-orange-500 transition-colors">
                {itemLabel(item)}
              </p>
            </Link>
          ))}
        </div>
      </div>
    </section>
  );
}

const ContinueWatchingRow = memo(ContinueWatchingRowImpl);
ContinueWatchingRow.displayName = "ContinueWatchingRow";

export default ContinueWatchingRow;
