import { beforeEach, describe, expect, it, vi } from "vitest";
import { getExecutionQuote } from "./api";
import { revalidateRows } from "./executionRevalidation";
import type { ExecutionQuote, FlipResult } from "./types";

vi.mock("./api", () => ({
  getExecutionQuote: vi.fn(),
}));

const mockedGetExecutionQuote = vi.mocked(getExecutionQuote);

function flipRow(overrides: Partial<FlipResult> = {}): FlipResult {
  const typeID = overrides.TypeID ?? 1001;
  return {
    TypeID: typeID,
    TypeName: `Item ${typeID}`,
    Volume: 1,
    BuyPrice: 100,
    BuyStation: "Buy Hub",
    BuySystemName: "Buy",
    BuySystemID: 300001,
    BuyRegionID: 10000002,
    BuyLocationID: 600001,
    SellPrice: 140,
    SellStation: "Sell Hub",
    SellSystemName: "Sell",
    SellSystemID: 300002,
    SellRegionID: 10000002,
    SellLocationID: 600002,
    ProfitPerUnit: 40,
    MarginPercent: 40,
    UnitsToBuy: 10,
    BuyOrderRemain: 10,
    SellOrderRemain: 10,
    TotalProfit: 400,
    ProfitPerJump: 40,
    BuyJumps: 1,
    SellJumps: 1,
    TotalJumps: 2,
    DailyVolume: 100,
    Velocity: 1,
    PriceTrend: 0,
    BuyCompetitors: 1,
    SellCompetitors: 1,
    DailyProfit: 400,
    ExpectedBuyPrice: 100,
    ExpectedSellPrice: 140,
    RealProfit: 400,
    FilledQty: 10,
    ...overrides,
  } as FlipResult;
}

function quote(overrides: Partial<ExecutionQuote> = {}): ExecutionQuote {
  return {
    type_id: 1001,
    requested_qty: 10,
    fill_qty: 10,
    buy_vwap: 100,
    sell_vwap: 140,
    buy_gross: 1000,
    sell_gross: 1400,
    buy_fees: 0,
    sell_fees: 0,
    total_fees: 0,
    shipping_cost: 0,
    shipping_jumps: 0,
    shipping_cost_per_m3_jump: 0,
    packaged_volume_m3: 1,
    filled_volume_m3: 10,
    net_profit: 400,
    profit_per_unit: 40,
    roi_percent: 40,
    decision: "SAFE",
    warnings: [],
    cache: { stale: false },
    buy: {
      vwap: 100,
      best_price: 100,
      gross_isk: 1000,
      fee_isk: 0,
      filled_qty: 10,
      can_fill: true,
      total_depth: 10,
      slippage_percent: 0,
      plan: {} as never,
    },
    sell: {
      vwap: 140,
      best_price: 140,
      gross_isk: 1400,
      fee_isk: 0,
      filled_qty: 10,
      can_fill: true,
      total_depth: 10,
      slippage_percent: 0,
      plan: {} as never,
    },
    ...overrides,
  };
}

describe("execution revalidation", () => {
  beforeEach(() => {
    mockedGetExecutionQuote.mockReset();
  });

  it("keeps backend CHANGED decisions visible even when prices still match", async () => {
    mockedGetExecutionQuote.mockResolvedValueOnce(quote({ decision: "CHANGED" }));

    const report = await revalidateRows([flipRow()]);

    expect(report.changed).toBe(1);
    expect(report.rows[0]).toMatchObject({
      status: "CHANGED",
      avoid: false,
      oldQty: 10,
      nowQty: 10,
      oldBuy: 100,
      nowBuy: 100,
      oldSell: 140,
      nowSell: 140,
      oldProfit: 400,
      nowProfit: 400,
      deltaProfit: 0,
      qtyChanged: false,
    });
    expect(report.rows[0].reasons).toContain("quote marked changed");
  });

  it("does not mark an unchanged quote as changed for informational ESI cache warnings", async () => {
    mockedGetExecutionQuote.mockResolvedValueOnce(
      quote({ warnings: ["esi_market_orders_may_be_cached", "buy_order_cache_meta_unavailable"] }),
    );

    const report = await revalidateRows([flipRow()]);

    expect(report.safe).toBe(1);
    expect(report.changed).toBe(0);
    expect(report.rows[0]).toMatchObject({ status: "SAFE", avoid: false });
  });

  it("summarizes SAFE, CHANGED, and DANGER rows with avoid reasons and profit deltas", async () => {
    mockedGetExecutionQuote
      .mockResolvedValueOnce(quote({ decision: "SAFE" }))
      .mockResolvedValueOnce(
        quote({
          decision: "SAFE",
          fill_qty: 8,
          buy_vwap: 105,
          sell_vwap: 142,
          net_profit: 296,
          profit_per_unit: 37,
        }),
      )
      .mockResolvedValueOnce(
        quote({
          decision: "SAFE",
          fill_qty: 0,
          net_profit: 0,
          profit_per_unit: 0,
          warnings: ["no_depth"],
        }),
      );

    const report = await revalidateRows([
      flipRow({ TypeID: 1001 }),
      flipRow({ TypeID: 1002 }),
      flipRow({ TypeID: 1003 }),
    ]);

    expect(report.safe).toBe(1);
    expect(report.changed).toBe(1);
    expect(report.danger).toBe(1);
    expect(report.totalOldProfit).toBe(1200);
    expect(report.totalNowProfit).toBe(696);
    expect(report.totalDeltaProfit).toBe(-504);

    expect(report.rows[1]).toMatchObject({
      status: "CHANGED",
      oldQty: 10,
      nowQty: 8,
      qtyChanged: true,
      deltaProfit: -104,
      avoid: false,
    });
    expect(report.rows[1].reasons).toEqual(
      expect.arrayContaining(["less depth than scan", "quantity changed", "buy VWAP moved"]),
    );

    expect(report.rows[2]).toMatchObject({
      status: "DANGER",
      nowQty: 0,
      nowProfit: 0,
      deltaProfit: -400,
      avoid: true,
    });
    expect(report.rows[2].reasons).toEqual(
      expect.arrayContaining(["no depth", "no executable quantity", "profit no longer positive"]),
    );
  });
});
