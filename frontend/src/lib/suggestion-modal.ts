"use client";

// Opens the global "Kino tavsiya qilish" sheet from anywhere.
export const OPEN_SUGGESTION_EVENT = "filmora:open-suggestion";

export interface SuggestionPrefill {
  title?: string;
  type?: "movie" | "series";
}

export function openSuggestion(prefill: SuggestionPrefill = {}) {
  window.dispatchEvent(new CustomEvent(OPEN_SUGGESTION_EVENT, { detail: prefill }));
}
