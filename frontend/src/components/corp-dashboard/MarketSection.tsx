import { useEffect, useMemo, useState } from "react";
import { getCorpOrders } from "../../lib/api";
import { type TranslationKey } from "../../lib/i18n";
import type { CorpDashboard, CorpMarketOrderDetail } from "../../lib/types";
import { CsvExportButton, KpiCard } from "./shared";
export function MarketSection({
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
  const mkt = dashboard.market_summary;
  const [orders, setOrders] = useState<CorpMarketOrderDetail[]>([]);
  const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState("");
  const [sortKey, setSortKey] = useState<"type" | "character" | "price" | "volume" | "location" | "issued">("price");
  const [sortAsc, setSortAsc] = useState(false);
  const [filterSide, setFilterSide] = useState<"all" | "buy" | "sell">("all");

  useEffect(() => {
    setLoading(true);
    getCorpOrders(mode).then(setOrders).catch(() => setOrders([])).finally(() => setLoading(false));
  }, [mode]);

  const toggleSort = (key: typeof sortKey) => {
    if (sortKey === key) setSortAsc(!sortAsc);
    else { setSortKey(key); setSortAsc(key === "type" || key === "character"); }
  };

  const filtered = useMemo(() => {
    let list = [...orders];
    if (filterSide === "buy") list = list.filter(o => o.is_buy_order);
    else if (filterSide === "sell") list = list.filter(o => !o.is_buy_order);
    if (search.trim()) {
      const q = search.toLowerCase();
      list = list.filter(o => o.type_name?.toLowerCase().includes(q) || o.character_name?.toLowerCase().includes(q) || o.location_name?.toLowerCase().includes(q));
    }
    list.sort((a, b) => {
      let cmp = 0;
      switch (sortKey) {
        case "type": cmp = (a.type_name || "").localeCompare(b.type_name || ""); break;
        case "character": cmp = (a.character_name || "").localeCompare(b.character_name || ""); break;
        case "price": cmp = a.price - b.price; break;
        case "volume": cmp = a.volume_remain - b.volume_remain; break;
        case "location": cmp = (a.location_name || "").localeCompare(b.location_name || ""); break;
        case "issued": cmp = (a.issued || "").localeCompare(b.issued || ""); break;
      }
      return sortAsc ? cmp : -cmp;
    });
    return list;
  }, [orders, filterSide, search, sortKey, sortAsc]);

  // Top traders
  const topTraders = useMemo(() => {
    const map = new Map<string, { name: string; orders: number; value: number }>();
    orders.forEach(o => {
      const name = o.character_name || `ID ${o.character_id}`;
      const existing = map.get(name);
      const val = o.price * o.volume_remain;
      if (existing) { existing.orders++; existing.value += val; }
      else map.set(name, { name, orders: 1, value: val });
    });
    return Array.from(map.values()).sort((a, b) => b.value - a.value).slice(0, 5);
  }, [orders]);

  const SortHeader = ({ label, field }: { label: string; field: typeof sortKey }) => (
    <th className="px-2 py-1.5 text-left cursor-pointer hover:text-eve-accent transition-colors select-none" onClick={() => toggleSort(field)}>
      {label} {sortKey === field && (sortAsc ? "\u2191" : "\u2193")}
    </th>
  );

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-4">
        <KpiCard label={t("corpBuyOrders")} value={String(mkt.active_buy_orders)} color="text-eve-profit" />
        <KpiCard label={t("corpSellOrders")} value={String(mkt.active_sell_orders)} color="text-eve-error" />
        <KpiCard label={t("corpTotalBuyValue")} value={`${formatIsk(mkt.total_buy_value)} ISK`} />
        <KpiCard label={t("corpTotalSellValue")} value={`${formatIsk(mkt.total_sell_value)} ISK`} />
        <KpiCard label={t("corpUniqueTraders")} value={String(mkt.unique_traders)} color="text-eve-accent" />
      </div>

      {/* Controls */}
      <div className="flex flex-wrap items-center gap-3">
        <div className="flex rounded-sm overflow-hidden border border-eve-border text-xs">
          {(["all", "buy", "sell"] as const).map(s => (
            <button key={s} onClick={() => setFilterSide(s)} className={`px-3 py-1 capitalize ${filterSide === s ? "bg-eve-accent/20 text-eve-accent" : "text-eve-dim hover:text-eve-text"}`}>
              {s === "all" ? t("corpPeriodAll") : s === "buy" ? t("corpBuy") : t("corpSell")}
            </button>
          ))}
        </div>
        <input type="text" value={search} onChange={e => setSearch(e.target.value)} placeholder={t("corpSearch")} className="px-2 py-1 text-xs bg-eve-dark border border-eve-border rounded-sm text-eve-text placeholder:text-eve-dim/50 w-48 focus:border-eve-accent outline-none" />
        <CsvExportButton data={filtered} filename="corp_orders" headers={["type_name","character_name","is_buy_order","price","volume_remain","volume_total","location_name","issued"]} t={t} />
      </div>

      {/* Top Traders */}
      {topTraders.length > 0 && (
        <div className="bg-eve-panel border border-eve-border rounded-sm p-4">
          <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-3">{t("corpTopTraders")}</div>
          <div className="grid grid-cols-1 sm:grid-cols-5 gap-3">
            {topTraders.map((tr, i) => (
              <div key={tr.name} className="bg-eve-dark/50 border border-eve-border/50 rounded-sm p-3 text-center">
                <div className="text-[10px] text-eve-dim">#{i + 1}</div>
                <div className="text-xs text-eve-text font-medium truncate">{tr.name}</div>
                <div className="text-xs text-eve-accent font-bold">{formatIsk(tr.value)} ISK</div>
                <div className="text-[10px] text-eve-dim">{tr.orders} {t("corpOrders").toLowerCase()}</div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Orders table */}
      <div className="bg-eve-panel border border-eve-border rounded-sm p-4">
        <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-3">{t("corpOrders")} ({filtered.length})</div>
        {loading ? (
          <div className="flex items-center justify-center py-8 text-eve-dim text-xs">
            <span className="inline-block w-4 h-4 border-2 border-eve-accent/40 border-t-eve-accent rounded-full animate-spin mr-2" />
          </div>
        ) : (
          <div className="border border-eve-border rounded-sm overflow-hidden max-h-[500px] overflow-y-auto">
            <table className="w-full text-xs">
              <thead className="bg-eve-panel sticky top-0 text-eve-dim">
                <tr>
                  <SortHeader label={t("corpOrderType")} field="type" />
                  <SortHeader label={t("corpCharacter")} field="character" />
                  <th className="px-2 py-1.5 text-left">{t("corpBuy")}/{t("corpSell")}</th>
                  <SortHeader label={t("corpPrice")} field="price" />
                  <SortHeader label={t("corpVolume")} field="volume" />
                  <SortHeader label={t("corpLocation")} field="location" />
                  <SortHeader label={t("corpIssued")} field="issued" />
                </tr>
              </thead>
              <tbody>
                {filtered.map(o => (
                  <tr key={o.order_id} className="border-t border-eve-border/30 hover:bg-eve-panel/50">
                    <td className="px-2 py-1.5">
                      <div className="flex items-center gap-2">
                        <img src={`https://images.evetech.net/types/${o.type_id}/icon?size=32`} alt="" className="w-4 h-4" />
                        <span className="text-eve-text">{o.type_name}</span>
                      </div>
                    </td>
                    <td className="px-2 py-1.5 text-eve-dim max-w-[100px] truncate">{o.character_name}</td>
                    <td className="px-2 py-1.5">
                      <span className={`px-1.5 py-0.5 text-[10px] font-bold uppercase rounded-sm border ${o.is_buy_order ? "bg-emerald-500/20 text-emerald-400 border-emerald-500/30" : "bg-amber-500/20 text-amber-400 border-amber-500/30"}`}>
                        {o.is_buy_order ? t("corpBuy") : t("corpSell")}
                      </span>
                    </td>
                    <td className="px-2 py-1.5 text-eve-accent text-right font-mono">{formatIsk(o.price)}</td>
                    <td className="px-2 py-1.5 text-right text-eve-dim">{o.volume_remain.toLocaleString()}/{o.volume_total.toLocaleString()}</td>
                    <td className="px-2 py-1.5 text-eve-dim max-w-[140px] truncate" title={o.location_name}>{o.location_name}</td>
                    <td className="px-2 py-1.5 text-eve-dim whitespace-nowrap">{o.issued?.slice(0, 10)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
