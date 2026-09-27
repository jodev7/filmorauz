"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { CheckCircle2, Clock, Crown, ExternalLink, Eye, Loader2, Pencil, Plus, Star, Trash2, Tv, XCircle } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { adminGetSeries, adminDeleteSeries, approveSeries, rejectSeries, AdminSeries } from "@/lib/api";
import { normalizeMediaUrl } from "@/lib/image-utils";
import { localizeSingleGenre } from "@/lib/localization";
import { useToast } from "@/components/admin/Toast";
import DeleteProgressModal from "@/components/admin/DeleteProgressModal";
import CascadeDeleteModal, { formatCascadeWarning } from "@/components/admin/CascadeDeleteModal";
import { Chip, EmptyState, IconBtn, PageHead, Pager, SearchBox, SkeletonList, StatTile, Tabs, Thumb, Tone, timeAgo } from "@/components/admin/kit";

type StatusFilter = "all" | "pending" | "approved" | "rejected";
type Sort = "new" | "views" | "rating" | "title";

const STATUS: Record<string, { label: string; tone: Tone }> = {
  approved: { label: "Saytda", tone: "green" },
  pending: { label: "Kutmoqda", tone: "yellow" },
  rejected: { label: "Rad etilgan", tone: "red" },
};
const statusOf = (s: AdminSeries) => s.approval_status || "approved";
const PAGE_SIZE = 20;

