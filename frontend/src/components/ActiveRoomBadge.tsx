"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { Users } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { getMyActiveRoom, WatchRoom } from "@/lib/api";

// Pill that appears in the navbar while the user has an open hosted room.
// Without this a host who clicks Back/Home had no way back into their own
// session — the room would keep running but be invisible until it timed
// out. Polls every 30s so it disappears within a minute of the room
// closing (host disconnect grace is 5min server-side).
export default function ActiveRoomBadge() {
  const { token, isAuthenticated } = useAuth();
  const pathname = usePathname();
  const [room, setRoom] = useState<WatchRoom | null>(null);

  useEffect(() => {
    if (!isAuthenticated || !token) {
      setRoom(null);
      return;
    }
    let cancelled = false;
    const refresh = async () => {
      try {
        const r = await getMyActiveRoom(token);
        if (!cancelled) setRoom(r);
      } catch {
        /* ignore */
      }
    };
    refresh();
    const t = setInterval(refresh, 30_000);
    return () => {
      cancelled = true;
      clearInterval(t);
    };
  }, [token, isAuthenticated, pathname]);

  if (!room) return null;
  // Hide while inside the room itself — the pill is a "return to room"
  // affordance, redundant when you're already there.
  if (pathname?.startsWith(`/watch-room/${room.id}`)) return null;

  return (
    <Link
      href={`/watch-room/${room.id}`}
      className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg bg-brand-red/15 border border-brand-red/40 text-xs text-brand-red hover:bg-brand-red/25 transition-colors"
      title={`Aktiv room: ${room.content_title || ""}`}
    >
      <Users className="w-4 h-4" />
      <span className="hidden sm:inline truncate max-w-[120px]">Roomga qaytish</span>
      <span className="sm:hidden">Room</span>
    </Link>
  );
}
