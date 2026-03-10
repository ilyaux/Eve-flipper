const STORAGE_KEY = "eve-flipper-presets";

export type PresetTab =
  | "flipper"
  | "region"
  | "contracts"
  | "route"
  | "station"
  | "industry"
  | "demand"
  | "plex";

/* eslint-disable @typescript-eslint/no-explicit-any */
export interface SavedPreset {
  id: string;
  name: string;
  tab: PresetTab;
  params: Record<string, any>;
  createdAt?: number;
}

export interface BuiltinPreset {
  id: string;
  nameKey: string;
  tab: PresetTab;
  params: Record<string, any>;
}
/* eslint-enable @typescript-eslint/no-explicit-any */

// ── Station Trading Settings ──

export interface StationTradingSettings {
  systemName?: string;
  selectedStationId?: number;
  includeStructures?: boolean;
  minMargin: number;
  brokerFee: number;
  salesTaxPercent: number;
  splitTradeFees?: boolean;
  buyBrokerFeePercent?: number;
  sellBrokerFeePercent?: number;
  buySalesTaxPercent?: number;
  sellSalesTaxPercent?: number;
  ctsProfile?: "balanced" | "aggressive" | "defensive";
  radius: number;
  minDailyVolume: number;
  minItemProfit: number;
  minDemandPerDay: number;
  minBfSPerDay?: number;
  avgPricePeriod: number;
  minPeriodROI: number;
  bvsRatioMin: number;
  bvsRatioMax: number;
  maxPVI: number;
  maxSDS: number;
  limitBuyToPriceLow: boolean;
  flagExtremePrices: boolean;
}

const PRESET_TAB_SET = new Set<PresetTab>([
  "flipper",
  "region",
  "contracts",
  "route",
  "station",
  "industry",
  "demand",
  "plex",
]);

const USER_BOUND_PRESET_KEYS = new Set<string>([
  "ignored_system_ids",
]);

function isPresetTab(value: unknown): value is PresetTab {
  return typeof value === "string" && PRESET_TAB_SET.has(value as PresetTab);
}

export function sanitizePresetParams(params: Record<string, any>): Record<string, any> {
  const out: Record<string, any> = { ...params };
  for (const key of USER_BOUND_PRESET_KEYS) {
    if (key in out) delete out[key];
  }
  return out;
}

const defaultScanPresetParams = {
  cargo_capacity: 5000,
  buy_radius: 5,
  sell_radius: 10,
  min_margin: 5,
  sales_tax_percent: 8,
  broker_fee_percent: 3,
  split_trade_fees: false,
  buy_broker_fee_percent: 3,
  sell_broker_fee_percent: 3,
  buy_sales_tax_percent: 0,
  sell_sales_tax_percent: 8,
  min_daily_volume: 0,
  max_investment: 0,
  min_item_profit: 0,
  min_period_roi: 0,
  max_dos: 0,
  min_demand_per_day: 0,
  purchase_demand_days: 0.5,
  avg_price_period: 14,
  shipping_cost_per_m3_jump: 0,
  min_s2b_per_day: 0,
  min_bfs_per_day: 0,
  min_s2b_bfs_ratio: 0,
  max_s2b_bfs_ratio: 0,
  category_ids: [] as number[],
  sell_order_mode: false,
  restrict_to_target_market: true,
  min_route_security: 0.45,
  source_regions: [],
  target_region: "",
  target_market_system: "Jita",
  target_market_location_id: 0,
  min_contract_price: 10_000_000,
  max_contract_margin: 100,
  min_priced_ratio: 0.8,
  require_history: false,
  contract_instant_liquidation: false,
  contract_hold_days: 7,
  contract_target_confidence: 80,
  exclude_rigs_with_ship: true,
  include_structures: false,
  route_min_hops: 2,
  route_max_hops: 5,
  route_target_system_name: "",
  route_min_isk_per_jump: 0,
  route_allow_empty_hops: false,
};

export function getPresetApplyBase(tab: string): Record<string, any> {
  const mapped = mapTabToPresetTab(tab);
  switch (mapped) {
    case "flipper":
    case "region":
    case "contracts":
    case "route":
      return { ...defaultScanPresetParams };
    default:
      return {};
  }
}

