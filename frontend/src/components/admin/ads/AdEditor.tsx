"use client";

import { useCallback, useMemo, useState } from "react";
import { Bot, Globe, Link2, Loader2, Megaphone, MousePointerClick, Send, Settings2, Tv2, Wallet } from "lucide-react";
import { Ad, AdInput, AdStatus } from "@/lib/api";
import { Modal } from "@/components/admin/kit";
import { Field, Segmented, SwitchRow, inputCls } from "@/components/admin/form/ui";
import AdMediaSlot, { validateFifteenSecondVideo } from "./AdMediaSlot";

export const emptyAdInput = (): AdInput => ({
  title: "",
  description: "",
  image_url: "",
  video_url: "",
  target_url: "",
  call_to_action: "",
  placements: [],
  status: "draft",
  duration_days: 30,
  price: 0,
  priority: 0,
  banner_media_url: "",
  banner_media_type: "image",
  inline_media_url: "",
  inline_media_type: "image",
  fixed_bottom_media_url: "",
  fixed_bottom_media_type: "image",
  popup_media_url: "",
  popup_media_type: "image",
  player_overlay_media_url: "",
  player_overlay_media_type: "video",
  telegram_media_url: "",
  telegram_media_type: "image",
  telegram_channels: [],
  telegram_bot_enabled: false,
  telegram_channel_enabled: false,
  player_enabled: false,
});

export function adToInput(ad: Ad): AdInput {
  return {
    title: ad.title,
    description: ad.description || "",
    image_url: ad.image_url || "",
    video_url: ad.video_url || "",
    target_url: ad.target_url,
    call_to_action: ad.call_to_action || "",
    placements: ad.placements || [],
    status: ad.status,
    duration_days: ad.duration_days || 30,
    price: ad.price,
    priority: ad.priority || 0,
    banner_media_url: ad.banner_media_url || "",
    banner_media_type: ad.banner_media_type || "image",
    inline_media_url: ad.inline_media_url || "",
    inline_media_type: ad.inline_media_type || "image",
    fixed_bottom_media_url: ad.fixed_bottom_media_url || "",
    fixed_bottom_media_type: ad.fixed_bottom_media_type || "image",
    popup_media_url: ad.popup_media_url || "",
    popup_media_type: ad.popup_media_type || "image",
    player_overlay_media_url: ad.player_overlay_media_url || "",
    player_overlay_media_type: ad.player_overlay_media_type || "image",
    telegram_media_url: ad.telegram_media_url || "",
    telegram_media_type: ad.telegram_media_type || "image",
    telegram_channels: [],
    telegram_bot_enabled: ad.telegram_bot_enabled || false,
    telegram_channel_enabled: ad.telegram_channel_enabled || false,
    player_enabled: ad.player_enabled || false,
  };
}

const PLACEMENTS = [
  { value: "website", label: "Sayt", desc: "Banner, popup, player", icon: Globe },
  { value: "telegram_channel", label: "Telegram kanal", desc: "Kanallarga post", icon: Send },
  { value: "telegram_bot", label: "Telegram bot", desc: "Bot foydalanuvchilariga", icon: Bot },
];

const WEBSITE_SLOTS: { slot: keyof AdInput; typeKey: keyof AdInput; label: string; size: string; aspect: string; video?: boolean }[] = [
  { slot: "banner_media_url", typeKey: "banner_media_type", label: "Banner (sahifa tepasi)", size: "1200×300 · 4:1", aspect: "aspect-[4/1]" },
  { slot: "inline_media_url", typeKey: "inline_media_type", label: "Kontent orasida", size: "1200×400 · 3:1", aspect: "aspect-[3/1]" },
  { slot: "fixed_bottom_media_url", typeKey: "fixed_bottom_media_type", label: "Pastki qotirilgan", size: "1200×180", aspect: "aspect-[20/3]" },
  { slot: "popup_media_url", typeKey: "popup_media_type", label: "Popup oyna", size: "900×600 · 3:2", aspect: "aspect-[3/2]" },
  { slot: "player_overlay_media_url", typeKey: "player_overlay_media_type", label: "Player ichida (video)", size: "~15 soniya", aspect: "aspect-video", video: true },
];

