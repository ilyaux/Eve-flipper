export interface FlipResult {
  TypeID: number;
  TypeName: string;
  Volume: number;
  IskPerM3?: number;
  BuyPrice: number;
  BestAskPrice?: number;
  BestAskQty?: number;
  BuyStation: string;
  BuySystemName: string;
  BuySystemID: number;
  BuyRegionID?: number;
  BuyRegionName?: string;
  BuyLocationID?: number;
  SellPrice: number;
  BestBidPrice?: number;
  BestBidQty?: number;
  SellStation: string;
  SellSystemName: string;
  SellSystemID: number;
  SellRegionID?: number;
  SellRegionName?: string;
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
  S2BPerDay?: number;
  BfSPerDay?: number;
  S2BBfSRatio?: number;
  RealMarginPercent?: number;
  HistoryAvailable?: boolean;
  BuyCompetitors: number;
  SellCompetitors: number;
  DailyProfit: number;
  /** Expected fill prices from execution plan (order book depth) */
  ExpectedBuyPrice?: number;
  ExpectedSellPrice?: number;
  ExpectedProfit?: number;
  RealProfit?: number;
  FilledQty?: number;
  CanFill?: boolean;
  SlippageBuyPct?: number;
  SlippageSellPct?: number;
  // Regional day-trader enrichments (for EveGuru-style regional view in ScanResultsTable)
  DaySecurity?: number;
  DaySourceUnits?: number;
  DayTargetDemandPerDay?: number;
  DayTargetSupplyUnits?: number;
  DayTargetDOS?: number;
  DayAssets?: number;
  DayActiveOrders?: number;
  DaySourceAvgPrice?: number;
  DayTargetNowPrice?: number;
  DayTargetPeriodPrice?: number;
  DayNowProfit?: number;
  DayPeriodProfit?: number;
  DayROINow?: number;
  DayROIPeriod?: number;
  DayCapitalRequired?: number;
  DayShippingCost?: number;
  DayCategoryID?: number;
  DayGroupID?: number;
  DayGroupName?: string;
  /** ISK profit per m³ of cargo per jump — efficiency metric for haulers */
  DayIskPerM3Jump?: number;
  /** Composite 0-100 trade score (ROI 35% + Demand 25% + DOS 20% + Margin 20%) */
  DayTradeScore?: number;
  /** Last N daily average prices for the target region (spark-line chart) */
  DayPriceHistory?: number[];
  /** Lowest sell order price at the destination — populated in sell-order mode */
  DayTargetLowestSell?: number;
}

export interface RegionalDayTradeItem {
  type_id: number;
  type_name: string;
  source_system_id: number;
  source_system_name: string;
  source_station_name: string;
  source_location_id: number;
  source_region_id: number;
  source_region_name: string;
  target_system_id: number;
  target_system_name: string;
  target_station_name: string;
  target_location_id: number;
  target_region_id: number;
  target_region_name: string;
  purchase_units: number;
  source_units: number;
  target_demand_per_day: number;
  target_supply_units: number;
  target_dos: number;
  assets: number;
  active_orders: number;
  source_avg_price: number;
  target_now_price: number;
  target_period_price: number;
  target_now_profit: number;
  target_period_profit: number;
  roi_now: number;
  roi_period: number;
  capital_required: number;
  item_volume: number;
  shipping_cost: number;
  jumps: number;
  margin_now: number;
  margin_period: number;
  category_id?: number;
  group_id?: number;
  group_name?: string;
  trade_score?: number;
  target_price_history?: number[];
  target_lowest_sell?: number;
}

export interface RegionalDayTradeHub {
  source_system_id: number;
  source_system_name: string;
  source_region_id: number;
  source_region_name: string;
  security: number;
  purchase_units: number;
  source_units: number;
  target_demand_per_day: number;
  target_supply_units: number;
  target_dos: number;
  assets: number;
  active_orders: number;
  target_now_profit: number;
  target_period_profit: number;
  capital_required: number;
  shipping_cost: number;
  item_count: number;
  items: RegionalDayTradeItem[];
}

export interface ContractResult {
  ContractID: number;
  Title: string;
  Price: number;
  MarketValue: number;
  Profit: number;
  MarginPercent: number;
  ExpectedProfit?: number;
  ExpectedMarginPercent?: number;
  SellConfidence?: number;
  EstLiquidationDays?: number;
  ConservativeValue?: number;
  CarryCost?: number;
  Volume: number;
  StationName: string;
  SystemName?: string;
  RegionName?: string;
  LiquidationSystemName?: string;
  LiquidationRegionName?: string;
  LiquidationJumps?: number;
  ItemCount: number;
  Jumps: number;
  ProfitPerJump: number;
}

export interface ContractItem {
  type_id: number;
  type_name: string;
  quantity: number;
  is_included: boolean;
  is_blueprint_copy: boolean;
  group_id?: number;
  group_name?: string;
  category_id?: number;
  is_ship?: boolean;
  is_rig?: boolean;
  record_id: number;
  item_id: number;
  material_efficiency?: number;
  time_efficiency?: number;
  runs?: number;
  flag?: number;      // Item location flag (46-53 = fitted rigs, 0/1/5 = cargo/hangar)
  singleton?: boolean; // True for fitted items
  damage?: number;    // Damage level 0.0-1.0 (0.1 = 10% damaged)
}

