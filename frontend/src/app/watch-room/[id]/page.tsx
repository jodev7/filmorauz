"use client";

import { useEffect, useRef, useState, useCallback } from "react";
import { useParams, useSearchParams, useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth-context";
import Hls from "hls.js";
import {
  getWatchRoom,
  listRoomMessages,
  createRoomInvite,
  searchRoomUsers,
  getProtectedMediaAccess,
  WatchRoom,
  WatchRoomMessage,
  RoomUserResult,
} from "@/lib/api";
import { normalizeMediaUrl } from "@/lib/image-utils";
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
  const [inviteOpen, setInviteOpen] = useState(false);
  const [linkPopup, setLinkPopup] = useState<{ url: string; copied: boolean } | null>(null);

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
        // Resolve via the same protected-media access flow the regular watch
        // page uses. Raw master_playlist_url/video_url on the doc are
        // typically B2 keys that require a signed CDN URL — without this
        // step the room sees an empty/forbidden URL and renders black.
        let resolved = "";
        try {
          const access = await getProtectedMediaAccess(
            r.content_type === "movie"
              ? { movieId: r.content_id, token }
              : { episodeId: r.content_id, token },
          );
          const playback = access.playback_url || "";
          resolved = playback.startsWith("https://cdn.filmorauz.net/media/")
            ? playback
            : normalizeMediaUrl(playback, "") || playback;
        } catch {
          /* fall through to direct-URL fallback */
        }

        if (!resolved) {
          // Last-resort fallback: try fetching the public movie/episode doc
          // and using whatever direct URL is set on it (works for legacy
          // movies that don't go through protected media).
          const apiBase = process.env.NEXT_PUBLIC_API_URL || "";
          if (r.content_type === "movie") {
            const m = await fetch(`${apiBase}/movies/${r.content_id}`).then((res) =>
              res.ok ? res.json() : null,
            );
            if (m)
              resolved =
                m.master_playlist_url ||
                m.streaming_url ||
                m.playlist_url ||
                m.video_url ||
                "";
          } else {
            const ep = await fetch(`${apiBase}/episodes/${r.content_id}`).then((res) =>
              res.ok ? res.json() : null,
            );
            if (ep)
              resolved =
                ep.video_url ||
                ep.master_playlist_url ||
                ep.streaming_url ||
                ep.playlist_url ||
                "";
          }
        }

        if (!cancelled) {
          if (!resolved) {
            setLoadError(
              "Bu kontent uchun video manzili topilmadi. Admin tomonidan tasdiqlanmagan bo'lishi mumkin.",
            );
          } else {
            setVideoSrc(resolved);
          }
        }
        const msgs = await listRoomMessages(roomID).catch(() => ({ items: [] as WatchRoomMessage[] }));
        if (!cancelled) {
          // Backend returns {items: null} when there are no messages yet —
          // guard against the null before calling .map().
          const items = msgs.items || [];
          setChat(
            items.map((m) => ({
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
      } else if (e.type === "member_snapshot") {
        const map: Record<string, RoomMember> = {};
        for (const m of e.members) map[m.userID] = m;
        setMembers(map);
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

  // ── Attach HLS.js for .m3u8 sources. Native <video> can't play HLS on
  // Chrome/Firefox/Edge; without this the player just shows black.
  useEffect(() => {
    const v = videoRef.current;
    if (!v || !videoSrc) return;
    const isHls = /\.m3u8(\?|$)/i.test(videoSrc) || videoSrc.includes("/master.m3u8");
    if (!isHls) {
      v.src = videoSrc;
      return;
    }
    // Safari plays HLS natively.
    if (v.canPlayType("application/vnd.apple.mpegurl")) {
      v.src = videoSrc;
      return;
    }
    if (!Hls.isSupported()) {
      v.src = videoSrc;
      return;
    }
    const hls = new Hls({ enableWorker: true });
    hls.loadSource(videoSrc);
    hls.attachMedia(v);
    return () => {
      hls.destroy();
    };
  }, [videoSrc]);

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
      let copied = false;
      try {
        await navigator.clipboard.writeText(url);
        copied = true;
      } catch {
        /* clipboard may be unavailable on non-https or older Safari */
      }
      setLinkPopup({ url, copied });
    } catch (e) {
      setLinkPopup({ url: e instanceof Error ? e.message : "Taklif yaratishda xatolik", copied: false });
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
    const callbackUrl =
      typeof window !== "undefined"
        ? `${window.location.pathname}${window.location.search}`
        : `/watch-room/${roomID}`;
    return (
      <div className="min-h-screen flex items-center justify-center bg-brand-dark text-white px-4">
        <div className="max-w-md w-full bg-brand-card border border-brand-border rounded-xl p-6 text-center space-y-4">
          <Users className="w-12 h-12 text-brand-red mx-auto" />
          <h2 className="text-xl font-semibold">Birga ko&apos;rish room&apos;iga kirish</h2>
          <p className="text-sm text-gray-400">
            Bu room maxfiy. Qo&apos;shilish uchun avval ro&apos;yxatdan o&apos;tib, Telegram orqali
            tizimga kirishingiz kerak.
          </p>
          <p className="text-xs text-gray-500">
            Taklif havolasi sizning hisobingizga bog&apos;langan bo&apos;ladi.
          </p>
          <button
            onClick={() => {
              try {
                window.localStorage.setItem("post_login_redirect", callbackUrl);
              } catch {
                /* ignore */
              }
              router.push("/?login=1");
            }}
            className="w-full px-4 py-2 bg-brand-red rounded-lg font-medium"
          >
            Ro&apos;yxatdan o&apos;tish / Kirish
          </button>
        </div>
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
                onClick={() => setInviteOpen(true)}
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
                  onClick={() => setInviteOpen(true)}
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
                <li className="text-xs text-gray-500">Hozircha sizdan boshqa hech kim yo&apos;q</li>
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
                  <span className="truncate flex-1">{m.userName || "Foydalanuvchi"}</span>
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
                  <span className="text-gray-400 text-xs">{c.userName || "Foydalanuvchi"}: </span>
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

      {/* Invite modal */}
      {inviteOpen && room && token && (
        <InviteModal
          token={token}
          roomID={room.id}
          contentTitle={room.content_title || ""}
          onClose={() => setInviteOpen(false)}
          onLinkReady={handleCopyInviteLink}
        />
      )}

      {/* Invite link popup */}
      {linkPopup && (
        <div
          className="fixed inset-0 z-50 bg-black/70 flex items-center justify-center p-4"
          onClick={() => setLinkPopup(null)}
        >
          <div
            className="bg-brand-card border border-brand-border rounded-xl w-full max-w-md overflow-hidden"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="px-4 py-3 border-b border-brand-border flex items-center justify-between">
              <h3 className="font-semibold">Taklif havolasi</h3>
              <button onClick={() => setLinkPopup(null)} className="text-gray-400 hover:text-white">
                <X className="w-5 h-5" />
              </button>
            </div>
            <div className="p-4 space-y-3">
              {linkPopup.copied && (
                <p className="text-xs text-green-300">Havola nusxa olindi ✓</p>
              )}
              <p className="text-xs text-gray-400">Ushbu havolani do&apos;stlaringizga yuboring:</p>
              <div className="bg-brand-dark border border-brand-border rounded p-2 text-xs break-all font-mono select-all">
                {linkPopup.url}
              </div>
              <div className="flex gap-2">
                <button
                  onClick={async () => {
                    try {
                      await navigator.clipboard.writeText(linkPopup.url);
                      setLinkPopup({ ...linkPopup, copied: true });
                    } catch {
                      /* ignore */
                    }
                  }}
                  className="flex-1 px-3 py-2 bg-brand-red rounded-lg text-sm flex items-center justify-center gap-2"
                >
                  <Copy className="w-4 h-4" /> Nusxa olish
                </button>
                <button
                  onClick={() => setLinkPopup(null)}
                  className="px-3 py-2 bg-brand-dark border border-brand-border rounded-lg text-sm"
                >
                  Yopish
                </button>
              </div>
              <p className="text-[10px] text-gray-500">
                Havola 6 soat amal qiladi (premium uchun 24 soat). Faqat ro&apos;yxatdan o&apos;tgan
                foydalanuvchilar kira oladi.
              </p>
            </div>
          </div>
        </div>
      )}

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

function InviteModal({
  token,
  roomID,
  contentTitle,
  onClose,
  onLinkReady,
}: {
  token: string;
  roomID: string;
  contentTitle: string;
  onClose: () => void;
  onLinkReady: () => void;
}) {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<RoomUserResult[]>([]);
  const [searching, setSearching] = useState(false);
  const [sentTo, setSentTo] = useState<Record<string, boolean>>({});
  const [error, setError] = useState("");

  // Debounced search.
  useEffect(() => {
    const q = query.trim();
    if (!q) {
      setResults([]);
      return;
    }
    setSearching(true);
    setError("");
    const t = setTimeout(async () => {
      try {
        const r = await searchRoomUsers(token, q);
        setResults(r.items || []);
      } catch (e) {
        setError(e instanceof Error ? e.message : "Qidirishda xato");
      } finally {
        setSearching(false);
      }
    }, 300);
    return () => clearTimeout(t);
  }, [query, token]);

  const handleInviteUser = async (userID: string) => {
    setError("");
    try {
      await createRoomInvite(token, roomID, { target_user_id: userID, max_uses: 1 });
      setSentTo((prev) => ({ ...prev, [userID]: true }));
    } catch (e) {
      setError(e instanceof Error ? e.message : "Yuborib bo'lmadi");
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 bg-black/70 flex items-center justify-center p-4"
      onClick={onClose}
    >
      <div
        className="bg-brand-card border border-brand-border rounded-xl w-full max-w-md max-h-[80vh] flex flex-col overflow-hidden"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="px-4 py-3 border-b border-brand-border flex items-center justify-between">
          <h3 className="font-semibold">Taklif yuborish</h3>
          <button onClick={onClose} className="text-gray-400 hover:text-white">
            <X className="w-5 h-5" />
          </button>
        </div>
        <div className="p-4 space-y-3">
          <p className="text-xs text-gray-400">
            <span className="text-white">{contentTitle}</span> uchun room
          </p>
          <div>
            <p className="text-xs text-gray-400 mb-1">Telegram ID yoki ism orqali qidiring:</p>
            <input
              autoFocus
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="123456789 yoki Ali"
              className="w-full bg-brand-dark border border-brand-border rounded px-3 py-2 text-sm focus:outline-none focus:border-brand-red"
            />
          </div>
          {error && <p className="text-xs text-red-400">{error}</p>}
          <div className="max-h-60 overflow-y-auto -mx-1">
            {searching && <p className="text-xs text-gray-500 px-1">Qidirilmoqda…</p>}
            {!searching && query && results.length === 0 && (
              <p className="text-xs text-gray-500 px-1">Hech kim topilmadi</p>
            )}
            <ul className="space-y-1">
              {results.map((u) => (
                <li
                  key={u.id}
                  className="flex items-center gap-2 px-1 py-1.5 hover:bg-brand-dark rounded"
                >
                  {u.avatar ? (
                    <MediaImage
                      src={u.avatar}
                      alt={u.display_name}
                      className="w-8 h-8 rounded-full object-cover"
                    />
                  ) : (
                    <div className="w-8 h-8 rounded-full bg-brand-dark flex items-center justify-center text-xs">
                      {(u.display_name || "?").slice(0, 1)}
                    </div>
                  )}
                  <div className="flex-1 min-w-0">
                    <p className="text-sm truncate">{u.display_name || "Foydalanuvchi"}</p>
                    {u.telegram_id ? (
                      <p className="text-[10px] text-gray-500 font-mono truncate">TG: {u.telegram_id}</p>
                    ) : (
                      <p className="text-[10px] text-gray-500 font-mono truncate">{u.id}</p>
                    )}
                  </div>
                  <button
                    disabled={!!sentTo[u.id]}
                    onClick={() => handleInviteUser(u.id)}
                    className={`px-2 py-1 text-xs rounded ${
                      sentTo[u.id]
                        ? "bg-green-600/40 text-green-200"
                        : "bg-brand-red text-white hover:bg-red-700"
                    }`}
                  >
                    {sentTo[u.id] ? "Yuborildi" : "Taklif"}
                  </button>
                </li>
              ))}
            </ul>
          </div>
          <div className="border-t border-brand-border pt-3">
            <button
              onClick={() => {
                onLinkReady();
                onClose();
              }}
              className="w-full px-3 py-2 bg-brand-dark border border-brand-border hover:border-brand-red rounded-lg text-sm flex items-center justify-center gap-2"
            >
              <Copy className="w-4 h-4" /> Yoki taklif havolasini nusxa olish
            </button>
          </div>
        </div>
      </div>
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
