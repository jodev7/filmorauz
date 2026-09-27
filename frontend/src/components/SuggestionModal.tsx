"use client";

import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import Link from "next/link";
import { X, Loader2, Film, Tv, Link2, ImagePlus, Lightbulb, CheckCircle2, Send, Search, Bell, Sparkles } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { createSuggestion, searchMovies, Movie, SuggestionFormData } from "@/lib/api";
import { getLocalizedTitle } from "@/lib/localization";
import { DEFAULT_POSTER_PLACEHOLDER } from "@/lib/image-utils";
import MediaImage from "@/components/ui/MediaImage";
import TelegramLoginModal from "@/components/TelegramLoginModal";

interface SuggestionModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess?: () => void;
  initialTitle?: string;
  initialType?: "movie" | "series";
}

type Found = Movie & { target_type?: string };

const EMPTY: SuggestionFormData = { type: "movie", title: "", message: "", source_url: "" };

/**
 * "Kino tavsiya qilish": a bottom sheet on phones, a dialog on desktop.
 * Checks while typing whether the title is already on the site, so users
 * find it instead of requesting a duplicate.
 */
export default function SuggestionModal({ isOpen, onClose, onSuccess, initialTitle, initialType }: SuggestionModalProps) {
  const { token, isAuthenticated } = useAuth();
  const [form, setForm] = useState<SuggestionFormData>(EMPTY);
  const [year, setYear] = useState("");
  const [loading, setLoading] = useState(false);
  const [success, setSuccess] = useState(false);
  const [error, setError] = useState("");
  const [image, setImage] = useState<File | null>(null);
  const [preview, setPreview] = useState<string | null>(null);
  const [found, setFound] = useState<Found[]>([]);
  const [checking, setChecking] = useState(false);
  const [showExtra, setShowExtra] = useState(false);
  const [loginOpen, setLoginOpen] = useState(false);
  const [mounted, setMounted] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);
  const titleRef = useRef<HTMLInputElement>(null);

  useEffect(() => setMounted(true), []);

  // Reset / prefill each time it opens.
  useEffect(() => {
    if (!isOpen) return;
    setForm({ ...EMPTY, title: initialTitle ?? "", type: initialType ?? "movie" });
    setYear("");
    setSuccess(false);
    setError("");
    setShowExtra(false);
    setImage(null);
    setPreview(null);
    const t = setTimeout(() => titleRef.current?.focus(), 150);
    document.body.style.overflow = "hidden";
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    document.addEventListener("keydown", onKey);
    return () => {
      clearTimeout(t);
      document.body.style.overflow = "";
      document.removeEventListener("keydown", onKey);
    };
  }, [isOpen, initialTitle, initialType, onClose]);

  useEffect(() => () => void (preview && URL.revokeObjectURL(preview)), [preview]);

  // "Saytda bormi?" — debounced lookup of the typed title.
  useEffect(() => {
    if (!isOpen) return;
    const q = form.title.trim();
    if (q.length < 3) {
      setFound([]);
      return;
    }
    setChecking(true);
    let active = true;
    const t = setTimeout(() => {
      searchMovies(q)
        .then((r) => active && setFound(((r || []) as Found[]).slice(0, 3)))
        .catch(() => active && setFound([]))
        .finally(() => active && setChecking(false));
    }, 400);
    return () => {
      active = false;
      clearTimeout(t);
    };
  }, [form.title, isOpen]);

  const pickImage = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    if (!["image/jpeg", "image/jpg", "image/png", "image/webp", "image/gif"].includes(file.type)) {
      setError("Faqat JPG, PNG, WebP yoki GIF rasm yuklang");
      return;
    }
    const max = file.type === "image/gif" ? 20 : 10;
    if (file.size > max * 1024 * 1024) {
      setError(`Rasm hajmi ${max}MB dan oshmasin`);
      return;
    }
    setImage(file);
    setPreview(URL.createObjectURL(file));
    setError("");
  };

  const clearImage = () => {
    setImage(null);
    setPreview(null);
    if (fileRef.current) fileRef.current.value = "";
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!isAuthenticated || !token) {
      setLoginOpen(true);
      return;
    }
    const title = form.title.trim();
    if (!title) {
      setError("Kino yoki serial nomini yozing");
      titleRef.current?.focus();
      return;
    }
    const parts = [form.message.trim()];
    if (year.trim()) parts.push(`Yili: ${year.trim()}`);
    const message = parts.filter(Boolean).join("\n") || "Saytga qo'shishni so'rayman.";
    setLoading(true);
    setError("");
    try {
      await createSuggestion(token, { ...form, title, message, image: image || undefined });
      setSuccess(true);
      onSuccess?.();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Xatolik yuz berdi");
    } finally {
      setLoading(false);
    }
  };

  if (!isOpen || !mounted) return null;

  const typeBtn = (value: "movie" | "series", label: string, Icon: React.ElementType) => (
    <button
      type="button"
      role="radio"
      aria-checked={form.type === value}
      onClick={() => setForm({ ...form, type: value })}
      className={`flex flex-1 items-center justify-center gap-2 rounded-lg py-2.5 text-sm font-medium transition-colors ${
        form.type === value ? "bg-orange-500 text-white shadow" : "text-gray-400 hover:text-white"
      }`}
    >
      <Icon size={16} /> {label}
    </button>
  );

  const content = (
    <div className="fixed inset-0 z-[9999] flex items-end justify-center sm:items-center sm:p-4" role="dialog" aria-modal="true" aria-labelledby="suggest-title">
      <div className="absolute inset-0 bg-black/75 backdrop-blur-sm animate-[fadeIn_.15s_ease-out]" onClick={onClose} />
      <div className="relative flex max-h-[92dvh] w-full flex-col overflow-hidden rounded-t-3xl border border-white/10 bg-[#101018] shadow-2xl animate-[sheetUp_.22s_ease-out] sm:max-w-lg sm:rounded-3xl">
        {/* Drag handle (phones) */}
        <div className="flex justify-center pt-2.5 sm:hidden" aria-hidden="true">
          <span className="h-1.5 w-10 rounded-full bg-white/20" />
        </div>

        {/* Header */}
        <div className="relative overflow-hidden px-5 pb-4 pt-3 sm:px-6 sm:pt-6">
          <div className="pointer-events-none absolute -right-10 -top-16 h-40 w-40 rounded-full bg-orange-500/20 blur-3xl" aria-hidden="true" />
          <button onClick={onClose} className="absolute right-4 top-3 rounded-full p-2 text-gray-400 hover:bg-white/5 hover:text-white sm:top-5" aria-label="Yopish">
            <X size={20} />
          </button>
          <div className="flex items-center gap-3">
            <span className="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl bg-gradient-to-br from-yellow-400 to-orange-600 shadow-lg shadow-orange-500/30">
              <Lightbulb size={22} className="text-white" />
            </span>
            <div className="min-w-0 pr-8">
              <h2 id="suggest-title" className="text-lg font-bold text-white">Kino tavsiya qilish</h2>
              <p className="text-sm text-gray-400">Saytda yo&apos;q kinoni yozing — biz qo&apos;shamiz</p>
            </div>
          </div>
        </div>

        <div className="flex-1 overflow-y-auto overscroll-contain px-5 pb-5 sm:px-6 sm:pb-6">
          {success ? (
            <div className="py-6 text-center">
              <div className="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-emerald-500/15">
                <CheckCircle2 className="h-9 w-9 text-emerald-400" />
              </div>
              <h3 className="text-xl font-bold text-white">Rahmat! Tavsiya yuborildi</h3>
              <p className="mx-auto mt-2 max-w-xs text-sm text-gray-400">
                &quot;{form.title.trim()}&quot; qo&apos;shilsa, sizga saytda va Telegram&apos;da xabar beramiz.
              </p>
              <div className="mt-6 flex flex-col gap-2 sm:flex-row sm:justify-center">
                <button
                  onClick={() => {
                    setSuccess(false);
                    setForm({ ...EMPTY, type: form.type });
                    setYear("");
                    clearImage();
                    setTimeout(() => titleRef.current?.focus(), 50);
                  }}
                  className="rounded-xl border border-white/10 px-5 py-2.5 text-sm text-white hover:bg-white/5"
                >
                  Yana tavsiya qilish
                </button>
                <button onClick={onClose} className="rounded-xl bg-orange-500 px-5 py-2.5 text-sm font-semibold text-white hover:bg-orange-600">
                  Tayyor
                </button>
              </div>
            </div>
          ) : (
            <form onSubmit={submit} className="space-y-4">
              {/* How it works */}
              <ol className="grid grid-cols-3 gap-2 text-center text-[11px] text-gray-400">
                {[
                  { icon: Send, text: "Siz yuborasiz" },
                  { icon: Search, text: "Admin ko'rib chiqadi" },
                  { icon: Bell, text: "Qo'shilsa xabar beramiz" },
                ].map(({ icon: Icon, text }, i) => (
                  <li key={text} className="flex flex-col items-center gap-1 rounded-xl bg-white/[0.03] px-1 py-2">
                    <span className="flex h-7 w-7 items-center justify-center rounded-full bg-orange-500/15 text-orange-400">
                      <Icon size={14} />
                    </span>
                    <span>
                      {i + 1}. {text}
                    </span>
                  </li>
                ))}
              </ol>

              {/* Type */}
              <div role="radiogroup" aria-label="Tur" className="flex gap-1 rounded-xl border border-white/10 bg-black/30 p-1">
                {typeBtn("movie", "Kino", Film)}
                {typeBtn("series", "Serial", Tv)}
              </div>

              {/* Title + year */}
              <div className="grid grid-cols-[1fr_88px] gap-2">
                <div>
                  <label htmlFor="suggest-name" className="mb-1.5 block text-xs font-medium text-gray-400">
                    Nomi <span className="text-orange-400">*</span>
                  </label>
                  <input
                    id="suggest-name"
                    ref={titleRef}
                    value={form.title}
                    onChange={(e) => setForm({ ...form, title: e.target.value })}
                    maxLength={120}
                    placeholder={form.type === "series" ? "Masalan: Dune: Prophecy" : "Masalan: Interstellar"}
                    className="w-full rounded-xl border border-white/10 bg-black/30 px-4 py-3 text-white placeholder-gray-600 focus:border-orange-500 focus:outline-none"
                  />
                </div>
                <div>
                  <label htmlFor="suggest-year" className="mb-1.5 block text-xs font-medium text-gray-400">
                    Yili
                  </label>
                  <input
                    id="suggest-year"
                    value={year}
                    onChange={(e) => setYear(e.target.value.replace(/\D/g, "").slice(0, 4))}
                    inputMode="numeric"
                    placeholder="2024"
                    className="w-full rounded-xl border border-white/10 bg-black/30 px-3 py-3 text-center text-white placeholder-gray-600 focus:border-orange-500 focus:outline-none"
                  />
                </div>
              </div>

              {/* Already on the site? */}
              {(checking || found.length > 0) && form.title.trim().length >= 3 && (
                <div className="rounded-xl border border-sky-500/25 bg-sky-500/[0.07] p-3" aria-live="polite">
                  {checking && found.length === 0 ? (
                    <p className="flex items-center gap-2 text-xs text-sky-200">
                      <Loader2 size={13} className="animate-spin" /> Saytda bor-yo&apos;qligini tekshiryapmiz…
                    </p>
                  ) : (
                    <>
                      <p className="mb-2 flex items-center gap-1.5 text-xs font-medium text-sky-200">
                        <Sparkles size={13} /> Balki shuni qidiryapsiz? Bular saytda allaqachon bor:
                      </p>
                      <ul className="space-y-1">
                        {found.map((m) => (
                          <li key={m.id}>
                            <Link
                              href={m.target_type === "series" ? `/series/${m.slug}` : `/movies/${m.slug}`}
                              onClick={onClose}
                              className="flex items-center gap-3 rounded-lg p-1.5 hover:bg-white/5"
                            >
                              <MediaImage src={m.poster_url} alt="" fallbackSrc={DEFAULT_POSTER_PLACEHOLDER} className="h-11 w-8 shrink-0 rounded object-cover" />
                              <span className="min-w-0 flex-1">
                                <span className="line-clamp-1 text-sm text-white">{getLocalizedTitle(m)}</span>
                                <span className="text-[11px] text-gray-400">
                                  {m.year} · {m.target_type === "series" ? "Serial" : "Kino"}
                                </span>
                              </span>
                              <span className="shrink-0 text-xs text-sky-300">Ko&apos;rish →</span>
                            </Link>
                          </li>
                        ))}
                      </ul>
                    </>
                  )}
                </div>
              )}

              {/* Message */}
              <div>
                <label htmlFor="suggest-msg" className="mb-1.5 block text-xs font-medium text-gray-400">
                  Izoh <span className="text-gray-600">(ixtiyoriy)</span>
                </label>
                <textarea
                  id="suggest-msg"
                  value={form.message}
                  onChange={(e) => setForm({ ...form, message: e.target.value })}
                  rows={2}
                  maxLength={1000}
                  placeholder="Qaysi tilda, qaysi fasl, aktyorlar… — admin topishiga yordam beradi"
                  className="w-full resize-none rounded-xl border border-white/10 bg-black/30 px-4 py-3 text-sm text-white placeholder-gray-600 focus:border-orange-500 focus:outline-none"
                />
              </div>

              {/* Optional link + image */}
              {!showExtra && !form.source_url && !preview ? (
                <button type="button" onClick={() => setShowExtra(true)} className="text-sm text-orange-400 hover:underline">
                  + Havola yoki rasm qo&apos;shish
                </button>
              ) : (
                <div className="space-y-3">
                  <div className="relative">
                    <Link2 className="absolute left-3.5 top-1/2 -translate-y-1/2 text-gray-500" size={16} />
                    <input
                      type="url"
                      value={form.source_url}
                      onChange={(e) => setForm({ ...form, source_url: e.target.value })}
                      placeholder="IMDb, Kinopoisk yoki boshqa havola"
                      className="w-full rounded-xl border border-white/10 bg-black/30 py-3 pl-10 pr-4 text-sm text-white placeholder-gray-600 focus:border-orange-500 focus:outline-none"
                    />
                  </div>
                  {preview ? (
                    <div className="relative">
                      <MediaImage src={preview} alt="Tanlangan rasm" className="max-h-40 w-full rounded-xl border border-white/10 bg-black/30 object-contain" />
                      <button type="button" onClick={clearImage} className="absolute right-2 top-2 rounded-full bg-black/70 p-1.5 text-white hover:bg-black" aria-label="Rasmni olib tashlash">
                        <X size={14} />
                      </button>
                    </div>
                  ) : (
                    <label className="flex cursor-pointer items-center gap-3 rounded-xl border border-dashed border-white/15 px-4 py-3 text-sm text-gray-400 transition-colors hover:border-orange-500/50 hover:text-gray-200">
                      <input ref={fileRef} type="file" accept="image/jpeg,image/jpg,image/png,image/webp,image/gif" onChange={pickImage} className="hidden" />
                      <ImagePlus size={18} className="text-gray-500" />
                      Poster yoki skrinshot (ixtiyoriy)
                    </label>
                  )}
                </div>
              )}

              {error && (
                <p className="rounded-xl border border-red-500/30 bg-red-500/10 px-3 py-2 text-sm text-red-300" role="alert">
                  {error}
                </p>
              )}

              <div className="sticky bottom-0 -mx-5 -mb-5 space-y-2 border-t border-white/5 bg-[#101018]/95 px-5 pb-[max(1.25rem,env(safe-area-inset-bottom))] pt-3 backdrop-blur sm:-mx-6 sm:-mb-6 sm:px-6">
              <button
                type="submit"
                disabled={loading || !form.title.trim()}
                className="flex w-full items-center justify-center gap-2 rounded-xl bg-gradient-to-r from-orange-500 to-orange-600 py-3.5 font-semibold text-white shadow-lg shadow-orange-500/20 transition hover:from-orange-400 hover:to-orange-600 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {loading ? <Loader2 className="h-5 w-5 animate-spin" /> : <Send size={18} />}
                {loading ? "Yuborilmoqda…" : isAuthenticated ? "Tavsiyani yuborish" : "Kirish va yuborish"}
              </button>
              {!isAuthenticated && <p className="text-center text-xs text-gray-500">Yuborish uchun Telegram orqali kirasiz</p>}
              </div>
            </form>
          )}
        </div>
      </div>
      <TelegramLoginModal isOpen={loginOpen} onClose={() => setLoginOpen(false)} />
    </div>
  );

  return createPortal(content, document.body);
}
