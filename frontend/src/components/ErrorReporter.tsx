"use client";

import { useEffect } from "react";
import { reportClientError, reportError } from "@/lib/error-reporter";

/** Installs global handlers for uncaught errors and unhandled promise rejections. */
export default function ErrorReporter() {
  useEffect(() => {
    const onError = (e: ErrorEvent) => {
      if (e.error) reportError("window", e.error);
      else if (e.message) reportClientError("window", e.message, e.filename ? `at ${e.filename}:${e.lineno}:${e.colno}` : "");
    };
    const onRejection = (e: PromiseRejectionEvent) => reportError("promise", e.reason);
    window.addEventListener("error", onError);
    window.addEventListener("unhandledrejection", onRejection);
    return () => {
      window.removeEventListener("error", onError);
      window.removeEventListener("unhandledrejection", onRejection);
    };
  }, []);
  return null;
}
