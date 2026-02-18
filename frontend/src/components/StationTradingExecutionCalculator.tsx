import { useState, useEffect, useCallback, useRef } from "react";
import { Modal } from "./Modal";
import { getExecutionPlan } from "../lib/api";
import { useI18n, type TranslationKey } from "../lib/i18n";
import type { ExecutionPlanResult, DepthLevel } from "../lib/types";

export interface StationTradingExecutionCalculatorProps {
  open: boolean;
  onClose: () => void;
  typeID: number;
  typeName: string;
  regionID: number;
  stationID: number;
  defaultQuantity?: number;
  /** Broker fee % (e.g. 3) for profit estimate */
  brokerFeePercent?: number;
  /** Buy-side broker fee %. Used when split fees are enabled. */
  buyBrokerFeePercent?: number;
  /** Sell-side broker fee %. Used when split fees are enabled. */
  sellBrokerFeePercent?: number;
  /** Sales tax % (e.g. 8) — deducted from sell revenue in profit */
  salesTaxPercent?: number;
  /** Buy-side tax %. Used when split fees are enabled. */
  buySalesTaxPercent?: number;
  /** Sell-side tax %. Used when split fees are enabled. */
  sellSalesTaxPercent?: number;
  /** Days of market history for impact calibration (λ, η, n*). Optional. */
  impactDays?: number;
}

function formatISK(value: number): string {
  if (value >= 1e9) return `${(value / 1e9).toFixed(2)}B`;
  if (value >= 1e6) return `${(value / 1e6).toFixed(2)}M`;
  if (value >= 1e3) return `${(value / 1e3).toFixed(1)}K`;
  return value.toFixed(0);
}

/** Block: effective price to place one side (buy or sell) at this station */
function OrderSideBlock({
  title,
  plan,
  t,
  isBuy,
}: {
  title: string;
  plan: ExecutionPlanResult | null;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
  isBuy: boolean;
}) {
  if (!plan) return null;
  return (
    <div className="border border-eve-border rounded-sm overflow-hidden">
      <div className="px-3 py-2 bg-eve-panel border-b border-eve-border text-xs font-semibold text-eve-accent uppercase tracking-wider">
        {title}
      </div>
      <table className="w-full text-sm">
        <tbody className="text-eve-text">
          <tr className="border-b border-eve-border">
            <td className="px-3 py-1.5 text-eve-dim w-40">{t("execPlanBestPrice")}</td>
            <td className="px-3 py-1.5 font-mono text-eve-accent">{formatISK(plan.best_price)}</td>
          </tr>
          <tr className="border-b border-eve-border">
            <td className="px-3 py-1.5 text-eve-dim">{t("execPlanStationEffectivePrice")}</td>
            <td className="px-3 py-1.5 font-mono text-eve-accent">{formatISK(plan.expected_price)}</td>
          </tr>
          <tr className="border-b border-eve-border">
            <td className="px-3 py-1.5 text-eve-dim">{t("execPlanSlippage")}</td>
            <td className="px-3 py-1.5 font-mono">{plan.slippage_percent.toFixed(2)}%</td>
          </tr>
          <tr className="border-b border-eve-border">
            <td className="px-3 py-1.5 text-eve-dim">{t("execPlanTotalCost")}</td>
            <td className="px-3 py-1.5 font-mono text-eve-accent">{formatISK(plan.total_cost)}</td>
          </tr>
          <tr className="border-b border-eve-border">
            <td className="px-3 py-1.5 text-eve-dim">{t("execPlanCanFill")}</td>
            <td className="px-3 py-1.5">{plan.can_fill ? "✓" : "✗"}</td>
          </tr>
          <tr className="border-b border-eve-border">
            <td className="px-3 py-1.5 text-eve-dim">{t("execPlanDepth")}</td>
            <td className="px-3 py-1.5 font-mono">{plan.total_depth.toLocaleString()}</td>
          </tr>
          {plan.optimal_slices > 1 && (
            <tr className="border-b border-eve-border">
              <td className="px-3 py-1.5 text-eve-dim">{t("execPlanSlices")}</td>
              <td className="px-3 py-1.5 font-mono">
                {plan.optimal_slices} × ~{formatISK(plan.total_cost / plan.optimal_slices)}{" "}
                <span className="text-eve-dim text-xs">({t("execPlanGap")} ~{plan.suggested_min_gap} min)</span>
              </td>
            </tr>
          )}
          <tr>
            <td className="px-3 py-1.5 text-eve-dim">{t("execPlanStationPlaceAt")}</td>
            <td className="px-3 py-1.5 font-mono text-eve-accent">
              {isBuy ? "≤ " : "≥ "}
              {formatISK(plan.expected_price)}
            </td>
          </tr>
        </tbody>
      </table>
      {plan.depth_levels && plan.depth_levels.length > 0 && (
        <div className="px-3 py-2 border-t border-eve-border bg-eve-bg/50">
          <div className="text-[10px] uppercase tracking-wider text-eve-dim mb-1.5">{t("execPlanFillCurve")}</div>
          <FillCurveChart levels={plan.depth_levels} expectedPrice={plan.expected_price} isBuy={isBuy} />
          <p className="text-[10px] text-eve-dim leading-tight mt-1.5">{t("execPlanFillCurveHint")}</p>
        </div>
      )}
    </div>
  );
}

