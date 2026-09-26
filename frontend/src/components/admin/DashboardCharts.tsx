"use client";

import { useEffect, useMemo, useState } from "react";
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { TrendingDown, TrendingUp, Minus, Table2, LineChart as LineChartIcon } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { getAdminDashboardTimeseries, DailyPoint, DashboardTimeseries, PeriodTotals } from "@/lib/api";

// Admin UI is dark-only: single-series charts share the dark-mode series-1
// step; ink stays in text tokens, never the series color.
const SERIES_COLOR = "#3987e5";
const GRID_COLOR = "rgba(255,255,255,0.06)";
const AXIS_TEXT = "#9ca3af";

type MetricKey = "new_users" | "views" | "active_viewers" | "premium_sales";

const METRICS: {
  key: MetricKey;
  label: string;
  total: (t: PeriodTotals) => number;
  totalLabel: string;
}[] = [
  { key: "new_users", label: "Yangi foydalanuvchilar", total: (t) => t.new_users, totalLabel: "jami" },
  { key: "views", label: "Ko'rishlar", total: (t) => t.views, totalLabel: "jami" },
  { key: "active_viewers", label: "Faol tomoshabinlar", total: (t) => t.avg_active_viewers, totalLabel: "kunlik o'rtacha" },
  { key: "premium_sales", label: "Premium sotuvlar", total: (t) => t.premium_sales, totalLabel: "jami" },
];

const RANGES = [7, 30, 90] as const;

function formatNumber(n: number): string {
  return Math.round(n).toLocaleString("uz-UZ");
}

function formatDay(date: string): string {
  const [, m, d] = date.split("-");
  return `${d}.${m}`;
}

// Percent change vs the previous period. null when there's no baseline.
function percentChange(current: number, previous: number): number | null {
  if (previous === 0) return current === 0 ? 0 : null;
  return ((current - previous) / previous) * 100;
}

function DeltaBadge({ current, previous, days }: { current: number; previous: number; days: number }) {
  const pct = percentChange(current, previous);
  const title = `Oldingi ${days} kun: ${formatNumber(previous)}`;
  if (pct === null) {
    return <span className="text-xs text-gray-500" title={title}>yangi</span>;
  }
  const rounded = Math.round(pct);
  // Status colors ship with an icon + sign, never color alone.
  if (rounded === 0) {
    return (
      <span className="inline-flex items-center gap-1 text-xs text-gray-400" title={title}>
        <Minus size={12} aria-hidden /> 0%
      </span>
    );
  }
  const up = rounded > 0;
  return (
    <span
      className={`inline-flex items-center gap-1 text-xs font-medium ${up ? "text-emerald-400" : "text-red-400"}`}
      title={title}
    >
      {up ? <TrendingUp size={12} aria-hidden /> : <TrendingDown size={12} aria-hidden />}
      {up ? "+" : ""}
      {rounded}%
    </span>
  );
}

function ChartTooltip({
  active,
  payload,
  label,
  metricLabel,
}: {
  active?: boolean;
  payload?: { value?: number | string }[];
  label?: string | number;
  metricLabel: string;
}) {
  if (!active || !payload || payload.length === 0) return null;
  const value = Number(payload[0].value ?? 0);
  return (
    <div className="rounded-lg border border-brand-border bg-brand-dark/95 px-3 py-2 shadow-lg">
      <p className="text-sm font-semibold text-white">{formatNumber(value)}</p>
      <p className="text-xs text-gray-400">
        {metricLabel} · {typeof label === "string" ? formatDay(label) : label}
      </p>
    </div>
  );
}

function MetricChart({ data, metric }: { data: DailyPoint[]; metric: (typeof METRICS)[number] }) {
  const gradientId = `grad-${metric.key}`;
  return (
    <div className="h-36" role="img" aria-label={`${metric.label} — kunlik grafik`}>
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={data} margin={{ top: 6, right: 4, bottom: 0, left: -18 }}>
          <defs>
            <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={SERIES_COLOR} stopOpacity={0.28} />
              <stop offset="100%" stopColor={SERIES_COLOR} stopOpacity={0} />
            </linearGradient>
          </defs>
          <CartesianGrid vertical={false} stroke={GRID_COLOR} />
          <XAxis
            dataKey="date"
            tickFormatter={formatDay}
            tick={{ fill: AXIS_TEXT, fontSize: 11 }}
            axisLine={false}
            tickLine={false}
            minTickGap={24}
          />
          <YAxis
            allowDecimals={false}
            tick={{ fill: AXIS_TEXT, fontSize: 11 }}
            axisLine={false}
            tickLine={false}
            width={48}
          />
          <Tooltip
            cursor={{ stroke: "rgba(255,255,255,0.35)", strokeWidth: 1 }}
            content={<ChartTooltip metricLabel={metric.label} />}
          />
          <Area
            type="monotone"
            dataKey={metric.key}
            stroke={SERIES_COLOR}
            strokeWidth={2}
            fill={`url(#${gradientId})`}
            dot={false}
            activeDot={{ r: 4, stroke: "#111", strokeWidth: 2 }}
            isAnimationActive={false}
          />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  );
}

