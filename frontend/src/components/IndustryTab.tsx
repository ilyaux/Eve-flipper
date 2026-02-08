import { useState, useCallback, useRef, useMemo, useEffect } from "react";
import { useI18n } from "@/lib/i18n";
import { analyzeIndustry, searchBuildableItems } from "@/lib/api";
import type { IndustryAnalysis, IndustryParams, MaterialNode, FlatMaterial, BuildableItem } from "@/lib/types";
import { formatISK } from "@/lib/format";
import {
  TabSettingsPanel,
  SettingsField,
  SettingsNumberInput,
  SettingsGrid,
} from "./TabSettingsPanel";
import { SystemAutocomplete } from "./SystemAutocomplete";
import { EmptyState } from "./EmptyState";
import { useGlobalToast } from "./Toast";
import { ExecutionPlannerPopup } from "./ExecutionPlannerPopup";

// Format duration in seconds to human-readable string
function formatDuration(seconds: number): string {
  if (seconds <= 0) return "—";
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;
  const parts: string[] = [];
  if (d > 0) parts.push(`${d}d`);
  if (h > 0) parts.push(`${h}h`);
  if (m > 0) parts.push(`${m}m`);
  if (parts.length === 0) parts.push(`${s}s`);
  return parts.join(" ");
}

// Highlight matching text in search results
function HighlightMatch({ text, query }: { text: string; query: string }) {
  if (!query.trim()) return <>{text}</>;
  
  const lowerText = text.toLowerCase();
  const lowerQuery = query.toLowerCase().trim();
  const index = lowerText.indexOf(lowerQuery);
  
  if (index === -1) return <>{text}</>;
  
  return (
    <>
      {text.slice(0, index)}
      <span className="text-eve-accent font-medium">{text.slice(index, index + query.length)}</span>
      {text.slice(index + query.length)}
    </>
  );
}

interface Props {
  onError?: (msg: string) => void;
  isLoggedIn?: boolean;
}

