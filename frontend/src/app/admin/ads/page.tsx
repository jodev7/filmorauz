"use client";

import { useEffect, useState, useCallback } from "react";
import {
  PlusCircle,
  Pencil,
  Trash2,
  Loader2,
  Megaphone,
  Eye,
  MousePointerClick,
  DollarSign,
  TrendingUp,
  Play,
  Pause,
  Upload,
  Send,
  History,
  Bot,
  Tv2,
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
  uploadAdMedia,
} from "@/lib/api";

const SIMPLIFIED_PLACEMENTS = [
  { value: "website", label: "Website" },
  { value: "telegram_channel", label: "Telegram Channel" },
  { value: "telegram_bot", label: "Telegram Bot" },
];

// Validate that a video file has ~15s duration (accepts 10–20s range)
function validatePlayerVideoDuration(file: File): Promise<string | null> {
  return new Promise((resolve) => {
    const url = URL.createObjectURL(file);
    const video = document.createElement("video");
    video.preload = "metadata";
    video.onloadedmetadata = () => {
      URL.revokeObjectURL(url);
      const dur = Math.round(video.duration);
      if (dur < 10 || dur > 20) {
        resolve(`Video player reklama uchun faqat 15 soniyalik video yuklash mumkin (joriy: ${dur}s)`);
      } else {
        resolve(null);
      }
    };
    video.onerror = () => { URL.revokeObjectURL(url); resolve(null); };
    video.src = url;
  });
}

// Detect media category from the File's MIME type.
function fileMediaType(file: File): "image" | "video" | null {
  if (file.type.startsWith("image/")) return "image";
  if (file.type.startsWith("video/")) return "video";
  // Fallback: sniff from extension
  const ext = file.name.split(".").pop()?.toLowerCase();
  if (["jpg", "jpeg", "png", "webp", "gif"].includes(ext ?? "")) return "image";
  if (["mp4", "webm", "mov"].includes(ext ?? "")) return "video";
  return null;
}

function MediaUpload({
  value,
  uploading,
  onUpload,
  onClear,
  // Default: image-only (website slots). Pass acceptVideo or both to widen.
  acceptImage = true,
  acceptVideo = false,
  validateFile,
}: {
  value: string;
  uploading: boolean;
  onUpload: (file: File, type: "image" | "video") => Promise<void>;
  onClear: () => void;
  acceptImage?: boolean;
  acceptVideo?: boolean;
  validateFile?: (file: File) => Promise<string | null>;
}) {
  const [localError, setLocalError] = useState<string | null>(null);

  // Build the <input accept="..."> attribute using canonical MIME types so browsers
  // filter correctly regardless of OS.
  const imageMIMEs = "image/jpeg,image/png,image/webp,image/gif";
  const videoMIMEs = "video/mp4,video/webm,video/quicktime";
  const accept =
    acceptImage && acceptVideo
      ? `${imageMIMEs},${videoMIMEs}`
      : acceptImage
      ? imageMIMEs
      : videoMIMEs;

  // Human-readable hint shown below the button
  const hint =
    acceptImage && acceptVideo
      ? "JPG, PNG, WEBP, GIF · MP4, WEBM, MOV"
      : acceptImage
      ? "JPG, PNG, WEBP, GIF"
      : "MP4, WEBM, MOV";

  // Whether to show image preview vs text for an uploaded value
  const isImageValue = value
    ? /\.(jpe?g|png|webp|gif)(\?|$)/i.test(value)
    : acceptImage && !acceptVideo;

  return (
    <div className="space-y-1">
      <label className="flex items-center gap-2 cursor-pointer w-full bg-brand-dark border border-brand-border rounded-lg px-3 py-2 text-sm hover:border-brand-red transition">
        {uploading ? (
          <Loader2 size={14} className="animate-spin text-brand-red shrink-0" />
        ) : (
          <Upload size={14} className="text-gray-400 shrink-0" />
        )}
        <span className="text-gray-400 truncate">
          {value ? "Yuklandi ✓" : "Tanlang..."}
        </span>
        <input
          type="file"
          accept={accept}
          className="hidden"
          disabled={uploading}
          onChange={async (e) => {
            const file = e.target.files?.[0];
            if (!file) return;
            // Reset input so the same file can be re-selected after an error
            e.target.value = "";
            setLocalError(null);

            const type = fileMediaType(file);
            if (!type) {
              setLocalError(`Qo'llab-quvvatlanmaydigan fayl turi. Ruxsat etilgan: ${hint}`);
              return;
            }
            if (type === "image" && !acceptImage) {
              setLocalError("Bu slot faqat video qabul qiladi.");
              return;
            }
            if (type === "video" && !acceptVideo) {
              setLocalError("Bu slot faqat rasmlarni qabul qiladi.");
              return;
            }
            if (validateFile) {
              const validationError = await validateFile(file);
              if (validationError) {
                setLocalError(validationError);
                return;
              }
            }
            try {
              await onUpload(file, type);
            } catch (err) {
              const msg = err instanceof Error ? err.message : "Yuklash muvaffaqiyatsiz tugadi";
              setLocalError(msg);
            }
          }}
        />
      </label>
      <p className="text-[11px] text-gray-600">{hint}</p>
      {localError && (
        <p className="text-[11px] text-red-400">{localError}</p>
      )}
      {value && !localError && (
        <>
          {isImageValue ? (
            <img src={value} alt="preview" className="w-full h-20 object-cover rounded border border-brand-border" />
          ) : (
            <p className="text-xs text-green-400 truncate">{value.split("/").pop()}</p>
          )}
          <button type="button" onClick={onClear} className="text-xs text-red-400 hover:text-red-300">Tozalash</button>
        </>
      )}
    </div>
  );
}

