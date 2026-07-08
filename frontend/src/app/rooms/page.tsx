"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import Navbar from "@/components/Navbar";
import MediaImage from "@/components/ui/MediaImage";
import { listPublicRooms, listFeaturedRooms, PublicRoomListItem } from "@/lib/api";
import { Users, Loader2, Crown, RefreshCw, Globe2, Play, Sparkles, Clock } from "lucide-react";

// Countdown until a premiere's scheduled start. Returns "" once it's live.
function useCountdown(target?: string): string {
  const [label, setLabel] = useState("");
  useEffect(() => {
    if (!target) {
      setLabel("");
      return;
    }
    const tick = () => {
      const diff = new Date(target).getTime() - Date.now();
      if (diff <= 0) {
        setLabel("");
        return;
      }
      const d = Math.floor(diff / 86400000);
      const h = Math.floor((diff % 86400000) / 3600000);
      const m = Math.floor((diff % 3600000) / 60000);
      const s = Math.floor((diff % 60000) / 1000);
      setLabel(d > 0 ? `${d}k ${h}s ${m}d` : h > 0 ? `${h}s ${m}d ${s}son` : `${m}d ${s}son`);
    };
    tick();
    const t = setInterval(tick, 1000);
    return () => clearInterval(t);
  }, [target]);
  return label;
}

