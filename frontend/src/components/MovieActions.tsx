"use client";

import { useState, useEffect, useCallback } from "react";
import { Heart, Eye, Loader2 } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { useI18n } from "@/lib/i18n";
import { 
  addFavorite, 
  removeFavorite, 
  checkIsFavorite, 
  recordWatchHistory,
  recordView,
  Movie 
} from "@/lib/api";

interface MovieActionsProps {
  movie: Movie;
}

export default function MovieActions({ movie }: MovieActionsProps) {
  const { t } = useI18n();
  const { isAuthenticated, token } = useAuth();
  const [isFavorite, setIsFavorite] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [isInitialLoading, setIsInitialLoading] = useState(true);

  // Check if movie is favorite on mount
  useEffect(() => {
    if (!isAuthenticated || !token) {
      setIsInitialLoading(false);
      return;
    }

    checkIsFavorite(token, movie.id)
      .then(setIsFavorite)
      .catch(console.error)
      .finally(() => setIsInitialLoading(false));
  }, [isAuthenticated, token, movie.id]);

  // Record view on mount (public) - only once per browser session
  useEffect(() => {
    // Use sessionStorage to prevent multiple view increments on page reload
    const viewKey = `viewed_${movie.id}`;
    if (sessionStorage.getItem(viewKey)) {
      return;
    }
    
    // Mark as viewed and record
    sessionStorage.setItem(viewKey, "true");
    recordView(movie.id).catch(console.error);
  }, [movie.id]);

  // Record watch history on mount (authenticated)
  const handleRecordHistory = useCallback(() => {
    if (!isAuthenticated || !token) return;
    recordWatchHistory(token, movie.id).catch(console.error);
  }, [isAuthenticated, token, movie.id]);

  useEffect(() => {
    if (isAuthenticated && token) {
      handleRecordHistory();
    }
  }, [isAuthenticated, token, handleRecordHistory]);

  const handleToggleFavorite = async () => {
    if (!isAuthenticated || !token) {
      // Could show login prompt here
      return;
    }

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
    <div className="flex items-center gap-4">
      {/* View Count */}
      <div className="flex items-center gap-1.5 text-gray-400">
        <Eye size={16} />
        <span className="text-sm">
          {movie.views ?? 0} ko'rish
        </span>
      </div>

      {/* Favorite Button */}
      {isAuthenticated ? (
        <button
          onClick={handleToggleFavorite}
          disabled={isLoading || isInitialLoading}
          className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg transition-colors ${
            isFavorite 
              ? "bg-brand-red text-white" 
              : "bg-brand-card border border-brand-border text-gray-300 hover:border-brand-red hover:text-brand-red"
          }`}
        >
          {isLoading || isInitialLoading ? (
            <Loader2 size={16} className="animate-spin" />
          ) : (
            <Heart size={16} fill={isFavorite ? "currentColor" : "none"} />
          )}
          <span className="text-sm">
            {isFavorite ? t("movie.inFavorites") : t("movie.addToFavorites")}
          </span>
        </button>
      ) : (
        <button
          onClick={handleToggleFavorite}
          disabled={true}
          className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-brand-card border border-brand-border text-gray-500 cursor-not-allowed opacity-50"
        >
          <Heart size={16} />
          <span className="text-sm">{t("movie.addToFavorites")}</span>
        </button>
      )}
    </div>
  );
}
