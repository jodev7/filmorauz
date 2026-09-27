"use client";

import { memo, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { ChevronLeft, ChevronRight, Play, History, X, Check, MoreVertical } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { getContinueWatching, ContinueWatchingItem, hideFromContinueWatching, markAsWatched, restoreToContinueWatching } from "@/lib/api";
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

function itemTarget(item: ContinueWatchingItem): { type: "movie" | "episode"; id: string } {
  if (item.type === "episode" || item.target_type === "episode") {
    return { type: "episode", id: item.episode_id || item.target_id };
  }
  return { type: "movie", id: item.movie_id || item.target_id };
}

function itemKey(item: ContinueWatchingItem): string {
  return `${item.target_type}-${item.target_id}-${item.episode_id || ""}`;
}

function ContinueWatchingRowImpl() {
  const { token, isLoading } = useAuth();
  const [items, setItems] = useState<ContinueWatchingItem[]>([]);
  const [loaded, setLoaded] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);
  const [menuFor, setMenuFor] = useState<string | null>(null);
  const [undo, setUndo] = useState<{ item: ContinueWatchingItem; index: number; label: string } | null>(null);

  // Close the card menu on outside click / Escape.
  useEffect(() => {
    if (!menuFor) return;
    const close = () => setMenuFor(null);
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && close();
    window.addEventListener("click", close);
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("click", close);
      window.removeEventListener("keydown", onKey);
    };
  }, [menuFor]);

  useEffect(() => {
    if (!undo) return;
    const t = setTimeout(() => setUndo(null), 5000);
    return () => clearTimeout(t);
  }, [undo]);

  const act = async (item: ContinueWatchingItem, action: "hide" | "watched") => {
    if (!token) return;
    setMenuFor(null);
    const key = itemKey(item);
    const index = items.findIndex((i) => itemKey(i) === key);
    setItems((prev) => prev.filter((i) => itemKey(i) !== key));
    const { type, id } = itemTarget(item);
    try {
      if (action === "hide") await hideFromContinueWatching(token, type, id);
      else await markAsWatched(token, type, id);
      setUndo(action === "hide" ? { item, index, label: "Ro'yxatdan olib tashlandi" } : null);
    } catch {
      // restore on failure
      setItems((prev) => {
        const next = [...prev];
        next.splice(Math.max(0, index), 0, item);
        return next;
      });
    }
  };

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
  if (!loaded || (items.length === 0 && !undo)) return null;

  return (
    <section className="max-w-7xl mx-auto px-4 py-6">
      <div className="flex items-center justify-between mb-4">
        <h2 className="font-display text-xl sm:text-2xl tracking-wide text-white flex items-center gap-2">
          <History size={20} className="text-orange-500" aria-hidden="true" />
          Tomosha qilishda davom eting
        </h2>
      </div>

      <div className="relative">
        {!menuFor && (
        <>
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
        </>
        )}

        <div
          ref={scrollRef}
          className="flex gap-3 overflow-x-auto scrollbar-hide pb-2 snap-x"
          style={{ scrollbarWidth: "none", msOverflowStyle: "none" }}
        >
          {items.map((item) => {
            const key = itemKey(item);
            return (
              <div key={key} className="group relative shrink-0 w-[220px] sm:w-[260px] snap-start">
                <Link href={resumeHref(item)} className="block">
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
                  <p className="mt-2 pr-6 text-sm font-medium text-white line-clamp-1 group-hover:text-orange-500 transition-colors">
                    {itemLabel(item)}
                  </p>
                </Link>

                {/* Card menu: hide / mark watched */}
                <button
                  type="button"
                  onClick={(e) => {
                    e.stopPropagation();
                    setMenuFor((m) => (m === key ? null : key));
                  }}
                  className="absolute right-2 top-2 z-10 flex h-8 w-8 items-center justify-center rounded-full bg-black/70 text-white opacity-100 transition-opacity hover:bg-black sm:opacity-0 sm:group-hover:opacity-100 focus:opacity-100"
                  aria-label="Amallar"
                  aria-haspopup="menu"
                  aria-expanded={menuFor === key}
                >
                  <MoreVertical size={16} />
                </button>
                {menuFor === key && (
                  <div
                    role="menu"
                    onClick={(e) => e.stopPropagation()}
                    className="absolute right-2 top-11 z-30 w-52 overflow-hidden rounded-xl border border-white/10 bg-[#15151f] py-1 shadow-2xl"
                  >
                    <button
                      role="menuitem"
                      onClick={() => act(item, "watched")}
                      className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm text-gray-200 hover:bg-white/5"
                    >
                      <Check size={15} className="text-emerald-400" /> Ko&apos;rildi deb belgilash
                    </button>
                    <button
                      role="menuitem"
                      onClick={() => act(item, "hide")}
                      className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm text-gray-200 hover:bg-white/5"
                    >
                      <X size={15} className="text-gray-400" /> Ro&apos;yxatdan olib tashlash
                    </button>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      </div>
      {undo && (
        <div className="mt-2 flex items-center gap-3 text-sm text-gray-400" role="status">
          {undo.label}.
          <button
            onClick={() => {
              const { type, id } = itemTarget(undo.item);
              if (token) restoreToContinueWatching(token, type, id).catch(() => {});
              setItems((prev) => {
                const next = [...prev];
                next.splice(Math.max(0, undo.index), 0, undo.item);
                return next;
              });
              setUndo(null);
            }}
            className="font-medium text-orange-400 hover:underline"
          >
            Qaytarish
          </button>
        </div>
      )}
    </section>
  );
}

const ContinueWatchingRow = memo(ContinueWatchingRowImpl);
ContinueWatchingRow.displayName = "ContinueWatchingRow";

export default ContinueWatchingRow;
