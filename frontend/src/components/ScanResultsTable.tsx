import {
  Fragment,
  memo,
  startTransition,
  useState,
  useMemo,
  useCallback,
  useEffect,
  useRef,
} from "react";
import type { FlipResult, StationCacheMeta, WatchlistItem } from "@/lib/types";
import { formatISK, formatMargin } from "@/lib/format";
import { useI18n, type TranslationKey } from "@/lib/i18n";
import {
  addToWatchlist,
  clearStationTradeStates,
  deleteStationTradeStates,
  getStationTradeStates,
  getWatchlist,
  openMarketInGame,
  rebootStationCache,
  removeFromWatchlist,
  setStationTradeState,
  setWaypointInGame,
} from "@/lib/api";
import { useGlobalToast } from "./Toast";
import { EmptyState, type EmptyReason } from "./EmptyState";
import { ExecutionPlannerPopup } from "./ExecutionPlannerPopup";
import { handleEveUIError } from "@/lib/handleEveUIError";
import { BatchBuilderPopup } from "./BatchBuilderPopup";

const PAGE_SIZE = 100;
const GROUP_PAGE_SIZE = 50; // rows shown per group before "Show all" button
const CACHE_TTL_FALLBACK_MS = 20 * 60 * 1000;
const COLUMN_PREFS_STORAGE_PREFIX = "eve-scan-columns:v1:";
const ITEM_GROUPING_STORAGE_KEY = "eve-radius-group-by-item:v1";

type SortKey = keyof FlipResult;
type SortDir = "asc" | "desc";
type RegionGroupSortMode = "period_profit" | "now_profit" | "trade_score";
type HiddenMode = "done" | "ignored";
type HiddenFilterTab = "all" | "done" | "ignored";

type HiddenFlipEntry = {
  key: string;
  mode: HiddenMode;
  updatedAt: string;
  typeName: string;
  buyStation: string;
  sellStation: string;
  stateTypeID: number;
  stateStationID: number;
  stateRegionID: number;
};

type CacheMetaView = {
  currentRevision: number;
  lastRefreshAt: number;
  nextExpiryAt: number;
  scopeLabel: string;
  regionCount: number;
};

interface Props {
  results: FlipResult[];
  scanning: boolean;
  progress: string;
  cacheMeta?: StationCacheMeta | null;
  tradeStateTab?: "radius" | "region";
  scanCompletedWithZero?: boolean;
  salesTaxPercent?: number;
  brokerFeePercent?: number;
  splitTradeFees?: boolean;
  buyBrokerFeePercent?: number;
  sellBrokerFeePercent?: number;
  buySalesTaxPercent?: number;
  sellSalesTaxPercent?: number;
  showRegions?: boolean;
  columnProfile?: "default" | "region_eveguru";
  isLoggedIn?: boolean;
  cargoLimit?: number;
}

type ColumnDef = {
  key: SortKey;
  labelKey: TranslationKey;
  width: string;
  numeric: boolean;
  tooltipKey?: TranslationKey;
};

/* ─── Column definitions ─── */

const baseColumnDefs: ColumnDef[] = [
  {
    key: "TypeName",
    labelKey: "colItem",
    width: "min-w-[180px]",
    numeric: false,
  },
  {
    key: "BuyPrice",
    labelKey: "colBuyPrice",
    width: "min-w-[120px]",
    numeric: true,
  },
  {
    key: "BestAskQty",
    labelKey: "colBestAskQty",
    width: "min-w-[90px]",
    numeric: true,
  },
  {
    key: "ExpectedBuyPrice",
    labelKey: "colExpectedBuyPrice",
    width: "min-w-[120px]",
    numeric: true,
  },
  {
    key: "BuyStation",
    labelKey: "colBuyStation",
    width: "min-w-[150px]",
    numeric: false,
  },
  {
    key: "SellPrice",
    labelKey: "colSellPrice",
    width: "min-w-[120px]",
    numeric: true,
  },
  {
    key: "BestBidQty",
    labelKey: "colBestBidQty",
    width: "min-w-[90px]",
    numeric: true,
  },
  {
    key: "ExpectedSellPrice",
    labelKey: "colExpectedSellPrice",
    width: "min-w-[120px]",
    numeric: true,
  },
  {
    key: "SellStation",
    labelKey: "colSellStation",
    width: "min-w-[150px]",
    numeric: false,
  },
  {
    key: "MarginPercent",
    labelKey: "colMargin",
    width: "min-w-[80px]",
    numeric: true,
  },
  {
    key: "IskPerM3",
    labelKey: "colIskPerM3",
    width: "min-w-[90px]",
    numeric: true,
  },
  {
    key: "UnitsToBuy",
    labelKey: "colUnitsToBuy",
    width: "min-w-[80px]",
    numeric: true,
  },
  {
    key: "FilledQty",
    labelKey: "colFilledQty",
    width: "min-w-[80px]",
    numeric: true,
  },
  {
    key: "CanFill",
    labelKey: "colCanFill",
    width: "min-w-[70px]",
    numeric: false,
  },
  {
    key: "BuyOrderRemain",
    labelKey: "colAcceptQty",
    width: "min-w-[80px]",
    numeric: true,
  },
  {
    key: "RealProfit",
    labelKey: "colRealProfit",
    width: "min-w-[120px]",
    numeric: true,
  },
  {
    key: "TotalProfit",
    labelKey: "colProfit",
    width: "min-w-[120px]",
    numeric: true,
  },
  {
    key: "ExpectedProfit",
    labelKey: "colExpectedProfit",
    width: "min-w-[100px]",
    numeric: true,
  },
  {
    key: "ProfitPerJump",
    labelKey: "colProfitPerJump",
    width: "min-w-[110px]",
    numeric: true,
  },
  {
    key: "TotalJumps",
    labelKey: "colJumps",
    width: "min-w-[60px]",
    numeric: true,
  },
  {
    key: "DailyVolume",
    labelKey: "colDailyVolume",
    width: "min-w-[80px]",
    numeric: true,
  },
  {
    key: "S2BPerDay",
    labelKey: "colS2BPerDay",
    width: "min-w-[90px]",
    numeric: true,
    tooltipKey: "colS2BPerDayHint",
  },
  {
    key: "BfSPerDay",
    labelKey: "colBfSPerDay",
    width: "min-w-[90px]",
    numeric: true,
    tooltipKey: "colBfSPerDayHint",
  },
  {
    key: "S2BBfSRatio",
    labelKey: "colS2BBfSRatio",
    width: "min-w-[90px]",
    numeric: true,
    tooltipKey: "colS2BBfSRatioHint",
  },
  {
    key: "DailyProfit",
    labelKey: "colDailyProfit",
    width: "min-w-[110px]",
    numeric: true,
  },
  {
    key: "PriceTrend",
    labelKey: "colPriceTrend",
    width: "min-w-[70px]",
    numeric: true,
  },
  {
    key: "BuyCompetitors",
    labelKey: "colBuyCompetitors",
    width: "min-w-[70px]",
    numeric: true,
  },
  {
    key: "SellCompetitors",
    labelKey: "colSellCompetitors",
    width: "min-w-[70px]",
    numeric: true,
  },
];

const regionColumnDefs: ColumnDef[] = [
  {
    key: "BuyRegionName" as SortKey,
    labelKey: "colBuyRegion" as TranslationKey,
    width: "min-w-[120px]",
    numeric: false,
  },
  {
    key: "SellRegionName" as SortKey,
    labelKey: "colSellRegion" as TranslationKey,
    width: "min-w-[120px]",
    numeric: false,
  },
];

const regionEveGuruColumnDefs: ColumnDef[] = [
  {
    key: "DaySecurity",
    labelKey: "colSecurity",
    width: "min-w-[80px]",
    numeric: true,
    tooltipKey: "colSecurityHint",
  },
  {
    key: "TypeName",
    labelKey: "colItem",
    width: "min-w-[220px]",
    numeric: false,
  },
  {
    key: "BuyStation",
    labelKey: "colBuyStation",
    width: "min-w-[220px]",
    numeric: false,
  },
  {
    key: "SellStation",
    labelKey: "colSellStation",
    width: "min-w-[220px]",
    numeric: false,
  },
  {
    key: "UnitsToBuy",
    labelKey: "colPurchaseUnits",
    width: "min-w-[110px]",
    numeric: true,
    tooltipKey: "colPurchaseUnitsHint",
  },
  {
    key: "DaySourceUnits",
    labelKey: "colSourceUnits",
    width: "min-w-[110px]",
    numeric: true,
    tooltipKey: "colSourceUnitsHint",
  },
  {
    key: "DayTargetDemandPerDay",
    labelKey: "colTargetDemandDay",
    width: "min-w-[120px]",
    numeric: true,
    tooltipKey: "colTargetDemandDayHint",
  },
  {
    key: "DayTargetSupplyUnits",
    labelKey: "colTargetSupplyUnits",
    width: "min-w-[120px]",
    numeric: true,
    tooltipKey: "colTargetSupplyUnitsHint",
  },
  {
    key: "DayTargetDOS",
    labelKey: "colTargetDOS",
    width: "min-w-[90px]",
    numeric: true,
    tooltipKey: "metricDOSDesc",
  },
  {
    key: "DayAssets",
    labelKey: "colAssets",
    width: "min-w-[90px]",
    numeric: true,
    tooltipKey: "colAssetsHint",
  },
  {
    key: "DayActiveOrders",
    labelKey: "colActiveOrders",
    width: "min-w-[110px]",
    numeric: true,
    tooltipKey: "colActiveOrdersHint",
  },
  {
    key: "DaySourceAvgPrice",
    labelKey: "colSourceAvgPrice",
    width: "min-w-[120px]",
    numeric: true,
    tooltipKey: "colSourceAvgPriceHint",
  },
  {
    key: "DayTargetNowPrice",
    labelKey: "colTargetNowPrice",
    width: "min-w-[120px]",
    numeric: true,
    tooltipKey: "colTargetNowPriceHint",
  },
  {
    key: "DayTargetPeriodPrice",
    labelKey: "colTargetPeriodPrice",
    width: "min-w-[130px]",
    numeric: true,
    tooltipKey: "colTargetPeriodPriceHint",
  },
  {
    key: "DayNowProfit",
    labelKey: "colNowProfit",
    width: "min-w-[120px]",
    numeric: true,
    tooltipKey: "colNowProfitHint",
  },
  {
    key: "DayPeriodProfit",
    labelKey: "colPeriodProfit",
    width: "min-w-[130px]",
    numeric: true,
    tooltipKey: "colPeriodProfitHint",
  },
  {
    key: "DayROINow",
    labelKey: "colROINow",
    width: "min-w-[90px]",
    numeric: true,
    tooltipKey: "metricNowROIDesc",
  },
  {
    key: "DayROIPeriod",
    labelKey: "colROIPeriod",
    width: "min-w-[100px]",
    numeric: true,
    tooltipKey: "metricPeriodROIDesc",
  },
  {
    key: "DayCapitalRequired",
    labelKey: "colCapitalRequired",
    width: "min-w-[120px]",
    numeric: true,
    tooltipKey: "colCapitalRequiredHint",
  },
  {
    key: "DayShippingCost",
    labelKey: "colShippingCost",
    width: "min-w-[120px]",
    numeric: true,
    tooltipKey: "colShippingCostHint",
  },
  {
    key: "DayIskPerM3Jump",
    labelKey: "colIskPerM3Jump",
    width: "min-w-[120px]",
    numeric: true,
    tooltipKey: "colIskPerM3JumpHint",
  },
  {
    key: "DayTradeScore",
    labelKey: "colTradeScore",
    width: "min-w-[90px]",
    numeric: true,
    tooltipKey: "colTradeScoreHint",
  },
];

function buildColumnDefs(
  showRegions: boolean,
  columnProfile: "default" | "region_eveguru",
): ColumnDef[] {
  if (columnProfile === "region_eveguru") {
    return regionEveGuruColumnDefs;
  }
  if (!showRegions) return baseColumnDefs;
  const cols = [...baseColumnDefs];
  const sellIdx = cols.findIndex((c) => c.key === "SellStation");
  if (sellIdx >= 0) cols.splice(sellIdx + 1, 0, regionColumnDefs[1]);
  const buyIdx = cols.findIndex((c) => c.key === "BuyStation");
  if (buyIdx >= 0) cols.splice(buyIdx + 1, 0, regionColumnDefs[0]);
  return cols;
}

function formatCountdown(totalSec: number): string {
  const sec = Math.max(0, Math.floor(totalSec));
  const mm = Math.floor(sec / 60)
    .toString()
    .padStart(2, "0");
  const ss = (sec % 60).toString().padStart(2, "0");
  return `${mm}:${ss}`;
}

function mapServerCacheMeta(
  meta: StationCacheMeta | null | undefined,
  fallbackScope: string,
  fallbackRegionCount: number,
  fallbackBaseTs: number,
): CacheMetaView {
  if (!meta) {
    return {
      currentRevision: Math.floor(fallbackBaseTs / 1000),
      lastRefreshAt: fallbackBaseTs,
      nextExpiryAt: fallbackBaseTs + CACHE_TTL_FALLBACK_MS,
      scopeLabel: fallbackScope,
      regionCount: fallbackRegionCount,
    };
  }
  const lastRefreshTs = meta.last_refresh_at
    ? Date.parse(meta.last_refresh_at)
    : fallbackBaseTs;
  const nextExpiryTs = meta.next_expiry_at
    ? Date.parse(meta.next_expiry_at)
    : fallbackBaseTs + Math.max(60, meta.min_ttl_sec || 60) * 1000;
  return {
    currentRevision:
      meta.current_revision && Number.isFinite(meta.current_revision)
        ? meta.current_revision
        : Math.floor(nextExpiryTs / 1000),
    lastRefreshAt: Number.isFinite(lastRefreshTs) ? lastRefreshTs : fallbackBaseTs,
    nextExpiryAt: Number.isFinite(nextExpiryTs)
      ? nextExpiryTs
      : fallbackBaseTs + CACHE_TTL_FALLBACK_MS,
    scopeLabel: fallbackScope,
    regionCount: Math.max(1, fallbackRegionCount),
  };
}

function hash53(input: string): number {
  let h1 = 0xdeadbeef ^ input.length;
  let h2 = 0x41c6ce57 ^ input.length;
  for (let i = 0; i < input.length; i++) {
    const ch = input.charCodeAt(i);
    h1 = Math.imul(h1 ^ ch, 2654435761);
    h2 = Math.imul(h2 ^ ch, 1597334677);
  }
  h1 = Math.imul(h1 ^ (h1 >>> 16), 2246822507) ^ Math.imul(h2 ^ (h2 >>> 13), 3266489909);
  h2 = Math.imul(h2 ^ (h2 >>> 16), 2246822507) ^ Math.imul(h1 ^ (h1 >>> 13), 3266489909);
  return 4294967296 * (2097151 & h2) + (h1 >>> 0);
}

function flipStateKey(row: FlipResult): string {
  return [
    row.TypeID ?? 0,
    row.BuyLocationID ?? 0,
    row.SellLocationID ?? 0,
    row.BuySystemID ?? 0,
    row.SellSystemID ?? 0,
  ].join(":");
}

