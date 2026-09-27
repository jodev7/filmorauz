"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Bot, CheckCircle2, Clock, ExternalLink, FileCode2, FileText, Globe, Loader2, RefreshCw, Search, Send, XCircle } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { useToast } from "@/components/admin/Toast";
import { Chip, EmptyState, GhostButton, PageHead, SkeletonList, Tabs, Tone, fmtDate, timeAgo } from "@/components/admin/kit";
import { inputCls } from "@/components/admin/form/ui";

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

interface LLMSStatus {
  movies: number;
  series: number;
  built_at: string;
  index_bytes: number;
  full_bytes: number;
  index_url: string;
  full_url: string;
}

const PROVIDER: Record<Provider, { label: string; tone: Tone }> = {
  indexnow: { label: "IndexNow", tone: "sky" },
  google_indexing: { label: "Google Indexing", tone: "blue" },
  search_console: { label: "Search Console", tone: "violet" },
};

const kb = (n: number) => (n > 1024 * 1024 ? `${(n / 1024 / 1024).toFixed(1)} MB` : `${Math.max(1, Math.round(n / 1024))} KB`);

function Check({ label, hint, on }: { label: string; hint: string; on: boolean }) {
  return (
    <div className={`rounded-2xl border p-4 ${on ? "border-emerald-500/25 bg-emerald-500/[0.04]" : "border-white/10 bg-[#12121a]"}`}>
      <div className="flex items-center justify-between gap-2">
        <p className="text-sm font-medium text-white">{label}</p>
        {on ? <CheckCircle2 size={18} className="text-emerald-400" /> : <XCircle size={18} className="text-gray-600" />}
      </div>
      <p className="mt-1 text-xs text-gray-500">{hint}</p>
      <p className={`mt-2 text-xs font-semibold ${on ? "text-emerald-300" : "text-gray-500"}`}>{on ? "Sozlangan" : "Sozlanmagan"}</p>
    </div>
  );
}

