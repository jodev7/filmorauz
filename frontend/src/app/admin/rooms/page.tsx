"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth-context";
import {
  adminListWatchRooms,
  adminGetRoomStats,
  adminCreatePremiereRoom,
  searchMovies,
  AdminRoomSnapshot,
  AdminRoomStats,
  Movie,
} from "@/lib/api";
import { getSeries, Series } from "@/lib/series-api";
import { normalizeMediaUrl } from "@/lib/image-utils";
import {
  Activity,
  Calendar,
  Crown,
  ExternalLink,
  Globe2,
  Layers,
  Loader2,
  Lock,
  Pause,
  Play,
  Plus,
  RefreshCw,
  Search,
  Sparkles,
  Trophy,
  Tv,
  Users,
  Video,
  XCircle,
} from "lucide-react";
import { Avatar, Chip, EmptyState, GhostButton, Modal, PageHead, PrimaryButton, SkeletonList, StatTile, Thumb, timeAgo } from "@/components/admin/kit";
import { Field, Segmented, inputCls } from "@/components/admin/form/ui";

const fmtPos = (s: number) => {
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = Math.floor(s % 60);
  return h ? `${h}:${String(m).padStart(2, "0")}:${String(sec).padStart(2, "0")}` : `${m}:${String(sec).padStart(2, "0")}`;
};

function Poster({ src, alt, className }: { src?: string; alt: string; className: string }) {
  return <Thumb src={src ? normalizeMediaUrl(src) : undefined} alt={alt} className={className} icon={Video} />;
}

