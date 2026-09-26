import {
  Activity,
  Ban,
  Bell,
  Download,
  Folder,
  FolderHeart,
  Globe,
  History,
  LayoutDashboard,
  Lightbulb,
  List,
  Megaphone,
  MessageCircle,
  MessageSquare,
  PlusCircle,
  Send,
  Settings,
  Tv,
  Users,
  Video,
  Wallet,
  type LucideIcon,
} from "lucide-react";
import type { AdminBadges } from "@/lib/api";

export type AdminRole = "admin" | "superadmin" | string | undefined;

export interface AdminNavItem {
  href: string;
  icon: LucideIcon;
  label: string;
  // Extra words the command palette matches on.
  keywords?: string;
  // Which attention counter to show as a badge next to the item.
  badge?: keyof AdminBadges;
  superadminOnly?: boolean;
}

export interface AdminNavGroup {
  id: string;
  label: string;
  items: AdminNavItem[];
}

export const ADMIN_NAV: AdminNavGroup[] = [
  {
    id: "overview",
    label: "Umumiy",
    items: [
      { href: "/admin/dashboard", icon: LayoutDashboard, label: "Dashboard", keywords: "bosh panel statistika" },
      { href: "/admin/analytics", icon: Activity, label: "Analitika", keywords: "qidiruv funnel hisobot" },
    ],
  },
  {
    id: "content",
    label: "Kontent",
    items: [
      { href: "/admin/movies", icon: List, label: "Movies", keywords: "kinolar filmlar", badge: "pending_approvals" },
      { href: "/admin/movies/new", icon: PlusCircle, label: "Add Movie", keywords: "yangi kino qo'shish" },
      { href: "/admin/series", icon: Tv, label: "Series", keywords: "seriallar" },
      { href: "/admin/series/new", icon: PlusCircle, label: "Add Series", keywords: "yangi serial qo'shish" },
      { href: "/admin/collections", icon: FolderHeart, label: "Collections", keywords: "kolleksiyalar to'plam" },
      { href: "/admin/ingestion", icon: Download, label: "Import Movies", keywords: "ingestion import yuklash" },
      { href: "/admin/clips", icon: Video, label: "Clips", keywords: "kliplar instagram publish", badge: "failed_publish_jobs_7d" },
      { href: "/admin/content", icon: Folder, label: "Content", keywords: "papkalar" },
      { href: "/admin/seo", icon: Globe, label: "SEO" },
    ],
  },
  {
    id: "community",
    label: "Foydalanuvchilar",
    items: [
      { href: "/admin/users", icon: Users, label: "Users", keywords: "foydalanuvchilar premium" },
      { href: "/admin/users/banned", icon: Ban, label: "Ban olganlar" },
      { href: "/admin/users/ban-history", icon: History, label: "Ban tarixi" },
      { href: "/admin/appeals", icon: MessageCircle, label: "Apellyatsiyalar", keywords: "appeals", badge: "pending_appeals" },
      { href: "/admin/suggestions", icon: Lightbulb, label: "Tavsiyalar", keywords: "suggestions", badge: "pending_suggestions" },
      { href: "/admin/comments", icon: MessageSquare, label: "Comments", keywords: "kommentlar izohlar moderatsiya", badge: "pending_comments" },
      { href: "/admin/comments/settings", icon: Settings, label: "Comment Settings" },
      { href: "/admin/rooms", icon: Users, label: "Watch Rooms", keywords: "xonalar" },
    ],
  },
  {
    id: "marketing",
    label: "Marketing",
    items: [
      { href: "/admin/telegram-post", icon: Send, label: "Telegram Post", keywords: "kanal post" },
      { href: "/admin/ads", icon: Megaphone, label: "Ads", keywords: "reklama", superadminOnly: true },
      { href: "/admin/announcements", icon: Bell, label: "E'lonlar", keywords: "announcements", superadminOnly: true },
    ],
  },
  {
    id: "system",
    label: "Tizim",
    items: [
      { href: "/admin/expenses", icon: Wallet, label: "Xarajatlar", keywords: "expenses moliya", superadminOnly: true },
    ],
  },
];

export function visibleNav(role: AdminRole): AdminNavGroup[] {
  const isSuperAdmin = role === "superadmin";
  return ADMIN_NAV.map((g) => ({
    ...g,
    items: g.items.filter((i) => !i.superadminOnly || isSuperAdmin),
  })).filter((g) => g.items.length > 0);
}

// Longest-prefix match so /admin/movies/123/edit highlights "Movies" but
// /admin/movies/new highlights "Add Movie".
export function activeHref(pathname: string, groups: AdminNavGroup[]): string | null {
  let best: string | null = null;
  for (const g of groups) {
    for (const i of g.items) {
      if ((pathname === i.href || pathname.startsWith(i.href + "/")) && (!best || i.href.length > best.length)) {
        best = i.href;
      }
    }
  }
  return best;
}
