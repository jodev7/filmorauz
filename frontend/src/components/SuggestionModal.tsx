"use client";

import { useState, useEffect, useRef } from "react";
import { createPortal } from "react-dom";
import { X, Loader2, Film, Tv, ExternalLink, Upload, Image as ImageIcon } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { createSuggestion, SuggestionFormData } from "@/lib/api";

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
  const [selectedImage, setSelectedImage] = useState<File | null>(null);
  const [imagePreview, setImagePreview] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [mounted, setMounted] = useState(false);

  useEffect(() => {
    setMounted(true);
    return () => setMounted(false);
  }, []);

  useEffect(() => {
    if (isOpen) {
      document.body.style.overflow = "hidden";
    } else {
      document.body.style.overflow = "";
    }
    return () => {
      document.body.style.overflow = "";
    };
  }, [isOpen]);

  useEffect(() => {
    return () => {
      if (imagePreview) {
        URL.revokeObjectURL(imagePreview);
      }
    };
  }, [imagePreview]);

  // Handle Escape key
  useEffect(() => {
    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === "Escape" && isOpen) {
        onClose();
      }
    };
    document.addEventListener("keydown", handleEscape);
    return () => document.removeEventListener("keydown", handleEscape);
  }, [isOpen, onClose]);
  
  const [formData, setFormData] = useState<SuggestionFormData>({
    type: "movie",
    title: "",
    message: "",
    source_url: "",
  });

  const handleImageSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    const validTypes = ["image/jpeg", "image/jpg", "image/png", "image/webp"];
    if (!validTypes.includes(file.type)) {
      setError("Faqat JPG, JPEG, PNG va WebP formatlari qabul qilinadi");
      return;
    }

    if (file.size > 10 * 1024 * 1024) {
      setError("Rasm hajmi 10MB dan oshmasligi kerak");
      return;
    }

    setSelectedImage(file);
    if (imagePreview) {
      URL.revokeObjectURL(imagePreview);
    }
    setImagePreview(URL.createObjectURL(file));
    setError("");
  };

  const handleRemoveImage = () => {
    if (selectedImage && imagePreview) {
      URL.revokeObjectURL(imagePreview);
    }
    setSelectedImage(null);
    setImagePreview(null);
    if (fileInputRef.current) {
      fileInputRef.current.value = "";
    }
  };

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
      await createSuggestion(token, {
        ...formData,
        image: selectedImage || undefined,
      });
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
        handleRemoveImage();
      }, 2000);
    } catch (err: any) {
      setError(err.message || "Xatolik yuz berdi");
    } finally {
      setLoading(false);
    }
  };

  if (!isOpen || !mounted) return null;

  const modalContent = (
    <div className="fixed inset-0 z-[9999] overflow-hidden">
      <div
        className="absolute inset-0 bg-black/80 backdrop-blur-sm"
        onClick={onClose}
      />
      <div className="flex items-center justify-center min-h-full p-4 sm:p-6">
        <div 
          className="relative bg-brand-card border border-brand-border rounded-2xl p-6 w-full max-w-lg max-h-[calc(100vh-2rem)] sm:max-h-[90vh] overflow-y-auto shadow-2xl"
          onClick={(e) => e.stopPropagation()}
        >
          <button
            onClick={onClose}
            className="absolute top-4 right-4 text-gray-400 hover:text-white p-1 z-10"
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

              <div>
                <label className="block text-sm text-gray-400 mb-2">
                  Rasm (ixtiyoriy)
                </label>
                {imagePreview ? (
                  <div className="relative mt-2">
                    <img
                      src={imagePreview}
                      alt="Preview"
                      className="w-full max-h-48 object-contain rounded-lg border border-brand-border bg-brand-dark"
                    />
                    <button
                      type="button"
                      onClick={handleRemoveImage}
                      className="absolute top-2 right-2 p-1.5 bg-black/70 rounded-full text-white hover:bg-black/90"
                    >
                      <X size={16} />
                    </button>
                  </div>
                ) : (
                  <label className="flex items-center justify-center w-full h-24 border-2 border-dashed border-brand-border rounded-lg cursor-pointer hover:border-brand-red/50 hover:bg-brand-dark/50 transition-colors">
                    <input
                      ref={fileInputRef}
                      type="file"
                      accept="image/jpeg,image/jpg,image/png,image/webp"
                      onChange={handleImageSelect}
                      className="hidden"
                    />
                    <div className="text-center">
                      <Upload className="w-6 h-6 text-gray-500 mx-auto mb-1" />
                      <span className="text-sm text-gray-500">
                        Rasm tanlang (JPG, PNG, WebP)
                      </span>
                      <span className="text-xs text-gray-600 block mt-0.5">
                        Max 10MB
                      </span>
                    </div>
                  </label>
                )}
              </div>

              {error && (
                <div className="p-3 bg-red-500/20 border border-red-500/50 rounded-lg text-red-400 text-sm">
                  {error}
                </div>
              )}

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
    </div>
  );

  if (typeof window === "undefined") return null;

  return createPortal(modalContent, document.body);
}