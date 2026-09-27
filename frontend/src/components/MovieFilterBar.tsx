"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { SlidersHorizontal, X } from "lucide-react";
import { getMovieFilterFacets, MovieFilterFacets } from "@/lib/api";

const FILTER_KEYS = ["year_from", "year_to", "min_rating", "country", "duration", "free", "sort"] as const;

const selectCls =
  "bg-brand-card border border-white/10 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-brand-red";

/**
 * Advanced filter for /movies: year range, minimum rating, country,
 * duration, free-only and sort. State lives in the URL (?year_from=…), so
 * filtered lists can be shared and survive refresh; the page re-renders on
 * the server with the new params.
 */
export default function MovieFilterBar() {
  const router = useRouter();
  const params = useSearchParams();
  const [facets, setFacets] = useState<MovieFilterFacets | null>(null);
  const [open, setOpen] = useState(() => FILTER_KEYS.some((k) => params.get(k)));

  useEffect(() => {
    getMovieFilterFacets().then(setFacets).catch(() => {});
  }, []);

  const years = useMemo(() => {
    const max = facets?.year_max || new Date().getFullYear();
    const min = Math.max(1950, facets?.year_min || 1980);
    const out: number[] = [];
    for (let y = max; y >= min; y--) out.push(y);
    return out;
  }, [facets]);

  const activeCount = FILTER_KEYS.filter((k) => k !== "sort" && params.get(k)).length;

  const update = (key: (typeof FILTER_KEYS)[number], value: string) => {
    const next = new URLSearchParams(params.toString());
    if (value) next.set(key, value);
    else next.delete(key);
    next.delete("page"); // new filter → first page
    router.push(`/movies?${next.toString()}`, { scroll: false });
  };

  const clearAll = () => {
    const next = new URLSearchParams(params.toString());
    FILTER_KEYS.forEach((k) => next.delete(k));
    next.delete("page");
    const qs = next.toString();
    router.push(qs ? `/movies?${qs}` : "/movies", { scroll: false });
  };

  const v = (k: string) => params.get(k) || "";

  return (
    <div className="mb-6">
      <div className="flex flex-wrap items-center gap-2">
        <button
          onClick={() => setOpen((o) => !o)}
          aria-expanded={open}
          className="inline-flex items-center gap-2 rounded-lg border border-white/10 bg-brand-card px-3 py-2 text-sm text-gray-200 hover:border-gray-500"
        >
          <SlidersHorizontal size={15} />
          Filtr
          {activeCount > 0 && (
            <span className="rounded-full bg-brand-red px-1.5 text-[11px] font-semibold text-white">{activeCount}</span>
          )}
        </button>
        <select value={v("sort") || "new"} onChange={(e) => update("sort", e.target.value === "new" ? "" : e.target.value)} className={selectCls} aria-label="Saralash">
          <option value="new">Yangi qo&apos;shilganlar</option>
          <option value="popular">Eng ko&apos;p ko&apos;rilgan</option>
          <option value="rating">Reyting bo&apos;yicha</option>
          <option value="year">Chiqqan yili bo&apos;yicha</option>
        </select>
        {activeCount > 0 && (
          <button onClick={clearAll} className="inline-flex items-center gap-1 text-sm text-gray-400 hover:text-white">
            <X size={14} /> Tozalash
          </button>
        )}
      </div>

      {open && (
        <div className="mt-3 grid grid-cols-2 gap-2 rounded-xl border border-white/10 bg-brand-card/60 p-3 sm:grid-cols-3 lg:grid-cols-6">
          <label className="flex flex-col gap-1 text-xs text-gray-400">
            Yildan
            <select value={v("year_from")} onChange={(e) => update("year_from", e.target.value)} className={selectCls}>
              <option value="">Istalgan</option>
              {years.map((y) => (
                <option key={y} value={y}>{y}</option>
              ))}
            </select>
          </label>
          <label className="flex flex-col gap-1 text-xs text-gray-400">
            Yilgacha
            <select value={v("year_to")} onChange={(e) => update("year_to", e.target.value)} className={selectCls}>
              <option value="">Istalgan</option>
              {years.map((y) => (
                <option key={y} value={y}>{y}</option>
              ))}
            </select>
          </label>
          <label className="flex flex-col gap-1 text-xs text-gray-400">
            Reyting
            <select value={v("min_rating")} onChange={(e) => update("min_rating", e.target.value)} className={selectCls}>
              <option value="">Istalgan</option>
              <option value="4.5">4.5★ va yuqori</option>
              <option value="4">4★ va yuqori</option>
              <option value="3">3★ va yuqori</option>
            </select>
          </label>
          <label className="flex flex-col gap-1 text-xs text-gray-400">
            Mamlakat
            <select value={v("country")} onChange={(e) => update("country", e.target.value)} className={selectCls}>
              <option value="">Istalgan</option>
              {(facets?.countries || []).map((c) => (
                <option key={c} value={c}>{c}</option>
              ))}
            </select>
          </label>
          <label className="flex flex-col gap-1 text-xs text-gray-400">
            Davomiylik
            <select value={v("duration")} onChange={(e) => update("duration", e.target.value)} className={selectCls}>
              <option value="">Istalgan</option>
              <option value="short">1.5 soatgacha</option>
              <option value="medium">1.5–2 soat</option>
              <option value="long">2 soatdan uzun</option>
            </select>
          </label>
          <label className="flex items-end gap-2 pb-2 text-sm text-gray-300 cursor-pointer select-none">
            <input
              type="checkbox"
              checked={v("free") === "1"}
              onChange={(e) => update("free", e.target.checked ? "1" : "")}
              className="h-4 w-4 accent-brand-red"
            />
            Faqat bepul
          </label>
        </div>
      )}
    </div>
  );
}
