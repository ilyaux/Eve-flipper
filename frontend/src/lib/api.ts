import type { AppConfig, AppStatus, AuthStatus, CharacterInfo, ContractResult, DemandRegionResponse, DemandRegionsResponse, ExecutionPlanResult, FlipResult, HotZonesResponse, OptimizerDiagnostic, PortfolioPnL, PortfolioOptimization, RegionOpportunities, RouteResult, ScanParams, ScanRecord, StationInfo, StationTrade, UndercutStatus, WatchlistItem } from "./types";

const BASE = import.meta.env.VITE_API_URL || "http://127.0.0.1:13370";

// Helper to handle HTTP errors consistently
async function handleResponse<T>(res: Response): Promise<T> {
  if (!res.ok) {
    let errorMessage = `HTTP ${res.status}`;
    try {
      const err = await res.json();
      errorMessage = err.error || err.message || errorMessage;
    } catch {
      // Response body is not JSON
    }
    throw new Error(errorMessage);
  }
  return res.json();
}

// Generic NDJSON message type
type NdjsonGenericMessage<T> =
  | { type: "progress"; message: string }
  | { type: "result"; data: T[]; count?: number }
  | { type: "error"; message: string };

// Generic NDJSON streaming helper to eliminate code duplication
async function streamNdjson<T>(
  url: string,
  body: object,
  onProgress: (msg: string) => void,
  signal?: AbortSignal,
  errorMessage = "Request failed"
): Promise<T[]> {
  const res = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
    signal,
  });

  if (!res.ok) {
    let errMsg = errorMessage;
    try {
      const err = await res.json();
      errMsg = err.error || err.message || errMsg;
    } catch {
      // Response body is not JSON
    }
    throw new Error(errMsg);
  }

  if (!res.body) {
    throw new Error("Response body is null");
  }
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let results: T[] = [];

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });

    const lines = buffer.split("\n");
    buffer = lines.pop() ?? "";

    for (const line of lines) {
      if (!line.trim()) continue;
      const msg = JSON.parse(line) as NdjsonGenericMessage<T>;
      if (msg.type === "progress") {
        onProgress(msg.message);
      } else if (msg.type === "result") {
        results = msg.data ?? [];
      } else if (msg.type === "error") {
        throw new Error(msg.message);
      }
    }
  }

  // Handle remaining buffer
  if (buffer.trim()) {
    const msg = JSON.parse(buffer) as NdjsonGenericMessage<T>;
    if (msg.type === "result") results = msg.data ?? [];
    else if (msg.type === "error") throw new Error(msg.message);
  }

  return results;
}

export async function getStatus(): Promise<AppStatus> {
  const res = await fetch(`${BASE}/api/status`);
  return handleResponse<AppStatus>(res);
}

export async function getConfig(): Promise<AppConfig> {
  const res = await fetch(`${BASE}/api/config`);
  return handleResponse<AppConfig>(res);
}

export async function updateConfig(patch: Partial<AppConfig>): Promise<AppConfig> {
  const res = await fetch(`${BASE}/api/config`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(patch),
  });
  return handleResponse<AppConfig>(res);
}

export async function autocomplete(query: string): Promise<string[]> {
  const res = await fetch(`${BASE}/api/systems/autocomplete?q=${encodeURIComponent(query)}`);
  const data = await handleResponse<{ systems?: string[] }>(res);
  return data.systems ?? [];
}

export async function autocompleteRegion(query: string): Promise<string[]> {
  const res = await fetch(`${BASE}/api/regions/autocomplete?q=${encodeURIComponent(query)}`);
  const data = await handleResponse<{ regions?: string[] }>(res);
  return data.regions ?? [];
}

export async function scan(
  params: ScanParams,
  onProgress: (msg: string) => void,
  signal?: AbortSignal
): Promise<FlipResult[]> {
  return streamNdjson<FlipResult>(`${BASE}/api/scan`, params, onProgress, signal, "Scan failed");
}

