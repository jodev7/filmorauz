"use client";

import { useCallback, useEffect, useState } from "react";
import { Server, Cpu, MemoryStick, HardDrive, CheckCircle2, XCircle, AlertTriangle, RefreshCw, Clock } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { getAdminSystemStatus, SystemHostStatus } from "@/lib/api";

function formatUptime(sec: number): string {
  if (!sec || sec < 0) return "—";
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  if (d > 0) return `${d}k ${h}s`;
  if (h > 0) return `${h}s ${m}d`;
  return `${m}d`;
}

function severityColor(percent: number): string {
  if (percent >= 90) return "bg-red-500";
  if (percent >= 75) return "bg-yellow-500";
  return "bg-emerald-500";
}

function serviceBadge(status: string) {
  if (status === "up") {
    return (
      <span className="inline-flex items-center gap-1 text-[11px] px-2 py-0.5 rounded-full bg-emerald-500/15 border border-emerald-500/40 text-emerald-400">
        <CheckCircle2 size={11} /> up
      </span>
    );
  }
  if (status === "down") {
    return (
      <span className="inline-flex items-center gap-1 text-[11px] px-2 py-0.5 rounded-full bg-red-500/15 border border-red-500/40 text-red-400">
        <XCircle size={11} /> down
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1 text-[11px] px-2 py-0.5 rounded-full bg-gray-500/15 border border-gray-500/40 text-gray-400">
      <AlertTriangle size={11} /> {status || "unknown"}
    </span>
  );
}

function Bar({ label, percent, used, total, unit, icon }: {
  label: string;
  percent: number;
  used: number | string;
  total: number | string;
  unit: string;
  icon: React.ReactNode;
}) {
  const safePct = Math.max(0, Math.min(100, percent || 0));
  return (
    <div>
      <div className="flex items-center justify-between text-[11px] mb-1">
        <span className="inline-flex items-center gap-1 text-gray-400">
          {icon}
          {label}
        </span>
        <span className="text-gray-300">
          {used}/{total} {unit} · {safePct.toFixed(1)}%
        </span>
      </div>
      <div className="h-1.5 rounded-full bg-brand-dark overflow-hidden">
        <div
          className={`h-full ${severityColor(safePct)}`}
          style={{ width: `${safePct}%` }}
        />
      </div>
    </div>
  );
}

function HostCard({ host }: { host: SystemHostStatus }) {
  const healthy = host.ok && !host.error;
  return (
    <div
      className={`rounded-xl border p-4 ${
        healthy ? "bg-brand-card border-brand-border" : "bg-red-500/5 border-red-500/40"
      }`}
    >
      <div className="flex items-start justify-between gap-3 mb-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <Server size={16} className={healthy ? "text-emerald-400" : "text-red-400"} />
            <h3 className="text-white font-semibold truncate">
              {host.hostname || host.host || host.service}
            </h3>
          </div>
          <p className="text-[11px] text-gray-500 mt-0.5 truncate">
            {host.service} · {host.host || "—"}
          </p>
        </div>
        <div className="text-right">
          {healthy ? (
            <span className="inline-flex items-center gap-1 text-[11px] px-2 py-0.5 rounded-full bg-emerald-500/15 border border-emerald-500/40 text-emerald-400">
              <CheckCircle2 size={11} /> online
            </span>
          ) : (
            <span className="inline-flex items-center gap-1 text-[11px] px-2 py-0.5 rounded-full bg-red-500/15 border border-red-500/40 text-red-400">
              <XCircle size={11} /> offline
            </span>
          )}
        </div>
      </div>

      {!healthy && host.error ? (
        <p className="text-xs text-red-300/80 bg-red-500/10 rounded p-2 mb-3 break-words">{host.error}</p>
      ) : null}

      {healthy && (
        <>
          <div className="grid grid-cols-2 gap-3 mb-3">
            <Bar
              label={`CPU${host.cpu_cores ? ` (${host.cpu_cores})` : ""}`}
              percent={host.cpu_percent}
              used={host.cpu_percent?.toFixed(1) ?? "0"}
              total="100"
              unit="%"
              icon={<Cpu size={11} />}
            />
            <Bar
              label="RAM"
              percent={host.memory?.percent ?? 0}
              used={host.memory?.used_mb ?? 0}
              total={host.memory?.total_mb ?? 0}
              unit="MB"
              icon={<MemoryStick size={11} />}
            />
            <Bar
              label="Disk /"
              percent={host.disk?.percent ?? 0}
              used={host.disk?.used_gb ?? 0}
              total={host.disk?.total_gb ?? 0}
              unit="GB"
              icon={<HardDrive size={11} />}
            />
            <div className="text-[11px] text-gray-400">
              <div className="flex items-center gap-1 mb-1">
                <Clock size={11} />
                Uptime
              </div>
              <div className="text-gray-200">{formatUptime(host.uptime_seconds || 0)}</div>
              <div className="text-[10px] text-gray-600 mt-0.5">
                load {host.load_avg?.map((v) => v.toFixed(2)).join(" / ") ?? "—"}
              </div>
            </div>
          </div>

          {host.services && Object.keys(host.services).length > 0 && (
            <div className="flex flex-wrap items-center gap-1.5 pt-2 border-t border-brand-border">
              {Object.entries(host.services).map(([name, state]) => (
                <span key={name} className="inline-flex items-center gap-1.5 text-[11px] text-gray-400">
                  <span className="text-gray-300">{name}</span>
                  {serviceBadge(state)}
                </span>
              ))}
            </div>
          )}
        </>
      )}
    </div>
  );
}

export default function SystemStatusBlock() {
  const { token } = useAuth();
  const [hosts, setHosts] = useState<SystemHostStatus[] | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    if (!token) return;
    try {
      const res = await getAdminSystemStatus(token);
      setHosts(res.hosts);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load");
    } finally {
      setLoading(false);
    }
  }, [token]);

  useEffect(() => {
    void reload();
    const t = setInterval(reload, 30_000);
    return () => clearInterval(t);
  }, [reload]);

  return (
    <div className="mb-8 sm:mb-10">
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2">
          <Server size={16} className="text-blue-400" />
          <h2 className="text-base sm:text-lg font-semibold text-white">VPS holati</h2>
          <span className="text-xs text-gray-500">har 30 soniyada yangilanadi</span>
        </div>
        <button
          onClick={reload}
          className="flex items-center gap-1 text-xs text-gray-400 hover:text-white"
          title="Yangilash"
        >
          <RefreshCw size={12} /> Yangilash
        </button>
      </div>

      {loading && !hosts ? (
        <div className="bg-brand-card border border-brand-border rounded-xl p-6 text-center text-gray-500 text-sm">
          Yuklanmoqda...
        </div>
      ) : error ? (
        <div className="bg-red-500/10 border border-red-500/40 rounded-xl p-4 text-sm text-red-400">
          {error}
        </div>
      ) : hosts && hosts.length > 0 ? (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
          {hosts.map((h, i) => (
            <HostCard key={`${h.service}-${i}`} host={h} />
          ))}
        </div>
      ) : (
        <div className="bg-brand-card border border-brand-border rounded-xl p-6 text-center text-gray-500 text-sm">
          Hostlar topilmadi.
        </div>
      )}
    </div>
  );
}
