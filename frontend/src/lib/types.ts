export interface FlipResult {
  TypeID: number;
  TypeName: string;
  Volume: number;
  BuyPrice: number;
  BuyStation: string;
  BuySystemName: string;
  BuySystemID: number;
  BuyRegionID?: number;
  BuyLocationID?: number;
  SellPrice: number;
  SellStation: string;
  SellSystemName: string;
  SellSystemID: number;
  SellRegionID?: number;
  SellLocationID?: number;
  ProfitPerUnit: number;
  MarginPercent: number;
  UnitsToBuy: number;
  BuyOrderRemain: number;
  SellOrderRemain: number;
  TotalProfit: number;
  ProfitPerJump: number;
  BuyJumps: number;
  SellJumps: number;
  TotalJumps: number;
  DailyVolume: number;
  Velocity: number;
  PriceTrend: number;
  BuyCompetitors: number;
  SellCompetitors: number;
  /** Expected fill prices from execution plan (order book depth) */
  ExpectedBuyPrice?: number;
  ExpectedSellPrice?: number;
  ExpectedProfit?: number;
  SlippageBuyPct?: number;
  SlippageSellPct?: number;
}

export interface ContractResult {
  ContractID: number;
  Title: string;
  Price: number;
  MarketValue: number;
  Profit: number;
  MarginPercent: number;
  Volume: number;
  StationName: string;
  ItemCount: number;
  Jumps: number;
  ProfitPerJump: number;
}

export type NdjsonContractMessage =
  | { type: "progress"; message: string }
  | { type: "result"; data: ContractResult[]; count: number }
  | { type: "error"; message: string };

export interface RouteHop {
  SystemName: string;
  StationName: string;
  DestSystemName: string;
  TypeName: string;
  TypeID: number;
  BuyPrice: number;
  SellPrice: number;
  Units: number;
  Profit: number;
  Jumps: number;
  RegionID?: number;
}

export interface RouteResult {
  Hops: RouteHop[];
  TotalProfit: number;
  TotalJumps: number;
  ProfitPerJump: number;
  HopCount: number;
}

export type NdjsonRouteMessage =
  | { type: "progress"; message: string }
  | { type: "result"; data: RouteResult[]; count: number }
  | { type: "error"; message: string };

export interface WatchlistItem {
  type_id: number;
  type_name: string;
  added_at: string;
  alert_min_margin: number;
}

export interface ScanRecord {
  id: number;
  timestamp: string;
  tab: string;
  system: string;
  count: number;
  top_profit: number;
  total_profit: number;
  duration_ms: number;
  params: Record<string, unknown>;
}

export interface StationTrade {
  TypeID: number;
  TypeName: string;
  Volume: number;
  BuyPrice: number;
  SellPrice: number;
  Spread: number;
  MarginPercent: number;
  ProfitPerUnit: number;
  DailyVolume: number;
  BuyOrderCount: number;
  SellOrderCount: number;
  BuyVolume: number;
  SellVolume: number;
  TotalProfit: number;
  ROI: number;
  StationName: string;
  StationID: number;
  // EVE Guru style metrics
  CapitalRequired: number;
  NowROI: number;
  PeriodROI: number;
  BuyUnitsPerDay: number;
  SellUnitsPerDay: number;
  BvSRatio: number;
  DOS: number;
  VWAP: number;
  PVI: number;
  OBDS: number;
  SDS: number;
  CI: number;
  CTS: number;
  AvgPrice: number;
  PriceHigh: number;
  PriceLow: number;
  IsExtremePriceFlag: boolean;
  IsHighRiskFlag: boolean;
  /** Expected fill prices from execution plan (order book depth) */
  ExpectedBuyPrice?: number;
  ExpectedSellPrice?: number;
  ExpectedProfit?: number;
  SlippageBuyPct?: number;
  SlippageSellPct?: number;
}

export type NdjsonStationMessage =
  | { type: "progress"; message: string }
  | { type: "result"; data: StationTrade[]; count: number }
  | { type: "error"; message: string };

export interface StationInfo {
  id: number;
  name: string;
  system_id: number;
  region_id: number;
}

// Execution plan (slippage / fill curve)
export interface DepthLevel {
  price: number;
  volume: number;
  cumulative: number;
  volume_filled: number;
}

/** Calibrated market impact params (Kyle's λ, η from history). */
export interface ImpactParams {
  lambda: number;
  eta: number;
  sigma_sq: number;
  sigma: number;
  days_used: number;
  valid: boolean;
}

/** Impact estimate for a quantity: ΔP (linear/√V) and TWAP n*. */
export interface ImpactEstimate {
  linear_impact: number;
  sqrt_impact: number;
  recommended_impact: number;
  optimal_slices_twap: number;
  params: ImpactParams;
}

export interface ExecutionPlanResult {
  best_price: number;
  expected_price: number;
  slippage_percent: number;
  total_cost: number;
  depth_levels: DepthLevel[];
  total_depth: number;
  can_fill: boolean;
  optimal_slices: number;
  suggested_min_gap: number;
  /** Set when market history available (Kyle's λ, √V, TWAP n*). */
  impact?: ImpactEstimate;
}

export interface ScanParams {
  system_name: string;
  cargo_capacity: number;
  buy_radius: number;
  sell_radius: number;
  min_margin: number;
  sales_tax_percent: number;
  min_daily_volume?: number;
  max_investment?: number;
  max_results?: number;
  /** Route security: 0 = all space, 0.45 = highsec only, 0.7 = min 0.7 */
  min_route_security?: number;
  /** Target region name for regional arbitrage (empty = search all by radius) */
  target_region?: string;
  // Contract-specific filters
  min_contract_price?: number;
  max_contract_margin?: number;
  min_priced_ratio?: number;
  require_history?: boolean;
}

