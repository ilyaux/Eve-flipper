import type { Dispatch, SetStateAction } from "react";
import { useI18n } from "@/lib/i18n";
import { formatISK } from "@/lib/format";
import { useGlobalToast } from "../Toast";
import type { FlatMaterial, IndustryActivityStep, IndustryAnalysis, IndustryCoverageResult } from "@/lib/types";
import { formatDuration } from "./industryHelpers";
import { IndustryMaterialTree } from "./IndustryMaterialTree";
import { IndustryShoppingList } from "./IndustryShoppingList";
import { IndustrySummaryCard } from "./IndustrySummaryCard";

interface IndustryAnalysisResultsPanelProps {
  result: IndustryAnalysis;
  viewMode: "tree" | "shopping";
  setViewMode: Dispatch<SetStateAction<"tree" | "shopping">>;
  salesTaxPercent: number;
  brokerFee: number;
  onOpenExecutionPlan: (material: FlatMaterial) => void;
  isLoggedIn?: boolean;
  coverage?: IndustryCoverageResult | null;
  coverageLoading?: boolean;
  coverageMeta?: string;
  coverageScope?: "single" | "all";
  onCoverageScopeChange?: (scope: "single" | "all") => void;
  coverageUseSelectedStation?: boolean;
  onCoverageUseSelectedStationChange?: (enabled: boolean) => void;
  coverageStationLabel?: string;
  coverageDefaultBPCRuns?: number;
  onCoverageDefaultBPCRunsChange?: (runs: number) => void;
  onRefreshCoverage?: () => void;
  onSeedLedgerDraft?: () => void;
}

