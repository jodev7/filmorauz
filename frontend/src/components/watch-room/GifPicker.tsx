"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Search, X, Loader2 } from "lucide-react";

// Instagram-style GIF picker. Shows trending on open and searches as the
// user types (debounced). Selecting a GIF hands its URL back to the parent,
// which sends it as a kind:"gif" chat message.
//
// Goes through OUR backend proxy (/api/gifs/search) so the GIPHY API key
// stays server-side and is never shipped to the browser. GIPHY's terms still
// require the "Powered by GIPHY" attribution rendered below the grid.

const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080/api";

type GifItem = {
  id: string;
  title?: string;
  preview: string; // small thumb for the grid
  url: string; // full gif sent into chat
};

export default function GifPicker({
  onSelect,
  onClose,
}: {
  onSelect: (gifUrl: string) => void;
  onClose: () => void;
}) {
  const [query, setQuery] = useState("");
  const [gifs, setGifs] = useState<GifItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const fetchGifs = useCallback(async (q: string) => {
    setLoading(true);
    setError(null);
    try {
      const params = new URLSearchParams({ limit: "24" });
      if (q.trim()) params.set("q", q.trim());
      // Plain (unauthenticated) fetch so the backend response cache can serve
      // repeat searches — GIF results aren't user-specific.
      const res = await fetch(`${API_BASE}/gifs/search?${params.toString()}`);
      if (res.status === 503) {
        setError("GIF xizmati sozlanmagan.");
        setGifs([]);
        return;
      }
      if (!res.ok) throw new Error(String(res.status));
      const json = await res.json();
      setGifs(Array.isArray(json.gifs) ? json.gifs : []);
    } catch {
      setError("GIF'larni yuklab bo'lmadi.");
      setGifs([]);
    } finally {
      setLoading(false);
    }
  }, []);

  // Initial trending load.
  useEffect(() => {
    fetchGifs("");
  }, [fetchGifs]);

  // Debounced search on query change.
  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => fetchGifs(query), 350);
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
  }, [query, fetchGifs]);

  return (
    <div className="absolute bottom-12 left-2 right-2 z-30 bg-brand-dark border border-white/10 rounded-xl shadow-2xl flex flex-col max-h-80 overflow-hidden">
      <div className="flex items-center gap-2 p-2 border-b border-white/10">
        <Search className="w-4 h-4 text-gray-400 shrink-0" />
        <input
          autoFocus
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="GIF qidirish…"
          className="flex-1 min-w-0 bg-transparent text-sm focus:outline-none"
        />
        <button onClick={onClose} className="p-1 text-gray-400 hover:text-white shrink-0">
          <X className="w-4 h-4" />
        </button>
      </div>

      <div className="flex-1 overflow-y-auto p-2">
        {loading && (
          <div className="flex items-center justify-center py-8">
            <Loader2 className="w-6 h-6 animate-spin text-brand-red" />
          </div>
        )}
        {!loading && error && <p className="text-xs text-gray-400 text-center py-6">{error}</p>}
        {!loading && !error && gifs.length === 0 && (
          <p className="text-xs text-gray-500 text-center py-6">GIF topilmadi.</p>
        )}
        {!loading && !error && gifs.length > 0 && (
          // Masonry-ish two-column grid.
          <div className="columns-2 gap-2 [&>*]:mb-2">
            {gifs.map((g) => (
              <button
                key={g.id}
                onClick={() => onSelect(g.url)}
                className="block w-full overflow-hidden rounded-lg hover:ring-2 hover:ring-brand-red transition"
              >
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img
                  src={g.preview}
                  alt={g.title || "GIF"}
                  loading="lazy"
                  className="w-full h-auto glass-card"
                />
              </button>
            ))}
          </div>
        )}
      </div>

      <div className="px-2 py-1 border-t border-white/10 text-[10px] text-gray-500 text-right">
        Powered by GIPHY
      </div>
    </div>
  );
}
