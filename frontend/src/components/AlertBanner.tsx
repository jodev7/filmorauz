"use client";

import { useEffect, useRef, useState, useCallback } from "react";
import { usePathname } from "next/navigation";
import { AlertTriangle } from "lucide-react";
import { Announcement, getActiveAnnouncements } from "@/lib/api";

const POLL_INTERVAL_MS = 60_000;

/**
 * AlertBanner renders active "alert"-type announcements as a thin red bar
 * pinned to the very top of every page (homepage, movies, series, episodes,
 * genres, collections, …). Admin sets the text + time window; the backend only
 * returns it while `now` is inside that window.
 *
 * Layout coupling: the fixed Navbar sits at `top: var(--site-alert-h)` and the
 * page body has `padding-top: var(--site-alert-h)` (globals.css). This
 * component measures its own height and publishes it into that variable so the
 * whole page shifts down by exactly the banner height — no overlap. When no
 * alert is active the variable is reset to 0.
 */
export default function AlertBanner() {
  const pathname = usePathname();
  const [items, setItems] = useState<Announcement[]>([]);
  const ref = useRef<HTMLDivElement | null>(null);

  const refresh = useCallback(async () => {
    const data = await getActiveAnnouncements();
    setItems(data);
  }, []);

  useEffect(() => {
    refresh();
    const t = setInterval(refresh, POLL_INTERVAL_MS);
    return () => clearInterval(t);
    // Re-fetch on navigation so the banner appears right after a route change.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pathname]);

  // Hide on admin pages — the maintenance notice is for site visitors, not the
  // admin managing it.
  const onAdmin = pathname?.startsWith("/admin");
  const alert = onAdmin
    ? undefined
    : items.find((a) => a.type === "alert");

  // Publish the banner height into --site-alert-h so the fixed navbar and page
  // content shift down. Reset to 0 whenever the banner isn't shown.
  useEffect(() => {
    const root = document.documentElement;
    if (!alert) {
      root.style.setProperty("--site-alert-h", "0px");
      return;
    }
    const apply = () => {
      const h = ref.current?.offsetHeight ?? 0;
      root.style.setProperty("--site-alert-h", `${h}px`);
    };
    apply();
    window.addEventListener("resize", apply);
    return () => {
      window.removeEventListener("resize", apply);
      root.style.setProperty("--site-alert-h", "0px");
    };
  }, [alert]);

  if (!alert) return null;

  return (
    <div
      ref={ref}
      role="alert"
      className="fixed top-0 left-0 right-0 z-[80] bg-red-600 text-white text-[11px] sm:text-xs leading-snug px-3 py-1.5 flex items-center justify-center gap-1.5 text-center pt-[calc(env(safe-area-inset-top)_+_0.375rem)]"
    >
      <AlertTriangle size={13} className="shrink-0" />
      <span className="font-medium">
        {alert.title}
        {alert.body ? ` — ${alert.body}` : ""}
      </span>
    </div>
  );
}
