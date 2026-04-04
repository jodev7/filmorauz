"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth-context";
import { getAppeals, getAppealStats, reviewAppeal, BanAppeal } from "@/lib/api";
import { 
  MessageSquare, Search, Filter, CheckCircle, XCircle, Clock, 
  ChevronLeft, ChevronRight, RefreshCw, Check, X, Eye, Ban,
  User, Calendar, AlertTriangle, Loader2, ExternalLink
} from "lucide-react";
import Link from "next/link";

export default function AdminAppealsPage() {
  const { user, token, isLoading: authLoading } = useAuth();
  const router = useRouter();
  
  const [appeals, setAppeals] = useState<BanAppeal[]>([]);
  const [stats, setStats] = useState({ pending: 0, approved: 0, rejected: 0, total: 0 });
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState("all");
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [total, setTotal] = useState(0);
  
  // Review modal state
  const [showReviewModal, setShowReviewModal] = useState(false);
  const [selectedAppeal, setSelectedAppeal] = useState<BanAppeal | null>(null);
  const [reviewAction, setReviewAction] = useState<"approve" | "reject">("reject");
  const [adminNote, setAdminNote] = useState("");
  const [unbanUser, setUnbanUser] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [submitSuccess, setSubmitSuccess] = useState(false);

  // Auth check
  useEffect(() => {
    if (authLoading) return;
    
    if (!token || !user) {
      router.push("/admin/login");
      return;
    }
    
    if (user.role !== "admin" && user.role !== "superadmin") {
      router.push("/");
      return;
    }
    
    loadData();
  }, [authLoading, token, user, router]);

  // Reload when filters change
  useEffect(() => {
    if (!token) return;
    loadData();
  }, [page, statusFilter, search, token]);

  const loadData = async () => {
    if (!token) return;
    
    setLoading(true);
    try {
      const [appealsData, statsData] = await Promise.all([
        getAppeals(token, { 
          page, 
          per_page: 10, 
          status: statusFilter,
          search: search || undefined 
        }),
        getAppealStats(token)
      ]);
      
      setAppeals(appealsData.appeals);
      setTotalPages(appealsData.total_pages);
      setTotal(appealsData.total);
      setStats(statsData.stats);
    } catch (error) {
      console.error("Failed to load appeals:", error);
    } finally {
      setLoading(false);
    }
  };

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault();
    setPage(1);
    loadData();
  };

  const openReviewModal = (appeal: BanAppeal) => {
    setSelectedAppeal(appeal);
    setReviewAction(appeal.status === "pending" ? "approve" : "reject");
    setAdminNote("");
    setUnbanUser(true);
    setShowReviewModal(true);
    setSubmitError(null);
    setSubmitSuccess(false);
  };

  const handleReview = async () => {
    if (!token || !selectedAppeal) return;
    
    setSubmitting(true);
    setSubmitError(null);
    
    try {
      const result = await reviewAppeal(token, selectedAppeal.id, {
        action: reviewAction,
        admin_note: adminNote || undefined,
        unban_user: reviewAction === "approve" ? unbanUser : false
      });
      
      setSubmitSuccess(true);
      
      // Close modal and reload after delay
      setTimeout(() => {
        setShowReviewModal(false);
        loadData();
      }, 1500);
    } catch (error: any) {
      setSubmitError(error.message || "Xatolik yuz berdi");
    } finally {
      setSubmitting(false);
    }
  };

  const formatDate = (dateString: string) => {
    if (!dateString) return "—";
    const date = new Date(dateString);
    return date.toLocaleDateString("uz-UZ", {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit"
    });
  };

  const getStatusBadge = (status: string) => {
    switch (status) {
      case "pending":
        return (
          <span className="inline-flex items-center gap-1 px-2 py-1 bg-yellow-500/20 text-yellow-400 text-xs font-medium rounded">
            <Clock className="w-3 h-3" />
            Ko'rib chiqilmoqda
          </span>
        );
      case "approved":
        return (
          <span className="inline-flex items-center gap-1 px-2 py-1 bg-green-500/20 text-green-400 text-xs font-medium rounded">
            <CheckCircle className="w-3 h-3" />
            Qabul qilindi
          </span>
        );
      case "rejected":
        return (
          <span className="inline-flex items-center gap-1 px-2 py-1 bg-red-500/20 text-red-400 text-xs font-medium rounded">
            <XCircle className="w-3 h-3" />
            Rad etildi
          </span>
        );
      default:
        return null;
    }
  };

  if (authLoading) {
    return (
      <div className="min-h-screen bg-brand-dark flex items-center justify-center">
        <div className="animate-spin w-10 h-10 border-3 border-brand-red border-t-transparent rounded-full" />
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-brand-dark p-6">
      <div className="max-w-7xl mx-auto">
        {/* Header */}
        <div className="flex items-center justify-between mb-8">
          <div>
            <h1 className="text-3xl font-display text-white">Apellyatsiyalar</h1>
            <p className="text-gray-400 mt-1">Foydalanuvchilarning ban apellyatsiyalarini ko'rib chiqish</p>
          </div>
          <button
            onClick={loadData}
            className="flex items-center gap-2 px-4 py-2 bg-brand-card hover:bg-brand-border text-white rounded-lg transition-colors"
          >
            <RefreshCw className={`w-4 h-4 ${loading ? "animate-spin" : ""}`} />
            Yangilash
          </button>
        </div>

        {/* Stats */}
        <div className="grid grid-cols-4 gap-4 mb-8">
          <div className="bg-brand-card border border-brand-border rounded-xl p-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 rounded-lg bg-yellow-500/20 flex items-center justify-center">
                <Clock className="w-5 h-5 text-yellow-400" />
              </div>
              <div>
                <p className="text-gray-400 text-sm">Kutilmoqda</p>
                <p className="text-white text-2xl font-bold">{stats.pending}</p>
              </div>
            </div>
          </div>
          
          <div className="bg-brand-card border border-brand-border rounded-xl p-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 rounded-lg bg-green-500/20 flex items-center justify-center">
                <CheckCircle className="w-5 h-5 text-green-400" />
              </div>
              <div>
                <p className="text-gray-400 text-sm">Qabul qilingan</p>
                <p className="text-white text-2xl font-bold">{stats.approved}</p>
              </div>
            </div>
          </div>
          
          <div className="bg-brand-card border border-brand-border rounded-xl p-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 rounded-lg bg-red-500/20 flex items-center justify-center">
                <XCircle className="w-5 h-5 text-red-400" />
              </div>
              <div>
                <p className="text-gray-400 text-sm">Rad etilgan</p>
                <p className="text-white text-2xl font-bold">{stats.rejected}</p>
              </div>
            </div>
          </div>
          
          <div className="bg-brand-card border border-brand-border rounded-xl p-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 rounded-lg bg-blue-500/20 flex items-center justify-center">
                <MessageSquare className="w-5 h-5 text-blue-400" />
              </div>
              <div>
                <p className="text-gray-400 text-sm">Jami</p>
                <p className="text-white text-2xl font-bold">{stats.total}</p>
              </div>
            </div>
          </div>
        </div>

        {/* Filters */}
        <div className="bg-brand-card border border-brand-border rounded-xl p-4 mb-6">
          <div className="flex flex-col md:flex-row gap-4">
            {/* Search */}
            <form onSubmit={handleSearch} className="flex-1 flex gap-2">
              <div className="relative flex-1">
                <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-gray-400" />
                <input
                  type="text"
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  placeholder="Qidirish... (foydalanuvchi nomi, telegram ID)"
                  className="w-full pl-10 pr-4 py-2 bg-brand-dark border border-brand-border rounded-lg text-white placeholder-gray-500 focus:outline-none focus:border-brand-red"
                />
              </div>
              <button
                type="submit"
                className="px-4 py-2 bg-brand-red hover:bg-red-600 text-white rounded-lg transition-colors"
              >
                Qidirish
              </button>
            </form>
            
            {/* Status Filter */}
            <div className="flex gap-2">
              {["all", "pending", "approved", "rejected"].map((status) => (
                <button
                  key={status}
                  onClick={() => { setStatusFilter(status); setPage(1); }}
                  className={`px-4 py-2 rounded-lg transition-colors ${
                    statusFilter === status
                      ? "bg-brand-red text-white"
                      : "bg-brand-dark text-gray-400 hover:text-white border border-brand-border"
                  }`}
                >
                  {status === "all" ? "Barchasi" :
                   status === "pending" ? "Kutilmoqda" :
                   status === "approved" ? "Qabul" :
                   "Rad"}
                </button>
              ))}
            </div>
          </div>
        </div>

        {/* Table */}
        <div className="bg-brand-card border border-brand-border rounded-xl overflow-hidden">
          {loading ? (
            <div className="flex items-center justify-center py-20">
              <div className="animate-spin w-10 h-10 border-3 border-brand-red border-t-transparent rounded-full" />
            </div>
          ) : appeals.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-20">
              <MessageSquare className="w-16 h-16 text-gray-600 mb-4" />
              <p className="text-gray-400 text-lg">Apellyatsiyalar topilmadi</p>
            </div>
          ) : (
            <>
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead>
                    <tr className="border-b border-brand-border">
                      <th className="px-4 py-3 text-left text-sm font-medium text-gray-400">Foydalanuvchi</th>
                      <th className="px-4 py-3 text-left text-sm font-medium text-gray-400">Ban sababi</th>
                      <th className="px-4 py-3 text-left text-sm font-medium text-gray-400">Xabar</th>
                      <th className="px-4 py-3 text-left text-sm font-medium text-gray-400">Yuborilgan</th>
                      <th className="px-4 py-3 text-left text-sm font-medium text-gray-400">Holat</th>
                      <th className="px-4 py-3 text-left text-sm font-medium text-gray-400">Amallar</th>
                    </tr>
                  </thead>
                  <tbody>
                    {appeals.map((appeal) => (
                      <tr key={appeal.id} className="border-b border-brand-border/50 hover:bg-brand-dark/50">
                        <td className="px-4 py-3">
                          <div className="flex items-center gap-3">
                            <div className="w-8 h-8 rounded-full bg-brand-red/20 flex items-center justify-center">
                              <User className="w-4 h-4 text-brand-red" />
                            </div>
                            <div>
                              <p className="text-white font-medium">@{appeal.username || "Noma'lum"}</p>
                              <p className="text-gray-500 text-xs">ID: {appeal.telegram_id}</p>
                            </div>
                          </div>
                        </td>
                        <td className="px-4 py-3">
                          <p className="text-gray-300 text-sm">{appeal.ban_reason || "—"}</p>
                          {appeal.ban_banned_by_name && (
                            <p className="text-gray-500 text-xs">Admin: {appeal.ban_banned_by_name}</p>
                          )}
                        </td>
                        <td className="px-4 py-3">
                          <p className="text-gray-300 text-sm max-w-xs truncate">{appeal.message}</p>
                        </td>
                        <td className="px-4 py-3">
                          <p className="text-gray-400 text-sm">{formatDate(appeal.created_at)}</p>
                        </td>
                        <td className="px-4 py-3">
                          {getStatusBadge(appeal.status)}
                          {appeal.admin_note && (
                            <p className="text-gray-500 text-xs mt-1 max-w-xs truncate">{appeal.admin_note}</p>
                          )}
                          {appeal.reviewed_by_username && (
                            <p className="text-gray-500 text-xs">
                              {appeal.reviewed_by_username} tomonidan
                            </p>
                          )}
                        </td>
                        <td className="px-4 py-3">
                          <div className="flex items-center gap-2">
                            {appeal.status === "pending" && (
                              <>
                                <button
                                  onClick={() => openReviewModal(appeal)}
                                  className="p-2 bg-green-500/20 hover:bg-green-500/30 text-green-400 rounded-lg transition-colors"
                                  title="Ko'rib chiqish"
                                >
                                  <Eye className="w-4 h-4" />
                                </button>
                              </>
                            )}
                            <Link
                              href={`/admin/users?search=${appeal.user_id}`}
                              className="p-2 bg-blue-500/20 hover:bg-blue-500/30 text-blue-400 rounded-lg transition-colors"
                              title="Foydalanuvchi profil"
                            >
                              <ExternalLink className="w-4 h-4" />
                            </Link>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              
              {/* Pagination */}
              {totalPages > 1 && (
                <div className="flex items-center justify-between px-4 py-3 border-t border-brand-border">
                  <p className="text-gray-400 text-sm">
                    Sahifa {page} / {totalPages} (Jami: {total})
                  </p>
                  <div className="flex items-center gap-2">
                    <button
                      onClick={() => setPage(page - 1)}
                      disabled={page === 1}
                      className="p-2 bg-brand-dark hover:bg-brand-border disabled:opacity-50 disabled:cursor-not-allowed text-white rounded-lg transition-colors"
                    >
                      <ChevronLeft className="w-4 h-4" />
                    </button>
                    <button
                      onClick={() => setPage(page + 1)}
                      disabled={page === totalPages}
                      className="p-2 bg-brand-dark hover:bg-brand-border disabled:opacity-50 disabled:cursor-not-allowed text-white rounded-lg transition-colors"
                    >
                      <ChevronRight className="w-4 h-4" />
                    </button>
                  </div>
                </div>
              )}
            </>
          )}
        </div>
      </div>

      {/* Review Modal */}
      {showReviewModal && selectedAppeal && (
        <div className="fixed inset-0 bg-black/70 flex items-center justify-center z-50 p-4">
          <div className="bg-brand-card border border-brand-border rounded-xl max-w-lg w-full p-6">
            <h2 className="text-xl font-display text-white mb-4">Apellyatsiyani ko'rib chiqish</h2>
            
            {/* User Info */}
            <div className="bg-brand-dark rounded-lg p-4 mb-4">
              <div className="flex items-center gap-3 mb-3">
                <div className="w-10 h-10 rounded-full bg-brand-red/20 flex items-center justify-center">
                  <User className="w-5 h-5 text-brand-red" />
                </div>
                <div>
                  <p className="text-white font-medium">@{selectedAppeal.username || "Noma'lum"}</p>
                  <p className="text-gray-500 text-sm">ID: {selectedAppeal.telegram_id}</p>
                </div>
              </div>
              
              {/* Ban Info */}
              {selectedAppeal.ban_reason && (
                <div className="flex items-start gap-2 mb-3 p-3 bg-brand-card rounded-lg">
                  <AlertTriangle className="w-4 h-4 text-red-400 mt-0.5" />
                  <div>
                    <p className="text-gray-500 text-xs">Ban sababi</p>
                    <p className="text-white text-sm">{selectedAppeal.ban_reason}</p>
                  </div>
                </div>
              )}
              
              {/* Appeal Message */}
              <div className="p-3 bg-brand-card rounded-lg">
                <p className="text-gray-500 text-xs mb-1">Apellyatsiya xabari</p>
                <p className="text-white text-sm">{selectedAppeal.message}</p>
              </div>
            </div>
            
            {/* Admin Note */}
            <div className="mb-4">
              <label className="block text-sm text-gray-400 mb-2">Admin izohi (ixtiyoriy)</label>
              <textarea
                value={adminNote}
                onChange={(e) => setAdminNote(e.target.value)}
                placeholder="Ichki izoh yoki javob..."
                className="w-full p-3 bg-brand-dark border border-brand-border rounded-lg text-white placeholder-gray-500 focus:outline-none focus:border-brand-red resize-none"
                rows={3}
              />
            </div>
            
            {/* Unban Option */}
            <div className="mb-6">
              <label className="flex items-center gap-3 cursor-pointer">
                <input
                  type="checkbox"
                  checked={unbanUser}
                  onChange={(e) => setUnbanUser(e.target.checked)}
                  disabled={reviewAction === "reject"}
                  className="w-5 h-5 rounded border-brand-border bg-brand-dark text-brand-red focus:ring-brand-red focus:ring-offset-0"
                />
                <span className="text-white">Foydalanuvchini bandan chiqarish</span>
              </label>
              <p className="text-gray-500 text-xs mt-1 ml-8">
                Agar belgilangan bo'lsa, apellyatsiya tasdiqlanganda foydalanuvchi avtomatik ravishda bandan chiqariladi
              </p>
            </div>
            
            {/* Success Message */}
            {submitSuccess && (
              <div className="mb-4 p-3 bg-green-500/20 border border-green-500/30 rounded-lg">
                <div className="flex items-center gap-2">
                  <CheckCircle className="w-5 h-5 text-green-400" />
                  <p className="text-green-400 font-medium">Apellyatsiya muvaffaqiyatli ko'rib chiqildi!</p>
                </div>
              </div>
            )}
            
            {/* Error Message */}
            {submitError && (
              <div className="mb-4 p-3 bg-red-500/20 border border-red-500/30 rounded-lg">
                <div className="flex items-center gap-2">
                  <XCircle className="w-5 h-5 text-red-400" />
                  <p className="text-red-400 text-sm">{submitError}</p>
                </div>
              </div>
            )}
            
            {/* Actions */}
            <div className="flex gap-3">
              <button
                onClick={handleReview}
                disabled={submitting || submitSuccess}
                className="flex-1 flex items-center justify-center gap-2 py-3 px-4 bg-brand-red hover:bg-red-600 disabled:opacity-50 text-white rounded-lg transition-colors font-medium"
              >
                {submitting ? (
                  <Loader2 className="w-5 h-5 animate-spin" />
                ) : (
                  <>
                    <Check className="w-5 h-5" />
                    <span>Tasdiqlash</span>
                  </>
                )}
              </button>
              <button
                onClick={() => setShowReviewModal(false)}
                className="py-3 px-4 bg-brand-dark hover:bg-brand-border text-gray-400 rounded-lg transition-colors font-medium"
              >
                Bekor qilish
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
