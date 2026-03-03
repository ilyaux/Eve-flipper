import { useState, useEffect, useMemo, useCallback, useRef } from "react";
import type {
  OrderDeskOrder,
  StationCacheMeta,
  StationAIScanSnapshot,
  StationCommandRow,
  StationCommandSummary,
  StationTrade,
  StationInfo,
  ScanParams,
  WatchlistItem,
} from "@/lib/types";
import {
  clearStationTradeStates,
  deleteStationTradeStates,
  getStationCommand,
  getStations,
  getStationTradeStates,
  getStructures,
  rebootStationCache,
  scanStation,
  setStationTradeState,
  getWatchlist,
  addToWatchlist,
  removeFromWatchlist,
  openMarketInGame,
  setWaypointInGame,
} from "@/lib/api";
import { formatISK, formatMargin, formatNumber } from "@/lib/format";
import { useI18n, type TranslationKey } from "@/lib/i18n";
import { MetricTooltip } from "./Tooltip";
import { EmptyState } from "./EmptyState";
import { StationTradingExecutionCalculator } from "./StationTradingExecutionCalculator";
import { useGlobalToast } from "./Toast";
import { handleEveUIError } from "@/lib/handleEveUIError";
import {
  TabSettingsPanel,
  SettingsField,
  SettingsNumberInput,
  SettingsCheckbox,
  SettingsSelect,
} from "./TabSettingsPanel";
import { SystemAutocomplete } from "./SystemAutocomplete";
import { PresetPicker } from "./PresetPicker";
import { StationAIAssistant } from "./StationAIAssistant";
import { SystemBlacklistButton } from "./SystemBlacklistButton";
import {
  STATION_BUILTIN_PRESETS,
  type StationTradingSettings,
} from "@/lib/presets";

type SortKey = keyof StationTrade;
type SortDir = "asc" | "desc";
type CTSProfile = "balanced" | "aggressive" | "defensive";
type BatchPreset = "safe" | "balanced" | "aggressive";
type HiddenTradeMode = "done" | "ignored";
type HiddenFilterTab = "all" | "done" | "ignored";

type StationBatchRow = {
  key: string;
  trade: StationTrade;
  command: StationCommandRow;
  desk: OrderDeskOrder[];
};

type PlannedStationBatchRow = StationBatchRow & {
  plannedAction: StationCommandRow["recommended_action"];
};

type HiddenTradeEntry = {
  key: string;
  typeID: number;
  typeName: string;
  stationID: number;
  stationName: string;
  regionID: number;
  mode: HiddenTradeMode;
  updatedAt: string;
};

type CacheMetaView = {
  currentRevision: number;
  lastRefreshAt: number;
  nextExpiryAt: number;
  scopeLabel: string;
  regionCount: number;
};

interface Props {
  params: ScanParams;
  /** Called when system (or other global param) is changed in this tab; updates global filter */
  onChange?: (params: ScanParams) => void;
  isLoggedIn?: boolean;
  /** Results loaded externally (e.g. from history); component will display them */
  loadedResults?: StationTrade[] | null;
}

// Metric tooltip keys mapping
type MetricTooltipKey =
  | "CTS"
  | "SDS"
  | "PVI"
  | "VWAP"
  | "OBDS"
  | "DOS"
  | "S2BBfSRatio"
  | "PeriodROI"
  | "NowROI";

const metricTooltipKeys: Partial<Record<SortKey, MetricTooltipKey>> = {
  CTS: "CTS",
  SDS: "SDS",
  PVI: "PVI",
  VWAP: "VWAP",
  OBDS: "OBDS",
  DOS: "DOS",
  S2BBfSRatio: "S2BBfSRatio",
  PeriodROI: "PeriodROI",
  NowROI: "NowROI",
};

const columnDefs: {
  key: SortKey;
  labelKey: TranslationKey;
  width: string;
  numeric: boolean;
}[] = [
  {
    key: "TypeName",
    labelKey: "colItem",
    width: "min-w-[150px]",
    numeric: false,
  },
  {
    key: "StationName",
    labelKey: "colStationName",
    width: "min-w-[150px]",
    numeric: false,
  },
  { key: "CTS", labelKey: "colCTS", width: "min-w-[60px]", numeric: true },
  {
    key: "ProfitPerUnit",
    labelKey: "colProfitPerUnit",
    width: "min-w-[90px]",
    numeric: true,
  },
  {
    key: "MarginPercent",
    labelKey: "colMargin",
    width: "min-w-[70px]",
    numeric: true,
  },
  {
    key: "PeriodROI",
    labelKey: "colPeriodROI",
    width: "min-w-[80px]",
    numeric: true,
  },
  {
    key: "S2BPerDay",
    labelKey: "colS2BPerDay",
    width: "min-w-[80px]",
    numeric: true,
  },
  {
    key: "BfSPerDay",
    labelKey: "colBfSPerDay",
    width: "min-w-[80px]",
    numeric: true,
  },
  {
    key: "S2BBfSRatio",
    labelKey: "colS2BBfSRatio",
    width: "min-w-[90px]",
    numeric: true,
  },
  { key: "DOS", labelKey: "colDOS", width: "min-w-[60px]", numeric: true },
  { key: "SDS", labelKey: "colSDS", width: "min-w-[50px]", numeric: true },
  {
    key: "DailyProfit",
    labelKey: "colDailyProfit",
    width: "min-w-[100px]",
    numeric: true,
  },
];

// Sentinel value for "All stations"
const ALL_STATIONS_ID = 0;
const STATION_PAGE_SIZE = 100;
const OPERATOR_PANEL_STORAGE_KEY = "station.operator_panel_width";
const OPERATOR_PANEL_COLLAPSED_KEY = "station.operator_panel_collapsed";
const OPERATOR_PANEL_MIN = 28;
const OPERATOR_PANEL_MAX = 62;
const OPERATOR_PANEL_DEFAULT = 50;
const STATION_CACHE_TTL_MS = 20 * 60 * 1000;
const settingsSectionClass =
  "rounded-sm border border-eve-border/60 bg-gradient-to-br from-eve-panel to-eve-dark/40";

function clampOperatorPanelWidth(width: number): number {
  if (!Number.isFinite(width)) return OPERATOR_PANEL_DEFAULT;
  return Math.max(OPERATOR_PANEL_MIN, Math.min(OPERATOR_PANEL_MAX, width));
}

function stationDailyProfit(row: StationTrade): number {
  // TotalProfit is full-book notional, not a daily metric — excluded from cascade.
  return (
    row.DailyProfit ??
    row.RealizableDailyProfit ??
    row.RealProfit ??
    row.TheoreticalDailyProfit ??
    0
  );
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
  meta: StationCacheMeta | undefined,
  fallbackScope: string,
  fallbackRegionCount: number,
): CacheMetaView {
  const now = Date.now();
  if (!meta) {
    return {
      currentRevision: Math.floor(now / 1000),
      lastRefreshAt: now,
      nextExpiryAt: now + STATION_CACHE_TTL_MS,
      scopeLabel: fallbackScope,
      regionCount: fallbackRegionCount,
    };
  }
  const nextExpiryTs = meta.next_expiry_at
    ? Date.parse(meta.next_expiry_at)
    : now + Math.max(60, meta.min_ttl_sec || 60) * 1000;
  const lastRefreshTs = meta.last_refresh_at
    ? Date.parse(meta.last_refresh_at)
    : now;
  return {
    currentRevision:
      meta.current_revision && Number.isFinite(meta.current_revision)
        ? meta.current_revision
        : Math.floor(nextExpiryTs / 1000),
    lastRefreshAt: Number.isFinite(lastRefreshTs) ? lastRefreshTs : now,
    nextExpiryAt: Number.isFinite(nextExpiryTs) ? nextExpiryTs : now + STATION_CACHE_TTL_MS,
    scopeLabel: fallbackScope,
    regionCount: Math.max(1, fallbackRegionCount),
  };
}

function rowRegionID(row: StationTrade, fallbackRegionID: number): number {
  return row.RegionID && row.RegionID > 0 ? row.RegionID : fallbackRegionID;
}

function rowSystemID(row: StationTrade, fallbackSystemID: number): number {
  return row.SystemID && row.SystemID > 0 ? row.SystemID : fallbackSystemID;
}

function normalizeStationResults(rows: StationTrade[]): StationTrade[] {
  return rows.map((r) => ({
    ...r,
    DailyProfit: stationDailyProfit(r),
    S2BPerDay: r.S2BPerDay ?? r.BuyUnitsPerDay ?? 0,
    BfSPerDay: r.BfSPerDay ?? r.SellUnitsPerDay ?? 0,
    S2BBfSRatio:
      r.S2BBfSRatio ??
      r.BvSRatio ??
      ((r.S2BPerDay ?? r.BuyUnitsPerDay ?? 0) > 0 &&
      (r.BfSPerDay ?? r.SellUnitsPerDay ?? 0) > 0
        ? (r.S2BPerDay ?? r.BuyUnitsPerDay ?? 0) /
          (r.BfSPerDay ?? r.SellUnitsPerDay ?? 0)
        : 0),
    HistoryAvailable: r.HistoryAvailable ?? false,
  }));
}

function computePlannedStationAction(
  row: Pick<StationBatchRow, "trade" | "command">,
  preset: BatchPreset,
): StationCommandRow["recommended_action"] {
  const rec = row.command.recommended_action;
  if (preset === "balanced") {
    return rec;
  }

  if (preset === "safe") {
    if (rec === "cancel") return "cancel";
    if (rec === "reprice") {
      if (
        row.command.priority >= 80 &&
        row.command.expected_delta_daily_profit > 0 &&
        row.trade.ConfidenceLabel !== "low"
      ) {
        return "reprice";
      }
      return "hold";
    }
    if (rec === "new_entry") {
      if (
        row.trade.ConfidenceLabel === "high" &&
        row.command.priority >= 85 &&
        row.command.expected_delta_daily_profit > 0
      ) {
        return "new_entry";
      }
      return "hold";
    }
    return "hold";
  }

  // aggressive
  if (rec === "hold") {
    if (
      stationDailyProfit(row.trade) > 0 &&
      row.command.expected_delta_daily_profit > 0 &&
      row.trade.ConfidenceLabel !== "low"
    ) {
      if (row.command.active_order_count > 0 || row.command.open_position_qty > 0) {
        return "reprice";
      }
      return "new_entry";
    }
  }

  return rec;
}

