"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Crown, UserX } from "lucide-react";
import MediaImage from "@/components/ui/MediaImage";
import { listRoomMembers } from "@/lib/api";
import type { RoomMember } from "@/lib/use-room-socket";

// Above this many members we stop relying on the WS roster (which large
// rooms don't push anyway) and switch to a REST-backed virtualized list.
const VIRTUALIZE_THRESHOLD = 60;
const ROW_H = 36; // px per row — keep in sync with the row markup height
const VIEWPORT_H = 240; // px visible window
const OVERSCAN = 6; // rows rendered above/below the window
const PAGE = 50; // roster page size
const REFRESH_MS = 8000; // refetch visible pages to reflect joins/leaves

type Props = {
  roomID: string;
  totalCount: number;
  wsMembers: RoomMember[];
  isHost: boolean;
  onKick: (userID: string) => void;
};

function MemberRow({
  m,
  isHost,
  onKick,
  style,
}: {
  m: RoomMember | undefined;
  isHost: boolean;
  onKick: (userID: string) => void;
  style?: React.CSSProperties;
}) {
  if (!m) {
    return (
      <li style={style} className="flex items-center gap-2 text-sm px-0.5">
        <div className="w-6 h-6 rounded-full bg-brand-dark/60 animate-pulse" />
        <div className="h-3 flex-1 rounded bg-brand-dark/60 animate-pulse" />
      </li>
    );
  }
  return (
    <li style={style} className="flex items-center gap-2 text-sm px-0.5">
      {m.userAvatar ? (
        <MediaImage src={m.userAvatar} alt={m.userName} className="w-6 h-6 rounded-full object-cover" />
      ) : (
        <div className="w-6 h-6 rounded-full bg-brand-dark flex items-center justify-center text-xs">
          {(m.userName || "?").slice(0, 1)}
        </div>
      )}
      <span className="truncate flex-1">{m.userName || "Foydalanuvchi"}</span>
      {m.isHost && <Crown className="w-3 h-3 text-yellow-400 shrink-0" />}
      {isHost && !m.isHost && (
        <button
          onClick={() => onKick(m.userID)}
          className="p-1 text-red-400 hover:bg-red-500/10 rounded"
          title="Chiqarish"
        >
          <UserX className="w-3.5 h-3.5" />
        </button>
      )}
    </li>
  );
}

// Small-room path: render the WS roster directly, no virtualization.
function PlainMemberList({ wsMembers, isHost, onKick }: Omit<Props, "roomID" | "totalCount">) {
  return (
    <ul className="mt-2 space-y-1 max-h-40 overflow-y-auto">
      {wsMembers.length === 0 && (
        <li className="text-xs text-gray-500">Hozircha sizdan boshqa hech kim yo&apos;q</li>
      )}
      {wsMembers.map((m) => (
        <MemberRow key={m.userID} m={m} isHost={isHost} onKick={onKick} />
      ))}
    </ul>
  );
}

// Large-room path: a windowed list rendering only the rows in (and near) the
// visible block — everything else stays hidden until the user scrolls there.
// Pages are fetched lazily over REST as they scroll into view.
function VirtualMemberList({ roomID, totalCount, isHost, onKick }: Omit<Props, "wsMembers">) {
  const [pages, setPages] = useState<Map<number, RoomMember[]>>(new Map());
  const [scrollTop, setScrollTop] = useState(0);
  const inflightRef = useRef<Set<number>>(new Set());

  const start = Math.max(0, Math.floor(scrollTop / ROW_H) - OVERSCAN);
  const end = Math.min(totalCount, Math.ceil((scrollTop + VIEWPORT_H) / ROW_H) + OVERSCAN);

  const fetchPage = useCallback(
    async (page: number, force = false) => {
      if (!force && inflightRef.current.has(page)) return;
      inflightRef.current.add(page);
      try {
        const res = await listRoomMembers(roomID, page * PAGE, PAGE);
        const mapped: RoomMember[] = (res.items || []).map((it) => ({
          userID: it.user_id,
          userName: it.user_name || "",
          userAvatar: it.user_avatar,
          isHost: it.is_host,
        }));
        setPages((prev) => {
          const next = new Map(prev);
          next.set(page, mapped);
          return next;
        });
      } catch {
        /* leave as placeholders; a later scroll/refresh retries */
      } finally {
        inflightRef.current.delete(page);
      }
    },
    [roomID],
  );

  // Load whatever pages the visible window touches.
  useEffect(() => {
    const firstPage = Math.floor(start / PAGE);
    const lastPage = Math.floor((Math.max(start, end - 1)) / PAGE);
    for (let p = firstPage; p <= lastPage; p++) {
      if (!pages.has(p)) fetchPage(p);
    }
  }, [start, end, pages, fetchPage]);

  // Periodically refetch the currently-visible pages so the roster reflects
  // joins/leaves without a full reload.
  useEffect(() => {
    const t = setInterval(() => {
      const firstPage = Math.floor(start / PAGE);
      const lastPage = Math.floor((Math.max(start, end - 1)) / PAGE);
      for (let p = firstPage; p <= lastPage; p++) fetchPage(p, true);
    }, REFRESH_MS);
    return () => clearInterval(t);
  }, [start, end, fetchPage]);

  const memberAt = (i: number): RoomMember | undefined => {
    const page = pages.get(Math.floor(i / PAGE));
    return page?.[i % PAGE];
  };

  const rows = [];
  for (let i = start; i < end; i++) {
    rows.push(
      <MemberRow
        key={i}
        m={memberAt(i)}
        isHost={isHost}
        onKick={onKick}
        style={{ position: "absolute", top: i * ROW_H, left: 0, right: 0, height: ROW_H }}
      />,
    );
  }

  return (
    <div
      className="mt-2 overflow-y-auto relative"
      style={{ height: VIEWPORT_H }}
      onScroll={(e) => setScrollTop((e.target as HTMLDivElement).scrollTop)}
    >
      <div style={{ height: totalCount * ROW_H, position: "relative" }}>{rows}</div>
    </div>
  );
}

export default function MemberList({ roomID, totalCount, wsMembers, isHost, onKick }: Props) {
  // Use the windowed REST list once the room is large enough that the WS
  // roster is no longer complete (presenceMode kicks in around the same size).
  if (totalCount > VIRTUALIZE_THRESHOLD) {
    return <VirtualMemberList roomID={roomID} totalCount={totalCount} isHost={isHost} onKick={onKick} />;
  }
  return <PlainMemberList wsMembers={wsMembers} isHost={isHost} onKick={onKick} />;
}
