"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { ArrowUpDown, Clock, Gauge, Link2, Loader2, MessageSquare, Minus, Plus, Reply, Settings, Shield, ShieldAlert, ShieldCheck } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { getCommentSettings, updateCommentSettings, CommentModerationSettings } from "@/lib/comments-api";
import { useToast } from "@/components/admin/Toast";
import { PageHead } from "@/components/admin/kit";
import { FormSection, Segmented, StickySaveBar, useLeaveGuard } from "@/components/admin/form/ui";
import ChipsInput from "@/components/admin/form/ChipsInput";

type Editable = Omit<CommentModerationSettings, "id" | "updated_at">;

function pick(s: CommentModerationSettings): Editable {
  return {
    comments_enabled: s.comments_enabled,
    replies_enabled: s.replies_enabled,
    block_links: s.block_links,
    max_comment_length: s.max_comment_length,
    max_reply_depth: s.max_reply_depth,
    require_moderation: s.require_moderation,
    banned_words: s.banned_words || [],
    auto_hide_banned_content: s.auto_hide_banned_content,
    comment_cooldown_seconds: s.comment_cooldown_seconds,
    max_comments_per_minute: s.max_comments_per_minute,
    default_sort: s.default_sort,
  };
}

/** Title + description on the left, a switch on the right. */
function Toggle({ checked, onChange, title, desc, icon: Icon }: { checked: boolean; onChange: (v: boolean) => void; title: string; desc: string; icon: typeof Shield }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      onClick={() => onChange(!checked)}
      className={`flex w-full items-center gap-3 rounded-xl border px-4 py-3 text-left transition ${
        checked ? "border-emerald-500/30 bg-emerald-500/[0.05]" : "border-white/10 bg-black/20 hover:border-white/20"
      }`}
    >
      <span className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-lg ${checked ? "bg-emerald-500/15 text-emerald-400" : "bg-white/5 text-gray-500"}`}>
        <Icon size={17} />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block text-sm font-medium text-white">{title}</span>
        <span className="block text-xs text-gray-500">{desc}</span>
      </span>
      <span className={`inline-flex h-6 w-11 shrink-0 items-center rounded-full p-0.5 transition-colors ${checked ? "bg-emerald-500" : "bg-white/15"}`}>
        <span className={`h-5 w-5 rounded-full bg-white shadow transition-transform ${checked ? "translate-x-5" : ""}`} />
      </span>
    </button>
  );
}

/** Number with −/+ steppers. */
function Stepper({ value, onChange, min, max, step = 1, suffix, title, desc, icon: Icon }: { value: number; onChange: (v: number) => void; min: number; max: number; step?: number; suffix?: string; title: string; desc: string; icon: typeof Shield }) {
  const clamp = (n: number) => Math.min(max, Math.max(min, n));
  return (
    <div className="flex flex-wrap items-center gap-3 rounded-xl border border-white/10 bg-black/20 px-4 py-3">
      <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-white/5 text-gray-400">
        <Icon size={17} />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block text-sm font-medium text-white">{title}</span>
        <span className="block text-xs text-gray-500">{desc}</span>
      </span>
      <div className="flex items-center rounded-xl border border-white/10 bg-black/30">
        <button type="button" onClick={() => onChange(clamp(value - step))} disabled={value <= min} className="p-2 text-gray-400 hover:text-white disabled:opacity-30" aria-label="Kamaytirish">
          <Minus size={14} />
        </button>
        <input
          type="number"
          value={value}
          min={min}
          max={max}
          onChange={(e) => onChange(clamp(parseInt(e.target.value) || min))}
          className="w-16 bg-transparent text-center text-sm font-semibold tabular-nums text-white focus:outline-none [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none"
        />
        <button type="button" onClick={() => onChange(clamp(value + step))} disabled={value >= max} className="p-2 text-gray-400 hover:text-white disabled:opacity-30" aria-label="Oshirish">
          <Plus size={14} />
        </button>
      </div>
      {suffix && <span className="w-12 text-xs text-gray-500">{suffix}</span>}
    </div>
  );
}

export default function CommentSettingsPage() {
  const { token, isLoading: authLoading, user } = useAuth();
  const router = useRouter();
  const toast = useToast();
  const [form, setForm] = useState<Editable | null>(null);
  const [saved, setSaved] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  const allowed = user?.role === "admin" || user?.role === "superadmin";

  useEffect(() => {
    if (!authLoading && (!token || !allowed)) router.push("/");
  }, [authLoading, token, allowed, router]);

  useEffect(() => {
    if (!token) return;
    getCommentSettings(token)
      .then((s) => {
        const p = pick(s);
        setForm(p);
        setSaved(JSON.stringify(p));
      })
      .catch(() => toast.error("Sozlamalarni yuklab bo'lmadi"))
      .finally(() => setLoading(false));
  }, [token, toast]);

  const dirty = useMemo(() => !!form && JSON.stringify(form) !== saved, [form, saved]);
  useLeaveGuard(dirty);

  const set = <K extends keyof Editable>(k: K, v: Editable[K]) => setForm((f) => (f ? { ...f, [k]: v } : f));

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token || !form) return;
    setSaving(true);
    try {
      const updated = pick(await updateCommentSettings(token, form));
      setForm(updated);
      setSaved(JSON.stringify(updated));
      toast.success("Sozlamalar saqlandi");
    } catch {
      toast.error("Saqlashda xatolik yuz berdi");
    } finally {
      setSaving(false);
    }
  };

  if (authLoading || !token || !allowed || loading) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="animate-spin text-gray-500" />
      </div>
    );
  }
  if (!form) {
    return <p className="p-8 text-center text-gray-500">Sozlamalar topilmadi</p>;
  }

  return (
    <form id="comment-settings" onSubmit={submit} className="mx-auto max-w-3xl p-4 sm:p-6">
      <PageHead icon={Settings} gradient="from-sky-500 to-indigo-600" title="Izoh sozlamalari" subtitle="Izohlar qanday ishlashi va qanday moderatsiya qilinishi" />

      {!form.comments_enabled && (
        <div className="mb-4 flex items-center gap-2 rounded-xl border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-sm text-amber-200">
          <ShieldAlert size={16} className="shrink-0" /> Izohlar o&apos;chirilgan — saytda hech kim izoh qoldira olmaydi.
        </div>
      )}

      <div className="space-y-5">
        <FormSection title="Umumiy" icon={<MessageSquare size={16} />}>
          <Toggle icon={MessageSquare} checked={form.comments_enabled} onChange={(v) => set("comments_enabled", v)} title="Izohlar yoqilgan" desc="Foydalanuvchilar izoh qoldira oladi" />
          <Toggle icon={Reply} checked={form.replies_enabled} onChange={(v) => set("replies_enabled", v)} title="Javoblar yoqilgan" desc="Izohlarga javob yozish mumkin" />
        </FormSection>

        <FormSection title="Moderatsiya" description="Mos kelgan izohlar avtomatik «Kutilmoqda» holatiga o'tadi" icon={<Shield size={16} />}>
          <Toggle icon={ShieldCheck} checked={form.require_moderation} onChange={(v) => set("require_moderation", v)} title="Hamma izoh tekshiruvdan o'tsin" desc="Admin tasdiqlamaguncha izoh saytda ko'rinmaydi" />
          <Toggle icon={Link2} checked={form.block_links} onChange={(v) => set("block_links", v)} title="Havolalarni bloklash" desc="Ichida URL bo'lgan izohlar tekshiruvga tushadi" />
          <Toggle icon={ShieldAlert} checked={form.auto_hide_banned_content} onChange={(v) => set("auto_hide_banned_content", v)} title="Taqiqlangan so'zli izohni yashirish" desc="Ro'yxatdagi so'z bo'lsa, izoh darhol yashiriladi" />
          <div className="rounded-xl border border-white/10 bg-black/20 p-4">
            <div className="mb-2 flex items-baseline justify-between">
              <p className="text-sm font-medium text-white">Taqiqlangan so&apos;zlar</p>
              <span className="text-xs text-gray-500">{form.banned_words.length} ta</span>
            </div>
            <ChipsInput value={form.banned_words} onChange={(v) => set("banned_words", v)} placeholder="So'z yozib Enter bosing yoki vergul bilan ro'yxat qo'ying" max={500} />
          </div>
        </FormSection>

        <FormSection title="Cheklovlar va spamga qarshi" icon={<Gauge size={16} />}>
          <Stepper icon={MessageSquare} title="Izoh uzunligi" desc="Eng ko'p belgilar soni" value={form.max_comment_length} onChange={(v) => set("max_comment_length", v)} min={10} max={10000} step={100} suffix="belgi" />
          <Stepper icon={Reply} title="Javob chuqurligi" desc="Javobga javob necha qavatgacha" value={form.max_reply_depth} onChange={(v) => set("max_reply_depth", v)} min={1} max={10} suffix="qavat" />
          <Stepper icon={Clock} title="Izohlar orasidagi kutish" desc="Bitta foydalanuvchi uchun" value={form.comment_cooldown_seconds} onChange={(v) => set("comment_cooldown_seconds", v)} min={0} max={300} step={5} suffix="soniya" />
          <Stepper icon={Gauge} title="Daqiqasiga eng ko'p izoh" desc="Spamga qarshi himoya" value={form.max_comments_per_minute} onChange={(v) => set("max_comments_per_minute", v)} min={1} max={20} suffix="ta" />
        </FormSection>

        <FormSection title="Ko'rinish" icon={<ArrowUpDown size={16} />}>
          <p className="text-xs text-gray-500">Izohlar standart qaysi tartibda chiqadi</p>
          <Segmented
            ariaLabel="Tartib"
            value={form.default_sort}
            onChange={(v) => set("default_sort", v)}
            options={[
              { value: "newest", label: "Avval yangilari" },
              { value: "oldest", label: "Avval eskilari" },
            ]}
          />
        </FormSection>
      </div>

      <StickySaveBar formId="comment-settings" dirty={dirty} saving={saving} disabled={!dirty} />
    </form>
  );
}
