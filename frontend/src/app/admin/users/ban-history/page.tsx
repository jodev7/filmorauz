"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { History, Search, Shield, Clock, User, ExternalLink, Ban, Unlock, RotateCcw } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { getBanHistory, unbanUser, BanHistoryRecord } from "@/lib/api";

export default function AdminBanHistoryPage() {
  const { token, isLoading: authLoading, user } = useAuth();
  const router = useRouter();

  const [history, setHistory] = useState<BanHistoryRecord[]>([]);
  const [total, setTotal] = useState(0);
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState<"all" | "active" | "unbanned" | "expired">("all");
  const [loading, setLoading] = useState(true);
  const [updatingId, setUpdatingId] = useState<string | null>(null);

  // Redirect if not admin
  useEffect(() => {
    if (!authLoading && (!token || (user?.role !== "admin" && user?.role !== "superadmin"))) {
      router.push("/");
    }
  }, [authLoading, token, user, router]);

  // Fetch ban history
  useEffect(() => {
    if (!token) return;

    setLoading(true);
    getBanHistory(token, { search: search || undefined, status })
      .then((data) => {
        setHistory(data.data);
        setTotal(data.total);
      })
      .catch(console.error)
      .finally(() => setLoading(false));
  }, [token, search, status]);

  const handleUnban = async (userId: string) => {
    if (!token) return;
    setUpdatingId(userId);
    try {
      await unbanUser(token, userId);
      // Refresh list
      const data = await getBanHistory(token, { search: search || undefined, status });
      setHistory(data.data);
      setTotal(data.total);
    } catch (error) {
      console.error("Failed to unban user:", error);
    } finally {
      setUpdatingId(null);
    }
  };

  // Format date
  const formatDate = (dateStr: string) => {
    if (!dateStr) return "—";
    const d = new Date(dateStr);
    return d.toLocaleDateString("uz-UZ", {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  };

  // Get display name
  const getDisplayName = (u: BanHistoryRecord) => {
    if (u.user_display_name) return u.user_display_name;
    if (u.user_username) return `@${u.user_username}`;
    if (u.user_telegram_id) return `ID: ${u.user_telegram_id}`;
    return "N/A";
  };

  // Get status badge
  const getStatusBadge = (status: string) => {
    switch (status) {
      case "active":
        return { class: "bg-yellow-500/20 text-yellow-400", label: "Faol" };
      case "unbanned":
        return { class: "bg-green-500/20 text-green-400", label: "Bekor qilingan" };
      case "expired":
        return { class: "bg-gray-500/20 text-gray-400", label: "Tugagan" };
      default:
        return { class: "bg-gray-500/20 text-gray-400", label: "Noma'lum" };
    }
  };

  // Stats
  const activeCount = history.filter(h => h.status === "active").length;
  const unbannedCount = history.filter(h => h.status === "unbanned").length;
  const expiredCount = history.filter(h => h.status === "expired").length;

  if (authLoading) {
    return (
      <div className="min-h-screen bg-brand-dark flex items-center justify-center">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-brand-red"></div>
      </div>
    );
  }

  if (!token || (user?.role !== "admin" && user?.role !== "superadmin")) {
    return null;
  }

  return (
    <div className="p-4 sm:p-8">
      <div className="mb-6 sm:mb-8">
        <h1 className="text-xl sm:text-2xl font-bold text-white">Ban tarixi</h1>
        <p className="text-gray-500 text-sm mt-1">
          Barcha ban va unbun amallarining tarixi
        </p>
      </div>

      {/* Filters */}
      <div className="flex flex-col sm:flex-row gap-4 mb-6">
        {/* Search */}
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" size={18} />
          <input
            type="text"
            placeholder="Foydalanuvchi, sabab yoki admin bo'yicha qidiring..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full bg-brand-card border border-brand-border rounded-lg pl-10 pr-4 py-2 text-white placeholder-gray-500 focus:outline-none focus:border-brand-red"
          />
        </div>

        {/* Status Filter */}
        <select
          value={status}
          onChange={(e) => setStatus(e.target.value as any)}
          className="bg-brand-card border border-brand-border rounded-lg px-4 py-2 text-white focus:outline-none focus:border-brand-red"
        >
          <option value="all">Barchasi</option>
          <option value="active">Faol</option>
          <option value="expired">Tugagan</option>
          <option value="unbanned">Bekor qilingan</option>
        </select>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-3 gap-4 mb-6">
        <div className="bg-brand-card border border-brand-border rounded-xl p-4">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-full bg-yellow-500/20 flex items-center justify-center">
              <Clock className="w-5 h-5 text-yellow-400" />
            </div>
            <div>
              <p className="text-2xl font-bold text-white">{activeCount}</p>
              <p className="text-xs text-gray-500">Faol ban</p>
            </div>
          </div>
        </div>
        <div className="bg-brand-card border border-brand-border rounded-xl p-4">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-full bg-green-500/20 flex items-center justify-center">
              <Unlock className="w-5 h-5 text-green-400" />
            </div>
            <div>
              <p className="text-2xl font-bold text-white">{unbannedCount}</p>
              <p className="text-xs text-gray-500">Bekor qilingan</p>
            </div>
          </div>
        </div>
        <div className="bg-brand-card border border-brand-border rounded-xl p-4">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-full bg-gray-500/20 flex items-center justify-center">
              <History className="w-5 h-5 text-gray-400" />
            </div>
            <div>
              <p className="text-2xl font-bold text-white">{expiredCount}</p>
              <p className="text-xs text-gray-500">Tugagan</p>
            </div>
          </div>
        </div>
      </div>

      {/* Ban History Table */}
      <div className="bg-brand-card border border-brand-border rounded-xl overflow-hidden">
        {loading ? (
          <div className="p-8 text-center">
            <p className="text-gray-500">Yuklanmoqda...</p>
          </div>
        ) : history.length === 0 ? (
          <div className="p-8 text-center">
            <History size={32} className="text-gray-600 mx-auto mb-3" />
            <p className="text-gray-500">Ban tarixi yo'q</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-brand-border text-gray-500 text-xs uppercase tracking-wider">
                  <th className="text-left px-3 sm:px-5 py-3">Foydalanuvchi</th>
                  <th className="text-left px-3 sm:px-5 py-3 hidden md:table-cell">Telegram ID</th>
                  <th className="text-left px-3 sm:px-5 py-3 hidden lg:table-cell">Sabab</th>
                  <th className="text-left px-3 sm:px-5 py-3 hidden lg:table-cell">Ban boshlangan</th>
                  <th className="text-left px-3 sm:px-5 py-3 hidden lg:table-cell">Ban tugaydi</th>
                  <th className="text-left px-3 sm:px-5 py-3 hidden lg:table-cell">Admin</th>
                  <th className="text-left px-3 sm:px-5 py-3 hidden lg:table-cell">Bekor qiluvchi</th>
                  <th className="text-left px-3 sm:px-5 py-3">Holat</th>
                  <th className="text-right px-3 sm:px-5 py-3">Amallar</th>
                </tr>
              </thead>
              <tbody>
                {history.map((h) => {
                  const statusBadge = getStatusBadge(h.status);
                  return (
                    <tr
                      key={h.id}
                      className="border-b border-brand-border/50 last:border-0 hover:bg-brand-border/30 transition-colors"
                    >
                      <td className="px-3 sm:px-5 py-3">
                        <div className="min-w-0">
                          <Link
                            href={`/user/${h.user_id}`}
                            className="text-white font-medium truncate max-w-[150px] hover:text-brand-red transition-colors cursor-pointer flex items-center gap-2"
                          >
                            {getDisplayName(h)}
                            <ExternalLink size={12} className="text-gray-500" />
                          </Link>
                          {h.user_username && (
                            <p className="text-gray-500 text-xs">@{h.user_username}</p>
                          )}
                        </div>
                      </td>
                      <td className="px-3 sm:px-5 py-3 hidden md:table-cell text-gray-400 font-mono text-xs">
                        {h.user_telegram_id || "—"}
                      </td>
                      <td className="px-3 sm:px-5 py-3 hidden lg:table-cell">
                        <div className="flex items-center gap-2">
                          <Shield className="w-4 h-4 text-red-400 flex-shrink-0" />
                          <span className="text-white truncate max-w-[100px]" title={h.reason}>
                            {h.reason || "Sabab ko'rsatilmagan"}
                          </span>
                        </div>
                      </td>
                      <td className="px-3 sm:px-5 py-3 hidden lg:table-cell text-gray-400 text-xs">
                        {formatDate(h.banned_at)}
                      </td>
                      <td className="px-3 sm:px-5 py-3 hidden lg:table-cell text-xs">
                        {h.is_permanent ? (
                          <span className="text-red-400">Doimiy</span>
                        ) : h.banned_until ? (
                          <span className="text-gray-400">{formatDate(h.banned_until)}</span>
                        ) : (
                          <span className="text-gray-500">—</span>
                        )}
                      </td>
                      <td className="px-3 sm:px-5 py-3 hidden lg:table-cell">
                        <div className="flex items-center gap-2">
                          <User className="w-4 h-4 text-orange-400 flex-shrink-0" />
                          <span className="text-white truncate max-w-[80px]">
                            {h.banned_by_username || "Noma'lum"}
                          </span>
                        </div>
                      </td>
                      <td className="px-3 sm:px-5 py-3 hidden lg:table-cell">
                        {h.unbanned_by_username ? (
                          <div className="flex items-center gap-2">
                            <User className="w-4 h-4 text-green-400 flex-shrink-0" />
                            <span className="text-white truncate max-w-[80px]">
                              {h.unbanned_by_username}
                            </span>
                          </div>
                        ) : (
                          <span className="text-gray-500">—</span>
                        )}
                      </td>
                      <td className="px-3 sm:px-5 py-3">
                        <span className={`inline-flex items-center gap-1 text-xs px-2 py-1 rounded ${statusBadge.class}`}>
                          {statusBadge.label}
                        </span>
                      </td>
                      <td className="px-3 sm:px-5 py-3 text-right">
                        <div className="flex items-center justify-end gap-2">
                          <Link
                            href={`/user/${h.user_id}`}
                            className="p-1.5 rounded-lg bg-blue-500/20 text-blue-400 hover:bg-blue-500/30 transition-colors"
                            title="Profilni ko'rish"
                          >
                            <ExternalLink size={14} />
                          </Link>
                          {h.status === "active" && (
                            <button
                              onClick={() => handleUnban(h.user_id)}
                              disabled={updatingId === h.user_id}
                              className="p-1.5 rounded-lg bg-green-500/20 text-green-400 hover:bg-green-500/30 transition-colors disabled:opacity-50"
                              title="Banni bekor qilish"
                            >
                              <Unlock size={14} />
                            </button>
                          )}
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
