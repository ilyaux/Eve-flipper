import { postClientTelemetry, type ClientTelemetryPayload } from "./api";

const SESSION_KEY = "eveflipper_telemetry_session_v1";

function createSessionID() {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return `${Date.now()}_${Math.random().toString(36).slice(2, 12)}`;
}

export function telemetrySessionID(): string {
  try {
    const existing = window.sessionStorage.getItem(SESSION_KEY);
    if (existing) return existing;
    const created = createSessionID();
    window.sessionStorage.setItem(SESSION_KEY, created);
    return created;
  } catch {
    return "session_unavailable";
  }
}

export function trackClientTelemetry(payload: Omit<ClientTelemetryPayload, "session_id">) {
  void postClientTelemetry({
    ...payload,
    session_id: telemetrySessionID(),
  });
}

export function publicScanParams(params: Record<string, unknown>) {
  const out: Record<string, unknown> = {};
  for (const key of [
    "system_name",
    "buy_radius",
    "sell_radius",
    "cargo_capacity",
    "min_margin",
    "min_daily_volume",
    "max_investment",
    "min_item_profit",
    "avg_price_period",
    "min_period_roi",
    "max_dos",
    "source_regions",
    "target_region",
    "target_market_system",
    "target_market_location_id",
    "category_ids",
    "sell_order_mode",
    "regional_diagnostic_mode",
    "route_mode",
    "route_target_system_name",
    "route_min_hops",
    "route_max_hops",
    "route_min_isk_per_jump",
  ]) {
    if (key in params) out[key] = params[key];
  }
  return out;
}
