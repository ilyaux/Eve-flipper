const STORAGE_KEY = "eve-flipper-presets";

export type PresetTab = "flipper" | "region" | "contracts" | "route" | "station";

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

export const BUILTIN_PRESETS: BuiltinPreset[] = [
  // ── Flipper ──
  {
    id: "flip-conservative",
    nameKey: "presetConservative",
    tab: "flipper",
    params: {
      min_margin: 15,
      buy_radius: 5,
      sell_radius: 5,
      min_daily_volume: 10,
      sales_tax_percent: 8,
      broker_fee_percent: 3,
      split_trade_fees: false,
    },
  },
  {
    id: "flip-normal",
    nameKey: "presetNormal",
    tab: "flipper",
    params: {
      min_margin: 5,
      buy_radius: 10,
      sell_radius: 10,
      min_daily_volume: 0,
      sales_tax_percent: 8,
      broker_fee_percent: 3,
      split_trade_fees: false,
    },
  },
  {
    id: "flip-aggressive",
    nameKey: "presetAggressive",
    tab: "flipper",
    params: {
      min_margin: 2,
      buy_radius: 20,
      sell_radius: 20,
      min_daily_volume: 0,
      sales_tax_percent: 8,
      broker_fee_percent: 3,
      split_trade_fees: false,
    },
  },

  // ── Regional Arbitrage ──
  {
    id: "region-quick",
    nameKey: "presetRegionQuick",
    tab: "region",
    params: {
      min_margin: 10,
      buy_radius: 5,
      cargo_capacity: 5000,
      sales_tax_percent: 8,
      broker_fee_percent: 3,
      split_trade_fees: false,
    },
  },
  {
    id: "region-deep",
    nameKey: "presetRegionDeep",
    tab: "region",
    params: {
      min_margin: 3,
      buy_radius: 20,
      cargo_capacity: 60000,
      sales_tax_percent: 8,
      broker_fee_percent: 3,
      split_trade_fees: false,
    },
  },

  // ── Contracts ──
  {
    id: "contract-safe",
    nameKey: "presetContractSafe",
    tab: "contracts",
    params: {
      min_contract_price: 50_000_000,
      max_contract_margin: 60,
      min_priced_ratio: 0.95,
      require_history: true,
    },
  },
  {
    id: "contract-normal",
    nameKey: "presetContractNormal",
    tab: "contracts",
    params: {
      min_contract_price: 10_000_000,
      max_contract_margin: 100,
      min_priced_ratio: 0.8,
      require_history: false,
    },
  },
  {
    id: "contract-risky",
    nameKey: "presetContractRisky",
    tab: "contracts",
    params: {
      min_contract_price: 1_000_000,
      max_contract_margin: 200,
      min_priced_ratio: 0.7,
      require_history: false,
    },
  },

  // ── Route ──
  {
    id: "route-highsec",
    nameKey: "presetRouteHighsec",
    tab: "route",
    params: {
      min_route_security: 0.45,
      min_margin: 5,
      cargo_capacity: 5000,
    },
  },
  {
    id: "route-allspace",
    nameKey: "presetRouteAllSpace",
    tab: "route",
    params: {
      min_route_security: 0,
      min_margin: 2,
      cargo_capacity: 60000,
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
      minMargin: 10,
      brokerFee: 3,
      salesTaxPercent: 8,
      splitTradeFees: false,
      ctsProfile: "defensive",
      minDailyVolume: 10,
      minItemProfit: 500_000,
      minDemandPerDay: 5,
      avgPricePeriod: 90,
      minPeriodROI: 5,
      maxPVI: 30,
      maxSDS: 30,
      flagExtremePrices: true,
    } satisfies Partial<StationTradingSettings>,
  },
  {
    id: "st-normal",
    nameKey: "presetStNormal",
    tab: "station",
    params: {
      minMargin: 5,
      brokerFee: 3,
      salesTaxPercent: 8,
      splitTradeFees: false,
      ctsProfile: "balanced",
      minDailyVolume: 5,
      minItemProfit: 0,
      minDemandPerDay: 1,
      avgPricePeriod: 90,
      minPeriodROI: 0,
      maxPVI: 0,
      maxSDS: 50,
      flagExtremePrices: true,
    } satisfies Partial<StationTradingSettings>,
  },
  {
    id: "st-aggressive",
    nameKey: "presetStAggressive",
    tab: "station",
    params: {
      minMargin: 2,
      brokerFee: 3,
      salesTaxPercent: 8,
      splitTradeFees: false,
      ctsProfile: "aggressive",
      minDailyVolume: 0,
      minItemProfit: 0,
      minDemandPerDay: 0,
      avgPricePeriod: 30,
      minPeriodROI: 0,
      maxPVI: 0,
      maxSDS: 100,
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
    const parsed = JSON.parse(raw) as SavedPreset[];
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

export function loadCustomPresets(tab?: string): SavedPreset[] {
  const all = loadAllCustomPresets();
  if (!tab) return all;
  const presetTab = mapTabToPresetTab(tab);
  // Show presets for this tab + legacy presets (those saved before tab-awareness)
  return all.filter((p) => p.tab === presetTab || !p.tab);
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
      if (!item.id || !item.name || !item.params) continue;
      if (existingIds.has(item.id)) {
        const idx = existing.findIndex((p) => p.id === item.id);
        if (idx >= 0) existing[idx] = item;
      } else {
        existing.push(item);
      }
      imported++;
    }
    localStorage.setItem(STORAGE_KEY, JSON.stringify(existing));
    return { imported };
  } catch {
    return { imported: 0, error: "Invalid JSON" };
  }
}
