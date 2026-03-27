import { useEffect, useRef, useState, type ReactNode } from "react";
import { getExecutionPlan, getStations, getStructures, type CharacterScope } from "../../lib/api";
import { formatISK } from "../../lib/format";
import { useI18n } from "../../lib/i18n";
import type { PLEXDashboard, PLEXGlobalPrice, StationInfo } from "../../lib/types";
import { SystemAutocomplete } from "../SystemAutocomplete";

const HOURS_PER_MONTH = 24 * 30;
const SP_PER_EXTRACTOR = 500_000;
const SCC_SURCHARGE_PCT = 0.5;
const EXTRACTABLE_SP_THRESHOLD = 5_500_000;
const JITA_REGION_ID = 10000002;
const JITA_STATION_ID = 60003760;

const IMPLANT_SETS = {
  0: [],
  1: [
    { typeID: 13283, name: "Limited Ocular Filter" },
    { typeID: 13284, name: "Limited Memory Augmentation" },
    { typeID: 13285, name: "Limited Neural Boost" },
    { typeID: 13287, name: "Limited Cybernetic Subprocessor" },
    { typeID: 13286, name: "Limited Social Adaptation Chip" },
  ],
  2: [
    { typeID: 14295, name: "Limited Ocular Filter - Beta" },
    { typeID: 14297, name: "Limited Memory Augmentation - Beta" },
    { typeID: 14296, name: "Limited Neural Boost - Beta" },
    { typeID: 14298, name: "Limited Cybernetic Subprocessor - Beta" },
    { typeID: 14299, name: "Limited Social Adaptation Chip - Beta" },
  ],
  3: [
    { typeID: 9899, name: "Ocular Filter - Basic" },
    { typeID: 9941, name: "Memory Augmentation - Basic" },
    { typeID: 9942, name: "Neural Boost - Basic" },
    { typeID: 10224, name: "Cybernetic Subprocessor - Basic" },
    { typeID: 9956, name: "Social Adaptation Chip - Basic" },
  ],
  4: [
    { typeID: 10216, name: "Ocular Filter - Standard" },
    { typeID: 10208, name: "Memory Augmentation - Standard" },
    { typeID: 10212, name: "Neural Boost - Standard" },
    { typeID: 10221, name: "Cybernetic Subprocessor - Standard" },
    { typeID: 10225, name: "Social Adaptation Chip - Standard" },
  ],
  5: [
    { typeID: 10217, name: "Ocular Filter - Improved" },
    { typeID: 10209, name: "Memory Augmentation - Improved" },
    { typeID: 10213, name: "Neural Boost - Improved" },
    { typeID: 10222, name: "Cybernetic Subprocessor - Improved" },
    { typeID: 10226, name: "Social Adaptation Chip - Improved" },
  ],
} as const;

const TRAINING_PROFILES = ([
  { idPrefix: "omega_20_20", primary: 20, secondary: 20, attrs: "20 / 20" },
  { idPrefix: "omega_27_21", primary: 27, secondary: 21, attrs: "27 / 21" },
] as const).flatMap((base) =>
  ([0, 1, 2, 3, 4, 5] as const).map((implantBonus) => {
    const primaryWithImplants = base.primary + implantBonus;
    const secondaryWithImplants = base.secondary + implantBonus;
    const spPerMinute = primaryWithImplants + 0.5 * secondaryWithImplants;
    return {
      id: `${base.idPrefix}_${implantBonus === 0 ? "none" : `plus${implantBonus}`}`,
      label: `Omega (${base.attrs}, ${implantBonus === 0 ? "None" : `+${implantBonus}`})`,
      cloneType: "Omega" as const,
      implants: implantBonus === 0 ? "None" : `+${implantBonus}`,
      implantBonus,
      spPerMinute,
      spPerHour: spPerMinute * 60,
      attrs: base.attrs,
    };
  }),
);

const EXTRACTOR_PACKS = [
  { id: "single", label: "1x Skill Extractor", totalPlex: 120, units: 1 },
  { id: "ten", label: "10x Skill Extractor", totalPlex: 1200, units: 10 },
  { id: "fifty", label: "50x Skill Extractor", totalPlex: 5600, units: 50 },
] as const;

const OMEGA_PLANS = [
  { id: "1m", label: "1 Month Omega", totalPlex: 500, months: 1 },
  { id: "3m", label: "3 Months Omega", totalPlex: 1200, months: 3 },
  { id: "6m", label: "6 Months Omega", totalPlex: 2100, months: 6 },
  { id: "12m", label: "12 Months Omega", totalPlex: 3600, months: 12 },
  { id: "24m", label: "24 Months Omega", totalPlex: 6600, months: 24 },
] as const;

const MCT_PLANS = [
  { id: "1x", label: "1x MCT", totalPlex: 350, units: 1 },
  { id: "2x", label: "2x MCT", totalPlex: 600, units: 2 },
  { id: "3x", label: "3x MCT", totalPlex: 800, units: 3 },
  { id: "6x", label: "6x MCT", totalPlex: 1500, units: 6 },
  { id: "12x", label: "12x MCT", totalPlex: 2700, units: 12 },
  { id: "24x", label: "24x MCT", totalPlex: 4600, units: 24 },
] as const;

type ExtractorSource = "market_buy" | "market_structure_buy" | "market_immediate" | "nes";
type InjectorMode = "sell_order" | "instant" | "custom_sell";
type PlexSource = "spot" | "buy_order" | "structure_buy";

interface InjectorDestination {
  id: string;
  systemName: string;
  systemID: number;
  regionID: number;
  includeStructures: boolean;
  locationID: number;
  locationName: string;
  percentage: number;
  sellPrice: number;
}

interface ImplantQuote {
  typeID: number;
  name: string;
  price: number;
}

interface FleetTrainingRow {
  chars: number;
  dirty: boolean;
}

function applyDiscount(base: number, discountPct: number) {
  return Math.max(0, base * (1 - discountPct / 100));
}

function describeOffer(totalPlex: number, unitCount: number, suffix: string, template: string) {
  const perUnit = unitCount > 0 ? totalPlex / unitCount : totalPlex;
  return template
    .replace("{total}", totalPlex.toFixed(0))
    .replace("{perUnit}", perUnit.toFixed(2))
    .replace("{suffix}", suffix);
}

function effectiveOfferTotal(defaultTotalPlex: number, discountPct: number, manualTotalPlex: number) {
  if (manualTotalPlex > 0) return manualTotalPlex;
  return applyDiscount(defaultTotalPlex, discountPct);
}

function createInjectorDestination(defaultToJita = false): InjectorDestination {
  return {
    id: Math.random().toString(36).slice(2, 10),
    systemName: defaultToJita ? "Jita" : "",
    systemID: 0,
    regionID: 0,
    includeStructures: false,
    locationID: 0,
    locationName: "",
    percentage: defaultToJita ? 100 : 0,
    sellPrice: 0,
  };
}

function orderFeeAmount(basePrice: number, pct: number) {
  return basePrice * (pct / 100);
}

function InfoPopover({ label, children }: { label: string; children: ReactNode }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="relative inline-flex items-center ml-1">
      <button
        type="button"
        onClick={() => setOpen((value) => !value)}
        onBlur={() => window.setTimeout(() => setOpen(false), 120)}
        className="w-4 h-4 rounded-full border border-eve-border text-[10px] text-eve-dim hover:text-eve-text hover:border-eve-accent/50"
        aria-label={label}
        title={label}
      >
        i
      </button>
      {open && (
        <div className="absolute left-0 top-5 z-20 w-64 rounded-sm border border-eve-border bg-eve-panel shadow-lg p-2 text-[11px] text-eve-text">
          {children}
        </div>
      )}
    </div>
  );
}