export function IndustryTab({ onError, isLoggedIn = false }: Props) {
  const { t } = useI18n();
  const { addToast } = useGlobalToast();

  // Search state
  const [searchQuery, setSearchQuery] = useState("");
  const [searchResults, setSearchResults] = useState<BuildableItem[]>([]);
  const [searching, setSearching] = useState(false);
  const [showDropdown, setShowDropdown] = useState(false);
  const [highlightedIndex, setHighlightedIndex] = useState(-1);
  const searchTimeoutRef = useRef<ReturnType<typeof setTimeout>>(undefined);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  // Selected item
  const [selectedItem, setSelectedItem] = useState<BuildableItem | null>(null);

  // Close dropdown when clicking outside
  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node) &&
          inputRef.current && !inputRef.current.contains(e.target as Node)) {
        setShowDropdown(false);
      }
    };
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  // Parameters
  const [runs, setRuns] = useState(1);
  const [me, setME] = useState(10);
  const [te, setTE] = useState(20);
  const [systemName, setSystemName] = useState("Jita");
  const [facilityTax, setFacilityTax] = useState(0);
  const [structureBonus, setStructureBonus] = useState(1);
  const [brokerFee, setBrokerFee] = useState(3);
  const [salesTaxPercent, setSalesTaxPercent] = useState(8);
  const [ownBlueprint, setOwnBlueprint] = useState(true);
  const [blueprintCost, setBlueprintCost] = useState(0);
  const [blueprintIsBPO, setBlueprintIsBPO] = useState(true);

  // Analysis state
  const [analyzing, setAnalyzing] = useState(false);
  const [progress, setProgress] = useState("");
  const [result, setResult] = useState<IndustryAnalysis | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  // View mode
  const [viewMode, setViewMode] = useState<"tree" | "shopping">("tree");

  // Execution plan popup (from shopping list)
  const [execPlanMaterial, setExecPlanMaterial] = useState<FlatMaterial | null>(null);

  // Search handler with debounce
  const handleSearch = useCallback((query: string) => {
    setSearchQuery(query);
    setHighlightedIndex(-1);
    // Clear previous selection when user types new query
    setSelectedItem(null);
    clearTimeout(searchTimeoutRef.current);

    if (!query.trim()) {
      setSearchResults([]);
      setShowDropdown(false);
      return;
    }

    searchTimeoutRef.current = setTimeout(async () => {
      setSearching(true);
      try {
        const results = await searchBuildableItems(query, 30);
        // Ensure we always have an array (API might return null)
        const safeResults = results ?? [];
        setSearchResults(safeResults);
        setShowDropdown(safeResults.length > 0);
        setHighlightedIndex(safeResults.length > 0 ? 0 : -1);
      } catch (e) {
        console.error("Search error:", e);
        setSearchResults([]);
        setShowDropdown(false);
      } finally {
        setSearching(false);
      }
    }, 200); // Faster debounce for better UX
  }, []);

  // Select item
  const handleSelectItem = useCallback((item: BuildableItem) => {
    setSelectedItem(item);
    setSearchQuery(item.type_name);
    setShowDropdown(false);
    setHighlightedIndex(-1);
    setResult(null);
  }, []);

  // Keyboard navigation
  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (!showDropdown || !searchResults || searchResults.length === 0) return;

    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        setHighlightedIndex(prev => 
          prev < searchResults.length - 1 ? prev + 1 : 0
        );
        break;
      case "ArrowUp":
        e.preventDefault();
        setHighlightedIndex(prev => 
          prev > 0 ? prev - 1 : searchResults.length - 1
        );
        break;
      case "Enter":
        e.preventDefault();
        if (highlightedIndex >= 0 && highlightedIndex < searchResults.length) {
          handleSelectItem(searchResults[highlightedIndex]);
        }
        break;
      case "Escape":
        setShowDropdown(false);
        setHighlightedIndex(-1);
        break;
    }
  }, [showDropdown, searchResults, highlightedIndex, handleSelectItem]);

  // Analyze
  const handleAnalyze = useCallback(async () => {
    if (!selectedItem) return;

    if (analyzing) {
      abortRef.current?.abort();
      return;
    }

    const controller = new AbortController();
    abortRef.current = controller;
    setAnalyzing(true);
    setProgress(t("scanStarting"));
    setResult(null);

    const params: IndustryParams = {
      type_id: selectedItem.type_id,
      runs,
      me,
      te,
      system_name: systemName,
      facility_tax: facilityTax,
      structure_bonus: structureBonus,
      broker_fee: brokerFee,
      sales_tax_percent: salesTaxPercent,
      max_depth: 10,
      own_blueprint: ownBlueprint,
      blueprint_cost: ownBlueprint ? 0 : blueprintCost,
      blueprint_is_bpo: blueprintIsBPO,
    };

    try {
      const analysis = await analyzeIndustry(params, setProgress, controller.signal);
      setResult(analysis);
      setProgress("");
    } catch (e: unknown) {
      if (e instanceof Error && e.name !== "AbortError") {
        setProgress(t("errorPrefix") + e.message);
        onError?.(e.message);
      }
    } finally {
      setAnalyzing(false);
    }
  }, [analyzing, selectedItem, runs, me, te, systemName, facilityTax, structureBonus, brokerFee, salesTaxPercent, t, onError]);

  return (
    <div className="flex-1 flex flex-col min-h-0">
      {/* Settings Panel */}
      <div className="shrink-0 m-2">
        <TabSettingsPanel
          title={t("industrySettings")}
          hint={t("industrySettingsHint")}
          icon="🏭"
          defaultExpanded={true}
          persistKey="industry"
          help={{ stepKeys: ["helpIndustryStep1", "helpIndustryStep2", "helpIndustryStep3"], wikiSlug: "Industry-Chain-Optimizer" }}
        >
          {/* Item Search */}
          <div className="mb-4">
            <SettingsField label={t("industrySelectItem")}>
              <div className="relative">
                <input
                  ref={inputRef}
                  type="text"
                  value={searchQuery}
                  onChange={(e) => handleSearch(e.target.value)}
                  onFocus={() => searchResults?.length > 0 && setShowDropdown(true)}
                  onKeyDown={handleKeyDown}
                  placeholder={t("industrySearchPlaceholder")}
                  className="w-full px-3 py-1.5 bg-eve-input border border-eve-border rounded-sm text-eve-text text-sm
                           focus:outline-none focus:border-eve-accent focus:ring-1 focus:ring-eve-accent/30 transition-colors"
                  autoComplete="off"
                />
                {searching && (
                  <div className="absolute right-2 top-1/2 -translate-y-1/2">
                    <span className="w-4 h-4 border-2 border-eve-accent border-t-transparent rounded-full animate-spin inline-block" />
                  </div>
                )}
                {showDropdown && searchResults && searchResults.length > 0 && (
                  <div 
                    ref={dropdownRef}
                    className="absolute z-50 w-full mt-1 bg-eve-dark border border-eve-border rounded-sm shadow-lg max-h-60 overflow-auto"
                  >
                    {searchResults.map((item, index) => (
                      <button
                        key={item.type_id}
                        onClick={() => handleSelectItem(item)}
                        onMouseEnter={() => setHighlightedIndex(index)}
                        className={`w-full px-3 py-2 text-left text-sm transition-colors flex items-center justify-between ${
                          index === highlightedIndex
                            ? "bg-eve-accent/20 text-eve-accent"
                            : "text-eve-text hover:bg-eve-accent/10"
                        } ${!item.has_blueprint ? "opacity-60" : ""}`}
                      >
                        <span>
                          <HighlightMatch text={item.type_name} query={searchQuery} />
                        </span>
                        {item.has_blueprint ? (
                          <span className="text-[10px] px-1.5 py-0.5 bg-green-500/20 text-green-400 rounded-sm ml-2">BP</span>
                        ) : (
                          <span className="text-[10px] px-1.5 py-0.5 bg-eve-dim/20 text-eve-dim rounded-sm ml-2">No BP</span>
                        )}
                      </button>
                    ))}
                  </div>
                )}
              </div>
            </SettingsField>
          </div>

          {/* Parameters Grid */}
          <SettingsGrid cols={5}>
            <SettingsField label={t("industryRuns")}>
              <SettingsNumberInput value={runs} onChange={setRuns} min={1} max={10000} />
            </SettingsField>
            <SettingsField label={t("industryME")}>
              <SettingsNumberInput value={me} onChange={setME} min={0} max={10} />
            </SettingsField>
            <SettingsField label={t("industryTE")}>
              <SettingsNumberInput value={te} onChange={setTE} min={0} max={20} />
            </SettingsField>
            <SettingsField label={t("system")}>
              <SystemAutocomplete value={systemName} onChange={setSystemName} isLoggedIn={isLoggedIn} />
            </SettingsField>
            <SettingsField label={t("industryFacilityTax")}>
              <SettingsNumberInput value={facilityTax} onChange={setFacilityTax} min={0} max={50} step={0.1} />
            </SettingsField>
          </SettingsGrid>

          {/* After broker: broker fee and sales tax */}
          <div className="mt-3 pt-3 border-t border-eve-border/30">
            <div className="text-[10px] uppercase tracking-wider text-eve-dim mb-2">{t("industryAfterBroker")}</div>
            <SettingsGrid cols={5}>
              <SettingsField label={t("brokerFee")}>
                <SettingsNumberInput value={brokerFee} onChange={setBrokerFee} min={0} max={10} step={0.1} />
              </SettingsField>
              <SettingsField label={t("salesTax")}>
                <SettingsNumberInput value={salesTaxPercent} onChange={setSalesTaxPercent} min={0} max={100} step={0.1} />
              </SettingsField>
            </SettingsGrid>
          </div>

          {/* Advanced Options */}
          <details className="mt-3 group">
            <summary className="cursor-pointer text-xs text-eve-dim hover:text-eve-accent transition-colors flex items-center gap-1">
              <span className="group-open:rotate-90 transition-transform">▶</span>
              {t("advancedFilters")}
            </summary>
            <div className="mt-3 pt-3 border-t border-eve-border/30">
              <SettingsGrid cols={3}>
                <SettingsField label={t("industryStructureBonus")}>
                  <SettingsNumberInput value={structureBonus} onChange={setStructureBonus} min={0} max={5} step={0.1} />
                </SettingsField>
              </SettingsGrid>
            </div>
          </details>

          {/* Blueprint ownership */}
          <div className="mt-3 pt-3 border-t border-eve-border/30">
            <label className="flex items-center gap-2 cursor-pointer text-xs">
              <input
                type="checkbox"
                checked={ownBlueprint}
                onChange={(e) => setOwnBlueprint(e.target.checked)}
                className="accent-eve-accent"
              />
              <span className="text-eve-text">{t("industryOwnBlueprint")}</span>
            </label>
            {!ownBlueprint && (
              <div className="mt-2 flex items-center gap-3 flex-wrap">
                <div className="flex items-center gap-1.5">
                  <label className="text-eve-dim text-xs">{t("industryBlueprintCost")}</label>
                  <input
                    type="number"
                    min="0"
                    value={blueprintCost || ""}
                    onChange={(e) => setBlueprintCost(parseFloat(e.target.value) || 0)}
                    className="w-32 px-1.5 py-0.5 bg-eve-input border border-eve-border rounded-sm text-xs text-eve-text font-mono"
                  />
                </div>
                <div className="flex items-center gap-1.5">
                  <button
                    onClick={() => setBlueprintIsBPO(true)}
                    className={`px-2 py-0.5 text-[10px] font-semibold rounded-sm border transition-colors ${blueprintIsBPO ? "border-eve-accent bg-eve-accent/10 text-eve-accent" : "border-eve-border text-eve-dim hover:text-eve-text"}`}
                  >
                    {t("industryBPO")}
                  </button>
                  <button
                    onClick={() => setBlueprintIsBPO(false)}
                    className={`px-2 py-0.5 text-[10px] font-semibold rounded-sm border transition-colors ${!blueprintIsBPO ? "border-eve-accent bg-eve-accent/10 text-eve-accent" : "border-eve-border text-eve-dim hover:text-eve-text"}`}
                  >
                    {t("industryBPC")}
                  </button>
                </div>
                {blueprintIsBPO && blueprintCost > 0 && runs > 0 && (
                  <span className="text-[10px] text-eve-dim italic">
                    ≈ {formatISK(blueprintCost / runs)} / run
                  </span>
                )}
              </div>
            )}
          </div>

          {/* Analyze Button */}
          <div className="mt-4 pt-3 border-t border-eve-border/30 flex items-center gap-4 flex-wrap">
            <button
              onClick={handleAnalyze}
              disabled={!selectedItem || (selectedItem && !selectedItem.has_blueprint)}
              className={`px-5 py-1.5 rounded-sm text-xs font-semibold uppercase tracking-wider transition-all
                ${analyzing
                  ? "bg-eve-error/80 text-white hover:bg-eve-error"
                  : "bg-eve-accent text-eve-dark hover:bg-eve-accent-hover shadow-eve-glow"
                }
                disabled:bg-eve-input disabled:text-eve-dim disabled:cursor-not-allowed disabled:shadow-none`}
            >
              {analyzing ? t("stop") : t("industryAnalyze")}
            </button>
            {progress && <span className="text-xs text-eve-dim">{progress}</span>}
            {selectedItem && !selectedItem.has_blueprint && (
              <span className="text-xs text-yellow-400">
                {t("industryNoBlueprint")}
              </span>
            )}
          </div>
        </TabSettingsPanel>
      </div>

      {/* Results */}
      {result && (
        <div className="flex-1 min-h-0 m-2 mt-0 flex flex-col">
          {/* Summary Cards */}
          <div className="shrink-0 grid grid-cols-2 md:grid-cols-4 gap-2 mb-2">
            <SummaryCard
              label={t("industryMarketPrice")}
              value={formatISK(result.market_buy_price ?? 0)}
              subtext={`${(result.total_quantity ?? 0).toLocaleString()} ${t("industryUnits")}`}
              color="text-eve-dim"
            />
            <SummaryCard
              label={t("industryBuildCost")}
              value={formatISK(result.optimal_build_cost ?? 0)}
              subtext={result.blueprint_cost_included > 0
                ? `${t("industryJobCost")}: ${formatISK(result.total_job_cost ?? 0)} · ${t("industryBPCostIncluded")}: ${formatISK(result.blueprint_cost_included)}`
                : `${t("industryJobCost")}: ${formatISK(result.total_job_cost ?? 0)}`}
              color="text-eve-accent"
            />
            <SummaryCard
              label={t("industrySavings")}
              value={formatISK(result.savings ?? 0)}
              subtext={`${(result.savings_percent ?? 0).toFixed(1)}%`}
              color={(result.savings ?? 0) > 0 ? "text-green-400" : "text-red-400"}
            />
            <SummaryCard
              label={t("industryProfit")}
              value={formatISK(result.profit ?? 0)}
              subtext={`${(result.profit_percent ?? 0).toFixed(1)}% ROI`}
              color={(result.profit ?? 0) > 0 ? "text-green-400" : "text-red-400"}
            />
          </div>

          {/* ISK/hour and Manufacturing Time */}
          <div className="shrink-0 grid grid-cols-2 md:grid-cols-4 gap-2 mb-2">
            <SummaryCard
              label={t("industryISKPerHour")}
              value={formatISK(result.isk_per_hour ?? 0)}
              color={(result.isk_per_hour ?? 0) > 0 ? "text-yellow-400" : "text-red-400"}
            />
            <SummaryCard
              label={t("industryMfgTime")}
              value={formatDuration(result.manufacturing_time ?? 0)}
              color="text-eve-dim"
            />
            <SummaryCard
              label={t("industrySellRevenue")}
              value={formatISK(result.sell_revenue ?? 0)}
              subtext={`-${salesTaxPercent}% tax -${brokerFee}% broker`}
              color="text-eve-dim"
            />
            <SummaryCard
              label={t("industryJobCost")}
              value={formatISK(result.total_job_cost ?? 0)}
              subtext={`SCI: ${((result.system_cost_index ?? 0) * 100).toFixed(2)}%`}
              color="text-eve-dim"
            />
          </div>

          {/* View Toggle + Export */}
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
                      (m) => `"${(m.type_name || "").replace(/"/g, '""')}",${m.quantity},${m.unit_price},${m.total_price},${m.volume}`
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

          {/* Content */}
          <div className="flex-1 min-h-0 overflow-auto border border-eve-border rounded-sm bg-eve-panel">
            {viewMode === "tree" ? (
              <MaterialTree node={result.material_tree} />
            ) : (
              <ShoppingList
                materials={result.flat_materials}
                regionId={result.region_id ?? 0}
                onOpenExecutionPlan={setExecPlanMaterial}
              />
            )}
          </div>
        </div>
      )}

      {/* Empty State */}
      {!result && !analyzing && (
        <div className="flex-1 flex items-center justify-center min-h-[200px]">
          <EmptyState reason="no_item_selected" wikiSlug="Industry-Chain-Optimizer" />
        </div>
      )}

      <ExecutionPlannerPopup
        open={execPlanMaterial !== null}
        onClose={() => setExecPlanMaterial(null)}
        typeID={execPlanMaterial?.type_id ?? 0}
        typeName={execPlanMaterial?.type_name ?? ""}
        regionID={result?.region_id ?? 0}
        defaultQuantity={execPlanMaterial?.quantity ?? 100}
        isBuy={true}
        salesTaxPercent={salesTaxPercent}
      />
    </div>
  );
}

