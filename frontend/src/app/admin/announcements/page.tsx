"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { Bell, Loader2, Plus, Trash2, Eye, X, ExternalLink, Pencil } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import {
  Announcement,
  AnnouncementInput,
  adminListAnnouncements,
  adminCreateAnnouncement,
  adminUpdateAnnouncement,
  adminDeleteAnnouncement,
} from "@/lib/api";

function toDatetimeLocal(iso: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (isNaN(d.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function fromDatetimeLocal(v: string): string {
  if (!v) return "";
  const d = new Date(v);
  if (isNaN(d.getTime())) return "";
  return d.toISOString();
}

function statusBadge(a: Announcement): { text: string; color: string } {
  if (!a.is_active) return { text: "O'chirilgan", color: "bg-gray-700/40 text-gray-300" };
  const now = Date.now();
  const start = new Date(a.starts_at).getTime();
  const end = new Date(a.ends_at).getTime();
  if (now < start) return { text: "Kutilmoqda", color: "bg-blue-500/20 text-blue-300" };
  if (now > end) return { text: "Tugagan", color: "bg-gray-700/40 text-gray-300" };
  return { text: "Faol", color: "bg-emerald-500/20 text-emerald-300" };
}

const EMPTY_INPUT: AnnouncementInput = {
  type: "modal",
  title: "",
  body: "",
  link_url: "",
  link_label: "",
  starts_at: "",
  ends_at: "",
  dismissible: true,
  is_active: true,
  priority: 0,
};

export default function AdminAnnouncementsPage() {
  const { token, isLoading: authLoading, user } = useAuth();
  const router = useRouter();

  const [items, setItems] = useState<Announcement[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const [formOpen, setFormOpen] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);
  const [form, setForm] = useState<AnnouncementInput>(EMPTY_INPUT);
  const [startLocal, setStartLocal] = useState("");
  const [endLocal, setEndLocal] = useState("");
  const [saving, setSaving] = useState(false);
  const [previewOf, setPreviewOf] = useState<Announcement | null>(null);

  useEffect(() => {
    if (!authLoading && (!token || user?.role !== "superadmin")) {
      router.replace("/admin/dashboard");
    }
  }, [authLoading, token, user, router]);

  const load = useCallback(async () => {
    if (!token) return;
    try {
      setLoading(true);
      const data = await adminListAnnouncements(token);
      setItems(data);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Yuklashda xatolik");
    } finally {
      setLoading(false);
    }
  }, [token]);

  useEffect(() => {
    load();
  }, [load]);

  const openCreate = () => {
    setEditId(null);
    setForm(EMPTY_INPUT);
    const now = new Date();
    const inHour = new Date(Date.now() + 3 * 60 * 60 * 1000);
    setStartLocal(toDatetimeLocal(now.toISOString()));
    setEndLocal(toDatetimeLocal(inHour.toISOString()));
    setFormOpen(true);
  };

  const openEdit = (a: Announcement) => {
    setEditId(a.id);
    setForm({
      type: a.type === "alert" ? "alert" : "modal",
      title: a.title,
      body: a.body,
      link_url: a.link_url || "",
      link_label: a.link_label || "",
      starts_at: a.starts_at,
      ends_at: a.ends_at,
      dismissible: a.dismissible,
      is_active: a.is_active,
      priority: a.priority,
    });
    setStartLocal(toDatetimeLocal(a.starts_at));
    setEndLocal(toDatetimeLocal(a.ends_at));
    setFormOpen(true);
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token) return;
    const startsISO = fromDatetimeLocal(startLocal);
    const endsISO = fromDatetimeLocal(endLocal);
    if (!form.title.trim()) {
      setError("Sarlavha kiritilishi shart");
      return;
    }
    if (!startsISO || !endsISO) {
      setError("Boshlanish va tugash vaqtini kiriting");
      return;
    }
    if (new Date(endsISO) <= new Date(startsISO)) {
      setError("Tugash vaqti boshlanishidan keyin bo'lishi kerak");
      return;
    }
    const payload: AnnouncementInput = { ...form, starts_at: startsISO, ends_at: endsISO };
    try {
      setSaving(true);
      setError("");
      if (editId) {
        await adminUpdateAnnouncement(token, editId, payload);
      } else {
        await adminCreateAnnouncement(token, payload);
      }
      setFormOpen(false);
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Saqlashda xatolik");
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (id: string) => {
    if (!token) return;
    if (!confirm("O'chirishni tasdiqlaysizmi?")) return;
    try {
      await adminDeleteAnnouncement(token, id);
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "O'chirishda xatolik");
    }
  };

  const sorted = useMemo(() => items, [items]);

  if (authLoading || (user?.role !== "superadmin")) {
    return (
      <div className="min-h-[60vh] flex items-center justify-center">
        <Loader2 className="animate-spin text-gray-400" />
      </div>
    );
  }

  return (
    <div className="p-6 max-w-6xl mx-auto">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-display text-white flex items-center gap-2">
          <Bell className="text-orange-500" size={22} />
          E'lonlar (Modal popup)
        </h1>
        <button
          onClick={openCreate}
          className="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-orange-500 hover:bg-orange-600 text-white font-medium"
        >
          <Plus size={16} />
          Yangi e'lon
        </button>
      </div>

      {error && (
        <div className="mb-4 px-4 py-2.5 rounded-lg bg-red-500/15 border border-red-500/30 text-red-300 text-sm">
          {error}
        </div>
      )}

      {loading ? (
        <div className="flex items-center justify-center py-20">
          <Loader2 className="animate-spin text-gray-400" />
        </div>
      ) : sorted.length === 0 ? (
        <div className="rounded-xl border border-brand-border bg-brand-card p-10 text-center text-gray-400">
          Hozircha e&apos;lon yo&apos;q. Yuqoridagi tugma orqali yangisini yarating.
        </div>
      ) : (
        <div className="space-y-3">
          {sorted.map((a) => {
            const badge = statusBadge(a);
            return (
              <div
                key={a.id}
                className="rounded-xl border border-brand-border bg-brand-card p-4 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between"
              >
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 flex-wrap">
                    {a.type === "alert" ? (
                      <span className="text-xs px-2 py-0.5 rounded bg-red-600/20 text-red-300">Alert</span>
                    ) : (
                      <span className="text-xs px-2 py-0.5 rounded bg-blue-500/15 text-blue-300">Modal</span>
                    )}
                    <span className={`text-xs px-2 py-0.5 rounded ${badge.color}`}>{badge.text}</span>
                    {a.dismissible ? (
                      <span className="text-xs px-2 py-0.5 rounded bg-white/5 text-gray-300">yopiladi</span>
                    ) : (
                      <span className="text-xs px-2 py-0.5 rounded bg-yellow-500/20 text-yellow-300">majburiy</span>
                    )}
                    {a.priority > 0 && (
                      <span className="text-xs px-2 py-0.5 rounded bg-white/5 text-gray-300">priority {a.priority}</span>
                    )}
                  </div>
                  <h3 className="mt-2 text-white font-medium">{a.title}</h3>
                  {a.body && <p className="mt-1 text-sm text-gray-400 line-clamp-2">{a.body}</p>}
                  <p className="mt-2 text-xs text-gray-500">
                    {new Date(a.starts_at).toLocaleString("uz-UZ")} → {new Date(a.ends_at).toLocaleString("uz-UZ")}
                  </p>
                  <p className="mt-1 text-xs text-gray-500">
                    Yaratgan: <span className="text-gray-300">{a.created_by_name || "—"}</span>
                    {" · "}
                    {new Date(a.created_at).toLocaleString("uz-UZ")}
                  </p>
                  {a.link_url && (
                    <a
                      href={a.link_url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="mt-1 inline-flex items-center gap-1 text-xs text-orange-400 hover:text-orange-300"
                    >
                      {a.link_label || a.link_url}
                      <ExternalLink size={12} />
                    </a>
                  )}
                </div>
                <div className="flex gap-2 shrink-0">
                  <button
                    onClick={() => setPreviewOf(a)}
                    className="px-3 py-1.5 rounded bg-white/5 hover:bg-white/10 text-gray-200 text-sm inline-flex items-center gap-1"
                  >
                    <Eye size={14} /> Preview
                  </button>
                  <button
                    onClick={() => openEdit(a)}
                    className="px-3 py-1.5 rounded bg-white/5 hover:bg-white/10 text-gray-200 text-sm inline-flex items-center gap-1"
                  >
                    <Pencil size={14} /> Tahrir
                  </button>
                  <button
                    onClick={() => handleDelete(a.id)}
                    className="px-3 py-1.5 rounded bg-red-500/15 hover:bg-red-500/25 text-red-300 text-sm inline-flex items-center gap-1"
                  >
                    <Trash2 size={14} /> O&apos;chirish
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {formOpen && (
        <div className="fixed inset-0 z-[150] flex items-center justify-center px-4">
          <div className="absolute inset-0 bg-black/70 backdrop-blur-sm" onClick={() => setFormOpen(false)} />
          <form
            onSubmit={submit}
            className="relative w-full max-w-xl rounded-2xl bg-[#15151f] border border-[#1e1e2e] p-6 max-h-[90vh] overflow-y-auto"
          >
            <button
              type="button"
              onClick={() => setFormOpen(false)}
              className="absolute top-3 right-3 w-8 h-8 rounded-full bg-black/30 hover:bg-black/60 flex items-center justify-center"
            >
              <X size={16} className="text-white" />
            </button>
            <h2 className="text-lg text-white font-medium mb-4">
              {editId ? "E'lonni tahrirlash" : "Yangi e'lon"}
            </h2>

            <div className="space-y-3">
              <div>
                <label className="block text-xs text-gray-400 mb-1">Turi</label>
                <div className="grid grid-cols-2 gap-2">
                  <button
                    type="button"
                    onClick={() => setForm({ ...form, type: "modal" })}
                    className={`px-3 py-2 rounded-lg border text-sm transition-colors ${
                      form.type !== "alert"
                        ? "border-brand-red bg-brand-red/10 text-white"
                        : "border-white/10 bg-black/40 text-gray-400 hover:border-white/30"
                    }`}
                  >
                    Modal (popup)
                  </button>
                  <button
                    type="button"
                    onClick={() => setForm({ ...form, type: "alert" })}
                    className={`px-3 py-2 rounded-lg border text-sm transition-colors ${
                      form.type === "alert"
                        ? "border-red-500 bg-red-600/15 text-white"
                        : "border-white/10 bg-black/40 text-gray-400 hover:border-white/30"
                    }`}
                  >
                    Alert (tepada qizil chiziq)
                  </button>
                </div>
                {form.type === "alert" && (
                  <p className="mt-1.5 text-[11px] text-gray-500">
                    Har bir sahifa tepasida qizil, kichik shriftli chiziq. Belgilangan vaqt oralig'ida ko'rinadi.
                  </p>
                )}
              </div>
              <div>
                <label className="block text-xs text-gray-400 mb-1">
                  {form.type === "alert" ? "Alert matni *" : "Sarlavha *"}
                </label>
                <input
                  type="text"
                  value={form.title}
                  onChange={(e) => setForm({ ...form, title: e.target.value })}
                  required
                  className="w-full px-3 py-2 bg-black/40 border border-white/10 rounded-lg text-white"
                />
              </div>
              <div>
                <label className="block text-xs text-gray-400 mb-1">Matn</label>
                <textarea
                  rows={4}
                  value={form.body}
                  onChange={(e) => setForm({ ...form, body: e.target.value })}
                  className="w-full px-3 py-2 bg-black/40 border border-white/10 rounded-lg text-white"
                  placeholder="Bugun kechqurun 19:00 dan 22:00 gacha sayt ishlamaydi..."
                />
              </div>

              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs text-gray-400 mb-1">Boshlanish *</label>
                  <input
                    type="datetime-local"
                    value={startLocal}
                    onChange={(e) => setStartLocal(e.target.value)}
                    required
                    className="w-full px-3 py-2 bg-black/40 border border-white/10 rounded-lg text-white"
                  />
                </div>
                <div>
                  <label className="block text-xs text-gray-400 mb-1">Tugash *</label>
                  <input
                    type="datetime-local"
                    value={endLocal}
                    onChange={(e) => setEndLocal(e.target.value)}
                    required
                    className="w-full px-3 py-2 bg-black/40 border border-white/10 rounded-lg text-white"
                  />
                </div>
              </div>

              {form.type !== "alert" && (
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  <div>
                    <label className="block text-xs text-gray-400 mb-1">Link URL (ixtiyoriy)</label>
                    <input
                      type="url"
                      value={form.link_url}
                      onChange={(e) => setForm({ ...form, link_url: e.target.value })}
                      placeholder="https://..."
                      className="w-full px-3 py-2 bg-black/40 border border-white/10 rounded-lg text-white"
                    />
                  </div>
                  <div>
                    <label className="block text-xs text-gray-400 mb-1">Tugma matni</label>
                    <input
                      type="text"
                      value={form.link_label}
                      onChange={(e) => setForm({ ...form, link_label: e.target.value })}
                      placeholder="Batafsil"
                      className="w-full px-3 py-2 bg-black/40 border border-white/10 rounded-lg text-white"
                    />
                  </div>
                </div>
              )}

              <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 pt-2">
                <label className="flex items-center gap-2 text-sm text-gray-200">
                  <input
                    type="checkbox"
                    checked={form.is_active}
                    onChange={(e) => setForm({ ...form, is_active: e.target.checked })}
                  />
                  Faol
                </label>
                <label className="flex items-center gap-2 text-sm text-gray-200">
                  <input
                    type="checkbox"
                    checked={form.dismissible}
                    onChange={(e) => setForm({ ...form, dismissible: e.target.checked })}
                  />
                  Yopib bo&apos;ladi
                </label>
                <div>
                  <label className="block text-xs text-gray-400 mb-1">Priority</label>
                  <input
                    type="number"
                    value={form.priority}
                    onChange={(e) => setForm({ ...form, priority: Number(e.target.value) || 0 })}
                    className="w-full px-3 py-1.5 bg-black/40 border border-white/10 rounded-lg text-white"
                  />
                </div>
              </div>
            </div>

            <div className="mt-6 flex justify-end gap-2">
              <button
                type="button"
                onClick={() => setFormOpen(false)}
                className="px-4 py-2 rounded-lg bg-white/5 hover:bg-white/10 text-gray-200"
              >
                Bekor
              </button>
              <button
                type="submit"
                disabled={saving}
                className="px-4 py-2 rounded-lg bg-orange-500 hover:bg-orange-600 text-white inline-flex items-center gap-2 disabled:opacity-60"
              >
                {saving && <Loader2 size={16} className="animate-spin" />}
                {editId ? "Saqlash" : "Yaratish"}
              </button>
            </div>
          </form>
        </div>
      )}

      {previewOf && (
        <div className="fixed inset-0 z-[160] flex items-center justify-center px-4">
          <div className="absolute inset-0 bg-black/70 backdrop-blur-md" onClick={() => setPreviewOf(null)} />
          <div className="relative w-full max-w-md rounded-2xl bg-[#15151f] border border-[#1e1e2e] shadow-2xl overflow-hidden">
            <button
              onClick={() => setPreviewOf(null)}
              className="absolute top-3 right-3 w-9 h-9 rounded-full bg-black/30 hover:bg-black/60 flex items-center justify-center"
            >
              <X size={18} className="text-white" />
            </button>
            <div className="p-6">
              <h2 className="font-display text-xl text-white pr-10">{previewOf.title}</h2>
              {previewOf.body && (
                <p className="mt-3 text-sm text-gray-300 whitespace-pre-line">{previewOf.body}</p>
              )}
              <div className="mt-6 flex gap-3">
                {previewOf.link_url && (
                  <span className="flex-1 inline-flex items-center justify-center gap-2 px-4 py-2.5 rounded-lg bg-orange-500 text-white">
                    {previewOf.link_label || "Batafsil"}
                    <ExternalLink size={16} />
                  </span>
                )}
                {previewOf.dismissible && (
                  <span className={`${previewOf.link_url ? "sm:w-32" : "flex-1"} px-4 py-2.5 rounded-lg bg-white/5 text-white text-center`}>
                    {previewOf.link_url ? "Yopish" : "Tushundim"}
                  </span>
                )}
              </div>
              <p className="mt-4 text-xs text-gray-500 text-center">
                Preview — tugmalar bosilmaydi
              </p>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
