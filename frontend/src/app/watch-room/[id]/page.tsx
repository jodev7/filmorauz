"use client";

import { useEffect, useRef, useState, useCallback } from "react";
import { useParams, useSearchParams, useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth-context";
import RoomPlayer from "@/components/watch-room/RoomPlayer";
import {
  getWatchRoom,
  listRoomMessages,
  createRoomInvite,
  searchRoomUsers,
  getProtectedMediaAccess,
  changeRoomEpisode,
  closeWatchRoom,
  updateRoomTheme,
  WatchRoom,
  WatchRoomMessage,
  RoomUserResult,
} from "@/lib/api";
import { getSeriesEpisodesByID, SeasonWithEpisodes } from "@/lib/series-api";
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
  ArrowLeft,
  Home,
  AlertTriangle,
  SkipForward,
  List,
  XCircle,
  Palette,
  Share2,
} from "lucide-react";
import Navbar from "@/components/Navbar";
import Link from "next/link";

const EMOJI_PALETTE = ["😀", "😂", "❤️", "🔥", "👏", "🎉", "😮", "😢", "👍", "🤔", "😍", "🍿"];

// Preset gradients for the premium-only room theme picker. Each gradient
// is hard-coded as a (from, to) hex pair so the picker is deterministic
// and the backend's input validation stays tight.
const ROOM_THEMES: Array<{ label: string; from: string; to: string }> = [
  { label: "Default", from: "#0a0a0f", to: "#0a0a0f" },
  { label: "Ocean", from: "#0f172a", to: "#1e3a8a" },
  { label: "Sunset", from: "#7c2d12", to: "#7c1d6f" },
  { label: "Forest", from: "#052e16", to: "#14532d" },
  { label: "Crimson", from: "#1f0a0a", to: "#7f1d1d" },
  { label: "Gold", from: "#1c1917", to: "#854d0e" },
];

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
  // Host-disconnect countdown — non-null while the host is gone and the
  // grace timer is ticking down. Cleared on host_reconnected.
  const [hostGoneDeadline, setHostGoneDeadline] = useState<number | null>(null);
  const [graceLeft, setGraceLeft] = useState<number>(0);
  // Episode switcher state (series rooms only). Lazy-loaded on host open.
  const [showEpisodes, setShowEpisodes] = useState(false);
  const [episodes, setEpisodes] = useState<SeasonWithEpisodes[]>([]);
  // Inbound episode-change requests from guests (host-side toast).
  const [episodeRequestToast, setEpisodeRequestToast] = useState<
    { userName: string; reason: string; targetEpisodeID?: string; ts: number } | null
  >(null);
  // Confirm popup for host's "close room" action.
  const [closeConfirm, setCloseConfirm] = useState(false);
  const [closeBusy, setCloseBusy] = useState(false);
  const [themePickerOpen, setThemePickerOpen] = useState(false);
  // Live theme; mirrors room.theme but updates instantly when a
  // theme_change WS event arrives so guests repaint without reload.
  const [liveTheme, setLiveTheme] = useState<{ from: string; to: string } | null>(null);

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
        // Hard-gate closed rooms at fetch time. Without this check the
        // page kept rendering the "Ulanish…" badge forever because the
        // WS upgrade silently 404'd against a `status: closed` row
        // while the React page sat waiting for state.
        if (r.status !== "active") {
          setClosedReason("yopilgan");
          setRoom(r);
          return;
        }
        setRoom(r);
        if (r.theme && r.theme.from && r.theme.to) {
          setLiveTheme({ from: r.theme.from, to: r.theme.to });
        }
        // Resolve playback URL. We try TWO sources and prefer whichever
        // is a multi-variant master playlist (so the quality picker has
        // something to pick). Users reported that with protected-only the
        // picker shows "Bu video uchun boshqa sifat mavjud emas" because
        // the protected playback URL is a single-variant CDN URL.
        let resolved = "";
        let rawMaster = "";
        const apiBase = process.env.NEXT_PUBLIC_API_URL || "";
        // For series rooms, content_id is the SERIES id; the actual
        // playable resource is the current episode. Resolve via
        // current_episode_id so we don't hit /episodes/<seriesID> 404.
        const playableType: "movie" | "episode" =
          r.content_type === "movie" ? "movie" : "episode";
        const playableID =
          r.content_type === "series" ? (r.current_episode_id || "") : r.content_id;
        try {
          if (!playableID) {
            // Series with no current episode — nothing to play yet.
          } else if (playableType === "movie") {
            const m = await fetch(`${apiBase}/movies/${playableID}`).then((res) =>
              res.ok ? res.json() : null,
            );
            if (m) {
              rawMaster =
                m.master_playlist_url ||
                m.streaming_url ||
                m.playlist_url ||
                m.video_url ||
                "";
            }
          } else {
            const ep = await fetch(`${apiBase}/episodes/${playableID}`).then((res) =>
              res.ok ? res.json() : null,
            );
            if (ep) {
              // /episodes/:id returns {episode, previous_episode, next_episode}
              const epDoc = ep.episode || ep;
              rawMaster =
                epDoc.master_playlist_url ||
                epDoc.streaming_url ||
                epDoc.playlist_url ||
                epDoc.video_url ||
                "";
            }
          }
        } catch {
          /* ignore */
        }
        if (rawMaster && /\.m3u8(\?|$)/i.test(rawMaster) && /^https?:\/\//i.test(rawMaster)) {
          resolved = rawMaster;
        }
        if (!resolved && playableID) {
          try {
            const access = await getProtectedMediaAccess(
              playableType === "movie"
                ? { movieId: playableID, token }
                : { episodeId: playableID, token },
            );
            const playback = access.playback_url || "";
            resolved = playback.startsWith("https://cdn.filmorauz.net/media/")
              ? playback
              : normalizeMediaUrl(playback, "") || playback;
          } catch {
            /* fall through */
          }
        }
        if (!resolved && rawMaster) {
          resolved = rawMaster;
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
    sendEpisodeRequest,
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
        // System chat entry — "{name} room'ga qo'shildi".
        if (e.member.userID !== user?.id) {
          setChat((prev) => [
            ...prev,
            {
              userID: "system",
              userName: "system",
              kind: "text",
              text: `${e.member.userName || "Foydalanuvchi"} room'ga qo'shildi`,
              createdAt: new Date().toISOString(),
            },
          ]);
        }
        setMembers((prev) => ({ ...prev, [e.member.userID]: e.member }));
      } else if (e.type === "member_left") {
        const leavingMember = members[e.userID];
        const leaverName = leavingMember?.userName || "Foydalanuvchi";
        const wasHost = !!leavingMember?.isHost;
        // System chat entry — host gets a special "60s grace" notice.
        setChat((prev) => [
          ...prev,
          {
            userID: "system",
            userName: "system",
            kind: "text",
            text: wasHost
              ? `${leaverName} (host) chiqib ketdi. 5 daqiqa ichida qaytmasa room yopiladi.`
              : e.kicked
                ? `${leaverName} room'dan chiqarildi`
                : `${leaverName} chiqib ketdi`,
            createdAt: new Date().toISOString(),
          },
        ]);
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
      } else if (e.type === "host_disconnected") {
        setHostGoneDeadline(e.deadlineMs);
        setChat((prev) => [
          ...prev,
          {
            userID: "system",
            userName: "system",
            kind: "text",
            text: `Host vaqtincha uzildi. ${Math.round(e.graceSeconds / 60)} daqiqa ichida qaytmasa room yopiladi.`,
            createdAt: new Date().toISOString(),
          },
        ]);
      } else if (e.type === "host_reconnected") {
        setHostGoneDeadline(null);
        setChat((prev) => [
          ...prev,
          {
            userID: "system",
            userName: "system",
            kind: "text",
            text: "Host qaytib keldi.",
            createdAt: new Date().toISOString(),
          },
        ]);
      } else if (e.type === "episode_change") {
        // Re-resolve the new episode's playback URL and swap videoSrc.
        // Update the room object too so the title overlay matches.
        setRoom((prev) =>
          prev
            ? { ...prev, current_episode_id: e.episodeID, current_episode_title: e.episodeTitle }
            : prev,
        );
        (async () => {
          try {
            const access = await getProtectedMediaAccess({ episodeId: e.episodeID, token });
            const playback = access.playback_url || "";
            const url = playback.startsWith("https://cdn.filmorauz.net/media/")
              ? playback
              : normalizeMediaUrl(playback, "") || playback;
            if (url) setVideoSrc(url);
          } catch {
            /* ignore */
          }
        })();
        setChat((prev) => [
          ...prev,
          {
            userID: "system",
            userName: "system",
            kind: "text",
            text: `Epizod o'zgardi: ${e.episodeTitle}`,
            createdAt: new Date().toISOString(),
          },
        ]);
      } else if (e.type === "episode_request") {
        if (isHost) {
          setEpisodeRequestToast({
            userName: e.userName,
            reason: e.reason,
            targetEpisodeID: e.targetEpisodeID,
            ts: Date.now(),
          });
          setTimeout(() => setEpisodeRequestToast(null), 8000);
        }
      } else if (e.type === "theme_change") {
        setLiveTheme({ from: e.from, to: e.to });
      } else if (e.type === "reaction") {
        const id = `${e.reaction.ts}-${e.reaction.userID}-${Math.random()}`;
        setFloatingReactions((prev) => [...prev, { ...e.reaction, id }]);
        // Auto-remove after the float animation (~3s).
        setTimeout(() => {
          setFloatingReactions((prev) => prev.filter((r) => r.id !== id));
        }, 3000);
        // Also mirror the reaction into the side-chat log so users who
        // looked away can scroll back and see who reacted with what.
        setChat((prev) => [
          ...prev,
          {
            userID: e.reaction.userID,
            userName: e.reaction.userName,
            userAvatar: undefined,
            kind: "emoji",
            emoji: e.reaction.emoji,
            createdAt: new Date(e.reaction.ts).toISOString(),
          },
        ]);
      }
    }
  }, [events, user?.id]);

  // ── Sync guest player to host state via the RoomPlayer's sync API ───
  const syncApiRef = useRef<{ setPosition: (s: number) => void; setPlaying: (p: boolean) => void } | null>(null);
  useEffect(() => {
    if (isHost || !state || !syncApiRef.current) return;
    const target = effectivePosition(state);
    syncApiRef.current.setPosition(target);
    syncApiRef.current.setPlaying(state.isPlaying);
  }, [state, isHost]);

  // ── Host broadcasts player events to the hub ────────────────────────
  const onHostPlay = useCallback(
    (pos: number) => {
      if (isHost) sendHostAction("play", pos);
    },
    [isHost, sendHostAction],
  );
  const onHostPause = useCallback(
    (pos: number) => {
      if (isHost) sendHostAction("pause", pos);
    },
    [isHost, sendHostAction],
  );
  // Seek broadcasts are debounced so a frantic scrub (drag back and
  // forth across the timeline) doesn't spray dozens of state_sync
  // messages at every guest. Without this, guests can't keep up:
  // each new sync triggers a buffer flush + seek, the next one
  // arrives before the seek lands, and the guest's video silently
  // stops advancing. 200ms is short enough that releasing the scrub
  // bar feels instant but long enough to coalesce a hold-and-drag.
  const seekDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const onHostSeek = useCallback(
    (pos: number) => {
      if (!isHost) return;
      if (seekDebounceRef.current) clearTimeout(seekDebounceRef.current);
      seekDebounceRef.current = setTimeout(() => {
        sendHostAction("seek", pos);
        seekDebounceRef.current = null;
      }, 200);
    },
    [isHost, sendHostAction],
  );

  // ── Series-room: load episode list once for the switcher + auto-advance ─
  useEffect(() => {
    if (!room || room.content_type !== "series" || !room.series_id) return;
    let cancelled = false;
    (async () => {
      try {
        const list = await getSeriesEpisodesByID(room.series_id!);
        if (!cancelled) setEpisodes(list);
      } catch {
        /* non-fatal — switcher just stays empty */
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [room?.id, room?.series_id, room?.content_type]); // eslint-disable-line react-hooks/exhaustive-deps

  // ── Host-disconnect countdown ticker ─────────────────────────────────
  useEffect(() => {
    if (hostGoneDeadline == null) {
      setGraceLeft(0);
      return;
    }
    const update = () => {
      const left = Math.max(0, Math.round((hostGoneDeadline - Date.now()) / 1000));
      setGraceLeft(left);
    };
    update();
    const t = setInterval(update, 500);
    return () => clearInterval(t);
  }, [hostGoneDeadline]);

  // ── Helpers: flatten episodes, find next/prev for auto-advance ──────
  const flatEpisodes = (() => {
    const out: Array<{ id: string; title: string; seasonNumber: number; episodeNumber: number }> = [];
    for (const sw of episodes) {
      for (const ep of sw.episodes) {
        out.push({
          id: ep.id,
          title: ep.title,
          seasonNumber: sw.season.season_number,
          episodeNumber: ep.episode_number,
        });
      }
    }
    return out;
  })();
  const currentEpIdx = room?.current_episode_id
    ? flatEpisodes.findIndex((e) => e.id === room.current_episode_id)
    : -1;
  const nextEp = currentEpIdx >= 0 ? flatEpisodes[currentEpIdx + 1] : undefined;

  // ── Host: when current episode video ends, auto-advance ─────────────
  // We rely on the page-level <video> ref inside RoomPlayer firing an
  // "ended" event; bind it via the videoRef we already capture for guest
  // volume. RoomPlayer renders its own <video> so we listen via document.
  useEffect(() => {
    if (!isHost || !room || room.content_type !== "series" || !token) return;
    const v = document.querySelector("video");
    if (!v) return;
    const onEnded = async () => {
      if (!nextEp) return;
      try {
        await changeRoomEpisode(token, room.id, nextEp.id);
      } catch {
        /* ignore — switcher still works manually */
      }
    };
    v.addEventListener("ended", onEnded);
    return () => v.removeEventListener("ended", onEnded);
  }, [isHost, room, token, nextEp, videoSrc]);

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
    <div
      className="min-h-screen bg-brand-dark text-white"
      style={
        liveTheme
          ? {
              backgroundImage: `linear-gradient(135deg, ${liveTheme.from}, ${liveTheme.to})`,
              backgroundAttachment: "fixed",
            }
          : undefined
      }
    >
      <Navbar />
      {/* Navbar is position:fixed h-16 so the page content has to start
          below it manually — without pt-16 the back-row sits underneath
          the navbar and looks "tiqilib qolgan". */}
      <div className="max-w-7xl mx-auto px-2 sm:px-4 pt-20 sm:pt-24">
        <div className="flex items-center gap-2 mb-3">
          <button
            onClick={() => router.back()}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs sm:text-sm text-gray-300 bg-brand-card hover:bg-brand-card/70 border border-brand-border rounded-lg transition-colors"
          >
            <ArrowLeft className="w-4 h-4" /> Orqaga
          </button>
          <Link
            href="/"
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs sm:text-sm text-gray-300 bg-brand-card hover:bg-brand-card/70 border border-brand-border rounded-lg transition-colors"
          >
            <Home className="w-4 h-4" /> Bosh sahifa
          </Link>
          <div className="flex-1" />
          {/* Host-only "close room" button. Tears the room down for
              everyone — guests get a `room_closed` WS event and land
              on the "Room yopildi" screen. Required so a host can
              free up their one-active-room slot without waiting for
              the 5-min disconnect grace. */}
          {isHost && (user as { is_premium?: boolean } | null)?.is_premium && (
            <button
              onClick={() => setThemePickerOpen(true)}
              className="flex items-center gap-1.5 px-3 py-1.5 text-xs sm:text-sm text-yellow-300 bg-yellow-900/30 hover:bg-yellow-900/50 border border-yellow-700/50 rounded-lg transition-colors"
              title="Room mavzusi (premium)"
            >
              <Palette className="w-4 h-4" /> Mavzu
            </button>
          )}
          {isHost && (
            <button
              onClick={() => setCloseConfirm(true)}
              className="flex items-center gap-1.5 px-3 py-1.5 text-xs sm:text-sm text-red-300 bg-red-900/30 hover:bg-red-900/50 border border-red-700/50 rounded-lg transition-colors"
              title="Roomni yopish (host)"
            >
              <XCircle className="w-4 h-4" /> Roomni yopish
            </button>
          )}
        </div>
      </div>
      <div className="max-w-7xl mx-auto px-2 sm:px-4 pb-4 grid grid-cols-1 lg:grid-cols-[1fr_320px] gap-3 lg:gap-4">
        {/* ── Player + info ── */}
        <div className="space-y-3">
          <div className="bg-black rounded-xl overflow-hidden aspect-video relative">
            {videoSrc ? (
              <RoomPlayer
                src={videoSrc}
                title={room.content_title || ""}
                posterUrl={room.content_poster}
                isHost={isHost}
                isMoviePremium={!!room.owner_is_premium}
                persistKey={
                  isHost
                    ? room.content_type === "series"
                      ? // Series rooms: scope the saved position to the
                        // CURRENT episode so switching epizodes always
                        // starts the new one from 0 instead of resuming
                        // at the previous episode's playhead.
                        `${room.id}-${user.id}-${room.current_episode_id || "ep"}`
                      : `${room.id}-${user.id}`
                    : undefined
                }
                onHostPlay={onHostPlay}
                onHostPause={onHostPause}
                onHostSeek={onHostSeek}
                registerSync={(api) => {
                  syncApiRef.current = api;
                }}
                fullscreenOverlay={
                  <FullscreenChatOverlay reactions={floatingReactions} chat={chat} />
                }
                reactionsOverlay={floatingReactions.map((r, idx) => (
                  <FloatingReactionBubble key={r.id} reaction={r} index={idx} />
                ))}
              />
            ) : (
              <div className="flex items-center justify-center h-full">
                <Loader2 className="w-8 h-8 animate-spin text-brand-red" />
              </div>
            )}

            {/* Host-disconnect countdown overlay — only shown to guests
                while the host is gone and the grace timer is ticking. */}
            {hostGoneDeadline != null && !isHost && (
              <div className="absolute inset-0 z-30 bg-black/70 flex items-center justify-center text-center px-4">
                <div className="space-y-3">
                  <AlertTriangle className="w-10 h-10 text-yellow-400 mx-auto" />
                  <p className="text-white font-medium">Host vaqtincha uzildi</p>
                  <p className="text-3xl font-mono tabular-nums">
                    {String(Math.floor(graceLeft / 60)).padStart(2, "0")}:
                    {String(graceLeft % 60).padStart(2, "0")}
                  </p>
                  <p className="text-xs text-gray-400 max-w-xs">
                    Agar shu vaqt ichida qaytib kelmasa, room avtomatik yopiladi.
                  </p>
                </div>
              </div>
            )}
            <div className="absolute top-2 right-2 flex items-center gap-2 z-20 pointer-events-none">
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

          {/* Series-room controls: host has an episode switcher; guests
              can nudge the host with a "next episode" request. */}
          {room.content_type === "series" && (
            <div className="bg-brand-card border border-brand-border rounded-xl p-3 flex items-center gap-2 flex-wrap">
              <span className="text-xs text-gray-400 mr-1">Hozir:</span>
              <span className="text-sm font-medium truncate flex-1 min-w-0">
                {room.current_episode_title || "—"}
              </span>
              {isHost ? (
                <>
                  {nextEp && (
                    <button
                      onClick={async () => {
                        if (!token) return;
                        try {
                          await changeRoomEpisode(token, room.id, nextEp.id);
                        } catch (err) {
                          alert(err instanceof Error ? err.message : "Xato");
                        }
                      }}
                      className="px-2.5 py-1.5 bg-brand-dark border border-brand-border hover:border-brand-red rounded-lg text-xs flex items-center gap-1.5"
                    >
                      <SkipForward className="w-3.5 h-3.5" /> Keyingisi
                    </button>
                  )}
                  <button
                    onClick={() => setShowEpisodes(true)}
                    className="px-2.5 py-1.5 bg-brand-dark border border-brand-border hover:border-brand-red rounded-lg text-xs flex items-center gap-1.5"
                  >
                    <List className="w-3.5 h-3.5" /> Epizodlar
                  </button>
                </>
              ) : (
                nextEp && (
                  <button
                    onClick={() => sendEpisodeRequest(nextEp.id, "next")}
                    className="px-2.5 py-1.5 bg-brand-dark border border-brand-border hover:border-brand-red rounded-lg text-xs flex items-center gap-1.5"
                    title="Hostga keyingi epizodga o'tishni so'rash"
                  >
                    <SkipForward className="w-3.5 h-3.5" /> Keyingisini so&apos;rash
                  </button>
                )
              )}
            </div>
          )}

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
          <div className="flex-1 bg-brand-card border border-brand-border rounded-xl flex flex-col min-h-0 font-poppins">
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
                  {c.userID === "system" ? (
                    <span className="text-[11px] text-gray-500 italic">• {c.text}</span>
                  ) : (
                    <>
                      <span className="text-gray-400 text-xs">{c.userName || "Foydalanuvchi"}: </span>
                      {c.kind === "emoji" ? (
                        <span className="text-2xl font-emoji">{c.emoji}</span>
                      ) : (
                        <span className="break-words">{c.text}</span>
                      )}
                    </>
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
                      className="text-2xl hover:bg-brand-card rounded p-1 font-emoji"
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

      {/* Episode switcher modal (host, series rooms) */}
      {showEpisodes && room.content_type === "series" && token && (
        <div
          className="fixed inset-0 z-50 bg-black/70 flex items-center justify-center p-4"
          onClick={() => setShowEpisodes(false)}
        >
          <div
            className="bg-brand-card border border-brand-border rounded-xl w-full max-w-md max-h-[80vh] flex flex-col overflow-hidden"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="px-4 py-3 border-b border-brand-border flex items-center justify-between">
              <h3 className="font-semibold">Epizodlar</h3>
              <button onClick={() => setShowEpisodes(false)} className="text-gray-400 hover:text-white">
                <X className="w-5 h-5" />
              </button>
            </div>
            <div className="overflow-y-auto p-3 space-y-3">
              {episodes.length === 0 && (
                <p className="text-xs text-gray-500">Epizodlar topilmadi</p>
              )}
              {episodes.map((sw) => (
                <div key={sw.season.id}>
                  <p className="text-xs text-gray-400 mb-1">
                    Season {sw.season.season_number} — {sw.season.title}
                  </p>
                  <ul className="space-y-1">
                    {sw.episodes.map((ep) => {
                      const active = ep.id === room.current_episode_id;
                      return (
                        <li key={ep.id}>
                          <button
                            disabled={active}
                            onClick={async () => {
                              try {
                                await changeRoomEpisode(token, room.id, ep.id);
                                setShowEpisodes(false);
                              } catch (err) {
                                alert(err instanceof Error ? err.message : "Xato");
                              }
                            }}
                            className={`w-full text-left px-2 py-1.5 rounded text-sm flex items-center gap-2 ${
                              active
                                ? "bg-brand-red/20 text-brand-red"
                                : "hover:bg-brand-dark"
                            }`}
                          >
                            <span className="text-xs opacity-60 w-8">E{ep.episode_number}</span>
                            <span className="truncate flex-1">{ep.title}</span>
                            {active && <span className="text-[10px]">▶</span>}
                          </button>
                        </li>
                      );
                    })}
                  </ul>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      {/* Theme picker (premium host only) — 6 preset gradients +
          a "Default" reset. Updates broadcast via WS so all guests
          repaint their background without a reload. */}
      {themePickerOpen && isHost && room && token && (
        <div
          className="fixed inset-0 z-[10000] bg-black/75 flex items-center justify-center p-4"
          onClick={() => setThemePickerOpen(false)}
        >
          <div
            className="bg-brand-card border border-yellow-700/60 rounded-2xl w-full max-w-md overflow-hidden shadow-2xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="px-5 py-4 border-b border-brand-border flex items-center justify-between">
              <div className="flex items-center gap-2">
                <Palette className="w-5 h-5 text-yellow-400" />
                <h3 className="font-semibold text-white">Room mavzusi</h3>
                <span className="text-[10px] bg-yellow-500/20 text-yellow-300 px-1.5 rounded">PREMIUM</span>
              </div>
              <button onClick={() => setThemePickerOpen(false)} className="text-gray-400 hover:text-white">
                <X className="w-5 h-5" />
              </button>
            </div>
            <div className="p-4 grid grid-cols-3 gap-2">
              {ROOM_THEMES.map((t) => (
                <button
                  key={t.label}
                  onClick={async () => {
                    try {
                      await updateRoomTheme(token, room.id, t.from, t.to);
                      setLiveTheme({ from: t.from, to: t.to });
                      setThemePickerOpen(false);
                    } catch (err) {
                      alert(err instanceof Error ? err.message : "Xato");
                    }
                  }}
                  className="aspect-square rounded-lg border border-brand-border hover:border-yellow-500 transition-colors p-2 flex flex-col items-end justify-end text-[10px] text-white/90"
                  style={{ backgroundImage: `linear-gradient(135deg, ${t.from}, ${t.to})` }}
                >
                  {t.label}
                </button>
              ))}
            </div>
          </div>
        </div>
      )}

      {/* Close-room confirm popup (host-only) */}
      {closeConfirm && isHost && room && token && (
        <div
          className="fixed inset-0 z-[10000] bg-black/75 flex items-center justify-center p-4"
          onClick={() => !closeBusy && setCloseConfirm(false)}
        >
          <div
            className="bg-brand-card border border-red-700/60 rounded-2xl w-full max-w-md overflow-hidden shadow-2xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="px-5 py-4 border-b border-brand-border flex items-center justify-between">
              <div className="flex items-center gap-2">
                <XCircle className="w-5 h-5 text-red-400" />
                <h3 className="font-semibold text-white">Roomni yopish</h3>
              </div>
              <button
                onClick={() => !closeBusy && setCloseConfirm(false)}
                className="text-gray-400 hover:text-white"
              >
                <X className="w-5 h-5" />
              </button>
            </div>
            <div className="p-5 space-y-4">
              <p className="text-sm text-gray-300">
                Roomni yopmoqchimisiz? <span className="text-white font-semibold">Barcha a&apos;zolar</span>{" "}
                chiqariladi va siz yangi room ocha olasiz.
              </p>
              <div className="flex gap-2">
                <button
                  disabled={closeBusy}
                  onClick={async () => {
                    setCloseBusy(true);
                    try {
                      await closeWatchRoom(token, room.id);
                      // WS will broadcast room_closed → page navigates
                      // away via existing handler.
                    } catch (err) {
                      alert(err instanceof Error ? err.message : "Xato");
                      setCloseBusy(false);
                    }
                  }}
                  className="flex-1 px-4 py-2.5 bg-red-600 hover:bg-red-700 rounded-lg text-white text-sm font-semibold flex items-center justify-center gap-2 disabled:opacity-60"
                >
                  {closeBusy ? <Loader2 className="w-4 h-4 animate-spin" /> : <XCircle className="w-4 h-4" />}
                  Ha, yopish
                </button>
                <button
                  disabled={closeBusy}
                  onClick={() => setCloseConfirm(false)}
                  className="px-4 py-2.5 bg-brand-dark border border-brand-border rounded-lg text-sm text-gray-300 disabled:opacity-60"
                >
                  Bekor qilish
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Episode-request toast (host-only) */}
      {episodeRequestToast && isHost && (
        <div className="fixed bottom-4 right-4 z-[60] bg-brand-card border border-brand-red rounded-xl p-3 max-w-xs shadow-2xl">
          <p className="text-sm">
            <span className="font-medium">{episodeRequestToast.userName}</span>{" "}
            {episodeRequestToast.reason === "next"
              ? "keyingi epizodga o'tishni so'radi"
              : "epizod o'zgartirishni so'radi"}
          </p>
          <div className="flex gap-2 mt-2">
            {episodeRequestToast.targetEpisodeID && (
              <button
                onClick={async () => {
                  if (!token || !episodeRequestToast.targetEpisodeID) return;
                  try {
                    await changeRoomEpisode(token, room.id, episodeRequestToast.targetEpisodeID);
                    setEpisodeRequestToast(null);
                  } catch {
                    /* ignore */
                  }
                }}
                className="px-3 py-1.5 bg-brand-red text-white rounded text-xs"
              >
                O&apos;tish
              </button>
            )}
            <button
              onClick={() => setEpisodeRequestToast(null)}
              className="px-3 py-1.5 bg-brand-dark border border-brand-border rounded text-xs"
            >
              Yopish
            </button>
          </div>
        </div>
      )}

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
                {/* Native share sheet — Web Share API. Best UX on
                    mobile (Telegram / WhatsApp / SMS bottom sheet)
                    and falls back gracefully on desktops that don't
                    support it (button just isn't rendered). */}
                {typeof navigator !== "undefined" && "share" in navigator && (
                  <button
                    onClick={async () => {
                      try {
                        await navigator.share({
                          title: room?.content_title || "Birga ko'rish",
                          text: `Birga ko'ramiz: ${room?.content_title || "kinoga"}`,
                          url: linkPopup.url,
                        });
                      } catch {
                        /* user cancelled — ignore */
                      }
                    }}
                    className="px-3 py-2 bg-brand-dark border border-brand-border rounded-lg text-sm flex items-center gap-1"
                    title="Ulashish"
                  >
                    <Share2 className="w-4 h-4" />
                  </button>
                )}
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
        @keyframes twitchSlide {
          0% { transform: translateX(20px); opacity: 0; }
          100% { transform: translateX(0); opacity: 1; }
        }
        @keyframes twitchFade {
          0% { opacity: 1; }
          100% { opacity: 0; }
        }
      `}</style>

      {/* Inert imports kept for tree-shaking sanity */}
      <span className="hidden">
        <Smile className="w-0 h-0" />
      </span>
    </div>
  );
}

// FullscreenChatOverlay shows the most recent chat / reaction entries
// floating over the video in fullscreen mode — Twitch-style. Each entry
// auto-fades after ~10s. The list keeps re-bottom-aligned and never
// captures pointer events so the user can still click the video.
function FullscreenChatOverlay({
  chat,
  reactions,
}: {
  chat: RoomChatEntry[];
  reactions: FloatingReaction[];
}) {
  const [visible, setVisible] = useState<Array<{ key: string; node: React.ReactNode; until: number }>>(
    [],
  );
  const lastChatLen = useRef(0);
  const lastReactionLen = useRef(0);

  useEffect(() => {
    const now = Date.now();
    const additions: Array<{ key: string; node: React.ReactNode; until: number }> = [];
    if (chat.length > lastChatLen.current) {
      for (let i = lastChatLen.current; i < chat.length; i++) {
        const c = chat[i];
        additions.push({
          key: `c-${i}-${c.createdAt}`,
          until: now + 10_000,
          node:
            c.userID === "system" ? (
              <span className="italic text-gray-300">• {c.text}</span>
            ) : c.kind === "emoji" ? (
              <span>
                <span className="text-gray-300 text-xs mr-1">{c.userName}:</span>
                <span className="text-xl font-emoji">{c.emoji}</span>
              </span>
            ) : (
              <span>
                <span className="text-gray-300 text-xs mr-1">{c.userName}:</span>
                <span>{c.text}</span>
              </span>
            ),
        });
      }
      lastChatLen.current = chat.length;
    }
    // Reactions are ALSO mirrored into chat (see page.tsx event handler),
    // so we don't double-add them here — but the floating reactions
    // payload is also bridged so the fullscreen viewer sees emoji even on
    // a buggy chat path.
    if (reactions.length > lastReactionLen.current) {
      for (let i = lastReactionLen.current; i < reactions.length; i++) {
        const r = reactions[i];
        additions.push({
          key: `r-${r.id}`,
          until: now + 10_000,
          node: (
            <span>
              <span className="text-gray-300 text-xs mr-1">{r.userName}:</span>
              <span className="text-xl font-emoji">{r.emoji}</span>
            </span>
          ),
        });
      }
      lastReactionLen.current = reactions.length;
    }
    if (additions.length === 0) return;
    setVisible((prev) => [...prev, ...additions].slice(-8));
  }, [chat, reactions]);

  // Sweep expired entries every 500ms.
  useEffect(() => {
    const t = setInterval(() => {
      const now = Date.now();
      setVisible((prev) => prev.filter((e) => e.until > now));
    }, 500);
    return () => clearInterval(t);
  }, []);

  if (visible.length === 0) return null;
  return (
    <div className="flex flex-col gap-1.5 text-white text-sm font-poppins">
      {visible.map((e) => (
        <div
          key={e.key}
          className="bg-black/60 backdrop-blur-sm rounded px-2 py-1.5 max-w-full"
          style={{ animation: "twitchSlide 0.3s ease-out, twitchFade 1.5s ease-out 8.5s forwards" }}
        >
          {e.node}
        </div>
      ))}
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
      <span className="block leading-none drop-shadow-lg font-emoji">{reaction.emoji}</span>
      <span className="block text-[10px] text-white/80 mt-1 text-center whitespace-nowrap">
        {reaction.userName}
      </span>
    </div>
  );
}
