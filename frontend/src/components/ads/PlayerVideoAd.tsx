"use client";

import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { Ad, recordAdClick } from "@/lib/api";
import { normalizeMediaUrl } from "@/lib/image-utils";

/** Shared pre-/mid-roll creative. Countdown follows playback, not buffering. */
export default function PlayerVideoAd({ ad, url, onComplete, volume = 1 }: {
  ad: Ad; url: string; onComplete: () => void; volume?: number;
}) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const completeRef = useRef(onComplete);
  completeRef.current = onComplete;
  const completed = useRef(false);
  const [elapsed, setElapsed] = useState(0);
  const [blocked, setBlocked] = useState(false);
  const [muted, setMuted] = useState(false);
  const finish = () => {
    if (completed.current) return;
    completed.current = true;
    videoRef.current?.pause();
    completeRef.current();
  };

  useLayoutEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    video.volume = volume > 0 ? Math.min(1, volume) : 1;
    video.muted = false;
    // A pre-roll mounted by the Play click can start with audio while that
    // gesture is still active. If the ad arrives later, ask for a new click.
    void video.play().catch((error: DOMException) => {
      if (error.name !== "AbortError") setBlocked(true);
    });
  }, [url]);

  useEffect(() => {
    const video = videoRef.current;
    if (video) video.volume = volume > 0 ? Math.min(1, volume) : 1;
  }, [volume]);

  useEffect(() => {
    // Broken/stalled media must never trap the viewer behind the overlay.
    let previousTime = -1;
    let stalledSeconds = 0;
    const timer = setInterval(() => {
      const video = videoRef.current;
      if (!video || blocked) return;
      stalledSeconds = video.currentTime === previousTime ? stalledSeconds + 1 : 0;
      previousTime = video.currentTime;
      if (stalledSeconds >= 20 && !completed.current) {
        completed.current = true;
        video.pause();
        completeRef.current();
      }
    }, 1000);
    return () => clearInterval(timer);
  }, [blocked]);

  return (
    <div className="absolute inset-0 z-30 overflow-hidden bg-black" onClick={(e) => e.stopPropagation()}>
      <video ref={videoRef} src={normalizeMediaUrl(url, "")} playsInline
        className="h-full w-full object-contain"
        onTimeUpdate={(e) => {
          setElapsed(e.currentTarget.currentTime);
          if (e.currentTarget.currentTime >= 65) finish();
        }}
        onPlaying={() => setBlocked(false)}
        onVolumeChange={(e) => setMuted(e.currentTarget.muted || e.currentTarget.volume === 0)}
        onEnded={finish} onError={finish}
      />
      <span className="absolute left-3 top-3 rounded bg-black/70 px-2 py-1 text-xs text-white">Reklama</span>
      <button type="button" disabled={elapsed < 15} onClick={finish}
        className="absolute right-3 top-3 rounded-full bg-black/80 px-3 py-2 text-sm text-white disabled:cursor-default" aria-label="Reklamani yopish">
        {elapsed >= 15 ? "✕ Yopish" : `Yopish: ${Math.ceil(15 - elapsed)} s`}
      </button>
      {blocked && <button type="button" className="absolute inset-0 m-auto h-fit w-fit rounded-xl bg-white px-4 py-3 font-semibold text-black"
        onClick={() => {
          const video = videoRef.current;
          if (!video) return;
          video.muted = false;
          video.volume = volume > 0 ? Math.min(1, volume) : 1;
          // Called directly in the click handler to retain user activation.
          void video.play().then(() => setBlocked(false)).catch(() => setBlocked(true));
        }}>▶ Reklamani ovoz bilan boshlash</button>}
      <button type="button" className="absolute bottom-3 left-3 rounded bg-black/80 px-3 py-2 text-xs text-white"
        onClick={() => {
          const video = videoRef.current;
          if (!video) return;
          video.muted = !video.muted;
        }}>{muted ? "Ovozni yoqish" : "Ovozni o'chirish"}</button>
      <a href={ad.target_url} target="_blank" rel="noopener noreferrer"
        onClick={() => { void recordAdClick(ad.id, "player").catch(() => {}); }}
        className="absolute bottom-3 right-3 inline-flex items-center gap-1 rounded-full bg-orange-500 px-4 py-2 text-sm font-semibold text-white shadow-lg shadow-black/40 hover:bg-orange-400">{ad.call_to_action || "Saytga o'tish"} <span aria-hidden="true">→</span></a>
    </div>
  );
}
