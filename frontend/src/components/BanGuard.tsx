"use client";

import { useEffect } from "react";
import { useRouter, usePathname } from "next/navigation";
import { useAuth } from "@/lib/auth-context";

interface BanGuardProps {
  children: React.ReactNode;
  excludePaths?: string[];
}

/**
 * BanGuard - A wrapper component that redirects banned users to /banned
 * 
 * Usage:
 * <BanGuard excludePaths={["/banned", "/admin"]}>
 *   {children}
 * </BanGuard>
 * 
 * The excludePaths option allows you to exclude certain paths from the ban check.
 * For example, /banned should be accessible to banned users so they can see the ban message.
 * /admin paths are excluded because admin middleware handles ban checks separately.
 */
export default function BanGuard({ children, excludePaths = [] }: BanGuardProps) {
  const { user, isLoading, isBanned, token } = useAuth();
  const router = useRouter();
  const pathname = usePathname();

  useEffect(() => {
    // Only check when auth is loaded and user exists
    if (isLoading) return;
    
    // Skip check for excluded paths
    const isExcluded = excludePaths.some(path => pathname.startsWith(path));
    if (isExcluded) return;

    // If user is logged in and is banned, redirect to /banned
    if (user && isBanned && pathname !== "/banned") {
      router.push("/banned");
    }
  }, [user, isLoading, isBanned, pathname, router, excludePaths]);

  // Show loading state while checking auth
  if (isLoading) {
    return (
      <div className="min-h-screen bg-brand-dark flex items-center justify-center">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-brand-red"></div>
      </div>
    );
  }

  // If user is banned and not on excluded path, don't render children
  // The redirect will happen via the useEffect
  if (user && isBanned && pathname !== "/banned") {
    return (
      <div className="min-h-screen bg-brand-dark flex items-center justify-center">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-brand-red"></div>
      </div>
    );
  }

  return <>{children}</>;
}
