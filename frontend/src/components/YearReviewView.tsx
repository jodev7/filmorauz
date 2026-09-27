"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { Clock, Film, Tv, CalendarDays, Moon, Sun, Share2, Download, Star, MessageSquare, PenLine, Loader2, Sparkles } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { getYearReview, YearReview } from "@/lib/api";
import { localizeSingleGenre } from "@/lib/localization";
import OptimizedImage from "@/components/OptimizedImage";
import TelegramLoginModal from "@/components/TelegramLoginModal";

const MONTHS = ["Yanvar", "Fevral", "Mart", "Aprel", "May", "Iyun", "Iyul", "Avgust", "Sentabr", "Oktabr", "Noyabr", "Dekabr"];
const MONTHS_SHORT = ["Yan", "Fev", "Mar", "Apr", "May", "Iyn", "Iyl", "Avg", "Sen", "Okt", "Noy", "Dek"];
const WEEKDAYS = ["yakshanba", "dushanba", "seshanba", "chorshanba", "payshanba", "juma", "shanba"];

// 12 345 (whole numbers only; thin grouping like the rest of the site).
const fmt = (n: number) => String(Math.round(n)).replace(/\B(?=(\d{3})+(?!\d))/g, " ");

function hoursLabel(minutes: number): { value: string; unit: string } {
  if (minutes < 60) return { value: String(minutes), unit: "daqiqa" };
  const h = minutes / 60;
  return { value: h >= 100 ? fmt(Math.round(h)) : h.toFixed(1).replace(/\.0$/, ""), unit: "soat" };
}

// Fun equivalent for the hero number.
function equivalent(minutes: number): string | null {
  if (minutes < 130) return null;
  if (minutes >= 60 * 24) return `Bu — ${(minutes / 60 / 24).toFixed(1).replace(/\.0$/, "")} kun to'xtovsiz tomosha degani`;
  // Afrosiyob, Toshkent → Samarqand ≈ 2 soat 10 daqiqa
  return `Bu — Toshkentdan Samarqandga "Afrosiyob"da ${Math.round(minutes / 130)} marta borgandek`;
}

