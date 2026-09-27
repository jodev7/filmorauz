"use client";

import { useEffect, useState, useCallback } from "react";
import { usePathname } from "next/navigation";
import { Announcement, getActiveAnnouncements } from "@/lib/api";
import AnnouncementModal from "@/components/AnnouncementModal";

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

  // Only modal-type announcements are shown here; "alert" types render as a
  // top banner via AlertBanner instead.
  // Not on admin pages — like AlertBanner, it's for site visitors (admins
  // see a live preview in /admin/announcements).
  const onAdmin = pathname?.startsWith("/admin");
  const current = onAdmin
    ? undefined
    : items.find((a) => a.type !== "alert" && !dismissed.includes(a.id));
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

  const queued = items.filter((x) => x.type !== "alert" && !dismissed.includes(x.id)).length;
  const total = items.filter((x) => x.type !== "alert").length;
  const counter = total > 1 && queued > 0 ? `${total - queued + 1}/${total}` : undefined;

  return (
    <AnnouncementModal
      key={current.id}
      a={current}
      counter={counter}
      onClose={handleClose}
      onLink={handleLinkClick}
    />
  );
}
