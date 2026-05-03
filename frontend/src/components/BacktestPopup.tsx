import { useEffect, useMemo, useRef, useState } from "react";
import {
  ColorType,
  CrosshairMode,
  LineSeries,
  LineStyle,
  createChart,
} from "lightweight-charts";
import type { LineData, Time } from "lightweight-charts";
import { checkOrderBookCoverage, cleanupOrderBook, getOrderBookStats, runFlipBacktest } from "@/lib/api";
import type {
  FlipBacktestEquityPoint,
  FlipBacktestResult,
  FlipResult,
  OrderBookCleanupPlan,
  OrderBookCoverageResult,
  OrderBookStats,
} from "@/lib/types";
import { formatISK, formatMargin } from "@/lib/format";
import { Modal } from "./Modal";

type QuantityMode = "scan" | "fixed" | "budget";
type BuyPriceSource = "history" | "scan";
type StrategyMode = "hold" | "instant_flip";
type InstantPriceMode = "scan_spread" | "history_pair" | "recorded_orderbook";
type CooldownMode = "manual" | "route_time";
type RouteSafetyMode = "manual" | "auto";

interface Props {
  open: boolean;
  onClose: () => void;
  rows: FlipResult[];
  salesTaxPercent?: number;
  brokerFeePercent?: number;
  splitTradeFees?: boolean;
  buyBrokerFeePercent?: number;
  sellBrokerFeePercent?: number;
  buySalesTaxPercent?: number;
  sellSalesTaxPercent?: number;
  cargoCapacity?: number;
}

