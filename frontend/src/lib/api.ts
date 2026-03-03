import type {
  AlertHistoryEntry,
  AppConfig,
  AppStatus,
  AuthStatus,
  CharacterInfo,
  CharacterRoles,
  ContractDetails,
  ContractResult,
  CorpDashboard,
  CorpIndustryJob,
  CorpJournalEntry,
  CorpMarketOrderDetail,
  CorpMember,
  CorpMiningEntry,
  DemandRegionResponse,
  DemandRegionsResponse,
  ExecutionPlanResult,
  FlipResult,
  HotZonesResponse,
  IndustryJob,
  IndustryJobStatus,
  IndustryLedger,
  IndustryMaterialPlanRecord,
  IndustryPlanPatch,
  IndustryPlanPreview,
  IndustryPlanSummary,
  IndustryProject,
  IndustryProjectSnapshot,
  IndustryTaskRecord,
  IndustryTaskStatus,
  OptimizerDiagnostic,
  OrderDeskResponse,
  PLEXDashboard,
  PortfolioPnL,
  PortfolioOptimization,
  RegionOpportunities,
  RouteResult,
  ScanParams,
  ScanRecord,
  SolarSystemInfo,
  StationAIChatRequest,
  StationAIChatResponse,
  StationAIStreamMessage,
  StationCacheMeta,
  StationCommandResponse,
  StationInfo,
  StationsResponse,
  StationTrade,
  StationTradeState,
  StationTradeStateMode,
  UndercutStatus,
  WatchlistItem,
} from "./types";

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
  | { type: "result"; data: T[]; count?: number; scan_id?: number; cache_meta?: StationCacheMeta }
  | { type: "error"; message: string };

