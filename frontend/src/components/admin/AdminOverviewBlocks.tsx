"use client";

import { useCallback, useState } from "react";
import Link from "next/link";
import {
  AlertTriangle,
  Bell,
  CheckCircle2,
  Clock,
  Film,
  ImageOff,
  Lightbulb,
  MessageCircle,
  MessageSquare,
  Crown,
  Send,
  Server,
  TrendingDown,
  VideoOff,
  Wallet,
  FileText,
  PlayCircle,
  Flag,
  Bug,
  type LucideIcon,
} from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { getAdminOverview, AdminOverview, OverviewQualityItem } from "@/lib/api";
import { useVisibleInterval } from "@/lib/use-visible-interval";

function fmt(n: number | undefined): string {
  return typeof n === "number" ? n.toLocaleString("uz-UZ") : "—";
}

function usd(n: number): string {
  const sign = n < 0 ? "-" : "";
  return `${sign}$${Math.abs(n).toFixed(2)}`;
}

// ─── Needs attention ─────────────────────────────────────────────────────────

type AttentionRow = { label: string; value: number; href: string; icon: LucideIcon; tone: "warn" | "bad" };

function NeedsAttention({ data }: { data: AdminOverview["attention"] }) {
  const rows: AttentionRow[] = [
    { label: "Ko'rib chiqilmagan apellyatsiyalar", value: data.pending_appeals, href: "/admin/appeals", icon: MessageCircle, tone: "warn" },
    { label: "Yangi tavsiyalar", value: data.pending_suggestions, href: "/admin/suggestions", icon: Lightbulb, tone: "warn" },
    { label: "Moderatsiyadagi kommentlar", value: data.pending_comments, href: "/admin/comments", icon: MessageSquare, tone: "warn" },
    { label: "Shikoyat qilingan izohlar", value: data.reported_comments ?? 0, href: "/admin/comments", icon: Flag, tone: "bad" },
    { label: "Tasdiqlanmagan kinolar", value: data.pending_approvals, href: "/admin/movies", icon: Film, tone: "warn" },
    { label: "Video muammosi haqida xabarlar", value: data.playback_reports, href: "/admin/analytics", icon: PlayCircle, tone: "bad" },
    { label: "Xato bergan publish joblar (7 kun)", value: data.failed_publish_jobs_7d, href: "/admin/clips", icon: Send, tone: "bad" },
    { label: "3 kunda tugaydigan premiumlar", value: data.premium_expiring_3d, href: "/admin/users", icon: Crown, tone: "warn" },
    { label: "Yangi xatolar (24 soat)", value: data.open_errors_24h ?? 0, href: "/admin/errors", icon: Bug, tone: "bad" },
  ];
  const open = rows.filter((r) => r.value > 0);

  return (
    <div className="bg-brand-card border border-brand-border rounded-xl p-4 sm:p-5">
      <div className="flex items-center gap-2 mb-3">
        <Bell size={16} className="text-amber-400" />
        <h2 className="text-base font-semibold text-white">E&apos;tibor talab qiladi</h2>
        {open.length > 0 && (
          <span className="ml-auto text-xs text-gray-500">{open.length} ta bo&apos;lim</span>
        )}
      </div>
      {open.length === 0 ? (
        <p className="flex items-center gap-2 text-sm text-emerald-400">
          <CheckCircle2 size={15} /> Hammasi joyida — kutilayotgan ish yo&apos;q.
        </p>
      ) : (
        <ul className="divide-y divide-brand-border/60">
          {open.map((r) => {
            const Icon = r.icon;
            return (
              <li key={r.label}>
                <Link
                  href={r.href}
                  className="flex items-center gap-3 py-2 text-sm text-gray-300 hover:text-white transition-colors"
                >
                  <Icon size={15} className={r.tone === "bad" ? "text-red-400" : "text-amber-400"} aria-hidden />
                  <span className="flex-1 truncate">{r.label}</span>
                  <span
                    className={`min-w-[2rem] text-center rounded-full px-2 py-0.5 text-xs font-semibold ${
                      r.tone === "bad" ? "bg-red-500/15 text-red-300" : "bg-amber-500/15 text-amber-300"
                    }`}
                  >
                    {fmt(r.value)}
                  </span>
                </Link>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

// ─── Pipeline health ─────────────────────────────────────────────────────────

function Stat({ label, value, tone }: { label: string; value: number; tone?: "bad" | "warn" | "good" }) {
  const color =
    tone === "bad" && value > 0
      ? "text-red-400"
      : tone === "warn" && value > 0
        ? "text-amber-400"
        : tone === "good"
          ? "text-emerald-400"
          : "text-white";
  return (
    <div>
      <p className={`text-xl font-bold tabular-nums ${color}`}>{fmt(value)}</p>
      <p className="text-[11px] text-gray-500 leading-tight">{label}</p>
    </div>
  );
}

function PipelineHealth({ ingestion, publish }: { ingestion: AdminOverview["ingestion"]; publish: AdminOverview["publish_queue"] }) {
  const healthy = ingestion.stuck === 0 && ingestion.failed_24h === 0 && publish.failed_24h === 0;
  return (
    <div className="bg-brand-card border border-brand-border rounded-xl p-4 sm:p-5">
      <div className="flex items-center gap-2 mb-3">
        <Server size={16} className="text-blue-400" />
        <h2 className="text-base font-semibold text-white">Pipeline holati</h2>
        <span
          className={`ml-auto inline-flex items-center gap-1 text-xs ${healthy ? "text-emerald-400" : "text-amber-400"}`}
        >
          {healthy ? <CheckCircle2 size={13} /> : <AlertTriangle size={13} />}
          {healthy ? "Muammo yo'q" : "Tekshirish kerak"}
        </span>
      </div>
      <Link href="/admin/ingestion" className="block rounded-lg p-2 -mx-2 hover:bg-white/[0.03] transition-colors">
        <p className="text-xs text-gray-400 mb-2">Import navbati (ingestion)</p>
        <div className="grid grid-cols-3 sm:grid-cols-6 gap-3">
          <Stat label="Faol" value={ingestion.active} />
          <Stat label="Navbatda" value={ingestion.pending} />
          <Stat label="Qayta ishlanmoqda" value={ingestion.processing} />
          <Stat label="Qotib qolgan" value={ingestion.stuck} tone="warn" />
          <Stat label="Xato (24 soat)" value={ingestion.failed_24h} tone="bad" />
          <Stat label="Tayyor (24 soat)" value={ingestion.completed_24h} tone="good" />
        </div>
      </Link>
      <Link href="/admin/clips" className="mt-2 block rounded-lg p-2 -mx-2 hover:bg-white/[0.03] transition-colors">
        <p className="text-xs text-gray-400 mb-2">Ijtimoiy tarmoqlarga publish</p>
        <div className="grid grid-cols-3 gap-3">
          <Stat label="Rejalashtirilgan" value={publish.scheduled} />
          <Stat label="Yuklandi (24 soat)" value={publish.success_24h} tone="good" />
          <Stat label="Xato (24 soat)" value={publish.failed_24h} tone="bad" />
        </div>
      </Link>
    </div>
  );
}

// ─── Content quality ─────────────────────────────────────────────────────────

type QualityTab = "missing_poster" | "missing_video" | "low_views";

function ContentQuality({ data }: { data: AdminOverview["quality"] }) {
  const [tab, setTab] = useState<QualityTab>("missing_video");
  const tabs: { id: QualityTab; label: string; count: number; icon: LucideIcon }[] = [
    { id: "missing_video", label: "Videosiz", count: data.missing_video, icon: VideoOff },
    { id: "missing_poster", label: "Postersiz", count: data.missing_poster, icon: ImageOff },
    { id: "low_views", label: "Kam ko'rilgan", count: data.low_views, icon: TrendingDown },
  ];
  const items: OverviewQualityItem[] = data.samples[tab] ?? [];

  return (
    <div className="bg-brand-card border border-brand-border rounded-xl p-4 sm:p-5">
      <div className="flex items-center gap-2 mb-3">
        <FileText size={16} className="text-purple-400" />
        <h2 className="text-base font-semibold text-white">Kontent sifati</h2>
        <span className="ml-auto text-xs text-gray-500">Tavsifsiz: {fmt(data.missing_description)}</span>
      </div>
      <div className="flex flex-wrap gap-1.5 mb-3" role="tablist">
        {tabs.map((t) => {
          const Icon = t.icon;
          const active = tab === t.id;
          return (
            <button
              key={t.id}
              role="tab"
              aria-selected={active}
              onClick={() => setTab(t.id)}
              className={`inline-flex items-center gap-1.5 rounded-lg px-2.5 py-1 text-xs transition-colors ${
                active ? "bg-white text-black" : "text-gray-400 hover:text-white border border-brand-border"
              }`}
            >
              <Icon size={12} aria-hidden />
              {t.label}
              <span className={active ? "text-gray-600" : "text-gray-500"}>{fmt(t.count)}</span>
            </button>
          );
        })}
      </div>
      {items.length === 0 ? (
        <p className="flex items-center gap-2 text-sm text-emerald-400">
          <CheckCircle2 size={15} /> Bu toifada kino yo&apos;q.
        </p>
      ) : (
        <ul className="divide-y divide-brand-border/60">
          {items.map((m) => (
            <li key={m.id}>
              <Link
                href={`/admin/movies/${m.id}/edit`}
                className="flex items-center gap-3 py-2 text-sm text-gray-300 hover:text-white transition-colors"
              >
                <span className="flex-1 truncate">{m.title || m.slug || m.id}</span>
                {tab === "low_views" && <span className="text-xs text-gray-500">{fmt(m.views)} ko&apos;rish</span>}
              </Link>
            </li>
          ))}
        </ul>
      )}
      {tab === "low_views" && (
        <p className="mt-2 text-[11px] text-gray-600">14 kundan eski va 5 martadan kam ko&apos;rilgan kinolar.</p>
      )}
    </div>
  );
}

// ─── Finance ─────────────────────────────────────────────────────────────────

function FinanceSummary({ data }: { data: NonNullable<AdminOverview["finance"]> }) {
  const positive = data.net_usd >= 0;
  return (
    <div className="bg-brand-card border border-brand-border rounded-xl p-4 sm:p-5">
      <div className="flex items-center gap-2 mb-3">
        <Wallet size={16} className="text-emerald-400" />
        <h2 className="text-base font-semibold text-white">Moliya · {data.month}</h2>
        <Link href="/admin/expenses" className="ml-auto text-xs text-brand-red hover:text-orange-400">
          Xarajatlar
        </Link>
      </div>
      <div className="grid grid-cols-3 gap-3 mb-3">
        <div>
          <p className="text-xl font-bold text-white tabular-nums">{usd(data.revenue_usd)}</p>
          <p className="text-[11px] text-gray-500">
            Daromad · {fmt(data.stars_revenue)} ⭐ · {fmt(data.premium_sales)} ta sotuv
          </p>
        </div>
        <div>
          <p className="text-xl font-bold text-white tabular-nums">{usd(data.expenses_usd)}</p>
          <p className="text-[11px] text-gray-500">Xarajat</p>
        </div>
        <div>
          <p className={`text-xl font-bold tabular-nums ${positive ? "text-emerald-400" : "text-red-400"}`}>
            {usd(data.net_usd)}
          </p>
          <p className="text-[11px] text-gray-500 inline-flex items-center gap-1">
            {positive ? <CheckCircle2 size={11} aria-hidden /> : <AlertTriangle size={11} aria-hidden />}
            Sof natija
          </p>
        </div>
      </div>
      <dl className="grid grid-cols-3 gap-3 text-xs border-t border-brand-border/60 pt-3">
        <div>
          <dt className="text-gray-500">Doimiy (oylik)</dt>
          <dd className="text-gray-300 tabular-nums">{usd(data.recurring_expenses)}</dd>
        </div>
        <div>
          <dt className="text-gray-500">Bir martalik</dt>
          <dd className="text-gray-300 tabular-nums">{usd(data.one_off_expenses)}</dd>
        </div>
        <div>
          <dt className="text-gray-500">AI kliplar</dt>
          <dd className="text-gray-300 tabular-nums">{usd(data.ai_clip_cost)}</dd>
        </div>
      </dl>
      <p className="mt-2 text-[11px] text-gray-600 inline-flex items-center gap-1">
        <Clock size={10} aria-hidden /> Daromad taxminiy: 1 ⭐ ≈ ${data.stars_usd_rate}
      </p>
    </div>
  );
}

// ─── Container ───────────────────────────────────────────────────────────────

export default function AdminOverviewBlocks() {
  const { token } = useAuth();
  const [data, setData] = useState<AdminOverview | null>(null);
  const [failed, setFailed] = useState(false);

  // Refresh every minute while the tab is visible — these are queue-like
  // numbers an admin keeps the dashboard open for.
  const load = useCallback(
    (isActive: () => boolean) => {
      if (!token) return;
      getAdminOverview(token)
        .then((d) => {
          if (isActive()) {
            setData(d);
            setFailed(false);
          }
        })
        .catch(() => {
          if (isActive()) setFailed(true);
        });
    },
    [token]
  );
  useVisibleInterval(load, 60_000, !!token);

  if (!data) {
    return (
      <div className="mb-8 sm:mb-10 bg-brand-card border border-brand-border rounded-xl p-6 text-center text-sm text-gray-500">
        {failed ? "Umumiy ko'rinishni yuklab bo'lmadi." : "Yuklanmoqda..."}
      </div>
    );
  }

  return (
    <div className="mb-8 sm:mb-10 grid grid-cols-1 lg:grid-cols-2 gap-4">
      <NeedsAttention data={data.attention} />
      <PipelineHealth ingestion={data.ingestion} publish={data.publish_queue} />
      <ContentQuality data={data.quality} />
      {data.finance && <FinanceSummary data={data.finance} />}
    </div>
  );
}