export default function AdminSeriesPage() {
  const { token } = useAuth();
  const toast = useToast();
  const [series, setSeries] = useState<AdminSeries[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState<StatusFilter>("all");
  const [sort, setSort] = useState<Sort>("new");
  const [page, setPage] = useState(1);
  const [busy, setBusy] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<AdminSeries | null>(null);
  const [deleteJobId, setDeleteJobId] = useState<string | null>(null);

  useEffect(() => {
    if (!token) return;
    adminGetSeries(token)
      .then((d) => setSeries(d || []))
      .catch((e) => toast.error(e instanceof Error ? e.message : "Yuklab bo'lmadi"))
      .finally(() => setLoading(false));
  }, [token, toast]);

  useEffect(() => setPage(1), [search, status, sort]);

  const counts = useMemo(() => {
    const c: Record<StatusFilter, number> = { all: series.length, pending: 0, approved: 0, rejected: 0 };
    series.forEach((s) => c[statusOf(s) as StatusFilter]++);
    return c;
  }, [series]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    const list = series.filter(
      (s) =>
        (status === "all" || statusOf(s) === status) &&
        (!q || s.title.toLowerCase().includes(q) || s.slug.toLowerCase().includes(q) || (s.code || "").toLowerCase().includes(q) || s.genre?.some((g) => g.toLowerCase().includes(q)))
    );
    const by: Record<Sort, (a: AdminSeries, b: AdminSeries) => number> = {
      new: (a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
      views: (a, b) => (b.views || 0) - (a.views || 0),
      rating: (a, b) => (b.rating_avg || 0) - (a.rating_avg || 0),
      title: (a, b) => a.title.localeCompare(b.title),
    };
    return [...list].sort(by[sort]);
  }, [series, search, status, sort]);

  const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const pageItems = filtered.slice((Math.min(page, totalPages) - 1) * PAGE_SIZE, Math.min(page, totalPages) * PAGE_SIZE);

  const setApproval = async (s: AdminSeries, approve: boolean) => {
    if (!token) return;
    setBusy(s.id);
    try {
      if (approve) await approveSeries(token, s.id);
      else await rejectSeries(token, s.id);
      setSeries((prev) => prev.map((x) => (x.id === s.id ? { ...x, approval_status: approve ? "approved" : "rejected", is_published: approve } : x)));
      toast.success(approve ? `«${s.title}» saytga chiqarildi` : `«${s.title}» rad etildi`);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Xatolik");
    } finally {
      setBusy(null);
    }
  };

  const performDelete = async () => {
    const s = deleteTarget;
    if (!s || !token) return;
    setBusy(s.id);
    try {
      const res = await adminDeleteSeries(token, s.id);
      if (res.job_id) {
        setDeleteJobId(res.job_id);
      } else {
        setSeries((prev) => prev.filter((x) => x.id !== s.id));
        const warning = formatCascadeWarning(res?.deleted_b2);
        if (warning) toast.error(`O'chirildi, lekin ba'zi fayllar muammoli: ${warning}`);
        else toast.success(`«${s.title}» o'chirildi`);
        setDeleteTarget(null);
        setBusy(null);
      }
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "O'chirishda xatolik");
      setBusy(null);
    }
  };

  return (
    <div className="mx-auto max-w-7xl p-4 sm:p-6">
      <PageHead
        icon={Tv}
        gradient="from-indigo-500 to-violet-700"
        title="Seriallar"
        subtitle={`${series.length} ta serial`}
        actions={
          <Link href="/admin/series/new" className="inline-flex items-center gap-2 rounded-xl bg-orange-500 px-4 py-2.5 text-sm font-semibold text-white shadow-lg shadow-orange-500/20 hover:bg-orange-400">
            <Plus size={16} /> Yangi serial
          </Link>
        }
      />

      <div className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatTile icon={Tv} tone="violet" label="Jami" value={counts.all} active={status === "all"} onClick={() => setStatus("all")} />
        <StatTile icon={CheckCircle2} tone="green" label="Saytda" value={counts.approved} active={status === "approved"} onClick={() => setStatus("approved")} />
        <StatTile icon={Clock} tone="yellow" label="Tasdiq kutmoqda" value={counts.pending} active={status === "pending"} onClick={() => setStatus("pending")} />
        <StatTile icon={XCircle} tone="red" label="Rad etilgan" value={counts.rejected} active={status === "rejected"} onClick={() => setStatus("rejected")} />
      </div>

      <div className="mb-4 flex flex-col gap-3 lg:flex-row">
        <SearchBox value={search} onChange={setSearch} placeholder="Nomi, kodi, slug yoki janr..." />
        <Tabs<Sort>
          value={sort}
          onChange={setSort}
          items={[
            { key: "new", label: "Yangilari" },
            { key: "views", label: "Ko'p ko'rilgan" },
            { key: "rating", label: "Reyting" },
            { key: "title", label: "A–Z" },
          ]}
        />
      </div>

      {loading ? (
        <SkeletonList rows={6} height={96} />
      ) : filtered.length === 0 ? (
        <EmptyState
          icon={Tv}
          title="Serial topilmadi"
          text={search || status !== "all" ? "Qidiruv yoki filtrni o'zgartirib ko'ring." : "Birinchi serialni qo'shing."}
          action={
            !search && status === "all" ? (
              <Link href="/admin/series/new" className="inline-flex items-center gap-2 rounded-xl bg-white/10 px-4 py-2 text-sm text-white hover:bg-white/15">
                <Plus size={15} /> Serial qo&apos;shish
              </Link>
            ) : undefined
          }
        />
      ) : (
        <ul className="grid gap-3 xl:grid-cols-2">
          {pageItems.map((s) => {
            const st = STATUS[statusOf(s)] ?? STATUS.approved;
            const isBusy = busy === s.id;
            return (
              <li key={s.id} className={`flex gap-3.5 rounded-2xl border border-white/10 bg-[#12121a] p-3 transition hover:border-white/20 ${isBusy ? "opacity-60" : ""}`}>
                <Link href={`/admin/series/${s.id}/edit`} className="shrink-0">
                  <Thumb src={s.poster_url ? normalizeMediaUrl(s.poster_url) : undefined} alt={s.title} className="h-[104px] w-[70px] rounded-lg" icon={Tv} />
                </Link>
                <div className="flex min-w-0 flex-1 flex-col">
                  <div className="flex flex-wrap items-center gap-1.5">
                    <Chip tone={st.tone} dot>
                      {st.label}
                    </Chip>
                    {s.is_premium && (
                      <Chip tone="yellow" icon={Crown}>
                        Premium
                      </Chip>
                    )}
                    {s.is_completed && <Chip tone="blue">Tugagan</Chip>}
                    {s.code && <span className="font-mono text-[11px] text-orange-300">#{s.code}</span>}
                  </div>
                  <Link href={`/admin/series/${s.id}/edit`} className="mt-1 truncate font-semibold text-white hover:text-orange-300" title={s.title}>
                    {s.title}
                  </Link>
                  <p className="truncate text-xs text-gray-500">
                    {s.year || "—"}
                    {s.genre?.length ? ` · ${s.genre.slice(0, 3).map(localizeSingleGenre).join(", ")}` : ""}
                    {s.country ? ` · ${s.country}` : ""}
                  </p>
                  <div className="mt-auto flex flex-wrap items-center gap-3 pt-2 text-[11px] text-gray-500">
                    <span className="inline-flex items-center gap-1">
                      <Eye size={12} /> {(s.views || 0).toLocaleString()}
                    </span>
                    {s.rating_count > 0 && (
                      <span className="inline-flex items-center gap-1">
                        <Star size={12} className="text-yellow-400" /> {s.rating_avg.toFixed(1)} ({s.rating_count})
                      </span>
                    )}
                    <span>{timeAgo(s.created_at)} qo&apos;shilgan</span>
                  </div>
                </div>
                <div className="flex shrink-0 flex-col items-end justify-between gap-1">
                  <div className="flex items-center">
                    {isBusy ? (
                      <Loader2 size={16} className="m-2 animate-spin text-gray-500" />
                    ) : (
                      <>
                        {statusOf(s) !== "approved" && (
                          <IconBtn label="Tasdiqlash va saytga chiqarish" tone="green" onClick={() => setApproval(s, true)}>
                            <CheckCircle2 size={16} />
                          </IconBtn>
                        )}
                        {statusOf(s) !== "rejected" && (
                          <IconBtn label="Rad etish" tone="red" onClick={() => setApproval(s, false)}>
                            <XCircle size={16} />
                          </IconBtn>
                        )}
                      </>
                    )}
                  </div>
                  <div className="flex items-center">
                    {statusOf(s) === "approved" && (
                      <Link href={`/series/${s.slug}`} target="_blank" className="rounded-lg p-2 text-gray-400 hover:bg-white/5 hover:text-white" title="Saytda ko'rish" aria-label="Saytda ko'rish">
                        <ExternalLink size={15} />
                      </Link>
                    )}
                    <Link href={`/admin/series/${s.id}/edit`} className="rounded-lg p-2 text-gray-400 hover:bg-white/5 hover:text-white" title="Tahrirlash" aria-label="Tahrirlash">
                      <Pencil size={15} />
                    </Link>
                    <IconBtn label="O'chirish" tone="red" disabled={isBusy} onClick={() => setDeleteTarget(s)}>
                      <Trash2 size={15} />
                    </IconBtn>
                  </div>
                </div>
              </li>
            );
          })}
        </ul>
      )}

      <Pager page={Math.min(page, totalPages)} totalPages={totalPages} total={filtered.length} unit="ta serial" onChange={setPage} />

      <CascadeDeleteModal
        open={deleteTarget !== null && !deleteJobId}
        kind="series"
        title={deleteTarget?.title ?? ""}
        onConfirm={performDelete}
        onClose={() => {
          if (busy === null) setDeleteTarget(null);
        }}
      />
      {deleteJobId && (
        <DeleteProgressModal
          jobId={deleteJobId}
          isOpen
          onClose={() => {
            if (deleteTarget) setSeries((prev) => prev.filter((x) => x.id !== deleteTarget.id));
            setDeleteTarget(null);
            setDeleteJobId(null);
            setBusy(null);
          }}
        />
      )}
    </div>
  );
}
