import { useEffect, useMemo, useRef, useState } from "react";
import { SystemAutocomplete } from "./SystemAutocomplete";
import { RegionAutocomplete } from "./RegionAutocomplete";
import { useI18n } from "@/lib/i18n";
import { TabHelp } from "./TabHelp";
import { PresetPicker } from "./PresetPicker";
import { SystemBlacklistButton } from "./SystemBlacklistButton";
import { getPresetsForTab } from "@/lib/presets";
import { getStations, getStructures, getCharacterInfo } from "@/lib/api";
import type { ScanParams, StationInfo } from "@/lib/types";

// EVE skill IDs for trade fee calculation
const SKILL_ACCOUNTING = 3443;      // reduces sales tax by 11% per level
const SKILL_BROKER_RELATIONS = 3446; // reduces broker fee by 0.1% per level (station) + 0.03% (structure)

type TabForParams = "radius" | "region" | "contracts" | "route";

interface Props {
  params: ScanParams;
  onChange: (params: ScanParams) => void;
  isLoggedIn?: boolean;
  tab?: TabForParams;
}

const HELP_STEPS: Record<TabForParams, { steps: string[]; wiki: string }> = {
  radius: {
    steps: ["helpFlipperStep1", "helpFlipperStep2", "helpFlipperStep3"],
    wiki: "Getting-Started",
  },
  region: {
    steps: ["helpFlipperStep1", "helpFlipperStep2", "helpFlipperStep3"],
    wiki: "Getting-Started",
  },
  contracts: {
    steps: ["helpContractsStep1", "helpContractsStep2", "helpContractsStep3"],
    wiki: "Contract-Arbitrage",
  },
  route: {
    steps: ["helpRouteStep1", "helpRouteStep2", "helpRouteStep3"],
    wiki: "Route-Builder",
  },
};

const inputClass =
  "w-full px-2.5 py-1.5 bg-eve-input border border-eve-border rounded text-eve-text text-sm font-mono " +
  "focus:outline-none focus:border-eve-accent focus:ring-1 focus:ring-eve-accent/30 transition-colors " +
  "[appearance:textfield] [&::-webkit-outer-spin-button]:appearance-none [&::-webkit-inner-spin-button]:appearance-none";

const PERSIST_KEY = "eve-settings-expanded:params";

// EVE Online item categories for the regional day trader category filter.
// IDs are stable SDE constants. Labels are intentionally concise for chip display.
const EVE_CATEGORIES: { id: number; label: string; hint: string }[] = [
  { id: 6,  label: "Ships",       hint: "Ships (frigates, cruisers, capitals…)" },
  { id: 7,  label: "Modules",     hint: "Ship modules (armor, shield, propulsion…)" },
  { id: 8,  label: "Charges",     hint: "Ammunition and charges" },
  { id: 18, label: "Drones",      hint: "Combat, mining and utility drones" },
  { id: 20, label: "Implants",    hint: "Implants and boosters" },
  { id: 9,  label: "Blueprints",  hint: "Blueprints (originals and copies)" },
  { id: 32, label: "Subsystems",  hint: "T3 strategic cruiser subsystems" },
  { id: 35, label: "Deployables", hint: "Deployable structures and cans" },
  { id: 43, label: "PI",          hint: "Planetary industry commodities" },
  { id: 65, label: "Structures",  hint: "Upwell structures and components" },
];

const sectionClass =
  "rounded-sm border border-eve-border/60 bg-gradient-to-br from-eve-panel to-eve-dark/40";

const MAJOR_HUB_SOURCE_REGIONS = [
  "The Forge",
  "Domain",
  "Sinq Laison",
  "Metropolis",
  "Heimatar",
] as const;

const CARGO_INPUT_MAX = 1_000_000_000;

type SourceRegionMode = "major_hubs" | "radius" | "single_region";

function normalizeRegionName(name: string): string {
  return name.trim().toLowerCase();
}

function regionSetEquals(a: readonly string[], b: readonly string[]): boolean {
  if (a.length !== b.length) return false;
  const aset = new Set(a.map(normalizeRegionName));
  if (aset.size !== b.length) return false;
  for (const item of b) {
    if (!aset.has(normalizeRegionName(item))) return false;
  }
  return true;
}