export default function AdminRoomsPage() {
  const { token } = useAuth();
  const [rooms, setRooms] = useState<AdminRoomSnapshot[]>([]);
  const [stats, setStats] = useState<AdminRoomStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState("");
  const [premiereOpen, setPremiereOpen] = useState(false);
  const [expanded, setExpanded] = useState<string | null>(null);

  const load = useCallback(
    async (initial = false) => {
      if (!token) return;
      if (initial) setLoading(true);
      else setRefreshing(true);
      setError("");
      try {
        const [res, st] = await Promise.all([adminListWatchRooms(token), adminGetRoomStats(token).catch(() => null)]);
        if (st) setStats(st);
        setRooms((res.items || []).map((it) => ({ ...it, members: it.members || [] })));
      } catch (e) {
        setError(e instanceof Error ? e.message : "Yuklashda xato");
      } finally {
        setLoading(false);
        setRefreshing(false);
      }
    },
    [token]
  );

  useEffect(() => {
    load(true);
    const t = setInterval(() => load(false), 10_000);
    return () => clearInterval(t);
  }, [load]);

  const watching = rooms.reduce((n, r) => n + r.members.length, 0);

  return (
    <div className="mx-auto max-w-7xl p-4 sm:p-6">
      <PageHead
        icon={Users}
        gradient="from-violet-500 to-fuchsia-600"
        title="Birga ko'rish xonalari"
        subtitle={
          <span className="inline-flex items-center gap-1.5">
            <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-emerald-400" /> Jonli — har 10 soniyada yangilanadi
          </span>
        }
        actions={
          <>
            <GhostButton onClick={() => load(false)}>
              <RefreshCw size={15} className={refreshing ? "animate-spin" : ""} /> Yangilash
            </GhostButton>
            <PrimaryButton onClick={() => setPremiereOpen(true)}>
              <Sparkles size={16} /> Premyera xonasi
            </PrimaryButton>
          </>
        }
      />

      <div className="mb-6 grid grid-cols-2 gap-3 lg:grid-cols-5">
        <StatTile icon={Activity} tone="green" label="Hozir ochiq" value={stats?.active ?? rooms.length} />
        <StatTile icon={Users} tone="violet" label="Hozir tomosha qilmoqda" value={watching} />
        <StatTile icon={Calendar} tone="yellow" label="Bu oy" value={stats?.this_month ?? "—"} />
        <StatTile icon={Layers} tone="blue" label="Jami xonalar" value={stats?.total ?? "—"} />
        <StatTile icon={XCircle} tone="gray" label="Yopilgan" value={stats?.closed ?? "—"} />
      </div>

      {error && <div className="mb-4 rounded-xl border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-300">{error}</div>}

      <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_320px]">
        <section>
          <h2 className="mb-3 text-sm font-semibold text-white">Ochiq xonalar</h2>
          {loading ? (
            <SkeletonList rows={3} height={120} />
          ) : rooms.length === 0 ? (
            <EmptyState icon={Users} title="Hozir ochiq xona yo'q" text="Foydalanuvchilar xona ochganda shu yerda jonli ko'rinadi." />
          ) : (
            <ul className="space-y-3">
              {rooms.map(({ room, members }) => {
                const open = expanded === room.id;
                const shown = open ? members : members.slice(0, 6);
                return (
                  <li key={room.id} className="overflow-hidden rounded-2xl border border-white/10 bg-[#12121a]">
                    <div className="flex gap-4 p-4">
                      <Poster src={room.content_poster} alt={room.content_title || ""} className="h-24 w-16 shrink-0 rounded-lg" />
                      <div className="min-w-0 flex-1">
                        <div className="flex flex-wrap items-center gap-1.5">
                          {room.is_playing ? (
                            <Chip tone="green" icon={Play}>
                              {fmtPos(room.position_seconds)}
                            </Chip>
                          ) : (
                            <Chip tone="yellow" icon={Pause}>
                              {fmtPos(room.position_seconds)}
                            </Chip>
                          )}
                          <Chip icon={room.content_type === "movie" ? Video : Tv}>{room.content_type === "movie" ? "Kino" : "Serial"}</Chip>
                          <Chip icon={room.visibility === "private" ? Lock : Globe2}>{room.visibility === "private" ? "Maxfiy" : "Ochiq"}</Chip>
                          {room.kind === "premiere" && (
                            <Chip tone="orange" icon={Sparkles}>
                              Premyera
                            </Chip>
                          )}
                        </div>
                        <h3 className="mt-1.5 truncate font-semibold text-white" title={room.content_title}>
                          {room.content_title || "—"}
                          {room.current_episode_title && <span className="font-normal text-gray-400"> · {room.current_episode_title}</span>}
                        </h3>
                        <p className="mt-1 flex flex-wrap items-center gap-x-2 text-xs text-gray-500">
                          <Link href={`/user/${room.owner_id}`} className="inline-flex items-center gap-1 text-amber-300 hover:underline">
                            <Crown size={11} /> {room.owner_name || "Host"}
                          </Link>
                          {room.owner_is_premium && <span className="text-yellow-400">★ Premium</span>}
                          <span>· {timeAgo(room.created_at)} ochilgan</span>
                          <span>· max {room.max_members}</span>
                        </p>
                      </div>
                      <Link
                        href={`/watch-room/${room.id}`}
                        target="_blank"
                        className="self-start rounded-lg p-2 text-gray-400 hover:bg-white/5 hover:text-white"
                        title="Xonani ochish"
                        aria-label="Xonani ochish"
                      >
                        <ExternalLink size={15} />
                      </Link>
                    </div>
                    <div className="flex flex-wrap items-center gap-2 border-t border-white/5 bg-black/20 px-4 py-2.5">
                      <span className="mr-1 inline-flex items-center gap-1 text-xs text-gray-400">
                        <Users size={12} /> {members.length}
                      </span>
                      {members.length === 0 ? (
                        <span className="text-xs text-gray-600">Hech kim ulanmagan</span>
                      ) : (
                        shown.map((m) => (
                          <Link
                            key={m.user_id}
                            href={`/user/${m.user_id}`}
                            className="inline-flex items-center gap-1.5 rounded-full bg-white/5 py-0.5 pl-0.5 pr-2 text-xs text-gray-200 hover:bg-white/10"
                          >
                            <Avatar name={m.user_name || "Mehmon"} src={m.user_avatar ? normalizeMediaUrl(m.user_avatar) : undefined} size={20} />
                            <span className="max-w-[110px] truncate">{m.user_name || "Mehmon"}</span>
                            {m.is_host && <Crown size={10} className="text-yellow-400" />}
                          </Link>
                        ))
                      )}
                      {members.length > 6 && (
                        <button onClick={() => setExpanded(open ? null : room.id)} className="text-xs text-orange-400 hover:text-orange-300">
                          {open ? "Yig'ish" : `+${members.length - 6} ta`}
                        </button>
                      )}
                    </div>
                  </li>
                );
              })}
            </ul>
          )}
        </section>

        <aside className="space-y-4">
          <TopList
            title="Eng ko'p xona ochilgan"
            icon={Trophy}
            empty={!stats?.top_content?.length}
            items={(stats?.top_content || []).map((c) => ({
              key: c.content_id,
              left: <Poster src={c.content_poster} alt={c.content_title} className="h-10 w-7 rounded" />,
              title: c.content_title || "—",
              sub: c.content_type === "movie" ? "Kino" : "Serial",
              count: c.room_count,
            }))}
          />
          <TopList
            title="Eng faol hostlar"
            icon={Crown}
            empty={!stats?.top_hosts?.length}
            items={(stats?.top_hosts || []).map((h) => ({
              key: h.owner_id,
              left: <Avatar name={h.owner_name || "?"} src={h.owner_avatar ? normalizeMediaUrl(h.owner_avatar) : undefined} size={28} />,
              title: h.owner_name || "Foydalanuvchi",
              href: `/user/${h.owner_id}`,
              count: h.room_count,
            }))}
          />
        </aside>
      </div>

      {premiereOpen && <PremiereModal token={token} onClose={() => setPremiereOpen(false)} onCreated={() => load(false)} />}
    </div>
  );
}

