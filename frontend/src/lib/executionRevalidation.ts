import { getExecutionQuote, type CharacterScope } from "./api";
import type {
  ExecutionQuote,
  ExecutionRevalidationReport,
  ExecutionRevalidationRow,
  ExecutionRevalidationStatus,
  FlipResult,
} from "./types";

export interface RevalidationFeeOptions {
  brokerFeePercent?: number;
  salesTaxPercent?: number;
  splitTradeFees?: boolean;
  buyBrokerFeePercent?: number;
  sellBrokerFeePercent?: number;
  buySalesTaxPercent?: number;
  sellSalesTaxPercent?: number;
}

export interface RevalidateRowsOptions extends RevalidationFeeOptions {
  characterId?: CharacterScope;
  signal?: AbortSignal;
  concurrency?: number;
}

function finiteNumber(value: unknown): number {
  const n = Number(value);
  return Number.isFinite(n) ? n : 0;
}

function pctDelta(oldValue: number, newValue: number): number {
  if (!(oldValue > 0)) return newValue > 0 ? 100 : 0;
  return ((newValue - oldValue) / oldValue) * 100;
}

export function executionRowKey(row: FlipResult): string {
  return [
    row.TypeID,
    finiteNumber(row.BuyLocationID) || row.BuyStation || row.BuySystemID,
    finiteNumber(row.SellLocationID) || row.SellStation || row.SellSystemID,
  ].join(":");
}

export function estimatedExecutableQty(row: FlipResult): number {
  const filled = Math.floor(finiteNumber(row.FilledQty));
  if (filled > 0) return filled;
  const recommended = Math.floor(finiteNumber(row.UnitsToBuy));
  if (recommended > 0) return recommended;
  const buyRemain = Math.floor(Math.max(0, finiteNumber(row.BuyOrderRemain)));
  const sellRemain = Math.floor(Math.max(0, finiteNumber(row.SellOrderRemain)));
  if (buyRemain > 0 && sellRemain > 0) return Math.min(buyRemain, sellRemain);
  return Math.max(buyRemain, sellRemain);
}

export function oldBuyPriceForRow(row: FlipResult): number {
  return finiteNumber(row.ExpectedBuyPrice) || finiteNumber(row.BuyPrice);
}

export function oldSellPriceForRow(row: FlipResult): number {
  return finiteNumber(row.ExpectedSellPrice) || finiteNumber(row.SellPrice);
}

export function oldProfitForRow(row: FlipResult, qty = estimatedExecutableQty(row)): number {
  const realProfit = finiteNumber(row.RealProfit);
  if (row.RealProfit != null && Number.isFinite(realProfit)) return realProfit;
  const dayPeriod = finiteNumber(row.DayPeriodProfit);
  if (row.DayPeriodProfit != null && Number.isFinite(dayPeriod)) return dayPeriod;
  const dayNow = finiteNumber(row.DayNowProfit);
  if (row.DayNowProfit != null && Number.isFinite(dayNow)) return dayNow;
  const total = finiteNumber(row.TotalProfit);
  if (total > 0) return total;
  return (oldSellPriceForRow(row) - oldBuyPriceForRow(row)) * qty;
}

function shippingInputsForRow(row: FlipResult, qty: number): {
  shippingRate: number;
  shippingJumps: number;
} {
  const volume = finiteNumber(row.Volume);
  let jumps = Math.floor(finiteNumber(row.SellJumps));
  if (jumps <= 0) {
    jumps = Math.floor(finiteNumber(row.TotalJumps) - finiteNumber(row.BuyJumps));
  }
  if (jumps <= 0) jumps = 0;
  const oldShipping = finiteNumber(row.DayShippingCost);
  if (!(oldShipping > 0) || !(volume > 0) || !(qty > 0) || !(jumps > 0)) {
    return { shippingRate: 0, shippingJumps: jumps };
  }
  return { shippingRate: oldShipping / (volume * qty * jumps), shippingJumps: jumps };
}