// ===================================================================
// Fill Curve SVG Chart — step chart showing price vs cumulative volume
// ===================================================================

const CHART_W = 320;
const CHART_H = 100;
const PAD = { top: 6, right: 8, bottom: 22, left: 52 };

function FillCurveChart({
  levels,
  expectedPrice,
  isBuy,
}: {
  levels: DepthLevel[];
  expectedPrice: number;
  isBuy: boolean;
}) {
  if (levels.length === 0) return null;

  // Data bounds
  const maxCum = levels[levels.length - 1].cumulative || 1;
  const prices = levels.map((l) => l.price);
  const minPrice = Math.min(...prices);
  const maxPrice = Math.max(...prices);
  const priceRange = maxPrice - minPrice || 1;

  // Coordinate mappers
  const plotW = CHART_W - PAD.left - PAD.right;
  const plotH = CHART_H - PAD.top - PAD.bottom;
  const xOf = (cum: number) => PAD.left + (cum / maxCum) * plotW;
  const yOf = (price: number) => PAD.top + plotH - ((price - minPrice) / priceRange) * plotH;

  // Build step path: for each level, draw horizontal then vertical
  const filledLevels = levels.filter((l) => l.volume_filled > 0);
  const totalFilled = filledLevels.reduce((s, l) => s + l.volume_filled, 0);

  let stepPath = "";
  let fillPath = "";

  // Step line (full order book visible)
  levels.forEach((lv, i) => {
    const x1 = xOf(i === 0 ? 0 : levels[i - 1].cumulative);
    const x2 = xOf(lv.cumulative);
    const y = yOf(lv.price);
    if (i === 0) {
      stepPath += `M${x1},${y}`;
    }
    stepPath += ` L${x2},${y}`;
    if (i < levels.length - 1) {
      stepPath += ` L${x2},${yOf(levels[i + 1].price)}`;
    }
  });

  // Filled area (the portion the user's order fills)
  if (filledLevels.length > 0) {
    let fc = 0;
    fillPath = `M${xOf(0)},${yOf(filledLevels[0].price)}`;
    filledLevels.forEach((lv) => {
      const x1 = xOf(fc);
      fc += lv.volume_filled;
      const x2 = xOf(fc);
      const y = yOf(lv.price);
      fillPath += ` L${x1},${y} L${x2},${y}`;
    });
    // Close down to baseline
    fillPath += ` L${xOf(totalFilled)},${yOf(minPrice)} L${xOf(0)},${yOf(minPrice)} Z`;
  }

  // Expected price line
  const epY = yOf(expectedPrice);

  // Axis labels — pick 3 price ticks
  const priceTicks = [minPrice, (minPrice + maxPrice) / 2, maxPrice];
  // Volume ticks — 0 and max
  const volTicks = [0, maxCum];

  return (
    <svg
      viewBox={`0 0 ${CHART_W} ${CHART_H}`}
      className="w-full h-auto"
      style={{ maxHeight: 110 }}
      role="img"
      aria-label="Fill curve chart"
    >
      {/* Grid lines */}
      {priceTicks.map((p, i) => (
        <line key={`hg${i}`} x1={PAD.left} x2={CHART_W - PAD.right} y1={yOf(p)} y2={yOf(p)}
          stroke="currentColor" className="text-eve-border" strokeWidth={0.5} strokeDasharray="2,2" />
      ))}

      {/* Filled area (green for buy, orange for sell) */}
      {fillPath && (
        <path d={fillPath} fill={isBuy ? "rgba(0,180,80,0.2)" : "rgba(230,149,0,0.2)"} />
      )}

      {/* Step line — full depth */}
      <path d={stepPath} fill="none" stroke="currentColor" className="text-eve-dim" strokeWidth={1} />

      {/* Filled portion highlighted */}
      {filledLevels.length > 0 && (() => {
        let fc = 0;
        let fp = `M${xOf(0)},${yOf(filledLevels[0].price)}`;
        filledLevels.forEach((lv) => {
          const x1 = xOf(fc);
          fc += lv.volume_filled;
          const x2 = xOf(fc);
          fp += ` L${x1},${yOf(lv.price)} L${x2},${yOf(lv.price)}`;
        });
        return <path d={fp} fill="none" stroke={isBuy ? "#00b450" : "#e69500"} strokeWidth={1.5} />;
      })()}

      {/* Expected price dashed line */}
      <line x1={PAD.left} x2={CHART_W - PAD.right} y1={epY} y2={epY}
        stroke="#e69500" strokeWidth={0.8} strokeDasharray="3,2" />
      <text x={CHART_W - PAD.right + 2} y={epY + 3} className="text-[7px] fill-eve-accent" textAnchor="start">
        avg
      </text>

      {/* Fill boundary vertical line */}
      {totalFilled > 0 && (
        <>
          <line x1={xOf(totalFilled)} x2={xOf(totalFilled)} y1={PAD.top} y2={CHART_H - PAD.bottom}
            stroke={isBuy ? "#00b450" : "#e69500"} strokeWidth={0.8} strokeDasharray="2,2" />
          <text x={xOf(totalFilled)} y={PAD.top - 1} className="text-[7px]"
            fill={isBuy ? "#00b450" : "#e69500"} textAnchor="middle">
            {formatISK(totalFilled)}
          </text>
        </>
      )}

      {/* Y axis labels (price) */}
      {priceTicks.map((p, i) => (
        <text key={`pl${i}`} x={PAD.left - 4} y={yOf(p) + 3}
          className="text-[7px] fill-eve-dim" textAnchor="end">
          {formatISK(p)}
        </text>
      ))}

      {/* X axis labels (volume) */}
      {volTicks.map((v, i) => (
        <text key={`vl${i}`} x={xOf(v)} y={CHART_H - PAD.bottom + 12}
          className="text-[7px] fill-eve-dim" textAnchor={i === 0 ? "start" : "end"}>
          {v === 0 ? "0" : formatISK(v)}
        </text>
      ))}

      {/* Axis labels */}
      <text x={CHART_W / 2} y={CHART_H - 2} className="text-[7px] fill-eve-dim" textAnchor="middle">
        volume
      </text>
      <text x={4} y={CHART_H / 2} className="text-[7px] fill-eve-dim" textAnchor="middle"
        transform={`rotate(-90, 4, ${CHART_H / 2})`}>
        price
      </text>

      {/* Plot border */}
      <rect x={PAD.left} y={PAD.top} width={plotW} height={plotH}
        fill="none" stroke="currentColor" className="text-eve-border" strokeWidth={0.5} />
    </svg>
  );
}