function MonthChart({ months, top }: { months: number[]; top: number }) {
  const [hover, setHover] = useState<number | null>(null);
  const max = Math.max(1, ...months);
  return (
    <figure>
      <div className="relative flex h-40 items-end gap-[2px]" role="img" aria-label="Oylar bo'yicha tomosha qilingan kontent soni">
        {months.map((n, i) => {
          const h = n === 0 ? 0 : Math.max(4, (n / max) * 100);
          const isTop = i + 1 === top;
          return (
            <div
              key={i}
              className="group relative flex h-full flex-1 items-end"
              onMouseEnter={() => setHover(i)}
              onMouseLeave={() => setHover(null)}
              onFocus={() => setHover(i)}
              onBlur={() => setHover(null)}
              tabIndex={0}
              aria-label={`${MONTHS[i]}: ${n} ta`}
            >
              <div
                className={`w-full rounded-t-[4px] transition-opacity ${isTop ? "bg-orange-500" : "bg-orange-500/45"} ${hover !== null && hover !== i ? "opacity-60" : ""}`}
                style={{ height: `${h}%` }}
              />
              {isTop && n > 0 && hover === null && (
                <span className="absolute left-1/2 -translate-x-1/2 text-xs font-medium text-white" style={{ bottom: `calc(${h}% + 4px)` }}>
                  {n}
                </span>
              )}
              {hover === i && (
                <span
                  className="pointer-events-none absolute left-1/2 z-10 -translate-x-1/2 whitespace-nowrap rounded-md border border-white/10 bg-[#1b1b26] px-2 py-1 text-xs text-white shadow-lg"
                  style={{ bottom: `calc(${h}% + 6px)` }}
                >
                  {MONTHS[i]}: <b>{n}</b> ta
                </span>
              )}
            </div>
          );
        })}
      </div>
      <div className="mt-2 flex gap-[2px] border-t border-white/10 pt-2">
        {MONTHS_SHORT.map((m) => (
          <span key={m} className="flex-1 text-center text-[10px] text-gray-500">
            {m}
          </span>
        ))}
      </div>
      <figcaption className="sr-only">
        <table>
          <tbody>
            {months.map((n, i) => (
              <tr key={i}>
                <th>{MONTHS[i]}</th>
                <td>{n}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </figcaption>
    </figure>
  );
}

function Stat({ icon, value, label }: { icon: React.ReactNode; value: string; label: string }) {
  return (
    <div className="glass-card rounded-2xl border border-white/10 p-4">
      <div className="mb-2 text-orange-400">{icon}</div>
      <p className="text-2xl font-bold text-white sm:text-3xl">{value}</p>
      <p className="mt-0.5 text-xs text-gray-400">{label}</p>
    </div>
  );
}

// Draws a 1080×1350 share card on a canvas (no server needed).
async function renderShareCard(r: YearReview, name: string): Promise<Blob | null> {
  const c = document.createElement("canvas");
  c.width = 1080;
  c.height = 1350;
  const g = c.getContext("2d");
  if (!g) return null;
  const grad = g.createLinearGradient(0, 0, 1080, 1350);
  grad.addColorStop(0, "#1a0f0a");
  grad.addColorStop(0.55, "#0c0c14");
  grad.addColorStop(1, "#1d1030");
  g.fillStyle = grad;
  g.fillRect(0, 0, 1080, 1350);
  g.fillStyle = "rgba(249,115,22,0.18)";
  g.beginPath();
  g.arc(900, 180, 320, 0, Math.PI * 2);
  g.fill();

  const font = (w: number, size: number) => `${w} ${size}px Inter, system-ui, sans-serif`;
  g.fillStyle = "#f97316";
  g.font = font(700, 44);
  g.fillText("FilmoraUz", 80, 120);
  g.fillStyle = "#ffffff";
  g.font = font(800, 92);
  g.fillText(`${r.year}-yil`, 80, 260);
  g.font = font(500, 40);
  g.fillStyle = "#d1d5db";
  g.fillText(`${name}ning kino yili`, 80, 320);

  const hl = hoursLabel(r.total_minutes);
  g.fillStyle = "#ffffff";
  g.font = font(800, 200);
  g.fillText(hl.value, 80, 560);
  g.font = font(600, 56);
  g.fillStyle = "#f97316";
  g.fillText(`${hl.unit} tomosha`, 80, 640);

  const rows: [string, string][] = [
    [fmt(r.movies_watched), "kino"],
    [fmt(r.episodes_watched), "serial qismi"],
    [fmt(r.active_days), "faol kun"],
  ];
  rows.forEach(([v, l], i) => {
    const x = 80 + i * 320;
    g.fillStyle = "#ffffff";
    g.font = font(800, 72);
    g.fillText(v, x, 800);
    g.fillStyle = "#9ca3af";
    g.font = font(500, 34);
    g.fillText(l, x, 850);
  });

  if (r.top_genres[0]) {
    g.fillStyle = "#9ca3af";
    g.font = font(500, 34);
    g.fillText("Sevimli janr", 80, 960);
    g.fillStyle = "#ffffff";
    g.font = font(800, 64);
    g.fillText(localizeSingleGenre(r.top_genres[0].key), 80, 1030);
  }
  if (r.top_titles[0]) {
    g.fillStyle = "#9ca3af";
    g.font = font(500, 34);
    g.fillText("Eng ko'p ko'rilgan", 80, 1120);
    g.fillStyle = "#ffffff";
    g.font = font(700, 48);
    const t = r.top_titles[0].title;
    g.fillText(t.length > 30 ? t.slice(0, 29) + "…" : t, 80, 1180);
  }
  g.fillStyle = "#6b7280";
  g.font = font(500, 30);
  g.fillText("filmorauz.net/year", 80, 1290);
  return new Promise((res) => c.toBlob((b) => res(b), "image/png"));
}

export default function YearReviewView({ initialYear }: { initialYear?: number }) {
  const { token, user, isLoading } = useAuth();
  const thisYear = new Date().getFullYear();
  const [year, setYear] = useState(initialYear && initialYear <= thisYear && initialYear >= 2023 ? initialYear : thisYear);
  const [data, setData] = useState<YearReview | null>(null);
  const [error, setError] = useState(false);
  const [loginOpen, setLoginOpen] = useState(false);
  const [sharing, setSharing] = useState(false);

  useEffect(() => {
    if (isLoading || !token) return;
    let active = true;
    setData(null);
    setError(false);
    getYearReview(token, year)
      .then((d) => active && setData(d))
      .catch(() => active && setError(true));
    return () => {
      active = false;
    };
  }, [token, isLoading, year]);

  const name = user?.display_name || user?.first_name || "Mening";
  const hl = useMemo(() => (data ? hoursLabel(data.total_minutes) : null), [data]);

  const share = async () => {
    if (!data) return;
    setSharing(true);
    try {
      const blob = await renderShareCard(data, name);
      if (!blob) return;
      const file = new File([blob], `filmorauz-${data.year}.png`, { type: "image/png" });
      if (navigator.canShare?.({ files: [file] })) {
        await navigator.share({ files: [file], title: `${data.year}-yil yakunim — FilmoraUz` });
      } else {
        const a = document.createElement("a");
        a.href = URL.createObjectURL(blob);
        a.download = file.name;
        a.click();
        setTimeout(() => URL.revokeObjectURL(a.href), 1000);
      }
    } catch {
      // cancelled
    } finally {
      setSharing(false);
    }
  };

  if (!isLoading && !token) {
    return (
      <div className="mx-auto max-w-lg px-4 py-20 text-center">
        <Sparkles className="mx-auto mb-4 text-orange-400" size={40} />
        <h1 className="font-display text-4xl text-white">Yil yakuni</h1>
        <p className="mt-3 text-gray-400">Bu yil nimalarni tomosha qilganingizni ko&apos;rish uchun tizimga kiring.</p>
        <button onClick={() => setLoginOpen(true)} className="mt-6 rounded-xl bg-orange-500 px-6 py-3 font-semibold text-white hover:bg-orange-600">
          Telegram orqali kirish
        </button>
        <TelegramLoginModal isOpen={loginOpen} onClose={() => setLoginOpen(false)} />
      </div>
    );
  }

  const years = [thisYear, thisYear - 1, thisYear - 2].filter((y) => y >= 2023);

  return (
    <div className="mx-auto max-w-4xl px-4 pb-16">
      <div className="mb-6 flex flex-wrap items-center gap-2">
        {years.map((y) => (
          <button
            key={y}
            onClick={() => setYear(y)}
            className={`rounded-full px-4 py-1.5 text-sm ${y === year ? "bg-orange-500 text-white" : "border border-white/10 text-gray-300 hover:border-orange-500/50"}`}
          >
            {y}
          </button>
        ))}
      </div>

      {error ? (
        <p className="py-20 text-center text-gray-400">Ma&apos;lumotni yuklab bo&apos;lmadi. Keyinroq qayta urinib ko&apos;ring.</p>
      ) : !data || !hl ? (
        <div className="space-y-4">
          <div className="h-56 animate-pulse rounded-3xl bg-white/5" />
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            {[0, 1, 2, 3].map((i) => (
              <div key={i} className="h-28 animate-pulse rounded-2xl bg-white/5" />
            ))}
          </div>
          <div className="flex justify-center py-4 text-gray-500">
            <Loader2 className="animate-spin" />
          </div>
        </div>
      ) : data.total_minutes === 0 && data.movies_watched + data.episodes_watched === 0 ? (
        <div className="glass-card rounded-3xl border border-white/10 px-6 py-16 text-center">
          <h1 className="font-display text-4xl text-white">{data.year}-yil</h1>
          <p className="mt-3 text-gray-400">Bu yil hali hech narsa tomosha qilmadingiz. Boshlash uchun ajoyib vaqt!</p>
          <Link href="/movies" className="mt-6 inline-block rounded-xl bg-orange-500 px-6 py-3 font-semibold text-white hover:bg-orange-600">
            Kinolarni ko&apos;rish
          </Link>
        </div>
      ) : (
        <div className="space-y-6">
          {/* Hero */}
          <section className="relative overflow-hidden rounded-3xl border border-orange-500/20 bg-gradient-to-br from-orange-600/25 via-[#12121a] to-violet-700/25 p-6 sm:p-10">
            <p className="text-sm text-orange-300">
              {data.year}-yil {data.complete ? "yakuni" : "(hozircha)"}
            </p>
            <h1 className="mt-1 font-display text-4xl tracking-wide text-white sm:text-5xl">{name}, bu sizning kino yilingiz</h1>
            <div className="mt-8 flex items-end gap-3">
              <span className="font-display text-7xl leading-none text-white sm:text-8xl">{hl.value}</span>
              <span className="pb-2 text-xl text-orange-300">{hl.unit} tomosha</span>
            </div>
            {equivalent(data.total_minutes) && <p className="mt-3 text-sm text-gray-300">{equivalent(data.total_minutes)}</p>}
            <button
              onClick={share}
              disabled={sharing}
              className="mt-6 inline-flex items-center gap-2 rounded-xl bg-white px-5 py-2.5 text-sm font-semibold text-black hover:bg-gray-200 disabled:opacity-60"
            >
              {sharing ? <Loader2 size={16} className="animate-spin" /> : typeof navigator !== "undefined" && "share" in navigator ? <Share2 size={16} /> : <Download size={16} />}
              Rasm sifatida ulashish
            </button>
          </section>

          {/* Stats */}
          <section className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            <Stat icon={<Film size={20} />} value={fmt(data.movies_watched)} label="kino" />
            <Stat icon={<Tv size={20} />} value={fmt(data.episodes_watched)} label={`serial qismi · ${data.series_watched} ta serial`} />
            <Stat icon={<CalendarDays size={20} />} value={fmt(data.active_days)} label="faol kun" />
            <Stat icon={<Clock size={20} />} value={fmt(data.completed)} label="oxirigacha ko'rilgan" />
          </section>

          <div className="grid gap-6 md:grid-cols-2">
            {/* Genres */}
            {data.top_genres.length > 0 && (
              <section className="glass-card rounded-2xl border border-white/10 p-5">
                <h2 className="text-sm text-gray-400">Sevimli janringiz</h2>
                <p className="mb-4 font-display text-3xl tracking-wide text-white">{localizeSingleGenre(data.top_genres[0].key)}</p>
                <ul className="space-y-2.5">
                  {data.top_genres.map((g) => {
                    const pct = (g.count / data.top_genres[0].count) * 100;
                    return (
                      <li key={g.key}>
                        <div className="mb-1 flex justify-between text-sm">
                          <span className="text-gray-200">{localizeSingleGenre(g.key)}</span>
                          <span className="text-gray-400">{g.count}</span>
                        </div>
                        <div className="h-2 overflow-hidden rounded-full bg-white/5">
                          <div className="h-full rounded-full bg-orange-500" style={{ width: `${pct}%` }} />
                        </div>
                      </li>
                    );
                  })}
                </ul>
              </section>
            )}

            {/* Months */}
            <section className="glass-card rounded-2xl border border-white/10 p-5">
              <h2 className="text-sm text-gray-400">Eng faol oyingiz</h2>
              <p className="mb-4 font-display text-3xl tracking-wide text-white">{data.top_month ? MONTHS[data.top_month - 1] : "—"}</p>
              <MonthChart months={data.months} top={data.top_month} />
              {data.top_weekday >= 0 && (
                <p className="mt-4 flex items-center gap-2 text-sm text-gray-300">
                  {data.night_owl ? <Moon size={15} className="text-violet-300" /> : <Sun size={15} className="text-yellow-300" />}
                  Ko&apos;proq {WEEKDAYS[data.top_weekday]} kunlari{data.night_owl ? ", asosan tunda" : ""} tomosha qilasiz
                  {data.night_owl ? " — haqiqiy tungi tomoshabin 🦉" : ""}
                </p>
              )}
            </section>
          </div>

          {/* Top titles */}
          {data.top_titles.length > 0 && (
            <section className="glass-card rounded-2xl border border-white/10 p-5">
              <h2 className="mb-4 text-sm text-gray-400">Eng ko&apos;p vaqt bergan kinolaringiz</h2>
              <ol className="grid grid-cols-2 gap-3 sm:grid-cols-5">
                {data.top_titles.map((t, i) => (
                  <li key={t.target_id}>
                    <Link href={t.target_type === "series" ? `/series/${t.slug}` : `/movies/${t.slug}`} className="group block">
                      <div className="relative overflow-hidden rounded-xl border border-white/10">
                        <OptimizedImage src={t.poster_url} alt={t.title} aspectRatio="2/3" />
                        <span className="absolute left-2 top-2 flex h-7 w-7 items-center justify-center rounded-full bg-orange-500 text-sm font-bold text-white">{i + 1}</span>
                      </div>
                      <p className="mt-2 line-clamp-1 text-sm font-medium text-white group-hover:text-orange-400">{t.title}</p>
                      <p className="text-xs text-gray-500">{t.minutes >= 60 ? `${Math.round(t.minutes / 6) / 10} soat` : `${t.minutes} daqiqa`}</p>
                    </Link>
                  </li>
                ))}
              </ol>
            </section>
          )}

          {/* Community */}
          {data.ratings + data.reviews + data.comments > 0 && (
            <section className="grid grid-cols-3 gap-3">
              <Stat icon={<Star size={20} />} value={fmt(data.ratings)} label="baho qo'ydingiz" />
              <Stat icon={<PenLine size={20} />} value={fmt(data.reviews)} label="taqriz yozdingiz" />
              <Stat icon={<MessageSquare size={20} />} value={fmt(data.comments)} label="izoh qoldirdingiz" />
            </section>
          )}
        </div>
      )}
    </div>
  );
}
