"use client";

import { useState } from "react";
import { Check, Share2 } from "lucide-react";

export default function ListShareButton({ title, slug }: { title: string; slug: string }) {
  const [copied, setCopied] = useState(false);
  const share = async () => {
    const url = `${window.location.origin}/lists/${slug}`;
    try {
      if (navigator.share && /Mobi|Android/i.test(navigator.userAgent)) {
        await navigator.share({ title, url });
        return;
      }
      await navigator.clipboard.writeText(url);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // cancelled
    }
  };
  return (
    <button
      onClick={share}
      className="inline-flex shrink-0 items-center gap-2 rounded-xl border border-white/10 px-4 py-2.5 text-sm text-white hover:border-orange-500/50 hover:bg-white/5"
    >
      {copied ? <Check size={16} className="text-emerald-400" /> : <Share2 size={16} />}
      {copied ? "Havola nusxalandi" : "Ulashish"}
    </button>
  );
}
