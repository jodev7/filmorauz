"use client";

import { useEffect } from "react";
import { useAuth } from "@/lib/auth-context";
import { getPresenceActivity, PRESENCE_ACTIVITY_EVENT } from "@/lib/presence-activity";

// NEXT_PUBLIC_API_URL already includes the /api prefix (see lib/api.ts), so
// only the route path is appended here. The previous version double-prefixed
// "/api/api/presence/heartbeat" and returned 404 in production.
const API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080/api";
const SESSION_KEY = "filmorauz_session_id";
const HEARTBEAT_MS = 45_000;

function getOrCreateSessionId(): string {
  if (typeof window === "undefined") return "";
  let id = localStorage.getItem(SESSION_KEY);
  if (!id) {
    id =
      typeof crypto !== "undefined" && "randomUUID" in crypto
        ? crypto.randomUUID()
        : `anon-${Date.now()}-${Math.random().toString(36).slice(2)}`;
    localStorage.setItem(SESSION_KEY, id);
  }
  return id;
}

/**
 * PresencePinger: posts a heartbeat to /api/presence/heartbeat every ~45s
 * so the admin dashboard can show "online now" counts (authed + anonymous).
 * Mounted once at the root layout — runs for every page, every visitor.
 */
export default function PresencePinger() {
  const { token } = useAuth();

  useEffect(() => {
    const sessionId = getOrCreateSessionId();
    if (!sessionId) return;

    const ping = () => {
      if (typeof document !== "undefined" && document.hidden) return;
      const headers: Record<string, string> = { "Content-Type": "application/json" };
      if (token) headers["Authorization"] = `Bearer ${token}`;
      fetch(`${API_URL}/presence/heartbeat`, {
        method: "POST",
        headers,
        // activity tells the admin panel which movie/episode this tab has open;
        // null on every other page, which clears the previous report.
        body: JSON.stringify({
          session_id: sessionId,
          activity: getPresenceActivity(),
        }),
        keepalive: true,
      }).catch(() => {});
    };

    ping();
    const t = setInterval(ping, HEARTBEAT_MS);
    const onVisible = () => {
      if (!document.hidden) ping();
    };
    document.addEventListener("visibilitychange", onVisible);
    window.addEventListener(PRESENCE_ACTIVITY_EVENT, ping);
    return () => {
      clearInterval(t);
      document.removeEventListener("visibilitychange", onVisible);
      window.removeEventListener(PRESENCE_ACTIVITY_EVENT, ping);
    };
  }, [token]);

  return null;
}
