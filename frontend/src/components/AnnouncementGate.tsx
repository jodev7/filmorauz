"use client";

import { useEffect, useState, useCallback } from "react";
import { usePathname } from "next/navigation";
import { X, ExternalLink } from "lucide-react";
import { Announcement, getActiveAnnouncements } from "@/lib/api";

const DISMISSED_KEY = "dismissed_announcements_v1";
const POLL_INTERVAL_MS = 60_000;

function readDismissed(): string[] {
  if (typeof window === "undefined") return [];
  try {
    const raw = window.localStorage.getItem(DISMISSED_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed.filter((x) => typeof x === "string") : [];
  } catch {
    return [];
  }
}

function persistDismissed(ids: string[]) {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(DISMISSED_KEY, JSON.stringify(ids));
  } catch {
    /* ignore */
  }
}

export default function AnnouncementGate() {
  const pathname = usePathname();
  const [items, setItems] = useState<Announcement[]>([]);
  const [dismissed, setDismissed] = useState<string[]>([]);

  const refresh = useCallback(async () => {
    const data = await getActiveAnnouncements();
    setItems(data);
  }, []);

  useEffect(() => {
    setDismissed(readDismissed());
  }, []);

  useEffect(() => {
    refresh();
    const t = setInterval(refresh, POLL_INTERVAL_MS);
    return () => clearInterval(t);
    // Re-fetch on path change so messages appear right after navigation.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pathname]);

  const current = items.find((a) => !dismissed.includes(a.id));
  if (!current) return null;

  const dismissNow = () => {
    const next = Array.from(new Set([...dismissed, current.id]));
    setDismissed(next);
    persistDismissed(next);
  };

  const handleClose = () => {
    if (!current.dismissible) return;
    dismissNow();
  };

  // Clicking the link counts as acknowledgement — close the modal and
  // remember it so it does not pop up again on later navigations.
  const handleLinkClick = () => {
    dismissNow();
  };

  const hasLink = !!current.link_url;

  return (
    <div className="fixed inset-0 z-[200] flex items-center justify-center px-4">
      <div
        className="absolute inset-0 bg-black/70 backdrop-blur-md"
        onClick={handleClose}
        aria-hidden="true"
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="announcement-title"
        className="relative w-full max-w-md rounded-2xl bg-[#15151f] border border-[#1e1e2e] shadow-2xl overflow-hidden"
      >
        {current.dismissible && (
          <button
            onClick={handleClose}
            className="absolute top-3 right-3 w-9 h-9 rounded-full bg-black/30 hover:bg-black/60 flex items-center justify-center transition-colors"
            aria-label="Yopish"
          >
            <X size={18} className="text-white" />
          </button>
        )}

        <div className="p-6 sm:p-7">
          <h2
            id="announcement-title"
            className="font-display text-xl sm:text-2xl text-white tracking-wide pr-10"
          >
            {current.title}
          </h2>
          {current.body && (
            <p className="mt-3 text-sm sm:text-base text-gray-300 whitespace-pre-line leading-relaxed">
              {current.body}
            </p>
          )}

          <div className="mt-6 flex flex-col sm:flex-row gap-3">
            {hasLink && (
              <a
                href={current.link_url}
                target="_blank"
                rel="noopener noreferrer"
                onClick={handleLinkClick}
                className="flex-1 inline-flex items-center justify-center gap-2 px-4 py-2.5 rounded-lg bg-orange-500 hover:bg-orange-600 text-white font-medium transition-colors"
              >
                {current.link_label || "Batafsil"}
                <ExternalLink size={16} />
              </a>
            )}
            {current.dismissible && (
              <button
                onClick={handleClose}
                className={`${hasLink ? "sm:w-32" : "flex-1"} px-4 py-2.5 rounded-lg bg-white/5 hover:bg-white/10 text-white font-medium transition-colors`}
              >
                {hasLink ? "Yopish" : "Tushundim"}
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
