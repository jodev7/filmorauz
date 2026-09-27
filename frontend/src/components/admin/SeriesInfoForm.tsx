"use client";

import { useMemo, useState } from "react";
import { Crown, Image as ImageIcon, Languages, Sparkles, Tags, Tv } from "lucide-react";
import { CreateSeriesData, uploadSeriesImage } from "@/lib/api";
import { buildSeoTitle, buildSeoDescription } from "@/lib/seo-template";
import { slugify } from "@/lib/slugify";
import { ErrorBanner, Field, FormSection, Segmented, StickySaveBar, SwitchRow, inputCls, useLeaveGuard } from "@/components/admin/form/ui";
import GenrePicker, { normalizeGenre } from "@/components/admin/form/GenrePicker";
import ChipsInput from "@/components/admin/form/ChipsInput";
import SlugInput from "@/components/admin/form/SlugInput";
import MediaUploadField from "@/components/admin/form/MediaUploadField";
import ContentPreviewCard from "@/components/admin/form/ContentPreviewCard";
import DraftBanner from "@/components/admin/form/DraftBanner";
import { useDraft } from "@/components/admin/form/useDraft";
import { COUNTRY_SUGGESTIONS, QUALITIES } from "@/components/admin/form/constants";

export const EMPTY_SERIES: CreateSeriesData = {
  title: "",
  title_uz: "",
  title_ru: "",
  slug: "",
  description: "",
  description_uz: "",
  description_ru: "",
  poster_url: "",
  backdrop_url: "",
  year: new Date().getFullYear(),
  genre: [],
  country: "",
  is_premium: false,
  quality: "1080p",
};

type Lang = "base" | "uz" | "ru";

/**
 * Series metadata form shared by "Yangi serial" and the series editor:
 * sections, live preview + completeness checklist, sticky save bar with
 * Ctrl+S, unsaved-changes guard and (for new series) draft autosave.
 */
