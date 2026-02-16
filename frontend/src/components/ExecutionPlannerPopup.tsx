import { useState, useEffect, useCallback } from "react";
import { Modal } from "./Modal";
import { getExecutionPlan } from "../lib/api";
import { useI18n, type TranslationKey } from "../lib/i18n";
import type { ExecutionPlanResult } from "../lib/types";

export interface ExecutionPlannerPopupProps {
  open: boolean;
  onClose: () => void;
  typeID: number;
  typeName: string;
  /** Region (and optional station) for BUY side */
  regionID: number;
  locationID?: number;
  /** Region (and optional station) for SELL side. If not set, same as regionID (e.g. station trading). */
  sellRegionID?: number;
  sellLocationID?: number;
  defaultQuantity?: number;
  isBuy?: boolean;
  /** Legacy broker fee % applied to both buy and sell sides when split is disabled. */
  brokerFeePercent?: number;
  /** Sales tax % (legacy sell tax). Deducted from sell revenue in profit calculation. */
  salesTaxPercent?: number;
  /** Enables side-specific buy/sell fees in calculator output. */
  splitTradeFees?: boolean;
  buyBrokerFeePercent?: number;
  sellBrokerFeePercent?: number;
  buySalesTaxPercent?: number;
  sellSalesTaxPercent?: number;
}

function formatISK(value: number): string {
  if (value >= 1e9) return `${(value / 1e9).toFixed(2)}B`;
  if (value >= 1e6) return `${(value / 1e6).toFixed(2)}M`;
  if (value >= 1e3) return `${(value / 1e3).toFixed(1)}K`;
  return value.toFixed(0);
}

function clampPercent(value: number): number {
  if (!Number.isFinite(value)) return 0;
  if (value < 0) return 0;
  if (value > 100) return 100;
  return value;
}

