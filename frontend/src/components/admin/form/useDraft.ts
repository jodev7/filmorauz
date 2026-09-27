"use client";

import { useEffect, useRef, useState } from "react";

/**
 * Autosaves a new-content form to localStorage so a refresh or crash doesn't
 * lose typed metadata. Returns the saved draft (if any) until it is restored
 * or discarded.
 */
export function useDraft<T>(key: string, value: T, enabled: boolean) {
  const [pending, setPending] = useState<{ value: T; savedAt: number } | null>(null);
  const ready = useRef(false);

  useEffect(() => {
    if (!enabled) return;
    try {
      const raw = localStorage.getItem(key);
      if (raw) setPending(JSON.parse(raw));
    } catch {
      // storage blocked / corrupt draft
    }
    ready.current = true;
  }, [key, enabled]);

  useEffect(() => {
    if (!enabled || !ready.current || pending) return;
    const t = setTimeout(() => {
      try {
        localStorage.setItem(key, JSON.stringify({ value, savedAt: Date.now() }));
      } catch {
        // ignore
      }
    }, 600);
    return () => clearTimeout(t);
  }, [key, value, enabled, pending]);

  const clear = () => {
    try {
      localStorage.removeItem(key);
    } catch {
      // ignore
    }
    setPending(null);
  };

  return { draft: pending, discardDraft: clear, acceptDraft: () => setPending(null), clearDraft: clear };
}
