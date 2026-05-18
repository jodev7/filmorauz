// src/lib/api.ts
// Central API client for all backend calls

import { logger } from "./logger";

const API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080/api";
const WORKER_URL = process.env.NEXT_PUBLIC_WORKER_URL || "http://localhost:8083";

interface UploadURLResponse {
  uploadUrl?: string;
  authorizationToken?: string;
  fileKey?: string;
  cdnUrl?: string;
  upload_url?: string;
  auth_token?: string;
  file_key?: string;
  cdn_url?: string;
}

export type VideoSourceType = 
  | "iframe_embed" 
  | "direct_mp4" 
  | "direct_hls" 
  | "external_restricted"
  | "ingestion"
  | "direct_upload";

export interface Movie {
  id: string;
  _id?: string;
  record_id?: string;
  code: string;
  title: string;
  title_uz?: string;
  original_title?: string;
  description: string;
  description_uz?: string;
  poster_url: string;
  poster_thumb_url?: string; // Thumbnail for cards
  poster_medium_url?: string; // Medium size for detail pages
  backdrop_url: string;
  year: number;
  genre: string[];
  genres_uz?: string[];
  country: string;
  countries_uz?: string[];
  video_url: string;
  embed_url: string;
  source_type: VideoSourceType;
  duration: number;
  quality: string;
  slug: string;
  views: number;
  rating_avg: number;
  rating_count: number;
  is_premium?: boolean;
  created_at: string;
  updated_at: string;
  // Approval workflow
  approval_status?: "pending" | "approved" | "rejected";
  is_published?: boolean;
  approved_at?: string | null;
  approved_by?: string;
  type?: "movie" | "episode";
  target_type?: "movie" | "episode" | "series";
  target_id?: string;
  episode_id?: string;
  series_id?: string;
  season_id?: string;
  season_number?: number;
  series_slug?: string;
  series_title?: string;
  episode_number?: number;
  episode_title?: string;
  premium_stream_url?: string;
  master_playlist_url?: string;
  available_qualities?: string[];
  generated_qualities?: string[];
  default_quality?: string;
  source_resolution?: string;
  thumbnails_base_url?: string;
  thumbnail_interval?: number;
  preview_sprite_url?: string;
  preview_vtt_url?: string;
}

export interface MovieInput {
  title: string;
  description: string;
  poster_url: string;
  backdrop_url: string;
  year: number;
  genre: string[];
  country: string;
  video_url: string;
  embed_url: string;
  source_type: VideoSourceType;
  duration: number;
  quality: string;
  is_premium?: boolean;
  slug?: string;
}

export interface ListResponse {
  data: Movie[];
  total: number;
  page: number;
  limit: number;
}

export interface GenreChip {
  label: string;
  slug: string;
}

export interface SeriesPreview {
  id: string;
  slug: string;
  title: string;
  description: string;
  poster_url: string;
  backdrop_url: string;
  year: number;
  genre: string[];
  country: string;
  views: number;
  rating_avg: number;
  rating_count: number;
  is_premium: boolean;
  is_completed: boolean;
  created_at: string;
  updated_at: string;
}

export interface HomepageResponse {
  hero: Movie[];
  genres: GenreChip[];
  new_movies: Movie[];
  trending: Movie[];
  premium_movies: Movie[];
  featured_movies: Movie[];
  featured_collections: Collection[];
  series: SeriesPreview[];
}

function normalizeGenreList(input: unknown): string[] {
  const seen = new Set<string>();
  const normalizeValue = (value: string): string => {
    const trimmed = value.trim().toLowerCase();
    if (!trimmed) return "";
    const hyphenated = trimmed.replace(/[_\s]+/g, "-").replace(/-+/g, "-");
    if (hyphenated === "science-fiction" || hyphenated === "sciencefiction" || hyphenated === "scifi") {
      return "sci-fi";
    }
    return hyphenated;
  };
  const pushUnique = (values: string[]): string[] => {
    const out: string[] = [];
    for (const value of values) {
      const normalized = normalizeValue(value);
      if (!normalized || seen.has(normalized)) continue;
      seen.add(normalized);
      out.push(normalized);
    }
    return out;
  };

  if (Array.isArray(input)) {
    return pushUnique(
      input
        .map((value) => (typeof value === "string" ? value : ""))
        .filter(Boolean)
    );
  }

  if (typeof input === "string") {
    const trimmed = input.trim();
    if (!trimmed) return [];

    if (trimmed.includes(",")) {
      return pushUnique(trimmed
        .split(",")
        .map((value) => value));
    }

    return pushUnique([trimmed]);
  }

  return [];
}

function normalizeMovieResponse(data: any): Movie {
  const primaryGenre = normalizeGenreList(data?.genre);
  const legacyGenres = normalizeGenreList(data?.genres);
  const movieGenre = normalizeGenreList(data?.movie_genre);
  const genre = primaryGenre.length > 0
    ? primaryGenre
    : legacyGenres.length > 0
      ? legacyGenres
      : movieGenre;

  return {
    ...data,
    genre,
  };
}

type BrowserCacheEntry<T> = {
  expiresAt: number;
  promise: Promise<T>;
};

const browserRequestCache = new Map<string, BrowserCacheEntry<any>>();

function dedupeBrowserRequest<T>(key: string, ttlMs: number, fetcher: () => Promise<T>): Promise<T> {
  if (typeof window === "undefined") {
    return fetcher();
  }

  const now = Date.now();
  const cached = browserRequestCache.get(key) as BrowserCacheEntry<T> | undefined;
  if (cached && cached.expiresAt > now) {
    return cached.promise;
  }

  const promise = fetcher().catch((error) => {
    browserRequestCache.delete(key);
    throw error;
  });
  browserRequestCache.set(key, { expiresAt: now + ttlMs, promise });
  return promise;
}

// ── Public API ────────────────────────────────────────────────

export async function getMovies(params?: {
  genre?: string;
  page?: number;
  limit?: number;
}): Promise<ListResponse> {
  const qs = new URLSearchParams();
  if (params?.genre) qs.set("genre", params.genre);
  if (params?.page) qs.set("page", String(params.page));
  if (params?.limit) qs.set("limit", String(params.limit));

  const res = await fetch(`${API_URL}/movies?${qs}`, {
    next: { revalidate: 60 }, // ISR: revalidate every 60s
  });
  if (!res.ok) throw new Error("Failed to fetch movies");
  const json = await res.json();
  return {
    ...json,
    data: (json.data || []).map((item: any) => normalizeMovieResponse(item)),
  };
}

export async function getHomepageData(): Promise<HomepageResponse> {
  try {
    const res = await fetch(`${API_URL}/homepage`, {
      next: { revalidate: 60 },
      signal: AbortSignal.timeout(5000), // 5s timeout
    });
    if (!res.ok) throw new Error("Failed to fetch homepage data");
    const json = await res.json();
    const mapHomepageMovie = (item: any): Movie => ({
      id: item.id,
      code: item.code || "",
      title: item.title || "",
      description: item.description || "",
      poster_url: item.poster_thumb_url || item.poster_url || "",
      poster_thumb_url: item.poster_thumb_url || item.poster_url || "",
      backdrop_url: item.backdrop_url || item.poster_url || "",
      year: item.year || 0,
      genre: item.genre || [],
      country: item.country || "",
      video_url: item.video_url || "",
      embed_url: item.embed_url || "",
      source_type: item.source_type || "iframe_embed",
      duration: item.duration || 0,
      quality: item.quality || "",
      slug: item.slug || "",
      views: item.views || 0,
      rating_avg: item.rating_avg || 0,
      rating_count: item.rating_count || 0,
      is_premium: item.is_premium || false,
      created_at: item.created_at || "",
      updated_at: item.updated_at || "",
    });

    return {
      hero: (json.hero || []).map(mapHomepageMovie),
      genres: json.genres || [],
      new_movies: (json.new_movies || []).map(mapHomepageMovie),
      trending: (json.trending || []).map((item: any) => ({
        id: item.id,
        title: item.title,
        slug: item.slug,
        poster_url: item.poster_url,
        backdrop_url: item.poster_url,
        year: item.year,
        genre: item.genre || [],
        description: "",
        code: "",
        video_url: "",
        embed_url: "",
        source_type: "iframe_embed",
        duration: 0,
        quality: "",
        country: "",
        views: item.views_in_period || 0,
        rating_avg: 0,
        rating_count: 0,
        created_at: "",
        updated_at: "",
      })),
      premium_movies: (json.premium_movies || []).map(mapHomepageMovie),
      featured_movies: (json.featured_movies || []).map(mapHomepageMovie),
      featured_collections: json.featured_collections || [],
      series: (json.series || []).map((item: any) => ({
        id: item.id,
        slug: item.slug || "",
        title: item.title || "",
        description: item.description || "",
        poster_url: item.poster_url || "",
        backdrop_url: item.backdrop_url || item.poster_url || "",
        year: item.year || 0,
        genre: item.genre || [],
        country: item.country || "",
        views: item.views || 0,
        rating_avg: item.rating_avg || 0,
        rating_count: item.rating_count || 0,
        is_premium: item.is_premium || false,
        is_completed: item.is_completed || false,
        created_at: item.created_at || "",
        updated_at: item.updated_at || "",
      })),
    };
  } catch (error) {
    if (process.env.NODE_ENV !== "production") {
      console.warn("getHomepageData failed:", error);
    }
    return {
      hero: [],
      genres: [],
      new_movies: [],
      trending: [],
      premium_movies: [],
      featured_movies: [],
      featured_collections: [],
      series: [],
    };
  }
}

export async function getMovie(slug: string): Promise<Movie> {
  const res = await fetch(`${API_URL}/movies/slug/${slug}`, {
    cache: "no-store",
  });
  if (!res.ok) throw new Error(`Movie not found: ${slug} (HTTP ${res.status})`);
  const json = await res.json();
  if (!json.data) throw new Error(`Movie not found: ${slug} (null data)`);
  return normalizeMovieResponse(json.data);
}