function PlanBlock({
  title,
  plan,
  t,
}: {
  title: string;
  plan: ExecutionPlanResult | null;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
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
            <td className="px-3 py-1.5 text-eve-dim">{t("execPlanExpectedPrice")}</td>
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
          {plan.depth_levels?.[0] != null && (
            <tr>
              <td className="px-3 py-1.5 text-eve-dim">{t("execPlanVolumeAtBest")}</td>
              <td className="px-3 py-1.5 font-mono">{plan.depth_levels[0].volume.toLocaleString()}</td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

export function ExecutionPlannerPopup({
  open,
  onClose,
  typeID,
  typeName,
  regionID,
  locationID = 0,
  sellRegionID,
  sellLocationID,
  defaultQuantity = 100,
  brokerFeePercent = 0,
  salesTaxPercent = 0,
  splitTradeFees = false,
  buyBrokerFeePercent,
  sellBrokerFeePercent,
  buySalesTaxPercent,
  sellSalesTaxPercent,
}: ExecutionPlannerPopupProps) {
  const { t } = useI18n();
  const [quantity, setQuantity] = useState(defaultQuantity);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [planBuy, setPlanBuy] = useState<ExecutionPlanResult | null>(null);
  const [planSell, setPlanSell] = useState<ExecutionPlanResult | null>(null);

  const buyRegion = regionID;
  const buyLocation = locationID || undefined;
  const sellRegion = sellRegionID ?? regionID;
  const sellLocation = sellLocationID ?? (locationID || undefined);

  const fetchBoth = useCallback(
    (qty: number) => {
      if (!typeID || !buyRegion) return;
      setError(null);
      setLoading(true);
      Promise.all([
        getExecutionPlan({
          type_id: typeID,
          region_id: buyRegion,
          location_id: buyLocation,
          quantity: qty,
          is_buy: true,
        }),
        getExecutionPlan({
          type_id: typeID,
          region_id: sellRegion,
          location_id: sellLocation,
          quantity: qty,
          is_buy: false,
        }),
      ])
        .then(([buy, sell]) => {
          setPlanBuy(buy);
          setPlanSell(sell);
        })
        .catch((e) => setError(e.message))
        .finally(() => setLoading(false));
    },
    [typeID, buyRegion, buyLocation, sellRegion, sellLocation]
  );

  // При открытии подтягиваем количество из строки и сразу считаем оба плана (покупка + продажа)
  useEffect(() => {
    if (!open || !typeID || !regionID) return;
    const q = defaultQuantity > 0 ? defaultQuantity : 100;
    setQuantity(q);
    setPlanBuy(null);
    setPlanSell(null);
    fetchBoth(q);
  }, [open, typeID, regionID, defaultQuantity]); // eslint-disable-line react-hooks/exhaustive-deps

  const handleCalculate = () => {
    fetchBoth(quantity);
  };

  const buyCost = planBuy?.total_cost ?? 0;
  const sellRevenueGross = planSell?.total_cost ?? 0;
  const buyBrokerPct = clampPercent(
    splitTradeFees
      ? (buyBrokerFeePercent ?? brokerFeePercent)
      : brokerFeePercent
  );
  const sellBrokerPct = clampPercent(
    splitTradeFees
      ? (sellBrokerFeePercent ?? brokerFeePercent)
      : brokerFeePercent
  );
  const buyTaxPct = clampPercent(splitTradeFees ? (buySalesTaxPercent ?? 0) : 0);
  const sellTaxPct = clampPercent(
    splitTradeFees
      ? (sellSalesTaxPercent ?? salesTaxPercent)
      : salesTaxPercent
  );
  const effectiveBuyCost =
    buyCost * (1 + (buyBrokerPct + buyTaxPct) / 100);
  const sellRevenueAfterFees =
    sellRevenueGross *
    (1 - sellBrokerPct / 100) *
    (1 - sellTaxPct / 100);
  const profit = sellRevenueAfterFees - effectiveBuyCost;
  const canFillBoth = planBuy?.can_fill && planSell?.can_fill;

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={`${t("execPlanCalculator")}: ${typeName}`}
      width="max-w-3xl"
    >
      <div className="p-4 flex flex-col gap-4">
        <p className="text-xs text-eve-dim">{t("execPlanCalculatorHint")}</p>
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

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <PlanBlock title={t("execPlanBuy")} plan={planBuy} t={t} />
          <PlanBlock title={t("execPlanSell")} plan={planSell} t={t} />
        </div>

        {/* Итого: смогу ли продать то что купил, какая прибыль/убыток */}
        {planBuy && planSell && (
          <div className="border border-eve-border rounded-sm p-3 bg-eve-panel">
            <div className="text-xs text-eve-dim uppercase tracking-wider mb-2">
              {t("execPlanCanSellWhatBought")}
            </div>
            <div className="flex flex-col gap-1 text-sm">
              {canFillBoth ? (
                <span className="text-green-400">✓ {t("execPlanCanFill")}</span>
              ) : (
                <span className="text-eve-error">
                  ✗ {!planBuy.can_fill ? t("execPlanBuy") : t("execPlanSell")} — {t("execPlanCanFill")} {t("execPlanCannotFill")}
                </span>
              )}
              {sellTaxPct > 0 && (
                <span className="text-eve-dim text-xs">
                  {t("execPlanAfterSalesTax", { pct: sellTaxPct })}
                </span>
              )}
              <span className="text-eve-text">
                {t("execPlanSummary", {
                  qty: quantity.toLocaleString(),
                  buyCost: formatISK(effectiveBuyCost),
                  sellRevenue: formatISK(sellRevenueAfterFees),
                  result:
                    profit >= 0
                      ? t("execPlanSummaryProfit", { profit: formatISK(profit) })
                      : t("execPlanSummaryLoss", { loss: formatISK(-profit) }),
                })}
              </span>
            </div>
          </div>
        )}
      </div>
    </Modal>
  );
}
