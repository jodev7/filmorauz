"use client";

import { useEffect } from "react";
import { reportError } from "@/lib/error-reporter";

// Last-resort boundary for errors in the root layout itself.
export default function GlobalError({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  useEffect(() => {
    reportError("react", error);
  }, [error]);

  return (
    <html lang="uz">
      <body style={{ margin: 0, minHeight: "100vh", display: "flex", alignItems: "center", justifyContent: "center", background: "#0A0A0F", color: "#fff", fontFamily: "system-ui, sans-serif", padding: 16 }}>
        <div style={{ textAlign: "center", maxWidth: 360 }}>
          <h1 style={{ fontSize: 22, margin: "0 0 8px" }}>Nimadir xato ketdi</h1>
          <p style={{ color: "#9CA3AF", fontSize: 14, margin: "0 0 20px" }}>Xatolik haqida xabar yuborildi.</p>
          <button onClick={reset} style={{ background: "#F97316", color: "#fff", border: 0, borderRadius: 12, padding: "12px 20px", fontWeight: 600, cursor: "pointer" }}>
            Qayta urinish
          </button>
        </div>
      </body>
    </html>
  );
}
