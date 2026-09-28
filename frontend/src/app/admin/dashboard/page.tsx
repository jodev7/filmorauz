"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { Film, PlusCircle, ExternalLink, Users, Eye, Star, Tv, Wifi, UserCheck, Globe, Activity, CalendarDays, CalendarRange, Monitor, MapPin, PlayCircle, ChevronLeft, ChevronRight, User as UserIcon, LayoutDashboard, Download, type LucideIcon } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { adminGetMovies, Movie, getAdminDashboardStats, getAdminShareStats, getAdminUserMetrics, getAdminTopMovies, getAdminTopSeries, getAdminOnlineStats, getAdminOnlineSessions, DashboardStats, AdminShareStats, UserMetrics, TopContentItem, OnlineStats, OnlineSessionsPage } from "@/lib/api";
import { normalizeMediaUrl } from "@/lib/image-utils";
import MediaImage from "@/components/ui/MediaImage";
import SystemStatusBlock from "@/components/admin/SystemStatusBlock";
import dynamic from "next/dynamic";
import AdminOverviewBlocks from "@/components/admin/AdminOverviewBlocks";
import DailyReportButton from "@/components/admin/DailyReportButton";
import { isSuperAdminRole } from "@/lib/roles";
import { useVisibleInterval } from "@/lib/use-visible-interval";

// recharts is heavy — load the charts section separately so the rest of the
// dashboard renders without waiting for it.
const DashboardCharts = dynamic(() => import("@/components/admin/DashboardCharts"), {
  ssr: false,
  loading: () => (
    <div className="mb-6 h-40 animate-pulse rounded-3xl border border-white/10 bg-[#12121a]" aria-hidden />
  ),
});

const UZ_WEEKDAYS = ["Yakshanba", "Dushanba", "Seshanba", "Chorshanba", "Payshanba", "Juma", "Shanba"];
const UZ_MONTHS = ["yanvar", "fevral", "mart", "aprel", "may", "iyun", "iyul", "avgust", "sentabr", "oktabr", "noyabr", "dekabr"];

const num = (n: number | undefined | null) => (typeof n === "number" ? n.toLocaleString() : "0");

// Short Uzbek relative-time label for the live-session "last seen" column.
function formatRelativeTime(iso: string): string {
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return "—";
  const diffSec = Math.max(0, Math.floor((Date.now() - then) / 1000));
  if (diffSec < 10) return "hozir";
  if (diffSec < 60) return `${diffSec} soniya oldin`;
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return `${diffMin} daqiqa oldin`;
  const diffHour = Math.floor(diffMin / 60);
  if (diffHour < 24) return `${diffHour} soat oldin`;
  return `${Math.floor(diffHour / 24)} kun oldin`;
}

