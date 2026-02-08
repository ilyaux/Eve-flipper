import { useState } from "react";
import { SystemAutocomplete } from "./SystemAutocomplete";
import { RegionAutocomplete } from "./RegionAutocomplete";
import { useI18n } from "@/lib/i18n";
import { TabHelp } from "./TabHelp";
import { PresetPicker } from "./PresetPicker";
import { getPresetsForTab } from "@/lib/presets";
import type { ScanParams } from "@/lib/types";

type TabForParams = "radius" | "region" | "contracts" | "route";

interface Props {
  params: ScanParams;
  onChange: (params: ScanParams) => void;
  isLoggedIn?: boolean;
  tab?: TabForParams;
}

const HELP_STEPS: Record<TabForParams, { steps: string[]; wiki: string }> = {
  radius: { steps: ["helpFlipperStep1", "helpFlipperStep2", "helpFlipperStep3"], wiki: "Getting-Started" },
  region: { steps: ["helpFlipperStep1", "helpFlipperStep2", "helpFlipperStep3"], wiki: "Getting-Started" },
  contracts: { steps: ["helpContractsStep1", "helpContractsStep2", "helpContractsStep3"], wiki: "Contract-Arbitrage" },
  route: { steps: ["helpRouteStep1", "helpRouteStep2", "helpRouteStep3"], wiki: "Route-Builder" },
};

const inputClass =
  "w-full px-2.5 py-1.5 bg-eve-input border border-eve-border rounded text-eve-text text-sm font-mono " +
  "focus:outline-none focus:border-eve-accent focus:ring-1 focus:ring-eve-accent/30 transition-colors " +
  "[appearance:textfield] [&::-webkit-outer-spin-button]:appearance-none [&::-webkit-inner-spin-button]:appearance-none";

const PERSIST_KEY = "eve-settings-expanded:params";

