"use client";

import { useState, useEffect, useRef, useCallback } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { ChevronLeft, ChevronRight, Clock, Calendar, Heart, Eye, Crown } from "lucide-react";
import VideoPlayer from "@/components/VideoPlayer";
import { recordView, recordWatchHistory, addFavorite, removeFavorite, checkIsFavorite, getRecommendations, saveWatchProgress, markWatchComplete, getAdsForWebsite, recordAdImpression, recordAdClick, getProtectedMediaAccess, Ad, Movie } from "@/lib/api";
import { pickWeightedRandomAd } from "@/lib/ads-utils";
import WebsiteAdSlot from "@/components/ads/WebsiteAdSlot";
import { useAuth } from "@/lib/auth-context";
import { useI18n } from "@/lib/i18n";
import { logger } from "@/lib/logger";
import { isMoviePremium, isUserPremium, PremiumLockOverlay, PremiumButton } from "@/components/PremiumComponents";
import { getLocalizedTitle, getLocalizedDescription, getLocalizedGenres, getLocalizedCountry } from "@/lib/localization";
import { formatDuration } from "@/lib/movie-utils";
import type { EpisodeLink } from "@/lib/series-api";
import { DEFAULT_POSTER_PLACEHOLDER, normalizeMediaUrl } from "@/lib/image-utils";
import MediaImage from "@/components/ui/MediaImage";

const PLAYER_AD_MANDATORY_SECS = 15;
const PLAYER_AD_REPEAT_MS = 10 * 60 * 1000; // 10 minutes
const MEDIA_ACCESS_MODE =
  (process.env.NEXT_PUBLIC_MEDIA_ACCESS_MODE || "").trim().toLowerCase() || "protected";
const CDN_BASE_URL =
  (process.env.NEXT_PUBLIC_CDN_BASE_URL || "").trim().replace(/\/+$/, "") ||
  "https://cdn.filmorauz.net/file/filmorauznet";

function resolvePublicPlaybackUrl(movie: Movie): string {
  const raw =
    movie.master_playlist_url ||
    movie.video_url ||
    movie.embed_url ||
    "";
  const value = raw.trim();
  if (!value) return "";
  if (value.startsWith("http://") || value.startsWith("https://")) {
    return value;
  }
  if (value.startsWith("videos/")) {
    return `${CDN_BASE_URL}/${value}`;
  }
  return normalizeMediaUrl(value, "");
}

