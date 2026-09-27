"use client";

import { useCallback, useEffect, useState } from "react";
import { Film, Loader2, Search, X } from "lucide-react";
import { adminSearchTmdb, TmdbSearchItem } from "@/lib/api";

/**
 * Pick the right TMDB title by hand when the automatic match fails (site
 * titles are Uzbek; TMDB knows the original / Russian / English ones).
 */
export default function TmdbPicker({
  open,
  type,
  token,
  initialQuery,
  initialYear,
  onPick,
  onClose,
}: {
  open: boolean;
  type: "movie" | "tv";
  token: string | null | undefined;
  initialQuery: string;
  initialYear?: number;
  onPick: (item: TmdbSearchItem) => void;
  onClose: () => void;
}) {
  const [q, setQ] = useState(initialQuery);
  const [year, setYear] = useState(initialYear ? String(initialYear) : "");
  const [items, setItems] = useState<TmdbSearchItem[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const search = useCallback(
    async (query: string, y: string) => {
      if (!token || !query.trim()) return;
      setBusy(true);
      setError("");
      try {
        setItems(await adminSearchTmdb(token, type, query.trim(), Number(y) || undefined));
      } catch (err) {
        setError(err instanceof Error ? err.message : "Qidirib bo'lmadi");
      } finally {
        setBusy(false);
      }
    },
    [token, type]
  );

  useEffect(() => {
    if (!open) return;
    setQ(initialQuery);
    setYear(initialYear ? String(initialYear) : "");
    setItems(null);
    void search(initialQuery, initialYear ? String(initialYear) : "");
  }, [open, initialQuery, initialYear, search]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  if (!open) return null;
  const label = type === "tv" ? "serial" : "kino";

  return (
    <div className="fixed inset-0 z-[60] flex items-end justify-center bg-black/70 p-0 sm:items-center sm:p-4" onClick={onClose} role="dialog" aria-modal="true" aria-labelledby="tmdb-picker-title">
      <div className="flex max-h-[85vh] w-full max-w-lg flex-col rounded-t-2xl border border-white/10 bg-[#12121a] shadow-2xl sm:rounded-2xl" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-start justify-between gap-3 border-b border-white/5 p-4">
          <div>
            <h2 id="tmdb-picker-title" className="font-semibold text-white">TMDB&apos;dan {label} tanlash</h2>
            <p className="mt-0.5 text-xs text-gray-500">Asl, inglizcha yoki ruscha nomi bilan qidiring</p>
          </div>
          <button type="button" onClick={onClose} className="rounded-lg p-1.5 text-gray-500 hover:bg-white/5 hover:text-white" aria-label="Yopish">
            <X size={16} />
          </button>
        </div>

        {/* Not a <form>: the picker is rendered inside the movie form. */}
        <div
          className="flex gap-2 p-4 pb-3"
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              void search(q, year);
            }
          }}
        >
          <input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="Masalan: Avengers Endgame"
            autoFocus
            className="min-w-0 flex-1 rounded-xl border border-white/10 bg-black/30 px-3 py-2 text-sm text-white placeholder-gray-600 focus:border-orange-500 focus:outline-none"
          />
          <input
            value={year}
            onChange={(e) => setYear(e.target.value.replace(/\D/g, "").slice(0, 4))}
            placeholder="Yil"
            inputMode="numeric"
            className="w-20 rounded-xl border border-white/10 bg-black/30 px-3 py-2 text-sm text-white placeholder-gray-600 focus:border-orange-500 focus:outline-none"
          />
          <button type="button" onClick={() => void search(q, year)} disabled={busy} className="inline-flex items-center justify-center rounded-xl bg-orange-500 px-3 text-white hover:bg-orange-400 disabled:opacity-60" aria-label="Qidirish">
            {busy ? <Loader2 size={16} className="animate-spin" /> : <Search size={16} />}
          </button>
        </div>

        <div className="min-h-[120px] flex-1 overflow-y-auto px-2 pb-3">
          {error && <p className="mx-2 rounded-lg bg-red-500/10 px-3 py-2 text-sm text-red-300">{error}</p>}
          {!error && items && items.length === 0 && (
            <p className="px-3 py-6 text-center text-sm text-gray-500">
              Hech narsa topilmadi. Boshqa nom bilan yoki yilsiz qidirib ko&apos;ring.
            </p>
          )}
          <ul>
            {(items || []).map((it) => (
              <li key={it.id}>
                <button
                  type="button"
                  onClick={() => onPick(it)}
                  className="flex w-full items-start gap-3 rounded-xl p-2 text-left hover:bg-white/5"
                >
                  <span className="flex h-[69px] w-[46px] shrink-0 items-center justify-center overflow-hidden rounded-md bg-white/5 text-gray-600">
                    {it.poster_url ? (
                      // eslint-disable-next-line @next/next/no-img-element
                      <img src={it.poster_url} alt="" className="h-full w-full object-cover" loading="lazy" />
                    ) : (
                      <Film size={16} />
                    )}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-medium text-white">
                      {it.title || it.original_title} {it.year ? <span className="text-gray-500">({it.year})</span> : null}
                    </span>
                    {it.original_title && it.original_title !== it.title && <span className="block truncate text-xs text-gray-400">{it.original_title}</span>}
                    {it.overview && <span className="mt-0.5 line-clamp-2 text-[11px] leading-snug text-gray-500">{it.overview}</span>}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </div>
      </div>
    </div>
  );
}