const CTA_SUGGESTIONS = ["Batafsil", "Ko'rish", "Sotib olish", "Obuna bo'lish", "Yuklab olish"];
const DURATIONS = [7, 14, 30, 90];

function Section({ icon: Icon, title, children, desc }: { icon: typeof Globe; title: string; desc?: string; children: React.ReactNode }) {
  return (
    <section className="space-y-4 rounded-2xl border border-white/10 bg-[#12121a] p-4 sm:p-5">
      <div className="flex items-start gap-2.5">
        <span className="mt-0.5 flex h-7 w-7 items-center justify-center rounded-lg bg-orange-500/10 text-orange-400">
          <Icon size={15} />
        </span>
        <div>
          <p className="text-sm font-semibold text-white">{title}</p>
          {desc && <p className="text-xs text-gray-500">{desc}</p>}
        </div>
      </div>
      {children}
    </section>
  );
}

/**
 * Create/edit ad. Owns its form state, so typing doesn't re-render the ads
 * list behind it; media slots own their upload state.
 */
export default function AdEditor({
  ad,
  token,
  onClose,
  onSave,
}: {
  ad: Ad | null;
  token: string | null;
  onClose: () => void;
  onSave: (input: AdInput) => Promise<void>;
}) {
  const [form, setForm] = useState<AdInput>(() => (ad ? adToInput(ad) : emptyAdInput()));
  const [busySlots, setBusySlots] = useState<Record<string, boolean>>({});
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  const set = useCallback(<K extends keyof AdInput>(k: K, v: AdInput[K]) => setForm((f) => ({ ...f, [k]: v })), []);
  const onSlotChange = useCallback((slot: string, url: string, type: "image" | "video") => {
    setForm((f) => {
      const next: AdInput = { ...f, [slot]: url };
      const typeKey = slot.replace(/_url$/, "_type") as keyof AdInput;
      (next as unknown as Record<string, unknown>)[typeKey] = type;
      return next;
    });
  }, []);
  const onSlotBusy = useCallback((slot: string, busy: boolean) => setBusySlots((b) => ({ ...b, [slot]: busy })), []);
  const uploading = Object.values(busySlots).some(Boolean);

  const has = (p: string) => form.placements.includes(p);
  const togglePlacement = (p: string) =>
    setForm((f) => {
      const on = !f.placements.includes(p);
      const placements = on ? [...f.placements, p] : f.placements.filter((x) => x !== p);
      return {
        ...f,
        placements,
        // Keep the delivery switches in step with the chosen placements.
        ...(p === "telegram_channel" ? { telegram_channel_enabled: on } : {}),
        ...(p === "telegram_bot" ? { telegram_bot_enabled: on } : {}),
      };
    });

  const missing = useMemo(() => {
    const m: string[] = [];
    if (!form.title.trim()) m.push("nomi");
    if (!form.target_url.trim()) m.push("havola");
    if (form.placements.length === 0) m.push("joylashuv");
    return m;
  }, [form.title, form.target_url, form.placements]);

  const save = async () => {
    if (missing.length) return setError(`To'ldiring: ${missing.join(", ")}`);
    if (!/^https?:\/\//.test(form.target_url.trim()) && !form.target_url.trim().startsWith("/")) {
      return setError("Havola https:// yoki / bilan boshlanishi kerak");
    }
    setSaving(true);
    setError("");
    try {
      await onSave({ ...form, title: form.title.trim(), target_url: form.target_url.trim() });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Saqlab bo'lmadi");
      setSaving(false);
    }
  };

  const tg = has("telegram_channel") || has("telegram_bot");

  return (
    <Modal
      open
      onClose={onClose}
      busy={saving}
      size="xl"
      icon={Megaphone}
      title={ad ? "Reklamani tahrirlash" : "Yangi reklama"}
      subtitle={ad ? ad.title : "Sayt va Telegram uchun reklama"}
      footer={
        <>
          <p className="mr-auto min-w-0 truncate text-xs text-red-400">{error || (uploading ? <span className="text-gray-500">Media yuklanmoqda...</span> : "")}</p>
          <button type="button" onClick={onClose} disabled={saving} className="rounded-xl px-4 py-2 text-sm text-gray-300 hover:bg-white/5">
            Bekor qilish
          </button>
          <button
            type="button"
            onClick={save}
            disabled={saving || uploading}
            className="inline-flex items-center gap-2 rounded-xl bg-orange-500 px-5 py-2 text-sm font-semibold text-white hover:bg-orange-400 disabled:opacity-50"
          >
            {saving && <Loader2 size={15} className="animate-spin" />}
            {ad ? "Saqlash" : "Yaratish"}
          </button>
        </>
      }
    >
      <div className="space-y-4">
        <Section icon={MousePointerClick} title="Asosiy">
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Nomi" required>
              <input value={form.title} onChange={(e) => set("title", e.target.value)} placeholder="Masalan: Uzum Market — kuzgi chegirma" className={inputCls} />
            </Field>
            <Field label="Havola" required hint="Bosilganda ochiladi">
              <div className="relative">
                <Link2 size={14} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" />
                <input value={form.target_url} onChange={(e) => set("target_url", e.target.value)} placeholder="https://..." className={`${inputCls} pl-9`} />
              </div>
            </Field>
          </div>
          <Field label="Qisqa tavsif">
            <textarea rows={2} value={form.description} onChange={(e) => set("description", e.target.value)} placeholder="Reklama haqida bir-ikki gap" className={`${inputCls} resize-none`} />
          </Field>
          <Field label="Tugma matni">
            <input value={form.call_to_action} onChange={(e) => set("call_to_action", e.target.value)} placeholder="Batafsil" className={inputCls} />
            <div className="mt-2 flex flex-wrap gap-1.5">
              {CTA_SUGGESTIONS.map((c) => (
                <button
                  key={c}
                  type="button"
                  onClick={() => set("call_to_action", c)}
                  className={`rounded-full border px-2.5 py-1 text-[11px] transition ${
                    form.call_to_action === c ? "border-orange-500/60 bg-orange-500/10 text-white" : "border-white/10 text-gray-400 hover:text-white"
                  }`}
                >
                  {c}
                </button>
              ))}
            </div>
          </Field>
        </Section>

        <Section icon={Globe} title="Qayerda ko'rsatiladi" desc="Bir nechtasini tanlash mumkin">
          <div className="grid gap-2.5 sm:grid-cols-3">
            {PLACEMENTS.map((p) => {
              const on = has(p.value);
              return (
                <button
                  key={p.value}
                  type="button"
                  role="checkbox"
                  aria-checked={on}
                  onClick={() => togglePlacement(p.value)}
                  className={`flex items-center gap-3 rounded-xl border p-3 text-left transition ${
                    on ? "border-orange-500/50 bg-orange-500/[0.07]" : "border-white/10 bg-black/20 hover:border-white/20"
                  }`}
                >
                  <span className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-lg ${on ? "bg-orange-500 text-white" : "bg-white/5 text-gray-400"}`}>
                    <p.icon size={17} />
                  </span>
                  <span className="min-w-0">
                    <span className="block text-sm font-medium text-white">{p.label}</span>
                    <span className="block truncate text-[11px] text-gray-500">{p.desc}</span>
                  </span>
                </button>
              );
            })}
          </div>
        </Section>

        {has("website") && (
          <Section icon={Globe} title="Sayt uchun rasmlar" desc="Bo'sh qolgan joyga banner rasmi ishlatiladi">
            <div className="grid gap-4 sm:grid-cols-2">
              {WEBSITE_SLOTS.map((s) => (
                <AdMediaSlot
                  key={s.slot}
                  slot={s.slot}
                  label={s.label}
                  size={s.size}
                  aspect={s.aspect}
                  value={(form[s.slot] as string) || ""}
                  mediaType={form[s.typeKey] as "image" | "video" | undefined}
                  image={!s.video}
                  video={!!s.video}
                  validate={s.video ? validateFifteenSecondVideo : undefined}
                  token={token}
                  onChange={onSlotChange}
                  onBusy={onSlotBusy}
                />
              ))}
            </div>
            <SwitchRow
              checked={!!form.player_enabled}
              onChange={(v) => set("player_enabled", v)}
              title="Player ichida ko'rsatish"
              description="Video pleyer ustida overlay reklama"
              icon={<Tv2 size={18} className={form.player_enabled ? "text-yellow-400" : "text-gray-500"} />}
            />
          </Section>
        )}

        {tg && (
          <Section icon={Send} title="Telegram">
            <div className="grid gap-4 sm:grid-cols-[220px_1fr]">
              <AdMediaSlot
                slot="telegram_media_url"
                label="Rasm yoki video"
                size="1:1 yoki 16:9"
                aspect="aspect-square"
                value={form.telegram_media_url || ""}
                mediaType={form.telegram_media_type}
                image
                video
                token={token}
                onChange={onSlotChange}
                onBusy={onSlotBusy}
              />
              <div className="space-y-2.5">
                <SwitchRow
                  checked={!!form.telegram_channel_enabled}
                  onChange={(v) => set("telegram_channel_enabled", v)}
                  title="Kanallarga yuborish"
                  description="Ro'yxatdagi Telegram kanallarga post"
                  icon={<Send size={18} className={form.telegram_channel_enabled ? "text-sky-400" : "text-gray-500"} />}
                />
                <SwitchRow
                  checked={!!form.telegram_bot_enabled}
                  onChange={(v) => set("telegram_bot_enabled", v)}
                  title="Bot orqali yuborish"
                  description="Botni ishga tushirgan foydalanuvchilarga"
                  icon={<Bot size={18} className={form.telegram_bot_enabled ? "text-sky-400" : "text-gray-500"} />}
                />
              </div>
            </div>
          </Section>
        )}

        <Section icon={Settings2} title="Holat va muddat">
          <Field label="Holat">
            <Segmented<AdStatus>
              ariaLabel="Holat"
              value={form.status}
              onChange={(v) => set("status", v)}
              options={[
                { value: "draft", label: "Qoralama" },
                { value: "active", label: "Faol" },
                { value: "paused", label: "To'xtatilgan" },
                { value: "expired", label: "Tugagan" },
              ]}
            />
          </Field>
          <div className="grid gap-4 sm:grid-cols-3">
            <Field label="Necha kun" hint="Faollashtirilgan paytdan boshlab">
              <input type="number" min={1} max={365} value={form.duration_days} onChange={(e) => set("duration_days", parseInt(e.target.value) || 1)} className={inputCls} />
              <div className="mt-2 flex gap-1.5">
                {DURATIONS.map((d) => (
                  <button
                    key={d}
                    type="button"
                    onClick={() => set("duration_days", d)}
                    className={`rounded-full border px-2.5 py-1 text-[11px] ${form.duration_days === d ? "border-orange-500/60 bg-orange-500/10 text-white" : "border-white/10 text-gray-400 hover:text-white"}`}
                  >
                    {d} kun
                  </button>
                ))}
              </div>
            </Field>
            <Field label="Narxi (USD)">
              <div className="relative">
                <Wallet size={14} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" />
                <input type="number" min={0} step="0.01" value={form.price} onChange={(e) => set("price", parseFloat(e.target.value) || 0)} className={`${inputCls} pl-9`} />
              </div>
            </Field>
            <Field label="Navbat (priority)" hint="Katta raqam oldin chiqadi">
              <input type="number" min={0} value={form.priority} onChange={(e) => set("priority", parseInt(e.target.value) || 0)} className={inputCls} />
            </Field>
          </div>
        </Section>
      </div>
    </Modal>
  );
}