// Player ad — full-overlay interrupt, 15s mandatory countdown
// starts only after user clicks play (started=true), calls onFirstComplete when first dismissed
function PlayerOverlayAd({
  started,
  onFirstComplete,
}: {
  started: boolean;
  onFirstComplete: () => void;
}) {
  const [currentAd, setCurrentAd] = useState<Ad | null>(null);
  const [visible, setVisible] = useState(false);
  const [countdown, setCountdown] = useState(PLAYER_AD_MANDATORY_SECS);
  const [canClose, setCanClose] = useState(false);
  const [adsLoaded, setAdsLoaded] = useState(false);

  const adsRef = useRef<Ad[]>([]);
  const currentAdIdRef = useRef<string | undefined>(undefined);
  const countdownTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const repeatTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const firstCompleteRef = useRef(false);
  const onFirstCompleteRef = useRef(onFirstComplete);
  onFirstCompleteRef.current = onFirstComplete;

  const startCountdown = useCallback(() => {
    setCountdown(PLAYER_AD_MANDATORY_SECS);
    setCanClose(false);
    if (countdownTimerRef.current) clearInterval(countdownTimerRef.current);
    countdownTimerRef.current = setInterval(() => {
      setCountdown((prev) => {
        if (prev <= 1) {
          clearInterval(countdownTimerRef.current!);
          countdownTimerRef.current = null;
          setCanClose(true);
          return 0;
        }
        return prev - 1;
      });
    }, 1000);
  }, []);

  const showAd = useCallback(() => {
    const list = adsRef.current;
    if (list.length === 0) return;
    const picked = pickWeightedRandomAd(list, currentAdIdRef.current);
    currentAdIdRef.current = picked.id;
    setCurrentAd(picked);
    setVisible(true);
    recordAdImpression(picked.id).catch(() => {});
    startCountdown();
  }, [startCountdown]);

  const dismiss = useCallback(() => {
    setVisible(false);
    if (countdownTimerRef.current) {
      clearInterval(countdownTimerRef.current);
      countdownTimerRef.current = null;
    }
    if (!firstCompleteRef.current) {
      firstCompleteRef.current = true;
      onFirstCompleteRef.current();
    }
    if (repeatTimerRef.current) clearTimeout(repeatTimerRef.current);
    repeatTimerRef.current = setTimeout(showAd, PLAYER_AD_REPEAT_MS);
  }, [showAd]);

  // Load ads on mount; set adsLoaded when ready so showAd can fire regardless of fetch vs play order
  useEffect(() => {
    let cancelled = false;
    getAdsForWebsite("watch_player_overlay")
      .then((data) => {
        if (cancelled) return;
        const valid = data.filter(
          (a) => a.player_overlay_media_url || a.banner_media_url || a.image_url
        );
        adsRef.current = valid;
        if (valid.length > 0) {
          setAdsLoaded(true);
        } else if (!firstCompleteRef.current) {
          firstCompleteRef.current = true;
          onFirstCompleteRef.current();
        }
      })
      .catch(() => {
        if (!firstCompleteRef.current) {
          firstCompleteRef.current = true;
          onFirstCompleteRef.current();
        }
      });
    return () => {
      cancelled = true;
      if (countdownTimerRef.current) clearInterval(countdownTimerRef.current);
      if (repeatTimerRef.current) clearTimeout(repeatTimerRef.current);
    };
  }, []);

  // Show first ad once both conditions are true: user started playback AND ads are fetched.
  // Using adsLoaded as state (not just a ref) ensures this effect re-runs whichever condition
  // becomes true second — fixing the race where user clicks play before fetch completes.
  useEffect(() => {
    if (started && adsLoaded && !firstCompleteRef.current) {
      showAd();
    }
  }, [started, adsLoaded, showAd]);

  if (!visible || !currentAd) return null;

  const url = currentAd.player_overlay_media_url || currentAd.banner_media_url || currentAd.image_url || "";
  const isVideo =
    currentAd.player_overlay_media_type === "video" ||
    currentAd.banner_media_type === "video" ||
    url.endsWith(".mp4") ||
    url.endsWith(".webm");

  const handleClick = () => {
    recordAdClick(currentAd.id).catch(() => {});
    window.open(currentAd.target_url, "_blank", "noopener,noreferrer");
  };

  return (
    <div className="absolute inset-0 z-20 bg-black overflow-hidden">
      <div className="relative w-full h-full cursor-pointer" onClick={handleClick}>
        {isVideo ? (
          <video
            src={normalizeMediaUrl(url, "")}
            className="absolute inset-0 w-full h-full object-contain"
            autoPlay
            muted
            loop
            playsInline
          />
        ) : (
          <img src={normalizeMediaUrl(url)} alt="Ad" className="absolute inset-0 w-full h-full object-contain" />
        )}
        <div className="absolute top-3 right-3">
          {canClose ? (
            <button
              onClick={(e) => { e.stopPropagation(); dismiss(); }}
              className="flex items-center justify-center rounded-full bg-black/70 text-white text-sm px-3 py-1.5 hover:bg-black/90 transition font-medium"
              aria-label="Close ad"
            >
              ✕ Yopish
            </button>
          ) : (
            <span className="text-xs text-white bg-black/70 px-3 py-1.5 rounded-full whitespace-nowrap pointer-events-none font-medium">
              Reklamani yopish uchun {countdown} soniya qoldi
            </span>
          )}
        </div>
        <span className="absolute top-3 left-3 text-[10px] text-gray-300 bg-black/60 px-2 py-1 rounded uppercase tracking-wide pointer-events-none">Reklama</span>
      </div>
    </div>
  );
}

