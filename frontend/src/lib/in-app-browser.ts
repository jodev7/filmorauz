// Detect whether the current page is being rendered inside an in-app
// browser (Telegram's webview most commonly, but also Instagram and FB
// when our share links land users there). The detection is intentionally
// loose — false positives just show an extra dismissible banner; false
// negatives are the real problem.
//
// We export both the boolean check and the human label so callers can
// render slightly different copy ("Telegramda" vs "Instagramda") without
// duplicating UA logic.

export type InAppBrowser = "telegram" | "instagram" | "facebook" | "other" | null;

export function detectInAppBrowser(): InAppBrowser {
  if (typeof navigator === "undefined") return null;
  const ua = navigator.userAgent || "";

  // Telegram's webview surfaces "Telegram" in the UA on Android; on iOS
  // the UA is the same as Safari, but `window.TelegramWebviewProxy` or
  // `window.Telegram` is present.
  if (
    /Telegram/i.test(ua) ||
    (typeof window !== "undefined" &&
      ((window as any).TelegramWebviewProxy !== undefined ||
        (window as any).Telegram?.WebApp !== undefined))
  ) {
    return "telegram";
  }

  if (/Instagram/i.test(ua)) return "instagram";
  if (/FBAN|FBAV|FB_IAB/i.test(ua)) return "facebook";

  // Generic webview heuristics — Android wv flag.
  if (/; wv\)/i.test(ua)) return "other";
  return null;
}

export function inAppBrowserLabel(b: InAppBrowser): string {
  switch (b) {
    case "telegram":
      return "Telegram";
    case "instagram":
      return "Instagram";
    case "facebook":
      return "Facebook";
    case "other":
      return "ilova";
    default:
      return "";
  }
}