// Generic NDJSON streaming helper to eliminate code duplication
async function streamNdjson<T>(
  url: string,
  body: object,
  onProgress: (msg: string) => void,
  signal?: AbortSignal,
  errorMessage = "Request failed",
  onResult?: (msg: Extract<NdjsonGenericMessage<T>, { type: "result" }>) => void
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
        onResult?.(msg);
      } else if (msg.type === "error") {
        throw new Error(msg.message);
      }
    }
  }

  // Handle remaining buffer
  if (buffer.trim()) {
    const msg = JSON.parse(buffer) as NdjsonGenericMessage<T>;
    if (msg.type === "result") {
      results = msg.data ?? [];
      onResult?.(msg);
    }
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

export async function getSystemsList(
  query?: string,
  limit?: number,
  signal?: AbortSignal,
): Promise<SolarSystemInfo[]> {
  const qp = new URLSearchParams();
  if (query && query.trim() !== "") qp.set("q", query.trim());
  if (limit != null && limit > 0) qp.set("limit", String(limit));
  const qs = qp.toString();
  const res = await fetch(`${BASE}/api/systems${qs ? `?${qs}` : ""}`, { signal });
  const data = await handleResponse<{ systems?: SolarSystemInfo[] }>(res);
  return data.systems ?? [];
}

export async function scan(
  params: ScanParams,
  onProgress: (msg: string) => void,
  signal?: AbortSignal,
  onMeta?: (meta: StationCacheMeta | undefined) => void
): Promise<FlipResult[]> {
  return streamNdjson<FlipResult>(
    `${BASE}/api/scan`,
    params,
    onProgress,
    signal,
    "Scan failed",
    (msg) => onMeta?.(msg.cache_meta),
  );
}

export async function scanMultiRegion(
  params: ScanParams,
  onProgress: (msg: string) => void,
  signal?: AbortSignal,
  onMeta?: (meta: StationCacheMeta | undefined) => void
): Promise<FlipResult[]> {
  return streamNdjson<FlipResult>(
    `${BASE}/api/scan/multi-region`,
    params,
    onProgress,
    signal,
    "Multi-region scan failed",
    (msg) => onMeta?.(msg.cache_meta),
  );
}

export async function scanRegionalDayTrader(
  params: ScanParams,
  onProgress: (msg: string) => void,
  signal?: AbortSignal,
  onMeta?: (meta: StationCacheMeta | undefined) => void,
  onSummary?: (summary: { count: number; targetRegionName: string; periodDays: number }) => void,
): Promise<FlipResult[]> {
  return streamNdjson<FlipResult>(
    `${BASE}/api/scan/regional-day`,
    params,
    onProgress,
    signal,
    "Regional day trader scan failed",
    (msg) => {
      onMeta?.(msg.cache_meta);
      const raw = msg as {
        count?: number;
        target_region_name?: string;
        period_days?: number;
      };
      onSummary?.({
        count: raw.count ?? 0,
        targetRegionName: raw.target_region_name ?? "",
        periodDays: raw.period_days ?? 14,
      });
    },
  );
}

export async function scanContracts(
  params: ScanParams,
  onProgress: (msg: string) => void,
  signal?: AbortSignal,
  onMeta?: (meta: StationCacheMeta | undefined) => void
): Promise<ContractResult[]> {
  return streamNdjson<ContractResult>(
    `${BASE}/api/scan/contracts`,
    params,
    onProgress,
    signal,
    "Contract scan failed",
    (msg) => onMeta?.(msg.cache_meta),
  );
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
      ignored_system_ids: params.ignored_system_ids ?? [],
      target_system_name: params.route_target_system_name,
      cargo_capacity: params.cargo_capacity,
      min_margin: params.min_margin,
      min_isk_per_jump: params.route_min_isk_per_jump,
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
      allow_empty_hops: params.route_allow_empty_hops,
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

export async function getStations(systemName: string, signal?: AbortSignal): Promise<StationsResponse> {
  const res = await fetch(`${BASE}/api/stations?system=${encodeURIComponent(systemName)}`, { signal });
  return handleResponse<StationsResponse>(res);
}

export async function getStructures(systemId: number, regionId: number, signal?: AbortSignal): Promise<StationInfo[]> {
  const res = await fetch(`${BASE}/api/auth/structures?system_id=${systemId}&region_id=${regionId}`, { signal });
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
  signal?: AbortSignal;
}): Promise<ExecutionPlanResult> {
  const res = await fetch(`${BASE}/api/execution/plan`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    signal: params.signal,
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
    cts_profile?: "balanced" | "aggressive" | "defensive";
    split_trade_fees?: boolean;
    buy_broker_fee_percent?: number;
    sell_broker_fee_percent?: number;
    buy_sales_tax_percent?: number;
    sell_sales_tax_percent?: number;
    min_daily_volume?: number;
    // EVE Guru Profit Filters
    min_item_profit?: number;
    min_demand_per_day?: number;
    min_s2b_per_day?: number;
    min_bfs_per_day?: number;
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
    ignored_system_ids?: number[];
  },
  onProgress: (msg: string) => void,
  signal?: AbortSignal,
  onMeta?: (meta: StationCacheMeta | undefined) => void
): Promise<StationTrade[]> {
  return streamNdjson<StationTrade>(
    `${BASE}/api/scan/station`,
    params,
    onProgress,
    signal,
    "Station scan failed",
    (msg) => onMeta?.(msg.cache_meta),
  );
}

export interface StationTradeStatesResponse {
  tab: string;
  pruned?: number;
  states: StationTradeState[];
}

export async function getStationTradeStates(params?: {
  tab?: string;
  currentRevision?: number;
}): Promise<StationTradeStatesResponse> {
  const qp = new URLSearchParams();
  if (params?.tab) qp.set("tab", params.tab);
  if (params?.currentRevision != null) {
    qp.set("current_revision", String(params.currentRevision));
  }
  const qs = qp.toString();
  const res = await fetch(`${BASE}/api/auth/station/trade-states${qs ? `?${qs}` : ""}`);
  const data = await handleResponse<StationTradeStatesResponse>(res);
  return {
    ...data,
    states: Array.isArray(data.states) ? data.states : [],
  };
}

export async function setStationTradeState(params: {
  tab?: string;
  type_id: number;
  station_id: number;
  region_id?: number;
  mode: StationTradeStateMode;
  until_revision?: number;
}): Promise<{ ok: boolean }> {
  const res = await fetch(`${BASE}/api/auth/station/trade-states/set`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(params),
  });
  return handleResponse<{ ok: boolean }>(res);
}

export async function deleteStationTradeStates(params: {
  tab?: string;
  keys: Array<{
    type_id: number;
    station_id: number;
    region_id?: number;
  }>;
}): Promise<{ ok: boolean; deleted: number }> {
  const res = await fetch(`${BASE}/api/auth/station/trade-states/delete`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(params),
  });
  return handleResponse<{ ok: boolean; deleted: number }>(res);
}