// Get movie by ID
export async function getMovieById(id: string): Promise<Movie> {
  const res = await fetch(`${API_URL}/movies/${id}`, {
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Movie not found");
  const json = await res.json();
  const data = json.data;
  
  // Ensure views is explicitly set
  const movieWithViews = {
    ...normalizeMovieResponse(data),
    views: data?.views ?? 0,
  };
  
  return movieWithViews;
}

export async function searchMovies(q: string): Promise<Movie[]> {
  const res = await fetch(`${API_URL}/search?q=${encodeURIComponent(q)}`, {
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Search failed");
  const json = await res.json();
  return json.data;
}

// Trending movie response type - convert to Movie format for compatibility
export interface TrendingMovie {
  id: string;
  title: string;
  slug: string;
  poster_url: string;
  year: number;
  genre: string[];
  views_in_period: number;
}

// Convert trending movie to Movie format for carousel compatibility
function trendingToMovie(t: TrendingMovie): Movie {
  return {
    id: t.id,
    title: t.title,
    slug: t.slug,
    poster_url: t.poster_url,
    backdrop_url: t.poster_url,
    year: t.year,
    genre: t.genre,
    description: "",
    code: "",
    video_url: "",
    embed_url: "",
    source_type: "iframe_embed",
    duration: 0,
    quality: "",
    country: "",
    views: t.views_in_period,
    rating_avg: 0,
    rating_count: 0,
    created_at: "",
    updated_at: "",
  };
}

// Recommendation movie response type
export interface RecommendationMovie {
  id: string;
  title: string;
  slug: string;
  poster_url: string;
  year: number;
  genre: string[];
  score?: number;
}

// Convert recommendation to Movie format for carousel compatibility
function recommendationToMovie(r: RecommendationMovie): Movie {
  return {
    id: r.id,
    title: r.title,
    slug: r.slug,
    poster_url: r.poster_url,
    backdrop_url: r.poster_url,
    year: r.year,
    genre: r.genre,
    description: "",
    code: "",
    video_url: "",
    embed_url: "",
    source_type: "iframe_embed",
    duration: 0,
    quality: "",
    country: "",
    views: 0,
    rating_avg: 0,
    rating_count: 0,
    created_at: "",
    updated_at: "",
  };
}

// Get trending movies - returns as Movie[] for carousel compatibility
export async function getTrendingMovies(period: string = "24h", limit: number = 12): Promise<Movie[]> {
  const res = await fetch(`${API_URL}/movies/trending?period=${period}&limit=${limit}`, {
    next: { revalidate: 60 }, // ISR: public trending list, 60s cache
  });
  if (!res.ok) throw new Error("Failed to fetch trending movies");
  const json = await res.json();
  const trending: TrendingMovie[] = json.data || [];
  return trending.map(trendingToMovie);
}

// Get movie recommendations - returns as Movie[] for carousel compatibility
// Uses new endpoint with hybrid scoring (content + popularity + user personalization)
export async function getRecommendations(movieId: string, limit: number = 12): Promise<Movie[]> {
  const res = await fetch(`${API_URL}/movies/recommendations?movie_id=${movieId}&limit=${limit}`, {
    cache: "no-store",
  });
  if (!res.ok) {
    // Fallback to old endpoint if new one fails
    const fallbackRes = await fetch(`${API_URL}/movies/${movieId}/recommendations?limit=${limit}`, {
      cache: "no-store",
    });
    if (!fallbackRes.ok) throw new Error("Failed to fetch recommendations");
    const json = await fallbackRes.json();
    const recommendations: RecommendationMovie[] = json.data || [];
    return recommendations.map(recommendationToMovie);
  }
  const json = await res.json();
  const movies: Movie[] = json.data || [];
  return movies;
}

// ── Auth ──────────────────────────────────────────────────────

export async function login(
  email: string,
  password: string
): Promise<{ token: string }> {
  const res = await fetch(`${API_URL}/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password }),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Login failed");
  }
  return res.json();
}

// Telegram Auth Types
export interface TelegramAuthStartResponse {
  code: string;
  bot_url: string;
  expires_at: string;
}

export interface TelegramAuthStatusResponse {
  status: "pending" | "completed" | "expired";
  code: string;
  expires_at: string;
  user?: {
    id: string;
    telegram_id: number;
    username: string;
    first_name: string;
    last_name: string;
    role: string;
    auth_provider: string;
    ban?: BanInfo;
  };
  token?: string;
}

export interface PremiumStarsSessionResponse {
  token: string;
  package: string;
  stars_price: number;
  expires_at: string;
  bot_url: string;
}

export interface BanInfo {
  is_banned: boolean;
  reason?: string;
  banned_at?: string;
  banned_until?: string | null;
  banned_by_user_id?: string;
  banned_by_username?: string;
}

export interface CurrentUser {
  id: string;
  telegram_id?: number;
  username?: string;
  first_name?: string;
  last_name?: string;
  display_name?: string;
  profile_image_url?: string;
  photo_url?: string; // Alternative field from Telegram
  role: string;
  is_premium?: boolean;
  is_premium_active?: boolean;
  premium_started_at?: string | null;
  premium_expires_at?: string | null;
  wallet_balance?: number;
  profile_style?: ProfileStyle;
  privacy?: PrivacySettings;
  ban?: BanInfo;
  auth_provider: string;
  created_at?: string;
  last_login_at?: string;
}

// Profile style customization (premium only)
export interface ProfileStyle {
  frame?: string;    // "none", "gold"
  theme?: string;    // "default", "gold_dark"
  gradient?: string; // "none", "gold", "purple"
}

// Privacy settings (premium only)
export interface PrivacySettings {
  is_private?: boolean;
}

export interface AuthMeResponse {
  authenticated: boolean;
  user?: CurrentUser;
}

export interface AuthBootstrapResponse extends AuthMeResponse {
  unread_count?: number;
  flags?: {
    is_premium?: boolean;
    is_premium_active?: boolean;
  };
}

// Start Telegram login flow
export async function startTelegramLogin(): Promise<TelegramAuthStartResponse> {
  const res = await fetch(`${API_URL}/auth/telegram/start`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to start login");
  }
  return res.json();
}

// Check Telegram login status (polling)
export async function getTelegramAuthStatus(code: string): Promise<TelegramAuthStatusResponse> {
  const res = await fetch(`${API_URL}/auth/telegram/status/${code}`, {
    cache: "no-store",
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to get status");
  }
  return res.json();
}

export async function createPremiumStarsSession(
  token: string,
  packageId: string
): Promise<PremiumStarsSessionResponse> {
  const res = await fetch(`${API_URL}/premium/stars/session`, {
    method: "POST",
    headers: authHeaders(token),
    body: JSON.stringify({ package: packageId }),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to create premium purchase session");
  }
  return res.json();
}

// Get current authenticated user
export async function getCurrentUser(token: string): Promise<AuthMeResponse> {
  const res = await fetch(`${API_URL}/auth/me`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) {
    if (res.status === 401) {
      return { authenticated: false };
    }
    throw new Error("Failed to get user");
  }
  return res.json();
}

export async function getAuthBootstrap(token: string): Promise<AuthBootstrapResponse> {
  return dedupeBrowserRequest(`auth-bootstrap:${token}`, 5000, async () => {
    const res = await fetch(`${API_URL}/auth/bootstrap`, {
      headers: authHeaders(token),
      cache: "no-store",
    });
    if (!res.ok) {
      if (res.status === 401) {
        return { authenticated: false };
      }
      throw new Error("Failed to get auth bootstrap");
    }
    return res.json();
  });
}

// Logout
export async function logout(token: string): Promise<void> {
  const res = await fetch(`${API_URL}/auth/logout`, {
    method: "POST",
    headers: authHeaders(token),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Logout failed");
  }
}

// Refresh token - extends session without requiring re-login
export async function refreshAuthToken(token: string): Promise<{ token: string; authenticated: boolean }> {
  const res = await fetch(`${API_URL}/auth/refresh`, {
    method: "POST",
    headers: authHeaders(token),
  });
  if (!res.ok) {
    throw new Error("Token refresh failed");
  }
  return res.json();
}

// Update user profile (display name / first name)
export async function updateProfile(token: string, displayName: string): Promise<void> {
  const res = await fetch(`${API_URL}/auth/me`, {
    method: "PATCH",
    headers: authHeaders(token),
    body: JSON.stringify({ first_name: displayName }),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to update profile");
  }
}

// Update user's preferred language
export async function updateLanguageCode(token: string, languageCode: string): Promise<void> {
  const res = await fetch(`${API_URL}/auth/me/language`, {
    method: "PATCH",
    headers: authHeaders(token),
    body: JSON.stringify({ language_code: languageCode }),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to update language");
  }
}

// Upload profile image
export async function uploadProfileImage(token: string, file: File): Promise<string> {
  const formData = new FormData();
  formData.append("image", file);

  const res = await fetch(`${API_URL}/auth/upload/profile-image`, {
    method: "POST",
    headers: {
      "Authorization": `Bearer ${token}`,
    },
    body: formData,
  });

  if (!res.ok) {
    const error = await res.json().catch(() => ({ error: "Upload failed" }));
    throw new Error(error.error || "Failed to upload profile image");
  }

  const data = await res.json();
  return data.profile_image_url || data.user?.profile_image_url || "";
}

// Update profile image by URL
export async function updateProfileImageByURL(token: string, imageURL: string): Promise<void> {
  const formData = new FormData();
  formData.append("image_url", imageURL);

  const res = await fetch(`${API_URL}/auth/upload/profile-image`, {
    method: "POST",
    headers: {
      "Authorization": `Bearer ${token}`,
    },
    body: formData,
  });

  if (!res.ok) {
    const error = await res.json().catch(() => ({ error: "Update failed" }));
    throw new Error(error.error || "Failed to update profile image");
  }
}

// Update profile style (premium only)
export async function updateProfileStyle(token: string, profileStyle: ProfileStyle): Promise<void> {
  const res = await fetch(`${API_URL}/auth/profile-style`, {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
      "Authorization": `Bearer ${token}`,
    },
    body: JSON.stringify(profileStyle),
  });

  if (!res.ok) {
    const error = await res.json().catch(() => ({ error: "Update failed" }));
    throw new Error(error.error || "Failed to update profile style");
  }
}

// Update privacy settings (premium only)
export async function updatePrivacySettings(token: string, privacy: PrivacySettings): Promise<void> {
  const res = await fetch(`${API_URL}/auth/privacy`, {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
      "Authorization": `Bearer ${token}`,
    },
    body: JSON.stringify(privacy),
  });

  if (!res.ok) {
    const error = await res.json().catch(() => ({ error: "Update failed" }));
    throw new Error(error.error || "Failed to update privacy settings");
  }
}

// ── User History & Favorites API ──────────────────────────────────

export interface WatchHistoryItem {
  id: string;
  record_id?: string;
  movie_id?: string;
  target_type: "movie" | "episode" | "series";
  target_id: string;
  series_id?: string;
  season_id?: string;
  episode_id?: string;
  season_number?: number;
  watched_at: string;
  last_position_sec?: number;
  duration_sec?: number;
  progress_percent?: number;
  completed?: boolean;
  title: string;
  poster_url: string;
  backdrop_url?: string;
  slug: string;
  code: string;
  year: number;
  quality: string;
  website_url: string;
  type?: "movie" | "episode" | "series";
  series_title?: string;
  series_slug?: string;
  episode_number?: number;
  episode_title?: string;
}

export interface FavoriteItem {
  id: string;
  record_id?: string;
  movie_id?: string;
  target_type: "movie" | "episode" | "series";
  target_id: string;
  series_id?: string;
  season_id?: string;
  episode_id?: string;
  season_number?: number;
  created_at: string;
  title: string;
  poster_url: string;
  backdrop_url?: string;
  slug: string;
  code: string;
  year: number;
  quality: string;
  website_url: string;
  type?: "movie" | "episode" | "series";
  series_title?: string;
  series_slug?: string;
  episode_number?: number;
  episode_title?: string;
}

interface TargetOptions {
  targetType?: "movie" | "episode" | "series";
  keepalive?: boolean;
}

export interface WatchProgressSummary {
  current_time: number;
  duration: number;
  progress_percent: number;
  completed?: boolean;
}

// Record watch history (authenticated)
export async function recordWatchHistory(token: string, targetId: string, options?: TargetOptions): Promise<void> {
  const res = await fetch(`${API_URL}/user/history/watch`, {
    method: "POST",
    headers: authHeaders(token),
    body: JSON.stringify({
      movie_id: targetId,
      target_id: targetId,
      target_type: options?.targetType || "movie",
    }),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: "Failed to record watch history" }));
    throw new Error(err.error || "Failed to record watch history");
  }
}

// Save watch progress (authenticated)
export async function saveWatchProgress(token: string, targetId: string, positionSec: number, durationSec: number, options?: TargetOptions): Promise<void> {
  const res = await fetch(`${API_URL}/watch/${targetId}/progress`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify({ positionSec, durationSec, target_type: options?.targetType || "movie" }),
  });
  if (!res.ok) throw new Error("Failed to save progress");
}

export async function saveUnifiedWatchProgress(
  token: string,
  targetId: string,
  currentTime: number,
  duration: number,
  options?: TargetOptions
): Promise<WatchProgressSummary> {
  const normalizedTargetId = String(targetId || "").trim();
  const normalizedCurrentTime = Math.floor(currentTime);
  const normalizedDuration = Math.floor(duration);
  if (!normalizedTargetId) {
    throw new Error("Missing target_id for watch progress");
  }
  if (!Number.isFinite(normalizedCurrentTime) || !Number.isFinite(normalizedDuration) || normalizedDuration <= 0) {
    throw new Error("Watch progress requires finite current_time and duration");
  }
  const payload = {
    target_type: options?.targetType || "movie",
    target_id: normalizedTargetId,
    current_time: normalizedCurrentTime,
    duration: normalizedDuration,
  };
  if (process.env.NODE_ENV !== "production") {
    console.debug("[progress] save payload", payload);
  }
  const res = await fetch(`${API_URL}/watch/progress`, {
    method: "POST",
    headers: authHeaders(token),
    keepalive: Boolean(options?.keepalive),
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: "Failed to save progress" }));
    throw new Error(err.error || "Failed to save progress");
  }
  return res.json();
}

export async function getWatchProgress(
  token: string,
  targetId: string,
  options?: TargetOptions
): Promise<WatchProgressSummary> {
  const normalizedTargetId = String(targetId || "").trim();
  if (!normalizedTargetId) {
    throw new Error("Missing target_id for watch progress");
  }
  const params = new URLSearchParams({
    target_type: options?.targetType || "movie",
    target_id: normalizedTargetId,
  });
  const res = await fetch(`${API_URL}/watch/progress?${params.toString()}`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: "Failed to get progress" }));
    throw new Error(err.error || "Failed to get progress");
  }
  return res.json();
}

export async function resetWatchProgress(
  token: string,
  targetId: string,
  options?: TargetOptions
): Promise<WatchProgressSummary> {
  const normalizedTargetId = String(targetId || "").trim();
  if (!normalizedTargetId) {
    throw new Error("Missing target_id for watch progress reset");
  }
  const payload = {
    target_type: options?.targetType || "movie",
    target_id: normalizedTargetId,
  };
  const res = await fetch(`${API_URL}/watch/progress/reset`, {
    method: "POST",
    headers: authHeaders(token),
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: "Failed to reset progress" }));
    throw new Error(err.error || "Failed to reset progress");
  }
  return res.json();
}

// Mark watch as complete (authenticated)
export async function markWatchComplete(token: string, targetId: string, durationSec?: number, options?: TargetOptions): Promise<void> {
  const res = await fetch(`${API_URL}/watch/${targetId}/complete`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify({ durationSec, target_type: options?.targetType || "movie" }),
  });
  if (!res.ok) throw new Error("Failed to mark complete");
}

// Get watch history (authenticated)
export async function getWatchHistory(token: string): Promise<WatchHistoryItem[]> {
  const res = await fetch(`${API_URL}/user/history`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) {
    throw new Error("Failed to get watch history");
  }
  const json = await res.json();
  return json.data || [];
}

// Add to favorites (authenticated)
export async function addFavorite(token: string, targetId: string, options?: TargetOptions): Promise<void> {
  const query = options?.targetType ? `?target_type=${encodeURIComponent(options.targetType)}` : "";
  const res = await fetch(`${API_URL}/user/favorites/${targetId}${query}`, {
    method: "POST",
    headers: authHeaders(token),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: "Failed to add favorite" }));
    throw new Error(err.error || "Failed to add favorite");
  }
}

// Remove from favorites (authenticated)
export async function removeFavorite(token: string, targetId: string, options?: TargetOptions): Promise<void> {
  const query = options?.targetType ? `?target_type=${encodeURIComponent(options.targetType)}` : "";
  const res = await fetch(`${API_URL}/user/favorites/${targetId}${query}`, {
    method: "DELETE",
    headers: authHeaders(token),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: "Failed to remove favorite" }));
    throw new Error(err.error || "Failed to remove favorite");
  }
}

// Get favorites (authenticated)
export async function getFavorites(token: string): Promise<FavoriteItem[]> {
  const res = await fetch(`${API_URL}/user/favorites`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) {
    throw new Error("Failed to get favorites");
  }
  const json = await res.json();
  return json.data || [];
}

// Check if target is favorite (authenticated)
export async function checkIsFavorite(token: string, targetId: string, options?: TargetOptions): Promise<boolean> {
  const res = await fetch(`${API_URL}/user/favorites`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) {
    return false;
  }
  const json = await res.json();
  const favorites: FavoriteItem[] = json.data || [];
  const targetType = options?.targetType || "movie";
  return favorites.some((f) => {
    const favoriteTargetId = f.target_id || f.movie_id;
    const favoriteTargetType = f.target_type || f.type || "movie";
    return favoriteTargetId === targetId && favoriteTargetType === targetType;
  });
}

// Record view (public - no auth required)
export async function recordView(targetId: string): Promise<void> {
  await fetch(`${API_URL}/movies/${targetId}/view`, {
    method: "POST",
  });
}

// ── Rating API ─────────────────────────────────────────────────

export interface RatingSummary {
  rating_avg: number;
  rating_count: number;
  user_rating?: number;
}

// Get rating summary for a movie (public - no auth required, but can pass token for user rating)
export async function getRatingSummary(movieId: string, token?: string): Promise<RatingSummary> {
  const headers: HeadersInit = {};
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }
  const res = await fetch(`${API_URL}/v1/movies/${movieId}/rating-summary`, {
    headers,
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Failed to fetch rating summary");
  return res.json();
}

// Set rating (authenticated)
export async function setRating(token: string, movieId: string, rating: number): Promise<RatingSummary> {
  const res = await fetch(`${API_URL}/v1/movies/${movieId}/rating`, {
    method: "POST",
    headers: authHeaders(token),
    body: JSON.stringify({ rating }),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: "Failed to set rating" }));
    throw new Error(err.error || "Failed to set rating");
  }
  const json = await res.json();
  return json.data || json;
}

// Delete rating (authenticated)
export async function deleteRating(token: string, movieId: string): Promise<RatingSummary> {
  const res = await fetch(`${API_URL}/v1/movies/${movieId}/rating`, {
    method: "DELETE",
    headers: authHeaders(token),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: "Failed to delete rating" }));
    throw new Error(err.error || "Failed to delete rating");
  }
  const json = await res.json();
  return json.data || json;
}

// ── Series Rating API ───────────────────────────────────────────────

// Get rating summary for a series (public - no auth required, but can pass token for user rating)
export async function getSeriesRatingSummary(seriesId: string, token?: string): Promise<RatingSummary> {
  const headers: HeadersInit = {};
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }
  const res = await fetch(`${API_URL}/v1/series/${seriesId}/rating-summary`, {
    headers,
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Failed to fetch series rating summary");
  return res.json();
}

// Set series rating (authenticated)
export async function setSeriesRating(token: string, seriesId: string, rating: number): Promise<RatingSummary> {
  const res = await fetch(`${API_URL}/v1/series/${seriesId}/rating`, {
    method: "POST",
    headers: authHeaders(token),
    body: JSON.stringify({ rating }),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: "Failed to set series rating" }));
    throw new Error(err.error || "Failed to set series rating");
  }
  const json = await res.json();
  return json.data || json;
}

// Delete series rating (authenticated)
export async function deleteSeriesRating(token: string, seriesId: string): Promise<RatingSummary> {
  const res = await fetch(`${API_URL}/v1/series/${seriesId}/rating`, {
    method: "DELETE",
    headers: authHeaders(token),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: "Failed to delete series rating" }));
    throw new Error(err.error || "Failed to delete series rating");
  }
  const json = await res.json();
  return json.data || json;
}

// ── Episode Rating API ───────────────────────────────────────

export async function getEpisodeRatingSummary(episodeId: string, token?: string): Promise<RatingSummary> {
  const headers: HeadersInit = {};
  if (token) headers["Authorization"] = `Bearer ${token}`;
  const res = await fetch(`${API_URL}/v1/episodes/${episodeId}/rating-summary`, {
    headers,
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Failed to fetch episode rating summary");
  return res.json();
}

export async function setEpisodeRating(token: string, episodeId: string, rating: number): Promise<RatingSummary> {
  const res = await fetch(`${API_URL}/v1/episodes/${episodeId}/rating`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
    body: JSON.stringify({ rating }),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: "Failed to set rating" }));
    throw new Error(err.error || "Failed to set rating");
  }
  const json = await res.json();
  return json.data || json;
}

export async function deleteEpisodeRating(token: string, episodeId: string): Promise<RatingSummary> {
  const res = await fetch(`${API_URL}/v1/episodes/${episodeId}/rating`, {
    method: "DELETE",
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: "Failed to delete rating" }));
    throw new Error(err.error || "Failed to delete rating");
  }
  const json = await res.json();
  return json.data || json;
}

// ── Admin API (JWT required) ──────────────────────────────────

function authHeaders(token: string) {
  return {
    "Content-Type": "application/json",
    Authorization: `Bearer ${token}`,
  };
}

export async function adminCreateMovie(
  token: string,
  input: MovieInput
): Promise<Movie> {
  const res = await fetch(`${API_URL}/admin/movies`, {
    method: "POST",
    headers: authHeaders(token),
    body: JSON.stringify(input),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Create failed");
  }
  const json = await res.json();
  return json.data;
}

export async function adminUpdateMovie(
  token: string,
  id: string,
  input: MovieInput
): Promise<Movie> {
  const res = await fetch(`${API_URL}/admin/movies/${id}`, {
    method: "PUT",
    headers: authHeaders(token),
    body: JSON.stringify(input),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Update failed");
  }
  const json = await res.json();
  return json.data;
}

// CascadeDeleteSummary mirrors the structured response from the
// admin movie/series delete endpoints. Used by the admin UI to show
// what was removed and surface partial-failure warnings.
export interface CascadeDeleteB2Summary {
  files_deleted: number;
  prefixes_deleted: string[];
  skipped: string[];
  errors: string[];
}

export interface MovieDeleteResponse {
  success: boolean;
  message?: string;
  job_id?: string;
  status?: string;
  deleted_db?: {
    movie_id: string;
    title: string;
    clips_deleted: number;
    instagram_schedules_deleted: number;
    publish_jobs_deleted: number;
  };
  deleted_b2?: CascadeDeleteB2Summary;
}

export interface SeriesDeleteResponse {
  success: boolean;
  message?: string;
  job_id?: string;
  status?: string;
  deleted_db?: {
    series_id: string;
    title: string;
    seasons_deleted: number;
    episodes_deleted: number;
    clips_deleted: number;
    instagram_schedules_deleted: number;
    publish_jobs_deleted: number;
  };
  deleted_b2?: CascadeDeleteB2Summary;
}

export async function adminDeleteMovie(
  token: string,
  id: string
): Promise<MovieDeleteResponse> {
  const res = await fetch(`${API_URL}/admin/movies/${id}`, {
    method: "DELETE",
    headers: authHeaders(token),
  });
  const json = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(json?.error || "Delete failed");
  }
  return json as MovieDeleteResponse;
}

export async function adminGetMovies(token: string): Promise<Movie[]> {
  const res = await fetch(`${API_URL}/admin/movies?limit=500`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Failed to fetch");
  const json = await res.json();
  return (json.data || []).map((item: any) => normalizeMovieResponse(item));
}

export async function approveMovie(token: string, id: string): Promise<void> {
  const res = await fetch(`${API_URL}/admin/movies/${id}/approve`, {
    method: "PATCH",
    headers: authHeaders(token),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to approve movie");
  }
}

export async function rejectMovie(token: string, id: string): Promise<void> {
  const res = await fetch(`${API_URL}/admin/movies/${id}/reject`, {
    method: "PATCH",
    headers: authHeaders(token),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to reject movie");
  }
}

export type MovieAssetType = "poster" | "backdrop" | "video" | "temp_movie";

export interface MovieAssetUploadResponse {
  message: string;
  url: string;
  type: MovieAssetType;
  filename: string;
  temp_key?: string; // For temp_movie uploads
}

export interface BackendMediaUploadResponse {
  success: boolean;
  url: string;
  path: string;
}

export interface MediaAccessResponse {
  success: boolean;
  protected: boolean;
  playback_url: string;
  expires_at: string;
  cookie_name?: string;
}

export async function getProtectedMediaAccess(params: {
  movieId?: string;
  episodeId?: string;
  token?: string | null;
}): Promise<MediaAccessResponse> {
  const qs = new URLSearchParams();
  if (params.movieId) qs.set("movie_id", params.movieId);
  if (params.episodeId) qs.set("episode_id", params.episodeId);

  const headers: HeadersInit = {};
  if (params.token) {
    headers["Authorization"] = `Bearer ${params.token}`;
  }

  const res = await fetch(`${API_URL}/media/access-token?${qs.toString()}`, {
    headers,
    cache: "no-store",
    credentials: "include",
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: "Failed to get media access" }));
    throw new Error(err.message || err.error || "Failed to get media access");
  }
  return res.json();
}

export async function uploadMovieAsset(
  token: string,
  file: File,
  type: MovieAssetType,
  onProgress?: (progress: number) => void
): Promise<MovieAssetUploadResponse> {
  let uploadUrl: string;
  let formField: string;

  switch (type) {
    case "poster":
      uploadUrl = `${API_URL}/admin/upload/movie-poster`;
      formField = "file";
      break;
    case "backdrop":
      uploadUrl = `${API_URL}/admin/upload/movie-backdrop`;
      formField = "file";
      break;
    case "temp_movie":
    case "video":
      uploadUrl = `${WORKER_URL}/upload-temp-movie`;
      formField = "file";
      break;
    default:
      uploadUrl = `${API_URL}/admin/movies/upload`;
      formField = "file";
  }

  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", uploadUrl);
    xhr.setRequestHeader("Authorization", `Bearer ${token}`);

    xhr.upload.onprogress = (event) => {
      if (onProgress && event.lengthComputable) {
        const percent = Math.round((event.loaded / event.total) * 100);
        onProgress(percent);
      }
    };

    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        const response = JSON.parse(xhr.responseText);
        resolve({
          message: "Uploaded successfully",
          url: response.url,
          type,
          filename: file.name,
          temp_key: response.temp_key || response.path,
        });
      } else {
        let errorMsg = "Upload failed";
        try {
          const errResp = JSON.parse(xhr.responseText);
          errorMsg = errResp.error || errResp.message || errorMsg;
        } catch {}
        reject(new Error(errorMsg));
      }
    };

    xhr.onerror = () => reject(new Error("Upload failed"));

    const formData = new FormData();
    formData.append(formField, file);
    xhr.send(formData);
  });
}

function uploadBackendMediaToEndpoint(
  token: string,
  file: File,
  endpoint: string,
  onProgress?: (progress: UploadProgressInfo) => void
): Promise<BackendMediaUploadResponse> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", `${API_URL}${endpoint}`);
    xhr.setRequestHeader("Authorization", `Bearer ${token}`);

    const startedAt = Date.now();
    xhr.upload.onprogress = (event) => {
      if (!onProgress) return;
      const elapsedSec = Math.max(0.001, (Date.now() - startedAt) / 1000);
      const speedMBps = event.loaded / 1024 / 1024 / elapsedSec;
      const info: UploadProgressInfo = {
        loaded: event.loaded,
        total: event.lengthComputable ? event.total : undefined,
        uploadedMB: event.loaded / 1024 / 1024,
        speedMBps,
      };
      if (event.lengthComputable) {
        info.progress = Math.round((event.loaded / event.total) * 100);
        const remaining = event.total - event.loaded;
        info.etaSeconds = Math.round(remaining / 1024 / 1024 / Math.max(0.001, speedMBps));
      }
      onProgress(info);
    };

    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        try {
          const resp = JSON.parse(xhr.responseText);
          resolve({
            success: resp.success ?? true,
            url: resp.url,
            path: resp.path || resp.file_key || "",
          });
        } catch {
          reject(new Error("Invalid upload response"));
        }
      } else {
        let msg = `Upload failed (${xhr.status})`;
        try {
          const err = JSON.parse(xhr.responseText);
          msg = err.error || err.message || msg;
        } catch {}
        reject(new Error(msg));
      }
    };

    xhr.onerror = () => reject(new Error("Tarmoq xatosi. Serverga ulanib bo'lmadi."));
    xhr.ontimeout = () => reject(new Error("Yuklash vaqti tugadi."));
    xhr.onabort = () => reject(new Error("Yuklash to'xtatildi."));

    const formData = new FormData();
    formData.append("file", file);
    xhr.send(formData);
  });
}

// Direct browser-to-B2 upload.
export interface UploadProgressInfo {
  progress?: number;
  loaded: number;
  total?: number;
  uploadedMB: number;
  speedMBps?: number;
  etaSeconds?: number;
}

export async function directB2Upload(
  token: string,
  file: File,
  type: "poster" | "backdrop" | "video",
  onProgress?: (progress: UploadProgressInfo) => void
): Promise<{ url: string; file_key: string }> {
  const maxSize = type === "video" ? 5 * 1024 * 1024 * 1024 : 20 * 1024 * 1024;
  const allowedTypes = type === "video"
    ? ["video/mp4", "video/webm", "video/ogg", "video/quicktime"]
    : ["image/jpeg", "image/jpg", "image/png", "image/webp"];

  if (file.size > maxSize) {
    throw new Error(`File too large (max ${Math.round(maxSize / 1024 / 1024)}MB)`);
  }
  if (file.type && !allowedTypes.includes(file.type)) {
    throw new Error(type === "video"
      ? "Invalid video type. Allowed: mp4, webm, ogg, mov"
      : "Invalid image type. Allowed: jpg, png, webp");
  }

  const qs = new URLSearchParams({
    type,
    filename: file.name,
    size: String(file.size),
  });
  if (file.type) qs.set("contentType", file.type);

  logger.debug("[B2Upload] requesting upload URL", { type, size: file.size });
  const authRes = await fetch(`${API_URL}/upload/b2-url?${qs}`, {
    headers: { Authorization: `Bearer ${token}` },
    cache: "no-store",
  });
  if (!authRes.ok) {
    const err = await authRes.json().catch(() => ({ error: "Failed to get upload URL" }));
    const msg = err.error || `Failed to get upload URL (status ${authRes.status})`;
    logger.error("[B2Upload] authorize failed", authRes.status);
    throw new Error(msg);
  }

  const uploadAuth: UploadURLResponse = await authRes.json();
  const uploadUrl = uploadAuth.uploadUrl || uploadAuth.upload_url;
  const authorizationToken = uploadAuth.authorizationToken || uploadAuth.auth_token;
  const fileKey = uploadAuth.fileKey || uploadAuth.file_key;
  const cdnUrl = uploadAuth.cdnUrl || uploadAuth.cdn_url;

  if (!uploadUrl || !authorizationToken || !fileKey) {
    logger.error("[B2Upload] incomplete auth response");
    throw new Error("Upload authorization response is incomplete");
  }
  logger.debug("[B2Upload] got fresh URL", { type });

  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    const startedAt = Date.now();
    let lastProgressAt = 0;

    xhr.open("POST", uploadUrl);
    xhr.setRequestHeader("Authorization", authorizationToken);
    xhr.setRequestHeader("X-Bz-File-Name", encodeURIComponent(fileKey));
    xhr.setRequestHeader("X-Bz-Content-Sha1", "do_not_verify");
    xhr.setRequestHeader("Content-Type", file.type || "application/octet-stream");

    xhr.upload.onprogress = (event) => {
      if (!onProgress) return;

      const now = Date.now();
      const shouldUpdate = now - lastProgressAt >= 400 || (event.lengthComputable && event.loaded >= event.total);
      if (!shouldUpdate) return;
      lastProgressAt = now;

      const elapsedSeconds = Math.max((now - startedAt) / 1000, 0.001);
      const speedBytesPerSecond = event.loaded / elapsedSeconds;
      const hasTotal = event.lengthComputable && event.total > 0;
      const total = hasTotal ? event.total : undefined;
      const progress = total ? Math.min(100, Math.round((event.loaded / total) * 100)) : undefined;
      const etaSeconds = total && speedBytesPerSecond > 0
        ? Math.max(0, Math.round((total - event.loaded) / speedBytesPerSecond))
        : undefined;

      onProgress({
        progress,
        loaded: event.loaded,
        total,
        uploadedMB: event.loaded / 1024 / 1024,
        speedMBps: speedBytesPerSecond / 1024 / 1024,
        etaSeconds,
      });
    };

    xhr.upload.onload = () => {
      if (onProgress) {
        onProgress({
          progress: 100,
          loaded: file.size,
          total: file.size || undefined,
          uploadedMB: file.size / 1024 / 1024,
          speedMBps: undefined,
          etaSeconds: 0,
        });
      }
    };

    xhr.onload = async () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        logger.debug("[B2Upload] B2 upload succeeded", { type });
        try {
          const completeRes = await fetch(`${API_URL}/upload/b2-complete`, {
            method: "POST",
            headers: authHeaders(token),
            body: JSON.stringify({
              fileKey,
              fileName: file.name,
              size: file.size,
              type,
              contentType: file.type,
            }),
          });
          if (!completeRes.ok) {
            const err = await completeRes.json().catch(() => ({ error: "Upload completed but metadata save failed" }));
            const msg = err.error || "Upload completed but metadata save failed";
            logger.error("[B2Upload] complete failed", completeRes.status);
            reject(new Error(msg));
            return;
          }
          const completed = await completeRes.json();
          logger.debug("[B2Upload] upload finalized", { type });
          resolve({ url: completed.url || cdnUrl || "", file_key: completed.file_key || completed.fileKey || fileKey });
        } catch (err) {
          logger.error("[B2Upload] complete threw", err);
          reject(err instanceof Error ? err : new Error("Upload completed but metadata save failed"));
        }
      } else {
        let errorDetail = "";
        try {
          const errResp = JSON.parse(xhr.responseText);
          errorDetail = errResp.message || errResp.code || errResp.error || "";
        } catch {
          errorDetail = xhr.responseText?.slice(0, 200) || "";
        }
        const errorMsg = errorDetail
          ? `B2 upload rejected (${xhr.status}): ${errorDetail}`
          : `B2 upload rejected with status ${xhr.status}`;
        logger.error("[B2Upload] B2 upload rejected", xhr.status);
        reject(new Error(errorMsg));
      }
    };

    // onerror fires when the TCP connection is dropped before a response arrives
    // (e.g. nginx client_max_body_size exceeded, or genuine network failure).
    // There is no response body to read in this case.
    xhr.onerror = () => {
      const sizeMB = (file.size / 1024 / 1024).toFixed(1);
      const msg = file.size > 100 * 1024 * 1024
        ? `Yuklash uzildi: fayl juda katta bo'lishi mumkin (${sizeMB} MB) yoki tarmoq xatosi.`
        : `Yuklash uzildi (${sizeMB} MB): tarmoq xatosi yoki B2 server ulanmadi.`;
      logger.error("[B2Upload] xhr.onerror");
      reject(new Error(msg));
    };
    xhr.ontimeout = () => {
      logger.error("[B2Upload] xhr.ontimeout");
      reject(new Error("Yuklash vaqti tugadi (timeout). Qaytadan urinib ko'ring."));
    };
    xhr.onabort = () => {
      logger.warn("[B2Upload] xhr.onabort");
      reject(new Error("Yuklash bekor qilindi."));
    };

    xhr.send(file);
  });
}

// Backend-proxied upload for movie poster/backdrop. The browser POSTs the file
// to an admin upload endpoint; the backend uploads to B2 and returns the final
// CDN URL + storage path.
export async function backendUploadMovieImage(
  token: string,
  file: File,
  type: "poster" | "backdrop",
  onProgress?: (progress: UploadProgressInfo) => void
): Promise<{ url: string; file_key: string }> {
  const endpoint = type === "poster" ? "/admin/upload/movie-poster" : "/admin/upload/movie-backdrop";
  const resp = await uploadBackendMediaToEndpoint(token, file, endpoint, onProgress);
  return { url: resp.url, file_key: resp.path };
}

export async function uploadSeriesImage(
  token: string,
  file: File,
  type: "poster" | "backdrop",
  onProgress?: (progress: UploadProgressInfo) => void
): Promise<{ url: string; file_key: string }> {
  const endpoint = type === "poster" ? "/admin/upload/series-poster" : "/admin/upload/series-backdrop";
  const resp = await uploadBackendMediaToEndpoint(token, file, endpoint, onProgress);
  return { url: resp.url, file_key: resp.path };
}

export async function uploadCollectionPoster(
  token: string,
  file: File,
  onProgress?: (progress: UploadProgressInfo) => void
): Promise<{ url: string; file_key: string }> {
  const resp = await uploadBackendMediaToEndpoint(token, file, "/admin/upload/collection-poster", onProgress);
  return { url: resp.url, file_key: resp.path };
}

// ── Ingestion API ────────────────────────────────────────────────

export type IngestionStatus = 
  | "queued"
  | "pending" 
  | "parsing" 
  | "downloading" 
  | "downloaded"
  | "ready_to_process"
  | "processing" 
  | "uploading" 
  | "completed" 
  | "failed"
  | "download_failed"
  | "parsing_complete"
  | "enriching_metadata"
  | "finding_poster"
  | "generating_poster"
  | "uploading_poster"
  | "generating_backdrop"
  | "creating_movie"
  | "sending_notification"
  | "cutting_video"
  | "removing_watermark"
  | "adding_logo"
  | "hls_processing"
  | "finalizing_storage";

export interface IngestionLog {
  timestamp: string;
  message: string;
  level: string;
}

export interface VideoSource {
  quality: string;
  url: string;
  type: string;
}

export interface ParsedMetadata {
  title: string;
  description: string;
  poster: string;
  img: string;
  backdrop: string;
  year: number;
  genres: string[];
  country: string;
  duration: number;
  video_page_url: string;
  video_urls: VideoSource[];
}

export interface IngestionJob {
  id: string;
  movie_id?: string;
  title?: string;
  source: string;
  source_id: string;
  detail_url: string;
  status: IngestionStatus;
  progress: number;
  logs: IngestionLog[];
  metadata?: ParsedMetadata;
  error?: string;
  local_path?: string;
  output_path?: string;
  playlist_path?: string;
  source_file_deleted?: boolean;
  retry_count: number;
  created_at: string;
  updated_at: string;
  started_at?: string;
  completed_at?: string;
  download_started_at?: string;
  download_finished_at?: string;
  queued_for_processing_at?: string;
  processing_started_at?: string;
  processing_finished_at?: string;
  // Real-time download progress fields
  stage?: string;
  downloaded_bytes?: number;
  total_bytes?: number;
  speed_mbps?: number;
  eta_seconds?: number;
  last_progress_at?: string;
  worker_id?: string;
  locked_until?: string;
  message?: string;
  // Direct upload fields
  temp_file_url?: string;
  temp_file_key?: string;
  quality?: string;
  is_premium?: boolean;
  // Serial/episode identification
  content_type?: string; // "" | "movie" | "episode" | "serial_parent"
  season_id?: string;
  episode_id?: string;
  series_id?: string;
  series_slug?: string;
  season_number?: number;
  episode_number?: number;
  // Serial parent summary (populated by background extractor)
  seasons_count?: number;
  episode_count?: number;
  child_jobs_created?: number;
  missing_episodes?: number[];
  // Source quality (selected by parser, validated by worker after download)
  source_quality?: string;
  selected_quality?: string;
  selected_video_url?: string;
  source_resolution?: string;
  available_qualities?: string[];
  generated_qualities?: string[];
  classifier_confidence?: number;
  classifier_evidence?: string;
  file_size?: number;
}

export interface SearchResult {
  title: string;
  year: number;
  poster: string;
  img: string;
  description: string;
  source_id: string;
  detail_url: string;
  source: string;
  type?: "movie" | "serial";
  confidence?: number;
  available_qualities?: string[];
  selected_quality?: string;
  selected_video_url?: string;
}

export interface IngestionJobInput {
  source: string;
  source_id: string;
  detail_url?: string;
  title?: string;
}

// Search movies on a source
export async function searchSource(
  source: string, 
  query: string
): Promise<{ results: SearchResult[] }> {
  const res = await fetch(
    `${API_URL}/ingestion/search?source=${source}&q=${encodeURIComponent(query)}`,
    { cache: "no-store" }
  );
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Search failed");
  }
  return res.json();
}

// Get movie details from a source
export async function getSourceDetails(
  source: string, 
  sourceId: string,
  detailUrl?: string
): Promise<ParsedMetadata> {
  let url = `${API_URL}/ingestion/details?source=${source}&id=${sourceId}`;
  if (detailUrl) {
    url += `&url=${encodeURIComponent(detailUrl)}`;
  }
  const res = await fetch(url, { cache: "no-store" });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to get details");
  }
  return res.json();
}