export interface ContractDetails {
  contract_id: number;
  items: ContractItem[];
}

export type NdjsonContractMessage =
  | { type: "progress"; message: string }
  | { type: "result"; data: ContractResult[]; count: number; cache_meta?: StationCacheMeta }
  | { type: "error"; message: string };

export interface RouteHop {
  SystemName: string;
  StationName: string;
  SystemID: number;
  EmptyJumps?: number;
  DestSystemName: string;
  DestStationName?: string;
  DestSystemID: number;
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
  TargetSystemName?: string;
  TargetJumps?: number;
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
  alert_enabled?: boolean;
  alert_metric?: "margin_percent" | "total_profit" | "profit_per_unit" | "daily_volume";
  alert_threshold?: number;
}

export interface AlertHistoryEntry {
  id: number;
  watchlist_type_id: number;
  type_name: string;
  alert_metric: string;
  alert_threshold: number;
  current_value: number;
  message: string;
  channels_sent: string[];
  channels_failed?: Record<string, string>;
  sent_at: string;
  scan_id?: number;
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
  DailyProfit?: number;
  TheoreticalDailyProfit?: number;
  RealizableDailyProfit?: number;
  ConfidenceScore?: number;
  ConfidenceLabel?: string;
  HasExecutionEvidence?: boolean;
  ROI: number;
  StationName: string;
  StationID: number;
  SystemID?: number;
  RegionID?: number;
  // EVE Guru style metrics
  CapitalRequired: number;
  NowROI: number;
  PeriodROI: number;
  BuyUnitsPerDay: number;
  SellUnitsPerDay: number;
  BvSRatio: number;
  S2BPerDay?: number;
  BfSPerDay?: number;
  S2BBfSRatio?: number;
  RealMarginPercent?: number;
  HistoryAvailable?: boolean;
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
  RealProfit?: number;
  FilledQty?: number;
  CanFill?: boolean;
  SlippageBuyPct?: number;
  SlippageSellPct?: number;
}

export type NdjsonStationMessage =
  | { type: "progress"; message: string }
  | { type: "result"; data: StationTrade[]; count: number; cache_meta?: StationCacheMeta }
  | { type: "error"; message: string };

export interface StationInfo {
  id: number;
  name: string;
  system_id: number;
  region_id: number;
  is_structure?: boolean;
}

export interface StationsResponse {
  stations: StationInfo[];
  region_id: number;
  system_id: number;
}

export interface SolarSystemInfo {
  id: number;
  name: string;
  security: number;
  region_id: number;
}

// Execution plan (slippage / fill curve)
export interface DepthLevel {
  price: number;
  volume: number;
  cumulative: number;
  volume_filled: number;
}

/** Calibrated market impact params (Amihud illiquidity, σ from history). */
export interface ImpactParams {
  amihud: number;
  sigma: number;
  sigma_sq: number;
  avg_daily_volume: number;
  days_used: number;
  valid: boolean;
}

/** Impact estimate for a quantity: ΔP% (linear/√V) and TWAP slices. */
export interface ImpactEstimate {
  linear_impact_pct: number;
  sqrt_impact_pct: number;
  recommended_impact_pct: number;
  recommended_impact_isk: number;
  optimal_slices: number;
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
  ignored_system_ids?: number[];
  cargo_capacity: number;
  buy_radius: number;
  sell_radius: number;
  min_margin: number;
  sales_tax_percent: number;
  broker_fee_percent: number;
  split_trade_fees?: boolean;
  buy_broker_fee_percent?: number;
  sell_broker_fee_percent?: number;
  buy_sales_tax_percent?: number;
  sell_sales_tax_percent?: number;
  min_daily_volume?: number;
  max_investment?: number;
  min_item_profit?: number;
  min_period_roi?: number;
  max_dos?: number;
  min_demand_per_day?: number;
  purchase_demand_days?: number;
  min_s2b_per_day?: number;
  min_bfs_per_day?: number;
  min_s2b_bfs_ratio?: number;
  max_s2b_bfs_ratio?: number;
  avg_price_period?: number;
  shipping_cost_per_m3_jump?: number;
  /** Route security: 0 = all space, 0.45 = highsec only, 0.7 = min 0.7 */
  min_route_security?: number;
  /** Optional source-region scope for regional trade (empty = buy radius from System). */
  source_regions?: string[];
  /** Target region name for regional arbitrage (empty = search all by radius) */
  target_region?: string;
  /** Optional destination marketplace system for regional day trader. */
  target_market_system?: string;
  /** Optional destination marketplace location_id (station/structure). */
  target_market_location_id?: number;
  // Contract-specific filters
  min_contract_price?: number;
  max_contract_margin?: number;
  min_priced_ratio?: number;
  require_history?: boolean;
  contract_instant_liquidation?: boolean;
  contract_hold_days?: number;
  contract_target_confidence?: number;
  exclude_rigs_with_ship?: boolean;
  route_min_hops?: number;
  route_max_hops?: number;
  route_target_system_name?: string;
  route_min_isk_per_jump?: number;
  route_allow_empty_hops?: boolean;
  // Player structures
  include_structures?: boolean;
  /** Category filter for regional day trader. Empty = all. */
  category_ids?: number[];
  /** When true, use lowest sell order at destination as revenue price instead of highest buy order. */
  sell_order_mode?: boolean;
}