export const BUILTIN_PRESETS: BuiltinPreset[] = [
  // ── Flipper ──
  {
    id: "flip-conservative",
    nameKey: "presetConservative",
    tab: "flipper",
    params: {
      ...defaultScanPresetParams,
      min_margin: 15,
      buy_radius: 5,
      sell_radius: 5,
      min_daily_volume: 10,
      min_route_security: 0.45,
    },
  },
  {
    id: "flip-normal",
    nameKey: "presetNormal",
    tab: "flipper",
    params: {
      ...defaultScanPresetParams,
      min_margin: 5,
      buy_radius: 10,
      sell_radius: 10,
    },
  },
  {
    id: "flip-aggressive",
    nameKey: "presetAggressive",
    tab: "flipper",
    params: {
      ...defaultScanPresetParams,
      min_margin: 2,
      buy_radius: 20,
      sell_radius: 20,
      min_route_security: 0,
      include_structures: true,
    },
  },

  // ── Regional Arbitrage ──
  {
    id: "region-safe",
    nameKey: "presetRegionSafe",
    tab: "region",
    params: {
      ...defaultScanPresetParams,
      min_margin: 18,
      min_item_profit: 10_000_000,
      min_period_roi: 6,
      min_demand_per_day: 2,
      max_dos: 45,
      avg_price_period: 14,
      shipping_cost_per_m3_jump: 0,
      buy_radius: 5,
      cargo_capacity: 12_000,
      min_route_security: 0.45,
      source_regions: ["The Forge", "Domain", "Sinq Laison", "Metropolis", "Heimatar"],
      target_region: "",
    },
  },
  {
    id: "region-normal",
    nameKey: "presetRegionNormal",
    tab: "region",
    params: {
      ...defaultScanPresetParams,
      min_margin: 12,
      min_item_profit: 4_000_000,
      min_period_roi: 4,
      min_demand_per_day: 1,
      max_dos: 75,
      avg_price_period: 14,
      shipping_cost_per_m3_jump: 0,
      buy_radius: 8,
      cargo_capacity: 20_000,
      min_route_security: 0.45,
      source_regions: ["The Forge", "Domain", "Sinq Laison", "Metropolis", "Heimatar"],
      target_region: "",
    },
  },
  {
    id: "region-eg-like",
    nameKey: "presetRegionEGLike",
    tab: "region",
    params: {
      ...defaultScanPresetParams,
      cargo_capacity: 0,
      min_margin: 15,
      min_item_profit: 15_000_000,
      min_period_roi: 0,
      min_demand_per_day: 3,
      purchase_demand_days: 0.5,
      max_dos: 0,
      avg_price_period: 30,
      shipping_cost_per_m3_jump: 0,
      min_route_security: 0.45,
      source_regions: ["The Forge", "Domain", "Sinq Laison", "Metropolis", "Heimatar"],
      target_market_system: "Jita",
      target_market_location_id: 60003760,
      sell_order_mode: true,
    },
  },
  {
    id: "region-wide",
    nameKey: "presetRegionWide",
    tab: "region",
    params: {
      ...defaultScanPresetParams,
      min_margin: 8,
      min_item_profit: 1_500_000,
      min_period_roi: 2,
      min_demand_per_day: 0.5,
      max_dos: 120,
      avg_price_period: 14,
      shipping_cost_per_m3_jump: 0,
      buy_radius: 20,
      cargo_capacity: 60_000,
      min_route_security: 0,
      source_regions: ["The Forge", "Domain", "Sinq Laison", "Metropolis", "Heimatar"],
      target_region: "",
    },
  },
  {
    id: "region-quick",
    nameKey: "presetRegionQuick",
    tab: "region",
    params: {
      ...defaultScanPresetParams,
      min_margin: 20,
      min_item_profit: 15_000_000,
      min_period_roi: 5,
      min_demand_per_day: 2,
      max_dos: 90,
      avg_price_period: 14,
      shipping_cost_per_m3_jump: 0,
      buy_radius: 5,
      cargo_capacity: 5000,
      min_route_security: 0.45,
      source_regions: ["The Forge", "Domain", "Sinq Laison", "Metropolis", "Heimatar"],
      target_region: "",
    },
  },
  {
    id: "region-deep",
    nameKey: "presetRegionDeep",
    tab: "region",
    params: {
      ...defaultScanPresetParams,
      min_margin: 10,
      min_item_profit: 5_000_000,
      min_period_roi: 3,
      min_demand_per_day: 1,
      max_dos: 120,
      avg_price_period: 14,
      shipping_cost_per_m3_jump: 0,
      buy_radius: 20,
      cargo_capacity: 60000,
      sell_radius: 20,
      min_route_security: 0,
      source_regions: ["The Forge", "Domain", "Sinq Laison", "Metropolis", "Heimatar"],
      target_region: "",
    },
  },

  // ── Contracts ──
  {
    id: "contract-safe",
    nameKey: "presetContractSafe",
    tab: "contracts",
    params: {
      ...defaultScanPresetParams,
      min_contract_price: 50_000_000,
      max_contract_margin: 60,
      min_priced_ratio: 0.95,
      require_history: true,
      contract_instant_liquidation: true,
      contract_hold_days: 7,
      contract_target_confidence: 90,
      exclude_rigs_with_ship: true,
      min_margin: 12,
      buy_radius: 15,
      sell_radius: 10,
      min_route_security: 0.45,
    },
  },
  {
    id: "contract-normal",
    nameKey: "presetContractNormal",
    tab: "contracts",
    params: {
      ...defaultScanPresetParams,
      min_contract_price: 10_000_000,
      max_contract_margin: 100,
      min_priced_ratio: 0.8,
      require_history: false,
      contract_instant_liquidation: false,
      contract_hold_days: 7,
      contract_target_confidence: 80,
      exclude_rigs_with_ship: true,
      min_margin: 5,
      buy_radius: 10,
      sell_radius: 10,
    },
  },
  {
    id: "contract-risky",
    nameKey: "presetContractRisky",
    tab: "contracts",
    params: {
      ...defaultScanPresetParams,
      min_contract_price: 1_000_000,
      max_contract_margin: 200,
      min_priced_ratio: 0.7,
      require_history: false,
      contract_instant_liquidation: false,
      contract_hold_days: 10,
      contract_target_confidence: 70,
      exclude_rigs_with_ship: true,
      min_margin: 3,
      buy_radius: 20,
      sell_radius: 20,
    },
  },

  // ── Route ──
  {
    id: "route-highsec",
    nameKey: "presetRouteHighsec",
    tab: "route",
    params: {
      ...defaultScanPresetParams,
      min_route_security: 0.45,
      min_margin: 5,
      cargo_capacity: 5000,
      route_min_hops: 2,
      route_max_hops: 5,
    },
  },
  {
    id: "route-allspace",
    nameKey: "presetRouteAllSpace",
    tab: "route",
    params: {
      ...defaultScanPresetParams,
      min_route_security: 0,
      min_margin: 2,
      cargo_capacity: 60000,
      route_min_hops: 2,
      route_max_hops: 7,
    },
  },
];