function TopList({
  title,
  icon: Icon,
  items,
  empty,
}: {
  title: string;
  icon: typeof Trophy;
  empty: boolean;
  items: { key: string; left: React.ReactNode; title: string; sub?: string; href?: string; count: number }[];
}) {
  const max = Math.max(1, ...items.map((i) => i.count));
  return (
    <section className="rounded-2xl border border-white/10 bg-[#12121a] p-4">
      <h3 className="mb-3 flex items-center gap-2 text-sm font-semibold text-white">
        <Icon size={15} className="text-amber-400" /> {title}
      </h3>
      {empty ? (
        <p className="py-4 text-center text-xs text-gray-500">Hozircha ma&apos;lumot yo&apos;q</p>
      ) : (
        <ol className="space-y-2.5">
          {items.map((it, i) => (
            <li key={it.key} className="flex items-center gap-2.5">
              <span className="w-4 text-xs tabular-nums text-gray-500">{i + 1}</span>
              {it.left}
              <div className="min-w-0 flex-1">
                {it.href ? (
                  <Link href={it.href} className="block truncate text-sm text-gray-200 hover:text-white">
                    {it.title}
                  </Link>
                ) : (
                  <p className="truncate text-sm text-gray-200">{it.title}</p>
                )}
                <div className="mt-1 h-1 overflow-hidden rounded-full bg-white/5">
                  <div className="h-full rounded-full bg-gradient-to-r from-violet-500 to-fuchsia-500" style={{ width: `${(it.count / max) * 100}%` }} />
                </div>
              </div>
              <span className="text-xs font-semibold tabular-nums text-white">{it.count}</span>
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}

type Picked = { type: "movie" | "series"; id: string; title: string; poster?: string };

function PremiereModal({ token, onClose, onCreated }: { token: string | null; onClose: () => void; onCreated: () => void }) {
  const router = useRouter();
  const [contentType, setContentType] = useState<"movie" | "series">("movie");
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Picked[]>([]);
  const [searching, setSearching] = useState(false);
  const [picked, setPicked] = useState<Picked | null>(null);
  const [maxMembers, setMaxMembers] = useState(5000);
  const [pinPriority, setPinPriority] = useState(0);
  const [scheduledAt, setScheduledAt] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    const q = query.trim();
    if (q.length < 2) {
      setResults([]);
      return;
    }
    let cancelled = false;
    setSearching(true);
    const t = setTimeout(async () => {
      try {
        if (contentType === "movie") {
          const movies: Movie[] = await searchMovies(q);
          if (!cancelled) setResults(movies.slice(0, 8).map((m) => ({ type: "movie", id: m.id, title: m.title, poster: m.poster_url })));
        } else {
          const res = await getSeries(1, 50);
          const list = (res.data || []).filter((s: Series) => s.title.toLowerCase().includes(q.toLowerCase()));
          if (!cancelled) setResults(list.slice(0, 8).map((s) => ({ type: "series", id: s.id, title: s.title, poster: s.poster_url })));
        }
      } catch {
        if (!cancelled) setResults([]);
      } finally {
        if (!cancelled) setSearching(false);
      }
    }, 300);
    return () => {
      cancelled = true;
      clearTimeout(t);
    };
  }, [query, contentType]);

  const submit = async () => {
    if (!token || !picked) return;
    setSubmitting(true);
    setError("");
    try {
      const room = await adminCreatePremiereRoom(token, {
        content_type: picked.type,
        content_id: picked.id,
        max_members: maxMembers,
        pin_priority: pinPriority,
        scheduled_start_at: scheduledAt ? new Date(scheduledAt).toISOString() : undefined,
      });
      onCreated();
      router.push(`/watch-room/${room.id}`);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Yaratishda xato");
      setSubmitting(false);
    }
  };

  return (
    <Modal
      open
      onClose={onClose}
      busy={submitting}
      icon={Sparkles}
      iconTone="yellow"
      title="Premyera xonasi"
      subtitle="Katta auditoriya uchun rasmiy birga ko'rish xonasi"
      footer={
        <>
          <p className="mr-auto truncate text-xs text-red-400">{error}</p>
          <button type="button" onClick={onClose} className="rounded-xl px-4 py-2 text-sm text-gray-300 hover:bg-white/5">
            Bekor qilish
          </button>
          <button
            type="button"
            onClick={submit}
            disabled={!picked || submitting}
            className="inline-flex items-center gap-2 rounded-xl bg-orange-500 px-5 py-2 text-sm font-semibold text-white hover:bg-orange-400 disabled:opacity-50"
          >
            {submitting ? <Loader2 size={15} className="animate-spin" /> : <Plus size={15} />} Premyerani ochish
          </button>
        </>
      }
    >
      <div className="space-y-4">
        <Segmented<"movie" | "series">
          ariaLabel="Kontent turi"
          value={contentType}
          onChange={(v) => {
            setContentType(v);
            setResults([]);
            setPicked(null);
          }}
          options={[
            { value: "movie", label: "Kino" },
            { value: "series", label: "Serial" },
          ]}
        />
        {picked ? (
          <div className="flex items-center gap-3 rounded-xl border border-orange-500/30 bg-orange-500/[0.06] p-2.5">
            <Poster src={picked.poster} alt={picked.title} className="h-16 w-11 shrink-0 rounded-md" />
            <span className="min-w-0 flex-1 truncate font-medium text-white">{picked.title}</span>
            <button type="button" onClick={() => setPicked(null)} className="rounded-lg px-2.5 py-1 text-xs text-gray-300 hover:bg-white/5">
              O&apos;zgartirish
            </button>
          </div>
        ) : (
          <div>
            <div className="relative">
              <Search size={15} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" />
              <input autoFocus value={query} onChange={(e) => setQuery(e.target.value)} placeholder={contentType === "movie" ? "Kino nomi..." : "Serial nomi..."} className={`${inputCls} pl-9`} />
              {searching && <Loader2 size={15} className="absolute right-3 top-1/2 -translate-y-1/2 animate-spin text-gray-500" />}
            </div>
            {results.length > 0 && (
              <ul className="mt-2 max-h-64 overflow-y-auto rounded-xl border border-white/10 bg-black/30 p-1">
                {results.map((r) => (
                  <li key={r.id}>
                    <button
                      type="button"
                      onClick={() => {
                        setPicked(r);
                        setQuery("");
                        setResults([]);
                      }}
                      className="flex w-full items-center gap-2.5 rounded-lg p-1.5 text-left hover:bg-white/5"
                    >
                      <Poster src={r.poster} alt={r.title} className="h-11 w-8 shrink-0 rounded" />
                      <span className="truncate text-sm text-white">{r.title}</span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>
        )}
        <div className="grid gap-3 sm:grid-cols-3">
          <Field label="Maksimal a'zo">
            <input type="number" min={2} max={10000} value={maxMembers} onChange={(e) => setMaxMembers(Number(e.target.value))} className={inputCls} />
          </Field>
          <Field label="Pin ustuvorligi">
            <input type="number" value={pinPriority} onChange={(e) => setPinPriority(Number(e.target.value))} className={inputCls} />
          </Field>
          <Field label="Boshlanish (ixtiyoriy)">
            <input type="datetime-local" value={scheduledAt} onChange={(e) => setScheduledAt(e.target.value)} className={`${inputCls} [color-scheme:dark]`} />
          </Field>
        </div>
      </div>
    </Modal>
  );
}