export default function SeriesInfoForm({
  initial,
  token,
  mode,
  onSubmit,
  submitLabel = "Saqlash",
  previewHref,
  code,
  seasonsCount,
  episodesCount,
}: {
  initial?: CreateSeriesData;
  token?: string | null;
  mode: "create" | "edit";
  onSubmit: (data: CreateSeriesData) => Promise<void>;
  submitLabel?: string;
  previewHref?: string;
  code?: string;
  seasonsCount?: number;
  episodesCount?: number;
}) {
  const start = useMemo<CreateSeriesData>(
    () => ({ ...EMPTY_SERIES, ...(initial ?? {}), genre: (initial?.genre ?? []).map(normalizeGenre) }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    []
  );
  const [form, setForm] = useState<CreateSeriesData>(start);
  const [saved, setSaved] = useState(JSON.stringify(start));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState({ poster: false, backdrop: false });
  const [lang, setLang] = useState<Lang>("base");

  const set = <K extends keyof CreateSeriesData>(k: K, v: CreateSeriesData[K]) => setForm((p) => ({ ...p, [k]: v }));
  const uploading = busy.poster || busy.backdrop;
  const dirty = JSON.stringify(form) !== saved;
  useLeaveGuard(dirty || uploading);
  const { draft, acceptDraft, discardDraft, clearDraft } = useDraft("filmora_admin_series_draft", form, mode === "create" && dirty);

  const upload = (type: "poster" | "backdrop") => (file: File, onProgress: Parameters<typeof uploadSeriesImage>[3]) => {
    if (!token) return Promise.reject(new Error("Tizimga qayta kiring"));
    return uploadSeriesImage(token, file, type, onProgress);
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    if (uploading) return setError("Rasm yuklanib bo'lishini kuting.");
    if (!form.title.trim()) return setError("Serial nomini kiriting");
    const data: CreateSeriesData = {
      ...form,
      slug: form.slug || slugify(form.title),
      genre: Array.from(new Set((form.genre ?? []).map(normalizeGenre).filter(Boolean))),
    };
    setSaving(true);
    try {
      await onSubmit(data);
      clearDraft();
      setForm(data);
      setSaved(JSON.stringify(data));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Xatolik yuz berdi");
    } finally {
      setSaving(false);
    }
  };

  const titleKey = lang === "uz" ? "title_uz" : lang === "ru" ? "title_ru" : "title";
  const descKey = lang === "uz" ? "description_uz" : lang === "ru" ? "description_ru" : "description";
  const filled = (l: Lang) =>
    l === "base" ? !!form.title : l === "uz" ? !!(form.title_uz || form.description_uz) : !!(form.title_ru || form.description_ru);

  const checklist = [
    { label: "Nomi va yili", ok: !!form.title.trim() && !!form.year },
    { label: "Tavsif (80+ belgi)", ok: (form.description ?? "").trim().length >= 80 },
    { label: "Poster", ok: !!form.poster_url },
    { label: "Backdrop (fon rasm)", ok: !!form.backdrop_url },
    { label: "Kamida 1 ta janr", ok: (form.genre ?? []).length > 0 },
    { label: "O'zbekcha nom", ok: !!form.title_uz },
    ...(mode === "edit" ? [{ label: "Qismlar qo'shilgan", ok: (episodesCount ?? 0) > 0 }] : []),
  ];

  return (
    <>
      <form id="series-form" onSubmit={submit} className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_320px]">
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
            icon={<Tv size={16} />}
            actions={
              <button
                type="button"
                onClick={() =>
                  setForm((p) => ({
                    ...p,
                    title: buildSeoTitle(p.title, p.year ?? 0),
                    description: buildSeoDescription(p.title, p.year ?? 0, p.description ?? ""),
                  }))
                }
                className="inline-flex shrink-0 items-center gap-1.5 rounded-lg border border-orange-500/30 bg-orange-500/10 px-3 py-1.5 text-xs font-medium text-orange-300 hover:bg-orange-500/20"
              >
                <Sparkles size={13} /> SEO shablon
              </button>
            }
          >
            {/* Language tabs for title + description */}
            <div role="tablist" aria-label="Til" className="flex gap-1 border-b border-white/10">
              {(
                [
                  ["base", "Asosiy"],
                  ["uz", "O'zbekcha"],
                  ["ru", "Ruscha"],
                ] as [Lang, string][]
              ).map(([l, label]) => (
                <button
                  key={l}
                  type="button"
                  role="tab"
                  aria-selected={lang === l}
                  onClick={() => setLang(l)}
                  className={`relative flex items-center gap-1.5 px-3 py-2 text-xs font-medium ${lang === l ? "text-white" : "text-gray-500 hover:text-gray-300"}`}
                >
                  {l !== "base" && <Languages size={12} />}
                  {label}
                  {filled(l) && <span className="h-1.5 w-1.5 rounded-full bg-emerald-400" aria-label="to'ldirilgan" />}
                  {lang === l && <span className="absolute inset-x-2 -bottom-px h-0.5 rounded-full bg-orange-500" />}
                </button>
              ))}
            </div>

            <div className="grid gap-4 sm:grid-cols-[1fr_120px]">
              <Field label={lang === "base" ? "Nomi" : lang === "uz" ? "Nomi (o'zbekcha)" : "Nomi (ruscha)"} required={lang === "base"} htmlFor="s-title">
                <input
                  id="s-title"
                  value={(form[titleKey] as string) ?? ""}
                  onChange={(e) => set(titleKey, e.target.value)}
                  placeholder={lang === "ru" ? "Название сериала" : "Serial nomi"}
                  className={inputCls}
                />
              </Field>
              <Field label="Yili" htmlFor="s-year">
                <input
                  id="s-year"
                  type="number"
                  inputMode="numeric"
                  value={form.year || ""}
                  onChange={(e) => set("year", parseInt(e.target.value) || 0)}
                  className={inputCls}
                />
              </Field>
            </div>
            {lang === "base" && (
              <Field label="Slug (havola)" htmlFor="s-slug" hint="Nomdan avtomatik yasaladi">
                <SlugInput id="s-slug" value={form.slug || ""} title={form.title} onChange={(v) => set("slug", v)} prefix="/series/" lockedInitially={mode === "edit"} />
              </Field>
            )}
            <Field
              label={lang === "base" ? "Tavsif" : lang === "uz" ? "Tavsif (o'zbekcha)" : "Tavsif (ruscha)"}
              htmlFor="s-desc"
              right={<span className="text-[11px] text-gray-600">{((form[descKey] as string) ?? "").length} belgi</span>}
            >
              <textarea
                id="s-desc"
                value={(form[descKey] as string) ?? ""}
                onChange={(e) => set(descKey, e.target.value)}
                rows={5}
                placeholder={lang === "ru" ? "Описание…" : "Syujet haqida qisqacha…"}
                className={`${inputCls} resize-y`}
              />
            </Field>
            {code && (
              <p className="text-xs text-gray-500">
                Serial kodi: <span className="font-mono text-gray-300">{code}</span>
              </p>
            )}
          </FormSection>

          <FormSection title="Rasmlar" description="Faylni tashlang, tanlang yoki Ctrl+V bilan qo'ying" icon={<ImageIcon size={16} />}>
            <div className="grid gap-4 sm:grid-cols-[180px_1fr]">
              <MediaUploadField
                label="Poster"
                kind="poster"
                value={form.poster_url ?? ""}
                onChange={(v) => set("poster_url", v)}
                upload={upload("poster")}
                onBusyChange={(b) => setBusy((s) => ({ ...s, poster: b }))}
              />
              <MediaUploadField
                label="Backdrop (fon)"
                kind="backdrop"
                value={form.backdrop_url ?? ""}
                onChange={(v) => set("backdrop_url", v)}
                upload={upload("backdrop")}
                hint="16:9 gorizontal kadr"
                onBusyChange={(b) => setBusy((s) => ({ ...s, backdrop: b }))}
              />
            </div>
          </FormSection>

          <FormSection title="Tasnif" icon={<Tags size={16} />}>
            <Field label="Janrlar">
              <GenrePicker value={form.genre ?? []} onChange={(v) => set("genre", v)} />
            </Field>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="Davlat">
                <ChipsInput
                  value={form.country ? form.country.split(",").map((s) => s.trim()).filter(Boolean) : []}
                  onChange={(v) => set("country", v.join(", "))}
                  placeholder="South Korea"
                  suggestions={COUNTRY_SUGGESTIONS.slice(0, 6)}
                  max={5}
                />
              </Field>
              <Field label="Sifat">
                <Segmented ariaLabel="Sifat" value={form.quality || "1080p"} options={QUALITIES.map((q) => ({ value: q, label: q }))} onChange={(v) => set("quality", v)} />
              </Field>
            </div>
          </FormSection>

          <FormSection title="Kirish" icon={<Crown size={16} />}>
            <SwitchRow
              checked={!!form.is_premium}
              onChange={(v) => set("is_premium", v)}
              title="Premium serial"
              description="Faqat premium obunachilar tomosha qila oladi"
              icon={<Crown size={18} className={form.is_premium ? "text-yellow-400" : "text-gray-500"} />}
            />
          </FormSection>
        </div>

        <aside className="space-y-4 xl:sticky xl:top-6 xl:self-start">
          <ContentPreviewCard
            kindLabel={mode === "edit" && seasonsCount !== undefined ? `Serial · ${seasonsCount} fasl · ${episodesCount ?? 0} qism` : "Serial"}
            title={form.title_uz || form.title}
            year={form.year}
            genres={form.genre ?? []}
            posterUrl={form.poster_url}
            backdropUrl={form.backdrop_url}
            quality={form.quality}
            isPremium={form.is_premium}
            href={previewHref}
            checklist={checklist}
          />
        </aside>
      </form>
      <StickySaveBar formId="series-form" dirty={dirty} saving={saving} disabled={uploading} label={uploading ? "Yuklanmoqda…" : submitLabel} />
    </>
  );
}
