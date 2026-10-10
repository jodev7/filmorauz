"use client";

import { createContext, useCallback, useContext, useEffect, useState } from "react";

// Shared "is the inline player open" state for a movie detail page. The play
// button (in the details column) and the player section (full-width below)
// live in different parts of the tree, so they coordinate through context.
type WatchPlayerCtx = {
  open: boolean;
  openPlayer: () => void;
  closePlayer: () => void;
};

const Ctx = createContext<WatchPlayerCtx | null>(null);

export function WatchPlayerProvider({
  children,
  initialOpen = false,
}: {
  children: React.ReactNode;
  initialOpen?: boolean;
}) {
  const [open, setOpen] = useState(initialOpen);
  const openPlayer = useCallback(() => {
    setOpen(true);
    // Defer the scroll until the player has had a chance to mount.
    setTimeout(() => {
      document.getElementById("player")?.scrollIntoView({ behavior: "smooth", block: "start" });
    }, 50);
  }, []);
  const closePlayer = useCallback(() => setOpen(false), []);
  // Deep-link support: ?play=1 auto-opens the player. Read client-side (instead
  // of the server page's searchParams) so the detail page can be statically
  // cached (ISR) rather than force-dynamic.
  useEffect(() => {
    try {
      if (new URLSearchParams(window.location.search).get("play") === "1") openPlayer();
    } catch {
      /* no-op */
    }
  }, [openPlayer]);
  return <Ctx.Provider value={{ open, openPlayer, closePlayer }}>{children}</Ctx.Provider>;
}

export function useWatchPlayer(): WatchPlayerCtx {
  const ctx = useContext(Ctx);
  if (!ctx) {
    // Outside a provider (defensive) — no-op so consumers don't crash.
    return { open: false, openPlayer: () => {}, closePlayer: () => {} };
  }
  return ctx;
}
