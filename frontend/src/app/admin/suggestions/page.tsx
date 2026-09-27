"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { CheckCircle2, Clock, ExternalLink, Film, Image as ImageIcon, Lightbulb, Link2, Loader2, MessageSquareText, Search, Tv, XCircle } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import {
  Suggestion,
  adminListSuggestions,
  adminUpdateSuggestion,
  adminGetSuggestionStats,
  adminLinkSuggestion,
  adminGlobalSearch,
  AdminSearchResult,
} from "@/lib/api";
import { useToast } from "@/components/admin/Toast";
import { normalizeMediaUrl } from "@/lib/image-utils";
import { Avatar, Chip, EmptyState, IconBtn, Modal, PageHead, Pager, SkeletonList, StatTile, Tabs, Thumb, Tone, fmtDate, timeAgo } from "@/components/admin/kit";
import { inputCls } from "@/components/admin/form/ui";

type StatusFilter = "all" | "pending" | "accepted" | "rejected";

const STATUS: Record<string, { label: string; tone: Tone }> = {
  pending: { label: "Kutmoqda", tone: "yellow" },
  accepted: { label: "Qabul qilindi", tone: "green" },
  rejected: { label: "Rad etildi", tone: "red" },
};

const nameOf = (s: Suggestion) => s.user?.full_name || s.user?.username || s.user?.telegram_username || s.user_name || `Foydalanuvchi ${s.user_id?.slice(-4) ?? ""}`;

const QUICK_REPLIES = {
  accept: ["Rahmat! Tez orada qo'shamiz", "Qo'shildi, tomosha qiling!"],
  reject: ["Afsuski, bu kontentni topa olmadik", "Mualliflik huquqi sababli qo'sha olmaymiz", "Allaqachon saytda bor"],
};

function LinkContentModal({ token, suggestion, onClose, onLinked }: { token: string; suggestion: Suggestion; onClose: () => void; onLinked: () => void }) {
  const toast = useToast();
  const [q, setQ] = useState(suggestion.title);
  const [results, setResults] = useState<AdminSearchResult[]>([]);
  const [searching, setSearching] = useState(false);
  const [linking, setLinking] = useState<string | null>(null);

  useEffect(() => {
    const query = q.trim();
    if (query.length < 2) {
      setResults([]);
      return;
    }
    const ctrl = new AbortController();
    setSearching(true);
    const t = setTimeout(() => {
      adminGlobalSearch(token, query, ctrl.signal)
        .then((r) => setResults(r.filter((x) => x.kind === "movie" || x.kind === "series")))
        .catch(() => {})
        .finally(() => !ctrl.signal.aborted && setSearching(false));
    }, 250);
    return () => {
      clearTimeout(t);
      ctrl.abort();
    };
  }, [q, token]);

  const link = async (r: AdminSearchResult) => {
    setLinking(r.id);
    try {
      const res = await adminLinkSuggestion(token, suggestion.id, r.kind as "movie" | "series", r.id);
      toast.success(res.notified ? "Bog'landi — foydalanuvchiga xabar yuborildi" : "Bog'landi");
      onLinked();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Bog'lab bo'lmadi");
    } finally {
      setLinking(null);
    }
  };

  return (
    <Modal open onClose={onClose} icon={Link2} iconTone="green" title="Qo'shilgan kontentni tanlang" subtitle={`«${suggestion.title}» — foydalanuvchiga havola bilan xabar boradi`}>
      <div className="relative mb-3">
        <Search size={15} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" />
        <input autoFocus value={q} onChange={(e) => setQ(e.target.value)} placeholder="Kino yoki serial nomi / kodi" className={`${inputCls} pl-9`} />
        {searching && <Loader2 size={15} className="absolute right-3 top-1/2 -translate-y-1/2 animate-spin text-gray-500" />}
      </div>
      <ul className="max-h-80 space-y-1 overflow-y-auto">
        {results.length === 0 ? (
          <li className="py-8 text-center text-sm text-gray-500">{searching ? "Qidirilmoqda..." : "Hech narsa topilmadi"}</li>
        ) : (
          results.map((r) => (
            <li key={`${r.kind}:${r.id}`} className="flex items-center gap-3 rounded-xl p-2 hover:bg-white/5">
              <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-white/5 text-gray-400">{r.kind === "series" ? <Tv size={16} /> : <Film size={16} />}</span>
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm text-white">{r.title}</p>
                <p className="truncate text-xs text-gray-500">{r.subtitle}</p>
              </div>
              <button
                onClick={() => link(r)}
                disabled={!!linking}
                className="inline-flex items-center gap-1.5 rounded-lg bg-emerald-500/15 px-3 py-1.5 text-xs text-emerald-300 hover:bg-emerald-500/25 disabled:opacity-50"
              >
                {linking === r.id ? <Loader2 size={13} className="animate-spin" /> : <Link2 size={13} />}
                Bog&apos;lash
              </button>
            </li>
          ))
        )}
      </ul>
    </Modal>
  );
}