export function ParametersPanel({ params, onChange, isLoggedIn = false, tab = "radius" }: Props) {
  const { t } = useI18n();
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [expanded, setExpanded] = useState(() => {
    const stored = localStorage.getItem(PERSIST_KEY);
    if (stored !== null) return stored === "1";
    return true;
  });
  const help = HELP_STEPS[tab];

  const toggleExpanded = () => {
    setExpanded((prev) => {
      const next = !prev;
      localStorage.setItem(PERSIST_KEY, next ? "1" : "0");
      return next;
    });
  };

  const set = <K extends keyof ScanParams>(key: K, value: ScanParams[K]) => {
    onChange({ ...params, [key]: value });
  };

  return (
    <div className="bg-eve-panel border border-eve-border rounded-sm overflow-visible">
      {/* Header: collapse toggle + preset picker + help */}
      <div className="flex items-center justify-between gap-3 px-3 py-2 border-b border-eve-border/60 bg-eve-panel/80">
        <button
          onClick={toggleExpanded}
          className="flex items-center gap-2 text-left hover:bg-eve-accent/5 transition-colors rounded-sm px-1 -ml-1"
        >
          <span className="text-eve-accent text-sm">⚙</span>
          <span className="text-sm font-medium text-eve-text">{t("scanParameters")}</span>
          <span className="text-eve-dim text-xs">{expanded ? "▲" : "▼"}</span>
        </button>
        <div className="flex items-center gap-2" onClick={(e) => e.stopPropagation()}>
          <PresetPicker params={params} onApply={onChange} tab={tab} builtinPresets={getPresetsForTab(tab)} align="right" />
          {help && <TabHelp stepKeys={help.steps} wikiSlug={help.wiki} />}
        </div>
      </div>

      {expanded && (
      <div className="p-3">
        {/* Main grid - 4 columns on desktop, 2 on mobile */}
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-x-4 gap-y-3">
          {/* Row 1 */}
          <Field label={t("system")}>
            <SystemAutocomplete
              value={params.system_name}
              onChange={(v) => set("system_name", v)}
              isLoggedIn={isLoggedIn}
              includeStructures={params.include_structures}
              onIncludeStructuresChange={(v) => set("include_structures", v)}
            />
          </Field>

          {tab === "region" ? (
            <Field label={t("targetRegion") || "Target Region"} hint={t("targetRegionHint")}>
              <RegionAutocomplete
                value={params.target_region ?? ""}
                onChange={(v) => set("target_region", v)}
                placeholder="Delve, Catch, Vale of the Silent..."
              />
            </Field>
          ) : (
            <Field label={t("paramsCargo")}>
              <NumberInput
                value={params.cargo_capacity}
                onChange={(v) => set("cargo_capacity", v)}
                min={1}
                max={1000000}
              />
            </Field>
          )}
          
          <Field label={t("paramsBuy")}>
            <NumberInput
              value={params.buy_radius}
              onChange={(v) => set("buy_radius", v)}
              min={1}
              max={50}
            />
          </Field>
          
          {!(tab === "region" && params.target_region) && (
            <Field label={t("paramsSell")}>
              <NumberInput
                value={params.sell_radius}
                onChange={(v) => set("sell_radius", v)}
                min={1}
                max={50}
              />
            </Field>
          )}

          {/* Row 2 */}
          {tab === "region" && (
            <Field label={t("paramsCargo")}>
              <NumberInput
                value={params.cargo_capacity}
                onChange={(v) => set("cargo_capacity", v)}
                min={1}
                max={1000000}
              />
            </Field>
          )}
          
          <Field label={t("paramsMargin")}>
            <NumberInput
              value={params.min_margin}
              onChange={(v) => set("min_margin", v)}
              min={0.1}
              max={1000}
              step={0.1}
            />
          </Field>
          
          <Field label={t("paramsTax")}>
            <NumberInput
              value={params.sales_tax_percent}
              onChange={(v) => set("sales_tax_percent", v)}
              min={0}
              max={100}
              step={0.1}
            />
          </Field>

          <Field label={t("paramsBrokerFee")}>
            <NumberInput
              value={params.broker_fee_percent}
              onChange={(v) => set("broker_fee_percent", v)}
              min={0}
              max={10}
              step={0.1}
            />
          </Field>
          
          <Field label={t("paramsResults")}>
            <select
              value={params.max_results ?? 100}
              onChange={(e) => set("max_results", parseInt(e.target.value))}
              className={inputClass}
            >
              <option value={50}>50</option>
              <option value={100}>100</option>
              <option value={250}>250</option>
              <option value={500}>500</option>
              <option value={1000}>1000</option>
            </select>
          </Field>
        </div>

        {/* Advanced filters toggle */}
        <div className="border-t border-eve-border/50 mt-3 pt-2">
          <button
            type="button"
            onClick={() => setShowAdvanced((a) => !a)}
            className="flex items-center gap-1.5 text-[11px] uppercase tracking-wider text-eve-dim hover:text-eve-accent font-medium transition-colors"
          >
            <span className={`transition-transform ${showAdvanced ? "rotate-90" : ""}`}>▸</span>
            {t("advancedFilters")}
          </button>
          
          {showAdvanced && (
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-x-4 gap-y-3 mt-3">
              <Field label={t("paramsSecurity")}>
                <select
                  value={String(params.min_route_security ?? 0)}
                  onChange={(e) => set("min_route_security", parseFloat(e.target.value))}
                  className={inputClass}
                >
                  <option value="0">{t("routeSecurityAll")}</option>
                  <option value="0.45">{t("routeSecurityHighsec")}</option>
                  <option value="0.5">{t("routeSecurityMin05")}</option>
                  <option value="0.7">{t("routeSecurityMin07")}</option>
                </select>
              </Field>
              
              <Field label={t("minDailyVolume")}>
                <NumberInput
                  value={params.min_daily_volume ?? 0}
                  onChange={(v) => set("min_daily_volume", v)}
                  min={0}
                  max={999999999}
                />
              </Field>
              
              <Field label={t("maxInvestment")}>
                <NumberInput
                  value={params.max_investment ?? 0}
                  onChange={(v) => set("max_investment", v)}
                  min={0}
                  max={999999999999}
                />
              </Field>
            </div>
          )}
        </div>
      </div>
      )}
    </div>
  );
}

function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-1 min-w-0">
      <label className="flex items-center gap-1 text-[10px] uppercase tracking-wider text-eve-dim font-medium truncate" title={hint}>
        {label}
        {hint && (
          <span className="text-eve-dim/60 hover:text-eve-accent cursor-help" title={hint}>
            <svg className="w-3 h-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <circle cx="12" cy="12" r="10" />
              <path d="M12 16v-4M12 8h.01" />
            </svg>
          </span>
        )}
      </label>
      {children}
    </div>
  );
}

function NumberInput({
  value,
  onChange,
  min,
  max,
  step = 1,
}: {
  value: number;
  onChange: (v: number) => void;
  min: number;
  max: number;
  step?: number;
}) {
  return (
    <input
      type="number"
      value={value}
      onChange={(e) => {
        const v = parseFloat(e.target.value);
        if (!isNaN(v) && v >= min && v <= max) onChange(v);
      }}
      min={min}
      max={max}
      step={step}
      className={inputClass}
    />
  );
}