function InjectorDestinationRow({
  value,
  onChange,
  onRemove,
  canRemove,
  isLoggedIn,
  activeCharacterId,
  maxPercentage,
}: {
  value: InjectorDestination;
  onChange: (next: InjectorDestination) => void;
  onRemove: () => void;
  canRemove: boolean;
  isLoggedIn: boolean;
  activeCharacterId?: CharacterScope;
  maxPercentage: number;
}) {
  const { t } = useI18n();
  const [stations, setStations] = useState<StationInfo[]>([]);
  const [structures, setStructures] = useState<StationInfo[]>([]);
  const [loadingStations, setLoadingStations] = useState(false);
  const [loadingStructures, setLoadingStructures] = useState(false);
  const [loadingPrice, setLoadingPrice] = useState(false);
  const latestValueRef = useRef(value);

  useEffect(() => {
    latestValueRef.current = value;
  }, [value]);

  const mergeValue = (patch: Partial<InjectorDestination>) => {
    const next = { ...latestValueRef.current, ...patch };
    latestValueRef.current = next;
    onChange(next);
  };

  useEffect(() => {
    if (!value.systemName.trim()) {
      setStations([]);
      setStructures([]);
      return;
    }

    const controller = new AbortController();
    setLoadingStations(true);
    getStations(value.systemName, controller.signal)
      .then((resp) => {
        if (controller.signal.aborted) return;
        setStations(resp.stations);
        const current = latestValueRef.current;
        mergeValue({
          systemID: resp.system_id,
          regionID: resp.region_id,
          locationID: resp.stations.some((entry) => entry.id === current.locationID) ? current.locationID : 0,
          locationName: resp.stations.some((entry) => entry.id === current.locationID) ? current.locationName : "",
        });
      })
      .catch(() => {
        if (controller.signal.aborted) return;
        setStations([]);
        setStructures([]);
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoadingStations(false);
      });

    return () => controller.abort();
  }, [value.systemName]);

  useEffect(() => {
    if (!isLoggedIn || !value.includeStructures || value.systemID <= 0 || value.regionID <= 0) {
      setStructures([]);
      setLoadingStructures(false);
      return;
    }

    const controller = new AbortController();
    setLoadingStructures(true);
    getStructures(value.systemID, value.regionID, controller.signal, activeCharacterId)
      .then((resp) => {
        if (controller.signal.aborted) return;
        setStructures(resp);
      })
      .catch(() => {
        if (controller.signal.aborted) return;
        setStructures([]);
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoadingStructures(false);
      });

    return () => controller.abort();
  }, [activeCharacterId, isLoggedIn, value.includeStructures, value.regionID, value.systemID]);

  useEffect(() => {
    if (value.regionID <= 0 || (value.locationID === 0 && value.systemID <= 0)) {
      setLoadingPrice(false);
      return;
    }

    const controller = new AbortController();
    setLoadingPrice(true);
    getExecutionPlan({
      type_id: 40520,
      region_id: value.regionID,
      system_id: value.locationID === 0 ? value.systemID || undefined : undefined,
      location_id: value.locationID || undefined,
      quantity: 1,
      is_buy: true,
      character_id: activeCharacterId,
      signal: controller.signal,
    })
      .then((plan) => {
        if (controller.signal.aborted) return;
        mergeValue({ sellPrice: plan.best_price || plan.expected_price || 0 });
      })
      .catch(() => {
        if (controller.signal.aborted) return;
        mergeValue({ sellPrice: 0 });
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoadingPrice(false);
      });
    return () => controller.abort();
  }, [activeCharacterId, value.locationID, value.regionID, value.systemID]);

  const locationOptions = value.includeStructures && isLoggedIn ? [...stations, ...structures] : stations;
  locationOptions.sort((left, right) => left.name.localeCompare(right.name));

  return (
    <div className="grid grid-cols-1 lg:grid-cols-[minmax(180px,1.15fr)_minmax(240px,1fr)_92px_132px_auto] gap-2 items-end border border-eve-border/40 rounded-sm p-2">
      <div className="flex flex-col gap-1">
        <span className="text-[11px] text-eve-dim">{t("plexSpfarmSystem")}</span>
        <SystemAutocomplete
          value={value.systemName}
          onChange={(nextSystem) => onChange({ ...value, systemName: nextSystem, locationID: 0, locationName: "", sellPrice: 0 })}
          showLocationButton={false}
          isLoggedIn={isLoggedIn}
          includeStructures={value.includeStructures}
          onIncludeStructuresChange={(nextInclude) => onChange({ ...value, includeStructures: nextInclude, locationID: 0, locationName: "", sellPrice: 0 })}
        />
      </div>
      <label className="flex flex-col gap-1">
        <span className="text-[11px] text-eve-dim">{t("plexSpfarmStationStructure")}</span>
        <select
          value={String(value.locationID || 0)}
          onChange={(e) => {
            const nextId = Number(e.target.value) || 0;
            const nextLocation = locationOptions.find((entry) => entry.id === nextId);
            onChange({
              ...value,
              locationID: nextId,
              locationName: nextLocation?.name ?? "",
            });
          }}
          className="px-2 py-1 bg-eve-input border border-eve-border rounded-sm text-xs text-eve-text"
        >
          <option value="0">
            {loadingStations || loadingStructures ? t("plexLoading") : t("plexSpfarmAnyStationInSystem")}
          </option>
          {locationOptions.map((location) => (
            <option key={location.id} value={location.id}>
              {location.is_structure ? `[STR] ${location.name}` : location.name}
            </option>
          ))}
        </select>
      </label>
      <label className="flex flex-col gap-1" title={t("plexSpfarmOutputPercentHint")}>
        <span className="text-[11px] text-eve-dim">{t("plexSpfarmOutputPercent")}</span>
        <input
          type="number"
          min="0"
          max={maxPercentage}
          step="1"
          value={value.percentage}
          onChange={(e) => onChange({ ...value, percentage: Math.min(maxPercentage, Math.max(0, parseInt(e.target.value, 10) || 0)) })}
          className="px-2 py-1 bg-eve-input border border-eve-border rounded-sm text-xs text-eve-text font-mono"
        />
      </label>
      <label className="flex flex-col gap-1">
        <span className="text-[11px] text-eve-dim">{t("plexSpfarmLiveSellPrice")}</span>
        <div className="px-2 py-1 bg-eve-panel border border-eve-border rounded-sm text-xs text-eve-text font-mono min-h-[34px] flex items-center">
          {loadingPrice ? t("plexLoading") : value.sellPrice > 0 ? formatISK(value.sellPrice) : t("plexNoData")}
        </div>
      </label>
      <button
        type="button"
        onClick={onRemove}
        disabled={!canRemove}
        className="px-3 py-1 rounded-sm border border-eve-border text-xs uppercase tracking-wider text-eve-dim hover:text-eve-text disabled:opacity-40"
      >
        {t("plexSpfarmRemove")}
      </button>
    </div>
  );
}