export default function AdminSuggestionsPage() {
  const { token } = useAuth();
  const toast = useToast();
  const [linkTarget, setLinkTarget] = useState<Suggestion | null>(null);
  const [suggestions, setSuggestions] = useState<Suggestion[]>([]);
  const [loading, setLoading] = useState(true);
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("pending");
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [stats, setStats] = useState({ total: 0, pending: 0, accepted: 0, rejected: 0 });
  const [action, setAction] = useState<{ s: Suggestion; kind: "accept" | "reject"; message: string } | null>(null);
  const [saving, setSaving] = useState(false);
  const [preview, setPreview] = useState<string | null>(null);
  const limit = 20;

  const fetchList = useCallback(async () => {
    if (!token) return;
    setLoading(true);
    try {
      const data = await adminListSuggestions(token, page, limit, statusFilter);
      setSuggestions(data.suggestions || []);
      setTotal(data.total || 0);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Yuklab bo'lmadi");
    } finally {
      setLoading(false);
    }
  }, [token, page, statusFilter, toast]);

  const fetchStats = useCallback(async () => {
    if (!token) return;
    try {
      setStats(await adminGetSuggestionStats(token));
    } catch {
      /* optional */
    }
  }, [token]);

  useEffect(() => {
    fetchList();
  }, [fetchList]);
  useEffect(() => {
    fetchStats();
  }, [fetchStats]);

  const submitAction = async () => {
    if (!token || !action) return;
    setSaving(true);
    try {
      await adminUpdateSuggestion(token, action.s.id, { status: action.kind === "accept" ? "accepted" : "rejected", admin_message: action.message });
      toast.success(action.kind === "accept" ? "Tavsiya qabul qilindi" : "Tavsiya rad etildi");
      setAction(null);
      fetchList();
      fetchStats();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Xatolik yuz berdi");
    } finally {
      setSaving(false);
    }
  };

  const pickFilter = (f: StatusFilter) => {
    setStatusFilter(f);
    setPage(1);
  };

  return (
    <div className="mx-auto max-w-6xl p-4 sm:p-6">
      <PageHead icon={Lightbulb} gradient="from-amber-400 to-orange-600" title="Tavsiyalar" subtitle="Foydalanuvchilar qo'shishni so'ragan kino va seriallar" />

      <div className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatTile icon={Clock} tone="yellow" label="Kutmoqda" value={stats.pending} active={statusFilter === "pending"} onClick={() => pickFilter("pending")} />
        <StatTile icon={CheckCircle2} tone="green" label="Qabul qilindi" value={stats.accepted} active={statusFilter === "accepted"} onClick={() => pickFilter("accepted")} />
        <StatTile icon={XCircle} tone="red" label="Rad etildi" value={stats.rejected} active={statusFilter === "rejected"} onClick={() => pickFilter("rejected")} />
        <StatTile icon={Lightbulb} tone="gray" label="Jami" value={stats.total} active={statusFilter === "all"} onClick={() => pickFilter("all")} />
      </div>

      <div className="mb-4">
        <Tabs<StatusFilter>
          value={statusFilter}
          onChange={pickFilter}
          items={[
            { key: "pending", label: "Kutmoqda", count: stats.pending },
            { key: "accepted", label: "Qabul qilingan", count: stats.accepted },
            { key: "rejected", label: "Rad etilgan", count: stats.rejected },
            { key: "all", label: "Hammasi", count: stats.total },
          ]}
        />
      </div>

      {loading ? (
        <SkeletonList rows={5} height={112} />
      ) : suggestions.length === 0 ? (
        <EmptyState icon={Lightbulb} title={statusFilter === "pending" ? "Yangi tavsiya yo'q" : "Tavsiya topilmadi"} text={statusFilter === "pending" ? "Hammasi ko'rib chiqilgan 🎉" : undefined} />
      ) : (
        <ul className="grid gap-3 lg:grid-cols-2">
          {suggestions.map((s) => {
            const st = STATUS[s.status] ?? STATUS.pending;
            const img = s.image_url ? normalizeMediaUrl(s.image_url) : "";
            return (
              <li key={s.id} className="flex flex-col rounded-2xl border border-white/10 bg-[#12121a] p-4 transition hover:border-white/20">
                <div className="flex gap-3">
                  {img ? (
                    <button type="button" onClick={() => setPreview(img)} className="h-20 w-14 shrink-0 overflow-hidden rounded-lg" aria-label="Rasmni ko'rish">
                      <Thumb src={img} className="h-full w-full" icon={s.type === "series" ? Tv : Film} />
                    </button>
                  ) : (
                    <span className="flex h-20 w-14 shrink-0 items-center justify-center rounded-lg bg-white/5 text-gray-600">{s.type === "series" ? <Tv size={18} /> : <Film size={18} />}</span>
                  )}
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-1.5">
                      <Chip tone={st.tone} dot>
                        {st.label}
                      </Chip>
                      <Chip tone={s.type === "series" ? "blue" : "violet"} icon={s.type === "series" ? Tv : Film}>
                        {s.type === "series" ? "Serial" : "Kino"}
                      </Chip>
                    </div>
                    <h3 className="mt-1.5 truncate font-semibold text-white" title={s.title}>
                      {s.title}
                    </h3>
                    <Link href={`/user/${s.user_id}`} className="mt-1 inline-flex items-center gap-1.5 text-xs text-gray-400 hover:text-white">
                      <Avatar name={nameOf(s)} size={18} />
                      {nameOf(s)}
                      <span className="text-gray-600" title={fmtDate(s.created_at)}>
                        · {timeAgo(s.created_at)}
                      </span>
                    </Link>
                  </div>
                </div>

                {s.message && (
                  <p className="mt-3 line-clamp-3 rounded-xl bg-black/20 px-3 py-2 text-sm text-gray-300">
                    <MessageSquareText size={12} className="mr-1 inline text-gray-500" />
                    {s.message}
                  </p>
                )}
                {s.admin_message && <p className="mt-2 text-xs text-emerald-300/90">Javob: {s.admin_message}</p>}

                <div className="mt-auto flex flex-wrap items-center gap-2 pt-3">
                  {s.source_url && (
                    <a href={s.source_url} target="_blank" rel="noopener noreferrer" className="inline-flex items-center gap-1 text-xs text-sky-400 hover:underline">
                      <ExternalLink size={12} /> Manba
                    </a>
                  )}
                  {img && (
                    <button type="button" onClick={() => setPreview(img)} className="inline-flex items-center gap-1 text-xs text-sky-400 hover:underline">
                      <ImageIcon size={12} /> Rasm
                    </button>
                  )}
                  {s.linked_slug && (
                    <a
                      href={`/${s.linked_type === "series" ? "series" : "movies"}/${s.linked_slug}`}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="inline-flex max-w-[220px] items-center gap-1 truncate text-xs text-emerald-300 hover:underline"
                    >
                      <Link2 size={12} /> {s.linked_title || s.linked_slug}
                    </a>
                  )}
                  <div className="ml-auto flex items-center gap-0.5">
                    {s.status !== "rejected" && !s.linked_id && (
                      <button onClick={() => setLinkTarget(s)} className="inline-flex items-center gap-1.5 rounded-lg bg-emerald-500/15 px-2.5 py-1.5 text-xs font-medium text-emerald-300 hover:bg-emerald-500/25">
                        <Link2 size={13} /> Qo&apos;shildi
                      </button>
                    )}
                    {s.status === "pending" && (
                      <>
                        <IconBtn label="Qabul qilish" tone="green" onClick={() => setAction({ s, kind: "accept", message: "" })}>
                          <CheckCircle2 size={16} />
                        </IconBtn>
                        <IconBtn label="Rad etish" tone="red" onClick={() => setAction({ s, kind: "reject", message: "" })}>
                          <XCircle size={16} />
                        </IconBtn>
                      </>
                    )}
                  </div>
                </div>
              </li>
            );
          })}
        </ul>
      )}

      <Pager page={page} totalPages={Math.ceil(total / limit)} total={total} unit="ta tavsiya" onChange={setPage} />

      <Modal
        open={!!action}
        onClose={() => setAction(null)}
        busy={saving}
        size="md"
        icon={action?.kind === "accept" ? CheckCircle2 : XCircle}
        iconTone={action?.kind === "accept" ? "green" : "red"}
        title={action?.kind === "accept" ? "Tavsiyani qabul qilish" : "Tavsiyani rad etish"}
        subtitle={action ? `«${action.s.title}»` : undefined}
        footer={
          <>
            <button type="button" onClick={() => setAction(null)} className="rounded-xl px-4 py-2 text-sm text-gray-300 hover:bg-white/5">
              Bekor qilish
            </button>
            <button
              type="button"
              onClick={submitAction}
              disabled={saving}
              className={`inline-flex items-center gap-2 rounded-xl px-5 py-2 text-sm font-semibold text-white disabled:opacity-50 ${action?.kind === "accept" ? "bg-emerald-600 hover:bg-emerald-500" : "bg-red-600 hover:bg-red-500"}`}
            >
              {saving && <Loader2 size={15} className="animate-spin" />}
              {action?.kind === "accept" ? "Qabul qilish" : "Rad etish"}
            </button>
          </>
        }
      >
        {action && (
          <div className="space-y-3">
            <label className="block text-xs font-medium text-gray-400" htmlFor="sugg-msg">
              Foydalanuvchiga xabar (ixtiyoriy)
            </label>
            <textarea id="sugg-msg" rows={3} value={action.message} onChange={(e) => setAction({ ...action, message: e.target.value })} placeholder="Foydalanuvchiga xabar..." className={`${inputCls} resize-none`} />
            <div className="flex flex-wrap gap-1.5">
              {QUICK_REPLIES[action.kind].map((q) => (
                <button key={q} type="button" onClick={() => setAction({ ...action, message: q })} className="rounded-full border border-white/10 px-2.5 py-1 text-[11px] text-gray-400 hover:border-white/25 hover:text-white">
                  {q}
                </button>
              ))}
            </div>
            {action.kind === "accept" && <p className="text-xs text-gray-500">Kino saytga qo&apos;shilgach «Qo&apos;shildi» tugmasi bilan bog&apos;lang — foydalanuvchiga havola boradi.</p>}
          </div>
        )}
      </Modal>

      {linkTarget && token && (
        <LinkContentModal
          token={token}
          suggestion={linkTarget}
          onClose={() => setLinkTarget(null)}
          onLinked={() => {
            setLinkTarget(null);
            fetchList();
            fetchStats();
          }}
        />
      )}

      {preview && (
        <div className="fixed inset-0 z-[160] flex items-center justify-center bg-black/85 p-4" onClick={() => setPreview(null)}>
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img src={preview} alt="" className="max-h-[85vh] max-w-full rounded-xl object-contain" />
        </div>
      )}
    </div>
  );
}