function cleanReason(reason: unknown): string {
  return String(reason ?? "").replace(/_/g, " ").trim();
}

function pushReason(reasons: string[], reason: unknown): void {
  const clean = cleanReason(reason);
  if (clean && !reasons.includes(clean)) {
    reasons.push(clean);
  }
}

function warningReasons(quote: ExecutionQuote): string[] {
  const reasons: string[] = [];
  for (const warning of quote.warnings ?? []) {
    switch (warning) {
      case "esi_market_orders_may_be_cached":
      case "buy_order_cache_meta_unavailable":
      case "sell_order_cache_meta_unavailable":
        break;
      default:
        pushReason(reasons, warning);
    }
  }
  pushReason(reasons, quote.partial_reason);
  return reasons;
}

function classifyRevalidation(row: FlipResult, quote: ExecutionQuote, oldProfit: number): {
  status: ExecutionRevalidationStatus;
  avoid: boolean;
  reasons: string[];
} {
  const reasons = warningReasons(quote);
  const decision = String(quote.decision ?? "").toUpperCase();
  const oldQty = estimatedExecutableQty(row);
  const qtyChanged = quote.fill_qty !== oldQty;
  const deltaProfit = quote.net_profit - oldProfit;
  const profitChangePct = Math.abs(pctDelta(oldProfit, quote.net_profit));
  const buyDriftPct = Math.abs(pctDelta(oldBuyPriceForRow(row), quote.buy_vwap));
  const sellDriftPct = Math.abs(pctDelta(oldSellPriceForRow(row), quote.sell_vwap));

  if (decision === "DANGER") pushReason(reasons, "quote marked danger");
  if (decision === "CHANGED") pushReason(reasons, "quote marked changed");
  if (quote.fill_qty <= 0) pushReason(reasons, "no executable quantity");
  if (quote.fill_qty < oldQty) pushReason(reasons, "less depth than scan");
  if (quote.net_profit <= 0) pushReason(reasons, "profit no longer positive");
  if (quote.cache?.stale) pushReason(reasons, "market cache is stale");

  const danger =
    decision === "DANGER" ||
    quote.fill_qty <= 0 ||
    quote.net_profit <= 0 ||
    quote.fill_qty < Math.max(1, Math.floor(oldQty * 0.5));
  if (danger) {
    return { status: "DANGER", avoid: true, reasons };
  }

  if (qtyChanged) pushReason(reasons, "quantity changed");
  if (profitChangePct >= 10 || Math.abs(deltaProfit) >= 1_000_000) {
    pushReason(reasons, "profit changed materially");
  }
  if (buyDriftPct >= 2) pushReason(reasons, "buy VWAP moved");
  if (sellDriftPct >= 2) pushReason(reasons, "sell VWAP moved");
  if ((quote.cache?.buy_age_seconds ?? 0) > 900 || (quote.cache?.sell_age_seconds ?? 0) > 900) {
    pushReason(reasons, "cache age is high");
  }

  if (reasons.length > 0) {
    return { status: "CHANGED", avoid: false, reasons };
  }
  return { status: "SAFE", avoid: false, reasons: ["still executable"] };
}

