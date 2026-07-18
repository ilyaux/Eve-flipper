import { describe, expect, it } from "vitest";
import { executionRowKey } from "@/lib/executionRevalidation";
import type { ExecutionQuote, ExecutionRevalidationRow, FlipResult } from "@/lib/types";
import { buildBatch } from "./BatchBuilderPopup";

function flipRow(overrides: Partial<FlipResult> = {}): FlipResult {
  return {
    TypeID: 2001,
    TypeName: "Batch Item",
    Volume: 2,
    BuyPrice: 100,
    BuyStation: "Buy Hub",
    BuySystemName: "Buy",
    BuySystemID: 300001,
    BuyRegionID: 10000002,
    BuyLocationID: 600001,
    SellPrice: 125,
    SellStation: "Sell Hub",
    SellSystemName: "Sell",
    SellSystemID: 300002,
    SellRegionID: 10000002,
    SellLocationID: 600002,
    ProfitPerUnit: 10,
    MarginPercent: 10,
    UnitsToBuy: 10,
    BuyOrderRemain: 10,
    SellOrderRemain: 10,
    TotalProfit: 100,
    ProfitPerJump: 10,
    BuyJumps: 1,
    SellJumps: 1,
    TotalJumps: 2,
    DailyVolume: 100,
    Velocity: 1,
    PriceTrend: 0,
    BuyCompetitors: 1,
    SellCompetitors: 1,
    DailyProfit: 100,
    ...overrides,
  } as FlipResult;
}

function quote(overrides: Partial<ExecutionQuote> = {}): ExecutionQuote {
  return {
    decision: "SAFE",
    fill_qty: 3,
    buy_vwap: 120,
    profit_per_unit: 25,
    ...overrides,
  } as ExecutionQuote;
}

function freshRow(
  row: FlipResult,
  overrides: Partial<ExecutionRevalidationRow> = {},
): ExecutionRevalidationRow {
  return {
    key: executionRowKey(row),
    row,
    quote: quote(),
    status: "SAFE",
    oldQty: 10,
    nowQty: 3,
    oldBuy: 100,
    nowBuy: 120,
    oldSell: 125,
    nowSell: 145,
    oldProfit: 100,
    nowProfit: 75,
    deltaProfit: -25,
    qtyChanged: true,
    avoid: false,
    reasons: ["quantity changed"],
    ...overrides,
  };
}

describe("BatchBuilderPopup buildBatch", () => {
  it("uses fresh execution quote quantity, capital, and profit when revalidated", () => {
    const row = flipRow();
    const fresh = freshRow(row);

    const batch = buildBatch(row, [row], 100, new Map([[fresh.key, fresh]]));

    expect(batch.lines).toHaveLength(1);
    expect(batch.lines[0]).toMatchObject({
      units: 3,
      volume: 6,
      profit: 75,
      capital: 360,
      revalidationStatus: "SAFE",
      reasons: ["quantity changed"],
    });
  });

  it("excludes fresh quote rows marked avoid", () => {
    const row = flipRow();
    const fresh = freshRow(row, {
      quote: quote({ decision: "DANGER", fill_qty: 0, profit_per_unit: 0 }),
      status: "DANGER",
      nowQty: 0,
      nowProfit: 0,
      avoid: true,
      reasons: ["profit no longer positive"],
    });

    const batch = buildBatch(row, [row], 100, new Map([[fresh.key, fresh]]));

    expect(batch.lines).toHaveLength(0);
    expect(batch.totalProfit).toBe(0);
  });
});