function flipStateIDs(row: FlipResult): {
  typeID: number;
  stationID: number;
  regionID: number;
} {
  const typeID =
    row.TypeID > 0 && row.TypeID < 2_147_483_647
      ? row.TypeID
      : (hash53(flipStateKey(row)) % 2_147_483_000) + 1;
  const stationID = hash53(`flip:${flipStateKey(row)}`) || 1;
  const regionID = row.BuyRegionID || row.SellRegionID || 0;
  return { typeID, stationID, regionID };
}

function tradeStateIndexKey(typeID: number, stationID: number, regionID: number): string {
  return `${typeID}:${stationID}:${regionID}`;
}

/* ─── Row identity ───
 * Stable per-row object id to avoid duplicate keys when data has collisions.
 */
let _nextRowId = 1;
const _rowIdMap = new WeakMap<FlipResult, number>();
function getRowId(row: FlipResult): number {
  let id = _rowIdMap.get(row);
  if (id == null) {
    id = _nextRowId++;
    _rowIdMap.set(row, id);
  }
  return id;
}

/* ─── IndexedRow: carries stable identity for rows ─── */
interface IndexedRow {
  id: number; // stable id from WeakMap
  row: FlipResult;
}

interface RegionGroup {
  key: string;
  label: string;
  rows: IndexedRow[];
  sortValue: number;
  metricCount: number;
}

interface ItemGroup {
  key: string;
  label: string;
  rows: IndexedRow[];
}

/* ─── Filter helpers ─── */

function passesNumericFilter(num: number, fval: string): boolean {
  const trimmed = fval.trim();
  if (!trimmed) return true;
  // Range: "100-500"
  if (trimmed.includes("-") && !trimmed.startsWith("-")) {
    const [minS, maxS] = trimmed.split("-");
    const mn = parseFloat(minS);
    const mx = parseFloat(maxS);
    if (!isNaN(mn) && !isNaN(mx) && (num < mn || num > mx)) return false;
    return true;
  }
  if (trimmed.startsWith(">=")) {
    const v = parseFloat(trimmed.slice(2));
    return isNaN(v) || num >= v;
  }
  if (trimmed.startsWith(">")) {
    const v = parseFloat(trimmed.slice(1));
    return isNaN(v) || num > v;
  }
  if (trimmed.startsWith("<=")) {
    const v = parseFloat(trimmed.slice(2));
    return isNaN(v) || num <= v;
  }
  if (trimmed.startsWith("<")) {
    const v = parseFloat(trimmed.slice(1));
    return isNaN(v) || num < v;
  }
  if (trimmed.startsWith("=")) {
    const v = parseFloat(trimmed.slice(1));
    return isNaN(v) || num === v;
  }
  // Plain number: >= threshold
  const mn = parseFloat(trimmed);
  return isNaN(mn) || num >= mn;
}

function passesTextFilter(val: unknown, fval: string): boolean {
  return String(val ?? "")
    .toLowerCase()
    .includes(fval.toLowerCase());
}

function rowProfitPerUnit(row: FlipResult): number {
  if (row.RealProfit != null && row.FilledQty != null && row.FilledQty > 0) {
    const realPerUnit = row.RealProfit / row.FilledQty;
    if (Number.isFinite(realPerUnit)) return realPerUnit;
  }
  const fallback = row.ProfitPerUnit;
  return Number.isFinite(fallback) ? fallback : 0;
}

function rowIskPerM3(row: FlipResult): number {
  const volume = Number(row.Volume);
  if (!Number.isFinite(volume) || volume <= 0) return 0;
  return rowProfitPerUnit(row) / volume;
}

function rowS2BPerDay(row: FlipResult): number {
  if (row.S2BPerDay != null && Number.isFinite(row.S2BPerDay)) {
    return row.S2BPerDay;
  }
  const total = Number(row.DailyVolume);
  if (!Number.isFinite(total) || total <= 0) return 0;
  const buyDepth = Number(row.BuyOrderRemain);
  const sellDepth = Number(row.SellOrderRemain);
  if (buyDepth <= 0 && sellDepth <= 0) return total / 2;
  if (buyDepth <= 0) return 0;
  if (sellDepth <= 0) return total;
  return (total * buyDepth) / (buyDepth + sellDepth);
}

function rowBfSPerDay(row: FlipResult): number {
  if (row.BfSPerDay != null && Number.isFinite(row.BfSPerDay)) {
    return row.BfSPerDay;
  }
  const total = Number(row.DailyVolume);
  if (!Number.isFinite(total) || total <= 0) return 0;
  const s2b = rowS2BPerDay(row);
  const bfs = total - s2b;
  return bfs > 0 ? bfs : 0;
}

function rowS2BBfSRatio(row: FlipResult): number {
  if (row.S2BBfSRatio != null && Number.isFinite(row.S2BBfSRatio)) {
    return row.S2BBfSRatio;
  }
  const bfs = rowBfSPerDay(row);
  if (bfs <= 0) return 0;
  return rowS2BPerDay(row) / bfs;
}

function getCellValue(row: FlipResult, key: SortKey): unknown {
  if (key === "IskPerM3") {
    if (row.IskPerM3 != null && Number.isFinite(row.IskPerM3)) {
      return row.IskPerM3;
    }
    return rowIskPerM3(row);
  }
  if (key === "S2BPerDay") return rowS2BPerDay(row);
  if (key === "BfSPerDay") return rowBfSPerDay(row);
  if (key === "S2BBfSRatio") return rowS2BBfSRatio(row);
  return row[key];
}

function passesFilter(row: FlipResult, col: ColumnDef, fval: string): boolean {
  if (!fval) return true;
  const cellVal = getCellValue(row, col.key);
  return col.numeric
    ? passesNumericFilter(cellVal as number, fval)
    : passesTextFilter(cellVal, fval);
}

/* ─── Cell formatting ─── */

function fmtCell(col: ColumnDef, row: FlipResult): string {
  const val = getCellValue(row, col.key);
  if (
    col.key === "ExpectedProfit" ||
    col.key === "RealProfit" ||
    col.key === "ExpectedBuyPrice" ||
    col.key === "ExpectedSellPrice"
  ) {
    if (val == null || Number.isNaN(val)) return "\u2014";
    if (Number(val) <= 0) return "\u2014";
    return formatISK(val as number);
  }
  if (col.key === "BestAskQty" || col.key === "BestBidQty") {
    if (val == null || Number(val) <= 0) return "\u2014";
    return Number(val).toLocaleString();
  }
  if (col.key === "CanFill") {
    if (val == null) return "\u2014";
    return val ? "✓" : "✕";
  }
  if (
    col.key === "BuyPrice" ||
    col.key === "SellPrice" ||
    col.key === "TotalProfit" ||
    col.key === "ProfitPerJump" ||
    col.key === "DailyProfit" ||
    col.key === "IskPerM3"
  ) {
    return formatISK(val as number);
  }
  if (
    col.key === "DaySourceAvgPrice" ||
    col.key === "DayTargetNowPrice" ||
    col.key === "DayTargetPeriodPrice" ||
    col.key === "DayNowProfit" ||
    col.key === "DayPeriodProfit" ||
    col.key === "DayCapitalRequired" ||
    col.key === "DayShippingCost"
  ) {
    return formatISK(Number(val ?? 0));
  }
  if (col.key === "DayROINow" || col.key === "DayROIPeriod") {
    return formatMargin(Number(val ?? 0));
  }
  if (col.key === "DaySecurity") {
    const sec = Number(val);
    return Number.isFinite(sec) ? sec.toFixed(1) : "\u2014";
  }
  if (col.key === "DayIskPerM3Jump") {
    const v = Number(val ?? 0);
    if (!v || !Number.isFinite(v)) return "—";
    return formatISK(v) + "/m³j";
  }
  if (col.key === "DayTradeScore") {
    const v = Number(val ?? 0);
    if (!Number.isFinite(v)) return "—";
    return v.toFixed(0);
  }
  if (col.key === "DayTargetDOS") {
    const dos = Number(val);
    return Number.isFinite(dos) ? dos.toFixed(2) : "\u2014";
  }
  if (col.key === "MarginPercent") return formatMargin(val as number);
  if (col.key === "S2BBfSRatio") {
    const ratio = Number(val);
    return Number.isFinite(ratio) ? ratio.toFixed(2) : "\u2014";
  }
  if (col.key === "PriceTrend") {
    const v = val as number;
    return (v >= 0 ? "+" : "") + v.toFixed(1) + "%";
  }
  if (typeof val === "number") return val.toLocaleString();
  return String(val ?? "");
}

/* ═══════════════════════════════════════════════════════════════════════
 * COMPONENT
 * ═══════════════════════════════════════════════════════════════════════ */

