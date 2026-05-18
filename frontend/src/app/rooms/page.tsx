"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { listPublicRooms, WatchRoom } from "@/lib/api";
import MediaImage from "@/components/ui/MediaImage";
import { Loader2, Users, Crown } from "lucide-react";

export default function PublicRoomsPage() {
  const [rooms, setRooms] = useState<WatchRoom[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      try {
        const res = await listPublicRooms();
        if (!cancelled) setRooms(res.items || []);
      } catch (e) {
        if (!cancelled) setError(e instanceof Error ? e.message : "Failed to load");
      } finally {
        if (!cancelled) setLoading(false);
      }
    };
    load();
    const t = setInterval(load, 10_000);
    return () => {
      cancelled = true;
      clearInterval(t);
    };
  }, []);

  return (
    <div className="min-h-screen bg-brand-dark text-white">
      <div className="max-w-7xl mx-auto px-4 py-8">
        <h1 className="text-3xl font-display mb-2">Public watch rooms</h1>
        <p className="text-gray-400 mb-6">
          Join a room and watch movies / serials together in sync with chat.
        </p>
        {loading && (
          <div className="flex justify-center py-12">
            <Loader2 className="w-8 h-8 animate-spin text-brand-red" />
          </div>
        )}
        {error && <p className="text-red-400">{error}</p>}
        {!loading && rooms.length === 0 && (
          <p className="text-gray-500">No public rooms right now. Start one from a movie page.</p>
        )}
        <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
          {rooms.map((r) => (
            <Link
              key={r.id}
              href={`/watch-room/${r.id}`}
              className="bg-brand-card border border-brand-border rounded-xl overflow-hidden hover:border-brand-red transition-colors"
            >
              <div className="aspect-[2/3] bg-brand-dark">
                {r.content_poster && (
                  <MediaImage src={r.content_poster} alt={r.content_title || ""} className="w-full h-full object-cover" />
                )}
              </div>
              <div className="p-3">
                <h3 className="font-semibold truncate" title={r.content_title}>
                  {r.content_title || "Untitled"}
                </h3>
                <p className="text-xs text-gray-400 mt-1 flex items-center gap-1">
                  <Crown className="w-3 h-3 text-yellow-400" /> {r.owner_name || "Host"}
                  {r.owner_is_premium && (
                    <span className="ml-1 text-[10px] bg-yellow-500/20 text-yellow-300 px-1 rounded">PRO</span>
                  )}
                </p>
                <p className="text-xs text-gray-500 mt-1 flex items-center gap-1">
                  <Users className="w-3 h-3" /> Max {r.max_members} • {r.is_playing ? "Playing" : "Paused"}
                </p>
              </div>
            </Link>
          ))}
        </div>
      </div>
    </div>
  );
}