const PLACEMENT_LABELS: Record<string, string> = {
  website: "Website",
  telegram_channel: "Telegram Channel",
  telegram_bot: "Telegram Bot",
};

function getPlacementLabel(placement: string): string {
  return PLACEMENT_LABELS[placement] || placement.replace(/_/g, " ");
}

const STATUS_COLORS: Record<AdStatus, string> = {
  draft: "bg-gray-700 text-gray-300",
  active: "bg-green-900 text-green-300",
  paused: "bg-yellow-900 text-yellow-300",
  expired: "bg-red-900 text-red-300",
};

const STATUS_LABELS: Record<AdStatus, string> = {
  draft: "Qoralama",
  active: "Faol",
  paused: "To'xtatilgan",
  expired: "Tugagan",
};

function getEffectiveStatus(ad: Ad): AdStatus {
  if (ad.status === "active" && ad.ends_at && new Date(ad.ends_at) < new Date()) {
    return "expired";
  }
  return ad.status;
}

function getRemainingText(endsAt?: string): string {
  if (!endsAt) return "—";
  const now = new Date();
  const end = new Date(endsAt);
  const diffMs = end.getTime() - now.getTime();
  const diffDays = Math.ceil(diffMs / (1000 * 60 * 60 * 24));
  if (diffDays < 0) return "Tugagan";
  if (diffDays === 0) return "Bugun tugaydi";
  if (diffDays === 1) return "1 kun qoldi";
  return `${diffDays} kun qoldi`;
}

