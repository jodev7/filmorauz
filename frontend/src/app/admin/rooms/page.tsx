"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useAuth } from "@/lib/auth-context";
import { adminListWatchRooms, AdminRoomSnapshot } from "@/lib/api";
import MediaImage from "@/components/ui/MediaImage";
import { Loader2, Users, Crown, RefreshCw, Video, Tv } from "lucide-react";

export default function AdminRoomsPage() {
  const { token } = useAuth();
  const [rooms, setRooms] = useState<AdminRoomSnapshot[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState("");

  const load = async (isInitial = false) => {
    if (!token) return;
    if (isInitial) setLoading(true);
    else setRefreshing(true);
    setError("");
    try {
      const res = await adminListWatchRooms(token);
      // Guard against null members on a room not currently held by the hub.
      const items = (res.items || []).map((it) => ({
        ...it,
        members: it.members || [],
      }));
      setRooms(items);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Yuklashda xato");
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  };

  useEffect(() => {
    load(true);
    const t = setInterval(() => load(false), 10_000); // auto-refresh
    return () => clearInterval(t);
  }, [token]); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="min-h-screen bg-brand-dark text-white">
      <div className="max-w-7xl mx-auto px-4 py-8">
        <div className="flex items-center justify-between mb-6">
          <div>
            <h1 className="text-3xl font-display">Watch Rooms</h1>
            <p className="text-gray-400 text-sm mt-1">
              Hozir faol bo&apos;lgan birgalikda ko&apos;rish room&apos;lari va a&apos;zolari
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

        {error && <p className="text-red-400 mb-4">{error}</p>}

        {!loading && rooms.length === 0 && (
          <div className="bg-brand-card border border-brand-border rounded-xl p-12 text-center">
            <Users className="w-12 h-12 text-gray-600 mx-auto mb-3" />
            <p className="text-gray-400">Hozir faol room yo&apos;q</p>
          </div>
        )}

        <div className="space-y-3">
          {rooms.map((r) => (
            <div
              key={r.room.id}
              className="bg-brand-card border border-brand-border rounded-xl p-4 flex flex-col lg:flex-row gap-4"
            >
              {/* Poster + content */}
              <div className="flex gap-3 flex-1 min-w-0">
                {r.room.content_poster && (
                  <div className="w-16 h-24 sm:w-20 sm:h-28 shrink-0 rounded-md overflow-hidden">
                    <MediaImage
                      src={r.room.content_poster}
                      alt={r.room.content_title || ""}
                      className="w-full h-full object-cover"
                    />
                  </div>
                )}
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 flex-wrap">
                    {r.room.content_type === "episode" ? (
                      <Tv className="w-4 h-4 text-blue-400" />
                    ) : (
                      <Video className="w-4 h-4 text-brand-red" />
                    )}
                    <h3 className="font-semibold truncate" title={r.room.content_title}>
                      {r.room.content_title || "—"}
                    </h3>
                    <span className="text-[10px] px-1.5 py-0.5 bg-brand-dark rounded text-gray-400">
                      {r.room.content_type === "episode" ? "Serial" : "Movie"}
                    </span>
                    <span
                      className={`text-[10px] px-1.5 py-0.5 rounded ${
                        r.room.is_playing ? "bg-green-500/20 text-green-300" : "bg-yellow-500/20 text-yellow-300"
                      }`}
                    >
                      {r.room.is_playing ? "▶ O'ynamoqda" : "⏸ Pauza"}
                    </span>
                  </div>
                  <p className="text-xs text-gray-500 mt-1">
                    Room ID: <span className="font-mono">{r.room.id}</span>
                  </p>
                  <p className="text-xs text-gray-500">
                    Pozitsiya: {Math.floor(r.room.position_seconds / 60)}:
                    {String(Math.floor(r.room.position_seconds % 60)).padStart(2, "0")} • Max {r.room.max_members} • {r.room.visibility === "private" ? "Maxfiy" : "Ochiq"}
                  </p>
                  <p className="text-xs text-gray-500 mt-1">
                    Host:{" "}
                    <Link
                      href={`/user/${r.room.owner_id}`}
                      className="text-yellow-300 hover:underline inline-flex items-center gap-1"
                    >
                      <Crown className="w-3 h-3" />
                      {r.room.owner_name || r.room.owner_id}
                    </Link>
                    {r.room.owner_is_premium && (
                      <span className="ml-2 text-[10px] bg-yellow-500/20 text-yellow-300 px-1 rounded">
                        PREMIUM
                      </span>
                    )}
                  </p>
                </div>
              </div>

              {/* Members */}
              <div className="lg:w-80 shrink-0">
                <p className="text-xs text-gray-400 mb-2 flex items-center gap-1">
                  <Users className="w-3 h-3" /> A&apos;zolar ({r.members.length})
                </p>
                {r.members.length === 0 ? (
                  <p className="text-xs text-gray-600">Hech kim ulanmagan</p>
                ) : (
                  <ul className="space-y-1">
                    {r.members.map((m) => (
                      <li key={m.user_id} className="flex items-center gap-2 text-sm">
                        {m.user_avatar ? (
                          <MediaImage
                            src={m.user_avatar}
                            alt={m.user_name}
                            className="w-6 h-6 rounded-full object-cover"
                          />
                        ) : (
                          <div className="w-6 h-6 rounded-full bg-brand-dark flex items-center justify-center text-xs">
                            {(m.user_name || "?").slice(0, 1)}
                          </div>
                        )}
                        <Link
                          href={`/user/${m.user_id}`}
                          className="truncate flex-1 hover:text-brand-red hover:underline"
                          title={m.user_id}
                        >
                          {m.user_name || "Mehmon"}
                        </Link>
                        {m.is_host && <Crown className="w-3 h-3 text-yellow-400 shrink-0" />}
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
