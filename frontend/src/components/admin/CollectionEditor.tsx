"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { ArrowDown, ArrowUp, Check, Eye, Film, Image as ImageIcon, Info, Layers, Plus, Search, Settings2, Star, Tv, X } from "lucide-react";
import { adminGetAllMovies, adminGetSeries, createCollection, getAdminCollectionById, updateCollection, uploadCollectionPoster, AdminSeries, CollectionInput, Movie } from "@/lib/api";
import { normalizeMediaUrl } from "@/lib/image-utils";
import { localizeSingleGenre } from "@/lib/localization";
import { useToast } from "@/components/admin/Toast";
import AdminPageHeader from "@/components/admin/form/PageHeader";
import MediaUploadField from "@/components/admin/form/MediaUploadField";
import SlugInput from "@/components/admin/form/SlugInput";
import { ErrorBanner, Field, FormSection, StickySaveBar, SwitchRow, inputCls, useLeaveGuard } from "@/components/admin/form/ui";
import { SkeletonList, Tabs, Thumb } from "@/components/admin/kit";

type Item = { id: string; title: string; year?: number; genre?: string[]; country?: string; poster_url?: string };

const EMPTY: CollectionInput = { title: "", slug: "", description: "", poster_url: "", is_published: true, is_featured: false, sort_order: 0, movie_ids: [], series_ids: [] };

const splitCountries = (raw?: string) =>
  (raw || "")
    .split(/[,/]/)
    .map((c) => c.trim())
    .filter(Boolean);

