"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import {
  PlusCircle,
  Pencil,
  Trash2,
  Search,
  Loader2,
  Film,
  Tv,
  Star,
  CheckCircle,
  XCircle,
  Clock,
} from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import {
  adminGetSeries,
  adminDeleteSeries,
  approveSeries,
  rejectSeries,
  AdminSeries,
} from "@/lib/api";

type StatusFilter = "all" | "pending" | "approved" | "rejected";

function ApprovalBadge({ status }: { status?: string }) {
  if (!status || status === "approved") {
    return (
      <span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-green-500/15 text-green-400">
        <CheckCircle size={10} />
        Tasdiqlangan
      </span>
    );
  }
  if (status === "pending") {
    return (
      <span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-yellow-500/15 text-yellow-400">
        <Clock size={10} />
        Kutmoqda
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-red-500/15 text-red-400">
      <XCircle size={10} />
      Rad etilgan
    </span>
  );
}

export default function AdminSeriesPage() {
  const { token } = useAuth();
  const [series, setSeries] = useState<AdminSeries[]>([]);
  const [filtered, setFiltered] = useState<AdminSeries[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [deleting, setDeleting] = useState<string | null>(null);
  const [approving, setApproving] = useState<string | null>(null);
  const [rejecting, setRejecting] = useState<string | null>(null);

  const fetchSeries = async () => {
    if (!token) return;
    try {
      const data = await adminGetSeries(token);
      setSeries(data || []);
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchSeries();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token]);

  // Client-side filter: search + status tab
  useEffect(() => {
    let result = series;

    if (statusFilter !== "all") {
      result = result.filter((s) => {
        const st = s.approval_status || "approved";
        return st === statusFilter;
      });
    }

    if (search.trim()) {
      const q = search.toLowerCase();
      result = result.filter(
        (s) =>
          s.title.toLowerCase().includes(q) ||
          s.slug.toLowerCase().includes(q) ||
          s.genre?.some((g) => g.toLowerCase().includes(q))
      );
    }

    setFiltered(result);
  }, [search, series, statusFilter]);

  const handleDelete = async (s: AdminSeries) => {
    if (
      !confirm(
        `Haqiqatan ham "${s.title}" serialni o'chirmoqchimisiz? Bu amalni bekor qilib bo'lmaydi.`
      )
    )
      return;

    setDeleting(s.id);
    try {
      await adminDeleteSeries(token!, s.id);
      setSeries((prev) => prev.filter((item) => item.id !== s.id));
    } catch (err) {
      alert(err instanceof Error ? err.message : "O'chirishda xato");
    } finally {
      setDeleting(null);
    }
  };

  const handleApprove = async (s: AdminSeries) => {
    setApproving(s.id);
    try {
      await approveSeries(token!, s.id);
      setSeries((prev) =>
        prev.map((item) =>
          item.id === s.id
            ? { ...item, approval_status: "approved", is_published: true }
            : item
        )
      );
    } catch (err) {
      alert(err instanceof Error ? err.message : "Tasdiqlashda xato");
    } finally {
      setApproving(null);
    }
  };

  const handleReject = async (s: AdminSeries) => {
    setRejecting(s.id);
    try {
      await rejectSeries(token!, s.id);
      setSeries((prev) =>
        prev.map((item) =>
          item.id === s.id
            ? { ...item, approval_status: "rejected", is_published: false }
            : item
        )
      );
    } catch (err) {
      alert(err instanceof Error ? err.message : "Rad etishda xato");
    } finally {
      setRejecting(null);
    }
  };

  const counts = {
    all: series.length,
    pending: series.filter((s) => (s.approval_status || "approved") === "pending").length,
    approved: series.filter((s) => (s.approval_status || "approved") === "approved").length,
    rejected: series.filter((s) => (s.approval_status || "approved") === "rejected").length,
  };

  const tabs: { key: StatusFilter; label: string }[] = [
    { key: "all", label: `Barchasi (${counts.all})` },
    { key: "pending", label: `Kutmoqda (${counts.pending})` },
    { key: "approved", label: `Tasdiqlangan (${counts.approved})` },
    { key: "rejected", label: `Rad etilgan (${counts.rejected})` },
  ];

  return (
    <div className="p-4 sm:p-8">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-6">
        <div>
          <h1 className="text-2xl font-bold text-white">Seriallar</h1>
          <p className="text-gray-400 text-sm mt-1">
            {series.length} ta serial
          </p>
        </div>
        <Link
          href="/admin/series/new"
          className="inline-flex items-center gap-2 bg-brand-red hover:bg-orange-700 text-white px-4 py-2 rounded-lg font-medium transition-colors"
        >
          <PlusCircle className="w-5 h-5" />
          Yangi serial
        </Link>
      </div>

      {/* Status filter tabs */}
      <div className="flex gap-1 mb-4 flex-wrap">
        {tabs.map((tab) => (
          <button
            key={tab.key}
            onClick={() => setStatusFilter(tab.key)}
            className={`px-3 py-1.5 rounded-lg text-xs font-medium transition-colors ${
              statusFilter === tab.key
                ? "bg-brand-red text-white"
                : "bg-brand-card text-gray-400 hover:text-white border border-brand-border"
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* Search */}
      <div className="relative mb-6">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-gray-400" />
        <input
          type="text"
          placeholder="Seriallarni qidirish..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="w-full pl-10 pr-4 py-3 bg-brand-card border border-brand-border rounded-lg text-white placeholder-gray-500 focus:outline-none focus:border-brand-red"
        />
      </div>

      {/* Loading */}
      {loading && (
        <div className="flex items-center justify-center py-12">
          <Loader2 className="w-8 h-8 text-brand-red animate-spin" />
        </div>
      )}

      {/* Empty */}
      {!loading && filtered.length === 0 && (
        <div className="text-center py-12">
          <Film className="w-16 h-16 text-gray-600 mx-auto mb-4" />
          <p className="text-gray-400 text-lg">Seriallar topilmadi</p>
          {statusFilter === "all" && !search && (
            <Link
              href="/admin/series/new"
              className="inline-block mt-4 text-brand-red hover:underline"
            >
              Yangi serial qo&apos;shing
            </Link>
          )}
        </div>
      )}

      {/* Table */}
      {!loading && filtered.length > 0 && (
        <div className="bg-brand-card border border-brand-border rounded-lg overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-brand-dark/50">
                <tr>
                  <th className="text-left text-xs font-medium text-gray-400 uppercase tracking-wider px-4 py-3">
                    Poster
                  </th>
                  <th className="text-left text-xs font-medium text-gray-400 uppercase tracking-wider px-4 py-3">
                    Nomi
                  </th>
                  <th className="text-left text-xs font-medium text-gray-400 uppercase tracking-wider px-4 py-3">
                    Yil
                  </th>
                  <th className="text-left text-xs font-medium text-gray-400 uppercase tracking-wider px-4 py-3 hidden lg:table-cell">
                    Janr
                  </th>
                  <th className="text-left text-xs font-medium text-gray-400 uppercase tracking-wider px-4 py-3">
                    Holat
                  </th>
                  <th className="text-right text-xs font-medium text-gray-400 uppercase tracking-wider px-4 py-3">
                    Amallar
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-brand-border">
                {filtered.map((s) => (
                  <tr key={s.id} className="hover:bg-brand-dark/30">
                    <td className="px-4 py-3">
                      <div className="w-10 h-14 bg-gray-800 rounded overflow-hidden">
                        {s.poster_url ? (
                          // eslint-disable-next-line @next/next/no-img-element
                          <img
                            src={s.poster_url}
                            alt={s.title}
                            className="w-full h-full object-cover"
                          />
                        ) : (
                          <div className="w-full h-full flex items-center justify-center">
                            <Film className="w-5 h-5 text-gray-600" />
                          </div>
                        )}
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <div>
                        <p className="font-medium text-white">{s.title}</p>
                        <p className="text-sm text-gray-500">/{s.slug}</p>
                        {s.is_premium && (
                          <span className="inline-flex items-center gap-1 text-xs text-yellow-400 mt-0.5">
                            <Star className="w-3 h-3" />
                            Premium
                          </span>
                        )}
                      </div>
                    </td>
                    <td className="px-4 py-3 text-gray-300">{s.year}</td>
                    <td className="px-4 py-3 hidden lg:table-cell">
                      <div className="flex flex-wrap gap-1">
                        {s.genre?.slice(0, 2).map((g, i) => (
                          <span
                            key={i}
                            className="px-2 py-0.5 bg-brand-dark text-gray-300 text-xs rounded capitalize"
                          >
                            {g}
                          </span>
                        ))}
                        {s.genre && s.genre.length > 2 && (
                          <span className="text-gray-500 text-xs">
                            +{s.genre.length - 2}
                          </span>
                        )}
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <ApprovalBadge status={s.approval_status} />
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex items-center justify-end gap-1">
                        {/* Approve */}
                        {s.approval_status !== "approved" && (
                          <button
                            onClick={() => handleApprove(s)}
                            disabled={approving === s.id}
                            title="Tasdiqlash"
                            className="p-2 text-green-500 hover:text-green-300 rounded-lg hover:bg-green-500/10 transition-colors disabled:opacity-50"
                          >
                            {approving === s.id ? (
                              <Loader2 size={14} className="animate-spin" />
                            ) : (
                              <CheckCircle size={14} />
                            )}
                          </button>
                        )}

                        {/* Reject */}
                        {s.approval_status !== "rejected" && (
                          <button
                            onClick={() => handleReject(s)}
                            disabled={rejecting === s.id}
                            title="Rad etish"
                            className="p-2 text-red-500 hover:text-red-300 rounded-lg hover:bg-red-500/10 transition-colors disabled:opacity-50"
                          >
                            {rejecting === s.id ? (
                              <Loader2 size={14} className="animate-spin" />
                            ) : (
                              <XCircle size={14} />
                            )}
                          </button>
                        )}

                        {/* Edit */}
                        <Link
                          href={`/admin/series/${s.id}/edit`}
                          className="p-2 text-gray-400 hover:text-white hover:bg-brand-dark rounded-lg transition-colors"
                          title="Tahrirlash"
                        >
                          <Pencil className="w-4 h-4" />
                        </Link>

                        {/* View on site — only for approved */}
                        {s.approval_status === "approved" && (
                          <Link
                            href={`/series/${s.slug}`}
                            target="_blank"
                            className="p-2 text-gray-400 hover:text-white hover:bg-brand-dark rounded-lg transition-colors"
                            title="Ko'rish"
                          >
                            <Tv className="w-4 h-4" />
                          </Link>
                        )}

                        {/* Delete */}
                        <button
                          onClick={() => handleDelete(s)}
                          disabled={deleting === s.id}
                          className="p-2 text-gray-400 hover:text-red-400 hover:bg-brand-dark rounded-lg transition-colors disabled:opacity-50"
                          title="O'chirish"
                        >
                          {deleting === s.id ? (
                            <Loader2 className="w-4 h-4 animate-spin" />
                          ) : (
                            <Trash2 className="w-4 h-4" />
                          )}
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}
