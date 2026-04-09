"use client";

import { useState, useEffect, useRef } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Search, Menu, X, Film, User, LogIn, Crown } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { useAuth } from "@/lib/auth-context";
import { searchMovies, Movie } from "@/lib/api";
import TelegramLoginModal from "./TelegramLoginModal";
import NotificationBell from "./NotificationBell";
import { resolveIsPremium } from "./PremiumComponents";
import { getLocalizedTitle, getLocalizedGenres } from "@/lib/localization";

export default function Navbar() {
  const { t } = useI18n();
  const [searchOpen, setSearchOpen] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Movie[]>([]);
  const [searching, setSearching] = useState(false);
  const searchRef = useRef<HTMLDivElement>(null);
  const router = useRouter();
  const { isAuthenticated, user, isLoading } = useAuth();
  const [loginModalOpen, setLoginModalOpen] = useState(false);

  // Handle Telegram auth status check on page load (if URL has auth code)
  useEffect(() => {
    const url = new URL(window.location.href);
    const authCode = url.searchParams.get("auth_code");
    const authStatus = url.searchParams.get("auth_status");
    
    if (authCode && authStatus) {
      // Clear URL params
      url.searchParams.delete("auth_code");
      url.searchParams.delete("auth_status");
      window.history.replaceState({}, "", url.toString());
      
      // If completed, just refresh - the auth context will pick up the new cookie
      if (authStatus === "completed") {
        window.location.reload();
      }
    }
  }, []);

  // Debounced search
  useEffect(() => {
    if (!query.trim()) {
      setResults([]);
      return;
    }
    const timer = setTimeout(async () => {
      setSearching(true);
      try {
        const data = await searchMovies(query);
        setResults(data || []);
      } catch {
        setResults([]);
      } finally {
        setSearching(false);
      }
    }, 350);
    return () => clearTimeout(timer);
  }, [query]);

  // Close dropdown on outside click
  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (searchRef.current && !searchRef.current.contains(e.target as Node)) {
        setResults([]);
        setSearchOpen(false);
      }
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, []);

  // Close mobile menu on resize to desktop
  useEffect(() => {
    function handleResize() {
      if (window.innerWidth >= 768) {
        setMenuOpen(false);
      }
    }
    window.addEventListener("resize", handleResize);
    return () => window.removeEventListener("resize", handleResize);
  }, []);

  const handleMovieClick = (slug: string) => {
    setResults([]);
    setSearchOpen(false);
    setQuery("");
    setMenuOpen(false);
    router.push(`/movies/${slug}`);
  };

  const handleSearchSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (query.trim()) {
      setResults([]);
      setSearchOpen(false);
      setMenuOpen(false);
      router.push(`/movies?search=${encodeURIComponent(query)}`);
    }
  };

  return (
    <header className="fixed top-0 left-0 right-0 z-[70] bg-brand-dark/90 backdrop-blur-md border-b border-brand-border">
      <nav className="max-w-7xl mx-auto px-4 h-16 flex items-center justify-between gap-4">
        {/* Logo */}
        <Link href="/" className="flex items-center gap-2 shrink-0">
          <Film className="text-brand-red" size={22} />
          <span className="font-display text-xl sm:text-2xl tracking-wider text-white">
            FILMORA<span className="text-brand-red">UZ</span>
          </span>
          {resolveIsPremium(user) && (
            <span className="hidden sm:inline-flex items-center gap-1 text-xs bg-gradient-to-r from-yellow-500 to-amber-600 text-black px-2 py-0.5 rounded-full font-medium shadow-[0_0_10px_rgba(234,179,8,0.3)]">
              <Crown size={10} />
              Premium
            </span>
          )}
        </Link>

        {/* Desktop nav links */}
        <div className="hidden md:flex items-center gap-5 text-sm font-medium text-gray-400">
          <Link href="/" className="hover:text-white transition-colors">
            {t("common.home")}
          </Link>
          <Link href="/movies" className="hover:text-white transition-colors">
            {t("common.movies")}
          </Link>
          <Link href="/series" className="hover:text-white transition-colors">
            Seriallar
          </Link>
          <Link
            href="/movies?genre=Action"
            className="hover:text-white transition-colors"
          >
            {t("common.action")}
          </Link>
          <Link
            href="/movies?genre=Drama"
            className="hover:text-white transition-colors"
          >
            {t("common.drama")}
          </Link>
          <Link
            href="/movies?genre=Comedy"
            className="hover:text-white transition-colors"
          >
            {t("common.comedy")}
          </Link>
          <Link
            href="/premium"
            className="flex items-center gap-1.5 text-yellow-500 hover:text-yellow-400 transition-colors"
          >
            <Crown size={14} />
            Premium
          </Link>
        </div>

        {/* Right side: Search + mobile menu */}
        <div className="flex items-center gap-2 sm:gap-3">
          {/* Search */}
          <div ref={searchRef} className="relative">
            {searchOpen ? (
              <form onSubmit={handleSearchSubmit} className="flex items-center">
                <input
                  autoFocus
                  type="text"
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  placeholder={t("common.searchPlaceholder")}
                  className="bg-brand-card border border-brand-border rounded-lg px-3 py-2 text-sm text-white placeholder-gray-500 focus:outline-none focus:border-brand-red w-44 sm:w-64 transition-all"
                />
                <button
                  type="button"
                  onClick={() => {
                    setSearchOpen(false);
                    setQuery("");
                    setResults([]);
                  }}
                  className="ml-2 text-gray-400 hover:text-white"
                >
                  <X size={18} />
                </button>
              </form>
            ) : (
              <button
                onClick={() => setSearchOpen(true)}
                className="text-gray-400 hover:text-white transition-colors p-1"
                aria-label={t("common.search")}
              >
                <Search size={20} />
              </button>
            )}

            {/* Search dropdown */}
            {results.length > 0 && (
              <div className="absolute top-full right-0 mt-2 w-72 bg-brand-card border border-brand-border rounded-xl overflow-hidden shadow-2xl">
                {searching && (
                  <div className="px-4 py-2 text-xs text-gray-500">
                    {t("common.loading")}
                  </div>
                )}
                {results.slice(0, 6).map((movie) => (
                  <button
                    key={movie.id}
                    onClick={() => handleMovieClick(movie.slug)}
                    className="w-full flex items-center gap-3 px-4 py-3 hover:bg-brand-border transition-colors text-left"
                  >
                    {/* eslint-disable-next-line @next/next/no-img-element */}
                    <img
                      src={movie.poster_url}
                      alt={getLocalizedTitle(movie)}
                      className="w-8 h-12 object-cover rounded shrink-0"
                      onError={(e) => {
                        (e.target as HTMLImageElement).src =
                          "/placeholder-poster.jpg";
                      }}
                    />
                    <div className="min-w-0">
                      <p className="text-sm font-medium text-white truncate">
                        {movie.code && (
                          <span className="text-gray-500 font-mono text-xs mr-1">
                            #{movie.code}
                          </span>
                        )}
                        {getLocalizedTitle(movie)}
                      </p>
                      <p className="text-xs text-gray-500">
                        {movie.year} · {getLocalizedGenres(movie)?.join(", ")}
                      </p>
                    </div>
                  </button>
                ))}
              </div>
            )}
          </div>

          {/* Auth button or User avatar */}
          {!isLoading && (
            isAuthenticated ? (
              <div className="flex items-center gap-2">
                <NotificationBell />
                <Link
                  href="/user"
                  className="flex items-center justify-center w-10 h-10 rounded-full bg-brand-card border border-brand-border hover:border-brand-red/50 transition-colors relative"
                  aria-label="Profile"
                >
                  {(user?.profile_image_url || user?.photo_url) ? (
                    <img
                      src={user.profile_image_url || user.photo_url}
                      alt="Profile"
                      className={`w-full h-full rounded-full object-cover ${resolveIsPremium(user) ? 'ring-2 ring-yellow-500 ring-offset-2 ring-offset-brand-dark' : ''}`}
                    />
                  ) : (
                    <User size={20} className={resolveIsPremium(user) ? 'text-yellow-400' : 'text-gray-400'} />
                  )}
                  {/* Premium crown indicator */}
                  {resolveIsPremium(user) && (
                    <div className="absolute -top-1 -right-1 bg-yellow-500 rounded-full p-0.5">
                      <Crown size={10} className="text-black" />
                    </div>
                  )}
                </Link>
              </div>
            ) : (
              <button
                onClick={() => setLoginModalOpen(true)}
                className="flex items-center justify-center w-10 h-10 rounded-full bg-brand-card border border-brand-border hover:border-brand-red/50 transition-colors"
                aria-label="Login"
              >
                <LogIn size={20} className="text-gray-400" />
              </button>
            )
          )}

          {/* Mobile menu toggle */}
          <button
            onClick={() => setMenuOpen(!menuOpen)}
            className="md:hidden text-gray-400 hover:text-white transition-colors p-1"
            aria-label="Toggle menu"
          >
            {menuOpen ? <X size={22} /> : <Menu size={22} />}
          </button>
        </div>
      </nav>

      {/* Mobile menu */}
      {menuOpen && (
        <div className="md:hidden border-t border-brand-border bg-brand-dark/95 backdrop-blur-md px-4 py-4 flex flex-col gap-1">
          {/* Mobile search */}
          <form
            onSubmit={handleSearchSubmit}
            className="flex items-center gap-2 mb-3"
          >
            <div className="relative flex-1">
              <Search
                size={15}
                className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500"
              />
              <input
                type="text"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder={t("common.searchPlaceholder")}
                className="w-full bg-brand-card border border-brand-border rounded-lg pl-9 pr-4 py-2.5 text-sm text-white placeholder-gray-600 focus:outline-none focus:border-brand-red transition-colors"
              />
            </div>
            <button
              type="submit"
              className="bg-brand-red text-white px-3 py-2.5 rounded-lg text-sm font-medium"
            >
              {t("common.search")}
            </button>
          </form>

          {/* Mobile search results */}
          {results.length > 0 && (
            <div className="bg-brand-card border border-brand-border rounded-xl overflow-hidden mb-3">
              {results.slice(0, 4).map((movie) => (
                <button
                  key={movie.id}
                  onClick={() => handleMovieClick(movie.slug)}
                  className="w-full flex items-center gap-3 px-4 py-3 hover:bg-brand-border transition-colors text-left border-b border-brand-border/50 last:border-0"
                >
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img
                    src={movie.poster_url}
                    alt={getLocalizedTitle(movie)}
                    className="w-8 h-12 object-cover rounded shrink-0"
                    onError={(e) => {
                      (e.target as HTMLImageElement).src = "/placeholder-poster.jpg";
                    }}
                  />
                  <div className="min-w-0">
                    <p className="text-sm font-medium text-white truncate">{getLocalizedTitle(movie)}</p>
                    <p className="text-xs text-gray-500">{movie.year}</p>
                  </div>
                </button>
              ))}
            </div>
          )}

          {/* Nav links */}
          {[
            { href: "/", label: t("common.home") },
            { href: "/movies", label: t("common.movies") },
            { href: "/series", label: "Seriallar" },
            { href: "/movies?genre=Action", label: t("common.action") },
            { href: "/movies?genre=Drama", label: t("common.drama") },
            { href: "/movies?genre=Comedy", label: t("common.comedy") },
          ].map((item) => (
            <Link
              key={item.href}
              href={item.href}
              onClick={() => setMenuOpen(false)}
              className="flex items-center px-3 py-3 rounded-lg text-sm font-medium text-gray-400 hover:text-white hover:bg-brand-border transition-colors"
            >
              {item.label}
            </Link>
          ))}

          {/* Premium link - mobile */}
          <Link
            href="/premium"
            onClick={() => setMenuOpen(false)}
            className="flex items-center px-3 py-3 rounded-lg text-sm font-semibold text-yellow-500 hover:text-yellow-400 hover:bg-yellow-500/10 transition-colors"
          >
            <Crown size={16} className="mr-2" />
            Premium
          </Link>

          {/* Mobile Auth link */}
          {!isLoading && (
            isAuthenticated ? (
              <Link
                href="/user"
                onClick={() => setMenuOpen(false)}
                className="flex items-center px-3 py-3 rounded-lg text-sm font-medium text-gray-400 hover:text-white hover:bg-brand-border transition-colors"
              >
                <User size={16} className="mr-2" />
                Profil
              </Link>
            ) : (
              <button
                onClick={() => {
                  setMenuOpen(false);
                  setLoginModalOpen(true);
                }}
                className="flex items-center px-3 py-3 rounded-lg text-sm font-medium text-gray-400 hover:text-white hover:bg-brand-border transition-colors"
              >
                <LogIn size={16} className="mr-2" />
                Kirish
              </button>
            )
          )}
        </div>
      )}

      {/* Telegram Login Modal */}
      <TelegramLoginModal
        isOpen={loginModalOpen}
        onClose={() => setLoginModalOpen(false)}
      />
    </header>
  );
}
