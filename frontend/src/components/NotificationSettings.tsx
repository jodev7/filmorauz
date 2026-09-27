"use client";

import { useEffect, useRef, useState } from "react";
import { Bell, BellOff, Globe, Loader2, Send, Smartphone, Check } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import {
  deletePushSubscription,
  getNotificationSettings,
  NotificationSettings as Settings,
  NotifyChannel,
  NotifyPrefs,
  saveNotificationSettings,
  savePushSubscription,
  sendTestPush,
} from "@/lib/api";
import { getPushState, PushState, subscribePush, unsubscribePush } from "@/lib/web-push";

const CATEGORY_META: Record<string, { label: string; hint: string; telegram: boolean }> = {
  new_episode: { label: "Yangi qismlar", hint: "Obuna bo'lgan serialingizga yangi qism chiqqanda", telegram: true },
  comments: { label: "Izohlar", hint: "Izohingizga javob yozishsa yoki yoqtirishsa", telegram: false },
  suggestions: { label: "Takliflar", hint: "Siz so'ragan kino saytga qo'shilganda", telegram: true },
  rooms: { label: "Birga ko'rish", hint: "Sizni xonaga taklif qilishsa", telegram: false },
  premium: { label: "Premium va bonuslar", hint: "Obuna muddati, referral bonuslari", telegram: true },
  account: { label: "Akkaunt", hint: "Ban va murojaatlar — saytda doim ko'rsatiladi", telegram: false },
};

const CHANNELS: { key: NotifyChannel; label: string; icon: React.ReactNode }[] = [
  { key: "site", label: "Sayt", icon: <Globe size={14} /> },
  { key: "telegram", label: "Telegram", icon: <Send size={14} /> },
  { key: "push", label: "Push", icon: <Smartphone size={14} /> },
];

function Toggle({ on, disabled, onChange, label }: { on: boolean; disabled?: boolean; onChange: () => void; label: string }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={on}
      aria-label={label}
      disabled={disabled}
      onClick={onChange}
      className={`relative h-6 w-11 shrink-0 rounded-full transition-colors disabled:cursor-not-allowed disabled:opacity-40 ${on ? "bg-orange-500" : "bg-white/15"}`}
    >
      <span className={`absolute top-0.5 h-5 w-5 rounded-full bg-white shadow transition-transform ${on ? "translate-x-[22px]" : "translate-x-0.5"}`} />
    </button>
  );
}

