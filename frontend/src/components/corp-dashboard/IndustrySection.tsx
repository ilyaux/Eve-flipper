import { useEffect, useMemo, useState } from "react";
import { getCorpIndustryJobs } from "../../lib/api";
import { type TranslationKey } from "../../lib/i18n";
import type { CorpDashboard, CorpIndustryJob } from "../../lib/types";
import { BarChart, CsvExportButton, DateRangeSelector, KpiCard } from "./shared";
export function IndustrySection({
  dashboard,
  mode,
  formatIsk,
  t,
}: {
  dashboard: CorpDashboard;
  mode: "demo" | "live";
  formatIsk: (v: number) => string;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}) {
  const ind = dashboard.industry_summary;
  const [jobs, setJobs] = useState<CorpIndustryJob[]>([]);
  const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState("");
  const [sortKey, setSortKey] = useState<"product" | "installer" | "activity" | "status" | "runs" | "end_date">("end_date");
  const [sortAsc, setSortAsc] = useState(false);
  const [activityFilter, setActivityFilter] = useState("all");
  const [days, setDays] = useState(30);

  useEffect(() => {
    setLoading(true);
    getCorpIndustryJobs(mode).then(setJobs).catch(() => setJobs([])).finally(() => setLoading(false));
  }, [mode]);

  const toggleSort = (key: typeof sortKey) => {
    if (sortKey === key) setSortAsc(!sortAsc);
    else { setSortKey(key); setSortAsc(key === "product" || key === "installer"); }
  };

  const cutoff = useMemo(() => {
    if (days === 0) return "";
    const d = new Date(); d.setDate(d.getDate() - days);
    return d.toISOString().slice(0, 10);
  }, [days]);

  const activities = useMemo(() => {
    const set = new Set(jobs.map(j => j.activity));
    return Array.from(set).sort();
  }, [jobs]);

  const filtered = useMemo(() => {
    let list = [...jobs];
    if (cutoff) list = list.filter(j => j.start_date.slice(0, 10) >= cutoff);
    if (activityFilter !== "all") list = list.filter(j => j.activity === activityFilter);
    if (search.trim()) {
      const q = search.toLowerCase();
      list = list.filter(j => j.product_name?.toLowerCase().includes(q) || j.installer_name?.toLowerCase().includes(q) || j.location_name?.toLowerCase().includes(q));
    }
    list.sort((a, b) => {
      let cmp = 0;
      switch (sortKey) {
        case "product": cmp = (a.product_name || "").localeCompare(b.product_name || ""); break;
        case "installer": cmp = (a.installer_name || "").localeCompare(b.installer_name || ""); break;
        case "activity": cmp = a.activity.localeCompare(b.activity); break;
        case "status": cmp = a.status.localeCompare(b.status); break;
        case "runs": cmp = a.runs - b.runs; break;
        case "end_date": cmp = (a.end_date || "").localeCompare(b.end_date || ""); break;
      }
      return sortAsc ? cmp : -cmp;
    });
    return list;
  }, [jobs, cutoff, activityFilter, search, sortKey, sortAsc]);

  // Daily completed jobs trend
  const dailyTrend = useMemo(() => {
    const completed = jobs.filter(j => j.status === "delivered" && (!cutoff || j.end_date.slice(0, 10) >= cutoff));
    const byDay: Record<string, number> = {};
    completed.forEach(j => { const d = j.end_date.slice(0, 10); byDay[d] = (byDay[d] || 0) + 1; });
    return Object.entries(byDay).sort(([a], [b]) => a.localeCompare(b)).map(([date, count]) => ({ date, count }));
  }, [jobs, cutoff]);

  const SortHeader = ({ label, field }: { label: string; field: typeof sortKey }) => (
    <th className="px-2 py-1.5 text-left cursor-pointer hover:text-eve-accent transition-colors select-none" onClick={() => toggleSort(field)}>
      {label} {sortKey === field && (sortAsc ? "\u2191" : "\u2193")}
    </th>
  );

  const statusColor = (s: string) => {
    if (s === "active") return "bg-blue-500/20 text-blue-400 border-blue-500/30";
    if (s === "delivered") return "bg-emerald-500/20 text-emerald-400 border-emerald-500/30";
    if (s === "cancelled") return "bg-red-500/20 text-red-400 border-red-500/30";
    if (s === "ready") return "bg-amber-500/20 text-amber-400 border-amber-500/30";
    return "bg-eve-dim/20 text-eve-dim border-eve-dim/30";
  };

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 sm:grid-cols-3 gap-4">
        <KpiCard label={t("corpActiveJobs")} value={String(ind.active_jobs)} color="text-eve-accent" />
        <KpiCard label={t("corpCompletedJobs")} value={String(ind.completed_jobs_30d)} />
        <KpiCard label="Production Value" value={`${formatIsk(ind.production_value)} ISK`} color="text-eve-profit" />
      </div>

      {/* Controls */}
      <div className="flex flex-wrap items-center gap-3">
        <DateRangeSelector value={days} onChange={setDays} t={t} />
        <select value={activityFilter} onChange={e => setActivityFilter(e.target.value)} className="px-2 py-1 text-xs bg-eve-dark border border-eve-border rounded-sm text-eve-text">
          <option value="all">{t("corpAllActivities")}</option>
          {activities.map(a => <option key={a} value={a}>{a.replace(/_/g, " ")}</option>)}
        </select>
        <input type="text" value={search} onChange={e => setSearch(e.target.value)} placeholder={t("corpSearch")} className="px-2 py-1 text-xs bg-eve-dark border border-eve-border rounded-sm text-eve-text placeholder:text-eve-dim/50 w-48 focus:border-eve-accent outline-none" />
        <CsvExportButton data={filtered} filename="corp_industry" headers={["product_name","installer_name","activity","runs","status","location_name","start_date","end_date"]} t={t} />
      </div>

      {/* Trend chart */}
      {dailyTrend.length > 0 && (
        <BarChart
          data={dailyTrend.map(d => ({ date: d.date, value: d.count }))}
          label={t("corpJobsTrend")}
          formatValue={(v) => `${v} jobs`}
          color="blue"
        />
      )}

      {/* Jobs table */}
      <div className="bg-eve-panel border border-eve-border rounded-sm p-4">
        <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-3">{t("corpIndustry")} ({filtered.length})</div>
        {loading ? (
          <div className="flex items-center justify-center py-8 text-eve-dim text-xs">
            <span className="inline-block w-4 h-4 border-2 border-eve-accent/40 border-t-eve-accent rounded-full animate-spin mr-2" />
          </div>
        ) : (
          <div className="border border-eve-border rounded-sm overflow-hidden max-h-[500px] overflow-y-auto">
            <table className="w-full text-xs">
              <thead className="bg-eve-panel sticky top-0 text-eve-dim">
                <tr>
                  <SortHeader label={t("corpProduct")} field="product" />
                  <SortHeader label={t("corpInstaller")} field="installer" />
                  <SortHeader label={t("corpActivity")} field="activity" />
                  <SortHeader label={t("corpRuns")} field="runs" />
                  <SortHeader label={t("corpStatus")} field="status" />
                  <th className="px-2 py-1.5 text-left">{t("corpLocation")}</th>
                  <SortHeader label={t("corpEndDate")} field="end_date" />
                </tr>
              </thead>
              <tbody>
                {filtered.map(j => (
                  <tr key={j.job_id} className="border-t border-eve-border/30 hover:bg-eve-panel/50">
                    <td className="px-2 py-1.5">
                      <div className="flex items-center gap-2">
                        <img src={`https://images.evetech.net/types/${j.product_type_id}/icon?size=32`} alt="" className="w-4 h-4" />
                        <span className="text-eve-text">{j.product_name}</span>
                      </div>
                    </td>
                    <td className="px-2 py-1.5 text-eve-dim">{j.installer_name}</td>
                    <td className="px-2 py-1.5 text-eve-dim capitalize">{j.activity.replace(/_/g, " ")}</td>
                    <td className="px-2 py-1.5 text-eve-accent text-right">{j.runs}</td>
                    <td className="px-2 py-1.5">
                      <span className={`px-1.5 py-0.5 text-[10px] font-bold uppercase rounded-sm border ${statusColor(j.status)}`}>{j.status}</span>
                    </td>
                    <td className="px-2 py-1.5 text-eve-dim max-w-[140px] truncate" title={j.location_name}>{j.location_name}</td>
                    <td className="px-2 py-1.5 text-eve-dim whitespace-nowrap">{j.end_date?.slice(0, 10)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Top Products */}
      {ind.top_products && ind.top_products.length > 0 && (
        <div className="bg-eve-panel border border-eve-border rounded-sm p-4">
          <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-3">{t("corpTopProducts")}</div>
          <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3">
            {ind.top_products.slice(0, 10).map((p) => (
              <div key={p.type_id} className="bg-eve-dark/50 border border-eve-border/50 rounded-sm p-3">
                <div className="flex items-center gap-2 mb-1">
                  <img src={`https://images.evetech.net/types/${p.type_id}/icon?size=32`} alt="" className="w-5 h-5" />
                  <span className="text-xs text-eve-text font-medium truncate">{p.type_name}</span>
                </div>
                <div className="text-xs text-eve-accent font-bold">{p.runs} runs</div>
                <div className="text-[10px] text-eve-dim">{p.jobs} jobs</div>
                {p.estimated_isk ? <div className="text-[10px] text-eve-profit font-mono">{formatIsk(p.estimated_isk)} ISK</div> : null}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
