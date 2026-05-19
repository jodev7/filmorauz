"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import Hls from "hls.js";
import {
  Play,
  Pause,
  Volume2,
  VolumeX,
  Maximize,
  Minimize,
  Settings,
  Loader2,
  Crown,
  RotateCcw,
} from "lucide-react";
import { useAuth } from "@/lib/auth-context";

type QualityLevel = { index: number; label: string; height: number };

// Hoisted out of the component so both the hls.js path and the
// master-playlist fetch (used as fallback for native-HLS browsers and
// the on-demand "refresh" path) can call it.
function buildFromVariants(raw: Array<{ height: number; bitrate: number }>): QualityLevel[] {
  const out: QualityLevel[] = [{ index: -1, label: "Auto", height: -1 }];
  const seenHeights = new Set<number>();
  const sorted = raw
    .map((l, idx) => ({ idx, height: l.height || 0, bitrate: l.bitrate || 0 }))
    .sort((a, b) => (b.height || b.bitrate) - (a.height || a.bitrate));
  sorted.forEach(({ idx, height, bitrate }) => {
    if (height > 0 && !seenHeights.has(height)) {
      seenHeights.add(height);
      out.push({ index: idx, label: `${height}p`, height });
    } else if (height <= 0 && bitrate > 0) {
      out.push({
        index: idx,
        label: `${Math.round(bitrate / 1000)} kbps`,
        height: bitrate,
      });
    }
  });
  return out;
}

type Props = {
  src: string;
  title?: string;
  posterUrl?: string;
  isHost: boolean;
  isMoviePremium?: boolean;
  // Stable key used to persist the host's playback position to localStorage
  // (so a re-entry resumes where they left off). Typically `${roomID}-host`.
  persistKey?: string;
  // Host events — forwarded to the room hub by the parent.
  onHostPlay?: (positionSeconds: number) => void;
  onHostPause?: (positionSeconds: number) => void;
  onHostSeek?: (positionSeconds: number) => void;
  // Programmatic state sync — parent calls these on guest clients when the
  // hub broadcasts host actions. We accept it via a setter ref instead of a
  // useImperativeHandle so the parent stays declarative.
  registerSync?: (api: { setPosition: (sec: number) => void; setPlaying: (p: boolean) => void }) => void;
  // Slot for the Twitch-style chat overlay rendered inside fullscreen.
  fullscreenOverlay?: React.ReactNode;
};

