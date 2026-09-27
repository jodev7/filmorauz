"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Ban, CalendarClock, Clock, ExternalLink, History, Infinity as InfinityIcon, Loader2, ShieldAlert, Unlock, UserX } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { isStaffRole } from "@/lib/roles";
import { getBannedUsers, unbanUser, BannedUser } from "@/lib/api";
import { useToast } from "@/components/admin/Toast";
import { Avatar, Chip, EmptyState, IconBtn, Modal, PageHead, SearchBox, SkeletonList, StatTile, Tabs, Tone, fmtDate, timeAgo, useDebounced } from "@/components/admin/kit";

type Filter = "all" | "active" | "permanent" | "expired";

const STATUS: Record<string, { label: string; tone: Tone }> = {
  active: { label: "Muddatli", tone: "yellow" },
  permanent: { label: "Doimiy", tone: "red" },
  expired: { label: "Tugagan", tone: "gray" },
};

const nameOf = (u: BannedUser) => u.display_name || (u.username ? `@${u.username}` : u.telegram_id ? `ID ${u.telegram_id}` : "Noma'lum");

/** "3 kun qoldi" + how much of the ban has passed. */
function banProgress(u: BannedUser): { text: string; pct: number } | null {
  if (u.ban_status !== "active" || !u.banned_until) return null;
  const start = new Date(u.banned_at).getTime();
  const end = new Date(u.banned_until).getTime();
  const now = Date.now();
  const left = end - now;
  const hours = Math.ceil(left / 3600e3);
  const text = hours <= 0 ? "tugamoqda" : hours < 48 ? `${hours} soat qoldi` : `${Math.ceil(hours / 24)} kun qoldi`;
  return { text, pct: Math.min(100, Math.max(0, ((now - start) / Math.max(1, end - start)) * 100)) };
}

