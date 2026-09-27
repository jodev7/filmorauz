"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { ListVideo, Plus, Lock, Globe, Trash2, Pencil, Link2, Loader2, Check } from "lucide-react";
import { createList, deleteList, getMyLists, updateList, UserListSummary } from "@/lib/api";
import { normalizeMediaUrl } from "@/lib/image-utils";

function Covers({ covers }: { covers: string[] }) {
  const cells = [0, 1, 2, 3];
  return (
    <div className="grid aspect-[4/3] grid-cols-2 grid-rows-2 gap-0.5 overflow-hidden rounded-t-xl bg-white/5">
      {cells.map((i) =>
        covers[i] ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img key={i} src={normalizeMediaUrl(covers[i])} alt="" loading="lazy" className="h-full w-full object-cover" />
        ) : (
          <div key={i} className="bg-white/[0.03]" />
        )
      )}
    </div>
  );
}

function ListForm({
  initial,
  onSubmit,
  onCancel,
  busy,
}: {
  initial?: Partial<UserListSummary>;
  onSubmit: (v: { title: string; description: string; is_public: boolean }) => void;
  onCancel: () => void;
  busy: boolean;
}) {
  const [title, setTitle] = useState(initial?.title ?? "");
  const [description, setDescription] = useState(initial?.description ?? "");
  const [isPublic, setIsPublic] = useState(initial?.is_public ?? true);
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        if (title.trim()) onSubmit({ title: title.trim(), description: description.trim(), is_public: isPublic });
      }}
      className="glass-card space-y-3 rounded-xl border border-white/10 p-4"
    >
      <input
        autoFocus
        value={title}
        onChange={(e) => setTitle(e.target.value)}
        maxLength={60}
        placeholder="Ro'yxat nomi, masalan: Oilaviy kechalar"
        className="w-full rounded-lg border border-white/10 bg-black/30 px-3 py-2.5 text-sm text-white placeholder-gray-500 focus:border-orange-500 focus:outline-none"
      />
      <textarea
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        maxLength={300}
        rows={2}
        placeholder="Qisqacha tavsif (ixtiyoriy)"
        className="w-full resize-none rounded-lg border border-white/10 bg-black/30 px-3 py-2 text-sm text-white placeholder-gray-500 focus:border-orange-500 focus:outline-none"
      />
      <div className="flex flex-wrap items-center justify-between gap-2">
        <label className="flex cursor-pointer items-center gap-2 text-sm text-gray-300">
          <input type="checkbox" checked={isPublic} onChange={(e) => setIsPublic(e.target.checked)} className="accent-orange-500" />
          Havola orqali hamma ko&apos;ra oladi
        </label>
        <div className="flex gap-2">
          <button type="button" onClick={onCancel} className="rounded-lg px-3 py-2 text-sm text-gray-400 hover:text-white">
            Bekor
          </button>
          <button
            type="submit"
            disabled={!title.trim() || busy}
            className="inline-flex items-center gap-1.5 rounded-lg bg-orange-500 px-4 py-2 text-sm font-medium text-white hover:bg-orange-600 disabled:opacity-50"
          >
            {busy && <Loader2 size={14} className="animate-spin" />} Saqlash
          </button>
        </div>
      </div>
    </form>
  );
}

