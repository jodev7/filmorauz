"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  Bold,
  CheckCircle2,
  Clock,
  Code,
  Eye,
  EyeOff,
  Hash,
  Italic,
  Link2,
  Loader2,
  RotateCcw,
  Send,
  Strikethrough,
  Underline,
  User,
  Users,
  XCircle,
} from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { useToast } from "@/components/admin/Toast";
import { listTelegramPosts, sendTelegramPost, uploadTelegramPostMedia, TelegramPost, TelegramPostResult } from "@/lib/api";
import { normalizeMediaUrl } from "@/lib/image-utils";
import MediaUploadField from "@/components/admin/form/MediaUploadField";
import { Field, SwitchRow, inputCls } from "@/components/admin/form/ui";
import { renderTelegramHtml, telegramTextLength } from "@/components/admin/telegramHtml";

const pad = (n: number) => String(n).padStart(2, "0");
const MONTHS = ["yan", "fev", "mar", "apr", "may", "iyn", "iyl", "avg", "sen", "okt", "noy", "dek"];
function fmt(iso: string): string {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return "";
  return `${d.getDate()}-${MONTHS[d.getMonth()]}, ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

const TOOLS: { icon: typeof Bold; label: string; open: string; close: string }[] = [
  { icon: Bold, label: "Qalin", open: "<b>", close: "</b>" },
  { icon: Italic, label: "Kursiv", open: "<i>", close: "</i>" },
  { icon: Underline, label: "Tagiga chizilgan", open: "<u>", close: "</u>" },
  { icon: Strikethrough, label: "O'chirilgan", open: "<s>", close: "</s>" },
  { icon: Code, label: "Kod", open: "<code>", close: "</code>" },
  { icon: EyeOff, label: "Spoyler", open: "<tg-spoiler>", close: "</tg-spoiler>" },
];

function Stat({ label, sent, blocked, failed, icon: Icon }: { label: string; sent: number; blocked: number; failed: number; icon: typeof Hash }) {
  return (
    <div className="rounded-xl border border-white/10 bg-black/20 p-3">
      <p className="flex items-center gap-1.5 text-xs text-gray-400">
        <Icon size={13} /> {label}
      </p>
      <p className="mt-1 text-xl font-bold text-emerald-400">{sent}</p>
      <p className="text-[11px] text-gray-500">
        yuborildi
        {blocked > 0 && <span className="text-amber-400"> · {blocked} bloklagan</span>}
        {failed > 0 && <span className="text-red-400"> · {failed} xato</span>}
      </p>
    </div>
  );
}

export default function TelegramPostPage() {
  const { token } = useAuth();
  const toast = useToast();
  const [text, setText] = useState("");
  const [imageUrl, setImageUrl] = useState("");
  const [imageBusy, setImageBusy] = useState(false);
  const [toChannels, setToChannels] = useState(true);
  const [toBot, setToBot] = useState(false);
  const [withButton, setWithButton] = useState(false);
  const [buttonText, setButtonText] = useState("");
  const [buttonUrl, setButtonUrl] = useState("");
  const [error, setError] = useState("");
  const [sending, setSending] = useState(false);
  const [result, setResult] = useState<TelegramPostResult | null>(null);
  const [history, setHistory] = useState<TelegramPost[]>([]);
  const [loadingHistory, setLoadingHistory] = useState(true);
  const [spoilers, setSpoilers] = useState(false);
  const textRef = useRef<HTMLTextAreaElement>(null);

  const loadHistory = useCallback(async () => {
    if (!token) return;
    try {
      setHistory(await listTelegramPosts(token));
    } catch {
      /* shown as empty */
    } finally {
      setLoadingHistory(false);
    }
  }, [token]);

  useEffect(() => {
    loadHistory();
  }, [loadHistory]);

  const limit = imageUrl ? 1024 : 4096;
  const length = telegramTextLength(text);
  const over = length > limit;

  const wrap = (open: string, close: string) => {
    const el = textRef.current;
    if (!el) return;
    const { selectionStart: a, selectionEnd: b } = el;
    const next = text.slice(0, a) + open + text.slice(a, b) + close + text.slice(b);
    setText(next);
    requestAnimationFrame(() => {
      el.focus();
      el.setSelectionRange(a + open.length, b + open.length);
    });
  };

  const insertLink = () => {
    const url = prompt("Havola (https://...)");
    if (!url) return;
    if (!/^https?:\/\//.test(url)) return toast.error("Havola http:// yoki https:// bilan boshlanishi kerak");
    wrap(`<a href="${url.replace(/"/g, "")}">`, "</a>");
  };

  const validate = (): string => {
    if (!text.trim()) return "Post matnini yozing";
    if (over) return `Matn juda uzun: ${length}/${limit}${imageUrl ? " (rasmli postda Telegram 1024 belgigacha ruxsat beradi)" : ""}`;
    if (!toChannels && !toBot) return "Kamida bitta manzilni tanlang";
    if (withButton) {
      if (!buttonText.trim()) return "Tugma matnini kiriting";
      if (!/^https?:\/\//.test(buttonUrl.trim())) return "Tugma havolasi http:// yoki https:// bilan boshlanishi kerak";
    }
    return "";
  };

  const send = async () => {
    const v = validate();
    setError(v);
    if (v || !token) return;
    const where = [toChannels && "kanallarga", toBot && "barcha bot foydalanuvchilariga"].filter(Boolean).join(" va ");
    if (!confirm(`Post ${where} yuborilsinmi?`)) return;
    setSending(true);
    setResult(null);
    try {
      const res = await sendTelegramPost(token, {
        text: text.trim(),
        image_url: imageUrl || undefined,
        send_to_channels: toChannels,
        send_to_bot: toBot,
        ...(withButton ? { inline_button_text: buttonText.trim(), inline_button_url: buttonUrl.trim() } : {}),
      });
      setResult(res);
      toast.success("Post yuborildi");
      await loadHistory();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Yuborib bo'lmadi");
    } finally {
      setSending(false);
    }
  };

  const reuse = (p: TelegramPost) => {
    setText(p.text);
    setImageUrl(p.image_url || "");
    setToChannels(p.send_to_channels);
    setToBot(p.send_to_bot_users);
    setWithButton(!!p.inline_button_text);
    setButtonText(p.inline_button_text || "");
    setButtonUrl(p.inline_button_url || "");
    setResult(null);
    setError("");
    window.scrollTo({ top: 0, behavior: "smooth" });
  };

  const reset = () => {
    setText("");
    setImageUrl("");
    setWithButton(false);
    setButtonText("");
    setButtonUrl("");
    setResult(null);
    setError("");
  };

  const now = new Date();
  const previewImg = imageUrl ? normalizeMediaUrl(imageUrl) : "";

  return (
    <div className="mx-auto max-w-6xl p-4 sm:p-6">
      <header className="mb-6 flex flex-wrap items-start justify-between gap-3">
        <div className="flex items-start gap-3">
          <span className="flex h-11 w-11 items-center justify-center rounded-2xl bg-gradient-to-br from-sky-400 to-blue-600 shadow-lg shadow-sky-500/20">
            <Send size={19} className="text-white" />
          </span>
          <div>
            <h1 className="text-2xl font-bold text-white">Telegram post</h1>
            <p className="text-sm text-gray-500">Kanallarga va bot foydalanuvchilariga xabar yuborish</p>
          </div>
        </div>
        {(text || imageUrl) && (
          <button onClick={reset} className="inline-flex items-center gap-1.5 rounded-xl px-3 py-2 text-sm text-gray-400 hover:bg-white/5 hover:text-white">
            <RotateCcw size={14} /> Tozalash
          </button>
        )}
      </header>

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_380px]">
        {/* Composer */}
        <div className="space-y-5">
          <section className="rounded-2xl border border-white/10 bg-[#12121a] p-4 sm:p-5">
            <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
              <label htmlFor="tg-text" className="text-sm font-semibold text-white">
                Post matni
              </label>
              <div className="flex items-center gap-0.5 rounded-xl border border-white/10 bg-black/30 p-0.5">
                {TOOLS.map((t) => (
                  <button key={t.label} type="button" title={t.label} aria-label={t.label} onClick={() => wrap(t.open, t.close)} className="rounded-lg p-1.5 text-gray-400 hover:bg-white/10 hover:text-white">
                    <t.icon size={15} />
                  </button>
                ))}
                <button type="button" title="Havola" aria-label="Havola" onClick={insertLink} className="rounded-lg p-1.5 text-gray-400 hover:bg-white/10 hover:text-white">
                  <Link2 size={15} />
                </button>
              </div>
            </div>
            <textarea
              id="tg-text"
              ref={textRef}
              value={text}
              onChange={(e) => {
                setText(e.target.value);
                setError("");
              }}
              rows={9}
              placeholder={"🎬 <b>Yangi kino!</b>\n\nBugun saytda premyera — tomosha qiling 👇"}
              className={`${inputCls} resize-y font-mono text-[13px] leading-relaxed`}
            />
            <div className="mt-1.5 flex items-center justify-between text-[11px]">
              <span className="text-gray-600">Matnni belgilab, formatlash tugmasini bosing</span>
              <span className={over ? "font-semibold text-red-400" : length > limit * 0.9 ? "text-amber-400" : "text-gray-500"}>
                {length}/{limit}
              </span>
            </div>
          </section>

          <MediaUploadField
            label="Rasm (ixtiyoriy)"
            kind="backdrop"
            value={imageUrl}
            onChange={setImageUrl}
            onBusyChange={setImageBusy}
            hint="Rasmli postda matn 1024 belgigacha bo'lishi mumkin"
            upload={async (file) => {
              if (!token) throw new Error("Tizimga qayta kiring");
              return { url: await uploadTelegramPostMedia(token, file) };
            }}
          />

          <section className="rounded-2xl border border-white/10 bg-[#12121a] p-4 sm:p-5">
            <p className="mb-3 text-sm font-semibold text-white">Kimga yuboriladi</p>
            <div className="grid gap-3 sm:grid-cols-2">
              {[
                { on: toChannels, set: setToChannels, icon: Hash, title: "Kanallar", desc: "Barcha ulangan Telegram kanallar" },
                { on: toBot, set: setToBot, icon: Users, title: "Bot foydalanuvchilari", desc: "Botni ishga tushirgan hamma foydalanuvchi" },
              ].map((d) => (
                <button
                  key={d.title}
                  type="button"
                  role="checkbox"
                  aria-checked={d.on}
                  onClick={() => {
                    d.set(!d.on);
                    setError("");
                  }}
                  className={`flex items-start gap-3 rounded-xl border p-3.5 text-left transition ${
                    d.on ? "border-sky-500/50 bg-sky-500/[0.08]" : "border-white/10 bg-black/20 hover:border-white/20"
                  }`}
                >
                  <span className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-lg ${d.on ? "bg-sky-500 text-white" : "bg-white/5 text-gray-400"}`}>
                    <d.icon size={17} />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block text-sm font-medium text-white">{d.title}</span>
                    <span className="block text-xs text-gray-500">{d.desc}</span>
                  </span>
                  <span className={`mt-1 flex h-5 w-5 shrink-0 items-center justify-center rounded-md border ${d.on ? "border-sky-400 bg-sky-500" : "border-white/20"}`}>
                    {d.on && <CheckCircle2 size={13} className="text-white" />}
                  </span>
                </button>
              ))}
            </div>
          </section>

          <section className="space-y-3 rounded-2xl border border-white/10 bg-[#12121a] p-4 sm:p-5">
            <SwitchRow
              checked={withButton}
              onChange={(v) => {
                setWithButton(v);
                setError("");
              }}
              title="Tugma qo'shish"
              description="Post ostida havolali inline tugma"
              icon={<Link2 size={18} className={withButton ? "text-sky-400" : "text-gray-500"} />}
            />
            {withButton && (
              <div className="grid gap-3 sm:grid-cols-[180px_1fr]">
                <Field label="Tugma matni" required>
                  <input value={buttonText} onChange={(e) => setButtonText(e.target.value)} placeholder="▶️ Tomosha qilish" className={inputCls} />
                </Field>
                <Field label="Havola" required>
                  <input type="url" value={buttonUrl} onChange={(e) => setButtonUrl(e.target.value)} placeholder="https://filmorauz.net/movies/..." className={inputCls} />
                </Field>
              </div>
            )}
          </section>

          {error && (
            <p className="flex items-center gap-2 rounded-xl border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-300">
              <XCircle size={16} className="shrink-0" /> {error}
            </p>
          )}

          <button
            onClick={send}
            disabled={sending || imageBusy || !text.trim() || (!toChannels && !toBot)}
            className="flex w-full items-center justify-center gap-2 rounded-2xl bg-gradient-to-r from-sky-500 to-blue-600 py-3.5 font-semibold text-white shadow-lg shadow-sky-500/20 transition hover:brightness-110 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {sending ? <Loader2 size={18} className="animate-spin" /> : <Send size={18} />}
            {sending ? "Yuborilmoqda..." : "Postni yuborish"}
          </button>

          {result && (
            <section className="rounded-2xl border border-emerald-500/30 bg-emerald-500/[0.05] p-4">
              <p className="mb-3 flex items-center gap-2 text-sm font-semibold text-emerald-300">
                <CheckCircle2 size={16} /> Natija
              </p>
              <div className="grid gap-3 sm:grid-cols-2">
                {toChannels && <Stat label="Kanallar" icon={Hash} sent={result.channels_sent} blocked={result.channels_blocked} failed={result.channels_failed} />}
                {toBot && <Stat label="Bot foydalanuvchilari" icon={Users} sent={result.bot_sent} blocked={result.bot_blocked} failed={result.bot_failed} />}
              </div>
            </section>
          )}
        </div>

        {/* Telegram-like preview */}
        <aside className="lg:sticky lg:top-6 lg:self-start">
          <div className="mb-2 flex items-center justify-between">
            <p className="text-[11px] font-semibold uppercase tracking-wider text-gray-500">Telegram&apos;da ko&apos;rinishi</p>
            {text.includes("tg-spoiler") && (
              <button onClick={() => setSpoilers((v) => !v)} className="inline-flex items-center gap-1 text-[11px] text-gray-400 hover:text-white">
                {spoilers ? <EyeOff size={12} /> : <Eye size={12} />} spoyler
              </button>
            )}
          </div>
          <div className="overflow-hidden rounded-3xl border border-white/10 bg-[#0e1621] shadow-2xl">
            <div className="flex items-center gap-3 border-b border-black/30 bg-[#17212b] px-4 py-3">
              <span className="flex h-9 w-9 items-center justify-center rounded-full bg-gradient-to-br from-orange-500 to-rose-600 text-sm font-bold text-white">F</span>
              <div>
                <p className="text-sm font-semibold text-white">FilmoraUz</p>
                <p className="text-[11px] text-gray-400">kanal</p>
              </div>
            </div>
            <div className="min-h-[320px] bg-[#0e1621] bg-[radial-gradient(rgba(255,255,255,0.035)_1px,transparent_1px)] p-3 [background-size:16px_16px]">
              {!text.trim() && !imageUrl ? (
                <p className="mt-24 text-center text-sm text-gray-600">Matn yozing — post shu yerda ko&apos;rinadi</p>
              ) : (
                <div className="max-w-[92%]">
                  <div className="overflow-hidden rounded-2xl rounded-bl-md bg-[#182533] shadow">
                    {previewImg && (
                      // eslint-disable-next-line @next/next/no-img-element
                      <img src={previewImg} alt="" className="max-h-72 w-full object-cover" />
                    )}
                    {text.trim() && (
                      <div
                        className={`tg-preview break-words px-3 pt-2 text-[14px] leading-snug text-white ${spoilers ? "tg-show-spoilers" : ""}`}
                        dangerouslySetInnerHTML={{ __html: renderTelegramHtml(text.trim()) }}
                      />
                    )}
                    <p className="flex items-center justify-end gap-1 px-3 pb-1.5 pt-1 text-[11px] text-[#6d7f8f]">
                      <Eye size={11} /> 1 · {pad(now.getHours())}:{pad(now.getMinutes())}
                    </p>
                  </div>
                  {withButton && (
                    <div className="mt-1 rounded-xl bg-[#182533]/80 px-3 py-2 text-center text-[13px] font-medium text-white backdrop-blur">
                      {buttonText || "Tugma"} <span className="text-[#6d7f8f]">↗</span>
                    </div>
                  )}
                </div>
              )}
            </div>
          </div>
        </aside>
      </div>

      {/* History */}
      <section className="mt-10">
        <h2 className="mb-4 flex items-center gap-2 text-lg font-semibold text-white">
          Yuborilgan postlar
          {history.length > 0 && <span className="rounded-full bg-white/5 px-2 py-0.5 text-xs font-normal text-gray-400">{history.length}</span>}
        </h2>
        {loadingHistory ? (
          <div className="grid gap-3 md:grid-cols-2">
            {[0, 1].map((i) => (
              <div key={i} className="h-32 animate-pulse rounded-2xl bg-white/[0.03]" />
            ))}
          </div>
        ) : history.length === 0 ? (
          <p className="rounded-2xl border border-dashed border-white/10 px-4 py-10 text-center text-sm text-gray-500">Hali post yuborilmagan</p>
        ) : (
          <ul className="grid gap-3 md:grid-cols-2">
            {history.map((p) => (
              <li key={p.id} className="flex gap-3 rounded-2xl border border-white/10 bg-[#12121a] p-3.5">
                {p.image_url && (
                  // eslint-disable-next-line @next/next/no-img-element
                  <img src={normalizeMediaUrl(p.image_url)} alt="" className="h-20 w-20 shrink-0 rounded-xl object-cover" />
                )}
                <div className="min-w-0 flex-1">
                  <div className="tg-preview line-clamp-3 break-words text-sm text-gray-200" dangerouslySetInnerHTML={{ __html: renderTelegramHtml(p.text) }} />
                  {p.inline_button_text && (
                    <span className="mt-1.5 inline-flex max-w-full items-center gap-1 truncate rounded-lg border border-sky-500/30 bg-sky-500/10 px-2 py-0.5 text-[11px] text-sky-300">
                      <Link2 size={10} /> {p.inline_button_text}
                    </span>
                  )}
                  <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-gray-500">
                    {p.send_to_channels && (
                      <span className="inline-flex items-center gap-1">
                        <Hash size={11} /> {p.channels_sent_count}
                        {p.channels_failed_count > 0 && <span className="text-red-400">/{p.channels_failed_count} xato</span>}
                      </span>
                    )}
                    {p.send_to_bot_users && (
                      <span className="inline-flex items-center gap-1">
                        <Users size={11} /> {p.bot_sent_count}
                        {p.bot_blocked_count > 0 && <span className="text-amber-400">/{p.bot_blocked_count} bloklagan</span>}
                      </span>
                    )}
                    <span className="inline-flex items-center gap-1">
                      <Clock size={11} /> {fmt(p.sent_at)}
                    </span>
                    {p.sent_by_name && (
                      <span className="inline-flex items-center gap-1">
                        <User size={11} /> {p.sent_by_name}
                      </span>
                    )}
                  </div>
                </div>
                <button onClick={() => reuse(p)} title="Qayta ishlatish" aria-label="Qayta ishlatish" className="self-start rounded-lg p-2 text-gray-500 hover:bg-white/5 hover:text-white">
                  <RotateCcw size={15} />
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
