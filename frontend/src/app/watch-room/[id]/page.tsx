"use client";

import { useEffect, useRef, useState, useCallback } from "react";
import { useParams, useSearchParams, useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth-context";
import {
  getWatchRoom,
  listRoomMessages,
  createRoomInvite,
  WatchRoom,
  WatchRoomMessage,
} from "@/lib/api";
import {
  useRoomSocket,
  effectivePosition,
  RoomMember,
  RoomChatEntry,
  RoomReaction,
} from "@/lib/use-room-socket";
import MediaImage from "@/components/ui/MediaImage";
import {
  Loader2,
  Send,
  Smile,
  Copy,
  Users,
  Crown,
  X,
  UserX,
  PartyPopper,
} from "lucide-react";

const EMOJI_PALETTE = ["😀", "😂", "❤️", "🔥", "👏", "🎉", "😮", "😢", "👍", "🤔", "😍", "🍿"];

type FloatingReaction = RoomReaction & { id: string };

export default function WatchRoomPage() {
  const params = useParams<{ id: string }>();
  const searchParams = useSearchParams();
  const router = useRouter();
  const { user, token, isLoading: authLoading } = useAuth();

  const roomID = params?.id || "";
  const inviteCode = searchParams?.get("invite") || undefined;

  const [room, setRoom] = useState<WatchRoom | null>(null);
  const [loadError, setLoadError] = useState<string>("");
  const [videoSrc, setVideoSrc] = useState<string>("");
  const [members, setMembers] = useState<Record<string, RoomMember>>({});
  const [chat, setChat] = useState<RoomChatEntry[]>([]);
  const [closedReason, setClosedReason] = useState<string>("");
  const [kicked, setKicked] = useState(false);
  const [chatInput, setChatInput] = useState("");
  const [emojiOpen, setEmojiOpen] = useState(false);
  const [showMembers, setShowMembers] = useState(false); // mobile drawer
  const [typingUsers, setTypingUsers] = useState<Record<string, string>>({}); // userID → name
  const [floatingReactions, setFloatingReactions] = useState<FloatingReaction[]>([]);

  const videoRef = useRef<HTMLVideoElement | null>(null);
  const chatScrollRef = useRef<HTMLDivElement | null>(null);
  const typingTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const isHost = !!user && !!room && user.id === room.owner_id;

  // ── Load room metadata + chat history ─────────────────────────────────
  useEffect(() => {
    if (!roomID) return;
    let cancelled = false;
    (async () => {
      try {
        const r = await getWatchRoom(roomID);
        if (cancelled) return;
        setRoom(r);
        const apiBase = process.env.NEXT_PUBLIC_API_URL || "";
        if (r.content_type === "movie") {
          const m = await fetch(`${apiBase}/movies/${r.content_id}`).then((res) => res.json());
          if (!cancelled) setVideoSrc(m.master_playlist_url || m.streaming_url || "");
        } else {
          const ep = await fetch(`${apiBase}/episodes/${r.content_id}`).then((res) => res.json());
          if (!cancelled) setVideoSrc(ep.video_url || ep.master_playlist_url || "");
        }
        const msgs = await listRoomMessages(roomID).catch(() => ({ items: [] as WatchRoomMessage[] }));
        if (!cancelled) {
          setChat(
            msgs.items.map((m) => ({
              userID: m.user_id,
              userName: m.user_name || "",
              userAvatar: m.user_avatar,
              kind: m.kind,
              text: m.text,
              emoji: m.emoji,
              createdAt: m.created_at,
            })),
          );
        }
      } catch (e) {
        if (!cancelled) setLoadError(e instanceof Error ? e.message : "Failed to load room");
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [roomID]);

  const {
    connected,
    state,
    events,
    sendHostAction,
    sendChat,
    sendTyping,
    sendReaction,
    sendKick,
  } = useRoomSocket(!authLoading && token && room ? roomID : null, token, inviteCode);

  // ── Drain WS events ──────────────────────────────────────────────────
  const lastEventIndexRef = useRef(0);
  useEffect(() => {
    if (events.length === lastEventIndexRef.current) return;
    const newOnes = events.slice(lastEventIndexRef.current);
    lastEventIndexRef.current = events.length;
    for (const e of newOnes) {
      if (e.type === "chat") {
        setChat((prev) => [...prev, e.chat]);
      } else if (e.type === "member_joined") {
        setMembers((prev) => ({ ...prev, [e.member.userID]: e.member }));
      } else if (e.type === "member_left") {
        setMembers((prev) => {
          const next = { ...prev };
          delete next[e.userID];
          return next;
        });
      } else if (e.type === "closed") {
        setClosedReason(e.reason);
      } else if (e.type === "kicked") {
        setKicked(true);
      } else if (e.type === "typing") {
        setTypingUsers((prev) => {
          const next = { ...prev };
          if (e.typing.isTyping && e.typing.userID !== user?.id) {
            next[e.typing.userID] = e.typing.userName;
          } else {
            delete next[e.typing.userID];
          }
          return next;
        });
      } else if (e.type === "reaction") {
        const id = `${e.reaction.ts}-${e.reaction.userID}-${Math.random()}`;
        setFloatingReactions((prev) => [...prev, { ...e.reaction, id }]);
        // Auto-remove after the float animation (~3s).
        setTimeout(() => {
          setFloatingReactions((prev) => prev.filter((r) => r.id !== id));
        }, 3000);
      }
    }
  }, [events, user?.id]);

  // ── Sync video element to host state (guests only) ───────────────────
  useEffect(() => {
    if (isHost) return;
    const v = videoRef.current;
    if (!v || !state) return;
    const target = effectivePosition(state);
    const drift = Math.abs(v.currentTime - target);
    if (drift > 1.5) v.currentTime = target;
    if (state.isPlaying && v.paused) v.play().catch(() => undefined);
    else if (!state.isPlaying && !v.paused) v.pause();
  }, [state, isHost]);

  // ── Host: broadcast player events ────────────────────────────────────
  const onHostPlay = useCallback(() => {
    if (!isHost) return;
    const v = videoRef.current;
    if (!v) return;
    sendHostAction("play", v.currentTime);
  }, [isHost, sendHostAction]);

  const onHostPause = useCallback(() => {
    if (!isHost) return;
    const v = videoRef.current;
    if (!v) return;
    sendHostAction("pause", v.currentTime);
  }, [isHost, sendHostAction]);

  const onHostSeeked = useCallback(() => {
    if (!isHost) return;
    const v = videoRef.current;
    if (!v) return;
    sendHostAction("seek", v.currentTime);
  }, [isHost, sendHostAction]);

  // Chat auto-scroll
  useEffect(() => {
    const el = chatScrollRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
  }, [chat.length]);

  // ── Chat input + typing ──────────────────────────────────────────────
  const onChatInputChange = (val: string) => {
    setChatInput(val);
    sendTyping(true);
    if (typingTimeoutRef.current) clearTimeout(typingTimeoutRef.current);
    typingTimeoutRef.current = setTimeout(() => sendTyping(false), 1500);
  };

  const handleSendChat = () => {
    const trimmed = chatInput.trim();
    if (!trimmed) return;
    sendChat("text", trimmed);
    setChatInput("");
    sendTyping(false);
    if (typingTimeoutRef.current) clearTimeout(typingTimeoutRef.current);
  };

  const handleSendEmoji = (emoji: string) => {
    sendReaction(emoji);
    setEmojiOpen(false);
  };

  const handleCopyInviteLink = useCallback(async () => {
    if (!token || !room) return;
    try {
      const inv = await createRoomInvite(token, room.id, {});
      const url = `${window.location.origin}/watch-room/${room.id}?invite=${inv.code}`;
      await navigator.clipboard.writeText(url);
      alert("Taklif havolasi nusxa olindi:\n\n" + url);
    } catch (e) {
      alert(e instanceof Error ? e.message : "Taklif yaratishda xatolik");
    }
  }, [token, room]);

  const handleKick = (userID: string) => {
    if (!isHost) return;
    if (!confirm("Bu foydalanuvchini room'dan chiqarishni tasdiqlaysizmi?")) return;
    sendKick(userID);
  };

  // ── Render guards ────────────────────────────────────────────────────
  if (authLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-brand-dark">
        <Loader2 className="w-8 h-8 animate-spin text-brand-red" />
      </div>
    );
  }
  if (!user || !token) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-brand-dark text-white">
        <p>Iltimos tizimga kiring</p>
      </div>
    );
  }
  if (loadError) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-brand-dark text-white">
        <p>{loadError}</p>
      </div>
    );
  }
  if (!room) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-brand-dark">
        <Loader2 className="w-8 h-8 animate-spin text-brand-red" />
      </div>
    );
  }
  if (kicked) {
    return (
      <div className="min-h-screen flex flex-col items-center justify-center bg-brand-dark text-white gap-4 px-4 text-center">
        <UserX className="w-12 h-12 text-red-400" />
        <h2 className="text-2xl font-bold">Sizni room'dan chiqarishdi</h2>
        <p className="text-gray-400">Host sizni chiqardi.</p>
        <button
          onClick={() => router.push("/")}
          className="px-4 py-2 bg-brand-red rounded-lg"
        >
          Bosh sahifaga
        </button>
      </div>
    );
  }
  if (closedReason) {
    return (
      <div className="min-h-screen flex flex-col items-center justify-center bg-brand-dark text-white gap-4 px-4 text-center">
        <h2 className="text-2xl font-bold">Room yopildi</h2>
        <p className="text-gray-400">Sabab: {closedReason}</p>
        <button
          onClick={() => router.push("/")}
          className="px-4 py-2 bg-brand-red rounded-lg"
        >
          Bosh sahifaga
        </button>
      </div>
    );
  }

  const memberList = Object.values(members);
  const typingNames = Object.values(typingUsers);

  return (
    <div className="min-h-screen bg-brand-dark text-white">
      <div className="max-w-7xl mx-auto p-2 sm:p-4 grid grid-cols-1 lg:grid-cols-[1fr_320px] gap-3 lg:gap-4">
        {/* ── Player + info ── */}
        <div className="space-y-3">
          <div className="bg-black rounded-xl overflow-hidden aspect-video relative">
            {videoSrc ? (
              <video
                ref={videoRef}
                src={videoSrc}
                controls={isHost}
                playsInline
                className="w-full h-full"
                onPlay={onHostPlay}
                onPause={onHostPause}
                onSeeked={onHostSeeked}
              />
            ) : (
              <div className="flex items-center justify-center h-full">
                <Loader2 className="w-8 h-8 animate-spin text-brand-red" />
              </div>
            )}

            {/* Floating reactions overlay */}
            <div className="pointer-events-none absolute inset-0 overflow-hidden">
              {floatingReactions.map((r, idx) => (
                <FloatingReactionBubble key={r.id} reaction={r} index={idx} />
              ))}
            </div>

            {!isHost && (
              <div className="absolute top-2 left-2 bg-black/60 text-[10px] sm:text-xs px-2 py-1 rounded">
                Mehmon — host boshqaradi
              </div>
            )}
            {!isHost && <GuestVolumeControl videoRef={videoRef} />}
            <div className="absolute top-2 right-2 flex items-center gap-2">
              <span
                className={`text-[10px] sm:text-xs px-2 py-1 rounded ${
                  connected ? "bg-green-500/80" : "bg-yellow-500/80"
                }`}
              >
                {connected ? "Online" : "Ulanish…"}
              </span>
            </div>
          </div>

          {/* Mobile member toggle */}
          <div className="lg:hidden flex gap-2">
            <button
              onClick={() => setShowMembers((v) => !v)}
              className="flex-1 px-3 py-2 bg-brand-card border border-brand-border rounded-lg text-sm flex items-center justify-center gap-2"
            >
              <Users className="w-4 h-4" />
              A'zolar ({memberList.length})
            </button>
            {isHost && (
              <button
                onClick={handleCopyInviteLink}
                className="flex-1 px-3 py-2 bg-brand-red rounded-lg text-sm flex items-center justify-center gap-2"
              >
                <Copy className="w-4 h-4" /> Taklif havolasi
              </button>
            )}
          </div>

          <div className="bg-brand-card border border-brand-border rounded-xl p-3 sm:p-4">
            <div className="flex items-start gap-3 sm:gap-4">
              {room.content_poster && (
                <div className="w-14 h-20 sm:w-20 sm:h-28 shrink-0 overflow-hidden rounded-md">
                  <MediaImage src={room.content_poster} alt={room.content_title || ""} className="w-full h-full object-cover" />
                </div>
              )}
              <div className="flex-1 min-w-0">
                <h1 className="text-base sm:text-xl font-semibold truncate">{room.content_title}</h1>
                <p className="text-xs sm:text-sm text-gray-400 mt-1 flex items-center gap-1 flex-wrap">
                  <Crown className="w-3 h-3 text-yellow-400" /> {room.owner_name}
                  {room.owner_is_premium && (
                    <span className="text-[10px] bg-yellow-500/20 text-yellow-300 px-1.5 rounded">PREMIUM</span>
                  )}
                </p>
                <p className="text-[10px] sm:text-xs text-gray-500 mt-1 break-all">
                  Max: {room.max_members} • {room.visibility === "private" ? "Maxfiy" : "Ochiq"}
                </p>
              </div>
              {isHost && (
                <button
                  onClick={handleCopyInviteLink}
                  className="hidden sm:flex px-3 py-2 bg-brand-red rounded-lg text-sm items-center gap-2"
                >
                  <Copy className="w-4 h-4" /> Taklif
                </button>
              )}
            </div>
          </div>
        </div>

        {/* ── Sidebar: members + chat ── */}
        <div className="flex flex-col gap-3 lg:max-h-[calc(100vh-2rem)]">
          {/* Members panel — collapsed on mobile unless toggled */}
          <div className={`${showMembers ? "block" : "hidden"} lg:block bg-brand-card border border-brand-border rounded-xl p-3`}>
            <div className="flex items-center gap-2 text-sm font-medium">
              <Users className="w-4 h-4" /> A'zolar ({memberList.length})
            </div>
            <ul className="mt-2 space-y-1 max-h-40 overflow-y-auto">
              {memberList.length === 0 && (
                <li className="text-xs text-gray-500">Mehmonlar kutilmoqda…</li>
              )}
              {memberList.map((m) => (
                <li key={m.userID} className="flex items-center gap-2 text-sm">
                  {m.userAvatar ? (
                    <MediaImage
                      src={m.userAvatar}
                      alt={m.userName}
                      className="w-6 h-6 rounded-full object-cover"
                    />
                  ) : (
                    <div className="w-6 h-6 rounded-full bg-brand-dark flex items-center justify-center text-xs">
                      {m.userName.slice(0, 1)}
                    </div>
                  )}
                  <span className="truncate flex-1">{m.userName || "Mehmon"}</span>
                  {m.isHost && <Crown className="w-3 h-3 text-yellow-400 shrink-0" />}
                  {isHost && !m.isHost && (
                    <button
                      onClick={() => handleKick(m.userID)}
                      className="p-1 text-red-400 hover:bg-red-500/10 rounded"
                      title="Chiqarish"
                    >
                      <UserX className="w-3.5 h-3.5" />
                    </button>
                  )}
                </li>
              ))}
            </ul>
          </div>

          {/* Chat — always visible */}
          <div className="flex-1 bg-brand-card border border-brand-border rounded-xl flex flex-col min-h-0">
            <div className="px-3 py-2 border-b border-brand-border text-sm font-medium flex items-center justify-between">
              <span>Chat</span>
              {typingNames.length > 0 && (
                <span className="text-[11px] text-gray-400 italic truncate ml-2">
                  {typingNames.slice(0, 2).join(", ")}
                  {typingNames.length > 2 && ` +${typingNames.length - 2}`} yozmoqda…
                </span>
              )}
            </div>
            <div ref={chatScrollRef} className="flex-1 overflow-y-auto p-3 space-y-2 min-h-[200px] lg:max-h-[60vh]">
              {chat.length === 0 && <p className="text-xs text-gray-500">Hozircha xabar yo&apos;q.</p>}
              {chat.map((c, idx) => (
                <div key={idx} className="text-sm">
                  <span className="text-gray-400 text-xs">{c.userName || "Mehmon"}: </span>
                  {c.kind === "emoji" ? (
                    <span className="text-2xl">{c.emoji}</span>
                  ) : (
                    <span className="break-words">{c.text}</span>
                  )}
                </div>
              ))}
            </div>
            <div className="border-t border-brand-border p-2 relative">
              {emojiOpen && (
                <div className="absolute bottom-12 left-2 right-2 bg-brand-dark border border-brand-border rounded-lg p-2 grid grid-cols-6 gap-1">
                  {EMOJI_PALETTE.map((e) => (
                    <button
                      key={e}
                      onClick={() => handleSendEmoji(e)}
                      className="text-2xl hover:bg-brand-card rounded p-1"
                    >
                      {e}
                    </button>
                  ))}
                </div>
              )}
              <div className="flex items-center gap-1">
                <button
                  onClick={() => setEmojiOpen((v) => !v)}
                  className="p-2 text-gray-400 hover:text-white"
                  aria-label="Reaktsiya"
                  title="Reaktsiya yuborish (video ustida ko'rinadi)"
                >
                  {emojiOpen ? <X className="w-4 h-4" /> : <PartyPopper className="w-4 h-4" />}
                </button>
                <input
                  value={chatInput}
                  onChange={(e) => onChatInputChange(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") handleSendChat();
                  }}
                  placeholder="Xabar yozing…"
                  className="flex-1 bg-brand-dark border border-brand-border rounded px-2 py-1.5 text-sm focus:outline-none focus:border-brand-red"
                />
                <button
                  onClick={handleSendChat}
                  disabled={!chatInput.trim()}
                  className="p-2 text-brand-red disabled:opacity-40"
                  aria-label="Yuborish"
                >
                  <Send className="w-4 h-4" />
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Animation styles for floating reactions */}
      <style jsx global>{`
        @keyframes floatUp {
          0% { transform: translateY(20px); opacity: 0; }
          15% { opacity: 1; }
          80% { opacity: 1; }
          100% { transform: translateY(-180px); opacity: 0; }
        }
      `}</style>

      {/* Inert imports kept for tree-shaking sanity */}
      <span className="hidden">
        <Smile className="w-0 h-0" />
      </span>
    </div>
  );
}

