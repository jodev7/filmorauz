"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth-context";
import { createWatchRoom } from "@/lib/api";
import { Users, Loader2 } from "lucide-react";

type Props = {
  contentType: "movie" | "episode";
  contentID: string;
  className?: string;
};

// WatchTogetherButton creates a private room for the current movie/episode
// and immediately routes the user into it as the host. Free users hit the
// daily-quota error from the backend if they exceed 3 rooms/day.
export default function WatchTogetherButton({ contentType, contentID, className }: Props) {
  const router = useRouter();
  const { user, token } = useAuth();
  const [busy, setBusy] = useState(false);

  const handleClick = async () => {
    if (!user || !token) {
      router.push("/");
      return;
    }
    if (busy) return;
    setBusy(true);
    try {
      const room = await createWatchRoom(token, {
        content_type: contentType,
        content_id: contentID,
        visibility: "private",
      });
      router.push(`/watch-room/${room.id}`);
    } catch (e) {
      alert(e instanceof Error ? e.message : "Failed to start room");
      setBusy(false);
    }
  };

  return (
    <button
      onClick={handleClick}
      disabled={busy}
      className={
        className ||
        "px-4 py-2 bg-brand-card border border-brand-border hover:border-brand-red rounded-lg flex items-center gap-2 text-white transition-colors disabled:opacity-50"
      }
    >
      {busy ? <Loader2 className="w-4 h-4 animate-spin" /> : <Users className="w-4 h-4" />}
      Watch together
    </button>
  );
}
