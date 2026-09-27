"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { RefreshCw, WifiOff, TriangleAlert } from "lucide-react";
import Navbar from "@/components/Navbar";
import SearchLauncher from "@/components/SearchLauncher";
import { reportError } from "@/lib/error-reporter";

// Route-level error boundary: friendly fallback + report to /admin/errors.
export default function RouteError({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  const [offline, setOffline] = useState(false);

  useEffect(() => {
    reportError("react", error);
    setOffline(typeof navigator !== "undefined" && navigator.onLine === false);
    const on = () => setOffline(false);
    window.addEventListener("online", on);
    return () => window.removeEventListener("online", on);
  }, [error]);

  return (
    <>
      <Navbar />
      <main className="flex min-h-[80vh] items-center justify-center px-4 pt-24">
        <div className="w-full max-w-lg rounded-3xl border border-white/10 bg-gradient-to-br from-red-600/10 via-[#101018] to-[#101018] p-8 text-center">
          <span className="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-white/5">
            {offline ? <WifiOff className="text-gray-300" /> : <TriangleAlert className="text-orange-400" />}
          </span>
          <h1 className="text-2xl font-semibold text-white">{offline ? "Internet aloqasi yo'q" : "Nimadir xato ketdi"}</h1>
          <p className="mt-2 text-sm text-gray-400">
            {offline
              ? "Ulanishni tekshiring — aloqa tiklanishi bilan qayta urinib ko'ring."
              : "Sahifani ko'rsatishda xatolik bo'ldi. Biz bu haqda xabar oldik va tuzatamiz."}
          </p>
          <div className="mt-6 flex flex-wrap justify-center gap-3">
            <button onClick={reset} className="inline-flex items-center gap-2 rounded-xl bg-orange-500 px-5 py-2.5 text-sm font-semibold text-white hover:bg-orange-600">
              <RefreshCw size={15} /> Qayta urinish
            </button>
            <Link href="/" className="rounded-xl border border-white/10 px-5 py-2.5 text-sm text-gray-200 hover:bg-white/5">
              Bosh sahifa
            </Link>
          </div>
          <div className="mt-6">
            <SearchLauncher />
          </div>
          {error.digest && <p className="mt-4 font-mono text-[11px] text-gray-600">Xato kodi: {error.digest}</p>}
        </div>
      </main>
    </>
  );
}
