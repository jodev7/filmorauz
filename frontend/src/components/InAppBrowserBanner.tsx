"use client";

import { useEffect, useState } from "react";
import { ExternalLink, X } from "lucide-react";
import { detectInAppBrowser, inAppBrowserLabel } from "@/lib/in-app-browser";

const DISMISS_KEY = "fmu_inapp_dismissed";

// Mounted from RootLayout. Shows once per device until the user
// dismisses or opens the site in an external browser. The "open in
// browser" CTA isn't a real escape — Telegram strips that intent — but
// it doubles as a hint to use the ⋯ menu's "Open in browser" item.
export default function InAppBrowserBanner() {
  const [show, setShow] = useState(false);
  const [label, setLabel] = useState("");

  useEffect(() => {
    if (typeof window === "undefined") return;
    try {
      if (window.localStorage.getItem(DISMISS_KEY) === "1") return;
    } catch {
      // Private mode / storage disabled — fall through and just show.
    }
    const b = detectInAppBrowser();
    if (b === "telegram" || b === "instagram" || b === "facebook") {
      setLabel(inAppBrowserLabel(b));
      setShow(true);
    }
  }, []);

  if (!show) return null;

  const dismiss = () => {
    try {
      window.localStorage.setItem(DISMISS_KEY, "1");
    } catch {}
    setShow(false);
  };

  return (
    <div className="fixed top-0 inset-x-0 z-[200] bg-gradient-to-r from-amber-600/95 to-amber-500/95 text-white text-sm shadow-lg backdrop-blur-sm">
      <div className="max-w-6xl mx-auto px-3 py-2 flex items-center gap-2">
        <ExternalLink size={16} className="flex-shrink-0" />
        <p className="flex-1 leading-snug">
          {label} ichida ekansiz. Yaxshi tajriba uchun yuqori o&apos;ng burchakdagi <b>⋯</b> menyusidan{" "}
          <b>&quot;Brauzerda ochish&quot;</b> ni tanlang.
        </p>
        <button
          onClick={dismiss}
          aria-label="Yopish"
          className="flex-shrink-0 p-1 rounded hover:bg-black/10 transition-colors"
        >
          <X size={16} />
        </button>
      </div>
    </div>
  );
}
