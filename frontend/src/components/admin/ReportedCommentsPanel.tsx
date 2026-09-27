"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { Flag, Check, Trash2, Loader2, EyeOff } from "lucide-react";
import {
  getReportedComments,
  dismissCommentReports,
  adminDeleteComment,
  REPORT_REASON_LABELS,
  ReportedComment,
} from "@/lib/comments-api";
import { useToast } from "@/components/admin/Toast";

/**
 * Moderation queue for user-reported comments ("shikoyatlar"). Comments with
 * 3+ reports are already hidden from the site until a decision is made here.
 */
export default function ReportedCommentsPanel({ token, onChanged }: { token: string; onChanged?: () => void }) {
  const toast = useToast();
  const [rows, setRows] = useState<ReportedComment[] | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  const load = useCallback(() => {
    getReportedComments(token)
      .then(setRows)
      .catch(() => setRows([]));
  }, [token]);

  useEffect(() => {
    load();
  }, [load]);

  if (!rows || rows.length === 0) return null;

  const act = async (row: ReportedComment, action: "dismiss" | "delete") => {
    if (action === "delete" && !window.confirm("Izoh o'chirilsinmi?")) return;
    setBusy(row.id);
    try {
      if (action === "dismiss") await dismissCommentReports(token, row.id);
      else await adminDeleteComment(token, row.id);
      setRows((prev) => (prev || []).filter((r) => r.id !== row.id));
      toast.success(action === "dismiss" ? "Shikoyat yopildi, izoh qoldirildi" : "Izoh o'chirildi");
      onChanged?.();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Xatolik");
    } finally {
      setBusy(null);
    }
  };

  const targetHref = (r: ReportedComment) =>
    r.target_slug ? (r.target_type === "episode" ? `/series/${r.target_slug}` : `/movies/${r.target_slug}`) : null;

  return (
    <section className="mb-8 rounded-xl border border-amber-500/30 bg-amber-500/5 p-4">
      <h2 className="mb-3 flex items-center gap-2 text-base font-semibold text-white">
        <Flag size={16} className="text-amber-400" /> Shikoyat qilingan izohlar
        <span className="rounded-full bg-amber-500/20 px-2 text-xs text-amber-300">{rows.length}</span>
      </h2>
      <ul className="divide-y divide-white/10">
        {rows.map((r) => {
          const href = targetHref(r);
          return (
            <li key={r.id} className="flex flex-col gap-2 py-3 sm:flex-row sm:items-start">
              <div className="min-w-0 flex-1">
                <p className="text-sm text-gray-200 break-words">{r.content}</p>
                <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-gray-500">
                  <Link href={`/admin/users?search=${r.author_id}`} className="text-gray-300 hover:text-white">
                    {r.author_name || "Foydalanuvchi"}
                  </Link>
                  {href && (
                    <>
                      <span>·</span>
                      <Link href={href} target="_blank" className="hover:text-white">
                        {r.target_title || "kontent"}
                      </Link>
                    </>
                  )}
                  <span>·</span>
                  <span className="font-semibold text-amber-300">{r.reports_count} ta shikoyat</span>
                  {(r.reasons || []).map((reason) => (
                    <span key={reason} className="rounded-full bg-white/10 px-2 py-0.5 text-gray-300">
                      {REPORT_REASON_LABELS[reason] ?? reason}
                    </span>
                  ))}
                  {r.auto_hidden && (
                    <span className="inline-flex items-center gap-1 rounded-full bg-red-500/15 px-2 py-0.5 text-red-300">
                      <EyeOff size={11} /> Saytdan yashirilgan
                    </span>
                  )}
                </div>
              </div>
              <div className="flex shrink-0 gap-2">
                <button
                  onClick={() => act(r, "dismiss")}
                  disabled={busy === r.id}
                  className="inline-flex items-center gap-1.5 rounded-lg bg-green-500/15 px-3 py-1.5 text-xs text-green-300 hover:bg-green-500/25 disabled:opacity-50"
                  title="Izoh qoidaga zid emas — shikoyatni yopish"
                >
                  {busy === r.id ? <Loader2 size={13} className="animate-spin" /> : <Check size={13} />} Qoldirish
                </button>
                <button
                  onClick={() => act(r, "delete")}
                  disabled={busy === r.id}
                  className="inline-flex items-center gap-1.5 rounded-lg bg-red-500/15 px-3 py-1.5 text-xs text-red-300 hover:bg-red-500/25 disabled:opacity-50"
                >
                  <Trash2 size={13} /> O&apos;chirish
                </button>
              </div>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