// ── Station Trading ──

export const STATION_BUILTIN_PRESETS: BuiltinPreset[] = [
  {
    id: "st-conservative",
    nameKey: "presetStConservative",
    tab: "station",
    params: {
      selectedStationId: 0,
      includeStructures: false,
      minMargin: 10,
      brokerFee: 3,
      salesTaxPercent: 8,
      splitTradeFees: false,
      buyBrokerFeePercent: 3,
      sellBrokerFeePercent: 3,
      buySalesTaxPercent: 0,
      sellSalesTaxPercent: 8,
      ctsProfile: "defensive",
      radius: 0,
      minDailyVolume: 10,
      minItemProfit: 500_000,
      minDemandPerDay: 5,
      minBfSPerDay: 0,
      avgPricePeriod: 90,
      minPeriodROI: 5,
      bvsRatioMin: 0,
      bvsRatioMax: 0,
      maxPVI: 30,
      maxSDS: 30,
      limitBuyToPriceLow: false,
      flagExtremePrices: true,
    } satisfies Partial<StationTradingSettings>,
  },
  {
    id: "st-normal",
    nameKey: "presetStNormal",
    tab: "station",
    params: {
      selectedStationId: 0,
      includeStructures: false,
      minMargin: 5,
      brokerFee: 3,
      salesTaxPercent: 8,
      splitTradeFees: false,
      buyBrokerFeePercent: 3,
      sellBrokerFeePercent: 3,
      buySalesTaxPercent: 0,
      sellSalesTaxPercent: 8,
      ctsProfile: "balanced",
      radius: 0,
      minDailyVolume: 5,
      minItemProfit: 0,
      minDemandPerDay: 1,
      minBfSPerDay: 0,
      avgPricePeriod: 90,
      minPeriodROI: 0,
      bvsRatioMin: 0,
      bvsRatioMax: 0,
      maxPVI: 0,
      maxSDS: 50,
      limitBuyToPriceLow: false,
      flagExtremePrices: true,
    } satisfies Partial<StationTradingSettings>,
  },
  {
    id: "st-aggressive",
    nameKey: "presetStAggressive",
    tab: "station",
    params: {
      selectedStationId: 0,
      includeStructures: false,
      minMargin: 2,
      brokerFee: 3,
      salesTaxPercent: 8,
      splitTradeFees: false,
      buyBrokerFeePercent: 3,
      sellBrokerFeePercent: 3,
      buySalesTaxPercent: 0,
      sellSalesTaxPercent: 8,
      ctsProfile: "aggressive",
      radius: 0,
      minDailyVolume: 0,
      minItemProfit: 0,
      minDemandPerDay: 0,
      minBfSPerDay: 0,
      avgPricePeriod: 30,
      minPeriodROI: 0,
      bvsRatioMin: 0,
      bvsRatioMax: 0,
      maxPVI: 0,
      maxSDS: 100,
      limitBuyToPriceLow: false,
      flagExtremePrices: false,
    } satisfies Partial<StationTradingSettings>,
  },
];