export default function RoomPlayer({
  src,
  title,
  posterUrl,
  isHost,
  isMoviePremium,
  persistKey,
  onHostPlay,
  onHostPause,
  onHostSeek,
  registerSync,
  fullscreenOverlay,
}: Props) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const hlsRef = useRef<Hls | null>(null);
  const { user } = useAuth();

  const [playing, setPlaying] = useState(false);
  const [muted, setMuted] = useState(false);
  const [volume, setVolume] = useState(1);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [buffering, setBuffering] = useState(false);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [showSettings, setShowSettings] = useState(false);
  const [qualities, setQualities] = useState<QualityLevel[]>([]);
  const [selectedLevel, setSelectedLevel] = useState<number>(-1); // -1 = auto
  const [controlsVisible, setControlsVisible] = useState(true);
  // Progress-bar hover state: fractional position (0..1) and pixel-x for
  // the tooltip. Null when the cursor isn't over the bar.
  const [scrubHover, setScrubHover] = useState<{ frac: number; x: number } | null>(null);
  const controlsTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const clickTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // ── Attach HLS / set src ─────────────────────────────────────────────
  // Helper: fetch the master playlist URL and parse #EXT-X-STREAM-INF
  // lines into quality levels. Used both from the main hls effect and
  // on-demand when the user opens an empty quality dropdown.
  const parseMasterPlaylist = async (url: string) => {
    try {
      const r = await fetch(url, { credentials: "omit" });
      if (!r.ok) return [] as Array<{ height: number; bitrate: number }>;
      const text = await r.text();
      const variants: Array<{ height: number; bitrate: number }> = [];
      const re = /#EXT-X-STREAM-INF:([^\n]+)/g;
      let mm: RegExpExecArray | null;
      while ((mm = re.exec(text)) !== null) {
        const attrs = mm[1];
        const resMatch = /RESOLUTION=\d+x(\d+)/.exec(attrs);
        const bwMatch = /BANDWIDTH=(\d+)/.exec(attrs);
        variants.push({
          height: resMatch ? Number(resMatch[1]) : 0,
          bitrate: bwMatch ? Number(bwMatch[1]) : 0,
        });
      }
      return variants;
    } catch {
      return [] as Array<{ height: number; bitrate: number }>;
    }
  };
  // Stable reference to the master playlist URL so the dropdown can
  // re-trigger a fetch later. Mirrors `src` 1:1.
  const lastSrcRef = useRef<string>("");
  // Tracks whether the current picker entries came from hls.js (whose
  // indices are valid `hls.currentLevel` arguments) or from the manual
  // master-playlist parse (whose indices are master-order positions
  // that may NOT match hls.levels). Click handling needs to know so it
  // can map height → hls.levels index instead of trusting the raw idx.
  const qualitiesSourceRef = useRef<"hls" | "manual" | null>(null);

  useEffect(() => {
    const v = videoRef.current;
    if (!v || !src) return;
    lastSrcRef.current = src;
    qualitiesSourceRef.current = null;
    const isHls = /\.m3u8(\?|$)/i.test(src) || src.includes("/master.m3u8");

    // ALWAYS try to parse the master playlist for the quality picker —
    // even on Safari (native HLS) where hls.js isn't used. Without this
    // Safari users would never see a populated picker.
    if (isHls) {
      void parseMasterPlaylist(src).then((variants) => {
        // Only populate from the manual parse if hls.js hasn't already
        // taken over. Manual indices are master-order positions; using
        // them as `hls.currentLevel` arguments can land on the wrong
        // resolution because hls.js reorders levels internally.
        if (variants.length > 0 && qualitiesSourceRef.current !== "hls") {
          qualitiesSourceRef.current = "manual";
          setQualities(buildFromVariants(variants));
        }
      });
    }

    // Non-HLS source — just hand the URL to the video element.
    if (!isHls) {
      v.src = src;
      return;
    }
    // PREFER hls.js whenever it's supported, even on browsers (like
    // some Chromium / Edge builds) that happen to advertise native
    // HLS playback. Native playback works but gives us no API to
    // switch quality, so the picker is dead. hls.js gives us both
    // playback and level switching. Native HLS is only used as a
    // true last resort (Safari without MSE workarounds).
    if (!Hls.isSupported()) {
      v.src = src;
      return;
    }
    const hls = new Hls({ enableWorker: true, startLevel: -1, capLevelToPlayerSize: false });
    hlsRef.current = hls;
    hls.loadSource(src);
    hls.attachMedia(v);
    const buildLevels = buildFromVariants;
    // Hls-sourced events ALWAYS overwrite the picker because their
    // indices map 1:1 to `hls.currentLevel`. Manual-fetch entries are
    // a fallback only used when hls.js hasn't populated yet.
    const setFromHls = (next: QualityLevel[]) => {
      if (next.length <= 1) return; // ignore stub events
      qualitiesSourceRef.current = "hls";
      setQualities(next);
    };
    hls.on(Hls.Events.MANIFEST_PARSED, (_, data) => {
      setFromHls(buildLevels(data.levels));
    });
    hls.on(Hls.Events.LEVEL_LOADED, () => {
      if (hls.levels && hls.levels.length > 0) {
        setFromHls(buildLevels(hls.levels));
      }
    });
    hls.on(Hls.Events.LEVELS_UPDATED, (_, data) => {
      if (data.levels.length > 0) setFromHls(buildLevels(data.levels));
    });
    // (Manual master.m3u8 parse already kicked off above, runs in
    // parallel with hls.js to populate the picker as fast as possible.)
    const lateCheck = setTimeout(() => {
      if (hls.levels && hls.levels.length > 0) {
        setQualities((curr) => (curr.length > 1 ? curr : buildLevels(hls.levels)));
      }
    }, 1500);
    hls.on(Hls.Events.LEVEL_SWITCHED, (_, data) => {
      // Keep the picker label in sync when auto-bitrate switches.
      if (selectedLevel === -1) {
        // Auto mode — don't override the user's selection.
        return;
      }
      setSelectedLevel(data.level);
    });
    return () => {
      clearTimeout(lateCheck);
      hls.destroy();
      hlsRef.current = null;
    };
  }, [src]);

  // ── Host position persistence: resume on re-entry ──────────────────
  // Wait for metadata before applying the saved position so the seek
  // actually lands (currentTime set before duration is known is dropped
  // by some browsers). Only the host benefits — guests follow host state.
  const positionLoadedRef = useRef(false);
  useEffect(() => {
    if (!isHost || !persistKey) return;
    const v = videoRef.current;
    if (!v) return;
    // Critical: reset the "already applied" flag whenever src changes.
    // Without this reset the restore only fires the very first time the
    // component sees a non-empty src, so a host who navigated away and
    // back never resumes from their saved position.
    positionLoadedRef.current = false;
    const apply = () => {
      if (positionLoadedRef.current) return;
      try {
        const raw = window.localStorage.getItem(`watchroom:pos:${persistKey}`);
        const saved = raw ? Number(raw) : 0;
        // Lowered the threshold from 5s → 2s so even a quick scrub
        // before navigating away is restored.
        if (saved > 2 && (!v.duration || saved < v.duration - 2)) {
          v.currentTime = saved;
          positionLoadedRef.current = true;
        } else if (v.duration > 0) {
          // Duration known but no usable saved value — mark loaded so
          // the canplay listener doesn't keep re-trying.
          positionLoadedRef.current = true;
        }
      } catch {
        /* localStorage unavailable */
      }
    };
    if (v.readyState >= 1 /* HAVE_METADATA */) apply();
    v.addEventListener("loadedmetadata", apply);
    // Belt-and-braces: canplay fires later than loadedmetadata on slow
    // connections and is the latest point at which we can still seek
    // reliably before the user notices playback starting at 0.
    v.addEventListener("canplay", apply);
    return () => {
      v.removeEventListener("loadedmetadata", apply);
      v.removeEventListener("canplay", apply);
    };
  }, [isHost, persistKey, src]);

  // Persist host position aggressively: every 3s, on every pause, and on
  // page unload. Without these the timer-only path could miss the most
  // recent position when a host clicked away mid-scene.
  useEffect(() => {
    if (!isHost || !persistKey) return;
    const v = videoRef.current;
    if (!v) return;
    const key = `watchroom:pos:${persistKey}`;
    const save = () => {
      try {
        if (v.currentTime > 2) {
          window.localStorage.setItem(key, String(v.currentTime));
        }
      } catch {
        /* ignore */
      }
    };
    const t = setInterval(save, 3000);
    v.addEventListener("pause", save);
    v.addEventListener("seeked", save);
    window.addEventListener("beforeunload", save);
    return () => {
      clearInterval(t);
      v.removeEventListener("pause", save);
      v.removeEventListener("seeked", save);
      window.removeEventListener("beforeunload", save);
      save(); // one last write on component unmount (route change)
    };
  }, [isHost, persistKey]);

  // ── Register sync API for the parent so it can drive guests ──────────
  // Queues the latest target while the video is still loading metadata;
  // applies it the moment readyState says we have at least HAVE_METADATA.
  // Without this, guests on a slow connection got stuck in "Buffering…"
  // because every host action tried to seek before the manifest was ready.
  const pendingSyncRef = useRef<{ position: number | null; playing: boolean | null }>({
    position: null,
    playing: null,
  });
  useEffect(() => {
    if (!registerSync) return;
    const apply = () => {
      const v = videoRef.current;
      if (!v || v.readyState < 1) return;
      const { position, playing } = pendingSyncRef.current;
      // 0.7s drift threshold — was 1.5s; guests reported a multi-second
      // lag between their playhead and the host's. Tighter snap matches
      // the heartbeat (now 2s) so noticeable drift is corrected quickly.
      if (position !== null && Math.abs(v.currentTime - position) > 0.7) {
        v.currentTime = position;
      }
      if (playing !== null) {
        if (playing && v.paused) v.play().catch(() => undefined);
        else if (!playing && !v.paused) v.pause();
      }
    };
    const v = videoRef.current;
    if (v) {
      // Re-apply on every readiness milestone — without seeked /
      // playing listeners the guest could end up with a pending sync
      // that never gets re-tried after the previous seek settled. The
      // result was guests "freezing" after a frantic host scrub: the
      // first seek lost its target while buffering and no further
      // event triggered a catch-up to the new (debounced) target.
      v.addEventListener("loadedmetadata", apply);
      v.addEventListener("canplay", apply);
      v.addEventListener("seeked", apply);
      v.addEventListener("playing", apply);
    }
    registerSync({
      setPosition: (sec: number) => {
        pendingSyncRef.current.position = sec;
        apply();
      },
      setPlaying: (p: boolean) => {
        pendingSyncRef.current.playing = p;
        apply();
      },
    });
    return () => {
      if (v) {
        v.removeEventListener("loadedmetadata", apply);
        v.removeEventListener("canplay", apply);
        v.removeEventListener("seeked", apply);
        v.removeEventListener("playing", apply);
      }
    };
  }, [registerSync, src]);

  // ── Local video element listeners (always wired; host broadcasts) ───
  useEffect(() => {
    const v = videoRef.current;
    if (!v) return;
    const onPlay = () => {
      setPlaying(true);
      setBuffering(false);
      if (isHost) onHostPlay?.(v.currentTime);
    };
    const onPause = () => {
      setPlaying(false);
      if (isHost) onHostPause?.(v.currentTime);
    };
    const onSeeked = () => {
      if (isHost) onHostSeek?.(v.currentTime);
    };
    const onTime = () => setCurrentTime(v.currentTime);
    const onDur = () => setDuration(v.duration || 0);
    const onWaiting = () => setBuffering(true);
    const onPlaying = () => setBuffering(false);
    v.addEventListener("play", onPlay);
    v.addEventListener("pause", onPause);
    v.addEventListener("seeked", onSeeked);
    v.addEventListener("timeupdate", onTime);
    v.addEventListener("durationchange", onDur);
    v.addEventListener("waiting", onWaiting);
    v.addEventListener("playing", onPlaying);
    return () => {
      v.removeEventListener("play", onPlay);
      v.removeEventListener("pause", onPause);
      v.removeEventListener("seeked", onSeeked);
      v.removeEventListener("timeupdate", onTime);
      v.removeEventListener("durationchange", onDur);
      v.removeEventListener("waiting", onWaiting);
      v.removeEventListener("playing", onPlaying);
    };
  }, [isHost, onHostPlay, onHostPause, onHostSeek]);

  // ── Fullscreen listener ─────────────────────────────────────────────
  useEffect(() => {
    const onFs = () => setIsFullscreen(!!document.fullscreenElement);
    document.addEventListener("fullscreenchange", onFs);
    return () => document.removeEventListener("fullscreenchange", onFs);
  }, []);

  // ── Auto-hide controls after 2.5s of mouse idleness ─────────────────
  useEffect(() => {
    if (controlsTimer.current) clearTimeout(controlsTimer.current);
    if (!controlsVisible) return;
    controlsTimer.current = setTimeout(() => {
      if (playing) setControlsVisible(false);
    }, 2500);
    return () => {
      if (controlsTimer.current) clearTimeout(controlsTimer.current);
    };
  }, [controlsVisible, playing]);

  const showControls = () => setControlsVisible(true);

  // ── Actions ─────────────────────────────────────────────────────────
  const togglePlay = () => {
    if (!isHost) return; // guests can't drive playback
    const v = videoRef.current;
    if (!v) return;
    if (v.paused) v.play().catch(() => undefined);
    else v.pause();
  };

  const onScrubChange = (frac: number) => {
    if (!isHost) return;
    const v = videoRef.current;
    if (!v || !duration) return;
    v.currentTime = duration * frac;
  };

  const toggleMute = () => {
    const v = videoRef.current;
    if (!v) return;
    v.muted = !v.muted;
    setMuted(v.muted);
  };

  const onVolumeChange = (val: number) => {
    const v = videoRef.current;
    if (!v) return;
    v.volume = val;
    setVolume(val);
    if (val > 0 && v.muted) {
      v.muted = false;
      setMuted(false);
    }
  };

  const toggleFullscreen = async () => {
    const el = containerRef.current;
    const v = videoRef.current as
      | (HTMLVideoElement & {
          webkitEnterFullscreen?: () => void;
          webkitExitFullscreen?: () => void;
          webkitDisplayingFullscreen?: boolean;
        })
      | null;
    // iOS Safari doesn't support Fullscreen API on container divs —
    // only the <video> element itself can go fullscreen via the legacy
    // webkit API. Use it when available; fall back to the standard
    // container API on desktop / Android Chrome.
    if (v && typeof v.webkitEnterFullscreen === "function") {
      if (v.webkitDisplayingFullscreen) {
        v.webkitExitFullscreen?.();
      } else {
        v.webkitEnterFullscreen();
      }
      return;
    }
    if (!el) return;
    if (document.fullscreenElement) {
      await document.exitFullscreen().catch(() => undefined);
    } else {
      // Some Android browsers also need to fullscreen the <video>
      // directly when the container request is denied.
      try {
        await el.requestFullscreen();
      } catch {
        if (v && "requestFullscreen" in v) {
          await (v as HTMLVideoElement).requestFullscreen().catch(() => undefined);
        }
      }
    }
  };

  const setQuality = (idx: number) => {
    const hls = hlsRef.current;
    if (hls) {
      if (idx === -1) {
        hls.currentLevel = -1;
        hls.loadLevel = -1;
      } else {
        const target = qualities.find((q) => q.index === idx);
        let resolvedIdx = -1;
        if (target && hls.levels && hls.levels.length > 0) {
          for (let i = 0; i < hls.levels.length; i++) {
            if (hls.levels[i].height === target.height) {
              resolvedIdx = i;
              break;
            }
          }
          if (resolvedIdx < 0) {
            for (let i = 0; i < hls.levels.length; i++) {
              if (hls.levels[i].bitrate === target.height) {
                resolvedIdx = i;
                break;
              }
            }
          }
          if (resolvedIdx < 0 && idx >= 0 && idx < hls.levels.length) {
            resolvedIdx = idx;
          }
        }
        if (resolvedIdx >= 0) {
          // Just set currentLevel — in hls.js v1.x this is the canonical
          // "switch right now" API and it handles flushing the forward
          // buffer + reloading from the new variant on its own. The
          // previous approach (currentLevel + loadLevel + nextLoadLevel
          // + manual BUFFER_FLUSHING + seek nudge) was fighting itself:
          // the seek would land in an already-buffered range and skip
          // the just-issued flush, so the switch sometimes "stuck" at
          // the old quality. Trusting the single API is reliable.
          hls.currentLevel = resolvedIdx;
        }
      }
    }
    setSelectedLevel(idx);
    setShowSettings(false);
  };

  const qualityLabel = useMemo(
    () => qualities.find((q) => q.index === selectedLevel)?.label ?? "Auto",
    [qualities, selectedLevel],
  );

  const progressPct = duration > 0 ? (currentTime / duration) * 100 : 0;

  const formatTime = (t: number) => {
    if (!isFinite(t) || t < 0) return "0:00";
    const h = Math.floor(t / 3600);
    const m = Math.floor((t % 3600) / 60);
    const s = Math.floor(t % 60);
    return h > 0
      ? `${h}:${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`
      : `${m}:${String(s).padStart(2, "0")}`;
  };

  return (
    <div
      ref={containerRef}
      className={`relative w-full h-full bg-black select-none group ${
        isMoviePremium ? "ring-2 ring-yellow-500/40" : ""
      }`}
      onMouseMove={showControls}
      onMouseLeave={() => playing && setControlsVisible(false)}
      onClick={() => {
        // Wait 220ms — if a 2nd click comes in (dblclick) we cancel the
        // play-toggle and let onDoubleClick fire instead.
        if (clickTimerRef.current) clearTimeout(clickTimerRef.current);
        clickTimerRef.current = setTimeout(() => {
          togglePlay();
          clickTimerRef.current = null;
        }, 220);
      }}
      onDoubleClick={(e) => {
        e.stopPropagation();
        if (clickTimerRef.current) {
          clearTimeout(clickTimerRef.current);
          clickTimerRef.current = null;
        }
        toggleFullscreen();
      }}
    >
      <video
        ref={videoRef}
        poster={posterUrl}
        playsInline
        webkit-playsinline="true"
        x5-playsinline="true"
        className="w-full h-full"
      />

      {/* Buffering spinner */}
      {buffering && (
        <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
          <Loader2 className="w-10 h-10 animate-spin text-white/80" />
        </div>
      )}

      {/* Title + premium badge (top) */}
      <div
        className={`absolute top-0 left-0 right-0 p-3 bg-gradient-to-b from-black/70 to-transparent transition-opacity ${
          controlsVisible ? "opacity-100" : "opacity-0"
        }`}
      >
        <div className="flex items-center gap-2 text-white">
          {isMoviePremium && (
            <span className="flex items-center gap-1 text-[10px] bg-yellow-500/30 text-yellow-200 px-1.5 py-0.5 rounded">
              <Crown className="w-3 h-3" /> PREMIUM
            </span>
          )}
          {user && (user as { isPremium?: boolean }).isPremium && (
            <span className="text-[10px] bg-yellow-500/30 text-yellow-200 px-1.5 py-0.5 rounded flex items-center gap-1">
              <Crown className="w-3 h-3" /> PRO
            </span>
          )}
          <span className="text-sm font-medium truncate flex-1">{title}</span>
          {!isHost && (
            <span className="text-[10px] bg-black/40 px-2 py-0.5 rounded">Mehmon</span>
          )}
        </div>
      </div>

      {/* Floating big-play overlay when paused (host only) */}
      {!playing && isHost && (
        <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
          <div className="bg-black/50 rounded-full p-4">
            <Play className="w-10 h-10 text-white" />
          </div>
        </div>
      )}

      {/* Bottom controls */}
      <div
        className={`absolute bottom-0 left-0 right-0 p-2 sm:p-3 bg-gradient-to-t from-black/80 to-transparent transition-opacity ${
          controlsVisible ? "opacity-100" : "opacity-0 pointer-events-none"
        }`}
        onClick={(e) => e.stopPropagation()}
      >
        {/* Progress bar — onMouseMove drives the hover dot + time
            tooltip so users can see where they're about to seek to.
            Guests get the preview too (they can't actually seek but
            knowing the time helps for chat coordination). */}
        <div
          className={`relative h-1.5 bg-white/20 rounded-full mb-2 group/scrub ${isHost ? "cursor-pointer" : "cursor-not-allowed"}`}
          onMouseMove={(e) => {
            const rect = e.currentTarget.getBoundingClientRect();
            const frac = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
            setScrubHover({ frac, x: e.clientX - rect.left });
          }}
          onMouseLeave={() => setScrubHover(null)}
          onClick={(e) => {
            if (!isHost) return;
            const rect = e.currentTarget.getBoundingClientRect();
            const frac = (e.clientX - rect.left) / rect.width;
            onScrubChange(Math.max(0, Math.min(1, frac)));
          }}
        >
          <div
            className="absolute inset-y-0 left-0 bg-brand-red rounded-full"
            style={{ width: `${progressPct}%` }}
          />
          {/* Hover dot + time tooltip. Shown while the cursor is over
              the bar. Pointer-events disabled so it doesn't swallow
              the click handler on the bar itself. */}
          {scrubHover && duration > 0 && (
            <>
              <div
                className="absolute top-1/2 -translate-y-1/2 -translate-x-1/2 w-3 h-3 rounded-full bg-white border-2 border-brand-red shadow-md pointer-events-none"
                style={{ left: `${scrubHover.x}px` }}
              />
              <div
                className="absolute -top-7 -translate-x-1/2 px-1.5 py-0.5 text-[10px] tabular-nums bg-black/90 text-white rounded pointer-events-none whitespace-nowrap"
                style={{ left: `${scrubHover.x}px` }}
              >
                {formatTime(duration * scrubHover.frac)}
              </div>
            </>
          )}
          {/* Always-on playhead dot at the current position (only when
              not hovering, so it doesn't double up with the hover dot). */}
          {!scrubHover && progressPct > 0 && (
            <div
              className="absolute top-1/2 -translate-y-1/2 -translate-x-1/2 w-2.5 h-2.5 rounded-full bg-white opacity-0 group-hover/scrub:opacity-100 transition-opacity pointer-events-none"
              style={{ left: `${progressPct}%` }}
            />
          )}
        </div>

        <div className="flex items-center gap-2 sm:gap-3 text-white">
          {/* Play/Pause (host only) */}
          {isHost ? (
            <button onClick={togglePlay} className="hover:text-brand-red transition-colors">
              {playing ? <Pause className="w-5 h-5" /> : <Play className="w-5 h-5" />}
            </button>
          ) : (
            <span className="text-xs opacity-70 px-1">
              {playing ? <Pause className="w-4 h-4 inline" /> : <Play className="w-4 h-4 inline" />}
            </span>
          )}

          {/* Volume */}
          <button onClick={toggleMute} className="hover:text-brand-red transition-colors">
            {muted || volume === 0 ? <VolumeX className="w-5 h-5" /> : <Volume2 className="w-5 h-5" />}
          </button>
          <input
            type="range"
            min={0}
            max={1}
            step={0.01}
            value={muted ? 0 : volume}
            onChange={(e) => onVolumeChange(Number(e.target.value))}
            className="w-14 sm:w-24 accent-brand-red"
          />

          {/* Time */}
          <span className="text-xs tabular-nums">
            {formatTime(currentTime)} / {formatTime(duration)}
          </span>

          <div className="flex-1" />

          {/* Quality — onPointerDown also stops propagation so the
              container-level click timer doesn't fire and the dropdown
              isn't immediately closed by a phantom outside-click. */}
          <div className="relative">
            <button
              onPointerDown={(e) => e.stopPropagation()}
              onClick={(e) => {
                e.stopPropagation();
                const nextOpen = !showSettings;
                setShowSettings(nextOpen);
                // On-demand refresh: if the picker is about to open with
                // only "Auto" (or empty), re-fetch the master playlist
                // and populate. This is the last line of defense for
                // streams where the initial parse silently dropped the
                // variants (CORS hiccup, hls.js stub levels, etc.).
                if (nextOpen && qualities.length <= 1 && lastSrcRef.current) {
                  void parseMasterPlaylist(lastSrcRef.current).then((variants) => {
                    if (variants.length > 0 && qualitiesSourceRef.current !== "hls") {
                      qualitiesSourceRef.current = "manual";
                      setQualities(buildFromVariants(variants));
                    }
                  });
                }
              }}
              className="flex items-center gap-1 text-xs hover:text-brand-red transition-colors px-1 py-0.5"
              type="button"
            >
              <Settings className="w-4 h-4" /> {qualityLabel}
            </button>
            {showSettings && (
              <div
                onPointerDown={(e) => e.stopPropagation()}
                onClick={(e) => e.stopPropagation()}
                className="absolute right-0 bottom-8 bg-black/95 border border-white/10 rounded-lg overflow-hidden min-w-[120px] z-50 shadow-xl"
              >
                {qualities.map((q) => (
                  <button
                    key={q.index}
                    type="button"
                    onPointerDown={(e) => e.stopPropagation()}
                    onClick={(e) => {
                      e.stopPropagation();
                      setQuality(q.index);
                    }}
                    className={`block w-full px-3 py-2 text-left text-xs hover:bg-white/10 ${
                      q.index === selectedLevel ? "text-brand-red font-semibold" : "text-white"
                    }`}
                  >
                    {q.label}
                  </button>
                ))}
                {qualities.length <= 1 && (
                  <div className="px-3 py-2 text-[10px] text-gray-500 border-t border-white/10">
                    Bu video uchun boshqa sifat mavjud emas
                  </div>
                )}
              </div>
            )}
          </div>

          {/* Restart (host) */}
          {isHost && (
            <button
              onClick={() => {
                const v = videoRef.current;
                if (v) v.currentTime = 0;
              }}
              className="hover:text-brand-red transition-colors"
              title="Boshidan"
            >
              <RotateCcw className="w-4 h-4" />
            </button>
          )}

          {/* Fullscreen */}
          <button onClick={toggleFullscreen} className="hover:text-brand-red transition-colors">
            {isFullscreen ? <Minimize className="w-5 h-5" /> : <Maximize className="w-5 h-5" />}
          </button>
        </div>
      </div>

      {/* Fullscreen-only chat overlay slot */}
      {isFullscreen && fullscreenOverlay && (
        <div className="absolute top-12 right-3 z-10 pointer-events-none w-72 max-w-[50%]">
          {fullscreenOverlay}
        </div>
      )}
    </div>
  );
}
