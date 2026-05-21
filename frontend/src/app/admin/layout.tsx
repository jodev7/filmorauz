"use client";

import { useEffect } from "react";
import { useRouter, usePathname } from "next/navigation";
import Link from "next/link";
import { Film, LayoutDashboard, List, LogOut, PlusCircle, Download, Users, FolderHeart, Folder, MessageSquare, Settings, Tv, Ban, History, MessageCircle, Video, Megaphone, Send, Lightbulb } from "lucide-react";
import { useAuth } from "@/lib/auth-context";

export default function AdminLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const { isAuthenticated, isLoading, user, logout } = useAuth();
  const router = useRouter();
  const pathname = usePathname();

  // SECURITY FIX: Check if user has admin/superadmin role
  const isAdmin = user?.role === "admin" || user?.role === "superadmin";
  const isSuperAdmin = user?.role === "superadmin";

  // Paths that require superadmin role specifically (backend enforces the
  // same via middleware.RequireSuperAdmin on /api/superadmin/*).
  const isSuperAdminPath = pathname.startsWith("/admin/ads");

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
    if (!isLoading && isAuthenticated && isAdmin && !isSuperAdmin && isSuperAdminPath) {
      router.replace("/admin/dashboard");
    }
  }, [isAuthenticated, isLoading, pathname, router, isAdmin, isSuperAdmin, isSuperAdminPath]);

  // Don't render sidebar on login page
  if (pathname === "/admin/login") {
    return <>{children}</>;
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

  const navItems = [
    { href: "/admin/dashboard", icon: LayoutDashboard, label: "Dashboard" },
    { href: "/admin/movies", icon: List, label: "Movies" },
    { href: "/admin/movies/new", icon: PlusCircle, label: "Add Movie" },
    { href: "/admin/series", icon: Tv, label: "Series" },
    { href: "/admin/series/new", icon: PlusCircle, label: "Add Series" },
    { href: "/admin/collections", icon: FolderHeart, label: "Collections" },
    { href: "/admin/ingestion", icon: Download, label: "Import Movies" },
    { href: "/admin/rooms", icon: Users, label: "Watch Rooms" },
    { href: "/admin/clips", icon: Video, label: "Clips" },
    { href: "/admin/content", icon: Folder, label: "Content" },
    { href: "/admin/users", icon: Users, label: "Users" },
    { href: "/admin/users/banned", icon: Ban, label: "Ban olganlar" },
    { href: "/admin/users/ban-history", icon: History, label: "Ban tarixi" },
    { href: "/admin/appeals", icon: MessageCircle, label: "Apellyatsiyalar" },
    { href: "/admin/suggestions", icon: Lightbulb, label: "Tavsiyalar" },
    { href: "/admin/comments", icon: MessageSquare, label: "Comments" },
    { href: "/admin/comments/settings", icon: Settings, label: "Comment Settings" },
    ...(isSuperAdmin ? [
      { href: "/admin/ads", icon: Megaphone, label: "Ads" },
    ] : []),
    ...(isAdmin ? [
      { href: "/admin/telegram-post", icon: Send, label: "Telegram Post" },
    ] : []),
  ];

  return (
    <div className="min-h-screen bg-brand-dark flex">
      {/* Sidebar */}
      <aside className="w-60 bg-brand-card border-r border-brand-border flex flex-col shrink-0">
        {/* Logo */}
        <div className="px-6 py-5 border-b border-brand-border">
          <Link href="/" className="flex items-center gap-2">
            <Film className="text-brand-red" size={20} />
            <span className="font-display text-xl tracking-wider text-white">
              FILMORA<span className="text-brand-red">UZ</span>
            </span>
          </Link>
          <p className="text-xs text-gray-600 mt-1 ml-7">Admin Panel</p>
        </div>

        {/* Nav */}
        <nav className="flex-1 px-3 py-4 space-y-1">
          {navItems.map((item) => {
            const active = pathname === item.href;
            return (
              <Link
                key={item.href}
                href={item.href}
                className={`flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm font-medium transition-colors ${
                  active
                    ? "bg-brand-red text-white"
                    : "text-gray-400 hover:text-white hover:bg-brand-border"
                }`}
              >
                <item.icon size={17} />
                {item.label}
              </Link>
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
      </aside>

      {/* Main content */}
      <div className="flex-1 overflow-auto">
        {children}
      </div>
    </div>
  );
}