/**
 * Station Trading Execution Calculator.
 * One station: place BUY order at effective price, place SELL order at effective price.
 * Shows fill curve, slippage, optional TWAP-style slice suggestion.
 * Impact model: Amihud illiquidity, σ√(Q/V) square-root law, volume-based TWAP slicing.
 */
export function StationTradingExecutionCalculator({
  open,
  onClose,
  typeID,
  typeName,
  regionID,
  stationID,
  defaultQuantity = 100,
  brokerFeePercent = 3,
  buyBrokerFeePercent,
  sellBrokerFeePercent,
  salesTaxPercent = 0,
  buySalesTaxPercent,
  sellSalesTaxPercent,
  impactDays,
}: StationTradingExecutionCalculatorProps) {
  const { t } = useI18n();
  const [quantity, setQuantity] = useState(defaultQuantity);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [planBuy, setPlanBuy] = useState<ExecutionPlanResult | null>(null);
  const [planSell, setPlanSell] = useState<ExecutionPlanResult | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const requestSeqRef = useRef(0);

  const fetchBoth = useCallback(
    (qty: number) => {
      if (!typeID || !regionID || !stationID) return;
      const requestID = ++requestSeqRef.current;
      abortRef.current?.abort();
      const controller = new AbortController();
      abortRef.current = controller;
      setError(null);
      setLoading(true);
      const loc = stationID || undefined;
      Promise.all([
        getExecutionPlan({
          type_id: typeID,
          region_id: regionID,
          location_id: loc,
          quantity: qty,
          is_buy: true,
          impact_days: impactDays,
          signal: controller.signal,
        }),
        getExecutionPlan({
          type_id: typeID,
          region_id: regionID,
          location_id: loc,
          quantity: qty,
          is_buy: false,
          impact_days: impactDays,
          signal: controller.signal,
        }),
      ])
        .then(([buy, sell]) => {
          if (controller.signal.aborted || requestID !== requestSeqRef.current) return;
          setPlanBuy(buy);
          setPlanSell(sell);
        })
        .catch((e: unknown) => {
          if (controller.signal.aborted || requestID !== requestSeqRef.current) return;
          const message = e instanceof Error ? e.message : "Failed to calculate execution plan";
          setError(message);
        })
        .finally(() => {
          if (controller.signal.aborted || requestID !== requestSeqRef.current) return;
          setLoading(false);
        });
    },
    [typeID, regionID, stationID, impactDays]
  );

  useEffect(() => {
    if (!open || !typeID || !regionID || !stationID) {
      abortRef.current?.abort();
      return;
    }
    const q = defaultQuantity > 0 ? defaultQuantity : 100;
    setQuantity(q);
    setPlanBuy(null);
    setPlanSell(null);
    fetchBoth(q);
    return () => abortRef.current?.abort();
  }, [open, typeID, regionID, stationID, defaultQuantity]); // eslint-disable-line react-hooks/exhaustive-deps

  const handleCalculate = () => fetchBoth(quantity);

  // Station trading = limit orders: we place BUY at bid (pay bid when filled), SELL at ask (receive ask when filled).
  // planBuy = ask side (sell orders), planSell = bid side (buy orders).
  const bidTotal = planSell?.total_cost ?? 0;   // what we pay when our buy order fills (we placed at bid)
  const askTotal = planBuy?.total_cost ?? 0;    // what we receive when our sell order fills (we placed at ask)
  const buyBrokerPct = buyBrokerFeePercent ?? brokerFeePercent;
  const sellBrokerPct = sellBrokerFeePercent ?? brokerFeePercent;
  const buyTaxPct = buySalesTaxPercent ?? 0;
  const sellTaxPct = sellSalesTaxPercent ?? salesTaxPercent;
  const sellRevenueMult = Math.max(0, 1 - (sellBrokerPct + sellTaxPct) / 100);
  const effectiveBuyCost = bidTotal * (1 + (buyBrokerPct + buyTaxPct) / 100);
  const sellAfterTax = askTotal * sellRevenueMult;
  const profit = sellAfterTax - effectiveBuyCost;
  const canFillBoth = planBuy?.can_fill && planSell?.can_fill;

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={`${t("execPlanStationCalculator")}: ${typeName}`}
      width="max-w-3xl"
    >
      <div className="p-4 flex flex-col gap-4">
        <p className="text-xs text-eve-dim">{t("execPlanStationHint")}</p>
        <div className="flex flex-wrap items-center gap-4">
          <label className="flex items-center gap-2 text-sm text-eve-dim">
            <span>{t("execPlanQuantity")}:</span>
            <input
              type="number"
              min={1}
              value={quantity}
              onChange={(e) => setQuantity(Math.max(1, parseInt(e.target.value, 10) || 1))}
              className="w-28 px-2 py-1 bg-eve-input border border-eve-border rounded-sm text-eve-text font-mono"
            />
          </label>
          <button
            type="button"
            onClick={handleCalculate}
            disabled={loading}
            className="px-3 py-1.5 bg-eve-accent/20 border border-eve-accent rounded-sm text-eve-accent hover:bg-eve-accent/30 disabled:opacity-50 text-sm font-medium"
          >
            {loading ? "..." : t("execPlanCalculate")}
          </button>
        </div>

        {error && <div className="text-eve-error text-sm">{error}</div>}

        {/* Left = place BUY limit at BID (we pay bid when filled). Right = place SELL limit at ASK (we receive ask when filled). */}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <OrderSideBlock
            title={t("execPlanStationPlaceBuy")}
            plan={planSell}
            t={t}
            isBuy={true}
          />
          <OrderSideBlock
            title={t("execPlanStationPlaceSell")}
            plan={planBuy}
            t={t}
            isBuy={false}
          />
        </div>

        {/* Impact from history (Amihud, σ, TWAP slices) — collapsed by default */}
        {(planBuy?.impact || planSell?.impact) && (() => {
          const imp = planBuy?.impact ?? planSell?.impact!;
          const p = imp.params;
          return (
            <details className="group border border-eve-border rounded-sm overflow-hidden" open={false}>
              <summary className="list-none cursor-pointer">
                <div className="px-3 py-2 bg-eve-panel border-b border-eve-border text-xs font-semibold text-eve-accent uppercase tracking-wider flex items-center gap-2 hover:bg-eve-panel/80">
                  <span className="group-open:rotate-90 transition-transform">▶</span>
                  {t("execPlanImpactFromHistory")}
                </div>
              </summary>
              <p className="px-3 py-2 text-xs text-eve-dim border-b border-eve-border bg-eve-bg/30">
                {t("execPlanImpactBlockIntro")}
              </p>
              <table className="w-full text-sm">
                <tbody className="text-eve-text">
                  <tr className="border-b border-eve-border">
                    <td className="px-3 py-1.5 text-eve-dim w-44">{t("execPlanImpactAmihud")}</td>
                    <td className="px-3 py-1.5 font-mono">{p.amihud.toExponential(4)}</td>
                  </tr>
                  <tr className="border-b border-eve-border">
                    <td colSpan={2} className="px-3 py-0.5 pb-1.5 text-[10px] text-eve-dim italic">{t("execPlanImpactAmihudHuman")}</td>
                  </tr>
                  <tr className="border-b border-eve-border">
                    <td className="px-3 py-1.5 text-eve-dim w-44">{t("execPlanImpactSigmaLabel")}</td>
                    <td className="px-3 py-1.5 font-mono">{(p.sigma * 100).toFixed(2)}%</td>
                  </tr>
                  <tr className="border-b border-eve-border">
                    <td colSpan={2} className="px-3 py-0.5 pb-1.5 text-[10px] text-eve-dim italic">{t("execPlanImpactSigmaHuman")}</td>
                  </tr>
                  <tr className="border-b border-eve-border">
                    <td className="px-3 py-1.5 text-eve-dim w-44">{t("execPlanImpactAvgVolume")}</td>
                    <td className="px-3 py-1.5 font-mono">{Math.round(p.avg_daily_volume).toLocaleString()}</td>
                  </tr>
                  <tr className="border-b border-eve-border">
                    <td colSpan={2} className="px-3 py-0.5 pb-1.5 text-[10px] text-eve-dim italic">{t("execPlanImpactAvgVolumeHuman")}</td>
                  </tr>
                  <tr className="border-b border-eve-border">
                    <td className="px-3 py-1.5 text-eve-dim w-44">{t("execPlanImpactForQ")}</td>
                    <td className="px-3 py-1.5 font-mono text-eve-accent">
                      ≈ {imp.recommended_impact_pct.toFixed(2)}%
                      {imp.recommended_impact_isk > 0 && <span className="text-eve-dim ml-2">({formatISK(imp.recommended_impact_isk)})</span>}
                    </td>
                  </tr>
                  <tr className="border-b border-eve-border">
                    <td colSpan={2} className="px-3 py-0.5 pb-1.5 text-[10px] text-eve-dim italic">{t("execPlanImpactForQHuman")}</td>
                  </tr>
                  <tr>
                    <td className="px-3 py-1.5 text-eve-dim w-44">{t("execPlanImpactTwapLabel")}</td>
                    <td className="px-3 py-1.5 font-mono">{imp.optimal_slices} {t("execPlanSlices")}</td>
                  </tr>
                  <tr>
                    <td colSpan={2} className="px-3 py-0.5 pb-1.5 text-[10px] text-eve-dim italic">{t("execPlanImpactTwapHuman")}</td>
                  </tr>
                </tbody>
              </table>
              <div className="px-3 py-1.5 text-[10px] text-eve-dim border-t border-eve-border">
                {t("execPlanImpactDaysUsed", { days: p.days_used })}
              </div>
            </details>
          );
        })()}

        {planBuy && planSell && (
          <>
            <div className="border border-eve-border rounded-sm p-3 bg-eve-panel">
              <div className="text-xs text-eve-dim uppercase tracking-wider mb-2">
                {t("execPlanStationPlaceOrders")}
              </div>
              <div className="flex flex-col gap-1 text-sm text-eve-text">
                <span className="text-eve-dim text-xs">{t("execPlanStationPlaceOrdersHint")}</span>
                <span>
                  {t("execPlanStationPlaceOrdersSpread", {
                    bid: formatISK(planSell.best_price),
                    ask: formatISK(planBuy.best_price),
                    spread: formatISK(planBuy.best_price - planSell.best_price),
                  })}
                </span>
                <span>
                  {(() => {
                    const placeBuyCost =
                      planSell.best_price *
                      (1 + (buyBrokerPct + buyTaxPct) / 100);
                    const placeSellRev = planBuy.best_price * sellRevenueMult;
                    const placeProfit = placeSellRev - placeBuyCost;
                    return placeProfit >= 0
                      ? t("execPlanStationPlaceOrdersProfit", { profit: formatISK(placeProfit) })
                      : t("execPlanStationPlaceOrdersLoss", { loss: formatISK(-placeProfit) });
                  })()}
                </span>
              </div>
            </div>
            <div className="border border-eve-border rounded-sm p-3 bg-eve-panel">
              <div className="text-xs text-eve-dim uppercase tracking-wider mb-2">
                {t("execPlanStationSummary")}
              </div>
              <div className="flex flex-col gap-1 text-sm">
                {canFillBoth ? (
                  <span className="text-green-400">✓ {t("execPlanCanFill")}</span>
                ) : (
                  <span className="text-eve-error">
                    ✗ {!planSell.can_fill ? t("execPlanBuy") : t("execPlanSell")} — {t("execPlanCanFill")} {t("execPlanCannotFill")}
                  </span>
                )}
                {sellTaxPct > 0 && (
                  <span className="text-eve-dim text-xs">
                    {t("execPlanAfterSalesTax", { pct: sellTaxPct })}
                  </span>
                )}
                <span className="text-eve-dim text-xs">
                  {t("execPlanStationLimitOrderSummary")}
                </span>
                <span className="text-eve-text">
                  {t("execPlanStationSummary", {
                    qty: quantity.toLocaleString(),
                    buyCost: formatISK(effectiveBuyCost),
                    sellRevenue: formatISK(sellAfterTax),
                    result:
                      profit >= 0
                        ? t("execPlanSummaryProfit", { profit: formatISK(profit) })
                        : t("execPlanSummaryLoss", { loss: formatISK(-profit) }),
                  })}
                </span>
              </div>
            </div>
          </>
        )}

        <p className="text-[10px] text-eve-dim border-t border-eve-border pt-2">
          {t("execPlanStationImpactNote")}
        </p>
      </div>
    </Modal>
  );
}
