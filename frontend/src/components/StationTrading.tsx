import { useState, useEffect, useMemo, useCallback, useRef } from "react";
import type { StationTrade, StationInfo, ScanParams, WatchlistItem } from "@/lib/types";
import { getStations, scanStation, getWatchlist, addToWatchlist, removeFromWatchlist } from "@/lib/api";
import { formatISK, formatMargin, formatNumber } from "@/lib/format";
import { useI18n, type TranslationKey } from "@/lib/i18n";
import { MetricTooltip } from "./Tooltip";
import { EmptyState } from "./EmptyState";
import { StationTradingExecutionCalculator } from "./StationTradingExecutionCalculator";
import { useGlobalToast } from "./Toast";
import {
  TabSettingsPanel,
  SettingsField,
  SettingsNumberInput,
  SettingsCheckbox,
  SettingsGrid,
  SettingsSelect,
} from "./TabSettingsPanel";
import { SystemAutocomplete } from "./SystemAutocomplete";

type SortKey = keyof StationTrade;
type SortDir = "asc" | "desc";

interface Props {
  params: ScanParams;
  /** Called when system (or other global param) is changed in this tab; updates global filter */
  onChange?: (params: ScanParams) => void;
  isLoggedIn?: boolean;
}

// Metric tooltip keys mapping
type MetricTooltipKey = "CTS" | "SDS" | "PVI" | "VWAP" | "OBDS" | "DOS" | "BvSRatio" | "PeriodROI" | "NowROI";

const metricTooltipKeys: Partial<Record<SortKey, MetricTooltipKey>> = {
  CTS: "CTS",
  SDS: "SDS",
  PVI: "PVI",
  VWAP: "VWAP",
  OBDS: "OBDS",
  DOS: "DOS",
  BvSRatio: "BvSRatio",
  PeriodROI: "PeriodROI",
  NowROI: "NowROI",
};

const columnDefs: { key: SortKey; labelKey: TranslationKey; width: string; numeric: boolean }[] = [
  { key: "TypeName", labelKey: "colItem", width: "min-w-[150px]", numeric: false },
  { key: "StationName", labelKey: "colStationName", width: "min-w-[150px]", numeric: false },
  { key: "CTS", labelKey: "colCTS", width: "min-w-[60px]", numeric: true },
  { key: "ProfitPerUnit", labelKey: "colProfitPerUnit", width: "min-w-[90px]", numeric: true },
  { key: "MarginPercent", labelKey: "colMargin", width: "min-w-[70px]", numeric: true },
  { key: "PeriodROI", labelKey: "colPeriodROI", width: "min-w-[80px]", numeric: true },
  { key: "BuyUnitsPerDay", labelKey: "colBuyPerDay", width: "min-w-[80px]", numeric: true },
  { key: "BvSRatio", labelKey: "colBvS", width: "min-w-[60px]", numeric: true },
  { key: "DOS", labelKey: "colDOS", width: "min-w-[60px]", numeric: true },
  { key: "SDS", labelKey: "colSDS", width: "min-w-[50px]", numeric: true },
  { key: "TotalProfit", labelKey: "colDailyProfit", width: "min-w-[100px]", numeric: true },
];

// Sentinel value for "All stations"
const ALL_STATIONS_ID = 0;

