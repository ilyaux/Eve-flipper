import type { AlertHistoryEntry, AppConfig, AppStatus, AuthStatus, CharacterInfo, CharacterRoles, ContractDetails, ContractResult, CorpDashboard, CorpIndustryJob, CorpJournalEntry, CorpMarketOrderDetail, CorpMember, CorpMiningEntry, DemandRegionResponse, DemandRegionsResponse, ExecutionPlanResult, FlipResult, HotZonesResponse, OptimizerDiagnostic, OrderDeskResponse, PLEXDashboard, PortfolioPnL, PortfolioOptimization, RegionOpportunities, RouteResult, ScanParams, ScanRecord, StationInfo, StationsResponse, StationTrade, UndercutStatus, WatchlistItem } from "./types";

const BASE = import.meta.env.VITE_API_URL || "";

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

export async function testAlertChannels(message?: string): Promise<{ sent: string[]; failed?: Record<string, string> }> {
  const res = await fetch(`${BASE}/api/alerts/test`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ message: message ?? "" }),
  });
  return handleResponse<{ sent: string[]; failed?: Record<string, string> }>(res);
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
      split_trade_fees: params.split_trade_fees,
      buy_broker_fee_percent: params.buy_broker_fee_percent,
      sell_broker_fee_percent: params.sell_broker_fee_percent,
      buy_sales_tax_percent: params.buy_sales_tax_percent,
      sell_sales_tax_percent: params.sell_sales_tax_percent,
      min_hops: minHops,
      max_hops: maxHops,
      min_route_security: params.min_route_security,
      include_structures: params.include_structures,
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
    body: JSON.stringify({
      type_id: typeId,
      type_name: typeName,
      alert_min_margin: alertMinMargin,
      alert_enabled: alertMinMargin > 0,
      alert_metric: "margin_percent",
      alert_threshold: alertMinMargin,
    }),
  });
  return handleResponse<AddWatchlistResult>(res);
}

export async function removeFromWatchlist(typeId: number): Promise<WatchlistItem[]> {
  const res = await fetch(`${BASE}/api/watchlist/${typeId}`, { method: "DELETE" });
  return handleResponse<WatchlistItem[]>(res);
}