export function StationTrading({
  params,
  onChange,
  isLoggedIn = false,
  loadedResults,
}: Props) {
  const { t } = useI18n();
  const operatorModeDevOnly = import.meta.env.DEV;

  const [stations, setStations] = useState<StationInfo[]>([]);
  const [selectedStationId, setSelectedStationId] =
    useState<number>(ALL_STATIONS_ID);
  const [minMargin, setMinMargin] = useState(params.min_margin ?? 0);
  const [brokerFee, setBrokerFee] = useState(3.0);
  const [salesTaxPercent, setSalesTaxPercent] = useState(8);
  const [splitTradeFees, setSplitTradeFees] = useState(false);
  const [buyBrokerFeePercent, setBuyBrokerFeePercent] = useState(3.0);
  const [sellBrokerFeePercent, setSellBrokerFeePercent] = useState(3.0);
  const [buySalesTaxPercent, setBuySalesTaxPercent] = useState(0);
  const [sellSalesTaxPercent, setSellSalesTaxPercent] = useState(8);
  const [ctsProfile, setCTSProfile] = useState<CTSProfile>("balanced");
  const [radius, setRadius] = useState(0);
  const [minDailyVolume, setMinDailyVolume] = useState(5);
  const [results, setResults] = useState<StationTrade[]>([]);
  const [scanning, setScanning] = useState(false);
  const [progress, setProgress] = useState("");
  const [autoRefreshEnabled, setAutoRefreshEnabled] = useState(false);
  const [loadingStations, setLoadingStations] = useState(false);
  const abortRef = useRef<AbortController | null>(null);
  const scanInFlightRef = useRef(false);
  const autoRefreshSignatureRef = useRef<string>("");
  const autoRefreshLastRunRef = useRef<number>(0);

  // System-level metadata (always available even with no NPC stations)
  const [systemRegionId, setSystemRegionId] = useState<number>(0);
  const [systemId, setSystemId] = useState<number>(0);

  // Player structure support
  const [includeStructures, setIncludeStructures] = useState(false);
  const [structureStations, setStructureStations] = useState<StationInfo[]>([]);
  const [loadingStructures, setLoadingStructures] = useState(false);
  const [operatorMode, setOperatorMode] = useState(false);
  const [commandRowsByKey, setCommandRowsByKey] = useState<
    Record<string, StationCommandRow>
  >({});
  const [orderDeskByKey, setOrderDeskByKey] = useState<
    Record<string, OrderDeskOrder[]>
  >({});
  const [commandSummary, setCommandSummary] =
    useState<StationCommandSummary | null>(null);
  const [selectedBatchKeys, setSelectedBatchKeys] = useState<Set<string>>(
    new Set(),
  );
  const [batchPreset, setBatchPreset] = useState<BatchPreset>("balanced");
  const [operatorPanelWidth, setOperatorPanelWidth] = useState<number>(() => {
    if (typeof window === "undefined") return OPERATOR_PANEL_DEFAULT;
    const stored = Number(window.localStorage.getItem(OPERATOR_PANEL_STORAGE_KEY));
    return clampOperatorPanelWidth(stored);
  });
  const [operatorPanelCollapsed, setOperatorPanelCollapsed] = useState<boolean>(
    () => {
      if (typeof window === "undefined") return false;
      return window.localStorage.getItem(OPERATOR_PANEL_COLLAPSED_KEY) === "1";
    },
  );
  const [operatorSplitDragging, setOperatorSplitDragging] = useState(false);
  const operatorSplitRef = useRef<HTMLDivElement | null>(null);
  const [hiddenTradeMap, setHiddenTradeMap] = useState<
    Record<string, HiddenTradeEntry>
  >({});
  const [showHiddenRows, setShowHiddenRows] = useState(false);
  const [ignoredModalOpen, setIgnoredModalOpen] = useState(false);
  const [ignoredSearch, setIgnoredSearch] = useState("");
  const [ignoredTab, setIgnoredTab] = useState<HiddenFilterTab>("all");
  const [ignoredSelectedKeys, setIgnoredSelectedKeys] = useState<Set<string>>(
    new Set(),
  );
  const [cacheMeta, setCacheMeta] = useState<CacheMetaView | null>(null);
  const [cacheNowTs, setCacheNowTs] = useState<number>(Date.now());
  const [cacheRebooting, setCacheRebooting] = useState(false);

  // EVE Guru Profit Filters
  const [minItemProfit, setMinItemProfit] = useState(0);
  const [minDemandPerDay, setMinDemandPerDay] = useState(1);
  const [minBfSPerDay, setMinBfSPerDay] = useState(0);

  // Risk Profile
  const [avgPricePeriod, setAvgPricePeriod] = useState(90);
  const [minPeriodROI, setMinPeriodROI] = useState(0);
  const [bvsRatioMin, setBvsRatioMin] = useState(0);
  const [bvsRatioMax, setBvsRatioMax] = useState(0);
  const [maxPVI, setMaxPVI] = useState(0);
  const [maxSDS, setMaxSDS] = useState(50);

  // Price Limits
  const [limitBuyToPriceLow, setLimitBuyToPriceLow] = useState(false);
  const [flagExtremePrices, setFlagExtremePrices] = useState(true);
  const [showAdvanced, setShowAdvanced] = useState(false);

  const activeAdvancedCount = useMemo(
    () =>
      Number(minDemandPerDay > 1) +
      Number(minBfSPerDay > 0) +
      Number(ctsProfile !== "balanced") +
      Number(avgPricePeriod !== 90) +
      Number(minPeriodROI > 0) +
      Number(bvsRatioMin > 0) +
      Number(bvsRatioMax > 0) +
      Number(maxPVI > 0) +
      Number(maxSDS < 50) +
      Number(limitBuyToPriceLow) +
      Number(!flagExtremePrices),
    [
      minDemandPerDay,
      minBfSPerDay,
      ctsProfile,
      avgPricePeriod,
      minPeriodROI,
      bvsRatioMin,
      bvsRatioMax,
      maxPVI,
      maxSDS,
      limitBuyToPriceLow,
      flagExtremePrices,
    ],
  );

  // Sort
  const [sortKey, setSortKey] = useState<SortKey>("CTS");
  const [sortDir, setSortDir] = useState<SortDir>("desc");
  const [page, setPage] = useState(0);

  // Execution plan popup
  const [execPlanRow, setExecPlanRow] = useState<StationTrade | null>(null);

  // Context menu (right-click)
  const [contextMenu, setContextMenu] = useState<{
    x: number;
    y: number;
    row: StationTrade;
  } | null>(null);
  const contextMenuRef = useRef<HTMLDivElement>(null);
  const [pinnedKeys, setPinnedKeys] = useState<Set<string>>(new Set());

  // Accept externally loaded results (from history)
  useEffect(() => {
    if (loadedResults !== undefined && loadedResults !== null) {
      setResults(normalizeStationResults(loadedResults));
      setCommandRowsByKey({});
      setOrderDeskByKey({});
      setCommandSummary(null);
      setSelectedBatchKeys(new Set());
    }
  }, [loadedResults]);

  useEffect(() => {
    if ((!isLoggedIn || !operatorModeDevOnly) && operatorMode) {
      setOperatorMode(false);
    }
  }, [isLoggedIn, operatorMode, operatorModeDevOnly]);

  useEffect(() => {
    if (typeof window === "undefined") return;
    window.localStorage.setItem(
      OPERATOR_PANEL_STORAGE_KEY,
      String(operatorPanelWidth),
    );
  }, [operatorPanelWidth]);

  useEffect(() => {
    if (typeof window === "undefined") return;
    window.localStorage.setItem(
      OPERATOR_PANEL_COLLAPSED_KEY,
      operatorPanelCollapsed ? "1" : "0",
    );
  }, [operatorPanelCollapsed]);

  useEffect(
    () => () => {
      document.body.style.userSelect = "";
    },
    [],
  );

  useEffect(() => {
    if (!cacheMeta) return;
    const timer = window.setInterval(() => {
      setCacheNowTs(Date.now());
    }, 1000);
    return () => window.clearInterval(timer);
  }, [cacheMeta]);

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
        if (hiddenTradeMap[key]) {
          next.add(key);
        }
      }
      return next.size === prev.size ? prev : next;
    });
  }, [hiddenTradeMap]);

  // Watchlist
  const { addToast } = useGlobalToast();
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

  // Current settings object for preset system
  const stationSettings = useMemo<StationTradingSettings>(
    () => ({
      systemName: params.system_name,
      selectedStationId,
      includeStructures,
      minMargin,
      brokerFee,
      salesTaxPercent,
      splitTradeFees,
      buyBrokerFeePercent,
      sellBrokerFeePercent,
      buySalesTaxPercent,
      sellSalesTaxPercent,
      ctsProfile,
      radius,
      minDailyVolume,
      minItemProfit,
      minDemandPerDay,
      minBfSPerDay,
      avgPricePeriod,
      minPeriodROI,
      bvsRatioMin,
      bvsRatioMax,
      maxPVI,
      maxSDS,
      limitBuyToPriceLow,
      flagExtremePrices,
    }),
    [
      params.system_name,
      selectedStationId,
      includeStructures,
      minMargin,
      brokerFee,
      salesTaxPercent,
      splitTradeFees,
      buyBrokerFeePercent,
      sellBrokerFeePercent,
      buySalesTaxPercent,
      sellSalesTaxPercent,
      ctsProfile,
      radius,
      minDailyVolume,
      minItemProfit,
      minDemandPerDay,
      minBfSPerDay,
      avgPricePeriod,
      minPeriodROI,
      bvsRatioMin,
      bvsRatioMax,
      maxPVI,
      maxSDS,
      limitBuyToPriceLow,
      flagExtremePrices,
    ],
  );

  const handlePresetApply = useCallback((s: Record<string, any>) => {
    // eslint-disable-line @typescript-eslint/no-explicit-any
    const st = s as StationTradingSettings;
    const nextSystemName = typeof st.systemName === "string" ? st.systemName.trim() : "";
    const systemChanged = Boolean(nextSystemName) && nextSystemName !== params.system_name;
    if (systemChanged) {
      onChange?.({ ...params, system_name: nextSystemName });
    }
    if (st.selectedStationId !== undefined && !systemChanged) {
      setSelectedStationId(st.selectedStationId);
    }
    if (st.includeStructures !== undefined) {
      setIncludeStructures(st.includeStructures);
    }
    if (st.minMargin !== undefined) setMinMargin(st.minMargin);
    if (st.brokerFee !== undefined) setBrokerFee(st.brokerFee);
    if (st.salesTaxPercent !== undefined)
      setSalesTaxPercent(st.salesTaxPercent);
    if (st.splitTradeFees !== undefined) setSplitTradeFees(st.splitTradeFees);
    if (st.buyBrokerFeePercent !== undefined)
      setBuyBrokerFeePercent(st.buyBrokerFeePercent);
    if (st.sellBrokerFeePercent !== undefined)
      setSellBrokerFeePercent(st.sellBrokerFeePercent);
    if (st.buySalesTaxPercent !== undefined)
      setBuySalesTaxPercent(st.buySalesTaxPercent);
    if (st.sellSalesTaxPercent !== undefined)
      setSellSalesTaxPercent(st.sellSalesTaxPercent);
    if (
      st.ctsProfile === "balanced" ||
      st.ctsProfile === "aggressive" ||
      st.ctsProfile === "defensive"
    ) {
      setCTSProfile(st.ctsProfile);
    }
    if (st.radius !== undefined) setRadius(st.radius);
    if (st.minDailyVolume !== undefined) setMinDailyVolume(st.minDailyVolume);
    if (st.minItemProfit !== undefined) setMinItemProfit(st.minItemProfit);
    if (st.minDemandPerDay !== undefined)
      setMinDemandPerDay(st.minDemandPerDay);
    if (st.minBfSPerDay !== undefined) setMinBfSPerDay(st.minBfSPerDay);
    if (st.avgPricePeriod !== undefined) setAvgPricePeriod(st.avgPricePeriod);
    if (st.minPeriodROI !== undefined) setMinPeriodROI(st.minPeriodROI);
    if (st.bvsRatioMin !== undefined) setBvsRatioMin(st.bvsRatioMin);
    if (st.bvsRatioMax !== undefined) setBvsRatioMax(st.bvsRatioMax);
    if (st.maxPVI !== undefined) setMaxPVI(st.maxPVI);
    if (st.maxSDS !== undefined) setMaxSDS(st.maxSDS);
    if (st.limitBuyToPriceLow !== undefined)
      setLimitBuyToPriceLow(st.limitBuyToPriceLow);
    if (st.flagExtremePrices !== undefined)
      setFlagExtremePrices(st.flagExtremePrices);
  }, [onChange, params]);

  // Keep station sales-tax aligned with global params.
  useEffect(() => {
    const pct = params.sales_tax_percent ?? 8;
    setSalesTaxPercent(pct);
  }, [params.sales_tax_percent]);

  // Load stations when system changes
  useEffect(() => {
    if (!params.system_name) return;
    const controller = new AbortController();
    setLoadingStations(true);
    getStations(params.system_name, controller.signal)
      .then((resp) => {
        if (controller.signal.aborted) return;
        setStations(resp.stations);
        setSystemRegionId(resp.region_id);
        setSystemId(resp.system_id);
        setSelectedStationId(ALL_STATIONS_ID);
        setStructureStations([]); // reset structures on system change
      })
      .catch(() => {
        if (controller.signal.aborted) return;
        setStations([]);
        setSystemRegionId(0);
        setSystemId(0);
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoadingStations(false);
      });
    return () => controller.abort();
  }, [params.system_name]);

  // Fetch structures when toggle is enabled
  useEffect(() => {
    if (!includeStructures || !systemId || !systemRegionId) {
      setStructureStations([]);
      return;
    }
    const controller = new AbortController();
    setLoadingStructures(true);
    getStructures(systemId, systemRegionId, controller.signal)
      .then((data) => {
        if (controller.signal.aborted) return;
        setStructureStations(data);
      })
      .catch(() => {
        if (controller.signal.aborted) return;
        setStructureStations([]);
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoadingStructures(false);
      });
    return () => controller.abort();
  }, [includeStructures, systemId, systemRegionId]);

  // Combined stations (NPC + structures when toggle is on)
  const allStations = useMemo(() => {
    if (includeStructures && structureStations.length > 0) {
      return [...stations, ...structureStations];
    }
    return stations;
  }, [stations, structureStations, includeStructures]);

  // If structure view is turned off, keep selection within NPC station scope.
  useEffect(() => {
    if (includeStructures || selectedStationId === ALL_STATIONS_ID) return;
    if (!stations.some((st) => st.id === selectedStationId)) {
      setSelectedStationId(ALL_STATIONS_ID);
    }
  }, [includeStructures, selectedStationId, stations]);

  // Region ID comes from system metadata, not from stations
  const regionId = systemRegionId;

  const canScan =
    params.system_name &&
    (allStations.length > 0 || radius > 0 || selectedStationId === ALL_STATIONS_ID) &&
    regionId > 0;

  function stationRowKey(row: StationTrade) {
    return `${row.TypeID}-${row.StationID}`;
  }

  function stationDeskKey(typeID: number, stationID: number) {
    return `${typeID}-${stationID}`;
  }

  const refreshHiddenStates = useCallback(
    async (currentRevision?: number, knownRows?: StationTrade[]) => {
      try {
        const res = await getStationTradeStates({
          tab: "station",
          currentRevision,
        });
        const states = Array.isArray(res.states) ? res.states : [];
        const rows = knownRows ?? [];
        const rowsByKey = new Map<string, StationTrade>();
        for (const row of rows) {
          rowsByKey.set(stationRowKey(row), row);
        }
        setHiddenTradeMap((prev) => {
          const next: Record<string, HiddenTradeEntry> = {};
          for (const state of states) {
            const key = `${state.type_id}-${state.station_id}`;
            const row = rowsByKey.get(key);
            const prevEntry = prev[key];
            const resolvedRegionID =
              state.region_id > 0
                ? state.region_id
                : row
                  ? rowRegionID(row, regionId)
                  : (prevEntry?.regionID ?? regionId);
            next[key] = {
              key,
              typeID: state.type_id,
              typeName:
                row?.TypeName ?? prevEntry?.typeName ?? `Type ${state.type_id}`,
              stationID: state.station_id,
              stationName:
                row?.StationName ??
                prevEntry?.stationName ??
                `Station ${state.station_id}`,
              regionID: resolvedRegionID,
              mode: state.mode,
              updatedAt: state.updated_at,
            };
          }
          return next;
        });
      } catch {
        // best-effort; keep local state untouched if backend is unavailable
      }
    },
    [regionId],
  );

  useEffect(() => {
    void refreshHiddenStates();
  }, [refreshHiddenStates]);

  const setRowHiddenState = useCallback(
    async (row: StationTrade, mode: HiddenTradeMode) => {
      const key = stationRowKey(row);
      const untilRevision =
        mode === "done"
          ? Math.max(1, cacheMeta?.currentRevision ?? Math.floor(Date.now() / 1000))
          : 0;
      const entry: HiddenTradeEntry = {
        key,
        typeID: row.TypeID,
        typeName: row.TypeName,
        stationID: row.StationID,
        stationName: row.StationName,
        regionID: rowRegionID(row, regionId),
        mode,
        updatedAt: new Date().toISOString(),
      };
      setHiddenTradeMap((prev) => ({ ...prev, [key]: entry }));
      setSelectedBatchKeys((prev) => {
        if (!prev.has(key)) return prev;
        const next = new Set(prev);
        next.delete(key);
        return next;
      });
      setContextMenu(null);
      try {
        await setStationTradeState({
          tab: "station",
          type_id: row.TypeID,
          station_id: row.StationID,
          region_id: rowRegionID(row, regionId),
          mode,
          until_revision: untilRevision,
        });
      } catch {
        addToast("Failed to save hidden trade state", "error", 2600);
        void refreshHiddenStates(cacheMeta?.currentRevision);
      }
    },
    [addToast, cacheMeta?.currentRevision, refreshHiddenStates, regionId],
  );

  const unhideRowsByKeys = useCallback(
    async (keys: string[]) => {
      if (keys.length === 0) return;
      const uniqueKeys = [...new Set(keys)];
      const payload = uniqueKeys
        .map((key) => hiddenTradeMap[key])
        .filter(Boolean)
        .map((entry) => ({
          type_id: entry.typeID,
          station_id: entry.stationID,
          region_id: entry.regionID,
        }));
      setHiddenTradeMap((prev) => {
        let changed = false;
        const next = { ...prev };
        for (const key of uniqueKeys) {
          if (next[key]) {
            delete next[key];
            changed = true;
          }
        }
        return changed ? next : prev;
      });
      setIgnoredSelectedKeys((prev) => {
        if (prev.size === 0) return prev;
        const next = new Set(prev);
        for (const key of uniqueKeys) {
          next.delete(key);
        }
        return next.size === prev.size ? prev : next;
      });
      try {
        if (payload.length > 0) {
          await deleteStationTradeStates({ tab: "station", keys: payload });
        }
      } catch {
        addToast("Failed to unhide trades", "error", 2600);
        void refreshHiddenStates(cacheMeta?.currentRevision);
      }
    },
    [addToast, cacheMeta?.currentRevision, hiddenTradeMap, refreshHiddenStates],
  );

  const unhideRowByKey = useCallback(
    (key: string) => {
      void unhideRowsByKeys([key]);
    },
    [unhideRowsByKeys],
  );

  const clearDoneHiddenRows = useCallback(async () => {
    const doneKeys = Object.values(hiddenTradeMap)
      .filter((entry) => entry.mode === "done")
      .map((entry) => entry.key);
    if (doneKeys.length === 0) return;
    setHiddenTradeMap((prev) => {
      let changed = false;
      const next: Record<string, HiddenTradeEntry> = {};
      for (const [key, entry] of Object.entries(prev)) {
        if (entry.mode === "done") {
          changed = true;
          continue;
        }
        next[key] = entry;
      }
      return changed ? next : prev;
    });
    try {
      await clearStationTradeStates({ tab: "station", mode: "done" });
    } catch {
      addToast("Failed to clear done trades", "error", 2600);
      void refreshHiddenStates(cacheMeta?.currentRevision);
    }
  }, [addToast, cacheMeta?.currentRevision, hiddenTradeMap, refreshHiddenStates]);

  const clearAllHiddenRows = useCallback(async () => {
    if (Object.keys(hiddenTradeMap).length === 0) return;
    setHiddenTradeMap({});
    setIgnoredSelectedKeys(new Set());
    try {
      await clearStationTradeStates({ tab: "station" });
    } catch {
      addToast("Failed to clear hidden trades", "error", 2600);
      void refreshHiddenStates(cacheMeta?.currentRevision);
    }
  }, [addToast, cacheMeta?.currentRevision, hiddenTradeMap, refreshHiddenStates]);

  const togglePin = useCallback((key: string) => {
    setPinnedKeys((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }, []);

  const copyText = useCallback(
    (text: string) => {
      navigator.clipboard.writeText(text);
      addToast(t("copied"), "success", 2000);
      setContextMenu(null);
    },
    [addToast, t],
  );

  // Keep context menu inside viewport
  useEffect(() => {
    if (contextMenu && contextMenuRef.current) {
      const menu = contextMenuRef.current;
      const rect = menu.getBoundingClientRect();
      const padding = 10;
      let x = contextMenu.x;
      let y = contextMenu.y;
      if (x + rect.width > window.innerWidth - padding)
        x = window.innerWidth - rect.width - padding;
      if (y + rect.height > window.innerHeight - padding)
        y = window.innerHeight - rect.height - padding;
      x = Math.max(padding, x);
      y = Math.max(padding, y);
      menu.style.left = `${x}px`;
      menu.style.top = `${y}px`;
    }
  }, [contextMenu]);

  const handleScan = useCallback(async () => {
    if (scanInFlightRef.current) {
      abortRef.current?.abort();
      return;
    }
    if (!canScan) return;

    scanInFlightRef.current = true;
    const controller = new AbortController();
    abortRef.current = controller;
    setScanning(true);
    setProgress(t("scanStarting"));

    try {
      const scanParams: Parameters<typeof scanStation>[0] = {
        min_margin: minMargin,
        ignored_system_ids: params.ignored_system_ids ?? [],
        sales_tax_percent: splitTradeFees ? sellSalesTaxPercent : salesTaxPercent,
        broker_fee: splitTradeFees ? sellBrokerFeePercent : brokerFee,
        cts_profile: ctsProfile,
        split_trade_fees: splitTradeFees,
        buy_broker_fee_percent: splitTradeFees
          ? buyBrokerFeePercent
          : undefined,
        sell_broker_fee_percent: splitTradeFees
          ? sellBrokerFeePercent
          : undefined,
        buy_sales_tax_percent: splitTradeFees ? buySalesTaxPercent : undefined,
        sell_sales_tax_percent: splitTradeFees
          ? sellSalesTaxPercent
          : undefined,
        min_daily_volume: minDailyVolume,
        // EVE Guru Profit Filters
        min_item_profit: minItemProfit > 0 ? minItemProfit : undefined,
        min_s2b_per_day: minDemandPerDay > 0 ? minDemandPerDay : undefined,
        min_bfs_per_day: minBfSPerDay > 0 ? minBfSPerDay : undefined,
        // Risk Profile
        avg_price_period: avgPricePeriod,
        min_period_roi: minPeriodROI > 0 ? minPeriodROI : undefined,
        bvs_ratio_min: bvsRatioMin > 0 ? bvsRatioMin : undefined,
        bvs_ratio_max: bvsRatioMax > 0 ? bvsRatioMax : undefined,
        max_pvi: maxPVI > 0 ? maxPVI : undefined,
        max_sds: maxSDS > 0 ? maxSDS : undefined,
        limit_buy_to_price_low: limitBuyToPriceLow,
        flag_extreme_prices: flagExtremePrices,
      };

      if (radius > 0) {
        // Radius-based scan
        scanParams.system_name = params.system_name;
        scanParams.radius = radius;
      } else if (selectedStationId !== ALL_STATIONS_ID) {
        // Single station
        scanParams.station_id = selectedStationId;
        scanParams.region_id = regionId;
      } else {
        // All stations in region
        scanParams.station_id = 0;
        scanParams.region_id = regionId;
      }

      const singleStationMode = radius === 0 && selectedStationId !== ALL_STATIONS_ID;
      // Include structures for radius/all scans. Single-station mode stays strictly row-scoped.
      if (includeStructures && !singleStationMode) {
        scanParams.include_structures = true;
        if (structureStations.length > 0) {
          scanParams.structure_ids = structureStations.map((s) => s.id);
        }
      }

      if (operatorModeDevOnly && operatorMode && isLoggedIn) {
        const commandRes = await getStationCommand({
          ...scanParams,
          target_eta_days: 3,
          lookback_days: 180,
          max_results: 1500,
        });
        const nextCommandMap: Record<string, StationCommandRow> = {};
        const nextDeskMap: Record<string, OrderDeskOrder[]> = {};
        for (const order of commandRes.order_desk.orders) {
          const key = stationDeskKey(order.type_id, order.location_id);
          if (!nextDeskMap[key]) {
            nextDeskMap[key] = [];
          }
          nextDeskMap[key].push(order);
        }
        const trades = commandRes.command.rows.map((r) => {
          nextCommandMap[`${r.trade.TypeID}-${r.trade.StationID}`] = r;
          return r.trade;
        });
        const normalizedTrades = normalizeStationResults(trades);
        setCommandRowsByKey(nextCommandMap);
        setOrderDeskByKey(nextDeskMap);
        setCommandSummary(commandRes.command.summary);
        setSelectedBatchKeys(new Set());
        setResults(normalizedTrades);
        const scanFinishedAt = Date.now();
        const cacheView = mapServerCacheMeta(
          commandRes.cache_meta,
          commandRes.scan_scope || "Station scope",
          Math.max(1, commandRes.region_count || 1),
        );
        setCacheMeta(cacheView);
        setCacheNowTs(scanFinishedAt);
        await refreshHiddenStates(cacheView.currentRevision, normalizedTrades);
        setProgress(
          `Station Command: ${commandRes.command.summary.new_entry_count} new / ${commandRes.command.summary.reprice_count} reprice / ${commandRes.command.summary.cancel_count} cancel`,
        );
      } else {
        let resultMeta: StationCacheMeta | undefined;
        const res = await scanStation(
          scanParams,
          () => {
            // Keep UI progress stable for long multi-phase scans.
            // Backend can emit many internal phase updates; we intentionally
            // suppress them here to avoid jumpy status text.
          },
          controller.signal,
          (meta) => {
            resultMeta = meta;
          },
        );
        const normalizedResults = normalizeStationResults(res);
        setCommandRowsByKey({});
        setOrderDeskByKey({});
        setCommandSummary(null);
        setSelectedBatchKeys(new Set());
        setResults(normalizedResults);
        const scanFinishedAt = Date.now();
        const scopeLabel =
          radius > 0
            ? `${params.system_name} +${radius} jumps`
            : selectedStationId !== ALL_STATIONS_ID
              ? `Station ${selectedStationId}`
              : `Region ${regionId} (all)`;
        const cacheView = mapServerCacheMeta(
          resultMeta,
          scopeLabel,
          1,
        );
        setCacheMeta(cacheView);
        setCacheNowTs(scanFinishedAt);
        await refreshHiddenStates(cacheView.currentRevision, normalizedResults);
      }
    } catch (e: unknown) {
      if (e instanceof Error && e.name !== "AbortError") {
        setProgress(t("errorPrefix") + e.message);
      }
    } finally {
      if (abortRef.current === controller) {
        abortRef.current = null;
      }
      scanInFlightRef.current = false;
      setScanning(false);
    }
  }, [
    canScan,
    selectedStationId,
    regionId,
    params,
    minMargin,
    brokerFee,
    salesTaxPercent,
    splitTradeFees,
    buyBrokerFeePercent,
    sellBrokerFeePercent,
    buySalesTaxPercent,
    sellSalesTaxPercent,
    ctsProfile,
    radius,
    minDailyVolume,
    minItemProfit,
    minDemandPerDay,
    minBfSPerDay,
    avgPricePeriod,
    minPeriodROI,
    bvsRatioMin,
    bvsRatioMax,
    maxPVI,
    maxSDS,
    limitBuyToPriceLow,
    flagExtremePrices,
    includeStructures,
    structureStations,
    operatorMode,
    isLoggedIn,
    refreshHiddenStates,
    t,
  ]);

  useEffect(() => {
    if (!autoRefreshEnabled) return;
    const CHECK_INTERVAL = 15_000;
    const COOLDOWN_MS = 90_000;
    const timer = window.setInterval(() => {
      if (scanning || scanInFlightRef.current) return;
      if (!canScan) return;
      if (!cacheMeta?.nextExpiryAt) return;
      if (Date.now() < cacheMeta.nextExpiryAt) return;

      const signature = `${cacheMeta.currentRevision}:${cacheMeta.nextExpiryAt}`;
      const now = Date.now();
      const sameSnapshot = autoRefreshSignatureRef.current === signature;
      if (sameSnapshot && now - autoRefreshLastRunRef.current < COOLDOWN_MS) {
        return;
      }

      autoRefreshSignatureRef.current = signature;
      autoRefreshLastRunRef.current = now;
      void handleScan();
    }, CHECK_INTERVAL);
    return () => window.clearInterval(timer);
  }, [autoRefreshEnabled, cacheMeta, canScan, handleScan, scanning]);

  const sorted = useMemo(() => {
    const copy = [...results];
    copy.sort((a, b) => {
      const av = a[sortKey];
      const bv = b[sortKey];
      if (typeof av === "number" && typeof bv === "number") {
        return sortDir === "asc" ? av - bv : bv - av;
      }
      return sortDir === "asc"
        ? String(av).localeCompare(String(bv))
        : String(bv).localeCompare(String(av));
    });
    return copy;
  }, [results, sortKey, sortDir]);

  const hiddenEntries = useMemo(() => {
    return Object.values(hiddenTradeMap).sort((a, b) =>
      b.updatedAt.localeCompare(a.updatedAt),
    );
  }, [hiddenTradeMap]);

  const hiddenCounts = useMemo(() => {
    let done = 0;
    let ignored = 0;
    for (const row of hiddenEntries) {
      if (row.mode === "done") done++;
      if (row.mode === "ignored") ignored++;
    }
    return {
      total: hiddenEntries.length,
      done,
      ignored,
    };
  }, [hiddenEntries]);

  const filteredHiddenEntries = useMemo(() => {
    const q = ignoredSearch.trim().toLowerCase();
    return hiddenEntries.filter((entry) => {
      if (ignoredTab !== "all" && entry.mode !== ignoredTab) {
        return false;
      }
      if (!q) return true;
      return (
        entry.typeName.toLowerCase().includes(q) ||
        entry.stationName.toLowerCase().includes(q)
      );
    });
  }, [hiddenEntries, ignoredTab, ignoredSearch]);

  const displayRows = useMemo(() => {
    if (showHiddenRows) return sorted;
    return sorted.filter((row) => !hiddenTradeMap[stationRowKey(row)]);
  }, [sorted, showHiddenRows, hiddenTradeMap]);

  const cacheSecondsLeft = useMemo(() => {
    if (!cacheMeta) return null;
    return Math.floor((cacheMeta.nextExpiryAt - cacheNowTs) / 1000);
  }, [cacheMeta, cacheNowTs]);

  const cacheBadgeText = useMemo(() => {
    if (!cacheMeta || cacheSecondsLeft == null) return "Cache n/a";
    if (cacheSecondsLeft <= 0) return t("cacheStale");
    return t("cacheLabel", { time: formatCountdown(cacheSecondsLeft) });
  }, [cacheMeta, cacheSecondsLeft, t]);

  const riskCounters = useMemo(() => {
    let highRisk = 0;
    let extreme = 0;
    for (const row of displayRows) {
      if (row.IsHighRiskFlag) highRisk++;
      if (row.IsExtremePriceFlag) extreme++;
    }
    return { highRisk, extreme };
  }, [displayRows]);

  const handleRebootCache = useCallback(async () => {
    if (cacheRebooting) return;
    setCacheRebooting(true);
    try {
      const res = await rebootStationCache();
      const now = Date.now();
      setCacheMeta((prev) =>
        prev
          ? {
              ...prev,
              currentRevision: Math.floor(now / 1000),
              lastRefreshAt: now,
              nextExpiryAt: now,
            }
          : prev,
      );
      setCacheNowTs(now);
      addToast(t("cacheRebooted", { count: res.cleared }), "success", 2400);
      addToast(t("cacheRebootRescanHint"), "info", 2600);
      setProgress(`${t("cacheRebooted", { count: res.cleared })}. ${t("cacheRebootRescanHint")}`);
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : t("cacheRebootFailed");
      addToast(msg, "error", 2800);
    } finally {
      setCacheRebooting(false);
    }
  }, [addToast, cacheRebooting, t]);

  const { pageRows, totalPages, safePage } = useMemo(() => {
    const totalPages = Math.max(1, Math.ceil(displayRows.length / STATION_PAGE_SIZE));
    const safePage = Math.min(page, totalPages - 1);
    const pageRows = displayRows.slice(
      safePage * STATION_PAGE_SIZE,
      (safePage + 1) * STATION_PAGE_SIZE,
    );
    return { pageRows, totalPages, safePage };
  }, [displayRows, page]);

  useEffect(() => {
    setPage(0);
  }, [results, sortKey, sortDir, showHiddenRows, hiddenTradeMap]);

  useEffect(() => {
    if (!(operatorMode && isLoggedIn)) {
      setSelectedBatchKeys(new Set());
      return;
    }
    const visible = new Set(displayRows.map((r) => stationRowKey(r)));
    setSelectedBatchKeys((prev) => {
      if (prev.size === 0) return prev;
      const next = new Set([...prev].filter((k) => visible.has(k)));
      return next.size === prev.size ? prev : next;
    });
  }, [operatorMode, isLoggedIn, displayRows]);

  const toggleSort = (key: SortKey) => {
    if (sortKey === key) setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    else {
      setSortKey(key);
      setSortDir("desc");
    }
  };

  const summary = useMemo(() => {
    if (displayRows.length === 0) return null;
    const totalProfit = displayRows.reduce(
      (sum, r) => sum + stationDailyProfit(r),
      0,
    );
    const avgMargin =
      displayRows.reduce((sum, r) => sum + r.MarginPercent, 0) /
      displayRows.length;
    const avgCTS =
      displayRows.reduce((sum, r) => sum + r.CTS, 0) / displayRows.length;
    return { totalProfit, avgMargin, avgCTS, count: displayRows.length };
  }, [displayRows]);
  const showOperatorColumns = operatorModeDevOnly && operatorMode && isLoggedIn;
  const operatorPanelAvailable = showOperatorColumns && displayRows.length > 0;
  const operatorPanelVisible =
    operatorPanelAvailable && !operatorPanelCollapsed;

  const actionLabel = (action?: StationCommandRow["recommended_action"]) => {
    switch (action) {
      case "new_entry":
        return "new";
      case "reprice":
        return "reprice";
      case "cancel":
        return "cancel";
      case "hold":
        return "hold";
      default:
        return "n/a";
    }
  };

  const actionBadgeClass = (action?: StationCommandRow["recommended_action"]) => {
    switch (action) {
      case "new_entry":
        return "text-emerald-300 border-emerald-500/40 bg-emerald-500/10";
      case "reprice":
        return "text-eve-accent border-eve-accent/40 bg-eve-accent/10";
      case "cancel":
        return "text-red-300 border-red-500/40 bg-red-500/10";
      case "hold":
        return "text-eve-dim border-eve-border/60 bg-eve-dark/70";
      default:
        return "text-eve-dim border-eve-border/40 bg-eve-dark/50";
    }
  };

  const formatDays = (v?: number) => {
    if (v == null || !Number.isFinite(v)) return "\u2014";
    if (v < 10) return `${v.toFixed(2)}d`;
    return `${v.toFixed(1)}d`;
  };

  const formatBandISK = (
    band?: { p50: number; p95: number } | null,
    fallback = "\u2014",
  ) => {
    if (!band) return fallback;
    const p50 = band.p50;
    const p95 = band.p95;
    if (!Number.isFinite(p50) || !Number.isFinite(p95)) return fallback;
    const fmt = (v: number) => `${v >= 0 ? "+" : ""}${formatISK(v)}`;
    return `${fmt(p50)} / ${fmt(p95)}`;
  };

  const toggleBatchRow = useCallback((key: string) => {
    setSelectedBatchKeys((prev) => {
      const next = new Set(prev);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  }, []);

  const toggleBatchPage = useCallback(() => {
    const pageKeys = pageRows.map((row) => stationRowKey(row));
    if (pageKeys.length === 0) return;
    setSelectedBatchKeys((prev) => {
      const next = new Set(prev);
      const allSelected = pageKeys.every((key) => next.has(key));
      for (const key of pageKeys) {
        if (allSelected) next.delete(key);
        else next.add(key);
      }
      return next;
    });
  }, [pageRows]);

  const selectActionableRows = useCallback(() => {
    const next = new Set<string>();
    for (const row of displayRows) {
      const key = stationRowKey(row);
      const commandRow = commandRowsByKey[key];
      if (!commandRow) continue;
      const planned = computePlannedStationAction(
        { trade: row, command: commandRow },
        batchPreset,
      );
      if (planned === "hold") continue;
      next.add(key);
    }
    setSelectedBatchKeys(next);
  }, [displayRows, commandRowsByKey, batchPreset]);

  const clearBatchSelection = useCallback(() => {
    setSelectedBatchKeys(new Set());
  }, []);

  const selectByPlannedAction = useCallback(
    (action: StationCommandRow["recommended_action"]) => {
      const next = new Set<string>();
      for (const row of displayRows) {
        const key = stationRowKey(row);
        const commandRow = commandRowsByKey[key];
        if (!commandRow) continue;
        const planned = computePlannedStationAction(
          { trade: row, command: commandRow },
          batchPreset,
        );
        if (planned === action) {
          next.add(key);
        }
      }
      setSelectedBatchKeys(next);
    },
    [displayRows, commandRowsByKey, batchPreset],
  );

  const startOperatorSplitDrag = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      if (!operatorPanelVisible || !operatorSplitRef.current) return;
      e.preventDefault();
      setOperatorSplitDragging(true);
      document.body.style.userSelect = "none";

      const onMove = (ev: MouseEvent) => {
        const container = operatorSplitRef.current;
        if (!container) return;
        const rect = container.getBoundingClientRect();
        if (rect.width <= 0) return;
        const leftPercent = ((ev.clientX - rect.left) / rect.width) * 100;
        setOperatorPanelWidth(clampOperatorPanelWidth(leftPercent));
      };

      const onUp = () => {
        setOperatorSplitDragging(false);
        document.body.style.userSelect = "";
        window.removeEventListener("mousemove", onMove);
        window.removeEventListener("mouseup", onUp);
      };

      window.addEventListener("mousemove", onMove);
      window.addEventListener("mouseup", onUp);
    },
    [operatorPanelVisible],
  );

  const selectedBatchRows = useMemo(() => {
    if (selectedBatchKeys.size === 0) {
      return [] as StationBatchRow[];
    }

    const picked: StationBatchRow[] = [];

    for (const row of displayRows) {
      const key = stationRowKey(row);
      if (!selectedBatchKeys.has(key)) continue;
      const commandRow = commandRowsByKey[key];
      if (!commandRow) continue;
      picked.push({
        key,
        trade: row,
        command: commandRow,
        desk: orderDeskByKey[key] ?? [],
      });
    }

    return picked;
  }, [selectedBatchKeys, displayRows, commandRowsByKey, orderDeskByKey]);

  const operatorBatchRows = useMemo(() => {
    const rows: PlannedStationBatchRow[] = selectedBatchRows.map((row) => ({
      ...row,
      plannedAction: computePlannedStationAction(
        { trade: row.trade, command: row.command },
        batchPreset,
      ),
    }));
    const actionRank: Record<StationCommandRow["recommended_action"], number> = {
      cancel: 4,
      reprice: 3,
      new_entry: 2,
      hold: 1,
    };
    rows.sort((a, b) => {
      const actionDelta = actionRank[b.plannedAction] - actionRank[a.plannedAction];
      if (actionDelta !== 0) {
        return actionDelta;
      }
      if (a.command.priority !== b.command.priority) {
        return b.command.priority - a.command.priority;
      }
      if (
        a.command.expected_delta_daily_profit !==
        b.command.expected_delta_daily_profit
      ) {
        return (
          b.command.expected_delta_daily_profit -
          a.command.expected_delta_daily_profit
        );
      }
      return a.trade.TypeName.localeCompare(b.trade.TypeName);
    });
    return rows;
  }, [selectedBatchRows, batchPreset]);

  const batchSummary = useMemo(() => {
    if (operatorBatchRows.length === 0) return null;
    const byAction: Record<StationCommandRow["recommended_action"], number> = {
      new_entry: 0,
      reprice: 0,
      hold: 0,
      cancel: 0,
    };
    let totalDelta = 0;
    let changed = 0;
    for (const row of operatorBatchRows) {
      byAction[row.plannedAction]++;
      if (row.plannedAction !== "hold") {
        totalDelta += row.command.expected_delta_daily_profit;
      }
      if (row.plannedAction !== row.command.recommended_action) {
        changed++;
      }
    }
    return {
      count: operatorBatchRows.length,
      totalDelta,
      changed,
      ...byAction,
    };
  }, [operatorBatchRows]);

  const copyBatchPlan = useCallback(() => {
    if (operatorBatchRows.length === 0) return;

    const lines: string[] = [];
    lines.push(
      `Station Batch Plan (${new Date().toISOString()})`,
      `Preset: ${batchPreset}`,
      `Rows: ${operatorBatchRows.length}`,
      "",
    );

    for (const row of operatorBatchRows) {
      const plan = row.plannedAction.toUpperCase();
      const rec = row.command.recommended_action.toUpperCase();
      const delta = `${row.command.expected_delta_daily_profit >= 0 ? "+" : ""}${formatISK(row.command.expected_delta_daily_profit)}`;
      const deskOrder = row.desk[0];
      const deskHint = deskOrder
        ? ` | desk: ${deskOrder.is_buy_order ? "BUY" : "SELL"} -> ${formatISK(deskOrder.suggested_price)}`
        : "";
      const profitBand = ` | profit p50/p95: ${row.command.forecast?.daily_profit ? formatBandISK(row.command.forecast.daily_profit) : "\u2014"}`;
      const etaBand = ` | eta p50/p95: ${
        row.command.forecast?.eta_days
          ? `${formatDays(row.command.forecast.eta_days.p50)} / ${formatDays(row.command.forecast.eta_days.p95)}`
          : "\u2014"
      }`;
      lines.push(
        `${plan} (rec:${rec}) | ${row.trade.TypeName} @ ${row.trade.StationName} | delta/day ${delta}${profitBand}${etaBand}${deskHint}`,
      );
    }

    navigator.clipboard.writeText(lines.join("\n"));
    addToast(t("copied"), "success", 2000);
  }, [operatorBatchRows, batchPreset, addToast, t]);

  useEffect(() => {
    if (!(showOperatorColumns && displayRows.length > 0)) return;

    const onKeyDown = (e: KeyboardEvent) => {
      if (!(e.ctrlKey && e.shiftKey)) return;
      const key = e.key.toLowerCase();
      if (key === "a") {
        e.preventDefault();
        selectActionableRows();
      } else if (key === "c") {
        if (selectedBatchKeys.size === 0) return;
        e.preventDefault();
        copyBatchPlan();
      } else if (key === "x") {
        if (selectedBatchKeys.size === 0) return;
        e.preventDefault();
        clearBatchSelection();
      }
    };

    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [
    showOperatorColumns,
    displayRows.length,
    selectedBatchKeys.size,
    selectActionableRows,
    copyBatchPlan,
    clearBatchSelection,
  ]);

  const formatCell = (
    col: (typeof columnDefs)[number],
    row: StationTrade,
  ): string => {
    const val = row[col.key];
    if (
      col.key === "BuyPrice" ||
      col.key === "SellPrice" ||
      col.key === "Spread" ||
      col.key === "TotalProfit" ||
      col.key === "DailyProfit" ||
      col.key === "ProfitPerUnit" ||
      col.key === "CapitalRequired" ||
      col.key === "VWAP"
    ) {
      const n =
        col.key === "DailyProfit"
          ? stationDailyProfit(row)
          : (val as number | undefined);
      return n != null && Number.isFinite(n) ? formatISK(n) : "\u2014";
    }
    if (
      col.key === "MarginPercent" ||
      col.key === "NowROI" ||
      col.key === "PeriodROI" ||
      col.key === "PVI"
    ) {
      const n = val as number | undefined;
      return n != null && Number.isFinite(n) ? formatMargin(n) : "\u2014";
    }
    if (col.key === "S2BBfSRatio" || col.key === "DOS" || col.key === "OBDS") {
      return (val as number).toFixed(2);
    }
    if (col.key === "CTS") {
      return (val as number).toFixed(1);
    }
    if (typeof val === "number") return formatNumber(val);
    return String(val);
  };

  // Get row class with risk indicators
  const getRowClass = (
    row: StationTrade,
    index: number,
    commandRow?: StationCommandRow,
  ) => {
    let base = `border-b border-eve-border/50 hover:bg-eve-accent/5 transition-colors ${
      index % 2 === 0 ? "bg-eve-panel" : "bg-eve-dark"
    }`;
    if (row.IsHighRiskFlag) base += " border-l-2 border-l-eve-error";
    else if (row.IsExtremePriceFlag) base += " border-l-2 border-l-yellow-500";
    if (showOperatorColumns && commandRow) {
      if (commandRow.recommended_action === "cancel") {
        base += " bg-red-950/20";
      } else if (commandRow.recommended_action === "reprice") {
        base += " bg-eve-accent/5";
      } else if (commandRow.recommended_action === "new_entry") {
        base += " bg-emerald-900/10";
      }
    }
    return base;
  };

  // Get CTS color class
  const getCTSColor = (cts: number) => {
    if (cts >= 70) return "text-green-400";
    if (cts >= 40) return "text-yellow-400";
    return "text-red-400";
  };

  // Get SDS color class
  const getSDSColor = (sds: number) => {
    if (sds >= 50) return "text-red-400";
    if (sds >= 30) return "text-yellow-400";
    return "text-green-400";
  };

  // Build station options for select
  const stationOptions = useMemo(() => {
    const opts = [{ value: ALL_STATIONS_ID, label: t("allStations") }];
    for (const st of allStations) {
      const label = st.is_structure ? `\u{1F3D7}\uFE0F ${st.name}` : st.name;
      opts.push({ value: st.id, label });
    }
    return opts;
  }, [allStations, t]);

  const selectedStationLabel = useMemo(() => {
    if (selectedStationId === ALL_STATIONS_ID) return t("allStations");
    const selected = allStations.find((s) => s.id === selectedStationId);
    return selected?.name ?? t("allStations");
  }, [allStations, selectedStationId, t]);

  const aiScanSnapshot = useMemo<StationAIScanSnapshot>(() => {
    const scopeMode =
      radius > 0
        ? "radius"
        : selectedStationId !== ALL_STATIONS_ID
          ? "single_station"
          : "region_all";
    const singleStationMode = scopeMode === "single_station";
    const structuresApplied = includeStructures && !singleStationMode;
    const structureIDs = structuresApplied
      ? structureStations.map((s) => s.id).slice(0, 300)
      : [];

    return {
      scope_mode: scopeMode,
      system_name: params.system_name || "",
      region_id: regionId,
      station_id: selectedStationId === ALL_STATIONS_ID ? 0 : selectedStationId,
      radius,
      min_margin: minMargin,
      sales_tax_percent: splitTradeFees ? sellSalesTaxPercent : salesTaxPercent,
      broker_fee: splitTradeFees ? sellBrokerFeePercent : brokerFee,
      split_trade_fees: splitTradeFees,
      buy_broker_fee_percent: buyBrokerFeePercent,
      sell_broker_fee_percent: sellBrokerFeePercent,
      buy_sales_tax_percent: buySalesTaxPercent,
      sell_sales_tax_percent: sellSalesTaxPercent,
      cts_profile: ctsProfile,
      min_daily_volume: minDailyVolume,
      min_item_profit: minItemProfit,
      min_s2b_per_day: minDemandPerDay,
      min_bfs_per_day: minBfSPerDay,
      avg_price_period: avgPricePeriod,
      min_period_roi: minPeriodROI,
      bvs_ratio_min: bvsRatioMin,
      bvs_ratio_max: bvsRatioMax,
      max_pvi: maxPVI,
      max_sds: maxSDS,
      limit_buy_to_price_low: limitBuyToPriceLow,
      flag_extreme_prices: flagExtremePrices,
      include_structures: includeStructures,
      structures_applied: structuresApplied,
      structure_count: structuresApplied ? structureStations.length : 0,
      structure_ids: structureIDs,
    };
  }, [
    avgPricePeriod,
    brokerFee,
    buyBrokerFeePercent,
    buySalesTaxPercent,
    bvsRatioMax,
    bvsRatioMin,
    ctsProfile,
    flagExtremePrices,
    includeStructures,
    limitBuyToPriceLow,
    maxPVI,
    maxSDS,
    minBfSPerDay,
    minDailyVolume,
    minDemandPerDay,
    minItemProfit,
    minMargin,
    minPeriodROI,
    params.system_name,
    radius,
    regionId,
    salesTaxPercent,
    selectedStationId,
    sellBrokerFeePercent,
    sellSalesTaxPercent,
    splitTradeFees,
    structureStations,
  ]);

  const contextHiddenEntry = contextMenu
    ? hiddenTradeMap[stationRowKey(contextMenu.row)]
    : undefined;

  return (
    <div className="flex-1 flex flex-col min-h-0">
      {/* Settings Panel - unified design */}
      <div className="shrink-0 m-2">
        <TabSettingsPanel
          title={t("stationSettings")}
          hint={t("stationSettingsHint")}
          icon="🏪"
          defaultExpanded={true}
          persistKey="station"
          help={{
            stepKeys: [
              "helpStationStep1",
              "helpStationStep2",
              "helpStationStep3",
            ],
            wikiSlug: "Station-Trading",
          }}
          headerExtra={
            <PresetPicker
              params={stationSettings}
              onApply={handlePresetApply}
              tab="station"
              builtinPresets={STATION_BUILTIN_PRESETS}
              align="right"
            />
          }
        >
          <div className="space-y-3">
            <div className="grid grid-cols-1 xl:grid-cols-12 gap-3">
              <section className={`${settingsSectionClass} xl:col-span-8 p-3`}>
                <PanelSectionHeader
                  icon="⌁"
                  title={t("system")}
                  subtitle={t("stationSelect")}
                />

                <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-x-3 gap-y-3 mt-2">
                  <SettingsField label={t("system")}>
                    <SystemAutocomplete
                      value={params.system_name}
                      onChange={(v) => onChange?.({ ...params, system_name: v })}
                      showLocationButton={true}
                      isLoggedIn={isLoggedIn}
                      extraActionSlots={1}
                      extraAction={
                        <SystemBlacklistButton
                          compact
                          value={params.ignored_system_ids ?? []}
                          onChange={(ids) =>
                            onChange?.({ ...params, ignored_system_ids: ids })
                          }
                        />
                      }
                    />
                  </SettingsField>

                  <SettingsField label={t("stationSelect")}>
                    {loadingStations || loadingStructures ? (
                      <div className="h-[34px] flex items-center text-xs text-eve-dim">
                        {loadingStructures
                          ? t("loadingStructures")
                          : t("loadingStations")}
                      </div>
                    ) : allStations.length === 0 ? (
                      <div className="h-[34px] flex items-center text-xs text-eve-dim">
                        {stations.length === 0 && !isLoggedIn
                          ? t("noNpcStationsLoginHint")
                          : stations.length === 0 &&
                              isLoggedIn &&
                              !includeStructures
                            ? t("noNpcStationsToggleHint")
                            : includeStructures
                              ? t("noStationsOrInaccessible")
                              : t("noStations")}
                      </div>
                    ) : (
                      <SettingsSelect
                        value={selectedStationId}
                        onChange={(v) => setSelectedStationId(Number(v))}
                        options={stationOptions}
                      />
                    )}
                  </SettingsField>

                  {isLoggedIn && (
                    <SettingsField label={t("includeStructures")}>
                      <SettingsCheckbox
                        checked={includeStructures}
                        onChange={setIncludeStructures}
                      />
                    </SettingsField>
                  )}

                  {isLoggedIn && operatorModeDevOnly && (
                    <SettingsField label="Operator mode">
                      <SettingsCheckbox
                        checked={operatorMode}
                        onChange={setOperatorMode}
                      />
                    </SettingsField>
                  )}

                  <SettingsField label={t("stationRadius")}>
                    <SettingsNumberInput
                      value={radius}
                      onChange={(v) => setRadius(Math.max(0, Math.min(50, v)))}
                      min={0}
                      max={50}
                    />
                  </SettingsField>

                  <SettingsField label={t("minMargin")}>
                    <SettingsNumberInput
                      value={minMargin}
                      onChange={setMinMargin}
                      min={0}
                      step={0.1}
                    />
                  </SettingsField>

                  <SettingsField label={t("minDailyVolume")}>
                    <SettingsNumberInput
                      value={minDailyVolume}
                      onChange={setMinDailyVolume}
                      min={0}
                    />
                  </SettingsField>

                  <SettingsField label={t("minItemProfit")}>
                    <SettingsNumberInput
                      value={minItemProfit}
                      onChange={setMinItemProfit}
                      min={0}
                    />
                  </SettingsField>
                </div>
              </section>

              <section className={`${settingsSectionClass} xl:col-span-4 p-3`}>
                <PanelSectionHeader
                  icon="∑"
                  title={t("splitTradeFees")}
                  subtitle={t("splitTradeFeesHint")}
                />

                <div className="mt-2">
                  <SettingsField label={t("splitTradeFees")}>
                    <div className="h-[34px] px-2.5 py-1.5 bg-eve-input border border-eve-border rounded text-eve-text text-sm flex items-center justify-between">
                      <span className="text-eve-dim text-xs">
                        {t("splitTradeFeesHint")}
                      </span>
                      <input
                        type="checkbox"
                        checked={splitTradeFees}
                        onChange={(e) => {
                          const enabled = e.target.checked;
                          if (enabled) {
                            setBuyBrokerFeePercent(brokerFee);
                            setSellBrokerFeePercent(brokerFee);
                            setBuySalesTaxPercent(0);
                            setSellSalesTaxPercent(salesTaxPercent);
                          } else {
                            setBrokerFee(sellBrokerFeePercent);
                            setSalesTaxPercent(sellSalesTaxPercent);
                          }
                          setSplitTradeFees(enabled);
                        }}
                        className="accent-eve-accent"
                      />
                    </div>
                  </SettingsField>
                </div>

                {!splitTradeFees && (
                  <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-1 gap-3 mt-3">
                    <SettingsField label={t("brokerFee")}>
                      <SettingsNumberInput
                        value={brokerFee}
                        onChange={setBrokerFee}
                        min={0}
                        max={10}
                        step={0.1}
                      />
                    </SettingsField>

                    <SettingsField label={t("salesTax")}>
                      <SettingsNumberInput
                        value={salesTaxPercent}
                        onChange={(v) =>
                          setSalesTaxPercent(Math.max(0, Math.min(100, v)))
                        }
                        min={0}
                        max={100}
                        step={0.1}
                      />
                    </SettingsField>
                  </div>
                )}

                {splitTradeFees && (
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mt-3">
                    <SettingsField label={t("buyBrokerFee")}>
                      <SettingsNumberInput
                        value={buyBrokerFeePercent}
                        onChange={(v) =>
                          setBuyBrokerFeePercent(Math.max(0, Math.min(100, v)))
                        }
                        min={0}
                        max={100}
                        step={0.1}
                      />
                    </SettingsField>
                    <SettingsField label={t("sellBrokerFee")}>
                      <SettingsNumberInput
                        value={sellBrokerFeePercent}
                        onChange={(v) =>
                          setSellBrokerFeePercent(Math.max(0, Math.min(100, v)))
                        }
                        min={0}
                        max={100}
                        step={0.1}
                      />
                    </SettingsField>
                    <SettingsField label={t("buySalesTax")}>
                      <SettingsNumberInput
                        value={buySalesTaxPercent}
                        onChange={(v) =>
                          setBuySalesTaxPercent(Math.max(0, Math.min(100, v)))
                        }
                        min={0}
                        max={100}
                        step={0.1}
                      />
                    </SettingsField>
                    <SettingsField label={t("sellSalesTax")}>
                      <SettingsNumberInput
                        value={sellSalesTaxPercent}
                        onChange={(v) =>
                          setSellSalesTaxPercent(Math.max(0, Math.min(100, v)))
                        }
                        min={0}
                        max={100}
                        step={0.1}
                      />
                    </SettingsField>
                  </div>
                )}
              </section>
            </div>

            <section className={`${settingsSectionClass} p-3`}>
              <button
                type="button"
                onClick={() => setShowAdvanced((prev) => !prev)}
                className="w-full flex items-center justify-between gap-3 text-[11px] uppercase tracking-wider text-eve-dim hover:text-eve-accent font-medium transition-colors"
              >
                <span className="flex items-center gap-1.5">
                  <span
                    className={`transition-transform ${
                      showAdvanced ? "rotate-90" : ""
                    }`}
                  >
                    ▸
                  </span>
                  {t("advancedFilters")}
                </span>
                {activeAdvancedCount > 0 && (
                  <span className="px-1.5 py-0.5 rounded-sm border border-eve-accent/40 text-eve-accent text-[10px] font-mono">
                    {activeAdvancedCount}
                  </span>
                )}
              </button>

              {showAdvanced && (
                <div className="mt-3 pt-3 border-t border-eve-border/40 space-y-3">
                  <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5 gap-x-3 gap-y-3">
                    <SettingsField label={t("minS2BPerDay")}>
                      <SettingsNumberInput
                        value={minDemandPerDay}
                        onChange={setMinDemandPerDay}
                        min={0}
                        step={0.1}
                      />
                    </SettingsField>
                    <SettingsField label={t("minBfSPerDay")}>
                      <SettingsNumberInput
                        value={minBfSPerDay}
                        onChange={setMinBfSPerDay}
                        min={0}
                        step={0.1}
                      />
                    </SettingsField>
                    <SettingsField label={t("ctsProfile")}>
                      <SettingsSelect
                        value={ctsProfile}
                        onChange={(v) => {
                          if (
                            v === "balanced" ||
                            v === "aggressive" ||
                            v === "defensive"
                          ) {
                            setCTSProfile(v);
                            return;
                          }
                          setCTSProfile("balanced");
                        }}
                        options={[
                          {
                            value: "balanced",
                            label: t("ctsProfileBalanced"),
                          },
                          {
                            value: "aggressive",
                            label: t("ctsProfileAggressive"),
                          },
                          {
                            value: "defensive",
                            label: t("ctsProfileDefensive"),
                          },
                        ]}
                      />
                    </SettingsField>
                    <SettingsField label={t("avgPricePeriod")}>
                      <SettingsNumberInput
                        value={avgPricePeriod}
                        onChange={setAvgPricePeriod}
                        min={7}
                        max={365}
                      />
                    </SettingsField>
                    <SettingsField label={t("minPeriodROI")}>
                      <SettingsNumberInput
                        value={minPeriodROI}
                        onChange={setMinPeriodROI}
                        min={0}
                      />
                    </SettingsField>
                    <SettingsField label={t("maxPVI")}>
                      <SettingsNumberInput
                        value={maxPVI}
                        onChange={setMaxPVI}
                        min={0}
                      />
                    </SettingsField>
                    <SettingsField label={t("maxSDS")}>
                      <SettingsNumberInput
                        value={maxSDS}
                        onChange={setMaxSDS}
                        min={0}
                        max={100}
                      />
                    </SettingsField>
                  </div>

                  <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-x-3 gap-y-3">
                    <SettingsField label={t("bvsRatioMin")}>
                      <SettingsNumberInput
                        value={bvsRatioMin}
                        onChange={setBvsRatioMin}
                        min={0}
                        step={0.1}
                      />
                    </SettingsField>
                    <SettingsField label={t("bvsRatioMax")}>
                      <SettingsNumberInput
                        value={bvsRatioMax}
                        onChange={setBvsRatioMax}
                        min={0}
                        step={0.1}
                      />
                    </SettingsField>
                    <SettingsField label={t("limitBuyToPriceLow")}>
                      <SettingsCheckbox
                        checked={limitBuyToPriceLow}
                        onChange={setLimitBuyToPriceLow}
                      />
                    </SettingsField>
                    <SettingsField label={t("flagExtremePrices")}>
                      <SettingsCheckbox
                        checked={flagExtremePrices}
                        onChange={setFlagExtremePrices}
                      />
                    </SettingsField>
                  </div>
                </div>
              )}
            </section>
          </div>

          {/* Scan button inside settings */}
          <div className="mt-3 pt-3 border-t border-eve-border/30 flex items-center justify-between gap-3">
            <label className="inline-flex items-center gap-1.5 cursor-pointer select-none text-eve-dim hover:text-eve-text transition-colors text-xs">
              <input
                type="checkbox"
                checked={autoRefreshEnabled}
                onChange={(e) => setAutoRefreshEnabled(e.target.checked)}
                className="accent-eve-accent"
              />
              Auto-refresh
              {autoRefreshEnabled && (
                <span className="inline-flex items-center gap-1 text-eve-accent">
                  <span className="w-1.5 h-1.5 rounded-full bg-eve-accent animate-pulse" />
                  active
                </span>
              )}
            </label>
            <button
              onClick={handleScan}
              disabled={!canScan}
              className={`px-5 py-1.5 rounded-sm text-xs font-semibold uppercase tracking-wider transition-all
                ${
                  scanning
                    ? "bg-eve-error/80 text-white hover:bg-eve-error"
                    : "bg-eve-accent text-eve-dark hover:bg-eve-accent-hover shadow-eve-glow"
                }
                disabled:bg-eve-input disabled:text-eve-dim disabled:cursor-not-allowed disabled:shadow-none`}
            >
              {scanning ? t("stop") : t("scan")}
            </button>
          </div>
        </TabSettingsPanel>
      </div>

      {/* Status */}
      <div className="shrink-0 px-2 py-1.5">
        <div className="rounded-sm border border-eve-border/70 bg-gradient-to-r from-eve-dark/85 via-eve-panel/80 to-eve-dark/85 px-2 py-1.5 text-xs text-eve-dim flex flex-wrap items-center gap-2">
          {scanning ? (
            <div className="flex items-center gap-2 min-h-[22px]">
              <span className="w-2 h-2 rounded-full bg-eve-accent animate-pulse" />
              <span className="text-eve-text">{progress}</span>
            </div>
          ) : results.length > 0 ? (
            <div className="flex flex-wrap items-center gap-1.5">
              <StatusChip
                label="Opportunities"
                value={results.length}
                tone="accent"
              />
              <StatusChip
                label="Visible"
                value={displayRows.length}
                tone="neutral"
              />
              {hiddenCounts.total > 0 && (
                <StatusChip label="Hidden" value={hiddenCounts.total} tone="neutral" />
              )}
              <StatusChip
                label={t("highRisk")}
                value={riskCounters.highRisk}
                tone={riskCounters.highRisk > 0 ? "danger" : "neutral"}
              />
              <StatusChip
                label={t("extremePrice")}
                value={riskCounters.extreme}
                tone={riskCounters.extreme > 0 ? "warning" : "neutral"}
              />
              {showOperatorColumns && commandSummary && (
                <StatusChip
                  label="Action"
                  value={`${commandSummary.new_entry_count}/${commandSummary.reprice_count}/${commandSummary.cancel_count}/${commandSummary.hold_count}`}
                  tone="accent"
                />
              )}
              {showOperatorColumns && selectedBatchKeys.size > 0 && (
                <StatusChip
                  label="Selected"
                  value={selectedBatchKeys.size}
                  tone="success"
                />
              )}
            </div>
          ) : (
            <div className="min-h-[22px] flex items-center text-eve-dim">
              {t("scanStarting")}
            </div>
          )}
          <div className="flex-1" />
          {!scanning && results.length > 0 && (
            <div className="flex items-center gap-2 mr-1">
              <label className="inline-flex items-center gap-1 px-2 py-0.5 rounded-sm border border-eve-border/60 bg-eve-dark/40 text-[11px] cursor-pointer">
                <input
                  type="checkbox"
                  checked={showHiddenRows}
                  onChange={(e) => setShowHiddenRows(e.target.checked)}
                className="accent-eve-accent"
              />
              <span>Show hidden</span>
            </label>
            <button
              type="button"
              onClick={() => setIgnoredModalOpen(true)}
              className="px-2 py-0.5 rounded-sm border border-eve-border/60 bg-eve-dark/40 text-[11px] hover:border-eve-accent/50 hover:text-eve-accent transition-colors"
              title="Open hidden rows manager"
            >
              Ignored ({hiddenCounts.total})
            </button>
              <button
                type="button"
                onClick={() => {
                  void handleRebootCache();
                }}
                disabled={cacheRebooting}
              className={`px-2 py-0.5 rounded-sm border bg-eve-dark/40 text-[11px] transition-colors disabled:opacity-40 disabled:cursor-not-allowed ${
                cacheSecondsLeft != null && cacheSecondsLeft <= 0
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
                cacheSecondsLeft != null && cacheSecondsLeft <= 0
                  ? "border-red-500/50 text-red-300 bg-red-950/30"
                  : "border-eve-border/60 text-eve-accent bg-eve-dark/40 hover:border-eve-accent/50"
              }`}
              title={
                cacheMeta
                  ? `Scope: ${cacheMeta.scopeLabel}\nRegions: ${cacheMeta.regionCount}\nLast refresh: ${new Date(cacheMeta.lastRefreshAt).toLocaleTimeString()}\nNext expiry: ${new Date(cacheMeta.nextExpiryAt).toLocaleTimeString()}`
                  : "Cache metadata unavailable"
              }
              >
                {cacheBadgeText}
              </button>
              {cacheSecondsLeft != null && cacheSecondsLeft <= 0 && (
                <span className="text-red-300 text-[11px]">{t("cacheStaleHint")}</span>
              )}
            </div>
          )}
          {!scanning && displayRows.length > STATION_PAGE_SIZE && (
            <div className="flex items-center gap-1 text-eve-dim ml-1 pl-1 border-l border-eve-border/40">
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
        </div>
      </div>

      <div
        ref={operatorPanelVisible ? operatorSplitRef : undefined}
        className={`flex-1 min-h-0 min-w-0 flex px-2 pb-2 ${
          operatorPanelAvailable ? "gap-2" : ""
        }`}
      >
        {operatorPanelVisible && (
          <>
            <aside
              className="relative min-h-0 rounded-sm border border-eve-border/70 bg-eve-panel/70 p-3 flex flex-col"
              style={{ width: `${operatorPanelWidth}%` }}
            >
              <button
                type="button"
                onClick={() => setOperatorPanelCollapsed(true)}
                className="absolute -left-3 top-1/2 -translate-y-1/2 h-12 w-3 rounded-l-sm border border-r-0 border-eve-border/60 bg-eve-dark/90 text-eve-dim hover:text-eve-accent hover:border-eve-accent/50 transition-colors"
                title="Collapse Mass Order Operations"
              >
                ‹
              </button>
              <div className="flex items-center justify-between gap-2">
                <span className="text-xs text-eve-dim uppercase tracking-wide">
                  Mass order operations
                </span>
                <div className="flex items-center gap-1">
                  <button
                    type="button"
                    onClick={() =>
                      setOperatorPanelWidth((w) =>
                        clampOperatorPanelWidth(w - 5),
                      )
                    }
                    className="px-1.5 py-0.5 rounded-sm border border-eve-border/60 text-eve-dim hover:text-eve-accent hover:border-eve-accent/50 transition-colors"
                    title="Narrow panel"
                  >
                    −
                  </button>
                  <button
                    type="button"
                    onClick={() => setOperatorPanelWidth(OPERATOR_PANEL_DEFAULT)}
                    className="px-1.5 py-0.5 rounded-sm border border-eve-border/60 text-eve-dim hover:text-eve-accent hover:border-eve-accent/50 transition-colors text-[10px] font-mono"
                    title="Reset to 50/50"
                  >
                    {Math.round(operatorPanelWidth)}%
                  </button>
                  <button
                    type="button"
                    onClick={() =>
                      setOperatorPanelWidth((w) =>
                        clampOperatorPanelWidth(w + 5),
                      )
                    }
                    className="px-1.5 py-0.5 rounded-sm border border-eve-border/60 text-eve-dim hover:text-eve-accent hover:border-eve-accent/50 transition-colors"
                    title="Widen panel"
                  >
                    +
                  </button>
                </div>
              </div>

              <div className="mt-2 rounded-sm border border-eve-border/50 bg-eve-dark/40 p-2 space-y-2">
                <div className="flex items-center gap-2 text-[10px]">
                  <span className="text-eve-dim uppercase tracking-wide shrink-0">
                    Strategy
                  </span>
                  <div className="flex flex-wrap items-center gap-1">
                    {(["safe", "balanced", "aggressive"] as BatchPreset[]).map(
                      (preset) => (
                        <button
                          key={preset}
                          type="button"
                          onClick={() => setBatchPreset(preset)}
                          className={`px-2 py-0.5 rounded-sm border transition-colors text-[11px] ${
                            batchPreset === preset
                              ? "border-eve-accent text-eve-accent bg-eve-accent/10"
                              : "border-eve-border/60 text-eve-text hover:border-eve-accent/50 hover:text-eve-accent"
                          }`}
                        >
                          {preset}
                        </button>
                      ),
                    )}
                  </div>
                </div>

                <div className="flex flex-wrap items-center gap-1 text-[11px]">
                  <button
                    type="button"
                    onClick={selectActionableRows}
                    className="px-2 py-0.5 rounded-sm border border-eve-accent/60 text-eve-accent hover:bg-eve-accent/10 transition-colors"
                    title="Select all actionable rows by current preset"
                  >
                    Actionable
                  </button>
                  <button
                    type="button"
                    onClick={toggleBatchPage}
                    className="px-2 py-0.5 rounded-sm border border-eve-border/60 text-eve-text hover:border-eve-accent/50 hover:text-eve-accent transition-colors"
                    title="Toggle all rows on current page"
                  >
                    Page
                  </button>
                  <button
                    type="button"
                    onClick={clearBatchSelection}
                    disabled={selectedBatchKeys.size === 0}
                    className="px-2 py-0.5 rounded-sm border border-eve-border/60 text-eve-text hover:border-eve-accent/50 hover:text-eve-accent transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
                    title="Clear selected rows"
                  >
                    Clear
                  </button>
                  <button
                    type="button"
                    onClick={copyBatchPlan}
                    disabled={selectedBatchKeys.size === 0}
                    className="px-2 py-0.5 rounded-sm border border-eve-accent/60 text-eve-accent hover:bg-eve-accent/10 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
                    title="Copy batch execution plan to clipboard"
                  >
                    Copy
                  </button>
                </div>

                <div className="flex flex-wrap items-center gap-1 text-[11px]">
                  <span className="text-eve-dim uppercase tracking-wide text-[10px] mr-1">
                    Plan filter
                  </span>
                  <button
                    type="button"
                    onClick={() => selectByPlannedAction("reprice")}
                    className="px-2 py-0.5 rounded-sm border border-eve-border/60 text-eve-text hover:border-eve-accent/50 hover:text-eve-accent transition-colors"
                  >
                    Reprice
                  </button>
                  <button
                    type="button"
                    onClick={() => selectByPlannedAction("cancel")}
                    className="px-2 py-0.5 rounded-sm border border-eve-border/60 text-eve-text hover:border-eve-accent/50 hover:text-eve-accent transition-colors"
                  >
                    Cancel
                  </button>
                  <button
                    type="button"
                    onClick={() => selectByPlannedAction("new_entry")}
                    className="px-2 py-0.5 rounded-sm border border-eve-border/60 text-eve-text hover:border-eve-accent/50 hover:text-eve-accent transition-colors"
                  >
                    New
                  </button>
                </div>
              </div>

              {batchSummary && (
                <div className="mt-2 grid grid-cols-2 gap-1 text-[10px]">
                  <div className="rounded-sm border border-eve-border/40 bg-eve-dark/40 px-2 py-1">
                    <span className="text-eve-dim">Preset</span>
                    <span className="ml-1 text-eve-accent font-mono">{batchPreset}</span>
                  </div>
                  <div className="rounded-sm border border-eve-border/40 bg-eve-dark/40 px-2 py-1">
                    <span className="text-eve-dim">Rows</span>
                    <span className="ml-1 text-eve-accent font-mono">{batchSummary.count}</span>
                  </div>
                  <div className="rounded-sm border border-eve-border/40 bg-eve-dark/40 px-2 py-1">
                    <span className="text-eve-dim">Actions</span>
                    <span className="ml-1 text-eve-accent font-mono">
                      n{batchSummary.new_entry} r{batchSummary.reprice} c{batchSummary.cancel} h{batchSummary.hold}
                    </span>
                  </div>
                  <div className="rounded-sm border border-eve-border/40 bg-eve-dark/40 px-2 py-1">
                    <span className="text-eve-dim">Changed</span>
                    <span className="ml-1 text-eve-accent font-mono">{batchSummary.changed}</span>
                  </div>
                  <div className="col-span-2 rounded-sm border border-eve-border/40 bg-eve-dark/40 px-2 py-1">
                    <span className="text-eve-dim">Delta/day</span>
                    <span className="ml-1 text-eve-accent font-mono">
                      {batchSummary.totalDelta >= 0 ? "+" : ""}
                      {formatISK(batchSummary.totalDelta)}
                    </span>
                  </div>
                </div>
              )}

              <details className="mt-2 text-[10px] text-eve-dim">
                <summary className="cursor-pointer select-none hover:text-eve-text transition-colors">
                  Shortcuts
                </summary>
                <div className="mt-1 font-mono">
                  `Ctrl+Shift+A` actionable | `Ctrl+Shift+C` copy | `Ctrl+Shift+X` clear
                </div>
              </details>

              <div className="mt-2 flex-1 min-h-0 overflow-auto border border-eve-border/40 rounded-sm bg-eve-dark/50 eve-scrollbar">
                {operatorBatchRows.length > 0 ? (
                  <table className="w-full text-[11px]">
                    <thead className="sticky top-0 z-10 bg-eve-dark/95 border-b border-eve-border/50">
                      <tr>
                        <th className="px-2 py-1 text-left text-eve-dim uppercase tracking-wide font-medium">
                          Plan
                        </th>
                        <th className="px-2 py-1 text-left text-eve-dim uppercase tracking-wide font-medium">
                          Rec
                        </th>
                        <th className="px-2 py-1 text-left text-eve-dim uppercase tracking-wide font-medium">
                          Item
                        </th>
                        <th className="px-2 py-1 text-left text-eve-dim uppercase tracking-wide font-medium">
                          Station
                        </th>
                        <th className="px-2 py-1 text-right text-eve-dim uppercase tracking-wide font-medium">
                          Delta/day
                        </th>
                        <th className="px-2 py-1 text-right text-eve-dim uppercase tracking-wide font-medium">
                          Profit P50/P95
                        </th>
                        <th className="px-2 py-1 text-right text-eve-dim uppercase tracking-wide font-medium">
                          ETA P50/P95
                        </th>
                        <th className="px-2 py-1 text-right text-eve-dim uppercase tracking-wide font-medium">
                          Desk
                        </th>
                        <th className="px-2 py-1 text-right text-eve-dim uppercase tracking-wide font-medium">
                          Prio
                        </th>
                        <th className="px-2 py-1 text-left text-eve-dim uppercase tracking-wide font-medium">
                          Reason
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {operatorBatchRows.map((row) => {
                        const desk = row.desk[0];
                        return (
                          <tr
                            key={row.key}
                            className="border-b border-eve-border/20 hover:bg-eve-accent/5 transition-colors"
                          >
                            <td className="px-2 py-1">
                              <span
                                className={`inline-flex items-center px-1 py-px rounded-sm border ${actionBadgeClass(
                                  row.plannedAction,
                                )}`}
                              >
                                {actionLabel(row.plannedAction)}
                              </span>
                            </td>
                            <td className="px-2 py-1">
                              <span
                                className={`inline-flex items-center px-1 py-px rounded-sm border ${
                                  row.plannedAction === row.command.recommended_action
                                    ? "border-eve-border/40 text-eve-dim"
                                    : actionBadgeClass(row.command.recommended_action)
                                }`}
                              >
                                {actionLabel(row.command.recommended_action)}
                              </span>
                            </td>
                            <td className="px-2 py-1 text-eve-text max-w-[220px] truncate">
                              {row.trade.TypeName}
                            </td>
                            <td className="px-2 py-1 text-eve-dim max-w-[220px] truncate">
                              {row.trade.StationName}
                            </td>
                            <td
                              className={`px-2 py-1 text-right font-mono ${
                                row.command.expected_delta_daily_profit >= 0
                                  ? "text-emerald-300"
                                  : "text-red-300"
                              }`}
                            >
                              {row.command.expected_delta_daily_profit >= 0 ? "+" : ""}
                              {formatISK(row.command.expected_delta_daily_profit)}
                            </td>
                            <td className="px-2 py-1 text-right text-eve-accent font-mono">
                              {formatBandISK(row.command.forecast?.daily_profit)}
                            </td>
                            <td className="px-2 py-1 text-right text-eve-dim font-mono">
                              {row.command.forecast?.eta_days
                                ? `${formatDays(row.command.forecast.eta_days.p50)} / ${formatDays(
                                    row.command.forecast.eta_days.p95,
                                  )}`
                                : "\u2014"}
                            </td>
                            <td className="px-2 py-1 text-right text-eve-dim font-mono">
                              {desk
                                ? `${desk.is_buy_order ? "BUY" : "SELL"} ${formatISK(
                                    desk.suggested_price,
                                  )}`
                                : "\u2014"}
                            </td>
                            <td className="px-2 py-1 text-right text-eve-accent font-mono">
                              {row.command.priority}
                            </td>
                            <td
                              className="px-2 py-1 text-eve-dim max-w-[280px] truncate"
                              title={row.command.action_reason || "no action explanation"}
                            >
                              {row.command.action_reason || "\u2014"}
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                ) : (
                  <div className="px-3 py-2 text-[11px] text-eve-dim">
                    Select rows in table or use "Select actionable".
                  </div>
                )}
              </div>
            </aside>

            <div
              onMouseDown={startOperatorSplitDrag}
              className={`w-2 shrink-0 rounded-sm border cursor-col-resize transition-colors ${
                operatorSplitDragging
                  ? "border-eve-accent bg-eve-accent/20"
                  : "border-eve-border/60 bg-eve-dark/70 hover:border-eve-accent/50"
              }`}
              title="Drag to resize panels"
            >
              <div className="h-full flex items-center justify-center text-eve-dim text-[10px]">
                ⋮
              </div>
            </div>
          </>
        )}

        {operatorPanelAvailable && !operatorPanelVisible && (
          <button
            type="button"
            onClick={() => setOperatorPanelCollapsed(false)}
            className="shrink-0 h-full w-6 rounded-sm border border-eve-border/60 bg-eve-dark/80 text-eve-dim hover:text-eve-accent hover:border-eve-accent/50 transition-colors"
            title="Expand Mass Order Operations"
          >
            <span className="block -rotate-90 text-[10px] tracking-wide uppercase">
              Mass Ops ›
            </span>
          </button>
        )}

        {/* Table */}
        <div
          className={`min-h-0 min-w-0 overflow-x-auto overflow-y-auto border border-eve-border rounded-sm table-scroll-wrapper table-scroll-no-fade table-scroll-container eve-scrollbar ${
            operatorPanelVisible ? "" : "flex-1"
          }`}
          style={
            operatorPanelVisible
              ? { width: `${100 - operatorPanelWidth}%` }
              : undefined
          }
        >
        <table className="w-full min-w-max text-sm">
          <thead className="sticky top-0 z-10">
            <tr className="bg-eve-dark border-b border-eve-border">
              {showOperatorColumns && (
                <th className="w-8 px-1 py-2 text-center">
                  <input
                    type="checkbox"
                    checked={
                      pageRows.length > 0 &&
                      pageRows.every((row) =>
                        selectedBatchKeys.has(stationRowKey(row)),
                      )
                    }
                    onChange={toggleBatchPage}
                    className="accent-eve-accent cursor-pointer"
                  />
                </th>
              )}
              <th className="min-w-[24px] px-1 py-2"></th>
              <th
                className="min-w-[32px] px-1 py-2 text-center text-[10px] uppercase tracking-wider text-eve-dim"
                title={t("execPlanTitle")}
              >
                📊
              </th>
              {showOperatorColumns && (
                <>
                  <th className="min-w-[92px] px-2 py-2 text-left text-[10px] uppercase tracking-wider text-eve-dim font-medium">
                    Action
                  </th>
                  <th className="min-w-[96px] px-2 py-2 text-right text-[10px] uppercase tracking-wider text-eve-dim font-medium">
                    Delta/day
                  </th>
                </>
              )}
              {columnDefs.map((col) => {
                const tooltipKey = metricTooltipKeys[col.key];
                return (
                  <th
                    key={col.key}
                    onClick={() => toggleSort(col.key)}
                    className={`${col.width} px-2 py-2 text-left text-[10px] uppercase tracking-wider
                      text-eve-dim font-medium cursor-pointer select-none
                      hover:text-eve-accent transition-colors ${
                        sortKey === col.key ? "text-eve-accent" : ""
                      }`}
                  >
                    <span className="inline-flex items-center">
                      {t(col.labelKey)}
                      {sortKey === col.key && (
                        <span className="ml-1">
                          {sortDir === "asc" ? "▲" : "▼"}
                        </span>
                      )}
                      {tooltipKey && (
                        <MetricTooltipContent metricKey={tooltipKey} t={t} />
                      )}
                    </span>
                  </th>
                );
              })}
            </tr>
          </thead>
          <tbody>
            {pageRows.map((row, i) => {
              const key = stationRowKey(row);
              const commandRow = commandRowsByKey[key];
              const batchSelected = selectedBatchKeys.has(key);
              const hiddenEntry = hiddenTradeMap[key];
              return (
                <tr
                  key={key}
                  className={`${getRowClass(row, safePage * STATION_PAGE_SIZE + i, commandRow)} ${
                    pinnedKeys.has(key) ? "bg-eve-accent/10 border-l-2 border-l-eve-accent" : ""
                  } ${hiddenEntry ? "opacity-60" : ""}`}
                  onContextMenu={(e) => {
                    e.preventDefault();
                    setContextMenu({ x: e.clientX, y: e.clientY, row });
                  }}
                >
                  {showOperatorColumns && (
                    <td className="w-8 px-1 py-1 text-center">
                      <input
                        type="checkbox"
                        checked={batchSelected}
                        onChange={() => toggleBatchRow(key)}
                        className="accent-eve-accent cursor-pointer"
                      />
                    </td>
                  )}
                  {/* Risk indicator */}
                  <td className="px-1 py-1 text-center">
                    {hiddenEntry
                      ? hiddenEntry.mode === "ignored"
                        ? "✖"
                        : "✓"
                      : row.IsHighRiskFlag
                        ? "🚨"
                        : row.IsExtremePriceFlag
                          ? "⚠️"
                          : ""}
                  </td>
                  <td className="px-1 py-1 text-center">
                    {rowRegionID(row, regionId) > 0 && (
                      <button
                        type="button"
                        onClick={() => setExecPlanRow(row)}
                        className="text-eve-dim hover:text-eve-accent transition-colors text-sm"
                        title={t("execPlanTitle")}
                      >
                        📊
                      </button>
                    )}
                  </td>
                  {showOperatorColumns && (
                    <>
                      <td className="px-2 py-1 text-left">
                        <span
                          className={`inline-flex items-center px-1.5 py-0.5 rounded-sm border text-[10px] uppercase tracking-wide ${actionBadgeClass(commandRow?.recommended_action)}`}
                          title={commandRow?.action_reason || "no action explanation"}
                        >
                          {actionLabel(commandRow?.recommended_action)}
                        </span>
                      </td>
                      <td
                        className={`px-2 py-1 text-right font-mono ${
                          (commandRow?.expected_delta_daily_profit ?? 0) >= 0
                            ? "text-emerald-300"
                            : "text-red-300"
                        }`}
                      >
                        {commandRow
                          ? `${commandRow.expected_delta_daily_profit >= 0 ? "+" : ""}${formatISK(commandRow.expected_delta_daily_profit)}`
                          : "\u2014"}
                      </td>
                    </>
                  )}
                  {columnDefs.map((col) => (
                  <td
                    key={col.key}
                    className={`px-2 py-1 ${col.width} truncate ${
                      col.key === "CTS"
                        ? `font-mono font-bold ${getCTSColor(row.CTS)}`
                        : col.key === "SDS"
                          ? `font-mono ${getSDSColor(row.SDS)}`
                          : col.numeric
                            ? "text-eve-accent font-mono"
                            : "text-eve-text"
                    }`}
                  >
                    {col.key === "TypeName" ? (
                      <div className="flex items-center gap-1">
                        <span className="truncate">{formatCell(col, row)}</span>
                        {isLoggedIn && (
                          <button
                            type="button"
                            className="shrink-0 text-eve-dim hover:text-eve-accent transition-colors"
                            title={t("openMarket")}
                            onClick={async (e) => {
                              e.stopPropagation();
                              try {
                                await openMarketInGame(row.TypeID);
                                addToast(t("actionSuccess"), "success", 2000);
                              } catch (err: any) {
                                const { messageKey, duration } =
                                  handleEveUIError(err);
                                addToast(t(messageKey), "error", duration);
                              }
                            }}
                          >
                            🎮
                          </button>
                        )}
                      </div>
                    ) : (
                      formatCell(col, row)
                    )}
                  </td>
                  ))}
                </tr>
              );
            })}
            {displayRows.length === 0 && !scanning && (
              <tr>
                <td
                  colSpan={columnDefs.length + 2 + (showOperatorColumns ? 3 : 0)}
                  className="p-0"
                >
                  {results.length > 0 && hiddenCounts.total > 0 && !showHiddenRows ? (
                    <div className="p-6 text-center text-sm text-eve-dim">
                      All rows are hidden by filters. Enable{" "}
                      <span className="text-eve-accent">Show hidden</span> or open{" "}
                      <span className="text-eve-accent">Ignored ({hiddenCounts.total})</span>.
                    </div>
                  ) : (
                    <EmptyState reason="no_scan_yet" wikiSlug="Station-Trading" />
                  )}
                </td>
              </tr>
            )}
          </tbody>
        </table>
        </div>
      </div>

      {/* Summary */}
      {summary && displayRows.length > 0 && (
        <div className="shrink-0 flex items-center gap-6 px-3 py-1.5 border-t border-eve-border text-xs">
          <span className="text-eve-dim">
            {t("totalProfit")}:{" "}
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
          <span className="text-eve-dim">
            {t("avgCTS")}:{" "}
            <span
              className={`font-mono font-semibold ${getCTSColor(summary.avgCTS)}`}
            >
              {summary.avgCTS.toFixed(1)}
            </span>
          </span>
          {showOperatorColumns && commandSummary && (
            <span className="text-eve-dim">
              actions:{" "}
              <span className="text-eve-accent font-mono font-semibold">
                {commandSummary.new_entry_count} new / {commandSummary.reprice_count} reprice / {commandSummary.cancel_count} cancel
              </span>
            </span>
          )}
          {showOperatorColumns && selectedBatchKeys.size > 0 && (
            <span className="text-eve-dim italic">
              (selected {selectedBatchKeys.size})
            </span>
          )}
        </div>
      )}

      {/* Context menu (right-click) */}
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
              onClick={() => copyText(contextMenu.row.StationName ?? "")}
            />
            <ContextItem
              label={t("copyTradeRoute")}
              onClick={() =>
                copyText(
                  `${contextMenu.row.TypeName} @ ${contextMenu.row.StationName}`,
                )
              }
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
                  : `⭐ ${t("trackItem")}`
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
                label="Unhide in scan"
                onClick={() => {
                  unhideRowByKey(contextHiddenEntry.key);
                  setContextMenu(null);
                }}
              />
            ) : (
              <>
                <ContextItem
                  label="Mark done (hide until refresh)"
                  onClick={() => {
                    void setRowHiddenState(contextMenu.row, "done");
                  }}
                />
                <ContextItem
                  label="Ignore (hide until unignore)"
                  onClick={() => {
                    void setRowHiddenState(contextMenu.row, "ignored");
                  }}
                />
              </>
            )}
            {rowRegionID(contextMenu.row, regionId) > 0 && (
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
                  label={`🎯 ${t("setDestination")}`}
                  onClick={async () => {
                    try {
                      await setWaypointInGame(
                        rowSystemID(contextMenu.row, systemId),
                      );
                      addToast(t("actionSuccess"), "success", 2000);
                    } catch (err: any) {
                      const { messageKey, duration } = handleEveUIError(err);
                      addToast(t(messageKey), "error", duration);
                    }
                    setContextMenu(null);
                  }}
                />
              </>
            )}
            <div className="h-px bg-eve-border my-1" />
            <ContextItem
              label={
                pinnedKeys.has(stationRowKey(contextMenu.row))
                  ? t("unpinRow")
                  : t("pinRow")
              }
              onClick={() => {
                togglePin(stationRowKey(contextMenu.row));
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
          <div className="fixed z-[61] left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 w-[min(1000px,92vw)] h-[min(680px,88vh)] rounded-sm border border-eve-border bg-eve-panel shadow-eve-glow-strong p-3 flex flex-col">
            <div className="flex items-center justify-between gap-3">
              <div>
                <h3 className="text-sm uppercase tracking-wider text-eve-text font-semibold">
                  Ignored Trades
                </h3>
                <p className="text-[11px] text-eve-dim mt-0.5">
                  done {hiddenCounts.done} | ignored {hiddenCounts.ignored} |
                  total {hiddenCounts.total}
                </p>
              </div>
              <button
                type="button"
                onClick={() => setIgnoredModalOpen(false)}
                className="px-2 py-1 rounded-sm border border-eve-border/60 text-eve-dim hover:text-eve-accent hover:border-eve-accent/50 transition-colors text-xs"
              >
                Close
              </button>
            </div>

            <div className="mt-3 flex flex-wrap items-center gap-2">
              <input
                value={ignoredSearch}
                onChange={(e) => setIgnoredSearch(e.target.value)}
                placeholder="Search item or station"
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
                    {tab}
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
                Unignore selected
              </button>
              <button
                type="button"
                onClick={() => {
                  void clearDoneHiddenRows();
                }}
                disabled={hiddenCounts.done === 0}
                className="px-2 py-1 rounded-sm border border-eve-border/60 text-eve-text hover:border-eve-accent/40 hover:text-eve-accent transition-colors text-xs disabled:opacity-40 disabled:cursor-not-allowed"
              >
                Clear done
              </button>
              <button
                type="button"
                onClick={() => {
                  void clearAllHiddenRows();
                }}
                disabled={hiddenCounts.total === 0}
                className="px-2 py-1 rounded-sm border border-red-500/50 text-red-300 hover:bg-red-500/10 transition-colors text-xs disabled:opacity-40 disabled:cursor-not-allowed"
              >
                Clear all
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
                        Item
                      </th>
                      <th className="px-2 py-1 text-left text-eve-dim uppercase tracking-wide">
                        Station
                      </th>
                      <th className="px-2 py-1 text-left text-eve-dim uppercase tracking-wide">
                        Type
                      </th>
                      <th className="px-2 py-1 text-left text-eve-dim uppercase tracking-wide">
                        Updated
                      </th>
                      <th className="px-2 py-1 text-right text-eve-dim uppercase tracking-wide">
                        Action
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
                        <td className="px-2 py-1 text-eve-dim">{entry.stationName}</td>
                        <td className="px-2 py-1">
                          <span
                            className={`inline-flex items-center px-1.5 py-0.5 rounded-sm border text-[10px] uppercase tracking-wide ${
                              entry.mode === "ignored"
                                ? "border-red-500/40 text-red-300 bg-red-950/30"
                                : "border-eve-accent/40 text-eve-accent bg-eve-accent/10"
                            }`}
                          >
                            {entry.mode}
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
                            Unignore
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              ) : (
                <div className="h-full flex items-center justify-center text-eve-dim text-xs">
                  No hidden rows for current filter.
                </div>
              )}
            </div>
          </div>
        </>
      )}

      <StationTradingExecutionCalculator
        open={execPlanRow !== null}
        onClose={() => setExecPlanRow(null)}
        typeID={execPlanRow?.TypeID ?? 0}
        typeName={execPlanRow?.TypeName ?? ""}
        regionID={execPlanRow ? rowRegionID(execPlanRow, regionId) : regionId}
        stationID={execPlanRow?.StationID ?? 0}
        defaultQuantity={100}
        brokerFeePercent={splitTradeFees ? undefined : brokerFee}
        salesTaxPercent={splitTradeFees ? undefined : salesTaxPercent}
        buyBrokerFeePercent={splitTradeFees ? buyBrokerFeePercent : undefined}
        sellBrokerFeePercent={splitTradeFees ? sellBrokerFeePercent : undefined}
        buySalesTaxPercent={splitTradeFees ? buySalesTaxPercent : undefined}
        sellSalesTaxPercent={splitTradeFees ? sellSalesTaxPercent : undefined}
        impactDays={avgPricePeriod}
      />
      <StationAIAssistant
        params={params}
        rows={displayRows}
        totalRows={results.length}
        commandRowsByKey={commandRowsByKey}
        regionID={regionId}
        selectedStationLabel={selectedStationLabel}
        scanSnapshot={aiScanSnapshot}
        disabled={scanning}
      />
    </div>
  );
}

function StatusChip({
  label,
  value,
  tone = "neutral",
}: {
  label: string;
  value: string | number;
  tone?: "neutral" | "accent" | "danger" | "warning" | "success";
}) {
  const toneClass =
    tone === "accent"
      ? "border-eve-accent/50 bg-eve-accent/10 text-eve-accent"
      : tone === "danger"
        ? "border-red-500/50 bg-red-950/30 text-red-300"
        : tone === "warning"
          ? "border-amber-500/50 bg-amber-950/30 text-amber-200"
          : tone === "success"
            ? "border-emerald-500/50 bg-emerald-950/30 text-emerald-300"
            : "border-eve-border/60 bg-eve-dark/50 text-eve-dim";
  return (
    <span
      className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-sm border font-mono tabular-nums ${toneClass}`}
    >
      <span className="text-[10px] uppercase tracking-wide opacity-80">{label}</span>
      <span className="text-[11px] text-eve-text font-semibold">{value}</span>
    </span>
  );
}

function PanelSectionHeader({
  title,
  subtitle,
  icon,
}: {
  title: string;
  subtitle?: string;
  icon?: string;
}) {
  return (
    <div className="flex items-center gap-2 border-b border-eve-border/40 pb-2">
      {icon && (
        <span className="text-[11px] text-eve-accent shrink-0">{icon}</span>
      )}
      <div className="min-w-0">
        <h4 className="text-[11px] uppercase tracking-wider text-eve-text font-semibold truncate">
          {title}
        </h4>
        {subtitle && (
          <p className="text-[10px] text-eve-dim truncate">{subtitle}</p>
        )}
      </div>
    </div>
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

// Helper component for metric tooltips
function MetricTooltipContent({
  metricKey,
  t,
}: {
  metricKey: MetricTooltipKey;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}) {
  const tooltipData: Record<
    MetricTooltipKey,
    {
      titleKey: TranslationKey;
      descKey: TranslationKey;
      goodKey?: TranslationKey;
      badKey?: TranslationKey;
    }
  > = {
    CTS: {
      titleKey: "metricCTSTitle",
      descKey: "metricCTSDesc",
      goodKey: "metricCTSGood",
      badKey: "metricCTSBad",
    },
    SDS: {
      titleKey: "metricSDSTitle",
      descKey: "metricSDSDesc",
      goodKey: "metricSDSGood",
      badKey: "metricSDSBad",
    },
    PVI: {
      titleKey: "metricPVITitle",
      descKey: "metricPVIDesc",
      goodKey: "metricPVIGood",
      badKey: "metricPVIBad",
    },
    VWAP: { titleKey: "metricVWAPTitle", descKey: "metricVWAPDesc" },
    OBDS: { titleKey: "metricOBDSTitle", descKey: "metricOBDSDesc" },
    DOS: {
      titleKey: "metricDOSTitle",
      descKey: "metricDOSDesc",
      goodKey: "metricDOSGood",
      badKey: "metricDOSBad",
    },
    S2BBfSRatio: {
      titleKey: "metricBvSTitle",
      descKey: "metricBvSDesc",
      goodKey: "metricBvSGood",
      badKey: "metricBvSBad",
    },
    PeriodROI: {
      titleKey: "metricPeriodROITitle",
      descKey: "metricPeriodROIDesc",
    },
    NowROI: { titleKey: "metricNowROITitle", descKey: "metricNowROIDesc" },
  };

  const data = tooltipData[metricKey];

  return (
    <MetricTooltip
      title={t(data.titleKey)}
      description={t(data.descKey)}
      goodRange={data.goodKey ? t(data.goodKey) : undefined}
      badRange={data.badKey ? t(data.badKey) : undefined}
    />
  );
}
