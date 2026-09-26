"use client";

import { Fragment, useCallback, useEffect, useState } from "react";
import { Bug, CheckCircle2, ChevronDown, Loader2, Monitor, RefreshCw, Server } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { getAdminErrors, resolveAdminError, ErrorGroup } from "@/lib/api";
import { useToast } from "@/components/admin/Toast";

function ago(iso: string): string {
  const diff = (Date.now() - new Date(iso).getTime()) / 1000;
  if (!isFinite(diff)) return "—";
  if (diff < 60) return "hozir";
  if (diff < 3600) return `${Math.floor(diff / 60)} daq oldin`;
  if (diff < 86400) return `${Math.floor(diff / 3600)} soat oldin`;
  return `${Math.floor(diff / 86400)} kun oldin`;
}

const KIND_LABEL: Record<string, string> = {
  window: "JS xato",
  promise: "Promise",
  react: "React",
  panic: "Panic",
  http5xx: "5xx javob",
};

export default function AdminErrorsPage() {
  const { token } = useAuth();
  const toast = useToast();
  const [source, setSource] = useState<"" | "client" | "server">("");
  const [showResolved, setShowResolved] = useState(false);
  const [rows, setRows] = useState<ErrorGroup[] | null>(null);
  const [open, setOpen] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  const load = useCallback(() => {
    if (!token) return;
    setRows(null);
    getAdminErrors(token, source, showResolved)
      .then(setRows)
      .catch(() => {
        setRows([]);
        toast.error("Xatolar ro'yxatini yuklab bo'lmadi");
      });
  }, [token, source, showResolved, toast]);

  useEffect(() => {
    load();
  }, [load]);

  const resolve = async (g: ErrorGroup) => {
    if (!token) return;
    setBusy(g.id);
    try {
      await resolveAdminError(token, g.id);
      setRows((prev) => (prev || []).filter((r) => showResolved || r.id !== g.id).map((r) => (r.id === g.id ? { ...r, resolved: true } : r)));
      toast.success("Hal qilindi deb belgilandi — qaytalansa yana ochiladi");
    } catch {
      toast.error("Belgilab bo'lmadi");
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="p-4 sm:p-8">
      <div className="mb-6 flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-bold text-white sm:text-2xl">
            <Bug size={22} className="text-brand-red" /> Xatolar
          </h1>
          <p className="mt-1 text-sm text-gray-500">
            Saytdagi (brauzer) va serverdagi xatolar, bir xillari guruhlangan. 30 kun saqlanadi.
          </p>
        </div>
        <button onClick={load} className="inline-flex items-center gap-1.5 rounded-lg border border-brand-border px-3 py-2 text-sm text-gray-300 hover:text-white">
          <RefreshCw size={14} /> Yangilash
        </button>
      </div>

      <div className="mb-4 flex flex-wrap items-center gap-2">
        {([["", "Hammasi"], ["client", "Sayt (brauzer)"], ["server", "Server"]] as const).map(([v, label]) => (
          <button
            key={v || "all"}
            onClick={() => setSource(v)}
            aria-pressed={source === v}
            className={`rounded-lg px-3 py-1.5 text-sm ${source === v ? "bg-white text-black" : "border border-brand-border text-gray-400 hover:text-white"}`}
          >
            {label}
          </button>
        ))}
        <label className="ml-2 inline-flex cursor-pointer select-none items-center gap-2 text-sm text-gray-400">
          <input type="checkbox" checked={showResolved} onChange={(e) => setShowResolved(e.target.checked)} className="accent-brand-red" />
          Hal qilinganlar ham
        </label>
      </div>

      <div className="overflow-hidden rounded-xl border border-brand-border bg-brand-card">
        {rows === null ? (
          <div className="flex items-center justify-center gap-2 py-12 text-gray-500">
            <Loader2 size={18} className="animate-spin" /> Yuklanmoqda...
          </div>
        ) : rows.length === 0 ? (
          <p className="flex items-center justify-center gap-2 py-12 text-sm text-emerald-400">
            <CheckCircle2 size={16} /> Ochiq xatolar yo&apos;q
          </p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-brand-border text-xs uppercase tracking-wider text-gray-500">
                  <th className="px-4 py-3 text-left">Xato</th>
                  <th className="px-4 py-3 text-right">Soni</th>
                  <th className="hidden px-4 py-3 text-right md:table-cell">Foydalanuvchi</th>
                  <th className="px-4 py-3 text-right">Oxirgi</th>
                  <th className="px-4 py-3" />
                </tr>
              </thead>
              <tbody>
                {rows.map((g) => {
                  const isOpen = open === g.id;
                  return (
                    <Fragment key={g.id}>
                      <tr className={`border-b border-brand-border/50 hover:bg-white/[0.02] ${g.resolved ? "opacity-50" : ""}`}>
                        <td className="max-w-[520px] px-4 py-2.5">
                          <button onClick={() => setOpen(isOpen ? null : g.id)} className="flex w-full items-start gap-2 text-left" aria-expanded={isOpen}>
                            {g.source === "server" ? (
                              <Server size={14} className="mt-0.5 shrink-0 text-purple-400" aria-label="Server" />
                            ) : (
                              <Monitor size={14} className="mt-0.5 shrink-0 text-blue-400" aria-label="Brauzer" />
                            )}
                            <span className="min-w-0 flex-1">
                              <span className="block truncate font-mono text-xs text-gray-100">{g.message}</span>
                              <span className="mt-0.5 block truncate text-[11px] text-gray-500">
                                {KIND_LABEL[g.kind] ?? g.kind} · {g.last_url}
                              </span>
                            </span>
                            <ChevronDown size={13} className={`mt-0.5 shrink-0 text-gray-600 transition-transform ${isOpen ? "rotate-180" : ""}`} />
                          </button>
                        </td>
                        <td className="px-4 py-2.5 text-right font-semibold tabular-nums text-white">{g.count.toLocaleString("uz-UZ")}</td>
                        <td className="hidden px-4 py-2.5 text-right tabular-nums text-gray-400 md:table-cell">{g.user_count || "—"}</td>
                        <td className="whitespace-nowrap px-4 py-2.5 text-right text-gray-400">{ago(g.last_seen)}</td>
                        <td className="px-4 py-2.5 text-right">
                          {!g.resolved && (
                            <button
                              onClick={() => resolve(g)}
                              disabled={busy === g.id}
                              className="inline-flex items-center gap-1 rounded-lg bg-green-500/15 px-2.5 py-1 text-xs text-green-300 hover:bg-green-500/25 disabled:opacity-50"
                            >
                              {busy === g.id ? <Loader2 size={12} className="animate-spin" /> : <CheckCircle2 size={12} />} Hal qilindi
                            </button>
                          )}
                        </td>
                      </tr>
                      {isOpen && (
                        <tr className="border-b border-brand-border/50 bg-black/20">
                          <td colSpan={5} className="px-4 py-3">
                            <div className="mb-2 flex flex-wrap gap-x-6 gap-y-1 text-xs text-gray-400">
                              <span>Birinchi: {new Date(g.first_seen).toLocaleString("uz-UZ")}</span>
                              <span>Oxirgi: {new Date(g.last_seen).toLocaleString("uz-UZ")}</span>
                              {g.release && <span>Versiya: {g.release}</span>}
                              {g.last_user_agent && <span className="truncate">Qurilma: {g.last_user_agent}</span>}
                            </div>
                            {g.stack ? (
                              <pre className="max-h-72 overflow-auto rounded-lg bg-black/40 p-3 text-[11px] leading-relaxed text-gray-300">{g.stack}</pre>
                            ) : (
                              <p className="text-xs text-gray-500">Stack mavjud emas.</p>
                            )}
                          </td>
                        </tr>
                      )}
                    </Fragment>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
