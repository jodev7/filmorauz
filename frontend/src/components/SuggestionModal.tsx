"use client";

import { useState } from "react";
import { X, Loader2, Film, Tv, ExternalLink, Image } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { createSuggestion, SuggestionInput } from "@/lib/api";

interface SuggestionModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess?: () => void;
}

export default function SuggestionModal({ isOpen, onClose, onSuccess }: SuggestionModalProps) {
  const { token, isAuthenticated } = useAuth();
  const [loading, setLoading] = useState(false);
  const [success, setSuccess] = useState(false);
  const [error, setError] = useState("");
  
  const [formData, setFormData] = useState<SuggestionInput>({
    type: "movie",
    title: "",
    message: "",
    source_url: "",
  });

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    
    if (!isAuthenticated || !token) {
      setError("Tizimga kirishingiz kerak");
      return;
    }

    if (!formData.title.trim() || !formData.message.trim()) {
      setError("Iltimos, barcha majburiy maydonlarni to'ldiring");
      return;
    }

    setLoading(true);
    setError("");

    try {
      await createSuggestion(token, formData);
      setSuccess(true);
      setTimeout(() => {
        onSuccess?.();
        onClose();
        setSuccess(false);
        setFormData({
          type: "movie",
          title: "",
          message: "",
          source_url: "",
        });
      }, 2000);
    } catch (err: any) {
      setError(err.message || "Xatolik yuz berdi");
    } finally {
      setLoading(false);
    }
  };

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-[100] flex items-center justify-center">
      <div
        className="absolute inset-0 bg-black/70 backdrop-blur-sm"
        onClick={onClose}
      />
      
      <div className="relative bg-brand-card border border-brand-border rounded-2xl p-6 w-full max-w-lg mx-4 max-h-[90vh] overflow-y-auto">
        <button
          onClick={onClose}
          className="absolute top-4 right-4 text-gray-400 hover:text-white"
        >
          <X size={20} />
        </button>

        <h2 className="text-xl font-bold text-white mb-1">Kino tavsiya qilish</h2>
        <p className="text-sm text-gray-400 mb-6">
          Platformaga qo'shishni xohlagan kinoyingizni tavsiya qiling
        </p>

        {success ? (
          <div className="text-center py-8">
            <div className="w-16 h-16 bg-green-500/20 rounded-full flex items-center justify-center mx-auto mb-4">
              <Film className="w-8 h-8 text-green-400" />
            </div>
            <h3 className="text-xl font-bold text-white mb-2">Tavsiya yuborildi!</h3>
            <p className="text-gray-400">
              Tavsiyangiz muvaffaqiyatli qabul qilindi. Tez orada ko'rib chiqamiz.
            </p>
          </div>
        ) : (
          <form onSubmit={handleSubmit} className="space-y-4">
            {/* Type Selection */}
            <div>
              <label className="block text-sm text-gray-400 mb-2">Tur</label>
              <div className="flex gap-3">
                <button
                  type="button"
                  onClick={() => setFormData({ ...formData, type: "movie" })}
                  className={`flex-1 flex items-center justify-center gap-2 px-4 py-3 rounded-lg border transition-colors ${
                    formData.type === "movie"
                      ? "bg-brand-red border-brand-red text-white"
                      : "bg-brand-dark border-brand-border text-gray-400 hover:text-white"
                  }`}
                >
                  <Film size={18} />
                  Kino
                </button>
                <button
                  type="button"
                  onClick={() => setFormData({ ...formData, type: "series" })}
                  className={`flex-1 flex items-center justify-center gap-2 px-4 py-3 rounded-lg border transition-colors ${
                    formData.type === "series"
                      ? "bg-brand-red border-brand-red text-white"
                      : "bg-brand-dark border-brand-border text-gray-400 hover:text-white"
                  }`}
                >
                  <Tv size={18} />
                  Serial
                </button>
              </div>
            </div>

            {/* Title */}
            <div>
              <label className="block text-sm text-gray-400 mb-2">
                Kino/Serial nomi <span className="text-red-400">*</span>
              </label>
              <input
                type="text"
                value={formData.title}
                onChange={(e) => setFormData({ ...formData, title: e.target.value })}
                className="w-full px-4 py-3 bg-brand-dark border border-brand-border rounded-lg text-white placeholder-gray-500 focus:outline-none focus:border-brand-red"
                placeholder="Kino nomini kiriting"
                required
              />
            </div>

            {/* Message */}
            <div>
              <label className="block text-sm text-gray-400 mb-2">
                Xabar <span className="text-red-400">*</span>
              </label>
              <textarea
                value={formData.message}
                onChange={(e) => setFormData({ ...formData, message: e.target.value })}
                className="w-full px-4 py-3 bg-brand-dark border border-brand-border rounded-lg text-white placeholder-gray-500 focus:outline-none focus:border-brand-red resize-none"
                rows={3}
                placeholder="Nima uchun bu kino/serialni tavsiya qilmoqchisiz?"
                required
              />
            </div>

            {/* Source URL (Optional) */}
            <div>
              <label className="block text-sm text-gray-400 mb-2">
                Manba URL (ixtiyoriy)
              </label>
              <div className="relative">
                <ExternalLink className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" size={18} />
                <input
                  type="url"
                  value={formData.source_url}
                  onChange={(e) => setFormData({ ...formData, source_url: e.target.value })}
                  className="w-full pl-10 pr-4 py-3 bg-brand-dark border border-brand-border rounded-lg text-white placeholder-gray-500 focus:outline-none focus:border-brand-red"
                  placeholder="https://..."
                />
              </div>
              <p className="text-xs text-gray-500 mt-1">
                Kinoni topishingiz mumkin bo'lgan havola (IMDb, Kinopoisk va h.k.)
              </p>
            </div>

            {/* Error Message */}
            {error && (
              <div className="p-3 bg-red-500/20 border border-red-500/50 rounded-lg text-red-400 text-sm">
                {error}
              </div>
            )}

            {/* Submit Button */}
            <button
              type="submit"
              disabled={loading}
              className="w-full py-3 bg-brand-red hover:bg-brand-red/90 disabled:opacity-50 disabled:cursor-not-allowed rounded-lg text-white font-medium flex items-center justify-center gap-2 transition-colors"
            >
              {loading ? (
                <>
                  <Loader2 className="w-5 h-5 animate-spin" />
                  Yuborilmoqda...
                </>
              ) : (
                <>
                  <Film className="w-5 h-5" />
                  Tavsiya yuborish
                </>
              )}
            </button>
          </form>
        )}
      </div>
    </div>
  );
}