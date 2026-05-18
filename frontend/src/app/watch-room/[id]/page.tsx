"use client";

import { useEffect, useMemo, useRef, useState, useCallback } from "react";
import { useParams, useSearchParams, useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth-context";
import {
  getWatchRoom,
  listRoomMessages,
  createRoomInvite,
  WatchRoom,
  WatchRoomMessage,
} from "@/lib/api";
import { useRoomSocket, effectivePosition, RoomMember, RoomChatEntry } from "@/lib/use-room-socket";
import MediaImage from "@/components/ui/MediaImage";
import { Loader2, Send, Smile, Copy, Users, Crown, X, Play, Pause } from "lucide-react";

const EMOJI_PALETTE = ["😀", "😂", "❤️", "🔥", "👏", "🎉", "😮", "😢", "👍", "🤔", "😍", "🍿"];

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
  const [chatInput, setChatInput] = useState("");
  const [emojiOpen, setEmojiOpen] = useState(false);

  const videoRef = useRef<HTMLVideoElement | null>(null);
  const chatScrollRef = useRef<HTMLDivElement | null>(null);

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
        // Resolve video URL from the content (movie or episode).
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

  // ── WS connect ────────────────────────────────────────────────────────
  const { connected, state, events, sendHostAction, sendChat } = useRoomSocket(
    !authLoading && token && room ? roomID : null,
    token,
    inviteCode,
  );

  // ── Drain WS events into UI state ─────────────────────────────────────
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
      }
    }
  }, [events]);

  // ── Sync video element to host state (guests only) ───────────────────
  useEffect(() => {
    if (isHost) return; // host drives the timeline
    const v = videoRef.current;
    if (!v || !state) return;
    const target = effectivePosition(state);
    const drift = Math.abs(v.currentTime - target);
    if (drift > 1.5) {
      v.currentTime = target;
    }
    if (state.isPlaying && v.paused) {
      v.play().catch(() => undefined);
    } else if (!state.isPlaying && !v.paused) {
      v.pause();
    }
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

  const handleSendChat = () => {
    const trimmed = chatInput.trim();
    if (!trimmed) return;
    sendChat("text", trimmed);
    setChatInput("");
  };

  const handleSendEmoji = (emoji: string) => {
    sendChat("emoji", emoji);
    setEmojiOpen(false);
  };

  const handleCopyInviteLink = useCallback(async () => {
    if (!token || !room) return;
    try {
      const inv = await createRoomInvite(token, room.id, {});
      const url = `${window.location.origin}/watch-room/${room.id}?invite=${inv.code}`;
      await navigator.clipboard.writeText(url);
      alert("Invite link copied to clipboard.\n\n" + url);
    } catch (e) {
      alert(e instanceof Error ? e.message : "Failed to create invite");
    }
  }, [token, room]);

  // ── Loading / error / closed states ──────────────────────────────────
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
        <p>Tizimga kiring</p>
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
  if (closedReason) {
    return (
      <div className="min-h-screen flex flex-col items-center justify-center bg-brand-dark text-white gap-4">
        <h2 className="text-2xl font-bold">Room closed</h2>
        <p className="text-gray-400">Reason: {closedReason}</p>
        <button
          onClick={() => router.push("/rooms")}
          className="px-4 py-2 bg-brand-red rounded-lg"
        >
          Browse rooms
        </button>
      </div>
    );
  }

  const memberList = Object.values(members);

  return (
    <div className="min-h-screen bg-brand-dark text-white">
      <div className="max-w-7xl mx-auto p-4 grid grid-cols-1 lg:grid-cols-[1fr_320px] gap-4">
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
            {!isHost && (
              <div className="absolute top-2 left-2 bg-black/60 text-xs px-2 py-1 rounded">
                Guest — host controls playback
              </div>
            )}
            {!isHost && (
              <GuestVolumeControl videoRef={videoRef} />
            )}
            <div className="absolute top-2 right-2 flex items-center gap-2">
              <span
                className={`text-xs px-2 py-1 rounded ${connected ? "bg-green-500/80" : "bg-yellow-500/80"}`}
              >
                {connected ? "Live" : "Connecting…"}
              </span>
            </div>
          </div>
          <div className="bg-brand-card border border-brand-border rounded-xl p-4">
            <div className="flex items-start gap-4">
              {room.content_poster && (
                <div className="w-20 h-28 shrink-0 overflow-hidden rounded-md">
                  <MediaImage src={room.content_poster} alt={room.content_title || ""} className="w-full h-full object-cover" />
                </div>
              )}
              <div className="flex-1">
                <h1 className="text-xl font-semibold">{room.content_title}</h1>
                <p className="text-sm text-gray-400 mt-1">
                  Hosted by <Crown className="w-3 h-3 text-yellow-400 inline" /> {room.owner_name}
                  {room.owner_is_premium && <span className="ml-2 text-xs bg-yellow-500/20 text-yellow-300 px-1.5 rounded">PREMIUM</span>}
                </p>
                <p className="text-xs text-gray-500 mt-2">
                  Room ID: {room.id} • Visibility: {room.visibility} • Max members: {room.max_members}
                </p>
              </div>
              {isHost && (
                <button
                  onClick={handleCopyInviteLink}
                  className="px-3 py-2 bg-brand-red rounded-lg text-sm flex items-center gap-2"
                >
                  <Copy className="w-4 h-4" /> Invite link
                </button>
              )}
            </div>
          </div>
        </div>

        {/* ── Sidebar: members + chat ── */}
        <div className="flex flex-col gap-3 lg:max-h-[calc(100vh-2rem)]">
          <div className="bg-brand-card border border-brand-border rounded-xl p-3">
            <div className="flex items-center gap-2 text-sm font-medium">
              <Users className="w-4 h-4" /> Members ({memberList.length})
            </div>
            <ul className="mt-2 space-y-1 max-h-32 overflow-y-auto">
              {memberList.length === 0 && <li className="text-xs text-gray-500">Waiting for guests…</li>}
              {memberList.map((m) => (
                <li key={m.userID} className="flex items-center gap-2 text-sm">
                  {m.userAvatar ? (
                    <MediaImage src={m.userAvatar} alt={m.userName} className="w-6 h-6 rounded-full object-cover" />
                  ) : (
                    <div className="w-6 h-6 rounded-full bg-brand-dark flex items-center justify-center text-xs">
                      {m.userName.slice(0, 1)}
                    </div>
                  )}
                  <span className="truncate">{m.userName || "Guest"}</span>
                  {m.isHost && <Crown className="w-3 h-3 text-yellow-400 shrink-0" />}
                </li>
              ))}
            </ul>
          </div>

          <div className="flex-1 bg-brand-card border border-brand-border rounded-xl flex flex-col min-h-0">
            <div className="px-3 py-2 border-b border-brand-border text-sm font-medium">Chat</div>
            <div ref={chatScrollRef} className="flex-1 overflow-y-auto p-3 space-y-2 min-h-[200px]">
              {chat.length === 0 && <p className="text-xs text-gray-500">No messages yet.</p>}
              {chat.map((c, idx) => (
                <div key={idx} className="text-sm">
                  <span className="text-gray-400 text-xs">{c.userName || "Guest"}: </span>
                  {c.kind === "emoji" ? (
                    <span className="text-2xl">{c.emoji}</span>
                  ) : (
                    <span>{c.text}</span>
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
                  aria-label="Emoji"
                >
                  {emojiOpen ? <X className="w-4 h-4" /> : <Smile className="w-4 h-4" />}
                </button>
                <input
                  value={chatInput}
                  onChange={(e) => setChatInput(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") handleSendChat();
                  }}
                  placeholder="Message…"
                  className="flex-1 bg-brand-dark border border-brand-border rounded px-2 py-1.5 text-sm focus:outline-none focus:border-brand-red"
                />
                <button
                  onClick={handleSendChat}
                  disabled={!chatInput.trim()}
                  className="p-2 text-brand-red disabled:opacity-40"
                  aria-label="Send"
                >
                  <Send className="w-4 h-4" />
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

// Small floating volume slider for guests since their <video controls> is off.
function GuestVolumeControl({ videoRef }: { videoRef: React.RefObject<HTMLVideoElement | null> }) {
  const [vol, setVol] = useState(1);
  const [muted, setMuted] = useState(false);
  return (
    <div className="absolute bottom-4 right-4 bg-black/70 rounded-lg px-3 py-2 flex items-center gap-2">
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
        className="w-20"
      />
    </div>
  );
}

// Suppress unused imports if tree-shaking warnings appear.
void Play;
void Pause;
