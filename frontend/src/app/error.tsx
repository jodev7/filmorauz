"use client";

import { useEffect } from "react";
import Link from "next/link";
import { reportError } from "@/lib/error-reporter";

// Route-level error boundary: friendly fallback + report to /admin/errors.
export default function RouteError({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  useEffect(() => {
    reportError("react", error);
  }, [error]);

  return (
    <main className="flex min-h-[70vh] items-center justify-center px-4">
      <div className="max-w-md text-center">
        <h1 className="font-display text-3xl text-white">Nimadir xato ketdi</h1>
        <p className="mt-2 text-sm text-gray-400">
          Xatolik haqida xabar avtomatik yuborildi. Sahifani qayta yuklab ko&apos;ring.
        </p>
        <div className="mt-6 flex justify-center gap-3">
          <button onClick={reset} className="rounded-xl bg-brand-red px-5 py-2.5 text-sm font-semibold text-white hover:bg-orange-600">
            Qayta urinish
          </button>
          <Link href="/" className="rounded-xl border border-white/10 px-5 py-2.5 text-sm text-gray-200 hover:border-white/30">
            Bosh sahifa
          </Link>
        </div>
      </div>
    </main>
  );
}
