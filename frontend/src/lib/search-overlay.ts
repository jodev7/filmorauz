"use client";

// Tiny event bus so any button (navbar, bottom nav, pages) can open the
// global search overlay mounted once in the root layout.
export const OPEN_SEARCH_EVENT = "filmora:open-search";

export function openSearch(query = "") {
  window.dispatchEvent(new CustomEvent(OPEN_SEARCH_EVENT, { detail: { query } }));
}

const RECENT_KEY = "filmora_recent_searches";

export function readRecentSearches(): string[] {
  try {
    const v = JSON.parse(localStorage.getItem(RECENT_KEY) || "[]");
    return Array.isArray(v) ? v.filter((x) => typeof x === "string").slice(0, 8) : [];
  } catch {
    return [];
  }
}

export function pushRecentSearch(q: string): string[] {
  const clean = q.trim().replace(/\s+/g, " ").slice(0, 60);
  if (!clean) return readRecentSearches();
  const next = [clean, ...readRecentSearches().filter((x) => x.toLowerCase() !== clean.toLowerCase())].slice(0, 8);
  try {
    localStorage.setItem(RECENT_KEY, JSON.stringify(next));
  } catch {
    // storage blocked
  }
  return next;
}

export function removeRecentSearch(q: string): string[] {
  const next = readRecentSearches().filter((x) => x !== q);
  try {
    localStorage.setItem(RECENT_KEY, JSON.stringify(next));
  } catch {
    // storage blocked
  }
  return next;
}