async function revalidateOne(row: FlipResult, options: RevalidateRowsOptions): Promise<ExecutionRevalidationRow> {
  const oldQty = estimatedExecutableQty(row);
  const oldBuy = oldBuyPriceForRow(row);
  const oldSell = oldSellPriceForRow(row);
  const oldProfit = oldProfitForRow(row, oldQty);
  const buyRegionID = finiteNumber(row.BuyRegionID) || finiteNumber(row.SellRegionID);
  const sellRegionID = finiteNumber(row.SellRegionID) || buyRegionID;
  const { shippingRate, shippingJumps } = shippingInputsForRow(row, oldQty);

  try {
    if (!row.TypeID || !buyRegionID || !sellRegionID || oldQty <= 0) {
      throw new Error("missing type, region, or quantity");
    }
    const quote = await getExecutionQuote({
      type_id: row.TypeID,
      quantity: oldQty,
      buy_region_id: buyRegionID,
      buy_system_id: finiteNumber(row.BuySystemID),
      buy_location_id: finiteNumber(row.BuyLocationID),
      sell_region_id: sellRegionID,
      sell_system_id: finiteNumber(row.SellSystemID),
      sell_location_id: finiteNumber(row.SellLocationID),
      packaged_volume_m3: finiteNumber(row.Volume),
      shipping_cost_per_m3_jump: shippingRate,
      shipping_jumps: shippingJumps,
      broker_fee_percent: options.brokerFeePercent,
      sales_tax_percent: options.salesTaxPercent,
      split_trade_fees: options.splitTradeFees,
      buy_broker_fee_percent: options.buyBrokerFeePercent,
      sell_broker_fee_percent: options.sellBrokerFeePercent,
      buy_sales_tax_percent: options.buySalesTaxPercent,
      sell_sales_tax_percent: options.sellSalesTaxPercent,
      character_id: options.characterId,
      signal: options.signal,
    });
    const classified = classifyRevalidation(row, quote, oldProfit);
    return {
      key: executionRowKey(row),
      row,
      quote,
      status: classified.status,
      oldQty,
      nowQty: quote.fill_qty,
      oldBuy,
      nowBuy: quote.buy_vwap,
      oldSell,
      nowSell: quote.sell_vwap,
      oldProfit,
      nowProfit: quote.net_profit,
      deltaProfit: quote.net_profit - oldProfit,
      qtyChanged: quote.fill_qty !== oldQty,
      avoid: classified.avoid,
      reasons: classified.reasons,
    };
  } catch (error) {
    const reason = error instanceof Error ? error.message : "revalidation failed";
    return {
      key: executionRowKey(row),
      row,
      status: "DANGER",
      oldQty,
      nowQty: 0,
      oldBuy,
      nowBuy: 0,
      oldSell,
      nowSell: 0,
      oldProfit,
      nowProfit: 0,
      deltaProfit: -oldProfit,
      qtyChanged: true,
      avoid: true,
      reasons: [reason],
    };
  }
}

async function runLimited<T>(tasks: Array<() => Promise<T>>, concurrency: number): Promise<T[]> {
  const results = new Array<T>(tasks.length);
  let next = 0;
  async function worker() {
    for (;;) {
      const index = next;
      next += 1;
      if (index >= tasks.length) return;
      results[index] = await tasks[index]();
    }
  }
  const workers = Array.from({ length: Math.max(1, Math.min(concurrency, tasks.length)) }, () => worker());
  await Promise.all(workers);
  return results;
}

export async function revalidateRows(
  rows: FlipResult[],
  options: RevalidateRowsOptions = {},
): Promise<ExecutionRevalidationReport> {
  const unique = new Map<string, FlipResult>();
  for (const row of rows) {
    unique.set(executionRowKey(row), row);
  }
  const tasks = Array.from(unique.values()).map((row) => () => revalidateOne(row, options));
  const resultRows = await runLimited(tasks, options.concurrency ?? 4);
  return {
    createdAt: new Date().toISOString(),
    rows: resultRows,
    safe: resultRows.filter((row) => row.status === "SAFE").length,
    changed: resultRows.filter((row) => row.status === "CHANGED").length,
    danger: resultRows.filter((row) => row.status === "DANGER").length,
    totalOldProfit: resultRows.reduce((sum, row) => sum + row.oldProfit, 0),
    totalNowProfit: resultRows.reduce((sum, row) => sum + row.nowProfit, 0),
    totalDeltaProfit: resultRows.reduce((sum, row) => sum + row.deltaProfit, 0),
  };
}