/** Profile section: the user's personal lists with create/edit/delete/share. */
export default function UserListsSection({ token }: { token: string }) {
  const [lists, setLists] = useState<UserListSummary[] | null>(null);
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [copied, setCopied] = useState<string | null>(null);

  useEffect(() => {
    getMyLists(token).then(setLists).catch(() => setLists([]));
  }, [token]);

  const run = async (key: string, fn: () => Promise<void>) => {
    setBusy(key);
    setError("");
    try {
      await fn();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Xatolik");
    } finally {
      setBusy(null);
    }
  };

  const share = async (l: UserListSummary) => {
    const url = `${window.location.origin}/lists/${l.share_slug}`;
    try {
      if (navigator.share && /Mobi|Android/i.test(navigator.userAgent)) {
        await navigator.share({ title: l.title, url });
      } else {
        await navigator.clipboard.writeText(url);
        setCopied(l.id);
        setTimeout(() => setCopied(null), 2000);
      }
    } catch {
      // cancelled
    }
  };

  if (lists === null) {
    return (
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
        {[0, 1, 2, 3].map((i) => (
          <div key={i} className="aspect-[4/5] animate-pulse rounded-xl bg-white/5" />
        ))}
      </div>
    );
  }

  return (
    <div>
      <div className="mb-4 flex items-center justify-between gap-2">
        <p className="text-sm text-gray-500">{lists.length} ta ro&apos;yxat</p>
        {!creating && (
          <button
            onClick={() => setCreating(true)}
            className="inline-flex items-center gap-1.5 rounded-lg bg-orange-500 px-3 py-2 text-sm font-medium text-white hover:bg-orange-600"
          >
            <Plus size={15} /> Yangi ro&apos;yxat
          </button>
        )}
      </div>
      {error && <p className="mb-3 text-sm text-red-400">{error}</p>}
      {creating && (
        <div className="mb-4">
          <ListForm
            busy={busy === "create"}
            onCancel={() => setCreating(false)}
            onSubmit={(v) =>
              run("create", async () => {
                const l = await createList(token, v);
                setLists((prev) => [l, ...(prev || [])]);
                setCreating(false);
              })
            }
          />
        </div>
      )}

      {lists.length === 0 && !creating ? (
        <div className="glass-card rounded-xl border border-white/5 py-10 text-center">
          <ListVideo className="mx-auto mb-2 h-8 w-8 text-gray-600" />
          <p className="text-sm text-gray-400">O&apos;zingizning kino ro&apos;yxatlaringizni tuzing va do&apos;stlaringiz bilan ulashing</p>
          <p className="mt-1 text-xs text-gray-600">Kino sahifasidagi &quot;Ro&apos;yxatga&quot; tugmasi orqali ham qo&apos;shish mumkin</p>
        </div>
      ) : (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
          {lists.map((l) =>
            editing === l.id ? (
              <div key={l.id} className="col-span-2 sm:col-span-3 lg:col-span-4">
                <ListForm
                  initial={l}
                  busy={busy === l.id}
                  onCancel={() => setEditing(null)}
                  onSubmit={(v) =>
                    run(l.id, async () => {
                      await updateList(token, l.id, v);
                      setLists((prev) => prev?.map((x) => (x.id === l.id ? { ...x, ...v } : x)) ?? prev);
                      setEditing(null);
                    })
                  }
                />
              </div>
            ) : (
              <div key={l.id} className="group glass-card overflow-hidden rounded-xl border border-white/10 transition-colors hover:border-orange-500/40">
                <Link href={`/lists/${l.share_slug}`} className="block">
                  <Covers covers={l.covers} />
                  <div className="p-3">
                    <p className="line-clamp-1 text-sm font-semibold text-white group-hover:text-orange-400">{l.title}</p>
                    <p className="mt-0.5 flex items-center gap-1 text-xs text-gray-500">
                      {l.is_public ? <Globe size={11} /> : <Lock size={11} />}
                      {l.count} ta · {l.is_public ? "ochiq" : "yopiq"}
                    </p>
                  </div>
                </Link>
                <div className="flex border-t border-white/5">
                  <button
                    onClick={() => share(l)}
                    disabled={!l.is_public}
                    title={l.is_public ? "Havolani nusxalash" : "Yopiq ro'yxatni ulashib bo'lmaydi"}
                    className="flex flex-1 items-center justify-center gap-1 py-2 text-xs text-gray-400 hover:bg-white/5 hover:text-white disabled:opacity-40"
                  >
                    {copied === l.id ? <Check size={13} className="text-emerald-400" /> : <Link2 size={13} />}
                    {copied === l.id ? "Nusxalandi" : "Ulashish"}
                  </button>
                  <button onClick={() => setEditing(l.id)} className="px-3 text-gray-400 hover:bg-white/5 hover:text-white" aria-label="Tahrirlash">
                    <Pencil size={13} />
                  </button>
                  <button
                    onClick={() => {
                      if (window.confirm(`"${l.title}" ro'yxati o'chirilsinmi?`))
                        run(l.id, async () => {
                          await deleteList(token, l.id);
                          setLists((prev) => prev?.filter((x) => x.id !== l.id) ?? prev);
                        });
                    }}
                    className="px-3 text-gray-400 hover:bg-red-500/10 hover:text-red-400"
                    aria-label="O'chirish"
                  >
                    {busy === l.id ? <Loader2 size={13} className="animate-spin" /> : <Trash2 size={13} />}
                  </button>
                </div>
              </div>
            )
          )}
        </div>
      )}
    </div>
  );
}

