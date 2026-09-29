"use client";

import { useEffect, useRef, useState } from "react";
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

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    video.volume = volume > 0 ? Math.min(1, volume) : 1;
    video.muted = false;
    // Do not silently fall back to muted autoplay: let the viewer enable sound.
    void video.play().catch(() => setBlocked(true));
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
        onPlaying={() => setBlocked(false)} onEnded={finish} onError={finish}
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
          setMuted(false);
          void video.play().catch(() => setBlocked(true));
        }}>▶ Reklamani ovoz bilan boshlash</button>}
      <button type="button" className="absolute bottom-3 left-3 rounded bg-black/80 px-3 py-2 text-xs text-white"
        onClick={() => {
          const video = videoRef.current;
          if (!video) return;
          video.muted = !video.muted;
          setMuted(video.muted);
        }}>{muted ? "Ovozni yoqish" : "Ovozni o'chirish"}</button>
      <a href={ad.target_url} target="_blank" rel="noopener noreferrer"
        onClick={() => { void recordAdClick(ad.id).catch(() => {}); }}
        className="absolute bottom-3 right-3 rounded bg-white px-3 py-2 text-xs font-semibold text-black">{ad.call_to_action || "Batafsil"}</a>
    </div>
  );
}
