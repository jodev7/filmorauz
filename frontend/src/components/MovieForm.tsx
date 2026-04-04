"use client";

import { useState } from "react";
import { Loader2, Plus, X, Info } from "lucide-react";
import { MovieInput, VideoSourceType } from "@/lib/api";

const QUALITIES = ["480p", "720p", "1080p", "1080p Ultra", "4K"];
const GENRE_OPTIONS = [
  "Action", "Adventure", "Animation", "Comedy", "Crime",
  "Documentary", "Drama", "Fantasy", "Horror", "Mystery",
  "Romance", "Sci-Fi", "Thriller", "Western",
];

// Video source type options with descriptions
const SOURCE_TYPE_OPTIONS: { value: VideoSourceType; label: string; description: string }[] = [
  { 
    value: "iframe_embed", 
    label: "Iframe Embed", 
    description: "Use for YouTube, Vimeo, or other iframe-based players. Requires embed URL." 
  },
  { 
    value: "direct_mp4", 
    label: "Direct MP4", 
    description: "Direct video file link (.mp4). May return 403 if CDN blocks hotlinking." 
  },
  { 
    value: "direct_hls", 
    label: "Direct HLS (.m3u8)", 
    description: "HLS streaming format for adaptive quality." 
  },
  { 
    value: "external_restricted", 
    label: "External (Restricted)", 
    description: "For sources that block external playback. Shows restricted message." 
  },
];

interface Props {
  initialData?: Partial<MovieInput>;
  onSubmit: (data: MovieInput) => Promise<void>;
  submitLabel?: string;
}

const emptyForm: MovieInput = {
  title: "",
  description: "",
  poster_url: "",
  backdrop_url: "",
  year: new Date().getFullYear(),
  genre: [],
  country: "",
  video_url: "",
  embed_url: "",
  source_type: "iframe_embed",
  duration: 0,
  quality: "1080p",
  is_premium: false,
};

