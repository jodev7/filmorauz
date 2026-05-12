"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import {
  CheckCircle,
  XCircle,
  Clock,
  Loader2,
  ExternalLink,
  User,
  Image,
} from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import {
  Suggestion,
  SuggestionListResponse,
  adminListSuggestions,
  adminUpdateSuggestion,
  adminGetSuggestionStats,
} from "@/lib/api";
import { normalizeMediaUrl } from "@/lib/image-utils";

type StatusFilter = "all" | "pending" | "accepted" | "rejected";

function StatusBadge({ status }: { status?: string }) {
  if (!status || status === "pending") {
    return (
      <span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-yellow-500/15 text-yellow-400">
        <Clock size={10} />
        Kutmoqda
      </span>
    );
  }
  if (status === "accepted") {
    return (
      <span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-green-500/15 text-green-400">
        <CheckCircle size={10} />
        Qabul qilindi
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-red-500/15 text-red-400">
      <XCircle size={10} />
      Rad etildi
    </span>
  );
}

function TypeBadge({ type }: { type?: string }) {
  if (type === "series") {
    return (
      <span className="inline-flex items-center text-xs px-2 py-0.5 rounded-full bg-blue-500/15 text-blue-400">
        Serial
      </span>
    );
  }
  return (
    <span className="inline-flex items-center text-xs px-2 py-0.5 rounded-full bg-purple-500/15 text-purple-400">
      Kino
    </span>
  );
}

export default function AdminSuggestionsPage() {
  const { token } = useAuth();
  const [suggestions, setSuggestions] = useState<Suggestion[]>([]);
  const [loading, setLoading] = useState(true);
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [limit] = useState(20);
  const [updating, setUpdating] = useState<string | null>(null);
  const [stats, setStats] = useState({ total: 0, pending: 0, accepted: 0, rejected: 0 });
  const [actionModal, setActionModal] = useState<{
    suggestion: Suggestion;
    action: "accept" | "reject";
    message: string;
  } | null>(null);

  const fetchSuggestions = async () => {
    if (!token) return;
    try {
      const data: SuggestionListResponse = await adminListSuggestions(
        token,
        page,
        limit,
        statusFilter
      );
      setSuggestions(data.suggestions || []);
      setTotal(data.total || 0);
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  const fetchStats = async () => {
    if (!token) return;
    try {
      const data = await adminGetSuggestionStats(token);
      setStats(data);
    } catch (err) {
      console.error(err);
    }
  };

  useEffect(() => {
    fetchSuggestions();
  }, [token, page, statusFilter]);

  useEffect(() => {
    fetchStats();
  }, [token]);

  const handleUpdate = async () => {
    if (!token || !actionModal) return;
    setUpdating(actionModal.suggestion.id);
    try {
      await adminUpdateSuggestion(token, actionModal.suggestion.id, {
        status: actionModal.action === "accept" ? "accepted" : "rejected",
        admin_message: actionModal.message,
      });
      setActionModal(null);
      fetchSuggestions();
      fetchStats();
    } catch (err) {
      console.error(err);
      alert("Xatolik yuz berdi");
    } finally {
      setUpdating(null);
    }
  };

  const totalPages = Math.ceil(total / limit);

  const formatDate = (dateStr?: string) => {
    if (!dateStr) return "-";
    return new Date(dateStr).toLocaleDateString("uz-UZ", {
      day: "2-digit",
      month: "2-digit",
      year: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <Loader2 className="w-8 h-8 animate-spin text-brand-red" />
      </div>
    );
  }

  return (
    <div className="p-6">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold text-white">Tavsiyalar</h1>
      </div>

      {/* Stats Cards */}
      <div className="grid grid-cols-4 gap-4 mb-6">
        <div className="bg-brand-card border border-brand-border rounded-lg p-4">
          <div className="text-2xl font-bold text-white">{stats.total}</div>
          <div className="text-sm text-gray-400">Jami</div>
        </div>
        <div className="bg-brand-card border border-brand-border rounded-lg p-4">
          <div className="text-2xl font-bold text-yellow-400">{stats.pending}</div>
          <div className="text-sm text-gray-400">Kutmoqda</div>
        </div>
        <div className="bg-brand-card border border-brand-border rounded-lg p-4">
          <div className="text-2xl font-bold text-green-400">{stats.accepted}</div>
          <div className="text-sm text-gray-400">Qabul qilindi</div>
        </div>
        <div className="bg-brand-card border border-brand-border rounded-lg p-4">
          <div className="text-2xl font-bold text-red-400">{stats.rejected}</div>
          <div className="text-sm text-gray-400">Rad etildi</div>
        </div>
      </div>

      {/* Filters */}
      <div className="flex gap-2 mb-6">
        {(["all", "pending", "accepted", "rejected"] as StatusFilter[]).map((status) => (
          <button
            key={status}
            onClick={() => {
              setStatusFilter(status);
              setPage(1);
            }}
            className={`px-4 py-2 rounded-lg text-sm font-medium transition-colors ${
              statusFilter === status
                ? "bg-brand-red text-white"
                : "bg-brand-card border border-brand-border text-gray-400 hover:text-white"
            }`}
          >
            {status === "all" && "Hammasi"}
            {status === "pending" && "Kutmoqda"}
            {status === "accepted" && "Qabul qilindi"}
            {status === "rejected" && "Rad etildi"}
          </button>
        ))}
      </div>

      {/* Table */}
      <div className="bg-brand-card border border-brand-border rounded-lg overflow-hidden">
        <table className="w-full">
          <thead className="bg-brand-dark border-b border-brand-border">
            <tr>
              <th className="text-left px-4 py-3 text-sm font-medium text-gray-400">Kino/Serial</th>
              <th className="text-left px-4 py-3 text-sm font-medium text-gray-400">Tur</th>
              <th className="text-left px-4 py-3 text-sm font-medium text-gray-400">Foydalanuvchi</th>
              <th className="text-left px-4 py-3 text-sm font-medium text-gray-400">Xabar</th>
              <th className="text-left px-4 py-3 text-sm font-medium text-gray-400">Status</th>
              <th className="text-left px-4 py-3 text-sm font-medium text-gray-400">Sana</th>
              <th className="text-right px-4 py-3 text-sm font-medium text-gray-400">Amallar</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-brand-border">
            {suggestions.length === 0 ? (
              <tr>
                <td colSpan={7} className="px-4 py-8 text-center text-gray-500">
                  Tavsiyalar topilmadi
                </td>
              </tr>
            ) : (
              suggestions.map((suggestion) => (
                <tr key={suggestion.id} className="hover:bg-brand-dark/50">
                  <td className="px-4 py-3">
                    <div className="font-medium text-white">{suggestion.title}</div>
                    {suggestion.source_url && (
                      <a
                        href={suggestion.source_url}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="text-xs text-blue-400 hover:underline flex items-center gap-1 mt-1"
                      >
                        <ExternalLink size={10} />
                        Manba
                      </a>
                    )}
                    {suggestion.image_url && (
                      <a
                        href={normalizeMediaUrl(suggestion.image_url)}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="text-xs text-blue-400 hover:underline flex items-center gap-1 mt-1"
                      >
                        <Image size={10} />
                        Rasm
                      </a>
                    )}
                  </td>
                  <td className="px-4 py-3">
                    <TypeBadge type={suggestion.type} />
                  </td>
                  <td className="px-4 py-3">
                    <Link
                      href={`/user/${suggestion.user_id}`}
                      className="text-blue-400 hover:underline flex items-center gap-1"
                    >
                      <User size={12} />
                      {suggestion.user?.username || 
                       suggestion.user?.full_name || 
                       suggestion.user?.telegram_username || 
                       suggestion.user_name || 
                       suggestion.user_id?.slice(0, 8)}
                    </Link>
                  </td>
                  <td className="px-4 py-3">
                    <div className="max-w-xs text-sm text-gray-300 truncate">
                      {suggestion.message}
                    </div>
                    {suggestion.admin_message && (
                      <div className="max-w-xs text-xs text-green-400 mt-1">
                        Admin: {suggestion.admin_message}
                      </div>
                    )}
                  </td>
                  <td className="px-4 py-3">
                    <StatusBadge status={suggestion.status} />
                  </td>
                  <td className="px-4 py-3 text-sm text-gray-400">
                    {formatDate(suggestion.created_at)}
                  </td>
                  <td className="px-4 py-3 text-right">
                    {suggestion.status === "pending" && (
                      <div className="flex justify-end gap-2">
                        <button
                          onClick={() =>
                            setActionModal({
                              suggestion,
                              action: "accept",
                              message: "",
                            })
                          }
                          className="p-2 rounded-lg bg-green-500/20 text-green-400 hover:bg-green-500/30"
                          title="Qabul qilish"
                        >
                          <CheckCircle size={16} />
                        </button>
                        <button
                          onClick={() =>
                            setActionModal({
                              suggestion,
                              action: "reject",
                              message: "",
                            })
                          }
                          className="p-2 rounded-lg bg-red-500/20 text-red-400 hover:bg-red-500/30"
                          title="Rad etish"
                        >
                          <XCircle size={16} />
                        </button>
                      </div>
                    )}
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="flex justify-center items-center gap-2 mt-6">
          <button
            onClick={() => setPage(Math.max(1, page - 1))}
            disabled={page === 1}
            className="px-4 py-2 rounded-lg bg-brand-card border border-brand-border text-gray-400 hover:text-white disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Oldingi
          </button>
          <span className="text-gray-400">
            {page} / {totalPages}
          </span>
          <button
            onClick={() => setPage(Math.min(totalPages, page + 1))}
            disabled={page === totalPages}
            className="px-4 py-2 rounded-lg bg-brand-card border border-brand-border text-gray-400 hover:text-white disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Keyingi
          </button>
        </div>
      )}

      {/* Action Modal */}
      {actionModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div
            className="absolute inset-0 bg-black/70 backdrop-blur-sm"
            onClick={() => setActionModal(null)}
          />
          <div className="relative bg-brand-card border border-brand-border rounded-2xl p-6 w-full max-w-md mx-4">
            <h2 className="text-xl font-bold text-white mb-4">
              {actionModal.action === "accept" ? "Qabul qilish" : "Rad etish"}
            </h2>
            <p className="text-gray-400 mb-4">
              "{actionModal.suggestion.title}" tavsiyasini{" "}
              {actionModal.action === "accept" ? "qabul qilmoqchimisiz" : "rad etmoqchimisiz"}?
            </p>
            <div className="mb-4">
              <label className="block text-sm text-gray-400 mb-2">
                Admin xabari (ixtiyoriy)
              </label>
              <textarea
                value={actionModal.message}
                onChange={(e) =>
                  setActionModal({ ...actionModal, message: e.target.value })
                }
                className="w-full px-4 py-2 bg-brand-dark border border-brand-border rounded-lg text-white resize-none"
                rows={3}
                placeholder="Foydalanuvchiga xabar..."
              />
            </div>
            <div className="flex justify-end gap-3">
              <button
                onClick={() => setActionModal(null)}
                className="px-4 py-2 rounded-lg bg-brand-card border border-brand-border text-gray-400 hover:text-white"
              >
                Bekor qilish
              </button>
              <button
                onClick={handleUpdate}
                disabled={updating === actionModal.suggestion.id}
                className={`px-4 py-2 rounded-lg text-white ${
                  actionModal.action === "accept"
                    ? "bg-green-500 hover:bg-green-600"
                    : "bg-red-500 hover:bg-red-600"
                } disabled:opacity-50`}
              >
                {updating === actionModal.suggestion.id ? (
                  <Loader2 className="w-4 h-4 animate-spin" />
                ) : actionModal.action === "accept" ? (
                  "Qabul qilish"
                ) : (
                  "Rad etish"
                )}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
