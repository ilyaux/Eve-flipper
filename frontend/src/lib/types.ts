export interface FlipResult {
  TypeID: number;
  TypeName: string;
  Volume: number;
  BuyPrice: number;
  BuyStation: string;
  BuySystemName: string;
  BuySystemID: number;
  BuyRegionID?: number;
  BuyRegionName?: string;
  BuyLocationID?: number;
  SellPrice: number;
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
  BuyCompetitors: number;
  SellCompetitors: number;
  DailyProfit: number;
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
  SystemName?: string;
  RegionName?: string;
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
  DestStationName?: string;
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
  cargo_capacity: number;
  buy_radius: number;
  sell_radius: number;
  min_margin: number;
  sales_tax_percent: number;
  broker_fee_percent: number;
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

export interface PortfolioPnL {
  daily_pnl: DailyPnLEntry[];
  summary: PortfolioPnLStats;
  top_items: ItemPnL[];
  top_stations: StationPnL[];
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