export default function AdminDashboard() {
  const { token, user } = useAuth();
  // Each block tracks its own state: `null` = still loading. A failing
  // request only affects its own block instead of blanking the dashboard.
  const [recentMovies, setRecentMovies] = useState<Movie[] | null>(null);
  const [stats, setStats] = useState<DashboardStats | null>(null);
  const [statsFailed, setStatsFailed] = useState(false);

  // Share stats
  const [shareStats, setShareStats] = useState<AdminShareStats | null>(null);

  // User metrics
  const [userMetrics, setUserMetrics] = useState<UserMetrics | null>(null);
  
  // Top content
  const [topMovies, setTopMovies] = useState<TopContentItem[] | null>(null);
  const [topSeries, setTopSeries] = useState<TopContentItem[] | null>(null);

  // Live activity (online + DAU/WAU/MAU). Refreshed on a short interval.
  const [onlineStats, setOnlineStats] = useState<OnlineStats | null>(null);

  // Detailed live-session list (paginated: IP, device, clickable name).
  const [sessions, setSessions] = useState<OnlineSessionsPage | null>(null);
  const [sessionsPage, setSessionsPage] = useState(1);
  const SESSIONS_PER_PAGE = 20;

  useEffect(() => {
    if (!token) return;
    let cancelled = false;
    // Independent requests (not Promise.all): each block renders as soon as
    // its own data arrives, and one failure doesn't hide the others.
    const run = <T,>(p: Promise<T>, onOk: (v: T) => void, onErr?: () => void) => {
      p.then((v) => {
        if (!cancelled) onOk(v);
      }).catch((err) => {
        console.error(err);
        if (!cancelled) onErr?.();
      });
    };

    // Only the 5 newest are shown — counts come from the stats endpoint.
    run(adminGetMovies(token, 5), setRecentMovies, () => setRecentMovies([]));
    run(getAdminDashboardStats(token), setStats, () => setStatsFailed(true));
    run(getAdminShareStats(token), setShareStats);
    run(getAdminUserMetrics(token), setUserMetrics);
    run(getAdminTopMovies(token), (d) => setTopMovies(d.data || []), () => setTopMovies([]));
    run(getAdminTopSeries(token), (d) => setTopSeries(d.data || []), () => setTopSeries([]));

    return () => {
      cancelled = true;
    };
  }, [token]);

  // Live polling — paused while the tab is hidden (see useVisibleInterval).
  const loadOnlineStats = useCallback(
    (isActive: () => boolean) => {
      if (!token) return;
      getAdminOnlineStats(token)
        .then((data) => {
          if (isActive()) setOnlineStats(data);
        })
        .catch(() => {});
    },
    [token]
  );
  useVisibleInterval(loadOnlineStats, 15_000);

  const loadSessions = useCallback(
    (isActive: () => boolean) => {
      if (!token) return;
      getAdminOnlineSessions(token, sessionsPage, SESSIONS_PER_PAGE)
        .then((data) => {
          if (isActive()) setSessions(data);
        })
        .catch(() => {});
    },
    [token, sessionsPage]
  );
  useVisibleInterval(loadSessions, 15_000);

  const formatCount = (n: number | undefined) =>
    typeof n === "number" ? n.toLocaleString() : statsFailed || stats ? "—" : "…";

  // Format date for display
  const formatDate = (dateStr: string) => {
    if (!dateStr) return "—";
    const d = new Date(dateStr);
    return d.toLocaleDateString("uz-UZ");
  };

  // Get display name for user
  const getUserDisplayName = (u: { display_name?: string; first_name?: string; last_name?: string; username?: string; telegram_id?: number }) => {
    if (u.first_name || u.last_name) {
      return [u.first_name, u.last_name].filter(Boolean).join(" ");
    }
    if (u.display_name) return u.display_name;
    if (u.username) return `@${u.username}`;
    if (u.telegram_id) return `ID: ${u.telegram_id}`;
    return "N/A";
  };

  // Get role badge color
  const getRoleBadgeColor = (role: string) => {
    switch (role) {
      case "superadmin": return "bg-red-500/20 text-red-400";
      case "admin": return "bg-orange-500/20 text-orange-400";
      default: return "bg-green-500/20 text-green-400";
    }
  };

  const hour = new Date().getHours();
  const greeting = hour < 5 ? "Xayrli tun" : hour < 12 ? "Xayrli tong" : hour < 18 ? "Xayrli kun" : "Xayrli kech";
  // Browsers often lack the uz-UZ locale ("M09 27, Sun"), so format by hand.
  const now = new Date();
  const today = `${UZ_WEEKDAYS[now.getDay()]}, ${now.getDate()}-${UZ_MONTHS[now.getMonth()]}`;
  const maxTopMovie = Math.max(1, ...(topMovies || []).slice(0, 5).map((i) => i.views_count || 0));
  const maxTopSeries = Math.max(1, ...(topSeries || []).slice(0, 5).map((i) => i.views_count || 0));

  return (
    <div className="mx-auto max-w-7xl p-4 sm:p-6">
      {/* ── Hero ─────────────────────────────────────────────────── */}
      <header className="relative mb-6 overflow-hidden rounded-3xl border border-white/10 bg-[#12121a] p-5 sm:p-7">
        <div className="pointer-events-none absolute -right-20 -top-24 h-72 w-72 rounded-full bg-orange-500/20 blur-3xl" aria-hidden />
        <div className="pointer-events-none absolute -bottom-24 left-1/3 h-64 w-64 rounded-full bg-fuchsia-600/10 blur-3xl" aria-hidden />
        <div className="relative flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between">
          <div className="flex items-start gap-4">
            <span className="flex h-12 w-12 shrink-0 items-center justify-center rounded-2xl bg-gradient-to-br from-orange-500 to-rose-600 shadow-lg shadow-orange-500/25">
              <LayoutDashboard size={22} className="text-white" />
            </span>
            <div>
              <p className="text-xs font-medium text-gray-500">{today}</p>
              <h1 className="text-2xl font-bold text-white sm:text-3xl">
                {greeting}
                {user?.display_name ? `, ${user.display_name}` : ""}
              </h1>
              <p className="mt-1 text-sm text-gray-400">Platformada hozir nima bo&apos;layotganini shu yerda ko&apos;rasiz.</p>
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {[
              { href: "/admin/movies/new", label: "Kino qo'shish", icon: PlusCircle },
              { href: "/admin/series/new", label: "Serial qo'shish", icon: Tv },
              { href: "/admin/ingestion", label: "Import", icon: Download },
            ].map(({ href, label, icon: Icon }) => (
              <Link
                key={href}
                href={href}
                className="inline-flex items-center gap-2 rounded-xl border border-white/10 bg-white/[0.04] px-3.5 py-2.5 text-sm text-gray-200 transition hover:border-orange-500/40 hover:bg-orange-500/10 hover:text-white"
              >
                <Icon size={16} /> {label}
              </Link>
            ))}
            {token && isSuperAdminRole(user?.role) && <DailyReportButton token={token} />}
          </div>
        </div>

        {/* KPI strip */}
        <div className="relative mt-6 grid grid-cols-2 gap-3 md:grid-cols-4">
          {[
            { label: "Jami kinolar", value: formatCount(stats?.movies?.total), hint: `+${formatCount(stats?.movies?.added_this_month)} bu oyda`, icon: Film, tone: "from-orange-500/20 text-orange-300" },
            { label: "Foydalanuvchilar", value: userMetrics ? num(userMetrics.total_users) : "…", hint: userMetrics ? `${num(userMetrics.premium_users)} premium` : "", icon: Users, tone: "from-violet-500/20 text-violet-300" },
            { label: "Premium konversiya", value: userMetrics ? `${(userMetrics.conversion_rate || 0).toFixed(1)}%` : "…", hint: "foydalanuvchilardan", icon: Star, tone: "from-amber-500/20 text-amber-300" },
            { label: "Jami ko'rishlar", value: userMetrics ? num(userMetrics.total_views) : "…", hint: shareStats ? `${num(shareStats.total_shares_created)} ulashish · ${num(shareStats.total_share_opens)} ochilgan` : "", icon: Eye, tone: "from-sky-500/20 text-sky-300" },
          ].map(({ label, value, hint, icon: Icon, tone }) => (
            <div key={label} className="rounded-2xl border border-white/10 bg-black/25 p-4">
              <div className="mb-2 flex items-center justify-between">
                <span className="text-xs text-gray-400">{label}</span>
                <span className={`flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br to-transparent ${tone}`}>
                  <Icon size={15} />
                </span>
              </div>
              <p className="text-2xl font-bold tabular-nums text-white sm:text-3xl">{value}</p>
              {hint && <p className="mt-0.5 truncate text-[11px] text-gray-500">{hint}</p>}
            </div>
          ))}
        </div>
      </header>

      {/* ── Live activity ────────────────────────────────────────── */}
      <section className="mb-6 grid gap-3 lg:grid-cols-[1.1fr_2fr]">
        <div className="relative overflow-hidden rounded-3xl border border-emerald-500/25 bg-gradient-to-br from-emerald-500/[0.12] via-[#12121a] to-[#12121a] p-5">
          <div className="flex items-center gap-2">
            <span className="relative flex h-2.5 w-2.5">
              <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75" />
              <span className="relative inline-flex h-2.5 w-2.5 rounded-full bg-emerald-500" />
            </span>
            <span className="text-sm font-semibold text-emerald-300">Hozir onlayn</span>
            <span className="ml-auto text-[11px] text-gray-500">har 15 soniyada</span>
          </div>
          <p className="mt-3 text-5xl font-bold tabular-nums text-white">{onlineStats?.online?.total ?? "—"}</p>
          <div className="mt-4 grid grid-cols-2 gap-2">
            <div className="rounded-xl bg-black/25 px-3 py-2">
              <p className="flex items-center gap-1.5 text-[11px] text-gray-400">
                <UserCheck size={12} className="text-sky-400" /> Login bo&apos;lgan
              </p>
              <p className="text-lg font-semibold tabular-nums text-white">{onlineStats?.online?.authenticated ?? "—"}</p>
            </div>
            <div className="rounded-xl bg-black/25 px-3 py-2">
              <p className="flex items-center gap-1.5 text-[11px] text-gray-400">
                <Globe size={12} className="text-amber-400" /> Mehmonlar
              </p>
              <p className="text-lg font-semibold tabular-nums text-white">{onlineStats?.online?.anonymous ?? "—"}</p>
            </div>
          </div>
        </div>
        <div className="grid grid-cols-3 gap-3">
          {[
            { label: "DAU", sub: "24 soat", value: onlineStats?.active?.dau, icon: Activity, tone: "text-pink-300 bg-pink-500/15" },
            { label: "WAU", sub: "7 kun", value: onlineStats?.active?.wau, icon: CalendarDays, tone: "text-violet-300 bg-violet-500/15" },
            { label: "MAU", sub: "30 kun", value: onlineStats?.active?.mau, icon: CalendarRange, tone: "text-cyan-300 bg-cyan-500/15" },
          ].map(({ label, sub, value, icon: Icon, tone }) => (
            <div key={label} className="flex flex-col justify-between rounded-3xl border border-white/10 bg-[#12121a] p-4 sm:p-5">
              <span className={`flex h-9 w-9 items-center justify-center rounded-xl ${tone}`}>
                <Icon size={17} />
              </span>
              <div className="mt-3">
                <p className="text-2xl font-bold tabular-nums text-white sm:text-3xl">{typeof value === "number" ? value.toLocaleString() : "—"}</p>
                <p className="text-xs text-gray-400">
                  <span className="font-semibold text-gray-300">{label}</span> · {sub}
                </p>
              </div>
            </div>
          ))}
        </div>
      </section>

      {/* Needs attention, pipeline health, content quality, finance */}
      <AdminOverviewBlocks />

      {/* VPS / fleet status */}
      <SystemStatusBlock />

      {/* Daily trends + period-over-period deltas */}
      <DashboardCharts />

      {/* ── Top content ──────────────────────────────────────────── */}
      <div className="mb-6 grid grid-cols-1 gap-4 lg:grid-cols-2">
        {[
          { title: "Eng ko'p ko'rilgan kinolar", href: "/admin/movies", items: topMovies, max: maxTopMovie, icon: Film, path: "/movies/" },
          { title: "Eng ko'p ko'rilgan seriallar", href: "/admin/series", items: topSeries, max: maxTopSeries, icon: Tv, path: "/series/" },
        ].map(({ title, href, items, max, icon: Icon, path }) => (
          <Panel key={title} title={title} icon={Icon} href={href}>
            {items === null ? (
              <PanelSkeleton />
            ) : items.length === 0 ? (
              <p className="px-4 py-8 text-center text-sm text-gray-500">Ma&apos;lumot yo&apos;q</p>
            ) : (
              <ol className="divide-y divide-white/5">
                {items.slice(0, 5).map((item, index) => {
                  const poster = item.poster_url ? normalizeMediaUrl(item.poster_url) : "";
                  const pct = Math.round(((item.views_count || 0) / max) * 100);
                  return (
                    <li key={item.slug}>
                      <Link href={`${path}${item.slug}`} target="_blank" className="flex items-center gap-3 px-4 py-3 transition hover:bg-white/[0.03]">
                        <span
                          className={`flex h-7 w-7 shrink-0 items-center justify-center rounded-lg text-xs font-bold ${
                            index === 0 ? "bg-amber-400 text-black" : index === 1 ? "bg-gray-300 text-black" : index === 2 ? "bg-orange-700 text-white" : "bg-white/5 text-gray-400"
                          }`}
                        >
                          {index + 1}
                        </span>
                        {poster ? (
                          <MediaImage src={poster} alt={item.title} className="h-12 w-9 shrink-0 rounded-md object-cover" />
                        ) : (
                          <span className="flex h-12 w-9 shrink-0 items-center justify-center rounded-md bg-white/5 text-gray-600">
                            <Icon size={14} />
                          </span>
                        )}
                        <div className="min-w-0 flex-1">
                          <p className="truncate text-sm font-medium text-white">{item.title}</p>
                          <div className="mt-1.5 flex items-center gap-2">
                            <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-white/5">
                              <div className="h-full rounded-full bg-gradient-to-r from-orange-500 to-rose-500" style={{ width: `${pct}%` }} />
                            </div>
                            <span className="shrink-0 text-[11px] tabular-nums text-gray-400">{(item.views_count || 0).toLocaleString()}</span>
                          </div>
                        </div>
                      </Link>
                    </li>
                  );
                })}
              </ol>
            )}
          </Panel>
        ))}
      </div>

      {/* ── Recent movies & users ────────────────────────────────── */}
      <div className="mb-6 grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Panel title="So'nggi kinolar" icon={Film} href="/admin/movies">
          {recentMovies === null ? (
            <PanelSkeleton />
          ) : recentMovies.length === 0 ? (
            <div className="px-4 py-8 text-center">
              <p className="text-sm text-gray-500">Hali kinolar yo&apos;q.</p>
              <Link href="/admin/movies/new" className="mt-2 inline-block text-sm text-orange-400 hover:underline">
                Birinchi kinoni qo&apos;shing
              </Link>
            </div>
          ) : (
            <ul className="divide-y divide-white/5">
              {recentMovies.map((movie) => (
                <li key={movie.id} className="flex items-center gap-3 px-4 py-3 transition hover:bg-white/[0.03]">
                  <MediaImage src={normalizeMediaUrl(movie.poster_url)} alt={movie.title} className="h-12 w-9 shrink-0 rounded-md object-cover" />
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium text-white">{movie.title}</p>
                    <p className="text-[11px] text-gray-500">
                      {movie.code && <span className="mr-1.5 font-mono">#{movie.code}</span>}
                      {movie.year || ""}
                      {movie.quality && <span className="ml-1.5 rounded bg-emerald-500/15 px-1.5 py-0.5 text-emerald-300">{movie.quality}</span>}
                    </p>
                  </div>
                  <Link
                    href={`/admin/movies/${movie.id}/edit`}
                    className="inline-flex shrink-0 items-center gap-1 rounded-lg px-2.5 py-1.5 text-xs text-gray-400 transition hover:bg-white/5 hover:text-white"
                  >
                    <ExternalLink size={12} /> Tahrirlash
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </Panel>

        <Panel title="So'nggi foydalanuvchilar" icon={Users} href="/admin/users">
          {!stats ? (
            statsFailed ? <p className="px-4 py-8 text-center text-sm text-gray-500">Yuklab bo&apos;lmadi</p> : <PanelSkeleton />
          ) : (stats.users?.recent || []).length === 0 ? (
            <p className="px-4 py-8 text-center text-sm text-gray-500">Hali foydalanuvchilar yo&apos;q.</p>
          ) : (
            <ul className="divide-y divide-white/5">
              {(stats.users?.recent || []).map((u) => {
                const name = getUserDisplayName(u);
                return (
                  <li key={u.id} className="flex items-center gap-3 px-4 py-3 transition hover:bg-white/[0.03]">
                    <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-sky-500 to-indigo-600 text-sm font-semibold text-white">
                      {name.replace(/^@/, "").charAt(0).toUpperCase() || "?"}
                    </span>
                    <div className="min-w-0 flex-1">
                      <Link href={`/user/${u.id}`} className="block truncate text-sm font-medium text-white hover:text-orange-300">
                        {name}
                      </Link>
                      <p className="text-[11px] text-gray-500">
                        {u.username ? `@${u.username} · ` : ""}
                        {formatDate(u.created_at)}
                      </p>
                    </div>
                    <span className={`shrink-0 rounded-md px-2 py-0.5 text-[11px] ${getRoleBadgeColor(u.role)}`}>
                      {u.role === "superadmin" ? "Super Admin" : u.role === "admin" ? "Admin" : "Foydalanuvchi"}
                    </span>
                  </li>
                );
              })}
            </ul>
          )}
        </Panel>
      </div>

      {/* ── Live sessions ────────────────────────────────────────── */}
      <Panel title="Onlayn sessiyalar" icon={Wifi} right={<span className="text-xs text-gray-500">{sessions ? `${sessions.total ?? 0} ta faol` : "Yuklanmoqda..."}</span>}>
        <div className="hidden grid-cols-[1.4fr_0.9fr_1fr_1.4fr_0.9fr] gap-3 border-b border-white/5 px-4 py-2.5 text-[11px] uppercase tracking-wide text-gray-500 sm:grid">
          <span>Foydalanuvchi</span>
          <span>IP manzil</span>
          <span>Qurilma</span>
          <span>Ko&apos;rmoqda</span>
          <span className="text-right">Oxirgi faollik</span>
        </div>

        {sessions && (sessions.sessions || []).length === 0 && <div className="px-4 py-8 text-center text-sm text-gray-500">Hozircha faol sessiya yo&apos;q</div>}

        {sessions?.sessions?.map((s) => (
          <div
            key={s.session_id}
            className="grid grid-cols-2 items-center gap-2 border-b border-white/5 px-4 py-3 text-sm last:border-b-0 hover:bg-white/[0.02] sm:grid-cols-[1.4fr_0.9fr_1fr_1.4fr_0.9fr] sm:gap-3"
          >
            <div className="col-span-2 flex min-w-0 items-center gap-2 sm:col-span-1">
              {s.type === "authenticated" ? (
                <>
                  <span className="h-2 w-2 shrink-0 rounded-full bg-sky-400" />
                  {s.user_id ? (
                    <Link href={`/user/${s.user_id}`} className="truncate font-medium text-sky-300 hover:text-sky-200 hover:underline">
                      {s.name || "Foydalanuvchi"}
                    </Link>
                  ) : (
                    <span className="truncate text-white">{s.name}</span>
                  )}
                  {s.role && s.role !== "user" && (
                    <span className="shrink-0 rounded bg-orange-500/15 px-1.5 py-0.5 text-[10px] uppercase text-orange-300">{s.role === "superadmin" ? "SA" : "admin"}</span>
                  )}
                </>
              ) : (
                <>
                  <span className="h-2 w-2 shrink-0 rounded-full bg-amber-400" />
                  <span className="flex items-center gap-1.5 truncate text-gray-400">
                    <UserIcon size={13} /> Anonim
                  </span>
                </>
              )}
            </div>
            <div className="flex min-w-0 items-center gap-1.5 text-gray-300">
              <MapPin size={13} className="shrink-0 text-gray-500 sm:hidden" />
              <span className="truncate font-mono text-xs">{s.ip || "—"}</span>
            </div>
            <div className="flex min-w-0 items-center gap-1.5 text-gray-300">
              <Monitor size={13} className="shrink-0 text-gray-500" />
              <span className="truncate text-xs">{s.device}</span>
            </div>
            <div className="col-span-2 flex min-w-0 items-center gap-1.5 sm:col-span-1">
              {s.watching ? (
                <>
                  <PlayCircle size={13} className="shrink-0 text-orange-400" />
                  {s.watching.url ? (
                    <Link href={s.watching.url} className="truncate text-xs text-gray-200 hover:text-white hover:underline" title={s.watching.title}>
                      {s.watching.title || s.watching.slug}
                    </Link>
                  ) : (
                    <span className="truncate text-xs text-gray-200" title={s.watching.title}>
                      {s.watching.title || s.watching.slug}
                    </span>
                  )}
                  {s.watching.type === "episode" && <span className="shrink-0 rounded bg-white/10 px-1.5 py-0.5 text-[10px] text-gray-300">qism</span>}
                </>
              ) : (
                <span className="text-xs text-gray-600">—</span>
              )}
            </div>
            <div className="col-span-2 text-right text-xs text-gray-500 sm:col-span-1">{formatRelativeTime(s.last_seen)}</div>
          </div>
        ))}

        {sessions && sessions.total_pages > 1 && (
          <div className="flex items-center justify-center gap-3 border-t border-white/5 px-4 py-3">
            <button
              onClick={() => setSessionsPage((p) => Math.max(1, p - 1))}
              disabled={sessionsPage <= 1}
              className="flex items-center gap-1 rounded-lg border border-white/10 px-3 py-1.5 text-sm text-gray-300 transition hover:border-orange-500/50 disabled:cursor-not-allowed disabled:opacity-40"
            >
              <ChevronLeft size={16} /> Oldingi
            </button>
            <span className="text-sm tabular-nums text-gray-400">
              {sessions.page} / {sessions.total_pages}
            </span>
            <button
              onClick={() => setSessionsPage((p) => Math.min(sessions.total_pages, p + 1))}
              disabled={sessionsPage >= sessions.total_pages}
              className="flex items-center gap-1 rounded-lg border border-white/10 px-3 py-1.5 text-sm text-gray-300 transition hover:border-orange-500/50 disabled:cursor-not-allowed disabled:opacity-40"
            >
              Keyingi <ChevronRight size={16} />
            </button>
          </div>
        )}
      </Panel>
    </div>
  );
}

function Panel({
  title,
  icon: Icon,
  href,
  right,
  children,
}: {
  title: string;
  icon: LucideIcon;
  href?: string;
  right?: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <section className="overflow-hidden rounded-3xl border border-white/10 bg-[#12121a]">
      <header className="flex items-center justify-between gap-3 border-b border-white/5 px-4 py-3.5">
        <h2 className="flex items-center gap-2 font-semibold text-white">
          <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-white/5 text-gray-300">
            <Icon size={15} />
          </span>
          {title}
        </h2>
        {right ??
          (href && (
            <Link href={href} className="text-xs text-gray-400 transition hover:text-orange-300">
              Hammasi →
            </Link>
          ))}
      </header>
      {children}
    </section>
  );
}

function PanelSkeleton() {
  return (
    <div className="space-y-2 p-4">
      {[0, 1, 2, 3].map((i) => (
        <div key={i} className="h-12 animate-pulse rounded-xl bg-white/[0.04]" />
      ))}
    </div>
  );
}
