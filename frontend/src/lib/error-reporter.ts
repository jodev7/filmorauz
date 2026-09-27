// Browser error reporting to our own backend (/api/client-errors) — shows up
// grouped on /admin/errors. No third-party service involved.
import Cookies from "js-cookie";

const API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080/api";
const RELEASE = process.env.NEXT_PUBLIC_APP_VERSION || "";
const MAX_PER_PAGE = 15;

// Known noise that isn't actionable.
const IGNORE = [
  /ResizeObserver loop/i,
  /^Script error\.?$/i, // opaque cross-origin error
  /chrome-extension:|moz-extension:|safari-extension:/i,
  /Non-Error promise rejection captured/i,
  /AbortError|The user aborted a request|signal is aborted/i,
  /Load failed|Failed to fetch|NetworkError when attempting to fetch/i, // offline / flaky network
];

const sent = new Set<string>();

export type ClientErrorKind = "window" | "promise" | "react";

export function reportClientError(kind: ClientErrorKind, message: string, stack = ""): void {
  if (typeof window === "undefined" || !message) return;
  if (IGNORE.some((rx) => rx.test(message) || rx.test(stack))) return;
  const key = `${kind}|${message}|${stack.slice(0, 200)}`;
  if (sent.has(key) || sent.size >= MAX_PER_PAGE) return;
  sent.add(key);

  const headers: Record<string, string> = { "Content-Type": "application/json" };
  const token = Cookies.get("auth_token");
  if (token) headers.Authorization = `Bearer ${token}`;

  fetch(`${API_URL}/client-errors`, {
    method: "POST",
    headers,
    body: JSON.stringify({
      kind,
      message: message.slice(0, 1000),
      stack: stack.slice(0, 4000),
      url: window.location.href.slice(0, 500),
      release: RELEASE,
    }),
    keepalive: true,
  }).catch(() => {});
}

export function reportError(kind: ClientErrorKind, err: unknown): void {
  if (err instanceof Error) reportClientError(kind, `${err.name}: ${err.message}`, err.stack || "");
  else reportClientError(kind, typeof err === "string" ? err : JSON.stringify(err ?? "unknown"));
}
