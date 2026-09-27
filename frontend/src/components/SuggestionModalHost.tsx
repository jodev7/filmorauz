"use client";

import { useCallback, useEffect, useState } from "react";
import SuggestionModal from "@/components/SuggestionModal";
import { OPEN_SUGGESTION_EVENT, SuggestionPrefill } from "@/lib/suggestion-modal";

/** Single app-wide instance of the suggestion sheet (see openSuggestion). */
export default function SuggestionModalHost() {
  const [prefill, setPrefill] = useState<SuggestionPrefill | null>(null);
  useEffect(() => {
    const onOpen = (e: Event) => setPrefill((e as CustomEvent<SuggestionPrefill>).detail || {});
    window.addEventListener(OPEN_SUGGESTION_EVENT, onOpen);
    return () => window.removeEventListener(OPEN_SUGGESTION_EVENT, onOpen);
  }, []);
  const close = useCallback(() => setPrefill(null), []);
  return <SuggestionModal isOpen={prefill !== null} onClose={close} initialTitle={prefill?.title} initialType={prefill?.type} />;
}