export default function AdminBannedUsersPage() {
  const { token, isLoading: authLoading, user } = useAuth();
  const router = useRouter();
  const toast = useToast();

  const [list, setList] = useState<BannedUser[]>([]);
  const [searchInput, setSearchInput] = useState("");
  const search = useDebounced(searchInput.trim(), 350);
  const [filter, setFilter] = useState<Filter>("all");
  const [loading, setLoading] = useState(true);
  const [confirm, setConfirm] = useState<BannedUser | null>(null);
  const [unbanning, setUnbanning] = useState(false);

  useEffect(() => {
    if (!authLoading && (!token || !isStaffRole(user?.role))) router.push("/");
  }, [authLoading, token, user, router]);

  // The endpoint isn't paginated, so load everything once and filter here —
  // the counters then stay right whichever tab is open.
  const load = useCallback(async () => {
    if (!token) return;
    setLoading(true);
    try {
      setList((await getBannedUsers(token, { search: search || undefined, status: "all" })).data);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Yuklab bo'lmadi");
    } finally {
      setLoading(false);
    }
  }, [token, search, toast]);

  useEffect(() => {
    load();
  }, [load]);

  const counts = useMemo(() => {
    const c: Record<Filter, number> = { all: list.length, active: 0, permanent: 0, expired: 0 };
    list.forEach((u) => c[u.ban_status]++);
    return c;
  }, [list]);
  const visible = filter === "all" ? list : list.filter((u) => u.ban_status === filter);

  const doUnban = async () => {
    if (!token || !confirm) return;
    setUnbanning(true);
    try {
      await unbanUser(token, confirm.id);
      toast.success(`${nameOf(confirm)} bandan chiqarildi`);
      setConfirm(null);
      load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Bandan chiqarib bo'lmadi");
    } finally {
      setUnbanning(false);
    }
  };

  if (authLoading || !token || !isStaffRole(user?.role)) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="animate-spin text-gray-500" />
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-6xl p-4 sm:p-6">
      <PageHead
        icon={UserX}
        gradient="from-red-500 to-rose-700"
        title="Ban olganlar"
        subtitle="Hozir bloklangan va bani tugagan foydalanuvchilar"
        actions={
          <Link
            href="/admin/users/ban-history"
            className="inline-flex items-center gap-2 rounded-xl border border-white/10 bg-white/[0.03] px-3.5 py-2.5 text-sm text-gray-300 transition hover:bg-white/[0.07] hover:text-white"
          >
            <History size={15} /> Ban tarixi
          </Link>
        }
      />

      <div className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatTile icon={Ban} tone="red" label="Jami" value={counts.all} active={filter === "all"} onClick={() => setFilter("all")} />
        <StatTile icon={Clock} tone="yellow" label="Muddatli ban" value={counts.active} active={filter === "active"} onClick={() => setFilter("active")} />
        <StatTile icon={InfinityIcon} tone="orange" label="Doimiy ban" value={counts.permanent} active={filter === "permanent"} onClick={() => setFilter("permanent")} />
        <StatTile icon={CalendarClock} tone="gray" label="Muddati tugagan" value={counts.expired} active={filter === "expired"} onClick={() => setFilter("expired")} />
      </div>

      <div className="mb-4 flex flex-col gap-3 lg:flex-row">
        <SearchBox value={searchInput} onChange={setSearchInput} placeholder="Username, Telegram ID yoki sabab..." />
        <Tabs<Filter>
          value={filter}
          onChange={setFilter}
          items={[
            { key: "all", label: "Hammasi", count: counts.all },
            { key: "active", label: "Muddatli", count: counts.active },
            { key: "permanent", label: "Doimiy", count: counts.permanent },
            { key: "expired", label: "Tugagan", count: counts.expired },
          ]}
        />
      </div>

      {loading ? (
        <SkeletonList rows={5} height={84} />
      ) : visible.length === 0 ? (
        <EmptyState icon={UserX} title="Ban olgan foydalanuvchi yo'q" text={search ? "Qidiruvni o'zgartirib ko'ring." : undefined} />
      ) : (
        <ul className="space-y-2.5">
          {visible.map((u) => {
            const st = STATUS[u.ban_status] ?? STATUS.active;
            const prog = banProgress(u);
            const canUnban = u.ban_status === "active" || u.ban_status === "permanent";
            return (
              <li key={u.id} className="flex flex-wrap items-center gap-4 rounded-2xl border border-white/10 bg-[#12121a] p-4 transition hover:border-white/20">
                <div className="flex min-w-[220px] flex-1 items-center gap-3">
                  <Avatar name={nameOf(u)} size={40} />
                  <div className="min-w-0">
                    <Link href={`/user/${u.id}`} className="block truncate font-medium text-white hover:text-orange-300">
                      {nameOf(u)}
                    </Link>
                    <p className="truncate text-xs text-gray-500">
                      {u.username && `@${u.username} · `}
                      <span className="font-mono">{u.telegram_id || "—"}</span>
                    </p>
                  </div>
                </div>

                <div className="min-w-[200px] flex-1">
                  <p className="flex items-center gap-1.5 text-sm text-gray-200">
                    <ShieldAlert size={14} className="shrink-0 text-red-400" />
                    <span className="truncate" title={u.ban_reason}>
                      {u.ban_reason || "Sabab ko'rsatilmagan"}
                    </span>
                  </p>
                  <p className="mt-0.5 text-[11px] text-gray-500">
                    {u.banned_by_username || "Noma'lum"} · <span title={fmtDate(u.banned_at)}>{timeAgo(u.banned_at)}</span>
                  </p>
                </div>

                <div className="w-full sm:w-44">
                  <Chip tone={st.tone} dot>
                    {st.label}
                  </Chip>
                  {prog ? (
                    <>
                      <div className="mt-1.5 h-1 overflow-hidden rounded-full bg-white/5">
                        <div className="h-full rounded-full bg-amber-400" style={{ width: `${prog.pct}%` }} />
                      </div>
                      <p className="mt-1 text-[11px] text-gray-500">
                        {prog.text} · {fmtDate(u.banned_until, false)}
                      </p>
                    </>
                  ) : u.ban_status === "expired" && u.banned_until ? (
                    <p className="mt-1 text-[11px] text-gray-500">{fmtDate(u.banned_until, false)} da tugagan</p>
                  ) : null}
                </div>

                <div className="ml-auto flex items-center gap-1">
                  <Link href={`/user/${u.id}`} className="rounded-lg p-2 text-gray-400 hover:bg-white/5 hover:text-white" title="Profil" aria-label="Profil">
                    <ExternalLink size={15} />
                  </Link>
                  {canUnban && (
                    <IconBtn label="Bandan chiqarish" tone="green" onClick={() => setConfirm(u)}>
                      <Unlock size={16} />
                    </IconBtn>
                  )}
                </div>
              </li>
            );
          })}
        </ul>
      )}

      <Modal
        open={!!confirm}
        onClose={() => setConfirm(null)}
        busy={unbanning}
        size="sm"
        icon={Unlock}
        iconTone="green"
        title="Bandan chiqarish"
        subtitle={confirm ? nameOf(confirm) : undefined}
        footer={
          <>
            <button type="button" onClick={() => setConfirm(null)} className="rounded-xl px-4 py-2 text-sm text-gray-300 hover:bg-white/5">
              Bekor qilish
            </button>
            <button type="button" onClick={doUnban} disabled={unbanning} className="inline-flex items-center gap-2 rounded-xl bg-emerald-600 px-5 py-2 text-sm font-semibold text-white hover:bg-emerald-500 disabled:opacity-50">
              {unbanning && <Loader2 size={15} className="animate-spin" />} Bandan chiqarish
            </button>
          </>
        }
      >
        {confirm && (
          <p className="text-sm text-gray-300">
            Foydalanuvchi darhol saytdan to&apos;liq foydalana oladi.
            {confirm.ban_reason && <span className="mt-2 block text-xs text-gray-500">Ban sababi: {confirm.ban_reason}</span>}
          </p>
        )}
      </Modal>
    </div>
  );
}
