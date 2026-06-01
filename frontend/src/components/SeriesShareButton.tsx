"use client";

import { useState } from "react";
import { Share2, Copy, Check, Send } from "lucide-react";

interface Props {
  seriesSlug: string;
  seriesTitle: string;
}

// Series don't use the tracked movie-share backend (which is movie-id bound),
// so we share the direct /series/<slug> URL — copy or send to Telegram.
// Mirrors the movie ShareButton's UI/UX.
export default function SeriesShareButton({ seriesSlug, seriesTitle }: Props) {
  const [open, setOpen] = useState(false);
  const [copied, setCopied] = useState(false);

  const siteUrl = process.env.NEXT_PUBLIC_SITE_URL || "https://filmorauz.net";
  const shareUrl = `${siteUrl.replace(/\/$/, "")}/series/${seriesSlug}`;

  const copyToClipboard = async () => {
    try {
      await navigator.clipboard.writeText(shareUrl);
    } catch {
      const input = document.createElement("input");
      input.value = shareUrl;
      document.body.appendChild(input);
      input.select();
      document.execCommand("copy");
      document.body.removeChild(input);
    }
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const shareToTelegram = () => {
    const text = encodeURIComponent(`📺 ${seriesTitle}\nFILMORAUZ'da tomosha qiling`);
    const url = encodeURIComponent(shareUrl);
    window.open(`https://t.me/share/url?url=${url}&text=${text}`, "_blank");
  };

  if (open) {
    return (
      <div className="flex flex-col gap-2">
        <div className="flex items-center gap-2">
          <input
            type="text"
            value={shareUrl}
            readOnly
            className="flex-1 bg-brand-dark border border-brand-border rounded-lg px-3 py-2 text-white text-sm"
          />
          <button
            onClick={copyToClipboard}
            className="p-2 bg-brand-card border border-brand-border rounded-lg text-white hover:bg-brand-border transition-colors"
            title="Havolani nusxalash"
          >
            {copied ? <Check size={18} className="text-green-400" /> : <Copy size={18} />}
          </button>
          <button
            onClick={shareToTelegram}
            className="p-2 bg-[#229ED9] rounded-lg text-white hover:bg-[#1c8ac4] transition-colors"
            title="Telegramda ulashish"
          >
            <Send size={18} />
          </button>
        </div>
        <button
          onClick={() => setOpen(false)}
          className="text-sm text-gray-400 hover:text-white transition-colors self-start"
        >
          Yopish
        </button>
      </div>
    );
  }

  return (
    <button
      onClick={() => setOpen(true)}
      className="flex items-center gap-2 px-4 py-2 bg-brand-card border border-brand-border rounded-lg text-white hover:bg-brand-border transition-colors"
    >
      <Share2 size={18} />
      Ulashish
    </button>
  );
}
