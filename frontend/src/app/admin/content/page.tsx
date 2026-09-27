"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { Clapperboard, Folder, FolderPlus, Plus, Trash2, Video } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import {
  adminListContentFolders,
  adminCreateContentFolder,
  adminDeleteContentFolder,
  adminUploadContentPoster,
  ContentFolder,
} from "@/lib/api";
import { normalizeMediaUrl } from "@/lib/image-utils";
import { useToast } from "@/components/admin/Toast";
import { EmptyState, GhostButton, IconBtn, Modal, PageHead, PrimaryButton, SearchBox, SkeletonList, StatTile, Thumb, fmtDate } from "@/components/admin/kit";
import { ErrorBanner, Field, inputCls } from "@/components/admin/form/ui";
import MediaUploadField from "@/components/admin/form/MediaUploadField";

export default function AdminContentPage() {
  const { token } = useAuth();
  const toast = useToast();
  const [folders, setFolders] = useState<ContentFolder[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [showCreate, setShowCreate] = useState(false);
  const [deleting, setDeleting] = useState<string | null>(null);

  const reload = useCallback(async () => {
    if (!token) return;
    try {
      setFolders((await adminListContentFolders(token)) || []);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Yuklab bo'lmadi");
    } finally {
      setLoading(false);
    }
  }, [token, toast]);

  useEffect(() => {
    void reload();
  }, [reload]);

  const totalClips = useMemo(() => folders.reduce((s, f) => s + (f.clips_count || 0), 0), [folders]);
  const emptyFolders = useMemo(() => folders.filter((f) => !f.clips_count).length, [folders]);

  const visible = useMemo(() => {
    const q = search.trim().toLowerCase();
    return q ? folders.filter((f) => f.title.toLowerCase().includes(q) || (f.description || "").toLowerCase().includes(q)) : folders;
  }, [folders, search]);

  const handleDelete = async (folder: ContentFolder) => {
    if (!token) return;
    if (!window.confirm(`«${folder.title}» papkasini va uning ichidagi barcha cliplarni o'chirasizmi?`)) return;
    setDeleting(folder.id);
    try {
      await adminDeleteContentFolder(token, folder.id);
      setFolders((list) => list.filter((f) => f.id !== folder.id));
      toast.success("Papka o'chirildi");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "O'chirishda xato");
    } finally {
      setDeleting(null);
    }
  };

  return (
    <div className="mx-auto max-w-7xl p-4 sm:p-6">
      <PageHead
        icon={Clapperboard}
        gradient="from-fuchsia-500 to-purple-700"
        title="Content"
        subtitle="CapCut'dan eksport qilingan kliplar va ularni Instagram / YouTube / TikTok'ga rejalashtirish"
        actions={
          <PrimaryButton onClick={() => setShowCreate(true)}>
            <Plus size={16} /> Yangi papka
          </PrimaryButton>
        }
      />

      <div className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-3">
        <StatTile icon={Folder} tone="violet" label="Papkalar" value={folders.length} />
        <StatTile icon={Video} tone="sky" label="Jami cliplar" value={totalClips} />
        <StatTile icon={FolderPlus} tone="gray" label="Bo'sh papkalar" value={emptyFolders} hint={emptyFolders ? "Clip yuklanmagan" : undefined} />
      </div>

      {folders.length > 6 && (
        <div className="mb-4">
          <SearchBox value={search} onChange={setSearch} placeholder="Papka nomi..." />
        </div>
      )}

      {loading ? (
        <SkeletonList rows={3} height={140} />
      ) : visible.length === 0 ? (
        <EmptyState
          icon={Folder}
          title={folders.length ? "Papka topilmadi" : "Hali papka yo'q"}
          text={folders.length ? "Qidiruvni o'zgartirib ko'ring." : "Har bir kino yoki kampaniya uchun alohida papka yarating va kliplarni shu yerga yuklang."}
          action={
            !folders.length ? (
              <GhostButton onClick={() => setShowCreate(true)}>
                <Plus size={15} /> Birinchi papkani yaratish
              </GhostButton>
            ) : undefined
          }
        />
      ) : (
        <ul className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 xl:grid-cols-5">
          {visible.map((folder) => (
            <li
              key={folder.id}
              className={`group relative overflow-hidden rounded-2xl border border-white/10 bg-[#12121a] transition hover:-translate-y-0.5 hover:border-fuchsia-500/40 hover:shadow-xl hover:shadow-fuchsia-500/5 ${
                deleting === folder.id ? "pointer-events-none opacity-50" : ""
              }`}
            >
              <Link href={`/admin/content/${folder.id}`} className="block">
                <div className="relative aspect-[2/3] overflow-hidden bg-gradient-to-br from-fuchsia-500/15 via-purple-500/10 to-transparent">
                  <Thumb
                    src={folder.poster_url ? normalizeMediaUrl(folder.poster_url) : undefined}
                    alt={folder.title}
                    className="h-full w-full transition duration-300 group-hover:scale-105"
                    icon={Folder}
                  />
                  <div className="absolute inset-0 bg-gradient-to-t from-black/90 via-black/20 to-transparent" />
                  <span className="absolute right-2 top-2 inline-flex items-center gap-1 rounded-full bg-black/70 px-2 py-0.5 text-[11px] font-medium text-white">
                    <Video size={11} /> {folder.clips_count}
                  </span>
                  <div className="absolute inset-x-0 bottom-0 p-3">
                    <h3 className="line-clamp-2 font-semibold leading-tight text-white">{folder.title}</h3>
                    {folder.description && <p className="mt-1 line-clamp-2 text-[11px] text-gray-300">{folder.description}</p>}
                    {folder.created_at && <p className="mt-1.5 text-[10px] text-gray-500">{fmtDate(folder.created_at, false)}</p>}
                  </div>
                </div>
              </Link>
              <div className="absolute left-2 top-2 rounded-lg bg-black/70 opacity-0 transition group-hover:opacity-100 focus-within:opacity-100">
                <IconBtn label="O'chirish" tone="red" onClick={() => handleDelete(folder)}>
                  <Trash2 size={14} />
                </IconBtn>
              </div>
            </li>
          ))}
        </ul>
      )}

      <CreateFolderModal
        open={showCreate}
        onClose={() => setShowCreate(false)}
        onCreated={() => {
          setShowCreate(false);
          toast.success("Papka yaratildi");
          void reload();
        }}
      />
    </div>
  );
}

