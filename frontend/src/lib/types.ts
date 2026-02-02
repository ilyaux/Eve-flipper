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

export interface ScanParams {
  system_name: string;
  cargo_capacity: number;
  buy_radius: number;
  sell_radius: number;
  min_margin: number;
  sales_tax_percent: number;
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
