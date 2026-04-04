"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Users, Search, ChevronLeft, ChevronRight, User, Shield, Crown, Ban, Unlock, X, ShieldCheck } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { getAdminUsers, updateAdminUserRole, updateAdminUserPremium, banUser, unbanUser, AdminUser } from "@/lib/api";

// Check if a user is SuperAdmin (case-insensitive)
const isSuperAdmin = (role: string): boolean => {
  return role?.toLowerCase() === "superadmin";
};

export default function AdminUsersPage() {
  const { token, isLoading: authLoading, user } = useAuth();
  const router = useRouter();

  const [users, setUsers] = useState<AdminUser[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [limit] = useState(20);
  const [totalPages, setTotalPages] = useState(0);
  const [search, setSearch] = useState("");
  const [role, setRole] = useState("all");
  const [loading, setLoading] = useState(true);
  const [updatingId, setUpdatingId] = useState<string | null>(null);
  const [showBanModal, setShowBanModal] = useState(false);
  const [banTargetUser, setBanTargetUser] = useState<AdminUser | null>(null);
  const [banDuration, setBanDuration] = useState(1);
  const [banReason, setBanReason] = useState("");
  const [customBanReason, setCustomBanReason] = useState("");
  const [banning, setBanning] = useState(false);

  // Redirect if not admin
  useEffect(() => {
    if (!authLoading && (!token || (user?.role !== "admin" && user?.role !== "superadmin"))) {
      router.push("/");
    }
  }, [authLoading, token, user, router]);

  // Fetch users
  useEffect(() => {
    if (!token) return;

    setLoading(true);
    getAdminUsers(token, { page, limit, search: search || undefined, role: role !== "all" ? role : undefined })
      .then((data) => {
        setUsers(data.data);
        setTotal(data.total);
        setTotalPages(data.total_pages);
      })
      .catch(console.error)
      .finally(() => setLoading(false));
  }, [token, page, search, role]);

  const handleRoleChange = async (userId: string, newRole: string) => {
    if (!token) return;
    setUpdatingId(userId);
    try {
      await updateAdminUserRole(token, userId, newRole);
      // Refresh users - use a small delay to ensure DB update is visible
      await new Promise(resolve => setTimeout(resolve, 100));
      const data = await getAdminUsers(token, { page, limit, search: search || undefined, role: role !== "all" ? role : undefined });
      setUsers(data.data);
    } catch (error) {
      console.error("Failed to update role:", error);
    } finally {
      setUpdatingId(null);
    }
  };

  const handlePremiumChange = async (userId: string, isPremium: boolean, durationDays?: number | null) => {
    if (!token || user?.role !== "superadmin") return;
    setUpdatingId(userId);
    try {
      await updateAdminUserPremium(token, userId, isPremium, null, durationDays);
      // Refresh users - use a small delay to ensure DB update is visible
      await new Promise(resolve => setTimeout(resolve, 100));
      const data = await getAdminUsers(token, { page, limit, search: search || undefined, role: role !== "all" ? role : undefined });
      setUsers(data.data);
    } catch (error) {
      console.error("Failed to update premium:", error);
    } finally {
      setUpdatingId(null);
    }
  };

  const handleBanUser = async () => {
    if (!token || !banTargetUser) return;
    
    // Use custom reason if "Boshqa sabab" is selected
    const finalReason = banReason === "Boshqa sabab" ? customBanReason : banReason;
    
    if (!finalReason.trim()) {
      return;
    }
    
    setBanning(true);
    try {
      await banUser(token, banTargetUser.id, banDuration, finalReason);
      setShowBanModal(false);
      setBanTargetUser(null);
      setBanReason("");
      setCustomBanReason("");
      setBanDuration(1);
      // Refresh users
      await new Promise(resolve => setTimeout(resolve, 100));
      const data = await getAdminUsers(token, { page, limit, search: search || undefined, role: role !== "all" ? role : undefined });
      setUsers(data.data);
    } catch (error) {
      console.error("Failed to ban user:", error);
    } finally {
      setBanning(false);
    }
  };

  const handleUnbanUser = async (userId: string) => {
    if (!token) return;
    setUpdatingId(userId);
    try {
      await unbanUser(token, userId);
      // Refresh users
      await new Promise(resolve => setTimeout(resolve, 100));
      const data = await getAdminUsers(token, { page, limit, search: search || undefined, role: role !== "all" ? role : undefined });
      setUsers(data.data);
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
    return d.toLocaleDateString("uz-UZ");
  };

  // Get display name
  const getDisplayName = (u: AdminUser) => {
    if (u.display_name) return u.display_name;
    if (u.username) return `@${u.username}`;
    if (u.telegram_id) return `ID: ${u.telegram_id}`;
    return "N/A";
  };

  // Get role badge color
  const getRoleBadgeColor = (r: string) => {
    switch (r) {
      case "superadmin": return "bg-red-500/20 text-red-400";
      case "admin": return "bg-orange-500/20 text-orange-400";
      default: return "bg-green-500/20 text-green-400";
    }
  };

  // Get role icon
  const getRoleIcon = (r: string) => {
    switch (r) {
      case "superadmin": return <Crown size={14} />;
      case "admin": return <Shield size={14} />;
      default: return <User size={14} />;
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
        <h1 className="text-xl sm:text-2xl font-bold text-white">Foydalanuvchilar</h1>
        <p className="text-gray-500 text-sm mt-1">
          Barcha foydalanuvchilarni boshqarish
        </p>
      </div>

      {/* Filters */}
      <div className="flex flex-col sm:flex-row gap-4 mb-6">
        {/* Search */}
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" size={18} />
          <input
            type="text"
            placeholder="Username, Telegram ID yoki MongoDB ID bo'yicha qidiring..."
            value={search}
            onChange={(e) => {
              setSearch(e.target.value);
              setPage(1);
            }}
            className="w-full bg-brand-card border border-brand-border rounded-lg pl-10 pr-4 py-2 text-white placeholder-gray-500 focus:outline-none focus:border-brand-red"
          />
        </div>

        {/* Role Filter */}
        <select
          value={role}
          onChange={(e) => {
            setRole(e.target.value);
            setPage(1);
          }}
          className="bg-brand-card border border-brand-border rounded-lg px-4 py-2 text-white focus:outline-none focus:border-brand-red"
        >
          <option value="all">Barcha rollar</option>
          <option value="user">Foydalanuvchi</option>
          <option value="admin">Admin</option>
          <option value="superadmin">Super Admin</option>
        </select>
      </div>

      {/* Users Table */}
      <div className="bg-brand-card border border-brand-border rounded-xl overflow-hidden">
        {loading ? (
          <div className="p-8 text-center">
            <p className="text-gray-500">Yuklanmoqda...</p>
          </div>
        ) : users.length === 0 ? (
          <div className="p-8 text-center">
            <Users size={32} className="text-gray-600 mx-auto mb-3" />
            <p className="text-gray-500">Foydalanuvchilar topilmadi</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-brand-border text-gray-500 text-xs uppercase tracking-wider">
                  <th className="text-left px-3 sm:px-5 py-3">Foydalanuvchi</th>
                  <th className="text-left px-3 sm:px-5 py-3 hidden md:table-cell">Telegram ID</th>
                  <th className="text-left px-3 sm:px-5 py-3 hidden sm:table-cell">Premium</th>
                  <th className="text-left px-3 sm:px-5 py-3 hidden sm:table-cell">Rol</th>
                  <th className="text-left px-3 sm:px-5 py-3 hidden lg:table-cell">Ro'yxatdan o'tgan</th>
                  <th className="text-left px-3 sm:px-5 py-3 hidden lg:table-cell">Oxirgi kirish</th>
                  <th className="text-right px-3 sm:px-5 py-3">Amallar</th>
                </tr>
              </thead>
              <tbody>
                {users.map((u) => (
                  <tr
                    key={u.id}
                    className="border-b border-brand-border/50 last:border-0 hover:bg-brand-border/30 transition-colors"
                  >
                    <td className="px-3 sm:px-5 py-3">
                      <div className="min-w-0">
                        <Link
                          href={`/user/${u.id}`}
                          className="text-white font-medium truncate max-w-[150px] hover:text-brand-red transition-colors cursor-pointer"
                        >
                          {getDisplayName(u)}
                        </Link>
                        {u.username && (
                          <p className="text-gray-500 text-xs">@{u.username}</p>
                        )}
                      </div>
                    </td>
                    <td className="px-3 sm:px-5 py-3 hidden md:table-cell text-gray-400 font-mono text-xs">
                      {u.telegram_id || "—"}
                    </td>
                    <td className="px-3 sm:px-5 py-3 hidden sm:table-cell">
                      {user?.role === "superadmin" ? (
                        <div className="flex flex-col gap-1">
                          {/* Premium status badge */}
                          <select
                            value={u.is_premium ? "true" : "false"}
                            onChange={(e) => handlePremiumChange(u.id, e.target.value === "true", null)}
                            disabled={updatingId === u.id}
                            className={`text-xs px-2 py-1 rounded border-0 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed ${
                              u.is_premium 
                                ? "bg-yellow-500/20 text-yellow-400" 
                                : "bg-gray-700 text-gray-400"
                            }`}
                          >
                            <option value="false">Free</option>
                            <option value="true">Premium</option>
                          </select>
                          {/* Duration dropdown for extending or assigning premium */}
                          {u.is_premium ? (
                            <>
                              <select
                                onChange={(e) => handlePremiumChange(u.id, true, parseInt(e.target.value))}
                                disabled={updatingId === u.id}
                                className="text-xs px-2 py-1 rounded border border-yellow-500/30 bg-yellow-500/10 text-yellow-400 cursor-pointer disabled:opacity-50"
                                defaultValue=""
                              >
                                <option value="" disabled>— Extend —</option>
                                <option value="1">+1 day</option>
                                <option value="30">+30 days</option>
                                <option value="90">+90 days</option>
                                <option value="180">+180 days</option>
                                <option value="365">+365 days</option>
                              </select>
                              {u.premium_expires_at && (
                                <span className="text-gray-500 text-xs">
                                  Until: {formatDate(u.premium_expires_at)}
                                </span>
                              )}
                            </>
                          ) : (
                            <select
                              onChange={(e) => handlePremiumChange(u.id, true, parseInt(e.target.value))}
                              disabled={updatingId === u.id}
                              className="text-xs px-2 py-1 rounded border border-yellow-500/30 bg-yellow-500/10 text-yellow-400 cursor-pointer disabled:opacity-50"
                              defaultValue=""
                            >
                              <option value="" disabled>— Assign —</option>
                              <option value="1">1 day</option>
                              <option value="30">30 days</option>
                              <option value="90">90 days</option>
                              <option value="180">180 days</option>
                              <option value="365">365 days</option>
                            </select>
                          )}
                        </div>
                      ) : (
                        u.is_premium ? (
                          <div className="flex flex-col gap-1">
                            <span className="inline-flex items-center gap-1 text-xs px-2 py-1 rounded bg-yellow-500/20 text-yellow-400">
                              Premium
                            </span>
                            {u.premium_expires_at && (
                              <span className="text-gray-500 text-xs">
                                Until: {formatDate(u.premium_expires_at)}
                              </span>
                            )}
                          </div>
                        ) : (
                          <span className="text-gray-500 text-xs">—</span>
                        )
                      )}
                    </td>
                    <td className="px-3 sm:px-5 py-3 hidden sm:table-cell">
                      {user?.role === "superadmin" ? (
                        isSuperAdmin(u.role) ? (
                          // SuperAdmin user - show protected badge, no dropdown
                          <span className="inline-flex items-center gap-1 text-xs px-2 py-1 rounded bg-yellow-500/20 text-yellow-400">
                            <ShieldCheck size={12} />
                            Super Admin
                          </span>
                        ) : (
                          <div className="flex items-center gap-2">
                            <select
                              value={u.role}
                              onChange={(e) => handleRoleChange(u.id, e.target.value)}
                              disabled={updatingId === u.id}
                              className={`text-xs px-2 py-1 rounded border-0 cursor-pointer ${getRoleBadgeColor(u.role)} disabled:opacity-50 disabled:cursor-not-allowed`}
                            >
                              <option value="user">Foydalanuvchi</option>
                              <option value="admin">Admin</option>
                              <option value="superadmin">Super Admin</option>
                            </select>
                          </div>
                        )
                      ) : (
                        <span className={`inline-flex items-center gap-1 text-xs px-2 py-1 rounded ${getRoleBadgeColor(u.role)}`}>
                          {getRoleIcon(u.role)}
                          {u.role === "superadmin" ? "Super Admin" : u.role === "admin" ? "Admin" : "Foydalanuvchi"}
                        </span>
                      )}
                    </td>
                    <td className="px-3 sm:px-5 py-3 hidden lg:table-cell text-gray-400 text-xs">
                      {formatDate(u.created_at)}
                    </td>
                    <td className="px-3 sm:px-5 py-3 hidden lg:table-cell text-gray-400 text-xs">
                      {formatDate(u.last_login_at)}
                    </td>
                    <td className="px-3 sm:px-5 py-3 text-right">
                      <div className="flex items-center justify-end gap-2">
                        {isSuperAdmin(u.role) ? (
                          // SuperAdmin - show protected indicator, no actions
                          <span className="text-yellow-400 text-xs flex items-center gap-1" title="Himoyalangan hisob">
                            <ShieldCheck size={12} />
                            Himoyalangan
                          </span>
                        ) : u.is_banned ? (
                          <button
                            onClick={() => handleUnbanUser(u.id)}
                            disabled={updatingId === u.id}
                            className="p-1.5 rounded-lg bg-green-500/20 text-green-400 hover:bg-green-500/30 transition-colors disabled:opacity-50"
                            title="Banni bekor qilish"
                          >
                            <Unlock size={14} />
                          </button>
                        ) : (
                          <button
                            onClick={() => {
                              setBanTargetUser(u);
                              setShowBanModal(true);
                            }}
                            disabled={updatingId === u.id}
                            className="p-1.5 rounded-lg bg-red-500/20 text-red-400 hover:bg-red-500/30 transition-colors disabled:opacity-50"
                            title="Foydalanuvchini ban qilish"
                          >
                            <Ban size={14} />
                          </button>
                        )}
                        <span className="text-gray-500 text-xs">
                          {u.auth_provider === "telegram" ? "Telegram" : u.auth_provider}
                        </span>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="flex items-center justify-between mt-4">
          <p className="text-gray-500 text-sm">
            Jami: {total} foydalanuvchi
          </p>
          <div className="flex items-center gap-2">
            <button
              onClick={() => setPage(p => Math.max(1, p - 1))}
              disabled={page === 1}
              className="p-2 rounded-lg bg-brand-card border border-brand-border text-white disabled:opacity-50 disabled:cursor-not-allowed hover:bg-brand-border transition-colors"
            >
              <ChevronLeft size={18} />
            </button>
            <span className="text-white text-sm px-2">
              {page} / {totalPages}
            </span>
            <button
              onClick={() => setPage(p => Math.min(totalPages, p + 1))}
              disabled={page === totalPages}
              className="p-2 rounded-lg bg-brand-card border border-brand-border text-white disabled:opacity-50 disabled:cursor-not-allowed hover:bg-brand-border transition-colors"
            >
              <ChevronRight size={18} />
            </button>
          </div>
        </div>
      )}

      {/* Ban Modal */}
      {showBanModal && banTargetUser && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
          {/* Backdrop */}
          <div 
            className="absolute inset-0 bg-black/70 backdrop-blur-sm"
            onClick={() => {
              setShowBanModal(false);
              setBanTargetUser(null);
              setBanReason("");
              setCustomBanReason("");
              setBanDuration(1);
            }}
          />
          
          {/* Modal */}
          <div className="relative bg-brand-card border border-brand-border rounded-2xl p-6 w-full max-w-md shadow-2xl">
            {/* Header */}
            <div className="flex items-center justify-between mb-6">
              <div className="flex items-center gap-3">
                <div className="w-10 h-10 rounded-full bg-red-500/20 flex items-center justify-center">
                  <Ban className="w-5 h-5 text-red-400" />
                </div>
                <div>
                  <h2 className="text-lg font-semibold text-white">Foydalanuvchini ban qilish</h2>
                  <p className="text-sm text-gray-400">
                    {getDisplayName(banTargetUser)}
                  </p>
                </div>
              </div>
              <button
                onClick={() => {
                  setShowBanModal(false);
                  setBanTargetUser(null);
                  setBanReason("");
                  setCustomBanReason("");
                  setBanDuration(1);
                }}
                className="p-2 rounded-lg hover:bg-brand-border/50 transition-colors text-gray-400 hover:text-white"
              >
                <X size={20} />
              </button>
            </div>

            {/* Form */}
            <div className="space-y-4">
              {/* Duration */}
              <div>
                <label className="block text-sm text-gray-400 mb-2">
                  Muddat <span className="text-red-400">*</span>
                </label>
                <select
                  value={banDuration}
                  onChange={(e) => setBanDuration(parseInt(e.target.value))}
                  className="w-full bg-brand-dark border border-brand-border rounded-lg px-4 py-2.5 text-white focus:outline-none focus:border-brand-red"
                >
                  <option value={1}>1 kun</option>
                  <option value={3}>3 kun</option>
                  <option value={7}>7 kun</option>
                  <option value={30}>30 kun</option>
                  <option value={0}>Doimiy</option>
                </select>
                {banDuration === 0 && (
                  <p className="text-xs text-red-400 mt-1">
                    ⚠️ Foydalanuvchi doimiy bloklanadi
                  </p>
                )}
              </div>

              {/* Reason */}
              <div>
                <label className="block text-sm text-gray-400 mb-2">
                  Sabab <span className="text-red-400">*</span>
                </label>
                <select
                  value={banReason}
                  onChange={(e) => setBanReason(e.target.value)}
                  className="w-full bg-brand-dark border border-brand-border rounded-lg px-4 py-2.5 text-white focus:outline-none focus:border-brand-red"
                >
                  <option value="">Sababni tanlang...</option>
                  <option value="Spam">Spam</option>
                  <option value="Haqoratli xulq-atvor">Haqoratli xulq-atvor</option>
                  <option value="Noqonuniy faoliyat">Noqonuniy faoliyat</option>
                  <option value="Qoidabuzarlik">Qoidabuzarlik</option>
                  <option value="Boshqa sabab">Boshqa sabab</option>
                </select>
                
                {/* Custom reason input - separate from the select */}
                {banReason === "Boshqa sabab" && (
                  <input
                    type="text"
                    value={customBanReason}
                    onChange={(e) => setCustomBanReason(e.target.value)}
                    placeholder="Sababni kiriting..."
                    className="w-full mt-2 bg-brand-dark border border-brand-border rounded-lg px-4 py-2.5 text-white placeholder-gray-500 focus:outline-none focus:border-brand-red"
                    autoFocus
                  />
                )}
              </div>

              {/* Warning */}
              <div className="p-3 bg-yellow-500/10 border border-yellow-500/30 rounded-lg">
                <p className="text-yellow-400 text-sm">
                  Ban muvaffaqiyatli amalga oshirilganda, foydalanuvchi saytdan to'liq bloklanadi.
                </p>
              </div>
            </div>

            {/* Actions */}
            <div className="flex gap-3 mt-6">
              <button
                onClick={() => {
                  setShowBanModal(false);
                  setBanTargetUser(null);
                  setBanReason("");
                  setCustomBanReason("");
                  setBanDuration(1);
                }}
                className="flex-1 px-4 py-2.5 bg-brand-dark hover:bg-brand-border/50 text-white rounded-lg transition-colors border border-brand-border"
              >
                Bekor qilish
              </button>
              <button
                onClick={handleBanUser}
                disabled={banning || !banReason || (banReason === "Boshqa sabab" && !customBanReason.trim())}
                className="flex-1 px-4 py-2.5 bg-red-500 hover:bg-red-600 text-white rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {banning ? "Yuklanmoqda..." : "Ban qilish"}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