export function IndustryAnalysisResultsPanel({
  result,
  viewMode,
  setViewMode,
  salesTaxPercent,
  brokerFee,
  onOpenExecutionPlan,
  isLoggedIn = false,
  coverage,
  coverageLoading = false,
  coverageMeta = "",
  coverageScope = "single",
  onCoverageScopeChange,
  coverageUseSelectedStation = false,
  onCoverageUseSelectedStationChange,
  coverageStationLabel = "",
  coverageDefaultBPCRuns = 1,
  onCoverageDefaultBPCRunsChange,
  onRefreshCoverage,
  onSeedLedgerDraft,
}: IndustryAnalysisResultsPanelProps) {
  const { t } = useI18n();
  const { addToast } = useGlobalToast();

  return (
    <div className="flex-1 min-h-0 m-2 mt-0 flex flex-col">
      <div className="shrink-0 grid grid-cols-2 md:grid-cols-4 gap-2 mb-2">
        <IndustrySummaryCard
          label={t("industryMarketPrice")}
          value={formatISK(result.market_buy_price ?? 0)}
          subtext={`${(result.total_quantity ?? 0).toLocaleString()} ${t("industryUnits")}`}
          color="text-eve-dim"
        />
        <IndustrySummaryCard
          label={t("industryBuildCost")}
          value={formatISK(result.optimal_build_cost ?? 0)}
          subtext={result.blueprint_cost_included > 0
            ? `${t("industryJobCost")}: ${formatISK(result.total_job_cost ?? 0)} · ${t("industryBPCostIncluded")}: ${formatISK(result.blueprint_cost_included)}`
            : `${t("industryJobCost")}: ${formatISK(result.total_job_cost ?? 0)}`}
          color="text-eve-accent"
        />
        <IndustrySummaryCard
          label={t("industrySavings")}
          value={formatISK(result.savings ?? 0)}
          subtext={`${(result.savings_percent ?? 0).toFixed(1)}%`}
          color={(result.savings ?? 0) > 0 ? "text-green-400" : "text-red-400"}
        />
        <IndustrySummaryCard
          label={t("industryProfit")}
          value={formatISK(result.profit ?? 0)}
          subtext={`${(result.profit_percent ?? 0).toFixed(1)}% ROI`}
          color={(result.profit ?? 0) > 0 ? "text-green-400" : "text-red-400"}
        />
      </div>

      <div className="shrink-0 grid grid-cols-2 md:grid-cols-4 gap-2 mb-2">
        <IndustrySummaryCard
          label={t("industryISKPerHour")}
          value={formatISK(result.isk_per_hour ?? 0)}
          color={(result.isk_per_hour ?? 0) > 0 ? "text-yellow-400" : "text-red-400"}
        />
        <IndustrySummaryCard
          label={t("industryMfgTime")}
          value={formatDuration(result.manufacturing_time ?? 0)}
          color="text-eve-dim"
        />
        <IndustrySummaryCard
          label={t("industrySellRevenue")}
          value={formatISK(result.sell_revenue ?? 0)}
          subtext={`-${salesTaxPercent}% tax -${brokerFee}% broker`}
          color="text-eve-dim"
        />
        <IndustrySummaryCard
          label={t("industryJobCost")}
          value={formatISK(result.total_job_cost ?? 0)}
          subtext={`SCI: ${((result.system_cost_index ?? 0) * 100).toFixed(2)}%`}
          color="text-eve-dim"
        />
      </div>

      {result.activity_plan && result.activity_plan.length > 0 && (
        <IndustryActivityPlan
          steps={result.activity_plan}
          inventionCost={result.invention_cost ?? 0}
          inventionAttempts={result.invention_attempts ?? 0}
          inventionProbability={result.invention_probability ?? 0}
        />
      )}

      <IndustryCoveragePanel
        coverage={coverage}
        loading={coverageLoading}
        isLoggedIn={isLoggedIn}
        meta={coverageMeta}
        scope={coverageScope}
        onScopeChange={onCoverageScopeChange}
        useSelectedStation={coverageUseSelectedStation}
        onUseSelectedStationChange={onCoverageUseSelectedStationChange}
        stationLabel={coverageStationLabel}
        defaultBPCRuns={coverageDefaultBPCRuns}
        onDefaultBPCRunsChange={onCoverageDefaultBPCRunsChange}
        onRefresh={onRefreshCoverage}
        onSeedLedgerDraft={onSeedLedgerDraft}
      />

      <div className="shrink-0 flex items-center gap-2 mb-2 flex-wrap">
        <button
          onClick={() => setViewMode("tree")}
          className={`px-3 py-1 text-xs rounded-sm transition-colors ${
            viewMode === "tree"
              ? "bg-eve-accent/20 text-eve-accent border border-eve-accent/30"
              : "text-eve-dim hover:text-eve-text border border-eve-border"
          }`}
        >
          {t("industryTreeView")}
        </button>
        <button
          onClick={() => setViewMode("shopping")}
          className={`px-3 py-1 text-xs rounded-sm transition-colors ${
            viewMode === "shopping"
              ? "bg-eve-accent/20 text-eve-accent border border-eve-accent/30"
              : "text-eve-dim hover:text-eve-text border border-eve-border"
          }`}
        >
          {t("industryShoppingList")}
        </button>
        {viewMode === "shopping" && result.flat_materials.length > 0 && (
          <>
            <button
              onClick={() => {
                const header = "Item\tQuantity\tUnit Price\tTotal\tVolume (m³)";
                const rows = result.flat_materials.map(
                  (m) => `${m.type_name}\t${m.quantity}\t${m.unit_price}\t${m.total_price}\t${m.volume}`
                );
                navigator.clipboard.writeText([header, ...rows].join("\n"));
                addToast(t("copied"), "success", 2000);
              }}
              className="px-3 py-1 text-xs rounded-sm text-eve-dim hover:text-eve-accent border border-eve-border hover:border-eve-accent/30 transition-colors"
            >
              {t("industryExportClipboard")}
            </button>
            <button
              onClick={() => {
                const header = "Item,Quantity,Unit Price,Total,Volume (m³)";
                const rows = result.flat_materials.map(
                  (m) => `"${(m.type_name || "").replace(/"/g, "\"\"")}",${m.quantity},${m.unit_price},${m.total_price},${m.volume}`
                );
                const csv = "\uFEFF" + [header, ...rows].join("\n");
                const blob = new Blob([csv], { type: "text/csv;charset=utf-8" });
                const url = URL.createObjectURL(blob);
                const a = document.createElement("a");
                a.href = url;
                a.download = `industry-shopping-list-${new Date().toISOString().slice(0, 10)}.csv`;
                a.click();
                URL.revokeObjectURL(url);
                addToast(t("industryExportCSV"), "success", 2000);
              }}
              className="px-3 py-1 text-xs rounded-sm text-eve-dim hover:text-eve-accent border border-eve-border hover:border-eve-accent/30 transition-colors"
            >
              {t("industryExportCSV")}
            </button>
          </>
        )}
      </div>

      <div className="flex-1 min-h-0 overflow-auto border border-eve-border rounded-sm bg-eve-panel">
        {viewMode === "tree" ? (
          <IndustryMaterialTree node={result.material_tree} />
        ) : (
          <IndustryShoppingList
            materials={result.flat_materials}
            regionId={result.region_id ?? 0}
            onOpenExecutionPlan={onOpenExecutionPlan}
          />
        )}
      </div>
    </div>
  );
}

