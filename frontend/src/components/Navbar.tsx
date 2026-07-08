"use client";

import { useState, useEffect, useRef } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Search, Menu, X, Film, User, LogIn, Crown, Lightbulb } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { useAuth } from "@/lib/auth-context";
import { searchMovies, Movie } from "@/lib/api";
import TelegramLoginModal from "./TelegramLoginModal";
import SuggestionModal from "./SuggestionModal";
import NotificationBell from "./NotificationBell";
import ActiveRoomBadge from "./ActiveRoomBadge";
import { resolveIsPremium } from "./PremiumComponents";
import Logo from "./Logo";
import { getLocalizedTitle, getLocalizedGenres } from "@/lib/localization";
import { DEFAULT_AVATAR_PLACEHOLDER, DEFAULT_POSTER_PLACEHOLDER, normalizeMediaUrl } from "@/lib/image-utils";
import MediaImage from "@/components/ui/MediaImage";

const GENRE_NAV_LINKS = [
  { href: "/movies?genre=animation", label: "Multifilmlar" },
  { href: "/movies?genre=anime", label: "Anime" },
  { href: "/movies?genre=dorama", label: "Dorama" },
];

export default function Navbar() {
  const { t } = useI18n();
  const [searchOpen, setSearchOpen] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Movie[]>([]);
  const [searching, setSearching] = useState(false);
  const searchRef = useRef<HTMLDivElement>(null);
  const router = useRouter();
  const { isAuthenticated, user, isLoading, checkAuthStatus } = useAuth();
  const [loginModalOpen, setLoginModalOpen] = useState(false);
  const [suggestionModalOpen, setSuggestionModalOpen] = useState(false);

  // Handle Telegram auth status check on page load (if URL has auth code)
  useEffect(() => {
    const url = new URL(window.location.href);
    const authCode = url.searchParams.get("auth_code");
    const authStatus = url.searchParams.get("auth_status");

    if (authCode && authStatus) {
      url.searchParams.delete("auth_code");
      url.searchParams.delete("auth_status");
      window.history.replaceState({}, "", url.toString());

      if (authStatus === "completed") {
        checkAuthStatus(authCode).then(() => {
          setTimeout(() => {
            window.location.reload();
          }, 1500);
        });
      }
    }
  }, [checkAuthStatus]);

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

  // Close mobile menu on resize to desktop. Tied to the same breakpoint as the
  // desktop nav (lg = 1024px) so iPad widths (768–1023) stay on the tablet
  // header where the nav links live in the menu, not the bar.
  useEffect(() => {
    function handleResize() {
      if (window.innerWidth >= 1024) {
        setMenuOpen(false);
      }
    }
    window.addEventListener("resize", handleResize);
    return () => window.removeEventListener("resize", handleResize);
  }, []);

  const handleMovieClick = (slug: string, targetType?: string) => {
    setResults([]);
    setSearchOpen(false);
    setQuery("");
    setMenuOpen(false);
    if (targetType === "series") {
      router.push(`/series/${slug}`);
    } else {
      router.push(`/movies/${slug}`);
    }
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
    <header className="fixed top-0 left-0 right-0 z-[70] px-2 sm:px-4 pt-[env(safe-area-inset-top)]">
      {/* Floating liquid-glass island — detached from the page edges with
          oval (pill) sides. The nav bar and the mobile drawer live inside the
          same rounded shell so the whole thing reads as one object. */}
      <div
        className={`max-w-7xl mx-auto mt-2 sm:mt-3 glass-strong transition-[border-radius] duration-300 ${
          menuOpen ? "rounded-[28px]" : "rounded-full"
        }`}
      >
      <nav className="w-full min-w-0 px-3 sm:px-5 h-16 flex items-center justify-between gap-1.5 sm:gap-3 lg:gap-4">
        {/* Logo */}
        <Link href="/" className="flex items-center gap-1.5 sm:gap-2 shrink-0 min-w-0">
          <Film className="text-orange-500 shrink-0" size={22} />
          <Logo className="text-lg sm:text-2xl" />
          {resolveIsPremium(user) && (
            <span className="hidden sm:inline-flex items-center gap-1 text-xs bg-gradient-to-r from-yellow-500 to-amber-600 text-black px-2 py-0.5 rounded-full font-medium shadow-[0_0_10px_rgba(234,179,8,0.3)]">
              <Crown size={10} />
              Premium
            </span>
          )}
        </Link>

        {/* Desktop nav links — only on ≥1024 (lg). At iPad widths (768–1023)
            the seven links + premium badge would crowd out the right-side
            icons, so on tablet we keep just the action icons in the bar and
            move the links into the menu drawer. */}
        <div className="hidden lg:flex items-center gap-1 text-sm font-medium text-zinc-400">
          <Link href="/" className="px-3 py-2 rounded-lg hover:text-white hover:bg-white/5 transition-colors">
            {t("common.home")}
          </Link>
          <Link href="/movies" className="px-3 py-2 rounded-lg hover:text-white hover:bg-white/5 transition-colors">
            {t("common.movies")}
          </Link>
          <Link href="/rooms" className="px-3 py-2 rounded-lg hover:text-white hover:bg-white/5 transition-colors">
            Roomlar
          </Link>
          <Link href="/series" className="px-3 py-2 rounded-lg hover:text-white hover:bg-white/5 transition-colors">
            Seriallar
          </Link>
          {GENRE_NAV_LINKS.map((item) => (
            <Link
              key={item.href}
              href={item.href}
              className="px-3 py-2 rounded-lg hover:text-white hover:bg-white/5 transition-colors"
            >
              {item.label}
            </Link>
          ))}
          <Link
            href="/premium"
            className="flex items-center gap-1.5 px-3 py-2 rounded-lg text-yellow-500 hover:text-yellow-400 hover:bg-yellow-500/10 transition-colors"
          >
            <Crown size={14} />
            Premium
          </Link>
        </div>

        {/* Right side: Search + mobile menu */}
        <div className="flex items-center gap-1.5 sm:gap-3 shrink-0">
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
                  className="bg-black/60 border border-white/15 rounded-full px-4 py-2 text-sm text-white placeholder-zinc-400 focus:outline-none focus:border-orange-500 w-44 sm:w-64 transition-all"
                />
                <button
                  type="button"
                  onClick={() => {
                    setSearchOpen(false);
                    setQuery("");
                    setResults([]);
                  }}
                  className="ml-2 text-zinc-300 hover:text-white"
                  aria-label="Qidiruvni yopish"
                >
                  <X size={18} aria-hidden="true" />
                </button>
              </form>
            ) : (
              <button
                onClick={() => setSearchOpen(true)}
                className="text-zinc-400 hover:text-white transition-colors p-1.5 rounded-lg hover:bg-white/5"
                aria-label={t("common.search")}
              >
                <Search size={20} />
              </button>
            )}

            {/* Search dropdown */}
            {results.length > 0 && (
              <div className="absolute top-full right-0 mt-2 w-[calc(100vw-2rem)] max-w-[18rem] sm:w-72 glass-strong rounded-2xl overflow-hidden shadow-2xl">
                {searching && (
                  <div className="px-4 py-2 text-xs text-zinc-500">
                    {t("common.loading")}
                  </div>
                )}
                {results.slice(0, 6).map((item: any) => {
                  const posterSrc = item.poster_url;
                  return (
                  <button
                    key={item.id}
                    onClick={() => handleMovieClick(item.slug, item.target_type)}
                    className="w-full flex items-center gap-3 px-4 py-3 hover:bg-[#1e1e2e] transition-colors text-left"
                  >
                    <MediaImage
                      src={posterSrc}
                      alt={getLocalizedTitle(item)}
                      fallbackSrc={DEFAULT_POSTER_PLACEHOLDER}
                      className="w-8 h-12 object-cover rounded shrink-0"
                    />
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center justify-between gap-2">
                        <p className="text-sm font-medium text-white truncate">
                          {item.code && (
                            <span className="text-zinc-500 font-mono text-[10px] mr-1">
                              #{item.code}
                            </span>
                          )}
                          {getLocalizedTitle(item)}
                        </p>
                        {item.quality && (
                          <span className="shrink-0 text-[10px] font-bold px-1 rounded bg-brand-red/20 text-brand-red border border-brand-red/30">
                            {item.quality}
                          </span>
                        )}
                      </div>
                      <p className="text-xs text-zinc-500">
                        {item.year} · {item.target_type === "series" ? "Serial" : "Kino"}
                      </p>
                    </div>
                  </button>
                  );
                })}
              </div>
            )}
          </div>

          {/* Auth button or User avatar */}
          {!isLoading && (
            isAuthenticated ? (
              <div className="flex items-center gap-2">
                <ActiveRoomBadge />
                <NotificationBell />
                <button
                  onClick={() => setSuggestionModalOpen(true)}
                  className="hidden sm:flex text-zinc-400 hover:text-white transition-colors p-1.5 rounded-lg hover:bg-white/5"
                  aria-label="Kino tavsiya qilish"
                  title="Kino tavsiya qilish"
                >
                  <Lightbulb size={20} />
                </button>
                <Link
                  href="/user"
                  className="flex items-center justify-center w-10 h-10 rounded-full glass-card border border-white/10 hover:border-orange-500/50 transition-colors relative"
                  aria-label="Profile"
                >
                  {(user?.profile_image_url || user?.photo_url) ? (() => {
                    const avatarSrc = user.profile_image_url || user.photo_url || "";
                    return (
                    <MediaImage
                      src={avatarSrc}
                      alt="Profile"
                      fallbackSrc={DEFAULT_AVATAR_PLACEHOLDER}
                      className={`w-full h-full rounded-full object-cover ${resolveIsPremium(user) ? 'ring-2 ring-yellow-500 ring-offset-2 ring-offset-[#0a0a0f]' : ''}`}
                    />
                    );
                  })() : (
                    <User size={20} className={resolveIsPremium(user) ? 'text-yellow-400' : 'text-zinc-400'} />
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
                className="flex items-center justify-center w-10 h-10 rounded-full glass-card border border-white/10 hover:border-orange-500/50 transition-colors"
                aria-label="Login"
              >
                <LogIn size={20} className="text-zinc-400" />
              </button>
            )
          )}

          {/* Mobile/tablet menu toggle — visible up to lg (1024) so iPad widths
              get the drawer with all nav links. */}
          <button
            onClick={() => setMenuOpen(!menuOpen)}
            className="lg:hidden text-zinc-400 hover:text-white transition-colors p-1.5 rounded-lg hover:bg-white/5"
            aria-label="Toggle menu"
          >
            {menuOpen ? <X size={22} /> : <Menu size={22} />}
          </button>
        </div>
      </nav>

      {/* Mobile menu */}
      {menuOpen && (
        <div className="menu-drop lg:hidden border-t border-white/10 px-4 py-4 flex flex-col gap-1">
          {/* Mobile search */}
          <form
            onSubmit={handleSearchSubmit}
            className="flex items-center gap-2 mb-3"
          >
            <div className="relative flex-1">
              <Search
                size={15}
                className="absolute left-3 top-1/2 -translate-y-1/2 text-zinc-500"
              />
              <input
                type="text"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder={t("common.searchPlaceholder")}
                className="w-full bg-black/60 border border-white/15 rounded-full pl-9 pr-4 py-2.5 text-sm text-white placeholder-zinc-400 focus:outline-none focus:border-orange-500 transition-colors"
              />
            </div>
            <button
              type="submit"
              className="bg-orange-500 text-white px-3 py-2.5 rounded-lg text-sm font-medium"
            >
              {t("common.search")}
            </button>
          </form>

          {/* Mobile search results */}
          {results.length > 0 && (
            <div className="glass-strong rounded-2xl overflow-hidden mb-3">
              {results.slice(0, 4).map((item: any) => {
                const posterSrc = item.poster_url;
                return (
                <button
                  key={item.id}
                  onClick={() => handleMovieClick(item.slug, item.target_type)}
                  className="w-full flex items-center gap-3 px-4 py-3 hover:bg-[#1e1e2e] transition-colors text-left border-b border-white/5 last:border-0"
                >
                  <MediaImage
                    src={posterSrc}
                    alt={getLocalizedTitle(item)}
                    fallbackSrc={DEFAULT_POSTER_PLACEHOLDER}
                    className="w-8 h-12 object-cover rounded shrink-0"
                  />
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center justify-between gap-2">
                      <p className="text-sm font-medium text-white truncate">{getLocalizedTitle(item)}</p>
                      {item.quality && (
                        <span className="shrink-0 text-[10px] font-bold px-1 rounded bg-brand-red/20 text-brand-red border border-brand-red/30">
                          {item.quality}
                        </span>
                      )}
                    </div>
                    <p className="text-xs text-zinc-500">{item.year} · {item.target_type === "series" ? "Serial" : "Kino"}</p>
                  </div>
                </button>
                );
              })}
            </div>
          )}

          {/* Nav links */}
          {[
            { href: "/", label: t("common.home") },
            { href: "/movies", label: t("common.movies") },
            { href: "/series", label: "Seriallar" },
            { href: "/rooms", label: "Roomlar" },
            ...GENRE_NAV_LINKS,
          ].map((item) => (
            <Link
              key={item.href}
              href={item.href}
              onClick={() => setMenuOpen(false)}
              className="flex items-center px-3 py-3 rounded-lg text-sm font-medium text-zinc-400 hover:text-white hover:bg-white/5 transition-colors"
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
                className="flex items-center px-3 py-3 rounded-lg text-sm font-medium text-zinc-400 hover:text-white hover:bg-white/5 transition-colors"
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
                className="flex items-center px-3 py-3 rounded-lg text-sm font-medium text-zinc-400 hover:text-white hover:bg-white/5 transition-colors"
              >
                <LogIn size={16} className="mr-2" />
                Kirish
              </button>
            )
          )}
        </div>
      )}
      </div>
      {/* ── end floating island ── */}

      {/* Telegram Login Modal */}
      <TelegramLoginModal
        isOpen={loginModalOpen}
        onClose={() => setLoginModalOpen(false)}
      />

      {/* Suggestion Modal */}
      <SuggestionModal
        isOpen={suggestionModalOpen}
        onClose={() => setSuggestionModalOpen(false)}
      />
    </header>
  );
}
