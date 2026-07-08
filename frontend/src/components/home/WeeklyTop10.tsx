import Link from "next/link";
import { Trophy, Medal, Eye } from "lucide-react";
import { Movie } from "@/lib/api";
import OptimizedImage from "@/components/OptimizedImage";
import SectionHeader from "./SectionHeader";
import { getLocalizedTitle } from "@/lib/localization";

interface WeeklyTop10Props {
  movies: Movie[];
}

// Compact view-count label: 12345 → "12.3K", 1200000 → "1.2M".
function formatViews(n: number): string {
  if (!n || n < 0) return "0";
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1).replace(/\.0$/, "")}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1).replace(/\.0$/, "")}K`;
  return String(n);
}

// Per-place podium styling. Index 0 = winner (centre, tallest, gold trophy);
// 1 = silver, 2 = bronze. `order` places the winner in the middle.
const PLACES = [
  {
    order: "order-2",
    poster: "w-24 sm:w-36",
    pillar: "h-24 sm:h-28",
    ring: "ring-yellow-400/60",
    glow: "shadow-[0_12px_45px_rgba(234,179,8,0.35)]",
    coin: "bg-gradient-to-br from-amber-200 via-yellow-400 to-amber-600 text-amber-950",
    pillarTint: "from-yellow-400/25 to-yellow-400/5",
    num: "text-yellow-300",
    Icon: Trophy,
  },
  {
    order: "order-1",
    poster: "w-20 sm:w-28",
    pillar: "h-16 sm:h-20",
    ring: "ring-slate-300/50",
    glow: "shadow-[0_10px_35px_rgba(203,213,225,0.22)]",
    coin: "bg-gradient-to-br from-slate-100 via-gray-300 to-slate-400 text-slate-800",
    pillarTint: "from-slate-300/20 to-slate-300/5",
    num: "text-slate-200",
    Icon: Medal,
  },
  {
    order: "order-3",
    poster: "w-20 sm:w-28",
    pillar: "h-12 sm:h-14",
    ring: "ring-amber-600/50",
    glow: "shadow-[0_10px_35px_rgba(180,83,9,0.24)]",
    coin: "bg-gradient-to-br from-orange-300 via-amber-500 to-amber-700 text-amber-950",
    pillarTint: "from-amber-600/20 to-amber-600/5",
    num: "text-amber-400",
    Icon: Medal,
  },
];

// "Haftalik Top 10" — the week's most-watched titles by view count. Top three
// stand on a cinema-style award podium (trophy + medals); ranks 4–10 sit in a
// glass number rail below.
export default function WeeklyTop10({ movies }: WeeklyTop10Props) {
  if (!movies || movies.length === 0) return null;

  const podium = movies.slice(0, 3);
  const rest = movies.slice(3, 10);

  return (
    <section className="max-w-7xl mx-auto px-4 py-8">
      <SectionHeader title="Haftalik Top 10" icon={Trophy} accent="yellow" />

      {/* ── Award podium ──────────────────────────────────────────── */}
      <div className="relative overflow-hidden rounded-3xl glass p-4 pt-6 sm:p-8">
        {/* Spotlight beam */}
        <div
          aria-hidden="true"
          className="pointer-events-none absolute -top-16 left-1/2 h-[320px] w-[520px] -translate-x-1/2 rounded-full bg-gradient-to-b from-yellow-300/18 via-amber-400/8 to-transparent blur-3xl"
        />

        <div className="relative flex items-end justify-center gap-3 sm:gap-6">
          {podium.map((movie, i) => {
            const p = PLACES[i];
            const title = getLocalizedTitle(movie);
            const { Icon } = p;
            return (
              <Link
                key={movie.id}
                href={`/movies/${movie.slug}`}
                className={`group flex w-[30%] max-w-[190px] flex-col items-center ${p.order}`}
              >
                {/* Trophy / medal */}
                <span
                  className={`mb-2 flex h-10 w-10 items-center justify-center rounded-full ring-1 ring-white/40 ${p.coin} ${p.glow}`}
                >
                  <Icon size={20} strokeWidth={2.2} />
                </span>

                {/* Poster */}
                <div
                  className={`relative ${p.poster} aspect-[2/3] overflow-hidden rounded-xl bg-white/5 ring-2 ${p.ring} ${p.glow} transition-transform duration-300 group-hover:-translate-y-1`}
                >
                  <OptimizedImage
                    src={movie.poster_thumb_url || movie.poster_url}
                    alt={title}
                    aspectRatio="2/3"
                    className="transition-transform duration-500 group-hover:scale-105"
                  />
                </div>

                {/* Title + views */}
                <h3 className="mt-2 line-clamp-1 w-full text-center text-xs sm:text-sm font-semibold tracking-tight text-white">
                  {title}
                </h3>
                {(movie.views || 0) > 0 && (
                  <span className="mt-0.5 flex items-center gap-1 text-[10px] sm:text-xs text-gray-400">
                    <Eye size={11} aria-hidden="true" />
                    {formatViews(movie.views)}
                  </span>
                )}

                {/* Pedestal pillar with the rank numeral */}
                <div
                  className={`mt-2 flex ${p.pillar} w-full items-start justify-center rounded-t-xl border-x border-t border-white/10 bg-gradient-to-b ${p.pillarTint} pt-1.5`}
                >
                  <span className={`text-2xl sm:text-3xl font-black ${p.num}`}>
                    {i + 1}
                  </span>
                </div>
              </Link>
            );
          })}
        </div>
      </div>

      {/* ── Ranks 4–10 rail ───────────────────────────────────────── */}
      {rest.length > 0 && (
        <div className="mt-4 flex gap-3 overflow-x-auto scrollbar-hide pb-2">
          {rest.map((movie, i) => {
            const rank = i + 4;
            const title = getLocalizedTitle(movie);
            return (
              <Link
                key={movie.id}
                href={`/movies/${movie.slug}`}
                className="group glass-card glass-hover flex shrink-0 items-center gap-3 rounded-2xl p-2.5 pr-4 w-[230px] sm:w-[250px]"
              >
                <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-white/5 text-sm font-bold text-gray-300">
                  {rank}
                </span>
                <div className="relative h-14 w-10 shrink-0 overflow-hidden rounded-lg bg-white/5">
                  <OptimizedImage
                    src={movie.poster_thumb_url || movie.poster_url}
                    alt={title}
                    aspectRatio="2/3"
                    className="transition-transform duration-500 group-hover:scale-105"
                  />
                </div>
                <div className="min-w-0 flex-1">
                  <h3 className="line-clamp-2 text-sm font-medium leading-tight text-white group-hover:text-orange-400 transition-colors">
                    {title}
                  </h3>
                  {(movie.views || 0) > 0 && (
                    <span className="mt-0.5 flex items-center gap-1 text-xs text-gray-400">
                      <Eye size={11} aria-hidden="true" />
                      {formatViews(movie.views)}
                    </span>
                  )}
                </div>
              </Link>
            );
          })}
        </div>
      )}
    </section>
  );
}
