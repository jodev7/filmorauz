"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { AlertTriangle, CheckCircle2, Clock, ExternalLink, Gavel, Loader2, MessageSquare, RefreshCw, Scale, ShieldOff, XCircle } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { isStaffRole } from "@/lib/roles";
import { getAppeals, getAppealStats, reviewAppeal, BanAppeal } from "@/lib/api";
import { useToast } from "@/components/admin/Toast";
import { Avatar, Chip, EmptyState, GhostButton, Modal, PageHead, Pager, SearchBox, SkeletonList, StatTile, Tabs, Tone, fmtDate, timeAgo, useDebounced } from "@/components/admin/kit";
import { Segmented, SwitchRow, inputCls } from "@/components/admin/form/ui";

type Filter = "all" | "pending" | "approved" | "rejected";

const STATUS: Record<string, { label: string; tone: Tone }> = {
  pending: { label: "Ko'rib chiqilmoqda", tone: "yellow" },
  approved: { label: "Qabul qilindi", tone: "green" },
  rejected: { label: "Rad etildi", tone: "red" },
};

const NOTES = {
  approve: ["Ogohlantirish bilan bandan chiqarildi", "Xato ban edi, uzr so'raymiz"],
  reject: ["Qoidabuzarlik tasdiqlandi", "Ban muddati tugashini kuting"],
};

