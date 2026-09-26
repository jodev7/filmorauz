"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { Dices, Loader2, Play, RefreshCw, Star, X } from "lucide-react";
import { getRandomMovie, Movie } from "@/lib/api";
import { getLocalizedTitle, localizeSingleGenre, getLocalizedDescription } from "@/lib/localization";
import { normalizeMediaUrl } from "@/lib/image-utils";
import MediaImage from "@/components/ui/MediaImage";

const GENRES = ["", "action", "comedy", "drama", "horror", "thriller", "sci-fi", "animation", "romance"];

/** "Nima ko'rsam ekan?" — shows one random playable movie, re-rollable. */
export function RandomMovieModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [genre, setGenre] = useState("");
  const [movie, setMovie] = useState<Movie | null>(null);
  const [loading, setLoading] = useState(false);
  const [empty, setEmpty] = useState(false);
  const seen = useRef<string[]>([]);

  const roll = useCallback(async (g: string) => {
    setLoading(true);
    setEmpty(false);
    try {
      let m = await getRandomMovie(g, seen.current);
      if (!m && seen.current.length > 0) {
        seen.current = []; // everything seen once — start over
        m = await getRandomMovie(g);
      }
      if (m) seen.current.push(m.id);
      setMovie(m);
      setEmpty(!m);
    } catch {
      setEmpty(true);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (open) roll(genre);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    document.body.style.overflow = "hidden";
    return () => {
      window.removeEventListener("keydown", onKey);
      document.body.style.overflow = "";
    };
  }, [open, onClose]);

  if (!open) return null;

  const pickGenre = (g: string) => {
    setGenre(g);
    seen.current = [];
    roll(g);
  };

  const title = movie ? getLocalizedTitle(movie) : "";
  const image = movie ? normalizeMediaUrl(movie.backdrop_url || movie.poster_url) : "";

  return (
    <div className="fixed inset-0 z-[80] flex items-end justify-center bg-black/75 p-0 sm:items-center sm:p-4" onClick={onClose} role="dialog" aria-modal="true" aria-labelledby="random-title">
      <div
        className="relative w-full max-w-lg overflow-hidden rounded-t-2xl border border-white/10 bg-[#101018] shadow-2xl sm:rounded-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between px-5 pt-4">
          <h2 id="random-title" className="flex items-center gap-2 text-base font-semibold text-white">
            <Dices size={18} className="text-orange-400" /> Nima ko&apos;rsam ekan?
          </h2>
          <button onClick={onClose} className="rounded-lg p-1.5 text-gray-400 hover:bg-white/5 hover:text-white" aria-label="Yopish">
            <X size={18} />
          </button>
        </div>

        <div className="scrollbar-hide flex gap-1.5 overflow-x-auto px-5 py-3">
          {GENRES.map((g) => (
            <button
              key={g || "all"}
              onClick={() => pickGenre(g)}
              className={`shrink-0 rounded-full px-3 py-1 text-xs transition-colors ${
                genre === g ? "bg-orange-500 text-white" : "border border-white/10 text-gray-300 hover:border-orange-500/50"
              }`}
            >
              {g ? localizeSingleGenre(g) : "Hammasi"}
            </button>
          ))}
        </div>

        <div className="px-5 pb-5">
          {loading && !movie ? (
            <div className="flex aspect-video items-center justify-center rounded-xl bg-white/5">
              <Loader2 className="animate-spin text-gray-500" />
            </div>
          ) : empty || !movie ? (
            <div className="flex aspect-video flex-col items-center justify-center rounded-xl bg-white/5 text-sm text-gray-400">
              Bu janrda kino topilmadi.
            </div>
          ) : (
            <div className={loading ? "opacity-50 transition-opacity" : "transition-opacity"}>
              <div className="relative aspect-video overflow-hidden rounded-xl bg-white/5">
                {image && <MediaImage src={image} alt={title} className="h-full w-full object-cover" />}
                <div className="absolute inset-0 bg-gradient-to-t from-[#101018] via-transparent to-transparent" />
              </div>
              <h3 className="mt-3 text-xl font-semibold text-white">{title}</h3>
              <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-gray-400">
                {movie.year ? <span>{movie.year}</span> : null}
                {movie.rating_avg ? (
                  <span className="inline-flex items-center gap-1">
                    <Star size={13} className="fill-yellow-400 text-yellow-400" /> {movie.rating_avg.toFixed(1)}
                  </span>
                ) : null}
                {(movie.genre || []).slice(0, 3).map((g) => (
                  <span key={g}>{localizeSingleGenre(g)}</span>
                ))}
              </div>
              <p className="mt-2 line-clamp-3 text-sm text-gray-300">{getLocalizedDescription(movie)}</p>
            </div>
          )}

          <div className="mt-4 flex gap-2">
            <button
              onClick={() => roll(genre)}
              disabled={loading}
              className="inline-flex flex-1 items-center justify-center gap-2 rounded-xl border border-white/10 py-3 text-sm font-medium text-white hover:bg-white/5 disabled:opacity-60"
            >
              {loading ? <Loader2 size={16} className="animate-spin" /> : <RefreshCw size={16} />} Boshqasi
            </button>
            {movie && (
              <Link
                href={`/movies/${movie.slug}?play=1`}
                onClick={onClose}
                className="inline-flex flex-1 items-center justify-center gap-2 rounded-xl bg-orange-500 py-3 text-sm font-semibold text-white hover:bg-orange-600"
              >
                <Play size={16} fill="white" /> Tomosha qilish
              </Link>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

/** Trigger button; `variant="tile"` matches the home quick-action tiles. */
export default function RandomMovieButton({ variant = "button", className = "" }: { variant?: "button" | "tile"; className?: string }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      {variant === "tile" ? (
        <button
          onClick={() => setOpen(true)}
          className={`glass glass-hover group flex items-center gap-3 rounded-2xl px-4 py-3.5 text-left text-sm font-medium text-white ${className}`}
        >
          <span className="glass-pill inline-flex h-9 w-9 items-center justify-center rounded-xl">
            <Dices size={18} className="text-orange-500" aria-hidden="true" />
          </span>
          <span className="tracking-tight">Tasodifiy kino</span>
        </button>
      ) : (
        <button
          onClick={() => setOpen(true)}
          className={`inline-flex items-center gap-2 rounded-xl border border-white/10 px-4 py-2.5 text-sm text-white hover:border-orange-500/50 hover:bg-white/5 ${className}`}
        >
          <Dices size={16} className="text-orange-400" /> Tasodifiy kino
        </button>
      )}
      <RandomMovieModal open={open} onClose={() => setOpen(false)} />
    </>
  );
}
