"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { AlertTriangle, Check, CornerDownRight, Film, Heart, Link2, Loader2, MessageSquare, Settings, ShieldAlert, Trash2, X } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { isStaffRole } from "@/lib/roles";
import { getAdminComments, updateCommentStatus, adminDeleteComment, AdminComment, CommentStatus } from "@/lib/comments-api";
import { normalizeMediaUrl } from "@/lib/image-utils";
import ReportedCommentsPanel from "@/components/admin/ReportedCommentsPanel";
import { useToast } from "@/components/admin/Toast";
import { Avatar, Chip, EmptyState, IconBtn, PageHead, Pager, SearchBox, SkeletonList, Tabs, Tone, fmtDate, timeAgo, useDebounced } from "@/components/admin/kit";

type Filter = CommentStatus | "all";

const STATUS: Record<string, { label: string; tone: Tone }> = {
  approved: { label: "Tasdiqlangan", tone: "green" },
  pending: { label: "Kutilmoqda", tone: "yellow" },
  rejected: { label: "Rad etilgan", tone: "red" },
};

const authorOf = (c: AdminComment) => c.user?.display_name || c.user?.username || c.user_display_name || "O'chirilgan foydalanuvchi";
const targetHref = (c: AdminComment) =>
  c.target_url || (c.target_type === "episode" && c.target_id ? `/episode/${c.target_id}` : c.target_type !== "episode" && c.target_slug ? `/movies/${c.target_slug}` : null);