export default function AdminAppealsPage() {
  const { user, token, isLoading: authLoading } = useAuth();
  const router = useRouter();
  const toast = useToast();

  const [appeals, setAppeals] = useState<BanAppeal[]>([]);
  const [stats, setStats] = useState({ pending: 0, approved: 0, rejected: 0, total: 0 });
  const [loading, setLoading] = useState(true);
  const [searchInput, setSearchInput] = useState("");
  const search = useDebounced(searchInput.trim(), 350);
  const [status, setStatus] = useState<Filter>("pending");
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [total, setTotal] = useState(0);

  const [review, setReview] = useState<BanAppeal | null>(null);
  const [decision, setDecision] = useState<"approve" | "reject">("approve");
  const [note, setNote] = useState("");
  const [unban, setUnban] = useState(true);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (authLoading) return;
    if (!token || !user) router.push("/admin/login");
    else if (!isStaffRole(user.role)) router.push("/");
  }, [authLoading, token, user, router]);

  useEffect(() => setPage(1), [search, status]);

  const load = useCallback(async () => {
    if (!token) return;
    setLoading(true);
    try {
      const [list, st] = await Promise.all([getAppeals(token, { page, per_page: 10, status, search: search || undefined }), getAppealStats(token)]);
      setAppeals(list.appeals);
      setTotalPages(list.total_pages);
      setTotal(list.total);
      setStats(st.stats);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Yuklab bo'lmadi");
    } finally {
      setLoading(false);
    }
  }, [token, page, status, search, toast]);

  useEffect(() => {
    load();
  }, [load]);

  const openReview = (a: BanAppeal) => {
    setReview(a);
    setDecision("approve");
    setNote("");
    setUnban(true);
  };

  const submit = async () => {
    if (!token || !review) return;
    setSubmitting(true);
    try {
      await reviewAppeal(token, review.id, { action: decision, admin_note: note || undefined, unban_user: decision === "approve" ? unban : false });
      toast.success(decision === "approve" ? (unban ? "Qabul qilindi va bandan chiqarildi" : "Apellyatsiya qabul qilindi") : "Apellyatsiya rad etildi");
      setReview(null);
      load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Xatolik yuz berdi");
    } finally {
      setSubmitting(false);
    }
  };

  if (authLoading) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="animate-spin text-gray-500" />
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-6xl p-4 sm:p-6">
      <PageHead
        icon={Scale}
        gradient="from-teal-500 to-emerald-700"
        title="Apellyatsiyalar"
        subtitle="Ban olgan foydalanuvchilarning shikoyatlari"
        actions={
          <GhostButton onClick={load}>
            <RefreshCw size={15} className={loading ? "animate-spin" : ""} /> Yangilash
          </GhostButton>
        }
      />

      <div className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatTile icon={Clock} tone="yellow" label="Kutilmoqda" value={stats.pending} active={status === "pending"} onClick={() => setStatus("pending")} />
        <StatTile icon={CheckCircle2} tone="green" label="Qabul qilingan" value={stats.approved} active={status === "approved"} onClick={() => setStatus("approved")} />
        <StatTile icon={XCircle} tone="red" label="Rad etilgan" value={stats.rejected} active={status === "rejected"} onClick={() => setStatus("rejected")} />
        <StatTile icon={MessageSquare} tone="blue" label="Jami" value={stats.total} active={status === "all"} onClick={() => setStatus("all")} />
      </div>

      <div className="mb-4 flex flex-col gap-3 lg:flex-row">
        <SearchBox value={searchInput} onChange={setSearchInput} placeholder="Username yoki Telegram ID..." />
        <Tabs<Filter>
          value={status}
          onChange={setStatus}
          items={[
            { key: "pending", label: "Kutilmoqda", count: stats.pending },
            { key: "approved", label: "Qabul", count: stats.approved },
            { key: "rejected", label: "Rad", count: stats.rejected },
            { key: "all", label: "Hammasi", count: stats.total },
          ]}
        />
      </div>

      {loading ? (
        <SkeletonList rows={4} height={140} />
      ) : appeals.length === 0 ? (
        <EmptyState icon={Scale} title={status === "pending" ? "Yangi apellyatsiya yo'q" : "Apellyatsiya topilmadi"} text={status === "pending" ? "Hammasi ko'rib chiqilgan." : undefined} />
      ) : (
        <ul className="space-y-3">
          {appeals.map((a) => {
            const st = STATUS[a.status] ?? STATUS.pending;
            return (
              <li key={a.id} className="rounded-2xl border border-white/10 bg-[#12121a] p-4 transition hover:border-white/20">
                <div className="flex flex-wrap items-start gap-3">
                  <Avatar name={a.username || "?"} size={40} />
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-medium text-white">@{a.username || "noma'lum"}</span>
                      <span className="font-mono text-[11px] text-gray-500">{a.telegram_id}</span>
                      <Chip tone={st.tone} dot>
                        {st.label}
                      </Chip>
                    </div>
                    <p className="text-xs text-gray-500" title={fmtDate(a.created_at)}>
                      {timeAgo(a.created_at)} yuborilgan
                    </p>
                  </div>
                  <div className="flex items-center gap-1.5">
                    <Link href={`/admin/users?search=${a.user_id}`} className="rounded-lg p-2 text-gray-400 hover:bg-white/5 hover:text-white" title="Foydalanuvchi" aria-label="Foydalanuvchi">
                      <ExternalLink size={15} />
                    </Link>
                    {a.status === "pending" && (
                      <button onClick={() => openReview(a)} className="inline-flex items-center gap-1.5 rounded-xl bg-orange-500 px-3.5 py-2 text-xs font-semibold text-white hover:bg-orange-400">
                        <Gavel size={14} /> Ko&apos;rib chiqish
                      </button>
                    )}
                  </div>
                </div>

                <div className="mt-3 grid gap-2.5 md:grid-cols-2">
                  <div className="rounded-xl border border-red-500/15 bg-red-500/[0.04] p-3">
                    <p className="mb-1 flex items-center gap-1.5 text-[11px] font-medium uppercase tracking-wide text-red-300/80">
                      <AlertTriangle size={12} /> Ban sababi
                    </p>
                    <p className="text-sm text-gray-200">{a.ban_reason || "Ko'rsatilmagan"}</p>
                    <p className="mt-1 text-[11px] text-gray-500">
                      {a.ban_banned_by_name && `${a.ban_banned_by_name} · `}
                      {a.ban_banned_until ? `${fmtDate(a.ban_banned_until, false)} gacha` : "doimiy"}
                    </p>
                  </div>
                  <div className="rounded-xl border border-white/10 bg-black/20 p-3">
                    <p className="mb-1 flex items-center gap-1.5 text-[11px] font-medium uppercase tracking-wide text-gray-400">
                      <MessageSquare size={12} /> Foydalanuvchi xabari
                    </p>
                    <p className="line-clamp-4 whitespace-pre-line text-sm text-gray-200">{a.message}</p>
                  </div>
                </div>

                {(a.admin_note || a.reviewed_by_username) && (
                  <p className="mt-2.5 text-xs text-gray-500">
                    {a.reviewed_by_username && <span className="text-gray-400">{a.reviewed_by_username}: </span>}
                    {a.admin_note || "izohsiz"}
                    {a.reviewed_at && ` · ${timeAgo(a.reviewed_at)}`}
                  </p>
                )}
              </li>
            );
          })}
        </ul>
      )}

      <Pager page={page} totalPages={totalPages} total={total} unit="ta apellyatsiya" onChange={setPage} />

      <Modal
        open={!!review}
        onClose={() => setReview(null)}
        busy={submitting}
        icon={Gavel}
        title="Apellyatsiyani ko'rib chiqish"
        subtitle={review ? `@${review.username || "noma'lum"}` : undefined}
        footer={
          <>
            <button type="button" onClick={() => setReview(null)} className="rounded-xl px-4 py-2 text-sm text-gray-300 hover:bg-white/5">
              Bekor qilish
            </button>
            <button
              type="button"
              onClick={submit}
              disabled={submitting}
              className={`inline-flex items-center gap-2 rounded-xl px-5 py-2 text-sm font-semibold text-white disabled:opacity-50 ${decision === "approve" ? "bg-emerald-600 hover:bg-emerald-500" : "bg-red-600 hover:bg-red-500"}`}
            >
              {submitting && <Loader2 size={15} className="animate-spin" />}
              {decision === "approve" ? "Qabul qilish" : "Rad etish"}
            </button>
          </>
        }
      >
        {review && (
          <div className="space-y-4">
            <div className="space-y-2 rounded-xl bg-black/20 p-3 text-sm">
              <p className="text-gray-400">
                <span className="text-red-300">Ban sababi:</span> {review.ban_reason || "—"}
              </p>
              <p className="whitespace-pre-line text-gray-200">{review.message}</p>
            </div>
            <Segmented<"approve" | "reject">
              ariaLabel="Qaror"
              value={decision}
              onChange={setDecision}
              options={[
                { value: "approve", label: "Qabul qilish" },
                { value: "reject", label: "Rad etish" },
              ]}
            />
            {decision === "approve" && (
              <SwitchRow
                checked={unban}
                onChange={setUnban}
                title="Bandan chiqarish"
                description="Foydalanuvchi darhol saytdan foydalana oladi"
                icon={<ShieldOff size={18} className={unban ? "text-yellow-400" : "text-gray-500"} />}
              />
            )}
            <div>
              <label htmlFor="appeal-note" className="mb-1.5 block text-xs font-medium text-gray-400">
                Admin izohi (ixtiyoriy)
              </label>
              <textarea id="appeal-note" rows={3} value={note} onChange={(e) => setNote(e.target.value)} className={`${inputCls} resize-none`} placeholder="Qarorga izoh..." />
              <div className="mt-2 flex flex-wrap gap-1.5">
                {NOTES[decision].map((n) => (
                  <button key={n} type="button" onClick={() => setNote(n)} className="rounded-full border border-white/10 px-2.5 py-1 text-[11px] text-gray-400 hover:border-white/25 hover:text-white">
                    {n}
                  </button>
                ))}
              </div>
            </div>
          </div>
        )}
      </Modal>
    </div>
  );
}