export async function updateWatchlistItem(typeId: number, patch: {
  alert_min_margin?: number;
  alert_enabled?: boolean;
  alert_metric?: "margin_percent" | "total_profit" | "profit_per_unit" | "daily_volume";
  alert_threshold?: number;
}): Promise<WatchlistItem[]> {
  const res = await fetch(`${BASE}/api/watchlist/${typeId}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(patch),
  });
  return handleResponse<WatchlistItem[]>(res);
}

export async function getAlertHistory(typeId?: number, limit?: number, offset?: number): Promise<AlertHistoryEntry[]> {
  const params = new URLSearchParams();
  if (typeId) params.set("type_id", String(typeId));
  if (limit) params.set("limit", String(limit));
  if (offset && offset > 0) params.set("offset", String(offset));
  const query = params.toString();
  const res = await fetch(`${BASE}/api/alerts/history${query ? `?${query}` : ""}`);
  return handleResponse<AlertHistoryEntry[]>(res);
}

// --- Station Trading ---

export async function getStations(systemName: string): Promise<StationsResponse> {
  const res = await fetch(`${BASE}/api/stations?system=${encodeURIComponent(systemName)}`);
  return handleResponse<StationsResponse>(res);
}

export async function getStructures(systemId: number, regionId: number): Promise<StationInfo[]> {
  const res = await fetch(`${BASE}/api/auth/structures?system_id=${systemId}&region_id=${regionId}`);
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
    split_trade_fees?: boolean;
    buy_broker_fee_percent?: number;
    sell_broker_fee_percent?: number;
    buy_sales_tax_percent?: number;
    sell_sales_tax_percent?: number;
    min_daily_volume?: number;
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
    // Player structures
    include_structures?: boolean;
    structure_ids?: number[];
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

export interface OrderDeskParams {
  salesTax?: number;
  brokerFee?: number;
  targetEtaDays?: number;
}

export async function getOrderDesk(params?: OrderDeskParams): Promise<OrderDeskResponse> {
  const qp = new URLSearchParams();
  if (params?.salesTax != null) qp.set("sales_tax", String(params.salesTax));
  if (params?.brokerFee != null) qp.set("broker_fee", String(params.brokerFee));
  if (params?.targetEtaDays != null) qp.set("target_eta_days", String(params.targetEtaDays));
  const qs = qp.toString();
  const res = await fetch(`${BASE}/api/auth/orders/desk${qs ? `?${qs}` : ""}`);
  return handleResponse<OrderDeskResponse>(res);
}

export interface PortfolioPnLParams {
  salesTax?: number;
  brokerFee?: number;
  ledgerLimit?: number;
}

export async function getPortfolioPnL(days: number = 30, params?: PortfolioPnLParams): Promise<PortfolioPnL> {
  const qp = new URLSearchParams();
  qp.set("days", String(days));
  if (params?.salesTax != null) qp.set("sales_tax", String(params.salesTax));
  if (params?.brokerFee != null) qp.set("broker_fee", String(params.brokerFee));
  if (params?.ledgerLimit != null) qp.set("ledger_limit", String(params.ledgerLimit));
  const res = await fetch(`${BASE}/api/auth/portfolio?${qp.toString()}`);
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

// --- PLEX+ ---

export interface PLEXDashboardParams {
  salesTax?: number;
  brokerFee?: number;
  nesExtractor?: number;
  nesMPTC?: number;
  nesOmega?: number;
  omegaUSD?: number;
}

export async function getPLEXDashboard(p?: PLEXDashboardParams, signal?: AbortSignal): Promise<PLEXDashboard> {
  const params = new URLSearchParams();
  if (p?.salesTax != null) params.set("sales_tax", p.salesTax.toString());
  if (p?.brokerFee != null) params.set("broker_fee", p.brokerFee.toString());
  if (p?.nesExtractor != null && p.nesExtractor > 0) params.set("nes_extractor", p.nesExtractor.toString());
  if (p?.nesMPTC != null && p.nesMPTC > 0) params.set("nes_mptc", p.nesMPTC.toString());
  if (p?.nesOmega != null && p.nesOmega > 0) params.set("nes_omega", p.nesOmega.toString());
  if (p?.omegaUSD != null && p.omegaUSD > 0) params.set("omega_usd", p.omegaUSD.toString());
  const qs = params.toString();
  const res = await fetch(`${BASE}/api/plex/dashboard${qs ? "?" + qs : ""}`, { signal });
  return handleResponse<PLEXDashboard>(res);
}

// --- Corporation ---

export async function getCharacterRoles(signal?: AbortSignal): Promise<CharacterRoles> {
  const res = await fetch(`${BASE}/api/auth/roles`, { signal });
  return handleResponse<CharacterRoles>(res);
}

export async function getCorpDashboard(mode: "demo" | "live" = "demo", signal?: AbortSignal): Promise<CorpDashboard> {
  const res = await fetch(`${BASE}/api/corp/dashboard?mode=${mode}`, { signal });
  return handleResponse<CorpDashboard>(res);
}

export async function getCorpJournal(mode: "demo" | "live" = "demo", division = 1, days = 90, signal?: AbortSignal): Promise<CorpJournalEntry[]> {
  const res = await fetch(`${BASE}/api/corp/journal?mode=${mode}&division=${division}&days=${days}`, { signal });
  return handleResponse<CorpJournalEntry[]>(res);
}

export async function getCorpMembers(mode: "demo" | "live" = "demo", signal?: AbortSignal): Promise<CorpMember[]> {
  const res = await fetch(`${BASE}/api/corp/members?mode=${mode}`, { signal });
  return handleResponse<CorpMember[]>(res);
}

export async function getCorpOrders(mode: "demo" | "live" = "demo", signal?: AbortSignal): Promise<CorpMarketOrderDetail[]> {
  const res = await fetch(`${BASE}/api/corp/orders?mode=${mode}`, { signal });
  return handleResponse<CorpMarketOrderDetail[]>(res);
}

export async function getCorpIndustryJobs(mode: "demo" | "live" = "demo", signal?: AbortSignal): Promise<CorpIndustryJob[]> {
  const res = await fetch(`${BASE}/api/corp/industry?mode=${mode}`, { signal });
  return handleResponse<CorpIndustryJob[]>(res);
}

export async function getCorpMiningLedger(mode: "demo" | "live" = "demo", signal?: AbortSignal): Promise<CorpMiningEntry[]> {
  const res = await fetch(`${BASE}/api/corp/mining?mode=${mode}`, { signal });
  return handleResponse<CorpMiningEntry[]>(res);
}

// --- UI Operations (in-game actions) ---

export async function openMarketInGame(typeID: number): Promise<void> {
  const res = await fetch(`${BASE}/api/ui/open-market`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ type_id: typeID }),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: "Unknown error" }));
    throw new Error(err.error || "Failed to open market window");
  }
}

export async function setWaypointInGame(solarSystemID: number, clearOther = true, addToBeginning = false): Promise<void> {
  const res = await fetch(`${BASE}/api/ui/set-waypoint`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      solar_system_id: solarSystemID,
      clear_other_waypoints: clearOther,
      add_to_beginning: addToBeginning,
    }),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: "Unknown error" }));
    throw new Error(err.error || "Failed to set waypoint");
  }
}

export async function openContractInGame(contractID: number): Promise<void> {
  const res = await fetch(`${BASE}/api/ui/open-contract`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ contract_id: contractID }),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: "Unknown error" }));
    throw new Error(err.error || "Failed to open contract window");
  }
}

export async function getContractDetails(contractID: number): Promise<ContractDetails> {
  const res = await fetch(`${BASE}/api/contracts/${contractID}/items`);
  if (!res.ok) {
    throw new Error("Failed to fetch contract details");
  }
  return res.json();
}