export function BacktestPopup({
  open,
  onClose,
  rows,
  salesTaxPercent = 0,
  brokerFeePercent = 0,
  splitTradeFees = false,
  buyBrokerFeePercent,
  sellBrokerFeePercent,
  buySalesTaxPercent,
  sellSalesTaxPercent,
  cargoCapacity = 0,
}: Props) {
  const [holdDays, setHoldDays] = useState(7);
  const [windowDays, setWindowDays] = useState(90);
  const [maxRows, setMaxRows] = useState(100);
  const [strategyMode, setStrategyMode] = useState<StrategyMode>("hold");
  const [instantPriceMode, setInstantPriceMode] = useState<InstantPriceMode>("scan_spread");
  const [entrySpacingDays, setEntrySpacingDays] = useState(1);
  const [travelCooldownDays, setTravelCooldownDays] = useState(1);
  const [orderbookCooldownMinutes, setOrderbookCooldownMinutes] = useState(60);
  const [orderbookMaxAgeMinutes, setOrderbookMaxAgeMinutes] = useState(15);
  const [cooldownMode, setCooldownMode] = useState<CooldownMode>("manual");
  const [routeCargoCapacity, setRouteCargoCapacity] = useState(Math.max(0, Math.round(cargoCapacity)));
  const [routeMinutesPerJump, setRouteMinutesPerJump] = useState(2);
  const [routeDockMinutes, setRouteDockMinutes] = useState(4);
  const [routeSafetyMultiplier, setRouteSafetyMultiplier] = useState(1);
  const [routeSafetyMode, setRouteSafetyMode] = useState<RouteSafetyMode>("manual");
  const [routeMinSecurity, setRouteMinSecurity] = useState(0);
  const [routeMinCooldownMinutes, setRouteMinCooldownMinutes] = useState(0);
  const [nonOverlapping, setNonOverlapping] = useState(true);
  const [quantityMode, setQuantityMode] = useState<QuantityMode>("scan");
  const [fixedQuantity, setFixedQuantity] = useState(100);
  const [budgetISK, setBudgetISK] = useState(100_000_000);
  const [buyPriceSource, setBuyPriceSource] = useState<BuyPriceSource>("history");
  const [volumeFillFraction, setVolumeFillFraction] = useState(100);
  const [skipUnfillable, setSkipUnfillable] = useState(false);
  const [buyPriceMarkup, setBuyPriceMarkup] = useState(0);
  const [sellPriceHaircut, setSellPriceHaircut] = useState(0);
  const [minROI, setMinROI] = useState(0);
  const [includeOpenTrades, setIncludeOpenTrades] = useState(false);
  const [loading, setLoading] = useState(false);
  const [coverageLoading, setCoverageLoading] = useState(false);
  const [error, setError] = useState("");
  const [coverageError, setCoverageError] = useState("");
  const [result, setResult] = useState<FlipBacktestResult | null>(null);
  const [coverage, setCoverage] = useState<OrderBookCoverageResult | null>(null);
  const [orderbookStats, setOrderbookStats] = useState<OrderBookStats | null>(null);
  const [orderbookStatsLoading, setOrderbookStatsLoading] = useState(false);
  const [orderbookStatsError, setOrderbookStatsError] = useState("");
  const [orderbookCleanup, setOrderbookCleanup] = useState<OrderBookCleanupPlan | null>(null);
  const [orderbookCleanupLoading, setOrderbookCleanupLoading] = useState(false);
  const [orderbookCleanupError, setOrderbookCleanupError] = useState("");
  const [orderbookKeepDays, setOrderbookKeepDays] = useState(90);
  const [orderbookVacuum, setOrderbookVacuum] = useState(false);

  const rowsForBacktest = useMemo(() => rows.slice(0, maxRows), [maxRows, rows]);
  const recordedBookMode = strategyMode === "instant_flip" && instantPriceMode === "recorded_orderbook";

  useEffect(() => {
    if (!open) return;
    setError("");
    setCoverageError("");
    setResult(null);
    setCoverage(null);
    setOrderbookCleanup(null);
    setOrderbookCleanupError("");
  }, [open, rows]);

  useEffect(() => {
    if (!open) return;
    setRouteCargoCapacity(Math.max(0, Math.round(cargoCapacity)));
  }, [cargoCapacity, open]);

  useEffect(() => {
    if (!open || !recordedBookMode || orderbookStats) return;
    void refreshOrderbookStats();
  }, [open, recordedBookMode, orderbookStats]);

  const buildBacktestPayload = () => ({
    rows: rowsForBacktest,
    strategy_mode: strategyMode,
    instant_price_mode: instantPriceMode,
    hold_days: holdDays,
    window_days: windowDays,
    max_rows: maxRows,
    entry_spacing_days: entrySpacingDays,
    travel_cooldown_days: travelCooldownDays,
    orderbook_cooldown_minutes: orderbookCooldownMinutes,
    orderbook_max_age_minutes: orderbookMaxAgeMinutes,
    cooldown_mode: cooldownMode,
    cargo_capacity: routeCargoCapacity,
    route_minutes_per_jump: routeMinutesPerJump,
    route_dock_minutes: routeDockMinutes,
    route_safety_multiplier: routeSafetyMultiplier,
    route_safety_mode: routeSafetyMode,
    route_min_security: routeMinSecurity,
    route_min_cooldown_minutes: routeMinCooldownMinutes,
    non_overlapping: nonOverlapping,
    quantity_mode: quantityMode,
    fixed_quantity: fixedQuantity,
    budget_isk: budgetISK,
    buy_price_source: buyPriceSource,
    volume_fill_fraction: volumeFillFraction,
    skip_unfillable: skipUnfillable,
    buy_price_markup_percent: buyPriceMarkup,
    sell_price_haircut_percent: sellPriceHaircut,
    min_roi_percent: minROI,
    exclude_open_trades: !includeOpenTrades,
    sales_tax_percent: salesTaxPercent,
    broker_fee_percent: brokerFeePercent,
    split_trade_fees: splitTradeFees,
    buy_broker_fee_percent: buyBrokerFeePercent,
    sell_broker_fee_percent: sellBrokerFeePercent,
    buy_sales_tax_percent: buySalesTaxPercent,
    sell_sales_tax_percent: sellSalesTaxPercent,
  } as const);

  const run = async () => {
    if (rowsForBacktest.length === 0 || loading) return;
    setLoading(true);
    setError("");
    try {
      const data = await runFlipBacktest(buildBacktestPayload());
      setResult(data);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Backtest failed");
    } finally {
      setLoading(false);
    }
  };

  const checkCoverage = async () => {
    if (!recordedBookMode || rowsForBacktest.length === 0 || coverageLoading) return;
    setCoverageLoading(true);
    setCoverageError("");
    try {
      const data = await checkOrderBookCoverage({
        rows: rowsForBacktest,
        window_days: windowDays,
        max_rows: maxRows,
        orderbook_max_age_minutes: orderbookMaxAgeMinutes,
        orderbook_cooldown_minutes: orderbookCooldownMinutes,
      });
      setCoverage(data);
    } catch (e) {
      setCoverageError(e instanceof Error ? e.message : "Coverage check failed");
    } finally {
      setCoverageLoading(false);
    }
  };

  const refreshOrderbookStats = async () => {
    if (orderbookStatsLoading) return;
    setOrderbookStatsLoading(true);
    setOrderbookStatsError("");
    try {
      const data = await getOrderBookStats(6);
      setOrderbookStats(data);
    } catch (e) {
      setOrderbookStatsError(e instanceof Error ? e.message : "Stats failed");
    } finally {
      setOrderbookStatsLoading(false);
    }
  };

  const runOrderbookCleanup = async (dryRun: boolean) => {
    if (!recordedBookMode || orderbookCleanupLoading) return;
    if (!dryRun) {
      const ok = window.confirm(`Delete recorded orderbooks older than ${orderbookKeepDays} days?`);
      if (!ok) return;
    }
    setOrderbookCleanupLoading(true);
    setOrderbookCleanupError("");
    try {
      const data = await cleanupOrderBook({
        keep_days: orderbookKeepDays,
        dry_run: dryRun,
        vacuum: !dryRun && orderbookVacuum,
      });
      setOrderbookCleanup(data);
      if (!dryRun) {
        setCoverage(null);
        await refreshOrderbookStats();
      }
    } catch (e) {
      setOrderbookCleanupError(e instanceof Error ? e.message : "Cleanup failed");
    } finally {
      setOrderbookCleanupLoading(false);
    }
  };

  const summary = result?.summary;
  const ledger = result?.ledger?.slice(-14).reverse() ?? [];

  return (
    <Modal open={open} onClose={onClose} title="Paper Backtest" width="max-w-6xl">
      <div className="p-4 space-y-4 text-xs text-eve-text">
        <div className="grid grid-cols-2 md:grid-cols-4 xl:grid-cols-6 gap-3">
          <SelectControl
            label="Mode"
            value={strategyMode}
            onChange={(v) => setStrategyMode(v as StrategyMode)}
            options={[
              ["hold", "Hold cycle"],
              ["instant_flip", "Instant flip"],
            ]}
          />
          {strategyMode === "instant_flip" && (
            <SelectControl
              label="Price model"
              value={instantPriceMode}
              onChange={(v) => setInstantPriceMode(v as InstantPriceMode)}
              options={[
                ["scan_spread", "Scan spread"],
                ["history_pair", "History pair"],
                ["recorded_orderbook", "Recorded book"],
              ]}
            />
          )}
          {strategyMode === "hold" ? (
            <NumberControl label="Hold days" min={1} max={90} value={holdDays} onChange={setHoldDays} />
          ) : instantPriceMode === "recorded_orderbook" && cooldownMode === "manual" ? (
            <NumberControl label="Cooldown min" min={1} max={10080} value={orderbookCooldownMinutes} onChange={setOrderbookCooldownMinutes} />
          ) : instantPriceMode === "recorded_orderbook" ? (
            <NumberControl label="Min cooldown" min={0} max={10080} value={routeMinCooldownMinutes} onChange={setRouteMinCooldownMinutes} />
          ) : (
            <NumberControl label="Cooldown days" min={1} max={30} value={travelCooldownDays} onChange={setTravelCooldownDays} />
          )}
          {strategyMode === "instant_flip" && instantPriceMode === "recorded_orderbook" && (
            <NumberControl label="Max age min" min={1} max={1440} value={orderbookMaxAgeMinutes} onChange={setOrderbookMaxAgeMinutes} />
          )}
          {strategyMode === "instant_flip" && instantPriceMode === "recorded_orderbook" && (
            <SelectControl
              label="Cooldown"
              value={cooldownMode}
              onChange={(v) => setCooldownMode(v as CooldownMode)}
              options={[
                ["manual", "Manual"],
                ["route_time", "Route time"],
              ]}
            />
          )}
          {strategyMode === "instant_flip" && instantPriceMode === "recorded_orderbook" && cooldownMode === "route_time" && (
            <>
              <NumberControl label="Cargo m3" min={0} max={10_000_000} value={routeCargoCapacity} onChange={setRouteCargoCapacity} />
              <NumberControl label="Min/jump" min={0.1} max={60} step={0.1} value={routeMinutesPerJump} onChange={setRouteMinutesPerJump} />
              <NumberControl label="Dock min" min={0} max={120} step={0.5} value={routeDockMinutes} onChange={setRouteDockMinutes} />
              <SelectControl
                label="Safety"
                value={routeSafetyMode}
                onChange={(v) => setRouteSafetyMode(v as RouteSafetyMode)}
                options={[
                  ["manual", "Manual"],
                  ["auto", "Auto risk"],
                ]}
              />
              {routeSafetyMode === "manual" ? (
                <NumberControl label="Safety x" min={0.1} max={10} step={0.1} value={routeSafetyMultiplier} onChange={setRouteSafetyMultiplier} />
              ) : (
                <NumberControl label="Min sec" min={0} max={1} step={0.05} value={routeMinSecurity} onChange={setRouteMinSecurity} />
              )}
            </>
          )}
          <NumberControl label="Window days" min={7} max={365} value={windowDays} onChange={setWindowDays} />
          <NumberControl label="Max rows" min={1} max={500} value={maxRows} onChange={setMaxRows} />
          <NumberControl label="Entry every" min={1} max={30} value={entrySpacingDays} onChange={setEntrySpacingDays} />
          <SelectControl
            label="Qty mode"
            value={quantityMode}
            onChange={(v) => setQuantityMode(v as QuantityMode)}
            options={[
              ["scan", "Scan qty"],
              ["fixed", "Fixed qty"],
              ["budget", "Budget"],
            ]}
          />
          {(strategyMode === "hold" || instantPriceMode === "history_pair") && (
            <SelectControl
              label="Buy price"
              value={buyPriceSource}
              onChange={(v) => setBuyPriceSource(v as BuyPriceSource)}
              options={[
                ["history", "History"],
                ["scan", "Scan price"],
              ]}
            />
          )}
          {quantityMode === "fixed" && (
            <NumberControl label="Fixed qty" min={1} max={1_000_000_000} value={fixedQuantity} onChange={setFixedQuantity} />
          )}
          {quantityMode === "budget" && (
            <NumberControl label="Budget ISK" min={1_000_000} max={10_000_000_000_000} step={1_000_000} value={budgetISK} onChange={setBudgetISK} />
          )}
          <NumberControl label="Volume %" min={1} max={100} value={volumeFillFraction} onChange={setVolumeFillFraction} />
          <NumberControl label="Buy markup %" min={0} max={100} value={buyPriceMarkup} onChange={setBuyPriceMarkup} />
          <NumberControl label="Sell haircut %" min={0} max={100} value={sellPriceHaircut} onChange={setSellPriceHaircut} />
          <NumberControl label="Min ROI %" min={-100} max={1000} value={minROI} onChange={setMinROI} />
        </div>

        <div className="flex flex-wrap items-center gap-3">
          <CheckControl label="Skip unfillable" checked={skipUnfillable} onChange={setSkipUnfillable} />
          {strategyMode === "hold" && (
            <>
              <CheckControl label="Non-overlap entries" checked={nonOverlapping} onChange={setNonOverlapping} />
              <CheckControl label="Include open MTM" checked={includeOpenTrades} onChange={setIncludeOpenTrades} />
            </>
          )}
          <button
            type="button"
            onClick={() => void run()}
            disabled={loading || rowsForBacktest.length === 0}
            className="px-3 py-1.5 rounded-sm bg-eve-accent text-black font-semibold uppercase tracking-wide disabled:opacity-50"
          >
            {loading ? "Running..." : "Run"}
          </button>
          {recordedBookMode && (
            <button
              type="button"
              onClick={() => void checkCoverage()}
              disabled={coverageLoading || rowsForBacktest.length === 0}
              className="px-3 py-1.5 rounded-sm border border-eve-border bg-eve-panel text-eve-text font-semibold uppercase tracking-wide disabled:opacity-50"
            >
              {coverageLoading ? "Checking..." : "Check coverage"}
            </button>
          )}
          <div className="text-eve-dim">
            {rowsForBacktest.length} / {rows.length} rows
          </div>
        </div>

        {recordedBookMode && (
          <CoveragePanel coverage={coverage} error={coverageError} loading={coverageLoading} />
        )}
        {recordedBookMode && (
          <OrderbookMaintenancePanel
            stats={orderbookStats}
            statsLoading={orderbookStatsLoading}
            statsError={orderbookStatsError}
            cleanup={orderbookCleanup}
            cleanupLoading={orderbookCleanupLoading}
            cleanupError={orderbookCleanupError}
            keepDays={orderbookKeepDays}
            vacuum={orderbookVacuum}
            onKeepDaysChange={setOrderbookKeepDays}
            onVacuumChange={setOrderbookVacuum}
            onRefresh={() => void refreshOrderbookStats()}
            onPreview={() => void runOrderbookCleanup(true)}
            onCleanup={() => void runOrderbookCleanup(false)}
          />
        )}

        {error && (
          <div className="border border-red-500/50 bg-red-950/30 text-red-300 rounded-sm px-3 py-2">
            {error}
          </div>
        )}
        {result?.warnings?.length ? (
          <div className="border border-amber-500/40 bg-amber-950/20 text-amber-200 rounded-sm px-3 py-2">
            {result.warnings.join("; ")}
          </div>
        ) : null}

        {summary ? (
          <>
            <div className="grid grid-cols-2 md:grid-cols-4 gap-2">
              <Metric label="Realized" value={formatISK(summary.realized_pnl)} tone={summary.realized_pnl >= 0 ? "profit" : "loss"} />
              <Metric label="Open MTM" value={formatISK(summary.mtm_pnl)} tone={summary.mtm_pnl >= 0 ? "profit" : "loss"} />
              <Metric label={summary.open_trades > 0 ? "Net incl MTM" : "Net PnL"} value={formatISK(summary.total_pnl)} tone={summary.total_pnl >= 0 ? "profit" : "loss"} />
              <Metric label="Win rate" value={formatMargin(summary.win_rate)} />
              <Metric label="Avg ROI" value={formatMargin(summary.avg_roi)} tone={summary.avg_roi >= 0 ? "profit" : "loss"} />
              <Metric label="Drawdown" value={formatISK(summary.max_drawdown_isk)} />
              <Metric label="Trades" value={`${summary.closed_trades}/${summary.open_trades} closed/open`} />
              <Metric label="Window" value={formatWindowMetric(summary)} />
              {summary.cooldown_mode === "route_time" && (
                <Metric label="Max route time" value={formatMinutes(summary.max_route_time_minutes ?? 0)} />
              )}
              {summary.cooldown_mode === "route_time" && (
                <Metric label="Max safety x" value={`${(summary.max_route_safety_multiplier ?? 0).toFixed(2)}x`} />
              )}
            </div>

            {summary.open_trades > 0 && (
              <div className="border border-amber-500/40 bg-amber-950/20 text-amber-200 rounded-sm px-3 py-2">
                Net includes open mark-to-market positions. For closed-trade validation, use Realized or disable Include open MTM.
              </div>
            )}

            <EquityChart points={result.equity ?? []} />

            <div className="grid grid-cols-1 lg:grid-cols-2 gap-3">
              <div className="border border-eve-border rounded-sm overflow-hidden">
                <div className="px-3 py-2 bg-eve-dark/60 text-eve-dim uppercase tracking-wide text-[10px]">
                  Items
                </div>
                <div className="max-h-64 overflow-auto">
                  <table className="w-full">
                    <thead className="text-eve-dim bg-eve-panel sticky top-0">
                      <tr>
                        <th className="px-2 py-1 text-left">Item</th>
                        <th className="px-2 py-1 text-right">PnL</th>
                        <th className="px-2 py-1 text-right">Win</th>
                        <th className="px-2 py-1 text-right">Fill</th>
                      </tr>
                    </thead>
                    <tbody>
                      {(result.items ?? []).slice(0, 20).map((item) => (
                        <tr key={item.type_id} className="border-t border-eve-border/40">
                          <td className="px-2 py-1 truncate max-w-[220px]">{item.type_name}</td>
                          <td className={`px-2 py-1 text-right font-mono ${item.total_pnl >= 0 ? "text-green-400" : "text-red-300"}`}>
                            {formatISK(item.total_pnl)}
                          </td>
                          <td className="px-2 py-1 text-right font-mono text-eve-dim">{formatMargin(item.win_rate)}</td>
                          <td className="px-2 py-1 text-right font-mono text-eve-dim">{formatMargin(item.fill_rate)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>

              <div className="border border-eve-border rounded-sm overflow-hidden">
                <div className="px-3 py-2 bg-eve-dark/60 text-eve-dim uppercase tracking-wide text-[10px]">
                  Ledger tail
                </div>
                <div className="max-h-64 overflow-auto">
                  <table className="w-full">
                    <thead className="text-eve-dim bg-eve-panel sticky top-0">
                      <tr>
                        <th className="px-2 py-1 text-left">Exit</th>
                        <th className="px-2 py-1 text-left">Item</th>
                        <th className="px-2 py-1 text-right">Qty</th>
                        <th className="px-2 py-1 text-right">PnL</th>
                        <th className="px-2 py-1 text-right">ROI</th>
                      </tr>
                    </thead>
                    <tbody>
                      {ledger.map((tr, idx) => (
                        <tr key={`${tr.type_id}:${tr.entry_date}:${idx}`} className="border-t border-eve-border/40">
                          <td className="px-2 py-1 text-eve-dim">{tr.exit_date}{tr.status === "open" ? " *" : ""}</td>
                          <td className="px-2 py-1 truncate max-w-[160px]">{tr.type_name}</td>
                          <td className="px-2 py-1 text-right font-mono text-eve-dim">{tr.quantity.toLocaleString()}</td>
                          <td className={`px-2 py-1 text-right font-mono ${tr.pnl >= 0 ? "text-green-400" : "text-red-300"}`}>
                            {formatISK(tr.pnl)}
                          </td>
                          <td className="px-2 py-1 text-right font-mono text-eve-dim">{formatMargin(tr.roi_percent)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            </div>
          </>
        ) : (
          <div className="border border-eve-border bg-eve-dark/40 rounded-sm px-3 py-6 text-center text-eve-dim">
            Run a backtest on the selected or visible scan rows.
          </div>
        )}
      </div>
    </Modal>
  );
}

function OrderbookMaintenancePanel({
  stats,
  statsLoading,
  statsError,
  cleanup,
  cleanupLoading,
  cleanupError,
  keepDays,
  vacuum,
  onKeepDaysChange,
  onVacuumChange,
  onRefresh,
  onPreview,
  onCleanup,
}: {
  stats: OrderBookStats | null;
  statsLoading: boolean;
  statsError: string;
  cleanup: OrderBookCleanupPlan | null;
  cleanupLoading: boolean;
  cleanupError: string;
  keepDays: number;
  vacuum: boolean;
  onKeepDaysChange: (value: number) => void;
  onVacuumChange: (value: boolean) => void;
  onRefresh: () => void;
  onPreview: () => void;
  onCleanup: () => void;
}) {
  const topTypes = stats?.top_types ?? [];
  const topLocations = stats?.top_locations ?? [];

  return (
    <div className="border border-eve-border rounded-sm overflow-hidden">
      <div className="flex flex-wrap items-center justify-between gap-2 px-3 py-2 bg-eve-dark/60">
        <div className="text-eve-dim uppercase tracking-wide text-[10px]">Orderbook DB</div>
        <button
          type="button"
          onClick={onRefresh}
          disabled={statsLoading}
          className="px-2 py-1 rounded-sm border border-eve-border bg-eve-panel text-eve-text font-semibold uppercase tracking-wide disabled:opacity-50"
        >
          {statsLoading ? "Refreshing..." : "Refresh stats"}
        </button>
      </div>

      <div className="p-3 space-y-3">
        {statsError && (
          <div className="border border-red-500/50 bg-red-950/30 text-red-300 rounded-sm px-3 py-2">
            {statsError}
          </div>
        )}
        {!statsLoading && !statsError && !stats && (
          <div className="text-eve-dim">Stats not loaded</div>
        )}

        {stats && (
          <>
            <div className="grid grid-cols-2 md:grid-cols-4 xl:grid-cols-6 gap-2">
              <Metric label="Snapshots" value={formatWhole(stats.snapshot_count)} />
              <Metric label="Levels" value={formatWhole(stats.level_count)} />
              <Metric label="Types" value={formatWhole(stats.unique_type_count)} />
              <Metric label="Locations" value={formatWhole(stats.unique_location_count)} />
              <Metric label="Volume left" value={formatWhole(stats.total_volume_remain)} />
              <Metric label="Approx size" value={formatBytes(stats.approx_bytes)} />
            </div>

            <div className="flex flex-wrap gap-x-4 gap-y-1 text-[10px] text-eve-dim">
              <span>Oldest {formatCapture(stats.oldest_captured_at)}</span>
              <span>Newest {formatCapture(stats.newest_captured_at)}</span>
            </div>

            <div className="grid grid-cols-1 lg:grid-cols-2 gap-3">
              <OrderbookTopTable
                title="Top types"
                firstLabel="Type"
                rows={topTypes.slice(0, 6).map((row) => ({
                  id: row.type_id,
                  snapshots: row.snapshot_count,
                  levels: row.level_count,
                  volume: row.volume_remain,
                }))}
              />
              <OrderbookTopTable
                title="Top locations"
                firstLabel="Location"
                rows={topLocations.slice(0, 6).map((row) => ({
                  id: row.location_id,
                  snapshots: row.snapshot_count,
                  levels: row.level_count,
                  volume: row.volume_remain,
                }))}
              />
            </div>
          </>
        )}

        <div className="flex flex-wrap items-end gap-3">
          <div className="w-36">
            <NumberControl label="Keep days" min={1} max={3650} value={keepDays} onChange={onKeepDaysChange} />
          </div>
          <CheckControl label="Vacuum after cleanup" checked={vacuum} onChange={onVacuumChange} />
          <button
            type="button"
            onClick={onPreview}
            disabled={cleanupLoading}
            className="px-3 py-1.5 rounded-sm border border-eve-border bg-eve-panel text-eve-text font-semibold uppercase tracking-wide disabled:opacity-50"
          >
            {cleanupLoading ? "Working..." : "Preview cleanup"}
          </button>
          <button
            type="button"
            onClick={onCleanup}
            disabled={cleanupLoading}
            className="px-3 py-1.5 rounded-sm bg-red-500/80 text-black font-semibold uppercase tracking-wide disabled:opacity-50"
          >
            Cleanup
          </button>
        </div>

        {cleanupError && (
          <div className="border border-red-500/50 bg-red-950/30 text-red-300 rounded-sm px-3 py-2">
            {cleanupError}
          </div>
        )}
        {cleanup && (
          <div className="border border-eve-border/60 bg-eve-dark/40 rounded-sm px-3 py-2">
            <div className="font-mono">
              {cleanup.dry_run ? "Preview" : "Deleted"}: {formatWhole(cleanup.snapshots_deleted)} snapshots / {formatWhole(cleanup.levels_deleted)} levels
            </div>
            <div className="text-[10px] text-eve-dim">
              Cutoff {formatCapture(cleanup.cutoff)} / remaining {formatCapture(cleanup.oldest_remaining)} to {formatCapture(cleanup.newest_remaining)}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

function OrderbookTopTable({
  title,
  firstLabel,
  rows,
}: {
  title: string;
  firstLabel: string;
  rows: Array<{ id: number; snapshots: number; levels: number; volume: number }>;
}) {
  return (
    <div className="border border-eve-border/60 rounded-sm overflow-hidden">
      <div className="px-3 py-2 bg-eve-dark/60 text-eve-dim uppercase tracking-wide text-[10px]">
        {title}
      </div>
      <table className="w-full">
        <thead className="text-eve-dim bg-eve-panel">
          <tr>
            <th className="px-2 py-1 text-left">{firstLabel}</th>
            <th className="px-2 py-1 text-right">Snaps</th>
            <th className="px-2 py-1 text-right">Levels</th>
            <th className="px-2 py-1 text-right">Volume</th>
          </tr>
        </thead>
        <tbody>
          {rows.length === 0 ? (
            <tr>
              <td colSpan={4} className="px-2 py-2 text-eve-dim text-center">No data</td>
            </tr>
          ) : (
            rows.map((row) => (
              <tr key={row.id} className="border-t border-eve-border/40">
                <td className="px-2 py-1 font-mono">{row.id}</td>
                <td className="px-2 py-1 text-right font-mono text-eve-dim">{formatWhole(row.snapshots)}</td>
                <td className="px-2 py-1 text-right font-mono text-eve-dim">{formatWhole(row.levels)}</td>
                <td className="px-2 py-1 text-right font-mono text-eve-dim">{formatWhole(row.volume)}</td>
              </tr>
            ))
          )}
        </tbody>
      </table>
    </div>
  );
}

function CoveragePanel({
  coverage,
  error,
  loading,
}: {
  coverage: OrderBookCoverageResult | null;
  error: string;
  loading: boolean;
}) {
  const rows = coverage?.rows
    ? [...coverage.rows].sort((a, b) => {
        if (a.status === b.status) return a.type_name.localeCompare(b.type_name);
        if (a.status === "ready") return 1;
        if (b.status === "ready") return -1;
        return a.status.localeCompare(b.status);
      })
    : [];

  return (
    <div className="border border-eve-border rounded-sm overflow-hidden">
      <div className="flex flex-wrap items-center justify-between gap-2 px-3 py-2 bg-eve-dark/60">
        <div className="text-eve-dim uppercase tracking-wide text-[10px]">Orderbook coverage</div>
        {coverage?.summary ? (
          <div className="font-mono text-[10px] text-eve-dim">
            {coverage.summary.rows_ready}/{coverage.summary.rows_tested} ready
          </div>
        ) : null}
      </div>

      <div className="p-3 space-y-3">
        {loading && <div className="text-eve-dim">Checking recorded books...</div>}
        {error && (
          <div className="border border-red-500/50 bg-red-950/30 text-red-300 rounded-sm px-3 py-2">
            {error}
          </div>
        )}
        {!loading && !error && !coverage && (
          <div className="text-eve-dim">Not checked</div>
        )}

        {coverage?.warnings?.length ? (
          <div className="border border-amber-500/40 bg-amber-950/20 text-amber-200 rounded-sm px-3 py-2">
            {coverage.warnings.join("; ")}
          </div>
        ) : null}

        {coverage?.summary ? (
          <>
            <div className="grid grid-cols-2 md:grid-cols-5 gap-2">
              <Metric label="Ready rows" value={`${coverage.summary.rows_ready}/${coverage.summary.rows_tested}`} tone={coverage.summary.rows_ready > 0 ? "profit" : "loss"} />
              <Metric label="Ready %" value={formatMargin(coverage.summary.ready_percent)} tone={coverage.summary.ready_percent > 0 ? "profit" : "loss"} />
              <Metric label="Paired books" value={formatWhole(coverage.summary.paired_books)} />
              <Metric label="Source books" value={formatWhole(coverage.summary.source_books)} />
              <Metric label="Target books" value={formatWhole(coverage.summary.target_books)} />
              <Metric label="Source depth" value={formatWhole(coverage.summary.source_depth)} />
              <Metric label="Target depth" value={formatWhole(coverage.summary.target_depth)} />
              <Metric label="Missing source" value={formatWhole(coverage.summary.rows_missing_source)} tone={coverage.summary.rows_missing_source > 0 ? "loss" : "neutral"} />
              <Metric label="Missing target" value={formatWhole(coverage.summary.rows_missing_target)} tone={coverage.summary.rows_missing_target > 0 ? "loss" : "neutral"} />
              <Metric label="No pairs" value={formatWhole(coverage.summary.rows_no_pairs)} tone={coverage.summary.rows_no_pairs > 0 ? "loss" : "neutral"} />
            </div>

            <div className="flex flex-wrap gap-x-4 gap-y-1 text-[10px] text-eve-dim">
              <span>Oldest {formatCapture(coverage.summary.oldest_capture)}</span>
              <span>Newest {formatCapture(coverage.summary.newest_capture)}</span>
              <span>Window {coverage.summary.backtest_days}d</span>
              <span>Max age {coverage.summary.max_age_minutes}m</span>
            </div>

            <div className="max-h-44 overflow-auto border border-eve-border/60 rounded-sm">
              <table className="w-full">
                <thead className="text-eve-dim bg-eve-panel sticky top-0">
                  <tr>
                    <th className="px-2 py-1 text-left">Item</th>
                    <th className="px-2 py-1 text-left">Status</th>
                    <th className="px-2 py-1 text-right">Src</th>
                    <th className="px-2 py-1 text-right">Tgt</th>
                    <th className="px-2 py-1 text-right">Pairs</th>
                    <th className="px-2 py-1 text-right">Depth</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.slice(0, 24).map((row, idx) => (
                    <tr key={`${row.type_id}:${row.status}:${idx}`} className="border-t border-eve-border/40">
                      <td className="px-2 py-1 truncate max-w-[220px]">{row.type_name || row.type_id}</td>
                      <td className={`px-2 py-1 ${row.status === "ready" ? "text-green-400" : "text-amber-200"}`}>
                        {coverageStatusLabel(row.status)}
                        {row.reason ? <span className="text-eve-dim"> · {row.reason}</span> : null}
                      </td>
                      <td className="px-2 py-1 text-right font-mono text-eve-dim">{formatWhole(row.source_books)}</td>
                      <td className="px-2 py-1 text-right font-mono text-eve-dim">{formatWhole(row.target_books)}</td>
                      <td className="px-2 py-1 text-right font-mono text-eve-dim">{formatWhole(row.paired_books)}</td>
                      <td className="px-2 py-1 text-right font-mono text-eve-dim">{formatWhole(Math.min(row.source_depth, row.target_depth))}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </>
        ) : null}
      </div>
    </div>
  );
}

function formatWindowMetric(summary: FlipBacktestResult["summary"]): string {
  if (summary.data_source === "recorded_orderbook") {
    if (summary.cooldown_mode === "route_time") {
      const safety = summary.route_safety_mode === "auto" ? "auto risk" : "manual risk";
      return `${summary.backtest_days}d / route avg ${formatMinutes(summary.avg_route_time_minutes ?? 0)} / ${safety}`;
    }
    return `${summary.backtest_days}d / cool ${summary.cooldown_minutes ?? 60}m / age ${summary.orderbook_max_age_minutes ?? 15}m`;
  }
  if (summary.strategy_mode === "instant_flip") {
    return `${summary.backtest_days}d / cooldown ${summary.travel_cooldown_days ?? 1}d`;
  }
  return `${summary.backtest_days}d / hold ${summary.hold_days}d`;
}

function formatMinutes(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return "0m";
  if (value < 60) return `${Math.round(value)}m`;
  const hours = value / 60;
  if (hours < 24) return `${hours.toFixed(hours >= 10 ? 0 : 1)}h`;
  const days = hours / 24;
  return `${days.toFixed(days >= 10 ? 0 : 1)}d`;
}

function coverageStatusLabel(status: string): string {
  switch (status) {
    case "ready":
      return "Ready";
    case "missing_source":
      return "Missing source";
    case "missing_target":
      return "Missing target";
    case "no_pairs":
      return "No pair";
    case "invalid_scope":
      return "Invalid scope";
    case "query_error":
      return "Query error";
    default:
      return status || "Unknown";
  }
}

function formatWhole(value: number): string {
  if (!Number.isFinite(value)) return "0";
  return Math.round(value).toLocaleString();
}

function formatBytes(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let size = value;
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit += 1;
  }
  return `${size.toFixed(size >= 10 || unit === 0 ? 0 : 1)} ${units[unit]}`;
}

function formatCapture(value: string): string {
  if (!value) return "none";
  const dt = new Date(value);
  if (Number.isNaN(dt.getTime())) return value;
  return dt.toLocaleString(undefined, {
    month: "short",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function NumberControl({
  label,
  min,
  max,
  step = 1,
  value,
  onChange,
}: {
  label: string;
  min: number;
  max: number;
  step?: number;
  value: number;
  onChange: (value: number) => void;
}) {
  return (
    <label className="space-y-1 min-w-0">
      <div className="text-eve-dim uppercase tracking-wide text-[10px] truncate">{label}</div>
      <input
        type="number"
        min={min}
        max={max}
        step={step}
        value={value}
        onChange={(e) => onChange(clampNumber(Number(e.target.value), min, max, value))}
        className="w-full px-2 py-1 bg-eve-input border border-eve-border rounded-sm text-eve-text"
      />
    </label>
  );
}

function SelectControl({
  label,
  value,
  onChange,
  options,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  options: Array<[string, string]>;
}) {
  return (
    <label className="space-y-1 min-w-0">
      <div className="text-eve-dim uppercase tracking-wide text-[10px] truncate">{label}</div>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="w-full px-2 py-1 bg-eve-input border border-eve-border rounded-sm text-eve-text"
      >
        {options.map(([optionValue, optionLabel]) => (
          <option key={optionValue} value={optionValue}>{optionLabel}</option>
        ))}
      </select>
    </label>
  );
}

function CheckControl({
  label,
  checked,
  onChange,
}: {
  label: string;
  checked: boolean;
  onChange: (value: boolean) => void;
}) {
  return (
    <label className="inline-flex items-center gap-2 text-eve-dim">
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        className="accent-eve-accent"
      />
      <span>{label}</span>
    </label>
  );
}

function EquityChart({ points }: { points: FlipBacktestEquityPoint[] }) {
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!containerRef.current || points.length === 0) return;

    const bgColor = cssColor("--eve-dark", "#0d1117");
    const textColor = cssColor("--eve-dim", "#8b949e");
    const gridColor = cssColor("--eve-border", "#30363d");
    const accentColor = cssColor("--eve-accent", "#e69500");
    const realizedColor = cssColor("--eve-success", "#3fb950");
    const drawdownColor = cssColor("--eve-error", "#dc3c3c");

    const chart = createChart(containerRef.current, {
      layout: {
        background: { type: ColorType.Solid, color: bgColor },
        textColor,
        fontFamily: "ui-monospace, SFMono-Regular, 'SF Mono', Menlo, monospace",
        fontSize: 10,
        attributionLogo: false,
      },
      grid: { vertLines: { color: gridColor }, horzLines: { color: gridColor } },
      crosshair: {
        mode: CrosshairMode.Normal,
        vertLine: { color: `${accentColor}66`, width: 1, style: LineStyle.Dashed },
        horzLine: { color: `${accentColor}66`, width: 1, style: LineStyle.Dashed },
      },
      rightPriceScale: { borderColor: gridColor, scaleMargins: { top: 0.12, bottom: 0.18 } },
      timeScale: { borderColor: gridColor, timeVisible: false, fixLeftEdge: true, fixRightEdge: true },
      localization: {
        priceFormatter: (price: number) => formatISK(price),
      },
      handleScale: { axisPressedMouseMove: { time: true, price: true } },
      handleScroll: { mouseWheel: true, pressedMouseMove: true },
    });

    const toLineData = (field: "equity" | "realized" | "drawdown"): LineData<Time>[] =>
      points.map((p) => ({
        time: p.date as Time,
        value: field === "drawdown" ? -p.drawdown : p[field],
      }));

    const equitySeries = chart.addSeries(LineSeries, {
      color: accentColor,
      lineWidth: 2,
      priceLineVisible: true,
      lastValueVisible: true,
    });
    equitySeries.setData(toLineData("equity"));

    const realizedSeries = chart.addSeries(LineSeries, {
      color: realizedColor,
      lineWidth: 1,
      priceLineVisible: false,
      lastValueVisible: false,
    });
    realizedSeries.setData(toLineData("realized"));

    const drawdownSeries = chart.addSeries(LineSeries, {
      color: drawdownColor,
      lineWidth: 1,
      lineStyle: LineStyle.Dashed,
      priceLineVisible: false,
      lastValueVisible: false,
    });
    drawdownSeries.setData(toLineData("drawdown"));

    chart.timeScale().fitContent();

    const resize = () => {
      if (!containerRef.current) return;
      const { width, height } = containerRef.current.getBoundingClientRect();
      chart.resize(width, height);
    };
    const ro = new ResizeObserver(resize);
    ro.observe(containerRef.current);
    resize();

    return () => {
      ro.disconnect();
      chart.remove();
    };
  }, [points]);

  return (
    <div className="border border-eve-border rounded-sm overflow-hidden">
      <div className="flex flex-wrap items-center justify-between gap-2 px-3 py-2 bg-eve-dark/60">
        <div className="text-eve-dim uppercase tracking-wide text-[10px]">Equity curve</div>
        <div className="flex items-center gap-3">
          <LegendDot color={cssColor("--eve-accent", "#e69500")} label="Equity" />
          <LegendDot color={cssColor("--eve-success", "#3fb950")} label="Realized" />
          <LegendDot color={cssColor("--eve-error", "#dc3c3c")} label="Drawdown" />
        </div>
      </div>
      {points.length > 0 ? (
        <div ref={containerRef} className="w-full h-[220px]" />
      ) : (
        <div className="h-[160px] flex items-center justify-center text-eve-dim">
          No equity data
        </div>
      )}
    </div>
  );
}

function LegendDot({ color, label }: { color: string; label: string }) {
  return (
    <div className="flex items-center gap-1">
      <span className="w-2 h-2 rounded-full shrink-0" style={{ backgroundColor: color }} />
      <span className="text-[10px] text-eve-dim">{label}</span>
    </div>
  );
}

function Metric({ label, value, tone = "neutral" }: { label: string; value: string; tone?: "neutral" | "profit" | "loss" }) {
  const valueClass = tone === "profit" ? "text-green-400" : tone === "loss" ? "text-red-300" : "text-eve-text";
  return (
    <div className="border border-eve-border/60 bg-eve-dark/60 rounded-sm px-2 py-1.5">
      <div className="text-[10px] uppercase tracking-wide text-eve-dim">{label}</div>
      <div className={`font-mono font-semibold ${valueClass}`}>{value}</div>
    </div>
  );
}

function clampNumber(value: number, min: number, max: number, fallback: number): number {
  if (!Number.isFinite(value)) return fallback;
  return Math.max(min, Math.min(max, value));
}

function cssColor(name: string, fallback: string): string {
  if (typeof window === "undefined") return fallback;
  const val = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  if (!val) return fallback;
  const parts = val.split(/\s+/).map(Number);
  if (parts.length === 3 && parts.every((n) => !Number.isNaN(n))) {
    return `#${parts.map((n) => n.toString(16).padStart(2, "0")).join("")}`;
  }
  return fallback;
}