export function StationTrading({ params, onChange, isLoggedIn = false }: Props) {
  const { t } = useI18n();

  const [stations, setStations] = useState<StationInfo[]>([]);
  const [selectedStationId, setSelectedStationId] = useState<number>(ALL_STATIONS_ID);
  const [brokerFee, setBrokerFee] = useState(3.0);
  const [salesTaxPercent, setSalesTaxPercent] = useState(8);
  const [radius, setRadius] = useState(0);
  const [minDailyVolume, setMinDailyVolume] = useState(5);
  const [results, setResults] = useState<StationTrade[]>([]);
  const [scanning, setScanning] = useState(false);
  const [progress, setProgress] = useState("");
  const [loadingStations, setLoadingStations] = useState(false);
  const abortRef = useRef<AbortController | null>(null);

  // EVE Guru Profit Filters
  const [minItemProfit, setMinItemProfit] = useState(0);
  const [minDemandPerDay, setMinDemandPerDay] = useState(1);

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

  // Sort
  const [sortKey, setSortKey] = useState<SortKey>("CTS");
  const [sortDir, setSortDir] = useState<SortDir>("desc");

  // Execution plan popup
  const [execPlanRow, setExecPlanRow] = useState<StationTrade | null>(null);

  // Context menu (right-click)
  const [contextMenu, setContextMenu] = useState<{ x: number; y: number; row: StationTrade } | null>(null);
  const contextMenuRef = useRef<HTMLDivElement>(null);
  const [pinnedKeys, setPinnedKeys] = useState<Set<string>>(new Set());

  // Watchlist
  const { addToast } = useGlobalToast();
  const [watchlist, setWatchlist] = useState<WatchlistItem[]>([]);
  useEffect(() => {
    getWatchlist().then(setWatchlist).catch(() => {});
  }, []);
  const watchlistIds = useMemo(() => new Set(watchlist.map((w) => w.type_id)), [watchlist]);

  // Sync sales tax from global params when they change (e.g. from ParametersPanel on other tabs)
  useEffect(() => {
    const pct = params.sales_tax_percent ?? 8;
    setSalesTaxPercent(pct);
  }, [params.sales_tax_percent]);

  // Load stations when system changes
  useEffect(() => {
    if (!params.system_name) return;
    setLoadingStations(true);
    getStations(params.system_name)
      .then((s) => {
        setStations(s);
        setSelectedStationId(ALL_STATIONS_ID);
      })
      .catch(() => setStations([]))
      .finally(() => setLoadingStations(false));
  }, [params.system_name]);

  const selectedStation = useMemo(
    () => stations.find((s) => s.id === selectedStationId) ?? null,
    [stations, selectedStationId]
  );

  // Determine region_id from selected station or first station
  const regionId = useMemo(() => {
    if (selectedStation) return selectedStation.region_id;
    if (stations.length > 0) return stations[0].region_id;
    return 0;
  }, [selectedStation, stations]);

  const canScan = params.system_name && (stations.length > 0 || radius > 0);

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
    [addToast, t]
  );

  // Keep context menu inside viewport
  useEffect(() => {
    if (contextMenu && contextMenuRef.current) {
      const menu = contextMenuRef.current;
      const rect = menu.getBoundingClientRect();
      const padding = 10;
      let x = contextMenu.x;
      let y = contextMenu.y;
      if (x + rect.width > window.innerWidth - padding) x = window.innerWidth - rect.width - padding;
      if (y + rect.height > window.innerHeight - padding) y = window.innerHeight - rect.height - padding;
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
        min_margin: params.min_margin,
        sales_tax_percent: salesTaxPercent,
        broker_fee: brokerFee,
        min_daily_volume: minDailyVolume,
        max_results: params.max_results,
        // EVE Guru Profit Filters
        min_item_profit: minItemProfit > 0 ? minItemProfit : undefined,
        min_demand_per_day: minDemandPerDay > 0 ? minDemandPerDay : undefined,
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

      const res = await scanStation(scanParams, setProgress, controller.signal);
      setResults(res);
    } catch (e: unknown) {
      if (e instanceof Error && e.name !== "AbortError") {
        setProgress(t("errorPrefix") + e.message);
      }
    } finally {
      setScanning(false);
    }
  }, [scanning, canScan, selectedStationId, regionId, params, brokerFee, salesTaxPercent, radius, minDailyVolume,
      minItemProfit, minDemandPerDay, avgPricePeriod, minPeriodROI, bvsRatioMin, bvsRatioMax,
      maxPVI, maxSDS, limitBuyToPriceLow, flagExtremePrices, t]);

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

  const toggleSort = (key: SortKey) => {
    if (sortKey === key) setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    else { setSortKey(key); setSortDir("desc"); }
  };

  const summary = useMemo(() => {
    if (sorted.length === 0) return null;
    const totalProfit = sorted.reduce((sum, r) => sum + r.TotalProfit, 0);
    const avgMargin = sorted.reduce((sum, r) => sum + r.MarginPercent, 0) / sorted.length;
    const avgCTS = sorted.reduce((sum, r) => sum + r.CTS, 0) / sorted.length;
    return { totalProfit, avgMargin, avgCTS, count: sorted.length };
  }, [sorted]);

  const formatCell = (col: (typeof columnDefs)[number], row: StationTrade): string => {
    const val = row[col.key];
    if (col.key === "BuyPrice" || col.key === "SellPrice" || col.key === "Spread" || col.key === "TotalProfit" || col.key === "ProfitPerUnit" || col.key === "CapitalRequired" || col.key === "VWAP") {
      const n = val as number | undefined;
      return n != null && Number.isFinite(n) ? formatISK(n) : "";
    }
    if (col.key === "MarginPercent" || col.key === "NowROI" || col.key === "PeriodROI" || col.key === "PVI") {
      return formatMargin(val as number);
    }
    if (col.key === "BvSRatio" || col.key === "DOS" || col.key === "OBDS") {
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
      index % 2 === 0 ? "bg-eve-panel" : "bg-[#161616]"
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
    for (const st of stations) {
      opts.push({ value: st.id, label: st.name });
    }
    return opts;
  }, [stations, t]);

  return (
    <div className="flex-1 flex flex-col min-h-0">
      {/* Settings Panel - unified design */}
      <div className="shrink-0 m-2">
        <TabSettingsPanel
          title={t("stationSettings")}
          hint={t("stationSettingsHint")}
          icon="🏪"
          defaultExpanded={true}
          help={{ stepKeys: ["helpStationStep1", "helpStationStep2", "helpStationStep3"], wikiSlug: "Station-Trading" }}
        >
          {/* System (from global filter or geolocation) & Station */}
          <SettingsGrid cols={5}>
            <SettingsField label={t("system")}>
              <SystemAutocomplete
                value={params.system_name}
                onChange={(v) => onChange?.({ ...params, system_name: v })}
                showLocationButton={true}
                isLoggedIn={isLoggedIn}
              />
            </SettingsField>
            <SettingsField label={t("stationSelect")}>
              {loadingStations ? (
                <div className="h-[34px] flex items-center text-xs text-eve-dim">{t("loadingStations")}</div>
              ) : stations.length === 0 ? (
                <div className="h-[34px] flex items-center text-xs text-eve-dim">{t("noStations")}</div>
              ) : (
                <SettingsSelect
                  value={selectedStationId}
                  onChange={(v) => setSelectedStationId(Number(v))}
                  options={stationOptions}
                />
              )}
            </SettingsField>

            <SettingsField label={t("stationRadius")}>
              <SettingsNumberInput
                value={radius}
                onChange={(v) => setRadius(Math.max(0, Math.min(50, v)))}
                min={0}
                max={50}
              />
            </SettingsField>

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
                onChange={(v) => setSalesTaxPercent(Math.max(0, Math.min(100, v)))}
                min={0}
                max={100}
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
          </SettingsGrid>

          {/* Advanced Filters - collapsible subsection */}
          <details className="mt-3 group">
            <summary className="cursor-pointer text-xs text-eve-dim hover:text-eve-accent transition-colors flex items-center gap-1">
              <span className="group-open:rotate-90 transition-transform">▶</span>
              {t("advancedFilters")}
            </summary>
            <div className="mt-3 pt-3 border-t border-eve-border/30">
              <SettingsGrid cols={5}>
                <SettingsField label={t("minDemandPerDay")}>
                  <SettingsNumberInput value={minDemandPerDay} onChange={setMinDemandPerDay} min={0} step={0.1} />
                </SettingsField>
                <SettingsField label={t("avgPricePeriod")}>
                  <SettingsNumberInput value={avgPricePeriod} onChange={setAvgPricePeriod} min={7} max={365} />
                </SettingsField>
                <SettingsField label={t("minPeriodROI")}>
                  <SettingsNumberInput value={minPeriodROI} onChange={setMinPeriodROI} min={0} />
                </SettingsField>
                <SettingsField label={t("maxPVI")}>
                  <SettingsNumberInput value={maxPVI} onChange={setMaxPVI} min={0} />
                </SettingsField>
                <SettingsField label={t("maxSDS")}>
                  <SettingsNumberInput value={maxSDS} onChange={setMaxSDS} min={0} max={100} />
                </SettingsField>
              </SettingsGrid>
              <div className="mt-3">
                <SettingsGrid cols={4}>
                  <SettingsField label={t("bvsRatioMin")}>
                    <SettingsNumberInput value={bvsRatioMin} onChange={setBvsRatioMin} min={0} step={0.1} />
                  </SettingsField>
                  <SettingsField label={t("bvsRatioMax")}>
                    <SettingsNumberInput value={bvsRatioMax} onChange={setBvsRatioMax} min={0} step={0.1} />
                  </SettingsField>
                  <SettingsField label={t("limitBuyToPriceLow")}>
                    <SettingsCheckbox checked={limitBuyToPriceLow} onChange={setLimitBuyToPriceLow} />
                  </SettingsField>
                  <SettingsField label={t("flagExtremePrices")}>
                    <SettingsCheckbox checked={flagExtremePrices} onChange={setFlagExtremePrices} />
                  </SettingsField>
                </SettingsGrid>
              </div>
            </div>
          </details>

          {/* Scan button inside settings */}
          <div className="mt-3 pt-3 border-t border-eve-border/30 flex justify-end">
            <button
              onClick={handleScan}
              disabled={!canScan}
              className={`px-5 py-1.5 rounded-sm text-xs font-semibold uppercase tracking-wider transition-all
                ${scanning
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
      </div>

      {/* Table */}
      <div className="flex-1 min-h-0 overflow-auto border border-eve-border rounded-sm">
        <table className="w-full text-sm">
          <thead className="sticky top-0 z-10">
            <tr className="bg-eve-dark border-b border-eve-border">
              <th className="min-w-[24px] px-1 py-2"></th>
              <th className="min-w-[32px] px-1 py-2 text-center text-[10px] uppercase tracking-wider text-eve-dim" title={t("execPlanTitle")}>📊</th>
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
                        <span className="ml-1">{sortDir === "asc" ? "▲" : "▼"}</span>
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
            {sorted.map((row, i) => (
              <tr
                key={stationRowKey(row)}
                className={`${getRowClass(row, i)} ${pinnedKeys.has(stationRowKey(row)) ? "bg-eve-accent/10 border-l-2 border-l-eve-accent" : ""}`}
                onContextMenu={(e) => {
                  e.preventDefault();
                  setContextMenu({ x: e.clientX, y: e.clientY, row });
                }}
              >
                {/* Risk indicator */}
                <td className="px-1 py-1 text-center">
                  {row.IsHighRiskFlag ? "🚨" : row.IsExtremePriceFlag ? "⚠️" : ""}
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
                      col.key === "CTS" ? `font-mono font-bold ${getCTSColor(row.CTS)}` :
                      col.key === "SDS" ? `font-mono ${getSDSColor(row.SDS)}` :
                      col.numeric ? "text-eve-accent font-mono" : "text-eve-text"
                    }`}
                  >
                    {formatCell(col, row)}
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
            <span className="text-eve-accent font-mono font-semibold">{formatISK(summary.totalProfit)}</span>
          </span>
          <span className="text-eve-dim">
            {t("avgMargin")}:{" "}
            <span className="text-eve-accent font-mono font-semibold">{formatMargin(summary.avgMargin)}</span>
          </span>
          <span className="text-eve-dim">
            {t("avgCTS")}:{" "}
            <span className={`font-mono font-semibold ${getCTSColor(summary.avgCTS)}`}>{summary.avgCTS.toFixed(1)}</span>
          </span>
        </div>
      )}

      {/* Context menu (right-click) */}
      {contextMenu && (
        <>
          <div className="fixed inset-0 z-50" onClick={() => setContextMenu(null)} />
          <div
            ref={contextMenuRef}
            className="fixed z-50 bg-eve-panel border border-eve-border rounded-sm shadow-eve-glow-strong py-1 min-w-[200px]"
            style={{ left: contextMenu.x, top: contextMenu.y }}
          >
            <ContextItem label={t("copyItem")} onClick={() => copyText(contextMenu.row.TypeName ?? "")} />
            <ContextItem label={t("copyBuyStation")} onClick={() => copyText(contextMenu.row.StationName ?? "")} />
            <ContextItem
              label={t("copyTradeRoute")}
              onClick={() => copyText(`${contextMenu.row.TypeName} @ ${contextMenu.row.StationName}`)}
            />
            <div className="h-px bg-eve-border my-1" />
            <ContextItem
              label={t("openInEveref")}
              onClick={() => {
                window.open(`https://everef.net/type/${contextMenu.row.TypeID}`, "_blank");
                setContextMenu(null);
              }}
            />
            <ContextItem
              label={t("openInJitaSpace")}
              onClick={() => {
                window.open(`https://www.jita.space/market/${contextMenu.row.TypeID}`, "_blank");
                setContextMenu(null);
              }}
            />
            <div className="h-px bg-eve-border my-1" />
            <ContextItem
              label={watchlistIds.has(contextMenu.row.TypeID) ? t("removeFromWatchlist") : `⭐ ${t("addToWatchlist")}`}
              onClick={() => {
                const row = contextMenu.row;
                if (watchlistIds.has(row.TypeID)) {
                  removeFromWatchlist(row.TypeID).then(setWatchlist).catch(() => {});
                } else {
                  addToWatchlist(row.TypeID, row.TypeName).then(setWatchlist).catch(() => {});
                }
                setContextMenu(null);
              }}
            />
            <ContextItem
              label={pinnedKeys.has(stationRowKey(contextMenu.row)) ? t("unpinRow") : t("pinRow")}
              onClick={() => {
                togglePin(stationRowKey(contextMenu.row));
                setContextMenu(null);
              }}
            />
            {regionId > 0 && (
              <ContextItem
                label={t("execPlanCalculator")}
                onClick={() => {
                  setExecPlanRow(contextMenu.row);
                  setContextMenu(null);
                }}
              />
            )}
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
        brokerFeePercent={brokerFee}
        salesTaxPercent={salesTaxPercent}
        impactDays={avgPricePeriod}
      />
    </div>
  );
}