export default function NotificationSettings() {
  const { token } = useAuth();
  const [settings, setSettings] = useState<Settings | null>(null);
  const [prefs, setPrefs] = useState<NotifyPrefs | null>(null);
  const [saveState, setSaveState] = useState<"idle" | "saving" | "saved" | "error">("idle");
  const [push, setPush] = useState<PushState | null>(null);
  const [pushBusy, setPushBusy] = useState(false);
  const [pushMsg, setPushMsg] = useState("");
  const saveTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (!token) return;
    getNotificationSettings(token)
      .then((s) => {
        setSettings(s);
        setPrefs(s.prefs);
      })
      .catch(() => setSaveState("error"));
    getPushState().then(setPush).catch(() => setPush("unsupported"));
  }, [token]);

  const update = (cat: string, ch: NotifyChannel) => {
    if (!prefs || !token) return;
    const next: NotifyPrefs = { ...prefs, [cat]: { ...prefs[cat], [ch]: !prefs[cat][ch] } };
    setPrefs(next);
    setSaveState("saving");
    if (saveTimer.current) clearTimeout(saveTimer.current);
    saveTimer.current = setTimeout(() => {
      saveNotificationSettings(token, next)
        .then((saved) => {
          setPrefs(saved);
          setSaveState("saved");
        })
        .catch(() => setSaveState("error"));
    }, 500);
  };

  const enablePush = async () => {
    if (!token || !settings?.push_public_key) return;
    setPushBusy(true);
    setPushMsg("");
    try {
      const sub = await subscribePush(settings.push_public_key);
      await savePushSubscription(token, sub);
      setPush("enabled");
      setPushMsg("Push yoqildi. Sinov xabarini yuborib ko'ring.");
    } catch (e) {
      const m = e instanceof Error ? e.message : "";
      if (m === "denied") {
        setPush("denied");
      } else if (m !== "dismissed") {
        setPushMsg(m || "Yoqib bo'lmadi");
      }
    } finally {
      setPushBusy(false);
    }
  };

  const disablePush = async () => {
    if (!token) return;
    setPushBusy(true);
    try {
      const endpoint = await unsubscribePush();
      if (endpoint) await deletePushSubscription(token, endpoint);
      setPush("disabled");
      setPushMsg("Bu qurilmada push o'chirildi.");
    } finally {
      setPushBusy(false);
    }
  };

  const test = async () => {
    if (!token) return;
    setPushBusy(true);
    setPushMsg("");
    try {
      await sendTestPush(token);
      setPushMsg("Sinov xabari yuborildi — bir necha soniyada keladi.");
    } catch (e) {
      setPushMsg(e instanceof Error ? e.message : "Yuborib bo'lmadi");
    } finally {
      setPushBusy(false);
    }
  };

  if (!settings || !prefs) {
    return (
      <div className="space-y-3">
        {[0, 1, 2, 3].map((i) => (
          <div key={i} className="h-16 animate-pulse rounded-xl bg-white/5" />
        ))}
      </div>
    );
  }

  const pushConfigured = !!settings.push_public_key;

  return (
    <div className="space-y-6">
      {/* Push on this device */}
      <section className="glass-card rounded-2xl border border-white/10 p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <h2 className="flex items-center gap-2 font-semibold text-white">
              <Smartphone size={18} className="text-orange-400" /> Shu qurilmada push bildirishnomalar
            </h2>
            <p className="mt-1 text-sm text-gray-400">
              {!pushConfigured
                ? "Push bildirishnomalar hozircha serverda yoqilmagan."
                : push === "enabled"
                ? "Yoqilgan. Sayt yopiq bo'lsa ham telefon/kompyuteringizga xabar keladi."
                : push === "denied"
                ? "Brauzer sozlamalarida bildirishnomalar bloklangan. Sayt sozlamalaridan ruxsat bering."
                : push === "ios-install"
                ? "iPhone'da push ishlashi uchun saytni \"Ulashish → Uy ekraniga qo'shish\" orqali o'rnating va ilovadan oching."
                : push === "unsupported"
                ? "Bu brauzer push bildirishnomalarni qo'llab-quvvatlamaydi."
                : "Yangi qism chiqsa, sayt yopiq bo'lsa ham bildiramiz."}
            </p>
          </div>
          {pushConfigured && (push === "disabled" || push === "enabled") && (
            <div className="flex gap-2">
              {push === "enabled" ? (
                <>
                  <button onClick={test} disabled={pushBusy} className="rounded-lg border border-white/10 px-3 py-2 text-sm text-gray-200 hover:bg-white/5 disabled:opacity-50">
                    Sinab ko&apos;rish
                  </button>
                  <button onClick={disablePush} disabled={pushBusy} className="inline-flex items-center gap-1.5 rounded-lg border border-white/10 px-3 py-2 text-sm text-gray-300 hover:bg-white/5 disabled:opacity-50">
                    <BellOff size={15} /> O&apos;chirish
                  </button>
                </>
              ) : (
                <button onClick={enablePush} disabled={pushBusy} className="inline-flex items-center gap-1.5 rounded-lg bg-orange-500 px-4 py-2 text-sm font-medium text-white hover:bg-orange-600 disabled:opacity-50">
                  {pushBusy ? <Loader2 size={15} className="animate-spin" /> : <Bell size={15} />} Yoqish
                </button>
              )}
            </div>
          )}
        </div>
        {pushMsg && <p className="mt-3 text-sm text-gray-300" role="status">{pushMsg}</p>}
        {!settings.telegram_connected && (
          <p className="mt-3 text-xs text-gray-500">Telegram xabarlari uchun botga /start yuborgan bo&apos;lishingiz kerak.</p>
        )}
      </section>

      {/* Per-category matrix */}
      <section className="glass-card overflow-hidden rounded-2xl border border-white/10">
        <div className="flex items-center justify-between border-b border-white/10 px-5 py-3">
          <h2 className="font-semibold text-white">Qaysi xabarlar qayerga kelsin</h2>
          <span className="flex items-center gap-1 text-xs text-gray-500" role="status" aria-live="polite">
            {saveState === "saving" && (
              <>
                <Loader2 size={12} className="animate-spin" /> Saqlanmoqda
              </>
            )}
            {saveState === "saved" && (
              <>
                <Check size={12} className="text-emerald-400" /> Saqlandi
              </>
            )}
            {saveState === "error" && <span className="text-red-400">Saqlanmadi</span>}
          </span>
        </div>
        <div className="hidden grid-cols-[1fr_repeat(3,80px)] gap-2 border-b border-white/5 px-5 py-2 text-xs text-gray-500 sm:grid">
          <span />
          {CHANNELS.map((c) => (
            <span key={c.key} className="flex items-center justify-center gap-1">
              {c.icon} {c.label}
            </span>
          ))}
        </div>
        <ul className="divide-y divide-white/5">
          {settings.categories.map((cat) => {
            const meta = CATEGORY_META[cat] ?? { label: cat, hint: "", telegram: false };
            return (
              <li key={cat} className="grid grid-cols-1 gap-3 px-5 py-4 sm:grid-cols-[1fr_repeat(3,80px)] sm:items-center sm:gap-2">
                <div>
                  <p className="text-sm font-medium text-white">{meta.label}</p>
                  <p className="text-xs text-gray-500">{meta.hint}</p>
                </div>
                {CHANNELS.map((ch) => {
                  const unavailable =
                    (ch.key === "telegram" && !meta.telegram) || (ch.key === "site" && cat === "account") || (ch.key === "push" && !pushConfigured);
                  return (
                    <div key={ch.key} className="flex items-center justify-between gap-2 sm:justify-center">
                      <span className="flex items-center gap-1.5 text-xs text-gray-400 sm:hidden">
                        {ch.icon} {ch.label}
                      </span>
                      {ch.key === "telegram" && !meta.telegram ? (
                        <span className="w-11 text-center text-xs text-gray-600" title="Bu turdagi xabar Telegramga yuborilmaydi">—</span>
                      ) : (
                        <Toggle
                          on={!!prefs[cat]?.[ch.key]}
                          disabled={unavailable}
                          onChange={() => update(cat, ch.key)}
                          label={`${meta.label}: ${ch.label}`}
                        />
                      )}
                    </div>
                  );
                })}
              </li>
            );
          })}
        </ul>
      </section>
    </div>
  );
}
