import { useEffect, useState } from "react";
import { getPortfolioPnL, type CharacterScope } from "../../lib/api";
import { type TranslationKey } from "../../lib/i18n";
import type { ItemPnL, PortfolioPnL, StationPnL } from "../../lib/types";
import { StatCard } from "./shared";
type PnLPeriod = 7 | 30 | 90 | 180;

interface PnLTabProps {
  formatIsk: (v: number) => string;
  characterScope: CharacterScope;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}

export function PnLTab({ formatIsk, characterScope, t }: PnLTabProps) {
  const [period, setPeriod] = useState<PnLPeriod>(30);
  const [data, setData] = useState<PortfolioPnL | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [salesTax, setSalesTax] = useState(8);
  const [brokerFee, setBrokerFee] = useState(1);
  const [chartMode, setChartMode] = useState<"daily" | "cumulative" | "drawdown">("daily");
  const [itemView, setItemView] = useState<"profit" | "loss">("profit");
  const [bottomView, setBottomView] = useState<"items" | "stations">("items");

  useEffect(() => {
    setLoading(true);
    setError(null);
    getPortfolioPnL(period, { salesTax, brokerFee, ledgerLimit: 500, characterId: characterScope })
      .then(setData)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, [period, salesTax, brokerFee, characterScope]);

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full text-eve-dim text-xs">
        <span className="inline-block w-4 h-4 border-2 border-eve-accent/40 border-t-eve-accent rounded-full animate-spin mr-2" />
        {t("loading")}...
      </div>
    );
  }

  if (error) {
    return <div className="flex items-center justify-center h-full text-eve-error text-xs">{error}</div>;
  }

  if (!data || data.daily_pnl.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center h-full text-eve-dim text-xs space-y-2">
        <div>{t("pnlNoData")}</div>
        <div className="text-[10px] max-w-md text-center">{t("pnlNoDataHint")}</div>
      </div>
    );
  }

  const { summary } = data;

  // Separate top items into profit and loss
  const profitItems = data.top_items.filter((item) => item.net_pnl > 0).sort((a, b) => b.net_pnl - a.net_pnl);
  const lossItems = data.top_items.filter((item) => item.net_pnl < 0).sort((a, b) => a.net_pnl - b.net_pnl);

  return (
    <div className="space-y-4">
      {/* Period selector */}
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="text-xs text-eve-dim uppercase tracking-wider">{t("pnlTitle")}</div>
        <div className="flex items-center gap-2 flex-wrap">
          <div className="flex gap-1">
            {([7, 30, 90, 180] as PnLPeriod[]).map((p) => (
              <button
                key={p}
                onClick={() => setPeriod(p)}
                className={`px-2.5 py-1 text-[10px] rounded-sm border transition-colors ${
                  period === p
                    ? "bg-eve-accent/20 border-eve-accent text-eve-accent"
                    : "bg-eve-panel border-eve-border text-eve-dim hover:text-eve-text hover:border-eve-accent/50"
                }`}
              >
                {t(`pnlPeriod${p}d` as TranslationKey)}
              </button>
            ))}
          </div>
          <div className="flex items-center gap-1 text-[10px]">
            <span className="text-eve-dim">{t("pnlSalesTax")}</span>
            <input
              type="number"
              min={0}
              max={100}
              step={0.1}
              value={salesTax}
              onChange={(e) => setSalesTax(parseFloat(e.target.value) || 0)}
              className="w-14 px-1 py-0.5 rounded-sm border border-eve-border bg-eve-dark text-eve-text"
            />
          </div>
          <div className="flex items-center gap-1 text-[10px]">
            <span className="text-eve-dim">{t("pnlBrokerFee")}</span>
            <input
              type="number"
              min={0}
              max={100}
              step={0.1}
              value={brokerFee}
              onChange={(e) => setBrokerFee(parseFloat(e.target.value) || 0)}
              className="w-14 px-1 py-0.5 rounded-sm border border-eve-border bg-eve-dark text-eve-text"
            />
          </div>
        </div>
      </div>

      {/* Summary cards row 1: P&L, ROI, Win Rate */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <StatCard
          label={t("pnlTotalPnl")}
          value={`${summary.total_pnl >= 0 ? "+" : ""}${formatIsk(summary.total_pnl)} ISK`}
          color={summary.total_pnl >= 0 ? "text-eve-profit" : "text-eve-error"}
          large
        />
        <StatCard
          label={t("pnlROI")}
          value={`${summary.roi_percent >= 0 ? "+" : ""}${summary.roi_percent.toFixed(1)}%`}
          color={summary.roi_percent >= 0 ? "text-eve-profit" : "text-eve-error"}
        />
        <StatCard
          label={t("pnlWinRate")}
          value={`${summary.win_rate.toFixed(0)}%`}
          subvalue={`${summary.profitable_days}/${summary.total_days} ${t("pnlProfitableDays").toLowerCase()}`}
          color="text-eve-accent"
        />
        <StatCard
          label={t("pnlAvgDaily")}
          value={`${summary.avg_daily_pnl >= 0 ? "+" : ""}${formatIsk(summary.avg_daily_pnl)} ISK`}
          color={summary.avg_daily_pnl >= 0 ? "text-eve-profit" : "text-eve-error"}
        />
      </div>

      {/* Summary cards row 2: Best day, Worst day, Volume */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <StatCard
          label={t("pnlBestDay")}
          value={`+${formatIsk(summary.best_day_pnl)} ISK`}
          subvalue={summary.best_day_date}
          color="text-eve-profit"
        />
        <StatCard
          label={t("pnlWorstDay")}
          value={`${formatIsk(summary.worst_day_pnl)} ISK`}
          subvalue={summary.worst_day_date}
          color="text-eve-error"
        />
        <StatCard
          label={t("pnlTotalBought")}
          value={`${formatIsk(summary.total_bought)} ISK`}
        />
        <StatCard
          label={t("pnlTotalSold")}
          value={`${formatIsk(summary.total_sold)} ISK`}
        />
      </div>

      {/* Summary cards row 3: Sharpe, Max DD, Profit Factor, Expectancy */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <StatCard
          label={t("pnlSharpeRatio")}
          value={(summary.sharpe_ratio ?? 0) !== 0 ? (summary.sharpe_ratio ?? 0).toFixed(2) : "—"}
          subvalue={t("pnlSharpeHint")}
          color={(summary.sharpe_ratio ?? 0) > 1 ? "text-eve-profit" : (summary.sharpe_ratio ?? 0) > 0 ? "text-eve-accent" : "text-eve-error"}
        />
        <StatCard
          label={t("pnlMaxDrawdown")}
          value={(summary.max_drawdown_isk ?? 0) > 0 ? `-${formatIsk(summary.max_drawdown_isk ?? 0)} ISK` : "—"}
          subvalue={(summary.max_drawdown_pct ?? 0) > 0 ? `-${(summary.max_drawdown_pct ?? 0).toFixed(1)}% (${summary.max_drawdown_days ?? 0}d)` : undefined}
          color="text-eve-error"
        />
        <StatCard
          label={t("pnlProfitFactor")}
          value={(summary.profit_factor ?? 0) > 0 ? (summary.profit_factor ?? 0).toFixed(2) : "—"}
          subvalue={t("pnlProfitFactorHint")}
          color={(summary.profit_factor ?? 0) >= 1.5 ? "text-eve-profit" : (summary.profit_factor ?? 0) >= 1 ? "text-eve-accent" : "text-eve-error"}
        />
        <StatCard
          label={t("pnlExpectancy")}
          value={`${(summary.expectancy_per_trade ?? 0) >= 0 ? "+" : ""}${formatIsk(summary.expectancy_per_trade ?? 0)} ISK`}
          subvalue={t("pnlExpectancyHint")}
          color={(summary.expectancy_per_trade ?? 0) >= 0 ? "text-eve-profit" : "text-eve-error"}
        />
      </div>

      {/* Ledger quality / matching stats */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <StatCard
          label={t("pnlCoverageQty")}
          value={`${(data.coverage?.match_rate_qty_pct ?? 0).toFixed(1)}%`}
          subvalue={t("pnlCoverageHint")}
          color={(data.coverage?.match_rate_qty_pct ?? 0) >= 80 ? "text-eve-profit" : (data.coverage?.match_rate_qty_pct ?? 0) >= 50 ? "text-eve-accent" : "text-eve-error"}
        />
        <StatCard
          label={t("pnlMatchedSellQty")}
          value={(data.coverage?.matched_sell_qty ?? 0).toLocaleString()}
          subvalue={t("pnlTxns")}
        />
        <StatCard
          label={t("pnlUnmatchedSellQty")}
          value={(data.coverage?.unmatched_sell_qty ?? 0).toLocaleString()}
          subvalue={t("pnlCoverageHint")}
          color={(data.coverage?.unmatched_sell_qty ?? 0) > 0 ? "text-eve-warning" : "text-eve-dim"}
        />
        <StatCard
          label={t("pnlOpenCostBasis")}
          value={`${formatIsk(summary.open_cost_basis ?? 0)} ISK`}
          subvalue={`${summary.open_positions ?? 0} ${t("pnlOpenPositions").toLowerCase()}`}
        />
      </div>

      {/* Daily P&L Chart */}
      <div className="bg-eve-panel border border-eve-border rounded-sm p-3">
        <div className="flex items-center justify-between mb-3">
          <div className="text-[10px] text-eve-dim uppercase tracking-wider">
            {chartMode === "daily" ? t("pnlDailyChart") : chartMode === "cumulative" ? t("pnlCumulativeChart") : t("pnlDrawdownChart")}
          </div>
          <div className="flex gap-1">
            <button
              onClick={() => setChartMode("daily")}
              className={`px-2 py-0.5 text-[10px] rounded-sm border transition-colors ${
                chartMode === "daily"
                  ? "bg-eve-accent/20 border-eve-accent text-eve-accent"
                  : "bg-eve-dark border-eve-border text-eve-dim hover:text-eve-text"
              }`}
            >
              {t("pnlDailyChart")}
            </button>
            <button
              onClick={() => setChartMode("cumulative")}
              className={`px-2 py-0.5 text-[10px] rounded-sm border transition-colors ${
                chartMode === "cumulative"
                  ? "bg-eve-accent/20 border-eve-accent text-eve-accent"
                  : "bg-eve-dark border-eve-border text-eve-dim hover:text-eve-text"
              }`}
            >
              {t("pnlCumulativeChart")}
            </button>
            <button
              onClick={() => setChartMode("drawdown")}
              className={`px-2 py-0.5 text-[10px] rounded-sm border transition-colors ${
                chartMode === "drawdown"
                  ? "bg-red-500/20 border-red-500 text-red-400"
                  : "bg-eve-dark border-eve-border text-eve-dim hover:text-eve-text"
              }`}
            >
              {t("pnlDrawdownChart")}
            </button>
          </div>
        </div>
        <PnLChart data={data.daily_pnl} mode={chartMode} formatIsk={formatIsk} />
      </div>

      {/* Top Items / Station Breakdown */}
      <div className="bg-eve-panel border border-eve-border rounded-sm p-3">
        <div className="flex items-center justify-between mb-3">
          <div className="flex gap-2">
            <button
              onClick={() => setBottomView("items")}
              className={`px-2 py-0.5 text-[10px] rounded-sm border transition-colors ${
                bottomView === "items"
                  ? "bg-eve-accent/20 border-eve-accent text-eve-accent"
                  : "bg-eve-dark border-eve-border text-eve-dim hover:text-eve-text"
              }`}
            >
              {t("pnlTopItems")}
            </button>
            <button
              onClick={() => setBottomView("stations")}
              className={`px-2 py-0.5 text-[10px] rounded-sm border transition-colors ${
                bottomView === "stations"
                  ? "bg-eve-accent/20 border-eve-accent text-eve-accent"
                  : "bg-eve-dark border-eve-border text-eve-dim hover:text-eve-text"
              }`}
            >
              {t("pnlStationBreakdown")} ({data.top_stations?.length ?? 0})
            </button>
          </div>
          {bottomView === "items" && (
            <div className="flex gap-1">
              <button
                onClick={() => setItemView("profit")}
                className={`px-2 py-0.5 text-[10px] rounded-sm border transition-colors ${
                  itemView === "profit"
                    ? "bg-emerald-500/20 border-emerald-500 text-emerald-400"
                    : "bg-eve-dark border-eve-border text-eve-dim hover:text-eve-text"
                }`}
              >
                {t("pnlTopProfit")} ({profitItems.length})
              </button>
              <button
                onClick={() => setItemView("loss")}
                className={`px-2 py-0.5 text-[10px] rounded-sm border transition-colors ${
                  itemView === "loss"
                    ? "bg-red-500/20 border-red-500 text-red-400"
                    : "bg-eve-dark border-eve-border text-eve-dim hover:text-eve-text"
                }`}
              >
                {t("pnlTopLoss")} ({lossItems.length})
              </button>
            </div>
          )}
        </div>
        {bottomView === "items" ? (
          <PnLItemsTable
            items={itemView === "profit" ? profitItems : lossItems}
            formatIsk={formatIsk}
            t={t}
          />
        ) : (
          <PnLStationsTable
            stations={data.top_stations ?? []}
            formatIsk={formatIsk}
            t={t}
          />
        )}
      </div>

      {/* Realized ledger */}
      <div className="bg-eve-panel border border-eve-border rounded-sm p-3">
        <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-2">
          {t("pnlRealizedLedger")} ({data.ledger?.length ?? 0})
        </div>
        <PnLLedgerTable ledger={data.ledger ?? []} formatIsk={formatIsk} t={t} />
      </div>

      {/* Open positions */}
      <div className="bg-eve-panel border border-eve-border rounded-sm p-3">
        <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-2">
          {t("pnlOpenPositions")} ({data.open_positions?.length ?? 0})
        </div>
        <PnLOpenPositionsTable positions={data.open_positions ?? []} formatIsk={formatIsk} t={t} />
      </div>
    </div>
  );
}

// --- P&L Bar Chart (CSS-based) ---

function PnLChart({
  data,
  mode,
  formatIsk,
}: {
  data: PortfolioPnL["daily_pnl"];
  mode: "daily" | "cumulative" | "drawdown";
  formatIsk: (v: number) => string;
}) {
  if (data.length === 0) return null;

  const values = data.map((d) =>
    mode === "daily" ? d.net_pnl : mode === "cumulative" ? d.cumulative_pnl : (d.drawdown_pct ?? 0)
  );
  const maxAbs = Math.max(...values.map(Math.abs), 1);

  // For cumulative mode, compute range from min to max.
  const maxVal = Math.max(...values, 0);
  const minVal = Math.min(...values, 0);
  const range = maxVal - minVal || 1;

  // Show fewer bars if too many days
  const maxBars = 60;
  const step = data.length > maxBars ? Math.ceil(data.length / maxBars) : 1;
  const sampled = step > 1 ? data.filter((_, i) => i % step === 0) : data;
  const sampledValues = sampled.map((d) => (mode === "daily" ? d.net_pnl : d.cumulative_pnl));

  const barWidth = Math.max(2, Math.min(12, Math.floor(680 / sampled.length) - 1));
  const chartHeight = 120;
  const midY = chartHeight / 2;

  // For cumulative mode: compute the zero-line position.
  // The chart spans from minVal at bottom to maxVal at top.
  // Zero line is at (1 - (0 - minVal) / range) * chartHeight from top.
  const cumulativeZeroY = range > 0 ? (1 - (0 - minVal) / range) * chartHeight : chartHeight;

  return (
    <div className="relative">
      {/* Chart area */}
      <div className="relative" style={{ height: chartHeight }}>
        {mode === "drawdown" ? (
          /* Drawdown mode: all bars go downward from top (0%) */
          <div className="flex items-start justify-center gap-px h-full">
            {sampled.map((entry, i) => {
              const val = sampledValues[i]; // always <= 0
              const absMin = Math.max(...values.map((v) => Math.abs(v)), 1);
              const barH = Math.max(1, (Math.abs(val) / absMin) * (chartHeight - 8));
              return (
                <div
                  key={entry.date}
                  className="relative group"
                  style={{ width: barWidth, height: chartHeight }}
                >
                  <div
                    className="bg-red-500/60 hover:bg-red-400/80 transition-colors rounded-b-[1px]"
                    style={{ width: barWidth, height: barH }}
                  />
                  {/* Tooltip */}
                  <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-1 hidden group-hover:block z-10 pointer-events-none">
                    <div className="bg-eve-dark border border-eve-border rounded px-2 py-1 text-[10px] whitespace-nowrap shadow-lg">
                      <div className="text-eve-dim">{entry.date}</div>
                      <div className="text-red-400">{val.toFixed(1)}%</div>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        ) : mode === "daily" ? (
          /* Daily mode: bars grow from the center line */
          <div className="flex items-end justify-center gap-px h-full">
            {sampled.map((entry, i) => {
              const val = sampledValues[i];
              const pct = Math.abs(val) / maxAbs;
              const barH = Math.max(1, pct * (chartHeight / 2 - 4));
              const isPositive = val >= 0;

              return (
                <div
                  key={entry.date}
                  className="relative group flex flex-col items-center"
                  style={{ width: barWidth, height: chartHeight }}
                >
                  {/* Top half */}
                  <div className="flex-1 flex items-end justify-center">
                    {isPositive && (
                      <div
                        className="rounded-t-[1px] bg-emerald-500/80 hover:bg-emerald-400 transition-colors"
                        style={{ width: barWidth, height: barH }}
                      />
                    )}
                  </div>
                  {/* Bottom half */}
                  <div className="flex-1 flex items-start justify-center">
                    {!isPositive && (
                      <div
                        className="rounded-b-[1px] bg-red-500/80 hover:bg-red-400 transition-colors"
                        style={{ width: barWidth, height: barH }}
                      />
                    )}
                  </div>

                  {/* Tooltip */}
                  <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-1 hidden group-hover:block z-10 pointer-events-none">
                    <div className="bg-eve-dark border border-eve-border rounded px-2 py-1 text-[10px] whitespace-nowrap shadow-lg">
                      <div className="text-eve-dim">{entry.date}</div>
                      <div className={isPositive ? "text-emerald-400" : "text-red-400"}>
                        {val >= 0 ? "+" : ""}{formatIsk(val)} ISK
                      </div>
                      <div className="text-eve-dim">{entry.transactions} txns</div>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        ) : (
          /* Cumulative mode: bars grow from the zero line, both up and down */
          <div className="flex items-end justify-center gap-px h-full">
            {sampled.map((entry, i) => {
              const val = sampledValues[i];
              const isPositive = val >= 0;

              // Bar top and height relative to chart:
              // Chart: top=maxVal, bottom=minVal
              // Zero line is at cumulativeZeroY from top.
              // For positive val: bar goes from zeroY up by (val/range)*chartHeight
              // For negative val: bar goes from zeroY down by (|val|/range)*chartHeight
              const barH = Math.max(1, (Math.abs(val) / range) * chartHeight);
              const barTop = isPositive ? cumulativeZeroY - barH : cumulativeZeroY;

              return (
                <div
                  key={entry.date}
                  className="relative group"
                  style={{ width: barWidth, height: chartHeight }}
                >
                  <div
                    className={`absolute transition-colors ${
                      isPositive
                        ? "bg-emerald-500/80 hover:bg-emerald-400 rounded-t-[1px]"
                        : "bg-red-500/80 hover:bg-red-400 rounded-b-[1px]"
                    }`}
                    style={{
                      width: barWidth,
                      height: barH,
                      top: barTop,
                    }}
                  />

                  {/* Tooltip */}
                  <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-1 hidden group-hover:block z-10 pointer-events-none">
                    <div className="bg-eve-dark border border-eve-border rounded px-2 py-1 text-[10px] whitespace-nowrap shadow-lg">
                      <div className="text-eve-dim">{entry.date}</div>
                      <div className={isPositive ? "text-emerald-400" : "text-red-400"}>
                        {val >= 0 ? "+" : ""}{formatIsk(val)} ISK
                      </div>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        )}

        {/* Zero line */}
        {mode === "daily" ? (
          <div
            className="absolute left-0 right-0 border-t border-eve-border/50"
            style={{ top: midY }}
          />
        ) : (
          <div
            className="absolute left-0 right-0 border-t border-eve-border/50"
            style={{ top: cumulativeZeroY }}
          />
        )}
      </div>

      {/* X-axis labels */}
      <div className="flex justify-between mt-1 px-1">
        <span className="text-[9px] text-eve-dim">{sampled[0]?.date.slice(5)}</span>
        {sampled.length > 2 && (
          <span className="text-[9px] text-eve-dim">{sampled[Math.floor(sampled.length / 2)]?.date.slice(5)}</span>
        )}
        <span className="text-[9px] text-eve-dim">{sampled[sampled.length - 1]?.date.slice(5)}</span>
      </div>

      {/* Y-axis labels */}
      <div className="absolute left-0 top-0 bottom-0 flex flex-col justify-between pointer-events-none" style={{ width: 0 }}>
        <span className="text-[9px] text-eve-dim -translate-x-full pr-1">
          {mode === "drawdown" ? "0%" : `+${formatIsk(mode === "daily" ? maxAbs : maxVal)}`}
        </span>
        <span className="text-[9px] text-eve-dim -translate-x-full pr-1">
          {mode === "drawdown" ? "" : "0"}
        </span>
        <span className="text-[9px] text-eve-dim -translate-x-full pr-1">
          {mode === "drawdown"
            ? `${Math.min(...values).toFixed(1)}%`
            : mode === "daily" ? `-${formatIsk(maxAbs)}` : `${formatIsk(minVal)}`}
        </span>
      </div>
    </div>
  );
}

// --- P&L Items Table ---

function PnLItemsTable({
  items,
  formatIsk,
  t,
}: {
  items: ItemPnL[];
  formatIsk: (v: number) => string;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}) {
  if (items.length === 0) {
    return <div className="text-center text-eve-dim text-xs py-4">{t("pnlNoData")}</div>;
  }

  const maxAbsPnl = Math.max(...items.map((i) => Math.abs(i.net_pnl)), 1);

  return (
    <div className="border border-eve-border rounded-sm overflow-hidden">
      <table className="w-full text-xs">
        <thead className="bg-eve-panel">
          <tr className="text-eve-dim">
            <th className="px-3 py-2 text-left">{t("pnlItemName")}</th>
            <th className="px-3 py-2 text-right">{t("pnlItemPnl")}</th>
            <th className="px-3 py-2 text-right">{t("pnlItemMargin")}</th>
            <th className="px-3 py-2 text-right">{t("pnlItemBought")}</th>
            <th className="px-3 py-2 text-right">{t("pnlItemSold")}</th>
            <th className="px-3 py-2 text-right">{t("pnlItemTxns")}</th>
          </tr>
        </thead>
        <tbody>
          {items.slice(0, 20).map((item) => {
            const isProfit = item.net_pnl >= 0;
            const barPct = (Math.abs(item.net_pnl) / maxAbsPnl) * 100;

            return (
              <tr key={item.type_id} className="border-t border-eve-border/50 hover:bg-eve-panel/50">
                <td className="px-3 py-2 text-eve-text">
                  <div className="flex items-center gap-2">
                    <img
                      src={`https://images.evetech.net/types/${item.type_id}/icon?size=32`}
                      alt=""
                      className="w-5 h-5"
                    />
                    <span className="truncate max-w-[180px]">{item.type_name || `Type #${item.type_id}`}</span>
                  </div>
                </td>
                <td className="px-3 py-2 text-right">
                  <div className="flex items-center justify-end gap-2">
                    <div className="w-16 h-1.5 bg-eve-dark rounded-full overflow-hidden">
                      <div
                        className={`h-full rounded-full ${isProfit ? "bg-emerald-500" : "bg-red-500"}`}
                        style={{ width: `${barPct}%` }}
                      />
                    </div>
                    <span className={isProfit ? "text-eve-profit" : "text-eve-error"}>
                      {isProfit ? "+" : ""}{formatIsk(item.net_pnl)}
                    </span>
                  </div>
                </td>
                <td className="px-3 py-2 text-right text-eve-dim">
                  {item.margin_percent !== 0 ? `${item.margin_percent.toFixed(1)}%` : "—"}
                </td>
                <td className="px-3 py-2 text-right text-eve-dim">
                  {formatIsk(item.total_bought)}
                </td>
                <td className="px-3 py-2 text-right text-eve-dim">
                  {formatIsk(item.total_sold)}
                </td>
                <td className="px-3 py-2 text-right text-eve-dim">
                  {item.transactions}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
      {items.length > 20 && (
        <div className="text-center text-eve-dim text-xs py-2 bg-eve-panel">
          {t("andMore", { count: items.length - 20 })}
        </div>
      )}
    </div>
  );
}

// --- P&L Stations Table ---

function PnLStationsTable({
  stations,
  formatIsk,
  t,
}: {
  stations: StationPnL[];
  formatIsk: (v: number) => string;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}) {
  if (stations.length === 0) {
    return <div className="text-center text-eve-dim text-xs py-4">{t("pnlNoData")}</div>;
  }

  const maxAbsPnl = Math.max(...stations.map((s) => Math.abs(s.net_pnl)), 1);

  return (
    <div className="border border-eve-border rounded-sm overflow-hidden">
      <table className="w-full text-xs">
        <thead className="bg-eve-panel">
          <tr className="text-eve-dim">
            <th className="px-3 py-2 text-left">{t("pnlStationName")}</th>
            <th className="px-3 py-2 text-right">{t("pnlStationPnl")}</th>
            <th className="px-3 py-2 text-right">{t("pnlStationBought")}</th>
            <th className="px-3 py-2 text-right">{t("pnlStationSold")}</th>
            <th className="px-3 py-2 text-right">{t("pnlStationTxns")}</th>
          </tr>
        </thead>
        <tbody>
          {stations.map((st) => {
            const isProfit = st.net_pnl >= 0;
            const barPct = (Math.abs(st.net_pnl) / maxAbsPnl) * 100;

            return (
              <tr key={st.location_id} className="border-t border-eve-border/50 hover:bg-eve-panel/50">
                <td className="px-3 py-2 text-eve-text max-w-[220px] truncate" title={st.location_name}>
                  {st.location_name || `#${st.location_id}`}
                </td>
                <td className="px-3 py-2 text-right">
                  <div className="flex items-center justify-end gap-2">
                    <div className="w-16 h-1.5 bg-eve-dark rounded-full overflow-hidden">
                      <div
                        className={`h-full rounded-full ${isProfit ? "bg-emerald-500" : "bg-red-500"}`}
                        style={{ width: `${barPct}%` }}
                      />
                    </div>
                    <span className={isProfit ? "text-eve-profit" : "text-eve-error"}>
                      {isProfit ? "+" : ""}{formatIsk(st.net_pnl)}
                    </span>
                  </div>
                </td>
                <td className="px-3 py-2 text-right text-eve-dim">{formatIsk(st.total_bought)}</td>
                <td className="px-3 py-2 text-right text-eve-dim">{formatIsk(st.total_sold)}</td>
                <td className="px-3 py-2 text-right text-eve-dim">{st.transactions}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function PnLLedgerTable({
  ledger,
  formatIsk,
  t,
}: {
  ledger: PortfolioPnL["ledger"];
  formatIsk: (v: number) => string;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}) {
  if (!ledger || ledger.length === 0) {
    return <div className="text-center text-eve-dim text-xs py-4">{t("pnlNoData")}</div>;
  }

  return (
    <div className="border border-eve-border rounded-sm overflow-hidden">
      <table className="w-full text-xs">
        <thead className="bg-eve-panel">
          <tr className="text-eve-dim">
            <th className="px-2 py-1.5 text-left">{t("pnlLedgerDate")}</th>
            <th className="px-2 py-1.5 text-left">{t("pnlLedgerItem")}</th>
            <th className="px-2 py-1.5 text-right">{t("pnlLedgerQty")}</th>
            <th className="px-2 py-1.5 text-right">{t("pnlLedgerBuy")}</th>
            <th className="px-2 py-1.5 text-right">{t("pnlLedgerSell")}</th>
            <th className="px-2 py-1.5 text-right">{t("pnlLedgerHold")}</th>
            <th className="px-2 py-1.5 text-right">{t("pnlLedgerPnl")}</th>
            <th className="px-2 py-1.5 text-right">{t("pnlLedgerMargin")}</th>
          </tr>
        </thead>
        <tbody>
          {ledger.slice(0, 120).map((row, idx) => {
            const isProfit = (row.realized_pnl ?? 0) >= 0;
            return (
              <tr key={`${row.sell_transaction_id}-${row.buy_transaction_id}-${idx}`} className="border-t border-eve-border/50 hover:bg-eve-panel/50">
                <td className="px-2 py-1.5 text-eve-dim">{(row.sell_date ?? "").slice(0, 10)}</td>
                <td className="px-2 py-1.5 text-eve-text truncate max-w-[220px]" title={row.type_name}>
                  {row.type_name || `#${row.type_id}`}
                </td>
                <td className="px-2 py-1.5 text-right text-eve-dim">{(row.quantity ?? 0).toLocaleString()}</td>
                <td className="px-2 py-1.5 text-right text-eve-dim">{formatIsk(row.buy_total ?? 0)}</td>
                <td className="px-2 py-1.5 text-right text-eve-dim">{formatIsk(row.sell_total ?? 0)}</td>
                <td className="px-2 py-1.5 text-right text-eve-dim">{row.holding_days ?? 0}d</td>
                <td className={`px-2 py-1.5 text-right ${isProfit ? "text-eve-profit" : "text-eve-error"}`}>
                  {isProfit ? "+" : ""}{formatIsk(row.realized_pnl ?? 0)}
                </td>
                <td className={`px-2 py-1.5 text-right ${isProfit ? "text-eve-profit" : "text-eve-error"}`}>
                  {(row.margin_percent ?? 0).toFixed(1)}%
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
      {ledger.length > 120 && (
        <div className="text-center text-eve-dim text-xs py-2 bg-eve-panel">
          {t("andMore", { count: ledger.length - 120 })}
        </div>
      )}
    </div>
  );
}

function PnLOpenPositionsTable({
  positions,
  formatIsk,
  t,
}: {
  positions: PortfolioPnL["open_positions"];
  formatIsk: (v: number) => string;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}) {
  if (!positions || positions.length === 0) {
    return <div className="text-center text-eve-dim text-xs py-4">{t("pnlNoData")}</div>;
  }

  return (
    <div className="border border-eve-border rounded-sm overflow-hidden">
      <table className="w-full text-xs">
        <thead className="bg-eve-panel">
          <tr className="text-eve-dim">
            <th className="px-3 py-2 text-left">{t("pnlOpenItem")}</th>
            <th className="px-3 py-2 text-right">{t("pnlOpenQty")}</th>
            <th className="px-3 py-2 text-right">{t("pnlOpenAvgCost")}</th>
            <th className="px-3 py-2 text-right">{t("pnlOpenCostBasis")}</th>
            <th className="px-3 py-2 text-right">{t("pnlOpenOldest")}</th>
          </tr>
        </thead>
        <tbody>
          {positions.map((row) => (
            <tr key={`${row.type_id}-${row.location_id}`} className="border-t border-eve-border/50 hover:bg-eve-panel/50">
              <td className="px-3 py-2 text-eve-text truncate max-w-[260px]" title={row.type_name}>
                {row.type_name || `#${row.type_id}`}
              </td>
              <td className="px-3 py-2 text-right text-eve-dim">{(row.quantity ?? 0).toLocaleString()}</td>
              <td className="px-3 py-2 text-right text-eve-dim">{formatIsk(row.avg_cost ?? 0)}</td>
              <td className="px-3 py-2 text-right text-eve-text">{formatIsk(row.cost_basis ?? 0)}</td>
              <td className="px-3 py-2 text-right text-eve-dim">{row.oldest_lot_date || "—"}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// --- Optimizer Tab ---

