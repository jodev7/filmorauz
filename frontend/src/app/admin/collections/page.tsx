"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { ExternalLink, Eye, EyeOff, Film, Layers, Pencil, Plus, Star, Trash2, Tv } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { getAdminCollections, deleteCollection, updateCollection, CollectionInput } from "@/lib/api";
import { normalizeMediaUrl } from "@/lib/image-utils";
import { useToast } from "@/components/admin/Toast";
import { Chip, EmptyState, IconBtn, PageHead, SearchBox, SkeletonList, StatTile, Tabs, Thumb } from "@/components/admin/kit";

type Filter = "all" | "published" | "draft" | "featured";

export default function AdminCollectionsPage() {
  const { token } = useAuth();
  const toast = useToast();
  const [collections, setCollections] = useState<CollectionInput[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState<Filter>("all");
  const [busy, setBusy] = useState<string | null>(null);

  useEffect(() => {
    if (!token) return;
    getAdminCollections(token)
      .then((d) => setCollections(d || []))
      .catch((e) => toast.error(e instanceof Error ? e.message : "Yuklab bo'lmadi"))
      .finally(() => setLoading(false));
  }, [token, toast]);

  const counts = useMemo(
    () => ({
      all: collections.length,
      published: collections.filter((c) => c.is_published).length,
      draft: collections.filter((c) => !c.is_published).length,
      featured: collections.filter((c) => c.is_featured).length,
    }),
    [collections]
  );

  const visible = useMemo(() => {
    const q = search.trim().toLowerCase();
    return collections
      .filter((c) => (filter === "all" ? true : filter === "published" ? c.is_published : filter === "draft" ? !c.is_published : c.is_featured))
      .filter((c) => !q || c.title.toLowerCase().includes(q) || c.slug.toLowerCase().includes(q))
      .sort((a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0));
  }, [collections, filter, search]);

  const toggle = async (c: CollectionInput, key: "is_published" | "is_featured") => {
    if (!token || !c.id) return;
    setBusy(c.id);
    try {
      const next = { ...c, [key]: !c[key] };
      await updateCollection(token, c.id, next);
      setCollections((list) => list.map((x) => (x.id === c.id ? next : x)));
      toast.success(key === "is_published" ? (next.is_published ? "Saytga chiqarildi" : "Saytdan yashirildi") : next.is_featured ? "Tanlanganlarga qo'shildi" : "Tanlanganlardan olindi");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Xatolik");
    } finally {
      setBusy(null);
    }
  };

  const remove = async (c: CollectionInput) => {
    if (!token || !c.id || !confirm(`«${c.title}» to'plamini o'chirasizmi? Bu amalni qaytarib bo'lmaydi.`)) return;
    setBusy(c.id);
    try {
      await deleteCollection(token, c.id);
      setCollections((list) => list.filter((x) => x.id !== c.id));
      toast.success("To'plam o'chirildi");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "O'chirishda xato");
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="mx-auto max-w-7xl p-4 sm:p-6">
      <PageHead
        icon={Layers}
        gradient="from-pink-500 to-rose-700"
        title="To'plamlar"
        subtitle="Mavzuli kino va serial to'plamlari"
        actions={
          <Link href="/admin/collections/new" className="inline-flex items-center gap-2 rounded-xl bg-orange-500 px-4 py-2.5 text-sm font-semibold text-white shadow-lg shadow-orange-500/20 hover:bg-orange-400">
            <Plus size={16} /> Yangi to&apos;plam
          </Link>
        }
      />

      <div className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatTile icon={Layers} tone="violet" label="Jami" value={counts.all} active={filter === "all"} onClick={() => setFilter("all")} />
        <StatTile icon={Eye} tone="green" label="Saytda" value={counts.published} active={filter === "published"} onClick={() => setFilter("published")} />
        <StatTile icon={EyeOff} tone="gray" label="Yashirin" value={counts.draft} active={filter === "draft"} onClick={() => setFilter("draft")} />
        <StatTile icon={Star} tone="yellow" label="Tanlangan" value={counts.featured} active={filter === "featured"} onClick={() => setFilter("featured")} />
      </div>

      <div className="mb-4 flex flex-col gap-3 lg:flex-row">
        <SearchBox value={search} onChange={setSearch} placeholder="To'plam nomi yoki slug..." />
        <Tabs<Filter>
          value={filter}
          onChange={setFilter}
          items={[
            { key: "all", label: "Hammasi", count: counts.all },
            { key: "published", label: "Saytda", count: counts.published },
            { key: "draft", label: "Yashirin", count: counts.draft },
            { key: "featured", label: "Tanlangan", count: counts.featured },
          ]}
        />
      </div>

      {loading ? (
        <SkeletonList rows={4} height={120} />
      ) : visible.length === 0 ? (
        <EmptyState
          icon={Layers}
          title="To'plam topilmadi"
          text={collections.length ? "Qidiruv yoki filtrni o'zgartirib ko'ring." : "Masalan: «Oilaviy kechalar uchun», «Eng yaxshi Marvel kinolari»."}
          action={
            !collections.length ? (
              <Link href="/admin/collections/new" className="inline-flex items-center gap-2 rounded-xl bg-white/10 px-4 py-2 text-sm text-white hover:bg-white/15">
                <Plus size={15} /> To&apos;plam yaratish
              </Link>
            ) : undefined
          }
        />
      ) : (
        <ul className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {visible.map((c) => {
            const movies = c.movie_ids?.length || 0;
            const series = c.series_ids?.length || 0;
            const isBusy = busy === c.id;
            return (
              <li key={c.id} className={`group flex flex-col overflow-hidden rounded-2xl border border-white/10 bg-[#12121a] transition hover:border-white/20 ${isBusy ? "opacity-60" : ""}`}>
                <Link href={`/admin/collections/${c.id}/edit`} className="relative block aspect-[16/7] overflow-hidden bg-gradient-to-br from-pink-500/20 via-violet-500/10 to-transparent">
                  <Thumb src={c.poster_url ? normalizeMediaUrl(c.poster_url) : undefined} alt={c.title} className="h-full w-full transition group-hover:scale-[1.03]" icon={Layers} />
                  <div className="absolute inset-0 bg-gradient-to-t from-[#12121a] via-transparent to-transparent" />
                  <div className="absolute left-3 top-3 flex gap-1.5">
                    <Chip tone={c.is_published ? "green" : "gray"} dot>
                      {c.is_published ? "Saytda" : "Yashirin"}
                    </Chip>
                    {c.is_featured && (
                      <Chip tone="yellow" icon={Star}>
                        Tanlangan
                      </Chip>
                    )}
                  </div>
                </Link>
                <div className="flex flex-1 flex-col p-4 pt-2">
                  <Link href={`/admin/collections/${c.id}/edit`} className="truncate font-semibold text-white hover:text-orange-300">
                    {c.title}
                  </Link>
                  <p className="truncate font-mono text-[11px] text-gray-600">/{c.slug}</p>
                  {c.description && <p className="mt-1.5 line-clamp-2 text-sm text-gray-400">{c.description}</p>}
                  <div className="mt-auto flex items-center gap-3 pt-3 text-xs text-gray-400">
                    <span className="inline-flex items-center gap-1">
                      <Film size={12} /> {movies} kino
                    </span>
                    {series > 0 && (
                      <span className="inline-flex items-center gap-1">
                        <Tv size={12} /> {series} serial
                      </span>
                    )}
                    <span className="text-gray-600">#{c.sort_order ?? 0}</span>
                    <div className="ml-auto flex items-center">
                      <IconBtn label={c.is_published ? "Saytdan yashirish" : "Saytga chiqarish"} tone="green" disabled={isBusy} onClick={() => toggle(c, "is_published")}>
                        {c.is_published ? <EyeOff size={15} /> : <Eye size={15} />}
                      </IconBtn>
                      <IconBtn label={c.is_featured ? "Tanlanganlardan olish" : "Tanlanganlarga qo'shish"} tone="yellow" disabled={isBusy} onClick={() => toggle(c, "is_featured")}>
                        <Star size={15} className={c.is_featured ? "fill-yellow-400 text-yellow-400" : ""} />
                      </IconBtn>
                      <Link href={`/collections/${c.slug}`} target="_blank" className="rounded-lg p-2 text-gray-400 hover:bg-white/5 hover:text-white" title="Saytda ko'rish" aria-label="Saytda ko'rish">
                        <ExternalLink size={15} />
                      </Link>
                      <Link href={`/admin/collections/${c.id}/edit`} className="rounded-lg p-2 text-gray-400 hover:bg-white/5 hover:text-white" title="Tahrirlash" aria-label="Tahrirlash">
                        <Pencil size={15} />
                      </Link>
                      <IconBtn label="O'chirish" tone="red" disabled={isBusy} onClick={() => remove(c)}>
                        <Trash2 size={15} />
                      </IconBtn>
                    </div>
                  </div>
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
