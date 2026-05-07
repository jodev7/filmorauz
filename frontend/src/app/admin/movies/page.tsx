"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import {
  PlusCircle,
  Pencil,
  Trash2,
  ExternalLink,
  Search,
  Loader2,
  CheckCircle,
  XCircle,
  Clock,
} from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import {
  adminGetMovies,
  adminDeleteMovie,
  approveMovie,
  rejectMovie,
  Movie,
} from "@/lib/api";
import { normalizeMediaUrl } from "@/lib/image-utils";
import MediaImage from "@/components/ui/MediaImage";
import CascadeDeleteModal, {
  formatCascadeWarning,
} from "@/components/admin/CascadeDeleteModal";
import DeleteProgressModal from "@/components/admin/DeleteProgressModal";

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

export default function AdminMoviesPage() {
  const { token } = useAuth();
  const [movies, setMovies] = useState<Movie[]>([]);
  const [filtered, setFiltered] = useState<Movie[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [deleting, setDeleting] = useState<string | null>(null);
  const [deleteJobId, setDeleteJobId] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Movie | null>(null);
  const [approving, setApproving] = useState<string | null>(null);
  const [rejecting, setRejecting] = useState<string | null>(null);

  const fetchMovies = async () => {
    if (!token) return;
    try {
      const data = await adminGetMovies(token);
      setMovies(data || []);
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchMovies();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token]);

  // Client-side filter: search + status tab
  useEffect(() => {
    let result = movies;

    if (statusFilter !== "all") {
      result = result.filter((m) => {
        const s = m.approval_status || "approved";
        return s === statusFilter;
      });
    }

    if (search.trim()) {
      const q = search.toLowerCase();
      result = result.filter(
        (m) =>
          m.title.toLowerCase().includes(q) ||
          (m.code && String(m.code).includes(q)) ||
          m.genre?.some((g) => g.toLowerCase().includes(q))
      );
    }

    setFiltered(result);
  }, [search, movies, statusFilter]);

  const handleDeleteClick = (movie: Movie) => {
    setDeleteTarget(movie);
  };

  const performDelete = async () => {
    const movie = deleteTarget;
    if (!movie || !token) return;
    setDeleting(movie.id);
    try {
      const response = await adminDeleteMovie(token, movie.id);
      if (response.job_id) {
        setDeleteJobId(response.job_id);
      } else {
        // Fallback for non-async (if job_id missing)
        setMovies((prev) => prev.filter((m) => m.id !== movie.id));
        const warning = formatCascadeWarning(response?.deleted_b2);
        if (warning) alert(`"${movie.title}" o'chirildi, lekin ba'zi fayllar muammoli:\n\n${warning}`);
        setDeleteTarget(null);
      }
    } catch (err: any) {
      alert(err.message || "O'chirishda xatolik");
      setDeleting(null);
    }
  };

  const handleApprove = async (movie: Movie) => {
    setApproving(movie.id);
    try {
      await approveMovie(token!, movie.id);
      setMovies((prev) =>
        prev.map((m) =>
          m.id === movie.id
            ? { ...m, approval_status: "approved", is_published: true }
            : m
        )
      );
    } catch (err) {
      alert(err instanceof Error ? err.message : "Tasdiqlashda xato");
    } finally {
      setApproving(null);
    }
  };

  const handleReject = async (movie: Movie) => {
    const movieId = movie._id || movie.id;
    setRejecting(movieId);
    try {
      await rejectMovie(token!, movieId);
      setMovies((prev) =>
        prev.map((m) =>
          (m._id || m.id) === movieId
            ? { ...m, approval_status: "rejected", is_published: false }
            : m
        )
      );
    } catch (err) {
      alert(err instanceof Error ? err.message : "Rad etishda xato");
    } finally {
      setRejecting(null);
    }
  };

  const counts = {
    all: movies.length,
    pending: movies.filter((m) => (m.approval_status || "approved") === "pending").length,
    approved: movies.filter((m) => (m.approval_status || "approved") === "approved").length,
    rejected: movies.filter((m) => (m.approval_status || "approved") === "rejected").length,
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
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6 sm:mb-8">
        <div>
          <h1 className="text-xl sm:text-2xl font-bold text-white">Kinolar</h1>
          <p className="text-gray-500 text-sm mt-1">
            {movies.length} ta kino mavjud
          </p>
        </div>
        <Link
          href="/admin/movies/new"
          className="inline-flex items-center gap-2 bg-brand-red hover:bg-orange-700 text-white font-medium px-4 sm:px-5 py-2 sm:py-2.5 rounded-lg transition-colors text-sm"
        >
          <PlusCircle size={16} />
          <span className="hidden sm:inline">Kino qo&apos;shish</span>
          <span className="sm:hidden">Qo&apos;shish</span>
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
      <div className="relative mb-6 max-w-sm">
        <Search
          size={15}
          className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500"
        />
        <input
          type="text"
          placeholder="Kinolarni qidiring..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="w-full bg-brand-card border border-brand-border rounded-lg pl-9 pr-4 py-2.5 text-sm text-white placeholder-gray-600 focus:outline-none focus:border-brand-red transition-colors"
        />
      </div>

      {/* Table */}
      {loading ? (
        <div className="flex items-center gap-2 text-gray-500 py-12 justify-center">
          <Loader2 size={18} className="animate-spin" />
          Kinolar yuklanmoqda...
        </div>
      ) : filtered.length === 0 ? (
        <div className="bg-brand-card border border-brand-border rounded-xl p-8 sm:p-12 text-center">
          <p className="text-gray-500">
            {search || statusFilter !== "all"
              ? "Filtrlarga mos kinolar yo'q."
              : "Hali kinolar yo'q."}
          </p>
          {!search && statusFilter === "all" && (
            <Link
              href="/admin/movies/new"
              className="mt-3 inline-block text-sm text-brand-red hover:underline"
            >
              Birinchi kinoni qo&apos;shing →
            </Link>
          )}
        </div>
      ) : (
        <div className="bg-brand-card border border-brand-border rounded-xl overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-brand-border text-gray-500 text-xs uppercase tracking-wider">
                  <th className="text-left px-3 sm:px-5 py-3">Kino</th>
                  <th className="text-left px-3 sm:px-5 py-3 hidden lg:table-cell">
                    Janr
                  </th>
                  <th className="text-left px-3 sm:px-5 py-3 hidden md:table-cell">
                    Yil
                  </th>
                  <th className="text-left px-3 sm:px-5 py-3 hidden md:table-cell">
                    Holat
                  </th>
                  <th className="text-right px-3 sm:px-5 py-3">Amallar</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((movie) => {
                  const adminPosterSrc = normalizeMediaUrl(movie.poster_url);
                  return (
                  <tr
                    key={movie.id}
                    className="border-b border-brand-border/50 last:border-0 hover:bg-brand-border/20 transition-colors"
                  >
                    {/* Poster + title */}
                    <td className="px-3 sm:px-5 py-3">
                      <div className="flex items-center gap-2 sm:gap-3">
                        <MediaImage
                          src={adminPosterSrc}
                          alt={movie.title}
                          className="w-8 h-12 sm:w-9 sm:h-14 object-cover rounded shrink-0"
                        />
                        <div className="min-w-0">
                          <p className="text-white font-medium truncate max-w-[150px] sm:max-w-[200px]">
                            {movie.code && (
                              <span className="text-gray-500 font-mono text-xs mr-1">
                                #{movie.code}
                              </span>
                            )}
                            {movie.title}
                          </p>
                          <p className="text-gray-600 text-xs font-mono truncate max-w-[150px] sm:max-w-[200px]">
                            {movie.slug}
                          </p>
                        </div>
                      </div>
                    </td>

                    {/* Genre */}
                    <td className="px-3 sm:px-5 py-3 hidden lg:table-cell">
                      <div className="flex flex-wrap gap-1">
                        {movie.genre?.slice(0, 2).map((g) => (
                          <span
                            key={g}
                            className="text-xs bg-brand-border text-gray-400 px-2 py-0.5 rounded-full capitalize"
                          >
                            {g}
                          </span>
                        ))}
                      </div>
                    </td>

                    {/* Year */}
                    <td className="px-3 sm:px-5 py-3 text-gray-400 hidden md:table-cell">
                      {movie.year}
                    </td>

                    {/* Approval status */}
                    <td className="px-3 sm:px-5 py-3 hidden md:table-cell">
                      <ApprovalBadge status={movie.approval_status} />
                    </td>

                    {/* Actions */}
                    <td className="px-3 sm:px-5 py-3">
                      <div className="flex items-center justify-end gap-1">
                        {/* Approve — shown for pending or rejected */}
                        {movie.approval_status !== "approved" && (
                          <button
                            onClick={() => handleApprove(movie)}
                            disabled={approving === movie.id}
                            title="Tasdiqlash"
                            className="p-2 text-green-500 hover:text-green-300 rounded-lg hover:bg-green-500/10 transition-colors disabled:opacity-50"
                          >
                            {approving === movie.id ? (
                              <Loader2 size={14} className="animate-spin" />
                            ) : (
                              <CheckCircle size={14} />
                            )}
                          </button>
                        )}

                        {/* Reject — shown for pending or approved */}
                        {movie.approval_status !== "rejected" && (
                          <button
                            onClick={() => handleReject(movie)}
                            disabled={rejecting === movie.id}
                            title="Rad etish"
                            className="p-2 text-red-500 hover:text-red-300 rounded-lg hover:bg-red-500/10 transition-colors disabled:opacity-50"
                          >
                            {rejecting === movie.id ? (
                              <Loader2 size={14} className="animate-spin" />
                            ) : (
                              <XCircle size={14} />
                            )}
                          </button>
                        )}

                        {/* View on site — only for approved */}
                        {movie.approval_status === "approved" && (
                          <Link
                            href={`/movies/${movie.slug}`}
                            target="_blank"
                            title="Saytda ko'rish"
                            className="p-2 text-gray-500 hover:text-gray-300 rounded-lg hover:bg-brand-border transition-colors"
                          >
                            <ExternalLink size={14} />
                          </Link>
                        )}

                        {/* Edit */}
                        <Link
                          href={`/admin/movies/${movie.id}/edit`}
                          title="Tahrirlash"
                          className="p-2 text-gray-400 hover:text-white rounded-lg hover:bg-brand-border transition-colors"
                        >
                          <Pencil size={14} />
                        </Link>

                        {/* Delete */}
                        <button
                          onClick={() => handleDeleteClick(movie)}
                          disabled={deleting === movie.id}
                          title="O'chirish"
                          className="p-2 text-gray-500 hover:text-red-400 rounded-lg hover:bg-red-400/10 transition-colors disabled:opacity-50"
                        >
                          {deleting === movie.id ? (
                            <Loader2 size={14} className="animate-spin" />
                          ) : (
                            <Trash2 size={14} />
                          )}
                        </button>
                      </div>
                    </td>
                  </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}

      <CascadeDeleteModal
        open={deleteTarget !== null && !deleteJobId}
        kind="movie"
        title={deleteTarget?.title ?? ""}
        onConfirm={performDelete}
        onClose={() => {
          if (deleting === null) setDeleteTarget(null);
        }}
      />

      {deleteJobId && (
        <DeleteProgressModal
          jobId={deleteJobId}
          isOpen={true}
          onClose={() => {
            if (deleteTarget) {
              setMovies((prev) => prev.filter((m) => m.id !== deleteTarget.id));
              setDeleteTarget(null);
            }
            setDeleteJobId(null);
            setDeleting(null);
          }}
        />
      )}
    </div>
  );
}