// Create an ingestion job
export async function createIngestionJob(
  token: string,
  input: IngestionJobInput
): Promise<IngestionJob> {
  const res = await fetch(`${API_URL}/admin/ingestion/jobs`, {
    method: "POST",
    headers: authHeaders(token),
    body: JSON.stringify(input),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to create job");
  }
  const json = await res.json();
  return json.data;
}

// Direct upload input type
export interface DirectUploadInput {
  title: string;
  temp_file_url: string;
  temp_file_key?: string; // B2 temp key for cleanup tracking
  poster_url?: string;
  backdrop_url?: string;
  year?: number;
  genres?: string[];
  country?: string;
  duration?: number;
  quality?: string;
  is_premium?: boolean;
}

// Create a direct upload ingestion job (for direct MP4 upload flow)
export async function createDirectUploadJob(
  token: string,
  input: DirectUploadInput
): Promise<IngestionJob> {
  const res = await fetch(`${API_URL}/admin/ingestion/direct-upload`, {
    method: "POST",
    headers: authHeaders(token),
    body: JSON.stringify(input),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to create direct upload job");
  }
  const json = await res.json();
  return json.data;
}

// Get all ingestion jobs
export async function getIngestionJobs(
  token: string,
  params?: { status?: string; source?: string; page?: number; limit?: number; skip?: number }
): Promise<{ data: IngestionJob[]; page: number; total: number; totalPages: number; limit: number; skip: number; status_counts?: Record<string, number> }> {
  const qs = new URLSearchParams();
  if (params?.status) qs.set("status", params.status);
  if (params?.source) qs.set("source", params.source);
  if (params?.page) qs.set("page", String(params.page));
  if (params?.limit) qs.set("limit", String(params.limit));
  if (params?.skip) qs.set("skip", String(params.skip));

  const res = await fetch(`${API_URL}/admin/ingestion/jobs?${qs}`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Failed to fetch jobs");
  return res.json();
}

export async function getDeleteJobStatus(token: string, jobId: string): Promise<any> {
  const res = await fetch(`${API_URL}/admin/delete-jobs/${jobId}`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: "Failed to fetch job status" }));
    throw new Error(err.error || "Failed to fetch job status");
  }
  return res.json();
}

