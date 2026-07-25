"use client";

import { useEffect } from "react";

/**
 * Tracks what the current tab is watching so PresencePinger can report it with
 * every heartbeat and the admin "Onlayn sessiyalar" list can show the movie or
 * episode next to each session.
 *
 * A plain module-level value (not React context) because the reader —
 * PresencePinger — is mounted in the root layout, far above the watch page in
 * the tree, and only needs the latest value at ping time.
 */
export interface PresenceActivity {
  type: "movie" | "episode";
  content_id?: string;
  title?: string;
  slug?: string;
  url?: string;
}

let current: PresenceActivity | null = null;

// Fired whenever the reported content changes so PresencePinger can send an
// immediate heartbeat instead of waiting up to 45s for the next interval.
export const PRESENCE_ACTIVITY_EVENT = "filmorauz:presence-activity";

export function getPresenceActivity(): PresenceActivity | null {
  return current;
}

export function setPresenceActivity(activity: PresenceActivity | null) {
  current = activity;
  if (typeof window !== "undefined") {
    window.dispatchEvent(new Event(PRESENCE_ACTIVITY_EVENT));
  }
}

/**
 * Reports the given content as "currently open" for as long as the component is
 * mounted, and clears it on unmount (navigating away from the watch page).
 * Pass null to report nothing.
 */
export function useReportPresenceActivity(activity: PresenceActivity | null) {
  const key = activity
    ? [activity.type, activity.content_id, activity.slug, activity.title, activity.url].join("|")
    : "";

  useEffect(() => {
    setPresenceActivity(activity);
    return () => {
      // Only clear if we are still the reported one — a fast client-side
      // navigation can mount the next watch page before this cleanup runs.
      if (getPresenceActivity() === activity) setPresenceActivity(null);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);
}