export async function clearStationTradeStates(params?: {
  tab?: string;
  mode?: StationTradeStateMode;
}): Promise<{ ok: boolean; deleted: number }> {
  const res = await fetch(`${BASE}/api/auth/station/trade-states/clear`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(params ?? {}),
  });
  return handleResponse<{ ok: boolean; deleted: number }>(res);
}

export async function rebootStationCache(): Promise<{
  ok: boolean;
  cleared: number;
  rebooted_at?: string;
}> {
  const res = await fetch(`${BASE}/api/auth/station/cache/reboot`, {
    method: "POST",
  });
  return handleResponse<{ ok: boolean; cleared: number; rebooted_at?: string }>(res);
}

// --- Industry Ledger (auth) ---

export interface IndustryProjectsResponse {
  projects: IndustryProject[];
  count: number;
}

export async function getAuthIndustryProjects(params?: {
  status?: string;
  limit?: number;
}): Promise<IndustryProjectsResponse> {
  const qp = new URLSearchParams();
  if (params?.status) qp.set("status", params.status);
  if (params?.limit != null && params.limit > 0) qp.set("limit", String(params.limit));
  const qs = qp.toString();
  const res = await fetch(`${BASE}/api/auth/industry/projects${qs ? `?${qs}` : ""}`);
  const data = await handleResponse<IndustryProjectsResponse>(res);
  return {
    projects: Array.isArray(data.projects) ? data.projects : [],
    count: Number.isFinite(data.count) ? data.count : 0,
  };
}

export interface IndustryProjectCreatePayload {
  name: string;
  status?: string;
  strategy?: "conservative" | "balanced" | "aggressive";
  notes?: string;
  params?: unknown;
}

export interface IndustryProjectCreateResponse {
  ok: boolean;
  project: IndustryProject;
}

export async function createAuthIndustryProject(
  payload: IndustryProjectCreatePayload
): Promise<IndustryProjectCreateResponse> {
  const res = await fetch(`${BASE}/api/auth/industry/projects`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  return handleResponse<IndustryProjectCreateResponse>(res);
}

export async function getAuthIndustryProjectSnapshot(
  projectID: number
): Promise<IndustryProjectSnapshot> {
  const res = await fetch(`${BASE}/api/auth/industry/projects/${projectID}/snapshot`);
  const data = await handleResponse<IndustryProjectSnapshot>(res);
  return {
    ...data,
    tasks: Array.isArray(data.tasks) ? data.tasks : [],
    jobs: Array.isArray(data.jobs) ? data.jobs : [],
    materials: Array.isArray(data.materials) ? data.materials : [],
    blueprints: Array.isArray(data.blueprints) ? data.blueprints : [],
    material_diff: Array.isArray(data.material_diff) ? data.material_diff : [],
  };
}

export interface IndustryProjectPlanResponse {
  ok: boolean;
  summary: IndustryPlanSummary;
}

export async function planAuthIndustryProject(
  projectID: number,
  patch: IndustryPlanPatch
): Promise<IndustryProjectPlanResponse> {
  const res = await fetch(`${BASE}/api/auth/industry/projects/${projectID}/plan`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(patch),
  });
  return handleResponse<IndustryProjectPlanResponse>(res);
}

export async function previewAuthIndustryProjectPlan(
  projectID: number,
  patch: IndustryPlanPatch
): Promise<IndustryPlanPreview> {
  const res = await fetch(`${BASE}/api/auth/industry/projects/${projectID}/plan/preview`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(patch),
  });
  const data = await handleResponse<IndustryPlanPreview>(res);
  return {
    ...data,
    tasks: Array.isArray(data.tasks) ? data.tasks : [],
    jobs: Array.isArray(data.jobs) ? data.jobs : [],
    warnings: Array.isArray(data.warnings) ? data.warnings : [],
  };
}

export interface IndustryProjectMaterialRebalancePayload {
  scope?: "single" | "all";
  character_id?: number;
  lookback_days?: number;
  strategy?: "preserve" | "buy" | "build";
  warehouse_scope?: "global" | "location_first" | "strict_location";
  location_ids?: number[];
}

