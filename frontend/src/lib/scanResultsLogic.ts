import type { FlipResult, RouteState, SystemDanger } from "./types";

export type RouteSafetyFilter = "all" | "green" | "yellow" | "red" | "unknown";

function finiteNumber(value: unknown, fallback = 0): number {
  const n = Number(value);
  return Number.isFinite(n) ? n : fallback;
}

export function tripJumpsBreakdown(row: Pick<FlipResult, "BuyJumps" | "SellJumps" | "TotalJumps">): {
  total: number;
  pickup: number;
  trade: number;
  title: string;
} {
  const pickup = Math.max(0, Math.floor(finiteNumber(row.BuyJumps)));
  const trade = Math.max(0, Math.floor(finiteNumber(row.SellJumps)));
  const explicitTotal = Math.floor(finiteNumber(row.TotalJumps));
  const total = explicitTotal > 0 ? explicitTotal : pickup + trade;
  const title =
    pickup > 0
      ? `Total trip: ${total} jumps (${pickup} pickup + ${trade} buy-to-sell)`
      : `Buy-to-sell route: ${total} jumps`;
  return { total, pickup, trade, title };
}

export function summarizeRouteSystems(systems: SystemDanger[]): {
  danger: "green" | "yellow" | "red";
  kills: number;
  totalISK: number;
} {
  let danger: "green" | "yellow" | "red" = "green";
  let kills = 0;
  let totalISK = 0;
  for (const system of systems) {
    kills += finiteNumber(system.KillsTotal);
    totalISK += finiteNumber(system.TotalISK);
    if (system.DangerLevel === "red") {
      danger = "red";
    } else if (system.DangerLevel === "yellow" && danger === "green") {
      danger = "yellow";
    }
  }
  return { danger, kills, totalISK };
}

export function hasRouteSafetySummary(
  entry: RouteState | undefined,
): entry is Extract<RouteState, { status: "summary" | "full" }> {
  return entry?.status === "summary" || entry?.status === "full";
}

export function routeSafetyMatchesFilter(
  entry: RouteState | undefined,
  filter: RouteSafetyFilter,
): boolean {
  if (filter === "all") return true;
  if (!hasRouteSafetySummary(entry)) return filter === "unknown";
  return entry.danger === filter;
}
