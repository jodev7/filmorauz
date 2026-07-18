"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Film, PlusCircle, List, ExternalLink, Users, UserPlus, Share2, Eye, Star, Tv, Wifi, UserCheck, Globe, Activity, CalendarDays, CalendarRange, Monitor, MapPin, ChevronLeft, ChevronRight, User as UserIcon } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { adminGetMovies, Movie, getAdminDashboardStats, getAdminShareStats, getAdminUserMetrics, getAdminTopMovies, getAdminTopSeries, getAdminOnlineStats, getAdminOnlineSessions, DashboardStats, AdminShareStats, UserMetrics, TopContentItem, OnlineStats, OnlineSessionsPage } from "@/lib/api";
import { normalizeMediaUrl } from "@/lib/image-utils";
import MediaImage from "@/components/ui/MediaImage";
import SystemStatusBlock from "@/components/admin/SystemStatusBlock";

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
  const [movies, setMovies] = useState<Movie[]>([]);
  const [stats, setStats] = useState<DashboardStats | null>(null);
  const [loading, setLoading] = useState(true);

  // Share stats
  const [shareStats, setShareStats] = useState<AdminShareStats | null>(null);

  // User metrics
  const [userMetrics, setUserMetrics] = useState<UserMetrics | null>(null);
  
  // Top content
  const [topMovies, setTopMovies] = useState<TopContentItem[]>([]);
  const [topSeries, setTopSeries] = useState<TopContentItem[]>([]);

  // Live activity (online + DAU/WAU/MAU). Refreshed on a short interval.
  const [onlineStats, setOnlineStats] = useState<OnlineStats | null>(null);

  // Detailed live-session list (paginated: IP, device, clickable name).
  const [sessions, setSessions] = useState<OnlineSessionsPage | null>(null);
  const [sessionsPage, setSessionsPage] = useState(1);
  const SESSIONS_PER_PAGE = 20;

  useEffect(() => {
    if (!token) return;

    Promise.all([
      adminGetMovies(token),
      getAdminDashboardStats(token),
      getAdminShareStats(token),
      getAdminUserMetrics(token),
      getAdminTopMovies(token),
      getAdminTopSeries(token)
    ])
      .then(([moviesData, statsData, shareStatsData, userMetricsData, topMoviesData, topSeriesData]) => {
        setMovies(moviesData);
        setStats(statsData);
        setShareStats(shareStatsData);
        setUserMetrics(userMetricsData);
        setTopMovies(topMoviesData.data || []);
        setTopSeries(topSeriesData.data || []);
      })
      .catch(console.error)
      .finally(() => setLoading(false));
  }, [token]);

  useEffect(() => {
    if (!token) return;
    let cancelled = false;
    const load = () => {
      getAdminOnlineStats(token)
        .then((data) => {
          if (!cancelled) setOnlineStats(data);
        })
        .catch(() => {});
    };
    load();
    const t = setInterval(load, 15_000);
    return () => {
      cancelled = true;
      clearInterval(t);
    };
  }, [token]);

  useEffect(() => {
    if (!token) return;
    let cancelled = false;
    const load = () => {
      getAdminOnlineSessions(token, sessionsPage, SESSIONS_PER_PAGE)
        .then((data) => {
          if (!cancelled) setSessions(data);
        })
        .catch(() => {});
    };
    load();
    const t = setInterval(load, 15_000);
    return () => {
      cancelled = true;
      clearInterval(t);
    };
  }, [token, sessionsPage]);

  const recentMovies = movies.slice(0, 5);

  // Movies this month
  const thisMonthCount = movies.filter((m) => {
    if (!m.created_at) return false;
    const d = new Date(m.created_at);
    if (isNaN(d.getTime())) return false;
    const now = new Date();
    return d.getUTCMonth() === now.getUTCMonth() && d.getUTCFullYear() === now.getUTCFullYear();
  }).length;

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

  return (
    <div className="p-4 sm:p-8">
      <div className="mb-6 sm:mb-8">
        <h1 className="text-xl sm:text-2xl font-bold text-white">Boshqaruv paneli</h1>
        <p className="text-gray-500 text-sm mt-1">
          Xush kelibsiz{user?.display_name ? `, ${user.display_name}` : ""}. Bu yerda nima bo'layotganini ko'ring.
        </p>
      </div>

      {/* VPS / fleet status */}
      <SystemStatusBlock />

      {/* Live activity — online users (authed + anonymous) + DAU/WAU/MAU */}
      <div className="mb-8 sm:mb-10">
        <div className="flex items-center gap-2 mb-3">
          <span className="relative flex h-2.5 w-2.5">
            <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
            <span className="relative inline-flex rounded-full h-2.5 w-2.5 bg-emerald-500"></span>
          </span>
          <h2 className="text-base sm:text-lg font-semibold text-white">Jonli faollik</h2>
          <span className="text-xs text-gray-500">har 15 soniyada yangilanadi</span>
        </div>

        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-7 gap-3">
          <div className="bg-brand-card border border-emerald-500/30 rounded-xl p-4">
            <div className="flex items-center gap-2 mb-1.5">
              <Wifi size={16} className="text-emerald-400" />
              <span className="text-xs text-gray-400">Hozir onlayn</span>
            </div>
            <p className="text-2xl font-bold text-white">
              {onlineStats ? onlineStats.online.total : "—"}
            </p>
            <p className="text-[11px] text-gray-500 mt-0.5">jami (auth + anonim)</p>
          </div>

          <div className="bg-brand-card border border-brand-border rounded-xl p-4">
            <div className="flex items-center gap-2 mb-1.5">
              <UserCheck size={16} className="text-blue-400" />
              <span className="text-xs text-gray-400">Login bo'lgan</span>
            </div>
            <p className="text-2xl font-bold text-white">
              {onlineStats ? onlineStats.online.authenticated : "—"}
            </p>
            <p className="text-[11px] text-gray-500 mt-0.5">akkauntli onlayn</p>
          </div>

          <div className="bg-brand-card border border-brand-border rounded-xl p-4">
            <div className="flex items-center gap-2 mb-1.5">
              <Globe size={16} className="text-amber-400" />
              <span className="text-xs text-gray-400">Anonim</span>
            </div>
            <p className="text-2xl font-bold text-white">
              {onlineStats ? onlineStats.online.anonymous : "—"}
            </p>
            <p className="text-[11px] text-gray-500 mt-0.5">mehmonlar onlayn</p>
          </div>

          <div className="bg-brand-card border border-brand-border rounded-xl p-4">
            <div className="flex items-center gap-2 mb-1.5">
              <Activity size={16} className="text-pink-400" />
              <span className="text-xs text-gray-400">DAU</span>
            </div>
            <p className="text-2xl font-bold text-white">
              {onlineStats ? onlineStats.active.dau : "—"}
            </p>
            <p className="text-[11px] text-gray-500 mt-0.5">24 soat ichida</p>
          </div>

          <div className="bg-brand-card border border-brand-border rounded-xl p-4">
            <div className="flex items-center gap-2 mb-1.5">
              <CalendarDays size={16} className="text-purple-400" />
              <span className="text-xs text-gray-400">WAU</span>
            </div>
            <p className="text-2xl font-bold text-white">
              {onlineStats ? onlineStats.active.wau : "—"}
            </p>
            <p className="text-[11px] text-gray-500 mt-0.5">7 kun ichida</p>
          </div>

          <div className="bg-brand-card border border-brand-border rounded-xl p-4">
            <div className="flex items-center gap-2 mb-1.5">
              <CalendarRange size={16} className="text-cyan-400" />
              <span className="text-xs text-gray-400">MAU</span>
            </div>
            <p className="text-2xl font-bold text-white">
              {onlineStats ? onlineStats.active.mau : "—"}
            </p>
            <p className="text-[11px] text-gray-500 mt-0.5">30 kun ichida</p>
          </div>

          <div className="bg-brand-card border border-brand-border rounded-xl p-4 col-span-2 sm:col-span-3 lg:col-span-1">
            <div className="flex items-center gap-2 mb-1.5">
              <Tv size={16} className="text-gray-400" />
              <span className="text-xs text-gray-400">Holat</span>
            </div>
            <p className="text-sm text-gray-300">
              {onlineStats
                ? `${onlineStats.online.authenticated} login, ${onlineStats.online.anonymous} anonim`
                : "Yuklanmoqda..."}
            </p>
          </div>
        </div>
      </div>

      {/* Live sessions — detailed paginated list (IP, device, name) */}
      <div className="mb-8 sm:mb-10">
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-base sm:text-lg font-semibold text-white">
            Onlayn sessiyalar
          </h2>
          <span className="text-xs text-gray-500">
            {sessions ? `${sessions.total} ta faol sessiya` : "Yuklanmoqda..."}
          </span>
        </div>

        <div className="bg-brand-card border border-brand-border rounded-xl overflow-hidden">
          {/* Header row (desktop only) */}
          <div className="hidden sm:grid grid-cols-[1.6fr_1fr_1.2fr_0.9fr] gap-3 px-4 py-2.5 border-b border-brand-border text-[11px] uppercase tracking-wide text-gray-500">
            <span>Foydalanuvchi</span>
            <span>IP manzil</span>
            <span>Qurilma</span>
            <span className="text-right">Oxirgi faollik</span>
          </div>

          {sessions && sessions.sessions.length === 0 && (
            <div className="px-4 py-8 text-center text-sm text-gray-500">
              Hozircha faol sessiya yo'q
            </div>
          )}

          {sessions?.sessions.map((s) => (
            <div
              key={s.session_id}
              className="grid grid-cols-2 sm:grid-cols-[1.6fr_1fr_1.2fr_0.9fr] gap-2 sm:gap-3 px-4 py-3 border-b border-brand-border/50 last:border-b-0 items-center text-sm"
            >
              {/* User / anonymous */}
              <div className="flex items-center gap-2 min-w-0 col-span-2 sm:col-span-1">
                {s.type === "authenticated" ? (
                  <>
                    <span className="w-2 h-2 rounded-full bg-blue-400 shrink-0" />
                    {s.user_id ? (
                      <Link
                        href={`/user/${s.user_id}`}
                        className="text-blue-400 hover:text-blue-300 hover:underline truncate font-medium"
                      >
                        {s.name || "Foydalanuvchi"}
                      </Link>
                    ) : (
                      <span className="text-white truncate">{s.name}</span>
                    )}
                    {s.role && s.role !== "user" && (
                      <span className="text-[10px] px-1.5 py-0.5 rounded bg-brand-red/15 text-brand-red uppercase shrink-0">
                        {s.role === "superadmin" ? "SA" : "admin"}
                      </span>
                    )}
                  </>
                ) : (
                  <>
                    <span className="w-2 h-2 rounded-full bg-amber-400 shrink-0" />
                    <span className="flex items-center gap-1.5 text-gray-400 truncate">
                      <UserIcon size={13} /> Anonim
                    </span>
                  </>
                )}
              </div>

              {/* IP */}
              <div className="flex items-center gap-1.5 text-gray-300 min-w-0">
                <MapPin size={13} className="text-gray-500 shrink-0 sm:hidden" />
                <span className="truncate font-mono text-xs">{s.ip || "—"}</span>
              </div>

              {/* Device */}
              <div className="flex items-center gap-1.5 text-gray-300 min-w-0">
                <Monitor size={13} className="text-gray-500 shrink-0" />
                <span className="truncate text-xs">{s.device}</span>
              </div>

              {/* Last seen */}
              <div className="text-right text-xs text-gray-500 col-span-2 sm:col-span-1">
                {formatRelativeTime(s.last_seen)}
              </div>
            </div>
          ))}
        </div>

        {/* Pagination */}
        {sessions && sessions.total_pages > 1 && (
          <div className="flex items-center justify-center gap-3 mt-4">
            <button
              onClick={() => setSessionsPage((p) => Math.max(1, p - 1))}
              disabled={sessionsPage <= 1}
              className="flex items-center gap-1 px-3 py-1.5 rounded-lg border border-brand-border text-sm text-gray-300 hover:border-brand-red disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              <ChevronLeft size={16} /> Oldingi
            </button>
            <span className="text-sm text-gray-400">
              {sessions.page} / {sessions.total_pages}
            </span>
            <button
              onClick={() =>
                setSessionsPage((p) => Math.min(sessions.total_pages, p + 1))
              }
              disabled={sessionsPage >= sessions.total_pages}
              className="flex items-center gap-1 px-3 py-1.5 rounded-lg border border-brand-border text-sm text-gray-300 hover:border-brand-red disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              Keyingi <ChevronRight size={16} />
            </button>
          </div>
        )}
      </div>

      {/* Movie Stats */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 mb-8 sm:mb-10">
        <div className="bg-brand-card border border-brand-border rounded-xl p-5">
          <div className="flex items-center gap-3 mb-2">
            <div className="w-9 h-9 bg-brand-red/10 rounded-lg flex items-center justify-center">
              <Film size={18} className="text-brand-red" />
            </div>
            <span className="text-sm text-gray-400">Jami kinolar</span>
          </div>
          <p className="text-3xl font-bold text-white">
            {loading ? "—" : movies.length}
          </p>
        </div>

        <div className="bg-brand-card border border-brand-border rounded-xl p-5">
          <div className="flex items-center gap-3 mb-2">
            <div className="w-9 h-9 bg-blue-500/10 rounded-lg flex items-center justify-center">
              <List size={18} className="text-blue-400" />
            </div>
            <span className="text-sm text-gray-400">Bu oyda</span>
          </div>
          <p className="text-3xl font-bold text-white">
            {loading ? "—" : thisMonthCount}
          </p>
        </div>

        <div className="bg-brand-card border border-brand-border rounded-xl p-5">
          <div className="flex items-center gap-3 mb-2">
            <div className="w-9 h-9 bg-green-500/10 rounded-lg flex items-center justify-center">
              <PlusCircle size={18} className="text-green-400" />
            </div>
            <span className="text-sm text-gray-400">Tezkor harakat</span>
          </div>
          <Link
            href="/admin/movies/new"
            className="mt-1 inline-flex items-center gap-1.5 text-sm text-green-400 hover:text-green-300 font-medium"
          >
            <PlusCircle size={14} />
            Yangi kino qo'shish
          </Link>
        </div>
      </div>

      {/* User Stats */}
      {stats && userMetrics && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-8 sm:mb-10">
          <div className="bg-brand-card border border-brand-border rounded-xl p-5">
            <div className="flex items-center gap-3 mb-2">
              <div className="w-9 h-9 bg-purple-500/10 rounded-lg flex items-center justify-center">
                <Users size={18} className="text-purple-400" />
              </div>
              <span className="text-sm text-gray-400">Jami foydalanuvchilar</span>
            </div>
            <p className="text-3xl font-bold text-white">
              {userMetrics.total_users}
            </p>
          </div>

          <div className="bg-brand-card border border-brand-border rounded-xl p-5">
            <div className="flex items-center gap-3 mb-2">
              <div className="w-9 h-9 bg-pink-500/10 rounded-lg flex items-center justify-center">
                <UserPlus size={18} className="text-pink-400" />
              </div>
              <span className="text-sm text-gray-400">Premium foydalanuvchilar</span>
            </div>
            <p className="text-3xl font-bold text-white">
              {userMetrics.premium_users}
            </p>
          </div>

          <div className="bg-brand-card border border-brand-border rounded-xl p-5">
            <div className="flex items-center gap-3 mb-2">
              <div className="w-9 h-9 bg-green-500/10 rounded-lg flex items-center justify-center">
                <Star size={18} className="text-green-400" />
              </div>
              <span className="text-sm text-gray-400">Premium konversiya</span>
            </div>
            <p className="text-3xl font-bold text-white">
              {userMetrics.conversion_rate.toFixed(1)}%
            </p>
          </div>

          <div className="bg-brand-card border border-brand-border rounded-xl p-5">
            <div className="flex items-center gap-3 mb-2">
              <div className="w-9 h-9 bg-cyan-500/10 rounded-lg flex items-center justify-center">
                <Eye size={18} className="text-cyan-400" />
              </div>
              <span className="text-sm text-gray-400">Jami ko'rishlar</span>
            </div>
            <p className="text-3xl font-bold text-white">
              {userMetrics.total_views.toLocaleString()}
            </p>
          </div>
        </div>
      )}

      {/* Share Stats */}
      {shareStats && (
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-8 sm:mb-10">
          <div className="bg-brand-card border border-brand-border rounded-xl p-5">
            <div className="flex items-center gap-3 mb-2">
              <div className="w-9 h-9 bg-orange-500/10 rounded-lg flex items-center justify-center">
                <Share2 size={18} className="text-orange-400" />
              </div>
              <span className="text-sm text-gray-400">Jami ulashishlar</span>
            </div>
            <p className="text-3xl font-bold text-white">
              {shareStats.total_shares_created}
            </p>
          </div>

          <div className="bg-brand-card border border-brand-border rounded-xl p-5">
            <div className="flex items-center gap-3 mb-2">
              <div className="w-9 h-9 bg-yellow-500/10 rounded-lg flex items-center justify-center">
                <Share2 size={18} className="text-yellow-400" />
              </div>
              <span className="text-sm text-gray-400">Jami ko'rilgan</span>
            </div>
            <p className="text-3xl font-bold text-white">
              {shareStats.total_share_opens}
            </p>
          </div>
        </div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Recent movies */}
        <div>
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-base sm:text-lg font-semibold text-white">So'nggi kinolar</h2>
            <Link
              href="/admin/movies"
              className="text-sm text-brand-red hover:text-orange-400 transition-colors"
            >
              Hammasini ko'rish
            </Link>
          </div>

          {loading ? (
            <div className="bg-brand-card border border-brand-border rounded-xl p-6 text-center">
              <p className="text-gray-500 text-sm">Yuklanmoqda...</p>
            </div>
          ) : recentMovies.length === 0 ? (
            <div className="bg-brand-card border border-brand-border rounded-xl p-6 sm:p-8 text-center">
              <Film size={32} className="text-gray-600 mx-auto mb-3" />
              <p className="text-gray-500 text-sm">Hali kinolar yo'q.</p>
              <Link
                href="/admin/movies/new"
                className="mt-3 inline-block text-sm text-brand-red hover:underline"
              >
                Birinchi kinoni qo'shing
              </Link>
            </div>
          ) : (
            <div className="bg-brand-card border border-brand-border rounded-xl overflow-hidden">
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-brand-border text-gray-500 text-xs uppercase tracking-wider">
                      <th className="text-left px-3 sm:px-5 py-3">Kino</th>
                      <th className="text-left px-3 sm:px-5 py-3 hidden md:table-cell">Yil</th>
                      <th className="text-left px-3 sm:px-5 py-3 hidden md:table-cell">Sifat</th>
                      <th className="text-right px-3 sm:px-5 py-3">Amallar</th>
                    </tr>
                  </thead>
                  <tbody>
                    {recentMovies.map((movie) => (
                      <tr
                        key={movie.id}
                        className="border-b border-brand-border/50 last:border-0 hover:bg-brand-border/30 transition-colors"
                      >
                        <td className="px-3 sm:px-5 py-3">
                          <div className="flex items-center gap-2 sm:gap-3">
                            {(() => {
                              const rowPosterSrc = normalizeMediaUrl(movie.poster_url);
                              return (
                            <MediaImage
                              src={rowPosterSrc}
                              alt={movie.title}
                              className="w-8 h-10 sm:w-8 sm:h-12 object-cover rounded"
                            />
                              );
                            })()}
                            <div className="min-w-0">
                              <p className="text-white font-medium truncate max-w-[120px] sm:max-w-[180px]">
                                {movie.code && (
                                  <span className="text-gray-500 font-mono text-xs mr-1">
                                    [{movie.code}]
                                  </span>
                                )}
                                {movie.title}
                              </p>
                            </div>
                          </div>
                        </td>
                        <td className="px-3 sm:px-5 py-3 hidden md:table-cell text-gray-400">
                          {movie.year}
                        </td>
                        <td className="px-3 sm:px-5 py-3 hidden md:table-cell">
                          <span className="text-green-400 text-xs">{movie.quality}</span>
                        </td>
                        <td className="px-3 sm:px-5 py-3 text-right">
                          <Link
                            href={`/admin/movies/${movie.id}/edit`}
                            className="text-brand-red hover:text-orange-400 inline-flex items-center gap-1 text-xs"
                          >
                            <ExternalLink size={12} />
                            O'zgartirish
                          </Link>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </div>

        {/* Recent users */}
        <div>
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-base sm:text-lg font-semibold text-white">So'nggi foydalanuvchilar</h2>
            <Link
              href="/admin/users"
              className="text-sm text-brand-red hover:text-orange-400 transition-colors"
            >
              Hammasini ko'rish
            </Link>
          </div>

          {loading || !stats ? (
            <div className="bg-brand-card border border-brand-border rounded-xl p-6 text-center">
              <p className="text-gray-500 text-sm">Yuklanmoqda...</p>
            </div>
          ) : stats.users.recent.length === 0 ? (
            <div className="bg-brand-card border border-brand-border rounded-xl p-6 sm:p-8 text-center">
              <Users size={32} className="text-gray-600 mx-auto mb-3" />
              <p className="text-gray-500 text-sm">Hali foydalanuvchilar yo'q.</p>
            </div>
          ) : (
            <div className="bg-brand-card border border-brand-border rounded-xl overflow-hidden">
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-brand-border text-gray-500 text-xs uppercase tracking-wider">
                      <th className="text-left px-3 sm:px-5 py-3">Foydalanuvchi</th>
                      <th className="text-left px-3 sm:px-5 py-3 hidden sm:table-cell">Rol</th>
                      <th className="text-left px-3 sm:px-5 py-3 hidden md:table-cell">Ro'yxatdan o'tgan</th>
                    </tr>
                  </thead>
                  <tbody>
                    {stats.users.recent.map((u) => (
                      <tr
                        key={u.id}
                        className="border-b border-brand-border/50 last:border-0 hover:bg-brand-border/30 transition-colors"
                      >
                        <td className="px-3 sm:px-5 py-3">
                          <div className="min-w-0">
                            <p className="text-white font-medium truncate max-w-[150px]">
                              {getUserDisplayName(u)}
                            </p>
                            {u.username && (
                              <p className="text-gray-500 text-xs">@{u.username}</p>
                            )}
                          </div>
                        </td>
                        <td className="px-3 sm:px-5 py-3 hidden sm:table-cell">
                          <span className={`text-xs px-2 py-1 rounded ${getRoleBadgeColor(u.role)}`}>
                            {u.role === "superadmin" ? "Super Admin" : u.role === "admin" ? "Admin" : "Foydalanuvchi"}
                          </span>
                        </td>
                        <td className="px-3 sm:px-5 py-3 hidden md:table-cell text-gray-400 text-xs">
                          {formatDate(u.created_at)}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Top Content */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Top Movies */}
        <div>
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-base sm:text-lg font-semibold text-white">Eng ko'p ko'rilgan kinolar</h2>
            <Link
              href="/admin/movies"
              className="text-sm text-brand-red hover:text-orange-400 transition-colors"
            >
              Hammasini ko'rish
            </Link>
          </div>

          {loading || topMovies.length === 0 ? (
            <div className="bg-brand-card border border-brand-border rounded-xl p-6 text-center">
              <p className="text-gray-500 text-sm">Ma'lumot yo'q</p>
            </div>
          ) : (
            <div className="bg-brand-card border border-brand-border rounded-xl overflow-hidden">
              <div className="divide-y divide-brand-border">
                {topMovies.slice(0, 5).map((item, index) => {
                  const topPosterSrc = item.poster_url ? normalizeMediaUrl(item.poster_url) : "";
                  return (
                  <div key={item.slug} className="flex items-center gap-3 p-3 hover:bg-brand-dark/30">
                    <span className="w-6 text-center text-gray-500 text-sm font-medium">{index + 1}</span>
                    {topPosterSrc && (
                      <MediaImage src={topPosterSrc} alt={item.title} className="w-10 h-14 object-cover rounded" />
                    )}
                    <div className="flex-1 min-w-0">
                      <p className="text-white font-medium truncate">{item.title}</p>
                      <p className="text-gray-500 text-xs">{item.views_count?.toLocaleString()} ko'rish</p>
                    </div>
                  </div>
                  );
                })}
              </div>
            </div>
          )}
        </div>

        {/* Top Series */}
        <div>
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-base sm:text-lg font-semibold text-white">Eng ko'p ko'rilgan serialar</h2>
            <Link
              href="/admin/series"
              className="text-sm text-brand-red hover:text-orange-400 transition-colors"
            >
              Hammasini ko'rish
            </Link>
          </div>

          {loading || topSeries.length === 0 ? (
            <div className="bg-brand-card border border-brand-border rounded-xl p-6 text-center">
              <p className="text-gray-500 text-sm">Ma'lumot yo'q</p>
            </div>
          ) : (
            <div className="bg-brand-card border border-brand-border rounded-xl overflow-hidden">
              <div className="divide-y divide-brand-border">
                {topSeries.slice(0, 5).map((item, index) => {
                  const topSeriesPosterSrc = item.poster_url ? normalizeMediaUrl(item.poster_url) : "";
                  return (
                  <div key={item.slug} className="flex items-center gap-3 p-3 hover:bg-brand-dark/30">
                    <span className="w-6 text-center text-gray-500 text-sm font-medium">{index + 1}</span>
                    {topSeriesPosterSrc && (
                      <MediaImage src={topSeriesPosterSrc} alt={item.title} className="w-10 h-14 object-cover rounded" />
                    )}
                    <div className="flex-1 min-w-0">
                      <p className="text-white font-medium truncate">{item.title}</p>
                      <p className="text-gray-500 text-xs">{item.views_count?.toLocaleString()} ko'rish</p>
                    </div>
                  </div>
                  );
                })}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