function DataTable({ data }: { data: DailyPoint[] }) {
  return (
    <div className="max-h-80 overflow-auto rounded-lg border border-brand-border">
      <table className="w-full text-sm">
        <thead className="sticky top-0 bg-brand-dark text-xs uppercase tracking-wider text-gray-500">
          <tr>
            <th className="px-3 py-2 text-left">Sana</th>
            {METRICS.map((m) => (
              <th key={m.key} className="px-3 py-2 text-right">{m.label}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {[...data].reverse().map((d) => (
            <tr key={d.date} className="border-t border-brand-border/50 text-gray-300">
              <td className="px-3 py-1.5">{d.date}</td>
              {METRICS.map((m) => (
                <td key={m.key} className="px-3 py-1.5 text-right tabular-nums">{formatNumber(d[m.key])}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export default function DashboardCharts() {
  const { token } = useAuth();
  const [days, setDays] = useState<(typeof RANGES)[number]>(30);
  const [data, setData] = useState<DashboardTimeseries | null>(null);
  const [error, setError] = useState(false);
  const [view, setView] = useState<"chart" | "table">("chart");

  useEffect(() => {
    if (!token) return;
    let cancelled = false;
    setError(false);
    getAdminDashboardTimeseries(token, days)
      .then((d) => {
        if (!cancelled) setData(d);
      })
      .catch(() => {
        if (!cancelled) setError(true);
      });
    return () => {
      cancelled = true;
    };
  }, [token, days]);

  const series = useMemo(() => data?.series ?? [], [data]);
  const stale = data !== null && data.days !== days;

  return (
    <section className="mb-8 sm:mb-10" aria-labelledby="dashboard-charts-title">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
        <h2 id="dashboard-charts-title" className="text-base sm:text-lg font-semibold text-white">
          Dinamika
        </h2>
        <div className="flex items-center gap-2">
          <div className="flex rounded-lg border border-brand-border p-0.5" role="group" aria-label="Davr">
            {RANGES.map((r) => (
              <button
                key={r}
                onClick={() => setDays(r)}
                aria-pressed={days === r}
                className={`rounded-md px-2.5 py-1 text-xs font-medium transition-colors ${
                  days === r ? "bg-white text-black" : "text-gray-400 hover:text-white"
                }`}
              >
                {r} kun
              </button>
            ))}
          </div>
          <button
            onClick={() => setView((v) => (v === "chart" ? "table" : "chart"))}
            className="inline-flex items-center gap-1.5 rounded-lg border border-brand-border px-2.5 py-1.5 text-xs text-gray-400 hover:text-white"
            aria-pressed={view === "table"}
          >
            {view === "chart" ? <Table2 size={13} /> : <LineChartIcon size={13} />}
            {view === "chart" ? "Jadval" : "Grafik"}
          </button>
        </div>
      </div>

      {error && !data ? (
        <div className="rounded-xl border border-brand-border bg-brand-card p-6 text-center text-sm text-gray-500">
          Grafik ma&apos;lumotlarini yuklab bo&apos;lmadi.
        </div>
      ) : !data ? (
        <div className="rounded-xl border border-brand-border bg-brand-card p-6 text-center text-sm text-gray-500">
          Yuklanmoqda...
        </div>
      ) : view === "table" ? (
        <DataTable data={series} />
      ) : (
        <div className={`grid grid-cols-1 gap-4 md:grid-cols-2 ${stale ? "opacity-60" : ""}`}>
          {METRICS.map((m) => (
            <div key={m.key} className="rounded-xl border border-brand-border bg-brand-card p-4">
              <div className="mb-2 flex items-start justify-between gap-2">
                <div>
                  <p className="text-sm text-gray-400">{m.label}</p>
                  <p className="text-2xl font-bold text-white tabular-nums">
                    {formatNumber(m.total(data.current))}
                    <span className="ml-1.5 text-xs font-normal text-gray-500">{m.totalLabel}</span>
                  </p>
                </div>
                <DeltaBadge current={m.total(data.current)} previous={m.total(data.previous)} days={data.days} />
              </div>
              <MetricChart data={series} metric={m} />
            </div>
          ))}
        </div>
      )}
      {data && data.current.stars_revenue > 0 && view === "chart" && (
        <p className="mt-2 text-xs text-gray-500">
          Premium daromad ({data.days} kun): {formatNumber(data.current.stars_revenue)} ⭐ ·{" "}
          <DeltaBadge current={data.current.stars_revenue} previous={data.previous.stars_revenue} days={data.days} />
        </p>
      )}
    </section>
  );
}
