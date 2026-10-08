"use client";

import { useMemo, useState } from "react";
import { ArrowDown, ArrowUp, Image as ImageIcon, Link2, Loader2, Pencil, Plus, Power, Trash2 } from "lucide-react";
import { Ad, AdInput, BannerPlace, adminCreateAd, adminDeleteAd, adminPatchAdBanner, adminUpdateAd } from "@/lib/api";
import { normalizeMediaUrl } from "@/lib/image-utils";
import { bannersForPlace } from "@/lib/website-ad-media";
import { useToast } from "@/components/admin/Toast";
import { Chip, IconBtn, Modal, Tone } from "@/components/admin/kit";
import { Field, SwitchRow, inputCls } from "@/components/admin/form/ui";
import AdMediaSlot from "./AdMediaSlot";
import { adToInput, emptyAdInput } from "./AdEditor";

const PLACES: { key: BannerPlace; label: string; desc: string }[] = [
  { key: "top", label: "Sayt tepasi", desc: "Bosh sahifa va ro'yxat sahifalarining yuqorisi" },
  { key: "movie", label: "Kino sahifasi", desc: "Kino, serial va mavsum sahifalari" },
];

// Gap between banner_order values written on a reorder.
const ORDER_STEP = 10;

function bannerState(ad: Ad): { label: string; tone: Tone; live: boolean } {
  if (ad.ends_at && new Date(ad.ends_at) < new Date()) return { label: "Muddati tugagan", tone: "red", live: false };
  if (ad.status === "active") return { label: "Faol", tone: "green", live: true };
  if (ad.status === "draft") return { label: "Qoralama", tone: "gray", live: false };
  return { label: "O'chirilgan", tone: "yellow", live: false };
}

/**
 * Admin block for the banner carousels: one list per place, where banners are
 * added, removed, switched on/off and reordered. A banner is an ordinary ad
 * with a banner creative, so its link, schedule and per-banner stats come
 * from the ad itself.
 */
