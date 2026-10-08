"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Home, Clapperboard, Search, Bookmark, User } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { openSearch } from "@/lib/search-overlay";

// Pages where a bottom bar would get in the way (players, admin, rooms).
const HIDDEN_PREFIXES = ["/admin", "/watch/", "/watch-room/", "/episode/", "/banned"];

/** True on routes where the phone tab bar is not rendered. */
export function isBottomNavHidden(pathname: string | null): boolean {
  if (!pathname) return true;
  if (/^\/series\/[^/]+\/season\/\d+\/episode\//.test(pathname)) return true;
  return HIDDEN_PREFIXES.some((p) => pathname.startsWith(p));
}

/** Phone-only app-style tab bar. */
export default function BottomNav() {
  const pathname = usePathname();
  const { isAuthenticated, user } = useAuth();
  if (isBottomNavHidden(pathname)) return null;

  const isActive = (href: string) => (href === "/" ? pathname === "/" : pathname?.startsWith(href.split("?")[0]) ?? false);

  const item = (href: string, label: string, Icon: React.ElementType, active: boolean) => (
    <Link
      href={href}
      aria-current={active ? "page" : undefined}
      className={`flex flex-1 flex-col items-center justify-center gap-0.5 py-2 text-[10px] font-medium transition-colors ${
        active ? "text-orange-400" : "text-gray-400 active:text-white"
      }`}
    >
      <Icon size={21} strokeWidth={active ? 2.4 : 1.8} />
      {label}
    </Link>
  );

  return (
    <>
      {/* Spacer so page content/footers aren't hidden behind the bar. */}
      <div className="h-16 md:hidden" aria-hidden="true" />
      <nav
        aria-label="Asosiy navigatsiya"
        className="fixed inset-x-0 bottom-0 z-[65] border-t border-white/10 bg-[#0b0b12]/95 pb-[env(safe-area-inset-bottom)] backdrop-blur-lg md:hidden"
      >
        <div className="flex h-16 items-stretch">
          {item("/", "Bosh sahifa", Home, isActive("/"))}
          {item("/movies", "Kinolar", Clapperboard, isActive("/movies") || isActive("/series"))}
          <button
            onClick={() => openSearch()}
            className="flex flex-1 flex-col items-center justify-center gap-0.5 py-2 text-[10px] font-medium text-gray-400 active:text-white"
            aria-label="Qidiruv"
          >
            <span className="flex h-9 w-9 items-center justify-center rounded-full bg-orange-500 text-white shadow-lg shadow-orange-500/30">
              <Search size={18} strokeWidth={2.4} />
            </span>
          </button>
          {item(isAuthenticated ? "/user?tab=library" : "/user", "Kutubxona", Bookmark, false)}
          {item("/user", user ? "Profil" : "Kirish", User, pathname === "/user" || pathname?.startsWith("/user/") || false)}
        </div>
      </nav>
    </>
  );
}