function ContextItem({ label, onClick }: { label: string; onClick: () => void }) {
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
  t 
}: { 
  metricKey: MetricTooltipKey; 
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}) {
  const tooltipData: Record<MetricTooltipKey, { titleKey: TranslationKey; descKey: TranslationKey; goodKey?: TranslationKey; badKey?: TranslationKey }> = {
    CTS: { titleKey: "metricCTSTitle", descKey: "metricCTSDesc", goodKey: "metricCTSGood", badKey: "metricCTSBad" },
    SDS: { titleKey: "metricSDSTitle", descKey: "metricSDSDesc", goodKey: "metricSDSGood", badKey: "metricSDSBad" },
    PVI: { titleKey: "metricPVITitle", descKey: "metricPVIDesc", goodKey: "metricPVIGood", badKey: "metricPVIBad" },
    VWAP: { titleKey: "metricVWAPTitle", descKey: "metricVWAPDesc" },
    OBDS: { titleKey: "metricOBDSTitle", descKey: "metricOBDSDesc" },
    DOS: { titleKey: "metricDOSTitle", descKey: "metricDOSDesc", goodKey: "metricDOSGood", badKey: "metricDOSBad" },
    BvSRatio: { titleKey: "metricBvSTitle", descKey: "metricBvSDesc", goodKey: "metricBvSGood", badKey: "metricBvSBad" },
    PeriodROI: { titleKey: "metricPeriodROITitle", descKey: "metricPeriodROIDesc" },
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
