"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { MessageSquare, Search, ChevronLeft, ChevronRight, Check, X, Trash2, Eye, EyeOff } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { getAdminComments, updateCommentStatus, adminDeleteComment, AdminComment, CommentStatus } from "@/lib/comments-api";

export default function AdminCommentsPage() {
  const { token, isLoading: authLoading, user } = useAuth();
  const router = useRouter();

  const [comments, setComments] = useState<AdminComment[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [limit] = useState(20);
  const [totalPages, setTotalPages] = useState(0);
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState<CommentStatus | "all">("all");
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState<string | null>(null);

  // Redirect if not admin
  useEffect(() => {
    if (!authLoading && (!token || (user?.role !== "admin" && user?.role !== "superadmin"))) {
      router.push("/");
    }
  }, [authLoading, token, user, router]);

  // Fetch comments
  useEffect(() => {
    if (!token) return;

    setLoading(true);
    getAdminComments(token, { 
      page, 
      limit, 
      search: search || undefined, 
      status: status !== "all" ? status : undefined 
    })
      .then((data) => {
        setComments(data.data);
        setTotal(data.total);
        setTotalPages(data.total_pages);
      })
      .catch(console.error)
      .finally(() => setLoading(false));
  }, [token, page, search, status]);

  const handleStatusChange = async (commentId: string, newStatus: CommentStatus) => {
    if (!token) return;
    setActionLoading(commentId);
    try {
      await updateCommentStatus(token, commentId, newStatus);
      // Refresh comments
      const data = await getAdminComments(token, { 
        page, 
        limit, 
        search: search || undefined, 
        status: status !== "all" ? status : undefined 
      });
      setComments(data.data);
    } catch (error) {
      console.error("Failed to update status:", error);
    } finally {
      setActionLoading(null);
    }
  };

  const handleDelete = async (commentId: string) => {
    if (!token || !confirm("Haqiqatanham bu izohni o'chirmoqchimisiz?")) return;
    setActionLoading(commentId);
    try {
      await adminDeleteComment(token, commentId);
      // Refresh comments
      const data = await getAdminComments(token, { 
        page, 
        limit, 
        search: search || undefined, 
        status: status !== "all" ? status : undefined 
      });
      setComments(data.data);
    } catch (error) {
      console.error("Failed to delete comment:", error);
    } finally {
      setActionLoading(null);
    }
  };

  // Format date
  const formatDate = (dateStr: string) => {
    if (!dateStr) return "—";
    const d = new Date(dateStr);
    return d.toLocaleDateString("uz-UZ") + " " + d.toLocaleTimeString("uz-UZ", { hour: "2-digit", minute: "2-digit" });
  };

  // Get status badge
  const getStatusBadge = (s: string) => {
    switch (s) {
      case "approved":
        return { bg: "bg-green-500/20", text: "green-400", label: "Tasdiqlangan" };
      case "pending":
        return { bg: "bg-yellow-500/20", text: "yellow-400", label: "Kutilmoqda" };
      case "rejected":
        return { bg: "bg-red-500/20", text: "red-400", label: "Rad etilgan" };
      default:
        return { bg: "bg-gray-500/20", text: "gray-400", label: s };
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
        <h1 className="text-xl sm:text-2xl font-bold text-white">Izohlar Moderatsiyasi</h1>
        <p className="text-gray-500 text-sm mt-1">
          Film izohlarini boshqarish va moderatsiya qilish
        </p>
      </div>

      {/* Filters */}
      <div className="flex flex-col sm:flex-row gap-4 mb-6">
        {/* Search */}
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" size={18} />
          <input
            type="text"
            placeholder="Izoh matni bo'yicha qidirish..."
            value={search}
            onChange={(e) => {
              setSearch(e.target.value);
              setPage(1);
            }}
            className="w-full bg-brand-card border border-brand-border rounded-lg pl-10 pr-4 py-2 text-white placeholder-gray-500 focus:outline-none focus:border-brand-red"
          />
        </div>

        {/* Status Filter */}
        <select
          value={status}
          onChange={(e) => {
            setStatus(e.target.value as CommentStatus | "all");
            setPage(1);
          }}
          className="bg-brand-card border border-brand-border rounded-lg px-4 py-2 text-white focus:outline-none focus:border-brand-red"
        >
          <option value="all">Barcha holatlar</option>
          <option value="pending">Kutilmoqda</option>
          <option value="approved">Tasdiqlangan</option>
          <option value="rejected">Rad etilgan</option>
        </select>
      </div>

      {/* Comments Table */}
      <div className="bg-brand-card border border-brand-border rounded-xl overflow-hidden">
        {loading ? (
          <div className="p-8 text-center">
            <p className="text-gray-500">Yuklanmoqda...</p>
          </div>
        ) : comments.length === 0 ? (
          <div className="p-8 text-center">
            <MessageSquare size={32} className="text-gray-600 mx-auto mb-3" />
            <p className="text-gray-500">Izohlar topilmadi</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-brand-border text-gray-500 text-xs uppercase tracking-wider">
                  <th className="text-left px-3 sm:px-5 py-3">Foydalanuvchi</th>
                  <th className="text-left px-3 sm:px-5 py-3">Izoh</th>
                  <th className="text-left px-3 sm:px-5 py-3 hidden md:table-cell">Film</th>
                  <th className="text-left px-3 sm:px-5 py-3 hidden lg:table-cell">Holat</th>
                  <th className="text-left px-3 sm:px-5 py-3 hidden lg:table-cell">Sana</th>
                  <th className="text-right px-3 sm:px-5 py-3">Amallar</th>
                </tr>
              </thead>
              <tbody>
                {comments.map((comment) => {
                  const statusBadge = getStatusBadge(comment.status);
                  return (
                    <tr
                      key={comment.id}
                      className="border-b border-brand-border/50 last:border-0 hover:bg-brand-border/30 transition-colors"
                    >
                      <td className="px-3 sm:px-5 py-3">
                        <div className="min-w-0">
                          <p className="text-white font-medium truncate max-w-[150px]">
                            {comment.user?.display_name || comment.user?.username || "Noma'lum"}
                          </p>
                          {comment.user?.username && (
                            <p className="text-gray-500 text-xs">@{comment.user.username}</p>
                          )}
                        </div>
                      </td>
                      <td className="px-3 sm:px-5 py-3">
                        <p className="text-white max-w-[250px] truncate" title={comment.content}>
                          {comment.content}
                        </p>
                        {comment.parent_id && (
                          <p className="text-gray-500 text-xs mt-1">
                            ↳ Javob: {comment.parent_id.slice(0, 8)}...
                          </p>
                        )}
                      </td>
                      <td className="px-3 sm:px-5 py-3 hidden md:table-cell">
                        <p className="text-gray-400 max-w-[120px] truncate" title={comment.movie_title}>
                          {comment.movie_title || comment.movie_id}
                        </p>
                      </td>
                      <td className="px-3 sm:px-5 py-3 hidden lg:table-cell">
                        <span className={`inline-flex items-center text-xs px-2 py-1 rounded ${statusBadge.bg} ${statusBadge.text}`}>
                          {statusBadge.label}
                        </span>
                      </td>
                      <td className="px-3 sm:px-5 py-3 hidden lg:table-cell text-gray-400 text-xs">
                        {formatDate(comment.created_at)}
                      </td>
                      <td className="px-3 sm:px-5 py-3 text-right">
                        <div className="flex items-center justify-end gap-1">
                          {comment.status !== "approved" && (
                            <button
                              onClick={() => handleStatusChange(comment.id, "approved")}
                              disabled={actionLoading === comment.id}
                              className="p-2 rounded-lg bg-green-500/20 text-green-400 hover:bg-green-500/30 transition-colors disabled:opacity-50"
                              title="Tasdiqlash"
                            >
                              <Check size={16} />
                            </button>
                          )}
                          {comment.status !== "rejected" && (
                            <button
                              onClick={() => handleStatusChange(comment.id, "rejected")}
                              disabled={actionLoading === comment.id}
                              className="p-2 rounded-lg bg-yellow-500/20 text-yellow-400 hover:bg-yellow-500/30 transition-colors disabled:opacity-50"
                              title="Rad qilish"
                            >
                              <X size={16} />
                            </button>
                          )}
                          {comment.status === "pending" && (
                            <button
                              onClick={() => handleStatusChange(comment.id, "approved")}
                              disabled={actionLoading === comment.id}
                              className="p-2 rounded-lg bg-blue-500/20 text-blue-400 hover:bg-blue-500/30 transition-colors disabled:opacity-50"
                              title="Ko'rinish"
                            >
                              <Eye size={16} />
                            </button>
                          )}
                          <button
                            onClick={() => handleDelete(comment.id)}
                            disabled={actionLoading === comment.id}
                            className="p-2 rounded-lg bg-red-500/20 text-red-400 hover:bg-red-500/30 transition-colors disabled:opacity-50"
                            title="O'chirish"
                          >
                            <Trash2 size={16} />
                          </button>
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

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="flex items-center justify-between mt-4">
          <p className="text-gray-500 text-sm">
            Jami: {total} ta izoh
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
    </div>
  );
}
