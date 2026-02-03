export interface FlipResult {
  TypeID: number;
  TypeName: string;
  Volume: number;
  BuyPrice: number;
  BuyStation: string;
  BuySystemName: string;
  BuySystemID: number;
  SellPrice: number;
  SellStation: string;
  SellSystemName: string;
  SellSystemID: number;
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
}

export interface SkillSheet {
  skills: { skill_id: number; active_skill_level: number }[];
  total_sp: number;
}
