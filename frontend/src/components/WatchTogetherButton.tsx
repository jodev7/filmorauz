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
      alert("Birga ko'rish uchun avval ro'yxatdan o'ting va tizimga kiring.");
      router.push("/?login=1");
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
      const msg = e instanceof Error ? e.message : "Room yaratishda xato";
      // 401 keladigan eski token holatida foydalanuvchini qayta loginga qaytarish.
      if (msg.toLowerCase().includes("auth")) {
        alert("Sessiyangiz tugagan. Iltimos qaytadan tizimga kiring.");
        router.push("/?login=1");
      } else {
        alert(msg);
      }
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
      Birga ko&apos;rish
    </button>
  );
}
