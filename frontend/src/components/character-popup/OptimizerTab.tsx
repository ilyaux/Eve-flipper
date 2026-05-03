import { useEffect, useState } from "react";
import { getPortfolioOptimization, type CharacterScope, type OptimizerResult } from "../../lib/api";
import { type TranslationKey } from "../../lib/i18n";
import type { AllocationSuggestion, AssetStats, OptimizerDiagnostic, PortfolioCapital, PortfolioPositionRisk } from "../../lib/types";
import { StatCard } from "./shared";
type OptPeriod = 30 | 90 | 180;

interface OptimizerTabProps {
  formatIsk: (v: number) => string;
  characterScope: CharacterScope;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}

export function OptimizerTab({ formatIsk, characterScope, t }: OptimizerTabProps) {
  const [period, setPeriod] = useState<OptPeriod>(90);
  const [result, setResult] = useState<OptimizerResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [fetchError, setFetchError] = useState<string | null>(null);

  useEffect(() => {
    setLoading(true);
    setFetchError(null);
    getPortfolioOptimization(period, characterScope)
      .then(setResult)
      .catch((e) => setFetchError(e.message))
      .finally(() => setLoading(false));
  }, [period, characterScope]);

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full text-eve-dim text-xs">
        <span className="inline-block w-4 h-4 border-2 border-eve-accent/40 border-t-eve-accent rounded-full animate-spin mr-2" />
        {t("optLoading")}
      </div>
    );
  }

  if (fetchError) {
    return (
      <div className="flex flex-col items-center justify-center h-full text-xs space-y-2">
        <div className="text-eve-error">{fetchError}</div>
      </div>
    );
  }

  // Diagnostic view: show details when optimization can't run.
  if (result && !result.ok) {
    const diag = result.diagnostic;
    return (
      <div className="flex flex-col items-center justify-center h-full text-xs space-y-4 px-4">
        <div className="text-eve-dim text-sm">{t("optNoData")}</div>

        {diag ? (
          <div className="bg-eve-panel border border-eve-border rounded-sm p-4 max-w-lg w-full space-y-3">
            <div className="text-[10px] text-eve-accent uppercase tracking-wider mb-2">{t("optDiagTitle")}</div>

            <div className="grid grid-cols-2 gap-x-6 gap-y-1 text-[11px]">
              <span className="text-eve-dim">{t("optDiagTotalTxns")}</span>
              <span className="text-eve-text text-right">{diag.total_transactions}</span>
              <span className="text-eve-dim">{t("optDiagWithinLookback")}</span>
              <span className="text-eve-text text-right">{diag.within_lookback}</span>
              <span className="text-eve-dim">{t("optDiagUniqueDays")}</span>
              <span className={`text-right ${diag.unique_days < diag.min_days_required ? "text-eve-error" : "text-eve-text"}`}>
                {diag.unique_days}
              </span>
              <span className="text-eve-dim">{t("optDiagUniqueItems")}</span>
              <span className="text-eve-text text-right">{diag.unique_items}</span>
              <span className="text-eve-dim">{t("optDiagQualified")}</span>
              <span className={`text-right font-bold ${diag.qualified_items < 2 ? "text-eve-error" : "text-eve-profit"}`}>
                {diag.qualified_items} / {t("optDiagMinRequired", { n: 2 })}
              </span>
              <span className="text-eve-dim">{t("optDiagMinDays")}</span>
              <span className="text-eve-accent text-right">{diag.min_days_required} {t("optDiagDays")}</span>
            </div>

            {diag.top_items && diag.top_items.length > 0 && (
              <div>
                <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-1.5 mt-2">{t("optDiagTopItems")}</div>
                <table className="w-full text-[11px]">
                  <thead>
                    <tr className="text-eve-dim border-b border-eve-border">
                      <th className="text-left py-1 font-normal">{t("optAssetName")}</th>
                      <th className="text-right py-1 font-normal">{t("optDiagDays")}</th>
                      <th className="text-right py-1 font-normal">{t("optDiagTxnCount")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {diag.top_items.map((item) => (
                      <tr key={item.type_id} className="border-b border-eve-border/30">
                        <td className="py-1 text-eve-text">{item.type_name || `#${item.type_id}`}</td>
                        <td className={`py-1 text-right ${item.trading_days >= diag.min_days_required ? "text-eve-profit" : "text-eve-error"}`}>
                          {item.trading_days}d
                        </td>
                        <td className="py-1 text-right text-eve-dim">{item.transactions}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}

            <div className="text-[10px] text-eve-dim text-center mt-2 border-t border-eve-border pt-2">
              {t("optDiagExplanation")}
            </div>
          </div>
        ) : (
          <div className="text-[10px] max-w-md text-center text-eve-dim">{t("optNoDataHint")}</div>
        )}
      </div>
    );
  }

  if (!result || !result.ok) {
    return (
      <div className="flex flex-col items-center justify-center h-full text-eve-dim text-xs space-y-2">
        <div>{t("optNoData")}</div>
        <div className="text-[10px] max-w-md text-center">{t("optNoDataHint")}</div>
      </div>
    );
  }

  const data = result.data;
  const optimizerReady = data.optimizer_ready !== false && data.assets.length > 0;
  const positionRisks = data.position_risks ?? [];

  return (
    <div className="space-y-4">
      {/* Header + Period selector */}
      <div className="flex items-center justify-between">
        <div>
          <div className="text-xs text-eve-dim uppercase tracking-wider">{t("optTitle")}</div>
          <div className="text-[10px] text-eve-dim mt-0.5">{t("optDesc")}</div>
        </div>
        <div className="flex gap-1">
          {([30, 90, 180] as OptPeriod[]).map((p) => (
            <button
              key={p}
              onClick={() => setPeriod(p)}
              className={`px-2.5 py-1 text-[10px] rounded-sm border transition-colors ${
                period === p
                  ? "bg-eve-accent/20 border-eve-accent text-eve-accent"
                  : "bg-eve-panel border-eve-border text-eve-dim hover:text-eve-text hover:border-eve-accent/50"
              }`}
            >
              {t(`optPeriod${p}d` as TranslationKey)}
            </button>
          ))}
        </div>
      </div>

      {data.capital && <CapitalRiskPanel capital={data.capital} formatIsk={formatIsk} />}

      {!data.capital && data.warnings && data.warnings.length > 0 && (
        <div className="bg-amber-500/5 border border-amber-500/25 rounded-sm px-3 py-2 text-[10px] text-amber-300">
          {data.warnings.slice(0, 3).join(" | ")}
        </div>
      )}

      {data.optimizer_ready === false && (
        <OptimizerDiagnosticNotice diagnostic={data.diagnostic ?? null} />
      )}

      {positionRisks.length > 0 && (
        <PositionRiskTable risks={positionRisks} formatIsk={formatIsk} />
      )}

      {/* Portfolio comparison cards */}
      {optimizerReady && <div className="grid grid-cols-3 gap-3">
        <div className="bg-eve-panel border border-eve-border rounded-sm p-3">
          <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-1">{t("optCurrentPortfolio")}</div>
          <div className="text-lg font-bold text-eve-text">{data.current_sharpe.toFixed(2)}</div>
          <div className="text-xs text-eve-dim">{t("optSharpe")}</div>
        </div>
        <div className="bg-eve-panel border border-eve-accent/30 rounded-sm p-3">
          <div className="text-[10px] text-eve-accent uppercase tracking-wider mb-1">{t("optOptimalPortfolio")}</div>
          <div className="text-lg font-bold text-eve-accent">{data.optimal_sharpe.toFixed(2)}</div>
          <div className="text-xs text-eve-dim">{t("optSharpe")}</div>
        </div>
        <div className="bg-eve-panel border border-eve-border rounded-sm p-3">
          <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-1">{t("optMinVarPortfolio")}</div>
          <div className="text-lg font-bold text-eve-text">{data.min_var_sharpe.toFixed(2)}</div>
          <div className="text-xs text-eve-dim">{t("optSharpe")}</div>
        </div>
      </div>}

      {/* Diversification metrics */}
      {optimizerReady && <div className="grid grid-cols-2 gap-3">
        <StatCard
          label={t("optHHI")}
          value={data.hhi.toFixed(3)}
          subvalue={t("optHHIHint")}
          color={data.hhi < 0.15 ? "text-eve-profit" : data.hhi < 0.25 ? "text-eve-accent" : "text-eve-error"}
        />
        <StatCard
          label={t("optDivRatio")}
          value={data.diversification_ratio.toFixed(2)}
          subvalue={t("optDivRatioHint")}
          color={data.diversification_ratio > 1.2 ? "text-eve-profit" : "text-eve-accent"}
        />
      </div>}

      {/* Efficient Frontier */}
      {optimizerReady && data.efficient_frontier && data.efficient_frontier.length > 0 && (
        <div className="bg-eve-panel border border-eve-border rounded-sm p-3">
          <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-1">{t("optFrontier")}</div>
          <div className="text-[9px] text-eve-dim mb-2">{t("optFrontierHint")}</div>
          <EfficientFrontierChart
            frontier={data.efficient_frontier}
            currentWeights={data.current_weights}
            optimalWeights={data.optimal_weights}
            minVarWeights={data.min_var_weights}
            means={data.assets.map((a) => a.avg_daily_pnl)}
            covApprox={data.correlation_matrix}
            assets={data.assets}
            formatIsk={formatIsk}
          />
        </div>
      )}

      {/* Correlation Matrix */}
      {optimizerReady && data.correlation_matrix && data.assets.length > 1 && (
        <div className="bg-eve-panel border border-eve-border rounded-sm p-3">
          <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-1">{t("optCorrelation")}</div>
          <div className="text-[9px] text-eve-dim mb-2">{t("optCorrelationHint")}</div>
          <CorrelationMatrix assets={data.assets} matrix={data.correlation_matrix} />
        </div>
      )}

      {/* Asset Table */}
      {optimizerReady && <div className="bg-eve-panel border border-eve-border rounded-sm p-3">
        <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-2">{t("optAssets")}</div>
        <AssetTable assets={data.assets} currentWeights={data.current_weights} optimalWeights={data.optimal_weights} formatIsk={formatIsk} t={t} />
      </div>}

      {/* Suggestions */}
      {optimizerReady && data.suggestions && data.suggestions.filter((s) => s.action !== "hold").length > 0 && (
        <div className="bg-eve-panel border border-eve-border rounded-sm p-3">
          <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-2">{t("optSuggestions")}</div>
          <SuggestionsPanel suggestions={data.suggestions} t={t} />
        </div>
      )}
    </div>
  );
}

// --- Capital risk model ---

function CapitalRiskPanel({ capital, formatIsk }: { capital: PortfolioCapital; formatIsk: (v: number) => string }) {
  const riskColor = riskTextClass(capital.risk_level, capital.risk_score);
  const freeWidth = pctWidth(capital.free_capital_pct);
  const buyWidth = pctWidth(capital.locked_buy_pct);
  const inventoryWidth = pctWidth(capital.inventory_pct);

  return (
    <div className="bg-eve-panel border border-eve-border rounded-sm p-3 space-y-3">
      <div className="flex items-center justify-between gap-3">
        <div>
          <div className="text-[10px] text-eve-dim uppercase tracking-wider">Capital / Risk</div>
          <div className="text-[9px] text-eve-dim mt-0.5">wallet, open inventory, active orders, liquidity and concentration</div>
        </div>
        <div className={`text-xs font-bold uppercase tracking-wider ${riskColor}`}>
          {capital.risk_level || "low"} {formatPct(capital.risk_score)}
        </div>
      </div>

      <div className="grid grid-cols-4 gap-3">
        <StatCard label="Estimated equity" value={formatIsk(capital.estimated_equity_isk)} subvalue={`wallet ${formatIsk(capital.wallet_isk)}`} color="text-eve-text" />
        <StatCard label="Used capital" value={formatIsk(capital.used_capital_isk)} subvalue={`${formatPct(capital.inventory_pct)} inventory`} color="text-eve-accent" />
        <StatCard label="Buy orders" value={formatIsk(capital.active_buy_order_isk)} subvalue={`${formatPct(capital.locked_buy_pct)} locked`} color={capital.locked_buy_pct > 50 ? "text-eve-error" : "text-eve-text"} />
        <StatCard label="Sell backlog" value={formatIsk(capital.active_sell_order_isk)} subvalue={`${formatPct(capital.sell_backlog_pct)} of inventory mark`} color={capital.sell_backlog_pct > 80 ? "text-eve-error" : "text-eve-profit"} />
      </div>

      <div className="space-y-1.5">
        <div className="h-2 bg-eve-dark rounded-sm overflow-hidden flex">
          <div className="bg-eve-profit/70" style={{ width: `${freeWidth}%` }} />
          <div className="bg-eve-accent/80" style={{ width: `${buyWidth}%` }} />
          <div className="bg-sky-500/70" style={{ width: `${inventoryWidth}%` }} />
        </div>
        <div className="grid grid-cols-4 gap-2 text-[10px] text-eve-dim">
          <span>free {formatPct(capital.free_capital_pct)}</span>
          <span>buy lock {formatPct(capital.locked_buy_pct)}</span>
          <span>inventory {formatPct(capital.inventory_pct)}</span>
          <span className={capital.top_exposure_pct > 45 ? "text-eve-error" : ""}>top item {formatPct(capital.top_exposure_pct)}</span>
        </div>
      </div>

      {capital.warnings && capital.warnings.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {capital.warnings.slice(0, 5).map((warning) => (
            <span key={warning} className="px-2 py-0.5 rounded-sm border border-amber-500/25 bg-amber-500/5 text-[10px] text-amber-300">
              {warningLabel(warning)}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}

function OptimizerDiagnosticNotice({ diagnostic }: { diagnostic: OptimizerDiagnostic | null }) {
  return (
    <div className="bg-amber-500/5 border border-amber-500/25 rounded-sm p-3 text-xs">
      <div className="text-[10px] text-amber-300 uppercase tracking-wider mb-1">Markowitz model not ready</div>
      {diagnostic ? (
        <div className="grid grid-cols-4 gap-3 text-[10px] text-eve-dim">
          <span>txns <b className="text-eve-text">{diagnostic.total_transactions}</b></span>
          <span>lookback <b className="text-eve-text">{diagnostic.within_lookback}</b></span>
          <span>days <b className="text-eve-text">{diagnostic.unique_days}</b> / {diagnostic.min_days_required}</span>
          <span>items <b className="text-eve-text">{diagnostic.qualified_items}</b> / 2 qualified</span>
        </div>
      ) : (
        <div className="text-[10px] text-eve-dim">Capital risk is available, but the optimizer needs more closed trading history.</div>
      )}
    </div>
  );
}

function PositionRiskTable({ risks, formatIsk }: { risks: PortfolioPositionRisk[]; formatIsk: (v: number) => string }) {
  const rows = [...risks].sort((a, b) => b.risk_score - a.risk_score).slice(0, 20);

  return (
    <div className="bg-eve-panel border border-eve-border rounded-sm p-3">
      <div className="flex items-center justify-between mb-2">
        <div>
          <div className="text-[10px] text-eve-dim uppercase tracking-wider">Position risk</div>
          <div className="text-[9px] text-eve-dim mt-0.5">inventory exposure, active orders, liquidation speed and target allocation</div>
        </div>
        <div className="text-[10px] text-eve-dim">{rows.length} rows</div>
      </div>
      <div className="border border-eve-border rounded-sm overflow-x-auto">
        <table className="w-full text-xs min-w-[980px]">
          <thead className="bg-eve-panel">
            <tr className="text-eve-dim">
              <th className="px-3 py-2 text-left">Action</th>
              <th className="px-3 py-2 text-left">Item</th>
              <th className="px-3 py-2 text-right">Exposure</th>
              <th className="px-3 py-2 text-right">Target</th>
              <th className="px-3 py-2 text-right">Risk</th>
              <th className="px-3 py-2 text-right">DTL</th>
              <th className="px-3 py-2 text-right">Buy ord</th>
              <th className="px-3 py-2 text-right">Sell ord</th>
              <th className="px-3 py-2 text-right">Unrealized</th>
              <th className="px-3 py-2 text-right">Suggested</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => {
              const riskColor = riskTextClass(row.risk_level, row.risk_score);
              return (
                <tr key={row.type_id} className="border-t border-eve-border/50 hover:bg-eve-dark/40">
                  <td className="px-3 py-2">
                    <span className={`px-2 py-0.5 rounded-sm border text-[10px] font-bold uppercase ${actionBadgeClass(row.action)}`}>
                      {actionLabel(row.action)}
                    </span>
                    <div className="text-[9px] text-eve-dim mt-1">{reasonLabel(row.reason)}</div>
                  </td>
                  <td className="px-3 py-2 text-eve-text">
                    <div className="flex items-center gap-2">
                      <img src={`https://images.evetech.net/types/${row.type_id}/icon?size=32`} alt="" className="w-5 h-5" />
                      <div className="min-w-0">
                        <div className="truncate max-w-[180px]">{row.type_name || `#${row.type_id}`}</div>
                        <div className="text-[9px] text-eve-dim">
                          {row.inventory_qty.toLocaleString()} inv / {row.active_sell_qty.toLocaleString()} sell
                          {row.inventory_source ? ` / ${inventorySourceLabel(row.inventory_source)}` : ""}
                        </div>
                      </div>
                    </div>
                  </td>
                  <td className="px-3 py-2 text-right">
                    <div className="text-eve-text">{formatIsk(row.exposure_isk)}</div>
                    <div className="text-[9px] text-eve-dim">{formatPct(row.exposure_pct)}</div>
                  </td>
                  <td className="px-3 py-2 text-right">
                    <div className="text-eve-accent">{formatPct(row.target_pct)}</div>
                    <div className={`text-[9px] ${row.delta_pct >= 0 ? "text-eve-profit" : "text-eve-error"}`}>{formatSignedPct(row.delta_pct)}</div>
                  </td>
                  <td className={`px-3 py-2 text-right font-bold ${riskColor}`}>{formatPct(row.risk_score)}</td>
                  <td className="px-3 py-2 text-right text-eve-dim">{formatDays(row.days_to_liquidate)}</td>
                  <td className="px-3 py-2 text-right text-eve-dim">{formatIsk(row.active_buy_isk)}</td>
                  <td className="px-3 py-2 text-right text-eve-dim">{formatIsk(row.active_sell_isk)}</td>
                  <td className={`px-3 py-2 text-right ${row.unrealized_pnl >= 0 ? "text-eve-profit" : "text-eve-error"}`}>
                    <div>{row.unrealized_pnl >= 0 ? "+" : ""}{formatIsk(row.unrealized_pnl)}</div>
                    <div className="text-[9px]">{formatSignedPct(row.unrealized_roi_pct)}</div>
                  </td>
                  <td className="px-3 py-2 text-right">
                    {row.suggested_buy_isk > 0 ? (
                      <span className="text-eve-profit">buy {formatIsk(row.suggested_buy_isk)}</span>
                    ) : row.suggested_sell_isk > 0 ? (
                      <span className="text-eve-error">sell {formatIsk(row.suggested_sell_isk)}</span>
                    ) : (
                      <span className="text-eve-dim">hold</span>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function pctWidth(value: number) {
  if (!Number.isFinite(value) || value <= 0) return 0;
  return Math.max(0, Math.min(100, value));
}

function formatPct(value: number) {
  if (!Number.isFinite(value)) return "0.0%";
  return `${value.toFixed(1)}%`;
}

function formatSignedPct(value: number) {
  if (!Number.isFinite(value)) return "0.0%";
  return `${value >= 0 ? "+" : ""}${value.toFixed(1)}%`;
}

function formatDays(value: number) {
  if (!Number.isFinite(value) || value <= 0) return "-";
  if (value >= 365) return ">365d";
  return `${value.toFixed(value >= 10 ? 0 : 1)}d`;
}

function riskTextClass(level: string, score: number) {
  if (level === "high" || score >= 70) return "text-eve-error";
  if (level === "medium" || score >= 35) return "text-eve-accent";
  return "text-eve-profit";
}

function actionBadgeClass(action: string) {
  switch (action) {
    case "increase":
      return "bg-emerald-500/10 border-emerald-500/25 text-emerald-300";
    case "reduce":
    case "liquidate":
      return "bg-red-500/10 border-red-500/25 text-red-300";
    case "pause_buy":
      return "bg-amber-500/10 border-amber-500/25 text-amber-300";
    default:
      return "bg-eve-dark border-eve-border text-eve-dim";
  }
}

function actionLabel(action: string) {
  const labels: Record<string, string> = {
    increase: "increase",
    reduce: "reduce",
    liquidate: "liquidate",
    pause_buy: "pause buy",
    hold: "hold",
  };
  return labels[action] ?? action;
}

function reasonLabel(reason: string) {
  const labels: Record<string, string> = {
    balanced: "balanced",
    negative_slow_inventory: "negative and slow",
    over_concentrated: "over concentrated",
    sell_backlog: "sell backlog",
    above_target: "above target",
    below_target_good_risk: "below target, good risk",
    slow_liquidation: "slow liquidation",
    negative_pnl: "negative PnL",
  };
  return labels[reason] ?? reason;
}

function warningLabel(warning: string) {
  const labels: Record<string, string> = {
    single_item_concentration: "single item concentration",
    buy_orders_lock_most_capital: "buy orders lock most capital",
    large_sell_backlog: "large sell backlog",
    asset_inventory_reconciled: "inventory reconciled from assets",
    stale_txn_inventory_absent_from_assets: "stale transaction inventory absent from assets",
    asset_cost_basis_estimated: "asset cost basis estimated",
  };
  return labels[warning] ?? warning;
}

function inventorySourceLabel(source: string) {
  const labels: Record<string, string> = {
    transactions: "txn",
    active_sell: "sell order",
    active_buy: "buy order",
    assets: "assets",
    assets_match: "assets ok",
    assets_zero: "assets 0",
    assets_estimated_cost: "assets est cost",
  };
  return labels[source] ?? source;
}

// --- Efficient Frontier Chart (CSS-based scatter plot) ---

function EfficientFrontierChart({
  frontier,
  currentWeights,
  optimalWeights,
  minVarWeights,
  means,
  assets,
  formatIsk,
}: {
  frontier: { risk: number; return: number }[];
  currentWeights: number[];
  optimalWeights: number[];
  minVarWeights: number[];
  means: number[];
  covApprox: number[][];
  assets: AssetStats[];
  formatIsk: (v: number) => string;
}) {
  const chartW = 600;
  const chartH = 140;

  const allRisks = frontier.map((p) => p.risk);
  const allReturns = frontier.map((p) => p.return);

  const minRisk = Math.min(...allRisks) * 0.9;
  const maxRisk = Math.max(...allRisks) * 1.1 || 1;
  const minRet = Math.min(...allReturns) * 1.1;
  const maxRet = Math.max(...allReturns) * 1.1 || 1;

  const scaleX = (r: number) => ((r - minRisk) / (maxRisk - minRisk)) * chartW;
  const scaleY = (ret: number) => chartH - ((ret - minRet) / (maxRet - minRet)) * chartH;

  // Compute portfolio positions.
  const portRet = (w: number[]) => w.reduce((s, wi, i) => s + wi * means[i], 0);
  const portRisk = (w: number[]) => {
    // Approximate: use frontier's closest return point.
    const r = portRet(w);
    const closest = frontier.reduce((a, b) => Math.abs(a.return - r) < Math.abs(b.return - r) ? a : b);
    return closest.risk;
  };

  // Individual assets.
  const assetPoints = assets.map((a) => ({ risk: a.volatility, ret: a.avg_daily_pnl, name: a.type_name }));

  return (
    <div className="relative overflow-hidden" style={{ height: chartH + 20 }}>
      <svg width={chartW} height={chartH + 20} className="w-full" viewBox={`0 0 ${chartW} ${chartH + 20}`}>
        {/* Frontier curve */}
        <polyline
          fill="none"
          stroke="#58a6ff"
          strokeWidth={2}
          opacity={0.6}
          points={frontier.map((p) => `${scaleX(p.risk)},${scaleY(p.return)}`).join(" ")}
        />

        {/* Individual assets as small dots */}
        {assetPoints.map((a, i) => (
          <circle
            key={i}
            cx={scaleX(a.risk)}
            cy={scaleY(a.ret)}
            r={3}
            fill="#8b949e"
            opacity={0.5}
          >
            <title>{a.name}: risk={formatIsk(a.risk)}, return={formatIsk(a.ret)}</title>
          </circle>
        ))}

        {/* Current portfolio */}
        <circle cx={scaleX(portRisk(currentWeights))} cy={scaleY(portRet(currentWeights))} r={6} fill="#f0883e" stroke="#f0883e" strokeWidth={2}>
          <title>Current Portfolio</title>
        </circle>

        {/* Optimal portfolio */}
        <circle cx={scaleX(portRisk(optimalWeights))} cy={scaleY(portRet(optimalWeights))} r={6} fill="#58a6ff" stroke="#58a6ff" strokeWidth={2}>
          <title>Optimal (Max Sharpe)</title>
        </circle>

        {/* Min-var portfolio */}
        <circle cx={scaleX(portRisk(minVarWeights))} cy={scaleY(portRet(minVarWeights))} r={5} fill="#3fb950" stroke="#3fb950" strokeWidth={2}>
          <title>Minimum Variance</title>
        </circle>
      </svg>

      {/* Legend */}
      <div className="flex gap-4 justify-center mt-1 text-[9px]">
        <div className="flex items-center gap-1">
          <div className="w-2 h-2 rounded-full bg-[#f0883e]" />
          <span className="text-eve-dim">Current</span>
        </div>
        <div className="flex items-center gap-1">
          <div className="w-2 h-2 rounded-full bg-[#58a6ff]" />
          <span className="text-eve-dim">Optimal</span>
        </div>
        <div className="flex items-center gap-1">
          <div className="w-2 h-2 rounded-full bg-[#3fb950]" />
          <span className="text-eve-dim">Min Var</span>
        </div>
        <div className="flex items-center gap-1">
          <div className="w-2 h-2 rounded-full bg-[#8b949e] opacity-50" />
          <span className="text-eve-dim">Assets</span>
        </div>
      </div>
    </div>
  );
}

// --- Correlation Matrix Heatmap ---

function CorrelationMatrix({ assets, matrix }: { assets: AssetStats[]; matrix: number[][] }) {
  if (assets.length === 0) return null;

  const cellSize = Math.min(28, Math.floor(600 / assets.length));

  const corrColor = (v: number) => {
    if (v >= 0.5) return "bg-emerald-500/80";
    if (v >= 0.2) return "bg-emerald-500/40";
    if (v >= -0.2) return "bg-eve-dim/20";
    if (v >= -0.5) return "bg-red-500/40";
    return "bg-red-500/80";
  };

  return (
    <div className="overflow-x-auto">
      <div className="inline-flex flex-col gap-px">
        {/* Header row */}
        <div className="flex gap-px items-end" style={{ marginLeft: cellSize * 3 }}>
          {assets.map((a, j) => (
            <div
              key={j}
              className="text-[8px] text-eve-dim truncate text-center"
              style={{ width: cellSize }}
              title={a.type_name}
            >
              {a.type_name.slice(0, 4)}
            </div>
          ))}
        </div>
        {/* Matrix rows */}
        {assets.map((rowAsset, i) => (
          <div key={i} className="flex gap-px items-center">
            <div
              className="text-[8px] text-eve-dim truncate text-right pr-1"
              style={{ width: cellSize * 3 }}
              title={rowAsset.type_name}
            >
              {rowAsset.type_name.slice(0, 12)}
            </div>
            {matrix[i].map((val, j) => (
              <div
                key={j}
                className={`flex items-center justify-center text-[8px] font-mono rounded-[2px] ${corrColor(val)} ${
                  i === j ? "ring-1 ring-eve-accent/30" : ""
                }`}
                style={{ width: cellSize, height: cellSize }}
                title={`${rowAsset.type_name} × ${assets[j].type_name}: ${val.toFixed(2)}`}
              >
                {assets.length <= 10 ? val.toFixed(1) : ""}
              </div>
            ))}
          </div>
        ))}
      </div>
    </div>
  );
}

// --- Asset Table ---

function AssetTable({
  assets,
  currentWeights,
  optimalWeights,
  formatIsk,
  t,
}: {
  assets: AssetStats[];
  currentWeights: number[];
  optimalWeights: number[];
  formatIsk: (v: number) => string;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}) {
  return (
    <div className="border border-eve-border rounded-sm overflow-hidden">
      <table className="w-full text-xs">
        <thead className="bg-eve-panel">
          <tr className="text-eve-dim">
            <th className="px-3 py-2 text-left">{t("optAssetName")}</th>
            <th className="px-3 py-2 text-right">{t("optCurrentPct")}</th>
            <th className="px-3 py-2 text-right">{t("optOptimalPct")}</th>
            <th className="px-3 py-2 text-right">{t("optAssetPnL")}</th>
            <th className="px-3 py-2 text-right">{t("optAssetSharpe")}</th>
            <th className="px-3 py-2 text-right">{t("optAssetVol")}</th>
            <th className="px-3 py-2 text-right">{t("optAssetDays")}</th>
          </tr>
        </thead>
        <tbody>
          {assets.map((asset, i) => {
            return (
              <tr key={asset.type_id} className="border-t border-eve-border/50 hover:bg-eve-panel/50">
                <td className="px-3 py-2 text-eve-text">
                  <div className="flex items-center gap-2">
                    <img
                      src={`https://images.evetech.net/types/${asset.type_id}/icon?size=32`}
                      alt=""
                      className="w-5 h-5"
                    />
                    <span className="truncate max-w-[160px]">{asset.type_name || `#${asset.type_id}`}</span>
                  </div>
                </td>
                <td className="px-3 py-2 text-right text-eve-text">
                  <div className="flex items-center justify-end gap-1">
                    <div className="w-12 h-1.5 bg-eve-dark rounded-full overflow-hidden">
                      <div className="h-full rounded-full bg-[#f0883e]" style={{ width: `${currentWeights[i] * 100}%` }} />
                    </div>
                    <span>{(currentWeights[i] * 100).toFixed(1)}%</span>
                  </div>
                </td>
                <td className="px-3 py-2 text-right">
                  <div className="flex items-center justify-end gap-1">
                    <div className="w-12 h-1.5 bg-eve-dark rounded-full overflow-hidden">
                      <div className="h-full rounded-full bg-eve-accent" style={{ width: `${optimalWeights[i] * 100}%` }} />
                    </div>
                    <span className="text-eve-accent">{(optimalWeights[i] * 100).toFixed(1)}%</span>
                  </div>
                </td>
                <td className={`px-3 py-2 text-right ${asset.total_pnl >= 0 ? "text-eve-profit" : "text-eve-error"}`}>
                  {asset.total_pnl >= 0 ? "+" : ""}{formatIsk(asset.total_pnl)}
                </td>
                <td className={`px-3 py-2 text-right ${(asset.sharpe_ratio ?? 0) > 1 ? "text-eve-profit" : (asset.sharpe_ratio ?? 0) > 0 ? "text-eve-text" : "text-eve-error"}`}>
                  {(asset.sharpe_ratio ?? 0).toFixed(2)}
                </td>
                <td className="px-3 py-2 text-right text-eve-dim">{formatIsk(asset.volatility)}</td>
                <td className="px-3 py-2 text-right text-eve-dim">{asset.trading_days}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

// --- Suggestions Panel ---

function SuggestionsPanel({
  suggestions,
  t,
}: {
  suggestions: AllocationSuggestion[];
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}) {
  const actionable = suggestions.filter((s) => s.action !== "hold");
  if (actionable.length === 0) return null;

  const reasonLabels: Record<string, TranslationKey> = {
    high_sharpe: "optReasonHighSharpe",
    diversification: "optReasonDiversification",
    negative_returns: "optReasonNegativeReturns",
    poor_risk_adjusted: "optReasonPoorRiskAdjusted",
    overweight: "optReasonOverweight",
  };

  return (
    <div className="space-y-1.5">
      {actionable.map((s) => {
        const isIncrease = s.action === "increase";
        return (
          <div
            key={s.type_id}
            className={`flex items-center gap-3 px-3 py-2 rounded-sm border text-xs ${
              isIncrease
                ? "bg-emerald-500/5 border-emerald-500/20"
                : "bg-red-500/5 border-red-500/20"
            }`}
          >
            <span className={`text-[10px] font-bold uppercase tracking-wider ${isIncrease ? "text-emerald-400" : "text-red-400"}`}>
              {isIncrease ? t("optIncrease") : t("optDecrease")}
            </span>
            <img
              src={`https://images.evetech.net/types/${s.type_id}/icon?size=32`}
              alt=""
              className="w-5 h-5"
            />
            <span className="text-eve-text font-medium truncate max-w-[150px]">{s.type_name}</span>
            <span className="text-eve-dim">
              {s.current_pct.toFixed(1)}% → {s.optimal_pct.toFixed(1)}%
            </span>
            <span className={`font-mono ${isIncrease ? "text-emerald-400" : "text-red-400"}`}>
              {s.delta_pct >= 0 ? "+" : ""}{s.delta_pct.toFixed(1)}%
            </span>
            {s.reason && (
              <span className="text-eve-dim text-[10px] ml-auto">
                {t(reasonLabels[s.reason] || ("optReasonOverweight" as TranslationKey))}
              </span>
            )}
          </div>
        );
      })}
    </div>
  );
}
