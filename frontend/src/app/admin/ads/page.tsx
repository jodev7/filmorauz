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
} from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import {
  Ad,
  AdInput,
  AdStats,
  AdStatus,
  adminListAds,
  adminGetAdStats,
  adminCreateAd,
  adminUpdateAd,
  adminDeleteAd,
  uploadAdMedia,
} from "@/lib/api";

const PLACEMENTS = [
  { value: "homepage_banner", label: "Homepage Banner" },
  { value: "homepage_popup", label: "Homepage Popup" },
  { value: "movie_page_banner", label: "Movie Page Banner" },
  { value: "watch_page_banner", label: "Watch Page Banner" },
  { value: "profile_page_banner", label: "Profile Page Banner" },
  { value: "player_overlay_banner", label: "Player Overlay Banner" },
  { value: "player_popup", label: "Player Popup" },
];

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

  const openCreate = () => {
    setEditAd(null);
    setForm(emptyForm());
    setShowModal(true);
  };

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
    });
    setShowModal(true);
  };

  const handleSave = async () => {
    if (!token) return;
    setSaving(true);
    try {
      if (editAd) {
        await adminUpdateAd(token, editAd.id, form);
      } else {
        await adminCreateAd(token, form);
      }
      setShowModal(false);
      await load();
    } catch (err) {
      alert(err instanceof Error ? err.message : "Failed to save ad");
    } finally {
      setSaving(false);
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
        <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-4">
          <StatCard icon={<Megaphone size={18} />} label="Total Ads" value={stats.total_ads} />
          <StatCard icon={<Play size={18} className="text-green-400" />} label="Active" value={stats.active_ads} />
          <StatCard icon={<TrendingUp size={18} className="text-red-400" />} label="Expired" value={stats.expired_ads} />
          <StatCard icon={<Eye size={18} className="text-blue-400" />} label="Impressions" value={stats.impressions.toLocaleString()} />
          <StatCard icon={<MousePointerClick size={18} className="text-yellow-400" />} label="Clicks" value={stats.clicks.toLocaleString()} />
          <StatCard icon={<DollarSign size={18} className="text-green-400" />} label="Revenue" value={`$${stats.revenue.toFixed(2)}`} />
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
                <th className="px-4 py-3 font-medium text-right">Muddat</th>
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
                          {p.replace(/_/g, " ")}
                        </span>
                      ))}
                    </div>
                  </td>
                  <td className="px-4 py-3">
                    <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${STATUS_COLORS[ad.status]}`}>
                      {STATUS_LABELS[ad.status]}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-right text-gray-300">
                    {ad.duration_days ? `${ad.duration_days} kun` : "—"}
                  </td>
                  <td className="px-4 py-3 text-right text-gray-300">{ad.impressions.toLocaleString()}</td>
                  <td className="px-4 py-3 text-right text-gray-300">{ad.clicks.toLocaleString()}</td>
                  <td className="px-4 py-3 text-right text-gray-300">${ad.price.toFixed(2)}</td>
                  <td className="px-4 py-3">
                    <div className="flex items-center justify-end gap-2">
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
              <div className="grid grid-cols-2 gap-4">
                <Field label="Rasm (Image)">
                  <div className="space-y-1">
                    <label className="flex items-center gap-2 cursor-pointer w-full bg-brand-dark border border-brand-border rounded-lg px-3 py-2 text-sm hover:border-brand-red transition">
                      {uploadingImage ? (
                        <Loader2 size={14} className="animate-spin text-brand-red shrink-0" />
                      ) : (
                        <Upload size={14} className="text-gray-400 shrink-0" />
                      )}
                      <span className="text-gray-400 truncate">
                        {form.image_url ? "Rasm yuklandi ✓" : "Rasm tanlang..."}
                      </span>
                      <input
                        type="file"
                        accept="image/jpeg,image/png,image/webp,image/gif"
                        className="hidden"
                        disabled={uploadingImage}
                        onChange={async (e) => {
                          const file = e.target.files?.[0];
                          if (!file || !token) return;
                          setUploadingImage(true);
                          try {
                            const url = await uploadAdMedia(token, file, "image");
                            setForm((f) => ({ ...f, image_url: url }));
                          } catch (err) {
                            alert(err instanceof Error ? err.message : "Upload failed");
                          } finally {
                            setUploadingImage(false);
                          }
                        }}
                      />
                    </label>
                    {form.image_url && (
                      // eslint-disable-next-line @next/next/no-img-element
                      <img src={form.image_url} alt="preview" className="w-full h-20 object-cover rounded border border-brand-border" />
                    )}
                  </div>
                </Field>
                <Field label="Video">
                  <div className="space-y-1">
                    <label className="flex items-center gap-2 cursor-pointer w-full bg-brand-dark border border-brand-border rounded-lg px-3 py-2 text-sm hover:border-brand-red transition">
                      {uploadingVideo ? (
                        <Loader2 size={14} className="animate-spin text-brand-red shrink-0" />
                      ) : (
                        <Upload size={14} className="text-gray-400 shrink-0" />
                      )}
                      <span className="text-gray-400 truncate">
                        {form.video_url ? "Video yuklandi ✓" : "Video tanlang..."}
                      </span>
                      <input
                        type="file"
                        accept="video/mp4,video/webm"
                        className="hidden"
                        disabled={uploadingVideo}
                        onChange={async (e) => {
                          const file = e.target.files?.[0];
                          if (!file || !token) return;
                          setUploadingVideo(true);
                          try {
                            const url = await uploadAdMedia(token, file, "video");
                            setForm((f) => ({ ...f, video_url: url }));
                          } catch (err) {
                            alert(err instanceof Error ? err.message : "Upload failed");
                          } finally {
                            setUploadingVideo(false);
                          }
                        }}
                      />
                    </label>
                    {form.video_url && (
                      <p className="text-xs text-green-400 truncate">{form.video_url.split("/").pop()}</p>
                    )}
                  </div>
                </Field>
              </div>
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
                <div className="grid grid-cols-2 gap-2">
                  {PLACEMENTS.map((p) => (
                    <label key={p.value} className="flex items-center gap-2 cursor-pointer">
                      <input
                        type="checkbox"
                        checked={form.placements.includes(p.value)}
                        onChange={() => togglePlacement(p.value)}
                        className="accent-brand-red"
                      />
                      <span className="text-gray-300 text-sm">{p.label}</span>
                    </label>
                  ))}
                </div>
              </Field>
              <div className="grid grid-cols-3 gap-4">
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
              </div>
              <p className="text-xs text-gray-500">
                Reklama hozirdan boshlab tanlangan kun davomida ishlaydi
              </p>
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
