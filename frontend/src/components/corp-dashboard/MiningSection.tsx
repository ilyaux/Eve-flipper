import { useEffect, useMemo, useState } from "react";
import { getCorpMiningLedger } from "../../lib/api";
import { type TranslationKey } from "../../lib/i18n";
import type { CorpDashboard, CorpMiningEntry } from "../../lib/types";
import { BarChart, CsvExportButton, DateRangeSelector, KpiCard } from "./shared";
export function MiningSection({
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
  const mining = dashboard.mining_summary;
  const [entries, setEntries] = useState<CorpMiningEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState("");
  const [days, setDays] = useState(30);
  const [expandedMiner, setExpandedMiner] = useState<number | null>(null);
  const [sortKey, setSortKey] = useState<"name" | "volume" | "types" | "isk" | "last">("volume");
  const [sortAsc, setSortAsc] = useState(false);

  useEffect(() => {
    setLoading(true);
    getCorpMiningLedger(mode).then(setEntries).catch(() => setEntries([])).finally(() => setLoading(false));
  }, [mode]);

  const cutoff = useMemo(() => {
    if (days === 0) return "";
    const d = new Date(); d.setDate(d.getDate() - days);
    return d.toISOString().slice(0, 10);
  }, [days]);

  const filteredEntries = useMemo(() => {
    let list = entries;
    if (cutoff) list = list.filter(e => e.date >= cutoff);
    return list;
  }, [entries, cutoff]);

  // Build ore ISK lookup from mining summary top_ores (which now have estimated_isk)
  const oreIskPerUnit = useMemo(() => {
    const m = new Map<number, number>();
    for (const ore of mining.top_ores) {
      if (ore.estimated_isk && ore.quantity > 0) {
        m.set(ore.type_id, ore.estimated_isk / ore.quantity);
      }
    }
    return m;
  }, [mining.top_ores]);

  // Aggregate by miner
  const minerAgg = useMemo(() => {
    const map = new Map<number, { id: number; name: string; volume: number; isk: number; types: Set<string>; lastDate: string }>();
    filteredEntries.forEach(e => {
      const unitIsk = oreIskPerUnit.get(e.type_id) || 0;
      const existing = map.get(e.character_id);
      if (existing) {
        existing.volume += e.quantity;
        existing.isk += e.quantity * unitIsk;
        existing.types.add(e.type_name);
        if (e.date > existing.lastDate) existing.lastDate = e.date;
      } else {
        map.set(e.character_id, { id: e.character_id, name: e.character_name || `Miner ${e.character_id}`, volume: e.quantity, isk: e.quantity * unitIsk, types: new Set([e.type_name]), lastDate: e.date });
      }
    });
    let miners = Array.from(map.values());
    if (search.trim()) {
      const q = search.toLowerCase();
      miners = miners.filter(m => m.name.toLowerCase().includes(q));
    }
    miners.sort((a, b) => {
      let cmp = 0;
      switch (sortKey) {
        case "name": cmp = a.name.localeCompare(b.name); break;
        case "volume": cmp = a.volume - b.volume; break;
        case "types": cmp = a.types.size - b.types.size; break;
        case "isk": cmp = a.isk - b.isk; break;
        case "last": cmp = a.lastDate.localeCompare(b.lastDate); break;
      }
      return sortAsc ? cmp : -cmp;
    });
    return miners;
  }, [filteredEntries, oreIskPerUnit, search, sortKey, sortAsc]);

  // Daily volume trend
  const dailyTrend = useMemo(() => {
    const byDay: Record<string, number> = {};
    filteredEntries.forEach(e => { byDay[e.date] = (byDay[e.date] || 0) + e.quantity; });
    return Object.entries(byDay).sort(([a], [b]) => a.localeCompare(b)).map(([date, vol]) => ({ date, volume: vol }));
  }, [filteredEntries]);

  const toggleSort = (key: typeof sortKey) => {
    if (sortKey === key) setSortAsc(!sortAsc);
    else { setSortKey(key); setSortAsc(key === "name"); }
  };

  const SortHeader = ({ label, field }: { label: string; field: typeof sortKey }) => (
    <th className="px-2 py-1.5 text-left cursor-pointer hover:text-eve-accent transition-colors select-none" onClick={() => toggleSort(field)}>
      {label} {sortKey === field && (sortAsc ? "\u2191" : "\u2193")}
    </th>
  );

  // Get entries for expanded miner
  const minerEntries = useMemo(() => {
    if (expandedMiner === null) return [];
    return filteredEntries.filter(e => e.character_id === expandedMiner).sort((a, b) => b.date.localeCompare(a.date));
  }, [filteredEntries, expandedMiner]);

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 sm:grid-cols-3 gap-4">
        <KpiCard label={t("corpActiveMiners")} value={String(mining.active_miners)} color="text-eve-accent" />
        <KpiCard label={t("corpTotalVolume")} value={`${mining.total_volume_30d.toLocaleString()} units`} />
        <KpiCard label="Est. ISK" value={`${formatIsk(mining.estimated_isk)} ISK`} color="text-eve-profit" />
      </div>

      {/* Controls */}
      <div className="flex flex-wrap items-center gap-3">
        <DateRangeSelector value={days} onChange={setDays} t={t} />
        <input type="text" value={search} onChange={e => setSearch(e.target.value)} placeholder={t("corpSearch")} className="px-2 py-1 text-xs bg-eve-dark border border-eve-border rounded-sm text-eve-text placeholder:text-eve-dim/50 w-48 focus:border-eve-accent outline-none" />
        <CsvExportButton data={filteredEntries} filename="corp_mining" headers={["character_name","date","type_name","quantity"]} t={t} />
      </div>

      {/* Trend chart */}
      {dailyTrend.length > 0 && (
        <BarChart
          data={dailyTrend.map(d => ({ date: d.date, value: d.volume }))}
          label={t("corpMiningTrend")}
          formatValue={(v) => `${v.toLocaleString()} units`}
          color="emerald"
        />
      )}

      {/* Miner aggregation table */}
      <div className="bg-eve-panel border border-eve-border rounded-sm p-4">
        <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-3">{t("corpMiner")}s ({minerAgg.length})</div>
        {loading ? (
          <div className="flex items-center justify-center py-8 text-eve-dim text-xs">
            <span className="inline-block w-4 h-4 border-2 border-eve-accent/40 border-t-eve-accent rounded-full animate-spin mr-2" />
          </div>
        ) : (
          <div className="border border-eve-border rounded-sm overflow-hidden max-h-[500px] overflow-y-auto">
            <table className="w-full text-xs">
              <thead className="bg-eve-panel sticky top-0 text-eve-dim">
                <tr>
                  <SortHeader label={t("corpMiner")} field="name" />
                  <SortHeader label={t("corpTotalVolumeMined")} field="volume" />
                  <SortHeader label={t("corpTypesMined")} field="types" />
                  <SortHeader label="Est. ISK" field="isk" />
                  <SortHeader label={t("corpLastActive")} field="last" />
                </tr>
              </thead>
              <tbody>
                {minerAgg.map(m => (
                  <tr key={m.id} className={`border-t border-eve-border/30 cursor-pointer transition-colors ${expandedMiner === m.id ? "bg-eve-accent/5" : "hover:bg-eve-panel/50"}`} onClick={() => setExpandedMiner(expandedMiner === m.id ? null : m.id)}>
                    <td className="px-2 py-1.5">
                      <div className="flex items-center gap-2">
                        <img src={`https://images.evetech.net/characters/${m.id}/portrait?size=32`} alt="" className="w-5 h-5 rounded-sm" />
                        <span className="text-eve-text font-medium">{m.name}</span>
                      </div>
                    </td>
                    <td className="px-2 py-1.5 text-eve-accent font-mono text-right">{m.volume.toLocaleString()}</td>
                    <td className="px-2 py-1.5 text-eve-dim text-right">{m.types.size}</td>
                    <td className="px-2 py-1.5 text-eve-profit font-mono text-right">{m.isk > 0 ? formatIsk(m.isk) : "—"}</td>
                    <td className="px-2 py-1.5 text-eve-dim">{m.lastDate}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Expanded miner details */}
      {expandedMiner !== null && minerEntries.length > 0 && (
        <div className="bg-eve-dark/60 border border-eve-border rounded-sm p-4">
          <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-3">{t("corpMiningDetails")}: {minerAgg.find(m => m.id === expandedMiner)?.name}</div>
          <div className="border border-eve-border rounded-sm overflow-hidden max-h-60 overflow-y-auto">
            <table className="w-full text-xs">
              <thead className="bg-eve-panel sticky top-0 text-eve-dim">
                <tr>
                  <th className="px-2 py-1.5 text-left">Date</th>
                  <th className="px-2 py-1.5 text-left">{t("corpOreType")}</th>
                  <th className="px-2 py-1.5 text-right">{t("corpQuantity")}</th>
                  <th className="px-2 py-1.5 text-right">Est. ISK</th>
                </tr>
              </thead>
              <tbody>
                {minerEntries.slice(0, 100).map((e, i) => {
                  const unitIsk = oreIskPerUnit.get(e.type_id) || 0;
                  return (
                    <tr key={i} className="border-t border-eve-border/30">
                      <td className="px-2 py-1 text-eve-dim">{e.date}</td>
                      <td className="px-2 py-1 text-eve-text">
                        <div className="flex items-center gap-1.5">
                          <img src={`https://images.evetech.net/types/${e.type_id}/icon?size=32`} alt="" className="w-4 h-4" />
                          {e.type_name}
                        </div>
                      </td>
                      <td className="px-2 py-1 text-eve-accent text-right font-mono">{e.quantity.toLocaleString()}</td>
                      <td className="px-2 py-1 text-eve-profit text-right font-mono">{unitIsk > 0 ? formatIsk(e.quantity * unitIsk) : "—"}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}

// ============================================================
// Market Section
// ============================================================