export default function BannerManager({
  ads,
  token,
  onChanged,
}: {
  ads: Ad[];
  token: string | null;
  onChanged: () => Promise<void>;
}) {
  const toast = useToast();
  const [busyId, setBusyId] = useState<string | null>(null);
  const [editor, setEditor] = useState<{ ad: Ad | null; place: BannerPlace } | null>(null);

  const websiteAds = useMemo(() => ads.filter((a) => a.placements.includes("website")), [ads]);
  const lists = useMemo(
    () => PLACES.map((place) => ({ place, banners: bannersForPlace(websiteAds, place.key) })),
    [websiteAds],
  );
  const nextOrder = useMemo(
    () => Math.max(0, ...websiteAds.map((a) => a.banner_order || 0)) + ORDER_STEP,
    [websiteAds],
  );

  const run = async (id: string, action: () => Promise<void>, done?: string) => {
    if (!token) return;
    setBusyId(id);
    try {
      await action();
      if (done) toast.success(done);
      await onChanged();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Xatolik");
    } finally {
      setBusyId(null);
    }
  };

  const move = (banners: Ad[], from: number, to: number) => {
    if (!token || to < 0 || to >= banners.length) return;
    const next = [...banners];
    [next[from], next[to]] = [next[to], next[from]];
    void run(banners[from].id, async () => {
      // Renumber the whole list so the order is unambiguous from now on.
      await Promise.all(
        next.map((ad, i) => {
          const order = (i + 1) * ORDER_STEP;
          return (ad.banner_order || 0) === order ? null : adminPatchAdBanner(token, ad.id, { banner_order: order });
        }),
      );
    });
  };

  const toggle = (ad: Ad) => {
    if (!token) return;
    const state = bannerState(ad);
    if (ad.ends_at && new Date(ad.ends_at) < new Date()) {
      toast.error("Muddati tugagan — tahrirlab, muddatni yangilang");
      return;
    }
    void run(
      ad.id,
      () => adminPatchAdBanner(token, ad.id, { status: state.live ? "paused" : "active" }),
      state.live ? "Banner o'chirildi" : "Banner yoqildi",
    );
  };

  const remove = (ad: Ad) => {
    if (!token || !confirm(`"${ad.title}" banneri butunlay o'chirilsinmi?`)) return;
    void run(ad.id, () => adminDeleteAd(token, ad.id), "Banner o'chirildi");
  };

  return (
    <section className="mb-6 rounded-2xl border border-white/10 bg-[#12121a] p-4 sm:p-5">
      <div className="mb-4 flex items-start gap-2.5">
        <span className="mt-0.5 flex h-7 w-7 items-center justify-center rounded-lg bg-orange-500/10 text-orange-400">
          <ImageIcon size={15} />
        </span>
        <div>
          <p className="text-sm font-semibold text-white">Banner karusel</p>
          <p className="text-xs text-gray-500">
            Har bir joyga istalgancha banner. Faol bannerlar shu tartibda har 7 soniyada almashadi; har birining havolasi va statistikasi alohida.
          </p>
        </div>
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        {lists.map(({ place, banners }) => (
          <div key={place.key} className="rounded-xl border border-white/10 bg-black/20 p-3">
            <div className="mb-3 flex items-center justify-between gap-2">
              <div className="min-w-0">
                <p className="text-sm font-medium text-white">
                  {place.label} <span className="text-gray-500">· {banners.filter((b) => bannerState(b).live).length} ta faol</span>
                </p>
                <p className="truncate text-[11px] text-gray-500">{place.desc}</p>
              </div>
              <button
                type="button"
                onClick={() => setEditor({ ad: null, place: place.key })}
                className="inline-flex shrink-0 items-center gap-1 rounded-lg bg-orange-500 px-2.5 py-1.5 text-xs font-semibold text-white hover:bg-orange-400"
              >
                <Plus size={14} /> Banner
              </button>
            </div>

            {banners.length === 0 ? (
              <p className="py-6 text-center text-xs text-gray-500">Bu joyda hali banner yo&apos;q</p>
            ) : (
              <ul className="space-y-2">
                {banners.map((ad, i) => {
                  const state = bannerState(ad);
                  const stat = ad.slot_stats?.banner;
                  const views = stat?.impressions || 0;
                  const clicks = stat?.clicks || 0;
                  const busy = busyId === ad.id;
                  return (
                    <li key={ad.id} className={`rounded-xl border border-white/10 bg-[#12121a] p-2.5 ${state.live ? "" : "opacity-70"}`}>
                      <div className="flex items-center gap-2.5">
                        <span className="w-4 shrink-0 text-center text-xs tabular-nums text-gray-500">{i + 1}</span>
                        <div className="aspect-[4/1] w-28 shrink-0 overflow-hidden rounded-lg bg-white/5">
                          {/* eslint-disable-next-line @next/next/no-img-element */}
                          <img src={normalizeMediaUrl(ad.banner_media_url || ad.image_url)} alt="" loading="lazy" className="h-full w-full object-cover" />
                        </div>
                        <div className="min-w-0 flex-1">
                          <p className="truncate text-sm font-medium text-white">{ad.title}</p>
                          <p className="flex items-center gap-1 truncate text-[11px] text-gray-500">
                            <Link2 size={10} className="shrink-0" />
                            <span className="truncate">{ad.target_url}</span>
                          </p>
                        </div>
                        <Chip tone={state.tone} dot>
                          {state.label}
                        </Chip>
                      </div>
                      <div className="mt-2 flex items-center justify-between gap-2">
                        <p className="text-[11px] text-gray-400">
                          <span className="tabular-nums text-white">{views.toLocaleString()}</span> ko&apos;rish ·{" "}
                          <span className="tabular-nums text-white">{clicks.toLocaleString()}</span> bosish · CTR{" "}
                          <span className="tabular-nums text-orange-300">{views > 0 ? ((clicks / views) * 100).toFixed(1) : "0"}%</span>
                        </p>
                        <div className="flex items-center">
                          {busy ? (
                            <Loader2 size={15} className="m-2 animate-spin text-gray-500" />
                          ) : (
                            <>
                              <IconBtn label="Yuqoriga" disabled={i === 0} onClick={() => move(banners, i, i - 1)}>
                                <ArrowUp size={14} />
                              </IconBtn>
                              <IconBtn label="Pastga" disabled={i === banners.length - 1} onClick={() => move(banners, i, i + 1)}>
                                <ArrowDown size={14} />
                              </IconBtn>
                              <IconBtn label={state.live ? "O'chirib qo'yish" : "Yoqish"} tone={state.live ? "yellow" : "green"} onClick={() => toggle(ad)}>
                                <Power size={14} />
                              </IconBtn>
                              <IconBtn label="Tahrirlash" onClick={() => setEditor({ ad, place: place.key })}>
                                <Pencil size={14} />
                              </IconBtn>
                              <IconBtn label="Butunlay o'chirish" tone="red" onClick={() => remove(ad)}>
                                <Trash2 size={14} />
                              </IconBtn>
                            </>
                          )}
                        </div>
                      </div>
                    </li>
                  );
                })}
              </ul>
            )}
          </div>
        ))}
      </div>

      {editor && (
        <BannerEditor
          key={editor.ad?.id ?? `new-${editor.place}`}
          ad={editor.ad}
          place={editor.place}
          nextOrder={nextOrder}
          token={token}
          onClose={() => setEditor(null)}
          onSaved={async (msg) => {
            setEditor(null);
            toast.success(msg);
            await onChanged();
          }}
        />
      )}
    </section>
  );
}