function CreateFolderModal({ open, onClose, onCreated }: { open: boolean; onClose: () => void; onCreated: () => void }) {
  const { token } = useAuth();
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [posterURL, setPosterURL] = useState("");
  const [uploading, setUploading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setTitle("");
    setDescription("");
    setPosterURL("");
    setError(null);
  }, [open]);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token || !title.trim()) return;
    setSaving(true);
    setError(null);
    try {
      await adminCreateContentFolder(token, {
        title: title.trim(),
        poster_url: posterURL || undefined,
        description: description.trim() || undefined,
      });
      onCreated();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Yaratib bo'lmadi");
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      busy={saving}
      icon={FolderPlus}
      iconTone="violet"
      title="Yangi papka"
      subtitle="Kliplar shu papka ichida saqlanadi"
      footer={
        <>
          <GhostButton onClick={onClose} disabled={saving}>
            Bekor qilish
          </GhostButton>
          <button
            type="submit"
            form="content-folder-form"
            disabled={saving || uploading || !title.trim()}
            className="inline-flex items-center gap-2 rounded-xl bg-orange-500 px-4 py-2.5 text-sm font-semibold text-white hover:bg-orange-400 disabled:opacity-50"
          >
            {saving ? "Yaratilmoqda..." : "Yaratish"}
          </button>
        </>
      }
    >
      <form id="content-folder-form" onSubmit={submit} className="space-y-4">
        {error && <ErrorBanner message={error} />}
        <Field label="Nomi" required htmlFor="cf-title">
          <input id="cf-title" autoFocus value={title} onChange={(e) => setTitle(e.target.value)} className={inputCls} placeholder="Masalan: Avatar 3 — reklama kliplari" />
        </Field>
        <Field label="Tavsif" htmlFor="cf-desc">
          <textarea id="cf-desc" value={description} onChange={(e) => setDescription(e.target.value)} rows={2} className={`${inputCls} resize-none`} />
        </Field>
        <MediaUploadField
          label="Poster"
          value={posterURL}
          onChange={setPosterURL}
          onBusyChange={setUploading}
          upload={(file) => adminUploadContentPoster(token || "", file)}
          hint="Ixtiyoriy — papka kartasida ko'rinadi"
        />
      </form>
    </Modal>
  );
}