function GuestVolumeControl({ videoRef }: { videoRef: React.RefObject<HTMLVideoElement | null> }) {
  const [vol, setVol] = useState(1);
  const [muted, setMuted] = useState(false);
  return (
    <div className="absolute bottom-3 right-3 bg-black/70 rounded-lg px-2 py-1.5 flex items-center gap-1.5">
      <button
        onClick={() => {
          const v = videoRef.current;
          if (!v) return;
          v.muted = !v.muted;
          setMuted(v.muted);
        }}
        className="text-white text-xs"
      >
        {muted ? "🔇" : vol > 0.5 ? "🔊" : "🔉"}
      </button>
      <input
        type="range"
        min={0}
        max={1}
        step={0.01}
        value={vol}
        onChange={(e) => {
          const v = videoRef.current;
          const next = Number(e.target.value);
          setVol(next);
          if (v) v.volume = next;
        }}
        className="w-16 sm:w-24"
      />
    </div>
  );
}

function FloatingReactionBubble({ reaction, index }: { reaction: FloatingReaction; index: number }) {
  // Stagger horizontal position so multiple reactions don't stack.
  const leftPct = 20 + ((index * 13) % 60);
  return (
    <div
      className="absolute bottom-12 text-3xl sm:text-4xl"
      style={{
        left: `${leftPct}%`,
        animation: "floatUp 3s ease-out forwards",
      }}
    >
      <span className="block leading-none drop-shadow-lg">{reaction.emoji}</span>
      <span className="block text-[10px] text-white/80 mt-1 text-center whitespace-nowrap">
        {reaction.userName}
      </span>
    </div>
  );
}