export default function AdminCommentsPage() {
  const { token, isLoading: authLoading, user } = useAuth();
  const router = useRouter();
  const toast = useToast();

  const [comments, setComments] = useState<AdminComment[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(0);
  const [searchInput, setSearchInput] = useState("");
  const search = useDebounced(searchInput.trim(), 350);
  const [status, setStatus] = useState<Filter>("all");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState<Set<string>>(new Set());
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [expanded, setExpanded] = useState<Set<string>>(new Set());

  useEffect(() => {
    if (!authLoading && (!token || !isStaffRole(user?.role))) router.push("/");
  }, [authLoading, token, user, router]);

  useEffect(() => setPage(1), [search, status]);

  const load = useCallback(async () => {
    if (!token) return;
    setLoading(true);
    try {
      const data = await getAdminComments(token, { page, limit: 20, search: search || undefined, status: status !== "all" ? status : undefined });
      setComments(data.data);
      setTotal(data.total);
      setTotalPages(data.total_pages);
      setSelected(new Set());
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Izohlarni yuklab bo'lmadi");
    } finally {
      setLoading(false);
    }
  }, [token, page, search, status, toast]);

  useEffect(() => {
    load();
  }, [load]);

  const run = async (ids: string[], action: "approved" | "rejected" | "delete") => {
    if (!token || ids.length === 0) return;
    if (action === "delete" && !confirm(ids.length > 1 ? `${ids.length} ta izohni o'chirasizmi?` : "Izohni o'chirasizmi?")) return;
    setBusy((b) => new Set([...Array.from(b), ...ids]));
    let ok = 0;
    for (const id of ids) {
      try {
        if (action === "delete") await adminDeleteComment(token, id);
        else await updateCommentStatus(token, id, action);
        ok++;
      } catch {
        /* counted below */
      }
    }
    const verb = action === "delete" ? "o'chirildi" : action === "approved" ? "tasdiqlandi" : "rad etildi";
    if (ok === ids.length) toast.success(`${ok} ta izoh ${verb}`);
    else toast.error(`${ok}/${ids.length} ta izoh ${verb}, qolganida xato`);
    setBusy(new Set());
    await load();
  };

  const toggle = (set: Set<string>, id: string) => {
    const next = new Set(set);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    return next;
  };

  if (authLoading || !token || !isStaffRole(user?.role)) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="animate-spin text-gray-500" />
      </div>
    );
  }

  const allSelected = comments.length > 0 && comments.every((c) => selected.has(c.id));

  return (
    <div className="mx-auto max-w-6xl p-4 sm:p-6">
      <PageHead
        icon={MessageSquare}
        gradient="from-sky-500 to-indigo-600"
        title="Izohlar"
        subtitle="Kino va seriallardagi izohlarni moderatsiya qilish"
        actions={
          user?.role !== "moderator" ? (
            <Link
              href="/admin/comments/settings"
              className="inline-flex items-center gap-2 rounded-xl border border-white/10 bg-white/[0.03] px-3.5 py-2.5 text-sm text-gray-300 transition hover:bg-white/[0.07] hover:text-white"
            >
              <Settings size={15} /> Sozlamalar
            </Link>
          ) : undefined
        }
      />

      <ReportedCommentsPanel token={token} />

      <div className="mb-4 flex flex-col gap-3 lg:flex-row">
        <SearchBox value={searchInput} onChange={setSearchInput} placeholder="Izoh matni bo'yicha qidirish..." />
        <Tabs<Filter>
          value={status}
          onChange={setStatus}
          items={[
            { key: "all", label: "Hammasi" },
            { key: "pending", label: "Kutilmoqda" },
            { key: "approved", label: "Tasdiqlangan" },
            { key: "rejected", label: "Rad etilgan" },
          ]}
        />
      </div>

      {comments.length > 0 && !loading && (
        <div className="mb-3 flex flex-wrap items-center gap-2 rounded-xl border border-white/10 bg-[#12121a] px-3 py-2">
          <label className="inline-flex cursor-pointer items-center gap-2 text-xs text-gray-400">
            <input
              type="checkbox"
              checked={allSelected}
              onChange={() => setSelected(allSelected ? new Set() : new Set(comments.map((c) => c.id)))}
              className="h-4 w-4 accent-orange-500"
            />
            {selected.size > 0 ? `${selected.size} ta tanlandi` : "Hammasini tanlash"}
          </label>
          {selected.size > 0 && (
            <div className="ml-auto flex gap-1.5">
              <button onClick={() => run(Array.from(selected), "approved")} className="inline-flex items-center gap-1 rounded-lg bg-emerald-500/15 px-2.5 py-1.5 text-xs text-emerald-300 hover:bg-emerald-500/25">
                <Check size={13} /> Tasdiqlash
              </button>
              <button onClick={() => run(Array.from(selected), "rejected")} className="inline-flex items-center gap-1 rounded-lg bg-amber-500/15 px-2.5 py-1.5 text-xs text-amber-300 hover:bg-amber-500/25">
                <X size={13} /> Rad etish
              </button>
              <button onClick={() => run(Array.from(selected), "delete")} className="inline-flex items-center gap-1 rounded-lg bg-red-500/15 px-2.5 py-1.5 text-xs text-red-300 hover:bg-red-500/25">
                <Trash2 size={13} /> O&apos;chirish
              </button>
            </div>
          )}
        </div>
      )}

      {loading ? (
        <SkeletonList rows={6} height={96} />
      ) : comments.length === 0 ? (
        <EmptyState icon={MessageSquare} title="Izoh topilmadi" text={search || status !== "all" ? "Filtr yoki qidiruvni o'zgartirib ko'ring." : "Hali izoh qoldirilmagan."} />
      ) : (
        <ul className="space-y-2.5">
          {comments.map((c) => {
            const st = STATUS[c.status] ?? { label: c.status, tone: "gray" as Tone };
            const href = targetHref(c);
            const isBusy = busy.has(c.id);
            const long = c.content.length > 220;
            const open = expanded.has(c.id);
            const authorId = c.user?.id || c.user_id;
            const avatar = c.user?.avatar_url || c.user_avatar_url;
            return (
              <li
                key={c.id}
                className={`rounded-2xl border bg-[#12121a] p-4 transition ${selected.has(c.id) ? "border-orange-500/40" : "border-white/10 hover:border-white/20"} ${isBusy ? "opacity-60" : ""}`}
              >
                <div className="flex gap-3">
                  <input type="checkbox" checked={selected.has(c.id)} onChange={() => setSelected((s) => toggle(s, c.id))} className="mt-2.5 h-4 w-4 shrink-0 accent-orange-500" aria-label="Tanlash" />
                  <Avatar name={authorOf(c)} src={avatar ? normalizeMediaUrl(avatar) : undefined} size={36} />
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                      {authorId ? (
                        <Link href={`/user/${authorId}`} className="font-medium text-white hover:text-orange-300">
                          {authorOf(c)}
                        </Link>
                      ) : (
                        <span className="font-medium text-gray-400">{authorOf(c)}</span>
                      )}
                      {c.user?.username && <span className="text-xs text-gray-500">@{c.user.username}</span>}
                      <span className="text-xs text-gray-600" title={fmtDate(c.created_at)}>
                        · {timeAgo(c.created_at)}
                      </span>
                      <Chip tone={st.tone} dot>
                        {st.label}
                      </Chip>
                      {c.has_blocked_word && (
                        <Chip tone="red" icon={ShieldAlert}>
                          Taqiqlangan so&apos;z
                        </Chip>
                      )}
                      {c.has_link && (
                        <Chip tone="yellow" icon={Link2}>
                          Havola
                        </Chip>
                      )}
                      {c.is_spoiler && <Chip tone="violet">Spoyler</Chip>}
                      {(c.reports_count ?? 0) > 0 && (
                        <Chip tone="red" icon={AlertTriangle}>
                          {c.reports_count} shikoyat
                        </Chip>
                      )}
                    </div>
                    {c.parent_id && (
                      <p className="mt-1 inline-flex items-center gap-1 text-[11px] text-gray-500">
                        <CornerDownRight size={11} /> javob
                      </p>
                    )}
                    <p className={`mt-1.5 whitespace-pre-line break-words text-sm leading-relaxed text-gray-200 ${long && !open ? "line-clamp-3" : ""}`}>{c.content}</p>
                    {long && (
                      <button onClick={() => setExpanded((s) => toggle(s, c.id))} className="mt-0.5 text-xs text-orange-400 hover:text-orange-300">
                        {open ? "Yig'ish" : "To'liq o'qish"}
                      </button>
                    )}
                    <div className="mt-2 flex flex-wrap items-center gap-3 text-xs text-gray-500">
                      {href ? (
                        <Link href={href} target="_blank" className="inline-flex max-w-[260px] items-center gap-1 truncate hover:text-white">
                          <Film size={12} /> {c.target_title || c.movie_title || "Kontent"}
                        </Link>
                      ) : (
                        <span className="inline-flex items-center gap-1">
                          <Film size={12} /> {c.target_title || c.movie_title || "—"}
                        </span>
                      )}
                      {(c.likes_count ?? 0) > 0 && (
                        <span className="inline-flex items-center gap-1">
                          <Heart size={11} /> {c.likes_count}
                        </span>
                      )}
                    </div>
                  </div>
                  <div className="flex shrink-0 flex-col gap-0.5 sm:flex-row sm:items-start">
                    {c.status !== "approved" && (
                      <IconBtn label="Tasdiqlash" tone="green" disabled={isBusy} onClick={() => run([c.id], "approved")}>
                        <Check size={16} />
                      </IconBtn>
                    )}
                    {c.status !== "rejected" && (
                      <IconBtn label="Rad etish" tone="yellow" disabled={isBusy} onClick={() => run([c.id], "rejected")}>
                        <X size={16} />
                      </IconBtn>
                    )}
                    <IconBtn label="O'chirish" tone="red" disabled={isBusy} onClick={() => run([c.id], "delete")}>
                      <Trash2 size={16} />
                    </IconBtn>
                  </div>
                </div>
              </li>
            );
          })}
        </ul>
      )}

      <Pager page={page} totalPages={totalPages} total={total} unit="ta izoh" onChange={setPage} />
    </div>
  );
}
