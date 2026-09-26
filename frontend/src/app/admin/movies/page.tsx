"use client";

import { useEffect, useMemo, useState } from "react";
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
  Crown,
  FolderPlus,
  X,
} from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import {
  adminGetMovies,
  adminDeleteMovie,
  approveMovie,
  rejectMovie,
  adminBulkUpdateMovies,
  getAdminCollections,
  CollectionInput,
  Movie,
} from "@/lib/api";
import { useToast } from "@/components/admin/Toast";
import { readUrlNumber, readUrlParam, useSyncUrlParams } from "@/lib/url-state";
import { normalizeMediaUrl } from "@/lib/image-utils";
import MediaImage from "@/components/ui/MediaImage";
import CascadeDeleteModal, {
  formatCascadeWarning,
} from "@/components/admin/CascadeDeleteModal";
import DeleteProgressModal from "@/components/admin/DeleteProgressModal";

type StatusFilter = "all" | "pending" | "approved" | "rejected";

const STATUS_FILTERS: StatusFilter[] = ["all", "pending", "approved", "rejected"];

// Runs `fn` over items with a small concurrency limit so bulk actions that
// reuse single-movie endpoints don't flood the API.
async function runPool<T>(items: T[], limit: number, fn: (item: T) => Promise<void>): Promise<{ ok: number; failed: number }> {
  let ok = 0;
  let failed = 0;
  let index = 0;
  const workers = Array.from({ length: Math.min(limit, items.length) }, async () => {
    while (index < items.length) {
      const item = items[index++];
      try {
        await fn(item);
        ok++;
      } catch {
        failed++;
      }
    }
  });
  await Promise.all(workers);
  return { ok, failed };
}

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
  const toast = useToast();
  const [movies, setMovies] = useState<Movie[]>([]);
  const [filtered, setFiltered] = useState<Movie[]>([]);
  const [loading, setLoading] = useState(true);
  // Filters live in the URL (?q=&status=&page=) so refresh/back and shared
  // links keep the same view.
  const [search, setSearchState] = useState(() => readUrlParam("q", ""));
  const [statusFilter, setStatusFilterState] = useState<StatusFilter>(() => {
    const v = readUrlParam("status", "all") as StatusFilter;
    return STATUS_FILTERS.includes(v) ? v : "all";
  });
  const [deleting, setDeleting] = useState<string | null>(null);
  const [deleteJobId, setDeleteJobId] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Movie | null>(null);
  const [approving, setApproving] = useState<string | null>(null);
  const [rejecting, setRejecting] = useState<string | null>(null);
  // Client-side pagination so the admin list doesn't render hundreds of rows
  // at once and stretch the page.
  const PAGE_SIZE = 20;
  const [page, setPage] = useState(() => readUrlNumber("page", 1));
  const [selected, setSelected] = useState<Set<string>>(new Set());

  // Filter changes reset to page 1 in the same update (a follow-up effect
  // would also fire on mount and wipe ?page= from the URL).
  // Selection is cleared too, so a bulk action never hits rows the new
  // filter hides.
  const setSearch = (v: string) => {
    setSearchState(v);
    setPage(1);
    setSelected(new Set());
  };
  const setStatusFilter = (v: StatusFilter) => {
    setStatusFilterState(v);
    setPage(1);
    setSelected(new Set());
  };

  useSyncUrlParams({ q: search, status: statusFilter, page }, { q: "", status: "all", page: 1 });

  // ── Bulk selection ──
  const [bulkBusy, setBulkBusy] = useState<string | null>(null);
  const [collections, setCollections] = useState<CollectionInput[]>([]);
  const [collectionPick, setCollectionPick] = useState("");

  const fetchMovies = async () => {
    if (!token) return;
    try {
      const data = await adminGetMovies(token);
      setMovies(data || []);
    } catch (err) {
      console.error(err);
      toast.error("Kinolarni yuklab bo'lmadi");
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

  const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const safePage = Math.min(page, totalPages);
  const pageItems = filtered.slice((safePage - 1) * PAGE_SIZE, safePage * PAGE_SIZE);

  // Drop selections for movies that no longer exist (e.g. after delete).
  useEffect(() => {
    setSelected((prev) => {
      if (prev.size === 0) return prev;
      const ids = new Set(movies.map((m) => m.id));
      const next = new Set(Array.from(prev).filter((id) => ids.has(id)));
      return next.size === prev.size ? prev : next;
    });
  }, [movies]);

  useEffect(() => {
    if (!token || selected.size === 0 || collections.length > 0) return;
    getAdminCollections(token)
      .then(setCollections)
      .catch(() => {});
  }, [token, selected.size, collections.length]);

  const pageIds = useMemo(() => pageItems.map((m) => m.id), [pageItems]);
  const allOnPageSelected = pageIds.length > 0 && pageIds.every((id) => selected.has(id));

  const toggleOne = (id: string) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  const togglePage = () =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (allOnPageSelected) pageIds.forEach((id) => next.delete(id));
      else pageIds.forEach((id) => next.add(id));
      return next;
    });

  const selectedMovies = movies.filter((m) => selected.has(m.id));

  const reportBulk = (verb: string, r: { ok: number; failed: number }) => {
    if (r.failed === 0) toast.success(`${r.ok} ta kino ${verb}`);
    else toast.error(`${r.ok} ta kino ${verb}, ${r.failed} tasida xato`);
  };

  const bulkApprove = async () => {
    if (!token) return;
    const targets = selectedMovies.filter((m) => m.approval_status !== "approved");
    if (targets.length === 0) return toast.info("Tanlanganlar allaqachon tasdiqlangan");
    if (!window.confirm(`${targets.length} ta kino tasdiqlansinmi? Har biri uchun Telegram post yuborilishi mumkin.`)) return;
    setBulkBusy("approve");
    const done = new Set<string>();
    const r = await runPool(targets, 3, async (m) => {
      await approveMovie(token, m.id);
      done.add(m.id);
    });
    setMovies((prev) => prev.map((m) => (done.has(m.id) ? { ...m, approval_status: "approved", is_published: true } : m)));
    reportBulk("tasdiqlandi", r);
    setBulkBusy(null);
  };

  const bulkReject = async () => {
    if (!token) return;
    const targets = selectedMovies.filter((m) => m.approval_status !== "rejected");
    if (targets.length === 0) return toast.info("Tanlanganlar allaqachon rad etilgan");
    if (!window.confirm(`${targets.length} ta kino rad etilsinmi? Ular saytdan yashiriladi.`)) return;
    setBulkBusy("reject");
    const done = new Set<string>();
    const r = await runPool(targets, 3, async (m) => {
      await rejectMovie(token, m._id || m.id);
      done.add(m.id);
    });
    setMovies((prev) => prev.map((m) => (done.has(m.id) ? { ...m, approval_status: "rejected", is_published: false } : m)));
    reportBulk("rad etildi", r);
    setBulkBusy(null);
  };

  const bulkPremium = async (isPremium: boolean) => {
    if (!token) return;
    const ids = selectedMovies.map((m) => m.id);
    setBulkBusy(isPremium ? "premium" : "free");
    try {
      await adminBulkUpdateMovies(token, { ids, is_premium: isPremium });
      const idSet = new Set(ids);
      setMovies((prev) => prev.map((m) => (idSet.has(m.id) ? { ...m, is_premium: isPremium } : m)));
      toast.success(`${ids.length} ta kino ${isPremium ? "premium qilindi" : "bepul qilindi"}`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Xatolik");
    } finally {
      setBulkBusy(null);
    }
  };

  const bulkAddToCollection = async () => {
    if (!token || !collectionPick) return;
    const ids = selectedMovies.map((m) => m.id);
    const col = collections.find((c) => c.id === collectionPick);
    setBulkBusy("collection");
    try {
      await adminBulkUpdateMovies(token, { ids, add_to_collection: collectionPick });
      toast.success(`${ids.length} ta kino "${col?.title ?? "kolleksiya"}"ga qo'shildi`);
      setCollectionPick("");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Xatolik");
    } finally {
      setBulkBusy(null);
    }
  };

  const bulkDelete = async () => {
    if (!token) return;
    const targets = selectedMovies;
    if (
      !window.confirm(
        `${targets.length} ta kino butunlay o'chirilsinmi?\n\nVideo fayllar, kliplar, izohlar va boshqa bog'liq ma'lumotlar ham o'chadi. Bu amalni qaytarib bo'lmaydi.`
      )
    )
      return;
    setBulkBusy("delete");
    const done = new Set<string>();
    const r = await runPool(targets, 2, async (m) => {
      await adminDeleteMovie(token, m.id);
      done.add(m.id);
    });
    // Deletions run as background jobs; hide them from the list right away.
    setMovies((prev) => prev.filter((m) => !done.has(m.id)));
    reportBulk("o'chirish navbatiga qo'yildi", r);
    setBulkBusy(null);
  };

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
        if (warning) toast.error(`"${movie.title}" o'chirildi, lekin ba'zi fayllar muammoli: ${warning}`);
        else toast.success(`"${movie.title}" o'chirildi`);
        setDeleteTarget(null);
      }
    } catch (err: any) {
      toast.error(err.message || "O'chirishda xatolik");
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
      toast.error(err instanceof Error ? err.message : "Tasdiqlashda xato");
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
      toast.error(err instanceof Error ? err.message : "Rad etishda xato");
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

      {/* Bulk actions bar */}
      {selected.size > 0 && (
        <div className="sticky top-0 lg:top-2 z-30 mb-4 flex flex-wrap items-center gap-2 rounded-xl border border-brand-red/40 bg-brand-card/95 px-3 py-2.5 backdrop-blur">
          <span className="text-sm font-medium text-white mr-1">{selected.size} ta tanlandi</span>
          <button onClick={bulkApprove} disabled={!!bulkBusy} className="inline-flex items-center gap-1.5 rounded-lg border border-brand-border px-2.5 py-1.5 text-xs text-green-400 hover:bg-green-500/10 disabled:opacity-50">
            {bulkBusy === "approve" ? <Loader2 size={13} className="animate-spin" /> : <CheckCircle size={13} />} Tasdiqlash
          </button>
          <button onClick={bulkReject} disabled={!!bulkBusy} className="inline-flex items-center gap-1.5 rounded-lg border border-brand-border px-2.5 py-1.5 text-xs text-red-400 hover:bg-red-500/10 disabled:opacity-50">
            {bulkBusy === "reject" ? <Loader2 size={13} className="animate-spin" /> : <XCircle size={13} />} Rad etish
          </button>
          <button onClick={() => bulkPremium(true)} disabled={!!bulkBusy} className="inline-flex items-center gap-1.5 rounded-lg border border-brand-border px-2.5 py-1.5 text-xs text-amber-300 hover:bg-amber-500/10 disabled:opacity-50">
            {bulkBusy === "premium" ? <Loader2 size={13} className="animate-spin" /> : <Crown size={13} />} Premium
          </button>
          <button onClick={() => bulkPremium(false)} disabled={!!bulkBusy} className="inline-flex items-center gap-1.5 rounded-lg border border-brand-border px-2.5 py-1.5 text-xs text-gray-300 hover:bg-white/5 disabled:opacity-50">
            {bulkBusy === "free" ? <Loader2 size={13} className="animate-spin" /> : <Crown size={13} className="opacity-50" />} Bepul
          </button>
          <div className="inline-flex items-center gap-1">
            <select
              value={collectionPick}
              onChange={(e) => setCollectionPick(e.target.value)}
              className="max-w-[180px] rounded-lg border border-brand-border bg-brand-dark px-2 py-1.5 text-xs text-white"
              aria-label="Kolleksiya tanlash"
            >
              <option value="">Kolleksiyaga...</option>
              {collections.map((c) => (
                <option key={c.id} value={c.id}>{c.title}</option>
              ))}
            </select>
            <button onClick={bulkAddToCollection} disabled={!!bulkBusy || !collectionPick} className="inline-flex items-center gap-1.5 rounded-lg border border-brand-border px-2.5 py-1.5 text-xs text-blue-300 hover:bg-blue-500/10 disabled:opacity-50">
              {bulkBusy === "collection" ? <Loader2 size={13} className="animate-spin" /> : <FolderPlus size={13} />} Qo&apos;shish
            </button>
          </div>
          <button onClick={bulkDelete} disabled={!!bulkBusy} className="inline-flex items-center gap-1.5 rounded-lg border border-red-500/40 px-2.5 py-1.5 text-xs text-red-400 hover:bg-red-500/10 disabled:opacity-50">
            {bulkBusy === "delete" ? <Loader2 size={13} className="animate-spin" /> : <Trash2 size={13} />} O&apos;chirish
          </button>
          <button onClick={() => setSelected(new Set())} className="ml-auto inline-flex items-center gap-1 text-xs text-gray-500 hover:text-white">
            <X size={13} /> Bekor qilish
          </button>
        </div>
      )}

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
        <>
        <div className="bg-brand-card border border-brand-border rounded-xl overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-brand-border text-gray-500 text-xs uppercase tracking-wider">
                  <th className="w-10 pl-3 sm:pl-5 py-3">
                    <input
                      type="checkbox"
                      checked={allOnPageSelected}
                      onChange={togglePage}
                      className="accent-brand-red"
                      aria-label="Sahifadagi barcha kinolarni tanlash"
                    />
                  </th>
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
                {pageItems.map((movie) => {
                  const adminPosterSrc = normalizeMediaUrl(movie.poster_url);
                  return (
                  <tr
                    key={movie.id}
                    className={`border-b border-brand-border/50 last:border-0 hover:bg-brand-border/20 transition-colors ${
                      selected.has(movie.id) ? "bg-brand-red/5" : ""
                    }`}
                  >
                    <td className="w-10 pl-3 sm:pl-5 py-3">
                      <input
                        type="checkbox"
                        checked={selected.has(movie.id)}
                        onChange={() => toggleOne(movie.id)}
                        className="accent-brand-red"
                        aria-label={`${movie.title} ni tanlash`}
                      />
                    </td>
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
                            {movie.is_premium && (
                              <Crown size={11} className="inline ml-1 -mt-0.5 text-amber-400" aria-label="Premium" />
                            )}
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
        {totalPages > 1 && (
          <div className="flex items-center justify-center gap-3 mt-4">
            <button
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              disabled={safePage <= 1}
              className="px-3 py-1.5 bg-brand-card border border-brand-border rounded-lg text-sm text-gray-300 disabled:opacity-40 hover:border-gray-500 transition-colors"
            >
              Oldingi
            </button>
            <span className="text-sm text-gray-500">
              {safePage} / {totalPages} · {filtered.length} ta
            </span>
            <button
              onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
              disabled={safePage >= totalPages}
              className="px-3 py-1.5 bg-brand-card border border-brand-border rounded-lg text-sm text-gray-300 disabled:opacity-40 hover:border-gray-500 transition-colors"
            >
              Keyingi
            </button>
          </div>
        )}
        </>
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
