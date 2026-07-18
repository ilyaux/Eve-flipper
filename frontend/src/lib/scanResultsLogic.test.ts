import { describe, expect, it } from "vitest";
import {
  routeSafetyMatchesFilter,
  summarizeRouteSystems,
  tripJumpsBreakdown,
} from "./scanResultsLogic";
import type { RouteState, SystemDanger } from "./types";

describe("scan results route logic", () => {
  it("shows trip jumps as pickup plus buy-to-sell breakdown", () => {
    const breakdown = tripJumpsBreakdown({
      BuyJumps: 9,
      SellJumps: 17,
      TotalJumps: 26,
    });

    expect(breakdown.total).toBe(26);
    expect(breakdown.pickup).toBe(9);
    expect(breakdown.trade).toBe(17);
    expect(breakdown.title).toContain("9 pickup + 17 buy-to-sell");
  });

  it("falls back to buy-to-sell wording when there is no pickup leg", () => {
    const breakdown = tripJumpsBreakdown({
      BuyJumps: 0,
      SellJumps: 17,
      TotalJumps: 0,
    });

    expect(breakdown.total).toBe(17);
    expect(breakdown.title).toBe("Buy-to-sell route: 17 jumps");
  });

  it("keeps missing or partial route safety out of green/yellow/red filters", () => {
    const missing: RouteState = { status: "unknown", reason: "missing" };
    const summary: RouteState = {
      status: "summary",
      danger: "yellow",
      kills: 3,
      totalISK: 1_000_000,
    };

    expect(routeSafetyMatchesFilter(undefined, "unknown")).toBe(true);
    expect(routeSafetyMatchesFilter(missing, "unknown")).toBe(true);
    expect(routeSafetyMatchesFilter(missing, "green")).toBe(false);
    expect(routeSafetyMatchesFilter(summary, "yellow")).toBe(true);
    expect(routeSafetyMatchesFilter(summary, "green")).toBe(false);
  });

  it("summarizes dangerous systems conservatively", () => {
    const systems = [
      { DangerLevel: "green", KillsTotal: 1, TotalISK: 100 },
      { DangerLevel: "yellow", KillsTotal: 2, TotalISK: 200 },
      { DangerLevel: "red", KillsTotal: 3, TotalISK: 300 },
    ] as SystemDanger[];

    expect(summarizeRouteSystems(systems)).toEqual({
      danger: "red",
      kills: 6,
      totalISK: 600,
    });
  });
});
