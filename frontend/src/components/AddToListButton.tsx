"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { ListPlus, Check, Plus, Loader2, Lock } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { addToList, createList, getListsContaining, getMyLists, removeFromList, UserListSummary } from "@/lib/api";

interface Props {
  targetType: "movie" | "series";
  targetId: string;
  className: string;
  onNeedLogin: () => void;
  onFlash: (text: string) => void;
}

/** "Ro'yxatga qo'shish": popover with the user's lists as checkboxes. */
export default function AddToListButton({ targetType, targetId, className, onNeedLogin, onFlash }: Props) {
  const { isAuthenticated, token } = useAuth();
  const [open, setOpen] = useState(false);
  const [lists, setLists] = useState<UserListSummary[] | null>(null);
  const [inLists, setInLists] = useState<Set<string>>(new Set());
  const [busy, setBusy] = useState<string | null>(null);
  const [newTitle, setNewTitle] = useState("");
  const [error, setError] = useState("");
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open || !token) return;
    let active = true;
    Promise.all([getMyLists(token), getListsContaining(token, targetType, targetId)])
      .then(([mine, ids]) => {
        if (!active) return;
        setLists(mine);
        setInLists(new Set(ids));
      })
      .catch(() => active && setLists([]));
    return () => {
      active = false;
    };
  }, [open, token, targetType, targetId]);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("mousedown", onDown);
    window.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDown);
      window.removeEventListener("keydown", onKey);
    };
  }, [open]);

  const toggle = async (list: UserListSummary) => {
    if (!token) return;
    const has = inLists.has(list.id);
    setBusy(list.id);
    setError("");
    try {
      if (has) await removeFromList(token, list.id, targetType, targetId);
      else await addToList(token, list.id, targetType, targetId);
      setInLists((prev) => {
        const next = new Set(prev);
        if (has) next.delete(list.id);
        else next.add(list.id);
        return next;
      });
      setLists((prev) => prev?.map((l) => (l.id === list.id ? { ...l, count: l.count + (has ? -1 : 1) } : l)) ?? prev);
      onFlash(has ? `"${list.title}" ro'yxatidan olib tashlandi` : `"${list.title}" ro'yxatiga qo'shildi`);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Xatolik");
    } finally {
      setBusy(null);
    }
  };

  const create = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token || !newTitle.trim()) return;
    setBusy("new");
    setError("");
    try {
      const l = await createList(token, { title: newTitle.trim() });
      await addToList(token, l.id, targetType, targetId);
      setLists((prev) => [{ ...l, count: 1 }, ...(prev || [])]);
      setInLists((prev) => new Set(prev).add(l.id));
      setNewTitle("");
      onFlash(`"${l.title}" ro'yxati yaratildi`);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Xatolik");
    } finally {
      setBusy(null);
    }
  };

  const inAny = inLists.size > 0;

  return (
    <div className="relative" ref={ref}>
      <button
        onClick={() => (isAuthenticated ? setOpen((o) => !o) : onNeedLogin())}
        aria-haspopup="dialog"
        aria-expanded={open}
        className={className}
      >
        {inAny ? <Check size={16} /> : <ListPlus size={16} />}
        Ro&apos;yxatga
      </button>
      {open && (
        <div
          role="dialog"
          aria-label="Ro'yxatga qo'shish"
          className="absolute left-0 top-full z-40 mt-2 w-72 overflow-hidden rounded-xl border border-white/10 bg-[#15151f] shadow-2xl"
        >
          <div className="max-h-64 overflow-y-auto py-1">
            {lists === null ? (
              <div className="flex justify-center py-6 text-gray-500">
                <Loader2 size={18} className="animate-spin" />
              </div>
            ) : lists.length === 0 ? (
              <p className="px-4 py-4 text-sm text-gray-400">Hali ro&apos;yxatingiz yo&apos;q. Quyida birinchisini yarating.</p>
            ) : (
              lists.map((l) => {
                const checked = inLists.has(l.id);
                return (
                  <button
                    key={l.id}
                    onClick={() => toggle(l)}
                    disabled={busy === l.id}
                    role="menuitemcheckbox"
                    aria-checked={checked}
                    className="flex w-full items-center gap-3 px-4 py-2.5 text-left text-sm text-gray-200 hover:bg-white/5 disabled:opacity-60"
                  >
                    <span
                      className={`flex h-5 w-5 shrink-0 items-center justify-center rounded border ${
                        checked ? "border-orange-500 bg-orange-500 text-white" : "border-white/25"
                      }`}
                    >
                      {busy === l.id ? <Loader2 size={12} className="animate-spin" /> : checked && <Check size={13} strokeWidth={3} />}
                    </span>
                    <span className="min-w-0 flex-1 truncate">{l.title}</span>
                    {!l.is_public && <Lock size={12} className="text-gray-500" aria-label="Yopiq" />}
                    <span className="text-xs text-gray-500">{l.count}</span>
                  </button>
                );
              })
            )}
          </div>
          <form onSubmit={create} className="flex gap-2 border-t border-white/10 p-3">
            <input
              value={newTitle}
              onChange={(e) => setNewTitle(e.target.value)}
              maxLength={60}
              placeholder="Yangi ro'yxat nomi"
              className="min-w-0 flex-1 rounded-lg border border-white/10 bg-black/30 px-3 py-2 text-sm text-white placeholder-gray-500 focus:border-orange-500 focus:outline-none"
            />
            <button
              type="submit"
              disabled={!newTitle.trim() || busy === "new"}
              className="inline-flex items-center rounded-lg bg-orange-500 px-3 text-white hover:bg-orange-600 disabled:opacity-50"
              aria-label="Yaratish"
            >
              {busy === "new" ? <Loader2 size={15} className="animate-spin" /> : <Plus size={16} />}
            </button>
          </form>
          {error && <p className="px-3 pb-3 text-xs text-red-400">{error}</p>}
          <Link href="/user?tab=lists" className="block border-t border-white/10 px-4 py-2.5 text-xs text-gray-400 hover:bg-white/5 hover:text-white">
            Barcha ro&apos;yxatlarim →
          </Link>
        </div>
      )}
    </div>
  );
}