export interface IndustryProjectMaterialRebalanceResponse {
  ok: boolean;
  materials: IndustryMaterialPlanRecord[];
  summary: {
    project_id: number;
    updated: number;
    scope: "single" | "all";
    lookback_days: number;
    strategy: "preserve" | "buy" | "build";
    warehouse_scope: "global" | "location_first" | "strict_location";
    transactions: number;
    positions_total: number;
    positions_used: number;
    stock_types: number;
    stock_units: number;
    allocated_available: number;
    remaining_missing_qty: number;
    location_filter_count: number;
  };
}

export async function rebalanceAuthIndustryProjectMaterials(
  projectID: number,
  payload: IndustryProjectMaterialRebalancePayload
): Promise<IndustryProjectMaterialRebalanceResponse> {
  const res = await fetch(`${BASE}/api/auth/industry/projects/${projectID}/materials/rebalance`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload ?? {}),
  });
  const data = await handleResponse<IndustryProjectMaterialRebalanceResponse>(res);
  return {
    ...data,
    materials: Array.isArray(data.materials) ? data.materials : [],
  };
}

export interface IndustryProjectBlueprintSyncPayload {
  scope?: "single" | "all";
  character_id?: number;
  location_ids?: number[];
  default_bpc_runs?: number;
}

export interface IndustryProjectBlueprintSyncResponse {
  ok: boolean;
  summary: {
    project_id: number;
    scope: "single" | "all";
    characters: number;
    characters_used: number;
    blueprints_endpoint_characters?: number;
    assets_fallback_characters?: number;
    blueprint_rows_scanned?: number;
    assets_scanned: number;
    blueprints_detected: number;
    blueprints_upserted: number;
    default_bpc_runs: number;
    location_filter_count: number;
    warnings: string[];
  };
}

export async function syncAuthIndustryProjectBlueprintPool(
  projectID: number,
  payload: IndustryProjectBlueprintSyncPayload
): Promise<IndustryProjectBlueprintSyncResponse> {
  const res = await fetch(`${BASE}/api/auth/industry/projects/${projectID}/blueprints/sync`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload ?? {}),
  });
  return handleResponse<IndustryProjectBlueprintSyncResponse>(res);
}

export interface IndustryJobStatusUpdatePayload {
  job_id: number;
  status: IndustryJobStatus;
  started_at?: string;
  finished_at?: string;
  notes?: string;
}

export interface IndustryJobStatusUpdateResponse {
  ok: boolean;
  job: IndustryJob;
}