// Summary Card Component
function SummaryCard({
  label,
  value,
  subtext,
  color = "text-eve-accent",
}: {
  label: string;
  value: string;
  subtext?: string;
  color?: string;
}) {
  return (
    <div className="bg-eve-panel border border-eve-border rounded-sm p-3">
      <div className="text-[10px] uppercase tracking-wider text-eve-dim mb-1">{label}</div>
      <div className={`text-lg font-mono font-semibold ${color}`}>{value}</div>
      {subtext && <div className="text-xs text-eve-dim">{subtext}</div>}
    </div>
  );
}

// Material Tree Component
function MaterialTree({ node }: { node: MaterialNode }) {
  return (
    <div className="p-2">
      <TreeNode node={node} />
    </div>
  );
}

function TreeNode({ node, level = 0 }: { node: MaterialNode; level?: number }) {
  const [expanded, setExpanded] = useState(level < 2);
  const hasChildren = node.children && node.children.length > 0;
  const indent = level * 20;

  return (
    <div>
      <div
        className={`flex items-center py-1 px-2 hover:bg-eve-accent/5 rounded-sm ${
          node.should_build ? "" : "opacity-70"
        }`}
        style={{ paddingLeft: Math.min(indent + 8, 120) }}
      >
        {/* Expand/Collapse Toggle */}
        {hasChildren ? (
          <button
            onClick={() => setExpanded(!expanded)}
            className="w-4 h-4 flex items-center justify-center text-eve-dim hover:text-eve-accent mr-1"
          >
            {expanded ? "▼" : "▶"}
          </button>
        ) : (
          <span className="w-4 h-4 mr-1" />
        )}

        {/* Item Info */}
        <span className="flex-1 text-sm text-eve-text truncate">
          {node.type_name}
          <span className="text-eve-dim ml-2">×{node.quantity.toLocaleString()}</span>
        </span>

        {/* Prices */}
        <span className="text-xs text-eve-dim mx-2">
          Buy: {formatISK(node.buy_price)}
        </span>
        {!node.is_base && (
          <span className="text-xs text-eve-dim mx-2">
            Build: {formatISK(node.build_cost)}
          </span>
        )}

        {/* Decision Badge */}
        {!node.is_base && (
          <span
            className={`text-[10px] px-2 py-0.5 rounded-sm ${
              node.should_build
                ? "bg-green-500/20 text-green-400"
                : "bg-blue-500/20 text-blue-400"
            }`}
          >
            {node.should_build ? "BUILD" : "BUY"}
          </span>
        )}
        {node.is_base && (
          <span className="text-[10px] px-2 py-0.5 rounded-sm bg-eve-dim/20 text-eve-dim">
            BASE
          </span>
        )}
      </div>

      {/* Children */}
      {expanded && hasChildren && (
        <div>
          {node.children!.map((child, i) => (
            <TreeNode key={`${child.type_id}-${i}`} node={child} level={level + 1} />
          ))}
        </div>
      )}
    </div>
  );
}