function IndustryCoveragePanel({
  coverage,
  loading,
  isLoggedIn,
  meta,
  scope,
  onScopeChange,
  useSelectedStation,
  onUseSelectedStationChange,
  stationLabel,
  defaultBPCRuns,
  onDefaultBPCRunsChange,
  onRefresh,
  onSeedLedgerDraft,
}: {
  coverage?: IndustryCoverageResult | null;
  loading: boolean;
  isLoggedIn: boolean;
  meta: string;
  scope: "single" | "all";
  onScopeChange?: (scope: "single" | "all") => void;
  useSelectedStation: boolean;
  onUseSelectedStationChange?: (enabled: boolean) => void;
  stationLabel: string;
  defaultBPCRuns: number;
  onDefaultBPCRunsChange?: (runs: number) => void;
  onRefresh?: () => void;
  onSeedLedgerDraft?: () => void;
}) {
  const summary = coverage?.summary;
  const materialRows = coverage
    ? [...coverage.materials]
        .sort((a, b) => (b.missing_qty - a.missing_qty) || (b.required_qty - a.required_qty))
        .slice(0, 5)
    : [];
  const blueprintRows = coverage
    ? [...coverage.blueprints]
        .sort((a, b) => Number(a.status === "ready") - Number(b.status === "ready") || (b.required_runs - a.required_runs))
        .slice(0, 5)
    : [];
  const actionRows = coverage ? (coverage.actions ?? []).slice(0, 8) : [];

  return (
    <div className="shrink-0 border border-eve-border bg-eve-panel rounded-sm mb-2 overflow-hidden">
      <div className="px-3 py-2 flex items-center justify-between gap-3 border-b border-eve-border/60 flex-wrap">
        <div className="min-w-0">
          <div className="text-[10px] uppercase tracking-wider text-eve-dim">Character coverage</div>
          <div className="text-[9px] text-eve-dim truncate">
            {meta || (isLoggedIn ? "Checks owned assets and blueprints against this analysis" : "Login required for owned assets and blueprints")}
          </div>
        </div>
        <div className="flex items-center gap-2 flex-wrap">
          <select
            value={scope}
            onChange={(event) => onScopeChange?.(event.target.value as "single" | "all")}
            disabled={!isLoggedIn || loading || !onScopeChange}
            className="h-7 bg-eve-input border border-eve-border rounded-sm px-2 text-[10px] text-eve-text disabled:text-eve-dim"
          >
            <option value="single">Single char</option>
            <option value="all">All chars</option>
          </select>
          <label className="h-7 px-2 border border-eve-border rounded-sm flex items-center gap-1.5 text-[10px] text-eve-dim">
            <input
              type="checkbox"
              checked={useSelectedStation}
              disabled={!isLoggedIn || loading || !stationLabel || !onUseSelectedStationChange}
              onChange={(event) => onUseSelectedStationChange?.(event.target.checked)}
              className="accent-eve-accent"
            />
            <span className="truncate max-w-[160px]">{stationLabel ? "Selected station" : "No station"}</span>
          </label>
          <label className="h-7 px-2 border border-eve-border rounded-sm flex items-center gap-1.5 text-[10px] text-eve-dim">
            <span>BPC runs</span>
            <input
              type="number"
              min={1}
              max={1000}
              value={defaultBPCRuns}
              disabled={!isLoggedIn || loading || !onDefaultBPCRunsChange}
              onChange={(event) => onDefaultBPCRunsChange?.(Math.max(1, Math.min(1000, Math.round(Number(event.target.value) || 1))))}
              className="w-14 bg-transparent text-eve-text font-mono outline-none disabled:text-eve-dim"
            />
          </label>
          <button
            type="button"
            onClick={onRefresh}
            disabled={!isLoggedIn || loading || !onRefresh}
            className="h-7 shrink-0 px-3 text-[10px] font-semibold uppercase tracking-wider rounded-sm bg-eve-accent text-eve-dark hover:bg-eve-accent-hover disabled:bg-eve-input disabled:text-eve-dim disabled:cursor-not-allowed"
          >
            {loading ? "Checking" : "Check assets+BPs"}
          </button>
          <button
            type="button"
            onClick={onSeedLedgerDraft}
            disabled={!isLoggedIn || loading || !onSeedLedgerDraft}
            className="h-7 shrink-0 px-3 text-[10px] font-semibold uppercase tracking-wider rounded-sm border border-cyan-500/40 text-cyan-300 hover:bg-cyan-500/10 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Seed draft
          </button>
        </div>
      </div>

      {!coverage ? (
        <div className="px-3 py-2 text-[10px] text-eve-dim">
          No character coverage loaded for this analysis.
        </div>
      ) : (
        <>
          <div className="grid grid-cols-2 md:grid-cols-4 border-b border-eve-border/60">
            <CoverageMetric
              label="Can start"
              value={summary?.can_start_now ? "YES" : "NO"}
              valueClass={summary?.can_start_now ? "text-green-400" : "text-red-300"}
            />
            <CoverageMetric
              label="Materials"
              value={`${summary?.materials_covered ?? 0}/${summary?.materials ?? 0}`}
              sub={`${(summary?.material_coverage_pct ?? 0).toFixed(1)}%`}
              valueClass={(summary?.materials_missing ?? 0) === 0 ? "text-green-400" : "text-yellow-300"}
            />
            <CoverageMetric
              label="Missing units"
              value={formatQty(summary?.missing_units ?? 0)}
              valueClass={(summary?.missing_units ?? 0) === 0 ? "text-green-400" : "text-red-300"}
            />
            <CoverageMetric
              label="Blueprints"
              value={`${summary?.blueprints_ready ?? 0}/${summary?.blueprints ?? 0}`}
              valueClass={(summary?.blueprints_missing ?? 0) === 0 ? "text-green-400" : "text-yellow-300"}
            />
          </div>

          {(coverage.warnings ?? []).length > 0 && (
            <div className="px-3 py-1.5 border-b border-eve-border/60 text-[10px] text-yellow-300 truncate">
              {(coverage.warnings ?? []).slice(0, 2).join(" | ")}
            </div>
          )}

          {actionRows.length > 0 && (
            <div className="border-b border-eve-border/60 overflow-x-auto">
              <table className="w-full text-[10px] min-w-[720px]">
                <thead className="bg-eve-dark/40 text-eve-dim">
                  <tr>
                    <th className="px-3 py-1.5 text-right font-normal w-12">#</th>
                    <th className="px-3 py-1.5 text-left font-normal">Action</th>
                    <th className="px-3 py-1.5 text-left font-normal">Target</th>
                    <th className="px-3 py-1.5 text-right font-normal">Qty</th>
                    <th className="px-3 py-1.5 text-right font-normal">Status</th>
                  </tr>
                </thead>
                <tbody>
                  {actionRows.map((action) => (
                    <tr key={`${action.step}-${action.action}-${action.type_id ?? 0}`} className="border-t border-eve-border/40">
                      <td className="px-3 py-1.5 text-right text-eve-dim font-mono">{action.step}</td>
                      <td className="px-3 py-1.5 text-eve-text">{action.label || action.action}</td>
                      <td className="px-3 py-1.5 text-eve-dim truncate max-w-[320px]">{action.detail || action.type_name || "-"}</td>
                      <td className="px-3 py-1.5 text-right text-eve-dim">{formatQty(action.quantity ?? action.missing_qty ?? 0)}</td>
                      <td className="px-3 py-1.5 text-right">
                        <span className={`px-1.5 py-0.5 rounded-sm border uppercase ${coverageStatusClass(action.status)}`}>
                          {action.status}
                        </span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          <div className="grid grid-cols-1 xl:grid-cols-2">
            <div className="overflow-x-auto border-b xl:border-b-0 xl:border-r border-eve-border/60">
              <table className="w-full text-[10px] min-w-[520px]">
                <thead className="bg-eve-dark/40 text-eve-dim">
                  <tr>
                    <th className="px-3 py-1.5 text-left font-normal">Material</th>
                    <th className="px-3 py-1.5 text-right font-normal">Need</th>
                    <th className="px-3 py-1.5 text-right font-normal">Owned</th>
                    <th className="px-3 py-1.5 text-right font-normal">Missing</th>
                    <th className="px-3 py-1.5 text-right font-normal">Status</th>
                  </tr>
                </thead>
                <tbody>
                  {materialRows.length === 0 ? (
                    <tr>
                      <td colSpan={5} className="px-3 py-2 text-eve-dim">No material rows.</td>
                    </tr>
                  ) : materialRows.map((row) => (
                    <tr key={row.type_id} className="border-t border-eve-border/40">
                      <td className="px-3 py-1.5 text-eve-text truncate max-w-[220px]">{row.type_name || `#${row.type_id}`}</td>
                      <td className="px-3 py-1.5 text-right text-eve-dim">{formatQty(row.required_qty)}</td>
                      <td className="px-3 py-1.5 text-right text-eve-dim">{formatQty(row.available_qty)}</td>
                      <td className={`px-3 py-1.5 text-right ${row.missing_qty > 0 ? "text-red-300" : "text-green-400"}`}>{formatQty(row.missing_qty)}</td>
                      <td className="px-3 py-1.5 text-right">
                        <span className={`px-1.5 py-0.5 rounded-sm border uppercase ${coverageStatusClass(row.status)}`}>
                          {row.status}
                        </span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            <div className="overflow-x-auto">
              <table className="w-full text-[10px] min-w-[560px]">
                <thead className="bg-eve-dark/40 text-eve-dim">
                  <tr>
                    <th className="px-3 py-1.5 text-left font-normal">Blueprint</th>
                    <th className="px-3 py-1.5 text-right font-normal">Runs</th>
                    <th className="px-3 py-1.5 text-right font-normal">BPO/BPC</th>
                    <th className="px-3 py-1.5 text-right font-normal">BPC runs</th>
                    <th className="px-3 py-1.5 text-right font-normal">ME/TE</th>
                    <th className="px-3 py-1.5 text-right font-normal">Status</th>
                  </tr>
                </thead>
                <tbody>
                  {blueprintRows.length === 0 ? (
                    <tr>
                      <td colSpan={6} className="px-3 py-2 text-eve-dim">No blueprint rows.</td>
                    </tr>
                  ) : blueprintRows.map((row) => (
                    <tr key={row.blueprint_type_id} className="border-t border-eve-border/40">
                      <td className="px-3 py-1.5 text-eve-text truncate max-w-[220px]">
                        <div className="truncate">{row.blueprint_name || `#${row.blueprint_type_id}`}</div>
                        <div className="text-[9px] text-eve-dim truncate">{row.activity || "activity"}</div>
                      </td>
                      <td className="px-3 py-1.5 text-right text-eve-dim">{formatQty(row.required_runs)}</td>
                      <td className="px-3 py-1.5 text-right text-eve-dim">{formatQty(row.bpo_qty)}/{formatQty(row.bpc_qty)}</td>
                      <td className="px-3 py-1.5 text-right text-eve-dim">{row.bpo_qty > 0 ? "BPO" : formatQty(row.available_runs)}</td>
                      <td className="px-3 py-1.5 text-right text-eve-dim">{row.best_me}/{row.best_te}</td>
                      <td className="px-3 py-1.5 text-right">
                        <span className={`px-1.5 py-0.5 rounded-sm border uppercase ${coverageStatusClass(row.status)}`}>
                          {row.status}
                        </span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </>
      )}
    </div>
  );
}

function CoverageMetric({
  label,
  value,
  sub,
  valueClass,
}: {
  label: string;
  value: string;
  sub?: string;
  valueClass?: string;
}) {
  return (
    <div className="px-3 py-2 border-r border-eve-border/40 last:border-r-0">
      <div className="text-[9px] uppercase tracking-wider text-eve-dim">{label}</div>
      <div className={`text-xs font-mono ${valueClass ?? "text-eve-text"}`}>{value}</div>
      {sub && <div className="text-[9px] text-eve-dim">{sub}</div>}
    </div>
  );
}

function formatQty(value: number) {
  if (!Number.isFinite(value)) return "0";
  return Math.round(value).toLocaleString();
}

function coverageStatusClass(status: string) {
  if (status === "covered" || status === "ready") return "border-green-400/25 bg-green-500/10 text-green-300";
  if (status === "partial" || status === "needed") return "border-yellow-300/25 bg-yellow-400/10 text-yellow-200";
  return "border-red-300/25 bg-red-500/10 text-red-200";
}

function IndustryActivityPlan({
  steps,
  inventionCost,
  inventionAttempts,
  inventionProbability,
}: {
  steps: IndustryActivityStep[];
  inventionCost: number;
  inventionAttempts: number;
  inventionProbability: number;
}) {
  const rows = steps.slice(0, 8);
  const totalCost = steps.reduce((sum, step) => sum + (step.total_cost || 0), 0);
  const totalTime = steps.reduce((sum, step) => sum + (step.time_seconds || 0), 0);

  return (
    <div className="shrink-0 border border-eve-border bg-eve-panel rounded-sm mb-2 overflow-hidden">
      <div className="px-3 py-2 flex items-center justify-between border-b border-eve-border/60">
        <div>
          <div className="text-[10px] uppercase tracking-wider text-eve-dim">Activity plan</div>
          <div className="text-[9px] text-eve-dim">
            {rows.length} steps · {formatISK(totalCost)} · {formatDuration(totalTime)}
          </div>
        </div>
        {inventionCost > 0 && (
          <div className="text-right text-[10px]">
            <div className="text-eve-accent">{formatISK(inventionCost)} invention</div>
            <div className="text-eve-dim">
              {inventionAttempts.toFixed(2)} attempts @ {(inventionProbability * 100).toFixed(1)}%
            </div>
          </div>
        )}
      </div>
      <div className="overflow-x-auto">
        <table className="w-full text-[11px] min-w-[760px]">
          <thead className="text-eve-dim bg-eve-dark/40">
            <tr>
              <th className="px-3 py-1.5 text-left font-normal">Activity</th>
              <th className="px-3 py-1.5 text-left font-normal">Output</th>
              <th className="px-3 py-1.5 text-right font-normal">Runs</th>
              <th className="px-3 py-1.5 text-right font-normal">Materials</th>
              <th className="px-3 py-1.5 text-right font-normal">Job</th>
              <th className="px-3 py-1.5 text-right font-normal">Total</th>
              <th className="px-3 py-1.5 text-right font-normal">Time</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((step, index) => (
              <tr key={`${step.activity}-${step.blueprint_type_id}-${index}`} className="border-t border-eve-border/40">
                <td className="px-3 py-1.5">
                  <span className={`px-2 py-0.5 rounded-sm border text-[10px] uppercase ${activityClass(step.activity)}`}>
                    {activityLabel(step.activity)}
                  </span>
                </td>
                <td className="px-3 py-1.5 text-eve-text">
                  <div className="truncate max-w-[240px]">{step.product_name || `#${step.product_type_id}`}</div>
                  <div className="text-[9px] text-eve-dim truncate max-w-[240px]">{step.blueprint_name || `BP #${step.blueprint_type_id}`}</div>
                </td>
                <td className="px-3 py-1.5 text-right text-eve-dim">
                  {step.activity === "invention" && step.expected_attempts ? step.expected_attempts.toFixed(2) : step.runs.toLocaleString()}
                </td>
                <td className="px-3 py-1.5 text-right text-eve-dim">{formatISK(step.material_cost || 0)}</td>
                <td className="px-3 py-1.5 text-right text-eve-dim">{formatISK(step.job_cost || 0)}</td>
                <td className="px-3 py-1.5 text-right text-eve-accent">{formatISK(step.total_cost || 0)}</td>
                <td className="px-3 py-1.5 text-right text-eve-dim">{formatDuration(step.time_seconds || 0)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function activityLabel(activity: string) {
  if (activity === "manufacturing") return "manufacturing";
  if (activity === "reaction") return "reaction";
  if (activity === "invention") return "invention";
  return activity || "activity";
}

function activityClass(activity: string) {
  if (activity === "reaction") return "border-sky-500/25 bg-sky-500/10 text-sky-300";
  if (activity === "invention") return "border-purple-400/25 bg-purple-400/10 text-purple-200";
  return "border-eve-accent/25 bg-eve-accent/10 text-eve-accent";
}