export function SPFarmCard({
  farm,
  plexPrice,
  salesTax,
  brokerFee,
  isLoggedIn,
  activeCharacterId,
}: {
  farm: PLEXDashboard["sp_farm"];
  plexPrice: PLEXGlobalPrice;
  salesTax: number;
  brokerFee: number;
  isLoggedIn: boolean;
  activeCharacterId?: CharacterScope;
}) {
  const { t } = useI18n();
  const [profileId, setProfileId] = useState<(typeof TRAINING_PROFILES)[number]["id"]>("omega_27_21_plus5");
  const [extractorSource, setExtractorSource] = useState<ExtractorSource>("market_buy");
  const [injectorMode, setInjectorMode] = useState<InjectorMode>("sell_order");
  const [plexSource, setPlexSource] = useState<PlexSource>("spot");
  const [accounts, setAccounts] = useState(1);
  const [trainingCharsPerAccount, setTrainingCharsPerAccount] = useState(3);
  const [extractorPackId, setExtractorPackId] = useState<(typeof EXTRACTOR_PACKS)[number]["id"]>("fifty");
  const [omegaPlanId, setOmegaPlanId] = useState<(typeof OMEGA_PLANS)[number]["id"]>("12m");
  const [mctPlanId, setMctPlanId] = useState<(typeof MCT_PLANS)[number]["id"]>("24x");
  const [extractorDiscountPct, setExtractorDiscountPct] = useState(0);
  const [omegaDiscountPct, setOmegaDiscountPct] = useState(0);
  const [mctDiscountPct, setMctDiscountPct] = useState(0);
  const [manualExtractorPlex, setManualExtractorPlex] = useState(0);
  const [manualOmegaPlex, setManualOmegaPlex] = useState(0);
  const [manualMctPlex, setManualMctPlex] = useState(0);
  const [injectorDestinations, setInjectorDestinations] = useState<InjectorDestination[]>([createInjectorDestination(true)]);
  const [fleetTrainingChars, setFleetTrainingChars] = useState<FleetTrainingRow[]>([{ chars: 3, dirty: false }]);
  const [implantQuotes, setImplantQuotes] = useState<ImplantQuote[]>([]);
  const [loadingImplantQuotes, setLoadingImplantQuotes] = useState(false);

  useEffect(() => {
    setFleetTrainingChars(Array.from({ length: accounts }, () => ({ chars: trainingCharsPerAccount, dirty: false })));
  }, [accounts]);

  useEffect(() => {
    setFleetTrainingChars((prev) => prev.map((row) => (row.dirty ? row : { chars: trainingCharsPerAccount, dirty: false })));
  }, [trainingCharsPerAccount]);

  const profile = TRAINING_PROFILES.find((entry) => entry.id === profileId) ?? TRAINING_PROFILES[0];
  const extractorPack = EXTRACTOR_PACKS.find((entry) => entry.id === extractorPackId) ?? EXTRACTOR_PACKS[0];
  const omegaPlan = OMEGA_PLANS.find((entry) => entry.id === omegaPlanId) ?? OMEGA_PLANS[0];
  const mctPlan = MCT_PLANS.find((entry) => entry.id === mctPlanId) ?? MCT_PLANS[0];

  const formatProfileLabel = (attrs: string, implants: string) =>
    t("plexSpfarmTrainingProfileOption", { attrs, implant: implants === "None" ? t("plexSpfarmNone") : implants });
  const formatExtractorPackLabel = (units: number) => t("plexSpfarmExtractorPackOption", { count: units });
  const formatOmegaPlanLabel = (months: number) => t("plexSpfarmOmegaPlanOption", { months });
  const formatMctPlanLabel = (units: number) => t("plexSpfarmMctPlanOption", { count: units });

  useEffect(() => {
    const implants = IMPLANT_SETS[profile.implantBonus as keyof typeof IMPLANT_SETS] ?? [];
    if (implants.length === 0) {
      setImplantQuotes([]);
      setLoadingImplantQuotes(false);
      return;
    }

    const controller = new AbortController();
    setLoadingImplantQuotes(true);
    Promise.all(
      implants.map(async (implant) => {
        const plan = await getExecutionPlan({
          type_id: implant.typeID,
          region_id: JITA_REGION_ID,
          location_id: JITA_STATION_ID,
          quantity: 1,
          is_buy: true,
          signal: controller.signal,
        });
        return {
          typeID: implant.typeID,
          name: implant.name,
          price: plan.best_price || plan.expected_price || 0,
        };
      }),
    )
      .then((quotes) => {
        if (controller.signal.aborted) return;
        setImplantQuotes(quotes);
      })
      .catch(() => {
        if (controller.signal.aborted) return;
        setImplantQuotes(implants.map((implant) => ({ typeID: implant.typeID, name: implant.name, price: 0 })));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoadingImplantQuotes(false);
      });

    return () => controller.abort();
  }, [profile.implantBonus]);

  const effectiveBrokerFee = brokerFee;
  const effectiveSalesTax = salesTax;
  const sellOrderNetMultiplier = 1 - effectiveSalesTax / 100 - effectiveBrokerFee / 100;
  const instantSellNetMultiplier = 1 - effectiveSalesTax / 100;
  const spPerDay = profile.spPerHour * 24;
  const spPerMonth = profile.spPerHour * HOURS_PER_MONTH;
  const extractorsPerMonth = spPerMonth / SP_PER_EXTRACTOR;
  const defaultExtraQueuesPerAccount = Math.max(0, trainingCharsPerAccount - 1);

  const effectiveExtractorTotalPlex = effectiveOfferTotal(extractorPack.totalPlex, extractorDiscountPct, manualExtractorPlex);
  const effectiveOmegaTotalPlex = effectiveOfferTotal(omegaPlan.totalPlex, omegaDiscountPct, manualOmegaPlex);
  const effectiveMctTotalPlex = effectiveOfferTotal(mctPlan.totalPlex, mctDiscountPct, manualMctPlex);

  const effectiveExtractorPlex = extractorPack.units > 0 ? effectiveExtractorTotalPlex / extractorPack.units : effectiveExtractorTotalPlex;
  const effectiveOmegaPlex = omegaPlan.months > 0 ? effectiveOmegaTotalPlex / omegaPlan.months : effectiveOmegaTotalPlex;

  const plexBuyBase = plexSource === "spot" ? plexPrice.sell_price : plexPrice.buy_price;
  const plexOrderBrokerFeePct = plexSource === "structure_buy" ? 0 : effectiveBrokerFee;
  const plexOrderSccPct = plexSource === "spot" ? 0 : SCC_SURCHARGE_PCT;
  const plexOrderBrokerFee = plexSource === "spot" ? 0 : Math.max(plexBuyBase * (plexOrderBrokerFeePct / 100), plexOrderBrokerFeePct > 0 ? 100 : 0);
  const plexOrderSccFee = plexSource === "spot" ? 0 : orderFeeAmount(plexBuyBase, plexOrderSccPct);
  const plexUnitAcquisitionCost = plexBuyBase + plexOrderBrokerFee + plexOrderSccFee;

  const nesExtractorCost = effectiveExtractorPlex * plexUnitAcquisitionCost;
  const omegaMonthlyCostPerAccount = profile.cloneType === "Omega" ? effectiveOmegaPlex * plexUnitAcquisitionCost : 0;
  const fleetOmegaCost = accounts * omegaMonthlyCostPerAccount;
  const mctUnitCost = (effectiveMctTotalPlex / mctPlan.units) * plexUnitAcquisitionCost;
  const extractorMarketBuy = farm.extractor_buy_price > 0 ? farm.extractor_buy_price : farm.extractor_sell_price;
  const extractorBrokerFeePct = extractorSource === "market_structure_buy" ? 0 : effectiveBrokerFee;
  const extractorBrokerFee = extractorSource === "market_buy" || extractorSource === "market_structure_buy"
    ? Math.max(extractorMarketBuy * (extractorBrokerFeePct / 100), extractorBrokerFeePct > 0 ? 100 : 0)
    : 0;
  const extractorSccFee = extractorSource === "market_buy" || extractorSource === "market_structure_buy"
    ? orderFeeAmount(extractorMarketBuy, SCC_SURCHARGE_PCT)
    : 0;
  const extractorUnitCost = extractorSource === "market_buy" || extractorSource === "market_structure_buy"
    ? extractorMarketBuy + extractorBrokerFee + extractorSccFee
    : extractorSource === "market_immediate"
      ? farm.extractor_sell_price
      : nesExtractorCost;

  const jitaSellNet = farm.injector_sell_price * sellOrderNetMultiplier;
  const instantSellNet = farm.injector_buy_price * instantSellNetMultiplier;
  const destinationPctTotal = injectorDestinations.reduce((sum, entry) => sum + entry.percentage, 0);
  const customInjectorGross = injectorDestinations.reduce((sum, entry) => sum + (entry.percentage / 100) * entry.sellPrice, 0);
  const customInjectorNet = injectorDestinations.reduce((sum, entry) => sum + (entry.percentage / 100) * entry.sellPrice * sellOrderNetMultiplier, 0);
  const injectorNet = injectorMode === "instant" ? instantSellNet : injectorMode === "custom_sell" ? customInjectorNet : jitaSellNet;
  const injectorGross = injectorMode === "instant" ? farm.injector_buy_price : injectorMode === "custom_sell" ? customInjectorGross : farm.injector_sell_price;

  const perCharExtractorCost = extractorsPerMonth * extractorUnitCost;
  const perCharRevenue = extractorsPerMonth * injectorNet;
  const perCharProfit = perCharRevenue - perCharExtractorCost;
  const fleetRows = fleetTrainingChars.map((row, index) => {
    const chars = row.chars;
    const extraQueues = Math.max(0, chars - 1);
    const mctCost = extraQueues * mctUnitCost;
    const cost = omegaMonthlyCostPerAccount + mctCost + chars * perCharExtractorCost;
    const revenue = chars * perCharRevenue;
    const profit = revenue - cost;
    return { index, chars, extraQueues, cost, revenue, profit };
  });
  const monthlyMctRequired = fleetRows.reduce((sum, row) => sum + row.extraQueues, 0);
  const fleetMctCost = monthlyMctRequired * mctUnitCost;
  const fleetExtractorCost = fleetRows.reduce((sum, row) => sum + row.chars * perCharExtractorCost, 0);
  const fleetCost = fleetRows.reduce((sum, row) => sum + row.cost, 0);
  const fleetRevenue = fleetRows.reduce((sum, row) => sum + row.revenue, 0);
  const fleetProfit = fleetRows.reduce((sum, row) => sum + row.profit, 0);
  const fleetROI = fleetCost > 0 ? (fleetProfit / fleetCost) * 100 : 0;
  const accountCost = omegaMonthlyCostPerAccount + defaultExtraQueuesPerAccount * mctUnitCost + trainingCharsPerAccount * perCharExtractorCost;
  const accountRevenue = trainingCharsPerAccount * perCharRevenue;
  const accountProfit = accountRevenue - accountCost;
  const breakEvenInjectorGross = extractorsPerMonth > 0 && trainingCharsPerAccount > 0
    ? accountCost / (trainingCharsPerAccount * extractorsPerMonth * (injectorMode === "instant" ? instantSellNetMultiplier : sellOrderNetMultiplier))
    : 0;
  const accountPlexUsage =
    (profile.cloneType === "Omega" ? effectiveOmegaPlex : 0) +
    defaultExtraQueuesPerAccount * (effectiveMctTotalPlex / mctPlan.units) +
    (extractorSource === "nes" ? trainingCharsPerAccount * extractorsPerMonth * effectiveExtractorPlex : 0);
  const breakEvenPlex = accountPlexUsage > 0 ? accountRevenue / accountPlexUsage : 0;
  const startupImplantCost = implantQuotes.reduce((sum, quote) => sum + quote.price, 0);
  const daysToExtractable = profile.spPerHour > 0 ? EXTRACTABLE_SP_THRESHOLD / profile.spPerHour / 24 : 0;
  const monthsToExtractable = daysToExtractable / 30;
  const implantStartupCostPerAccount = startupImplantCost * trainingCharsPerAccount;
  const implantStartupCostFleet = startupImplantCost * fleetRows.reduce((sum, row) => sum + row.chars, 0);
  const omegaInitialCostPerAccount = effectiveOmegaTotalPlex * plexUnitAcquisitionCost;
  const omegaInitialCostFleet = omegaInitialCostPerAccount * accounts;
  const mctUnitsRequiredPerAccount = defaultExtraQueuesPerAccount * omegaPlan.months;
  const mctPackagesPerAccount = mctUnitsRequiredPerAccount > 0 ? Math.ceil(mctUnitsRequiredPerAccount / mctPlan.units) : 0;
  const mctUnitsBoughtPerAccount = mctPackagesPerAccount * mctPlan.units;
  const mctUnitsExtraPerAccount = Math.max(0, mctUnitsBoughtPerAccount - mctUnitsRequiredPerAccount);
  const mctInitialCostPerAccount = mctPackagesPerAccount * effectiveMctTotalPlex * plexUnitAcquisitionCost;
  const mctUnitsRequiredFleet = fleetRows.reduce((sum, row) => sum + row.extraQueues, 0) * omegaPlan.months;
  const mctPackagesFleet = mctUnitsRequiredFleet > 0 ? Math.ceil(mctUnitsRequiredFleet / mctPlan.units) : 0;
  const mctUnitsBoughtFleet = mctPackagesFleet * mctPlan.units;
  const mctUnitsExtraFleet = Math.max(0, mctUnitsBoughtFleet - mctUnitsRequiredFleet);
  const mctInitialCostFleet = mctPackagesFleet * effectiveMctTotalPlex * plexUnitAcquisitionCost;
  const initialCapitalPerAccount = omegaInitialCostPerAccount + mctInitialCostPerAccount + implantStartupCostPerAccount;
  const initialCapitalFleet = omegaInitialCostFleet + mctInitialCostFleet + implantStartupCostFleet;

  const comparisonRows = TRAINING_PROFILES.map((candidate) => {
    const candidateExtractors = (candidate.spPerHour * HOURS_PER_MONTH) / SP_PER_EXTRACTOR;
    const candidatePerCharCost = candidateExtractors * extractorUnitCost;
    const candidatePerCharRevenue = candidateExtractors * injectorNet;
    const candidateAccountCost = omegaMonthlyCostPerAccount + defaultExtraQueuesPerAccount * mctUnitCost + trainingCharsPerAccount * candidatePerCharCost;
    const candidateAccountRevenue = trainingCharsPerAccount * candidatePerCharRevenue;
    const candidateFleetProfit = accounts * (candidateAccountRevenue - candidateAccountCost);
    const candidatePerCharProfit = candidatePerCharRevenue - candidatePerCharCost;
    return {
      ...candidate,
      extractors: candidateExtractors,
      perCharProfit: candidatePerCharProfit,
      fleetProfit: candidateFleetProfit,
    };
  }).sort((left, right) => right.fleetProfit - left.fleetProfit);

  const positiveFleet = fleetProfit >= 0;

  return (
    <div className={`border rounded-sm p-3 ${positiveFleet ? "border-eve-success/30 bg-eve-success/5" : "border-eve-error/30 bg-eve-error/5"}`}>
      <div className="flex items-center justify-between gap-3 mb-3">
        <div>
          <h3 className="text-xs font-semibold text-eve-dim uppercase tracking-wider">{t("plexSpfarmWorkbench")}</h3>
          <p className="text-[11px] text-eve-dim mt-1">{t("plexSpfarmWorkbenchHint")}</p>
        </div>
        <div className="text-right text-[11px] text-eve-dim">
          <div>{t("plexSpfarmPlexSpot")}: <span className="font-mono text-eve-text">{formatISK(plexPrice.sell_price)}</span></div>
          <div>{t("plexSpfarmPlexSellBuy")}: <span className="font-mono text-eve-text">{formatISK(plexPrice.sell_price)} / {formatISK(plexPrice.buy_price)}</span></div>
          <div>{t("plexSpfarmInjectorSellBuy")}: <span className="font-mono text-eve-text">{formatISK(farm.injector_sell_price)} / {formatISK(farm.injector_buy_price)}</span></div>
          <div>{t("plexSpfarmExtractorSellBuy")}: <span className="font-mono text-eve-text">{formatISK(farm.extractor_sell_price)} / {formatISK(extractorMarketBuy)}</span></div>
        </div>
      </div>

      <div className="space-y-3">
        <div className="bg-eve-dark/60 border border-eve-border/60 rounded-sm p-3">
          <div className="text-[10px] text-eve-dim uppercase tracking-wider font-medium mb-2">{t("plexSpfarmScenario")}</div>
          <div className="grid grid-cols-2 gap-2 text-xs">
            <label className="flex flex-col gap-1">
              <span className="text-eve-dim">{t("plexSpfarmTrainingProfile")}</span>
              <select value={profileId} onChange={(e) => setProfileId(e.target.value as typeof profileId)} className="px-2 py-1 bg-eve-input border border-eve-border rounded-sm text-eve-text">
                {TRAINING_PROFILES.map((entry) => (
                  <option key={entry.id} value={entry.id}>{formatProfileLabel(entry.attrs, entry.implants)}</option>
                ))}
              </select>
            </label>
            <label className="flex flex-col gap-1">
              <span className="text-eve-dim">{t("plexSpfarmExtractorSource")}</span>
              <select value={extractorSource} onChange={(e) => setExtractorSource(e.target.value as ExtractorSource)} className="px-2 py-1 bg-eve-input border border-eve-border rounded-sm text-eve-text">
                <option value="market_buy">{t("plexSpfarmMarketBuyOrder")}</option>
                <option value="market_structure_buy">{t("plexSpfarmMarketStructureBuyOrder")}</option>
                <option value="market_immediate">{t("plexSpfarmMarketImmediate")}</option>
                <option value="nes">NES</option>
              </select>
            </label>
            <label className="flex flex-col gap-1">
              <span className="text-eve-dim">{t("plexSpfarmPlexSource")}</span>
              <select value={plexSource} onChange={(e) => setPlexSource(e.target.value as PlexSource)} className="px-2 py-1 bg-eve-input border border-eve-border rounded-sm text-eve-text">
                <option value="spot">{t("plexSpfarmPlexSpot")}</option>
                <option value="buy_order">{t("plexSpfarmPlexBuyOrder")}</option>
                <option value="structure_buy">{t("plexSpfarmPlexStructureBuy")}</option>
              </select>
            </label>
            <label className="flex flex-col gap-1">
              <span className="text-eve-dim">{t("plexSpfarmInjectorExit")}</span>
              <select value={injectorMode} onChange={(e) => setInjectorMode(e.target.value as InjectorMode)} className="px-2 py-1 bg-eve-input border border-eve-border rounded-sm text-eve-text">
                <option value="sell_order">{t("plexSpfarmJitaSellOrder")}</option>
                <option value="instant">{t("plexSpfarmImmediateSell")}</option>
                <option value="custom_sell">{t("plexSpfarmCustomSellOrder")}</option>
              </select>
            </label>
            <label className="flex flex-col gap-1">
              <span className="text-eve-dim">{t("plexSpfarmOmegaPlan")}</span>
              <select value={omegaPlanId} onChange={(e) => setOmegaPlanId(e.target.value as typeof omegaPlanId)} className="px-2 py-1 bg-eve-input border border-eve-border rounded-sm text-eve-text">
                {OMEGA_PLANS.map((entry) => (
                  <option key={entry.id} value={entry.id}>{formatOmegaPlanLabel(entry.months)}</option>
                ))}
              </select>
            </label>
            <label className="flex flex-col gap-1">
              <span className="text-eve-dim">{t("plexFleetAccounts")}</span>
              <input type="number" min="1" max="50" value={accounts} onChange={(e) => setAccounts(Math.max(1, parseInt(e.target.value, 10) || 1))} className="px-2 py-1 bg-eve-input border border-eve-border rounded-sm text-eve-text font-mono" />
            </label>
            <label className="flex flex-col gap-1">
              <span className="text-eve-dim">{t("plexSpfarmDefaultTrainingCharsPerAccount")}</span>
              <input type="number" min="1" max="3" value={trainingCharsPerAccount} onChange={(e) => setTrainingCharsPerAccount(Math.min(3, Math.max(1, parseInt(e.target.value, 10) || 1)))} className="px-2 py-1 bg-eve-input border border-eve-border rounded-sm text-eve-text font-mono" />
            </label>
            <label className="flex flex-col gap-1">
              <span className="text-eve-dim">{t("plexSpfarmMctPlan")}</span>
              <select value={mctPlanId} onChange={(e) => setMctPlanId(e.target.value as typeof mctPlanId)} className="px-2 py-1 bg-eve-input border border-eve-border rounded-sm text-eve-text">
                {MCT_PLANS.map((entry) => (
                  <option key={entry.id} value={entry.id}>{formatMctPlanLabel(entry.units)}</option>
                ))}
              </select>
            </label>
          </div>
          {injectorMode === "custom_sell" && (
            <div className="mt-3 border-t border-eve-border/40 pt-3">
              <div className="flex items-center justify-between gap-2 mb-2">
                <div>
                  <div className="text-[10px] text-eve-dim uppercase tracking-wider font-medium">{t("plexSpfarmInjectorOutputPanel")}</div>
                  <div className="text-[11px] text-eve-dim" title={t("plexSpfarmInjectorOutputPanelHint")}>
                    {t("plexSpfarmInjectorOutputPanelDescription")}
                  </div>
                </div>
                <button
                  type="button"
                  onClick={() => setInjectorDestinations((prev) => [...prev, createInjectorDestination(false)])}
                  className="px-2 py-1 rounded-sm border border-eve-border text-[10px] uppercase tracking-wider text-eve-dim hover:text-eve-text"
                >
                  {t("plexSpfarmAddDestination")}
                </button>
              </div>
              <div className="space-y-2">
                {injectorDestinations.map((entry) => (
                  <InjectorDestinationRow
                    key={entry.id}
                    value={entry}
                    isLoggedIn={isLoggedIn}
                    activeCharacterId={activeCharacterId}
                    maxPercentage={Math.max(0, 100 - (destinationPctTotal - entry.percentage))}
                    canRemove={injectorDestinations.length > 1}
                    onRemove={() => setInjectorDestinations((prev) => prev.filter((candidate) => candidate.id !== entry.id))}
                    onChange={(next) => setInjectorDestinations((prev) => prev.map((candidate) => candidate.id === entry.id ? next : candidate))}
                  />
                ))}
              </div>
              <div className={`mt-2 text-[11px] ${destinationPctTotal <= 100 ? "text-eve-dim" : "text-eve-warning"}`}>
                {t("plexSpfarmDestinationAllocationTotal", { value: destinationPctTotal.toFixed(1) })}
              </div>
            </div>
          )}
        </div>

        <div className="bg-eve-dark/60 border border-eve-border/60 rounded-sm p-3">
          <div className="flex items-center justify-between gap-2 mb-2" title={t("plexSpfarmNesControlsHint")}>
            <div className="text-[10px] text-eve-dim uppercase tracking-wider font-medium">{t("plexSpfarmNesControls")}</div>
            <div className="text-[10px] text-eve-dim">{t("plexSpfarmNesControlsResetHint")}</div>
          </div>
          <div className="space-y-2 text-xs">
            {extractorSource === "nes" && (
              <div className="grid grid-cols-[minmax(280px,1fr)_108px_88px] gap-2 items-end">
                <label className="flex flex-col gap-1">
                  <span className="text-eve-dim">{t("plexSpfarmExtractorPack")}</span>
                  <span className="text-[11px] text-eve-dim">
                    {describeOffer(effectiveExtractorTotalPlex, extractorPack.units, t("plexDepthExtractor").toLowerCase(), t("plexSpfarmOfferSummary"))}
                    {manualExtractorPlex <= 0 && extractorDiscountPct > 0 ? ` (${-extractorDiscountPct}%)` : ""}
                  </span>
                  <select value={extractorPackId} onChange={(e) => setExtractorPackId(e.target.value as typeof extractorPackId)} className="px-2 py-1 bg-eve-input border border-eve-border rounded-sm text-eve-text">
                    {EXTRACTOR_PACKS.map((entry) => (
                      <option key={entry.id} value={entry.id}>{formatExtractorPackLabel(entry.units)}</option>
                    ))}
                  </select>
                </label>
                <label className="flex flex-col gap-1" title={t("plexSpfarmCustomTotalPlexHint")}>
                  <span className="text-eve-dim">{t("plexSpfarmTotalPlex")}</span>
                  <input type="number" min="0" step="0.1" value={manualExtractorPlex || ""} onChange={(e) => setManualExtractorPlex(parseFloat(e.target.value) || 0)} className="px-2 py-1 bg-eve-input border border-eve-border rounded-sm text-eve-text font-mono" placeholder={String(extractorPack.totalPlex)} />
                </label>
                <label className="flex flex-col gap-1">
                  <span className="text-eve-dim">{t("plexSpfarmDiscountPercent")}</span>
                  <input type="number" min="0" max="100" step="0.1" value={extractorDiscountPct} onChange={(e) => setExtractorDiscountPct(Math.max(0, parseFloat(e.target.value) || 0))} className="px-2 py-1 bg-eve-input border border-eve-border rounded-sm text-eve-text font-mono" />
                </label>
              </div>
            )}
            <div className="grid grid-cols-[1fr_108px_88px] gap-2 items-end">
              <label className="flex flex-col gap-1">
                <span className="text-eve-dim">{t("plexSpfarmOmegaPlan")}</span>
                <span className="text-[11px] text-eve-dim">
                  {describeOffer(effectiveOmegaTotalPlex, omegaPlan.months, t("plexSpfarmMonthSuffix"), t("plexSpfarmOfferSummary"))}
                  {manualOmegaPlex <= 0 && omegaDiscountPct > 0 ? ` (${-omegaDiscountPct}%)` : ""}
                </span>
                <select value={omegaPlanId} onChange={(e) => setOmegaPlanId(e.target.value as typeof omegaPlanId)} className="px-2 py-1 bg-eve-input border border-eve-border rounded-sm text-eve-text">
                  {OMEGA_PLANS.map((entry) => (
                    <option key={entry.id} value={entry.id}>{formatOmegaPlanLabel(entry.months)}</option>
                  ))}
                </select>
              </label>
              <label className="flex flex-col gap-1" title={t("plexSpfarmCustomTotalPlexHint")}>
                <span className="text-eve-dim">{t("plexSpfarmTotalPlex")}</span>
                <input type="number" min="0" step="0.1" value={manualOmegaPlex || ""} onChange={(e) => setManualOmegaPlex(parseFloat(e.target.value) || 0)} className="px-2 py-1 bg-eve-input border border-eve-border rounded-sm text-eve-text font-mono" placeholder={String(omegaPlan.totalPlex)} />
              </label>
              <label className="flex flex-col gap-1">
                <span className="text-eve-dim">{t("plexSpfarmDiscountPercent")}</span>
                <input type="number" min="0" max="100" step="0.1" value={omegaDiscountPct} onChange={(e) => setOmegaDiscountPct(Math.max(0, parseFloat(e.target.value) || 0))} className="px-2 py-1 bg-eve-input border border-eve-border rounded-sm text-eve-text font-mono" />
              </label>
            </div>
            <div className="grid grid-cols-[1fr_108px_88px] gap-2 items-end">
              <label className="flex flex-col gap-1">
                <span className="text-eve-dim">{t("plexSpfarmMctPlan")}</span>
                <span className="text-[11px] text-eve-dim">
                  {describeOffer(effectiveMctTotalPlex, mctPlan.units, "MCT", t("plexSpfarmOfferSummary"))}
                  {manualMctPlex <= 0 && mctDiscountPct > 0 ? ` (${-mctDiscountPct}%)` : ""}
                </span>
                <select value={mctPlanId} onChange={(e) => setMctPlanId(e.target.value as typeof mctPlanId)} className="px-2 py-1 bg-eve-input border border-eve-border rounded-sm text-eve-text">
                  {MCT_PLANS.map((entry) => (
                    <option key={entry.id} value={entry.id}>{formatMctPlanLabel(entry.units)}</option>
                  ))}
                </select>
              </label>
              <label className="flex flex-col gap-1" title={t("plexSpfarmCustomTotalPlexHint")}>
                <span className="text-eve-dim">{t("plexSpfarmTotalPlex")}</span>
                <input type="number" min="0" step="0.1" value={manualMctPlex || ""} onChange={(e) => setManualMctPlex(parseFloat(e.target.value) || 0)} className="px-2 py-1 bg-eve-input border border-eve-border rounded-sm text-eve-text font-mono" placeholder={String(mctPlan.totalPlex)} />
              </label>
              <label className="flex flex-col gap-1">
                <span className="text-eve-dim">{t("plexSpfarmDiscountPercent")}</span>
                <input type="number" min="0" max="100" step="0.1" value={mctDiscountPct} onChange={(e) => setMctDiscountPct(Math.max(0, parseFloat(e.target.value) || 0))} className="px-2 py-1 bg-eve-input border border-eve-border rounded-sm text-eve-text font-mono" />
              </label>
            </div>
          </div>
        </div>

        <div className="bg-eve-dark/60 border border-eve-border/60 rounded-sm p-3">
          <h4 className="text-[10px] font-semibold text-eve-dim uppercase tracking-wider mb-2">{t("plexSpfarmFleetPanel")}</h4>
          <div className="text-[11px] text-eve-dim mb-2">{t("plexSpfarmFleetPanelHint")}</div>
          <div className="max-h-72 overflow-y-auto border border-eve-border/40 rounded-sm">
            <table className="w-full text-xs">
              <thead className="sticky top-0 bg-eve-panel">
                <tr className="text-eve-dim border-b border-eve-border">
                  <th className="text-left py-1.5 px-2 font-medium">{t("plexSpfarmAccount")}</th>
                  <th className="text-right py-1.5 px-2 font-medium">{t("plexSpfarmTrainingChars")}</th>
                  <th className="text-right py-1.5 px-2 font-medium">{t("plexCost")}</th>
                  <th className="text-right py-1.5 px-2 font-medium">{t("plexRevenue")}</th>
                  <th className="text-right py-1.5 px-2 font-medium">{t("plexProfit")}</th>
                </tr>
              </thead>
              <tbody>
                {fleetRows.map((row) => (
                  <tr key={row.index} className="border-b border-eve-border/20">
                    <td className="py-1.5 px-2 text-eve-text">{t("plexSpfarmAccountShort", { index: row.index + 1 })}</td>
                    <td className="py-1.5 px-2 text-right font-mono text-eve-text w-[88px]">
                      <input
                        type="number"
                        min="1"
                        max="3"
                        value={row.chars}
                        onChange={(e) => {
                          const nextChars = Math.min(3, Math.max(1, parseInt(e.target.value, 10) || 1));
                          setFleetTrainingChars((prev) => prev.map((entry, index) => index === row.index ? { chars: nextChars, dirty: true } : entry));
                        }}
                        className="w-14 ml-auto px-2 py-1 bg-eve-input border border-eve-border rounded-sm text-right text-xs text-eve-text font-mono"
                      />
                    </td>
                    <td className="py-1.5 px-2 text-right font-mono tabular-nums text-eve-text w-[150px]">{formatISK(row.cost)}</td>
                    <td className="py-1.5 px-2 text-right font-mono tabular-nums text-eve-text w-[150px]">{formatISK(row.revenue)}</td>
                    <td className={`py-1.5 px-2 text-right font-mono tabular-nums w-[150px] ${row.profit >= 0 ? "text-eve-success" : "text-eve-error"}`}>{row.profit >= 0 ? "+" : ""}{formatISK(row.profit)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-5 gap-3">
          <div className="bg-eve-dark/60 border border-eve-border/60 rounded-sm p-3">
            <div className="text-[10px] text-eve-dim uppercase tracking-wider">{t("plexSpfarmMonthlyProfitability")}</div>
            <div className={`mt-1 text-lg font-semibold font-mono ${fleetProfit >= 0 ? "text-eve-success" : "text-eve-error"}`}>{fleetProfit >= 0 ? "+" : ""}{formatISK(fleetProfit)}</div>
            <div className="text-[11px] text-eve-dim mt-1">{t("plexSpfarmFleetProfitPerMonth")}</div>
            <div className="text-[11px] text-eve-dim">{t("plexSpfarmPerChar")}: {perCharProfit >= 0 ? "+" : ""}{formatISK(perCharProfit)}</div>
            <div className="text-[11px] text-eve-dim">{t("plexSpfarmDefaultAccount")}: {accountProfit >= 0 ? "+" : ""}{formatISK(accountProfit)}</div>
            <div className={`text-[11px] ${fleetROI >= 0 ? "text-eve-success" : "text-eve-error"}`}>{fleetROI >= 0 ? "+" : ""}{fleetROI.toFixed(1)}% ROI</div>
          </div>
          <div className="bg-eve-dark/60 border border-eve-border/60 rounded-sm p-3">
            <div className="text-[10px] text-eve-dim uppercase tracking-wider">{t("plexSpfarmMonthlyAcquisitionCost")}</div>
            <div className="mt-1 text-lg font-semibold font-mono text-eve-text">{formatISK(fleetCost)}</div>
            <div className="text-[11px] text-eve-dim mt-1">{t("plexSpfarmTotalAcquisitionCost")}</div>
            <div className="text-[11px] text-eve-dim">{t("plexSpfarmOmegaUpfront")}: {formatISK(fleetOmegaCost)}</div>
            <div className="text-[11px] text-eve-dim">MCT: {formatISK(fleetMctCost)}</div>
            <div className="text-[11px] text-eve-dim">{t("plexExtractors")}: {formatISK(fleetExtractorCost)}</div>
          </div>
          <div className="bg-eve-dark/60 border border-eve-border/60 rounded-sm p-3">
            <div className="text-[10px] text-eve-dim uppercase tracking-wider">{t("plexStartupCost")}</div>
            <div className="mt-1 text-lg font-semibold font-mono text-eve-text">{loadingImplantQuotes ? t("plexLoading") : formatISK(startupImplantCost)}</div>
            <div className="text-[11px] text-eve-dim mt-1">{t("plexSpfarmImplantSetCostPerCharacter", { implants: profile.implants === "None" ? t("plexSpfarmNone") : profile.implants })}</div>
            {profile.implantBonus === 0 ? (
              <div className="text-[11px] text-eve-dim">{t("plexSpfarmNoImplantPurchaseRequired")}</div>
            ) : (
              implantQuotes.map((quote) => (
                <div key={quote.typeID} className="text-[11px] text-eve-dim flex justify-between gap-2">
                  <span className="truncate">{quote.name}</span>
                  <span className="font-mono text-eve-text">{quote.price > 0 ? formatISK(quote.price) : t("plexNoData")}</span>
                </div>
              ))
            )}
            <div className="text-[11px] text-eve-dim mt-2">{t("plexSpfarmTrainingTimeToExtractable", { days: daysToExtractable.toFixed(0), months: monthsToExtractable.toFixed(1) })}</div>
          </div>
          <div className="bg-eve-dark/60 border border-eve-border/60 rounded-sm p-3">
            <div className="text-[10px] text-eve-dim uppercase tracking-wider">{t("plexSpfarmInitialCapitalRequired")}</div>
            <div className="mt-1 text-lg font-semibold font-mono text-eve-text">{loadingImplantQuotes ? t("plexLoading") : formatISK(initialCapitalPerAccount)}</div>
            <div className="text-[11px] text-eve-dim mt-1">{t("plexSpfarmInitialCapitalPerAccount")}</div>
            <div className="text-[11px] text-eve-dim">{t("plexSpfarmOmegaUpfront")}: {formatISK(omegaInitialCostPerAccount)}</div>
            <div className="text-[11px] text-eve-dim">{t("plexSpfarmMctUpfront")}: {formatISK(mctInitialCostPerAccount)}</div>
            <div className="text-[11px] text-eve-dim">{t("plexSpfarmImplantsUpfront")}: {loadingImplantQuotes ? t("plexLoading") : formatISK(implantStartupCostPerAccount)}</div>
            <div className="text-[11px] text-eve-dim mt-2">{t("plexSpfarmInitialCapitalPerFleet")}: {loadingImplantQuotes ? t("plexLoading") : formatISK(initialCapitalFleet)}</div>
            <div className="text-[11px] text-eve-dim">{t("plexSpfarmFleetOmegaUpfront")}: {formatISK(omegaInitialCostFleet)}</div>
            <div className="text-[11px] text-eve-dim">{t("plexSpfarmFleetMctUpfront")}: {formatISK(mctInitialCostFleet)}</div>
            <div className="text-[11px] text-eve-dim">{t("plexSpfarmFleetImplantsUpfront")}: {loadingImplantQuotes ? t("plexLoading") : formatISK(implantStartupCostFleet)}</div>
            <div className="text-[11px] text-eve-dim mt-2">{t("plexSpfarmMctCoveragePerAccount", { required: mctUnitsRequiredPerAccount, bought: mctUnitsBoughtPerAccount, extra: mctUnitsExtraPerAccount })}</div>
            <div className="text-[11px] text-eve-dim">{t("plexSpfarmMctCoverageFleet", { required: mctUnitsRequiredFleet, bought: mctUnitsBoughtFleet, extra: mctUnitsExtraFleet })}</div>
          </div>
          <div className="bg-eve-dark/60 border border-eve-border/60 rounded-sm p-3">
            <div className="text-[10px] text-eve-dim uppercase tracking-wider">{t("plexBreakEven")}</div>
            <div className="mt-1 text-sm font-semibold font-mono text-eve-text">{formatISK(breakEvenInjectorGross)}</div>
            <div className="text-[11px] text-eve-dim mt-1">{t("plexSpfarmInjectorSellPrice")}</div>
            <div className="text-[11px] text-eve-dim">{breakEvenPlex > 0 ? `${formatISK(breakEvenPlex)} / PLEX` : t("plexSpfarmPlexBreakEvenNa")}</div>
          </div>
        </div>

        <div className="bg-eve-dark/60 border border-eve-border/60 rounded-sm p-3">
          <h4 className="text-[10px] font-semibold text-eve-dim uppercase tracking-wider mb-2">{t("plexSpfarmCurrentAssumptions")}</h4>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-x-4 gap-y-1 text-xs">
            <Row label={t("plexPath")} value={formatProfileLabel(profile.attrs, profile.implants)} />
            <Row label={t("plexSpfarmSpPerMinute")} value={profile.spPerMinute.toFixed(2)} />
            <Row label={t("plexSpfarmSpPerHour")} value={profile.spPerHour.toLocaleString()} />
            <Row label={t("plexSpfarmSpPerDay")} value={spPerDay.toLocaleString()} />
            <Row label={t("plexSpfarmSpPerMonth")} value={spPerMonth.toLocaleString()} />
            <Row label={t("plexSpfarmExtractorsPerMonthPerChar")} value={extractorsPerMonth.toFixed(2)} />
            <Row
              label={t("plexSpfarmExtractorCost")}
              value={formatISK(extractorUnitCost)}
              extra={(extractorSource === "market_buy" || extractorSource === "market_structure_buy") ? (
                <InfoPopover label={t("plexSpfarmExtractorFeeBreakdown")}>
                  <div className="space-y-1">
                    <div>{t("plexSpfarmBaseBuyOrder")}: <span className="font-mono">{formatISK(extractorMarketBuy)}</span></div>
                    <div>{t("plexSpfarmSccSurcharge")}: <span className="font-mono">{formatISK(extractorSccFee)}</span></div>
                    <div>{t("plexSpfarmBrokerFeeWithPct", { pct: extractorBrokerFeePct.toFixed(2) })}: <span className="font-mono">{formatISK(extractorBrokerFee)}</span></div>
                    <div>{t("plexSpfarmTotal")}: <span className="font-mono">{formatISK(extractorUnitCost)}</span></div>
                  </div>
                </InfoPopover>
              ) : undefined}
            />
            <Row label={t("plexSpfarmInjectorGross")} value={formatISK(injectorGross)} />
            <Row
              label={t("plexSpfarmInjectorNet")}
              value={formatISK(injectorNet)}
              extra={injectorMode !== "instant" ? (
                <InfoPopover label={t("plexSpfarmInjectorFeeBreakdown")}>
                  <div className="space-y-1">
                    <div>{t("plexSpfarmGrossSell")}: <span className="font-mono">{formatISK(injectorGross)}</span></div>
                    <div>{t("plexSpfarmBrokerFeeWithPct", { pct: effectiveBrokerFee.toFixed(2) })}: <span className="font-mono">{formatISK(orderFeeAmount(injectorGross, effectiveBrokerFee))}</span></div>
                    <div>{t("plexSpfarmSalesTaxWithPct", { pct: effectiveSalesTax.toFixed(2) })}: <span className="font-mono">{formatISK(orderFeeAmount(injectorGross, effectiveSalesTax))}</span></div>
                    <div>{t("plexSpfarmNet")}: <span className="font-mono">{formatISK(injectorNet)}</span></div>
                  </div>
                </InfoPopover>
              ) : undefined}
            />
            <Row label={t("plexSpfarmOmegaPerMonthPerAccount")} value={`${effectiveOmegaPlex.toFixed(2)} PLEX = ${formatISK(omegaMonthlyCostPerAccount)}`} />
            <Row label={t("plexSpfarmMctPerActiveQueue")} value={`${(effectiveMctTotalPlex / mctPlan.units).toFixed(2)} PLEX = ${formatISK(mctUnitCost)}`} />
            {extractorSource === "nes" && <Row label={t("plexSpfarmNesExtractor")} value={`${effectiveExtractorPlex.toFixed(1)} PLEX = ${formatISK(nesExtractorCost)}`} />}
            <Row
              label={t("plexSpfarmPlexAcquisition")}
              value={`${formatISK(plexUnitAcquisitionCost)} / PLEX`}
              extra={plexSource !== "spot" ? (
                <InfoPopover label={t("plexSpfarmPlexFeeBreakdown")}>
                  <div className="space-y-1">
                    <div>{t("plexSpfarmBaseBuyOrder")}: <span className="font-mono">{formatISK(plexBuyBase)}</span></div>
                    <div>{t("plexSpfarmSccSurcharge")}: <span className="font-mono">{formatISK(plexOrderSccFee)}</span></div>
                    <div>{t("plexSpfarmBrokerFeeWithPct", { pct: plexOrderBrokerFeePct.toFixed(2) })}: <span className="font-mono">{formatISK(plexOrderBrokerFee)}</span></div>
                    <div>{t("plexSpfarmTotal")}: <span className="font-mono">{formatISK(plexUnitAcquisitionCost)}</span></div>
                  </div>
                </InfoPopover>
              ) : undefined}
            />
            <Row label={t("plexFleetRevenue")} value={formatISK(fleetRevenue)} />
            <Row label={t("plexSpfarmMctRequirement")} value={t("plexSpfarmMctRequirementValue", { count: monthlyMctRequired })} />
          </div>
        </div>

        <div className="bg-eve-dark/60 border border-eve-border/60 rounded-sm p-3">
          <h4 className="text-[10px] font-semibold text-eve-dim uppercase tracking-wider mb-2">{t("plexSpfarmProfileComparison")}</h4>
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="text-eve-dim border-b border-eve-border">
                  <th className="text-left py-1.5 px-2 font-medium">{t("plexPath")}</th>
                  <th className="text-right py-1.5 px-2 font-medium">{t("plexSpfarmSpPerHour")}</th>
                  <th className="text-right py-1.5 px-2 font-medium">{t("plexSpfarmExtractorsPerMonth")}</th>
                  <th className="text-right py-1.5 px-2 font-medium">{t("plexSpfarmPerCharProfit")}</th>
                  <th className="text-right py-1.5 px-2 font-medium">{t("plexFleetProfit")}</th>
                </tr>
              </thead>
              <tbody>
                {comparisonRows.map((row) => (
                  <tr key={row.id} className={`border-b border-eve-border/30 ${row.id === profileId ? "bg-eve-accent/5" : ""}`}>
                    <td className="py-1.5 px-2 text-eve-text">{formatProfileLabel(row.attrs, row.implants)}</td>
                    <td className="py-1.5 px-2 text-right font-mono text-eve-text">{row.spPerHour.toLocaleString()}</td>
                    <td className="py-1.5 px-2 text-right font-mono text-eve-text">{row.extractors.toFixed(2)}</td>
                    <td className={`py-1.5 px-2 text-right font-mono ${row.perCharProfit >= 0 ? "text-eve-success" : "text-eve-error"}`}>{row.perCharProfit >= 0 ? "+" : ""}{formatISK(row.perCharProfit)}</td>
                    <td className={`py-1.5 px-2 text-right font-mono font-semibold ${row.fleetProfit >= 0 ? "text-eve-success" : "text-eve-error"}`}>{row.fleetProfit >= 0 ? "+" : ""}{formatISK(row.fleetProfit)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  );
}

function Row({ label, value, dim, extra }: { label: string; value: string; dim?: boolean; extra?: ReactNode }) {
  return (
    <div className="flex justify-between">
      <span className="text-eve-dim flex items-center">{label}{extra}</span>
      <span className={`font-mono ${dim ? "text-eve-dim" : "text-eve-text"}`}>{value}</span>
    </div>
  );
}
