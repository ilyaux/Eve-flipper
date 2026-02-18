import { useState, useEffect, useMemo, useCallback, useRef } from "react";
import type {
  StationTrade,
  StationInfo,
  ScanParams,
  WatchlistItem,
} from "@/lib/types";
import {
  getStations,
  getStructures,
  scanStation,
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
import {
  STATION_BUILTIN_PRESETS,
  type StationTradingSettings,
} from "@/lib/presets";

type SortKey = keyof StationTrade;
type SortDir = "asc" | "desc";
type CTSProfile = "balanced" | "aggressive" | "defensive";

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
const settingsSectionClass =
  "rounded-sm border border-eve-border/60 bg-gradient-to-br from-eve-panel to-eve-dark/40";

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

export function StationTrading({
  params,
  onChange,
  isLoggedIn = false,
  loadedResults,
}: Props) {
  const { t } = useI18n();

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
  const [loadingStations, setLoadingStations] = useState(false);
  const abortRef = useRef<AbortController | null>(null);

  // System-level metadata (always available even with no NPC stations)
  const [systemRegionId, setSystemRegionId] = useState<number>(0);
  const [systemId, setSystemId] = useState<number>(0);

  // Player structure support
  const [includeStructures, setIncludeStructures] = useState(false);
  const [structureStations, setStructureStations] = useState<StationInfo[]>([]);
  const [loadingStructures, setLoadingStructures] = useState(false);

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
    }
  }, [loadedResults]);

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
  }, []);

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

  // Region ID comes from system metadata, not from stations
  const regionId = systemRegionId;

  const canScan =
    params.system_name &&
    (allStations.length > 0 || radius > 0 || selectedStationId === ALL_STATIONS_ID) &&
    regionId > 0;

  function stationRowKey(row: StationTrade) {
    return `${row.TypeID}-${row.StationID}`;
  }

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
    if (scanning) {
      abortRef.current?.abort();
      return;
    }
    if (!canScan) return;

    const controller = new AbortController();
    abortRef.current = controller;
    setScanning(true);
    setProgress(t("scanStarting"));

    try {
      const scanParams: Parameters<typeof scanStation>[0] = {
        min_margin: minMargin,
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

      // Include player structures if toggle is on
      if (includeStructures && structureStations.length > 0) {
        scanParams.include_structures = true;
        scanParams.structure_ids = structureStations.map((s) => s.id);
      }

      const res = await scanStation(scanParams, setProgress, controller.signal);
      setResults(normalizeStationResults(res));
    } catch (e: unknown) {
      if (e instanceof Error && e.name !== "AbortError") {
        setProgress(t("errorPrefix") + e.message);
      }
    } finally {
      setScanning(false);
    }
  }, [
    scanning,
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
    t,
  ]);

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

  const { pageRows, totalPages, safePage } = useMemo(() => {
    const totalPages = Math.max(1, Math.ceil(sorted.length / STATION_PAGE_SIZE));
    const safePage = Math.min(page, totalPages - 1);
    const pageRows = sorted.slice(
      safePage * STATION_PAGE_SIZE,
      (safePage + 1) * STATION_PAGE_SIZE,
    );
    return { pageRows, totalPages, safePage };
  }, [sorted, page]);

  useEffect(() => {
    setPage(0);
  }, [results, sortKey, sortDir]);

  const toggleSort = (key: SortKey) => {
    if (sortKey === key) setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    else {
      setSortKey(key);
      setSortDir("desc");
    }
  };

  const summary = useMemo(() => {
    if (sorted.length === 0) return null;
    const totalProfit = sorted.reduce((sum, r) => sum + stationDailyProfit(r), 0);
    const avgMargin =
      sorted.reduce((sum, r) => sum + r.MarginPercent, 0) / sorted.length;
    const avgCTS = sorted.reduce((sum, r) => sum + r.CTS, 0) / sorted.length;
    return { totalProfit, avgMargin, avgCTS, count: sorted.length };
  }, [sorted]);

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
  const getRowClass = (row: StationTrade, index: number) => {
    let base = `border-b border-eve-border/50 hover:bg-eve-accent/5 transition-colors ${
      index % 2 === 0 ? "bg-eve-panel" : "bg-eve-dark"
    }`;
    if (row.IsHighRiskFlag) base += " border-l-2 border-l-eve-error";
    else if (row.IsExtremePriceFlag) base += " border-l-2 border-l-yellow-500";
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
          <div className="mt-3 pt-3 border-t border-eve-border/30 flex justify-end">
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
      <div className="shrink-0 flex items-center gap-2 px-2 py-1 text-xs text-eve-dim">
        {scanning ? (
          <span className="flex items-center gap-2">
            <span className="w-2 h-2 rounded-full bg-eve-accent animate-pulse" />
            {progress}
          </span>
        ) : results.length > 0 ? (
          <span className="flex items-center gap-4">
            <span>{t("foundStationDeals", { count: results.length })}</span>
            <span className="text-eve-dim">
              🚨 = {t("highRisk")} | ⚠️ = {t("extremePrice")}
            </span>
          </span>
        ) : null}
        <div className="flex-1" />
        {!scanning && sorted.length > STATION_PAGE_SIZE && (
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
      </div>

      {/* Table */}
      <div className="flex-1 min-h-0 overflow-auto border border-eve-border rounded-sm table-scroll-wrapper table-scroll-container">
        <table className="w-full text-sm">
          <thead className="sticky top-0 z-10">
            <tr className="bg-eve-dark border-b border-eve-border">
              <th className="min-w-[24px] px-1 py-2"></th>
              <th
                className="min-w-[32px] px-1 py-2 text-center text-[10px] uppercase tracking-wider text-eve-dim"
                title={t("execPlanTitle")}
              >
                📊
              </th>
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
            {pageRows.map((row, i) => (
              <tr
                key={stationRowKey(row)}
                className={`${getRowClass(row, safePage * STATION_PAGE_SIZE + i)} ${pinnedKeys.has(stationRowKey(row)) ? "bg-eve-accent/10 border-l-2 border-l-eve-accent" : ""}`}
                onContextMenu={(e) => {
                  e.preventDefault();
                  setContextMenu({ x: e.clientX, y: e.clientY, row });
                }}
              >
                {/* Risk indicator */}
                <td className="px-1 py-1 text-center">
                  {row.IsHighRiskFlag
                    ? "🚨"
                    : row.IsExtremePriceFlag
                      ? "⚠️"
                      : ""}
                </td>
                <td className="px-1 py-1 text-center">
                  {regionId > 0 && (
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
            ))}
            {results.length === 0 && !scanning && (
              <tr>
                <td colSpan={columnDefs.length + 2} className="p-0">
                  <EmptyState reason="no_scan_yet" wikiSlug="Station-Trading" />
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {/* Summary */}
      {summary && results.length > 0 && (
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
            {regionId > 0 && (
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
                      await setWaypointInGame(systemId);
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

      <StationTradingExecutionCalculator
        open={execPlanRow !== null}
        onClose={() => setExecPlanRow(null)}
        typeID={execPlanRow?.TypeID ?? 0}
        typeName={execPlanRow?.TypeName ?? ""}
        regionID={regionId}
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
    </div>
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