export default function MovieForm({
  initialData,
  onSubmit,
  submitLabel = "Save Movie",
}: Props) {
  // Ensure genre is always an array to prevent null/undefined crashes
  const safeInitialData = initialData ? {
    ...initialData,
    genre: Array.isArray(initialData.genre) ? initialData.genre : [],
  } : emptyForm;

  const [form, setForm] = useState<MovieInput>({ ...emptyForm, ...safeInitialData });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [genreInput, setGenreInput] = useState("");

  const set = (field: keyof MovieInput, value: unknown) => {
    setForm((prev) => ({ ...prev, [field]: value }));
  };

  const addGenre = () => {
    const g = genreInput.trim();
    if (g && !form.genre.includes(g)) {
      set("genre", [...form.genre, g]);
    }
    setGenreInput("");
  };

  const removeGenre = (g: string) => {
    set("genre", form.genre.filter((x) => x !== g));
  };

  const selectGenre = (g: string) => {
    if (form.genre.includes(g)) {
      removeGenre(g);
    } else {
      set("genre", [...form.genre, g]);
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    
    // Validate source type requirements
    if (form.source_type === "iframe_embed" && !form.embed_url) {
      setError("Embed URL is required for iframe embed source type");
      return;
    }
    if (["direct_mp4", "direct_hls", "external_restricted"].includes(form.source_type) && !form.video_url) {
      setError("Video URL is required for this source type");
      return;
    }

    setLoading(true);
    try {
      await onSubmit(form);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Something went wrong");
    } finally {
      setLoading(false);
    }
  };

  // Check if we need video_url field based on source type
  const requiresVideoURL = form.source_type !== "iframe_embed";
  // Check if we need embed_url field based on source type  
  const requiresEmbedURL = form.source_type === "iframe_embed";

  return (
    <form onSubmit={handleSubmit} className="space-y-6 max-w-3xl">
      {/* Error banner */}
      {error && (
        <div className="bg-red-400/10 border border-red-400/30 text-red-400 rounded-lg px-4 py-3 text-sm">
          {error}
        </div>
      )}

      {/* Two-column: Title + Year */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div className="md:col-span-2">
          <label className="block text-sm text-gray-400 mb-1.5">
            Title <span className="text-brand-red">*</span>
          </label>
          <input
            type="text"
            value={form.title}
            onChange={(e) => set("title", e.target.value)}
            placeholder="Movie title"
            required
            className="field"
          />
        </div>
        <div>
          <label className="block text-sm text-gray-400 mb-1.5">
            Year <span className="text-brand-red">*</span>
          </label>
          <input
            type="number"
            value={form.year}
            onChange={(e) => set("year", parseInt(e.target.value))}
            min={1900}
            max={2100}
            required
            className="field"
          />
        </div>
      </div>

      {/* Description */}
      <div>
        <label className="block text-sm text-gray-400 mb-1.5">
          Description <span className="text-brand-red">*</span>
        </label>
        <textarea
          value={form.description}
          onChange={(e) => set("description", e.target.value)}
          placeholder="Movie description..."
          required
          rows={4}
          className="field resize-none"
        />
      </div>

      {/* Poster URL + Backdrop URL */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div>
          <label className="block text-sm text-gray-400 mb-1.5">
            Poster URL <span className="text-brand-red">*</span>
          </label>
          <input
            type="url"
            value={form.poster_url}
            onChange={(e) => set("poster_url", e.target.value)}
            placeholder="https://..."
            required
            className="field"
          />
          {/* Preview */}
          {form.poster_url && (
            // eslint-disable-next-line @next/next/no-img-element
            <img
              src={form.poster_url}
              alt="Poster preview"
              className="mt-2 h-24 rounded object-cover border border-brand-border"
              onError={(e) => {
                (e.target as HTMLImageElement).style.display = "none";
              }}
            />
          )}
        </div>
        <div>
          <label className="block text-sm text-gray-400 mb-1.5">
            Backdrop URL
          </label>
          <input
            type="url"
            value={form.backdrop_url}
            onChange={(e) => set("backdrop_url", e.target.value)}
            placeholder="https://... (optional)"
            className="field"
          />
          {form.backdrop_url && (
            // eslint-disable-next-line @next/next/no-img-element
            <img
              src={form.backdrop_url}
              alt="Backdrop preview"
              className="mt-2 h-24 rounded object-cover border border-brand-border w-full"
              onError={(e) => {
                (e.target as HTMLImageElement).style.display = "none";
              }}
            />
          )}
        </div>
      </div>

      {/* Video Source Type */}
      <div>
        <label className="block text-sm text-gray-400 mb-1.5">
          Video Source Type <span className="text-brand-red">*</span>
        </label>
        <select
          value={form.source_type}
          onChange={(e) => set("source_type", e.target.value as VideoSourceType)}
          required
          className="field"
        >
          {SOURCE_TYPE_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
        <div className="mt-2 flex items-start gap-2 text-xs text-gray-500 bg-gray-900/50 p-2 rounded">
          <Info size={14} className="mt-0.5 flex-shrink-0" />
          <p>
            {SOURCE_TYPE_OPTIONS.find(o => o.value === form.source_type)?.description}
          </p>
        </div>
      </div>

      {/* Embed URL (for iframe_embed) */}
      {requiresEmbedURL && (
        <div>
          <label className="block text-sm text-gray-400 mb-1.5">
            Embed URL <span className="text-brand-red">*</span>
          </label>
          <input
            type="url"
            value={form.embed_url}
            onChange={(e) => set("embed_url", e.target.value)}
            placeholder="https://www.youtube.com/embed/VIDEO_ID or iframe embed URL"
            required={requiresEmbedURL}
            className="field"
          />
          <p className="text-xs text-gray-600 mt-1">
            Use the embed URL (not watch URL). For YouTube: use /embed/ format.
          </p>
        </div>
      )}

      {/* Video URL (for direct types) */}
      {requiresVideoURL && (
        <div>
          <label className="block text-sm text-gray-400 mb-1.5">
            Video URL <span className="text-brand-red">*</span>
          </label>
          <input
            type="url"
            value={form.video_url}
            onChange={(e) => set("video_url", e.target.value)}
            placeholder={form.source_type === "direct_hls" 
              ? "https://example.com/stream.m3u8" 
              : "https://example.com/video.mp4"}
            required={requiresVideoURL}
            className="field"
          />
          {form.source_type !== "external_restricted" && (
            <p className="text-xs text-gray-600 mt-1">
              Note: Some external MP4 links may return HTTP 403 if the CDN blocks hotlinking. 
              Consider using iframe embed for restricted sources.
            </p>
          )}
          {form.source_type === "external_restricted" && (
            <p className="text-xs text-amber-500 mt-1">
              This source type will display a restricted message to users. 
              Use this when the video source does not allow external playback.
            </p>
          )}
        </div>
      )}

      {/* Genre selector */}
      <div>
        <label className="block text-sm text-gray-400 mb-2">Genres</label>

        {/* Quick-select chips */}
        <div className="flex flex-wrap gap-2 mb-3">
          {GENRE_OPTIONS.map((g) => {
            const selected = form.genre.includes(g);
            return (
              <button
                key={g}
                type="button"
                onClick={() => selectGenre(g)}
                className={`text-xs px-3 py-1.5 rounded-full border transition-colors ${
                  selected
                    ? "bg-brand-red border-brand-red text-white"
                    : "border-brand-border text-gray-400 hover:border-gray-500 hover:text-white"
                }`}
              >
                {g}
              </button>
            );
          })}
        </div>

        {/* Custom genre input */}
        <div className="flex gap-2">
          <input
            type="text"
            value={genreInput}
            onChange={(e) => setGenreInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                addGenre();
              }
            }}
            placeholder="Custom genre..."
            className="field flex-1 py-2"
          />
          <button
            type="button"
            onClick={addGenre}
            className="px-3 py-2 bg-brand-border hover:bg-gray-600 text-white rounded-lg transition-colors"
          >
            <Plus size={16} />
          </button>
        </div>

        {/* Selected genres */}
        {form.genre.length > 0 && (
          <div className="flex flex-wrap gap-2 mt-2">
            {form.genre.map((g) => (
              <span
                key={g}
                className="flex items-center gap-1.5 text-xs bg-brand-red/20 text-brand-red border border-brand-red/30 px-2.5 py-1 rounded-full"
              >
                {g}
                <button
                  type="button"
                  onClick={() => removeGenre(g)}
                  className="hover:text-white"
                >
                  <X size={11} />
                </button>
              </span>
            ))}
          </div>
        )}
      </div>

      {/* Country + Duration + Quality */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div>
          <label className="block text-sm text-gray-400 mb-1.5">Country</label>
          <input
            type="text"
            value={form.country}
            onChange={(e) => set("country", e.target.value)}
            placeholder="e.g. USA"
            className="field"
          />
        </div>
        <div>
          <label className="block text-sm text-gray-400 mb-1.5">
            Duration (minutes)
          </label>
          <input
            type="number"
            value={form.duration || ""}
            onChange={(e) => set("duration", parseInt(e.target.value) || 0)}
            placeholder="120"
            min={0}
            className="field"
          />
        </div>
        <div>
          <label className="block text-sm text-gray-400 mb-1.5">Quality</label>
          <select
            value={form.quality}
            onChange={(e) => set("quality", e.target.value)}
            className="field"
          >
            <option value="">Select quality</option>
            {QUALITIES.map((q) => (
              <option key={q} value={q}>
                {q}
              </option>
            ))}
          </select>
        </div>
      </div>

      {/* Premium toggle */}
      <div className="flex items-center gap-3">
        <label className="flex items-center gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={form.is_premium || false}
            onChange={(e) => set("is_premium", e.target.checked)}
            className="w-5 h-5 rounded border-gray-600 bg-gray-700 text-yellow-500 focus:ring-yellow-500 focus:ring-offset-gray-900"
          />
          <span className="text-white font-medium">Premium Content</span>
        </label>
        <span className="text-gray-500 text-sm">Only premium users can watch this movie</span>
      </div>

      {/* Submit */}
      <div className="flex items-center gap-3 pt-2">
        <button
          type="submit"
          disabled={loading}
          className="flex items-center gap-2 bg-brand-red hover:bg-orange-700 disabled:opacity-60 disabled:cursor-not-allowed text-white font-semibold px-8 py-3 rounded-xl transition-colors"
        >
          {loading && <Loader2 size={16} className="animate-spin" />}
          {loading ? "Saving..." : submitLabel}
        </button>
      </div>

      {/* Field styles injected via style tag for simplicity */}
      <style jsx global>{`
        .field {
          width: 100%;
          background: #0a0a0f;
          border: 1px solid #1e1e2e;
          border-radius: 0.5rem;
          padding: 0.625rem 1rem;
          color: white;
          font-size: 0.875rem;
          transition: border-color 0.15s;
        }
        .field:focus {
          outline: none;
          border-color: #e63946;
        }
        .field::placeholder {
          color: #4b5563;
        }
        select.field option {
          background: #12121a;
        }
      `}</style>
    </form>
  );
}
