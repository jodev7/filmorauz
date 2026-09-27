"use client";

import { useEffect } from "react";
import { useAuth } from "@/lib/auth-context";
import { claimReferral } from "@/lib/api";

const STORAGE_KEY = "filmora_ref";
const MAX_AGE_MS = 7 * 24 * 60 * 60 * 1000;

function readStored(): { code: string; at: number } | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw);
    if (typeof parsed?.code !== "string" || typeof parsed?.at !== "number") return null;
    if (Date.now() - parsed.at > MAX_AGE_MS) {
      localStorage.removeItem(STORAGE_KEY);
      return null;
    }
    return parsed;
  } catch {
    return null;
  }
}

/**
 * Referral attribution. A visit with ?ref=CODE stores the code (7 days) and
 * cleans it from the URL; once the visitor logs in, the code is claimed. The
 * server only accepts it for brand-new accounts, so existing users clicking
 * a friend's link are simply ignored.
 */
export default function ReferralCapture() {
  const { isAuthenticated, token } = useAuth();

  useEffect(() => {
    try {
      const url = new URL(window.location.href);
      const code = url.searchParams.get("ref");
      if (!code) return;
      if (!readStored()) {
        localStorage.setItem(STORAGE_KEY, JSON.stringify({ code: code.slice(0, 16), at: Date.now() }));
      }
      url.searchParams.delete("ref");
      window.history.replaceState(window.history.state, "", url.pathname + url.search + url.hash);
    } catch {
      // storage blocked — referral just won't be attributed
    }
  }, []);

  useEffect(() => {
    if (!isAuthenticated || !token) return;
    const stored = readStored();
    if (!stored) return;
    claimReferral(token, stored.code)
      .then(() => {
        try {
          localStorage.removeItem(STORAGE_KEY);
        } catch {
          /* ignore */
        }
      })
      .catch(() => {
        // network error — retry on next load
      });
  }, [isAuthenticated, token]);

  return null;
}
