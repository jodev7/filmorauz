"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Bot,
  CalendarClock,
  DollarSign,
  Eye,
  Globe,
  History,
  Loader2,
  Megaphone,
  MousePointerClick,
  Pause,
  Pencil,
  Play,
  Plus,
  Send,
  Trash2,
  TrendingUp,
  XCircle,
} from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import {
  Ad,
  AdDelivery,
  AdInput,
  AdStats,
  AdStatus,
  adminListAds,
  adminGetAdStats,
  adminCreateAd,
  adminUpdateAd,
  adminDeleteAd,
  adminSendTelegramAd,
  adminGetAdDelivery,
} from "@/lib/api";
import { normalizeMediaUrl } from "@/lib/image-utils";
import { useToast } from "@/components/admin/Toast";
import { Chip, EmptyState, IconBtn, Modal, PageHead, PrimaryButton, SearchBox, SkeletonList, StatTile, Tabs, Tone, fmtDate } from "@/components/admin/kit";
import AdEditor, { adToInput } from "@/components/admin/ads/AdEditor";
import { isVideoUrl } from "@/components/admin/ads/AdMediaSlot";

const STATUS: Record<AdStatus, { label: string; tone: Tone }> = {
  draft: { label: "Qoralama", tone: "gray" },
  active: { label: "Faol", tone: "green" },
  paused: { label: "To'xtatilgan", tone: "yellow" },
  expired: { label: "Tugagan", tone: "red" },
};

const PLACEMENT: Record<string, { label: string; icon: typeof Globe }> = {
  website: { label: "Sayt", icon: Globe },
  telegram_channel: { label: "Kanal", icon: Send },
  telegram_bot: { label: "Bot", icon: Bot },
};

function effectiveStatus(ad: Ad): AdStatus {
  if (ad.status === "active" && ad.ends_at && new Date(ad.ends_at) < new Date()) return "expired";
  return ad.status;
}

function thumbOf(ad: Ad): { url: string; video: boolean } | null {
  const pairs: [string | undefined, string | undefined][] = [
    [ad.banner_media_url, ad.banner_media_type],
    [ad.inline_media_url, ad.inline_media_type],
    [ad.popup_media_url, ad.popup_media_type],
    [ad.background_media_url, ad.background_media_type],
    [ad.telegram_media_url, ad.telegram_media_type],
    [ad.image_url, "image"],
    [ad.player_overlay_media_url, ad.player_overlay_media_type],
  ];
  for (const [u, t] of pairs) if (u) return { url: normalizeMediaUrl(u), video: isVideoUrl(u, t) };
  return null;
}

function remaining(ad: Ad): { text: string; pct: number; warn: boolean } | null {
  if (!ad.ends_at) return null;
  const end = new Date(ad.ends_at).getTime();
  const start = ad.starts_at ? new Date(ad.starts_at).getTime() : end - (ad.duration_days || 30) * 86400e3;
  const now = Date.now();
  const days = Math.ceil((end - now) / 86400e3);
  const pct = Math.min(100, Math.max(0, ((now - start) / Math.max(1, end - start)) * 100));
  if (days < 0) return { text: "Tugagan", pct: 100, warn: false };
  if (days === 0) return { text: "Bugun tugaydi", pct, warn: true };
  return { text: `${days} kun qoldi`, pct, warn: days <= 3 };
}

type Filter = "all" | AdStatus;