export default function AdminSEOPage() {
  const { token } = useAuth();
  const toast = useToast();
  const [status, setStatus] = useState<SeoStatus | null>(null);
  const [notifierOff, setNotifierOff] = useState(false);
  const [events, setEvents] = useState<SeoEvent[]>([]);
  const [llms, setLlms] = useState<LLMSStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [working, setWorking] = useState<string | null>(null);
  const [urls, setUrls] = useState("");
  const [filter, setFilter] = useState<"all" | "ok" | "error" | "skipped">("all");

  const auth = useMemo(() => ({ Authorization: `Bearer ${token}` }), [token]);

  const load = useCallback(async () => {
    if (!token) return;
    setLoading(true);
    try {
      const [s, l] = await Promise.all([
        fetch(`${API_URL}/admin/seo/status`, { headers: auth }),
        fetch(`${API_URL}/admin/seo/llms`, { headers: auth }),
      ]);
      if (s.status === 404) {
        setNotifierOff(true);
      } else if (s.ok) {
        const j = await s.json();
        setStatus(j.status ?? null);
        setEvents(j.recent_events ?? []);
        setNotifierOff(!j.status?.enabled);
      }
      if (l.ok) setLlms(await l.json());
    } catch {
      toast.error("SEO holatini yuklab bo'lmadi");
    } finally {
      setLoading(false);
    }
  }, [token, auth, toast]);

  useEffect(() => {
    load();
  }, [load]);

  const call = async (key: string, path: string, body?: unknown, ok = "Bajarildi") => {
    if (!token) return;
    setWorking(key);
    try {
      const res = await fetch(`${API_URL}/admin/seo${path}`, {
        method: "POST",
        headers: { ...auth, "Content-Type": "application/json" },
        body: body ? JSON.stringify(body) : undefined,
      });
      const json = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(json?.error || res.statusText);
      toast.success(ok);
      await load();
      return json;
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Xatolik");
    } finally {
      setWorking(null);
    }
  };

  const sendUrls = () => {
    const list = urls
      .split(/\r?\n/)
      .map((s) => s.trim())
      .filter(Boolean);
    if (!list.length) return toast.error("Kamida bitta URL kiriting");
    call("urls", "/reindex", { urls: list }, `${list.length} ta URL yuborildi`).then((r) => r && setUrls(""));
  };

  const site = status?.site_url || llms?.index_url.replace(/\/llms\.txt$/, "") || "";
  const counts = useMemo(() => {
    const c = { all: events.length, ok: 0, error: 0, skipped: 0 };
    events.forEach((e) => c[e.status]++);
    return c;
  }, [events]);
  const shown = filter === "all" ? events : events.filter((e) => e.status === filter);

  const files = [
    { name: "sitemap.xml", desc: "Qidiruv tizimlari uchun barcha sahifalar", icon: FileCode2, path: "/sitemap.xml" },
    { name: "robots.txt", desc: "Crawlerlar uchun qoidalar", icon: FileText, path: "/robots.txt" },
    { name: "llms.txt", desc: "AI yordamchilar uchun sayt xaritasi", icon: Bot, path: "/llms.txt" },
    { name: "llms-full.txt", desc: "Har bir kino va serialning to'liq ma'lumoti", icon: Bot, path: "/llms-full.txt" },
  ];

  return (
    <div className="mx-auto max-w-6xl p-4 sm:p-6">
      <PageHead
        icon={Globe}
        gradient="from-emerald-500 to-teal-700"
        title="SEO"
        subtitle="Google, Bing, Yandex indekslash va AI uchun llms.txt"
        actions={
          <GhostButton onClick={load}>
            <RefreshCw size={15} className={loading ? "animate-spin" : ""} /> Yangilash
          </GhostButton>
        }
      />

      {notifierOff && !loading && (
        <div className="mb-5 rounded-2xl border border-amber-500/25 bg-amber-500/[0.06] px-4 py-3 text-sm text-amber-200">
          Avtomatik indekslash o&apos;chirilgan. Yoqish uchun backend <code className="rounded bg-black/30 px-1">.env</code> da <code className="rounded bg-black/30 px-1">SEO_NOTIFY_ENABLED=true</code> va provayder kalitlarini sozlang. Sitemap va llms.txt baribir ishlaydi.
        </div>
      )}

      <div className="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <Check label="Avtomatik yuborish" hint="Yangi kino qo'shilganda ping" on={!!status?.enabled} />
        <Check label="IndexNow" hint="Bing, Yandex, Seznam" on={!!status?.indexnow_configured} />
        <Check label="Google Indexing API" hint="Google'ga to'g'ridan-to'g'ri" on={!!status?.google_indexing_configured} />
        <Check label="Search Console" hint="Sitemap yuborish" on={!!status?.search_console_configured} />
      </div>

      <div className="mb-6 grid gap-4 lg:grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)]">
        <section className="rounded-2xl border border-white/10 bg-[#12121a] p-4 sm:p-5">
          <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold text-white">
            <FileText size={15} className="text-emerald-400" /> Fayllar
            <span className="text-xs font-normal text-gray-500">— kontent qo&apos;shilganda o&apos;zi yangilanadi</span>
          </h2>
          <ul className="space-y-2">
            {files.map((f) => (
              <li key={f.name} className="flex items-center gap-3 rounded-xl border border-white/5 bg-black/20 px-3 py-2.5">
                <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-white/5 text-gray-300">
                  <f.icon size={15} />
                </span>
                <div className="min-w-0 flex-1">
                  <p className="font-mono text-sm text-white">{f.name}</p>
                  <p className="truncate text-[11px] text-gray-500">{f.desc}</p>
                </div>
                {site && (
                  <a href={`${site}${f.path}`} target="_blank" rel="noopener noreferrer" className="rounded-lg p-2 text-gray-400 hover:bg-white/5 hover:text-white" aria-label={`${f.name} ni ochish`}>
                    <ExternalLink size={14} />
                  </a>
                )}
              </li>
            ))}
          </ul>
        </section>

        <section className="rounded-2xl border border-violet-500/20 bg-gradient-to-br from-violet-500/[0.07] to-transparent p-4 sm:p-5">
          <h2 className="mb-1 flex items-center gap-2 text-sm font-semibold text-white">
            <Bot size={16} className="text-violet-300" /> llms.txt (AI uchun)
          </h2>
          <p className="mb-4 text-xs text-gray-400">ChatGPT, Claude, Perplexity kabi AI&apos;lar saytni o&apos;qishi uchun. Har bir kino va serialning Markdown sahifasi ham bor.</p>
          {llms ? (
            <>
              <div className="grid grid-cols-2 gap-2">
                <div className="rounded-xl bg-black/25 p-3">
                  <p className="text-xl font-bold tabular-nums text-white">{llms.movies.toLocaleString()}</p>
                  <p className="text-[11px] text-gray-500">kino</p>
                </div>
                <div className="rounded-xl bg-black/25 p-3">
                  <p className="text-xl font-bold tabular-nums text-white">{llms.series.toLocaleString()}</p>
                  <p className="text-[11px] text-gray-500">serial</p>
                </div>
              </div>
              <p className="mt-3 text-xs text-gray-500">
                Oxirgi yangilanish: <span className="text-gray-300" title={fmtDate(llms.built_at)}>{timeAgo(llms.built_at)}</span> · {kb(llms.index_bytes)} / {kb(llms.full_bytes)}
              </p>
            </>
          ) : (
            <p className="text-sm text-gray-500">{loading ? "Yuklanmoqda..." : "Holat olinmadi"}</p>
          )}
          <button
            onClick={() => call("llms", "/llms", undefined, "llms.txt yangilandi")}
            disabled={!!working}
            className="mt-4 inline-flex items-center gap-2 rounded-xl bg-violet-600 px-4 py-2 text-sm font-medium text-white hover:bg-violet-500 disabled:opacity-50"
          >
            {working === "llms" ? <Loader2 size={15} className="animate-spin" /> : <RefreshCw size={15} />} Hozir yangilash
          </button>
        </section>
      </div>

      <section className="mb-6 rounded-2xl border border-white/10 bg-[#12121a] p-4 sm:p-5">
        <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold text-white">
          <Send size={15} className="text-sky-400" /> Qidiruv tizimlariga yuborish
        </h2>
        <div className="mb-4 flex flex-wrap gap-2">
          <button
            disabled={!!working || notifierOff}
            onClick={() => call("all", "/reindex/all", undefined, "Hamma kontent navbatga qo'yildi")}
            className="inline-flex items-center gap-2 rounded-xl bg-sky-600 px-4 py-2 text-sm font-medium text-white hover:bg-sky-500 disabled:opacity-50"
          >
            {working === "all" ? <Loader2 size={15} className="animate-spin" /> : <Send size={15} />} Hamma kontentni qayta yuborish
          </button>
          <button
            disabled={!!working || notifierOff}
            onClick={() => call("sitemap", "/sitemap-resubmit", undefined, "Sitemap yuborildi")}
            className="inline-flex items-center gap-2 rounded-xl bg-emerald-600 px-4 py-2 text-sm font-medium text-white hover:bg-emerald-500 disabled:opacity-50"
          >
            {working === "sitemap" ? <Loader2 size={15} className="animate-spin" /> : <RefreshCw size={15} />} Sitemap&apos;ni qayta yuborish
          </button>
        </div>
        <label htmlFor="seo-urls" className="mb-1.5 block text-xs font-medium text-gray-400">
          Aniq sahifalar (har qatorda bitta)
        </label>
        <textarea id="seo-urls" rows={3} value={urls} onChange={(e) => setUrls(e.target.value)} placeholder={"/movies/kino-slug\n/series/serial-slug"} className={`${inputCls} resize-y font-mono text-xs`} />
        <button
          onClick={sendUrls}
          disabled={!!working || notifierOff}
          className="mt-2 inline-flex items-center gap-2 rounded-xl border border-white/10 px-4 py-2 text-sm text-gray-200 hover:bg-white/5 disabled:opacity-50"
        >
          {working === "urls" ? <Loader2 size={15} className="animate-spin" /> : <Send size={15} />} Yuborish
        </button>
        {status?.indexnow_key && (
          <p className="mt-3 text-[11px] text-gray-500">
            IndexNow kaliti: <code className="rounded bg-black/30 px-1.5 py-0.5 text-gray-300">{status.indexnow_key}</code> — <code>/{status.indexnow_key}.txt</code> saytda ochiq bo&apos;lishi kerak.
          </p>
        )}
      </section>

      <section>
        <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
          <h2 className="text-sm font-semibold text-white">So&apos;nggi hodisalar</h2>
          <Tabs<"all" | "ok" | "error" | "skipped">
            value={filter}
            onChange={setFilter}
            items={[
              { key: "all", label: "Hammasi", count: counts.all },
              { key: "ok", label: "OK", count: counts.ok },
              { key: "error", label: "Xato", count: counts.error },
              { key: "skipped", label: "O'tkazilgan", count: counts.skipped },
            ]}
          />
        </div>
        {loading ? (
          <SkeletonList rows={5} height={52} />
        ) : shown.length === 0 ? (
          <EmptyState icon={Search} title="Hodisa yo'q" text="Kino qo'shilganda yoki qo'lda yuborganda shu yerda ko'rinadi." />
        ) : (
          <ul className="max-h-[60vh] divide-y divide-white/5 overflow-y-auto rounded-2xl border border-white/10 bg-[#12121a]">
            {shown.map((e, i) => (
              <li key={i} className="flex flex-wrap items-center gap-3 px-4 py-2.5">
                {e.status === "ok" ? <CheckCircle2 size={16} className="text-emerald-400" /> : e.status === "error" ? <XCircle size={16} className="text-red-400" /> : <Clock size={16} className="text-gray-500" />}
                <Chip tone={PROVIDER[e.provider]?.tone ?? "gray"}>{PROVIDER[e.provider]?.label ?? e.provider}</Chip>
                <span className="text-xs text-gray-400">{e.action}</span>
                <span className="min-w-0 flex-1 truncate font-mono text-xs text-gray-300" title={e.url || e.sitemap}>
                  {e.url || e.sitemap || "—"}
                  {e.count && e.count > 1 ? ` (×${e.count})` : ""}
                </span>
                {e.error && (
                  <span className="max-w-xs truncate text-[11px] text-red-300" title={e.error}>
                    {e.error}
                  </span>
                )}
                <span className="text-[11px] text-gray-500" title={fmtDate(e.created_at)}>
                  {timeAgo(e.created_at)}
                </span>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
