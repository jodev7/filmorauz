"use client";

import { useState, useRef, useEffect } from "react";
import { Send, Loader2, Upload, CheckCircle, XCircle, Users, Hash, Clock, User, Link, ToggleLeft, ToggleRight } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { sendTelegramPost, uploadTelegramPostMedia, listTelegramPosts, TelegramPost } from "@/lib/api";
import { normalizeMediaUrl } from "@/lib/image-utils";

interface TelegramPostResult {
  channels_sent: number;
  channels_failed: number;
  bot_sent: number;
  bot_failed: number;
  error?: string;
}

export default function TelegramPostPage() {
  const { token } = useAuth();
  const [text, setText] = useState("");
  const [imageUrl, setImageUrl] = useState("");
  const [sendToChannels, setSendToChannels] = useState(true);
  const [sendToBot, setSendToBot] = useState(false);
  const [useInlineButton, setUseInlineButton] = useState(false);
  const [buttonText, setButtonText] = useState("");
  const [buttonUrl, setButtonUrl] = useState("");
  const [validationError, setValidationError] = useState("");
  const [sending, setSending] = useState(false);
  const [result, setResult] = useState<TelegramPostResult | null>(null);
  const [uploading, setUploading] = useState(false);
  const [postHistory, setPostHistory] = useState<TelegramPost[]>([]);
  const [loadingHistory, setLoadingHistory] = useState(true);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const loadHistory = async () => {
    if (!token) return;
    try {
      const posts = await listTelegramPosts(token);
      setPostHistory(posts);
    } catch (err) {
      console.error("Failed to load post history:", err);
    } finally {
      setLoadingHistory(false);
    }
  };

  useEffect(() => {
    loadHistory();
  }, [token]);

  const handleImageUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file || !token) return;
    
    setUploading(true);
    try {
      const url = await uploadTelegramPostMedia(token, file);
      setImageUrl(url);
    } catch (err) {
      alert("Image upload failed");
    } finally {
      setUploading(false);
    }
  };

  const handleSend = async () => {
    setValidationError("");

    if (!text.trim()) {
      setValidationError("Matn kiritish shart.");
      return;
    }
    if (!sendToChannels && !sendToBot) {
      setValidationError("Kamida bitta manzil tanlang (Kanal yoki Bot).");
      return;
    }
    if (useInlineButton) {
      if (!buttonText.trim()) {
        setValidationError("Tugma matni kiritilishi shart.");
        return;
      }
      if (!buttonUrl.trim()) {
        setValidationError("Tugma URL kiritilishi shart.");
        return;
      }
      if (!buttonUrl.trim().startsWith("http://") && !buttonUrl.trim().startsWith("https://")) {
        setValidationError("URL http:// yoki https:// bilan boshlanishi kerak.");
        return;
      }
    }
    if (!token) return;

    setSending(true);
    setResult(null);

    try {
      const body: Record<string, unknown> = {
        text: text.trim(),
        image_url: imageUrl || undefined,
        send_to_channels: sendToChannels,
        send_to_bot: sendToBot,
      };
      if (useInlineButton) {
        body.inline_button_text = buttonText.trim();
        body.inline_button_url = buttonUrl.trim();
      }

      const res = await fetch(`${process.env.NEXT_PUBLIC_API_URL || ''}/superadmin/telegram-post`, {
        method: "POST",
        headers: { "Authorization": `Bearer ${token}`, "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });

      const data = await res.json();
      if (!res.ok) {
        setValidationError(data.error || "Yuborish muvaffaqiyatsiz.");
        return;
      }
      setResult(data);
      await loadHistory();
    } catch {
      setResult({ channels_sent: 0, channels_failed: 0, bot_sent: 0, bot_failed: 0, error: "Request failed" });
    } finally {
      setSending(false);
    }
  };

  return (
    <div className="p-6 max-w-2xl">
      <div className="mb-6">
        <h1 className="text-2xl font-display text-white tracking-wide">Telegram Post</h1>
        <p className="text-gray-500 text-sm mt-1">Send posts to Telegram channels and bot users</p>
      </div>

      <div className="space-y-4">
        <div>
          <label className="block text-sm text-gray-400 mb-1">Text *</label>
          <textarea
            value={text}
            onChange={(e) => setText(e.target.value)}
            rows={5}
            className="w-full bg-brand-dark border border-brand-border rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-brand-red resize-none"
            placeholder="Post text..."
          />
        </div>

        <div>
          <label className="block text-sm text-gray-400 mb-1">Image (optional)</label>
          <div className="flex items-center gap-3">
            <input
              type="file"
              accept="image/jpeg,image/png,image/webp,image/gif"
              className="hidden"
              ref={fileInputRef}
              onChange={handleImageUpload}
              disabled={uploading}
            />
            <button
              type="button"
              onClick={() => fileInputRef.current?.click()}
              disabled={uploading}
              className="flex items-center gap-2 px-3 py-2 bg-brand-dark border border-brand-border rounded-lg text-sm text-gray-400 hover:text-white hover:border-brand-red transition"
            >
              {uploading ? <Loader2 size={14} className="animate-spin" /> : <Upload size={14} />}
              {imageUrl ? "Image uploaded ✓" : "Upload image"}
            </button>
            {imageUrl && (
              <button
                onClick={() => setImageUrl("")}
                className="text-red-400 hover:text-red-300 text-sm"
              >
                Remove
              </button>
            )}
          </div>
          {imageUrl && (
            <img src={normalizeMediaUrl(imageUrl)} alt="Preview" className="mt-2 w-32 h-32 object-cover rounded border border-brand-border" />
          )}
        </div>

        <div className="flex items-center gap-6">
          <label className="flex items-center gap-2 cursor-pointer">
            <input
              type="checkbox"
              checked={sendToChannels}
              onChange={(e) => setSendToChannels(e.target.checked)}
              className="accent-brand-red"
            />
            <span className="text-gray-300 text-sm">Send to channels</span>
          </label>

          <label className="flex items-center gap-2 cursor-pointer">
            <input
              type="checkbox"
              checked={sendToBot}
              onChange={(e) => setSendToBot(e.target.checked)}
              className="accent-brand-red"
            />
            <span className="text-gray-300 text-sm">Send to bot users</span>
          </label>
        </div>

        {/* Inline button toggle */}
        <div className="border border-brand-border rounded-lg overflow-hidden">
          <button
            type="button"
            onClick={() => { setUseInlineButton(!useInlineButton); setValidationError(""); }}
            className="w-full flex items-center justify-between px-4 py-3 hover:bg-brand-border/30 transition"
          >
            <span className="flex items-center gap-2 text-sm text-gray-300">
              <Link size={15} className="text-blue-400" />
              Inline button qo&apos;shish
            </span>
            {useInlineButton
              ? <ToggleRight size={22} className="text-blue-400" />
              : <ToggleLeft size={22} className="text-gray-500" />}
          </button>

          {useInlineButton && (
            <div className="px-4 pb-4 pt-1 space-y-3 border-t border-brand-border bg-brand-dark/30">
              <div>
                <label className="block text-xs text-gray-400 mb-1">Tugma matni *</label>
                <input
                  type="text"
                  value={buttonText}
                  onChange={(e) => { setButtonText(e.target.value); setValidationError(""); }}
                  placeholder="Ko'rish"
                  className="w-full bg-brand-dark border border-brand-border rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-blue-500"
                />
              </div>
              <div>
                <label className="block text-xs text-gray-400 mb-1">Tugma URL *</label>
                <input
                  type="url"
                  value={buttonUrl}
                  onChange={(e) => { setButtonUrl(e.target.value); setValidationError(""); }}
                  placeholder="https://example.com"
                  className="w-full bg-brand-dark border border-brand-border rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-blue-500"
                />
              </div>
            </div>
          )}
        </div>

        {validationError && (
          <p className="text-red-400 text-sm flex items-center gap-2">
            <XCircle size={15} /> {validationError}
          </p>
        )}

        <button
          onClick={handleSend}
          disabled={sending || !text.trim() || (!sendToChannels && !sendToBot)}
          className="flex items-center justify-center gap-2 w-full py-3 bg-brand-red text-white rounded-lg font-medium hover:bg-orange-600 transition disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {sending ? <Loader2 size={18} className="animate-spin" /> : <Send size={18} />}
          {sending ? "Sending..." : "Send Post"}
        </button>

        {result && (
          <div className="mt-4 p-4 bg-brand-dark rounded-lg border border-brand-border">
            <h3 className="text-white font-medium mb-2">Results</h3>
            {result.error ? (
              <p className="text-red-400 text-sm">{result.error}</p>
            ) : (
              <div className="space-y-1 text-sm">
                <p className="flex items-center gap-2">
                  <span className="text-gray-400">Channels:</span>
                  <CheckCircle size={14} className="text-green-400" />
                  <span className="text-green-400">{result.channels_sent} sent</span>
                  {result.channels_failed > 0 && (
                    <>
                      <XCircle size={14} className="text-red-400 ml-2" />
                      <span className="text-red-400">{result.channels_failed} failed</span>
                    </>
                  )}
                </p>
                <p className="flex items-center gap-2">
                  <span className="text-gray-400">Bot users:</span>
                  <CheckCircle size={14} className="text-green-400" />
                  <span className="text-green-400">{result.bot_sent} sent</span>
                  {result.bot_failed > 0 && (
                    <>
                      <XCircle size={14} className="text-red-400 ml-2" />
                      <span className="text-red-400">{result.bot_failed} failed</span>
                    </>
                  )}
                </p>
              </div>
            )}
          </div>
        )}
      </div>

      {/* Post History */}
      <div className="mt-8">
        <h2 className="text-xl font-display text-white mb-4">Yuborilgan postlar</h2>
        {loadingHistory ? (
          <div className="flex justify-center py-8">
            <Loader2 className="animate-spin text-brand-red" size={24} />
          </div>
        ) : postHistory.length === 0 ? (
          <p className="text-gray-500 text-sm">Hali postlar yuborilmagan</p>
        ) : (
          <div className="space-y-3">
            {postHistory.map((post) => (
              <div key={post.id} className="bg-brand-dark border border-brand-border rounded-lg p-4">
                <div className="flex gap-4">
                  {post.image_url && (
                    <img src={normalizeMediaUrl(post.image_url)} alt="" className="w-16 h-16 object-cover rounded shrink-0" />
                  )}
                  <div className="flex-1 min-w-0">
                    <p className="text-white text-sm line-clamp-2">{post.text}</p>
                    {post.inline_button_text && (
                      <a
                        href={post.inline_button_url}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="inline-flex items-center gap-1.5 mt-1.5 px-2.5 py-1 rounded border border-blue-500/40 bg-blue-500/10 text-blue-400 text-xs hover:bg-blue-500/20 transition max-w-xs truncate"
                      >
                        <Link size={11} />
                        {post.inline_button_text}
                      </a>
                    )}
                    <div className="flex flex-wrap items-center gap-3 mt-2 text-xs text-gray-400">
                      {post.send_to_channels && (
                        <span className="flex items-center gap-1">
                          <Hash size={12} /> Kanallar: {post.channels_sent_count}
                          {post.channels_failed_count > 0 && <span className="text-red-400">/{post.channels_failed_count}</span>}
                        </span>
                      )}
                      {post.send_to_bot_users && (
                        <span className="flex items-center gap-1">
                          <Users size={12} /> Bot: {post.bot_sent_count}
                          {post.bot_failed_count > 0 && <span className="text-red-400">/{post.bot_failed_count}</span>}
                        </span>
                      )}
                      <span className="flex items-center gap-1">
                        <Clock size={12} /> {new Date(post.sent_at).toLocaleString("uz-UZ")}
                      </span>
                      <span className="flex items-center gap-1">
                        <User size={12} /> {post.sent_by_name}
                      </span>
                    </div>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