export default function AdminAdsPage() {
  const { token } = useAuth();
  const toast = useToast();
  const [ads, setAds] = useState<Ad[]>([]);
  const [stats, setStats] = useState<AdStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [editor, setEditor] = useState<{ ad: Ad | null } | null>(null);
  const [filter, setFilter] = useState<Filter>("all");
  const [query, setQuery] = useState("");
  const [busyId, setBusyId] = useState<string | null>(null);
  const [deliveryAd, setDeliveryAd] = useState<Ad | null>(null);
  const [deliveries, setDeliveries] = useState<AdDelivery[] | null>(null);

  const load = useCallback(async () => {
    if (!token) return;
    try {
      const [list, st] = await Promise.all([adminListAds(token), adminGetAdStats(token)]);
      setAds(list);
      setStats(st);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Yuklab bo'lmadi");
    } finally {
      setLoading(false);
    }
  }, [token, toast]);

  useEffect(() => {
    load();
  }, [load]);

  const counts = useMemo(() => {
    const c: Record<Filter, number> = { all: ads.length, draft: 0, active: 0, paused: 0, expired: 0 };
    ads.forEach((a) => c[effectiveStatus(a)]++);
    return c;
  }, [ads]);

  const visible = useMemo(() => {
    const q = query.trim().toLowerCase();
    return ads.filter((a) => (filter === "all" || effectiveStatus(a) === filter) && (!q || a.title.toLowerCase().includes(q) || (a.description || "").toLowerCase().includes(q)));
  }, [ads, filter, query]);

  const save = useCallback(
    async (input: AdInput) => {
      if (!token || !editor) return;
      if (editor.ad) await adminUpdateAd(token, editor.ad.id, input);
      else await adminCreateAd(token, input);
      toast.success(editor.ad ? "Reklama saqlandi" : "Reklama yaratildi");
      setEditor(null);
      await load();
    },
    [token, editor, load, toast]
  );

  const setStatus = async (ad: Ad, status: AdStatus) => {
    if (!token) return;
    setBusyId(ad.id);
    try {
      await adminUpdateAd(token, ad.id, { ...adToInput(ad), status });
      toast.success(status === "active" ? "Reklama yoqildi" : "Reklama to'xtatildi");
      await load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Xatolik");
    } finally {
      setBusyId(null);
    }
  };

  const sendTelegram = async (ad: Ad) => {
    if (!token || !confirm(`"${ad.title}" Telegramga yuborilsinmi?`)) return;
    setBusyId(ad.id);
    try {
      const res = await adminSendTelegramAd(token, ad.id);
      const ok = res.results.filter((r) => r.status === "success").length;
      const fail = res.results.filter((r) => r.status === "failed").length;
      if (fail) toast.error(`Telegram: ${ok} ta yuborildi, ${fail} ta xato`);
      else toast.success(`Telegram: ${ok} ta yuborildi`);
      await load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Telegramga yuborib bo'lmadi");
    } finally {
      setBusyId(null);
    }
  };

  const remove = async (ad: Ad) => {
    if (!token || !confirm(`"${ad.title}" reklamasini o'chirasizmi?`)) return;
    setBusyId(ad.id);
    try {
      await adminDeleteAd(token, ad.id);
      setAds((l) => l.filter((x) => x.id !== ad.id));
      toast.success("Reklama o'chirildi");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "O'chirib bo'lmadi");
    } finally {
      setBusyId(null);
    }
  };

  const openDelivery = async (ad: Ad) => {
    if (!token) return;
    setDeliveryAd(ad);
    setDeliveries(null);
    try {
      setDeliveries(await adminGetAdDelivery(token, ad.id));
    } catch {
      setDeliveries([]);
    }
  };

  const ctr = stats && stats.impressions > 0 ? ((stats.clicks / stats.impressions) * 100).toFixed(2) : "0";

  return (
    <div className="mx-auto max-w-7xl p-4 sm:p-6">
      <PageHead
        icon={Megaphone}
        title="Reklamalar"
        subtitle="Sayt va Telegram reklamalari, ko'rishlar va daromad"
        actions={
          <PrimaryButton onClick={() => setEditor({ ad: null })}>
            <Plus size={16} /> Yangi reklama
          </PrimaryButton>
        }
      />

      <div className="mb-3 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatTile icon={Play} tone="green" label="Faol reklamalar" value={stats?.active_ads ?? "—"} hint={stats ? `Jami ${stats.total_ads} ta` : undefined} />
        <StatTile icon={Eye} tone="blue" label="Ko'rishlar" value={stats ? stats.impressions.toLocaleString() : "—"} />
        <StatTile icon={MousePointerClick} tone="yellow" label="Bosishlar" value={stats ? stats.clicks.toLocaleString() : "—"} hint={stats ? `CTR ${ctr}%` : undefined} />
        <StatTile icon={DollarSign} tone="orange" label="Daromad" value={stats ? `$${stats.revenue.toFixed(2)}` : "—"} />
      </div>
      <div className="mb-6 flex flex-wrap gap-2 text-xs">
        <Chip tone="red" icon={TrendingUp}>
          Tugagan: {stats?.expired_ads ?? 0}
        </Chip>
        <Chip tone="sky" icon={Send}>
          Telegram yetkazildi: {(stats?.telegram_deliveries ?? 0).toLocaleString()}
        </Chip>
        {(stats?.telegram_failed ?? 0) > 0 && (
          <Chip tone="red" icon={XCircle}>
            Telegram xato: {stats?.telegram_failed}
          </Chip>
        )}
      </div>

      <div className="mb-4 flex flex-col gap-3 lg:flex-row">
        <SearchBox value={query} onChange={setQuery} placeholder="Reklama nomi bo'yicha qidirish..." />
        <Tabs<Filter>
          value={filter}
          onChange={setFilter}
          items={[
            { key: "all", label: "Hammasi", count: counts.all },
            { key: "active", label: "Faol", count: counts.active },
            { key: "draft", label: "Qoralama", count: counts.draft },
            { key: "paused", label: "To'xtatilgan", count: counts.paused },
            { key: "expired", label: "Tugagan", count: counts.expired },
          ]}
        />
      </div>

      {loading ? (
        <SkeletonList rows={4} height={104} />
      ) : visible.length === 0 ? (
        <EmptyState
          icon={Megaphone}
          title={ads.length === 0 ? "Hali reklama yo'q" : "Hech narsa topilmadi"}
          text={ads.length === 0 ? "Birinchi reklamani yarating — sayt bannerlari, popup yoki Telegram post." : "Filtr yoki qidiruvni o'zgartirib ko'ring."}
          action={
            ads.length === 0 ? (
              <PrimaryButton onClick={() => setEditor({ ad: null })}>
                <Plus size={16} /> Reklama yaratish
              </PrimaryButton>
            ) : undefined
          }
        />
      ) : (
        <ul className="space-y-3">
          {visible.map((ad) => {
            const st = effectiveStatus(ad);
            const thumb = thumbOf(ad);
            const rem = remaining(ad);
            const adCtr = ad.impressions > 0 ? ((ad.clicks / ad.impressions) * 100).toFixed(1) : "0";
            const busy = busyId === ad.id;
            return (
              <li key={ad.id} className={`rounded-2xl border border-white/10 bg-[#12121a] p-3.5 transition hover:border-white/20 sm:p-4 ${st === "expired" || st === "draft" ? "opacity-80" : ""}`}>
                <div className="flex flex-col gap-4 md:flex-row md:items-center">
                  <div className="flex min-w-0 flex-1 gap-3.5">
                    <div className="relative h-[72px] w-32 shrink-0 overflow-hidden rounded-xl bg-white/5">
                      {thumb ? (
                        thumb.video ? (
                          <video src={thumb.url} muted playsInline preload="metadata" className="h-full w-full object-cover" />
                        ) : (
                          // eslint-disable-next-line @next/next/no-img-element
                          <img src={thumb.url} alt="" loading="lazy" className="h-full w-full object-cover" />
                        )
                      ) : (
                        <span className="flex h-full w-full items-center justify-center">
                          <Megaphone size={20} className="text-gray-600" />
                        </span>
                      )}
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-1.5">
                        <Chip tone={STATUS[st].tone} dot>
                          {STATUS[st].label}
                        </Chip>
                        {ad.placements.map((p) => {
                          const meta = PLACEMENT[p];
                          return (
                            <Chip key={p} icon={meta?.icon}>
                              {meta?.label ?? p.replace(/_/g, " ")}
                            </Chip>
                          );
                        })}
                        {ad.priority > 0 && <Chip>#{ad.priority}</Chip>}
                      </div>
                      <h3 className="mt-1.5 truncate font-semibold text-white">{ad.title}</h3>
                      {ad.description && <p className="truncate text-sm text-gray-500">{ad.description}</p>}
                      {rem && st !== "draft" && (
                        <div className="mt-2 max-w-sm">
                          <div className="h-1 overflow-hidden rounded-full bg-white/5">
                            <div className={`h-full rounded-full ${st === "active" ? (rem.warn ? "bg-amber-400" : "bg-emerald-400") : "bg-white/15"}`} style={{ width: `${rem.pct}%` }} />
                          </div>
                          <p className={`mt-1 inline-flex items-center gap-1 text-[11px] ${rem.warn && st === "active" ? "text-amber-400" : "text-gray-500"}`}>
                            <CalendarClock size={11} /> {rem.text} · {fmtDate(ad.ends_at, false)} gacha
                          </p>
                        </div>
                      )}
                    </div>
                  </div>

                  <div className="grid grid-cols-4 gap-2 text-center md:w-[320px]">
                    {[
                      { label: "Ko'rish", value: ad.impressions.toLocaleString() },
                      { label: "Bosish", value: ad.clicks.toLocaleString() },
                      { label: "CTR", value: `${adCtr}%` },
                      { label: "Narx", value: `$${ad.price.toFixed(0)}` },
                    ].map((m) => (
                      <div key={m.label} className="rounded-xl bg-black/20 px-1 py-2">
                        <p className="text-sm font-semibold tabular-nums text-white">{m.value}</p>
                        <p className="text-[10px] text-gray-500">{m.label}</p>
                      </div>
                    ))}
                  </div>

                  <div className="flex items-center justify-end gap-0.5 md:flex-col md:items-end md:gap-1">
                    <div className="flex items-center gap-0.5">
                      {busy ? (
                        <Loader2 size={16} className="m-2 animate-spin text-gray-500" />
                      ) : st === "active" ? (
                        <IconBtn label="To'xtatish" tone="yellow" onClick={() => setStatus(ad, "paused")}>
                          <Pause size={15} />
                        </IconBtn>
                      ) : st !== "expired" ? (
                        <IconBtn label="Yoqish" tone="green" onClick={() => setStatus(ad, "active")}>
                          <Play size={15} />
                        </IconBtn>
                      ) : null}
                      {(ad.telegram_channel_enabled || ad.telegram_bot_enabled) && (
                        <IconBtn label="Telegramga yuborish" tone="blue" disabled={busy} onClick={() => sendTelegram(ad)}>
                          <Send size={15} />
                        </IconBtn>
                      )}
                      <IconBtn label="Yetkazish tarixi" onClick={() => openDelivery(ad)}>
                        <History size={15} />
                      </IconBtn>
                      <IconBtn label="Tahrirlash" onClick={() => setEditor({ ad })}>
                        <Pencil size={15} />
                      </IconBtn>
                      <IconBtn label="O'chirish" tone="red" disabled={busy} onClick={() => remove(ad)}>
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

      {editor && <AdEditor key={editor.ad?.id ?? "new"} ad={editor.ad} token={token} onClose={() => setEditor(null)} onSave={save} />}

      <Modal open={!!deliveryAd} onClose={() => setDeliveryAd(null)} icon={History} iconTone="sky" size="lg" title="Yetkazish tarixi" subtitle={deliveryAd?.title}>
        {deliveries === null ? (
          <SkeletonList rows={4} height={44} />
        ) : deliveries.length === 0 ? (
          <p className="py-10 text-center text-sm text-gray-500">Hali Telegramga yuborilmagan</p>
        ) : (
          <ul className="divide-y divide-white/5">
            {deliveries.map((d) => (
              <li key={d.id} className="flex items-center gap-3 py-2.5">
                <Chip tone={d.status === "success" ? "green" : "red"} dot>
                  {d.status === "success" ? "Yuborildi" : "Xato"}
                </Chip>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm text-white">{d.target}</p>
                  <p className="truncate text-[11px] text-gray-500">
                    {d.placement.replace(/_/g, " ")}
                    {d.error && <span className="text-red-400"> · {d.error}</span>}
                  </p>
                </div>
                <span className="shrink-0 text-[11px] text-gray-500">{fmtDate(d.sent_at)}</span>
              </li>
            ))}
          </ul>
        )}
      </Modal>
    </div>
  );
}