export async function scanMultiRegion(
  params: ScanParams,
  onProgress: (msg: string) => void,
  signal?: AbortSignal
): Promise<FlipResult[]> {
  return streamNdjson<FlipResult>(`${BASE}/api/scan/multi-region`, params, onProgress, signal, "Multi-region scan failed");
}

export async function scanContracts(
  params: ScanParams,
  onProgress: (msg: string) => void,
  signal?: AbortSignal
): Promise<ContractResult[]> {
  return streamNdjson<ContractResult>(`${BASE}/api/scan/contracts`, params, onProgress, signal, "Contract scan failed");
}

export async function findRoutes(
  params: ScanParams,
  minHops: number,
  maxHops: number,
  onProgress: (msg: string) => void,
  signal?: AbortSignal
): Promise<RouteResult[]> {
  return streamNdjson<RouteResult>(
    `${BASE}/api/route/find`,
    {
      system_name: params.system_name,
      cargo_capacity: params.cargo_capacity,
      min_margin: params.min_margin,
      sales_tax_percent: params.sales_tax_percent,
      broker_fee_percent: params.broker_fee_percent,
      min_hops: minHops,
      max_hops: maxHops,
      max_results: params.max_results,
      min_route_security: params.min_route_security,
    },
    onProgress,
    signal,
    "Route search failed"
  );
}

// --- Watchlist ---

export async function getWatchlist(): Promise<WatchlistItem[]> {
  const res = await fetch(`${BASE}/api/watchlist`);
  return handleResponse<WatchlistItem[]>(res);
}

export interface AddWatchlistResult {
  items: WatchlistItem[];
  inserted: boolean;
}

export async function addToWatchlist(typeId: number, typeName: string, alertMinMargin: number = 0): Promise<AddWatchlistResult> {
  const res = await fetch(`${BASE}/api/watchlist`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ type_id: typeId, type_name: typeName, alert_min_margin: alertMinMargin }),
  });
  return handleResponse<AddWatchlistResult>(res);
}

export async function removeFromWatchlist(typeId: number): Promise<WatchlistItem[]> {
  const res = await fetch(`${BASE}/api/watchlist/${typeId}`, { method: "DELETE" });
  return handleResponse<WatchlistItem[]>(res);
}