export interface AppConfig {
  system_name: string;
  cargo_capacity: number;
  buy_radius: number;
  sell_radius: number;
  min_margin: number;
  sales_tax_percent: number;
  opacity: number;
  window_x: number;
  window_y: number;
  window_w: number;
  window_h: number;
}

export interface AppStatus {
  sde_loaded: boolean;
  sde_systems: number;
  sde_types: number;
  esi_ok: boolean;
  esi_last_ok?: number; // Unix timestamp of last successful ESI check
}

export type NdjsonMessage =
  | { type: "progress"; message: string }
  | { type: "result"; data: FlipResult[]; count: number }
  | { type: "error"; message: string };

export interface AuthStatus {
  logged_in: boolean;
  character_id?: number;
  character_name?: string;
}

export interface CharacterInfo {
  character_id: number;
  character_name: string;
  wallet: number;
  orders: CharacterOrder[];
  order_history: HistoricalOrder[];
  transactions: WalletTransaction[];
  skills: SkillSheet | null;
}

export interface CharacterOrder {
  order_id: number;
  type_id: number;
  location_id: number;
  region_id: number;
  price: number;
  volume_remain: number;
  volume_total: number;
  is_buy_order: boolean;
  duration: number;
  issued: string;
  type_name?: string;
  location_name?: string;
}

export interface HistoricalOrder {
  order_id: number;
  type_id: number;
  location_id: number;
  region_id: number;
  price: number;
  volume_remain: number;
  volume_total: number;
  is_buy_order: boolean;
  state: "cancelled" | "expired" | "fulfilled";
  issued: string;
  type_name?: string;
  location_name?: string;
}

export interface WalletTransaction {
  transaction_id: number;
  date: string;
  type_id: number;
  location_id: number;
  unit_price: number;
  quantity: number;
  is_buy: boolean;
  type_name?: string;
  location_name?: string;
}

export interface SkillSheet {
  skills: { skill_id: number; active_skill_level: number }[];
  total_sp: number;
}

// --- Industry Types ---

export interface IndustryParams {
  type_id: number;
  runs: number;
  me: number; // Material Efficiency 0-10
  te: number; // Time Efficiency 0-20
  system_name: string;
  facility_tax: number;
  structure_bonus: number;
  broker_fee?: number;
  sales_tax_percent?: number;
  max_depth?: number;
}

export interface BlueprintInfo {
  blueprint_type_id: number;
  product_quantity: number;
  me: number;
  te: number;
  time: number;
}

export interface MaterialNode {
  type_id: number;
  type_name: string;
  quantity: number;
  is_base: boolean;
  buy_price: number;
  build_cost: number;
  should_build: boolean;
  job_cost: number;
  children: MaterialNode[] | null;
  blueprint: BlueprintInfo | null;
  depth: number;
}

export interface FlatMaterial {
  type_id: number;
  type_name: string;
  quantity: number;
  unit_price: number;
  total_price: number;
  volume: number;
}

export interface IndustryAnalysis {
  target_type_id: number;
  target_type_name: string;
  runs: number;
  total_quantity: number;
  market_buy_price: number;
  total_build_cost: number;
  optimal_build_cost: number;
  savings: number;
  savings_percent: number;
  total_job_cost: number;
  material_tree: MaterialNode;
  flat_materials: FlatMaterial[];
  system_cost_index: number;
  region_id: number;
  region_name?: string;
}

export type NdjsonIndustryMessage =
  | { type: "progress"; message: string }
  | { type: "result"; data: IndustryAnalysis }
  | { type: "error"; message: string };

export interface BuildableItem {
  type_id: number;
  type_name: string;
  has_blueprint: boolean;
}

export interface IndustrySystem {
  solar_system_id: number;
  solar_system_name: string;
  manufacturing: number;
  reaction: number;
  copying: number;
  invention: number;
}

// --- Demand / War Tracker Types ---

export interface DemandRegion {
  region_id: number;
  region_name: string;
  hot_score: number;
  status: "war" | "conflict" | "elevated" | "normal";
  kills_today: number;
  kills_baseline: number;
  isk_destroyed: number;
  active_players: number;
  top_ships: string[];
  updated_at?: string;
}

export interface DemandRegionsResponse {
  regions: DemandRegion[];
  count: number;
  cache_age_minutes: number;
  stale: boolean;
}

export interface HotZonesResponse {
  hot_zones: DemandRegion[];
  count: number;
  from_cache: boolean;
}

export interface DemandRegionResponse {
  region: DemandRegion;
  from_cache: boolean;
}

export interface TradeOpportunity {
  type_id: number;
  type_name: string;
  category: "ship" | "module" | "ammo";
  kills_per_day: number;
  jita_price: number;
  region_price: number;
  profit_per_unit: number;
  profit_percent: number;
  daily_volume: number;
  daily_profit: number;
  jita_volume: number;
  region_volume: number;
}

export interface RegionOpportunities {
  region_id: number;
  region_name: string;
  status: string;
  hot_score: number;
  security_class: "highsec" | "lowsec" | "nullsec";
  security_blocks: ("high" | "low" | "null")[];
  jumps_from_jita: number;
  main_system: string;
  ships: TradeOpportunity[];
  modules: TradeOpportunity[];
  ammo: TradeOpportunity[];
  total_potential: number;
}
