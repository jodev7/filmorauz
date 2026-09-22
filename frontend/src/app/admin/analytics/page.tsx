"use client";

import { useEffect, useState } from "react";
import { useAuth } from "@/lib/auth-context";
import {
  getAdminSearchSummary,
  getAdminTopContent,
  getAdminCompletionSummary,
  getAdminPremiumFunnel,
  getAdminPlaybackReports,
  SearchTermStat,
  TopContentPeriodStat,
  CompletionStat,
  PremiumFunnelSummary,
  PlaybackReport,
} from "@/lib/api";
import { normalizeMediaUrl } from "@/lib/image-utils";
import MediaImage from "@/components/ui/MediaImage";
import { Activity, Search, Star, AlertTriangle, MonitorPlay, TrendingUp, Filter } from "lucide-react";
import Link from "next/link";

export default function AnalyticsPage() {
  const { token } = useAuth();
  
  const [loading, setLoading] = useState(true);
  const [days, setDays] = useState(30);
  
  const [searchTop, setSearchTop] = useState<SearchTermStat[]>([]);
  const [searchZero, setSearchZero] = useState<SearchTermStat[]>([]);
  const [topContent, setTopContent] = useState<TopContentPeriodStat[]>([]);
  const [completionStats, setCompletionStats] = useState<CompletionStat[]>([]);
  const [funnel, setFunnel] = useState<PremiumFunnelSummary | null>(null);
  
  const [reports, setReports] = useState<PlaybackReport[]>([]);
  const [reportCounts, setReportCounts] = useState<Record<string, number>>({});
  const [reportStatus, setReportStatus] = useState("new");

  useEffect(() => {
    if (!token) return;
    setLoading(true);
    
    Promise.all([
      getAdminSearchSummary(token, days),
      getAdminTopContent(token, days),
      getAdminCompletionSummary(token),
      getAdminPremiumFunnel(token, days),
      getAdminPlaybackReports(token, reportStatus)
    ])
      .then(([searchData, contentData, completionData, funnelData, reportsData]) => {
        setSearchTop(searchData.top || []);
        setSearchZero(searchData.zero_results || []);
        setTopContent(contentData.data || []);
        setCompletionStats(completionData.data || []);
        setFunnel(funnelData);
        setReports(reportsData.reports || []);
        setReportCounts(reportsData.counts || {});
      })
      .catch(console.error)
      .finally(() => setLoading(false));
  }, [token, days, reportStatus]);

  if (loading && !searchTop.length) {
    return (
      <div className="p-4 sm:p-8 flex items-center justify-center min-h-[50vh]">
        <div className="animate-spin rounded-full h-8 w-8 border-t-2 border-b-2 border-brand-primary"></div>
      </div>
    );
  }

  return (
    <div className="p-4 sm:p-8 max-w-7xl mx-auto">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between mb-8 gap-4">
        <div>
          <h1 className="text-2xl font-bold text-white flex items-center gap-2">
            <Activity className="text-brand-primary" />
            Analitika
          </h1>
          <p className="text-gray-500 text-sm mt-1">
            Platformadagi barcha statistik ma'lumotlar va hisobotlar
          </p>
        </div>
        
        <div className="flex items-center gap-2">
          <label className="text-sm text-gray-400">Davr:</label>
          <select 
            value={days}
            onChange={(e) => setDays(Number(e.target.value))}
            className="bg-brand-dark/50 border border-brand-border text-white text-sm rounded-lg p-2 outline-none focus:border-brand-primary"
          >
            <option value={7}>Oxirgi 7 kun</option>
            <option value={30}>Oxirgi 30 kun</option>
            <option value={90}>Oxirgi 3 oy</option>
            <option value={365}>Oxirgi 1 yil</option>
          </select>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-6">
        {/* Top Content */}
        <div className="bg-brand-card border border-brand-border rounded-xl p-5">
          <h2 className="text-lg font-semibold text-white flex items-center gap-2 mb-4">
            <TrendingUp size={18} className="text-blue-400" />
            Eng ko'p ko'rilgan ({days} kun)
          </h2>
          {topContent.length === 0 ? (
            <p className="text-gray-500 text-sm">Ma'lumot yo'q</p>
          ) : (
            <div className="space-y-3">
              {topContent.map((item, i) => (
                <div key={i} className="flex items-center gap-3 p-2 hover:bg-white/5 rounded-lg transition-colors">
                  <div className="w-10 h-14 bg-brand-dark rounded overflow-hidden shrink-0 relative">
                    {item.poster_url ? (
                      <MediaImage src={normalizeMediaUrl(item.poster_url)} alt={item.title} className="w-full h-full object-cover" />
                    ) : (
                      <div className="w-full h-full flex items-center justify-center text-xs text-gray-600">Yo'q</div>
                    )}
                  </div>
                  <div className="flex-1 min-w-0">
                    <h4 className="text-sm font-medium text-white truncate">{item.title}</h4>
                    <p className="text-xs text-gray-500 mt-0.5 capitalize">{item.target_type}</p>
                  </div>
                  <div className="text-right">
                    <div className="text-sm font-bold text-white">{item.views.toLocaleString()}</div>
                    <div className="text-[10px] text-gray-500">ko'rishlar</div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Premium Funnel */}
        <div className="bg-brand-card border border-brand-border rounded-xl p-5">
          <h2 className="text-lg font-semibold text-white flex items-center gap-2 mb-4">
            <Star size={18} className="text-yellow-400" />
            Premium Sotib Olish ({days} kun)
          </h2>
          {funnel ? (
            <div className="grid grid-cols-2 gap-4">
              <div className="bg-brand-dark/50 rounded-lg p-4 border border-white/5">
                <div className="text-xs text-gray-400 mb-1">Premium oyna ko'rilgan</div>
                <div className="text-2xl font-bold text-white">{funnel.lock_views.toLocaleString()}</div>
              </div>
              <div className="bg-brand-dark/50 rounded-lg p-4 border border-white/5">
                <div className="text-xs text-gray-400 mb-1">Tugma bosilgan (CTA)</div>
                <div className="text-2xl font-bold text-white">{funnel.cta_clicks.toLocaleString()}</div>
              </div>
              <div className="bg-brand-dark/50 rounded-lg p-4 border border-white/5">
                <div className="text-xs text-gray-400 mb-1">To'lov boshlangan</div>
                <div className="text-2xl font-bold text-white">{funnel.sessions_started.toLocaleString()}</div>
              </div>
              <div className="bg-brand-dark/50 rounded-lg p-4 border border-emerald-500/20">
                <div className="text-xs text-emerald-400 mb-1">Muvaffaqiyatli to'lov</div>
                <div className="text-2xl font-bold text-emerald-500">{funnel.paid_sessions.toLocaleString()}</div>
              </div>
              <div className="col-span-2 bg-gradient-to-r from-yellow-500/10 to-transparent rounded-lg p-4 border border-yellow-500/20">
                <div className="text-xs text-yellow-400 mb-1">Jami daromad (Stars)</div>
                <div className="text-3xl font-bold text-yellow-500">{(funnel.stars_revenue || 0).toLocaleString()} ⭐️</div>
              </div>
            </div>
          ) : (
            <p className="text-gray-500 text-sm">Ma'lumot yo'q</p>
          )}
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-6">
        {/* Search Analytics */}
        <div className="bg-brand-card border border-brand-border rounded-xl p-5">
          <h2 className="text-lg font-semibold text-white flex items-center gap-2 mb-4">
            <Search size={18} className="text-gray-400" />
            Qidiruv Statistikasi
          </h2>
          
          <div className="grid grid-cols-2 gap-6">
            <div>
              <h3 className="text-sm font-medium text-emerald-400 mb-3 border-b border-white/10 pb-2">Top Qidiruvlar</h3>
              {searchTop.length === 0 ? (
                <p className="text-gray-500 text-xs">Ma'lumot yo'q</p>
              ) : (
                <ul className="space-y-2">
                  {searchTop.map((s, i) => (
                    <li key={i} className="flex justify-between items-center text-sm">
                      <span className="text-gray-300 truncate pr-2">{s.query}</span>
                      <span className="text-white font-mono bg-white/5 px-2 py-0.5 rounded text-xs">{s.count}</span>
                    </li>
                  ))}
                </ul>
              )}
            </div>
            
            <div>
              <h3 className="text-sm font-medium text-red-400 mb-3 border-b border-white/10 pb-2">Topilmaganlar (Zero Results)</h3>
              {searchZero.length === 0 ? (
                <p className="text-gray-500 text-xs">Ma'lumot yo'q</p>
              ) : (
                <ul className="space-y-2">
                  {searchZero.map((s, i) => (
                    <li key={i} className="flex justify-between items-center text-sm">
                      <span className="text-gray-300 truncate pr-2">{s.query}</span>
                      <span className="text-red-400 font-mono bg-red-500/10 px-2 py-0.5 rounded text-xs">{s.count}</span>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </div>
        </div>

        {/* Completion Rates */}
        <div className="bg-brand-card border border-brand-border rounded-xl p-5">
          <h2 className="text-lg font-semibold text-white flex items-center gap-2 mb-4">
            <MonitorPlay size={18} className="text-purple-400" />
            Tomosha qilish (Completion)
          </h2>
          
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm text-gray-400">
              <thead className="text-xs text-gray-500 uppercase bg-brand-dark/50">
                <tr>
                  <th className="px-3 py-2 rounded-l-lg">Kontent</th>
                  <th className="px-3 py-2 text-right">Boshlangan</th>
                  <th className="px-3 py-2 text-right">% O'rtacha</th>
                  <th className="px-3 py-2 text-right rounded-r-lg">Tugallangan</th>
                </tr>
              </thead>
              <tbody>
                {completionStats.length === 0 ? (
                  <tr>
                    <td colSpan={4} className="text-center py-4 text-gray-500">Ma'lumot yo'q</td>
                  </tr>
                ) : (
                  completionStats.slice(0, 8).map((c, i) => (
                    <tr key={i} className="border-b border-brand-border/50 last:border-0 hover:bg-white/5">
                      <td className="px-3 py-2 truncate max-w-[150px] text-white" title={c.title}>{c.title}</td>
                      <td className="px-3 py-2 text-right text-gray-300">{c.starts.toLocaleString()}</td>
                      <td className="px-3 py-2 text-right text-emerald-400">{c.avg_progress}%</td>
                      <td className="px-3 py-2 text-right text-brand-primary">{c.completion_rate}%</td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </div>
      </div>

      {/* Playback Reports */}
      <div className="bg-brand-card border border-brand-border rounded-xl p-5">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between mb-4 gap-4">
          <h2 className="text-lg font-semibold text-white flex items-center gap-2">
            <AlertTriangle size={18} className="text-orange-400" />
            Videodagi Muammolar (Reports)
          </h2>
          
          <div className="flex items-center gap-2 text-sm bg-brand-dark/50 p-1 rounded-lg">
            <button 
              onClick={() => setReportStatus("new")}
              className={`px-3 py-1.5 rounded-md transition-colors ${reportStatus === "new" ? "bg-brand-primary text-white" : "text-gray-400 hover:text-white"}`}
            >
              Yangi {reportCounts.new ? `(${reportCounts.new})` : ""}
            </button>
            <button 
              onClick={() => setReportStatus("reviewing")}
              className={`px-3 py-1.5 rounded-md transition-colors ${reportStatus === "reviewing" ? "bg-orange-500 text-white" : "text-gray-400 hover:text-white"}`}
            >
              Ko'rilmoqda {reportCounts.reviewing ? `(${reportCounts.reviewing})` : ""}
            </button>
            <button 
              onClick={() => setReportStatus("resolved")}
              className={`px-3 py-1.5 rounded-md transition-colors ${reportStatus === "resolved" ? "bg-emerald-500 text-white" : "text-gray-400 hover:text-white"}`}
            >
              Hal qilingan {reportCounts.resolved ? `(${reportCounts.resolved})` : ""}
            </button>
          </div>
        </div>
        
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm text-gray-400">
            <thead className="text-xs text-gray-500 uppercase bg-brand-dark/50">
              <tr>
                <th className="px-4 py-3 rounded-l-lg">Sana</th>
                <th className="px-4 py-3">Kontent</th>
                <th className="px-4 py-3">Sabab</th>
                <th className="px-4 py-3">Izoh</th>
                <th className="px-4 py-3 rounded-r-lg">Holat</th>
              </tr>
            </thead>
            <tbody>
              {reports.length === 0 ? (
                <tr>
                  <td colSpan={5} className="text-center py-8 text-gray-500">Bu holatda xisobotlar yo'q</td>
                </tr>
              ) : (
                reports.map((r, i) => (
                  <tr key={i} className="border-b border-brand-border/50 hover:bg-white/5 transition-colors">
                    <td className="px-4 py-3 whitespace-nowrap text-gray-500">
                      {new Date(r.created_at).toLocaleDateString('uz-UZ')} {new Date(r.created_at).toLocaleTimeString('uz-UZ', {hour: '2-digit', minute:'2-digit'})}
                    </td>
                    <td className="px-4 py-3">
                      <div className="text-white font-medium">{r.title}</div>
                      <div className="text-xs text-brand-primary uppercase">{r.target_type}</div>
                    </td>
                    <td className="px-4 py-3">
                      <span className="bg-white/5 text-gray-300 px-2 py-1 rounded text-xs">{r.reason}</span>
                    </td>
                    <td className="px-4 py-3 max-w-xs truncate" title={r.note}>{r.note || "—"}</td>
                    <td className="px-4 py-3">
                      <span className={`px-2 py-1 inline-flex text-xs leading-5 font-semibold rounded-full ${
                        r.status === 'new' ? 'bg-red-500/10 text-red-400' : 
                        r.status === 'reviewing' ? 'bg-orange-500/10 text-orange-400' : 
                        'bg-emerald-500/10 text-emerald-400'
                      }`}>
                        {r.status === 'new' ? 'Yangi' : r.status === 'reviewing' ? 'Jarayonda' : 'Hal qilingan'}
                      </span>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