export interface AppConfig {
  system_name: string;
  ignored_system_ids?: number[];
  cargo_capacity: number;
  buy_radius: number;
  sell_radius: number;
  min_margin: number;
  sales_tax_percent: number;
  broker_fee_percent: number;
  split_trade_fees?: boolean;
  buy_broker_fee_percent?: number;
  sell_broker_fee_percent?: number;
  buy_sales_tax_percent?: number;
  sell_sales_tax_percent?: number;
  min_daily_volume?: number;
  max_investment?: number;
  min_item_profit?: number;
  min_s2b_per_day?: number;
  min_bfs_per_day?: number;
  min_s2b_bfs_ratio?: number;
  max_s2b_bfs_ratio?: number;
  min_route_security?: number;
  avg_price_period?: number;
  min_period_roi?: number;
  max_dos?: number;
  min_demand_per_day?: number;
  purchase_demand_days?: number;
  shipping_cost_per_m3_jump?: number;
  source_regions?: string[];
  target_region?: string;
  target_market_system?: string;
  target_market_location_id?: number;
  category_ids?: number[];
  sell_order_mode?: boolean;
  alert_telegram: boolean;
  alert_discord: boolean;
  alert_desktop: boolean;
  alert_telegram_token: string;
  alert_telegram_chat_id: string;
  alert_discord_webhook: string;
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

export interface AuthCharacter {
  character_id: number;
  character_name: string;
  active: boolean;
}

export interface AuthStatus {
  logged_in: boolean;
  character_id?: number;
  character_name?: string;
  characters?: AuthCharacter[];
  auth_revision?: number;
}

export interface CharacterInfo {
  character_id: number;
  character_name: string;
  wallet: number;
  orders: CharacterOrder[];
  order_history: HistoricalOrder[];
  transactions: WalletTransaction[];
  skills: SkillSheet | null;
  risk?: CharacterRiskSummary | null;
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

export interface CharacterRiskSummary {
  risk_score: number;
  risk_level: "safe" | "balanced" | "high" | string;
  var_95: number;
  var_99: number;
  es_95: number;
  es_99: number;
  typical_daily_pnl: number;
  worst_day_loss: number;
  sample_days: number;
  window_days: number;
  capacity_multiplier: number;
  low_sample?: boolean;
  var_99_reliable?: boolean;
}

// --- Undercut Monitor Types ---

export interface UndercutStatus {
  order_id: number;
  position: number;
  total_orders: number;
  best_price: number;
  undercut_amount: number;
  undercut_pct: number;
  suggested_price: number;
  book_levels: BookLevel[];
}

export interface BookLevel {
  price: number;
  volume: number;
  is_player: boolean;
}

export interface OrderDeskSummary {
  total_orders: number;
  buy_orders: number;
  sell_orders: number;
  needs_reprice: number;
  needs_cancel: number;
  total_notional: number;
  median_eta_days: number;
  avg_eta_days: number;
  worst_eta_days: number;
  unknown_eta_count: number;
}

export interface OrderDeskSettings {
  sales_tax_percent: number;
  broker_fee_percent: number;
  target_eta_days: number;
  warn_expiry_days: number;
}

export interface OrderDeskOrder {
  order_id: number;
  type_id: number;
  type_name: string;
  location_id: number;
  location_name: string;
  region_id: number;
  is_buy_order: boolean;
  price: number;
  volume_remain: number;
  volume_total: number;
  notional: number;
  net_unit_isk: number;
  net_notional: number;
  position: number;
  total_orders: number;
  book_available: boolean;
  best_price: number;
  suggested_price: number;
  undercut_amount: number;
  undercut_pct: number;
  queue_ahead_qty: number;
  top_price_qty: number;
  avg_daily_volume: number;
  estimated_fill_per_day: number;
  eta_days: number;
  issued_at: string;
  expires_at: string;
  days_to_expire: number;
  recommendation: "hold" | "reprice" | "cancel" | string;
  reason: string;
}

export interface OrderDeskResponse {
  summary: OrderDeskSummary;
  orders: OrderDeskOrder[];
  settings: OrderDeskSettings;
}

export type StationCommandAction = "new_entry" | "reprice" | "hold" | "cancel";

export interface StationForecastBand {
  p50: number;
  p80: number;
  p95: number;
}

export interface StationCommandForecast {
  daily_volume: StationForecastBand;
  daily_profit: StationForecastBand;
  eta_days: StationForecastBand;
}

export interface StationCommandRow {
  trade: StationTrade;
  personalized_score: number;
  recommended_action: StationCommandAction;
  action_reason: string;
  priority: number;
  active_order_count: number;
  active_order_at_station: number;
  open_position_qty: number;
  expected_delta_daily_profit: number;
  forecast: StationCommandForecast;
}

export interface StationCommandSummary {
  rows: number;
  new_entry_count: number;
  reprice_count: number;
  hold_count: number;
  cancel_count: number;
  with_active_orders: number;
  with_open_positions: number;
}

export interface StationCommandResult {
  generated_at: string;
  summary: StationCommandSummary;
  rows: StationCommandRow[];
}

export interface StationCacheMeta {
  current_revision: number;
  last_refresh_at?: string;
  next_expiry_at?: string;
  min_ttl_sec: number;
  max_ttl_sec: number;
  regions: number;
  entries: number;
  stale: boolean;
}

export interface StationCommandResponse {
  generated_at: string;
  scope: "single" | "all";
  scan_scope: string;
  region_count: number;
  result_count: number;
  cache_meta?: StationCacheMeta;
  command: StationCommandResult;
  order_desk: OrderDeskResponse;
  inventory: {
    open_positions: number;
    open_quantity: number;
    transactions: number;
  };
}

export type StationTradeStateMode = "done" | "ignored";

export interface StationTradeState {
  user_id: string;
  tab: string;
  type_id: number;
  station_id: number;
  region_id: number;
  mode: StationTradeStateMode;
  until_revision: number;
  updated_at: string;
}

export interface StationAIContextRow {
  type_id: number;
  type_name: string;
  station_name: string;
  cts: number;
  margin_percent: number;
  daily_profit: number;
  daily_volume: number;
  s2b_bfs_ratio: number;
  action: string;
  reason: string;
  confidence: string;
  high_risk: boolean;
  extreme_price: boolean;
}

export interface StationAIContextSummary {
  total_rows: number;
  visible_rows: number;
  high_risk_rows: number;
  extreme_rows: number;
  avg_cts: number;
  avg_margin: number;
  avg_daily_profit: number;
  avg_daily_volume: number;
  actionable_rows: number;
}

export interface StationAIScanSnapshot {
  scope_mode: "radius" | "single_station" | "region_all";
  system_name: string;
  region_id: number;
  station_id: number;
  radius: number;
  min_margin: number;
  sales_tax_percent: number;
  broker_fee: number;
  split_trade_fees: boolean;
  buy_broker_fee_percent: number;
  sell_broker_fee_percent: number;
  buy_sales_tax_percent: number;
  sell_sales_tax_percent: number;
  cts_profile: string;
  min_daily_volume: number;
  min_item_profit: number;
  min_s2b_per_day: number;
  min_bfs_per_day: number;
  avg_price_period: number;
  min_period_roi: number;
  bvs_ratio_min: number;
  bvs_ratio_max: number;
  max_pvi: number;
  max_sds: number;
  limit_buy_to_price_low: boolean;
  flag_extreme_prices: boolean;
  include_structures: boolean;
  structures_applied: boolean;
  structure_count: number;
  structure_ids: number[];
}

export interface StationAIChatContext {
  tab_id?: string;
  tab_title?: string;
  system_name: string;
  station_scope: string;
  region_id: number;
  station_id: number;
  radius: number;
  min_margin: number;
  min_daily_volume: number;
  min_item_profit: number;
  scan_snapshot: StationAIScanSnapshot;
  summary: StationAIContextSummary;
  rows: StationAIContextRow[];
}

export interface StationAIHistoryMessage {
  role: "user" | "assistant";
  content: string;
}

export interface StationAIChatRequest {
  provider: "openrouter";
  api_key: string;
  model: string;
  planner_model?: string;
  temperature: number;
  max_tokens: number;
  assistant_name: string;
  locale: "ru" | "en";
  user_message: string;
  enable_wiki_context?: boolean;
  enable_web_research?: boolean;
  enable_planner?: boolean;
  wiki_repo?: string;
  history?: StationAIHistoryMessage[];
  context: StationAIChatContext;
}

export interface StationAIChatResponse {
  answer: string;
  provider: string;
  model: string;
  assistant: string;
  intent?: string;
  pipeline?: {
    planner_enabled?: boolean;
    planner_model?: string;
    response_mode?: string;
    context_level?: string;
    agents?: string[];
  };
  warnings?: string[];
  provider_id?: string;
  provider_usage?: Record<string, unknown>;
  usage?: StationAIUsage;
}

export interface StationAIUsage {
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
}

export type StationAIStreamMessage =
  | {
      type: "progress";
      message: string;
      progress_pct?: number;
      prompt_tokens_est?: number;
      completion_tokens_est?: number;
      total_tokens_est?: number;
    }
  | {
      type: "delta";
      delta: string;
      progress_pct?: number;
      completion_tokens_est?: number;
      total_tokens_est?: number;
    }
  | ({
      type: "usage";
      progress_pct?: number;
    } & StationAIUsage)
  | ({
      type: "result";
      progress_pct?: number;
      progress_text?: string;
    } & StationAIChatResponse)
  | { type: "error"; message: string };

// --- Industry Types ---

export interface IndustryParams {
  type_id: number;
  runs: number;
  me: number; // Material Efficiency 0-10
  te: number; // Time Efficiency 0-20
  system_name: string;
  station_id?: number; // Optional station/structure ID for price lookup
  facility_tax: number;
  structure_bonus: number;
  broker_fee?: number;
  sales_tax_percent?: number;
  max_depth?: number;
  own_blueprint?: boolean;
  blueprint_cost?: number;
  blueprint_is_bpo?: boolean;
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
  sell_revenue: number;
  profit: number;
  profit_percent: number;
  isk_per_hour: number;
  manufacturing_time: number;
  total_job_cost: number;
  material_tree: MaterialNode;
  flat_materials: FlatMaterial[];
  system_cost_index: number;
  region_id: number;
  region_name?: string;
  blueprint_cost_included: number;
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

export type IndustryProjectStatus = "draft" | "planned" | "active" | "completed" | "archived";

export type IndustryTaskStatus =
  | "planned"
  | "ready"
  | "active"
  | "paused"
  | "completed"
  | "blocked"
  | "cancelled";

export type IndustryJobStatus =
  | "planned"
  | "queued"
  | "active"
  | "paused"
  | "completed"
  | "failed"
  | "cancelled";

export interface IndustryProject {
  id: number;
  user_id: string;
  name: string;
  status: IndustryProjectStatus;
  strategy: "conservative" | "balanced" | "aggressive";
  notes: string;
  params: unknown;
  created_at: string;
  updated_at: string;
}

export interface IndustryTaskPlanInput {
  // Source task ID used to remap links on replace/append plan apply.
  task_id?: number;
  // Existing task ID, or negative row ref (-1 = first task row in current patch).
  parent_task_id?: number;
  name: string;
  activity: string;
  product_type_id?: number;
  target_runs?: number;
  planned_start?: string;
  planned_end?: string;
  priority?: number;
  status?: IndustryTaskStatus;
  constraints?: unknown;
}

export interface IndustryJobPlanInput {
  // Existing task ID, or negative row ref (-1 = first task row in current patch).
  task_id?: number;
  character_id?: number;
  facility_id?: number;
  activity: string;
  runs?: number;
  duration_seconds?: number;
  cost_isk?: number;
  status?: IndustryJobStatus;
  started_at?: string;
  finished_at?: string;
  external_job_id?: number;
  notes?: string;
}

export interface IndustryMaterialPlanInput {
  task_id?: number;
  type_id: number;
  type_name?: string;
  required_qty?: number;
  available_qty?: number;
  buy_qty?: number;
  build_qty?: number;
  unit_cost_isk?: number;
  source?: "market" | "stock" | "build" | "reprocess" | "contract";
}

export interface IndustryBlueprintPoolInput {
  blueprint_type_id: number;
  blueprint_name?: string;
  location_id?: number;
  quantity?: number;
  me?: number;
  te?: number;
  is_bpo?: boolean;
  available_runs?: number;
}

export interface IndustryPlanSchedulerInput {
  enabled?: boolean;
  slot_count?: number;
  max_job_runs?: number;
  max_job_duration_seconds?: number;
  window_days?: number;
  queue_status?: IndustryJobStatus;
}

export interface IndustryPlanPatch {
  replace?: boolean;
  replace_blueprints?: boolean;
  project_status?: IndustryProjectStatus;
  tasks?: IndustryTaskPlanInput[];
  jobs?: IndustryJobPlanInput[];
  materials?: IndustryMaterialPlanInput[];
  blueprints?: IndustryBlueprintPoolInput[];
  scheduler?: IndustryPlanSchedulerInput;
  strict_bp_bypass?: boolean;
}

export interface IndustryPlanSummary {
  project_id: number;
  project_status: IndustryProjectStatus;
  replaced: boolean;
  tasks_inserted: number;
  jobs_inserted: number;
  materials_upserted: number;
  blueprints_upserted: number;
  scheduler_applied?: boolean;
  jobs_split_from?: number;
  jobs_planned_total?: number;
  warnings?: string[];
  updated_at: string;
}

export interface IndustryTaskPreview {
  input_index: number;
  task_id: number;
  parent_task_id: number;
  name: string;
  activity: string;
  target_runs: number;
  planned_start: string;
  planned_end: string;
  priority: number;
}

export interface IndustryPlanPreview {
  project_id: number;
  replace: boolean;
  summary: IndustryPlanSummary;
  tasks: IndustryTaskPreview[];
  jobs: IndustryJobPlanInput[];
  warnings: string[];
}

export interface IndustryJob {
  id: number;
  user_id: string;
  project_id: number;
  task_id: number;
  character_id: number;
  facility_id: number;
  activity: string;
  runs: number;
  duration_seconds: number;
  cost_isk: number;
  status: IndustryJobStatus;
  started_at: string;
  finished_at: string;
  external_job_id: number;
  notes: string;
  created_at: string;
  updated_at: string;
}

export interface IndustryLedgerEntry {
  job_id: number;
  project_id: number;
  project_name: string;
  task_id: number;
  task_name: string;
  character_id: number;
  facility_id: number;
  activity: string;
  runs: number;
  duration_seconds: number;
  cost_isk: number;
  status: IndustryJobStatus;
  started_at: string;
  finished_at: string;
  external_job_id: number;
  notes: string;
  updated_at: string;
}

export interface IndustryLedger {
  project_id: number;
  status_filter: string;
  limit: number;
  total: number;
  planned: number;
  active: number;
  completed: number;
  failed: number;
  cancelled: number;
  total_cost_isk: number;
  entries: IndustryLedgerEntry[];
}

export interface IndustryTaskRecord {
  id: number;
  user_id: string;
  project_id: number;
  parent_task_id: number;
  name: string;
  activity: string;
  product_type_id: number;
  target_runs: number;
  planned_start: string;
  planned_end: string;
  priority: number;
  status: IndustryTaskStatus;
  constraints: unknown;
  created_at: string;
  updated_at: string;
}

export interface IndustryMaterialPlanRecord {
  id: number;
  user_id: string;
  project_id: number;
  task_id: number;
  type_id: number;
  type_name: string;
  required_qty: number;
  available_qty: number;
  buy_qty: number;
  build_qty: number;
  unit_cost_isk: number;
  source: "market" | "stock" | "build" | "reprocess" | "contract";
  updated_at: string;
}

export interface IndustryBlueprintPoolRecord {
  id: number;
  user_id: string;
  project_id: number;
  blueprint_type_id: number;
  blueprint_name: string;
  location_id: number;
  quantity: number;
  me: number;
  te: number;
  is_bpo: boolean;
  available_runs: number;
  updated_at: string;
}

export interface IndustryMaterialDiff {
  type_id: number;
  type_name: string;
  required_qty: number;
  available_qty: number;
  buy_qty: number;
  build_qty: number;
  missing_qty: number;
}

export interface IndustryProjectSnapshot {
  project: IndustryProject;
  tasks: IndustryTaskRecord[];
  jobs: IndustryJob[];
  materials: IndustryMaterialPlanRecord[];
  blueprints: IndustryBlueprintPoolRecord[];
  material_diff: IndustryMaterialDiff[];
}

// --- Portfolio P&L Types ---

export interface DailyPnLEntry {
  date: string;
  buy_total: number;
  sell_total: number;
  net_pnl: number;
  cumulative_pnl: number;
  drawdown_pct: number;
  transactions: number;
}

export interface PortfolioPnLStats {
  total_pnl: number;
  avg_daily_pnl: number;
  best_day_pnl: number;
  best_day_date: string;
  worst_day_pnl: number;
  worst_day_date: string;
  profitable_days: number;
  losing_days: number;
  total_days: number;
  win_rate: number;
  total_bought: number;
  total_sold: number;
  roi_percent: number;
  // Enhanced analytics
  sharpe_ratio: number;
  max_drawdown_pct: number;
  max_drawdown_isk: number;
  max_drawdown_days: number;
  calmar_ratio: number;
  profit_factor: number;
  avg_win: number;
  avg_loss: number;
  expectancy_per_trade: number;
  realized_trades: number;
  realized_quantity: number;
  open_positions: number;
  open_cost_basis: number;
  total_fees: number;
  total_taxes: number;
}

export interface StationPnL {
  location_id: number;
  location_name: string;
  total_bought: number;
  total_sold: number;
  net_pnl: number;
  transactions: number;
}

export interface ItemPnL {
  type_id: number;
  type_name: string;
  total_bought: number;
  total_sold: number;
  net_pnl: number;
  qty_bought: number;
  qty_sold: number;
  avg_buy_price: number;
  avg_sell_price: number;
  margin_percent: number;
  transactions: number;
}

export interface RealizedTrade {
  type_id: number;
  type_name: string;
  quantity: number;
  buy_transaction_id: number;
  sell_transaction_id: number;
  buy_date: string;
  sell_date: string;
  holding_days: number;
  buy_location_id: number;
  buy_location_name: string;
  sell_location_id: number;
  sell_location_name: string;
  buy_unit_price: number;
  sell_unit_price: number;
  buy_gross: number;
  sell_gross: number;
  buy_fee: number;
  sell_broker_fee: number;
  sell_tax: number;
  buy_total: number;
  sell_total: number;
  realized_pnl: number;
  margin_percent: number;
  unmatched?: boolean;
}

export interface OpenPosition {
  type_id: number;
  type_name: string;
  location_id: number;
  location_name: string;
  quantity: number;
  avg_cost: number;
  cost_basis: number;
  oldest_lot_date: string;
}

export interface MatchingCoverage {
  total_sell_qty: number;
  matched_sell_qty: number;
  unmatched_sell_qty: number;
  total_sell_value: number;
  matched_sell_value: number;
  unmatched_sell_value: number;
  match_rate_qty_pct: number;
  match_rate_value_pct: number;
}

export interface PortfolioSettings {
  lookback_days: number;
  sales_tax_percent: number;
  broker_fee_percent: number;
  ledger_limit: number;
  include_unmatched_sell: boolean;
}

export interface PortfolioPnL {
  daily_pnl: DailyPnLEntry[];
  summary: PortfolioPnLStats;
  top_items: ItemPnL[];
  top_stations: StationPnL[];
  ledger: RealizedTrade[];
  open_positions: OpenPosition[];
  coverage: MatchingCoverage;
  settings: PortfolioSettings;
}

// --- Portfolio Optimizer Types ---

export interface AssetStats {
  type_id: number;
  type_name: string;
  avg_daily_pnl: number;
  volatility: number;
  sharpe_ratio: number;
  current_weight: number;
  total_invested: number;
  total_pnl: number;
  trading_days: number;
}

export interface FrontierPoint {
  risk: number;
  return: number;
}

export interface AllocationSuggestion {
  type_id: number;
  type_name: string;
  action: "increase" | "decrease" | "hold";
  current_pct: number;
  optimal_pct: number;
  delta_pct: number;
  reason: string;
}

export interface PortfolioOptimization {
  assets: AssetStats[];
  correlation_matrix: number[][];
  current_weights: number[];
  optimal_weights: number[];
  min_var_weights: number[];
  efficient_frontier: FrontierPoint[];
  diversification_ratio: number;
  current_sharpe: number;
  optimal_sharpe: number;
  min_var_sharpe: number;
  hhi: number;
  suggestions: AllocationSuggestion[];
}

export interface DiagnosticItem {
  type_id: number;
  type_name: string;
  trading_days: number;
  transactions: number;
}

export interface OptimizerDiagnostic {
  total_transactions: number;
  within_lookback: number;
  unique_days: number;
  unique_items: number;
  qualified_items: number;
  min_days_required: number;
  top_items: DiagnosticItem[];
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
  category: "ship" | "module" | "ammo" | "drone";
  kills_per_day: number;
  jita_price: number;
  region_price: number;
  profit_per_unit: number;
  profit_percent: number;
  daily_volume: number;
  daily_profit: number;
  jita_volume: number;
  region_volume: number;
  data_source?: "killmail" | "static";
  volume?: number;
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

// --- PLEX+ Types ---

export interface PLEXGlobalPrice {
  buy_price: number;
  sell_price: number;
  spread: number;
  spread_pct: number;
  volume_24h: number;
  buy_orders: number;
  sell_orders: number;
  percentile_90d: number;
}

export interface ArbitragePath {
  name: string;
  type: "nes_sell" | "nes_process" | "market_process" | "spread";
  plex_cost: number;
  cost_isk: number;
  revenue_gross: number;
  revenue_isk: number;
  profit_isk: number;
  roi: number;
  viable: boolean;
  no_data: boolean;
  detail: string;
  break_even_plex: number;
  est_minutes: number;
  isk_per_hour: number;
  slippage_pct: number;
  adjusted_profit_isk: number;
}

export interface SPFarmResult {
  omega_cost_plex: number;
  omega_cost_isk: number;
  extractors_per_month: number;
  extractor_cost_plex: number;
  extractor_cost_isk: number;
  total_cost_isk: number;
  injectors_produced: number;
  injector_sell_price: number;
  revenue_isk: number;
  profit_isk: number;
  profit_per_day: number;
  roi: number;
  viable: boolean;
  extractors_plus5: number;
  profit_plus5: number;
  profit_per_day_plus5: number;
  roi_plus5: number;
  // Startup & multi-char
  startup_sp: number;
  startup_train_days: number;
  startup_cost_isk: number;
  payback_days: number;
  mptc_cost_plex: number;
  mptc_cost_isk: number;
  // Omega ISK value
  omega_isk_value: number;
  plex_unit_price: number;
  // Instant sell alternative
  instant_sell_revenue_isk: number;
  instant_sell_profit_isk: number;
  instant_sell_roi: number;
  instant_sell_profit_plus5: number;
  instant_sell_roi_plus5: number;
  break_even_plex: number;
}

export interface PLEXIndicators {
  sma7: number;
  sma30: number;
  bollinger_upper: number;
  bollinger_middle: number;
  bollinger_lower: number;
  rsi: number;
  change_24h: number;
  change_7d: number;
  change_30d: number;
  avg_volume_30d: number;
  volume_today: number;
  volume_sigma: number;
  ccp_sale_signal: boolean;
  volatility_20d: number;
  vol_regime: "low" | "medium" | "high" | "";
}

export interface PLEXSignal {
  action: "BUY" | "SELL" | "HOLD";
  confidence: number;
  reasons: string[];
}

export interface PricePoint {
  date: string;
  average: number;
  high: number;
  low: number;
  volume: number;
}

export interface ChartOverlayPoint {
  date: string;
  value: number;
}

export interface ChartOverlays {
  sma7?: ChartOverlayPoint[];
  sma30?: ChartOverlayPoint[];
  bollinger_upper?: ChartOverlayPoint[];
  bollinger_lower?: ChartOverlayPoint[];
}

export interface ArbHistoryPoint {
  date: string;
  profit_isk: number;
  roi: number;
}

export interface ArbHistoryData {
  extractor_nes?: ArbHistoryPoint[];
  sp_chain_nes?: ArbHistoryPoint[];
  mptc_nes?: ArbHistoryPoint[];
  sp_farm_profit?: ArbHistoryPoint[];
}

export interface DepthSummary {
  total_volume: number;
  best_price: number;
  worst_price: number;
  levels: number;
}

export interface MarketDepthInfo {
  plex_sell_depth_5: DepthSummary;
  extractor_sell_qty: number;
  extractor_buy_qty: number;
  injector_sell_qty: number;
  injector_buy_qty: number;
  mptc_sell_qty: number;
  mptc_buy_qty: number;
  extractor_fill_hours: number;
  injector_fill_hours: number;
  mptc_fill_hours: number;
  plex_fill_hours: number;
}

export interface InjectionTier {
  label: string;
  sp_received: number;
  isk_per_sp: number;
  efficiency: number;
}

export interface OmegaComparison {
  plex_needed: number;
  total_isk: number;
  real_money_usd: number;
  isk_per_usd: number;
}

export interface CrossHubArbitrage {
  item_name: string;
  type_id: number;
  best_hub: string;
  best_price: number;
  jita_price: number;
  diff_pct: number;
  profit_isk: number;
  viable: boolean;
}

export interface PLEXDashboard {
  plex_price: PLEXGlobalPrice;
  arbitrage: ArbitragePath[];
  sp_farm: SPFarmResult;
  indicators: PLEXIndicators | null;
  chart_overlays?: ChartOverlays | null;
  arb_history?: ArbHistoryData | null;
  market_depth?: MarketDepthInfo | null;
  signal: PLEXSignal;
  history: PricePoint[];
  injection_tiers?: InjectionTier[] | null;
  omega_comparison?: OmegaComparison | null;
  cross_hub?: CrossHubArbitrage[] | null;
}

// ============================================================
// Corporation types
// ============================================================

export interface CharacterRoles {
  roles: string[];
  is_director: boolean;
  corporation_id: number;
}

export interface CorpWalletDivision {
  division: number;
  name: string;
  balance: number;
}

export interface IncomeSource {
  category: string;
  label: string;
  amount: number;
  percent: number;
}

export interface DailyPnLEntry {
  date: string;
  revenue: number;
  expenses: number;
  net_income: number;
  cumulative: number;
  transactions: number;
}

export interface MemberContribution {
  character_id: number;
  name: string;
  total_isk: number;
  category: string;
  is_online: boolean;
}

export interface MemberSummary {
  total_members: number;
  active_last_7d: number;
  active_last_30d: number;
  inactive_30d: number;
  miners: number;
  ratters: number;
  traders: number;
  industrialists: number;
  pvpers: number;
  other: number;
}

export interface ProductEntry {
  type_id: number;
  type_name: string;
  runs: number;
  jobs: number;
  estimated_isk?: number;
}

export interface OreEntry {
  type_id: number;
  type_name: string;
  quantity: number;
  estimated_isk?: number;
}

export interface IndustrySummary {
  active_jobs: number;
  completed_jobs_30d: number;
  production_value: number;
  top_products: ProductEntry[];
}

export interface MiningSummary {
  total_volume_30d: number;
  estimated_isk: number;
  active_miners: number;
  top_ores: OreEntry[];
}

export interface MarketSummary {
  active_buy_orders: number;
  active_sell_orders: number;
  total_buy_value: number;
  total_sell_value: number;
  unique_traders: number;
}

export interface CorpJournalEntry {
  id: number;
  date: string;
  ref_type: string;
  amount: number;
  balance: number;
  description: string;
  first_party_id: number;
  first_party_name: string;
  second_party_id: number;
  second_party_name: string;
}

export interface CorpMember {
  character_id: number;
  name: string;
  last_login: string;
  logoff_date: string;
  ship_type_id: number;
  ship_name: string;
  location_id: number;
  system_id: number;
  system_name: string;
}

export interface CorpMarketOrderDetail {
  order_id: number;
  character_id: number;
  character_name: string;
  type_id: number;
  type_name: string;
  price: number;
  volume_remain: number;
  volume_total: number;
  is_buy_order: boolean;
  location_id: number;
  location_name: string;
  issued: string;
  duration: number;
  region_id: number;
}

export interface CorpIndustryJob {
  job_id: number;
  installer_id: number;
  installer_name: string;
  activity: string;
  blueprint_type_id: number;
  product_type_id: number;
  product_name: string;
  status: string;
  runs: number;
  start_date: string;
  end_date: string;
  location_id: number;
  location_name: string;
}

export interface CorpMiningEntry {
  character_id: number;
  character_name: string;
  date: string;
  type_id: number;
  type_name: string;
  quantity: number;
}

export interface CorpDashboard {
  info: {
    corporation_id: number;
    name: string;
    ticker: string;
    member_count: number;
  };
  is_demo: boolean;
  wallets: CorpWalletDivision[];
  total_balance: number;
  revenue_30d: number;
  expenses_30d: number;
  net_income_30d: number;
  revenue_7d: number;
  expenses_7d: number;
  net_income_7d: number;
  income_by_source: IncomeSource[];
  daily_pnl: DailyPnLEntry[];
  top_contributors: MemberContribution[];
  member_summary: MemberSummary;
  industry_summary: IndustrySummary;
  mining_summary: MiningSummary;
  market_summary: MarketSummary;
}
