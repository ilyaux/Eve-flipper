import { useEffect, useRef } from "react";
import {
  createChart,
  ColorType,
  LineStyle,
  CrosshairMode,
  LineSeries,
  HistogramSeries,
} from "lightweight-charts";
import type { IChartApi, ISeriesApi, LineData, Time } from "lightweight-charts";
import { useI18n } from "../../lib/i18n";
import type { ArbHistoryData, ChartOverlays, PricePoint } from "../../lib/types";
export function ArbHistoryChart({ data, themeKey }: { data: ArbHistoryData; themeKey?: string }) {
  const { t } = useI18n();
  const containerRef = useRef<HTMLDivElement>(null);
  const chartRef = useRef<IChartApi | null>(null);

  useEffect(() => {
    if (!containerRef.current) return;

    const hasData = (data.extractor_nes?.length ?? 0) > 0 ||
                    (data.sp_chain_nes?.length ?? 0) > 0 ||
                    (data.mptc_nes?.length ?? 0) > 0 ||
                    (data.sp_farm_profit?.length ?? 0) > 0;
    if (!hasData) return;

    const bgColor = cssColor("--eve-dark", "#0d1117");
    const txtColor = cssColor("--eve-dim", "#484f58");
    const grdColor = cssColor("--eve-border", "#21262d");

    const chart = createChart(containerRef.current, {
      layout: {
        background: { type: ColorType.Solid, color: bgColor },
        textColor: txtColor,
        fontFamily: "ui-monospace, SFMono-Regular, 'SF Mono', Menlo, monospace",
        fontSize: 10,
      },
      grid: { vertLines: { color: grdColor }, horzLines: { color: grdColor } },
      crosshair: { mode: CrosshairMode.Normal },
      rightPriceScale: { borderColor: grdColor, scaleMargins: { top: 0.1, bottom: 0.1 } },
      timeScale: { borderColor: grdColor, timeVisible: false, fixLeftEdge: true, fixRightEdge: true },
      handleScale: { axisPressedMouseMove: { time: true, price: true } },
      handleScroll: { mouseWheel: true, pressedMouseMove: true },
    });
    chartRef.current = chart;

    // Add zero line for reference
    const toLD = (pts: { date: string; profit_isk: number }[] | undefined): LineData<Time>[] =>
      pts?.map((p) => ({ time: p.date as Time, value: p.profit_isk })) ?? [];

    // NES Extractor — cyan
    if (data.extractor_nes?.length) {
      const s = chart.addSeries(LineSeries, { color: "#56d4dd", lineWidth: 1, priceLineVisible: false, lastValueVisible: false, crosshairMarkerVisible: true });
      s.setData(toLD(data.extractor_nes));
    }

    // SP Chain — purple
    if (data.sp_chain_nes?.length) {
      const s = chart.addSeries(LineSeries, { color: "#bc8cff", lineWidth: 1, priceLineVisible: false, lastValueVisible: false, crosshairMarkerVisible: true });
      s.setData(toLD(data.sp_chain_nes));
    }

    // MPTC — orange
    if (data.mptc_nes?.length) {
      const s = chart.addSeries(LineSeries, { color: "#d29922", lineWidth: 1, priceLineVisible: false, lastValueVisible: false, crosshairMarkerVisible: true });
      s.setData(toLD(data.mptc_nes));
    }

    // SP Farm monthly — green (thicker, most important)
    if (data.sp_farm_profit?.length) {
      const s = chart.addSeries(LineSeries, { color: "#3fb950", lineWidth: 2, priceLineVisible: true, lastValueVisible: true, crosshairMarkerVisible: true });
      s.setData(toLD(data.sp_farm_profit));
    }

    chart.timeScale().fitContent();

    // Resize observer
    const ro = new ResizeObserver(() => {
      if (containerRef.current && chartRef.current) {
        const { width, height } = containerRef.current.getBoundingClientRect();
        chartRef.current.resize(width, height);
      }
    });
    if (containerRef.current) ro.observe(containerRef.current);

    return () => { ro.disconnect(); chart.remove(); chartRef.current = null; };
  }, [data, themeKey]);

  return (
    <div className="bg-eve-dark border border-eve-border rounded-sm p-3">
      <h3 className="text-xs font-semibold text-eve-dim uppercase tracking-wider mb-1">{t("plexArbHistory")}</h3>
      <p className="text-[10px] text-eve-dim mb-2">{t("plexArbHistoryHint")}</p>
      {/* Legend */}
      <div className="flex items-center gap-3 mb-2 flex-wrap">
        <LegendDot color="#56d4dd" label={t("plexArbHistNES")} />
        <LegendDot color="#bc8cff" label={t("plexArbHistSP")} />
        <LegendDot color="#d29922" label={t("plexArbHistMPTC")} />
        <LegendDot color="#3fb950" label={t("plexArbHistSPFarm")} />
      </div>
      <div ref={containerRef} className="w-full rounded-sm h-[150px] sm:h-[180px] lg:h-[200px]" />
    </div>
  );
}

