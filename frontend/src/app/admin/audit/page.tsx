"use client";

import { Fragment, useEffect, useState } from "react";
import Link from "next/link";
import { ChevronDown, ChevronLeft, ChevronRight, Loader2, ScrollText, Search, X } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { getAdminAuditLogs, AuditLogEntry, AuditLogPage } from "@/lib/api";
import { readUrlBool, readUrlNumber, readUrlParam, useSyncUrlParams } from "@/lib/url-state";
import { ROLE_LABELS } from "@/lib/roles";

// Human-readable labels for the most common admin actions, keyed by
// "METHOD route-template". Unknown routes fall back to the raw route.
const ACTION_LABELS: Record<string, string> = {
  "POST /api/admin/movies": "Kino qo'shdi",
  "PUT /api/admin/movies/:id": "Kinoni tahrirladi",
  "DELETE /api/admin/movies/:id": "Kinoni o'chirdi",
  "PATCH /api/admin/movies/:id/approve": "Kinoni tasdiqladi",
  "PATCH /api/admin/movies/:id/reject": "Kinoni rad etdi",
  "POST /api/admin/movies/bulk-update": "Kinolarni ommaviy o'zgartirdi",
  "POST /api/admin/series": "Serial qo'shdi",
  "PUT /api/admin/series/:id": "Serialni tahrirladi",
  "DELETE /api/admin/series/:id": "Serialni o'chirdi",
  "DELETE /api/admin/series/:id/delete-cascade": "Serialni butunlay o'chirdi",
  "PATCH /api/admin/series/:id/approve": "Serialni tasdiqladi",
  "PATCH /api/admin/series/:id/reject": "Serialni rad etdi",
  "PUT /api/admin/episodes/:id": "Epizodni tahrirladi",
  "DELETE /api/admin/episodes/:id": "Epizodni o'chirdi",
  "PATCH /api/admin/users/:id/role/:role": "Rolni o'zgartirdi",
  "PATCH /api/admin/v1/users/:id/role": "Rolni o'zgartirdi",
  "PATCH /api/admin/users/:id/premium": "Premiumni o'zgartirdi",
  "POST /api/admin/users/:id/ban": "Ban qildi",
  "DELETE /api/admin/users/:id/ban": "Bandan chiqardi",
  "PATCH /api/superadmin/users/:id/wallet": "Hamyonni o'zgartirdi",
  "POST /api/admin/appeals/:id/review": "Apellyatsiyani ko'rib chiqdi",
  "PATCH /api/admin/suggestions/:id": "Tavsiyaga javob berdi",
  "PATCH /api/v1/admin/comments/:id/status": "Komment holatini o'zgartirdi",
  "DELETE /api/v1/admin/comments/:id": "Kommentni o'chirdi",
  "PUT /api/v1/admin/comment-settings": "Komment sozlamalarini o'zgartirdi",
  "POST /api/admin/collections": "Kolleksiya yaratdi",
  "PUT /api/admin/collections/:id": "Kolleksiyani tahrirladi",
  "DELETE /api/admin/collections/:id": "Kolleksiyani o'chirdi",
  "POST /api/admin/ingestion/import": "Katalogdan import qildi",
  "POST /api/admin/ingestion/manual": "Qo'lda import qildi",
  "POST /api/admin/ingestion/bulk-import": "Ommaviy import qildi",
  "POST /api/admin/ingestion/jobs/:id/retry": "Import jobni qayta ishga tushirdi",
  "DELETE /api/admin/ingestion/jobs/:id": "Import jobni o'chirdi",
  "POST /api/admin/clips/:id/publish/now": "Klipni publish qildi",
  "POST /api/admin/clips/:id/publish/schedule": "Klip publishini rejalashtirdi",
  "DELETE /api/admin/publish/jobs/:jobId": "Publishni bekor qildi",
  "PATCH /api/admin/publish/jobs/:jobId": "Publish vaqtini o'zgartirdi",
  "POST /api/superadmin/ads": "Reklama yaratdi",
  "PUT /api/superadmin/ads/:id": "Reklamani tahrirladi",
  "DELETE /api/superadmin/ads/:id": "Reklamani o'chirdi",
  "POST /api/superadmin/ads/:id/send-telegram": "Reklamani Telegramga yubordi",
  "POST /api/superadmin/telegram-post": "Telegram post yubordi",
  "POST /api/superadmin/expenses": "Xarajat qo'shdi",
  "DELETE /api/superadmin/expenses/:id": "Xarajatni o'chirdi",
  "POST /api/admin/announcements": "E'lon yaratdi",
  "PATCH /api/admin/announcements/:id": "E'lonni tahrirladi",
  "DELETE /api/admin/announcements/:id": "E'lonni o'chirdi",
};