// ── Tab mapping ──

const TAB_MAP: Record<string, PresetTab> = {
  radius: "flipper",
  region: "region",
  contracts: "contracts",
  route: "route",
  station: "station",
  industry: "industry",
  demand: "demand",
  plex: "plex",
};

export function mapTabToPresetTab(tab: string): PresetTab {
  return TAB_MAP[tab] || "flipper";
}

export function getPresetsForTab(tab: string): BuiltinPreset[] {
  return BUILTIN_PRESETS.filter((p) => p.tab === mapTabToPresetTab(tab));
}

// ── Storage helpers ──

function loadAllCustomPresets(): SavedPreset[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];

    let changed = false;
    const normalized: SavedPreset[] = [];

    for (const item of parsed) {
      if (!item || typeof item !== "object") {
        changed = true;
        continue;
      }
      const id = typeof item.id === "string" ? item.id.trim() : "";
      const name = typeof item.name === "string" ? item.name.trim() : "";
      const params =
        item.params && typeof item.params === "object" && !Array.isArray(item.params)
          ? sanitizePresetParams(item.params as Record<string, any>)
          : null;
      if (!id || !name || !params) {
        changed = true;
        continue;
      }

      let tab: PresetTab = "flipper";
      if (isPresetTab(item.tab)) {
        tab = item.tab;
      } else {
        changed = true;
      }

      const createdAt =
        typeof item.createdAt === "number" && Number.isFinite(item.createdAt)
          ? item.createdAt
          : undefined;

      normalized.push({ id, name, tab, params, createdAt });
    }

    if (changed) {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(normalized));
    }
    return normalized;
  } catch {
    return [];
  }
}

export function loadCustomPresets(tab?: string): SavedPreset[] {
  const all = loadAllCustomPresets();
  if (!tab) return all;
  const presetTab = mapTabToPresetTab(tab);
  return all.filter((p) => p.tab === presetTab);
}

export function saveCustomPreset(preset: SavedPreset): void {
  const list = loadAllCustomPresets();
  const idx = list.findIndex((p) => p.id === preset.id);
  if (idx >= 0) list[idx] = preset;
  else list.push(preset);
  localStorage.setItem(STORAGE_KEY, JSON.stringify(list));
}

export function deleteCustomPreset(id: string): void {
  const list = loadAllCustomPresets().filter((p) => p.id !== id);
  localStorage.setItem(STORAGE_KEY, JSON.stringify(list));
}

export function applyPreset<T>(current: T, presetParams: Partial<T>): T {
  return { ...current, ...presetParams };
}

export function nextPresetId(): string {
  return `custom-${Date.now()}`;
}

// ── Export / Import ──

export function exportPresets(): string {
  return JSON.stringify(loadAllCustomPresets(), null, 2);
}

export function importPresets(
  json: string,
): { imported: number; error?: string } {
  try {
    const parsed = JSON.parse(json);
    if (!Array.isArray(parsed)) {
      return { imported: 0, error: "Invalid format: expected array" };
    }
    const existing = loadAllCustomPresets();
    const existingIds = new Set(existing.map((p) => p.id));
    let imported = 0;
    for (const item of parsed) {
      if (!item || typeof item !== "object") continue;
      const id = typeof item.id === "string" ? item.id.trim() : "";
      const name = typeof item.name === "string" ? item.name.trim() : "";
      const params =
        item.params && typeof item.params === "object" && !Array.isArray(item.params)
          ? sanitizePresetParams(item.params as Record<string, any>)
          : null;
      if (!id || !name || !params) continue;

      const tab: PresetTab = isPresetTab(item.tab) ? item.tab : "flipper";
      const createdAt =
        typeof item.createdAt === "number" && Number.isFinite(item.createdAt)
          ? item.createdAt
          : undefined;
      const normalized: SavedPreset = { id, name, tab, params, createdAt };

      if (existingIds.has(id)) {
        const idx = existing.findIndex((p) => p.id === id);
        if (idx >= 0) existing[idx] = normalized;
      } else {
        existing.push(normalized);
        existingIds.add(id);
      }
      imported++;
    }
    localStorage.setItem(STORAGE_KEY, JSON.stringify(existing));
    return { imported };
  } catch {
    return { imported: 0, error: "Invalid JSON" };
  }
}
