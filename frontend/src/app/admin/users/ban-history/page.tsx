"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Ban, CalendarClock, Clock, ExternalLink, History, Loader2, ShieldAlert, Unlock, UserX } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { isStaffRole } from "@/lib/roles";
import { getBanHistory, unbanUser, BanHistoryRecord } from "@/lib/api";
import { useToast } from "@/components/admin/Toast";
import { Avatar, Chip, EmptyState, IconBtn, PageHead, SearchBox, SkeletonList, StatTile, Tabs, Tone, fmtDate, timeAgo, useDebounced } from "@/components/admin/kit";

type Filter = "all" | "active" | "unbanned" | "expired";

const STATUS: Record<string, { label: string; tone: Tone; dot: string }> = {
  active: { label: "Faol", tone: "red", dot: "bg-red-500" },
  unbanned: { label: "Bekor qilingan", tone: "green", dot: "bg-emerald-500" },
  expired: { label: "Tugagan", tone: "gray", dot: "bg-gray-500" },
};

const nameOf = (h: BanHistoryRecord) => h.user_display_name || (h.user_username ? `@${h.user_username}` : h.user_telegram_id ? `ID ${h.user_telegram_id}` : "Noma'lum");

function duration(h: BanHistoryRecord): string {
  if (h.is_permanent) return "Doimiy";
  if (!h.banned_until) return "—";
  const days = Math.round((new Date(h.banned_until).getTime() - new Date(h.banned_at).getTime()) / 86400e3);
  return days >= 1 ? `${days} kun` : `${Math.max(1, Math.round(days * 24))} soat`;
}