export async function updateAuthIndustryJobStatus(
  payload: IndustryJobStatusUpdatePayload
): Promise<IndustryJobStatusUpdateResponse> {
  const res = await fetch(`${BASE}/api/auth/industry/jobs/status`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  return handleResponse<IndustryJobStatusUpdateResponse>(res);
}

export interface IndustryTaskStatusUpdatePayload {
  task_id: number;
  status: IndustryTaskStatus;
  priority?: number;
}

export interface IndustryTaskStatusUpdateResponse {
  ok: boolean;
  task: IndustryTaskRecord;
}

export async function updateAuthIndustryTaskStatus(
  payload: IndustryTaskStatusUpdatePayload
): Promise<IndustryTaskStatusUpdateResponse> {
  const res = await fetch(`${BASE}/api/auth/industry/tasks/status`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  return handleResponse<IndustryTaskStatusUpdateResponse>(res);
}

export interface IndustryTaskBulkStatusUpdatePayload {
  task_ids: number[];
  status: IndustryTaskStatus;
  priority?: number;
}

export interface IndustryTaskBulkStatusUpdateResponse {
  ok: boolean;
  updated: number;
  tasks: IndustryTaskRecord[];
}

export async function updateAuthIndustryTaskStatusBulk(
  payload: IndustryTaskBulkStatusUpdatePayload
): Promise<IndustryTaskBulkStatusUpdateResponse> {
  const res = await fetch(`${BASE}/api/auth/industry/tasks/status/bulk`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  const data = await handleResponse<IndustryTaskBulkStatusUpdateResponse>(res);
  return {
    ...data,
    tasks: Array.isArray(data.tasks) ? data.tasks : [],
    updated: Number.isFinite(data.updated) ? data.updated : 0,
  };
}

export interface IndustryTaskPriorityUpdatePayload {
  task_id: number;
  priority: number;
}

export interface IndustryTaskPriorityUpdateResponse {
  ok: boolean;
  task: IndustryTaskRecord;
}

export async function updateAuthIndustryTaskPriority(
  payload: IndustryTaskPriorityUpdatePayload
): Promise<IndustryTaskPriorityUpdateResponse> {
  const res = await fetch(`${BASE}/api/auth/industry/tasks/priority`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  return handleResponse<IndustryTaskPriorityUpdateResponse>(res);
}

export interface IndustryTaskBulkPriorityUpdatePayload {
  task_ids: number[];
  priority: number;
}

export interface IndustryTaskBulkPriorityUpdateResponse {
  ok: boolean;
  updated: number;
  tasks: IndustryTaskRecord[];
}

export async function updateAuthIndustryTaskPriorityBulk(
  payload: IndustryTaskBulkPriorityUpdatePayload
): Promise<IndustryTaskBulkPriorityUpdateResponse> {
  const res = await fetch(`${BASE}/api/auth/industry/tasks/priority/bulk`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  const data = await handleResponse<IndustryTaskBulkPriorityUpdateResponse>(res);
  return {
    ...data,
    tasks: Array.isArray(data.tasks) ? data.tasks : [],
    updated: Number.isFinite(data.updated) ? data.updated : 0,
  };
}

export interface IndustryJobBulkStatusUpdatePayload {
  job_ids: number[];
  status: IndustryJobStatus;
  started_at?: string;
  finished_at?: string;
  notes?: string;
}

export interface IndustryJobBulkStatusUpdateResponse {
  ok: boolean;
  updated: number;
  jobs: IndustryJob[];
}

export async function updateAuthIndustryJobStatusBulk(
  payload: IndustryJobBulkStatusUpdatePayload
): Promise<IndustryJobBulkStatusUpdateResponse> {
  const res = await fetch(`${BASE}/api/auth/industry/jobs/status/bulk`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  const data = await handleResponse<IndustryJobBulkStatusUpdateResponse>(res);
  return {
    ...data,
    jobs: Array.isArray(data.jobs) ? data.jobs : [],
    updated: Number.isFinite(data.updated) ? data.updated : 0,
  };
}

export async function getAuthIndustryLedger(params?: {
  project_id?: number;
  status?: IndustryJobStatus;
  limit?: number;
}): Promise<IndustryLedger> {
  const qp = new URLSearchParams();
  if (params?.project_id != null && params.project_id > 0) qp.set("project_id", String(params.project_id));
  if (params?.status) qp.set("status", params.status);
  if (params?.limit != null && params.limit > 0) qp.set("limit", String(params.limit));
  const qs = qp.toString();
  const res = await fetch(`${BASE}/api/auth/industry/ledger${qs ? `?${qs}` : ""}`);
  const data = await handleResponse<IndustryLedger>(res);
  return {
    ...data,
    entries: Array.isArray(data.entries) ? data.entries : [],
  };
}

export async function stationAIChat(
  payload: StationAIChatRequest,
): Promise<StationAIChatResponse> {
  const res = await fetch(`${BASE}/api/auth/station/ai/chat`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  return handleResponse<StationAIChatResponse>(res);
}

export async function stationAIChatStream(
  payload: StationAIChatRequest,
  handlers: {
    onProgress?: (msg: Extract<StationAIStreamMessage, { type: "progress" }>) => void;
    onDelta?: (msg: Extract<StationAIStreamMessage, { type: "delta" }>) => void;
    onUsage?: (msg: Extract<StationAIStreamMessage, { type: "usage" }>) => void;
    onResult?: (msg: Extract<StationAIStreamMessage, { type: "result" }>) => void;
  },
  signal?: AbortSignal,
): Promise<StationAIChatResponse> {
  const res = await fetch(`${BASE}/api/auth/station/ai/chat/stream`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
    signal,
  });

  if (!res.ok) {
    let errMsg = "Station AI stream failed";
    try {
      const err = await res.json();
      errMsg = err.error || err.message || errMsg;
    } catch {
      // ignore non-json error body
    }
    throw new Error(errMsg);
  }
  if (!res.body) {
    throw new Error("Response body is null");
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let finalResult: StationAIChatResponse | null = null;

  const handleLine = (line: string) => {
    if (!line.trim()) return;
    const msg = JSON.parse(line) as StationAIStreamMessage;
    if (msg.type === "progress") {
      handlers.onProgress?.(msg);
      return;
    }
    if (msg.type === "delta") {
      handlers.onDelta?.(msg);
      return;
    }
    if (msg.type === "usage") {
      handlers.onUsage?.(msg);
      return;
    }
    if (msg.type === "result") {
      handlers.onResult?.(msg);
      finalResult = {
        answer: msg.answer,
        provider: msg.provider,
        model: msg.model,
        assistant: msg.assistant,
        intent: msg.intent,
        pipeline: msg.pipeline,
        warnings: msg.warnings,
        provider_id: msg.provider_id,
        provider_usage: msg.provider_usage,
        usage: msg.usage,
      };
      return;
    }
    if (msg.type === "error") {
      throw new Error(msg.message || "Station AI stream failed");
    }
  };

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });

    const lines = buffer.split("\n");
    buffer = lines.pop() ?? "";
    for (const line of lines) {
      handleLine(line);
    }
  }

  if (buffer.trim()) {
    handleLine(buffer);
  }
  if (!finalResult) {
    throw new Error("AI stream finished without final result");
  }
  return finalResult;
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

export type CharacterScope = number | "all";

function appendCharacterScope(params: URLSearchParams, characterId?: CharacterScope): void {
  if (characterId == null) return;
  if (characterId === "all") {
    params.set("scope", "all");
    return;
  }
  params.set("character_id", String(characterId));
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

export async function selectAuthCharacter(characterId: number): Promise<AuthStatus> {
  const res = await fetch(`${BASE}/api/auth/character/select`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ character_id: characterId }),
  });
  return handleResponse<AuthStatus>(res);
}

export async function deleteAuthCharacter(characterId: number): Promise<AuthStatus> {
  const res = await fetch(`${BASE}/api/auth/characters/${characterId}`, { method: "DELETE" });
  return handleResponse<AuthStatus>(res);
}

export async function getCharacterInfo(characterId?: CharacterScope): Promise<CharacterInfo> {
  const params = new URLSearchParams();
  appendCharacterScope(params, characterId);
  const query = params.toString();
  const res = await fetch(`${BASE}/api/auth/character${query ? `?${query}` : ""}`);
  return handleResponse<CharacterInfo>(res);
}

export interface CharacterLocation {
  solar_system_id: number;
  solar_system_name: string;
  station_id?: number;
  station_name?: string;
}

export async function getCharacterLocation(characterId?: number): Promise<CharacterLocation> {
  const params = new URLSearchParams();
  appendCharacterScope(params, characterId);
  const query = params.toString();
  const res = await fetch(`${BASE}/api/auth/location${query ? `?${query}` : ""}`);
  return handleResponse<CharacterLocation>(res);
}

export async function getUndercuts(characterId?: CharacterScope): Promise<UndercutStatus[]> {
  const params = new URLSearchParams();
  appendCharacterScope(params, characterId);
  const query = params.toString();
  const res = await fetch(`${BASE}/api/auth/undercuts${query ? `?${query}` : ""}`);
  return handleResponse<UndercutStatus[]>(res);
}

export interface OrderDeskParams {
  salesTax?: number;
  brokerFee?: number;
  targetEtaDays?: number;
  characterId?: CharacterScope;
}

export async function getOrderDesk(params?: OrderDeskParams): Promise<OrderDeskResponse> {
  const qp = new URLSearchParams();
  if (params?.salesTax != null) qp.set("sales_tax", String(params.salesTax));
  if (params?.brokerFee != null) qp.set("broker_fee", String(params.brokerFee));
  if (params?.targetEtaDays != null) qp.set("target_eta_days", String(params.targetEtaDays));
  appendCharacterScope(qp, params?.characterId);
  const qs = qp.toString();
  const res = await fetch(`${BASE}/api/auth/orders/desk${qs ? `?${qs}` : ""}`);
  return handleResponse<OrderDeskResponse>(res);
}

export interface StationCommandParams {
  station_id?: number;
  region_id?: number;
  system_name?: string;
  radius?: number;
  min_margin?: number;
  sales_tax_percent?: number;
  broker_fee?: number;
  cts_profile?: string;
  split_trade_fees?: boolean;
  buy_broker_fee_percent?: number;
  sell_broker_fee_percent?: number;
  buy_sales_tax_percent?: number;
  sell_sales_tax_percent?: number;
  min_daily_volume?: number;
  min_item_profit?: number;
  min_demand_per_day?: number;
  min_s2b_per_day?: number;
  min_bfs_per_day?: number;
  avg_price_period?: number;
  min_period_roi?: number;
  bvs_ratio_min?: number;
  bvs_ratio_max?: number;
  max_pvi?: number;
  max_sds?: number;
  limit_buy_to_price_low?: boolean;
  flag_extreme_prices?: boolean;
  include_structures?: boolean;
  structure_ids?: number[];
  target_eta_days?: number;
  lookback_days?: number;
  max_results?: number;
  characterId?: CharacterScope;
}

export async function getStationCommand(params: StationCommandParams): Promise<StationCommandResponse> {
  const { characterId, ...body } = params;
  const qp = new URLSearchParams();
  appendCharacterScope(qp, characterId);
  const qs = qp.toString();
  const res = await fetch(`${BASE}/api/auth/station/command${qs ? `?${qs}` : ""}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  return handleResponse<StationCommandResponse>(res);
}

export interface PortfolioPnLParams {
  salesTax?: number;
  brokerFee?: number;
  ledgerLimit?: number;
  characterId?: CharacterScope;
}

export async function getPortfolioPnL(days: number = 30, params?: PortfolioPnLParams): Promise<PortfolioPnL> {
  const qp = new URLSearchParams();
  qp.set("days", String(days));
  if (params?.salesTax != null) qp.set("sales_tax", String(params.salesTax));
  if (params?.brokerFee != null) qp.set("broker_fee", String(params.brokerFee));
  if (params?.ledgerLimit != null) qp.set("ledger_limit", String(params.ledgerLimit));
  appendCharacterScope(qp, params?.characterId);
  const res = await fetch(`${BASE}/api/auth/portfolio?${qp.toString()}`);
  return handleResponse<PortfolioPnL>(res);
}

export type OptimizerResult =
  | { ok: true; data: PortfolioOptimization }
  | { ok: false; diagnostic: OptimizerDiagnostic | null };

export async function getPortfolioOptimization(days: number = 90, characterId?: CharacterScope): Promise<OptimizerResult> {
  const qp = new URLSearchParams();
  qp.set("days", String(days));
  appendCharacterScope(qp, characterId);
  const res = await fetch(`${BASE}/api/auth/portfolio/optimize?${qp.toString()}`);
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

export async function searchBuildableItems(query: string, limit = 20, signal?: AbortSignal): Promise<BuildableItem[]> {
  const res = await fetch(`${BASE}/api/industry/search?q=${encodeURIComponent(query)}&limit=${limit}`, { signal });
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
  nesOmega?: number;
  omegaUSD?: number;
}

export async function getPLEXDashboard(p?: PLEXDashboardParams, signal?: AbortSignal): Promise<PLEXDashboard> {
  const params = new URLSearchParams();
  if (p?.salesTax != null) params.set("sales_tax", p.salesTax.toString());
  if (p?.brokerFee != null) params.set("broker_fee", p.brokerFee.toString());
  if (p?.nesExtractor != null && p.nesExtractor > 0) params.set("nes_extractor", p.nesExtractor.toString());
  if (p?.nesOmega != null && p.nesOmega > 0) params.set("nes_omega", p.nesOmega.toString());
  if (p?.omegaUSD != null && p.omegaUSD > 0) params.set("omega_usd", p.omegaUSD.toString());
  const qs = params.toString();
  const res = await fetch(`${BASE}/api/plex/dashboard${qs ? "?" + qs : ""}`, { signal });
  return handleResponse<PLEXDashboard>(res);
}

// --- Corporation ---

export async function getCharacterRoles(signal?: AbortSignal, characterId?: number): Promise<CharacterRoles> {
  const qp = new URLSearchParams();
  appendCharacterScope(qp, characterId);
  const qs = qp.toString();
  const res = await fetch(`${BASE}/api/auth/roles${qs ? `?${qs}` : ""}`, { signal });
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
