"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import {
  AlertTriangle,
  CalendarClock,
  Copy,
  Eye,
  Link2,
  Loader2,
  Megaphone,
  MousePointerClick,
  Pencil,
  Plus,
  Settings2,
  Trash2,
  Type,
  X,
} from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { useToast } from "@/components/admin/Toast";
import {
  Announcement,
  AnnouncementInput,
  adminListAnnouncements,
  adminCreateAnnouncement,
  adminUpdateAnnouncement,
  adminDeleteAnnouncement,
  uploadTelegramPostMedia,
} from "@/lib/api";
import AnnouncementModal, { ANNOUNCEMENT_THEMES, AnnouncementVariant, announcementTheme } from "@/components/AnnouncementModal";
import MediaUploadField from "@/components/admin/form/MediaUploadField";
import { Field, Segmented, SwitchRow, inputCls } from "@/components/admin/form/ui";

// ── helpers ──

const pad = (n: number) => String(n).padStart(2, "0");

function toLocal(iso: string | Date): string {
  const d = typeof iso === "string" ? new Date(iso) : iso;
  if (isNaN(d.getTime())) return "";
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function fromLocal(v: string): string {
  const d = new Date(v);
  return v && !isNaN(d.getTime()) ? d.toISOString() : "";
}

const MONTHS = ["yan", "fev", "mar", "apr", "may", "iyn", "iyl", "avg", "sen", "okt", "noy", "dek"];
function fmt(iso: string): string {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return "—";
  return `${d.getDate()}-${MONTHS[d.getMonth()]}, ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function humanSpan(ms: number): string {
  const m = Math.max(1, Math.round(ms / 60000));
  if (m < 60) return `${m} daqiqa`;
  const h = Math.round(m / 60);
  if (h < 48) return `${h} soat`;
  return `${Math.round(h / 24)} kun`;
}

type Status = "active" | "scheduled" | "ended" | "off";

function statusOf(a: Announcement, now = Date.now()): Status {
  if (!a.is_active) return "off";
  if (now < new Date(a.starts_at).getTime()) return "scheduled";
  if (now > new Date(a.ends_at).getTime()) return "ended";
  return "active";
}

const STATUS_META: Record<Status, { label: string; dot: string; chip: string }> = {
  active: { label: "Faol", dot: "bg-emerald-400", chip: "bg-emerald-500/15 text-emerald-300" },
  scheduled: { label: "Rejalashtirilgan", dot: "bg-sky-400", chip: "bg-sky-500/15 text-sky-300" },
  ended: { label: "Tugagan", dot: "bg-gray-500", chip: "bg-white/5 text-gray-400" },
  off: { label: "O'chirilgan", dot: "bg-gray-600", chip: "bg-white/5 text-gray-500" },
};

const EMPTY: AnnouncementInput = {
  type: "modal",
  title: "",
  body: "",
  link_url: "",
  link_label: "",
  variant: "info",
  image_url: "",
  starts_at: "",
  ends_at: "",
  dismissible: true,
  is_active: true,
  priority: 0,
};

const DURATIONS: { label: string; ms: number }[] = [
  { label: "1 soat", ms: 3600e3 },
  { label: "6 soat", ms: 6 * 3600e3 },
  { label: "1 kun", ms: 24 * 3600e3 },
  { label: "3 kun", ms: 3 * 24 * 3600e3 },
  { label: "1 hafta", ms: 7 * 24 * 3600e3 },
];

type Filter = "all" | Status;

// ── page ──

export default function AdminAnnouncementsPage() {
  const { token, isLoading: authLoading, user } = useAuth();
  const router = useRouter();
  const toast = useToast();

  const [items, setItems] = useState<Announcement[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [filter, setFilter] = useState<Filter>("all");

  const [editor, setEditor] = useState<{ id: string | null } | null>(null);
  const [form, setForm] = useState<AnnouncementInput>(EMPTY);
  const [startLocal, setStartLocal] = useState("");
  const [endLocal, setEndLocal] = useState("");
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState("");
  const [imageBusy, setImageBusy] = useState(false);
  const [previewOf, setPreviewOf] = useState<Announcement | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);

  useEffect(() => {
    if (!authLoading && (!token || user?.role !== "superadmin")) router.replace("/admin/dashboard");
  }, [authLoading, token, user, router]);

  const load = useCallback(async () => {
    if (!token) return;
    try {
      setLoading(true);
      setItems(await adminListAnnouncements(token));
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Yuklashda xatolik");
    } finally {
      setLoading(false);
    }
  }, [token]);

  useEffect(() => {
    load();
  }, [load]);

  const counts = useMemo(() => {
    const c: Record<Filter, number> = { all: items.length, active: 0, scheduled: 0, ended: 0, off: 0 };
    items.forEach((a) => c[statusOf(a)]++);
    return c;
  }, [items]);

  const visible = useMemo(() => {
    const order: Record<Status, number> = { active: 0, scheduled: 1, off: 2, ended: 3 };
    return items
      .filter((a) => filter === "all" || statusOf(a) === filter)
      .sort((x, y) => order[statusOf(x)] - order[statusOf(y)] || y.priority - x.priority);
  }, [items, filter]);

  const set = <K extends keyof AnnouncementInput>(k: K, v: AnnouncementInput[K]) => setForm((f) => ({ ...f, [k]: v }));

  const openCreate = () => {
    const now = new Date();
    setForm(EMPTY);
    setStartLocal(toLocal(now));
    setEndLocal(toLocal(new Date(now.getTime() + 24 * 3600e3)));
    setFormError("");
    setEditor({ id: null });
  };

  const fillFrom = (a: Announcement, copy: boolean) => {
    setForm({
      type: a.type === "alert" ? "alert" : "modal",
      title: copy ? `${a.title} (nusxa)` : a.title,
      body: a.body,
      link_url: a.link_url || "",
      link_label: a.link_label || "",
      variant: a.variant || "info",
      image_url: a.image_url || "",
      starts_at: a.starts_at,
      ends_at: a.ends_at,
      dismissible: a.dismissible,
      is_active: copy ? false : a.is_active,
      priority: a.priority,
    });
    if (copy) {
      const span = Math.max(3600e3, new Date(a.ends_at).getTime() - new Date(a.starts_at).getTime());
      const now = new Date();
      setStartLocal(toLocal(now));
      setEndLocal(toLocal(new Date(now.getTime() + span)));
    } else {
      setStartLocal(toLocal(a.starts_at));
      setEndLocal(toLocal(a.ends_at));
    }
    setFormError("");
    setEditor({ id: copy ? null : a.id });
  };

  const applyDuration = (ms: number) => {
    const start = startLocal ? new Date(startLocal) : new Date();
    setEndLocal(toLocal(new Date(start.getTime() + ms)));
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token || !editor) return;
    const starts = fromLocal(startLocal);
    const ends = fromLocal(endLocal);
    if (!form.title.trim()) return setFormError("Sarlavha kiritilishi shart");
    if (!starts || !ends) return setFormError("Boshlanish va tugash vaqtini kiriting");
    if (new Date(ends) <= new Date(starts)) return setFormError("Tugash vaqti boshlanishidan keyin bo'lishi kerak");
    if (form.link_url && !/^https?:\/\//.test(form.link_url.trim())) return setFormError("Havola http:// yoki https:// bilan boshlanishi kerak");
    const payload: AnnouncementInput = { ...form, title: form.title.trim(), starts_at: starts, ends_at: ends };
    try {
      setSaving(true);
      setFormError("");
      if (editor.id) await adminUpdateAnnouncement(token, editor.id, payload);
      else await adminCreateAnnouncement(token, payload);
      toast.success(editor.id ? "E'lon saqlandi" : "E'lon yaratildi");
      setEditor(null);
      await load();
    } catch (err) {
      setFormError(err instanceof Error ? err.message : "Saqlashda xatolik");
    } finally {
      setSaving(false);
    }
  };

  const toggleActive = async (a: Announcement) => {
    if (!token) return;
    setBusyId(a.id);
    try {
      await adminUpdateAnnouncement(token, a.id, {
        type: a.type === "alert" ? "alert" : "modal",
        title: a.title,
        body: a.body,
        link_url: a.link_url || "",
        link_label: a.link_label || "",
        variant: a.variant || "info",
        image_url: a.image_url || "",
        starts_at: a.starts_at,
        ends_at: a.ends_at,
        dismissible: a.dismissible,
        is_active: !a.is_active,
        priority: a.priority,
      });
      setItems((list) => list.map((x) => (x.id === a.id ? { ...x, is_active: !a.is_active } : x)));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Xatolik");
    } finally {
      setBusyId(null);
    }
  };

  const remove = async (a: Announcement) => {
    if (!token || !confirm(`"${a.title}" e'lonini o'chirasizmi?`)) return;
    try {
      await adminDeleteAnnouncement(token, a.id);
      setItems((list) => list.filter((x) => x.id !== a.id));
      toast.success("E'lon o'chirildi");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "O'chirishda xatolik");
    }
  };

  if (authLoading || user?.role !== "superadmin") {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="animate-spin text-gray-400" />
      </div>
    );
  }

  const isAlert = form.type === "alert";
  const previewData = { ...form, ends_at: fromLocal(endLocal) };
  const FILTERS: { key: Filter; label: string }[] = [
    { key: "all", label: "Hammasi" },
    { key: "active", label: "Faol" },
    { key: "scheduled", label: "Rejalashtirilgan" },
    { key: "ended", label: "Tugagan" },
    { key: "off", label: "O'chirilgan" },
  ];

  return (
    <div className="mx-auto max-w-6xl p-4 sm:p-6">
      {/* Header */}
      <header className="mb-6 flex flex-wrap items-start justify-between gap-3">
        <div className="flex items-start gap-3">
          <span className="flex h-11 w-11 items-center justify-center rounded-2xl bg-gradient-to-br from-orange-500 to-rose-600 shadow-lg shadow-orange-500/20">
            <Megaphone size={20} className="text-white" />
          </span>
          <div>
            <h1 className="text-2xl font-bold text-white">E&apos;lonlar</h1>
            <p className="text-sm text-gray-500">Saytda popup oyna yoki tepadagi ogohlantirish chizig&apos;i sifatida chiqadi</p>
          </div>
        </div>
        <button
          onClick={openCreate}
          className="inline-flex items-center gap-2 rounded-xl bg-orange-500 px-4 py-2.5 text-sm font-semibold text-white shadow-lg shadow-orange-500/20 hover:bg-orange-400"
        >
          <Plus size={16} /> Yangi e&apos;lon
        </button>
      </header>

      {/* Stats / filters */}
      <div className="mb-5 grid grid-cols-2 gap-3 sm:grid-cols-4">
        {(["active", "scheduled", "ended", "off"] as Status[]).map((s) => (
          <button
            key={s}
            onClick={() => setFilter(filter === s ? "all" : s)}
            className={`rounded-2xl border p-4 text-left transition ${
              filter === s ? "border-orange-500/50 bg-orange-500/[0.06]" : "border-white/10 bg-[#12121a] hover:border-white/20"
            }`}
          >
            <span className="flex items-center gap-2 text-xs text-gray-400">
              <span className={`h-2 w-2 rounded-full ${STATUS_META[s].dot} ${s === "active" && counts.active ? "animate-pulse" : ""}`} />
              {STATUS_META[s].label}
            </span>
            <span className="mt-1 block text-2xl font-bold text-white">{counts[s]}</span>
          </button>
        ))}
      </div>

      <div className="scrollbar-hide mb-4 flex gap-1 overflow-x-auto rounded-xl border border-white/10 bg-black/20 p-1">
        {FILTERS.map((f) => (
          <button
            key={f.key}
            onClick={() => setFilter(f.key)}
            className={`whitespace-nowrap rounded-lg px-3 py-1.5 text-xs font-medium transition ${
              filter === f.key ? "bg-white/10 text-white" : "text-gray-400 hover:text-white"
            }`}
          >
            {f.label} <span className="ml-1 text-gray-500">{counts[f.key]}</span>
          </button>
        ))}
      </div>

      {error && <div className="mb-4 rounded-xl border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-300">{error}</div>}

      {/* List */}
      {loading ? (
        <div className="space-y-3">
          {[0, 1, 2].map((i) => (
            <div key={i} className="h-28 animate-pulse rounded-2xl border border-white/5 bg-white/[0.03]" />
          ))}
        </div>
      ) : visible.length === 0 ? (
        <div className="flex flex-col items-center rounded-2xl border border-dashed border-white/10 bg-[#12121a] px-6 py-14 text-center">
          <span className="flex h-14 w-14 items-center justify-center rounded-2xl bg-white/5">
            <Megaphone size={24} className="text-gray-500" />
          </span>
          <p className="mt-4 font-medium text-white">{filter === "all" ? "Hali e'lon yo'q" : "Bu bo'limda e'lon yo'q"}</p>
          <p className="mt-1 max-w-sm text-sm text-gray-500">Texnik ishlar, yangi funksiya yoki aksiya haqida foydalanuvchilarga xabar bering.</p>
          <button onClick={openCreate} className="mt-5 inline-flex items-center gap-2 rounded-xl bg-white/10 px-4 py-2 text-sm text-white hover:bg-white/15">
            <Plus size={15} /> E&apos;lon yaratish
          </button>
        </div>
      ) : (
        <ul className="space-y-3">
          {visible.map((a) => {
            const st = statusOf(a);
            const theme = announcementTheme(a.variant);
            const Icon = a.type === "alert" ? AlertTriangle : theme.icon;
            const start = new Date(a.starts_at).getTime();
            const end = new Date(a.ends_at).getTime();
            const now = Date.now();
            const pct = st === "active" ? Math.min(100, Math.max(0, ((now - start) / (end - start)) * 100)) : st === "ended" ? 100 : 0;
            return (
              <li key={a.id} className={`group rounded-2xl border border-white/10 bg-[#12121a] p-4 transition hover:border-white/20 ${st === "ended" || st === "off" ? "opacity-70" : ""}`}>
                <div className="flex gap-4">
                  {a.image_url && a.type !== "alert" ? (
                    // eslint-disable-next-line @next/next/no-img-element
                    <img src={a.image_url} alt="" className="hidden h-20 w-32 shrink-0 rounded-xl object-cover sm:block" />
                  ) : (
                    <span
                      className={`flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-gradient-to-br ${
                        a.type === "alert" ? "from-red-500 to-red-700" : theme.gradient
                      }`}
                    >
                      <Icon size={20} className="text-white" />
                    </span>
                  )}
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-1.5">
                      <span className={`inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-[11px] font-medium ${STATUS_META[st].chip}`}>
                        <span className={`h-1.5 w-1.5 rounded-full ${STATUS_META[st].dot}`} />
                        {STATUS_META[st].label}
                      </span>
                      <span className="rounded-full bg-white/5 px-2 py-0.5 text-[11px] text-gray-400">
                        {a.type === "alert" ? "Tepadagi chiziq" : `Popup · ${theme.label}`}
                      </span>
                      {!a.dismissible && <span className="rounded-full bg-amber-500/15 px-2 py-0.5 text-[11px] text-amber-300">Majburiy</span>}
                      {a.priority > 0 && <span className="rounded-full bg-white/5 px-2 py-0.5 text-[11px] text-gray-400">#{a.priority}</span>}
                    </div>
                    <h3 className="mt-1.5 truncate font-semibold text-white">{a.title}</h3>
                    {a.body && <p className="mt-0.5 line-clamp-1 text-sm text-gray-400">{a.body}</p>}

                    <div className="mt-3">
                      <div className="h-1 overflow-hidden rounded-full bg-white/5">
                        <div className={`h-full rounded-full ${st === "active" ? "bg-emerald-400" : "bg-white/15"}`} style={{ width: `${pct}%` }} />
                      </div>
                      <div className="mt-1.5 flex flex-wrap items-center justify-between gap-2 text-[11px] text-gray-500">
                        <span className="inline-flex items-center gap-1">
                          <CalendarClock size={12} /> {fmt(a.starts_at)} → {fmt(a.ends_at)}
                        </span>
                        <span>
                          {st === "active" && `${humanSpan(end - now)} qoldi`}
                          {st === "scheduled" && `${humanSpan(start - now)} dan keyin boshlanadi`}
                          {st === "ended" && `${humanSpan(now - end)} oldin tugagan`}
                          {a.created_by_name && ` · ${a.created_by_name}`}
                        </span>
                      </div>
                    </div>
                  </div>
                  <div className="flex shrink-0 flex-col items-end justify-between gap-2">
                    <button
                      role="switch"
                      aria-checked={a.is_active}
                      aria-label={a.is_active ? "O'chirish" : "Yoqish"}
                      title={a.is_active ? "O'chirish" : "Yoqish"}
                      disabled={busyId === a.id}
                      onClick={() => toggleActive(a)}
                      className={`inline-flex h-6 w-11 items-center rounded-full p-0.5 transition-colors disabled:opacity-50 ${a.is_active ? "bg-emerald-500" : "bg-white/15"}`}
                    >
                      <span className={`h-5 w-5 rounded-full bg-white shadow transition-transform ${a.is_active ? "translate-x-5" : ""}`} />
                    </button>
                    <div className="flex gap-1">
                      {[
                        { icon: Eye, label: "Ko'rish", on: () => setPreviewOf(a) },
                        { icon: Pencil, label: "Tahrirlash", on: () => fillFrom(a, false) },
                        { icon: Copy, label: "Nusxa olish", on: () => fillFrom(a, true) },
                      ].map(({ icon: I, label, on }) => (
                        <button key={label} onClick={on} title={label} aria-label={label} className="rounded-lg p-2 text-gray-400 hover:bg-white/5 hover:text-white">
                          <I size={15} />
                        </button>
                      ))}
                      <button onClick={() => remove(a)} title="O'chirish" aria-label="O'chirish" className="rounded-lg p-2 text-gray-500 hover:bg-red-500/10 hover:text-red-400">
                        <Trash2 size={15} />
                      </button>
                    </div>
                  </div>
                </div>
              </li>
            );
          })}
        </ul>
      )}

      {/* Editor */}
      {editor && (
        <div className="fixed inset-0 z-[150] flex items-stretch justify-center sm:items-center sm:p-4">
          <div className="absolute inset-0 bg-black/70 backdrop-blur-sm" onClick={() => !saving && setEditor(null)} />
          <form
            onSubmit={submit}
            className="relative flex max-h-full w-full max-w-5xl flex-col overflow-hidden bg-[#0f0f16] sm:max-h-[92vh] sm:rounded-3xl sm:border sm:border-white/10"
          >
            <div className="flex items-center justify-between border-b border-white/5 px-5 py-4">
              <h2 className="font-semibold text-white">{editor.id ? "E'lonni tahrirlash" : "Yangi e'lon"}</h2>
              <button type="button" onClick={() => setEditor(null)} className="rounded-lg p-1.5 text-gray-500 hover:bg-white/5 hover:text-white" aria-label="Yopish">
                <X size={18} />
              </button>
            </div>

            <div className="grid min-h-0 flex-1 overflow-y-auto lg:grid-cols-[minmax(0,1fr)_400px] lg:overflow-hidden">
              {/* Form */}
              <div className="space-y-5 p-5 lg:overflow-y-auto">
                <Field label="Qayerda chiqadi">
                  <Segmented
                    ariaLabel="Turi"
                    value={form.type}
                    onChange={(v) => set("type", v)}
                    options={[
                      { value: "modal", label: "Popup oyna" },
                      { value: "alert", label: "Tepadagi chiziq" },
                    ]}
                  />
                </Field>

                {!isAlert && (
                  <Field label="Ko'rinishi">
                    <div className="grid grid-cols-3 gap-2 sm:grid-cols-6">
                      {(Object.keys(ANNOUNCEMENT_THEMES) as AnnouncementVariant[]).map((v) => {
                        const t = ANNOUNCEMENT_THEMES[v];
                        const active = (form.variant || "info") === v;
                        return (
                          <button
                            key={v}
                            type="button"
                            onClick={() => set("variant", v)}
                            className={`flex flex-col items-center gap-1.5 rounded-xl border p-2.5 text-[11px] transition ${
                              active ? "border-white/40 bg-white/[0.06] text-white" : "border-white/10 text-gray-400 hover:border-white/20 hover:text-white"
                            }`}
                          >
                            <span className={`flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br ${t.gradient}`}>
                              <t.icon size={16} className="text-white" />
                            </span>
                            {t.label}
                          </button>
                        );
                      })}
                    </div>
                  </Field>
                )}

                <div className="space-y-4 rounded-2xl border border-white/10 bg-[#12121a] p-4">
                  <p className="flex items-center gap-2 text-sm font-semibold text-white">
                    <Type size={15} className="text-orange-400" /> Matn
                  </p>
                  <Field label={isAlert ? "Chiziqdagi matn" : "Sarlavha"} required right={<span className="text-[11px] text-gray-600">{form.title.length}/120</span>}>
                    <input
                      value={form.title}
                      maxLength={120}
                      onChange={(e) => set("title", e.target.value)}
                      placeholder={isAlert ? "Bugun 19:00–20:00 texnik ishlar" : "Yangi imkoniyat: birga tomosha qiling!"}
                      className={inputCls}
                    />
                  </Field>
                  <Field label={isAlert ? "Qo'shimcha (ixtiyoriy)" : "Matn"} right={<span className="text-[11px] text-gray-600">{form.body.length}/600</span>}>
                    <textarea
                      rows={isAlert ? 2 : 4}
                      maxLength={600}
                      value={form.body}
                      onChange={(e) => set("body", e.target.value)}
                      placeholder={isAlert ? "Noqulaylik uchun uzr so'raymiz" : "Do'stlaringiz bilan bir vaqtda kino ko'ring va chatda gaplashing..."}
                      className={`${inputCls} resize-none`}
                    />
                  </Field>
                </div>

                {!isAlert && (
                  <>
                    <MediaUploadField
                      label="Muqova rasmi (ixtiyoriy)"
                      kind="backdrop"
                      value={form.image_url}
                      onChange={(u) => set("image_url", u)}
                      onBusyChange={setImageBusy}
                      hint="16:9 tavsiya etiladi. Rasm bo'lmasa, mavzu belgisi chiqadi."
                      upload={async (file) => {
                        if (!token) throw new Error("Tizimga qayta kiring");
                        return { url: await uploadTelegramPostMedia(token, file) };
                      }}
                    />
                    <div className="space-y-4 rounded-2xl border border-white/10 bg-[#12121a] p-4">
                      <p className="flex items-center gap-2 text-sm font-semibold text-white">
                        <MousePointerClick size={15} className="text-orange-400" /> Tugma (ixtiyoriy)
                      </p>
                      <div className="grid gap-3 sm:grid-cols-[1fr_180px]">
                        <Field label="Havola">
                          <div className="relative">
                            <Link2 size={14} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" />
                            <input
                              type="url"
                              value={form.link_url}
                              onChange={(e) => set("link_url", e.target.value)}
                              placeholder="https://filmorauz.net/premium"
                              className={`${inputCls} pl-9`}
                            />
                          </div>
                        </Field>
                        <Field label="Tugma matni">
                          <input value={form.link_label} onChange={(e) => set("link_label", e.target.value)} placeholder="Batafsil" className={inputCls} />
                        </Field>
                      </div>
                    </div>
                  </>
                )}

                <div className="space-y-4 rounded-2xl border border-white/10 bg-[#12121a] p-4">
                  <p className="flex items-center gap-2 text-sm font-semibold text-white">
                    <CalendarClock size={15} className="text-orange-400" /> Vaqt
                  </p>
                  <div className="grid gap-3 sm:grid-cols-2">
                    <Field label="Boshlanish" required>
                      <input type="datetime-local" value={startLocal} onChange={(e) => setStartLocal(e.target.value)} className={`${inputCls} [color-scheme:dark]`} />
                    </Field>
                    <Field label="Tugash" required>
                      <input type="datetime-local" value={endLocal} onChange={(e) => setEndLocal(e.target.value)} className={`${inputCls} [color-scheme:dark]`} />
                    </Field>
                  </div>
                  <div className="flex flex-wrap gap-1.5">
                    <button type="button" onClick={() => setStartLocal(toLocal(new Date()))} className="rounded-full border border-white/10 px-2.5 py-1 text-[11px] text-gray-300 hover:border-orange-500/50 hover:text-white">
                      Hozirdan
                    </button>
                    {DURATIONS.map((d) => (
                      <button key={d.label} type="button" onClick={() => applyDuration(d.ms)} className="rounded-full border border-white/10 px-2.5 py-1 text-[11px] text-gray-300 hover:border-orange-500/50 hover:text-white">
                        +{d.label}
                      </button>
                    ))}
                  </div>
                </div>

                <div className="space-y-3 rounded-2xl border border-white/10 bg-[#12121a] p-4">
                  <p className="flex items-center gap-2 text-sm font-semibold text-white">
                    <Settings2 size={15} className="text-orange-400" /> Sozlamalar
                  </p>
                  <SwitchRow checked={form.is_active} onChange={(v) => set("is_active", v)} title="Faol" description="O'chirilgan e'lon vaqti kelsa ham chiqmaydi" />
                  {!isAlert && (
                    <SwitchRow
                      checked={form.dismissible}
                      onChange={(v) => set("dismissible", v)}
                      title="Yopib bo'ladi"
                      description="O'chirilsa, foydalanuvchi faqat tugmani bosib yopa oladi"
                    />
                  )}
                  <Field label="Navbat (priority)" hint="Bir vaqtda bir nechta e'lon bo'lsa, kattasi birinchi chiqadi">
                    <input type="number" value={form.priority} onChange={(e) => set("priority", Number(e.target.value) || 0)} className={`${inputCls} max-w-[140px]`} />
                  </Field>
                </div>
              </div>

              {/* Live preview */}
              <div className="border-t border-white/5 bg-[radial-gradient(ellipse_at_top,rgba(249,115,22,0.08),transparent_60%)] p-5 lg:overflow-y-auto lg:border-l lg:border-t-0">
                <p className="mb-3 text-[11px] font-semibold uppercase tracking-wider text-gray-500">Jonli ko&apos;rinish</p>
                {isAlert ? (
                  <div className="overflow-hidden rounded-2xl border border-white/10 bg-[#0b0b10]">
                    <div className="flex items-center justify-center gap-1.5 bg-red-600 px-3 py-1.5 text-center text-[11px] font-medium text-white">
                      <AlertTriangle size={12} className="shrink-0" />
                      <span>
                        {form.title || "Chiziqdagi matn"}
                        {form.body ? ` — ${form.body}` : ""}
                      </span>
                    </div>
                    <div className="space-y-2 p-4">
                      <div className="h-3 w-24 rounded bg-white/10" />
                      <div className="h-20 rounded-xl bg-white/5" />
                      <div className="grid grid-cols-3 gap-2">
                        {[0, 1, 2].map((i) => (
                          <div key={i} className="aspect-[2/3] rounded-lg bg-white/5" />
                        ))}
                      </div>
                    </div>
                  </div>
                ) : (
                  <div className="flex justify-center">
                    <AnnouncementModal key={`${form.variant}-${form.image_url}`} a={previewData} overlay={false} onClose={() => undefined} />
                  </div>
                )}
              </div>
            </div>

            <div className="flex items-center justify-between gap-3 border-t border-white/5 bg-[#0f0f16] px-5 py-3">
              <p className="min-w-0 truncate text-sm text-red-400">{formError}</p>
              <div className="flex shrink-0 gap-2">
                <button type="button" onClick={() => setEditor(null)} className="rounded-xl px-4 py-2 text-sm text-gray-300 hover:bg-white/5">
                  Bekor qilish
                </button>
                <button
                  type="submit"
                  disabled={saving || imageBusy}
                  className="inline-flex items-center gap-2 rounded-xl bg-orange-500 px-5 py-2 text-sm font-semibold text-white hover:bg-orange-400 disabled:opacity-60"
                >
                  {saving && <Loader2 size={15} className="animate-spin" />}
                  {editor.id ? "Saqlash" : "E'lon qilish"}
                </button>
              </div>
            </div>
          </form>
        </div>
      )}

      {previewOf &&
        (previewOf.type === "alert" ? (
          <div className="fixed inset-x-0 top-0 z-[210] flex items-center justify-center gap-2 bg-red-600 px-3 py-2 text-xs font-medium text-white">
            <AlertTriangle size={13} /> {previewOf.title}
            {previewOf.body ? ` — ${previewOf.body}` : ""}
            <button onClick={() => setPreviewOf(null)} className="ml-3 rounded bg-black/20 px-2 py-0.5 hover:bg-black/40">
              Yopish
            </button>
          </div>
        ) : (
          <AnnouncementModal a={{ ...previewOf, dismissible: true }} onClose={() => setPreviewOf(null)} onLink={() => undefined} />
        ))}
    </div>
  );
}
