"use client";

import { useState, useEffect, useRef } from "react";
import Link from "next/link";
import { ChevronLeft, Clock, Calendar, Heart, Eye, Crown } from "lucide-react";
import VideoPlayer from "@/components/VideoPlayer";
import { recordView, recordWatchHistory, addFavorite, removeFavorite, checkIsFavorite, getRecommendations, saveWatchProgress, markWatchComplete, Movie } from "@/lib/api";
import { useAuth } from "@/lib/auth-context";
import { useI18n } from "@/lib/i18n";
import { isMoviePremium, isUserPremium, PremiumLockOverlay, PremiumButton, PremiumBadge } from "@/components/PremiumComponents";
import { getLocalizedTitle, getLocalizedDescription, getLocalizedGenres, getLocalizedCountry } from "@/lib/localization";

// Fetch watch progress for resume
async function getWatchProgressForResume(token: string, movieId: string): Promise<number> {
  try {
    const res = await fetch(`${process.env.NEXT_PUBLIC_API_URL || ''}/user/history`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (!res.ok) return 0;
    const json = await res.json();
    const history = json.data || [];
    const item = history.find((h: any) => h.movie_id === movieId);
    if (item && item.last_position_sec > 30 && item.last_position_sec < (item.duration_sec || 0) - 60) {
      return item.last_position_sec;
    }
  } catch {}
  return 0;
}

interface WatchPageClientProps {
  movie: Movie;
}

function RecommendationsRow({ movies }: { movies: Movie[] }) {
  if (movies.length === 0) return null;

  return (
    <div className="mt-12">
      <h3 className="font-display text-xl sm:text-2xl tracking-wide text-white mb-6">
        Sizga yoqishi mumkin
      </h3>
      <div className="flex gap-3 overflow-x-auto pb-2">
        {movies.slice(0, 8).map((movie) => (
          <a
            key={movie.id}
            href={`/movies/${movie.slug}`}
            className="shrink-0 w-[120px] sm:w-[140px] group block"
          >
            <div className="relative aspect-[2/3] overflow-hidden rounded-lg bg-brand-border">
              <img
                src={movie.poster_url}
                alt={getLocalizedTitle(movie)}
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
        ))}
      </div>
    </div>
  );
}

export default function WatchPageClient({ movie }: WatchPageClientProps) {
  const { t } = useI18n();
  const { user, token } = useAuth();
  const [isFavorite, setIsFavorite] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [recommendations, setRecommendations] = useState<Movie[]>([]);
  const [isLoadingRecommendations, setIsLoadingRecommendations] = useState(true);
  const [resumePosition, setResumePosition] = useState(0);
  const hasRecorded = useRef(false);

  // Get localized metadata (always Uzbek)
  const localizedTitle = getLocalizedTitle(movie);
  const localizedDescription = getLocalizedDescription(movie);
  const localizedGenres = getLocalizedGenres(movie);
  const localizedCountry = getLocalizedCountry(movie);

  // Fetch watch progress for resume
  useEffect(() => {
    if (!user || !token) return;
    getWatchProgressForResume(token, movie.id)
      .then(setResumePosition)
      .catch(console.error);
  }, [user, token, movie.id]);

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
      saveWatchProgress(token, movie.id, Math.floor(currentTime), Math.floor(duration)).catch(console.error);
    }
  };
  handleTimeUpdate.lastSave = 0;

  // Handle video end
  const handleVideoEnded = () => {
    if (!user || !token) return;
    markWatchComplete(token, movie.id, Math.floor(movie.duration * 60)).catch(console.error);
  };

  // Record view and watch history on mount
  useEffect(() => {
    // Avoid duplicate recordings in the same session
    if (hasRecorded.current) return;
    hasRecorded.current = true;

    // Record public view count (no auth required)
    recordView(movie.id).catch((err) => {
      console.error("Failed to record view:", err);
    });

    // Record watch history for authenticated users
    if (user && token) {
      recordWatchHistory(token, movie.id).catch((err) => {
        console.error("Failed to record watch history:", err);
      });
    }
  }, [movie.id, user, token]);

  // Check if movie is favorite
  useEffect(() => {
    if (!user || !token) return;
    checkIsFavorite(token, movie.id)
      .then(setIsFavorite)
      .catch(console.error);
  }, [user, token, movie.id]);

  const handleToggleFavorite = async () => {
    if (!user || !token) return;
    setIsLoading(true);
    try {
      if (isFavorite) {
        await removeFavorite(token, movie.id);
        setIsFavorite(false);
      } else {
        await addFavorite(token, movie.id);
        setIsFavorite(true);
      }
    } catch (error) {
      console.error("Failed to toggle favorite:", error);
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <>
      {/* Minimal top bar */}
      <div className="bg-brand-dark/95 backdrop-blur-sm border-b border-brand-border px-3 sm:px-4 py-2 sm:py-3 flex items-center gap-3 sm:gap-4">
        <Link
          href={`/movies/${movie.slug}`}
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
              {movie.duration} min
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
              style={{ backgroundImage: `url(${movie.poster_url})` }}
            />
            <div className="absolute inset-0 bg-black/60" />
            
            {/* Lock overlay */}
            <PremiumLockOverlay
              title="Premium Content"
              message="This movie is available for premium members only."
              className="absolute inset-0 m-auto"
            />
            
            {/* Upgrade button */}
            <div className="absolute bottom-8 left-0 right-0 flex justify-center">
              <PremiumButton
                onClick={() => window.open('https://t.me/filmorauz_bot', '_blank')}
              >
                <Crown size={18} />
                Upgrade to Premium
              </PremiumButton>
            </div>
          </div>
        ) : (
          <VideoPlayer
            videoUrl={movie.video_url}
            embedUrl={movie.embed_url}
            sourceType={movie.source_type}
            title={localizedTitle}
            posterUrl={movie.backdrop_url || movie.poster_url}
          />
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
                <span className="text-gray-300">{movie.duration} min</span>
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
              <span className="text-xs">{movie.views ?? 0} ko'rish</span>
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

        {/* Recommendations */}
        {!isLoadingRecommendations && recommendations.length > 0 && (
          <RecommendationsRow movies={recommendations} />
        )}
      </div>
    </>
  );
}