export async function updateWatchlistItem(typeId: number, alertMinMargin: number): Promise<WatchlistItem[]> {
  const res = await fetch(`${BASE}/api/watchlist/${typeId}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ alert_min_margin: alertMinMargin }),
  });
  return handleResponse<WatchlistItem[]>(res);
}

// --- Station Trading ---

export async function getStations(systemName: string): Promise<StationInfo[]> {
  const res = await fetch(`${BASE}/api/stations?system=${encodeURIComponent(systemName)}`);
  return handleResponse<StationInfo[]>(res);
}

export async function getExecutionPlan(params: {
  type_id: number;
  region_id: number;
  location_id?: number;
  quantity: number;
  is_buy: boolean;
  /** Days of history for impact calibration (λ, η, n*). From station trading "Period (days)" when present. */
  impact_days?: number;
}): Promise<ExecutionPlanResult> {
  const res = await fetch(`${BASE}/api/execution/plan`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      type_id: params.type_id,
      region_id: params.region_id,
      location_id: params.location_id ?? 0,
      quantity: params.quantity,
      is_buy: params.is_buy,
      impact_days: params.impact_days ?? 0,
    }),
  });
  return handleResponse<ExecutionPlanResult>(res);
}

export async function scanStation(
  params: {
    station_id?: number;
    region_id?: number;
    system_name?: string;
    radius?: number;
    min_margin: number;
    sales_tax_percent: number;
    broker_fee: number;
    min_daily_volume?: number;
    max_results?: number;
    // EVE Guru Profit Filters
    min_item_profit?: number;
    min_demand_per_day?: number;
    // Risk Profile
    avg_price_period?: number;
    min_period_roi?: number;
    bvs_ratio_min?: number;
    bvs_ratio_max?: number;
    max_pvi?: number;
    max_sds?: number;
    limit_buy_to_price_low?: boolean;
    flag_extreme_prices?: boolean;
  },
  onProgress: (msg: string) => void,
  signal?: AbortSignal
): Promise<StationTrade[]> {
  return streamNdjson<StationTrade>(`${BASE}/api/scan/station`, params, onProgress, signal, "Station scan failed");
}

// --- Scan History ---

export async function getScanHistory(limit: number = 50): Promise<ScanRecord[]> {
  const res = await fetch(`${BASE}/api/scan/history?limit=${limit}`);
  return handleResponse<ScanRecord[]>(res);
}

export async function getScanHistoryById(id: number): Promise<ScanRecord> {
  const res = await fetch(`${BASE}/api/scan/history/${id}`);
  return handleResponse<ScanRecord>(res);
}

export async function getScanHistoryResults(id: number): Promise<{ scan: ScanRecord; results: unknown[] }> {
  const res = await fetch(`${BASE}/api/scan/history/${id}/results`);
  return handleResponse<{ scan: ScanRecord; results: unknown[] }>(res);
}

export async function deleteScanHistory(id: number): Promise<void> {
  const res = await fetch(`${BASE}/api/scan/history/${id}`, { method: "DELETE" });
  if (!res.ok) {
    throw new Error("Delete failed");
  }
}

export async function clearScanHistory(olderThanDays: number = 7): Promise<{ deleted: number }> {
  const res = await fetch(`${BASE}/api/scan/history/clear`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ older_than_days: olderThanDays }),
  });
  return handleResponse<{ deleted: number }>(res);
}

// --- Auth ---

export function getLoginUrl(): string {
  return `${BASE}/api/auth/login`;
}

export async function getAuthStatus(): Promise<AuthStatus> {
  const res = await fetch(`${BASE}/api/auth/status`);
  return handleResponse<AuthStatus>(res);
}

export async function logout(): Promise<void> {
  const res = await fetch(`${BASE}/api/auth/logout`, { method: "POST" });
  if (!res.ok) {
    throw new Error("Logout failed");
  }
}

export async function getCharacterInfo(): Promise<CharacterInfo> {
  const res = await fetch(`${BASE}/api/auth/character`);
  return handleResponse<CharacterInfo>(res);
}

export interface CharacterLocation {
  solar_system_id: number;
  solar_system_name: string;
  station_id?: number;
  station_name?: string;
}

export async function getCharacterLocation(): Promise<CharacterLocation> {
  const res = await fetch(`${BASE}/api/auth/location`);
  return handleResponse<CharacterLocation>(res);
}

export async function getUndercuts(): Promise<UndercutStatus[]> {
  const res = await fetch(`${BASE}/api/auth/undercuts`);
  return handleResponse<UndercutStatus[]>(res);
}

export async function getPortfolioPnL(days: number = 30): Promise<PortfolioPnL> {
  const res = await fetch(`${BASE}/api/auth/portfolio?days=${days}`);
  return handleResponse<PortfolioPnL>(res);
}

export type OptimizerResult =
  | { ok: true; data: PortfolioOptimization }
  | { ok: false; diagnostic: OptimizerDiagnostic | null };

export async function getPortfolioOptimization(days: number = 90): Promise<OptimizerResult> {
  const res = await fetch(`${BASE}/api/auth/portfolio/optimize?days=${days}`);
  if (res.ok) {
    const data: PortfolioOptimization = await res.json();
    return { ok: true, data };
  }
  // Try to extract diagnostic from the 400 response body.
  try {
    const body = await res.json();
    return { ok: false, diagnostic: body.diagnostic ?? null };
  } catch {
    return { ok: false, diagnostic: null };
  }
}

// --- Industry ---

import type { IndustryParams, IndustryAnalysis, BuildableItem, IndustrySystem, NdjsonIndustryMessage } from "./types";

export async function analyzeIndustry(
  params: IndustryParams,
  onProgress: (msg: string) => void,
  signal?: AbortSignal
): Promise<IndustryAnalysis> {
  const res = await fetch(`${BASE}/api/industry/analyze`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(params),
    signal,
  });

  if (!res.ok) {
    let errMsg = "Analysis failed";
    try {
      const err = await res.json();
      errMsg = err.error || err.message || errMsg;
    } catch {
      // Response body is not JSON
    }
    throw new Error(errMsg);
  }

  if (!res.body) {
    throw new Error("Response body is null");
  }
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let result: IndustryAnalysis | null = null;

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });

    const lines = buffer.split("\n");
    buffer = lines.pop() ?? "";

    for (const line of lines) {
      if (!line.trim()) continue;
      const msg = JSON.parse(line) as NdjsonIndustryMessage;
      if (msg.type === "progress") {
        onProgress(msg.message);
      } else if (msg.type === "result") {
        result = msg.data;
      } else if (msg.type === "error") {
        throw new Error(msg.message);
      }
    }
  }

  // Handle remaining buffer
  if (buffer.trim()) {
    const msg = JSON.parse(buffer) as NdjsonIndustryMessage;
    if (msg.type === "result") result = msg.data;
    else if (msg.type === "error") throw new Error(msg.message);
  }

  if (!result) {
    throw new Error("No result received");
  }

  return result;
}

export async function searchBuildableItems(query: string, limit = 20): Promise<BuildableItem[]> {
  const res = await fetch(`${BASE}/api/industry/search?q=${encodeURIComponent(query)}&limit=${limit}`);
  return handleResponse<BuildableItem[]>(res);
}

export async function getIndustrySystems(): Promise<IndustrySystem[]> {
  const res = await fetch(`${BASE}/api/industry/systems`);
  return handleResponse<IndustrySystem[]>(res);
}

// --- Demand / War Tracker API ---

export async function getDemandRegions(): Promise<DemandRegionsResponse> {
  const res = await fetch(`${BASE}/api/demand/regions`);
  return handleResponse<DemandRegionsResponse>(res);
}

export async function getHotZones(limit = 20): Promise<HotZonesResponse> {
  const res = await fetch(`${BASE}/api/demand/hotzones?limit=${limit}`);
  return handleResponse<HotZonesResponse>(res);
}

export async function getDemandRegion(regionId: number): Promise<DemandRegionResponse> {
  const res = await fetch(`${BASE}/api/demand/region/${regionId}`);
  return handleResponse<DemandRegionResponse>(res);
}

export async function getRegionOpportunities(regionId: number): Promise<RegionOpportunities> {
  const res = await fetch(`${BASE}/api/demand/opportunities/${regionId}`);
  return handleResponse<RegionOpportunities>(res);
}

export async function getRegionFittings(regionId: number): Promise<{ region_id: number; items: unknown[]; count: number; from_cache: boolean }> {
  const res = await fetch(`${BASE}/api/demand/fittings/${regionId}`);
  return handleResponse<{ region_id: number; items: unknown[]; count: number; from_cache: boolean }>(res);
}

export async function refreshDemandData(onProgress?: (msg: string) => void): Promise<void> {
  const res = await fetch(`${BASE}/api/demand/refresh`, { method: "POST" });
  if (!res.ok) {
    let errMsg = "Refresh failed";
    try {
      const err = await res.json();
      errMsg = err.error || err.message || errMsg;
    } catch { /* not JSON */ }
    throw new Error(errMsg);
  }

  if (!res.body) {
    throw new Error("Response body is null");
  }
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });

    const lines = buffer.split("\n");
    buffer = lines.pop() ?? "";

    for (const line of lines) {
      if (!line.trim()) continue;
      const msg = JSON.parse(line) as { type: string; message?: string; status?: string };
      if (msg.type === "progress" && msg.message) {
        onProgress?.(msg.message);
      } else if (msg.type === "error") {
        throw new Error(msg.message || "Refresh failed");
      }
    }
  }

  if (buffer.trim()) {
    const msg = JSON.parse(buffer) as { type: string; message?: string };
    if (msg.type === "error") throw new Error(msg.message || "Refresh failed");
  }
}