export function ScanResultsTable({
  results,
  scanning,
  progress,
  cacheMeta,
  tradeStateTab = "radius",
  scanCompletedWithZero,
  salesTaxPercent,
  brokerFeePercent,
  splitTradeFees,
  buyBrokerFeePercent,
  sellBrokerFeePercent,
  buySalesTaxPercent,
  sellSalesTaxPercent,
  showRegions = false,
  columnProfile = "default",
  isLoggedIn = false,
  cargoLimit = 0,
}: Props) {
  const { t } = useI18n();
  const emptyReason: EmptyReason = scanCompletedWithZero
    ? "no_results"
    : "no_scan_yet";
  const { addToast } = useGlobalToast();

  const allColumnDefs = useMemo(
    () => buildColumnDefs(showRegions, columnProfile),
    [showRegions, columnProfile],
  );
  const columnPrefsKey = useMemo(
    () =>
      `${COLUMN_PREFS_STORAGE_PREFIX}${showRegions ? "region" : "radius"}:${columnProfile}`,
    [showRegions, columnProfile],
  );

  // ── State ──
  const [sortKey, setSortKey] = useState<SortKey>(() =>
    columnProfile === "region_eveguru" ? "DayPeriodProfit" : "RealProfit",
  );
  const [sortDir, setSortDir] = useState<SortDir>("desc");
  const [filters, setFilters] = useState<Record<string, string>>({});
  const [showFilters, setShowFilters] = useState(false);
  const [showColumnPanel, setShowColumnPanel] = useState(false);
  const [columnOrder, setColumnOrder] = useState<SortKey[]>(() =>
    allColumnDefs.map((col) => col.key),
  );
  const [hiddenColumns, setHiddenColumns] = useState<Set<SortKey>>(new Set());
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set());
  const [pinnedIds, setPinnedIds] = useState<Set<number>>(new Set());
  const [page, setPage] = useState(0);
  const [compactMode, setCompactMode] = useState(false);
  const [groupByItem, setGroupByItem] = useState<boolean>(() => {
    try {
      return localStorage.getItem(ITEM_GROUPING_STORAGE_KEY) === "1";
    } catch {
      return false;
    }
  });
  const [showHiddenRows, setShowHiddenRows] = useState(false);
  const [hiddenMap, setHiddenMap] = useState<Record<string, HiddenFlipEntry>>({});
  const [ignoredModalOpen, setIgnoredModalOpen] = useState(false);
  const [ignoredSearch, setIgnoredSearch] = useState("");
  const [ignoredTab, setIgnoredTab] = useState<HiddenFilterTab>("all");
  const [ignoredSelectedKeys, setIgnoredSelectedKeys] = useState<Set<string>>(new Set());
  const [cacheNowTs, setCacheNowTs] = useState<number>(Date.now());
  const [lastScanTs, setLastScanTs] = useState<number>(Date.now());
  const [cacheStaleSuppressedUntilTs, setCacheStaleSuppressedUntilTs] =
    useState<number>(0);
  const [cacheRebooting, setCacheRebooting] = useState(false);
  const [collapsedRegionGroups, setCollapsedRegionGroups] = useState<Set<string>>(
    new Set(),
  );
  const [expandedItemGroups, setExpandedItemGroups] = useState<Set<string>>(
    new Set(),
  );
  const [regionCollapseInitialized, setRegionCollapseInitialized] = useState(false);

  const isRegionGrouped = columnProfile === "region_eveguru";
  const isItemGrouped = !isRegionGrouped && groupByItem;
  const preferredSortKey: SortKey = isRegionGrouped
    ? "DayPeriodProfit"
    : "RealProfit";

  // ── Category / security / group filter state (region_eveguru only) ──
  const [categoryFilter, setCategoryFilter] = useState<Set<number>>(new Set());
  const [securityFilter, setSecurityFilter] = useState<
    "all" | "highsec" | "lowsec" | "nullsec"
  >("all");
  const [groupFilter, setGroupFilter] = useState<Set<string>>(new Set());
  const [regionGroupSortMode, setRegionGroupSortMode] =
    useState<RegionGroupSortMode>(() => {
      try {
        const raw = localStorage.getItem("eve-region-group-sort:v1");
        if (
          raw === "period_profit" ||
          raw === "now_profit" ||
          raw === "trade_score"
        ) {
          return raw;
        }
      } catch {
        // ignore storage errors
      }
      return "period_profit";
    });

  const orderedColumnDefs = useMemo(() => {
    const byKey = new Map(allColumnDefs.map((col) => [col.key, col] as const));
    const ordered = columnOrder
      .map((key) => byKey.get(key))
      .filter((col): col is ColumnDef => !!col);
    for (const col of allColumnDefs) {
      if (!ordered.some((existing) => existing.key === col.key)) {
        ordered.push(col);
      }
    }
    return ordered;
  }, [allColumnDefs, columnOrder]);

  const columnDefs = useMemo(
    () => orderedColumnDefs.filter((col) => !hiddenColumns.has(col.key)),
    [orderedColumnDefs, hiddenColumns],
  );

  // Watchlist
  const [watchlist, setWatchlist] = useState<WatchlistItem[]>([]);
  useEffect(() => {
    getWatchlist()
      .then(setWatchlist)
      .catch(() => {});
  }, []);
  const watchlistIds = useMemo(
    () => new Set(watchlist.map((w) => w.type_id)),
    [watchlist],
  );

  // Context menu
  const [contextMenu, setContextMenu] = useState<{
    x: number;
    y: number;
    id: number;
    row: FlipResult;
  } | null>(null);
  const contextMenuRef = useRef<HTMLDivElement>(null);
  const [execPlanRow, setExecPlanRow] = useState<FlipResult | null>(null);
  const [batchPlanRow, setBatchPlanRow] = useState<FlipResult | null>(null);
  const [dayDetailRow, setDayDetailRow] = useState<FlipResult | null>(null);
  const [filterPanelOpen, setFilterPanelOpen] = useState(false);
  const [filterSearch, setFilterSearch] = useState("");
  const filterPanelRef = useRef<HTMLDivElement>(null);
  const filterBtnRef = useRef<HTMLButtonElement>(null);
  // Per-group row limit for region mode (key → max rows shown)
  const [groupRowLimit, setGroupRowLimit] = useState<Map<string, number>>(new Map());

  useEffect(() => {
    if (contextMenu && contextMenuRef.current) {
      const menu = contextMenuRef.current;
      const rect = menu.getBoundingClientRect();
      const pad = 10;
      let x = contextMenu.x,
        y = contextMenu.y;
      if (x + rect.width > window.innerWidth - pad)
        x = window.innerWidth - rect.width - pad;
      if (y + rect.height > window.innerHeight - pad)
        y = window.innerHeight - rect.height - pad;
      menu.style.left = `${Math.max(pad, x)}px`;
      menu.style.top = `${Math.max(pad, y)}px`;
    }
  }, [contextMenu]);

  // Close filter panel on outside click
  useEffect(() => {
    if (!filterPanelOpen) return;
    const handler = (e: MouseEvent) => {
      const t = e.target as Node;
      if (!filterPanelRef.current?.contains(t) && !filterBtnRef.current?.contains(t)) {
        setFilterPanelOpen(false);
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [filterPanelOpen]);

  useEffect(() => {
    const defaultOrder = allColumnDefs.map((col) => col.key);
    const available = new Set(defaultOrder);
    let nextOrder = defaultOrder;
    const nextHidden = new Set<SortKey>();
    try {
      const raw = localStorage.getItem(columnPrefsKey);
      if (raw) {
        const parsed = JSON.parse(raw) as {
          order?: string[];
          hidden?: string[];
        };
        if (Array.isArray(parsed.order)) {
          const saved = parsed.order
            .filter((k): k is SortKey => available.has(k as SortKey));
          const missing = defaultOrder.filter((k) => !saved.includes(k));
          nextOrder = [...saved, ...missing];
        }
        if (Array.isArray(parsed.hidden)) {
          for (const key of parsed.hidden) {
            if (available.has(key as SortKey)) {
              nextHidden.add(key as SortKey);
            }
          }
        }
      }
    } catch {
      // Ignore malformed local settings.
    }
    if (nextHidden.size >= nextOrder.length && nextOrder.length > 0) {
      nextHidden.delete(nextOrder[0]);
    }
    setColumnOrder(nextOrder);
    setHiddenColumns(nextHidden);
  }, [columnPrefsKey, allColumnDefs]);

  useEffect(() => {
    if (columnOrder.length === 0) return;
    try {
      localStorage.setItem(
        columnPrefsKey,
        JSON.stringify({
          order: columnOrder,
          hidden: [...hiddenColumns],
        }),
      );
    } catch {
      // Ignore storage quota errors.
    }
  }, [columnPrefsKey, columnOrder, hiddenColumns]);

  useEffect(() => {
    try {
      localStorage.setItem("eve-region-group-sort:v1", regionGroupSortMode);
    } catch {
      // ignore storage quota errors
    }
  }, [regionGroupSortMode]);

  useEffect(() => {
    try {
      localStorage.setItem(ITEM_GROUPING_STORAGE_KEY, groupByItem ? "1" : "0");
    } catch {
      // ignore storage quota errors
    }
  }, [groupByItem]);

  useEffect(() => {
    if (columnDefs.length === 0) return;
    if (!columnDefs.some((col) => col.key === sortKey)) {
      if (columnDefs.some((col) => col.key === preferredSortKey)) {
        setSortKey(preferredSortKey);
      } else {
        setSortKey(columnDefs[0].key);
      }
      setSortDir("desc");
    }
  }, [columnDefs, preferredSortKey, sortKey]);

  const toggleColumnVisibility = useCallback(
    (key: SortKey, visible: boolean) => {
      setHiddenColumns((prev) => {
        const next = new Set(prev);
        if (visible) {
          next.delete(key);
          return next;
        }
        if (!next.has(key) && columnDefs.length <= 1) {
          addToast(t("columnsAtLeastOne"), "info", 2200);
          return prev;
        }
        next.add(key);
        return next;
      });
    },
    [columnDefs.length, addToast, t],
  );

  const moveColumn = useCallback((key: SortKey, dir: -1 | 1) => {
    setColumnOrder((prev) => {
      const idx = prev.indexOf(key);
      if (idx < 0) return prev;
      const nextIdx = idx + dir;
      if (nextIdx < 0 || nextIdx >= prev.length) return prev;
      const next = [...prev];
      [next[idx], next[nextIdx]] = [next[nextIdx], next[idx]];
      return next;
    });
  }, []);

  const resetColumns = useCallback(() => {
    setColumnOrder(allColumnDefs.map((col) => col.key));
  }, [allColumnDefs]);

  // ── Data pipeline: index → filter → sort ──
  const { indexed, filtered, sorted, variantByRowId } = useMemo(() => {
    const indexed: IndexedRow[] = results.map((row) => ({
      id: getRowId(row),
      row,
    }));

    const hasFilters = Object.values(filters).some((v) => !!v);
    const filtered = hasFilters
      ? indexed.filter((ir) => {
          for (const col of columnDefs) {
            const fval = filters[col.key];
            if (!fval) continue;
            if (!passesFilter(ir.row, col, fval)) return false;
          }
          return true;
        })
      : indexed;

    const sorted = filtered.slice();
    sorted.sort((a, b) => {
      const aPin = pinnedIds.has(a.id);
      const bPin = pinnedIds.has(b.id);
      if (aPin !== bPin) return aPin ? -1 : 1;

      const av = getCellValue(a.row, sortKey);
      const bv = getCellValue(b.row, sortKey);
      if (typeof av === "number" || typeof bv === "number") {
        if (av == null && bv == null) return 0;
        if (av == null) return 1;
        if (bv == null) return -1;
        const diff = (av as number) - (bv as number);
        return sortDir === "asc" ? diff : -diff;
      }
      const cmp = String(av ?? "").localeCompare(String(bv ?? ""));
      return sortDir === "asc" ? cmp : -cmp;
    });

    const totalByType = new Map<number, number>();
    for (const ir of sorted) {
      totalByType.set(ir.row.TypeID, (totalByType.get(ir.row.TypeID) ?? 0) + 1);
    }
    const seenByType = new Map<number, number>();
    const variantByRowId = new Map<number, { index: number; total: number }>();
    for (const ir of sorted) {
      const total = totalByType.get(ir.row.TypeID) ?? 0;
      const index = (seenByType.get(ir.row.TypeID) ?? 0) + 1;
      seenByType.set(ir.row.TypeID, index);
      if (total > 1) {
        variantByRowId.set(ir.id, { index, total });
      }
    }

    return { indexed, filtered, sorted, variantByRowId };
  }, [results, filters, columnDefs, sortKey, sortDir, pinnedIds]);

  // Available market groups derived from current results (region mode only)
  const availableGroups = useMemo<{ name: string; count: number }[]>(() => {
    if (!isRegionGrouped) return [];
    const counts = new Map<string, number>();
    for (const ir of sorted) {
      // Only show groups for selected categories (if any filter active)
      if (categoryFilter.size > 0 && !categoryFilter.has(ir.row.DayCategoryID ?? 0)) continue;
      const g = ir.row.DayGroupName ?? "";
      if (g) counts.set(g, (counts.get(g) ?? 0) + 1);
    }
    return [...counts.entries()]
      .map(([name, count]) => ({ name, count }))
      .sort((a, b) => b.count - a.count);
  }, [isRegionGrouped, sorted, categoryFilter]);

  // Available categories derived from current results (region mode only)
  const availableCategories = useMemo<{ id: number; name: string; count: number }[]>(() => {
    if (!isRegionGrouped) return [];
    const counts = new Map<number, number>();
    for (const ir of sorted) {
      const cid = ir.row.DayCategoryID ?? 0;
      if (cid > 0) counts.set(cid, (counts.get(cid) ?? 0) + 1);
    }
    return [...counts.entries()]
      .map(([id, count]) => ({ id, name: eveCategoryName(id), count }))
      .filter((c) => c.name !== "")
      .sort((a, b) => b.count - a.count);
  }, [isRegionGrouped, sorted]);

  const displaySorted = useMemo(() => {
    let rows = showHiddenRows ? sorted : sorted.filter((ir) => !hiddenMap[flipStateKey(ir.row)]);
    if (isRegionGrouped) {
      if (categoryFilter.size > 0) {
        rows = rows.filter((ir) => categoryFilter.has(ir.row.DayCategoryID ?? 0));
      }
      if (securityFilter !== "all") {
        rows = rows.filter((ir) => {
          const sec = ir.row.DaySecurity ?? 0;
          if (securityFilter === "highsec") return sec >= 0.45;
          if (securityFilter === "lowsec") return sec >= 0.1 && sec < 0.45;
          return sec < 0.1;
        });
      }
      if (groupFilter.size > 0) {
        rows = rows.filter((ir) => groupFilter.has(ir.row.DayGroupName ?? ""));
      }
    }
    return rows;
  }, [sorted, showHiddenRows, hiddenMap, isRegionGrouped, categoryFilter, securityFilter, groupFilter]);

  const regionGroups = useMemo<RegionGroup[]>(() => {
    if (!isRegionGrouped) return [];
    const metricForRow = (row: FlipResult): number => {
      if (regionGroupSortMode === "trade_score") {
        return Number(row.DayTradeScore ?? 0);
      }
      if (regionGroupSortMode === "now_profit") {
        return Number(
          row.DayNowProfit ??
            row.TotalProfit ??
            row.RealProfit ??
            row.ExpectedProfit ??
            0,
        );
      }
      return Number(
        row.DayPeriodProfit ??
          row.RealProfit ??
          row.ExpectedProfit ??
          row.TotalProfit ??
          0,
      );
    };
    const byKey = new Map<string, RegionGroup>();
    for (const ir of displaySorted) {
      const key = `${ir.row.BuySystemID}:${ir.row.BuySystemName}`;
      const itemMetric = metricForRow(ir.row);
      const metric = Number.isFinite(itemMetric) ? itemMetric : 0;
      const found = byKey.get(key);
      if (found) {
        found.rows.push(ir);
        found.sortValue += metric;
        found.metricCount++;
      } else {
        byKey.set(key, {
          key,
          label: ir.row.BuySystemName || t("hiddenUnknown"),
          rows: [ir],
          sortValue: metric,
          metricCount: 1,
        });
      }
    }
    const groups = [...byKey.values()];
    if (regionGroupSortMode === "trade_score") {
      for (const g of groups) {
        if (g.metricCount > 0) {
          g.sortValue /= g.metricCount;
        }
      }
    }
    groups.sort((a, b) => {
      if (a.sortValue === b.sortValue) {
        return a.label.localeCompare(b.label);
      }
      return b.sortValue - a.sortValue;
    });
    return groups;
  }, [displaySorted, isRegionGrouped, regionGroupSortMode, t]);

  const itemGroups = useMemo<ItemGroup[]>(() => {
    if (!isItemGrouped) return [];
    const byType = new Map<number, ItemGroup>();
    for (const ir of displaySorted) {
      const typeID = ir.row.TypeID ?? 0;
      const existing = byType.get(typeID);
      if (existing) {
        existing.rows.push(ir);
      } else {
        byType.set(typeID, {
          key: String(typeID),
          label: ir.row.TypeName || t("hiddenUnknown"),
          rows: [ir],
        });
      }
    }
    return [...byType.values()];
  }, [displaySorted, isItemGrouped, t]);

  useEffect(() => {
    if (!isItemGrouped) {
      setExpandedItemGroups(new Set());
      return;
    }
    const existing = new Set(itemGroups.map((g) => g.key));
    setExpandedItemGroups((prev) => {
      if (prev.size === 0) return prev;
      const next = new Set<string>();
      for (const key of prev) {
        if (existing.has(key)) next.add(key);
      }
      return next.size === prev.size ? prev : next;
    });
  }, [isItemGrouped, itemGroups]);

  const defaultCollapsedRegionGroups = useMemo(() => {
    if (!isRegionGrouped || regionGroups.length <= 1) {
      return new Set<string>();
    }
    return new Set(regionGroups.slice(1).map((g) => g.key));
  }, [isRegionGrouped, regionGroups]);

  useEffect(() => {
    if (!isRegionGrouped) {
      setCollapsedRegionGroups(new Set());
      setRegionCollapseInitialized(false);
      return;
    }
    // New scan result set: reset collapse state and apply default collapsed on first grouped render.
    setCollapsedRegionGroups(new Set());
    setRegionCollapseInitialized(false);
  }, [isRegionGrouped, results]);

  useEffect(() => {
    if (!isRegionGrouped || regionCollapseInitialized || regionGroups.length === 0) {
      return;
    }
    setCollapsedRegionGroups(new Set(defaultCollapsedRegionGroups));
    setRegionCollapseInitialized(true);
  }, [defaultCollapsedRegionGroups, isRegionGrouped, regionCollapseInitialized, regionGroups.length]);

  useEffect(() => {
    if (!isRegionGrouped) return;
    const existing = new Set(regionGroups.map((g) => g.key));
    setCollapsedRegionGroups((prev) => {
      if (prev.size === 0) return prev;
      const next = new Set<string>();
      for (const key of prev) {
        if (existing.has(key)) next.add(key);
      }
      return next.size === prev.size ? prev : next;
    });
  }, [isRegionGrouped, regionGroups]);

  const effectiveCollapsedRegionGroups = useMemo(() => {
    if (!isRegionGrouped) return collapsedRegionGroups;
    if (regionCollapseInitialized) return collapsedRegionGroups;
    return defaultCollapsedRegionGroups;
  }, [
    collapsedRegionGroups,
    defaultCollapsedRegionGroups,
    isRegionGrouped,
    regionCollapseInitialized,
  ]);

  const visibleRows = useMemo(() => {
    if (isRegionGrouped) {
      const rows: IndexedRow[] = [];
      for (const group of regionGroups) {
        if (effectiveCollapsedRegionGroups.has(group.key)) continue;
        rows.push(...group.rows);
      }
      return rows;
    }
    if (!isItemGrouped) return displaySorted;

    const rows: IndexedRow[] = [];
    for (const group of itemGroups) {
      if (group.rows.length === 0) continue;
      rows.push(group.rows[0]);
      if (group.rows.length <= 1 || !expandedItemGroups.has(group.key)) continue;
      const limit = Math.max(1, groupRowLimit.get(group.key) ?? GROUP_PAGE_SIZE);
      rows.push(...group.rows.slice(1, limit));
    }
    return rows;
  }, [
    displaySorted,
    effectiveCollapsedRegionGroups,
    expandedItemGroups,
    groupRowLimit,
    isItemGrouped,
    isRegionGrouped,
    itemGroups,
    regionGroups,
  ]);

  const { pageRows, totalPages, safePage } = useMemo(() => {
    if (isRegionGrouped || isItemGrouped) {
      return { pageRows: visibleRows, totalPages: 1, safePage: 0 };
    }
    const totalPages = Math.max(1, Math.ceil(displaySorted.length / PAGE_SIZE));
    const safePage = Math.min(page, totalPages - 1);
    const pageRows = displaySorted.slice(
      safePage * PAGE_SIZE,
      (safePage + 1) * PAGE_SIZE,
    );
    return { pageRows, totalPages, safePage };
  }, [displaySorted, isItemGrouped, isRegionGrouped, page, visibleRows]);

  // Reset page when data/filters/sort change
  useEffect(() => {
    setPage(0);
  }, [results, filters, sortKey, sortDir, showHiddenRows, hiddenMap, groupByItem]);

  // Reset selection/pins/context menu/group limits when results change
  useEffect(() => {
    setSelectedIds(new Set());
    setPinnedIds(new Set());
    setContextMenu(null);
    setGroupRowLimit(new Map());
    if (!scanning && results.length > 0) {
      setLastScanTs(Date.now());
    }
  }, [results]);

  // Drop filters for columns that are no longer visible
  useEffect(() => {
    const allowed = new Set(columnDefs.map((col) => col.key));
    setFilters((prev) => {
      let changed = false;
      const next: Record<string, string> = {};
      for (const [key, value] of Object.entries(prev)) {
        if (allowed.has(key as SortKey)) {
          next[key] = value;
        } else {
          changed = true;
        }
      }
      return changed ? next : prev;
    });
  }, [columnDefs]);

  // Prune selected rows that are hidden by filters
  useEffect(() => {
    if (selectedIds.size === 0) return;
    const visibleIds = new Set(visibleRows.map((ir) => ir.id));
    setSelectedIds((prev) => {
      if (prev.size === 0) return prev;
      const next = new Set([...prev].filter((id) => visibleIds.has(id)));
      return next.size === prev.size ? prev : next;
    });
  }, [selectedIds.size, visibleRows]);

  useEffect(() => {
    if (!ignoredModalOpen) {
      setIgnoredSearch("");
      setIgnoredTab("all");
      setIgnoredSelectedKeys(new Set());
    }
  }, [ignoredModalOpen]);

  useEffect(() => {
    setIgnoredSelectedKeys((prev) => {
      if (prev.size === 0) return prev;
      const next = new Set<string>();
      for (const key of prev) {
        if (hiddenMap[key]) next.add(key);
      }
      return next.size === prev.size ? prev : next;
    });
  }, [hiddenMap]);

  useEffect(() => {
    const timer = window.setInterval(() => {
      setCacheNowTs(Date.now());
    }, 1000);
    return () => window.clearInterval(timer);
  }, []);

  const cacheView = useMemo(() => {
    const scopeLabel =
      tradeStateTab === "region"
        ? t("hiddenScopeRegionScan")
        : t("hiddenScopeRadiusScan");
    const fallbackRegionCount = showRegions ? 2 : 1;
    return mapServerCacheMeta(cacheMeta, scopeLabel, fallbackRegionCount, lastScanTs);
  }, [cacheMeta, lastScanTs, showRegions, t, tradeStateTab]);

  const cacheSecondsLeft = useMemo(
    () => Math.floor((cacheView.nextExpiryAt - cacheNowTs) / 1000),
    [cacheNowTs, cacheView.nextExpiryAt],
  );
  const isCacheStale = useMemo(
    () =>
      cacheSecondsLeft <= 0 && cacheNowTs >= cacheStaleSuppressedUntilTs,
    [cacheNowTs, cacheSecondsLeft, cacheStaleSuppressedUntilTs],
  );

  const cacheBadgeText = useMemo(() => {
    if (isCacheStale) return t("cacheStale");
    return t("cacheLabel", { time: formatCountdown(cacheSecondsLeft) });
  }, [cacheSecondsLeft, isCacheStale, t]);

  useEffect(() => {
    setCacheStaleSuppressedUntilTs(0);
  }, [cacheView.currentRevision]);

  const refreshHiddenStates = useCallback(
    async (currentRevision?: number) => {
      try {
        const resp = await getStationTradeStates({
          tab: tradeStateTab,
          currentRevision,
        });
        const states = Array.isArray(resp.states) ? resp.states : [];
        const byStateKey = new Map<string, IndexedRow>();
        for (const ir of indexed) {
          const ids = flipStateIDs(ir.row);
          byStateKey.set(tradeStateIndexKey(ids.typeID, ids.stationID, ids.regionID), ir);
        }
        setHiddenMap((prev) => {
          const next: Record<string, HiddenFlipEntry> = {};
          for (const s of states) {
            const stateKey = tradeStateIndexKey(s.type_id, s.station_id, s.region_id);
            const ir = byStateKey.get(stateKey);
            const key = ir ? flipStateKey(ir.row) : stateKey;
            const prevEntry = prev[key];
            next[key] = {
              key,
              mode: s.mode,
              updatedAt: s.updated_at,
              typeName: ir?.row.TypeName ?? prevEntry?.typeName ?? t("hiddenTypeFallback", { id: s.type_id }),
              buyStation: ir?.row.BuyStation ?? prevEntry?.buyStation ?? t("hiddenUnknown"),
              sellStation: ir?.row.SellStation ?? prevEntry?.sellStation ?? t("hiddenUnknown"),
              stateTypeID: s.type_id,
              stateStationID: s.station_id,
              stateRegionID: s.region_id,
            };
          }
          return next;
        });
      } catch {
        // best effort
      }
    },
    [indexed, t, tradeStateTab],
  );

  useEffect(() => {
    if (scanning) return;
    void refreshHiddenStates(cacheView.currentRevision);
  }, [cacheView.currentRevision, refreshHiddenStates, scanning, results]);

  // ── Summary stats ──
  const summary = useMemo(() => {
    const baseRows = isItemGrouped && selectedIds.size === 0 ? displaySorted : visibleRows;
    const rows =
      selectedIds.size > 0
        ? visibleRows.filter((ir) => selectedIds.has(ir.id))
        : baseRows;
    if (rows.length === 0) return null;
    const totalProfit = rows.reduce(
      (s, ir) => s + (ir.row.RealProfit ?? ir.row.ExpectedProfit ?? ir.row.TotalProfit),
      0,
    );
    const avgMargin =
      rows.reduce((s, ir) => s + ir.row.MarginPercent, 0) / rows.length;
    return { totalProfit, avgMargin, count: rows.length };
  }, [displaySorted, isItemGrouped, selectedIds, visibleRows]);

  // ── Callbacks ──
  const toggleSort = useCallback(
    (key: SortKey) => {
      if (key === sortKey) {
        setSortDir((d) => (d === "asc" ? "desc" : "asc"));
      } else {
        setSortKey(key);
        setSortDir("desc");
      }
    },
    [sortKey],
  );

  const setFilter = useCallback((key: string, value: string) => {
    setFilters((f) => ({ ...f, [key]: value }));
  }, []);

  const clearFilters = useCallback(() => {
    setFilters({});
  }, []);
  const hasActiveFilters = Object.values(filters).some((v) => !!v);

  const toggleSelect = useCallback((id: number) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const toggleSelectAll = useCallback(() => {
    setSelectedIds((prev) => {
      if (prev.size === visibleRows.length) return new Set();
      return new Set(visibleRows.map((ir) => ir.id));
    });
  }, [visibleRows]);

  const togglePin = useCallback((id: number) => {
    setPinnedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const toggleItemGroupExpanded = useCallback((typeID: number) => {
    const key = String(typeID ?? 0);
    startTransition(() => {
      setExpandedItemGroups((prev) => {
        const next = new Set(prev);
        if (next.has(key)) next.delete(key);
        else next.add(key);
        return next;
      });
    });
  }, []);

  const handleContextMenu = useCallback(
    (e: React.MouseEvent, id: number, row: FlipResult) => {
      e.preventDefault();
      setContextMenu({ x: e.clientX, y: e.clientY, id, row });
    },
    [],
  );

  const copyText = useCallback(
    (text: string) => {
      navigator.clipboard.writeText(text);
      addToast(t("copied"), "success", 2000);
      setContextMenu(null);
    },
    [addToast, t],
  );

  const setRowHiddenState = useCallback(
    async (row: FlipResult, mode: HiddenMode) => {
      const key = flipStateKey(row);
      const ids = flipStateIDs(row);
      const entry: HiddenFlipEntry = {
        key,
        mode,
        updatedAt: new Date().toISOString(),
        typeName: row.TypeName,
        buyStation: row.BuyStation,
        sellStation: row.SellStation,
        stateTypeID: ids.typeID,
        stateStationID: ids.stationID,
        stateRegionID: ids.regionID,
      };
      setHiddenMap((prev) => ({ ...prev, [key]: entry }));
      setContextMenu(null);
      try {
        await setStationTradeState({
          tab: tradeStateTab,
          type_id: ids.typeID,
          station_id: ids.stationID,
          region_id: ids.regionID,
          mode,
          until_revision: mode === "done" ? cacheView.currentRevision : 0,
        });
      } catch {
        addToast(t("hiddenStateSaveFailed"), "error", 2600);
        void refreshHiddenStates(cacheView.currentRevision);
      }
    },
    [addToast, cacheView.currentRevision, refreshHiddenStates, t, tradeStateTab],
  );

  const unhideRowsByKeys = useCallback(
    async (keys: string[]) => {
      if (keys.length === 0) return;
      const unique = [...new Set(keys)];
      const payload = unique
        .map((k) => hiddenMap[k])
        .filter(Boolean)
        .map((e) => ({
          type_id: e.stateTypeID,
          station_id: e.stateStationID,
          region_id: e.stateRegionID,
        }));
      setHiddenMap((prev) => {
        const next = { ...prev };
        let changed = false;
        for (const key of unique) {
          if (next[key]) {
            delete next[key];
            changed = true;
          }
        }
        return changed ? next : prev;
      });
      setIgnoredSelectedKeys((prev) => {
        const next = new Set(prev);
        for (const key of unique) next.delete(key);
        return next;
      });
      try {
        if (payload.length > 0) {
          await deleteStationTradeStates({ tab: tradeStateTab, keys: payload });
        }
      } catch {
        addToast(t("hiddenStateUnhideFailed"), "error", 2600);
        void refreshHiddenStates(cacheView.currentRevision);
      }
    },
    [addToast, cacheView.currentRevision, hiddenMap, refreshHiddenStates, t, tradeStateTab],
  );

  const clearDoneHiddenRows = useCallback(async () => {
    const hasDone = Object.values(hiddenMap).some((h) => h.mode === "done");
    if (!hasDone) return;
    setHiddenMap((prev) => {
      const next: Record<string, HiddenFlipEntry> = {};
      for (const [key, entry] of Object.entries(prev)) {
        if (entry.mode !== "done") next[key] = entry;
      }
      return next;
    });
    try {
      await clearStationTradeStates({ tab: tradeStateTab, mode: "done" });
    } catch {
      addToast(t("hiddenStateClearDoneFailed"), "error", 2600);
      void refreshHiddenStates(cacheView.currentRevision);
    }
  }, [addToast, cacheView.currentRevision, hiddenMap, refreshHiddenStates, t, tradeStateTab]);

  const clearAllHiddenRows = useCallback(async () => {
    if (Object.keys(hiddenMap).length === 0) return;
    setHiddenMap({});
    setIgnoredSelectedKeys(new Set());
    try {
      await clearStationTradeStates({ tab: tradeStateTab });
    } catch {
      addToast(t("hiddenStateClearAllFailed"), "error", 2600);
      void refreshHiddenStates(cacheView.currentRevision);
    }
  }, [addToast, cacheView.currentRevision, hiddenMap, refreshHiddenStates, t, tradeStateTab]);

  const handleRebootCache = useCallback(async () => {
    if (cacheRebooting) return;
    setCacheRebooting(true);
    try {
      const res = await rebootStationCache();
      const nowTs = Date.now();
      setLastScanTs(nowTs);
      setCacheNowTs(nowTs);
      // Give backend cache reboot a short grace period before stale marker returns.
      setCacheStaleSuppressedUntilTs(nowTs + 45_000);
      addToast(t("cacheRebooted", { count: res.cleared }), "success", 2400);
      addToast(t("cacheRebootRescanHint"), "info", 2600);
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : t("cacheRebootFailed");
      addToast(msg, "error", 2800);
    } finally {
      setCacheRebooting(false);
    }
  }, [addToast, cacheRebooting, t]);

  const exportCSV = useCallback(() => {
    const rows =
      selectedIds.size > 0
        ? visibleRows.filter((ir) => selectedIds.has(ir.id))
        : visibleRows;
    const header = columnDefs.map((c) => t(c.labelKey)).join(",");
    const csvRows = rows.map((ir) =>
      columnDefs
        .map((col) => {
          const str = String(getCellValue(ir.row, col.key) ?? "");
          return str.includes(",") ? `"${str}"` : str;
        })
        .join(","),
    );
    const csv = [header, ...csvRows].join("\n");
    const blob = new Blob(["\uFEFF" + csv], { type: "text/csv;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `eve-flipper-${new Date().toISOString().slice(0, 10)}.csv`;
    a.click();
    URL.revokeObjectURL(url);
    addToast(`${t("exportCSV")}: ${rows.length} rows`, "success", 2000);
  }, [visibleRows, selectedIds, columnDefs, addToast, t]);

  const copyTable = useCallback(() => {
    const rows =
      selectedIds.size > 0
        ? visibleRows.filter((ir) => selectedIds.has(ir.id))
        : visibleRows;
    const header = columnDefs.map((c) => t(c.labelKey)).join("\t");
    const tsv = rows.map((ir) =>
      columnDefs.map((col) => fmtCell(col, ir.row)).join("\t"),
    );
    navigator.clipboard.writeText([header, ...tsv].join("\n"));
    addToast(t("copied"), "success", 2000);
  }, [visibleRows, selectedIds, columnDefs, addToast, t]);

  const hiddenEntries = useMemo(
    () =>
      Object.values(hiddenMap).sort((a, b) =>
        b.updatedAt.localeCompare(a.updatedAt),
      ),
    [hiddenMap],
  );
  const hiddenCounts = useMemo(() => {
    let done = 0;
    let ignored = 0;
    for (const row of hiddenEntries) {
      if (row.mode === "done") done++;
      if (row.mode === "ignored") ignored++;
    }
    return { total: hiddenEntries.length, done, ignored };
  }, [hiddenEntries]);
  const filteredHiddenEntries = useMemo(() => {
    const q = ignoredSearch.trim().toLowerCase();
    return hiddenEntries.filter((entry) => {
      if (ignoredTab !== "all" && entry.mode !== ignoredTab) return false;
      if (!q) return true;
      return (
        entry.typeName.toLowerCase().includes(q) ||
        entry.buyStation.toLowerCase().includes(q) ||
        entry.sellStation.toLowerCase().includes(q)
      );
    });
  }, [hiddenEntries, ignoredSearch, ignoredTab]);

  const contextHiddenEntry = contextMenu
    ? hiddenMap[flipStateKey(contextMenu.row)]
    : undefined;

  // Stable LMB handler for region detail panel
  const onLmbClick = useCallback((row: FlipResult) => {
    setDayDetailRow(row);
  }, []);

  // renderDataRow: renders a DataRow memo component — only the changed row re-renders
  const renderDataRow = useCallback(
    (
      ir: IndexedRow,
      globalIdx: number,
      itemVariantState?: { expandable: boolean; expanded: boolean },
    ) => (
      <DataRow
        key={ir.id}
        ir={ir}
        globalIdx={globalIdx}
        columnDefs={columnDefs}
        compactMode={compactMode}
        isPinned={pinnedIds.has(ir.id)}
        isSelected={selectedIds.has(ir.id)}
        variant={variantByRowId.get(ir.id)}
        rowHidden={hiddenMap[flipStateKey(ir.row)]}
        isItemGrouped={isItemGrouped}
        isRegionGrouped={isRegionGrouped}
        variantExpandable={itemVariantState?.expandable ?? false}
        variantExpanded={itemVariantState?.expanded ?? false}
        onToggleVariantGroup={toggleItemGroupExpanded}
        onContextMenu={handleContextMenu}
        onLmbClick={onLmbClick}
        onToggleSelect={toggleSelect}
        onTogglePin={togglePin}
        tFn={t}
      />
    ),
    [
      columnDefs,
      compactMode,
      handleContextMenu,
      hiddenMap,
      isItemGrouped,
      isRegionGrouped,
      onLmbClick,
      pinnedIds,
      selectedIds,
      t,
      toggleItemGroupExpanded,
      togglePin,
      toggleSelect,
      variantByRowId,
    ],
  );

  // ── Render ──
  return (
    <div className="relative flex-1 flex flex-col min-h-0">
      {/* Toolbar */}
      <div className="shrink-0 flex items-center gap-2 px-2 py-1.5 text-xs">
        <div className="flex items-center gap-2 text-eve-dim">
          {scanning ? (
            <span className="flex items-center gap-2">
              <span className="w-2 h-2 rounded-full bg-eve-accent animate-pulse" />
              {progress}
            </span>
          ) : results.length > 0 ? (
            filtered.length !== indexed.length ? (
              t("showing", { shown: filtered.length, total: indexed.length })
            ) : (
              t("foundDeals", { count: indexed.length })
            )
          ) : null}
          {!scanning && results.length > 0 && hiddenCounts.total > 0 && (
            <span className="text-eve-dim">
              |{" "}
              {t("hiddenVisibleSummary", {
                visible: displaySorted.length,
                hidden: hiddenCounts.total,
              })}
            </span>
          )}
          {pinnedIds.size > 0 && (
            <span className="text-eve-accent">
              📌 {t("pinned", { count: pinnedIds.size })}
            </span>
          )}
          {selectedIds.size > 0 && (
            <span className="text-eve-accent">
              {t("selected", { count: selectedIds.size })}
            </span>
          )}
          {!scanning && results.length > 0 && isCacheStale && (
            <span className="text-red-300">| {t("cacheStaleHint")}</span>
          )}
        </div>

        <div className="flex-1" />

        {/* Pagination */}
        {!isRegionGrouped && !isItemGrouped && displaySorted.length > PAGE_SIZE && (
          <div className="flex items-center gap-1 text-eve-dim">
            <button
              onClick={() => setPage(0)}
              disabled={safePage === 0}
              className="px-1.5 py-0.5 rounded-sm hover:text-eve-text disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
            >
              «
            </button>
            <button
              onClick={() => setPage((p) => Math.max(0, p - 1))}
              disabled={safePage === 0}
              className="px-1.5 py-0.5 rounded-sm hover:text-eve-text disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
            >
              ‹
            </button>
            <span className="px-2 text-eve-text font-mono tabular-nums">
              {safePage + 1} / {totalPages}
            </span>
            <button
              onClick={() => setPage((p) => Math.min(totalPages - 1, p + 1))}
              disabled={safePage >= totalPages - 1}
              className="px-1.5 py-0.5 rounded-sm hover:text-eve-text disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
            >
              ›
            </button>
            <button
              onClick={() => setPage(totalPages - 1)}
              disabled={safePage >= totalPages - 1}
              className="px-1.5 py-0.5 rounded-sm hover:text-eve-text disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
            >
              »
            </button>
          </div>
        )}

        {results.length > 0 && !scanning && (
          <>
            {!isRegionGrouped && (
              <label className="inline-flex items-center gap-1 px-2 py-0.5 rounded-sm border border-eve-border/60 bg-eve-dark/40 text-[11px] cursor-pointer">
                <input
                  type="checkbox"
                  checked={groupByItem}
                  onChange={(e) => setGroupByItem(e.target.checked)}
                  className="accent-eve-accent"
                />
                <span>Group by item</span>
              </label>
            )}
            <label className="inline-flex items-center gap-1 px-2 py-0.5 rounded-sm border border-eve-border/60 bg-eve-dark/40 text-[11px] cursor-pointer">
              <input
                type="checkbox"
                checked={showHiddenRows}
                onChange={(e) => setShowHiddenRows(e.target.checked)}
                className="accent-eve-accent"
              />
              <span>{t("showHidden")}</span>
            </label>
            <button
              type="button"
              onClick={() => setIgnoredModalOpen(true)}
              className="px-2 py-0.5 rounded-sm border border-eve-border/60 bg-eve-dark/40 text-[11px] hover:border-eve-accent/50 hover:text-eve-accent transition-colors"
              title={t("hiddenOpenManagerTitle")}
            >
              {t("hiddenButton", { count: hiddenCounts.total })}
            </button>
            <button
              type="button"
              onClick={() => {
                void handleRebootCache();
              }}
              disabled={cacheRebooting}
              className={`px-2 py-0.5 rounded-sm border bg-eve-dark/40 text-[11px] transition-colors disabled:opacity-40 disabled:cursor-not-allowed ${
                isCacheStale
                  ? "border-red-500/60 text-red-300 hover:bg-red-900/20"
                  : "border-eve-border/60 text-eve-dim hover:border-eve-accent/50 hover:text-eve-accent"
              }`}
              title={t("cacheHardResetTitle")}
            >
              {cacheRebooting ? t("cacheRebooting") : t("cacheReboot")}
            </button>
            <button
              type="button"
              className={`px-2 py-0.5 rounded-sm border text-[11px] font-mono transition-colors ${
                isCacheStale
                  ? "border-red-500/50 text-red-300 bg-red-950/30"
                  : "border-eve-border/60 text-eve-accent bg-eve-dark/40 hover:border-eve-accent/50"
              }`}
              title={`${t("cacheTooltipScope")}: ${cacheView.scopeLabel}\n${t("cacheTooltipRegions")}: ${cacheView.regionCount}\n${t("cacheTooltipLastRefresh")}: ${new Date(cacheView.lastRefreshAt).toLocaleTimeString()}\n${t("cacheTooltipNextExpiry")}: ${new Date(cacheView.nextExpiryAt).toLocaleTimeString()}`}
            >
              {cacheBadgeText}
            </button>
          </>
        )}

        {/* Action buttons */}
        <ToolbarBtn
          label={t("columnsButton")}
          title={t("columnsPanelTitle")}
          active={showColumnPanel}
          onClick={() => setShowColumnPanel((v) => !v)}
        />
        <ToolbarBtn
          label="⊞"
          title={showFilters ? t("clearFilters") : t("filterPlaceholder")}
          active={showFilters}
          onClick={() => setShowFilters((v) => !v)}
        />
        {hasActiveFilters && (
          <ToolbarBtn
            label="✕"
            title={t("clearFilters")}
            onClick={clearFilters}
          />
        )}
        {results.length > 0 && (
          <>
            <ToolbarBtn
              label={compactMode ? "⊞" : "⊟"}
              title={compactMode ? t("comfyRows") : t("compactRows")}
              active={compactMode}
              onClick={() => setCompactMode((v) => !v)}
            />
            <ToolbarBtn
              label="CSV"
              title={t("exportCSV")}
              onClick={exportCSV}
            />
            <ToolbarBtn
              label="⎘"
              title={t("copyTable")}
              onClick={copyTable}
            />
          </>
        )}
        {isRegionGrouped && results.length > 0 && !scanning && (
          <div className="inline-flex items-center gap-1 px-1 py-0.5 rounded-sm border border-eve-border/60 bg-eve-dark/40 text-[11px]">
            <span className="text-eve-dim px-1">Sort:</span>
            {([
              ["period_profit", "Period"],
              ["now_profit", "Now"],
              ["trade_score", "Score"],
            ] as const).map(([mode, label]) => {
              const active = regionGroupSortMode === mode;
              return (
                <button
                  key={mode}
                  type="button"
                  onClick={() => setRegionGroupSortMode(mode)}
                  className={`px-1.5 py-0.5 rounded-sm border transition-colors ${
                    active
                      ? "border-eve-accent/70 bg-eve-accent/15 text-eve-accent"
                      : "border-eve-border/50 text-eve-dim hover:text-eve-text"
                  }`}
                >
                  {label}
                </button>
              );
            })}
          </div>
        )}
        {/* Item filter button — region mode only */}
        {isRegionGrouped && results.length > 0 && !scanning && (() => {
          const activeCount = categoryFilter.size + groupFilter.size + (securityFilter !== "all" ? 1 : 0);
          return (
            <button
              ref={filterBtnRef}
              type="button"
              onClick={() => { setFilterPanelOpen((v) => !v); setFilterSearch(""); }}
              className={`relative px-2 py-0.5 rounded-sm border text-[11px] transition-colors ${
                filterPanelOpen || activeCount > 0
                  ? "border-eve-accent/70 bg-eve-accent/15 text-eve-accent"
                  : "border-eve-border/60 bg-eve-dark/40 text-eve-dim hover:border-eve-accent/50 hover:text-eve-accent"
              }`}
              title="Open item filter (category · group · security)"
            >
              ⚙ Filters{activeCount > 0 && (
                <span className="ml-1 px-1 rounded-full bg-eve-accent text-eve-dark text-[9px] font-bold">{activeCount}</span>
              )}
            </button>
          );
        })()}
      </div>

      {/* ── Region item filter popup ── */}
      {isRegionGrouped && filterPanelOpen && results.length > 0 && (
        <div
          ref={filterPanelRef}
          className="absolute z-40 top-[calc(var(--toolbar-h,36px)+2px)] left-0 right-0 mx-2 bg-eve-panel border border-eve-border rounded-sm shadow-2xl text-xs"
          style={{ maxHeight: "60vh", display: "flex", flexDirection: "column" }}
        >
          {/* Header */}
          <div className="flex items-center gap-2 px-3 py-2 border-b border-eve-border shrink-0">
            <span className="text-eve-accent font-semibold">Item Filter</span>

            {/* Security */}
            <div className="flex items-center gap-1 ml-2">
              <span className="text-eve-dim">Sec:</span>
              {(["all", "highsec", "lowsec", "nullsec"] as const).map((s) => {
                const labels = {
                  all: "All",
                  highsec: "≥0.5",
                  lowsec: "0.1-0.4",
                  nullsec: "<0.1",
                } as const;
                const activeColors = {
                  all: "border-eve-accent/70 bg-eve-accent/15 text-eve-accent",
                  highsec: "text-green-300 border-green-500/60 bg-green-900/20",
                  lowsec: "text-yellow-300 border-yellow-500/60 bg-yellow-900/20",
                  nullsec: "text-red-300 border-red-500/60 bg-red-900/20",
                } as const;
                const active = securityFilter === s;
                return (
                  <button key={s} onClick={() => setSecurityFilter(s)}
                    className={`px-1.5 py-0.5 rounded-sm border transition-colors ${active ? activeColors[s] : "border-eve-border/50 text-eve-dim hover:text-eve-text"}`}>
                    {labels[s]}
                  </button>
                );
              })}
            </div>

            <div className="flex-1" />

            {/* Reset all */}
            {(categoryFilter.size > 0 || groupFilter.size > 0 || securityFilter !== "all") && (
              <button onClick={() => { setCategoryFilter(new Set()); setGroupFilter(new Set()); setSecurityFilter("all"); }}
                className="px-2 py-0.5 rounded-sm border border-red-500/50 text-red-300 hover:bg-red-900/20 transition-colors">
                Reset all
              </button>
            )}

            {/* Search */}
            <input
              autoFocus
              type="text"
              placeholder="Search categories / groups…"
              value={filterSearch}
              onChange={(e) => setFilterSearch(e.target.value)}
              className="w-48 px-2 py-0.5 rounded-sm border border-eve-border bg-eve-dark text-eve-text placeholder-eve-dim focus:outline-none focus:border-eve-accent"
            />

            <button onClick={() => setFilterPanelOpen(false)} className="text-eve-dim hover:text-eve-text ml-1">✕</button>
          </div>

          {/* Scrollable body */}
          <div className="overflow-y-auto p-3 space-y-3">
            {/* Category section */}
            {availableCategories.length > 0 && (() => {
              const q = filterSearch.trim().toLowerCase();
              const cats = q ? availableCategories.filter(c => c.name.toLowerCase().includes(q)) : availableCategories;
              if (cats.length === 0) return null;
              return (
                <div>
                  <div className="flex items-center gap-2 mb-1.5">
                    <span className="text-eve-dim uppercase tracking-wider text-[10px]">Category</span>
                    {categoryFilter.size > 0 && (
                      <button onClick={() => { setCategoryFilter(new Set()); setGroupFilter(new Set()); }}
                        className="text-red-400 hover:text-red-300 text-[10px]">✕ clear</button>
                    )}
                  </div>
                  <div className="flex flex-wrap gap-1">
                    {cats.map(({ id, name, count }) => {
                      const active = categoryFilter.has(id);
                      return (
                        <button key={id}
                          onClick={() => {
                            setCategoryFilter((prev) => { const n = new Set(prev); if (active) n.delete(id); else n.add(id); return n; });
                            setGroupFilter(new Set());
                          }}
                          className={`px-2 py-0.5 rounded-sm border transition-colors ${active ? "border-eve-accent/70 bg-eve-accent/15 text-eve-accent" : "border-eve-border/50 text-eve-dim hover:border-eve-accent/40 hover:text-eve-text"}`}>
                          {name} <span className="opacity-60">{count}</span>
                        </button>
                      );
                    })}
                  </div>
                </div>
              );
            })()}

            {/* Group section */}
            {availableGroups.length > 0 && (() => {
              const q = filterSearch.trim().toLowerCase();
              const grps = q ? availableGroups.filter(g => g.name.toLowerCase().includes(q)) : availableGroups;
              if (grps.length === 0) return null;
              return (
                <div>
                  <div className="flex items-center gap-2 mb-1.5">
                    <span className="text-eve-dim uppercase tracking-wider text-[10px]">Group</span>
                    {groupFilter.size > 0 && (
                      <button onClick={() => setGroupFilter(new Set())}
                        className="text-red-400 hover:text-red-300 text-[10px]">✕ clear</button>
                    )}
                  </div>
                  <div className="flex flex-wrap gap-1">
                    {grps.map(({ name, count }) => {
                      const active = groupFilter.has(name);
                      return (
                        <button key={name}
                          onClick={() => setGroupFilter((prev) => { const n = new Set(prev); if (active) n.delete(name); else n.add(name); return n; })}
                          className={`px-2 py-0.5 rounded-sm border transition-colors ${active ? "border-sky-400/70 bg-sky-900/20 text-sky-300" : "border-eve-border/50 text-eve-dim hover:border-sky-400/40 hover:text-eve-text"}`}>
                          {name} <span className="opacity-60">{count}</span>
                        </button>
                      );
                    })}
                  </div>
                </div>
              );
            })()}
          </div>
        </div>
      )}

      {showColumnPanel && (
        <div className="shrink-0 px-2 pb-2">
          <div className="border border-eve-border rounded-sm bg-eve-dark/40 p-2 text-xs">
            <div className="flex items-center gap-2 mb-2">
              <span className="text-eve-dim">{t("columnsPanelTitle")}</span>
              <button
                type="button"
                onClick={() => setHiddenColumns(new Set())}
                className="px-2 py-0.5 rounded-sm border border-eve-border/60 hover:border-eve-accent/50 hover:text-eve-accent transition-colors"
              >
                {t("columnsShowAll")}
              </button>
              <button
                type="button"
                onClick={resetColumns}
                className="px-2 py-0.5 rounded-sm border border-eve-border/60 hover:border-eve-accent/50 hover:text-eve-accent transition-colors"
              >
                {t("columnsReset")}
              </button>
            </div>
            <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-4 xl:grid-cols-6 gap-1.5">
              {orderedColumnDefs.map((col, idx) => {
                const visible = !hiddenColumns.has(col.key);
                return (
                  <div
                    key={col.key}
                    tabIndex={0}
                    onKeyDown={(e) => {
                      if (e.key === "ArrowLeft" || e.key === "ArrowUp") {
                        e.preventDefault();
                        moveColumn(col.key, -1);
                      } else if (
                        e.key === "ArrowRight" ||
                        e.key === "ArrowDown"
                      ) {
                        e.preventDefault();
                        moveColumn(col.key, 1);
                      }
                    }}
                    className="flex items-center gap-1 rounded-sm border border-eve-border/40 bg-eve-panel/60 px-2 py-1"
                  >
                    <label className="flex items-center gap-1.5 min-w-0 flex-1">
                      <input
                        type="checkbox"
                        checked={visible}
                        onChange={(e) => toggleColumnVisibility(col.key, e.target.checked)}
                        className="accent-eve-accent"
                      />
                      <span className="truncate" title={t(col.labelKey)}>
                        {t(col.labelKey)}
                      </span>
                    </label>
                    <button
                      type="button"
                      onClick={() => moveColumn(col.key, -1)}
                      disabled={idx === 0}
                      className="px-1 rounded-sm border border-eve-border/40 disabled:opacity-30 disabled:cursor-not-allowed"
                      title={t("columnsMoveLeft")}
                    >
                      ←
                    </button>
                    <button
                      type="button"
                      onClick={() => moveColumn(col.key, 1)}
                      disabled={idx === orderedColumnDefs.length - 1}
                      className="px-1 rounded-sm border border-eve-border/40 disabled:opacity-30 disabled:cursor-not-allowed"
                      title={t("columnsMoveRight")}
                    >
                      →
                    </button>
                  </div>
                );
              })}
            </div>
          </div>
        </div>
      )}

      {/* Table */}
      <div className="flex-1 min-h-0 flex flex-col border border-eve-border rounded-sm overflow-auto table-scroll-container">
        <table className="w-full text-sm">
          <thead className="sticky top-0 z-10">
            <tr className="bg-eve-dark border-b border-eve-border">
              <th className="w-8 px-1 py-2 text-center">
                <input
                  type="checkbox"
                  checked={
                    visibleRows.length > 0 &&
                    selectedIds.size === visibleRows.length
                  }
                  onChange={toggleSelectAll}
                  className="accent-eve-accent cursor-pointer"
                />
              </th>
              <th className="w-8 px-1 py-2" />
              {columnDefs.map((col) => (
                <th
                  key={col.key}
                  onClick={() => toggleSort(col.key)}
                  title={
                    col.tooltipKey
                      ? `${t(col.labelKey)}: ${t(col.tooltipKey)}`
                      : t(col.labelKey)
                  }
                  className={`${col.width} px-3 py-2 text-left text-[11px] uppercase tracking-wider text-eve-dim font-medium cursor-pointer select-none hover:text-eve-accent transition-colors ${sortKey === col.key ? "text-eve-accent" : ""}`}
                >
                  <span className="inline-flex items-center gap-1">
                    {t(col.labelKey)}
                    {col.tooltipKey ? (
                      <span className="text-eve-dim/70 normal-case text-[10px]">?</span>
                    ) : null}
                  </span>
                  {sortKey === col.key && (
                    <span className="ml-1">
                      {sortDir === "asc" ? "▲" : "▼"}
                    </span>
                  )}
                </th>
              ))}
            </tr>
            {showFilters && (
              <tr className="bg-eve-dark/80 border-b border-eve-border">
                <th className="w-8" />
                <th className="w-8" />
                {columnDefs.map((col) => (
                  <th key={col.key} className={`${col.width} px-1 py-1`}>
                    <input
                      type="text"
                      value={filters[col.key] ?? ""}
                      onChange={(e) => setFilter(col.key, e.target.value)}
                      title={
                        col.tooltipKey
                          ? `${t(col.labelKey)}: ${t(col.tooltipKey)}`
                          : t(col.labelKey)
                      }
                      placeholder={
                        col.numeric ? "e.g. >100" : t("filterPlaceholder")
                      }
                      className="w-full px-2 py-0.5 bg-eve-input border border-eve-border rounded-sm text-eve-text text-xs font-mono placeholder:text-eve-dim/50 focus:outline-none focus:border-eve-accent/50 transition-colors"
                    />
                  </th>
                ))}
              </tr>
            )}
          </thead>
          <tbody>
            {isRegionGrouped
              ? (() => {
                  let rowIndex = 0;
                  return regionGroups.map((group) => {
                    const collapsed = effectiveCollapsedRegionGroups.has(group.key);
                    return (
                      <Fragment key={`group:${group.key}`}>
                        <tr className="border-b border-eve-border/60 bg-eve-dark/50">
                          <td colSpan={columnDefs.length + 2} className="px-2 py-1.5">
                            <button
                              type="button"
                              onClick={() => {
                                setRegionCollapseInitialized(true);
                                startTransition(() => {
                                  setCollapsedRegionGroups((prev) => {
                                    const next = regionCollapseInitialized
                                      ? new Set(prev)
                                      : new Set(defaultCollapsedRegionGroups);
                                    if (next.has(group.key)) next.delete(group.key);
                                    else next.add(group.key);
                                    return next;
                                  });
                                });
                              }}
                              className="inline-flex items-center gap-2 text-xs text-eve-text hover:text-eve-accent transition-colors"
                            >
                              <span className="text-eve-accent">
                                {collapsed ? "▶" : "▼"}
                              </span>
                              <span className="font-medium">{group.label}</span>
                              <span className="text-eve-dim">
                                {group.rows.length}
                              </span>
                              <span className="text-eve-accent font-mono">
                                {regionGroupSortMode === "trade_score"
                                  ? `Score ${group.sortValue.toFixed(1)}`
                                  : formatISK(group.sortValue)}
                              </span>
                            </button>
                          </td>
                        </tr>
                        {!collapsed && (() => {
                          const limit = groupRowLimit.get(group.key) ?? GROUP_PAGE_SIZE;
                          const sliced = group.rows.slice(0, limit);
                          const hasMore = group.rows.length > limit;
                          return (
                            <>
                              {sliced.map((ir) => {
                                const rendered = renderDataRow(ir, rowIndex);
                                rowIndex++;
                                return rendered;
                              })}
                              {hasMore && (
                                <tr className="bg-eve-dark/30">
                                  <td colSpan={columnDefs.length + 2} className="px-4 py-1.5 text-center">
                                    <button
                                      type="button"
                                      onClick={() =>
                                        setGroupRowLimit((prev) => {
                                          const next = new Map(prev);
                                          next.set(group.key, group.rows.length);
                                          return next;
                                        })
                                      }
                                      className="text-[11px] text-eve-dim hover:text-eve-accent transition-colors"
                                    >
                                      Show all {group.rows.length} items ({group.rows.length - limit} more) ↓
                                    </button>
                                  </td>
                                </tr>
                              )}
                            </>
                          );
                        })()}
                      </Fragment>
                    );
                  });
                })()
              : isItemGrouped
                ? (() => {
                    let rowIndex = 0;
                    return itemGroups.map((group) => {
                      if (group.rows.length === 0) return null;
                      const topRow = group.rows[0];
                      const expanded =
                        group.rows.length > 1 && expandedItemGroups.has(group.key);
                      const limit = Math.max(
                        1,
                        groupRowLimit.get(group.key) ?? GROUP_PAGE_SIZE,
                      );
                      const childRows = expanded ? group.rows.slice(1, limit) : [];
                      const hasMore = expanded && group.rows.length > limit;
                      const topRendered = renderDataRow(topRow, rowIndex, {
                        expandable: group.rows.length > 1,
                        expanded,
                      });
                      rowIndex++;
                      return (
                        <Fragment key={`item-group:${group.key}`}>
                          {topRendered}
                          {childRows.map((ir) => {
                            const rendered = renderDataRow(ir, rowIndex);
                            rowIndex++;
                            return rendered;
                          })}
                          {hasMore && (
                            <tr className="bg-eve-dark/30">
                              <td colSpan={columnDefs.length + 2} className="px-4 py-1.5 text-center">
                                <button
                                  type="button"
                                  onClick={() =>
                                    setGroupRowLimit((prev) => {
                                      const next = new Map(prev);
                                      next.set(group.key, group.rows.length);
                                      return next;
                                    })
                                  }
                                  className="text-[11px] text-eve-dim hover:text-eve-accent transition-colors"
                                >
                                  Show all {group.rows.length} items ({group.rows.length - limit} more) ↓
                                </button>
                              </td>
                            </tr>
                          )}
                        </Fragment>
                      );
                    });
                  })()
                : pageRows.map((ir, i) => renderDataRow(ir, safePage * PAGE_SIZE + i))}
            {displaySorted.length === 0 && !scanning && (
              <tr>
                <td colSpan={columnDefs.length + 2} className="p-0">
                  {results.length > 0 && hiddenCounts.total > 0 && !showHiddenRows ? (
                    <div className="p-6 text-center text-sm text-eve-dim">
                      {t("hiddenAllRowsPrefix")}{" "}
                      <span className="text-eve-accent">{t("showHidden")}</span>{" "}
                      {t("hiddenAllRowsOrOpen")}{" "}
                      <span className="text-eve-accent">
                        {t("hiddenButton", { count: hiddenCounts.total })}
                      </span>
                      .
                    </div>
                  ) : (
                    <EmptyState reason={emptyReason} wikiSlug="Getting-Started" />
                  )}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {/* Summary footer */}
      {summary && displaySorted.length > 0 && (
        <div className="shrink-0 flex items-center gap-6 px-3 py-1.5 border-t border-eve-border text-xs">
          <span className="text-eve-dim">
            {t("colRealProfit")}:{" "}
            <span className="text-eve-accent font-mono font-semibold">
              {formatISK(summary.totalProfit)}
            </span>
          </span>
          <span className="text-eve-dim">
            {t("avgMargin")}:{" "}
            <span className="text-eve-accent font-mono font-semibold">
              {formatMargin(summary.avgMargin)}
            </span>
          </span>
          {selectedIds.size > 0 && (
            <span className="text-eve-dim italic">
              ({t("selected", { count: selectedIds.size })})
            </span>
          )}
        </div>
      )}

      {/* Context menu */}
      {contextMenu && (
        <>
          <div
            className="fixed inset-0 z-50"
            onClick={() => setContextMenu(null)}
          />
          <div
            ref={contextMenuRef}
            className="fixed z-50 bg-eve-panel border border-eve-border rounded-sm shadow-eve-glow-strong py-1 min-w-[200px]"
            style={{ left: contextMenu.x, top: contextMenu.y }}
          >
            <ContextItem
              label={t("copyItem")}
              onClick={() => copyText(contextMenu.row.TypeName ?? "")}
            />
            <ContextItem
              label={t("copyBuyStation")}
              onClick={() => copyText(contextMenu.row.BuyStation ?? "")}
            />
            <ContextItem
              label={t("copySellStation")}
              onClick={() => copyText(contextMenu.row.SellStation ?? "")}
            />
            <ContextItem
              label={t("copyTradeRoute")}
              onClick={() =>
                copyText(
                  `Buy: ${contextMenu.row.TypeName} x${contextMenu.row.UnitsToBuy} @ ${contextMenu.row.BuyStation} \u2192 Sell: @ ${contextMenu.row.SellStation}`,
                )
              }
            />
            <ContextItem
              label={t("buildBatch")}
              onClick={() => {
                setBatchPlanRow(contextMenu.row);
                setContextMenu(null);
              }}
            />
            <ContextItem
              label={t("copySystemAutopilot")}
              onClick={() => copyText(contextMenu.row.BuySystemName)}
            />
            <div className="h-px bg-eve-border my-1" />
            <ContextItem
              label={t("openInEveref")}
              onClick={() => {
                window.open(
                  `https://everef.net/type/${contextMenu.row.TypeID}`,
                  "_blank",
                );
                setContextMenu(null);
              }}
            />
            <ContextItem
              label={t("openInJitaSpace")}
              onClick={() => {
                window.open(
                  `https://www.jita.space/market/${contextMenu.row.TypeID}`,
                  "_blank",
                );
                setContextMenu(null);
              }}
            />
            <div className="h-px bg-eve-border my-1" />
            <ContextItem
              label={
                watchlistIds.has(contextMenu.row.TypeID)
                  ? t("untrackItem")
                  : `\u2B50 ${t("trackItem")}`
              }
              onClick={() => {
                const row = contextMenu.row;
                if (watchlistIds.has(row.TypeID)) {
                  removeFromWatchlist(row.TypeID)
                    .then(setWatchlist)
                    .then(() =>
                      addToast(t("watchlistRemoved"), "success", 2000),
                    )
                    .catch(() => addToast(t("watchlistError"), "error", 3000));
                } else {
                  addToWatchlist(row.TypeID, row.TypeName)
                    .then((r) => {
                      setWatchlist(r.items);
                      addToast(
                        r.inserted
                          ? t("watchlistItemAdded")
                          : t("watchlistAlready"),
                        r.inserted ? "success" : "info",
                        2000,
                      );
                    })
                    .catch(() => addToast(t("watchlistError"), "error", 3000));
                }
                setContextMenu(null);
              }}
            />
            <div className="h-px bg-eve-border my-1" />
            {contextHiddenEntry ? (
              <ContextItem
                label={t("hiddenContextUnhide")}
                onClick={() => {
                  void unhideRowsByKeys([contextHiddenEntry.key]);
                  setContextMenu(null);
                }}
              />
            ) : (
              <>
                <ContextItem
                  label={t("hiddenContextMarkDone")}
                  onClick={() => {
                    void setRowHiddenState(contextMenu.row, "done");
                  }}
                />
                <ContextItem
                  label={t("hiddenContextIgnore")}
                  onClick={() => {
                    void setRowHiddenState(contextMenu.row, "ignored");
                  }}
                />
              </>
            )}
            {(contextMenu.row.BuyRegionID != null ||
              contextMenu.row.SellRegionID != null) && (
              <ContextItem
                label={t("placeDraft")}
                onClick={() => {
                  setExecPlanRow(contextMenu.row);
                  setContextMenu(null);
                }}
              />
            )}
            {/* EVE UI actions */}
            {isLoggedIn && (
              <>
                <div className="h-px bg-eve-border my-1" />
                <ContextItem
                  label={`🎮 ${t("openMarket")}`}
                  onClick={async () => {
                    try {
                      await openMarketInGame(contextMenu.row.TypeID);
                      addToast(t("actionSuccess"), "success", 2000);
                    } catch (err: any) {
                      const { messageKey, duration } = handleEveUIError(err);
                      addToast(t(messageKey), "error", duration);
                    }
                    setContextMenu(null);
                  }}
                />
                <ContextItem
                  label={`🎯 ${t("setDestination")} (Buy)`}
                  onClick={async () => {
                    try {
                      await setWaypointInGame(contextMenu.row.BuySystemID);
                      addToast(t("actionSuccess"), "success", 2000);
                    } catch (err: any) {
                      const { messageKey, duration } = handleEveUIError(err);
                      addToast(t(messageKey), "error", duration);
                    }
                    setContextMenu(null);
                  }}
                />
                {contextMenu.row.SellSystemID !== contextMenu.row.BuySystemID && (
                  <ContextItem
                    label={`🎯 ${t("setDestination")} (Sell)`}
                    onClick={async () => {
                      try {
                        await setWaypointInGame(contextMenu.row.SellSystemID);
                        addToast(t("actionSuccess"), "success", 2000);
                      } catch (err: any) {
                        addToast(t("actionFailed").replace("{error}", err.message), "error", 3000);
                      }
                      setContextMenu(null);
                    }}
                  />
                )}
              </>
            )}
            <div className="h-px bg-eve-border my-1" />
            <ContextItem
              label={
                pinnedIds.has(contextMenu.id) ? t("unpinRow") : t("pinRow")
              }
              onClick={() => {
                togglePin(contextMenu.id);
                setContextMenu(null);
              }}
            />
          </div>
        </>
      )}

      {ignoredModalOpen && (
        <>
          <div
            className="fixed inset-0 z-[60] bg-black/70"
            onClick={() => setIgnoredModalOpen(false)}
          />
          <div className="fixed z-[61] left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 w-[min(980px,92vw)] h-[min(680px,88vh)] rounded-sm border border-eve-border bg-eve-panel shadow-eve-glow-strong p-3 flex flex-col">
            <div className="flex items-center justify-between gap-3">
              <div>
                <h3 className="text-sm uppercase tracking-wider text-eve-text font-semibold">
                  {t("hiddenDealsTitle")}
                </h3>
                <p className="text-[11px] text-eve-dim mt-0.5">
                  {t("hiddenSummary", {
                    done: hiddenCounts.done,
                    ignored: hiddenCounts.ignored,
                    total: hiddenCounts.total,
                  })}
                </p>
              </div>
              <button
                type="button"
                onClick={() => setIgnoredModalOpen(false)}
                className="px-2 py-1 rounded-sm border border-eve-border/60 text-eve-dim hover:text-eve-accent hover:border-eve-accent/50 transition-colors text-xs"
              >
                {t("close")}
              </button>
            </div>

            <div className="mt-3 flex flex-wrap items-center gap-2">
              <input
                value={ignoredSearch}
                onChange={(e) => setIgnoredSearch(e.target.value)}
                placeholder={t("hiddenSearchItemOrStation")}
                className="h-8 px-2 min-w-[240px] rounded-sm border border-eve-border bg-eve-input text-eve-text text-xs"
              />
              <div className="flex items-center gap-1">
                {(["all", "done", "ignored"] as HiddenFilterTab[]).map((tab) => (
                  <button
                    key={tab}
                    type="button"
                    onClick={() => setIgnoredTab(tab)}
                    className={`px-2 py-1 rounded-sm border text-xs uppercase tracking-wide transition-colors ${
                      ignoredTab === tab
                        ? "border-eve-accent text-eve-accent bg-eve-accent/10"
                        : "border-eve-border/60 text-eve-dim hover:border-eve-accent/40 hover:text-eve-text"
                    }`}
                  >
                    {tab === "all"
                      ? t("hiddenFilterAll")
                      : tab === "done"
                        ? t("hiddenFilterDone")
                        : t("hiddenFilterIgnored")}
                  </button>
                ))}
              </div>
              <div className="flex-1" />
              <button
                type="button"
                onClick={() => {
                  void unhideRowsByKeys([...ignoredSelectedKeys]);
                }}
                disabled={ignoredSelectedKeys.size === 0}
                className="px-2 py-1 rounded-sm border border-eve-accent/60 text-eve-accent hover:bg-eve-accent/10 transition-colors text-xs disabled:opacity-40 disabled:cursor-not-allowed"
              >
                {t("hiddenUnignoreSelected")}
              </button>
              <button
                type="button"
                onClick={() => {
                  void clearDoneHiddenRows();
                }}
                disabled={hiddenCounts.done === 0}
                className="px-2 py-1 rounded-sm border border-eve-border/60 text-eve-text hover:border-eve-accent/40 hover:text-eve-accent transition-colors text-xs disabled:opacity-40 disabled:cursor-not-allowed"
              >
                {t("hiddenClearDone")}
              </button>
              <button
                type="button"
                onClick={() => {
                  void clearAllHiddenRows();
                }}
                disabled={hiddenCounts.total === 0}
                className="px-2 py-1 rounded-sm border border-red-500/50 text-red-300 hover:bg-red-500/10 transition-colors text-xs disabled:opacity-40 disabled:cursor-not-allowed"
              >
                {t("hiddenClearAll")}
              </button>
            </div>

            <div className="mt-3 flex-1 min-h-0 border border-eve-border/60 rounded-sm overflow-auto eve-scrollbar">
              {filteredHiddenEntries.length > 0 ? (
                <table className="w-full text-xs">
                  <thead className="sticky top-0 bg-eve-dark/95 border-b border-eve-border/60">
                    <tr>
                      <th className="w-8 px-2 py-1 text-center">
                        <input
                          type="checkbox"
                          checked={
                            filteredHiddenEntries.length > 0 &&
                            filteredHiddenEntries.every((entry) =>
                              ignoredSelectedKeys.has(entry.key),
                            )
                          }
                          onChange={(e) => {
                            if (!e.target.checked) {
                              setIgnoredSelectedKeys(new Set());
                              return;
                            }
                            setIgnoredSelectedKeys(
                              new Set(filteredHiddenEntries.map((entry) => entry.key)),
                            );
                          }}
                          className="accent-eve-accent"
                        />
                      </th>
                      <th className="px-2 py-1 text-left text-eve-dim uppercase tracking-wide">
                        {t("colItem")}
                      </th>
                      <th className="px-2 py-1 text-left text-eve-dim uppercase tracking-wide">
                        {t("hiddenColRoute")}
                      </th>
                      <th className="px-2 py-1 text-left text-eve-dim uppercase tracking-wide">
                        {t("colType")}
                      </th>
                      <th className="px-2 py-1 text-left text-eve-dim uppercase tracking-wide">
                        {t("updated")}
                      </th>
                      <th className="px-2 py-1 text-right text-eve-dim uppercase tracking-wide">
                        {t("orderDeskAction")}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {filteredHiddenEntries.map((entry, idx) => (
                      <tr
                        key={entry.key}
                        className={`border-b border-eve-border/30 ${
                          idx % 2 === 0 ? "bg-eve-panel" : "bg-eve-dark"
                        }`}
                      >
                        <td className="px-2 py-1 text-center">
                          <input
                            type="checkbox"
                            checked={ignoredSelectedKeys.has(entry.key)}
                            onChange={(e) => {
                              setIgnoredSelectedKeys((prev) => {
                                const next = new Set(prev);
                                if (e.target.checked) next.add(entry.key);
                                else next.delete(entry.key);
                                return next;
                              });
                            }}
                            className="accent-eve-accent"
                          />
                        </td>
                        <td className="px-2 py-1 text-eve-text">{entry.typeName}</td>
                        <td className="px-2 py-1 text-eve-dim truncate">
                          {`${entry.buyStation} -> ${entry.sellStation}`}
                        </td>
                        <td className="px-2 py-1">
                          <span
                            className={`inline-flex items-center px-1.5 py-0.5 rounded-sm border text-[10px] uppercase tracking-wide ${
                              entry.mode === "ignored"
                                ? "border-red-500/40 text-red-300 bg-red-950/30"
                                : "border-eve-accent/40 text-eve-accent bg-eve-accent/10"
                            }`}
                          >
                            {entry.mode === "ignored"
                              ? t("hiddenFilterIgnored")
                              : t("hiddenFilterDone")}
                          </span>
                        </td>
                        <td className="px-2 py-1 text-eve-dim font-mono">
                          {new Date(entry.updatedAt).toLocaleString()}
                        </td>
                        <td className="px-2 py-1 text-right">
                          <button
                            type="button"
                            onClick={() => {
                              void unhideRowsByKeys([entry.key]);
                            }}
                            className="px-2 py-0.5 rounded-sm border border-eve-accent/60 text-eve-accent hover:bg-eve-accent/10 transition-colors text-[11px]"
                          >
                            {t("hiddenUnignore")}
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              ) : (
                <div className="h-full flex items-center justify-center text-eve-dim text-xs">
                  {t("hiddenNoRowsForFilter")}
                </div>
              )}
            </div>
          </div>
        </>
      )}

      {dayDetailRow && (
        <DayDetailPanel row={dayDetailRow} onClose={() => setDayDetailRow(null)} />
      )}

      <ExecutionPlannerPopup
        open={execPlanRow !== null}
        onClose={() => setExecPlanRow(null)}
        typeID={execPlanRow?.TypeID ?? 0}
        typeName={execPlanRow?.TypeName ?? ""}
        regionID={execPlanRow?.BuyRegionID ?? 0}
        locationID={execPlanRow?.BuyLocationID ?? 0}
        sellRegionID={execPlanRow?.SellRegionID}
        sellLocationID={execPlanRow?.SellLocationID ?? 0}
        defaultQuantity={execPlanRow?.UnitsToBuy ?? 100}
        brokerFeePercent={brokerFeePercent}
        salesTaxPercent={salesTaxPercent}
        splitTradeFees={splitTradeFees}
        buyBrokerFeePercent={buyBrokerFeePercent}
        sellBrokerFeePercent={sellBrokerFeePercent}
        buySalesTaxPercent={buySalesTaxPercent}
        sellSalesTaxPercent={sellSalesTaxPercent}
      />

      <BatchBuilderPopup
        open={batchPlanRow !== null}
        onClose={() => setBatchPlanRow(null)}
        anchorRow={batchPlanRow}
        rows={results}
        defaultCargoM3={cargoLimit}
      />
    </div>
  );
}

/* ─── DataRow memo component — renders a single table row ─── */

interface DataRowProps {
  ir: IndexedRow;
  globalIdx: number;
  columnDefs: ColumnDef[];
  compactMode: boolean;
  isPinned: boolean;
  isSelected: boolean;
  variant: { index: number; total: number } | undefined;
  rowHidden: HiddenFlipEntry | undefined;
  isItemGrouped: boolean;
  isRegionGrouped: boolean;
  variantExpandable: boolean;
  variantExpanded: boolean;
  onToggleVariantGroup: (typeID: number) => void;
  onContextMenu: (e: import("react").MouseEvent, id: number, row: FlipResult) => void;
  onLmbClick: (row: FlipResult) => void;
  onToggleSelect: (id: number) => void;
  onTogglePin: (id: number) => void;
  tFn: (key: TranslationKey, vars?: Record<string, string | number>) => string;
}

/* ─── Trade Score Badge ─── */
function TradeScoreBadge({ score }: { score: number }) {
  const s = Math.round(score);
  const color =
    s >= 70 ? "text-green-300" : s >= 40 ? "text-yellow-300" : "text-red-400";
  const bar = Math.round((s / 100) * 10); // 0-10 blocks
  const filled = "█".repeat(bar);
  const empty = "░".repeat(10 - bar);
  return (
    <span
      className={`font-mono tabular-nums ${color}`}
      title={`Trade Score: ${s}/100\n${filled}${empty}`}
    >
      {s}
    </span>
  );
}

const DataRow = memo(
  function DataRow({
    ir, globalIdx, columnDefs, compactMode,
    isPinned, isSelected, variant, rowHidden,
    isItemGrouped, isRegionGrouped, variantExpandable, variantExpanded,
    onToggleVariantGroup, onContextMenu, onLmbClick,
    onToggleSelect, onTogglePin, tFn,
  }: DataRowProps) {
    return (
      <tr
        onContextMenu={(e) => onContextMenu(e, ir.id, ir.row)}
        onClick={(e) => {
          if (!isRegionGrouped) return;
          const target = e.target as HTMLElement;
          if (target.closest("input,button,a")) return;
          onLmbClick(ir.row);
        }}
        className={`border-b border-eve-border/50 hover:bg-eve-accent/5 transition-colors cursor-pointer ${compactMode ? "text-xs" : ""} ${
          isPinned
            ? "bg-eve-accent/10 border-l-2 border-l-eve-accent"
            : isSelected
              ? "bg-eve-accent/5"
              : globalIdx % 2 === 0
                ? "bg-eve-panel"
                : "bg-eve-dark"
        } ${rowHidden ? "opacity-60" : ""}`}
      >
        <td className={`w-8 px-1 text-center ${compactMode ? "py-1" : "py-1.5"}`}>
          <input
            type="checkbox"
            checked={isSelected}
            onChange={() => onToggleSelect(ir.id)}
            className="accent-eve-accent cursor-pointer"
          />
        </td>
        <td className={`w-8 px-1 text-center ${compactMode ? "py-1" : "py-1.5"}`}>
          <button
            onClick={() => onTogglePin(ir.id)}
            className={`text-xs cursor-pointer transition-opacity ${isPinned ? "opacity-100" : "opacity-30 hover:opacity-70"}`}
            title={isPinned ? tFn("unpinRow") : tFn("pinRow")}
          >
            📌
          </button>
        </td>
        {columnDefs.map((col) => (
          <td
            key={col.key}
            className={`px-3 ${compactMode ? "py-1" : "py-1.5"} ${col.width} ${col.key === "TypeName" ? "" : "truncate"} ${col.numeric ? "text-eve-accent font-mono" : "text-eve-text"}`}
          >
            {col.key === "TypeName" ? (
              <div className="flex items-center gap-1.5 min-w-0">
                <span className="truncate">{ir.row.TypeName}</span>
                {/* Price-spike warning: now-profit > 0 but period-profit < 0 means temp spike */}
                {(ir.row.DayNowProfit ?? 0) > 0 && (ir.row.DayPeriodProfit ?? 0) < 0 && (
                  <span
                    title="Price spike: current profit looks positive but historical average is below break-even. This trade may be risky."
                    className="shrink-0 inline-flex items-center px-1 py-px rounded-[2px] border border-yellow-400/50 bg-yellow-400/10 text-yellow-300 text-[9px] leading-none font-medium uppercase"
                  >
                    SPIKE
                  </span>
                )}
                {variant && (
                  variantExpandable && isItemGrouped ? (
                    <button
                      type="button"
                      onClick={(e) => {
                        e.stopPropagation();
                        onToggleVariantGroup(ir.row.TypeID);
                      }}
                      title={tFn("variantChipHint")}
                      className="shrink-0 inline-flex items-center gap-1 px-1 py-px rounded-[2px] border border-eve-accent/35 bg-eve-accent/10 text-eve-accent text-[9px] leading-none font-medium uppercase tracking-normal hover:border-eve-accent/70 hover:bg-eve-accent/20 transition-colors"
                    >
                      <span>
                        {tFn("variantChip", {
                          index: variant.index,
                          total: variant.total,
                        })}
                      </span>
                      <span className="text-[8px]">
                        {variantExpanded ? "▼" : "▶"}
                      </span>
                    </button>
                  ) : (
                    <span
                      title={tFn("variantChipHint")}
                      className="shrink-0 inline-flex items-center px-1 py-px rounded-[2px] border border-eve-accent/35 bg-eve-accent/10 text-eve-accent text-[9px] leading-none font-medium uppercase tracking-normal"
                    >
                      {tFn("variantChip", { index: variant.index, total: variant.total })}
                    </span>
                  )
                )}
              </div>
            ) : col.key === "DayTradeScore" ? (
              <TradeScoreBadge score={ir.row.DayTradeScore ?? 0} />
            ) : (
              fmtCell(col, ir.row)
            )}
          </td>
        ))}
      </tr>
    );
  },
  // Custom equality: only re-render if this row's relevant props changed
  (prev, next) =>
    prev.isPinned === next.isPinned &&
    prev.isSelected === next.isSelected &&
    prev.rowHidden === next.rowHidden &&
    prev.globalIdx === next.globalIdx &&
    prev.compactMode === next.compactMode &&
    prev.variant === next.variant &&
    prev.variantExpandable === next.variantExpandable &&
    prev.variantExpanded === next.variantExpanded &&
    prev.columnDefs === next.columnDefs &&
    prev.ir === next.ir &&
    prev.isItemGrouped === next.isItemGrouped &&
    prev.isRegionGrouped === next.isRegionGrouped &&
    prev.onToggleVariantGroup === next.onToggleVariantGroup &&
    prev.onContextMenu === next.onContextMenu &&
    prev.onLmbClick === next.onLmbClick &&
    prev.onToggleSelect === next.onToggleSelect &&
    prev.onTogglePin === next.onTogglePin,
);

/* ─── EVE category name lookup ─── */

function eveCategoryName(id: number): string {
  const MAP: Record<number, string> = {
    2:  "Celestial",
    4:  "Material",
    5:  "Accessories",
    6:  "Ship",
    7:  "Module",
    8:  "Charge",
    9:  "Blueprint",
    16: "Skill",
    17: "Commodity",
    18: "Drone",
    20: "Implant",
    22: "Deployable",
    23: "Structure",
    24: "Reaction",
    25: "Ore",
    30: "Apparel",
    32: "Structure Module",
    34: "Ancient Relic",
    35: "Decryptor",
    39: "Infrastructure",
    43: "PI Commodity",
    46: "Apparel",
    65: "Structure",
    66: "Struct. Module",
    87: "Fighter",
    91: "Struct. Component",
  };
  return MAP[id] ?? "";
}

/* ─── Regional Day Trader detail panel (LMB on row) ─── */

function DRRow({ label, value, accent, dim }: { label: string; value: string; accent?: boolean; dim?: boolean }) {
  return (
    <div className="flex items-center justify-between gap-2 text-xs">
      <span className="text-eve-dim">{label}</span>
      <span className={accent ? "text-eve-accent font-semibold" : dim ? "text-yellow-400" : "text-eve-text"}>{value}</span>
    </div>
  );
}

function calcConfidence(row: FlipResult): { score: number; label: string; color: string; hint: string } {
  const dos = row.DayTargetDOS ?? 0;
  const demand = row.DayTargetDemandPerDay ?? 0;
  const srcPrice = row.DaySourceAvgPrice ?? row.BuyPrice ?? 0;
  const tgtNow = row.DayTargetNowPrice ?? row.SellPrice ?? 0;
  const hasPeriodPrice = (row.DayTargetPeriodPrice ?? 0) > 0;
  let score = 100;
  const reasons: string[] = [];
  if (!hasPeriodPrice) { score -= 30; reasons.push("no period history"); }
  if (dos > 90)        { score -= 40; reasons.push(`DOS ${dos.toFixed(0)}d saturated`); }
  else if (dos > 30)   { score -= 15; reasons.push(`DOS ${dos.toFixed(0)}d elevated`); }
  if (demand > 0 && demand < 1) { score -= 30; reasons.push(`demand ${demand.toFixed(2)}/day thin`); }
  else if (demand > 0 && demand < 3) { score -= 12; reasons.push(`demand ${demand.toFixed(2)}/day moderate`); }
  if (srcPrice > 0 && tgtNow > 0 && (tgtNow - srcPrice) / srcPrice > 2) { score -= 25; reasons.push("spread >200%"); }
  score = Math.max(0, Math.min(100, score));
  const label = score >= 75 ? "High" : score >= 45 ? "Medium" : "Low";
  const color = score >= 75 ? "text-green-300 border-green-500/60 bg-green-900/20"
              : score >= 45 ? "text-yellow-300 border-yellow-500/60 bg-yellow-900/20"
                            : "text-red-300 border-red-500/60 bg-red-900/20";
  const hint = reasons.length > 0 ? `Score ${score}/100 — ${reasons.join("; ")}` : `Score ${score}/100 — no risk factors detected`;
  return { score, label, color, hint };
}

function DayDetailPanel({ row, onClose }: { row: FlipResult; onClose: () => void }) {
  const signals: { label: string; title: string }[] = [];
  const dos = row.DayTargetDOS ?? 0;
  const demand = row.DayTargetDemandPerDay ?? 0;
  if (dos > 90) signals.push({ label: "SAT", title: `Saturated: ${dos.toFixed(0)} days of supply` });
  if (demand > 0 && demand < 1) signals.push({ label: "LOW", title: `Low demand: ${demand.toFixed(2)} units/day` });
  const srcPrice = row.DaySourceAvgPrice ?? row.BuyPrice ?? 0;
  const tgtNow = row.DayTargetNowPrice ?? row.SellPrice ?? 0;
  if (srcPrice > 0 && tgtNow > 0 && (tgtNow - srcPrice) / srcPrice > 2)
    signals.push({ label: "ODD", title: `Spread ${(((tgtNow - srcPrice) / srcPrice) * 100).toFixed(0)}% — verify prices` });

  const confidence = calcConfidence(row);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60" onClick={onClose}>
      <div
        className="bg-eve-panel border border-eve-border rounded-sm shadow-2xl w-full max-w-lg mx-4 font-mono text-xs text-eve-text"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between px-3 py-2 border-b border-eve-border bg-eve-dark/60">
          <div className="flex items-center gap-2 min-w-0">
            <span className="font-semibold text-sm text-eve-accent truncate">{row.TypeName}</span>
            {row.DayGroupName && (
              <span className="text-[10px] text-eve-dim shrink-0">{row.DayGroupName}</span>
            )}
          </div>
          <div className="flex items-center gap-2 ml-3 shrink-0">
            <span
              className={`px-1.5 py-0.5 rounded-sm border text-[10px] font-bold cursor-help ${confidence.color}`}
              title={confidence.hint}
            >
              {confidence.label} {confidence.score}
            </span>
            <button onClick={onClose} className="text-eve-dim hover:text-eve-text">✕</button>
          </div>
        </div>

        <div className="p-3 space-y-3">
          {signals.length > 0 && (
            <div className="flex flex-wrap gap-1.5">
              {signals.map((s, i) => (
                <span key={i} title={s.title}
                  className="px-1.5 py-0.5 rounded-sm border border-yellow-500/60 text-yellow-400 bg-yellow-900/20 text-[10px] font-bold cursor-help">
                  ⚠ {s.label}
                </span>
              ))}
            </div>
          )}

          <div className="flex items-center gap-2 text-eve-dim flex-wrap">
            <span className="text-eve-text">{row.BuyStation || row.BuySystemName}</span>
            <span className="text-eve-accent">→ {row.SellJumps ?? row.TotalJumps ?? 0}j →</span>
            <span className="text-eve-text">{row.SellStation || row.SellSystemName}</span>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div className="rounded-sm border border-eve-border/50 bg-eve-dark/30 p-2 space-y-1">
              <div className="text-[10px] uppercase tracking-wider text-eve-dim font-semibold mb-1">Source (Buy)</div>
              <DRRow label="Price" value={formatISK(srcPrice)} />
              <DRRow label="Available" value={`${(row.SellOrderRemain ?? 0).toLocaleString()} units`} />
              <DRRow label="Region" value={row.BuyRegionName ?? "—"} />
            </div>
            <div className="rounded-sm border border-eve-border/50 bg-eve-dark/30 p-2 space-y-1">
              <div className="text-[10px] uppercase tracking-wider text-eve-dim font-semibold mb-1">Target (Sell)</div>
              <DRRow label="Now price" value={formatISK(tgtNow)} />
              <DRRow label="Period avg" value={formatISK(row.DayTargetPeriodPrice ?? 0)} />
              {(row.DayTargetLowestSell ?? 0) > 0 && (
                <DRRow label="Lowest ask" value={formatISK(row.DayTargetLowestSell ?? 0)} />
              )}
              <DRRow label="Demand/Day" value={(demand).toFixed(2)} />
              <DRRow label="Supply" value={(row.DayTargetSupplyUnits ?? 0).toLocaleString()} />
              <DRRow label="DOS" value={`${dos.toFixed(2)} days`} dim={dos > 30} />
            </div>
          </div>

          <div className="rounded-sm border border-eve-border/50 bg-eve-dark/30 p-2 space-y-1">
            <div className="text-[10px] uppercase tracking-wider text-eve-dim font-semibold mb-1">
              Position ({(row.UnitsToBuy ?? 0).toLocaleString()} units · {(row.Volume ?? 0).toFixed(2)} m³/unit)
            </div>
            <div className="grid grid-cols-2 gap-x-4">
              <DRRow label="Capital" value={formatISK(row.DayCapitalRequired ?? 0)} />
              <DRRow label="Shipping" value={formatISK(row.DayShippingCost ?? 0)} />
              <DRRow label="Now Profit" value={formatISK(row.DayNowProfit ?? row.TotalProfit ?? 0)} accent={(row.DayNowProfit ?? 0) > 0} />
              <DRRow label="Period Profit" value={formatISK(row.DayPeriodProfit ?? row.RealProfit ?? 0)} accent={(row.DayPeriodProfit ?? 0) > 0} />
              <DRRow label="ROI Now" value={formatMargin(row.DayROINow ?? 0)} accent={(row.DayROINow ?? 0) > 0} />
              <DRRow label="ROI Period" value={formatMargin(row.DayROIPeriod ?? 0)} accent={(row.DayROIPeriod ?? 0) > 0} />
              <DRRow label="Margin" value={formatMargin(row.MarginPercent ?? 0)} />
            </div>
          </div>

          {((row.DayAssets ?? 0) > 0 || (row.DayActiveOrders ?? 0) > 0) && (
            <div className="rounded-sm border border-eve-border/50 bg-eve-dark/30 p-2 space-y-1">
              <div className="text-[10px] uppercase tracking-wider text-eve-dim font-semibold mb-1">Inventory Coverage</div>
              {(row.DayAssets ?? 0) > 0 && <DRRow label="Assets in target" value={`${(row.DayAssets ?? 0).toLocaleString()} units`} />}
              {(row.DayActiveOrders ?? 0) > 0 && <DRRow label="Active sell orders" value={`${(row.DayActiveOrders ?? 0).toLocaleString()} units`} />}
            </div>
          )}

          {/* Trade Score + Price Spark-line */}
          <div className="rounded-sm border border-eve-border/50 bg-eve-dark/30 p-2 space-y-2">
            <div className="flex items-center justify-between">
              <div className="text-[10px] uppercase tracking-wider text-eve-dim font-semibold">
                Trade Score
              </div>
              {row.DayTradeScore != null && (
                <TradeScoreBadge score={row.DayTradeScore} />
              )}
            </div>
            {row.DayPriceHistory && row.DayPriceHistory.length >= 2 && (
              <PriceSparkLine prices={row.DayPriceHistory} />
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

/* ─── SVG Spark-line chart for 14-day price history ─── */
function PriceSparkLine({ prices }: { prices: number[] }) {
  const W = 380;
  const H = 48;
  const pad = 4;
  const innerW = W - pad * 2;
  const innerH = H - pad * 2;

  const min = Math.min(...prices);
  const max = Math.max(...prices);
  const range = max - min || 1;

  const pts = prices.map((p, i) => {
    const x = pad + (i / (prices.length - 1)) * innerW;
    const y = pad + innerH - ((p - min) / range) * innerH;
    return `${x.toFixed(1)},${y.toFixed(1)}`;
  });
  const polyline = pts.join(" ");
  const lastX = parseFloat(pts[pts.length - 1].split(",")[0]);
  const lastY = parseFloat(pts[pts.length - 1].split(",")[1]);

  // fill area under curve
  const fillPts = `${pad},${H - pad} ` + polyline + ` ${lastX},${H - pad}`;

  const trend = prices[prices.length - 1] - prices[0];
  const lineColor = trend >= 0 ? "#4ade80" : "#f87171"; // green / red

  return (
    <div>
      <svg width="100%" viewBox={`0 0 ${W} ${H}`} className="overflow-visible">
        <defs>
          <linearGradient id="sparkFill" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={lineColor} stopOpacity="0.3" />
            <stop offset="100%" stopColor={lineColor} stopOpacity="0.02" />
          </linearGradient>
        </defs>
        <polygon points={fillPts} fill="url(#sparkFill)" />
        <polyline
          points={polyline}
          fill="none"
          stroke={lineColor}
          strokeWidth="1.5"
          strokeLinejoin="round"
          strokeLinecap="round"
        />
        {/* last dot */}
        <circle cx={lastX} cy={lastY} r="3" fill={lineColor} />
      </svg>
      <div className="flex justify-between text-[9px] text-eve-dim mt-0.5 px-1">
        <span>{prices.length}d ago</span>
        <span className="text-eve-text font-mono">
          {formatISK(min)} – {formatISK(max)}
        </span>
        <span>today</span>
      </div>
    </div>
  );
}

/* ─── Small reusable pieces ─── */

function ToolbarBtn({
  label,
  title,
  active,
  onClick,
}: {
  label: string;
  title: string;
  active?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      onClick={onClick}
      title={title}
      className={`px-2 py-0.5 rounded-sm text-xs font-medium transition-colors cursor-pointer ${
        active
          ? "bg-eve-accent/20 text-eve-accent border border-eve-accent/30"
          : "text-eve-dim hover:text-eve-text border border-eve-border hover:border-eve-border-light"
      }`}
    >
      {label}
    </button>
  );
}

function ContextItem({
  label,
  onClick,
}: {
  label: string;
  onClick: () => void;
}) {
  return (
    <div
      onClick={onClick}
      className="px-4 py-1.5 text-sm text-eve-text hover:bg-eve-accent/20 hover:text-eve-accent cursor-pointer transition-colors"
    >
      {label}
    </div>
  );
}
