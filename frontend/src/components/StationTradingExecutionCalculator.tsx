import { useState, useEffect, useCallback } from "react";
import { Modal } from "./Modal";
import { getExecutionPlan } from "../lib/api";
import { useI18n, type TranslationKey } from "../lib/i18n";
import type { ExecutionPlanResult } from "../lib/types";

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
  /** Sales tax % (e.g. 8) — deducted from sell revenue in profit */
  salesTaxPercent?: number;
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
          <div className="text-[10px] uppercase tracking-wider text-eve-dim mb-1">{t("execPlanFillCurve")}</div>
          <div className="flex flex-wrap gap-x-2 gap-y-0.5 text-xs font-mono mb-1">
            {plan.depth_levels.slice(0, 8).map((lv, i) => (
              <span key={i} className="text-eve-dim">
                {formatISK(lv.price)}×{lv.volume_filled.toLocaleString()}
              </span>
            ))}
            {plan.depth_levels.length > 8 && (
              <span className="text-eve-dim">+{plan.depth_levels.length - 8}</span>
            )}
          </div>
          <p className="text-[10px] text-eve-dim leading-tight">{t("execPlanFillCurveHint")}</p>
        </div>
      )}
    </div>
  );
}

/**
 * Station Trading Execution Calculator.
 * One station: place BUY order at effective price, place SELL order at effective price.
 * Shows fill curve, slippage, optional TWAP-style slice suggestion.
 * Future: Kyle's λ, quadratic impact, history-calibrated λ.
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
  salesTaxPercent = 0,
  impactDays,
}: StationTradingExecutionCalculatorProps) {
  const { t } = useI18n();
  const [quantity, setQuantity] = useState(defaultQuantity);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [planBuy, setPlanBuy] = useState<ExecutionPlanResult | null>(null);
  const [planSell, setPlanSell] = useState<ExecutionPlanResult | null>(null);

  const fetchBoth = useCallback(
    (qty: number) => {
      if (!typeID || !regionID || !stationID) return;
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
        }),
        getExecutionPlan({
          type_id: typeID,
          region_id: regionID,
          location_id: loc,
          quantity: qty,
          is_buy: false,
          impact_days: impactDays,
        }),
      ])
        .then(([buy, sell]) => {
          setPlanBuy(buy);
          setPlanSell(sell);
        })
        .catch((e) => setError(e.message))
        .finally(() => setLoading(false));
    },
    [typeID, regionID, stationID, impactDays]
  );

  useEffect(() => {
    if (!open || !typeID || !regionID || !stationID) return;
    const q = defaultQuantity > 0 ? defaultQuantity : 100;
    setQuantity(q);
    setPlanBuy(null);
    setPlanSell(null);
    fetchBoth(q);
  }, [open, typeID, regionID, stationID, defaultQuantity]); // eslint-disable-line react-hooks/exhaustive-deps

  const handleCalculate = () => fetchBoth(quantity);

  // Station trading = limit orders: we place BUY at bid (pay bid when filled), SELL at ask (receive ask when filled).
  // planBuy = ask side (sell orders), planSell = bid side (buy orders).
  const bidTotal = planSell?.total_cost ?? 0;   // what we pay when our buy order fills (we placed at bid)
  const askTotal = planBuy?.total_cost ?? 0;    // what we receive when our sell order fills (we placed at ask)
  const brokerMult = 1 - brokerFeePercent / 100;
  const taxMult = 1 - salesTaxPercent / 100;
  const effectiveBuyCost = bidTotal * (1 + brokerFeePercent / 100);
  const sellAfterBroker = askTotal * brokerMult;
  const sellAfterTax = sellAfterBroker * taxMult;
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

        {/* Impact from history (Kyle's λ, √V, TWAP n*) — collapsed by default */}
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
                    <td className="px-3 py-1.5 text-eve-dim w-44">Kyle's λ (ΔP = λ×Q)</td>
                    <td className="px-3 py-1.5 font-mono">{p.lambda.toExponential(4)}</td>
                  </tr>
                  <tr className="border-b border-eve-border">
                    <td colSpan={2} className="px-3 py-0.5 pb-1.5 text-[10px] text-eve-dim italic">{t("execPlanImpactLambdaHuman")}</td>
                  </tr>
                  <tr className="border-b border-eve-border">
                    <td className="px-3 py-1.5 text-eve-dim w-44">η (ΔP = η×√Q)</td>
                    <td className="px-3 py-1.5 font-mono">{p.eta.toFixed(4)}</td>
                  </tr>
                  <tr className="border-b border-eve-border">
                    <td colSpan={2} className="px-3 py-0.5 pb-1.5 text-[10px] text-eve-dim italic">{t("execPlanImpactEtaHuman")}</td>
                  </tr>
                  <tr className="border-b border-eve-border">
                    <td className="px-3 py-1.5 text-eve-dim w-44">σ² (daily returns)</td>
                    <td className="px-3 py-1.5 font-mono">{p.sigma_sq.toExponential(4)}</td>
                  </tr>
                  <tr className="border-b border-eve-border">
                    <td colSpan={2} className="px-3 py-0.5 pb-1.5 text-[10px] text-eve-dim italic">{t("execPlanImpactSigmaHuman")}</td>
                  </tr>
                  <tr className="border-b border-eve-border">
                    <td className="px-3 py-1.5 text-eve-dim w-44">{t("execPlanImpactForQ")}</td>
                    <td className="px-3 py-1.5 font-mono text-eve-accent">≈ {formatISK(imp.recommended_impact)}</td>
                  </tr>
                  <tr className="border-b border-eve-border">
                    <td colSpan={2} className="px-3 py-0.5 pb-1.5 text-[10px] text-eve-dim italic">{t("execPlanImpactForQHuman")}</td>
                  </tr>
                  <tr>
                    <td className="px-3 py-1.5 text-eve-dim w-44">n* (TWAP)</td>
                    <td className="px-3 py-1.5 font-mono">{imp.optimal_slices_twap} {t("execPlanSlices")}</td>
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
                    const placeBuyCost = planSell.best_price * (1 + brokerFeePercent / 100);
                    const placeSellRev = planBuy.best_price * (1 - brokerFeePercent / 100) * (1 - salesTaxPercent / 100);
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
                {salesTaxPercent > 0 && (
                  <span className="text-eve-dim text-xs">
                    {t("execPlanAfterSalesTax", { pct: salesTaxPercent })}
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