/** Search + filter a catalog and pick items; picked ones are shown in order. */
function Picker({ kind, items, selected, onChange }: { kind: "movie" | "series"; items: Item[]; selected: string[]; onChange: (ids: string[]) => void }) {
  const [q, setQ] = useState("");
  const [year, setYear] = useState("");
  const [genre, setGenre] = useState("");
  const [country, setCountry] = useState("");

  const byId = useMemo(() => new Map(items.map((i) => [i.id, i])), [items]);
  const years = useMemo(() => Array.from(new Set(items.map((i) => i.year).filter((y): y is number => !!y && y > 0))).sort((a, b) => b - a), [items]);
  const genres = useMemo(() => Array.from(new Set(items.flatMap((i) => i.genre || []).filter(Boolean))).sort((a, b) => localizeSingleGenre(a).localeCompare(localizeSingleGenre(b))), [items]);
  const countries = useMemo(() => Array.from(new Set(items.flatMap((i) => splitCountries(i.country)))).sort((a, b) => a.localeCompare(b)), [items]);

  const results = useMemo(() => {
    const s = q.trim().toLowerCase();
    const picked = new Set(selected);
    return items
      .filter((i) => !picked.has(i.id))
      .filter((i) => (!s || i.title.toLowerCase().includes(s)) && (!year || i.year === Number(year)) && (!genre || (i.genre || []).includes(genre)) && (!country || splitCountries(i.country).includes(country)))
      .slice(0, 48);
  }, [items, selected, q, year, genre, country]);

  const move = (idx: number, dir: -1 | 1) => {
    const next = [...selected];
    const j = idx + dir;
    if (j < 0 || j >= next.length) return;
    [next[idx], next[j]] = [next[j], next[idx]];
    onChange(next);
  };
  const Icon = kind === "movie" ? Film : Tv;
  const selectCls = "rounded-xl border border-white/10 bg-black/30 px-2.5 py-2 text-xs text-white focus:border-orange-500 focus:outline-none";
  const filtered = !!(q || year || genre || country);

  return (
    <div className="grid gap-5 lg:grid-cols-2">
      {/* Picked, in collection order */}
      <div>
        <p className="mb-2 text-xs font-medium text-gray-400">
          Tanlangan: <span className="text-white">{selected.length}</span> — tartib saytda shunday bo&apos;ladi
        </p>
        {selected.length === 0 ? (
          <div className="flex h-40 flex-col items-center justify-center rounded-xl border border-dashed border-white/10 text-center text-xs text-gray-500">
            <Icon size={20} className="mb-2 text-gray-600" />
            O&apos;ngdagi ro&apos;yxatdan qo&apos;shing
          </div>
        ) : (
          <ol className="max-h-[520px] space-y-1.5 overflow-y-auto pr-1">
            {selected.map((id, idx) => {
              const it = byId.get(id);
              return (
                <li key={id} className="flex items-center gap-2.5 rounded-xl border border-white/10 bg-black/20 p-1.5 pr-2">
                  <span className="w-5 text-center text-[11px] tabular-nums text-gray-500">{idx + 1}</span>
                  <Thumb src={it?.poster_url ? normalizeMediaUrl(it.poster_url) : undefined} className="h-12 w-8 shrink-0 rounded" icon={Icon} />
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm text-white">{it?.title ?? "O'chirilgan kontent"}</p>
                    <p className="truncate text-[11px] text-gray-500">{it?.year || "—"}</p>
                  </div>
                  <button type="button" onClick={() => move(idx, -1)} disabled={idx === 0} className="rounded p-1 text-gray-500 hover:text-white disabled:opacity-25" aria-label="Yuqoriga">
                    <ArrowUp size={14} />
                  </button>
                  <button type="button" onClick={() => move(idx, 1)} disabled={idx === selected.length - 1} className="rounded p-1 text-gray-500 hover:text-white disabled:opacity-25" aria-label="Pastga">
                    <ArrowDown size={14} />
                  </button>
                  <button type="button" onClick={() => onChange(selected.filter((x) => x !== id))} className="rounded p-1 text-gray-500 hover:text-red-400" aria-label="Olib tashlash">
                    <X size={14} />
                  </button>
                </li>
              );
            })}
          </ol>
        )}
      </div>

      {/* Catalog */}
      <div>
        <div className="relative mb-2">
          <Search size={15} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" />
          <input value={q} onChange={(e) => setQ(e.target.value)} placeholder={kind === "movie" ? "Kino qidirish..." : "Serial qidirish..."} className={`${inputCls} pl-9`} />
        </div>
        <div className="mb-2 grid grid-cols-3 gap-1.5">
          <select value={genre} onChange={(e) => setGenre(e.target.value)} className={selectCls} aria-label="Janr">
            <option value="">Janr</option>
            {genres.map((g) => (
              <option key={g} value={g}>
                {localizeSingleGenre(g)}
              </option>
            ))}
          </select>
          <select value={year} onChange={(e) => setYear(e.target.value)} className={selectCls} aria-label="Yil">
            <option value="">Yil</option>
            {years.map((y) => (
              <option key={y} value={y}>
                {y}
              </option>
            ))}
          </select>
          <select value={country} onChange={(e) => setCountry(e.target.value)} className={selectCls} aria-label="Davlat">
            <option value="">Davlat</option>
            {countries.map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </select>
        </div>
        <div className="mb-2 flex items-center justify-between text-[11px] text-gray-500">
          <span>{results.length === 48 ? "48+ ta natija" : `${results.length} ta natija`}</span>
          <span className="flex gap-3">
            {results.length > 0 && filtered && (
              <button type="button" onClick={() => onChange([...selected, ...results.map((r) => r.id)])} className="text-orange-400 hover:text-orange-300">
                Hammasini qo&apos;shish
              </button>
            )}
            {filtered && (
              <button
                type="button"
                onClick={() => {
                  setQ("");
                  setYear("");
                  setGenre("");
                  setCountry("");
                }}
                className="hover:text-white"
              >
                Tozalash
              </button>
            )}
          </span>
        </div>
        <ul className="grid max-h-[460px] grid-cols-3 gap-2 overflow-y-auto pr-1 sm:grid-cols-4">
          {results.map((it) => (
            <li key={it.id}>
              <button type="button" onClick={() => onChange([...selected, it.id])} className="group relative block w-full overflow-hidden rounded-lg text-left" title={it.title}>
                <Thumb src={it.poster_url ? normalizeMediaUrl(it.poster_url) : undefined} alt={it.title} className="aspect-[2/3] w-full" icon={Icon} />
                <span className="absolute inset-0 flex items-center justify-center bg-black/60 opacity-0 transition group-hover:opacity-100">
                  <Plus size={22} className="text-white" />
                </span>
                <span className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/90 to-transparent px-1.5 pb-1 pt-4 text-[10px] leading-tight text-white">
                  <span className="line-clamp-2">{it.title}</span>
                </span>
              </button>
            </li>
          ))}
          {results.length === 0 && <li className="col-span-full py-8 text-center text-xs text-gray-500">Hech narsa topilmadi</li>}
        </ul>
      </div>
    </div>
  );
}

/** Create / edit a collection (shared by /admin/collections/new and /[id]/edit). */
export default function CollectionEditor({ token, collectionId }: { token: string | null; collectionId: string | null }) {
  const router = useRouter();
  const toast = useToast();
  const isNew = !collectionId;
  const [form, setForm] = useState<CollectionInput>(EMPTY);
  const [saved, setSaved] = useState(JSON.stringify(EMPTY));
  const [loading, setLoading] = useState(!isNew);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [uploading, setUploading] = useState(false);
  const [movies, setMovies] = useState<Movie[]>([]);
  const [series, setSeries] = useState<AdminSeries[]>([]);
  const [catalogLoading, setCatalogLoading] = useState(true);
  const [tab, setTab] = useState<"movie" | "series">("movie");

  useEffect(() => {
    if (!token || !collectionId) return;
    getAdminCollectionById(token, collectionId)
      .then((d) => {
        if (!d) return;
        const f: CollectionInput = {
          title: d.title,
          slug: d.slug,
          description: d.description || "",
          poster_url: d.poster_url || "",
          is_published: !!d.is_published,
          is_featured: !!d.is_featured,
          sort_order: d.sort_order || 0,
          movie_ids: d.movie_ids || [],
          series_ids: d.series_ids || [],
        };
        setForm(f);
        setSaved(JSON.stringify(f));
      })
      .catch((e) => setError(e instanceof Error ? e.message : "Yuklab bo'lmadi"))
      .finally(() => setLoading(false));
  }, [token, collectionId]);

  useEffect(() => {
    if (!token) return;
    Promise.all([adminGetAllMovies(token).catch(() => []), adminGetSeries(token).catch(() => [])])
      .then(([m, s]) => {
        setMovies(m || []);
        setSeries(s || []);
      })
      .finally(() => setCatalogLoading(false));
  }, [token]);

  const dirty = JSON.stringify(form) !== saved;
  useLeaveGuard((dirty && !saving) || uploading);
  const set = <K extends keyof CollectionInput>(k: K, v: CollectionInput[K]) => setForm((f) => ({ ...f, [k]: v }));

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token) return;
    if (!form.title.trim() || !form.slug.trim()) return setError("Nomi va slug kerak");
    setSaving(true);
    setError("");
    try {
      if (isNew) await createCollection(token, form);
      else await updateCollection(token, collectionId!, form);
      setSaved(JSON.stringify(form));
      toast.success(isNew ? "To'plam yaratildi" : "To'plam saqlandi");
      router.push("/admin/collections");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Xatolik yuz berdi");
      setSaving(false);
    }
  };

  if (loading) {
    return (
      <div className="mx-auto max-w-6xl p-4 sm:p-6">
        <SkeletonList rows={4} height={140} />
      </div>
    );
  }

  const movieItems: Item[] = movies;
  const seriesItems: Item[] = series;

  return (
    <form id="collection-form" onSubmit={submit} className="mx-auto max-w-6xl p-4 sm:p-6">
      <AdminPageHeader
        backHref="/admin/collections"
        backLabel="To'plamlar"
        title={isNew ? "Yangi to'plam" : form.title || "To'plam"}
        subtitle={<span className="font-mono text-xs">/collections/{form.slug || "…"}</span>}
      />
      <ErrorBanner message={error} />

      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_340px]">
        <div className="min-w-0 space-y-5">
          <FormSection title="Asosiy" icon={<Info size={16} />}>
            <Field label="Nomi" required htmlFor="col-title">
              <input id="col-title" value={form.title} onChange={(e) => set("title", e.target.value)} placeholder="Masalan: Oilaviy kechalar uchun" className={inputCls} />
            </Field>
            <Field label="Havola (slug)" required>
              <SlugInput value={form.slug} title={form.title} onChange={(v) => set("slug", v)} prefix="/collections/" lockedInitially={!isNew} />
            </Field>
            <Field label="Tavsif" hint={`${(form.description || "").length} belgi`}>
              <textarea rows={3} value={form.description} onChange={(e) => set("description", e.target.value)} placeholder="To'plam haqida qisqacha" className={`${inputCls} resize-none`} />
            </Field>
          </FormSection>

          <FormSection title="Tarkib" description={`${form.movie_ids?.length || 0} ta kino · ${form.series_ids?.length || 0} ta serial`} icon={<Layers size={16} />}>
            <Tabs<"movie" | "series">
              value={tab}
              onChange={setTab}
              items={[
                { key: "movie", label: "Kinolar", count: form.movie_ids?.length || 0 },
                { key: "series", label: "Seriallar", count: form.series_ids?.length || 0 },
              ]}
            />
            {catalogLoading ? (
              <SkeletonList rows={3} height={60} />
            ) : tab === "movie" ? (
              <Picker kind="movie" items={movieItems} selected={form.movie_ids || []} onChange={(ids) => set("movie_ids", ids)} />
            ) : (
              <Picker kind="series" items={seriesItems} selected={form.series_ids || []} onChange={(ids) => set("series_ids", ids)} />
            )}
          </FormSection>
        </div>

        <div className="space-y-5">
          <FormSection title="Muqova" icon={<ImageIcon size={16} />}>
            <MediaUploadField
              label="Muqova rasmi"
              kind="backdrop"
              value={form.poster_url || ""}
              onChange={(u) => set("poster_url", u)}
              onBusyChange={setUploading}
              hint="Keng (16:9) rasm yaxshi ko'rinadi"
              upload={(file, onProgress) => {
                if (!token) return Promise.reject(new Error("Tizimga qayta kiring"));
                return uploadCollectionPoster(token, file, onProgress);
              }}
            />
          </FormSection>
          <FormSection title="Sozlamalar" icon={<Settings2 size={16} />}>
            <SwitchRow checked={!!form.is_published} onChange={(v) => set("is_published", v)} title="Saytda ko'rinsin" description="O'chirilsa, faqat adminlar ko'radi" icon={<Eye size={18} className={form.is_published ? "text-yellow-400" : "text-gray-500"} />} />
            <SwitchRow checked={!!form.is_featured} onChange={(v) => set("is_featured", v)} title="Tanlangan" description="Bosh sahifada ajratib ko'rsatiladi" icon={<Star size={18} className={form.is_featured ? "text-yellow-400" : "text-gray-500"} />} />
            <Field label="Tartib raqami" hint="Kichik raqam oldin chiqadi">
              <input type="number" value={form.sort_order} onChange={(e) => set("sort_order", parseInt(e.target.value) || 0)} className={`${inputCls} max-w-[140px]`} />
            </Field>
          </FormSection>
          {(form.movie_ids?.length || 0) + (form.series_ids?.length || 0) === 0 && (
            <p className="flex items-center gap-2 rounded-xl border border-amber-500/20 bg-amber-500/[0.06] px-3 py-2 text-xs text-amber-200">
              <Check size={13} /> Kamida bitta kino yoki serial qo&apos;shing
            </p>
          )}
        </div>
      </div>

      <StickySaveBar formId="collection-form" dirty={dirty} saving={saving} disabled={uploading} label={isNew ? "Yaratish" : "Saqlash"} />
    </form>
  );
}
