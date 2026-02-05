import { useCallback, useMemo, useRef, useState } from "react";
import { findRoutes } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import type { RouteResult, RouteHop, ScanParams } from "@/lib/types";
import { ExecutionPlannerPopup } from "./ExecutionPlannerPopup";
import {
  TabSettingsPanel,
  SettingsField,
  SettingsNumberInput,
  SettingsGrid,
} from "./TabSettingsPanel";

type SortKey = "hops" | "profit" | "jumps" | "ppj";
type SortDir = "asc" | "desc";

interface Props {
  params: ScanParams;
}

function formatISK(v: number): string {
  if (v >= 1e9) return (v / 1e9).toFixed(1) + "B";
  if (v >= 1e6) return (v / 1e6).toFixed(1) + "M";
  if (v >= 1e3) return (v / 1e3).toFixed(1) + "K";
  return v.toFixed(0);
}

function formatISKFull(v: number): string {
  return v.toLocaleString("en-US", { maximumFractionDigits: 0 });
}

export function RouteBuilder({ params }: Props) {
  const { t } = useI18n();
  const [minHops, setMinHops] = useState<number | "">(2);
  const [maxHops, setMaxHops] = useState<number | "">(5);
  const [results, setResults] = useState<RouteResult[]>([]);
  const [scanning, setScanning] = useState(false);
  const [progress, setProgress] = useState("");
  const [selectedRoute, setSelectedRoute] = useState<RouteResult | null>(null);
  const [sortKey, setSortKey] = useState<SortKey>("profit");
  const [sortDir, setSortDir] = useState<SortDir>("desc");
  const abortRef = useRef<AbortController | null>(null);

  const toggleSort = (key: SortKey) => {
    if (sortKey === key) {
      setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    } else {
      setSortKey(key);
      setSortDir("desc");
    }
  };

  const sortedResults = useMemo(() => {
    if (results.length === 0) return results;
    const getter: Record<SortKey, (r: RouteResult) => number> = {
      hops: (r) => r.HopCount,
      profit: (r) => r.TotalProfit,
      jumps: (r) => r.TotalJumps,
      ppj: (r) => r.ProfitPerJump,
    };
    const get = getter[sortKey];
    const mul = sortDir === "asc" ? 1 : -1;
    return [...results].sort((a, b) => (get(a) - get(b)) * mul);
  }, [results, sortKey, sortDir]);

  const handleSearch = useCallback(async () => {
    if (scanning) {
      abortRef.current?.abort();
      return;
    }
    const controller = new AbortController();
    abortRef.current = controller;
    setScanning(true);
    setProgress(t("scanStarting"));
    setResults([]);
    setSelectedRoute(null);

    try {
      const res = await findRoutes(params, minHops || 2, maxHops || 5, setProgress, controller.signal);
      setResults(res);
    } catch (e: unknown) {
      if (e instanceof Error && e.name !== "AbortError") {
        setProgress(t("errorPrefix") + e.message);
      }
    } finally {
      setScanning(false);
    }
  }, [scanning, params, minHops, maxHops, t]);

  const routeSummary = (route: RouteResult) =>
    route.Hops.map((h) => h.SystemName).concat([route.Hops[route.Hops.length - 1]?.DestSystemName ?? ""]).filter(Boolean).join(" → ");
  const copyRouteSystems = (route: RouteResult) => {
    navigator.clipboard.writeText(routeSummary(route));
  };

  return (
    <div className="flex flex-col h-full">
      {/* Settings Panel - unified design */}
      <div className="shrink-0 m-2">
        <TabSettingsPanel
          title={t("routeSettings")}
          hint={t("routeSettingsHint")}
          icon="🗺"
          defaultExpanded={true}
          help={{ stepKeys: ["helpRouteStep1", "helpRouteStep2", "helpRouteStep3"], wikiSlug: "Route-Builder" }}
        >
          <div className="flex items-center gap-4 flex-wrap">
            <SettingsGrid cols={2}>
              <SettingsField label={t("routeMinHops")}>
                <SettingsNumberInput
                  value={typeof minHops === "number" ? minHops : 2}
                  onChange={(v) => setMinHops(v)}
                  min={1}
                  max={10}
                />
              </SettingsField>
              <SettingsField label={t("routeMaxHops")}>
                <SettingsNumberInput
                  value={typeof maxHops === "number" ? maxHops : 5}
                  onChange={(v) => setMaxHops(v)}
                  min={typeof minHops === "number" ? minHops : 1}
                  max={10}
                />
              </SettingsField>
            </SettingsGrid>

            <div className="flex items-center gap-3 ml-auto">
              <button
                onClick={handleSearch}
                disabled={!params.system_name}
                className={`px-5 py-1.5 rounded-sm text-xs font-semibold uppercase tracking-wider transition-all
                  ${scanning
                    ? "bg-eve-error/80 text-white hover:bg-eve-error"
                    : "bg-eve-accent text-eve-dark hover:bg-eve-accent-hover shadow-eve-glow"
                  }
                  disabled:bg-eve-input disabled:text-eve-dim disabled:cursor-not-allowed disabled:shadow-none`}
              >
                {scanning ? t("stop") : t("routeFind")}
              </button>
              {progress && <span className="text-[10px] text-eve-dim">{progress}</span>}
            </div>
          </div>
          {results.length > 0 && (
            <div className="mt-2 text-xs text-eve-dim">
              {t("routeFound", { count: results.length })}
            </div>
          )}
        </TabSettingsPanel>
      </div>

      {/* Results table */}
      <div className="flex-1 min-h-0 overflow-auto">
        {results.length > 0 ? (
          <table className="w-full text-xs">
            <thead className="sticky top-0 bg-eve-panel z-10">
              <tr className="text-eve-dim text-[10px] uppercase tracking-wider border-b border-eve-border">
                <th className="px-3 py-2 text-left font-medium">#</th>
                <th className="px-3 py-2 text-left font-medium">{t("routeColumn")}</th>
                <SortTh k="hops" cur={sortKey} dir={sortDir} onClick={toggleSort} align="right" label={t("routeHopsCol")} />
                <SortTh k="profit" cur={sortKey} dir={sortDir} onClick={toggleSort} align="right" label={t("colProfit")} />
                <SortTh k="jumps" cur={sortKey} dir={sortDir} onClick={toggleSort} align="right" label={t("colJumps")} />
                <SortTh k="ppj" cur={sortKey} dir={sortDir} onClick={toggleSort} align="right" label={t("colProfitPerJump")} />
              </tr>
            </thead>
            <tbody>
              {sortedResults.map((route, i) => (
                <tr
                  key={i}
                  onDoubleClick={() => setSelectedRoute(route)}
                  className="cursor-pointer hover:bg-eve-accent/10 border-b border-eve-border/30 transition-colors"
                >
                  <td className="px-3 py-2 text-eve-dim font-mono">{i + 1}</td>
                  <td className="px-3 py-2 text-eve-text max-w-[400px] truncate" title={routeSummary(route)}>
                    {routeSummary(route)}
                  </td>
                  <td className="px-3 py-2 text-right font-mono text-eve-dim">{route.HopCount}</td>
                  <td className="px-3 py-2 text-right font-mono text-green-400">{formatISK(route.TotalProfit)}</td>
                  <td className="px-3 py-2 text-right font-mono text-eve-dim">{route.TotalJumps}</td>
                  <td className="px-3 py-2 text-right font-mono text-yellow-400">{formatISK(route.ProfitPerJump)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : !scanning ? (
          <div className="flex items-center justify-center h-full text-eve-dim text-xs">
            {t("routePrompt")}
          </div>
        ) : null}
      </div>

      {/* Detail popup */}
      {selectedRoute && (
          <RouteDetailPopup
          route={selectedRoute}
          onClose={() => setSelectedRoute(null)}
          onCopySystems={copyRouteSystems}
          salesTaxPercent={params.sales_tax_percent ?? 0}
        />
      )}
    </div>
  );
}

function SortTh({
  k,
  cur,
  dir,
  onClick,
  align,
  label,
}: {
  k: SortKey;
  cur: SortKey;
  dir: SortDir;
  onClick: (k: SortKey) => void;
  align: "left" | "right";
  label: string;
}) {
  const active = cur === k;
  return (
    <th
      className={`px-3 py-2 font-medium cursor-pointer select-none hover:text-eve-accent transition-colors ${
        align === "right" ? "text-right" : "text-left"
      } ${active ? "text-eve-accent" : ""}`}
      onClick={() => onClick(k)}
    >
      {label}
      {active && (
        <span className="ml-1 text-[9px]">{dir === "asc" ? "\u25B2" : "\u25BC"}</span>
      )}
    </th>
  );
}

function RouteDetailPopup({
  route,
  onClose,
  onCopySystems,
  salesTaxPercent = 0,
}: {
  route: RouteResult;
  onClose: () => void;
  onCopySystems: (route: RouteResult) => void;
  salesTaxPercent?: number;
}) {
  const { t } = useI18n();
  const [execPlanHop, setExecPlanHop] = useState<RouteHop | null>(null);

  return (
    <>
    <div
      className="fixed inset-0 bg-black/60 flex items-center justify-center z-50"
      onClick={onClose}
    >
      <div
        className="bg-eve-panel border border-eve-border rounded-sm max-w-2xl w-full mx-4 max-h-[80vh] flex flex-col shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-4 py-3 border-b border-eve-border">
          <h2 className="text-sm font-semibold text-eve-accent uppercase tracking-wider">
            {t("routeDetails")}
          </h2>
          <button
            onClick={onClose}
            className="text-eve-dim hover:text-eve-text text-lg leading-none"
          >
            ✕
          </button>
        </div>

        {/* Hops */}
        <div className="flex-1 overflow-y-auto p-4 space-y-0">
          {route.Hops.map((hop, i) => (
            <div key={i}>
              {/* Hop card */}
              <div className="bg-eve-dark/50 border border-eve-border/50 rounded-sm p-3">
                <div className="flex items-center gap-2 mb-2">
                  <span className="w-6 h-6 flex items-center justify-center rounded-full bg-eve-accent/20 text-eve-accent text-[11px] font-bold">
                    {i + 1}
                  </span>
                  <span className="text-xs font-medium text-eve-text">
                    {hop.StationName || hop.SystemName}
                  </span>
                  {hop.RegionID != null && hop.RegionID > 0 && (
                    <button
                      type="button"
                      onClick={() => setExecPlanHop(hop)}
                      className="ml-auto px-2 py-0.5 text-[10px] rounded-sm text-eve-dim hover:text-eve-accent border border-eve-border hover:border-eve-accent/30 transition-colors"
                      title={t("execPlanTitle")}
                    >
                      📊 {t("execPlanTitle")}
                    </button>
                  )}
                </div>

                <div className="ml-8 space-y-1 text-xs">
                  <div className="flex items-center gap-2">
                    <span className="text-eve-dim">{t("routeBuy")}:</span>
                    <span className="text-eve-text font-medium">{hop.TypeName}</span>
                    <span className="text-eve-dim">×{hop.Units}</span>
                    <span className="text-eve-dim">@</span>
                    <span className="font-mono text-eve-text">{formatISKFull(hop.BuyPrice)} ISK</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-eve-dim">→ {t("routeDeliverTo")}:</span>
                    <span className="text-eve-text">{hop.DestSystemName}</span>
                    <span className="text-eve-dim font-mono">({hop.Jumps} {t("routeJumpsUnit")})</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-eve-dim">{t("routeSell")}:</span>
                    <span className="font-mono text-eve-text">@ {formatISKFull(hop.SellPrice)} ISK</span>
                    <span className="text-eve-dim">→</span>
                    <span className="font-mono text-green-400">+{formatISKFull(hop.Profit)} ISK</span>
                  </div>
                </div>
              </div>

              {/* Connector */}
              {i < route.Hops.length - 1 && (
                <div className="flex justify-center py-1">
                  <div className="flex flex-col items-center">
                    <div className="w-px h-2 bg-eve-border" />
                    <svg width="10" height="6" viewBox="0 0 10 6" className="text-eve-accent">
                      <path d="M5 6L0 0h10z" fill="currentColor" />
                    </svg>
                    <div className="w-px h-2 bg-eve-border" />
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>

        {/* Copy route button + Summary footer */}
        <div className="flex items-center gap-6 px-4 py-3 border-t border-eve-border text-xs">
          <div>
            <span className="text-eve-dim">{t("routeTotalProfit")}: </span>
            <span className="font-mono text-green-400 font-semibold">{formatISKFull(route.TotalProfit)} ISK</span>
          </div>
          <div>
            <span className="text-eve-dim">{t("routeTotalJumps")}: </span>
            <span className="font-mono text-eve-text">{route.TotalJumps}</span>
          </div>
          <div>
            <span className="text-eve-dim">ISK/{t("routeJumpsUnit")}: </span>
            <span className="font-mono text-yellow-400">{formatISK(route.ProfitPerJump)}</span>
          </div>
          <div>
            <span className="text-eve-dim">{t("routeHopsCol")}: </span>
            <span className="font-mono text-eve-text">{route.HopCount}</span>
          </div>
          <div className="ml-auto flex items-center gap-2">
            <button
              onClick={() => onCopySystems(route)}
              className="px-3 py-1 rounded-sm text-xs font-medium text-eve-dim hover:text-eve-accent border border-eve-border hover:border-eve-accent/30 transition-colors cursor-pointer"
            >
              {t("copyRouteSystems")}
            </button>
            <button
              onClick={() => {
                const lines = ["=== EVE Flipper Route ==="];
                route.Hops.forEach((hop, i) => {
                  lines.push(`[${i + 1}] ${hop.StationName || hop.SystemName}`);
                  lines.push(`    Buy: ${hop.TypeName} x${hop.Units} @ ${formatISKFull(hop.BuyPrice)} ISK`);
                  lines.push(`    → ${hop.DestSystemName} (${hop.Jumps} jumps)`);
                  lines.push(`    Sell: @ ${formatISKFull(hop.SellPrice)} ISK → Profit: ${formatISK(hop.Profit)}`);
                  lines.push("");
                });
                lines.push(`Total: ${formatISKFull(route.TotalProfit)} ISK / ${route.TotalJumps} jumps / ${formatISK(route.ProfitPerJump)} ISK/jump`);
                navigator.clipboard.writeText(lines.join("\n"));
              }}
              className="px-3 py-1 rounded-sm text-xs font-medium text-eve-dim hover:text-eve-accent border border-eve-border hover:border-eve-accent/30 transition-colors cursor-pointer"
            >
              {t("copyRoute")}
            </button>
          </div>
        </div>
      </div>
    </div>

    <ExecutionPlannerPopup
      open={execPlanHop !== null}
      onClose={() => setExecPlanHop(null)}
      typeID={execPlanHop?.TypeID ?? 0}
      typeName={execPlanHop?.TypeName ?? ""}
      regionID={execPlanHop?.RegionID ?? 0}
      defaultQuantity={execPlanHop?.Units ?? 100}
      isBuy={true}
      salesTaxPercent={salesTaxPercent}
    />
    </>
  );
}
