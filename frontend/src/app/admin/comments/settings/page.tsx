"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { Settings, Save, RefreshCw, AlertTriangle, Link, Shield, MessageSquare, Reply, Clock, Gauge, ArrowUpDown } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { getCommentSettings, updateCommentSettings, CommentModerationSettings } from "@/lib/comments-api";

export default function CommentSettingsPage() {
  const { token, isLoading: authLoading, user } = useAuth();
  const router = useRouter();

  const [settings, setSettings] = useState<CommentModerationSettings | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [bannedWordsInput, setBannedWordsInput] = useState("");
  const [successMessage, setSuccessMessage] = useState("");
  const [errorMessage, setErrorMessage] = useState("");

  // Redirect if not admin
  useEffect(() => {
    if (!authLoading && (!token || (user?.role !== "admin" && user?.role !== "superadmin"))) {
      router.push("/");
    }
  }, [authLoading, token, user, router]);

  // Fetch settings
  useEffect(() => {
    if (!token) return;

    setLoading(true);
    getCommentSettings(token)
      .then((data) => {
        setSettings(data);
        setBannedWordsInput((data.banned_words || []).join(", "));
      })
      .catch(console.error)
      .finally(() => setLoading(false));
  }, [token]);

  const handleSave = async () => {
    if (!token || !settings) return;

    setSaving(true);
    setErrorMessage("");
    setSuccessMessage("");

    try {
      // Parse banned words
      const bannedWords = bannedWordsInput
        .split(",")
        .map((w) => w.trim())
        .filter((w) => w.length > 0);

      const updated = await updateCommentSettings(token, {
        comments_enabled: settings.comments_enabled,
        replies_enabled: settings.replies_enabled,
        block_links: settings.block_links,
        max_comment_length: settings.max_comment_length,
        max_reply_depth: settings.max_reply_depth,
        require_moderation: settings.require_moderation,
        banned_words: bannedWords,
        auto_hide_banned_content: settings.auto_hide_banned_content,
        comment_cooldown_seconds: settings.comment_cooldown_seconds,
        max_comments_per_minute: settings.max_comments_per_minute,
        default_sort: settings.default_sort,
      });

      setSettings(updated);
      setBannedWordsInput((updated.banned_words || []).join(", "));
      setSuccessMessage("Sozlamalar muvaffaqiyatli saqlandi!");
    } catch (error) {
      console.error("Failed to save settings:", error);
      setErrorMessage("Sozlamalarni saqlashda xatolik yuz berdi");
    } finally {
      setSaving(false);
    }
  };

  if (authLoading) {
    return (
      <div className="min-h-screen bg-brand-dark flex items-center justify-center">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-brand-red"></div>
      </div>
    );
  }

  if (!token || (user?.role !== "admin" && user?.role !== "superadmin")) {
    return null;
  }

  return (
    <div className="p-4 sm:p-8">
      <div className="mb-6 sm:mb-8">
        <h1 className="text-xl sm:text-2xl font-bold text-white">Izohlar Sozlamalari</h1>
        <p className="text-gray-500 text-sm mt-1">
          Izohlarni moderatsiya qilish va boshqarish sozlamalari
        </p>
      </div>

      {loading ? (
        <div className="flex items-center justify-center py-12">
          <RefreshCw className="animate-spin text-gray-500" size={32} />
        </div>
      ) : settings ? (
        <div className="max-w-3xl">
          {/* Messages */}
          {successMessage && (
            <div className="mb-6 p-4 bg-green-500/20 border border-green-500/50 rounded-lg text-green-400">
              {successMessage}
            </div>
          )}
          {errorMessage && (
            <div className="mb-6 p-4 bg-red-500/20 border border-red-500/50 rounded-lg text-red-400">
              {errorMessage}
            </div>
          )}

          {/* Global Enable/Disable */}
          <div className="bg-brand-card border border-brand-border rounded-xl p-6 mb-6 space-y-6">
            <h2 className="text-lg font-semibold text-white flex items-center gap-2">
              <MessageSquare size={20} />
              Umumiy sozlamalar
            </h2>

            {/* Comments Enabled Toggle */}
            <div className="flex items-center justify-between gap-4">
              <div>
                <h3 className="text-white font-medium">Izohlar yoqilgan</h3>
                <p className="text-gray-500 text-sm">Foydalanuvchilar izoh qoldirishi mumkin</p>
              </div>
              <button
                onClick={() => setSettings({ ...settings, comments_enabled: !settings.comments_enabled })}
                className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
                  settings.comments_enabled ? "bg-brand-red" : "bg-gray-600"
                }`}
              >
                <span
                  className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                    settings.comments_enabled ? "translate-x-6" : "translate-x-1"
                  }`}
                />
              </button>
            </div>

            {/* Replies Enabled Toggle */}
            <div className="flex items-center justify-between gap-4">
              <div>
                <h3 className="text-white font-medium">Javoblar yoqilgan</h3>
                <p className="text-gray-500 text-sm">Foydalanuvchilar javob berishi mumkin</p>
              </div>
              <button
                onClick={() => setSettings({ ...settings, replies_enabled: !settings.replies_enabled })}
                className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
                  settings.replies_enabled ? "bg-brand-red" : "bg-gray-600"
                }`}
              >
                <span
                  className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                    settings.replies_enabled ? "translate-x-6" : "translate-x-1"
                  }`}
                />
              </button>
            </div>
          </div>

          {/* Moderation Settings */}
          <div className="bg-brand-card border border-brand-border rounded-xl p-6 mb-6 space-y-6">
            <h2 className="text-lg font-semibold text-white flex items-center gap-2">
              <Shield size={20} />
              Moderatsiya sozlamalari
            </h2>

            {/* Require Moderation Toggle */}
            <div className="flex items-center justify-between gap-4">
              <div>
                <h3 className="text-white font-medium">Moderatsiya talab qilinadi</h3>
                <p className="text-gray-500 text-sm">Barcha izohlar avtomatik ravishda tasdiq talab qiladi</p>
              </div>
              <button
                onClick={() => setSettings({ ...settings, require_moderation: !settings.require_moderation })}
                className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
                  settings.require_moderation ? "bg-brand-red" : "bg-gray-600"
                }`}
              >
                <span
                  className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                    settings.require_moderation ? "translate-x-6" : "translate-x-1"
                  }`}
                />
              </button>
            </div>

            {/* Block Links Toggle */}
            <div className="flex items-center justify-between gap-4">
              <div>
                <h3 className="text-white font-medium">Havolalarni bloklash</h3>
                <p className="text-gray-500 text-sm">Izohlardagi havolalarni (URL) avtomatik bloklash</p>
              </div>
              <button
                onClick={() => setSettings({ ...settings, block_links: !settings.block_links })}
                className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
                  settings.block_links ? "bg-brand-red" : "bg-gray-600"
                }`}
              >
                <span
                  className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                    settings.block_links ? "translate-x-6" : "translate-x-1"
                  }`}
                />
              </button>
            </div>

            {/* Auto-hide Banned Content Toggle */}
            <div className="flex items-center justify-between gap-4">
              <div>
                <h3 className="text-white font-medium">Ta'qiqlangan so'zlarni avtomatik yashirish</h3>
                <p className="text-gray-500 text-sm">Ta'qiqlangan so'zlar bo'lsa, izoh yashiriladi</p>
              </div>
              <button
                onClick={() => setSettings({ ...settings, auto_hide_banned_content: !settings.auto_hide_banned_content })}
                className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
                  settings.auto_hide_banned_content ? "bg-brand-red" : "bg-gray-600"
                }`}
              >
                <span
                  className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                    settings.auto_hide_banned_content ? "translate-x-6" : "translate-x-1"
                  }`}
                />
              </button>
            </div>

            {/* Banned Words */}
            <div className="flex items-start gap-3">
              <div className="p-2 bg-yellow-500/20 rounded-lg">
                <Shield className="text-yellow-400" size={20} />
              </div>
              <div className="flex-1">
                <h3 className="text-white font-medium">Ta'qiqlangan so'zlar</h3>
                <p className="text-gray-500 text-sm mt-1 mb-3">
                  Vergul bilan ajratib, ta'qiqlangan so'zlarni kiriting. Izohda bu so'zlar bo'lsa, u avtomatik ravishda "kutilmoqda" holatiga o'tadi.
                </p>
                <textarea
                  value={bannedWordsInput}
                  onChange={(e) => setBannedWordsInput(e.target.value)}
                  placeholder="so'z1, so'z2, so'z3..."
                  rows={4}
                  className="w-full bg-brand-dark border border-brand-border rounded-lg px-4 py-3 text-white placeholder-gray-500 focus:outline-none focus:border-brand-red resize-none"
                />
                <p className="text-gray-500 text-xs mt-2">
                  {bannedWordsInput.split(",").filter((w) => w.trim().length > 0).length} ta so'z kiritilgan
                </p>
              </div>
            </div>
          </div>

          {/* Limits & Rate Control */}
          <div className="bg-brand-card border border-brand-border rounded-xl p-6 mb-6 space-y-6">
            <h2 className="text-lg font-semibold text-white flex items-center gap-2">
              <Gauge size={20} />
              Cheklovlar va tezlik nazorati
            </h2>

            {/* Max Comment Length */}
            <div className="flex items-center justify-between gap-4">
              <div>
                <h3 className="text-white font-medium">Maksimal izoh uzunligi</h3>
                <p className="text-gray-500 text-sm">Belgi soni</p>
              </div>
              <input
                type="number"
                value={settings.max_comment_length}
                onChange={(e) => setSettings({ ...settings, max_comment_length: parseInt(e.target.value) || 2000 })}
                className="w-24 bg-brand-dark border border-brand-border rounded-lg px-3 py-2 text-white text-center"
                min={10}
                max={10000}
              />
            </div>

            {/* Max Reply Depth */}
            <div className="flex items-center justify-between gap-4">
              <div>
                <h3 className="text-white font-medium">Maksimal javob chuqurligi</h3>
                <p className="text-gray-500 text-sm">Qancha daraja chuqur javob berish mumkin</p>
              </div>
              <input
                type="number"
                value={settings.max_reply_depth}
                onChange={(e) => setSettings({ ...settings, max_reply_depth: parseInt(e.target.value) || 3 })}
                className="w-24 bg-brand-dark border border-brand-border rounded-lg px-3 py-2 text-white text-center"
                min={1}
                max={10}
              />
            </div>

            {/* Comment Cooldown */}
            <div className="flex items-center justify-between gap-4">
              <div className="flex items-center gap-3">
                <div className="p-2 bg-blue-500/20 rounded-lg">
                  <Clock className="text-blue-400" size={20} />
                </div>
                <div>
                  <h3 className="text-white font-medium">Kutish vaqti (soniyalar)</h3>
                  <p className="text-gray-500 text-sm">Yangi izoh qoldirishdan oldin kutish</p>
                </div>
              </div>
              <input
                type="number"
                value={settings.comment_cooldown_seconds}
                onChange={(e) => setSettings({ ...settings, comment_cooldown_seconds: parseInt(e.target.value) || 30 })}
                className="w-24 bg-brand-dark border border-brand-border rounded-lg px-3 py-2 text-white text-center"
                min={0}
                max={300}
              />
            </div>

            {/* Max Comments Per Minute */}
            <div className="flex items-center justify-between gap-4">
              <div>
                <h3 className="text-white font-medium">Daqiqada maksimal izohlar</h3>
                <p className="text-gray-500 text-sm">Spamga qarshi himoya</p>
              </div>
              <input
                type="number"
                value={settings.max_comments_per_minute}
                onChange={(e) => setSettings({ ...settings, max_comments_per_minute: parseInt(e.target.value) || 5 })}
                className="w-24 bg-brand-dark border border-brand-border rounded-lg px-3 py-2 text-white text-center"
                min={1}
                max={20}
              />
            </div>
          </div>

          {/* Display Settings */}
          <div className="bg-brand-card border border-brand-border rounded-xl p-6 mb-6 space-y-6">
            <h2 className="text-lg font-semibold text-white flex items-center gap-2">
              <ArrowUpDown size={20} />
              Ko'rinish sozlamalari
            </h2>

            {/* Default Sort */}
            <div className="flex items-center justify-between gap-4">
              <div>
                <h3 className="text-white font-medium">Standart tartiblash</h3>
                <p className="text-gray-500 text-sm">Yangi izohlar qanday tartibda ko'rsatiladi</p>
              </div>
              <select
                value={settings.default_sort}
                onChange={(e) => setSettings({ ...settings, default_sort: e.target.value })}
                className="bg-brand-dark border border-brand-border rounded-lg px-4 py-2 text-white"
              >
                <option value="newest">Eng yangi</option>
                <option value="oldest">Eng eski</option>
              </select>
            </div>
          </div>

          {/* Info Box */}
          <div className="p-4 bg-blue-500/10 border border-blue-500/30 rounded-lg mb-6">
            <div className="flex items-start gap-3">
              <AlertTriangle className="text-blue-400 shrink-0 mt-0.5" size={18} />
              <div className="text-sm text-gray-400">
                <p className="font-medium text-blue-400 mb-1">Moderatsiya haqida</p>
                <p>
                  Ta'qiqlangan so'zlar yoki havolalar bo'lgan izohlar avtomatik ravishda "Kutilmoqda" holatiga o'tadi. 
                  Admin panel orqali ularni ko'rib chiqishingiz va tasdiq yoki rad qilishingiz mumkin.
                </p>
              </div>
            </div>
          </div>

          {/* Save Button */}
          <div className="flex justify-end pt-4 border-t border-brand-border">
            <button
              onClick={handleSave}
              disabled={saving}
              className="flex items-center gap-2 bg-brand-red hover:bg-orange-700 disabled:opacity-50 disabled:cursor-not-allowed text-white font-medium px-6 py-3 rounded-lg transition-colors"
            >
              {saving ? (
                <RefreshCw className="animate-spin" size={18} />
              ) : (
                <Save size={18} />
              )}
              {saving ? "Saqlanmoqda..." : "Saqlash"}
            </button>
          </div>
        </div>
      ) : (
        <div className="text-center py-12">
          <Settings className="text-gray-600 mx-auto mb-3" size={32} />
          <p className="text-gray-500">Sozlamalar topilmadi</p>
        </div>
      )}
    </div>
  );
}
