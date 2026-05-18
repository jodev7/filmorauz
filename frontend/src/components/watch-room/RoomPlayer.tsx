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
  const controlsTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const clickTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // ── Attach HLS / set src ─────────────────────────────────────────────
  useEffect(() => {
    const v = videoRef.current;
    if (!v || !src) return;
    const isHls = /\.m3u8(\?|$)/i.test(src) || src.includes("/master.m3u8");

    // Safari native HLS path.
    if (!isHls || v.canPlayType("application/vnd.apple.mpegurl")) {
      v.src = src;
      return;
    }
    if (!Hls.isSupported()) {
      v.src = src;
      return;
    }
    const hls = new Hls({ enableWorker: true, startLevel: -1, capLevelToPlayerSize: false });
    hlsRef.current = hls;
    hls.loadSource(src);
    hls.attachMedia(v);
    const buildLevels = (raw: Array<{ height: number; bitrate: number }>) => {
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
    };
    hls.on(Hls.Events.MANIFEST_PARSED, (_, data) => {
      // eslint-disable-next-line no-console
      console.log("[room player] MANIFEST_PARSED levels =", data.levels.map((l) => ({ h: l.height, b: l.bitrate })));
      setQualities(buildLevels(data.levels));
    });
    // LEVEL_LOADED fires for each variant as it parses — refresh the
    // picker in case MANIFEST_PARSED came in with stub levels and the
    // real heights/bitrates land after sub-playlist parsing.
    hls.on(Hls.Events.LEVEL_LOADED, () => {
      if (hls.levels && hls.levels.length > 0) {
        setQualities(buildLevels(hls.levels));
      }
    });
    // Last-resort poll: some streams populate hls.levels after a delay
    // that doesn't line up with either of the events above. Re-read after
    // 1.5s to make sure the picker isn't stuck on "Auto".
    const lateCheck = setTimeout(() => {
      if (hls.levels && hls.levels.length > 0) {
        // eslint-disable-next-line no-console
        console.log(
          "[room player] late-check hls.levels =",
          hls.levels.map((l) => ({ h: l.height, b: l.bitrate })),
        );
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
    const apply = () => {
      if (positionLoadedRef.current) return;
      positionLoadedRef.current = true;
      try {
        const raw = window.localStorage.getItem(`watchroom:pos:${persistKey}`);
        const saved = raw ? Number(raw) : 0;
        if (saved > 5 && (!v.duration || saved < v.duration - 5)) {
          v.currentTime = saved;
        }
      } catch {
        /* localStorage unavailable */
      }
    };
    if (v.readyState >= 1 /* HAVE_METADATA */) apply();
    else v.addEventListener("loadedmetadata", apply, { once: true });
    return () => {
      v.removeEventListener("loadedmetadata", apply);
    };
  }, [isHost, persistKey, src]);

  // Persist host position every 5s while playing.
  useEffect(() => {
    if (!isHost || !persistKey) return;
    const v = videoRef.current;
    if (!v) return;
    const t = setInterval(() => {
      try {
        if (v.currentTime > 5) {
          window.localStorage.setItem(`watchroom:pos:${persistKey}`, String(v.currentTime));
        }
      } catch {
        /* ignore */
      }
    }, 5000);
    return () => clearInterval(t);
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
      if (position !== null && Math.abs(v.currentTime - position) > 1.5) {
        v.currentTime = position;
      }
      if (playing !== null) {
        if (playing && v.paused) v.play().catch(() => undefined);
        else if (!playing && !v.paused) v.pause();
      }
    };
    const v = videoRef.current;
    if (v) {
      v.addEventListener("loadedmetadata", apply);
      v.addEventListener("canplay", apply);
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
    if (!el) return;
    if (document.fullscreenElement) {
      await document.exitFullscreen().catch(() => undefined);
    } else {
      await el.requestFullscreen().catch(() => undefined);
    }
  };

  const setQuality = (idx: number) => {
    const hls = hlsRef.current;
    if (hls) {
      hls.currentLevel = idx;
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
        {/* Progress bar */}
        <div
          className={`relative h-1.5 bg-white/20 rounded-full mb-2 ${isHost ? "cursor-pointer" : "cursor-not-allowed"}`}
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

          {/* Quality */}
          <div className="relative">
            <button
              onClick={() => setShowSettings((v) => !v)}
              className="flex items-center gap-1 text-xs hover:text-brand-red transition-colors"
            >
              <Settings className="w-4 h-4" /> {qualityLabel}
            </button>
            {showSettings && qualities.length > 0 && (
              <div className="absolute right-0 bottom-7 bg-black/90 border border-white/10 rounded-lg overflow-hidden min-w-[100px]">
                {qualities.map((q) => (
                  <button
                    key={q.index}
                    onClick={() => setQuality(q.index)}
                    className={`block w-full px-3 py-1.5 text-left text-xs hover:bg-white/10 ${
                      q.index === selectedLevel ? "text-brand-red font-semibold" : "text-white"
                    }`}
                  >
                    {q.label}
                  </button>
                ))}
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
