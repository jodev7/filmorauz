"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import { ArrowUpRight, Clock, Film, Loader2, Search, Tv, TrendingUp, X } from "lucide-react";
import { getTrendingMovies, Movie, searchMovies } from "@/lib/api";
import { getLocalizedTitle, localizeSingleGenre } from "@/lib/localization";
import { DEFAULT_POSTER_PLACEHOLDER } from "@/lib/image-utils";
import MediaImage from "@/components/ui/MediaImage";
import { OPEN_SEARCH_EVENT, pushRecentSearch, readRecentSearches, removeRecentSearch } from "@/lib/search-overlay";

const QUICK_GENRES = ["action", "comedy", "drama", "horror", "animation", "anime"];

type Result = Movie & { target_type?: string };

/**
 * Full-screen search: recent + trending when empty, instant grouped results
 * with keyboard navigation while typing. Opened via openSearch(), the
 * navbar/bottom-nav buttons, "/" or Ctrl/⌘+K (outside the admin panel).
 */
export default function SearchOverlay() {
  const router = useRouter();
  const pathname = usePathname();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Result[]>([]);
  const [loading, setLoading] = useState(false);
  const [searched, setSearched] = useState("");
  const [recent, setRecent] = useState<string[]>([]);
  const [trending, setTrending] = useState<Movie[] | null>(null);
  const [active, setActive] = useState(-1);
  const inputRef = useRef<HTMLInputElement>(null);
  const reqId = useRef(0);

  const close = useCallback(() => {
    setOpen(false);
    setActive(-1);
  }, []);

  // Open triggers.
  useEffect(() => {
    const onOpen = (e: Event) => {
      const q = (e as CustomEvent<{ query?: string }>).detail?.query ?? "";
      setQuery(q);
      setOpen(true);
    };
    const onKey = (e: KeyboardEvent) => {
      if (pathname?.startsWith("/admin")) return;
      const target = e.target as HTMLElement | null;
      const typing = !!target && (target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.isContentEditable);
      if ((e.key === "k" && (e.metaKey || e.ctrlKey)) || (e.key === "/" && !typing)) {
        e.preventDefault();
        setOpen(true);
      }
    };
    window.addEventListener(OPEN_SEARCH_EVENT, onOpen);
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener(OPEN_SEARCH_EVENT, onOpen);
      window.removeEventListener("keydown", onKey);
    };
  }, [pathname]);

  // Close on navigation.
  useEffect(() => {
    setOpen(false);
  }, [pathname]);

  useEffect(() => {
    if (!open) return;
    setRecent(readRecentSearches());
    const t = setTimeout(() => inputRef.current?.focus(), 30);
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    if (trending === null) getTrendingMovies("7d", 8).then(setTrending).catch(() => setTrending([]));
    return () => {
      clearTimeout(t);
      document.body.style.overflow = prevOverflow;
    };
  }, [open, trending]);

  // Debounced search (latest request wins).
  useEffect(() => {
    if (!open) return;
    const q = query.trim();
    if (q.length < 2) {
      setResults([]);
      setSearched("");
      setLoading(false);
      return;
    }
    setLoading(true);
    const id = ++reqId.current;
    const t = setTimeout(() => {
      searchMovies(q)
        .then((r) => {
          if (id !== reqId.current) return;
          setResults((r || []) as Result[]);
          setSearched(q);
          setActive(-1);
        })
        .catch(() => id === reqId.current && setResults([]))
        .finally(() => id === reqId.current && setLoading(false));
    }, 250);
    return () => clearTimeout(t);
  }, [query, open]);

  const movies = useMemo(() => results.filter((r) => r.target_type !== "series").slice(0, 8), [results]);
  const series = useMemo(() => results.filter((r) => r.target_type === "series").slice(0, 5), [results]);
  const flat = useMemo(() => [...movies, ...series], [movies, series]);

  const go = (href: string, remember?: string) => {
    if (remember) setRecent(pushRecentSearch(remember));
    close();
    router.push(href);
  };
  const hrefOf = (r: Result) => (r.target_type === "series" ? `/series/${r.slug}` : `/movies/${r.slug}`);
  const searchAll = (q: string) => go(`/movies?search=${encodeURIComponent(q.trim())}`, q);

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Escape") {
      e.preventDefault();
      close();
    } else if (e.key === "ArrowDown" && flat.length) {
      e.preventDefault();
      setActive((a) => (a + 1) % flat.length);
    } else if (e.key === "ArrowUp" && flat.length) {
      e.preventDefault();
      setActive((a) => (a <= 0 ? flat.length - 1 : a - 1));
    } else if (e.key === "Enter") {
      e.preventDefault();
      if (active >= 0 && flat[active]) go(hrefOf(flat[active]), query);
      else if (query.trim()) searchAll(query);
    }
  };

  useEffect(() => {
    if (active < 0) return;
    document.getElementById(`search-opt-${active}`)?.scrollIntoView({ block: "nearest" });
  }, [active]);

  if (!open) return null;

  const q = query.trim();
  const renderRow = (r: Result, index: number) => (
    <li key={`${r.target_type || "movie"}-${r.id}`} role="option" aria-selected={active === index} id={`search-opt-${index}`}>
      <button
        onClick={() => go(hrefOf(r), query)}
        onMouseEnter={() => setActive(index)}
        className={`flex w-full items-center gap-3 rounded-xl px-3 py-2 text-left transition-colors ${active === index ? "bg-white/10" : "hover:bg-white/5"}`}
      >
        <MediaImage src={r.poster_url} alt="" fallbackSrc={DEFAULT_POSTER_PLACEHOLDER} className="h-14 w-10 shrink-0 rounded-md object-cover" />
        <span className="min-w-0 flex-1">
          <span className="line-clamp-1 text-sm font-medium text-white">{getLocalizedTitle(r)}</span>
          <span className="mt-0.5 flex items-center gap-2 text-xs text-gray-400">
            {r.year ? <span>{r.year}</span> : null}
            {r.genre?.[0] && <span>{localizeSingleGenre(r.genre[0])}</span>}
            {r.code && <span className="font-mono text-gray-500">#{r.code}</span>}
          </span>
        </span>
        {r.quality && <span className="shrink-0 rounded border border-orange-500/30 bg-orange-500/10 px-1.5 text-[10px] font-bold text-orange-400">{r.quality}</span>}
      </button>
    </li>
  );

  return (
    <div className="fixed inset-0 z-[90] flex justify-center bg-black/70 backdrop-blur-sm sm:items-start sm:pt-[10vh]" onMouseDown={close}>
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Qidiruv"
        onMouseDown={(e) => e.stopPropagation()}
        className="flex h-full w-full flex-col overflow-hidden bg-[#0e0e15] sm:h-auto sm:max-h-[75vh] sm:max-w-2xl sm:rounded-2xl sm:border sm:border-white/10 sm:shadow-2xl"
      >
        <div className="flex items-center gap-3 border-b border-white/10 px-4 pb-3 pt-[max(0.75rem,env(safe-area-inset-top))] sm:py-3">
          {loading ? <Loader2 size={20} className="shrink-0 animate-spin text-orange-400" /> : <Search size={20} className="shrink-0 text-gray-400" />}
          <input
            ref={inputRef}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={onKeyDown}
            placeholder="Kino, serial, aktyor yoki kod…"
            className="min-w-0 flex-1 bg-transparent py-2 text-base text-white placeholder-gray-500 focus:outline-none"
            role="combobox"
            aria-expanded={flat.length > 0}
            aria-controls="search-results"
            aria-activedescendant={active >= 0 ? `search-opt-${active}` : undefined}
            enterKeyHint="search"
            autoComplete="off"
          />
          {query && (
            <button onClick={() => { setQuery(""); inputRef.current?.focus(); }} className="rounded-full p-1 text-gray-400 hover:text-white" aria-label="Tozalash">
              <X size={16} />
            </button>
          )}
          <button onClick={close} className="shrink-0 rounded-lg px-2 py-1 text-sm text-gray-300 hover:bg-white/5 sm:hidden">
            Yopish
          </button>
          <kbd className="hidden shrink-0 rounded border border-white/15 px-1.5 text-[11px] text-gray-500 sm:block">Esc</kbd>
        </div>

        <div className="flex-1 overflow-y-auto overscroll-contain px-2 py-3" id="search-results" role="listbox">
          {q.length < 2 ? (
            <div className="space-y-6 px-2">
              {recent.length > 0 && (
                <section>
                  <h3 className="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-gray-500">
                    <Clock size={12} /> So&apos;nggi qidiruvlar
                  </h3>
                  <ul>
                    {recent.map((r) => (
                      <li key={r} className="group flex items-center">
                        <button onClick={() => setQuery(r)} className="flex-1 truncate rounded-lg px-2 py-2 text-left text-sm text-gray-200 hover:bg-white/5">
                          {r}
                        </button>
                        <button onClick={() => setRecent(removeRecentSearch(r))} className="rounded p-1.5 text-gray-600 hover:text-white" aria-label={`${r} ni o'chirish`}>
                          <X size={14} />
                        </button>
                      </li>
                    ))}
                  </ul>
                </section>
              )}
              <section>
                <h3 className="mb-2 text-xs font-semibold uppercase tracking-wider text-gray-500">Janrlar</h3>
                <div className="flex flex-wrap gap-2">
                  {QUICK_GENRES.map((g) => (
                    <button
                      key={g}
                      onClick={() => go(`/movies?genre=${g}`)}
                      className="rounded-full border border-white/10 px-3 py-1.5 text-sm text-gray-200 hover:border-orange-500/50 hover:text-white"
                    >
                      {localizeSingleGenre(g)}
                    </button>
                  ))}
                </div>
              </section>
              {trending && trending.length > 0 && (
                <section>
                  <h3 className="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-gray-500">
                    <TrendingUp size={12} /> Hozir ko&apos;p qidirilmoqda
                  </h3>
                  <ul className="grid grid-cols-1 gap-1 sm:grid-cols-2">
                    {trending.map((m) => (
                      <li key={m.id}>
                        <button onClick={() => go(`/movies/${m.slug}`)} className="flex w-full items-center gap-3 rounded-xl px-2 py-1.5 text-left hover:bg-white/5">
                          <MediaImage src={m.poster_url} alt="" fallbackSrc={DEFAULT_POSTER_PLACEHOLDER} className="h-12 w-8 shrink-0 rounded object-cover" />
                          <span className="line-clamp-1 text-sm text-gray-200">{getLocalizedTitle(m)}</span>
                        </button>
                      </li>
                    ))}
                  </ul>
                </section>
              )}
            </div>
          ) : !loading && searched === q && flat.length === 0 ? (
            <div className="px-4 py-12 text-center">
              <p className="text-gray-300">&quot;{q}&quot; bo&apos;yicha hech narsa topilmadi</p>
              <p className="mt-1 text-sm text-gray-500">Boshqacha yozib ko&apos;ring — masalan lotin yoki kirill harflarida, yoki kino kodini kiriting.</p>
            </div>
          ) : (
            <div className="space-y-4">
              {movies.length > 0 && (
                <section>
                  <h3 className="mb-1 flex items-center gap-1.5 px-3 text-xs font-semibold uppercase tracking-wider text-gray-500">
                    <Film size={12} /> Kinolar
                  </h3>
                  <ul>{movies.map((r, i) => renderRow(r, i))}</ul>
                </section>
              )}
              {series.length > 0 && (
                <section>
                  <h3 className="mb-1 flex items-center gap-1.5 px-3 text-xs font-semibold uppercase tracking-wider text-gray-500">
                    <Tv size={12} /> Seriallar
                  </h3>
                  <ul>{series.map((r, i) => renderRow(r, movies.length + i))}</ul>
                </section>
              )}
            </div>
          )}
        </div>

        {q.length >= 2 && (
          <button
            onClick={() => searchAll(q)}
            className="flex items-center justify-between border-t border-white/10 px-5 py-3 text-sm text-orange-400 hover:bg-white/5"
          >
            <span>
              &quot;{q}&quot; bo&apos;yicha barcha natijalar{results.length ? ` (${results.length})` : ""}
            </span>
            <ArrowUpRight size={16} />
          </button>
        )}
      </div>
    </div>
  );
}
