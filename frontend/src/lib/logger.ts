// Dev-only logger. In production builds (NODE_ENV !== "development")
// debug/info/log become no-ops so internal traces never leak to end users'
// browser consoles. Errors are kept but callers should avoid passing raw
// tokens, payloads, or URLs that may contain secrets.

const isDev =
  typeof process !== "undefined" && process.env.NODE_ENV === "development";

type LogArgs = unknown[];

export const logger = {
  debug: (...args: LogArgs) => {
    if (isDev) console.log(...args);
  },
  info: (...args: LogArgs) => {
    if (isDev) console.info(...args);
  },
  warn: (...args: LogArgs) => {
    if (isDev) console.warn(...args);
  },
  error: (...args: LogArgs) => {
    // Errors stay on in prod, but prefer short messages without payloads.
    console.error(...args);
  },
};
