"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useRouter, usePathname } from "next/navigation";
import Link from "next/link";
import { Film, LogOut, ChevronDown, Menu, X, Search } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { getAdminBadges, AdminBadges } from "@/lib/api";
import { useVisibleInterval } from "@/lib/use-visible-interval";
import { visibleNav, activeHref, badgeCount } from "@/components/admin/admin-nav";
import CommandPalette from "@/components/admin/CommandPalette";
import { ToastProvider } from "@/components/admin/Toast";
import { isFullAdminRole, isModeratorRole, moderatorCanOpen, MODERATOR_HOME } from "@/lib/roles";

const COLLAPSED_KEY = "admin-nav-collapsed";
const MODERATOR_RESULT_KINDS: ("user")[] = ["user"];

function readCollapsed(): Set<string> {
  try {
    const raw = localStorage.getItem(COLLAPSED_KEY);
    return new Set(raw ? (JSON.parse(raw) as string[]) : []);
  } catch {
    return new Set();
  }
}

export default function AdminLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const { isAuthenticated, isLoading, user, token, logout } = useAuth();
  const router = useRouter();
  const pathname = usePathname();

  // SECURITY FIX: Check if user has a staff role. `isAdmin` means "may enter
  // /admin at all" — moderators included, but they're confined to their pages.
  const isModerator = isModeratorRole(user?.role);
  const isAdmin = isFullAdminRole(user?.role) || isModerator;
  const isSuperAdmin = user?.role === "superadmin";
  const moderatorBlocked = isModerator && pathname !== "/admin/login" && !moderatorCanOpen(pathname);

  // Paths that require superadmin role specifically (backend enforces the
  // same via middleware.RequireSuperAdmin on /api/superadmin/*).
  const isSuperAdminPath =
    pathname.startsWith("/admin/ads") ||
    pathname.startsWith("/admin/expenses") ||
    pathname.startsWith("/admin/audit");

  // Protect all /admin/* except /admin/login
  useEffect(() => {
    // If not authenticated, redirect to login
    if (!isAuthenticated && pathname !== "/admin/login") {
      router.replace("/admin/login");
      return;
    }

    // SECURITY FIX: If authenticated but not admin, redirect to home or login
    // Only allow admin/superadmin users to access admin pages
    if (isAuthenticated && !isAdmin && pathname !== "/admin/login") {
      router.replace("/");
      return;
    }

    // Superadmin-only sections: a normal admin hitting the URL directly
    // gets bounced to the dashboard. Wait for the user profile to finish
    // loading so we don't flash-redirect a superadmin mid-boot.
    if (!isLoading && isAuthenticated && isAdmin && !isSuperAdmin && isSuperAdminPath && !isModerator) {
      router.replace("/admin/dashboard");
      return;
    }

    // Moderators only get the community pages (comments, appeals, ...).
    if (!isLoading && isAuthenticated && moderatorBlocked) {
      router.replace(MODERATOR_HOME);
    }
  }, [isAuthenticated, isLoading, pathname, router, isAdmin, isSuperAdmin, isSuperAdminPath, isModerator, moderatorBlocked]);

  // ── Sidebar state (declared before any early return — hooks order) ──
  const [badges, setBadges] = useState<AdminBadges | null>(null);
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const [mobileOpen, setMobileOpen] = useState(false);
  const [paletteOpen, setPaletteOpen] = useState(false);

  useEffect(() => {
    setCollapsed(readCollapsed());
  }, []);

  // Close the mobile drawer on navigation.
  useEffect(() => {
    setMobileOpen(false);
  }, [pathname]);

  const toggleGroup = useCallback((id: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      try {
        localStorage.setItem(COLLAPSED_KEY, JSON.stringify(Array.from(next)));
      } catch {
        // storage unavailable — collapse state just won't persist
      }
      return next;
    });
  }, []);

  // Sidebar badges refresh every minute while the tab is visible.
  const loadBadges = useCallback(
    (isActive: () => boolean) => {
      if (!token) return;
      getAdminBadges(token)
        .then((b) => {
          if (isActive()) setBadges(b);
        })
        .catch(() => {});
    },
    [token]
  );
  useVisibleInterval(loadBadges, 60_000, !!token && isAdmin && pathname !== "/admin/login");

  const nav = useMemo(() => visibleNav(user?.role), [user?.role]);
  const currentHref = activeHref(pathname, nav);

  // Don't render sidebar on login page
  if (pathname === "/admin/login") {
    return <ToastProvider>{children}</ToastProvider>;
  }

  if (!isAuthenticated) {
    return null; // Will redirect
  }

  // Block superadmin-only pages from rendering for non-superadmin admins,
  // matching the useEffect-driven redirect above. Prevents a flash of
  // restricted content before the router swap.
  if (!isLoading && isAdmin && !isSuperAdmin && isSuperAdminPath) {
    return null;
  }
  if (!isLoading && moderatorBlocked) {
    return null;
  }

  const sidebar = (
    <>
      {/* Logo */}
      <div className="px-6 py-5 border-b border-brand-border flex items-start justify-between">
        <div>
          <Link href="/" className="flex items-center gap-2">
            <Film className="text-brand-red" size={20} />
            <span className="font-display text-xl tracking-wider text-white">
              FILMORA<span className="text-brand-red">UZ</span>
            </span>
          </Link>
          <p className="text-xs text-gray-600 mt-1 ml-7">Admin Panel</p>
        </div>
        <button
          onClick={() => setMobileOpen(false)}
          className="lg:hidden text-gray-500 hover:text-white"
          aria-label="Menyuni yopish"
        >
          <X size={18} />
        </button>
      </div>

      {/* Search trigger */}
      <div className="px-3 pt-3">
        <button
          onClick={() => setPaletteOpen(true)}
          className="w-full flex items-center gap-2 px-3 py-2 rounded-lg border border-brand-border text-sm text-gray-500 hover:text-white hover:border-gray-500 transition-colors"
        >
          <Search size={15} />
          <span className="flex-1 text-left">Qidirish...</span>
          <kbd className="text-[10px] border border-brand-border rounded px-1.5 py-0.5">Ctrl K</kbd>
        </button>
      </div>

      {/* Nav */}
      <nav className="flex-1 px-3 py-3 space-y-3 overflow-y-auto" aria-label="Admin bo'limlari">
        {nav.map((group) => {
          const isCollapsed = collapsed.has(group.id);
          const groupBadge = group.items.reduce((sum, i) => sum + badgeCount(i.badge, badges), 0);
          return (
            <div key={group.id}>
              <button
                onClick={() => toggleGroup(group.id)}
                aria-expanded={!isCollapsed}
                className="w-full flex items-center gap-2 px-3 py-1 text-[11px] font-semibold uppercase tracking-wider text-gray-600 hover:text-gray-400"
              >
                <ChevronDown size={12} className={`transition-transform ${isCollapsed ? "-rotate-90" : ""}`} />
                <span className="flex-1 text-left">{group.label}</span>
                {isCollapsed && groupBadge > 0 && (
                  <span className="rounded-full bg-amber-500/20 px-1.5 text-[10px] text-amber-300 normal-case">{groupBadge}</span>
                )}
              </button>
              {!isCollapsed && (
                <div className="mt-1 space-y-0.5">
                  {group.items.map((item) => {
                    const active = currentHref === item.href;
                    const count = badgeCount(item.badge, badges);
                    return (
                      <Link
                        key={item.href}
                        href={item.href}
                        aria-current={active ? "page" : undefined}
                        className={`flex items-center gap-3 px-3 py-2 rounded-lg text-sm font-medium transition-colors ${
                          active
                            ? "bg-brand-red text-white"
                            : "text-gray-400 hover:text-white hover:bg-brand-border"
                        }`}
                      >
                        <item.icon size={17} />
                        <span className="flex-1 truncate">{item.label}</span>
                        {count > 0 && (
                          <span
                            className={`min-w-[1.25rem] rounded-full px-1.5 text-center text-[11px] font-semibold ${
                              active ? "bg-white/25 text-white" : "bg-amber-500/20 text-amber-300"
                            }`}
                            aria-label={`${count} ta kutilmoqda`}
                          >
                            {count > 99 ? "99+" : count}
                          </span>
                        )}
                      </Link>
                    );
                  })}
                </div>
              )}
            </div>
          );
        })}
      </nav>

      {/* Footer */}
      <div className="px-3 py-4 border-t border-brand-border">
        <Link
          href="/"
          className="flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm text-gray-400 hover:text-white hover:bg-brand-border transition-colors mb-1"
        >
          <Film size={17} />
          View Site
        </Link>
        <button
          onClick={() => {
            logout();
            router.push("/admin/login");
          }}
          className="w-full flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm text-gray-400 hover:text-red-400 hover:bg-brand-border transition-colors"
        >
          <LogOut size={17} />
          Logout
        </button>
      </div>
    </>
  );

  return (
    <ToastProvider>
    <div className="min-h-screen bg-brand-dark flex">
      {/* Desktop sidebar */}
      <aside className="hidden lg:flex w-60 bg-brand-card border-r border-brand-border flex-col shrink-0 sticky top-0 h-screen">
        {sidebar}
      </aside>

      {/* Mobile drawer */}
      {mobileOpen && (
        <div className="lg:hidden fixed inset-0 z-50 flex">
          <div className="absolute inset-0 bg-black/60" onClick={() => setMobileOpen(false)} aria-hidden />
          <aside className="relative w-64 max-w-[85vw] bg-brand-card border-r border-brand-border flex flex-col h-full">
            {sidebar}
          </aside>
        </div>
      )}

      {/* Main content */}
      <div className="flex-1 min-w-0 overflow-auto">
        {/* Mobile top bar */}
        <div className="lg:hidden sticky top-0 z-40 flex items-center gap-3 border-b border-brand-border bg-brand-card/95 backdrop-blur px-4 py-3">
          <button onClick={() => setMobileOpen(true)} className="text-gray-300" aria-label="Menyuni ochish">
            <Menu size={20} />
          </button>
          <span className="font-display tracking-wider text-white">
            FILMORA<span className="text-brand-red">UZ</span>
          </span>
          <button onClick={() => setPaletteOpen(true)} className="ml-auto text-gray-400" aria-label="Qidirish">
            <Search size={18} />
          </button>
        </div>
        {children}
      </div>

      <CommandPalette
        token={token}
        nav={nav}
        open={paletteOpen}
        onOpenChange={setPaletteOpen}
        resultKinds={isModerator ? MODERATOR_RESULT_KINDS : undefined}
      />
    </div>
    </ToastProvider>
  );
}
