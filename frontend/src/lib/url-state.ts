"use client";

import { useEffect, useRef } from "react";

type UrlValue = string | number | boolean | string[];

/**
 * Reads a query param on first render. Admin pages only render client-side
 * (the admin layout renders nothing until auth resolves), so reading
 * `window.location` in a useState initializer is hydration-safe here.
 */
export function readUrlParam(key: string, fallback: string): string {
  if (typeof window === "undefined") return fallback;
  const v = new URLSearchParams(window.location.search).get(key);
  return v ?? fallback;
}

export function readUrlNumber(key: string, fallback: number): number {
  const n = parseInt(readUrlParam(key, String(fallback)), 10);
  return Number.isFinite(n) && n > 0 ? n : fallback;
}

export function readUrlBool(key: string, fallback = false): boolean {
  const v = readUrlParam(key, fallback ? "1" : "0");
  return v === "1" || v === "true";
}

export function readUrlList(key: string): string[] {
  const v = readUrlParam(key, "");
  return v ? v.split(",").filter(Boolean) : [];
}

function serialize(v: UrlValue): string {
  if (Array.isArray(v)) return v.join(",");
  if (typeof v === "boolean") return v ? "1" : "0";
  return String(v);
}

/**
 * Mirrors page state into the URL query string (history.replaceState — no
 * navigation, no re-render). Values equal to their default are omitted so
 * the URL stays short. Params not listed in `values` are left untouched.
 */
export function useSyncUrlParams(values: Record<string, UrlValue>, defaults: Record<string, UrlValue>) {
  const serialized = JSON.stringify(Object.keys(values).map((k) => [k, serialize(values[k])]));
  const defaultsRef = useRef(defaults);
  defaultsRef.current = defaults;

  useEffect(() => {
    const entries = JSON.parse(serialized) as [string, string][];
    const params = new URLSearchParams(window.location.search);
    for (const [key, value] of entries) {
      const def = key in defaultsRef.current ? serialize(defaultsRef.current[key]) : "";
      if (value === "" || value === def) params.delete(key);
      else params.set(key, value);
    }
    const qs = params.toString();
    const next = `${window.location.pathname}${qs ? `?${qs}` : ""}${window.location.hash}`;
    if (next !== `${window.location.pathname}${window.location.search}${window.location.hash}`) {
      window.history.replaceState(window.history.state, "", next);
    }
  }, [serialized]);
}