// Fetch watch progress for resume
async function getWatchProgressForResume(token: string, targetId: string, targetType: "movie" | "episode"): Promise<number> {
  try {
    const res = await fetch(`${process.env.NEXT_PUBLIC_API_URL || ''}/user/history`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (!res.ok) return 0;
    const json = await res.json();
    const history = json.data || [];
    const item = history.find((h: any) => {
      const historyTargetId = h.target_id || h.movie_id;
      const historyTargetType = h.target_type || h.type || "movie";
      return historyTargetId === targetId && historyTargetType === targetType;
    });
    if (item && item.last_position_sec > 30 && item.last_position_sec < (item.duration_sec || 0) - 60) {
      return item.last_position_sec;
    }
  } catch {}
  return 0;
}

interface WatchPageClientProps {
  movie: Movie | null;
  episodeNavigation?: {
    previousEpisode?: EpisodeLink | null;
    nextEpisode?: EpisodeLink | null;
  };
}

function RecommendationsRow({ movies }: { movies: Movie[] }) {
  if (movies.length === 0) return null;

  return (
    <div className="mt-12">
      <h3 className="font-display text-xl sm:text-2xl tracking-wide text-white mb-6">
        Sizga yoqishi mumkin
      </h3>
      <div className="flex gap-3 overflow-x-auto pb-2">
        {movies.slice(0, 8).map((movie) => {
          const posterSrc = movie.poster_url;
          return (
          <a
            key={movie.id}
            href={`/movies/${movie.slug}`}
            className="shrink-0 w-[120px] sm:w-[140px] group block"
          >
            <div className="relative aspect-[2/3] overflow-hidden rounded-lg bg-brand-border">
              <MediaImage
                src={posterSrc}
                alt={getLocalizedTitle(movie)}
                fallbackSrc={DEFAULT_POSTER_PLACEHOLDER}
                className="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
              />
              {movie.quality && (
                <span className="absolute top-1 right-1 bg-brand-red text-white text-xs font-bold px-1.5 py-0.5 rounded">
                  {movie.quality}
                </span>
              )}
            </div>
            <p className="text-white text-xs mt-2 line-clamp-2 group-hover:text-brand-red transition-colors">
              {getLocalizedTitle(movie)}
            </p>
          </a>
          );
        })}
      </div>
    </div>
  );
}

export default function WatchPageClient({ movie, episodeNavigation }: WatchPageClientProps) {
  if (!movie) {
    return (
      <div className="flex items-center justify-center min-h-[50vh]">
        <p className="text-gray-400">Film topilmadi</p>
      </div>
    );
  }

  const { t } = useI18n();
  const { user, token } = useAuth();
  const router = useRouter();
  const [isFavorite, setIsFavorite] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [recommendations, setRecommendations] = useState<Movie[]>([]);
  const [isLoadingRecommendations, setIsLoadingRecommendations] = useState(true);
  const [resumePosition, setResumePosition] = useState(0);
  const [viewCount, setViewCount] = useState(movie.views ?? 0);
  const [resolvedPlaybackUrl, setResolvedPlaybackUrl] = useState<string | null>(null);
  const [playbackAccessError, setPlaybackAccessError] = useState<string | null>(null);
  const [autoNextCountdown, setAutoNextCountdown] = useState(5);
  const [showAutoNextCountdown, setShowAutoNextCountdown] = useState(false);
  const [showAutoNextPremiumCta, setShowAutoNextPremiumCta] = useState(false);
  const hasRecorded = useRef(false);
  const autoNextTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const autoNextIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // Player ad flow: user clicks play → ad shows → ad dismissed → video starts
  const [playIntended, setPlayIntended] = useState(false);
  const [videoCanStart, setVideoCanStart] = useState(false);

  const handlePlayIntent = useCallback(() => {
    setPlayIntended(true);
    if (isUserPremium(user)) {
      // Premium users have no ad overlay — start video directly on first click
      setVideoCanStart(true);
    }
  }, [user]);
  const handleAdFirstComplete = useCallback(() => setVideoCanStart(true), []);

  // Get localized metadata (always Uzbek)
  const localizedTitle = getLocalizedTitle(movie);
  const localizedDescription = getLocalizedDescription(movie);
  const localizedGenres = getLocalizedGenres(movie);
  const localizedCountry = getLocalizedCountry(movie);
  const isPremiumViewer = isUserPremium(user);
  const targetType = movie.type === "episode" ? "episode" : "movie";
  const backHref = movie.type === "episode"
    ? movie.series_slug
      ? `/series/${movie.series_slug}`
      : "/series"
    : `/movies/${movie.slug}`;

  useEffect(() => {
    let cancelled = false;
    setPlaybackAccessError(null);
    setResolvedPlaybackUrl(null);

    if (movie.source_type !== "direct_hls" && movie.source_type !== "direct_mp4") {
      const nonProtectedUrl = movie.embed_url || movie.video_url || null;
      setResolvedPlaybackUrl(nonProtectedUrl);
      return;
    }

    if (MEDIA_ACCESS_MODE !== "protected") {
      const publicSrc = resolvePublicPlaybackUrl(movie);
      logger.debug("[player-src] resolved");
      setResolvedPlaybackUrl(publicSrc || null);
      return;
    }

    const loadProtectedPlayback = async () => {
      try {
        const response = await getProtectedMediaAccess({
          movieId: movie.type === "episode" ? undefined : movie.id,
          episodeId: movie.type === "episode" ? movie.id : undefined,
          token,
        });
        const playbackUrl = response.playback_url || "";
        const finalSrc = playbackUrl.startsWith("https://cdn.filmorauz.net/media/")
          ? playbackUrl
          : normalizeMediaUrl(playbackUrl, "");
        if (!response.protected || !finalSrc.includes("/media/")) {
          throw new Error("Protected playback URL is invalid");
        }
        logger.debug("[media-token] resolved");
        if (!cancelled) {
          setResolvedPlaybackUrl(finalSrc);
        }
      } catch (error) {
        console.error("Failed to resolve protected media URL:", error);
        if (!cancelled) {
          setPlaybackAccessError("Himoyalangan video manbasi olinmadi.");
          setResolvedPlaybackUrl(null);
        }
      }
    };

    loadProtectedPlayback();
    return () => {
      cancelled = true;
    };
  }, [movie.embed_url, movie.id, movie.master_playlist_url, movie.source_type, movie.type, movie.video_url, token]);

  // Fetch watch progress for resume
  useEffect(() => {
    if (!user || !token) return;
    getWatchProgressForResume(token, movie.id, targetType)
      .then(setResumePosition)
      .catch(console.error);
  }, [user, token, movie.id, targetType]);

  // Fetch recommendations
  useEffect(() => {
    getRecommendations(movie.id, 12)
      .then(setRecommendations)
      .catch(console.error)
      .finally(() => setIsLoadingRecommendations(false));
  }, [movie.id]);

  // Save progress periodically during playback
  const handleTimeUpdate = (currentTime: number, duration: number) => {
    if (!user || !token) return;
    
    // Save progress every 10-15 seconds
    const now = Date.now();
    if (handleTimeUpdate.lastSave && now - handleTimeUpdate.lastSave < 10000) {
      return;
    }
    handleTimeUpdate.lastSave = now;
    
    // Don't save if near the end (will mark complete instead)
    if (currentTime < duration - 60) {
      saveWatchProgress(token, movie.id, Math.floor(currentTime), Math.floor(duration), { targetType }).catch(console.error);
    }
  };
  handleTimeUpdate.lastSave = 0;

  // Handle video end
  const handleVideoEnded = () => {
    if (user && token) {
      markWatchComplete(token, movie.id, Math.floor(movie.duration * 60), { targetType }).catch(console.error);
    }
    if (movie.type === "episode" && episodeNavigation?.nextEpisode) {
      if (isPremiumViewer) {
        setShowAutoNextPremiumCta(false);
        setAutoNextCountdown(5);
        setShowAutoNextCountdown(true);
      } else {
        setShowAutoNextCountdown(false);
        setShowAutoNextPremiumCta(true);
      }
    }
  };

  const cancelAutoNext = useCallback(() => {
    setShowAutoNextCountdown(false);
    setAutoNextCountdown(5);
    if (autoNextTimeoutRef.current) clearTimeout(autoNextTimeoutRef.current);
    if (autoNextIntervalRef.current) clearInterval(autoNextIntervalRef.current);
    autoNextTimeoutRef.current = null;
    autoNextIntervalRef.current = null;
  }, []);

  useEffect(() => {
    setShowAutoNextCountdown(false);
    setShowAutoNextPremiumCta(false);
    setAutoNextCountdown(5);
    if (autoNextTimeoutRef.current) clearTimeout(autoNextTimeoutRef.current);
    if (autoNextIntervalRef.current) clearInterval(autoNextIntervalRef.current);
    autoNextTimeoutRef.current = null;
    autoNextIntervalRef.current = null;
  }, [movie.id]);

  useEffect(() => {
    setViewCount(movie.views ?? 0);
  }, [movie.id, movie.views]);

  useEffect(() => {
    if (!showAutoNextCountdown || !episodeNavigation?.nextEpisode) return;

    autoNextIntervalRef.current = setInterval(() => {
      setAutoNextCountdown((prev) => (prev > 0 ? prev - 1 : 0));
    }, 1000);

    autoNextTimeoutRef.current = setTimeout(() => {
      router.push(`/episode/${episodeNavigation.nextEpisode?.id}`);
    }, 5000);

    return () => {
      if (autoNextTimeoutRef.current) clearTimeout(autoNextTimeoutRef.current);
      if (autoNextIntervalRef.current) clearInterval(autoNextIntervalRef.current);
      autoNextTimeoutRef.current = null;
      autoNextIntervalRef.current = null;
    };
  }, [episodeNavigation?.nextEpisode, router, showAutoNextCountdown]);

  // Record view and watch history on mount
  useEffect(() => {
    // Avoid duplicate recordings in the same session
    if (hasRecorded.current) return;
    hasRecorded.current = true;

    // Record public view count (no auth required)
    recordView(movie.id)
      .then(() => setViewCount((prev) => prev + 1))
      .catch((err) => {
      console.error("Failed to record view:", err);
    });

    // Record watch history for authenticated users
    if (user && token) {
      recordWatchHistory(token, movie.id, { targetType }).catch((err) => {
        console.error("Failed to record watch history:", err);
      });
    }
  }, [movie.id, user, token, targetType]);

  // Check if movie is favorite
  useEffect(() => {
    if (!user || !token) return;
    checkIsFavorite(token, movie.id, { targetType })
      .then(setIsFavorite)
      .catch(console.error);
  }, [user, token, movie.id, targetType]);

  const handleToggleFavorite = async () => {
    if (!user || !token) return;
    setIsLoading(true);
    try {
      if (isFavorite) {
        await removeFavorite(token, movie.id, { targetType });
        setIsFavorite(false);
      } else {
        await addFavorite(token, movie.id, { targetType });
        setIsFavorite(true);
      }
    } catch (error) {
      console.error("Failed to toggle favorite:", error);
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="pt-20 sm:pt-24 min-h-screen">
      {/* Minimal top bar */}
      <div className="h-[env(safe-area-inset-top)]" />
      <div className="bg-brand-dark/95 backdrop-blur-sm border-b border-brand-border px-3 sm:px-4 py-2 sm:py-3 flex items-center gap-3 sm:gap-4">
        <Link
          href={backHref}
          className="flex items-center gap-1.5 text-xs sm:text-sm text-gray-400 hover:text-white transition-colors"
        >
          <ChevronLeft size={18} />
          <span className="hidden sm:inline">Orqaga</span>
        </Link>
        <div className="h-4 sm:h-5 w-px bg-brand-border" />
        <h1 className="font-display text-sm sm:text-xl text-white tracking-wide truncate max-w-[200px] sm:max-w-none">
          {localizedTitle}
        </h1>
        <div className="flex items-center gap-3 sm:gap-4 ml-auto text-xs text-gray-500 shrink-0">
          {movie.year && (
            <span className="flex items-center gap-1">
              <Calendar size={12} />
              <span className="hidden sm:inline">{movie.year}</span>
            </span>
          )}
          {movie.duration > 0 && (
            <span className="flex items-center gap-1">
              <Clock size={12} />
              {formatDuration(movie.duration)}
            </span>
          )}
          {movie.quality && (
            <span className="text-brand-red font-bold">{movie.quality}</span>
          )}
        </div>
      </div>

      {/* Player and content */}
      <div className="max-w-6xl mx-auto px-3 sm:px-4 pt-4 sm:pt-6 pb-6 sm:pb-8">
        {/* Premium access control */}
        {isMoviePremium(movie) && !isUserPremium(user) ? (
          <div className="relative aspect-video rounded-xl overflow-hidden bg-brand-dark">
            {/* Blurred poster background */}
            <div
              className="absolute inset-0 bg-cover bg-center blur-xl scale-110"
              style={{ backgroundImage: `url(${normalizeMediaUrl(movie.poster_url, DEFAULT_POSTER_PLACEHOLDER)})` }}
            />
            <div className="absolute inset-0 bg-black/60" />
            
            {/* Lock overlay */}
            <PremiumLockOverlay
              title="Premium kontent"
              message="Bu kino faqat Premium foydalanuvchilar uchun."
              className="absolute inset-0 m-auto"
            />

            {/* Upgrade button */}
            <div className="absolute bottom-8 left-0 right-0 flex justify-center">
              <PremiumButton
                onClick={() => window.open('https://t.me/filmorauz_bot', '_blank')}
              >
                <Crown size={18} />
                Premium olish
              </PremiumButton>
            </div>
          </div>
        ) : (
          <div className="relative">
            {playbackAccessError ? (
              <div className="flex aspect-video items-center justify-center rounded-xl bg-gray-900 px-6 text-center">
                <p className="text-sm text-red-400">{playbackAccessError}</p>
              </div>
            ) : resolvedPlaybackUrl ? (
              <VideoPlayer
                videoUrl={resolvedPlaybackUrl}
                premiumStreamUrl={resolvedPlaybackUrl}
                embedUrl={movie.embed_url}
                sourceType={movie.source_type}
                title={localizedTitle}
                posterUrl={normalizeMediaUrl(movie.backdrop_url || movie.poster_url, DEFAULT_POSTER_PLACEHOLDER)}
                onPlayIntent={handlePlayIntent}
                forceStart={videoCanStart}
                onTimeUpdate={handleTimeUpdate}
                onEnded={handleVideoEnded}
                headerTitle={
                  movie.type === "episode" && movie.series_title
                    ? movie.series_title
                    : localizedTitle
                }
                headerSubtitle={
                  movie.type === "episode"
                    ? `${movie.episode_number ? `${movie.episode_number}-qism` : ""}${
                        localizedTitle && localizedTitle !== movie.series_title
                          ? ` — ${localizedTitle}`
                          : ""
                      }`.trim() || undefined
                    : undefined
                }
                seriesButtonUrl={
                  movie.type === "episode" && movie.series_slug
                    ? `/series/${movie.series_slug}`
                    : undefined
                }
              />
            ) : (
              <div className="flex aspect-video items-center justify-center rounded-xl bg-gray-900">
                <div className="h-10 w-10 animate-spin rounded-full border-2 border-white border-t-transparent" />
              </div>
            )}
            {!isUserPremium(user) && (
              <PlayerOverlayAd
                started={playIntended}
                onFirstComplete={handleAdFirstComplete}
              />
            )}
          </div>
        )}

        {movie.type === "episode" && (episodeNavigation?.previousEpisode || episodeNavigation?.nextEpisode) && (
          <div className="mt-4 rounded-2xl border border-brand-border bg-brand-card/70 p-3 sm:p-4">
            {showAutoNextCountdown && episodeNavigation?.nextEpisode && (
              <div className="mb-3 flex flex-col gap-3 rounded-xl border border-yellow-500/25 bg-yellow-500/10 p-3 sm:flex-row sm:items-center sm:justify-between">
                <div className="text-sm text-yellow-200">
                  {`Keyingi qism ${autoNextCountdown} soniyadan keyin boshlanadi`}
                </div>
                <button
                  onClick={cancelAutoNext}
                  className="inline-flex items-center justify-center rounded-lg border border-yellow-500/30 bg-brand-dark px-3 py-2 text-sm font-medium text-white transition-colors hover:border-yellow-400 hover:text-yellow-200"
                >
                  Bekor qilish
                </button>
              </div>
            )}
            {showAutoNextPremiumCta && episodeNavigation?.nextEpisode && (
              <div className="mb-3 flex flex-col gap-3 rounded-xl border border-yellow-500/25 bg-yellow-500/10 p-3 sm:flex-row sm:items-center sm:justify-between">
                <div className="text-sm text-yellow-200">
                  Auto-next faqat Premium foydalanuvchilar uchun
                </div>
                <Link
                  href="/premium"
                  className="inline-flex items-center justify-center rounded-lg bg-gradient-to-r from-yellow-500 to-amber-600 px-3 py-2 text-sm font-semibold text-black transition-opacity hover:opacity-90"
                >
                  Premium olish
                </Link>
              </div>
            )}
            <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              {episodeNavigation?.previousEpisode ? (
                <Link
                  href={`/episode/${episodeNavigation.previousEpisode.id}`}
                  className="inline-flex w-full items-center justify-center gap-2 rounded-xl border border-brand-border bg-brand-dark px-4 py-3 text-sm font-medium text-white transition-colors hover:border-brand-red hover:text-brand-red sm:w-auto sm:min-w-[220px] sm:justify-start"
                >
                  <ChevronLeft size={18} />
                  <span>Oldingi qism</span>
                </Link>
              ) : (
                <div className="hidden sm:block" />
              )}
              {episodeNavigation?.nextEpisode ? (
                <Link
                  href={`/episode/${episodeNavigation.nextEpisode.id}`}
                  className="inline-flex w-full items-center justify-center gap-2 rounded-xl border border-brand-border bg-brand-dark px-4 py-3 text-sm font-medium text-white transition-colors hover:border-brand-red hover:text-brand-red sm:w-auto sm:min-w-[220px] sm:justify-end"
                >
                  <span>{`Keyingi qism: ${episodeNavigation.nextEpisode.episode_number}-qism`}</span>
                  <ChevronRight size={18} />
                </Link>
              ) : (
                <div className="hidden sm:block" />
              )}
            </div>
          </div>
        )}

        <div className="mt-4 sm:mt-6 flex flex-col md:flex-row gap-4 sm:gap-6">
          <div className="flex-1">
            <h2 className="font-display text-2xl sm:text-3xl text-white tracking-wide mb-2">
              {localizedTitle}
            </h2>
            {movie.code && (
              <span className="inline-block bg-brand-red/20 text-brand-red text-xs font-mono px-2 py-0.5 rounded mb-2">
                #{movie.code}
              </span>
            )}
            <div className="flex flex-wrap gap-2 mb-3">
              {localizedGenres?.map((g) => (
                <span
                  key={g}
                  className="text-xs bg-brand-card border border-brand-border text-gray-400 px-2 py-0.5 rounded-full"
                >
                  {g}
                </span>
              ))}
            </div>
            <p className="text-gray-400 leading-relaxed text-sm sm:text-base">
              {localizedDescription}
            </p>
          </div>

          <div className="shrink-0 text-sm text-gray-500 space-y-2 min-w-[140px] sm:min-w-[160px]">
            <div>
              <span className="text-gray-600 block text-xs uppercase tracking-wider mb-0.5">Yil</span>
              <span className="text-gray-300">{movie.year}</span>
            </div>
            {localizedCountry && (
              <div>
                <span className="text-gray-600 block text-xs uppercase tracking-wider mb-0.5">Mamlakat</span>
                <span className="text-gray-300">{localizedCountry}</span>
              </div>
            )}
            {movie.duration > 0 && (
              <div>
                <span className="text-gray-600 block text-xs uppercase tracking-wider mb-0.5">Davomiyligi</span>
                <span className="text-gray-300">{formatDuration(movie.duration)}</span>
              </div>
            )}
            {movie.quality && (
              <div>
                <span className="text-gray-600 block text-xs uppercase tracking-wider mb-0.5">Sifat</span>
                <span className="text-brand-red font-semibold">{movie.quality}</span>
              </div>
            )}
            <div className="flex items-center gap-1.5 text-gray-400 pt-2">
              <Eye size={14} />
              <span className="text-xs">{viewCount} ko'rish</span>
            </div>
            {user && token && (
              <button
                onClick={handleToggleFavorite}
                disabled={isLoading}
                className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg transition-colors w-full justify-center ${
                  isFavorite
                    ? "bg-brand-red text-white"
                    : "bg-brand-card border border-brand-border text-gray-300 hover:border-brand-red hover:text-brand-red"
                }`}
              >
                <Heart size={14} fill={isFavorite ? "currentColor" : "none"} />
                <span className="text-xs">
                  {isFavorite ? t("movie.inFavorites") : t("movie.addToFavorites")}
                </span>
              </button>
            )}
          </div>
        </div>

        {/* Watch page inline block ad */}
        <div className="max-w-7xl mx-auto px-4 mt-6 mb-4">
          <WebsiteAdSlot placement="watch_page_inline_block" variant="inline" />
        </div>

        {/* Recommendations */}
        {!isLoadingRecommendations && recommendations.length > 0 && (
          <RecommendationsRow movies={recommendations} />
        )}
      </div>
    </div>
  );
}