// Retry a failed job
// stage: "download" | "process" | "upload" - which stage to retry from
// force: optional force flag to re-run even if already complete
export async function retryIngestionJob(
  token: string,
  jobId: string,
  stage: "download" | "process" | "upload" = "download",
  force: boolean = false
): Promise<void> {
  const params = new URLSearchParams();
  params.set("stage", stage);
  if (force) {
    params.set("force", "true");
  }
  
  const res = await fetch(`${API_URL}/admin/ingestion/jobs/${jobId}/retry?${params}`, {
    method: "POST",
    headers: authHeaders(token),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to retry job");
  }
}

export async function deleteIngestionJob(token: string, jobId: string): Promise<void> {
  const res = await fetch(`${API_URL}/admin/ingestion/jobs/${jobId}`, {
    method: "DELETE",
    headers: authHeaders(token),
  });
  if (!res.ok) {
    let msg = "Failed to delete job";
    try {
      const err = await res.json();
      msg = err.error || msg;
    } catch {
      /* keep default */
    }
    throw new Error(msg);
  }
}

export async function deleteIngestionSeries(
  token: string,
  seriesSlug: string,
): Promise<{ deleted: number }> {
  const res = await fetch(
    `${API_URL}/admin/ingestion/series/${encodeURIComponent(seriesSlug)}`,
    {
      method: "DELETE",
      headers: authHeaders(token),
    },
  );
  if (!res.ok) {
    let msg = "Failed to delete series jobs";
    try {
      const err = await res.json();
      msg = err.error || msg;
    } catch {
      /* keep default */
    }
    throw new Error(msg);
  }
  const data = await res.json().catch(() => ({}));
  return { deleted: Number(data.deleted) || 0 };
}

// ── Catalog & Source Import Types ───────────────────────────────────────

export interface CatalogItem {
  source_id: string;
  title: string;
  year: number;
  type: "movie" | "serial";
  poster: string;
  description: string;
  genres: string[];
  detail_url: string;
  confidence?: number;
  available_qualities?: string[];
  selected_quality?: string;
  selected_video_url?: string;
}

export interface CatalogResponse {
  items: CatalogItem[];
  page: number;
  limit: number;
  total: number;
  total_pages: number;
  has_more: boolean;
}

export interface ManualImportInput {
  video_url: string;
  title?: string;
  year?: number;
  poster?: string;
  backdrop?: string;
  type?: "movie" | "serial";
}

export interface CatalogImportInput {
  source: string;
  source_id: string;
  detail_url: string;
  title: string;
  type?: "movie" | "serial";
  year?: number;
  poster?: string;
  force_confirmed?: boolean;
}

export interface ImportConfirmationPayload {
  source: string;
  source_id: string;
  detail_url: string;
  title: string;
  year?: number;
  type?: string;
  poster?: string;
}

export interface ImportConfirmationResponse {
  error: string;
  reason?: string;
  confidence: number;
  selected: ImportConfirmationPayload;
  fetched: ImportConfirmationPayload;
  requires_confirmation: true;
}

export class ImportConfirmationError extends Error {
  response: ImportConfirmationResponse;

  constructor(response: ImportConfirmationResponse) {
    super(response.error || "Admin confirmation required");
    this.name = "ImportConfirmationError";
    this.response = response;
  }
}

export interface CatalogCategory {
  id: string;
  name: string;
  url: string;
  slug: string;
}

export interface CatalogCategoriesResponse {
  source: string;
  categories: CatalogCategory[];
}

// List genre/category links for a source
export async function listCatalogCategories(
  source: string
): Promise<CatalogCategoriesResponse> {
  const res = await fetch(
    `${API_URL}/ingestion/catalog/categories?source=${encodeURIComponent(source)}`,
    { cache: "no-store" }
  );
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to fetch categories");
  }
  return res.json();
}