// Shopping List Component
function ShoppingList({
  materials,
  regionId,
  onOpenExecutionPlan,
}: {
  materials: FlatMaterial[];
  regionId: number;
  onOpenExecutionPlan: (m: FlatMaterial) => void;
}) {
  const { t } = useI18n();
  const totalCost = useMemo(() => 
    materials.reduce((sum, m) => sum + m.total_price, 0), 
    [materials]
  );

  const totalVolume = useMemo(() => 
    materials.reduce((sum, m) => sum + m.volume, 0), 
    [materials]
  );

  return (
    <div>
      <table className="w-full text-sm">
        <thead className="sticky top-0 bg-eve-dark z-10">
          <tr className="text-eve-dim text-[10px] uppercase tracking-wider border-b border-eve-border">
            <th className="min-w-[32px] px-1 py-2" />
            <th className="px-3 py-2 text-left font-medium">Item</th>
            <th className="px-3 py-2 text-right font-medium">Quantity</th>
            <th className="px-3 py-2 text-right font-medium">Unit Price</th>
            <th className="px-3 py-2 text-right font-medium">Total</th>
            <th className="px-3 py-2 text-right font-medium">Volume</th>
          </tr>
        </thead>
        <tbody>
          {materials.map((m, i) => (
            <tr
              key={m.type_id}
              className={`border-b border-eve-border/50 hover:bg-eve-accent/5 ${
                i % 2 === 0 ? "bg-eve-panel" : "bg-eve-dark"
              }`}
            >
              <td className="px-1 py-1.5 text-center">
                {regionId > 0 && (
                  <button
                    type="button"
                    onClick={() => onOpenExecutionPlan(m)}
                    className="text-eve-dim hover:text-eve-accent transition-colors text-sm"
                    title={t("execPlanTitle")}
                  >
                    📊
                  </button>
                )}
              </td>
              <td className="px-3 py-1.5 text-eve-text">{m.type_name}</td>
              <td className="px-3 py-1.5 text-right font-mono text-eve-accent">
                {m.quantity.toLocaleString()}
              </td>
              <td className="px-3 py-1.5 text-right font-mono text-eve-dim">
                {formatISK(m.unit_price)}
              </td>
              <td className="px-3 py-1.5 text-right font-mono text-eve-accent">
                {formatISK(m.total_price)}
              </td>
              <td className="px-3 py-1.5 text-right font-mono text-eve-dim">
                {m.volume.toLocaleString(undefined, { maximumFractionDigits: 1 })} m³
              </td>
            </tr>
          ))}
        </tbody>
        <tfoot className="bg-eve-dark border-t border-eve-border">
          <tr>
            <td className="px-1 py-2" />
            <td className="px-3 py-2 text-eve-dim font-medium">Total</td>
            <td className="px-3 py-2 text-right font-mono text-eve-accent font-semibold">
              {materials.length} items
            </td>
            <td className="px-3 py-2" />
            <td className="px-3 py-2 text-right font-mono text-eve-accent font-semibold">
              {formatISK(totalCost)}
            </td>
            <td className="px-3 py-2 text-right font-mono text-eve-dim">
              {totalVolume.toLocaleString(undefined, { maximumFractionDigits: 1 })} m³
            </td>
          </tr>
        </tfoot>
      </table>
    </div>
  );
}