/** Compact create/edit form for one carousel banner. */
function BannerEditor({
  ad,
  place,
  nextOrder,
  token,
  onClose,
  onSaved,
}: {
  ad: Ad | null;
  place: BannerPlace;
  nextOrder: number;
  token: string | null;
  onClose: () => void;
  onSaved: (message: string) => Promise<void>;
}) {
  const [title, setTitle] = useState(ad?.title ?? "");
  const [link, setLink] = useState(ad?.target_url ?? "");
  const [image, setImage] = useState(ad?.banner_media_url ?? "");
  const [mobileImage, setMobileImage] = useState(ad?.banner_mobile_media_url ?? "");
  // An ad without places is in every carousel; make that explicit when editing.
  const [places, setPlaces] = useState<BannerPlace[]>(() =>
    ad ? (ad.banner_places?.length ? ad.banner_places : PLACES.map((p) => p.key)) : [place],
  );
  const [active, setActive] = useState(ad ? ad.status === "active" : true);
  // New banner: how long it runs. Existing banner: empty keeps the schedule.
  const [days, setDays] = useState(ad ? "" : "30");
  const [uploading, setUploading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  const save = async () => {
    if (!token) return;
    const cleanTitle = title.trim();
    const cleanLink = link.trim();
    if (!cleanTitle || !cleanLink) return setError("Nomi va havolani to'ldiring");
    if (!/^https?:\/\//.test(cleanLink) && !cleanLink.startsWith("/")) return setError("Havola https:// yoki / bilan boshlanishi kerak");
    if (!image) return setError("Banner rasmini yuklang");
    if (places.length === 0) return setError("Kamida bitta joyni tanlang");
    const duration = Math.min(365, Math.max(0, Math.floor(Number(days) || 0)));
    if (!ad && duration < 1) return setError("Necha kun ko'rsatilishini kiriting");

    const banner: Partial<AdInput> = {
      title: cleanTitle,
      target_url: cleanLink,
      banner_media_url: image,
      banner_media_type: "image",
      banner_mobile_media_url: mobileImage,
      banner_places: places,
    };
    setSaving(true);
    setError("");
    try {
      if (ad) {
        const status = active ? "active" : ad.status === "active" ? "paused" : ad.status;
        // duration_days 0 leaves starts_at/ends_at untouched on the backend.
        await adminUpdateAd(token, ad.id, { ...adToInput(ad), ...banner, status, duration_days: duration });
      } else {
        await adminCreateAd(token, {
          ...emptyAdInput(),
          ...banner,
          title: cleanTitle,
          target_url: cleanLink,
          placements: ["website"],
          status: active ? "active" : "paused",
          duration_days: duration,
          banner_order: nextOrder,
        });
      }
      await onSaved(ad ? "Banner saqlandi" : "Banner qo'shildi");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Saqlab bo'lmadi");
      setSaving(false);
    }
  };

  return (
    <Modal
      open
      onClose={onClose}
      busy={saving}
      size="lg"
      icon={ImageIcon}
      title={ad ? "Bannerni tahrirlash" : "Yangi banner"}
      subtitle="Karuseldagi bitta slayd"
      footer={
        <>
          <p className="mr-auto min-w-0 truncate text-xs text-red-400">{error}</p>
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
            {ad ? "Saqlash" : "Qo'shish"}
          </button>
        </>
      }
    >
      <div className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Nomi" required hint="Faqat admin panelda ko'rinadi">
            <input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Masalan: Keel — kuzgi aksiya" className={inputCls} />
          </Field>
          <Field label="Havola" required hint="Shu banner bosilganda ochiladi">
            <input value={link} onChange={(e) => setLink(e.target.value)} placeholder="https://..." className={inputCls} />
          </Field>
        </div>

        <AdMediaSlot
          slot="banner_media_url"
          label="Banner rasmi"
          size="1200×300 · 4:1"
          aspect="aspect-[4/1]"
          value={image}
          mediaType="image"
          token={token}
          onChange={(_, url) => setImage(url)}
          onBusy={(_, busy) => setUploading(busy)}
        />
        <div className="max-w-xs">
          <AdMediaSlot
            slot="banner_mobile_media_url"
            label="Telefon uchun rasm"
            size="800×400 · 2:1 · ixtiyoriy"
            aspect="aspect-[2/1]"
            value={mobileImage}
            mediaType="image"
            token={token}
            onChange={(_, url) => setMobileImage(url)}
            onBusy={(_, busy) => setUploading(busy)}
          />
        </div>

        <Field label="Qaysi joyda chiqadi" required>
          <div className="grid gap-2 sm:grid-cols-2">
            {PLACES.map((p) => {
              const on = places.includes(p.key);
              return (
                <button
                  key={p.key}
                  type="button"
                  role="checkbox"
                  aria-checked={on}
                  onClick={() => setPlaces((cur) => (on ? cur.filter((x) => x !== p.key) : [...cur, p.key]))}
                  className={`rounded-xl border p-3 text-left transition ${on ? "border-orange-500/50 bg-orange-500/[0.07]" : "border-white/10 bg-black/20 hover:border-white/20"}`}
                >
                  <span className="block text-sm font-medium text-white">{p.label}</span>
                  <span className="block text-[11px] text-gray-500">{p.desc}</span>
                </button>
              );
            })}
          </div>
        </Field>

        <div className="grid gap-4 sm:grid-cols-2">
          <Field label={ad ? "Muddatni yangilash (kun)" : "Necha kun"} hint={ad ? "Bo'sh qolsa, hozirgi muddat o'zgarmaydi" : "Bugundan boshlab"}>
            <input type="number" min={ad ? 0 : 1} max={365} value={days} onChange={(e) => setDays(e.target.value)} placeholder={ad ? "—" : "30"} className={inputCls} />
          </Field>
          <SwitchRow
            checked={active}
            onChange={setActive}
            title="Faol"
            description="O'chirilgan banner karuselda ko'rinmaydi"
            icon={<Power size={18} className={active ? "text-emerald-400" : "text-gray-500"} />}
          />
        </div>
      </div>
    </Modal>
  );
}
