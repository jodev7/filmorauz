"use client";

import { useMemo, useState } from "react";
import { Clapperboard, Crown, Film, Image as ImageIcon, Info, Sparkles, Tags, Upload, Video } from "lucide-react";
import {
  MovieInput,
  VideoSourceType,
  directB2Upload,
  backendUploadMovieImage,
  createDirectUploadJob,
  DirectUploadInput,
  IngestionJob,
} from "@/lib/api";
import { buildSeoTitle, buildSeoDescription } from "@/lib/seo-template";
import { logger } from "@/lib/logger";
import { ErrorBanner, Field, FormSection, Segmented, StickySaveBar, SwitchRow, inputCls, useLeaveGuard } from "@/components/admin/form/ui";
import GenrePicker, { normalizeGenre } from "@/components/admin/form/GenrePicker";
import ChipsInput from "@/components/admin/form/ChipsInput";
import SlugInput from "@/components/admin/form/SlugInput";
import MediaUploadField from "@/components/admin/form/MediaUploadField";
import ContentPreviewCard from "@/components/admin/form/ContentPreviewCard";
import DraftBanner from "@/components/admin/form/DraftBanner";
import { useDraft } from "@/components/admin/form/useDraft";
import { COUNTRY_SUGGESTIONS, QUALITIES } from "@/components/admin/form/constants";


const SOURCE_TYPES: { value: VideoSourceType; label: string; description: string; icon: typeof Film }[] = [
  { value: "direct_upload", label: "Fayl yuklash", description: "MP4 yuklanadi, HLS va sifatlarga avtomatik o'giriladi", icon: Upload },
  { value: "iframe_embed", label: "Iframe embed", description: "YouTube, Vimeo va boshqa embed pleyerlar", icon: Film },
  { value: "direct_hls", label: "HLS (.m3u8)", description: "Tayyor adaptiv oqim havolasi", icon: Video },
  { value: "direct_mp4", label: "To'g'ridan MP4", description: "Tayyor .mp4 havola (CDN bloklashi mumkin)", icon: Video },
  { value: "external_restricted", label: "Cheklangan", description: "Tashqi manba — saytda cheklov xabari chiqadi", icon: Info },
];

interface Props {
  initialData?: Partial<MovieInput>;
  onSubmit: (data: MovieInput) => Promise<void>;
  submitLabel?: string;
  token?: string;
  onDirectUploadJobCreated?: (job: IngestionJob) => void;
  /** "create" enables local draft autosave. */
  mode?: "create" | "edit";
  /** Public page link shown in the preview (edit). */
  previewHref?: string;
}

const emptyForm: MovieInput = {
  title: "",
  description: "",
  poster_url: "",
  backdrop_url: "",
  year: new Date().getFullYear(),
  genre: [],
  country: "",
  video_url: "",
  embed_url: "",
  source_type: "iframe_embed",
  duration: 0,
  quality: "1080p",
  is_premium: false,
  slug: "",
  cast: [],
  director: "",
};

function durationLabel(min: number) {
  if (!min) return "";
  const h = Math.floor(min / 60);
  const m = min % 60;
  return h ? `${h} soat ${m ? `${m} daqiqa` : ""}` : `${m} daqiqa`;
}