const emptyForm = (): AdInput => ({
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

export default function AdminAdsPage() {
  const { token } = useAuth();
  const [ads, setAds] = useState<Ad[]>([]);
  const [stats, setStats] = useState<AdStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [showModal, setShowModal] = useState(false);
  const [editAd, setEditAd] = useState<Ad | null>(null);
  const [form, setForm] = useState<AdInput>(emptyForm());
  const [saving, setSaving] = useState(false);
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [uploadingImage, setUploadingImage] = useState(false);
  const [uploadingVideo, setUploadingVideo] = useState(false);
  const [uploadingBanner, setUploadingBanner] = useState(false);
  const [uploadingInline, setUploadingInline] = useState(false);
  const [uploadingFixedBottom, setUploadingFixedBottom] = useState(false);
  const [uploadingPopup, setUploadingPopup] = useState(false);
  const [uploadingPlayerOverlay, setUploadingPlayerOverlay] = useState(false);
  const [uploadingTelegram, setUploadingTelegram] = useState(false);
  const [sendingTg, setSendingTg] = useState<string | null>(null);
  const [deliveryAd, setDeliveryAd] = useState<Ad | null>(null);
  const [deliveries, setDeliveries] = useState<AdDelivery[]>([]);
  const [loadingDelivery, setLoadingDelivery] = useState(false);

  const load = useCallback(async () => {
    if (!token) return;
    try {
      const [adsData, statsData] = await Promise.all([
        adminListAds(token),
        adminGetAdStats(token),
      ]);
      setAds(adsData);
      setStats(statsData);
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  }, [token]);

  useEffect(() => {
    load();
  }, [load]);

  const openEdit = (ad: Ad) => {
    setEditAd(ad);
    setForm({
      title: ad.title,
      description: ad.description || "",
      image_url: ad.image_url || "",
      video_url: ad.video_url || "",
      target_url: ad.target_url,
      call_to_action: ad.call_to_action || "",
      placements: ad.placements,
      status: ad.status,
      duration_days: ad.duration_days || 30,
      price: ad.price,
      priority: (ad as any).priority || 0,
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
    });
    setShowModal(true);
  };

  const openCreate = () => {
    setEditAd(null);
    setForm(emptyForm());
    setShowModal(true);
  };

  const handleSave = async () => {
    if (!token) return;
    setSaving(true);
    try {
      const payload = { ...form };
      if (editAd) {
        await adminUpdateAd(token, editAd.id, payload);
      } else {
        await adminCreateAd(token, payload);
      }
      setShowModal(false);
      await load();
    } catch (err) {
      alert(err instanceof Error ? err.message : "Failed to save ad");
    } finally {
      setSaving(false);
    }
  };

  const handleSendTelegram = async (ad: Ad) => {
    if (!token) return;
    setSendingTg(ad.id);
    try {
      const res = await adminSendTelegramAd(token, ad.id);
      const ok = res.results.filter((r) => r.status === "success").length;
      const fail = res.results.filter((r) => r.status === "failed").length;
      alert(`Telegram: ${ok} muvaffaqiyatli, ${fail} xato`);
      await load();
    } catch (err) {
      alert(err instanceof Error ? err.message : "Telegram yuborishda xato");
    } finally {
      setSendingTg(null);
    }
  };

  const openDelivery = async (ad: Ad) => {
    setDeliveryAd(ad);
    setLoadingDelivery(true);
    setDeliveries([]);
    try {
      const data = await adminGetAdDelivery(token!, ad.id);
      setDeliveries(data);
    } catch {
      setDeliveries([]);
    } finally {
      setLoadingDelivery(false);
    }
  };

  const handleDelete = async (ad: Ad) => {
    if (!confirm(`Delete ad "${ad.title}"?`)) return;
    setDeletingId(ad.id);
    try {
      await adminDeleteAd(token!, ad.id);
      await load();
    } catch (err) {
      alert(err instanceof Error ? err.message : "Failed to delete ad");
    } finally {
      setDeletingId(null);
    }
  };

  const togglePlacement = (val: string) => {
    setForm((f) => ({
      ...f,
      placements: f.placements.includes(val)
        ? f.placements.filter((p) => p !== val)
        : [...f.placements, val],
    }));
  };

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Megaphone className="text-brand-red" size={24} />
          <h1 className="text-2xl font-display text-white">Ads Management</h1>
        </div>
        <button
          onClick={openCreate}
          className="flex items-center gap-2 px-4 py-2 bg-brand-red text-white rounded-lg hover:bg-red-700 transition text-sm font-medium"
        >
          <PlusCircle size={16} />
          New Ad
        </button>
      </div>

      {/* Stats */}
      {stats && (
        <div className="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-8 gap-4">
          <StatCard icon={<Megaphone size={18} />} label="Jami" value={stats.total_ads} />
          <StatCard icon={<Play size={18} className="text-green-400" />} label="Faol" value={stats.active_ads} />
          <StatCard icon={<TrendingUp size={18} className="text-red-400" />} label="Tugagan" value={stats.expired_ads} />
          <StatCard icon={<Eye size={18} className="text-blue-400" />} label="Ko'rishlar" value={stats.impressions.toLocaleString()} />
          <StatCard icon={<MousePointerClick size={18} className="text-yellow-400" />} label="Bosishlar" value={stats.clicks.toLocaleString()} />
          <StatCard icon={<DollarSign size={18} className="text-green-400" />} label="Daromad" value={`$${stats.revenue.toFixed(2)}`} />
          <StatCard icon={<Send size={18} className="text-blue-400" />} label="TG Yetkazish" value={(stats.telegram_deliveries || 0).toLocaleString()} />
          <StatCard icon={<Send size={18} className="text-red-400" />} label="TG Xato" value={(stats.telegram_failed || 0).toLocaleString()} />
        </div>
      )}

      {/* Table */}
      {loading ? (
        <div className="flex justify-center py-16">
          <Loader2 className="animate-spin text-brand-red" size={32} />
        </div>
      ) : (
        <div className="bg-brand-card rounded-xl border border-brand-border overflow-hidden">
          <table className="w-full text-sm">
            <thead className="border-b border-brand-border">
              <tr className="text-gray-400 text-left">
                <th className="px-4 py-3 font-medium">Title</th>
                <th className="px-4 py-3 font-medium">Placements</th>
                <th className="px-4 py-3 font-medium">Status</th>
                <th className="px-4 py-3 font-medium text-right">Qolgan vaqt</th>
                <th className="px-4 py-3 font-medium text-right">Impressions</th>
                <th className="px-4 py-3 font-medium text-right">Clicks</th>
                <th className="px-4 py-3 font-medium text-right">Price</th>
                <th className="px-4 py-3 font-medium text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {ads.length === 0 && (
                <tr>
                  <td colSpan={8} className="px-4 py-12 text-center text-gray-500">
                    No ads yet. Create your first ad.
                  </td>
                </tr>
              )}
              {ads.map((ad) => (
                <tr key={ad.id} className="border-b border-brand-border/50 hover:bg-brand-border/20 transition">
                  <td className="px-4 py-3">
                    <div className="text-white font-medium">{ad.title}</div>
                    {ad.description && (
                      <div className="text-gray-500 text-xs truncate max-w-[200px]">{ad.description}</div>
                    )}
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex flex-wrap gap-1">
                      {ad.placements.map((p) => (
                        <span key={p} className="px-1.5 py-0.5 bg-brand-border rounded text-xs text-gray-300">
                          {getPlacementLabel(p)}
                        </span>
                      ))}
                    </div>
                  </td>
                  <td className="px-4 py-3">
                    {(() => { const eff = getEffectiveStatus(ad); return (
                      <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${STATUS_COLORS[eff]}`}>
                        {STATUS_LABELS[eff]}
                      </span>
                    ); })()}
                  </td>
                  <td className="px-4 py-3 text-right text-gray-300 text-xs">
                    {getEffectiveStatus(ad) === "expired"
                      ? <span className="text-red-400">Tugagan</span>
                      : <span className={ad.ends_at && new Date(ad.ends_at) < new Date(Date.now() + 3 * 86400000) ? "text-yellow-400" : ""}>{getRemainingText(ad.ends_at)}</span>
                    }
                  </td>
                  <td className="px-4 py-3 text-right text-gray-300">{ad.impressions.toLocaleString()}</td>
                  <td className="px-4 py-3 text-right text-gray-300">{ad.clicks.toLocaleString()}</td>
                  <td className="px-4 py-3 text-right text-gray-300">${ad.price.toFixed(2)}</td>
                  <td className="px-4 py-3">
                    <div className="flex items-center justify-end gap-1">
                      {(ad.telegram_channel_enabled || ad.telegram_bot_enabled) && (
                        <button
                          onClick={() => handleSendTelegram(ad)}
                          disabled={sendingTg === ad.id}
                          title="Telegramga yuborish"
                          className="p-1.5 text-gray-400 hover:text-blue-400 hover:bg-brand-border rounded transition disabled:opacity-50"
                        >
                          {sendingTg === ad.id ? <Loader2 size={14} className="animate-spin" /> : <Send size={14} />}
                        </button>
                      )}
                      <button
                        onClick={() => openDelivery(ad)}
                        title="Yetkazish tarixi"
                        className="p-1.5 text-gray-400 hover:text-yellow-400 hover:bg-brand-border rounded transition"
                      >
                        <History size={14} />
                      </button>
                      <button
                        onClick={() => openEdit(ad)}
                        className="p-1.5 text-gray-400 hover:text-white hover:bg-brand-border rounded transition"
                      >
                        <Pencil size={14} />
                      </button>
                      <button
                        onClick={() => handleDelete(ad)}
                        disabled={deletingId === ad.id}
                        className="p-1.5 text-gray-400 hover:text-red-400 hover:bg-brand-border rounded transition disabled:opacity-50"
                      >
                        {deletingId === ad.id ? <Loader2 size={14} className="animate-spin" /> : <Trash2 size={14} />}
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Modal */}
      {showModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm p-4">
          <div className="bg-brand-card border border-brand-border rounded-xl w-full max-w-2xl max-h-[90vh] overflow-y-auto">
            <div className="px-6 py-4 border-b border-brand-border flex items-center justify-between">
              <h2 className="text-lg font-semibold text-white">
                {editAd ? "Edit Ad" : "New Ad"}
              </h2>
              <button onClick={() => setShowModal(false)} className="text-gray-400 hover:text-white text-xl">✕</button>
            </div>
            <div className="p-6 space-y-4">
              <Field label="Title *">
                <input
                  value={form.title}
                  onChange={(e) => setForm((f) => ({ ...f, title: e.target.value }))}
                  className="w-full bg-brand-dark border border-brand-border rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-brand-red"
                  placeholder="Ad title"
                />
              </Field>
              <Field label="Description">
                <textarea
                  value={form.description}
                  onChange={(e) => setForm((f) => ({ ...f, description: e.target.value }))}
                  className="w-full bg-brand-dark border border-brand-border rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-brand-red resize-none"
                  rows={2}
                  placeholder="Short description"
                />
              </Field>
              {/* Media uploads based on placement */}
              {form.placements.includes("website") && (
                <div className="space-y-4 p-4 bg-brand-dark/50 rounded-lg border border-brand-border">
                  <p className="text-sm font-medium text-white">Website Creatives</p>
                  
                  <Field label="Banner (top of page)">
                    <div className="space-y-1">
                      <p className="text-xs text-gray-500">Recommended: 1200x300 (4:1)</p>
                      <MediaUpload
                        value={form.banner_media_url || ""}
                        uploading={uploadingBanner}
                        acceptImage
                        onUpload={async (file, type) => {
                          if (!token) return;
                          setUploadingBanner(true);
                          try {
                            const url = (await uploadAdMedia(token, file, type)) || "";
                            setForm(f => ({ ...f, banner_media_url: url, banner_media_type: type }));
                          } finally {
                            setUploadingBanner(false);
                          }
                        }}
                        onClear={() => setForm(f => ({ ...f, banner_media_url: "" }))}
                      />
                    </div>
                  </Field>

                  <Field label="Inline (between content)">
                    <div className="space-y-1">
                      <p className="text-xs text-gray-500">Recommended: 1200x400 (3:1)</p>
                      <MediaUpload
                        value={form.inline_media_url || ""}
                        uploading={uploadingInline}
                        acceptImage
                        onUpload={async (file, type) => {
                          if (!token) return;
                          setUploadingInline(true);
                          try {
                            const url = (await uploadAdMedia(token, file, type)) || "";
                            setForm(f => ({ ...f, inline_media_url: url, inline_media_type: type }));
                          } finally {
                            setUploadingInline(false);
                          }
                        }}
                        onClear={() => setForm(f => ({ ...f, inline_media_url: "" }))}
                      />
                    </div>
                  </Field>

                  <Field label="Fixed Bottom">
                    <div className="space-y-1">
                      <p className="text-xs text-gray-500">Recommended: 1200x180</p>
                      <MediaUpload
                        value={form.fixed_bottom_media_url || ""}
                        uploading={uploadingFixedBottom}
                        acceptImage
                        onUpload={async (file, type) => {
                          if (!token) return;
                          setUploadingFixedBottom(true);
                          try {
                            const url = (await uploadAdMedia(token, file, type)) || "";
                            setForm(f => ({ ...f, fixed_bottom_media_url: url, fixed_bottom_media_type: type }));
                          } finally {
                            setUploadingFixedBottom(false);
                          }
                        }}
                        onClear={() => setForm(f => ({ ...f, fixed_bottom_media_url: "" }))}
                      />
                    </div>
                  </Field>

                  <Field label="Popup">
                    <div className="space-y-1">
                      <p className="text-xs text-gray-500">Recommended: 900x600</p>
                      <MediaUpload
                        value={form.popup_media_url || ""}
                        uploading={uploadingPopup}
                        acceptImage
                        onUpload={async (file, type) => {
                          if (!token) return;
                          setUploadingPopup(true);
                          try {
                            const url = (await uploadAdMedia(token, file, type)) || "";
                            setForm(f => ({ ...f, popup_media_url: url, popup_media_type: type }));
                          } finally {
                            setUploadingPopup(false);
                          }
                        }}
                        onClear={() => setForm(f => ({ ...f, popup_media_url: "" }))}
                      />
                    </div>
                  </Field>

                  <Field label="Player Overlay">
                    <div className="space-y-1">
                      <p className="text-xs text-gray-500">Faqat 15 soniyalik video · MP4, WEBM, MOV</p>
                      <MediaUpload
                        value={form.player_overlay_media_url || ""}
                        uploading={uploadingPlayerOverlay}
                        acceptVideo
                        validateFile={validatePlayerVideoDuration}
                        onUpload={async (file, type) => {
                          if (!token) return;
                          setUploadingPlayerOverlay(true);
                          try {
                            const url = (await uploadAdMedia(token, file, type)) || "";
                            setForm(f => ({ ...f, player_overlay_media_url: url, player_overlay_media_type: "video" }));
                          } finally {
                            setUploadingPlayerOverlay(false);
                          }
                        }}
                        onClear={() => setForm(f => ({ ...f, player_overlay_media_url: "", player_overlay_media_type: "image" }))}
                      />
                    </div>
                  </Field>
                </div>
              )}

              {form.placements.includes("telegram_channel") || form.placements.includes("telegram_bot") ? (
                <div className="space-y-4 p-4 bg-brand-dark/50 rounded-lg border border-brand-border">
                  <p className="text-sm font-medium text-white">Telegram Media</p>
                  <p className="text-xs text-gray-500">Recommended: 1:1 or 16:9</p>
                  <MediaUpload
                    value={form.telegram_media_url || ""}
                    uploading={uploadingTelegram}
                    acceptImage
                    acceptVideo
                    onUpload={async (file, type) => {
                      if (!token) return;
                      setUploadingTelegram(true);
                      try {
                        const url = (await uploadAdMedia(token, file, type)) || "";
                        setForm(f => ({ ...f, telegram_media_url: url, telegram_media_type: type }));
                      } finally {
                        setUploadingTelegram(false);
                      }
                    }}
                    onClear={() => setForm(f => ({ ...f, telegram_media_url: "", telegram_media_type: "image" }))}
                  />
                </div>
              ) : null}
              <Field label="Havola (bosilganda ochiladi) *">
                <input
                  value={form.target_url}
                  onChange={(e) => setForm((f) => ({ ...f, target_url: e.target.value }))}
                  className="w-full bg-brand-dark border border-brand-border rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-brand-red"
                  placeholder="https://... (kino sahifasi, Telegram, tashqi sayt)"
                />
              </Field>
              <Field label="Call to Action">
                <input
                  value={form.call_to_action}
                  onChange={(e) => setForm((f) => ({ ...f, call_to_action: e.target.value }))}
                  className="w-full bg-brand-dark border border-brand-border rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-brand-red"
                  placeholder="e.g. Learn More, Buy Now"
                />
              </Field>
              <Field label="Placements *">
                <div className="grid grid-cols-3 gap-3">
                  {SIMPLIFIED_PLACEMENTS.map((p) => (
                    <label key={p.value} className="flex items-center gap-2 cursor-pointer p-3 bg-brand-dark border border-brand-border rounded-lg hover:border-brand-red/50 transition">
                      <input
                        type="checkbox"
                        checked={form.placements.includes(p.value)}
                        onChange={() => togglePlacement(p.value)}
                        className="accent-brand-red"
                      />
                      <span className="text-white text-sm font-medium">{p.label}</span>
                    </label>
                  ))}
                </div>
              </Field>
              <div className="grid grid-cols-4 gap-4">
                <Field label="Holat">
                  <select
                    value={form.status}
                    onChange={(e) => setForm((f) => ({ ...f, status: e.target.value as AdStatus }))}
                    className="w-full bg-brand-dark border border-brand-border rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-brand-red"
                  >
                    <option value="draft">Qoralama</option>
                    <option value="active">Faol</option>
                    <option value="paused">To&apos;xtatilgan</option>
                    <option value="expired">Tugagan</option>
                  </select>
                </Field>
                <Field label="Necha kun turadi">
                  <input
                    type="number"
                    min="1"
                    max="365"
                    value={form.duration_days}
                    onChange={(e) => setForm((f) => ({ ...f, duration_days: parseInt(e.target.value) || 1 }))}
                    className="w-full bg-brand-dark border border-brand-border rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-brand-red"
                  />
                </Field>
                <Field label="Narxi (USD)">
                  <input
                    type="number"
                    min="0"
                    step="0.01"
                    value={form.price}
                    onChange={(e) => setForm((f) => ({ ...f, price: parseFloat(e.target.value) || 0 }))}
                    className="w-full bg-brand-dark border border-brand-border rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-brand-red"
                  />
                </Field>
                <Field label="Priority">
                  <input
                    type="number"
                    min="0"
                    value={form.priority}
                    onChange={(e) => setForm((f) => ({ ...f, priority: parseInt(e.target.value) || 0 }))}
                    className="w-full bg-brand-dark border border-brand-border rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-brand-red"
                  />
                </Field>
              </div>
              <p className="text-xs text-gray-500">
                Reklama hozirdan boshlab tanlangan kun davomida ishlaydi
              </p>

              {/* ── Phase 2: Telegram ── */}
              <div className="border-t border-brand-border pt-4 space-y-3">
                <p className="text-xs font-semibold text-gray-400 uppercase tracking-wider flex items-center gap-1">
                  <Send size={12} /> Telegram
                </p>
                <div className="flex gap-4">
                  <label className="flex items-center gap-2 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={form.telegram_channel_enabled}
                      onChange={(e) => setForm((f) => ({ ...f, telegram_channel_enabled: e.target.checked }))}
                      className="accent-brand-red"
                    />
                    <span className="text-gray-300 text-sm">Kanallarga yuborish</span>
                  </label>
                  <label className="flex items-center gap-2 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={form.telegram_bot_enabled}
                      onChange={(e) => setForm((f) => ({ ...f, telegram_bot_enabled: e.target.checked }))}
                      className="accent-brand-red"
                    />
                    <span className="text-gray-300 text-sm flex items-center gap-1"><Bot size={12} /> Bot orqali</span>
                  </label>
                </div>
              </div>

              {/* ── Phase 2: Player ── */}
              <div className="border-t border-brand-border pt-4">
                <p className="text-xs font-semibold text-gray-400 uppercase tracking-wider flex items-center gap-1 mb-3">
                  <Tv2 size={12} /> Player
                </p>
                <label className="flex items-center gap-2 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={form.player_enabled}
                    onChange={(e) => setForm((f) => ({ ...f, player_enabled: e.target.checked }))}
                    className="accent-brand-red"
                  />
                  <span className="text-gray-300 text-sm">Player ichida ko&apos;rsatish</span>
                </label>
                <p className="text-xs text-gray-600 mt-1">
                  player_overlay_banner va player_popup joylashuvlarida ishlaydi
                </p>
              </div>
            </div>
            <div className="px-6 py-4 border-t border-brand-border flex justify-end gap-3">
              <button
                onClick={() => setShowModal(false)}
                className="px-4 py-2 text-gray-400 hover:text-white text-sm transition"
              >
                Cancel
              </button>
              <button
                onClick={handleSave}
                disabled={saving || !form.title || !form.target_url || form.placements.length === 0}
                className="flex items-center gap-2 px-4 py-2 bg-brand-red text-white rounded-lg hover:bg-red-700 transition text-sm font-medium disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {saving ? <Loader2 size={14} className="animate-spin" /> : null}
                {editAd ? "Save Changes" : "Create Ad"}
              </button>
            </div>
          </div>
        </div>
      )}
      {/* Delivery History Modal */}
      {deliveryAd && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm p-4">
          <div className="bg-brand-card border border-brand-border rounded-xl w-full max-w-2xl max-h-[80vh] flex flex-col">
            <div className="px-6 py-4 border-b border-brand-border flex items-center justify-between">
              <h2 className="text-lg font-semibold text-white flex items-center gap-2">
                <History size={16} /> Yetkazish tarixi — {deliveryAd.title}
              </h2>
              <button onClick={() => setDeliveryAd(null)} className="text-gray-400 hover:text-white text-xl">✕</button>
            </div>
            <div className="overflow-y-auto flex-1 p-4">
              {loadingDelivery ? (
                <div className="flex justify-center py-8"><Loader2 className="animate-spin text-brand-red" size={24} /></div>
              ) : deliveries.length === 0 ? (
                <p className="text-center text-gray-500 py-8">Hali yetkazish yo&apos;q</p>
              ) : (
                <table className="w-full text-sm">
                  <thead className="border-b border-brand-border text-gray-400">
                    <tr>
                      <th className="pb-2 text-left">Joylashuv</th>
                      <th className="pb-2 text-left">Maqsad</th>
                      <th className="pb-2 text-left">Holat</th>
                      <th className="pb-2 text-left">Vaqt</th>
                    </tr>
                  </thead>
                  <tbody>
                    {deliveries.map((d) => (
                      <tr key={d.id} className="border-b border-brand-border/30">
                        <td className="py-2 text-gray-300 text-xs">{d.placement.replace(/_/g, " ")}</td>
                        <td className="py-2 text-gray-300 text-xs">{d.target}</td>
                        <td className="py-2">
                          <span className={`px-1.5 py-0.5 rounded text-xs ${d.status === "success" ? "bg-green-900 text-green-300" : "bg-red-900 text-red-300"}`}>
                            {d.status === "success" ? "✓" : "✗"} {d.status}
                          </span>
                          {d.error && <span className="text-xs text-red-400 ml-1">{d.error}</span>}
                        </td>
                        <td className="py-2 text-gray-500 text-xs">{new Date(d.sent_at).toLocaleString()}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function StatCard({ icon, label, value }: { icon: React.ReactNode; label: string; value: string | number }) {
  return (
    <div className="bg-brand-card border border-brand-border rounded-xl p-4">
      <div className="flex items-center gap-2 text-gray-400 text-xs mb-2">
        {icon}
        {label}
      </div>
      <div className="text-white font-semibold text-xl">{value}</div>
    </div>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="space-y-1">
      <label className="text-xs text-gray-400 font-medium">{label}</label>
      {children}
    </div>
  );
}
