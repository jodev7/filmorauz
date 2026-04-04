"use client";

import BanGuard from "./BanGuard";

/**
 * BanGuardWrapper - Client component wrapper for BanGuard
 * This wraps the entire application to enforce ban restrictions globally
 */
export default function BanGuardWrapper({ children }: { children: React.ReactNode }) {
  return (
    <BanGuard excludePaths={["/banned", "/admin"]}>
      {children}
    </BanGuard>
  );
}
