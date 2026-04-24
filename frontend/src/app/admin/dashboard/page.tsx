"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Film, PlusCircle, List, ExternalLink, Users, UserPlus, Share2, Eye, Star, Tv } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { adminGetMovies, Movie, getAdminDashboardStats, getAdminShareStats, getAdminUserMetrics, getAdminTopMovies, getAdminTopSeries, DashboardStats, AdminShareStats, UserMetrics, TopContentItem } from "@/lib/api";
import { normalizeMediaUrl } from "@/lib/image-utils";
import MediaImage from "@/components/ui/MediaImage";

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
  const getUserDisplayName = (u: { display_name?: string; username?: string; telegram_id?: number }) => {
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
