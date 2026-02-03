import { useState, useEffect, useMemo, useCallback, useRef } from "react";
import type { StationTrade, StationInfo, ScanParams } from "@/lib/types";
import { getStations, scanStation } from "@/lib/api";
import { formatISK, formatMargin, formatNumber } from "@/lib/format";
import { useI18n, type TranslationKey } from "@/lib/i18n";

type SortKey = keyof StationTrade;
type SortDir = "asc" | "desc";

interface Props {
  params: ScanParams;
}

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

// Collapsible section component
function FilterSection({ title, defaultOpen, children }: { title: string; defaultOpen?: boolean; children: React.ReactNode }) {
  const [open, setOpen] = useState(defaultOpen ?? false);
  return (
    <div className="border border-eve-border rounded-sm mb-2">
      <button
        onClick={() => setOpen(!open)}
        className="w-full flex items-center justify-between px-2 py-1 text-xs text-eve-dim hover:text-eve-accent transition-colors"
      >
        <span className="font-medium uppercase tracking-wider">{title}</span>
        <span>{open ? "▼" : "▶"}</span>
      </button>
      {open && <div className="px-2 pb-2 flex flex-wrap gap-3">{children}</div>}
    </div>
  );
}

// Small input component
function SmallInput({ label, value, onChange, min, max, step, className }: {
  label: string;
  value: number;
  onChange: (v: number) => void;
  min?: number;
  max?: number;
  step?: number;
  className?: string;
}) {
  return (
    <div className={`flex items-center gap-1 ${className || ''}`}>
      <label className="text-eve-dim text-xs whitespace-nowrap">{label}:</label>
      <input
        type="number"
        value={value}
        onChange={(e) => onChange(parseFloat(e.target.value) || 0)}
        step={step}
        min={min}
        max={max}
        className="w-16 bg-eve-input border border-eve-border rounded-sm px-1 py-0.5 text-eve-text text-xs
                   [appearance:textfield] [&::-webkit-outer-spin-button]:appearance-none [&::-webkit-inner-spin-button]:appearance-none"
      />
    </div>
  );
}

// Checkbox input
function CheckboxInput({ label, checked, onChange }: { label: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <label className="flex items-center gap-1 text-xs text-eve-dim cursor-pointer">
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        className="accent-eve-accent"
      />
      {label}
    </label>
  );
}

export function StationTrading({ params }: Props) {
  const { t } = useI18n();

  const [stations, setStations] = useState<StationInfo[]>([]);
  const [selectedStationId, setSelectedStationId] = useState<number>(ALL_STATIONS_ID);
  const [brokerFee, setBrokerFee] = useState(3.0);
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
        sales_tax_percent: params.sales_tax_percent,
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
  }, [scanning, canScan, selectedStationId, regionId, params, brokerFee, radius, minDailyVolume,
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
      return formatISK(val as number);
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

  return (
    <div className="flex-1 flex flex-col min-h-0">
      {/* Controls */}
      <div className="shrink-0 px-2 py-1.5 border-b border-eve-border">
        {/* Main controls row */}
        <div className="flex items-center gap-3 text-xs flex-wrap mb-2">
          {/* Station selector */}
          <label className="text-eve-dim">{t("stationSelect")}:</label>
          {loadingStations ? (
            <span className="text-eve-dim">{t("loadingStations")}</span>
          ) : stations.length === 0 ? (
            <span className="text-eve-dim">{t("noStations")}</span>
          ) : (
            <select
              value={selectedStationId}
              onChange={(e) => setSelectedStationId(Number(e.target.value))}
              className="bg-eve-input border border-eve-border rounded-sm px-2 py-1 text-eve-text text-xs max-w-[300px] truncate"
            >
              <option value={ALL_STATIONS_ID}>{t("allStations")}</option>
              {stations.map((st) => (
                <option key={st.id} value={st.id}>
                  {st.name}
                </option>
              ))}
            </select>
          )}

          <SmallInput label={t("stationRadius")} value={radius} onChange={(v) => setRadius(Math.max(0, Math.min(50, v)))} min={0} max={50} />
          <SmallInput label={t("brokerFee")} value={brokerFee} onChange={setBrokerFee} min={0} max={10} step={0.1} />

          <div className="flex-1" />

          {/* Scan button */}
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

        {/* Filter sections */}
        <div className="grid grid-cols-1 md:grid-cols-3 gap-2">
          <FilterSection title={t("profitFilters")} defaultOpen={true}>
            <SmallInput label={t("minItemProfit")} value={minItemProfit} onChange={setMinItemProfit} min={0} />
            <SmallInput label={t("minDailyVolume")} value={minDailyVolume} onChange={setMinDailyVolume} min={0} />
            <SmallInput label={t("minDemandPerDay")} value={minDemandPerDay} onChange={setMinDemandPerDay} min={0} step={0.1} />
          </FilterSection>

          <FilterSection title={t("riskProfile")}>
            <SmallInput label={t("avgPricePeriod")} value={avgPricePeriod} onChange={setAvgPricePeriod} min={7} max={365} />
            <SmallInput label={t("minPeriodROI")} value={minPeriodROI} onChange={setMinPeriodROI} min={0} />
            <SmallInput label={t("maxPVI")} value={maxPVI} onChange={setMaxPVI} min={0} />
            <SmallInput label={t("maxSDS")} value={maxSDS} onChange={setMaxSDS} min={0} max={100} />
          </FilterSection>

          <FilterSection title={t("bvsAndLimits")}>
            <SmallInput label={t("bvsRatioMin")} value={bvsRatioMin} onChange={setBvsRatioMin} min={0} step={0.1} />
            <SmallInput label={t("bvsRatioMax")} value={bvsRatioMax} onChange={setBvsRatioMax} min={0} step={0.1} />
            <CheckboxInput label={t("limitBuyToPriceLow")} checked={limitBuyToPriceLow} onChange={setLimitBuyToPriceLow} />
            <CheckboxInput label={t("flagExtremePrices")} checked={flagExtremePrices} onChange={setFlagExtremePrices} />
          </FilterSection>
        </div>
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
              {columnDefs.map((col) => (
                <th
                  key={col.key}
                  onClick={() => toggleSort(col.key)}
                  className={`${col.width} px-2 py-2 text-left text-[10px] uppercase tracking-wider
                    text-eve-dim font-medium cursor-pointer select-none
                    hover:text-eve-accent transition-colors ${
                      sortKey === col.key ? "text-eve-accent" : ""
                    }`}
                >
                  {t(col.labelKey)}
                  {sortKey === col.key && (
                    <span className="ml-1">{sortDir === "asc" ? "▲" : "▼"}</span>
                  )}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {sorted.map((row, i) => (
              <tr
                key={`${row.TypeID}-${row.StationID}`}
                className={getRowClass(row, i)}
              >
                {/* Risk indicator */}
                <td className="px-1 py-1 text-center">
                  {row.IsHighRiskFlag ? "🚨" : row.IsExtremePriceFlag ? "⚠️" : ""}
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
                <td colSpan={columnDefs.length + 1} className="px-3 py-8 text-center text-eve-dim">
                  {t("stationPrompt")}
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
    </div>
  );
}
