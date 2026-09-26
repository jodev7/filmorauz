// Staff roles for the admin panel. The backend is the source of truth
// (middleware/roles.go allowlists moderator routes); these helpers only
// decide what the UI shows.

export type StaffRole = "moderator" | "admin" | "superadmin";

function norm(role?: string | null): string {
  return (role || "").trim().toLowerCase();
}

export function isSuperAdminRole(role?: string | null): boolean {
  return norm(role) === "superadmin";
}

/** admin or superadmin — full content/admin access. */
export function isFullAdminRole(role?: string | null): boolean {
  const r = norm(role);
  return r === "admin" || r === "superadmin";
}

export function isModeratorRole(role?: string | null): boolean {
  return norm(role) === "moderator";
}

/** Anyone allowed into /admin at all. */
export function isStaffRole(role?: string | null): boolean {
  return isFullAdminRole(role) || isModeratorRole(role);
}

/** Pages a moderator may open (prefix match). */
export const MODERATOR_PATHS = [
  "/admin/comments",
  "/admin/appeals",
  "/admin/suggestions",
  "/admin/users",
];

export const MODERATOR_HOME = "/admin/comments";

export function moderatorCanOpen(pathname: string): boolean {
  if (pathname === "/admin/comments/settings") return false;
  return MODERATOR_PATHS.some((p) => pathname === p || pathname.startsWith(p + "/"));
}

export const ROLE_LABELS: Record<string, string> = {
  user: "Foydalanuvchi",
  moderator: "Moderator",
  admin: "Admin",
  superadmin: "Super Admin",
};
