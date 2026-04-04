"use client";

import { useState } from "react";
import { Share2, Copy, Check, Send } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { createMovieShare, ShareResponse } from "@/lib/api";

interface Props {
  movieId: string;
  movieTitle: string;
  movieSlug: string;
}

export default function ShareButton({ movieId, movieTitle, movieSlug }: Props) {
  const { token } = useAuth();
  const [loading, setLoading] = useState(false);
  const [shareData, setShareData] = useState<ShareResponse | null>(null);
  const [copied, setCopied] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleShare = async () => {
    setLoading(true);
    setError(null);

    try {
      const data = await createMovieShare(token, movieId);
      setShareData(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create share");
    } finally {
      setLoading(false);
    }
  };

  const copyToClipboard = async () => {
    if (!shareData?.share_url) return;

    try {
      await navigator.clipboard.writeText(shareData.share_url);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Fallback for older browsers
      const input = document.createElement("input");
      input.value = shareData.share_url;
      document.body.appendChild(input);
      input.select();
      document.execCommand("copy");
      document.body.removeChild(input);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  };

  const shareToTelegram = () => {
    if (!shareData?.share_url) return;

    const text = encodeURIComponent(`🎬 ${movieTitle}\nWatch on FILMORAUZ`);
    const url = encodeURIComponent(shareData.share_url);
    window.open(`https://t.me/share/url?url=${url}&text=${text}`, "_blank");
  };

  // If we have share data, show the share options
  if (shareData) {
    return (
      <div className="flex flex-col gap-2">
        <div className="flex items-center gap-2">
          <input
            type="text"
            value={shareData.share_url}
            readOnly
            className="flex-1 bg-brand-dark border border-brand-border rounded-lg px-3 py-2 text-white text-sm"
          />
          <button
            onClick={copyToClipboard}
            className="p-2 bg-brand-card border border-brand-border rounded-lg text-white hover:bg-brand-border transition-colors"
            title="Copy link"
          >
            {copied ? <Check size={18} className="text-green-400" /> : <Copy size={18} />}
          </button>
          <button
            onClick={shareToTelegram}
            className="p-2 bg-[#229ED9] rounded-lg text-white hover:bg-[#1c8ac4] transition-colors"
            title="Share to Telegram"
          >
            <Send size={18} />
          </button>
        </div>
        <button
          onClick={() => setShareData(null)}
          className="text-sm text-gray-400 hover:text-white transition-colors"
        >
          Create new share link
        </button>
      </div>
    );
  }

  return (
    <button
      onClick={handleShare}
      disabled={loading}
      className="flex items-center gap-2 px-4 py-2 bg-brand-card border border-brand-border rounded-lg text-white hover:bg-brand-border transition-colors disabled:opacity-50"
    >
      <Share2 size={18} />
      {loading ? "Creating..." : "Share"}
    </button>
  );
}