export default function MovieForm({ initialData, onSubmit, submitLabel = "Saqlash", token, onDirectUploadJobCreated, mode = "edit", previewHref }: Props) {
  const initial = useMemo<MovieInput>(
    () => ({
      ...emptyForm,
      ...(initialData ?? {}),
      genre: Array.isArray(initialData?.genre) ? initialData!.genre.map(normalizeGenre).filter(Boolean) : [],
      cast: initialData?.cast ?? [],
    }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    []
  );
  const [form, setForm] = useState<MovieInput>(initial);
  const [saved, setSaved] = useState(JSON.stringify(initial));
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState({ poster: false, backdrop: false, video: false });
  const [tempFileKey, setTempFileKey] = useState("");
  const [jobCreated, setJobCreated] = useState(false);

  const set = <K extends keyof MovieInput>(field: K, value: MovieInput[K]) => setForm((prev) => ({ ...prev, [field]: value }));
  const isUploading = busy.poster || busy.backdrop || busy.video;
  const dirty = JSON.stringify(form) !== saved;
  useLeaveGuard((dirty && !jobCreated) || isUploading);

  const { draft, acceptDraft, discardDraft, clearDraft } = useDraft("filmora_admin_movie_draft", form, mode === "create" && dirty);

  const applySeoTemplate = () =>
    setForm((prev) => ({
      ...prev,
      title: buildSeoTitle(prev.title, prev.year),
      description: buildSeoDescription(prev.title, prev.year, prev.description),
    }));

  const imageUpload = (type: "poster" | "backdrop") => (file: File, onProgress: Parameters<typeof backendUploadMovieImage>[3]) => {
    if (!token) return Promise.reject(new Error("Tizimga qayta kiring"));
    return backendUploadMovieImage(token, file, type, onProgress);
  };
  const videoUpload = (file: File, onProgress: Parameters<typeof directB2Upload>[3]) => {
    if (!token) return Promise.reject(new Error("Tizimga qayta kiring"));
    return directB2Upload(token, file, "video", onProgress);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    if (isUploading) return setError("Fayl yuklanib bo'lishini kuting.");
    if (!form.title.trim()) return setError("Kino nomini kiriting");
    if (!form.description.trim()) return setError("Tavsif kiriting");
    if (form.source_type === "iframe_embed" && !form.embed_url) return setError("Embed URL kerak");
    if (["direct_mp4", "direct_hls", "external_restricted"].includes(form.source_type) && !form.video_url) return setError("Video URL kerak");

    const genre = Array.from(new Set(form.genre.map(normalizeGenre).filter(Boolean)));
    const cast = (form.cast ?? []).map((s) => s.trim()).filter(Boolean);
    const data: MovieInput = { ...form, genre, cast };

    const directJob = form.source_type === "direct_upload" || (form.source_type === "direct_mp4" && !!tempFileKey);
    setLoading(true);
    try {
      if (directJob) {
        if (!token) throw new Error("Tizimga qayta kiring");
        if (!form.poster_url) throw new Error("Poster yuklash majburiy");
        if (!form.video_url) throw new Error("Avval video faylni yuklang");
        const input: DirectUploadInput = {
          title: form.title,
          temp_file_url: form.video_url,
          temp_file_key: tempFileKey,
          poster_url: form.poster_url,
          backdrop_url: form.backdrop_url,
          year: form.year,
          genres: genre,
          country: form.country,
          duration: form.duration,
          quality: form.quality,
          is_premium: form.is_premium,
        };
        const job = await createDirectUploadJob(token, input);
        setJobCreated(true);
        clearDraft();
        setSaved(JSON.stringify(form));
        onDirectUploadJobCreated?.(job);
      } else {
        await onSubmit(data);
        clearDraft();
        setSaved(JSON.stringify(form));
      }
    } catch (err) {
      logger.error("[MovieForm] submit failed");
      setError(err instanceof Error ? err.message : "Xatolik yuz berdi");
    } finally {
      setLoading(false);
    }
  };

  const showsVideoUpload = form.source_type === "direct_upload" || form.source_type === "direct_mp4" || form.source_type === "direct_hls";
  const needsEmbed = form.source_type === "iframe_embed";
  const needsVideoUrl = form.source_type !== "iframe_embed" && form.source_type !== "direct_upload";

  const checklist = [
    { label: "Nomi va yili", ok: !!form.title.trim() && !!form.year },
    { label: "Tavsif (80+ belgi)", ok: form.description.trim().length >= 80 },
    { label: "Poster", ok: !!form.poster_url },
    { label: "Backdrop (fon rasm)", ok: !!form.backdrop_url },
    { label: "Video manbasi", ok: needsEmbed ? !!form.embed_url : !!form.video_url },
    { label: "Kamida 1 ta janr", ok: form.genre.length > 0 },
    { label: "Aktyorlar yoki rejissyor", ok: (form.cast ?? []).length > 0 || !!form.director },
  ];

  return (
    <>
      <form id="movie-form" onSubmit={handleSubmit} className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_320px]">
        <div className="min-w-0 space-y-6">
          {draft && (
            <DraftBanner
              savedAt={draft.savedAt}
              onRestore={() => {
                setForm(draft.value);
                acceptDraft();
              }}
              onDiscard={discardDraft}
            />
          )}
          <ErrorBanner message={error} />

          <FormSection
            title="Asosiy ma'lumot"
            icon={<Clapperboard size={16} />}
            actions={
              <button
                type="button"
                onClick={applySeoTemplate}
                title="Nom va yildan SEO sarlavha/tavsif yasash"
                className="inline-flex shrink-0 items-center gap-1.5 rounded-lg border border-orange-500/30 bg-orange-500/10 px-3 py-1.5 text-xs font-medium text-orange-300 hover:bg-orange-500/20"
              >
                <Sparkles size={13} /> SEO shablon
              </button>
            }
          >
            <div className="grid gap-4 sm:grid-cols-[1fr_120px]">
              <Field label="Nomi" required htmlFor="m-title">
                <input id="m-title" value={form.title} onChange={(e) => set("title", e.target.value)} placeholder="Masalan: Interstellar" className={inputCls} />
              </Field>
              <Field label="Yili" required htmlFor="m-year">
                <input
                  id="m-year"
                  type="number"
                  inputMode="numeric"
                  value={form.year || ""}
                  onChange={(e) => set("year", parseInt(e.target.value) || 0)}
                  min={1900}
                  max={2100}
                  className={inputCls}
                />
              </Field>
            </div>
            <Field label="Slug (havola)" htmlFor="m-slug" hint="Nomdan avtomatik yasaladi. Qo'lda o'zgartirsangiz, avto-rejim o'chadi.">
              <SlugInput id="m-slug" value={form.slug || ""} title={form.title} onChange={(v) => set("slug", v)} prefix="/movies/" lockedInitially={mode === "edit"} />
            </Field>
            <Field label="Tavsif" required htmlFor="m-desc" right={<span className="text-[11px] text-gray-600">{form.description.length} belgi</span>}>
              <textarea id="m-desc" value={form.description} onChange={(e) => set("description", e.target.value)} rows={5} placeholder="Syujet haqida qisqacha…" className={`${inputCls} resize-y`} />
            </Field>
          </FormSection>

          <FormSection title="Rasmlar" description="Faylni tashlang, tanlang yoki Ctrl+V bilan qo'ying" icon={<ImageIcon size={16} />}>
            <div className="grid gap-4 sm:grid-cols-[180px_1fr]">
              <MediaUploadField
                label="Poster"
                required
                kind="poster"
                value={form.poster_url}
                onChange={(v) => set("poster_url", v)}
                upload={imageUpload("poster")}
                onBusyChange={(b) => setBusy((s) => ({ ...s, poster: b }))}
              />
              <MediaUploadField
                label="Backdrop (fon)"
                kind="backdrop"
                value={form.backdrop_url}
                onChange={(v) => set("backdrop_url", v)}
                upload={imageUpload("backdrop")}
                hint="16:9 gorizontal kadr — kino sahifasi tepasida chiqadi"
                onBusyChange={(b) => setBusy((s) => ({ ...s, backdrop: b }))}
              />
            </div>
          </FormSection>

          <FormSection title="Video" description="Kino qayerdan ijro etiladi" icon={<Video size={16} />}>
            <div role="radiogroup" aria-label="Video manbasi" className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
              {SOURCE_TYPES.map(({ value, label, description, icon: Icon }) => {
                const on = form.source_type === value;
                return (
                  <button
                    key={value}
                    type="button"
                    role="radio"
                    aria-checked={on}
                    onClick={() => set("source_type", value)}
                    className={`flex items-start gap-3 rounded-xl border p-3 text-left transition-colors ${
                      on ? "border-orange-500 bg-orange-500/10" : "border-white/10 hover:border-white/25"
                    }`}
                  >
                    <Icon size={16} className={on ? "mt-0.5 text-orange-400" : "mt-0.5 text-gray-500"} />
                    <span>
                      <span className="block text-sm font-medium text-white">{label}</span>
                      <span className="block text-[11px] leading-snug text-gray-500">{description}</span>
                    </span>
                  </button>
                );
              })}
            </div>

            {needsEmbed && (
              <Field label="Embed URL" required htmlFor="m-embed" hint="YouTube uchun /embed/ ko'rinishidagi havola">
                <input id="m-embed" type="url" value={form.embed_url} onChange={(e) => set("embed_url", e.target.value.trim())} placeholder="https://www.youtube.com/embed/…" className={inputCls} />
              </Field>
            )}
            {showsVideoUpload && (
              <MediaUploadField
                label={form.source_type === "direct_upload" ? "Video fayl (qayta ishlash uchun)" : "Video fayl"}
                required={form.source_type === "direct_upload"}
                kind="video"
                value={form.video_url}
                onChange={(v) => set("video_url", v)}
                upload={videoUpload}
                onUploaded={(r) => setTempFileKey(r.file_key || "")}
                onBusyChange={(b) => setBusy((s) => ({ ...s, video: b }))}
                hint={
                  form.source_type === "direct_upload"
                    ? "Saqlagach navbatga qo'yiladi va HLS/sifatlarga o'giriladi. Yuklash davomida sahifani yopmang."
                    : undefined
                }
              />
            )}
            {needsVideoUrl && (
              <Field label="yoki video havolasi" htmlFor="m-video">
                <input id="m-video" type="url" value={form.video_url} onChange={(e) => set("video_url", e.target.value.trim())} placeholder="https://…/video.m3u8" className={inputCls} />
              </Field>
            )}
          </FormSection>

          <FormSection title="Tasnif" icon={<Tags size={16} />}>
            <Field label="Janrlar">
              <GenrePicker value={form.genre} onChange={(v) => set("genre", v)} />
            </Field>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="Davlat" hint="Bir nechta bo'lsa vergul bilan">
                <ChipsInput
                  value={form.country ? form.country.split(",").map((s) => s.trim()).filter(Boolean) : []}
                  onChange={(v) => set("country", v.join(", "))}
                  placeholder="USA"
                  suggestions={COUNTRY_SUGGESTIONS.slice(0, 6)}
                  max={5}
                />
              </Field>
              <Field label="Davomiyligi (daqiqa)" htmlFor="m-dur" hint={durationLabel(form.duration)}>
                <input id="m-dur" type="number" inputMode="numeric" min={0} value={form.duration || ""} onChange={(e) => set("duration", parseInt(e.target.value) || 0)} placeholder="120" className={inputCls} />
              </Field>
            </div>
            <Field label="Sifat">
              <Segmented ariaLabel="Sifat" value={form.quality || "1080p"} options={QUALITIES.map((q) => ({ value: q, label: q }))} onChange={(v) => set("quality", v)} />
            </Field>
            <div className="grid gap-4 sm:grid-cols-[1fr_220px]">
              <Field label="Aktyorlar" hint="Enter yoki vergul bilan; ro'yxatni birdan qo'yish ham mumkin">
                <ChipsInput value={form.cast ?? []} onChange={(v) => set("cast", v)} placeholder="Tom Hanks, Emma Watson…" />
              </Field>
              <Field label="Rejissyor" htmlFor="m-dir">
                <input id="m-dir" value={form.director ?? ""} onChange={(e) => set("director", e.target.value)} placeholder="Christopher Nolan" className={inputCls} />
              </Field>
            </div>
          </FormSection>

          <FormSection title="Kirish" icon={<Crown size={16} />}>
            <SwitchRow
              checked={!!form.is_premium}
              onChange={(v) => set("is_premium", v)}
              title="Premium kino"
              description="Faqat premium obunachilar tomosha qila oladi"
              icon={<Crown size={18} className={form.is_premium ? "text-yellow-400" : "text-gray-500"} />}
            />
          </FormSection>
        </div>

        <aside className="space-y-4 xl:sticky xl:top-6 xl:self-start">
          <ContentPreviewCard
            kindLabel="Kino"
            title={form.title}
            year={form.year}
            genres={form.genre}
            posterUrl={form.poster_url}
            backdropUrl={form.backdrop_url}
            quality={form.quality}
            isPremium={form.is_premium}
            href={previewHref}
            checklist={checklist}
          />
        </aside>
      </form>

      <StickySaveBar
        formId="movie-form"
        dirty={dirty}
        saving={loading}
        disabled={isUploading || jobCreated}
        label={isUploading ? "Yuklanmoqda…" : submitLabel}
        status={
          jobCreated ? (
            <span className="text-emerald-400">Qayta ishlash navbatiga qo&apos;yildi — holatini &quot;Import&quot; bo&apos;limida kuzating.</span>
          ) : undefined
        }
      />
    </>
  );
}