// List catalog items from a source with pagination
export async function listCatalog(
  source: string,
  params?: { page?: number; limit?: number; type?: string; category_url?: string }
): Promise<CatalogResponse> {
  const qs = new URLSearchParams();
  qs.set("source", source);
  if (params?.page) qs.set("page", params.page.toString());
  if (params?.limit) qs.set("limit", params.limit.toString());
  if (params?.type) qs.set("type", params.type);
  if (params?.category_url) qs.set("category_url", params.category_url);

  const res = await fetch(`${API_URL}/ingestion/catalog?${qs}`, {
    cache: "no-store",
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to fetch catalog");
  }
  return res.json();
}

// Create a manual import job from direct video URL
export async function createManualImport(
  token: string,
  input: ManualImportInput
): Promise<IngestionJob> {
  const res = await fetch(`${API_URL}/admin/ingestion/manual`, {
    method: "POST",
    headers: { ...authHeaders(token), "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to create manual import job");
  }
  const json = await res.json();
  return json.data;
}

// Import an item from the catalog. For series, the backend now returns 202
// with {ok, job_id, message} after queueing a parent job; the heavy episode
// extraction runs in the background.
export async function importFromCatalog(
  token: string,
  input: CatalogImportInput
): Promise<{ job_id?: string; message?: string; queued: boolean } & Record<string, unknown>> {
  const res = await fetch(`${API_URL}/admin/ingestion/import`, {
    method: "POST",
    headers: { ...authHeaders(token), "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    if (res.status === 409 && (err as { requires_confirmation?: boolean }).requires_confirmation) {
      throw new ImportConfirmationError(err as ImportConfirmationResponse);
    }
    throw new Error((err as { error?: string }).error || "Failed to import from catalog");
  }
  const json = (await res.json().catch(() => ({}))) as Record<string, unknown>;
  return { ...json, queued: res.status === 202 };
}

export interface BulkImportRequest {
  source: string;
  category_url: string;
  page_start: number;
  page_end: number;
  type?: string;
}

export interface BulkImportResponse {
  ok: boolean;
  job_id: string;
  message: string;
}

// Bulk import from category
export async function bulkImport(
  token: string,
  input: BulkImportRequest
): Promise<BulkImportResponse> {
  const res = await fetch(`${API_URL}/admin/ingestion/bulk-import`, {
    method: "POST",
    headers: authHeaders(token),
    body: JSON.stringify(input),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: "Failed to start bulk import" }));
    throw new Error(err.error || "Failed to start bulk import");
  }
  return res.json();
}

// Admin User types
export interface AdminUser {
  id: string;
  display_name: string;
  first_name?: string;
  last_name?: string;
  username: string;
  telegram_id: number;
  role: string;
  is_premium: boolean;
  is_premium_active: boolean;
  premium_expires_at?: string | null;
  wallet_balance?: number;
  auth_provider: string;
  created_at: string;
  last_login_at: string;
  is_banned?: boolean;
  ban_reason?: string;
  banned_until?: string | null;
  banned_by_username?: string;
}

export interface DashboardStats {
  users: {
    total: number;
    registered_today: number;
    registered_this_month: number;
    recent: AdminUser[];
  };
}

// Get admin dashboard stats including users
export async function getAdminDashboardStats(token: string): Promise<DashboardStats> {
  const res = await fetch(`${API_URL}/admin/dashboard/stats`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Failed to fetch dashboard stats");
  return res.json();
}

// Top content item for analytics
export interface TopContentItem {
  title: string;
  slug: string;
  views_count: number;
  poster_url?: string;
}

// Get top viewed movies for admin analytics
export async function getAdminTopMovies(token: string): Promise<{ data: TopContentItem[] }> {
  const res = await fetch(`${API_URL}/admin/analytics/top-movies`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Failed to fetch top movies");
  return res.json();
}

// Get top viewed series for admin analytics
export async function getAdminTopSeries(token: string): Promise<{ data: TopContentItem[] }> {
  const res = await fetch(`${API_URL}/admin/analytics/top-series`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Failed to fetch top series");
  return res.json();
}

// User metrics for admin analytics
export interface UserMetrics {
  total_users: number;
  premium_users: number;
  conversion_rate: number;
  total_views: number;
  movie_views: number;
  series_views: number;
}

// Get user metrics for admin dashboard
export async function getAdminUserMetrics(token: string): Promise<UserMetrics> {
  const res = await fetch(`${API_URL}/admin/analytics/users`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Failed to fetch user metrics");
  return res.json();
}

// Get paginated users list
export interface GetUsersParams {
  page?: number;
  limit?: number;
  search?: string;
  role?: string;
}

export async function getAdminUsers(
  token: string,
  params?: GetUsersParams
): Promise<{ data: AdminUser[]; total: number; page: number; limit: number; total_pages: number }> {
  const qs = new URLSearchParams();
  if (params?.page) qs.set("page", String(params.page));
  if (params?.limit) qs.set("limit", String(params.limit));
  if (params?.search) qs.set("search", params.search);
  if (params?.role) qs.set("role", params.role);

  const res = await fetch(`${API_URL}/admin/users?${qs}`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Failed to fetch users");
  return res.json();
}

// Update user role
export async function updateAdminUserRole(
  token: string,
  userId: string,
  role: string
): Promise<{ message: string; role: string; is_premium?: boolean }> {
  const res = await fetch(`${API_URL}/admin/v1/users/${userId}/role`, {
    method: "PATCH",
    headers: authHeaders(token),
    body: JSON.stringify({ role }),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to update user role");
  }
  return res.json();
}

// Update user premium status
export interface UpdateUserPremiumResponse {
  message: string;
  id: string;
  role: string;
  is_premium: boolean;
  premium_expires_at: string | null;
}

export async function updateAdminUserPremium(
  token: string,
  userId: string,
  isPremium: boolean,
  premiumExpiresAt?: string | null,
  durationDays?: number | null
): Promise<UpdateUserPremiumResponse> {
  const body: Record<string, unknown> = { is_premium: isPremium };
  
  if (durationDays && durationDays > 0) {
    body.duration_days = durationDays;
  } else if (premiumExpiresAt) {
    body.premium_expires_at = premiumExpiresAt;
  }

  const res = await fetch(`${API_URL}/admin/users/${userId}/premium`, {
    method: "PATCH",
    headers: authHeaders(token),
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || err.message || "Failed to update premium status");
  }
  return res.json();
}

// Buy premium using wallet balance
export async function buyPremium(
  token: string,
  planId: string
): Promise<{ success: boolean; message: string }> {
  const res = await fetch(`${API_URL}/user/premium/buy`, {
    method: "POST",
    headers: authHeaders(token),
    body: JSON.stringify({ plan_id: planId }),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to purchase premium");
  }
  return res.json();
}

// Update user wallet balance (superadmin only)
export async function updateUserWallet(
  token: string,
  userId: string,
  amount: number
): Promise<{ success: boolean; wallet_balance: number }> {
  const res = await fetch(`${API_URL}/superadmin/users/${userId}/wallet`, {
    method: "PATCH",
    headers: authHeaders(token),
    body: JSON.stringify({ amount }),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to update wallet balance");
  }
  return res.json();
}

// ── Collections API ────────────────────────────────────────────────

export interface CollectionMovie {
  id: string;
  code: string;
  title: string;
  slug: string;
  poster_url: string;
  year: number;
  genre: string[];
  genres: string[];
  duration: number;
  quality: string;
  rating_avg: number;
  rating_count: number;
  created_at: string;
}

export interface Collection {
  id: string;
  title: string;
  slug: string;
  description: string;
  poster_url: string;
  sort_order: number;
  movies: CollectionMovie[];
}

export interface CollectionInput {
  id?: string;
  title: string;
  slug: string;
  description?: string;
  poster_url?: string;
  is_published?: boolean;
  is_featured?: boolean;
  sort_order?: number;
  movie_ids?: string[];
}

// Get featured collections (public)
export async function getFeaturedCollections(): Promise<Collection[]> {
  const res = await fetch(`${API_URL}/collections/featured`, {
    next: { revalidate: 60 }, // ISR: public featured collections, 60s cache
  });
  if (!res.ok) throw new Error("Failed to fetch featured collections");
  const data = await res.json();
  return data.data || [];
}

// Get all published collections (public)
export async function getCollections(): Promise<Collection[]> {
  const res = await fetch(`${API_URL}/collections`, {
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Failed to fetch collections");
  const data = await res.json();
  return data.data || [];
}

// Get collection by slug (public)
export async function getCollectionBySlug(slug: string): Promise<Collection | null> {
  const res = await fetch(`${API_URL}/collections/slug/${slug}`, {
    cache: "no-store",
  });
  if (!res.ok) {
    if (res.status === 404) return null;
    throw new Error("Failed to fetch collection");
  }
  const data = await res.json();
  return data.data || null;
}

// Admin: Get all collections
export async function getAdminCollections(token: string): Promise<CollectionInput[]> {
  const res = await fetch(`${API_URL}/admin/collections`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Failed to fetch collections");
  const data = await res.json();
  return data.data || [];
}

// Admin: Get collection by ID
export async function getAdminCollectionById(
  token: string,
  id: string
): Promise<CollectionInput | null> {
  const res = await fetch(`${API_URL}/admin/collections/${id}`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) {
    if (res.status === 404) return null;
    throw new Error("Failed to fetch collection");
  }
  const data = await res.json();
  return data.data || null;
}

// Admin: Create collection
export async function createCollection(
  token: string,
  input: CollectionInput
): Promise<CollectionInput> {
  const res = await fetch(`${API_URL}/admin/collections`, {
    method: "POST",
    headers: authHeaders(token),
    body: JSON.stringify(input),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to create collection");
  }
  const data = await res.json();
  return data.data;
}

// Admin: Update collection
export async function updateCollection(
  token: string,
  id: string,
  input: CollectionInput
): Promise<CollectionInput> {
  const res = await fetch(`${API_URL}/admin/collections/${id}`, {
    method: "PUT",
    headers: authHeaders(token),
    body: JSON.stringify(input),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to update collection");
  }
  const data = await res.json();
  return data.data;
}

// Admin: Delete collection
export async function deleteCollection(token: string, id: string): Promise<void> {
  const res = await fetch(`${API_URL}/admin/collections/${id}`, {
    method: "DELETE",
    headers: authHeaders(token),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to delete collection");
  }
}

// User stats
export interface UserStats {
  comments_count: number;
  ratings_count: number;
  favorites_count: number;
  watched_count: number;
}

// Profile style for customization
export interface ProfileStyle {
  frame?: string;
  theme?: string;
  gradient?: string;
}

// Public user profile
export interface PublicUserProfile {
  id: string;
  display_name: string;
  username?: string;
  avatar_url?: string;
  role: string;
  is_premium: boolean;
  is_premium_active: boolean;
  premium_started_at?: string;
  premium_expires_at?: string;
  profile_style?: ProfileStyle;
  is_private?: boolean;
  created_at: string;
  last_login_at?: string;
  stats?: UserStats;
}

// Get public user profile.
// Pass `token` so the backend can identify the requester and allow owners/admins
// to view private profiles. Without a token, private profiles return 403.
export async function getPublicUser(userId: string, token?: string): Promise<PublicUserProfile> {
  const headers: Record<string, string> = {};
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }
  const res = await fetch(`${API_URL}/users/${userId}`, {
    cache: "no-store",
    headers,
  });
  if (!res.ok) {
    if (res.status === 404) {
      throw new Error("User not found");
    }
    if (res.status === 403) {
      throw new Error("Profile hidden");
    }
    throw new Error("Failed to fetch user");
  }
  const data = await res.json();
  return data.data;
}

// ========== SHARES ==========

export interface ShareResponse {
  share_code: string;
  share_url: string;
}

export interface MovieShareStats {
  shares_created_count: number;
  total_share_opens: number;
}

export interface UserShareStats {
  total_shares_created: number;
  total_share_opens: number;
  total_movies_shared: number;
  top_shared_movie?: {
    movie_id: string;
    title: string;
    share_count: number;
  };
}

export interface AdminShareStats {
  total_shares_created: number;
  total_share_opens: number;
  top_shared_movies: Array<{
    movie_id: string;
    title: string;
    shares_created_count: number;
    total_share_opens: number;
  }>;
  top_users_by_shares: Array<{
    user_id: string;
    display_name: string;
    shares_created_count: number;
  }>;
  recent_shares: Array<{
    id: string;
    code: string;
    movie_id: string;
    clicks: number;
    created_at: string;
    source: string;
  }>;
}

// Create a share for a movie
export async function createMovieShare(
  token: string | null,
  movieId: string
): Promise<ShareResponse> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  const res = await fetch(`${API_URL}/movies/share`, {
    method: "POST",
    headers,
    body: JSON.stringify({ movie_id: movieId }),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to create share");
  }
  return res.json();
}

// Record a share open event
export async function recordShareOpen(code: string): Promise<void> {
  const res = await fetch(`${API_URL}/shares/${code}/open`, {
    method: "POST",
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to record share open");
  }
}

// Get movie share stats
export async function getMovieShareStats(
  movieId: string
): Promise<MovieShareStats> {
  const res = await fetch(`${API_URL}/movies/share-stats?movie_id=${movieId}`);
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to get share stats");
  }
  return res.json();
}

// Get current user share stats
export async function getUserShareStats(
  token: string
): Promise<UserShareStats> {
  const res = await fetch(`${API_URL}/user/share-stats`, {
    headers: authHeaders(token),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to get share stats");
  }
  return res.json();
}

// Get admin share stats
export async function getAdminShareStats(
  token: string
): Promise<AdminShareStats> {
  const res = await fetch(`${API_URL}/admin/share-stats`, {
    headers: authHeaders(token),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to get share stats");
  }
  return res.json();
}

// Admin Series types
export interface AdminSeries {
  id: string;
  code?: string;
  slug: string;
  title: string;
  title_uz?: string;
  title_ru?: string;
  description: string;
  description_uz?: string;
  description_ru?: string;
  poster_url: string;
  backdrop_url: string;
  year: number;
  genre: string[];
  country: string;
  views: number;
  rating_avg: number;
  rating_count: number;
  is_premium: boolean;
  is_completed: boolean;
  quality?: string;
  created_at: string;
  updated_at: string;
  // Approval workflow
  approval_status?: "pending" | "approved" | "rejected";
  is_published?: boolean;
  approved_at?: string | null;
  approved_by?: string;
}

export interface CreateSeriesData {
  title: string;
  title_uz?: string;
  title_ru?: string;
  slug?: string;
  description?: string;
  description_uz?: string;
  description_ru?: string;
  poster_url?: string;
  backdrop_url?: string;
  year?: number;
  // Field name must match backend SeriesInput JSON tag "genre" (singular) —
  // sending "genres" caused admin edits to be silently dropped.
  genre?: string[];
  country?: string;
  is_premium?: boolean;
  quality?: string;
}

// Admin: Get all series (including pending/rejected — uses admin endpoint)
export async function adminGetSeries(token: string): Promise<AdminSeries[]> {
  const res = await fetch(`${API_URL}/admin/series?limit=500`, {
    headers: authHeaders(token),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to get series");
  }
  const data = await res.json();
  return data.data || [];
}

export async function approveSeries(token: string, id: string): Promise<void> {
  const res = await fetch(`${API_URL}/admin/series/${id}/approve`, {
    method: "PATCH",
    headers: authHeaders(token),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to approve series");
  }
}

export async function rejectSeries(token: string, id: string): Promise<void> {
  const res = await fetch(`${API_URL}/admin/series/${id}/reject`, {
    method: "PATCH",
    headers: authHeaders(token),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to reject series");
  }
}

// Admin: Create series
export async function adminCreateSeries(
  token: string,
  series: CreateSeriesData
): Promise<AdminSeries> {
  const headers = authHeaders(token);
  headers["Content-Type"] = "application/json";
  const res = await fetch(`${API_URL}/admin/series`, {
    method: "POST",
    headers,
    body: JSON.stringify(series),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to create series");
  }
  return res.json();
}

// Admin: Update series
export async function adminUpdateSeries(
  token: string,
  id: string,
  series: CreateSeriesData
): Promise<AdminSeries> {
  const headers = authHeaders(token);
  headers["Content-Type"] = "application/json";
  const res = await fetch(`${API_URL}/admin/series/${id}`, {
    method: "PUT",
    headers,
    body: JSON.stringify(series),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to update series");
  }
  return res.json();
}

// Admin: Delete series — cascade delete returns a structured summary so
// the admin UI can show what was removed and any partial B2 errors.
export async function adminDeleteSeries(
  token: string,
  id: string
): Promise<SeriesDeleteResponse> {
  const res = await fetch(`${API_URL}/admin/series/${id}`, {
    method: "DELETE",
    headers: authHeaders(token),
  });
  const json = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(json?.error || "Failed to delete series");
  }
  return json as SeriesDeleteResponse;
}

// Admin: Create season
export interface CreateSeasonData {
  season_number: number;
  title: string;
  poster_url?: string;
  description?: string;
  release_date?: string;
}

export interface UpdateSeasonData {
  title?: string;
  poster_url?: string;
  description?: string;
  release_date?: string;
}

export async function adminCreateSeason(
  token: string,
  seriesId: string,
  season: CreateSeasonData
): Promise<any> {
  const headers = authHeaders(token);
  headers["Content-Type"] = "application/json";
  const res = await fetch(`${API_URL}/admin/series/${seriesId}/seasons`, {
    method: "POST",
    headers,
    body: JSON.stringify(season),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to create season");
  }
  return res.json();
}

export async function adminUpdateSeason(
  token: string,
  seasonId: string,
  season: UpdateSeasonData
): Promise<any> {
  const headers = authHeaders(token);
  headers["Content-Type"] = "application/json";
  const res = await fetch(`${API_URL}/admin/seasons/${seasonId}`, {
    method: "PUT",
    headers,
    body: JSON.stringify(season),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to update season");
  }
  return res.json();
}

export async function adminDeleteSeason(
  token: string,
  seasonId: string
): Promise<void> {
  const res = await fetch(`${API_URL}/admin/seasons/${seasonId}`, {
    method: "DELETE",
    headers: authHeaders(token),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to delete season");
  }
}

// Admin: Create episode
export interface CreateEpisodeData {
  episode_number: number;
  title: string;
  description?: string;
  thumbnail_url?: string;
  video_url?: string;
  embed_url?: string;
  duration?: number;
  air_date?: string;
}

export async function adminCreateEpisode(
  token: string,
  seasonId: string,
  episode: CreateEpisodeData
): Promise<any> {
  const headers = authHeaders(token);
  headers["Content-Type"] = "application/json";
  const res = await fetch(`${API_URL}/admin/seasons/${seasonId}/episodes`, {
    method: "POST",
    headers,
    body: JSON.stringify(episode),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to create episode");
  }
  return res.json();
}

export async function adminDeleteEpisode(token: string, episodeId: string): Promise<void> {
  const headers = authHeaders(token);
  const res = await fetch(`${API_URL}/admin/episodes/${episodeId}`, {
    method: "DELETE",
    headers,
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to delete episode");
  }
}

// Admin: Update episode
export interface UpdateEpisodeData {
  season_id?: string;
  episode_number?: number;
  title?: string;
  description?: string;
  thumbnail_url?: string;
  video_url?: string;
  embed_url?: string;
  duration?: number;
  air_date?: string;
}

export async function adminUpdateEpisode(
  token: string,
  episodeId: string,
  data: UpdateEpisodeData
): Promise<any> {
  const headers = authHeaders(token);
  headers["Content-Type"] = "application/json";
  const res = await fetch(`${API_URL}/admin/episodes/${episodeId}`, {
    method: "PUT",
    headers,
    body: JSON.stringify(data),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to update episode");
  }
  return res.json();
}

// Admin: Reorder episodes in a season
export async function adminReorderEpisodes(
  token: string,
  seasonId: string,
  episodeIds: string[]
): Promise<void> {
  const headers = authHeaders(token);
  headers["Content-Type"] = "application/json";
  const res = await fetch(`${API_URL}/admin/seasons/${seasonId}/episodes/reorder`, {
    method: "POST",
    headers,
    body: JSON.stringify({ episode_ids: episodeIds }),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to reorder episodes");
  }
}

// Admin: Move episode to another season
export async function adminMoveEpisodeToSeason(
  token: string,
  episodeId: string,
  seasonId: string,
  episodeNumber: number
): Promise<void> {
  const headers = authHeaders(token);
  headers["Content-Type"] = "application/json";
  const res = await fetch(`${API_URL}/admin/episodes/${episodeId}/move`, {
    method: "POST",
    headers,
    body: JSON.stringify({ season_id: seasonId, episode_number: episodeNumber }),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to move episode");
  }
}

// Ban a user
export async function banUser(
  token: string,
  userId: string,
  durationDays: number,
  reason: string
): Promise<void> {
  const headers = authHeaders(token);
  headers["Content-Type"] = "application/json";
  const res = await fetch(`${API_URL}/admin/users/${userId}/ban`, {
    method: "POST",
    headers,
    body: JSON.stringify({ duration_days: durationDays, reason }),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to ban user");
  }
}

// Unban a user
export async function unbanUser(token: string, userId: string): Promise<void> {
  const headers = authHeaders(token);
  const res = await fetch(`${API_URL}/admin/users/${userId}/ban`, {
    method: "DELETE",
    headers,
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to unban user");
  }
}

// Banned user type
export interface BannedUser {
  id: string;
  display_name: string;
  username: string;
  telegram_id: number;
  role: string;
  auth_provider: string;
  is_premium: boolean;
  created_at: string;
  is_banned: boolean;
  ban_reason: string;
  banned_at: string;
  banned_until: string | null;
  banned_by_username: string;
  ban_status: "active" | "expired" | "permanent";
}

// Get banned users
export async function getBannedUsers(
  token: string,
  params?: { search?: string; status?: "all" | "active" | "expired" | "permanent" }
): Promise<{ data: BannedUser[]; total: number }> {
  const qs = new URLSearchParams();
  if (params?.search) qs.set("search", params.search);
  if (params?.status && params.status !== "all") qs.set("status", params.status);
  
  const res = await fetch(`${API_URL}/admin/users/banned?${qs.toString()}`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to get banned users");
  }
  return res.json();
}

// Ban history type
export interface BanHistoryRecord {
  id: string;
  user_id: string;
  user_display_name: string;
  user_username: string;
  user_telegram_id: number;
  reason: string;
  banned_at: string;
  banned_until: string | null;
  is_permanent: boolean;
  banned_by_username: string;
  unbanned_at: string | null;
  unbanned_by_username: string | null;
  status: "active" | "unbanned" | "expired";
}

// Get ban history
export async function getBanHistory(
  token: string,
  params?: { search?: string; status?: "all" | "active" | "unbanned" | "expired" }
): Promise<{ data: BanHistoryRecord[]; total: number }> {
  const qs = new URLSearchParams();
  if (params?.search) qs.set("search", params.search);
  if (params?.status && params.status !== "all") qs.set("status", params.status);
  
  const res = await fetch(`${API_URL}/admin/users/ban-history?${qs.toString()}`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to get ban history");
  }
  return res.json();
}

// Ban Appeal types
export interface BanAppeal {
  id: string;
  user_id: string;
  username: string;
  telegram_id: string;
  message: string;
  status: "pending" | "approved" | "rejected";
  admin_note?: string;
  created_at: string;
  reviewed_at?: string;
  reviewed_by_username?: string;
  ban_reason?: string;
  ban_banned_at?: string;
  ban_banned_until?: string;
  ban_banned_by_name?: string;
}

// Create appeal request
export interface CreateAppealRequest {
  message: string;
}

// Review appeal request
export interface ReviewAppealRequest {
  action: "approve" | "reject";
  admin_note?: string;
  unban_user?: boolean;
}

// Get my appeals
export async function getMyAppeals(token: string): Promise<{ appeals: BanAppeal[]; total: number }> {
  const res = await fetch(`${API_URL}/appeals/me`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to get appeals");
  }
  return res.json();
}

// Create appeal
export async function createAppeal(token: string, data: CreateAppealRequest): Promise<{ message: string; appeal: BanAppeal }> {
  const res = await fetch(`${API_URL}/appeals`, {
    method: "POST",
    headers: { ...authHeaders(token), "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to create appeal");
  }
  return res.json();
}

// Get appeals (admin)
export async function getAppeals(
  token: string,
  params?: { page?: number; per_page?: number; status?: string; search?: string }
): Promise<{ appeals: BanAppeal[]; total: number; page: number; per_page: number; total_pages: number }> {
  const qs = new URLSearchParams();
  if (params?.page) qs.set("page", params.page.toString());
  if (params?.per_page) qs.set("per_page", params.per_page.toString());
  if (params?.status && params.status !== "all") qs.set("status", params.status);
  if (params?.search) qs.set("search", params.search);
  
  const res = await fetch(`${API_URL}/admin/appeals?${qs.toString()}`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to get appeals");
  }
  return res.json();
}

// Get appeal stats (admin)
export async function getAppealStats(token: string): Promise<{ stats: { pending: number; approved: number; rejected: number; total: number } }> {
  const res = await fetch(`${API_URL}/admin/appeals/stats`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to get appeal stats");
  }
  return res.json();
}

// Review appeal (admin)
export async function reviewAppeal(token: string, appealId: string, data: ReviewAppealRequest): Promise<{ message: string; action: string; unbanned: boolean }> {
  const res = await fetch(`${API_URL}/admin/appeals/${appealId}/review`, {
    method: "POST",
    headers: { ...authHeaders(token), "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to review appeal");
  }
  return res.json();
}

// Get pending appeal count (admin)
export async function getPendingAppealCount(token: string): Promise<{ count: number }> {
  const res = await fetch(`${API_URL}/admin/appeals/pending-count`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to get pending count");
  }
  return res.json();
}

// Notification types
export interface Notification {
  id: string;
  user_id: string;
  type: "PREMIUM_ACTIVATED" | "PREMIUM_EXPIRING_SOON" | "PREMIUM_EXPIRED" | "BAN_APPLIED" | "BAN_REMOVED" | "APPEAL_SUBMITTED" | "APPEAL_APPROVED" | "APPEAL_REJECTED" | "COMMENT_REPLY";
  title: string;
  message: string;
  is_read: boolean;
  data?: Record<string, any>;
  action_url?: string;
  created_at: string;
  read_at?: string;
}

export interface AdsBatchResponse {
  placements: Record<string, Ad[]>;
}

// Get notifications
export async function getNotifications(
  token: string,
  params?: { page?: number; per_page?: number }
): Promise<{ notifications: Notification[]; total: number; unread_count: number }> {
  const qs = new URLSearchParams();
  if (params?.page) qs.set("page", params.page.toString());
  if (params?.per_page) qs.set("per_page", params.per_page.toString());
  
  const res = await fetch(`${API_URL}/notifications?${qs.toString()}`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to get notifications");
  }
  return res.json();
}

// Get unread notification count
export async function getUnreadNotificationCount(token: string): Promise<{ count: number }> {
  const res = await fetch(`${API_URL}/notifications/unread-count`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to get unread count");
  }
  return res.json();
}

// Mark notification as read
export async function markNotificationAsRead(token: string, notificationId: string): Promise<{ message: string }> {
  const res = await fetch(`${API_URL}/notifications/${notificationId}/read`, {
    method: "PATCH",
    headers: authHeaders(token),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to mark notification as read");
  }
  return res.json();
}

// Mark all notifications as read
export async function markAllNotificationsAsRead(token: string): Promise<{ message: string }> {
  const res = await fetch(`${API_URL}/notifications/read-all`, {
    method: "PATCH",
    headers: authHeaders(token),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to mark all notifications as read");
  }
  return res.json();
}

// ── Clips API ────────────────────────────────────────────────────────────────

export interface Clip {
  id: string;
  movie_id: string;
  movie_title: string;
  movie_slug: string;
  movie_code: string;
  filename: string;
  path: string;
  url: string;
  duration: number;
  sequence: number;
  storage_type: string;
  created_at: string;
}

export async function adminGetClips(token: string, limit = 100): Promise<Clip[]> {
  const res = await fetch(`${API_URL}/admin/clips?limit=${limit}`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Failed to fetch clips");
  const json = await res.json();
  return json.data || [];
}

export async function adminGetClipsByMovie(token: string, movieId: string): Promise<Clip[]> {
  const res = await fetch(`${API_URL}/admin/clips/movie/${movieId}`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Failed to fetch clips");
  const json = await res.json();
  return json.data || [];
}

export async function adminDeleteClipsByMovie(token: string, movieId: string): Promise<void> {
  const res = await fetch(`${API_URL}/admin/clips/movie/${movieId}`, {
    method: "DELETE",
    headers: authHeaders(token),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to delete clips");
  }
}

// ── Ads API ────────────────────────────────────────────────────────────────

export type AdStatus = "draft" | "active" | "paused" | "expired";

export interface Ad {
  id: string;
  title: string;
  description?: string;
  image_url?: string;
  video_url?: string;
  target_url: string;
  call_to_action?: string;
  placements: string[];
  status: AdStatus;
  starts_at?: string;
  ends_at?: string;
  duration_days?: number;
  price: number;
  priority: number;
  impressions: number;
  clicks: number;
  created_by: string;
  created_at: string;
  updated_at: string;
  // Website creatives (structured)
  banner_media_url?: string;
  banner_media_type?: "image" | "video";
  inline_media_url?: string;
  inline_media_type?: "image" | "video";
  fixed_bottom_media_url?: string;
  fixed_bottom_media_type?: "image" | "video";
  popup_media_url?: string;
  popup_media_type?: "image" | "video";
  player_overlay_media_url?: string;
  player_overlay_media_type?: "image" | "video";
  // Telegram shared media
  telegram_media_url?: string;
  telegram_media_type?: "image" | "video";
  // Phase 2
  telegram_channels?: string[];
  telegram_bot_enabled?: boolean;
  telegram_bot_chat_ids?: number[];
  telegram_channel_enabled?: boolean;
  player_enabled?: boolean;
  telegram_deliveries?: number;
  telegram_last_sent_at?: string;
}

export interface AdDelivery {
  id: string;
  ad_id: string;
  placement: string;
  target: string;
  status: "success" | "failed";
  message_id?: number;
  sent_at: string;
  error?: string;
}

export interface AdStats {
  total_ads: number;
  active_ads: number;
  expired_ads: number;
  impressions: number;
  clicks: number;
  revenue: number;
  telegram_deliveries?: number;
  telegram_failed?: number;
}

export interface AdInput {
  title: string;
  description?: string;
  image_url?: string;
  video_url?: string;
  target_url: string;
  call_to_action?: string;
  placements: string[];
  status: AdStatus;
  duration_days: number;
  price: number;
  priority: number;
  // Website creatives (structured)
  banner_media_url?: string;
  banner_media_type?: "image" | "video";
  inline_media_url?: string;
  inline_media_type?: "image" | "video";
  fixed_bottom_media_url?: string;
  fixed_bottom_media_type?: "image" | "video";
  popup_media_url?: string;
  popup_media_type?: "image" | "video";
  player_overlay_media_url?: string;
  player_overlay_media_type?: "image" | "video";
  // Telegram shared media
  telegram_media_url?: string;
  telegram_media_type?: "image" | "video";
  // Phase 2
  telegram_channels?: string[];
  telegram_bot_enabled?: boolean;
  telegram_bot_chat_ids?: number[];
  telegram_channel_enabled?: boolean;
  player_enabled?: boolean;
}

export async function adminListAds(token: string): Promise<Ad[]> {
  const res = await fetch(`${API_URL}/superadmin/ads`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Failed to fetch ads");
  const json = await res.json();
  return json.ads || [];
}

export async function adminGetAdStats(token: string): Promise<AdStats> {
  const res = await fetch(`${API_URL}/superadmin/ads/stats`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Failed to fetch ad stats");
  return res.json();
}

export async function adminCreateAd(token: string, input: AdInput): Promise<Ad> {
  const res = await fetch(`${API_URL}/superadmin/ads`, {
    method: "POST",
    headers: { ...authHeaders(token), "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to create ad");
  }
  return res.json();
}

export async function adminUpdateAd(token: string, id: string, input: Partial<AdInput>): Promise<Ad> {
  const res = await fetch(`${API_URL}/superadmin/ads/${id}`, {
    method: "PUT",
    headers: { ...authHeaders(token), "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to update ad");
  }
  return res.json();
}

export async function adminDeleteAd(token: string, id: string): Promise<void> {
  const res = await fetch(`${API_URL}/superadmin/ads/${id}`, {
    method: "DELETE",
    headers: authHeaders(token),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Failed to delete ad");
  }
}

export async function getAdsByPlacement(placement: string): Promise<Ad[]> {
  const res = await fetch(`${API_URL}/ads?placement=${placement}`, {
    cache: "no-store",
  });
  if (!res.ok) return [];
  const json = await res.json();
  return json.ads || [];
}

export async function getAdsBatch(placements: string[]): Promise<AdsBatchResponse> {
  const uniquePlacements = Array.from(new Set(placements.filter(Boolean))).sort();
  if (uniquePlacements.length === 0) {
    return { placements: {} };
  }

  return dedupeBrowserRequest(`ads-batch:${uniquePlacements.join(",")}`, 30000, async () => {
    const qs = new URLSearchParams({ placements: uniquePlacements.join(",") });
    const res = await fetch(`${API_URL}/ads/batch?${qs.toString()}`, {
      cache: "no-store",
    });
    if (!res.ok) {
      return { placements: {} };
    }
    const json = await res.json();
    return { placements: json.placements || {} };
  });
}

export async function getAdsForWebsite(placement: string): Promise<Ad[]> {
  const placements = [placement, "website"];
  const data = await getAdsBatch(placements);
  const seen = new Set<string>();
  const merged: Ad[] = [];
  placements.forEach((key) => {
    (data.placements[key] || []).forEach((ad) => {
      if (seen.has(ad.id)) return;
      seen.add(ad.id);
      merged.push(ad);
    });
  });
  return merged;
}

export async function recordAdImpression(id: string): Promise<void> {
  await fetch(`${API_URL}/ads/${id}/impression`, { method: "POST" });
}

export async function recordAdClick(id: string): Promise<void> {
  await fetch(`${API_URL}/ads/${id}/click`, { method: "POST" });
}

export async function adminSendTelegramAd(token: string, id: string): Promise<{ results: { target: string; placement: string; status: string; error?: string }[] }> {
  const res = await fetch(`${API_URL}/superadmin/ads/${id}/send-telegram`, {
    method: "POST",
    headers: authHeaders(token),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.error || "Failed to send telegram ad");
  }
  return res.json();
}

export async function adminGetAdDelivery(token: string, id: string): Promise<AdDelivery[]> {
  const res = await fetch(`${API_URL}/superadmin/ads/${id}/delivery`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) return [];
  const json = await res.json();
  return json.deliveries || [];
}

export async function uploadAdMedia(
  token: string,
  file: File,
  mediaType: "image" | "video"
): Promise<string> {
  const body = new FormData();
  body.append("file", file);
  body.append("media_type", mediaType);
  // Do NOT set Content-Type — browser must set multipart/form-data with boundary automatically
  const res = await fetch(`${API_URL}/admin/upload/ad-media`, {
    method: "POST",
    headers: { Authorization: `Bearer ${token}` },
    body,
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.error || "Upload failed");
  }
  const json = await res.json();
  return json.url as string;
}

// ── Telegram Post API ──────────────────────────────────────────────────────────

export interface TelegramPostRequest {
  text: string;
  image_url?: string;
  send_to_channels: boolean;
  send_to_bot: boolean;
  inline_button_text?: string;
  inline_button_url?: string;
}

export interface TelegramPostResult {
  channels_sent: number;
  channels_blocked: number;
  channels_failed: number;
  bot_sent: number;
  bot_blocked: number;
  bot_failed: number;
  errors?: string[];
}

export interface TelegramPost {
  id: string;
  text: string;
  image_url?: string;
  send_to_channels: boolean;
  send_to_bot_users: boolean;
  inline_button_text?: string;
  inline_button_url?: string;
  channels_sent_count: number;
  channels_blocked_count: number;
  channels_failed_count: number;
  bot_sent_count: number;
  bot_blocked_count: number;
  bot_failed_count: number;
  sent_by_user_id: string;
  sent_by_name: string;
  sent_at: string;
  created_at: string;
}

export async function listTelegramPosts(token: string): Promise<TelegramPost[]> {
  const res = await fetch(`${API_URL}/superadmin/telegram-posts`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Failed to fetch telegram posts");
  const json = await res.json();
  return json.posts || [];
}

export async function sendTelegramPost(
  token: string,
  data: TelegramPostRequest
): Promise<TelegramPostResult> {
  const res = await fetch(`${API_URL}/superadmin/telegram-post`, {
    method: "POST",
    headers: { ...authHeaders(token), "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.error || "Failed to send telegram post");
  }
  return res.json();
}

export async function uploadTelegramPostMedia(
  token: string,
  file: File
): Promise<string> {
  const body = new FormData();
  body.append("file", file);
  const res = await fetch(`${API_URL}/admin/upload/telegram-post-media`, {
    method: "POST",
    headers: { Authorization: `Bearer ${token}` },
    body,
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.error || "Upload failed");
  }
  const json = await res.json();
  return json.url as string;
}

// ── Suggestion API ──────────────────────────────────────────────────────────

export interface Suggestion {
  id: string;
  user_id: string;
  user_name: string;
  user?: {
    _id: string;
    username?: string;
    full_name?: string;
    telegram_username?: string;
  };
  type: "movie" | "series";
  title: string;
  message: string;
  source_url?: string;
  image_url?: string;
  image_storage_key?: string;
  image_mime_type?: string;
  image_size?: number;
  status: "pending" | "accepted" | "rejected";
  admin_message?: string;
  reviewed_by?: string;
  reviewed_at?: string;
  created_at: string;
  updated_at: string;
}

export interface SuggestionInput {
  type: "movie" | "series";
  title: string;
  message: string;
  source_url?: string;
}

export interface SuggestionFormData {
  type: "movie" | "series";
  title: string;
  message: string;
  source_url?: string;
  image?: File;
}

export interface SuggestionUpdateInput {
  status: "accepted" | "rejected";
  admin_message?: string;
}

export interface SuggestionListResponse {
  suggestions: Suggestion[];
  total: number;
  page: number;
  limit: number;
}

export async function createSuggestion(
  token: string,
  data: SuggestionFormData
): Promise<{ message: string; suggestion: Suggestion }> {
  const formData = new FormData();
  formData.append("type", data.type);
  formData.append("title", data.title);
  formData.append("message", data.message);
  if (data.source_url) {
    formData.append("source_url", data.source_url);
  }
  if (data.image) {
    formData.append("image", data.image);
  }

  const res = await fetch(`${API_URL}/suggestions`, {
    method: "POST",
    headers: {
      "Authorization": `Bearer ${token}`,
    },
    body: formData,
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.error || "Tavsiya yuborishda xatolik");
  }
  return res.json();
}

export async function getMySuggestions(
  token: string,
  page = 1,
  limit = 10
): Promise<SuggestionListResponse> {
  const qs = new URLSearchParams({
    page: String(page),
    limit: String(limit),
  });
  const res = await fetch(`${API_URL}/suggestions?${qs}`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Tavsiyalarni olishda xatolik");
  return res.json();
}

export async function adminListSuggestions(
  token: string,
  page = 1,
  limit = 20,
  status?: "pending" | "accepted" | "rejected" | "all"
): Promise<SuggestionListResponse> {
  const qs = new URLSearchParams({
    page: String(page),
    limit: String(limit),
  });
  if (status && status !== "all") {
    qs.set("status", status);
  }
  const res = await fetch(`${API_URL}/admin/suggestions?${qs}`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Tavsiyalarni olishda xatolik");
  return res.json();
}

export async function adminUpdateSuggestion(
  token: string,
  id: string,
  input: SuggestionUpdateInput
): Promise<{ message: string; suggestion: Suggestion }> {
  const res = await fetch(`${API_URL}/admin/suggestions/${id}`, {
    method: "PATCH",
    headers: { ...authHeaders(token), "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.error || "Tavsiyani yangilashda xatolik");
  }
  return res.json();
}

export async function adminGetSuggestionStats(
  token: string
): Promise<{ total: number; pending: number; accepted: number; rejected: number }> {
  const res = await fetch(`${API_URL}/admin/suggestions/stats`, {
    headers: authHeaders(token),
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Statistikani olishda xatolik");
  return res.json();
}

// ── Watch rooms (synchronized co-viewing) ─────────────────────────

export type RoomVisibility = "public" | "private";

export interface WatchRoom {
  id: string;
  owner_id: string;
  owner_name?: string;
  owner_avatar?: string;
  owner_is_premium?: boolean;
  content_type: "movie" | "episode";
  content_id: string;
  content_title?: string;
  content_poster?: string;
  content_slug?: string;
  series_id?: string;
  season_id?: string;
  visibility: RoomVisibility;
  max_members: number;
  position_seconds: number;
  is_playing: boolean;
  last_state_update: string;
  status: "active" | "closed";
  created_at: string;
  expires_at: string;
}

export interface WatchRoomInvite {
  id: string;
  room_id: string;
  code: string;
  target_user_id?: string;
  max_uses: number;
  uses: number;
  expires_at: string;
  created_at: string;
}

export interface WatchRoomMessage {
  id: string;
  room_id: string;
  user_id: string;
  user_name?: string;
  user_avatar?: string;
  kind: "text" | "emoji";
  text?: string;
  emoji?: string;
  created_at: string;
}

export async function createWatchRoom(
  token: string,
  input: { content_type: "movie" | "episode"; content_id: string; visibility?: RoomVisibility },
): Promise<WatchRoom> {
  const res = await fetch(`${API_URL}/rooms`, {
    method: "POST",
    headers: { ...authHeaders(token), "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error((err as { error?: string }).error || "Failed to create room");
  }
  return res.json();
}

export async function getWatchRoom(id: string): Promise<WatchRoom> {
  const res = await fetch(`${API_URL}/rooms/${id}`);
  if (!res.ok) throw new Error("Room not found");
  return res.json();
}

export async function listPublicRooms(): Promise<{ items: WatchRoom[] }> {
  const res = await fetch(`${API_URL}/rooms`);
  if (!res.ok) throw new Error("Failed to list rooms");
  return res.json();
}

export async function createRoomInvite(
  token: string,
  roomID: string,
  opts: { target_user_id?: string; max_uses?: number } = {},
): Promise<WatchRoomInvite> {
  const res = await fetch(`${API_URL}/rooms/${roomID}/invites`, {
    method: "POST",
    headers: { ...authHeaders(token), "Content-Type": "application/json" },
    body: JSON.stringify(opts),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error((err as { error?: string }).error || "Failed to create invite");
  }
  return res.json();
}

export async function listRoomMessages(roomID: string): Promise<{ items: WatchRoomMessage[] }> {
  const res = await fetch(`${API_URL}/rooms/${roomID}/messages`);
  if (!res.ok) throw new Error("Failed to list messages");
  return res.json();
}

export function getRoomWebSocketURL(roomID: string, token: string, inviteCode?: string): string {
  const base = (API_URL || "").replace(/\/api\/?$/, "").replace(/^http/, "ws");
  const params = new URLSearchParams({ token });
  if (inviteCode) params.set("invite", inviteCode);
  return `${base}/ws/rooms/${roomID}?${params.toString()}`;
}