export function ParametersPanel({
  params,
  onChange,
  isLoggedIn = false,
  tab = "radius",
}: Props) {
  const { t } = useI18n();
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [expanded, setExpanded] = useState(() => {
    const stored = localStorage.getItem(PERSIST_KEY);
    if (stored !== null) return stored === "1";
    return true;
  });
  const help = HELP_STEPS[tab];
  const targetMarketSystem = (params.target_market_system ?? "").trim();
  const sourceRegions = useMemo(
    () =>
      (params.source_regions ?? [])
        .map((name) => name.trim())
        .filter((name) => name.length > 0),
    [params.source_regions],
  );
  const sourceRegionMode: SourceRegionMode = useMemo(() => {
    if (tab !== "region") return "radius";
    if (sourceRegions.length === 0) return "radius";
    if (regionSetEquals(sourceRegions, MAJOR_HUB_SOURCE_REGIONS)) {
      return "major_hubs";
    }
    return "single_region";
  }, [sourceRegions, tab]);
  const singleSourceRegion = sourceRegions[0] ?? "";
  const includeStructures = Boolean(params.include_structures);
  const splitTradeFees = Boolean(params.split_trade_fees);
  const isFlowTab = tab === "radius" || tab === "region";
  const hideSellRadius = tab === "region" || tab === "route";
  const showBuyRadius =
    tab !== "route" && !(tab === "region" && sourceRegionMode !== "radius");
  const showCargoInMain = tab !== "region" && tab !== "contracts";

  const [targetStations, setTargetStations] = useState<StationInfo[]>([]);
  const [targetStructureStations, setTargetStructureStations] = useState<
    StationInfo[]
  >([]);
  const [targetSystemID, setTargetSystemID] = useState(0);
  const [targetRegionID, setTargetRegionID] = useState(0);
  const [loadingTargetStations, setLoadingTargetStations] = useState(false);
  const [loadingTargetStructures, setLoadingTargetStructures] = useState(false);

  const activeAdvancedCount =
    Number(tab !== "region" && (params.min_route_security ?? 0) > 0) +
    (isFlowTab
      ? Number((params.min_daily_volume ?? 0) > 0) +
        Number(
          tab === "region" &&
            (params.shipping_cost_per_m3_jump ?? 0) > 0,
        ) +
        Number((params.max_investment ?? 0) > 0) +
        Number((params.min_s2b_per_day ?? 0) > 0) +
        Number((params.min_bfs_per_day ?? 0) > 0) +
        Number((params.min_s2b_bfs_ratio ?? 0) > 0) +
        Number((params.max_s2b_bfs_ratio ?? 0) > 0) +
        Number(tab === "region" && (params.min_period_roi ?? 0) > 0) +
        Number(tab === "region" && (params.max_dos ?? 0) > 0) +
        Number(tab === "region" && (params.min_demand_per_day ?? 0) > 0) +
        Number(tab === "region" && (params.category_ids ?? []).length > 0)
      : 0);

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

  const setSourceRegionMode = (mode: SourceRegionMode) => {
    if (tab !== "region") return;
    if (mode === "major_hubs") {
      onChange({
        ...params,
        source_regions: [...MAJOR_HUB_SOURCE_REGIONS],
      });
      return;
    }
    if (mode === "radius") {
      onChange({
        ...params,
        source_regions: [],
      });
      return;
    }
    onChange({
      ...params,
      source_regions: singleSourceRegion ? [singleSourceRegion] : [],
    });
  };

  const setSingleSourceRegion = (regionName: string) => {
    const next = regionName.trim();
    onChange({
      ...params,
      source_regions: next ? [next] : [],
    });
  };

  const [esiSkillsLoading, setEsiSkillsLoading] = useState(false);
  const [esiSkillsMsg, setEsiSkillsMsg] = useState<string | null>(null);
  const esiMsgTimer = useRef<ReturnType<typeof setTimeout>>(undefined);

  const fetchSkillsFromESI = async () => {
    setEsiSkillsLoading(true);
    setEsiSkillsMsg(null);
    try {
      const info = await getCharacterInfo();
      const skills = info.skills?.skills ?? [];
      const accounting = skills.find((s) => s.skill_id === SKILL_ACCOUNTING)?.active_skill_level ?? 0;
      const brokerRel = skills.find((s) => s.skill_id === SKILL_BROKER_RELATIONS)?.active_skill_level ?? 0;

      // Accounting: base sales tax 8%, each level reduces by 11% of base (not of current)
      // Formula: tax = 8% * (1 - 0.11 * level)
      const salesTax = parseFloat((8 * (1 - 0.11 * accounting)).toFixed(2));
      // Broker Relations: base broker fee 3%, each level reduces by 0.1% (NPC station)
      const brokerFee = parseFloat(Math.max(0, 3 - brokerRel * 0.1).toFixed(2));

      onChange({
        ...params,
        sales_tax_percent: salesTax,
        broker_fee_percent: brokerFee,
        sell_sales_tax_percent: salesTax,
        buy_broker_fee_percent: brokerFee,
        sell_broker_fee_percent: brokerFee,
      });
      setEsiSkillsMsg(`✓ Accounting L${accounting} → tax ${salesTax}%  ·  Broker L${brokerRel} → fee ${brokerFee}%`);
    } catch {
      setEsiSkillsMsg("✗ ESI error — check character login");
    } finally {
      setEsiSkillsLoading(false);
      clearTimeout(esiMsgTimer.current);
      esiMsgTimer.current = setTimeout(() => setEsiSkillsMsg(null), 6000);
    }
  };

  const setLegacyBrokerFee = (v: number) => {
    onChange({
      ...params,
      broker_fee_percent: v,
      buy_broker_fee_percent: v,
      sell_broker_fee_percent: v,
    });
  };

  const setLegacySalesTax = (v: number) => {
    onChange({
      ...params,
      sales_tax_percent: v,
      sell_sales_tax_percent: v,
    });
  };

  const setSplitFees = (enabled: boolean) => {
    if (enabled) {
      const legacyBroker = params.broker_fee_percent ?? 0;
      const legacyTax = params.sales_tax_percent ?? 0;
      onChange({
        ...params,
        split_trade_fees: true,
        // Switching from legacy to split should mirror current legacy values,
        // not stale split values from older config snapshots.
        buy_broker_fee_percent: legacyBroker,
        sell_broker_fee_percent: legacyBroker,
        buy_sales_tax_percent: 0,
        sell_sales_tax_percent: legacyTax,
      });
      return;
    }
    onChange({
      ...params,
      split_trade_fees: false,
      broker_fee_percent:
        params.sell_broker_fee_percent ?? params.broker_fee_percent,
      sales_tax_percent:
        params.sell_sales_tax_percent ?? params.sales_tax_percent,
    });
  };

  useEffect(() => {
    if (tab !== "region") {
      setTargetStations([]);
      setTargetStructureStations([]);
      setTargetSystemID(0);
      setTargetRegionID(0);
      setLoadingTargetStations(false);
      return;
    }
    if (!targetMarketSystem) {
      setTargetStations([]);
      setTargetStructureStations([]);
      setTargetSystemID(0);
      setTargetRegionID(0);
      setLoadingTargetStations(false);
      return;
    }

    const controller = new AbortController();
    setLoadingTargetStations(true);
    getStations(targetMarketSystem, controller.signal)
      .then((resp) => {
        if (controller.signal.aborted) return;
        setTargetStations(resp.stations);
        setTargetSystemID(resp.system_id);
        setTargetRegionID(resp.region_id);
        setTargetStructureStations([]);
      })
      .catch(() => {
        if (controller.signal.aborted) return;
        setTargetStations([]);
        setTargetStructureStations([]);
        setTargetSystemID(0);
        setTargetRegionID(0);
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          setLoadingTargetStations(false);
        }
      });

    return () => controller.abort();
  }, [tab, targetMarketSystem]);

  useEffect(() => {
    if (
      tab !== "region" ||
      !targetMarketSystem ||
      !isLoggedIn ||
      !includeStructures ||
      targetSystemID <= 0 ||
      targetRegionID <= 0
    ) {
      setTargetStructureStations([]);
      setLoadingTargetStructures(false);
      return;
    }

    const controller = new AbortController();
    setLoadingTargetStructures(true);
    getStructures(targetSystemID, targetRegionID, controller.signal)
      .then((rows) => {
        if (controller.signal.aborted) return;
        setTargetStructureStations(rows);
      })
      .catch(() => {
        if (controller.signal.aborted) return;
        setTargetStructureStations([]);
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          setLoadingTargetStructures(false);
        }
      });

    return () => controller.abort();
  }, [
    includeStructures,
    isLoggedIn,
    tab,
    targetMarketSystem,
    targetRegionID,
    targetSystemID,
  ]);

  const targetMarketplaceStations = useMemo(() => {
    const merged =
      includeStructures && isLoggedIn
        ? [...targetStations, ...targetStructureStations]
        : [...targetStations];
    merged.sort((a, b) => a.name.localeCompare(b.name));
    return merged;
  }, [
    includeStructures,
    isLoggedIn,
    targetStations,
    targetStructureStations,
  ]);

  useEffect(() => {
    if (tab !== "region") return;
    const selectedLocationID = params.target_market_location_id ?? 0;
    if (selectedLocationID <= 0) return;
    if (loadingTargetStations || loadingTargetStructures) return;
    const exists = targetMarketplaceStations.some(
      (station) => station.id === selectedLocationID,
    );
    if (!exists) {
      onChange({ ...params, target_market_location_id: 0 });
    }
  }, [
    loadingTargetStations,
    loadingTargetStructures,
    onChange,
    params,
    tab,
    targetMarketplaceStations,
  ]);

  return (
    <div className="bg-eve-panel border border-eve-border rounded-sm overflow-visible">
      {/* Header: collapse toggle + preset picker + help */}
      <div className="flex items-center justify-between gap-3 px-3 py-2 border-b border-eve-border/60 bg-eve-panel/80">
        <button
          onClick={toggleExpanded}
          className="flex items-center gap-2 text-left hover:bg-eve-accent/5 transition-colors rounded-sm px-1 -ml-1"
        >
          <span className="text-eve-accent text-sm">⚙</span>
          <span className="text-sm font-medium text-eve-text">
            {t("scanParameters")}
          </span>
          <span className="text-eve-dim text-xs">{expanded ? "▲" : "▼"}</span>
        </button>
        <div
          className="flex items-center gap-2"
          onClick={(e) => e.stopPropagation()}
        >
          <PresetPicker
            params={params}
            onApply={onChange}
            tab={tab}
            builtinPresets={getPresetsForTab(tab)}
            align="right"
          />
          {help && <TabHelp stepKeys={help.steps} wikiSlug={help.wiki} />}
        </div>
      </div>

      {expanded && (
        <div className="p-3 space-y-2">
          {tab === "region" ? (
            /* ══ REGION TAB: compact 3-card layout ══ */
            <>
              {/* Card 1 — Route */}
              <div className={`${sectionClass} p-2.5`}>
                <div className="text-[9px] uppercase tracking-widest text-eve-accent/70 font-bold mb-2 flex items-center gap-1.5">
                  <span className="text-eve-accent">⌁</span> Route
                </div>
                <div className="grid grid-cols-2 lg:grid-cols-3 xl:grid-cols-5 gap-2">
                  <Field label={t("sourceRegions")} hint={t("sourceRegionsHint")}>
                    <select
                      value={sourceRegionMode}
                      onChange={(e) => setSourceRegionMode(e.target.value as SourceRegionMode)}
                      className={inputClass}
                    >
                      <option value="major_hubs">{t("sourceRegionsMajorHubs")}</option>
                      <option value="radius">{t("sourceRegionsRadius")}</option>
                      <option value="single_region">{t("sourceRegionsSingle")}</option>
                    </select>
                  </Field>
                  {sourceRegionMode === "single_region" && (
                    <Field label={t("sourceRegionSingle")} hint={t("sourceRegionSingleHint")}>
                      <RegionAutocomplete
                        value={singleSourceRegion}
                        onChange={setSingleSourceRegion}
                        placeholder={t("targetRegionPlaceholder")}
                      />
                    </Field>
                  )}
                  <Field label={t("targetMarketplaceSystem")} hint={t("targetMarketplaceSystemHint")}>
                    <SystemAutocomplete
                      value={params.target_market_system ?? ""}
                      onChange={(v) => onChange({ ...params, target_market_system: v, target_market_location_id: 0 })}
                      showLocationButton={false}
                      isLoggedIn={isLoggedIn}
                      includeStructures={params.include_structures}
                      onIncludeStructuresChange={(v) =>
                        onChange({ ...params, include_structures: v, target_market_location_id: 0 })
                      }
                      extraActionSlots={1}
                      extraAction={
                        <SystemBlacklistButton
                          compact
                          value={params.ignored_system_ids ?? []}
                          onChange={(ids) => set("ignored_system_ids", ids)}
                        />
                      }
                    />
                  </Field>
                  <Field label={t("targetMarketplaceLocation")} hint={t("targetMarketplaceLocationHint")}>
                    {!targetMarketSystem ? (
                      <div className="h-[34px] flex items-center text-xs text-eve-dim">{t("selectTargetMarketplaceSystemFirst")}</div>
                    ) : loadingTargetStations || loadingTargetStructures ? (
                      <div className="h-[34px] flex items-center text-xs text-eve-dim">{t("loadingDestinations")}</div>
                    ) : targetMarketplaceStations.length === 0 ? (
                      <div className="h-[34px] flex items-center text-xs text-eve-dim">{t("noDestinationsFound")}</div>
                    ) : (
                      <select
                        value={String(params.target_market_location_id ?? 0)}
                        onChange={(e) => set("target_market_location_id", Number(e.target.value) || 0)}
                        className={inputClass}
                      >
                        <option value="0">{t("anyStationInSystem")}</option>
                        {targetMarketplaceStations.map((station) => (
                          <option key={station.id} value={station.id}>
                            {station.is_structure ? `[STR] ${station.name}` : station.name}
                          </option>
                        ))}
                      </select>
                    )}
                  </Field>
                  <Field label={t("paramsSecurity")} hint={t("routeSecurityHint")}>
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
                  <Field label={t("paramsCargo")}>
                    <NumberInput value={params.cargo_capacity} onChange={(v) => set("cargo_capacity", v)} min={0} max={CARGO_INPUT_MAX} />
                  </Field>
                </div>
              </div>

              {/* Card 2 — Profitability Filters */}
              <div className={`${sectionClass} p-2.5`}>
                <div className="text-[9px] uppercase tracking-widest text-eve-accent/70 font-bold mb-2 flex items-center gap-1.5">
                  <span className="text-eve-accent">◈</span> Filters
                </div>
                <div className="grid grid-cols-3 lg:grid-cols-7 gap-2">
                  <Field label={t("minItemProfit")}>
                    <NumberInput value={params.min_item_profit ?? 0} onChange={(v) => set("min_item_profit", v)} min={0} max={999999999999} />
                  </Field>
                  <Field label={t("minPeriodROI")} hint={t("minPeriodROIHint")}>
                    <NumberInput value={params.min_period_roi ?? 0} onChange={(v) => set("min_period_roi", v)} min={0} max={10000} step={0.1} />
                  </Field>
                  <Field label={t("maxDOS")} hint={t("maxDOSHint")}>
                    <NumberInput value={params.max_dos ?? 0} onChange={(v) => set("max_dos", v)} min={0} max={9999} step={0.5} />
                  </Field>
                  <Field label={t("minDemandPerDay")} hint={t("minDemandPerDayHint")}>
                    <NumberInput value={params.min_demand_per_day ?? 0} onChange={(v) => set("min_demand_per_day", v)} min={0} max={999999} step={0.5} />
                  </Field>
                  <Field label={t("purchaseDemandDays")} hint={t("purchaseDemandDaysHint")}>
                    <NumberInput value={params.purchase_demand_days ?? 0.5} onChange={(v) => set("purchase_demand_days", v)} min={0} max={30} step={0.1} />
                  </Field>
                  <Field label={t("avgPricePeriod")}>
                    <NumberInput value={params.avg_price_period ?? 14} onChange={(v) => set("avg_price_period", Math.round(v))} min={1} max={365} />
                  </Field>
                  <Field label={t("minOrderMargin")}>
                    <NumberInput value={params.min_margin} onChange={(v) => set("min_margin", v)} min={0.1} max={1000} step={0.1} />
                  </Field>
                </div>
                {/* ── Revenue mode toggle ── */}
                <div className="mt-2.5 pt-2 border-t border-eve-border/40 flex items-center gap-2">
                  <span className="text-[9px] uppercase tracking-widest text-eve-dim font-bold shrink-0">
                    Revenue Mode
                  </span>
                  <div className="flex items-center rounded-sm border border-eve-border overflow-hidden text-[11px] font-medium">
                    <button
                      type="button"
                      onClick={() => set("sell_order_mode", false)}
                      className={`px-3 py-1 transition-colors ${
                        !params.sell_order_mode
                          ? "bg-eve-accent/20 text-eve-accent border-r border-eve-border"
                          : "text-eve-dim hover:text-eve-light border-r border-eve-border"
                      }`}
                    >
                      ⚡ Instant
                    </button>
                    <button
                      type="button"
                      title={t("sellOrderModeHint")}
                      onClick={() => set("sell_order_mode", true)}
                      className={`px-3 py-1 transition-colors ${
                        params.sell_order_mode
                          ? "bg-amber-400/15 text-amber-300"
                          : "text-eve-dim hover:text-eve-light"
                      }`}
                    >
                      📋 Sell Order
                    </button>
                  </div>
                  {params.sell_order_mode && (
                    <span className="text-[10px] text-amber-300/70 italic">
                      Revenue = lowest ask at destination
                    </span>
                  )}
                </div>
              </div>

              {/* Card 3 — Fees (compact inline bar) */}
              <div className={`${sectionClass} p-2.5`}>
                <div className="flex flex-wrap items-end gap-x-5 gap-y-2">
                  <span className="text-[9px] uppercase tracking-widest text-eve-accent/70 font-bold self-center shrink-0">
                    ∑ Fees
                  </span>
                  {!splitTradeFees ? (
                    <>
                      <div className="flex items-center gap-1.5">
                        <span className="text-[10px] text-eve-dim shrink-0">{t("paramsTax")}</span>
                        <div className="w-20">
                          <NumberInput value={params.sales_tax_percent} onChange={setLegacySalesTax} min={0} max={100} step={0.1} />
                        </div>
                      </div>
                      <div className="flex items-center gap-1.5">
                        <span className="text-[10px] text-eve-dim shrink-0">{t("paramsBrokerFee")}</span>
                        <div className="w-20">
                          <NumberInput value={params.broker_fee_percent} onChange={setLegacyBrokerFee} min={0} max={10} step={0.1} />
                        </div>
                      </div>
                    </>
                  ) : (
                    <>
                      <div className="flex items-center gap-1.5">
                        <span className="text-[10px] text-eve-dim shrink-0">{t("paramsBuyTax")}</span>
                        <div className="w-16">
                          <NumberInput value={params.buy_sales_tax_percent ?? 0} onChange={(v) => set("buy_sales_tax_percent", v)} min={0} max={100} step={0.1} />
                        </div>
                      </div>
                      <div className="flex items-center gap-1.5">
                        <span className="text-[10px] text-eve-dim shrink-0">{t("paramsSellTax")}</span>
                        <div className="w-16">
                          <NumberInput value={params.sell_sales_tax_percent ?? params.sales_tax_percent} onChange={(v) => set("sell_sales_tax_percent", v)} min={0} max={100} step={0.1} />
                        </div>
                      </div>
                      <div className="flex items-center gap-1.5">
                        <span className="text-[10px] text-eve-dim shrink-0">{t("paramsBuyBrokerFee")}</span>
                        <div className="w-16">
                          <NumberInput value={params.buy_broker_fee_percent ?? params.broker_fee_percent} onChange={(v) => set("buy_broker_fee_percent", v)} min={0} max={10} step={0.1} />
                        </div>
                      </div>
                      <div className="flex items-center gap-1.5">
                        <span className="text-[10px] text-eve-dim shrink-0">{t("paramsSellBrokerFee")}</span>
                        <div className="w-16">
                          <NumberInput value={params.sell_broker_fee_percent ?? params.broker_fee_percent} onChange={(v) => set("sell_broker_fee_percent", v)} min={0} max={10} step={0.1} />
                        </div>
                      </div>
                    </>
                  )}
                  <button
                    type="button"
                    disabled={!isLoggedIn || esiSkillsLoading}
                    onClick={fetchSkillsFromESI}
                    title={isLoggedIn ? "Auto-fill fees from Accounting + Broker Relations skill levels" : "Login via ESI to use this feature"}
                    className="flex items-center gap-1 px-2 py-1 rounded-sm text-[11px] border border-eve-accent/40 text-eve-accent bg-eve-accent/10 hover:bg-eve-accent/20 disabled:opacity-40 disabled:cursor-not-allowed transition-colors shrink-0"
                  >
                    {esiSkillsLoading ? <span className="animate-pulse">⟳</span> : "⚡"}
                    {esiSkillsLoading ? "Loading…" : "ESI Skills"}
                  </button>
                  <label className="flex items-center gap-1.5 cursor-pointer select-none ml-auto shrink-0">
                    <span className="text-[10px] text-eve-dim">{t("splitTradeFees")}</span>
                    <input
                      type="checkbox"
                      checked={splitTradeFees}
                      onChange={(e) => setSplitFees(e.target.checked)}
                      className="accent-eve-accent"
                    />
                  </label>
                </div>
                {esiSkillsMsg && (
                  <div className={`mt-1.5 text-[11px] font-mono ${esiSkillsMsg.startsWith("✓") ? "text-green-300" : "text-red-400"}`}>
                    {esiSkillsMsg}
                  </div>
                )}
              </div>

              {/* Advanced — region */}
              <section className={`${sectionClass} p-2.5`}>
                <button
                  type="button"
                  onClick={() => setShowAdvanced((a) => !a)}
                  className="w-full flex items-center justify-between gap-3 text-[11px] uppercase tracking-wider text-eve-dim hover:text-eve-accent font-medium transition-colors"
                >
                  <span className="flex items-center gap-1.5">
                    <span className={`transition-transform ${showAdvanced ? "rotate-90" : ""}`}>▸</span>
                    {t("advancedFilters")}
                  </span>
                  {activeAdvancedCount > 0 && (
                    <span className="px-1.5 py-0.5 rounded-sm border border-eve-accent/40 text-eve-accent text-[10px] font-mono">
                      {activeAdvancedCount}
                    </span>
                  )}
                </button>
                {showAdvanced && (
                  <div className="mt-2.5 pt-2.5 border-t border-eve-border/50 space-y-2">
                    <div className="grid grid-cols-3 gap-2">
                      <Field label={t("minDailyVolume")}>
                        <NumberInput value={params.min_daily_volume ?? 0} onChange={(v) => set("min_daily_volume", v)} min={0} max={999999999} />
                      </Field>
                      <Field label={t("maxInvestment")}>
                        <NumberInput value={params.max_investment ?? 0} onChange={(v) => set("max_investment", v)} min={0} max={999999999999} />
                      </Field>
                      <Field label="Shipping ISK/(m³·j)">
                        <NumberInput value={params.shipping_cost_per_m3_jump ?? 0} onChange={(v) => set("shipping_cost_per_m3_jump", v)} min={0} max={1000000} step={0.1} />
                      </Field>
                    </div>
                    <div className="grid grid-cols-2 lg:grid-cols-4 gap-2">
                      <Field label={t("minS2BPerDay")} hint={t("minS2BPerDayHint")}>
                        <NumberInput value={params.min_s2b_per_day ?? 0} onChange={(v) => set("min_s2b_per_day", v)} min={0} max={999999999} step={0.1} />
                      </Field>
                      <Field label={t("minBfSPerDay")} hint={t("minBfSPerDayHint")}>
                        <NumberInput value={params.min_bfs_per_day ?? 0} onChange={(v) => set("min_bfs_per_day", v)} min={0} max={999999999} step={0.1} />
                      </Field>
                      <Field label={t("minS2BBfSRatio")} hint={t("minS2BBfSRatioHint")}>
                        <NumberInput value={params.min_s2b_bfs_ratio ?? 0} onChange={(v) => set("min_s2b_bfs_ratio", v)} min={0} max={999999} step={0.1} />
                      </Field>
                      <Field label={t("maxS2BBfSRatio")} hint={t("maxS2BBfSRatioHint")}>
                        <NumberInput value={params.max_s2b_bfs_ratio ?? 0} onChange={(v) => set("max_s2b_bfs_ratio", v)} min={0} max={999999} step={0.1} />
                      </Field>
                    </div>
                    {/* ── Category filter ── */}
                    <div className="space-y-1.5">
                      <div className="flex items-center justify-between">
                        <span className="text-[10px] uppercase tracking-wider text-eve-dim font-medium">
                          {t("categoryFilter")}
                        </span>
                        {(params.category_ids ?? []).length > 0 && (
                          <button
                            type="button"
                            onClick={() => set("category_ids", [])}
                            className="text-[10px] text-eve-dim hover:text-eve-accent transition-colors"
                          >
                            {t("categoryFilterClear")}
                          </button>
                        )}
                      </div>
                      <div className="flex flex-wrap gap-1.5">
                        {EVE_CATEGORIES.map((cat) => {
                          const active = (params.category_ids ?? []).includes(cat.id);
                          return (
                            <button
                              key={cat.id}
                              type="button"
                              title={cat.hint}
                              onClick={() => {
                                const current = params.category_ids ?? [];
                                set(
                                  "category_ids",
                                  active
                                    ? current.filter((id) => id !== cat.id)
                                    : [...current, cat.id],
                                );
                              }}
                              className={`px-2 py-0.5 rounded-sm text-[11px] font-medium border transition-colors ${
                                active
                                  ? "bg-eve-accent/20 border-eve-accent text-eve-accent"
                                  : "border-eve-border text-eve-dim hover:border-eve-accent/50 hover:text-eve-light"
                              }`}
                            >
                              {cat.label}
                            </button>
                          );
                        })}
                      </div>
                    </div>
                  </div>
                )}
              </section>
            </>
          ) : (
            /* ══ NON-REGION TABS: original layout ══ */
            <>
          {/* Main sections */}
          <div className="grid grid-cols-1 xl:grid-cols-12 gap-3">
            <section className={`${sectionClass} xl:col-span-8 p-3`}>
              <SectionHeader
                title={t("system")}
                subtitle={t("paramsMargin")}
                icon="⌁"
              />

              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-x-4 gap-y-3 mt-2">
                <Field label={t("system")}>
                  <SystemAutocomplete
                    value={params.system_name}
                    onChange={(v) => set("system_name", v)}
                    isLoggedIn={isLoggedIn}
                    includeStructures={params.include_structures}
                    onIncludeStructuresChange={(v) => set("include_structures", v)}
                    extraActionSlots={1}
                    extraAction={
                      <SystemBlacklistButton
                        compact
                        value={params.ignored_system_ids ?? []}
                        onChange={(ids) => set("ignored_system_ids", ids)}
                      />
                    }
                  />
                </Field>

                {showCargoInMain && (
                  <Field label={t("paramsCargo")}>
                    <NumberInput
                      value={params.cargo_capacity}
                      onChange={(v) => set("cargo_capacity", v)}
                      min={0}
                      max={CARGO_INPUT_MAX}
                    />
                  </Field>
                )}

                {showBuyRadius && (
                  <Field label={t("paramsBuy")}>
                    <NumberInput
                      value={params.buy_radius}
                      onChange={(v) => set("buy_radius", v)}
                      min={1}
                      max={50}
                    />
                  </Field>
                )}

                {!hideSellRadius && (
                  <Field label={t("paramsSell")}>
                    <NumberInput
                      value={params.sell_radius}
                      onChange={(v) => set("sell_radius", v)}
                      min={1}
                      max={50}
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
              </div>
            </section>

            <section className={`${sectionClass} xl:col-span-4 p-3`}>
              <SectionHeader
                title={t("splitTradeFees")}
                subtitle={t("splitTradeFeesHint")}
                icon="∑"
              />

              <label className="mt-2 h-[34px] px-2.5 py-1.5 bg-eve-input border border-eve-border rounded text-eve-text text-sm flex items-center justify-between">
                <span className="text-eve-dim text-xs">
                  {t("splitTradeFeesHint")}
                </span>
                <input
                  type="checkbox"
                  checked={splitTradeFees}
                  onChange={(e) => setSplitFees(e.target.checked)}
                  className="accent-eve-accent"
                />
              </label>

              {!splitTradeFees && (
                <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-1 gap-3 mt-3">
                  <Field label={t("paramsTax")}>
                    <NumberInput
                      value={params.sales_tax_percent}
                      onChange={setLegacySalesTax}
                      min={0}
                      max={100}
                      step={0.1}
                    />
                  </Field>

                  <Field label={t("paramsBrokerFee")}>
                    <NumberInput
                      value={params.broker_fee_percent}
                      onChange={setLegacyBrokerFee}
                      min={0}
                      max={10}
                      step={0.1}
                    />
                  </Field>
                </div>
              )}

              {splitTradeFees && (
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mt-3">
                  <Field label={t("paramsBuyTax")}>
                    <NumberInput
                      value={params.buy_sales_tax_percent ?? 0}
                      onChange={(v) => set("buy_sales_tax_percent", v)}
                      min={0}
                      max={100}
                      step={0.1}
                    />
                  </Field>
                  <Field label={t("paramsSellTax")}>
                    <NumberInput
                      value={
                        params.sell_sales_tax_percent ?? params.sales_tax_percent
                      }
                      onChange={(v) => set("sell_sales_tax_percent", v)}
                      min={0}
                      max={100}
                      step={0.1}
                    />
                  </Field>
                  <Field label={t("paramsBuyBrokerFee")}>
                    <NumberInput
                      value={
                        params.buy_broker_fee_percent ??
                        params.broker_fee_percent
                      }
                      onChange={(v) => set("buy_broker_fee_percent", v)}
                      min={0}
                      max={10}
                      step={0.1}
                    />
                  </Field>
                  <Field label={t("paramsSellBrokerFee")}>
                    <NumberInput
                      value={
                        params.sell_broker_fee_percent ??
                        params.broker_fee_percent
                      }
                      onChange={(v) => set("sell_broker_fee_percent", v)}
                      min={0}
                      max={10}
                      step={0.1}
                    />
                  </Field>
                </div>
              )}

              {/* ESI skills autofill */}
              <div className="mt-3 flex flex-col gap-1">
                <div className="flex items-center gap-2">
                  <button
                    type="button"
                    disabled={!isLoggedIn || esiSkillsLoading}
                    onClick={fetchSkillsFromESI}
                    title={isLoggedIn ? "Auto-fill fees from ESI (Accounting + Broker Relations skill levels)" : "Login via ESI to use this feature"}
                    className="flex items-center gap-1.5 px-2 py-1 rounded-sm text-[11px] border border-eve-accent/40 text-eve-accent bg-eve-accent/10 hover:bg-eve-accent/20 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
                  >
                    {esiSkillsLoading ? (
                      <span className="animate-pulse">⟳</span>
                    ) : (
                      <span>⚡</span>
                    )}
                    {esiSkillsLoading ? "Loading…" : "Fetch from ESI"}
                  </button>
                  <span className="text-[10px] text-eve-dim">
                    Accounting + Broker Relations skills
                  </span>
                </div>
                {esiSkillsMsg && (
                  <span
                    className={`text-[11px] font-mono ${esiSkillsMsg.startsWith("✓") ? "text-green-300" : "text-red-400"}`}
                  >
                    {esiSkillsMsg}
                  </span>
                )}
              </div>
            </section>
          </div>

          {/* Advanced filters */}
          <section className={`${sectionClass} p-3`}>
            <button
              type="button"
              onClick={() => setShowAdvanced((a) => !a)}
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
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-x-4 gap-y-3 mt-3 pt-3 border-t border-eve-border/50">
                <Field label={t("paramsSecurity")}>
                  <select
                    value={String(params.min_route_security ?? 0)}
                    onChange={(e) =>
                      set("min_route_security", parseFloat(e.target.value))
                    }
                    className={inputClass}
                  >
                    <option value="0">{t("routeSecurityAll")}</option>
                    <option value="0.45">{t("routeSecurityHighsec")}</option>
                    <option value="0.5">{t("routeSecurityMin05")}</option>
                    <option value="0.7">{t("routeSecurityMin07")}</option>
                  </select>
                </Field>

                {isFlowTab && (
                  <>
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

                    <Field
                      label={t("minS2BPerDay")}
                      hint={t("minS2BPerDayHint")}
                    >
                      <NumberInput
                        value={params.min_s2b_per_day ?? 0}
                        onChange={(v) => set("min_s2b_per_day", v)}
                        min={0}
                        max={999999999}
                        step={0.1}
                      />
                    </Field>

                    <Field
                      label={t("minBfSPerDay")}
                      hint={t("minBfSPerDayHint")}
                    >
                      <NumberInput
                        value={params.min_bfs_per_day ?? 0}
                        onChange={(v) => set("min_bfs_per_day", v)}
                        min={0}
                        max={999999999}
                        step={0.1}
                      />
                    </Field>

                    <Field
                      label={t("minS2BBfSRatio")}
                      hint={t("minS2BBfSRatioHint")}
                    >
                      <NumberInput
                        value={params.min_s2b_bfs_ratio ?? 0}
                        onChange={(v) => set("min_s2b_bfs_ratio", v)}
                        min={0}
                        max={999999}
                        step={0.1}
                      />
                    </Field>

                    <Field
                      label={t("maxS2BBfSRatio")}
                      hint={t("maxS2BBfSRatioHint")}
                    >
                      <NumberInput
                        value={params.max_s2b_bfs_ratio ?? 0}
                        onChange={(v) => set("max_s2b_bfs_ratio", v)}
                        min={0}
                        max={999999}
                        step={0.1}
                      />
                    </Field>
                  </>
                )}
              </div>
            )}
          </section>
            </>
          )}
        </div>
      )}
    </div>
  );
}

function SectionHeader({
  title,
  subtitle,
  icon,
}: {
  title: string;
  subtitle?: string;
  icon?: string;
}) {
  return (
    <div className="flex items-center justify-between gap-3 border-b border-eve-border/40 pb-2">
      <div className="flex items-center gap-2 min-w-0">
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
      <label
        className="flex items-center gap-1 text-[10px] uppercase tracking-wider text-eve-dim font-medium truncate"
        title={hint}
      >
        {label}
        {hint && (
          <span
            className="text-eve-dim/60 hover:text-eve-accent cursor-help"
            title={hint}
          >
            <svg
              className="w-3 h-3"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
            >
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
