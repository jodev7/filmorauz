"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Users, Search, Ban, Unlock, X, Clock, Shield, User, AlertTriangle, ExternalLink } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { getBannedUsers, unbanUser, BannedUser } from "@/lib/api";

export default function AdminBannedUsersPage() {
  const { token, isLoading: authLoading, user } = useAuth();
  const router = useRouter();

  const [bannedUsers, setBannedUsers] = useState<BannedUser[]>([]);
  const [total, setTotal] = useState(0);
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState<"all" | "active" | "expired" | "permanent">("all");
  const [loading, setLoading] = useState(true);
  const [updatingId, setUpdatingId] = useState<string | null>(null);
  const [confirmUnban, setConfirmUnban] = useState<BannedUser | null>(null);

  // Redirect if not admin
  useEffect(() => {
    if (!authLoading && (!token || (user?.role !== "admin" && user?.role !== "superadmin"))) {
      router.push("/");
    }
  }, [authLoading, token, user, router]);

  // Fetch banned users
  useEffect(() => {
    if (!token) return;

    setLoading(true);
    getBannedUsers(token, { search: search || undefined, status })
      .then((data) => {
        setBannedUsers(data.data);
        setTotal(data.total);
      })
      .catch(console.error)
      .finally(() => setLoading(false));
  }, [token, search, status]);

  const handleUnban = async () => {
    if (!token || !confirmUnban) return;
    setUpdatingId(confirmUnban.id);
    try {
      await unbanUser(token, confirmUnban.id);
      setConfirmUnban(null);
      // Refresh list
      const data = await getBannedUsers(token, { search: search || undefined, status });
      setBannedUsers(data.data);
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
  const getDisplayName = (u: BannedUser) => {
    if (u.display_name) return u.display_name;
    if (u.username) return `@${u.username}`;
    if (u.telegram_id) return `ID: ${u.telegram_id}`;
    return "N/A";
  };

  // Get status badge
  const getStatusBadge = (status: string) => {
    switch (status) {
      case "active":
        return { class: "bg-yellow-500/20 text-yellow-400", label: "Faol" };
      case "expired":
        return { class: "bg-green-500/20 text-green-400", label: "Tugagan" };
      case "permanent":
        return { class: "bg-red-500/20 text-red-400", label: "Doimiy" };
      default:
        return { class: "bg-gray-500/20 text-gray-400", label: "Noma'lum" };
    }
  };

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
        <h1 className="text-xl sm:text-2xl font-bold text-white">Ban olgan foydalanuvchilar</h1>
        <p className="text-gray-500 text-sm mt-1">
          Barcha ban olgan foydalanuvchilarni boshqarish
        </p>
      </div>

      {/* Filters */}
      <div className="flex flex-col sm:flex-row gap-4 mb-6">
        {/* Search */}
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" size={18} />
          <input
            type="text"
            placeholder="Username, Telegram ID yoki sabab bo'yicha qidiring..."
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
          <option value="permanent">Doimiy</option>
        </select>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-3 gap-4 mb-6">
        <div className="bg-brand-card border border-brand-border rounded-xl p-4">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-full bg-red-500/20 flex items-center justify-center">
              <Ban className="w-5 h-5 text-red-400" />
            </div>
            <div>
              <p className="text-2xl font-bold text-white">{total}</p>
              <p className="text-xs text-gray-500">Jami ban</p>
            </div>
          </div>
        </div>
        <div className="bg-brand-card border border-brand-border rounded-xl p-4">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-full bg-yellow-500/20 flex items-center justify-center">
              <Clock className="w-5 h-5 text-yellow-400" />
            </div>
            <div>
              <p className="text-2xl font-bold text-white">
                {bannedUsers.filter(u => u.ban_status === "active").length}
              </p>
              <p className="text-xs text-gray-500">Faol ban</p>
            </div>
          </div>
        </div>
        <div className="bg-brand-card border border-brand-border rounded-xl p-4">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-full bg-gray-500/20 flex items-center justify-center">
              <AlertTriangle className="w-5 h-5 text-gray-400" />
            </div>
            <div>
              <p className="text-2xl font-bold text-white">
                {bannedUsers.filter(u => u.ban_status === "permanent").length}
              </p>
              <p className="text-xs text-gray-500">Doimiy ban</p>
            </div>
          </div>
        </div>
      </div>

      {/* Banned Users Table */}
      <div className="bg-brand-card border border-brand-border rounded-xl overflow-hidden">
        {loading ? (
          <div className="p-8 text-center">
            <p className="text-gray-500">Yuklanmoqda...</p>
          </div>
        ) : bannedUsers.length === 0 ? (
          <div className="p-8 text-center">
            <Users size={32} className="text-gray-600 mx-auto mb-3" />
            <p className="text-gray-500">Ban olgan foydalanuvchilar yo'q</p>
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
                  <th className="text-left px-3 sm:px-5 py-3">Holat</th>
                  <th className="text-right px-3 sm:px-5 py-3">Amallar</th>
                </tr>
              </thead>
              <tbody>
                {bannedUsers.map((u) => {
                  const statusBadge = getStatusBadge(u.ban_status);
                  return (
                    <tr
                      key={u.id}
                      className="border-b border-brand-border/50 last:border-0 hover:bg-brand-border/30 transition-colors"
                    >
                      <td className="px-3 sm:px-5 py-3">
                        <div className="min-w-0">
                          <Link
                            href={`/user/${u.id}`}
                            className="text-white font-medium truncate max-w-[150px] hover:text-brand-red transition-colors cursor-pointer flex items-center gap-2"
                          >
                            {getDisplayName(u)}
                            <ExternalLink size={12} className="text-gray-500" />
                          </Link>
                          {u.username && (
                            <p className="text-gray-500 text-xs">@{u.username}</p>
                          )}
                        </div>
                      </td>
                      <td className="px-3 sm:px-5 py-3 hidden md:table-cell text-gray-400 font-mono text-xs">
                        {u.telegram_id || "—"}
                      </td>
                      <td className="px-3 sm:px-5 py-3 hidden lg:table-cell">
                        <div className="flex items-center gap-2">
                          <Shield className="w-4 h-4 text-red-400 flex-shrink-0" />
                          <span className="text-white truncate max-w-[120px]" title={u.ban_reason}>
                            {u.ban_reason || "Sabab ko'rsatilmagan"}
                          </span>
                        </div>
                      </td>
                      <td className="px-3 sm:px-5 py-3 hidden lg:table-cell text-gray-400 text-xs">
                        {formatDate(u.banned_at)}
                      </td>
                      <td className="px-3 sm:px-5 py-3 hidden lg:table-cell text-xs">
                        {u.ban_status === "permanent" ? (
                          <span className="text-red-400">Doimiy</span>
                        ) : u.banned_until ? (
                          <span className="text-gray-400">{formatDate(u.banned_until)}</span>
                        ) : (
                          <span className="text-gray-500">—</span>
                        )}
                      </td>
                      <td className="px-3 sm:px-5 py-3 hidden lg:table-cell">
                        <div className="flex items-center gap-2">
                          <User className="w-4 h-4 text-orange-400 flex-shrink-0" />
                          <span className="text-white truncate max-w-[100px]">
                            {u.banned_by_username || "Noma'lum"}
                          </span>
                        </div>
                      </td>
                      <td className="px-3 sm:px-5 py-3">
                        <span className={`inline-flex items-center gap-1 text-xs px-2 py-1 rounded ${statusBadge.class}`}>
                          {statusBadge.label}
                        </span>
                      </td>
                      <td className="px-3 sm:px-5 py-3 text-right">
                        <div className="flex items-center justify-end gap-2">
                          <Link
                            href={`/user/${u.id}`}
                            className="p-1.5 rounded-lg bg-blue-500/20 text-blue-400 hover:bg-blue-500/30 transition-colors"
                            title="Profilni ko'rish"
                          >
                            <ExternalLink size={14} />
                          </Link>
                          {(u.ban_status === "active" || u.ban_status === "permanent") && (
                            <button
                              onClick={() => setConfirmUnban(u)}
                              disabled={updatingId === u.id}
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

      {/* Unban Confirmation Modal */}
      {confirmUnban && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
          {/* Backdrop */}
          <div 
            className="absolute inset-0 bg-black/70 backdrop-blur-sm"
            onClick={() => setConfirmUnban(null)}
          />
          
          {/* Modal */}
          <div className="relative bg-brand-card border border-brand-border rounded-2xl p-6 w-full max-w-md shadow-2xl">
            <div className="text-center mb-6">
              <div className="inline-flex items-center justify-center w-16 h-16 rounded-full bg-green-500/20 mb-4">
                <Unlock className="w-8 h-8 text-green-400" />
              </div>
              <h2 className="text-xl font-semibold text-white mb-2">
                Banni bekor qilish
              </h2>
              <p className="text-gray-400">
                {getDisplayName(confirmUnban)} foydalanuvchisining banni bekor qilmoqchimisiz?
              </p>
              {confirmUnban.ban_reason && (
                <p className="text-sm text-gray-500 mt-2">
                  Sabab: {confirmUnban.ban_reason}
                </p>
              )}
            </div>

            <div className="flex gap-3">
              <button
                onClick={() => setConfirmUnban(null)}
                className="flex-1 px-4 py-2.5 bg-brand-dark hover:bg-brand-border/50 text-white rounded-lg transition-colors border border-brand-border"
              >
                Bekor qilish
              </button>
              <button
                onClick={handleUnban}
                disabled={updatingId === confirmUnban.id}
                className="flex-1 px-4 py-2.5 bg-green-500 hover:bg-green-600 text-white rounded-lg transition-colors disabled:opacity-50"
              >
                {updatingId === confirmUnban.id ? "Yuklanmoqda..." : "Banni bekor qilish"}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