export default function PublicRoomsPage() {
  const [rooms, setRooms] = useState<PublicRoomListItem[]>([]);
  const [featured, setFeatured] = useState<PublicRoomListItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [err, setErr] = useState("");
  const router = useRouter();

  const load = async (initial = false) => {
    if (initial) setLoading(true);
    else setRefreshing(true);
    try {
      const [pub, feat] = await Promise.all([
        listPublicRooms(),
        listFeaturedRooms().catch(() => ({ items: [] as PublicRoomListItem[] })),
      ]);
      setRooms(pub.items || []);
      setFeatured(feat.items || []);
      setErr("");
    } catch (e) {
      setErr(e instanceof Error ? e.message : "Yuklashda xato");
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  };

  useEffect(() => {
    load(true);
    const t = setInterval(() => load(false), 15_000);
    return () => clearInterval(t);
  }, []);

  return (
    <div className="min-h-screen text-white">
      <Navbar />
      <div className="max-w-7xl mx-auto px-4 pt-20 sm:pt-24 pb-8">
        <div className="flex items-center justify-between mb-6">
          <div>
            <h1 className="text-2xl sm:text-3xl font-display flex items-center gap-2">
              <Globe2 className="w-6 h-6 text-brand-red" /> Ochiq roomlar
            </h1>
            <p className="text-gray-400 text-sm mt-1">
              Birga ko&apos;rishga ochiq xonalar. Istalganiga qo&apos;shilishingiz mumkin.
            </p>
          </div>
          <button
            onClick={() => load(false)}
            className="px-3 py-2 glass-card border border-white/10 rounded-lg flex items-center gap-2 text-sm hover:border-brand-red"
          >
            <RefreshCw className={`w-4 h-4 ${refreshing ? "animate-spin" : ""}`} /> Yangilash
          </button>
        </div>

        {loading && (
          <div className="flex justify-center py-12">
            <Loader2 className="w-8 h-8 animate-spin text-brand-red" />
          </div>
        )}
        {err && <p className="text-red-400 mb-4">{err}</p>}

        {!loading && featured.length > 0 && (
          <div className="mb-8">
            <h2 className="text-lg sm:text-xl font-display flex items-center gap-2 mb-3">
              <Sparkles className="w-5 h-5 text-yellow-400" /> Premyeralar
            </h2>
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
              {featured.map((r) => (
                <PremiereCard key={r.id} room={r} onJoin={() => router.push(`/watch-room/${r.id}`)} />
              ))}
            </div>
          </div>
        )}

        {!loading && rooms.length === 0 && (
          <div className="glass-card border border-white/10 rounded-xl p-12 text-center">
            <Users className="w-12 h-12 text-gray-600 mx-auto mb-3" />
            <p className="text-gray-400">Hozircha ochiq room yo&apos;q.</p>
            <p className="text-xs text-gray-500 mt-2">
              Birga ko&apos;rmoqchi bo&apos;lsangiz, kino sahifasidan &quot;Birga ko&apos;rish&quot; tugmasi orqali yangi room oching.
            </p>
          </div>
        )}

        {!loading && featured.length > 0 && rooms.length > 0 && (
          <h2 className="text-lg sm:text-xl font-display flex items-center gap-2 mb-3">
            <Globe2 className="w-5 h-5 text-brand-red" /> Ochiq roomlar
          </h2>
        )}

        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
          {rooms.map((r) => {
            const full = r.member_count >= r.max_members;
            return (
              <div
                key={r.id}
                className="glass-card border border-white/10 rounded-xl p-3 flex gap-3"
              >
                {r.content_poster && (
                  <div className="w-16 h-24 shrink-0 rounded-md overflow-hidden">
                    <MediaImage
                      src={r.content_poster}
                      alt={r.content_title || ""}
                      className="w-full h-full object-cover"
                    />
                  </div>
                )}
                <div className="flex-1 min-w-0 flex flex-col">
                  <h3 className="font-semibold truncate" title={r.content_title}>
                    {r.content_title || "—"}
                  </h3>
                  {r.current_episode_title && (
                    <p className="text-[11px] text-gray-400 truncate">
                      Ep: {r.current_episode_title}
                    </p>
                  )}
                  <p className="text-xs text-gray-500 mt-1 flex items-center gap-1 truncate">
                    <Crown className="w-3 h-3 text-yellow-400 shrink-0" />
                    <span className="truncate">{r.owner_name || "Foydalanuvchi"}</span>
                    {r.owner_is_premium && (
                      <span className="text-[9px] bg-yellow-500/20 text-yellow-300 px-1 rounded">PRO</span>
                    )}
                  </p>
                  <p className="text-xs text-gray-500 mt-0.5 flex items-center gap-2">
                    <span className="flex items-center gap-1">
                      <Users className="w-3 h-3" /> {r.member_count}/{r.max_members}
                    </span>
                    <span
                      className={`text-[10px] px-1.5 rounded ${
                        r.is_playing ? "bg-green-500/20 text-green-300" : "bg-yellow-500/20 text-yellow-300"
                      }`}
                    >
                      {r.is_playing ? "▶" : "⏸"}
                    </span>
                  </p>
                  <div className="flex-1" />
                  <button
                    disabled={full}
                    onClick={() => router.push(`/watch-room/${r.id}`)}
                    className={`mt-2 px-3 py-1.5 rounded-lg text-xs font-medium flex items-center justify-center gap-1.5 ${
                      full
                        ? "bg-brand-dark text-gray-500 cursor-not-allowed"
                        : "bg-brand-red hover:bg-red-700 text-white"
                    }`}
                  >
                    <Play className="w-3.5 h-3.5" />
                    {full ? "To'la" : "Qo'shilish"}
                  </button>
                </div>
              </div>
            );
          })}
        </div>

        <div className="mt-8 text-center">
          <Link
            href="/movies"
            className="text-xs text-gray-400 hover:text-white"
          >
            Yangi room ochish uchun kino sahifasiga o&apos;ting
          </Link>
        </div>
      </div>
    </div>
  );
}

function PremiereCard({ room: r, onJoin }: { room: PublicRoomListItem; onJoin: () => void }) {
  const countdown = useCountdown(r.scheduled_start_at);
  const isUpcoming = countdown !== "";
  const full = r.member_count >= r.max_members;
  return (
    <div className="relative overflow-hidden rounded-2xl border border-yellow-500/30 glass-card p-4 flex gap-4">
      <span className="absolute top-3 right-3 text-[10px] font-semibold px-2 py-0.5 rounded-full bg-yellow-500/20 text-yellow-300 flex items-center gap-1">
        <Sparkles className="w-3 h-3" /> PREMYERA
      </span>
      {r.content_poster && (
        <div className="w-24 h-36 shrink-0 rounded-lg overflow-hidden ring-1 ring-yellow-500/20">
          <MediaImage src={r.content_poster} alt={r.content_title || ""} className="w-full h-full object-cover" />
        </div>
      )}
      <div className="flex-1 min-w-0 flex flex-col">
        <h3 className="text-lg font-semibold truncate pr-20" title={r.content_title}>
          {r.content_title || "—"}
        </h3>
        {r.current_episode_title && (
          <p className="text-xs text-gray-400 truncate">Ep: {r.current_episode_title}</p>
        )}
        <p className="text-xs text-gray-500 mt-1 flex items-center gap-1 truncate">
          <Crown className="w-3 h-3 text-yellow-400 shrink-0" />
          <span className="truncate">{r.owner_name || "Admin"}</span>
        </p>

        {isUpcoming ? (
          <div className="mt-2 flex items-center gap-1.5 text-sm text-yellow-300">
            <Clock className="w-4 h-4" />
            <span className="tabular-nums font-medium">{countdown}</span>
            <span className="text-gray-400">qoldi</span>
          </div>
        ) : (
          <div className="mt-2">
            <span className="text-[11px] px-2 py-0.5 rounded-full bg-green-500/20 text-green-300 font-medium">
              ● JONLI
            </span>
          </div>
        )}

        <div className="flex-1" />
        <div className="flex items-center justify-between mt-3">
          <span className="text-xs text-gray-400 flex items-center gap-1">
            <Users className="w-3.5 h-3.5" /> {r.member_count}/{r.max_members}
          </span>
          <button
            disabled={full}
            onClick={onJoin}
            className={`px-4 py-1.5 rounded-lg text-sm font-medium flex items-center gap-1.5 ${
              full
                ? "bg-brand-dark text-gray-500 cursor-not-allowed"
                : "bg-brand-red hover:bg-red-700 text-white"
            }`}
          >
            <Play className="w-4 h-4" />
            {full ? "To'la" : isUpcoming ? "Kirish" : "Tomosha qilish"}
          </button>
        </div>
      </div>
    </div>
  );
}
