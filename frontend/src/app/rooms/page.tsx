"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import Navbar from "@/components/Navbar";
import MediaImage from "@/components/ui/MediaImage";
import { listPublicRooms, PublicRoomListItem } from "@/lib/api";
import { Users, Loader2, Crown, RefreshCw, Globe2, Play } from "lucide-react";

export default function PublicRoomsPage() {
  const [rooms, setRooms] = useState<PublicRoomListItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [err, setErr] = useState("");
  const router = useRouter();

  const load = async (initial = false) => {
    if (initial) setLoading(true);
    else setRefreshing(true);
    try {
      const res = await listPublicRooms();
      setRooms(res.items || []);
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
    <div className="min-h-screen bg-brand-dark text-white">
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
            className="px-3 py-2 bg-brand-card border border-brand-border rounded-lg flex items-center gap-2 text-sm hover:border-brand-red"
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

        {!loading && rooms.length === 0 && (
          <div className="bg-brand-card border border-brand-border rounded-xl p-12 text-center">
            <Users className="w-12 h-12 text-gray-600 mx-auto mb-3" />
            <p className="text-gray-400">Hozircha ochiq room yo&apos;q.</p>
            <p className="text-xs text-gray-500 mt-2">
              Birga ko&apos;rmoqchi bo&apos;lsangiz, kino sahifasidan &quot;Birga ko&apos;rish&quot; tugmasi orqali yangi room oching.
            </p>
          </div>
        )}

        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
          {rooms.map((r) => {
            const full = r.member_count >= r.max_members;
            return (
              <div
                key={r.id}
                className="bg-brand-card border border-brand-border rounded-xl p-3 flex gap-3"
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
