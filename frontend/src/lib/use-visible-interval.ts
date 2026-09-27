"use client";

import { useEffect } from "react";

/**
 * Runs `callback` immediately and then every `delayMs`, but only while the
 * browser tab is visible. When the tab is hidden the timer stops; when it
 * becomes visible again the callback runs once right away and polling resumes.
 *
 * The callback receives `isActive()` — it returns false once the effect has
 * been torn down (unmount or dependency change), so async callers can drop
 * stale responses instead of overwriting newer state.
 *
 * Pass a stable (useCallback-wrapped) callback: a new identity restarts the
 * polling and triggers an immediate run, which is what you want when e.g. the
 * page number changes.
 */
export function useVisibleInterval(
  callback: (isActive: () => boolean) => void,
  delayMs: number,
  enabled = true,
) {
  useEffect(() => {
    if (!enabled) return;

    let active = true;
    let timer: ReturnType<typeof setInterval> | null = null;
    const isActive = () => active;
    const tick = () => callback(isActive);

    const start = () => {
      if (timer === null) timer = setInterval(tick, delayMs);
    };
    const stop = () => {
      if (timer !== null) {
        clearInterval(timer);
        timer = null;
      }
    };
    const onVisibilityChange = () => {
      if (document.visibilityState === "visible") {
        tick();
        start();
      } else {
        stop();
      }
    };

    tick();
    if (document.visibilityState === "visible") start();
    document.addEventListener("visibilitychange", onVisibilityChange);

    return () => {
      active = false;
      stop();
      document.removeEventListener("visibilitychange", onVisibilityChange);
    };
  }, [callback, delayMs, enabled]);
}