function LegendDot({ color, label }: { color: string; label: string }) {
  return (
    <div className="flex items-center gap-1">
      <span className="w-2 h-2 rounded-full shrink-0" style={{ backgroundColor: color }} />
      <span className="text-[10px] text-eve-dim">{label}</span>
    </div>
  );
}

function cssColor(name: string, fallback: string): string {
  const val = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  if (!val) return fallback;
  const parts = val.split(/\s+/).map(Number);
  if (parts.length === 3 && parts.every(n => !isNaN(n))) {
    return `#${parts.map(n => n.toString(16).padStart(2, "0")).join("")}`;
  }
  return fallback;
}

/** Convert backend overlay points to lightweight-charts LineData format */
function toLineData(points: { date: string; value: number }[] | undefined): LineData<Time>[] {
  if (!points?.length) return [];
  return points.map((p) => ({ time: p.date as Time, value: p.value }));
}

export function PLEXChart({ history, overlays, themeKey }: { history: PricePoint[]; overlays?: ChartOverlays | null; themeKey?: string }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const chartRef = useRef<IChartApi | null>(null);
  const seriesRef = useRef<{
    price?: ISeriesApi<"Line">;
    sma7?: ISeriesApi<"Line">;
    sma30?: ISeriesApi<"Line">;
    bbUpper?: ISeriesApi<"Line">;
    bbLower?: ISeriesApi<"Line">;
    volume?: ISeriesApi<"Histogram">;
  }>({});

  useEffect(() => {
    if (!containerRef.current || history.length === 0) return;

    // Read theme colors from CSS variables
    const bgColor = cssColor("--eve-dark", "#0d1117");
    const textColor = cssColor("--eve-dim", "#484f58");
    const gridColor = cssColor("--eve-border", "#21262d");
    const accentColor = cssColor("--eve-accent", "#e69500");

    // Create chart
    const chart = createChart(containerRef.current, {
      layout: {
        background: { type: ColorType.Solid, color: bgColor },
        textColor,
        fontFamily: "ui-monospace, SFMono-Regular, 'SF Mono', Menlo, monospace",
        fontSize: 10,
      },
      grid: {
        vertLines: { color: gridColor },
        horzLines: { color: gridColor },
      },
      crosshair: {
        mode: CrosshairMode.Normal,
        vertLine: { color: accentColor + "40", width: 1, style: LineStyle.Dashed, labelBackgroundColor: accentColor },
        horzLine: { color: accentColor + "40", width: 1, style: LineStyle.Dashed, labelBackgroundColor: accentColor },
      },
      rightPriceScale: {
        borderColor: gridColor,
        scaleMargins: { top: 0.1, bottom: 0.2 },
      },
      timeScale: {
        borderColor: gridColor,
        timeVisible: false,
        fixLeftEdge: true,
        fixRightEdge: true,
      },
      handleScale: { axisPressedMouseMove: { time: true, price: true } },
      handleScroll: { mouseWheel: true, pressedMouseMove: true },
    });
    chartRef.current = chart;

    // Bollinger Bands from backend (draw first so they appear behind)
    const bbUpperData = toLineData(overlays?.bollinger_upper);
    const bbLowerData = toLineData(overlays?.bollinger_lower);
    if (bbUpperData.length > 0) {
      const bbUpperSeries = chart.addSeries(LineSeries, {
        color: accentColor + "40",
        lineWidth: 1,
        lineStyle: LineStyle.Dashed,
        priceLineVisible: false,
        lastValueVisible: false,
        crosshairMarkerVisible: false,
      });
      bbUpperSeries.setData(bbUpperData);
      seriesRef.current.bbUpper = bbUpperSeries;
    }
    if (bbLowerData.length > 0) {
      const bbLowerSeries = chart.addSeries(LineSeries, {
        color: accentColor + "40",
        lineWidth: 1,
        lineStyle: LineStyle.Dashed,
        priceLineVisible: false,
        lastValueVisible: false,
        crosshairMarkerVisible: false,
      });
      bbLowerSeries.setData(bbLowerData);
      seriesRef.current.bbLower = bbLowerSeries;
    }

    // SMA(30) from backend
    const successColor = cssColor("--eve-success", "#3fb950");
    const warningColor = cssColor("--eve-warning", "#d29922");
    const errorColor = cssColor("--eve-error", "#dc3c3c");

    const sma30Data = toLineData(overlays?.sma30);
    if (sma30Data.length > 0) {
      const sma30Series = chart.addSeries(LineSeries, {
        color: warningColor,
        lineWidth: 1,
        priceLineVisible: false,
        lastValueVisible: false,
        crosshairMarkerVisible: false,
      });
      sma30Series.setData(sma30Data);
      seriesRef.current.sma30 = sma30Series;
    }

    // SMA(7) from backend
    const sma7Data = toLineData(overlays?.sma7);
    if (sma7Data.length > 0) {
      const sma7Series = chart.addSeries(LineSeries, {
        color: successColor,
        lineWidth: 1,
        priceLineVisible: false,
        lastValueVisible: false,
        crosshairMarkerVisible: false,
      });
      sma7Series.setData(sma7Data);
      seriesRef.current.sma7 = sma7Series;
    }

    // Main price line — accent color
    const priceSeries = chart.addSeries(LineSeries, {
      color: accentColor,
      lineWidth: 2,
      priceLineVisible: true,
      lastValueVisible: true,
      crosshairMarkerVisible: true,
      crosshairMarkerRadius: 4,
    });
    priceSeries.setData(
      history.map((p) => ({ time: p.date as Time, value: p.average }))
    );
    seriesRef.current.price = priceSeries;

    // Volume histogram on a separate price scale
    const volumeSeries = chart.addSeries(HistogramSeries, {
      color: accentColor + "30",
      priceFormat: { type: "volume" },
      priceScaleId: "volume",
    });
    volumeSeries.priceScale().applyOptions({
      scaleMargins: { top: 0.85, bottom: 0 },
    });
    volumeSeries.setData(
      history.map((p, i) => ({
        time: p.date as Time,
        value: p.volume,
        color: i > 0 && p.average >= history[i - 1].average ? successColor + "40" : errorColor + "40",
      }))
    );
    seriesRef.current.volume = volumeSeries;

    // Fit content
    chart.timeScale().fitContent();

    // Handle container resize
    const ro = new ResizeObserver(() => {
      if (containerRef.current && chartRef.current) {
        const { width, height } = containerRef.current.getBoundingClientRect();
        chartRef.current.resize(width, height);
      }
    });
    if (containerRef.current) {
      ro.observe(containerRef.current);
    }

    return () => {
      ro.disconnect();
      chart.remove();
      chartRef.current = null;
      seriesRef.current = {};
    };
  }, [history, overlays, themeKey]);

  return (
    <div
      ref={containerRef}
      className="w-full rounded-sm h-[200px] sm:h-[250px] lg:h-[300px]"
    />
  );
}
