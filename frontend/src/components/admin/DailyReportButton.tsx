"use client";

import { useEffect, useState } from "react";
import { FileBarChart, Loader2, Send, X } from "lucide-react";
import { getDailyReportPreview, sendDailyReport } from "@/lib/api";
import { useToast } from "@/components/admin/Toast";

// Telegram HTML → plain text for the preview (never injected as HTML).
function htmlToText(html: string): string {
  if (typeof window === "undefined") return html;
  return new DOMParser().parseFromString(html, "text/html").body.textContent || "";
}

/**
 * Superadmin-only: preview yesterday's daily Telegram report and send it
 * now. The same report goes out automatically every morning.
 */
export default function DailyReportButton({ token }: { token: string }) {
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [preview, setPreview] = useState<{ text: string; recipients: number } | null>(null);
  const [loading, setLoading] = useState(false);
  const [sending, setSending] = useState(false);

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    setLoading(true);
    getDailyReportPreview(token)
      .then((r) => !cancelled && setPreview({ text: htmlToText(r.html), recipients: r.recipients }))
      .catch((err) => !cancelled && toast.error(err instanceof Error ? err.message : "Xatolik"))
      .finally(() => !cancelled && setLoading(false));
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, token]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && !sending && setOpen(false);
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, sending]);

  const send = async () => {
    setSending(true);
    try {
      const r = await sendDailyReport(token);
      if (r.sent > 0) toast.success(`Hisobot ${r.sent} ta chatga yuborildi`);
      else toast.error("Hech kimga yetib bormadi — bot bilan chat ochilganini tekshiring");
      if (r.sent > 0) setOpen(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Yuborib bo'lmadi");
    } finally {
      setSending(false);
    }
  };

  return (
    <>
      <button
        onClick={() => setOpen(true)}
        className="inline-flex items-center gap-2 rounded-lg border border-brand-border bg-brand-card px-3 py-2 text-sm text-gray-300 hover:border-gray-500 hover:text-white"
      >
        <FileBarChart size={15} className="text-sky-400" />
        Kunlik hisobot
      </button>

      {open && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4"
          onClick={() => !sending && setOpen(false)}
          role="dialog"
          aria-modal="true"
          aria-labelledby="daily-report-title"
        >
          <div
            className="flex max-h-[90vh] w-full max-w-md flex-col rounded-2xl border border-brand-border bg-brand-card p-5 shadow-2xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="mb-3 flex items-start justify-between gap-3">
              <div>
                <h2 id="daily-report-title" className="text-base font-semibold text-white">Kunlik Telegram hisobot</h2>
                <p className="mt-1 text-xs text-gray-500">
                  Har kuni ertalab superadminlarga avtomatik yuboriladi. Quyida kechagi kun hisoboti.
                </p>
              </div>
              <button
                onClick={() => setOpen(false)}
                disabled={sending}
                className="rounded-lg p-1.5 text-gray-500 hover:bg-white/5 hover:text-white"
                aria-label="Yopish"
              >
                <X size={16} />
              </button>
            </div>

            <div className="min-h-[160px] flex-1 overflow-y-auto rounded-xl border border-brand-border bg-brand-dark p-3">
              {loading || !preview ? (
                <div className="flex h-40 items-center justify-center text-gray-500">
                  <Loader2 size={18} className="animate-spin" />
                </div>
              ) : (
                <pre className="whitespace-pre-wrap break-words font-body text-[13px] leading-relaxed text-gray-200">{preview.text}</pre>
              )}
            </div>

            <div className="mt-4 flex items-center justify-between gap-2">
              <span className="text-xs text-gray-500">
                {preview ? `${preview.recipients} ta qabul qiluvchi` : ""}
              </span>
              <button
                onClick={send}
                disabled={sending || loading || !preview || preview.recipients === 0}
                className="inline-flex items-center gap-1.5 rounded-lg bg-sky-600 px-4 py-2 text-sm font-medium text-white hover:bg-sky-500 disabled:opacity-50"
              >
                {sending ? <Loader2 size={14} className="animate-spin" /> : <Send size={14} />}
                Hozir yuborish
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