export default function AdminBanHistoryPage() {
  const { token, isLoading: authLoading, user } = useAuth();
  const router = useRouter();
  const toast = useToast();

  const [history, setHistory] = useState<BanHistoryRecord[]>([]);
  const [searchInput, setSearchInput] = useState("");
  const search = useDebounced(searchInput.trim(), 350);
  const [filter, setFilter] = useState<Filter>("all");
  const [loading, setLoading] = useState(true);
  const [busyId, setBusyId] = useState<string | null>(null);

  useEffect(() => {
    if (!authLoading && (!token || !isStaffRole(user?.role))) router.push("/");
  }, [authLoading, token, user, router]);

  const load = useCallback(async () => {
    if (!token) return;
    setLoading(true);
    try {
      setHistory((await getBanHistory(token, { search: search || undefined, status: "all" })).data);
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
    const c: Record<Filter, number> = { all: history.length, active: 0, unbanned: 0, expired: 0 };
    history.forEach((h) => c[h.status]++);
    return c;
  }, [history]);
  const visible = filter === "all" ? history : history.filter((h) => h.status === filter);

  const doUnban = async (h: BanHistoryRecord) => {
    if (!token || !confirm(`${nameOf(h)} bandan chiqarilsinmi?`)) return;
    setBusyId(h.id);
    try {
      await unbanUser(token, h.user_id);
      toast.success(`${nameOf(h)} bandan chiqarildi`);
      load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Bandan chiqarib bo'lmadi");
    } finally {
      setBusyId(null);
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
    <div className="mx-auto max-w-5xl p-4 sm:p-6">
      <PageHead
        icon={History}
        gradient="from-slate-500 to-slate-700"
        title="Ban tarixi"
        subtitle="Barcha ban va bandan chiqarish amallari"
        actions={
          <Link
            href="/admin/users/banned"
            className="inline-flex items-center gap-2 rounded-xl border border-white/10 bg-white/[0.03] px-3.5 py-2.5 text-sm text-gray-300 transition hover:bg-white/[0.07] hover:text-white"
          >
            <UserX size={15} /> Ban olganlar
          </Link>
        }
      />

      <div className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatTile icon={Ban} tone="gray" label="Jami yozuvlar" value={counts.all} active={filter === "all"} onClick={() => setFilter("all")} />
        <StatTile icon={Clock} tone="red" label="Faol" value={counts.active} active={filter === "active"} onClick={() => setFilter("active")} />
        <StatTile icon={Unlock} tone="green" label="Bekor qilingan" value={counts.unbanned} active={filter === "unbanned"} onClick={() => setFilter("unbanned")} />
        <StatTile icon={CalendarClock} tone="gray" label="Muddati tugagan" value={counts.expired} active={filter === "expired"} onClick={() => setFilter("expired")} />
      </div>

      <div className="mb-4 flex flex-col gap-3 lg:flex-row">
        <SearchBox value={searchInput} onChange={setSearchInput} placeholder="Foydalanuvchi, sabab yoki admin..." />
        <Tabs<Filter>
          value={filter}
          onChange={setFilter}
          items={[
            { key: "all", label: "Hammasi", count: counts.all },
            { key: "active", label: "Faol", count: counts.active },
            { key: "unbanned", label: "Bekor qilingan", count: counts.unbanned },
            { key: "expired", label: "Tugagan", count: counts.expired },
          ]}
        />
      </div>

      {loading ? (
        <SkeletonList rows={5} height={100} />
      ) : visible.length === 0 ? (
        <EmptyState icon={History} title="Tarix bo'sh" text={search ? "Qidiruvni o'zgartirib ko'ring." : undefined} />
      ) : (
        <ol className="relative space-y-3 border-l border-white/10 pl-5">
          {visible.map((h) => {
            const st = STATUS[h.status] ?? STATUS.expired;
            return (
              <li key={h.id} className="relative">
                <span className={`absolute -left-[27px] top-5 h-3 w-3 rounded-full ring-4 ring-[#0b0b12] ${st.dot}`} />
                <div className="rounded-2xl border border-white/10 bg-[#12121a] p-4 transition hover:border-white/20">
                  <div className="flex flex-wrap items-start gap-3">
                    <Avatar name={nameOf(h)} size={36} />
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <Link href={`/user/${h.user_id}`} className="font-medium text-white hover:text-orange-300">
                          {nameOf(h)}
                        </Link>
                        <Chip tone={st.tone} dot>
                          {st.label}
                        </Chip>
                        <Chip tone={h.is_permanent ? "red" : "gray"}>{duration(h)}</Chip>
                      </div>
                      <p className="mt-1 flex items-center gap-1.5 text-sm text-gray-300">
                        <ShieldAlert size={13} className="shrink-0 text-red-400" />
                        <span className="truncate">{h.reason || "Sabab ko'rsatilmagan"}</span>
                      </p>
                    </div>
                    <div className="flex items-center gap-1">
                      <Link href={`/user/${h.user_id}`} className="rounded-lg p-2 text-gray-400 hover:bg-white/5 hover:text-white" title="Profil" aria-label="Profil">
                        <ExternalLink size={15} />
                      </Link>
                      {h.status === "active" && (
                        <IconBtn label="Bandan chiqarish" tone="green" disabled={busyId === h.id} onClick={() => doUnban(h)}>
                          {busyId === h.id ? <Loader2 size={16} className="animate-spin" /> : <Unlock size={16} />}
                        </IconBtn>
                      )}
                    </div>
                  </div>
                  <div className="mt-3 grid gap-2 text-xs sm:grid-cols-2">
                    <p className="rounded-lg bg-red-500/[0.05] px-3 py-2 text-gray-400">
                      <span className="text-red-300">Ban:</span> {h.banned_by_username || "Noma'lum"} · <span title={fmtDate(h.banned_at)}>{timeAgo(h.banned_at)}</span>
                      {!h.is_permanent && h.banned_until && <span className="text-gray-500"> · {fmtDate(h.banned_until, false)} gacha</span>}
                    </p>
                    {h.unbanned_at || h.unbanned_by_username ? (
                      <p className="rounded-lg bg-emerald-500/[0.05] px-3 py-2 text-gray-400">
                        <span className="text-emerald-300">Chiqarildi:</span> {h.unbanned_by_username || "avtomatik"}
                        {h.unbanned_at && <span title={fmtDate(h.unbanned_at)}> · {timeAgo(h.unbanned_at)}</span>}
                      </p>
                    ) : (
                      <p className="rounded-lg bg-white/[0.02] px-3 py-2 text-gray-600">{h.status === "expired" ? "Muddati o'zi tugagan" : "Hali bekor qilinmagan"}</p>
                    )}
                  </div>
                </div>
              </li>
            );
          })}
        </ol>
      )}
    </div>
  );
}