const METHOD_STYLES: Record<string, string> = {
  POST: "bg-emerald-500/15 text-emerald-300",
  PUT: "bg-blue-500/15 text-blue-300",
  PATCH: "bg-amber-500/15 text-amber-300",
  DELETE: "bg-red-500/15 text-red-300",
};

function actorName(e: AuditLogEntry): string {
  const a = e.actor;
  const clean = (s?: string) => {
    const t = (s || "").trim();
    return t && t !== "." && t !== "-" ? t : "";
  };
  const full = clean([a?.first_name, a?.last_name].filter(Boolean).join(" "));
  return full || clean(a?.display_name) || (a?.username ? `@${a.username}` : "") || e.actor_id || "Noma'lum";
}

function formatTime(iso: string): string {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return "—";
  return new Intl.DateTimeFormat("uz-UZ", {
    timeZone: "Asia/Tashkent",
    day: "2-digit",
    month: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).format(d);
}

function targetLink(e: AuditLogEntry): { href: string; label: string } | null {
  const id = e.params?.id;
  if (!id) return null;
  if (e.route.startsWith("/api/admin/movies/")) return { href: `/admin/movies/${id}/edit`, label: `Kino ${id.slice(-6)}` };
  if (e.route.startsWith("/api/admin/series/")) return { href: `/admin/series/${id}/edit`, label: `Serial ${id.slice(-6)}` };
  if (e.route.includes("/users/:id")) return { href: `/admin/users?search=${id}`, label: `User ${id.slice(-6)}` };
  return null;
}

const METHODS = ["", "POST", "PUT", "PATCH", "DELETE"];

export default function AdminAuditPage() {
  const { token } = useAuth();
  const [qInput, setQInput] = useState(() => readUrlParam("q", ""));
  const [q, setQ] = useState(() => readUrlParam("q", "").trim());
  const [method, setMethod] = useState(() => readUrlParam("method", ""));
  const [failed, setFailed] = useState(() => readUrlBool("failed"));
  const [actorId, setActorId] = useState(() => readUrlParam("actor", ""));
  const [page, setPage] = useState(() => readUrlNumber("page", 1));
  const [data, setData] = useState<AuditLogPage | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [expanded, setExpanded] = useState<string | null>(null);

  useSyncUrlParams({ q, method, failed, actor: actorId, page }, { q: "", method: "", failed: false, actor: "", page: 1 });

  useEffect(() => {
    const next = qInput.trim();
    if (next === q) return;
    const t = setTimeout(() => {
      setQ(next);
      setPage(1);
    }, 300);
    return () => clearTimeout(t);
  }, [qInput, q]);

  useEffect(() => {
    if (!token) return;
    let cancelled = false;
    setLoading(true);
    getAdminAuditLogs(token, { page, limit: 50, q, method, failed, actor_id: actorId })
      .then((d) => {
        if (!cancelled) {
          setData(d);
          setError(null);
        }
      })
      .catch((e) => {
        if (!cancelled) setError(e instanceof Error ? e.message : "Xatolik");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [token, page, q, method, failed, actorId]);

  const entries = data?.data ?? [];
  const actorFilterName = actorId ? entries.find((e) => e.actor_id === actorId) : undefined;

  return (
    <div className="p-4 sm:p-8">
      <div className="mb-6">
        <h1 className="text-xl sm:text-2xl font-bold text-white flex items-center gap-2">
          <ScrollText size={22} className="text-brand-red" /> Audit log
        </h1>
        <p className="text-gray-500 text-sm mt-1">
          Adminlar qilgan barcha o&apos;zgarishlar: kim, qachon, nima. 180 kun saqlanadi.
        </p>
      </div>

      <div className="flex flex-col sm:flex-row flex-wrap gap-2 mb-4">
        <div className="relative flex-1 min-w-[220px]">
          <Search size={15} className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" />
          <input
            value={qInput}
            onChange={(e) => setQInput(e.target.value)}
            placeholder="Yo'l yoki ID bo'yicha (masalan: movies, 66f1...)"
            className="w-full bg-brand-card border border-brand-border rounded-lg pl-9 pr-3 py-2 text-sm text-white placeholder-gray-600 focus:outline-none focus:border-brand-red"
          />
        </div>
        <select
          value={method}
          onChange={(e) => {
            setMethod(e.target.value);
            setPage(1);
          }}
          className="bg-brand-card border border-brand-border rounded-lg px-3 py-2 text-sm text-white"
          aria-label="Metod"
        >
          {METHODS.map((m) => (
            <option key={m} value={m}>{m || "Barcha amallar"}</option>
          ))}
        </select>
        <label className="inline-flex items-center gap-2 px-3 py-2 text-sm text-gray-400 cursor-pointer select-none">
          <input
            type="checkbox"
            checked={failed}
            onChange={(e) => {
              setFailed(e.target.checked);
              setPage(1);
            }}
            className="accent-brand-red"
          />
          Faqat xatolar
        </label>
        {actorId && (
          <button
            onClick={() => {
              setActorId("");
              setPage(1);
            }}
            className="inline-flex items-center gap-1.5 rounded-lg border border-brand-border px-3 py-2 text-sm text-gray-300 hover:text-white"
          >
            <X size={14} /> {actorFilterName ? actorName(actorFilterName) : "Admin filtri"}
          </button>
        )}
      </div>

      <div className="bg-brand-card border border-brand-border rounded-xl overflow-hidden">
        {loading && !data ? (
          <div className="flex items-center justify-center gap-2 py-12 text-gray-500">
            <Loader2 size={18} className="animate-spin" /> Yuklanmoqda...
          </div>
        ) : error ? (
          <p className="py-12 text-center text-sm text-red-400">{error}</p>
        ) : entries.length === 0 ? (
          <p className="py-12 text-center text-sm text-gray-500">Yozuvlar topilmadi.</p>
        ) : (
          <div className={`overflow-x-auto ${loading ? "opacity-60" : ""}`}>
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-brand-border text-gray-500 text-xs uppercase tracking-wider">
                  <th className="text-left px-4 py-3">Vaqt</th>
                  <th className="text-left px-4 py-3">Admin</th>
                  <th className="text-left px-4 py-3">Amal</th>
                  <th className="text-left px-4 py-3 hidden md:table-cell">Obyekt</th>
                  <th className="text-right px-4 py-3">Natija</th>
                </tr>
              </thead>
              <tbody>
                {entries.map((e) => {
                  const key = `${e.method} ${e.route}`;
                  const label = ACTION_LABELS[key];
                  const target = targetLink(e);
                  const isOpen = expanded === e.id;
                  const ok = e.status < 400;
                  return (
                    <Fragment key={e.id}>
                      <tr
                        className="border-b border-brand-border/50 hover:bg-white/[0.02] cursor-pointer"
                        onClick={() => setExpanded(isOpen ? null : e.id)}
                        aria-expanded={isOpen}
                      >
                        <td className="px-4 py-2.5 text-gray-400 whitespace-nowrap tabular-nums">{formatTime(e.created_at)}</td>
                        <td className="px-4 py-2.5">
                          <button
                            onClick={(ev) => {
                              ev.stopPropagation();
                              setActorId(e.actor_id);
                              setPage(1);
                            }}
                            className="text-white hover:text-brand-red text-left"
                            title="Faqat shu admin amallari"
                          >
                            {actorName(e)}
                          </button>
                          <span className="ml-2 text-[11px] text-gray-500">{ROLE_LABELS[e.actor_role] ?? e.actor_role}</span>
                        </td>
                        <td className="px-4 py-2.5">
                          <div className="flex items-center gap-2">
                            <span className={`rounded px-1.5 py-0.5 text-[10px] font-semibold ${METHOD_STYLES[e.method] ?? "bg-white/10 text-gray-300"}`}>
                              {e.method}
                            </span>
                            <span className="text-gray-200">{label ?? e.route}</span>
                            <ChevronDown size={13} className={`text-gray-600 transition-transform ${isOpen ? "rotate-180" : ""}`} />
                          </div>
                        </td>
                        <td className="px-4 py-2.5 hidden md:table-cell">
                          {target ? (
                            <Link href={target.href} onClick={(ev) => ev.stopPropagation()} className="text-blue-300 hover:underline font-mono text-xs">
                              {target.label}
                            </Link>
                          ) : (
                            <span className="text-gray-600">—</span>
                          )}
                        </td>
                        <td className="px-4 py-2.5 text-right">
                          <span className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${ok ? "bg-emerald-500/10 text-emerald-300" : "bg-red-500/10 text-red-300"}`}>
                            {ok ? "OK" : "Xato"} {e.status}
                          </span>
                        </td>
                      </tr>
                      {isOpen && (
                        <tr className="border-b border-brand-border/50 bg-black/20">
                          <td colSpan={5} className="px-4 py-3">
                            <dl className="grid grid-cols-1 md:grid-cols-2 gap-x-6 gap-y-1 text-xs">
                              <div><dt className="inline text-gray-500">Yo&apos;l: </dt><dd className="inline font-mono text-gray-300 break-all">{e.path}</dd></div>
                              <div><dt className="inline text-gray-500">Davomiyligi: </dt><dd className="inline text-gray-300">{e.duration_ms} ms</dd></div>
                              <div><dt className="inline text-gray-500">IP: </dt><dd className="inline font-mono text-gray-300">{e.ip || "—"}</dd></div>
                              <div className="truncate"><dt className="inline text-gray-500">Qurilma: </dt><dd className="inline text-gray-300" title={e.user_agent}>{e.user_agent || "—"}</dd></div>
                            </dl>
                            {(e.params && Object.keys(e.params).length > 0) || e.body ? (
                              <pre className="mt-2 max-h-64 overflow-auto rounded-lg bg-black/40 p-3 text-[11px] text-gray-300">
                                {JSON.stringify({ params: e.params, body: e.body }, null, 2)}
                              </pre>
                            ) : null}
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

      {data && data.total_pages > 1 && (
        <div className="flex items-center justify-between mt-4 text-sm text-gray-500">
          <span>Jami: {data.total.toLocaleString("uz-UZ")} ta yozuv</span>
          <div className="flex items-center gap-2">
            <button
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              disabled={page <= 1}
              className="p-2 rounded-lg bg-brand-card border border-brand-border text-white disabled:opacity-40"
              aria-label="Oldingi sahifa"
            >
              <ChevronLeft size={16} />
            </button>
            <span className="text-white">{page} / {data.total_pages}</span>
            <button
              onClick={() => setPage((p) => Math.min(data.total_pages, p + 1))}
              disabled={page >= data.total_pages}
              className="p-2 rounded-lg bg-brand-card border border-brand-border text-white disabled:opacity-40"
              aria-label="Keyingi sahifa"
            >
              <ChevronRight size={16} />
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
