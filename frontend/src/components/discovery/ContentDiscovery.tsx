"use client";

import { ReactNode, useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { Dices, Flame, Layers, Play, RefreshCw, Sparkles, Star, type LucideIcon } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { getForYou, getRandomMovies, getTrendingMovies, Movie } from "@/lib/api";
import type { Series } from "@/lib/series-api";
import { getLocalizedDescription, getLocalizedTitle, localizeSingleGenre } from "@/lib/localization";
import { normalizeMediaUrl } from "@/lib/image-utils";
import MediaImage from "@/components/ui/MediaImage";
import MovieCarousel from "@/components/MovieCarousel";
import SeriesCarousel from "@/components/SeriesCarousel";

/**
 * Discovery block shown under movie, series and episode pages:
 *  1. O'xshash — content-similar titles (passed in from the server page so
 *     they are part of the SSR HTML for SEO).
 *  2. Siz uchun tavsiya — personal picks for signed-in users, trending
 *     otherwise.
 *  3. Tasodifiy kinolar — a spotlight + row of random playable movies that
 *     can be re-shuffled.
 */
export default function ContentDiscovery({
  excludeIds = [],
  similarMovies = [],
  similarSeries = [],
  similarTitle = "O'xshash kinolar",
}: {
  /** Ids not to show in the personal / random rows (the current title). */
  excludeIds?: string[];
  similarMovies?: Movie[];
  similarSeries?: Series[];
  similarTitle?: string;
}) {
  return (
    <div className="space-y-10 sm:space-y-12">
      {(similarMovies.length > 0 || similarSeries.length > 0) && (
        <DiscoverySection icon={Layers} tone="from-sky-500 to-indigo-600" title={similarTitle} subtitle="Janri, davlati va yiliga ko'ra tanlangan">
          {similarSeries.length > 0 ? <SeriesCarousel series={similarSeries} /> : <MovieCarousel movies={similarMovies} />}
        </DiscoverySection>
      )}
      <RecommendedRow excludeIds={excludeIds} />
      <RandomRow excludeIds={excludeIds} />
    </div>
  );
}

export function DiscoverySection({
  icon: Icon,
  tone,
  title,
  subtitle,
  action,
  children,
}: {
  icon: LucideIcon;
  tone: string;
  title: string;
  subtitle?: ReactNode;
  action?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section>
      <header className="mb-4 flex items-end justify-between gap-3 sm:mb-5">
        <div className="flex min-w-0 items-center gap-3">
          <span className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-2xl bg-gradient-to-br ${tone} shadow-lg shadow-black/30 sm:h-11 sm:w-11`}>
            <Icon size={20} className="text-white" />
          </span>
          <div className="min-w-0">
            <h2 className="truncate font-display text-xl tracking-wide text-white sm:text-2xl">{title}</h2>
            {subtitle && <p className="truncate text-xs text-gray-500 sm:text-sm">{subtitle}</p>}
          </div>
        </div>
        {action}
      </header>
      {children}
    </section>
  );
}

function RowSkeleton() {
  return (
    <div className="flex gap-3 overflow-hidden sm:gap-4">
      {Array.from({ length: 6 }).map((_, i) => (
        <div key={i} className="aspect-[2/3] w-[140px] shrink-0 animate-pulse rounded-2xl bg-white/[0.05] sm:w-[180px]" />
      ))}
    </div>
  );
}

function RecommendedRow({ excludeIds }: { excludeIds: string[] }) {
  const { token, isLoading } = useAuth();
  const [movies, setMovies] = useState<Movie[] | null>(null);
  const [personal, setPersonal] = useState(false);
  const excludeKey = excludeIds.join(",");

  useEffect(() => {
    if (isLoading) return;
    let alive = true;
    const skip = new Set(excludeKey.split(","));
    (async () => {
      let list: Movie[] = [];
      let isPersonal = false;
      if (token) {
        try {
          list = (await getForYou(token, 18)).data;
          isPersonal = list.length > 0;
        } catch {
          // fall through to trending
        }
      }
      if (!list.length) {
        try {
          list = await getTrendingMovies("7d", 18);
        } catch {
          list = [];
        }
      }
      if (!alive) return;
      setPersonal(isPersonal);
      setMovies(list.filter((m) => !skip.has(m.id)));
    })();
    return () => {
      alive = false;
    };
  }, [token, isLoading, excludeKey]);

  if (movies && movies.length === 0) return null;
  return (
    <DiscoverySection
      icon={personal ? Sparkles : Flame}
      tone={personal ? "from-fuchsia-500 to-purple-700" : "from-orange-500 to-rose-600"}
      title={personal ? "Siz uchun tavsiya" : "Tavsiya qilamiz"}
      subtitle={personal ? "Ko'rgan kinolaringizga asoslangan" : "Shu hafta eng ko'p ko'rilganlar"}
    >
      {movies ? <MovieCarousel movies={movies} /> : <RowSkeleton />}
    </DiscoverySection>
  );
}

function RandomRow({ excludeIds }: { excludeIds: string[] }) {
  const [movies, setMovies] = useState<Movie[] | null>(null);
  const [loading, setLoading] = useState(false);
  const seen = useRef<string[]>([...excludeIds]);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      let list = await getRandomMovies(13, seen.current);
      // Ran out of unseen titles — start over.
      if (list.length < 4 && seen.current.length > excludeIds.length) {
        seen.current = [...excludeIds];
        list = await getRandomMovies(13, seen.current);
      }
      seen.current = [...seen.current, ...list.map((m) => m.id)].slice(-50);
      setMovies(list);
    } catch {
      setMovies((prev) => prev ?? []);
    } finally {
      setLoading(false);
    }
    // excludeIds only seeds the ref; it doesn't need to re-create the loader.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  if (movies && movies.length === 0) return null;
  const [spot, ...rest] = movies || [];

  return (
    <DiscoverySection
      icon={Dices}
      tone="from-emerald-500 to-teal-700"
      title="Tasodifiy kinolar"
      subtitle="Nima ko'rishni bilmayapsizmi? Omadingizni sinang"
      action={
        <button
          type="button"
          onClick={() => void load()}
          disabled={loading}
          className="inline-flex shrink-0 items-center gap-1.5 rounded-xl border border-white/10 bg-white/[0.04] px-3 py-2 text-xs font-medium text-white transition hover:border-emerald-500/50 hover:bg-emerald-500/10 disabled:opacity-60 sm:text-sm"
        >
          <RefreshCw size={14} className={loading ? "animate-spin" : ""} />
          Aralashtirish
        </button>
      }
    >
      {!movies ? (
        <div className="space-y-4">
          <div className="h-56 animate-pulse rounded-3xl bg-white/[0.05] sm:h-64" />
          <RowSkeleton />
        </div>
      ) : (
        <div className={`space-y-4 transition-opacity ${loading ? "opacity-60" : ""}`}>
          {spot && <Spotlight movie={spot} />}
          {rest.length > 0 && <MovieCarousel movies={rest} />}
        </div>
      )}
    </DiscoverySection>
  );
}

function Spotlight({ movie }: { movie: Movie }) {
  const title = getLocalizedTitle(movie);
  const description = getLocalizedDescription(movie);
  const genres = (movie.genre || []).slice(0, 3).map((g) => localizeSingleGenre(g));
  const rating = movie.rating_avg || 0;
  const href = `/movies/${movie.slug}`;
  return (
    <div className="group relative overflow-hidden rounded-3xl border border-white/10 bg-[#101018]">
      <MediaImage
        src={normalizeMediaUrl(movie.backdrop_url || movie.poster_url)}
        alt=""
        className="absolute inset-0 h-full w-full object-cover opacity-40 transition duration-700 group-hover:scale-105"
      />
      <div className="absolute inset-0 bg-gradient-to-r from-[#0b0b12] via-[#0b0b12]/85 to-transparent" />
      <div className="relative flex gap-4 p-4 sm:gap-6 sm:p-6">
        <Link href={href} className="w-24 shrink-0 overflow-hidden rounded-2xl border border-white/10 shadow-2xl sm:w-32">
          <MediaImage src={normalizeMediaUrl(movie.poster_url)} alt={title} className="aspect-[2/3] h-full w-full object-cover" />
        </Link>
        <div className="flex min-w-0 flex-1 flex-col">
          <span className="mb-1.5 inline-flex w-fit items-center gap-1 rounded-full bg-emerald-500/15 px-2.5 py-0.5 text-[11px] font-semibold text-emerald-300">
            <Dices size={12} /> Tasodifiy tanlov
          </span>
          <Link href={href} className="line-clamp-2 font-display text-2xl leading-tight tracking-wide text-white hover:text-orange-300 sm:text-3xl">
            {title}
          </Link>
          <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-gray-400">
            {movie.year > 0 && <span>{movie.year}</span>}
            {rating > 0 && (
              <span className="inline-flex items-center gap-1 text-amber-300">
                <Star size={12} className="fill-amber-300" /> {rating.toFixed(1)}
              </span>
            )}
            {genres.map((g) => (
              <span key={g} className="rounded-full bg-white/[0.06] px-2 py-0.5">
                {g}
              </span>
            ))}
          </div>
          {description && <p className="mt-2 line-clamp-2 max-w-2xl text-sm text-gray-300 sm:line-clamp-3">{description}</p>}
          <div className="mt-auto pt-3">
            <Link
              href={`${href}?play=1`}
              className="inline-flex items-center gap-2 rounded-xl bg-orange-500 px-4 py-2 text-sm font-semibold text-white shadow-lg shadow-orange-500/25 transition hover:bg-orange-400"
            >
              <Play size={15} className="fill-white" /> Tomosha qilish
            </Link>
          </div>
        </div>
      </div>
    </div>
  );
}

