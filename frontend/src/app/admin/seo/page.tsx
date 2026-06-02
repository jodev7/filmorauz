"use client";

import { useCallback, useEffect, useState } from "react";
import { useAuth } from "@/lib/auth-context";
import { RefreshCw, Send, Globe, CheckCircle2, XCircle, Clock } from "lucide-react";

const API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080/api";

type Provider = "indexnow" | "google_indexing" | "search_console";

interface SeoEvent {
  provider: Provider;
  action: string;
  url?: string;
  sitemap?: string;
  status: "ok" | "error" | "skipped";
  error?: string;
  count?: number;
  created_at: string;
}

interface SeoStatus {
  enabled: boolean;
  indexnow_configured: boolean;
  google_indexing_configured: boolean;
  search_console_configured: boolean;
  indexnow_key?: string;
  site_url: string;
}

interface StatusResponse {
  status?: SeoStatus;
  recent_events?: SeoEvent[];
  enabled?: boolean;
}

export default function AdminSEOPage() {
  const { token } = useAuth();
  const [data, setData] = useState<StatusResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [working, setWorking] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [reindexInput, setReindexInput] = useState("");

  const load = useCallback(async () => {
    if (!token) return;
    setLoading(true);
    try {
      const res = await fetch(`${API_URL}/admin/seo/status`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      const json = await res.json();
      setData(json);
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  }, [token]);

  useEffect(() => {
    load();
  }, [load]);

  const callAction = async (path: string, body?: any) => {
    if (!token) return;
    setWorking(true);
    setMessage(null);
    try {
      const res = await fetch(`${API_URL}/admin/seo${path}`, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${token}`,
          "Content-Type": "application/json",
        },
        body: body ? JSON.stringify(body) : undefined,
      });
      const json = await res.json().catch(() => ({}));
      if (!res.ok) {
        setMessage(`Xato: ${json?.error || res.statusText}`);
      } else {
        setMessage(`Bajarildi: ${JSON.stringify(json)}`);
      }
      await load();
    } catch (e: any) {
      setMessage(`Xato: ${e?.message || e}`);
    } finally {
      setWorking(false);
    }
  };

  const onReindex = () => {
    const urls = reindexInput
      .split(/\r?\n/)
      .map((s) => s.trim())
      .filter(Boolean);
    if (urls.length === 0) {
      setMessage("Kamida bitta URL kiriting");
      return;
    }
    callAction("/reindex", { urls });
  };

  const status = data?.status;
  const events = data?.recent_events ?? [];

  return (
    <div className="p-6 max-w-6xl mx-auto text-white">
      <h1 className="text-2xl font-semibold mb-2 flex items-center gap-2">
        <Globe className="w-6 h-6" /> SEO Indexing
      </h1>
      <p className="text-white/60 mb-6 text-sm">
        Google / Bing / Yandex avtomatik indekslash holati va qo&apos;lda re-ping.
      </p>

      {/* Status cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
        <StatusCard label="Notifier" enabled={!!status?.enabled} />
        <StatusCard label="IndexNow (Bing/Yandex)" enabled={!!status?.indexnow_configured} />
        <StatusCard label="Google Indexing API" enabled={!!status?.google_indexing_configured} />
        <StatusCard label="Search Console" enabled={!!status?.search_console_configured} />
      </div>

      {status?.indexnow_key && (
        <p className="text-xs text-white/50 mb-4">
          IndexNow key:{" "}
          <code className="bg-white/10 px-2 py-1 rounded">{status.indexnow_key}</code>
          {" — "}
          <code>/{status.indexnow_key}.txt</code> public&apos;da mavjud bo&apos;lishi shart.
        </p>
      )}

      {/* Actions */}
      <div className="bg-white/5 rounded-lg p-4 mb-6 space-y-4">
        <div className="flex flex-wrap gap-3">
          <button
            disabled={working}
            onClick={() => callAction("/reindex/all")}
            className="px-4 py-2 bg-blue-600 hover:bg-blue-500 rounded flex items-center gap-2 disabled:opacity-50"
          >
            <Send className="w-4 h-4" /> Hamma kontentni qayta yuborish
          </button>
          <button
            disabled={working}
            onClick={() => callAction("/sitemap-resubmit")}
            className="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 rounded flex items-center gap-2 disabled:opacity-50"
          >
            <RefreshCw className="w-4 h-4" /> Sitemap&apos;ni qayta yuborish
          </button>
          <button
            disabled={working}
            onClick={load}
            className="px-4 py-2 bg-white/10 hover:bg-white/20 rounded flex items-center gap-2"
          >
            <RefreshCw className="w-4 h-4" /> Yangilash
          </button>
        </div>

        <div>
          <label className="block text-sm text-white/70 mb-2">
            Aniq URL&apos;larni qayta yuborish (har qatorda bittadan)
          </label>
          <textarea
            value={reindexInput}
            onChange={(e) => setReindexInput(e.target.value)}
            rows={4}
            className="w-full bg-black/40 border border-white/10 rounded p-2 text-sm font-mono"
            placeholder={`/movies/example-slug\n/series/another-series`}
          />
          <button
            disabled={working}
            onClick={onReindex}
            className="mt-2 px-4 py-2 bg-purple-600 hover:bg-purple-500 rounded flex items-center gap-2 disabled:opacity-50"
          >
            <Send className="w-4 h-4" /> Yuborish
          </button>
        </div>

        {message && (
          <div className="text-sm text-white/80 bg-black/30 border border-white/10 rounded p-3">
            {message}
          </div>
        )}
      </div>

      {/* Events log */}
      <h2 className="text-lg font-semibold mb-3">So&apos;nggi hodisalar</h2>
      <div className="bg-white/5 rounded-lg overflow-hidden">
        <div className="max-h-[60vh] overflow-y-auto">
        <table className="w-full text-sm">
          <thead className="bg-white/10 text-left sticky top-0 z-10">
            <tr>
              <th className="p-3">Vaqt</th>
              <th className="p-3">Provider</th>
              <th className="p-3">Action</th>
              <th className="p-3">URL / Sitemap</th>
              <th className="p-3">Status</th>
            </tr>
          </thead>
          <tbody>
            {loading && (
              <tr>
                <td colSpan={5} className="p-4 text-center text-white/50">
                  Yuklanmoqda…
                </td>
              </tr>
            )}
            {!loading && events.length === 0 && (
              <tr>
                <td colSpan={5} className="p-4 text-center text-white/50">
                  Hodisalar yo&apos;q
                </td>
              </tr>
            )}
            {events.map((e, i) => (
              <tr key={i} className="border-t border-white/5">
                <td className="p-3 text-white/60 whitespace-nowrap">
                  {new Date(e.created_at).toLocaleString("uz-UZ")}
                </td>
                <td className="p-3">{e.provider}</td>
                <td className="p-3 text-white/70">{e.action}</td>
                <td className="p-3 text-xs break-all max-w-md">
                  {e.url || e.sitemap || "—"}
                  {e.count && e.count > 1 ? ` (×${e.count})` : ""}
                </td>
                <td className="p-3">
                  {e.status === "ok" && (
                    <span className="inline-flex items-center gap-1 text-emerald-400">
                      <CheckCircle2 className="w-4 h-4" /> ok
                    </span>
                  )}
                  {e.status === "error" && (
                    <span className="inline-flex items-center gap-1 text-red-400" title={e.error}>
                      <XCircle className="w-4 h-4" /> error
                    </span>
                  )}
                  {e.status === "skipped" && (
                    <span className="inline-flex items-center gap-1 text-white/40">
                      <Clock className="w-4 h-4" /> skipped
                    </span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        </div>
      </div>
    </div>
  );
}

function StatusCard({ label, enabled }: { label: string; enabled: boolean }) {
  return (
    <div className="bg-white/5 rounded-lg p-4">
      <div className="text-xs uppercase tracking-wide text-white/50">{label}</div>
      <div
        className={`mt-1 text-lg font-semibold ${
          enabled ? "text-emerald-400" : "text-white/40"
        }`}
      >
        {enabled ? "Sozlangan" : "Sozlanmagan"}
      </div>
    </div>
  );
}
