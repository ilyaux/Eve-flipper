import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useKeyboardShortcuts } from "./lib/useKeyboardShortcuts";
import { StatusBar } from "./components/StatusBar";
import { ParametersPanel } from "./components/ParametersPanel";
import { ContractParametersPanel } from "./components/ContractParametersPanel";
import { ScanResultsTable } from "./components/ScanResultsTable";
import { ContractResultsTable } from "./components/ContractResultsTable";
import { RouteBuilder } from "./components/RouteBuilder";
import { WatchlistTab } from "./components/WatchlistTab";
import { StationTrading } from "./components/StationTrading";
import { IndustryTab } from "./components/IndustryTab";
import { WarTracker } from "./components/WarTracker";
import { PlexTab } from "./components/PlexTab";
// import { MarketMakingTab } from "./components/MarketMakingTab";
import { ScanHistory } from "./components/ScanHistory";
import { CommandPalette } from "./components/CommandPalette";
import { KeyboardShortcutsHelp } from "./components/KeyboardShortcutsHelp";
import { LanguageSwitcher } from "./components/LanguageSwitcher";
import { ThemeSwitcher } from "./components/ThemeSwitcher";
import { useGlobalToast } from "./components/Toast";
import { Modal } from "./components/Modal";
import { CharacterPopup } from "./components/CharacterPopup";
import {
  applyAppUpdate,
  getUpdateCheckStatus,
  getConfig,
  skipAppUpdateForSession,
  updateConfig,
  scan,
  scanMultiRegion,
  scanRegionalDayTrader,
  scanContracts,
  testAlertChannels,
  getWatchlist,
} from "./lib/api";
import { useI18n } from "./lib/i18n";
import { formatISK } from "./lib/format";
import { useAuth } from "./lib/useAuth";
import { useVersionCheck } from "./lib/useVersionCheck";
import { useEsiStatus } from "./lib/useEsiStatus";
import type {
  ContractResult,
  FlipResult,
  RegionalDayTradeHub,
  RegionalDayTradeItem,
  RouteResult,
  ScanParams,
  StationCacheMeta,
  StationTrade,
} from "./lib/types";
import logo from "./assets/logo.svg";

type Tab =
  | "radius"
  | "region"
  | "contracts"
  | "station"
  | "route"
  | "industry"
  | "demand"
  | "plex";

type AlertChannels = {
  telegram: boolean;
  discord: boolean;
  desktop: boolean;
};

type PatronEntry = {
  name: string;
  tier?: string;
  since?: string;
  note?: string;
  url?: string;
};

type PatronFeed = {
  updated_at?: string;
  project?: string;
  patrons?: unknown[];
};

const defaultPatronsURL = "https://ilyaux.github.io/eve-flipper-data/patrons.json";
const patronsDataURL =
  (import.meta.env.VITE_PATRONS_URL as string | undefined)?.trim() ||
  defaultPatronsURL;
const REGION_SCAN_RESTORE_MAX_ROWS = 750;

type DesktopRuntimeWindow = Window & {
  __TAURI_INTERNALS__?: unknown;
  runtime?: { BrowserOpenURL?: (url: string) => void };
};

function getDesktopRuntimeFlags() {
  const runtime = window as unknown as DesktopRuntimeWindow;
  const isTauri = !!runtime.__TAURI_INTERNALS__;
  const isWails = typeof runtime.runtime?.BrowserOpenURL === "function";
  return { runtime, isTauri, isWails, isDesktop: isTauri || isWails };
}

function toRecord(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== "object") return null;
  return value as Record<string, unknown>;
}

function toNumber(value: unknown, fallback = 0): number {
  if (typeof value === "number") return Number.isFinite(value) ? value : fallback;
  if (typeof value === "string") {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : fallback;
  }
  return fallback;
}

function toInt(value: unknown, fallback = 0): number {
  return Math.trunc(toNumber(value, fallback));
}

function toText(value: unknown, fallback = ""): string {
  return typeof value === "string" ? value : fallback;
}

function toNumberArray(value: unknown): number[] | undefined {
  if (!Array.isArray(value)) return undefined;
  const out = value
    .map((entry) => toNumber(entry, Number.NaN))
    .filter((entry) => Number.isFinite(entry));
  return out.length > 0 ? out : undefined;
}

function normalizePatronEntries(value: unknown): PatronEntry[] {
  if (!Array.isArray(value)) return [];
  const out: PatronEntry[] = [];

  for (const entry of value) {
    if (typeof entry === "string") {
      const name = entry.trim();
      if (name !== "") out.push({ name });
      continue;
    }
    const rec = toRecord(entry);
    if (!rec) continue;
    const name = toText(rec.name, "").trim();
    if (name === "") continue;

    const patron: PatronEntry = { name };
    const tier = toText(rec.tier, "").trim();
    if (tier !== "") patron.tier = tier;
    const since = toText(rec.since, "").trim();
    if (since !== "") patron.since = since;
    const note = toText(rec.note, "").trim();
    if (note !== "") patron.note = note;
    const url = toText(rec.url, "").trim();
    if (url !== "") patron.url = url;

    out.push(patron);
  }

  return out;
}

function isFlipResultLike(value: unknown): value is FlipResult {
  const rec = toRecord(value);
  if (!rec) return false;
  return toInt(rec.TypeID, 0) > 0;
}

function isLegacyRegionalItemLike(value: unknown): value is RegionalDayTradeItem {
  const rec = toRecord(value);
  if (!rec) return false;
  return toInt(rec.type_id, 0) > 0;
}

function isLegacyRegionalHubLike(value: unknown): value is RegionalDayTradeHub {
  const rec = toRecord(value);
  return !!rec && Array.isArray(rec.items);
}

function legacyRegionalItemToFlip(
  item: RegionalDayTradeItem,
  hub?: RegionalDayTradeHub,
): FlipResult {
  const unitsToBuy = Math.max(0, toInt(item.purchase_units, 0));
  const sellJumps = Math.max(0, toInt(item.jumps, 0));
  const nowProfit = toNumber(item.target_now_profit, 0);
  const periodProfit = toNumber(item.target_period_profit, nowProfit);
  const demandPerDay = toNumber(item.target_demand_per_day, 0);
  const volume = toNumber(item.item_volume, 0);
  const perUnitNowProfit = unitsToBuy > 0 ? nowProfit / unitsToBuy : 0;
  const dailyProfit = demandPerDay > 0 ? perUnitNowProfit * demandPerDay : 0;
  const typeID = Math.max(0, toInt(item.type_id, 0));
  const typeNameRaw = toText(item.type_name, "").trim();
  const typeName = typeNameRaw !== "" ? typeNameRaw : `Type ${typeID}`;
  const sourceSystemName = toText(item.source_system_name, "");
  const targetSystemName = toText(item.target_system_name, "");
  const buyStation = toText(item.source_station_name, sourceSystemName);
  const sellStation = toText(item.target_station_name, targetSystemName);

  const row: FlipResult = {
    TypeID: typeID,
    TypeName: typeName,
    Volume: volume,
    BuyPrice: toNumber(item.source_avg_price, 0),
    BuyStation: buyStation,
    BuySystemName: sourceSystemName,
    BuySystemID: toInt(item.source_system_id, 0),
    BuyRegionID: toInt(item.source_region_id, 0),
    BuyRegionName: toText(item.source_region_name, ""),
    BuyLocationID: toInt(item.source_location_id, 0),
    SellPrice: toNumber(item.target_now_price, 0),
    SellStation: sellStation,
    SellSystemName: targetSystemName,
    SellSystemID: toInt(item.target_system_id, 0),
    SellRegionID: toInt(item.target_region_id, 0),
    SellRegionName: toText(item.target_region_name, ""),
    SellLocationID: toInt(item.target_location_id, 0),
    ProfitPerUnit: perUnitNowProfit,
    MarginPercent: toNumber(item.margin_now, 0),
    UnitsToBuy: unitsToBuy,
    BuyOrderRemain: toInt(item.target_supply_units, 0),
    SellOrderRemain: toInt(item.source_units, 0),
    TotalProfit: nowProfit,
    ProfitPerJump: sellJumps > 0 ? nowProfit / sellJumps : nowProfit,
    BuyJumps: 0,
    SellJumps: sellJumps,
    TotalJumps: sellJumps,
    DailyVolume: Math.round(demandPerDay),
    Velocity: 0,
    PriceTrend: 0,
    S2BPerDay: demandPerDay,
    BuyCompetitors: 0,
    SellCompetitors: 0,
    DailyProfit: dailyProfit,
    ExpectedBuyPrice: toNumber(item.source_avg_price, 0),
    ExpectedSellPrice: toNumber(item.target_period_price, toNumber(item.target_now_price, 0)),
    ExpectedProfit: periodProfit,
    RealProfit: periodProfit,
    DaySecurity: toNumber(hub?.security, 0),
    DaySourceUnits: toInt(item.source_units, 0),
    DayTargetDemandPerDay: demandPerDay,
    DayTargetSupplyUnits: toInt(item.target_supply_units, 0),
    DayTargetDOS: toNumber(item.target_dos, 0),
    DayAssets: toInt(item.assets, 0),
    DayActiveOrders: toInt(item.active_orders, 0),
    DaySourceAvgPrice: toNumber(item.source_avg_price, 0),
    DayTargetNowPrice: toNumber(item.target_now_price, 0),
    DayTargetPeriodPrice: toNumber(item.target_period_price, toNumber(item.target_now_price, 0)),
    DayNowProfit: nowProfit,
    DayPeriodProfit: periodProfit,
    DayROINow: toNumber(item.roi_now, 0),
    DayROIPeriod: toNumber(item.roi_period, 0),
    DayCapitalRequired: toNumber(item.capital_required, 0),
    DayShippingCost: toNumber(item.shipping_cost, 0),
    DayCategoryID: toInt(item.category_id, 0),
    DayGroupID: toInt(item.group_id, 0),
    DayGroupName: toText(item.group_name, ""),
    DayIskPerM3Jump:
      volume > 0 && sellJumps > 0
        ? perUnitNowProfit / (volume * sellJumps)
        : 0,
    DayTradeScore: toNumber(item.trade_score, 0),
    DayTargetLowestSell: toNumber(item.target_lowest_sell, 0),
  };
  const priceHistory = toNumberArray(item.target_price_history);
  if (priceHistory) row.DayPriceHistory = priceHistory;
  return row;
}

function normalizeRegionalResults(raw: unknown[]): FlipResult[] {
  if (!Array.isArray(raw) || raw.length === 0) return [];

  const out: FlipResult[] = [];

  for (const entry of raw) {
    if (isFlipResultLike(entry)) {
      out.push(entry);
      continue;
    }

    if (isLegacyRegionalItemLike(entry)) {
      out.push(legacyRegionalItemToFlip(entry));
      continue;
    }

    if (isLegacyRegionalHubLike(entry)) {
      for (const item of entry.items) {
        if (!isLegacyRegionalItemLike(item)) continue;
        out.push(legacyRegionalItemToFlip(item, entry));
      }
    }
  }

  return out.filter((row) => row.TypeID > 0);
}

function App() {
  const { t } = useI18n();
  const [bootSplashState, setBootSplashState] = useState<
    "visible" | "fading" | "hidden"
  >("visible");
  const showBootSplash = bootSplashState !== "hidden";

  const [params, setParams] = useState<ScanParams>({
    system_name: "Jita",
    ignored_system_ids: [],
    cargo_capacity: 5000,
    buy_radius: 5,
    sell_radius: 10,
    min_margin: 5,
    sales_tax_percent: 8,
    broker_fee_percent: 0,
    split_trade_fees: false,
    buy_broker_fee_percent: 0,
    sell_broker_fee_percent: 0,
    buy_sales_tax_percent: 0,
    sell_sales_tax_percent: 8,
    min_item_profit: 0,
    min_route_security: 0.45,
    avg_price_period: 14,
    purchase_demand_days: 0.5,
    shipping_cost_per_m3_jump: 0,
    source_regions: [
      "The Forge",
      "Domain",
      "Sinq Laison",
      "Metropolis",
      "Heimatar",
    ],
    target_market_system: "Jita",
    target_market_location_id: 0,
    contract_hold_days: 7,
    contract_target_confidence: 80,
    exclude_rigs_with_ship: true,
    route_min_hops: 2,
    route_max_hops: 5,
    route_target_system_name: "",
    route_min_isk_per_jump: 0,
    route_allow_empty_hops: false,
    route_mode: "balanced",
    route_ship_profile: "custom",
    route_cargo_capacity: 5000,
    route_minutes_per_jump: 2,
    route_dock_minutes: 4,
    route_safety_delay_percent: 0,
    sell_order_mode: false,
  });
  const configLoadedRef = useRef(false);
  const regionDefaultsAppliedRef = useRef(false);

  const [tab, setTabRaw] = useState<Tab>(() => {
    try {
      const saved = localStorage.getItem("eve-flipper-active-tab");
      if (
        saved &&
        [
          "radius",
          "region",
          "contracts",
          "station",
          "route",
          "industry",
          "demand",
          "plex",
        ].includes(saved)
      ) {
        return saved as Tab;
      }
    } catch {
      /* ignore */
    }
    return "radius";
  });
  const setTab = useCallback((t: Tab) => {
    setTabRaw(t);
    try {
      localStorage.setItem("eve-flipper-active-tab", t);
    } catch {
      /* ignore */
    }
  }, []);
  const {
    authStatus,
    loginPolling,
    handleLogin,
    handleLogout,
    handleSelectCharacter,
    handleDeleteCharacter,
    refreshAuthStatus,
  } = useAuth();
  const characterCount = authStatus.characters?.length ?? (authStatus.logged_in ? 1 : 0);
  const {
    appVersion,
    latestVersion,
    hasUpdate,
    dismissedForSession,
    autoUpdateSupported,
    platform: updatePlatform,
    releaseURL,
  } = useVersionCheck();
  const { esiAvailable } = useEsiStatus();

  const [radiusResults, setRadiusResults] = useState<FlipResult[]>([]);
  const [regionResults, setRegionResults] = useState<FlipResult[]>([]);
  const [contractResults, setContractResults] = useState<ContractResult[]>([]);
  const [radiusCacheMeta, setRadiusCacheMeta] = useState<StationCacheMeta | null>(null);
  const [regionCacheMeta, setRegionCacheMeta] = useState<StationCacheMeta | null>(null);
  const [contractCacheMeta, setContractCacheMeta] = useState<StationCacheMeta | null>(null);
  const [stationLoadedResults, setStationLoadedResults] = useState<
    StationTrade[] | null
  >(null);
  const [routeLoadedResults, setRouteLoadedResults] = useState<
    RouteResult[] | null
  >(null);

  const [scanning, setScanning] = useState(false);
  const [progress, setProgress] = useState("");
  const [regionRestorePrompt, setRegionRestorePrompt] = useState<{
    ts: number;
    results: FlipResult[];
  } | null>(null);
  const [autoRefreshRadius, setAutoRefreshRadius] = useState(false);
  const [autoRefreshRegion, setAutoRefreshRegion] = useState(false);

  const [showWatchlist, setShowWatchlist] = useState(false);
  const [showHistory, setShowHistory] = useState(false);
  const [showPatrons, setShowPatrons] = useState(false);
  const [showCharacter, setShowCharacter] = useState(false);
  const [showCommandPalette, setShowCommandPalette] = useState(false);
  const [showShortcutsHelp, setShowShortcutsHelp] = useState(false);
  const [showUpdateModal, setShowUpdateModal] = useState(false);
  const [updateApplying, setUpdateApplying] = useState(false);
  const [updateApplyError, setUpdateApplyError] = useState("");
  const [updateApplyStarted, setUpdateApplyStarted] = useState(false);
  const [patrons, setPatrons] = useState<PatronEntry[]>([]);
  const [patronsLoading, setPatronsLoading] = useState(false);
  const [patronsError, setPatronsError] = useState("");
  const [patronsUpdatedAt, setPatronsUpdatedAt] = useState("");
  const [patronsProject, setPatronsProject] = useState("");
  const [alertChannels, setAlertChannels] = useState<AlertChannels>({
    telegram: false,
    discord: false,
    desktop: true,
  });
  const [alertTelegramToken, setAlertTelegramToken] = useState("");
  const [alertTelegramChatID, setAlertTelegramChatID] = useState("");
  const [alertDiscordWebhook, setAlertDiscordWebhook] = useState("");
  const [alertTestLoading, setAlertTestLoading] = useState(false);

  const abortRef = useRef<AbortController | null>(null);
  const desktopAlertCooldownRef = useRef<Map<string, number>>(new Map());
  const radiusAutoRefreshSignatureRef = useRef<string>("");
  const radiusAutoRefreshLastRunRef = useRef<number>(0);
  const regionAutoRefreshSignatureRef = useRef<string>("");
  const regionAutoRefreshLastRunRef = useRef<number>(0);
  const regionPersistTimerRef = useRef<ReturnType<typeof setTimeout>>(undefined);
  const { addToast } = useGlobalToast();

  const openExternalURL = useCallback(async (url: string) => {
    const { runtime, isTauri, isWails } = getDesktopRuntimeFlags();
    if (isWails) {
      runtime.runtime?.BrowserOpenURL?.(url);
      return;
    }
    if (isTauri) {
      try {
        const { openUrl } = await import("@tauri-apps/plugin-opener");
        await openUrl(url);
        return;
      } catch {
        // fallback below
      }
    }
    window.open(url, "_blank", "noopener,noreferrer");
  }, []);

  const [contractScanCompleted, setContractScanCompleted] = useState(false);
  const contractFilterHints = useMemo(() => {
    if (contractResults.length > 0 || !contractScanCompleted) return undefined;
    return [
      `${t("minContractPrice")}: ${formatISK(params.min_contract_price ?? 10_000_000)}`,
      `${t("maxContractMargin")}: ${params.max_contract_margin ?? 100}%`,
      `${t("minPricedRatio")}: ${((params.min_priced_ratio ?? 0.8) * 100).toFixed(0)}%`,
      `${t("contractHoldDays")}: ${params.contract_hold_days ?? 7}`,
      `${t("contractTargetConfidence")}: ${params.contract_target_confidence ?? 80}%`,
    ];
  }, [
    contractResults.length,
    contractScanCompleted,
    params.min_contract_price,
    params.max_contract_margin,
    params.min_priced_ratio,
    params.contract_hold_days,
    params.contract_target_confidence,
    t,
  ]);

  // Keyboard shortcuts
  const shortcuts = useMemo(
    () => [
      {
        key: "s",
        modifiers: ["ctrl"] as const,
        handler: () => {
          if (tab !== "route" && tab !== "station" && params.system_name) {
            // Trigger scan via button click simulation
            document
              .querySelector<HTMLButtonElement>("[data-scan-button]")
              ?.click();
          }
        },
        description: "Start/Stop scan",
      },
      {
        key: "1",
        modifiers: ["alt"] as const,
        handler: () => setTab("radius"),
        description: "Switch to Radius tab",
      },
      {
        key: "2",
        modifiers: ["alt"] as const,
        handler: () => setTab("region"),
        description: "Switch to Region tab",
      },
      {
        key: "3",
        modifiers: ["alt"] as const,
        handler: () => setTab("contracts"),
        description: "Switch to Contracts tab",
      },
      {
        key: "4",
        modifiers: ["alt"] as const,
        handler: () => setTab("station"),
        description: "Switch to Station tab",
      },
      {
        key: "5",
        modifiers: ["alt"] as const,
        handler: () => setTab("route"),
        description: "Switch to Route tab",
      },
      {
        key: "w",
        modifiers: ["alt"] as const,
        handler: () => setShowWatchlist(true),
        description: "Open Watchlist",
      },
      {
        key: "h",
        modifiers: ["alt"] as const,
        handler: () => setShowHistory(true),
        description: "Open History",
      },
      {
        key: "k",
        modifiers: ["ctrl"] as const,
        handler: () => setShowCommandPalette((v) => !v),
        description: "Open Command Palette",
      },
      {
        key: "p",
        modifiers: ["ctrl", "alt"] as const,
        // Use "open" instead of toggle because this shortcut is also handled
        // by a direct window listener below; both can fire for one keypress.
        handler: () => setShowShortcutsHelp(true),
        description: "Show keyboard shortcuts",
      },
    ],
    [tab, params.system_name],
  );

  useKeyboardShortcuts(shortcuts);

  // Direct listener for Alt+Ctrl+P — bypasses useKeyboardShortcuts so it works
  // even when focus is inside an input or WebView2 intercepts modifier combos
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.code === "KeyP" && e.ctrlKey && e.altKey) {
        e.preventDefault();
        setShowShortcutsHelp(true);
      }
    };
    window.addEventListener("keydown", handler, { capture: true });
    return () => window.removeEventListener("keydown", handler, { capture: true });
  }, []);

  const toggleAlertChannel = useCallback(
    (channel: keyof AlertChannels) => {
      setAlertChannels((prev) => {
        const next = { ...prev, [channel]: !prev[channel] };
        if (!next.telegram && !next.discord && !next.desktop) {
          addToast(t("alertConfigAtLeastOne"), "warning", 2500);
          return prev;
        }
        return next;
      });
    },
    [addToast, t],
  );

  const handleTestAlert = useCallback(async () => {
    setAlertTestLoading(true);
    try {
      const desktopMsg = `${t("appTitle")}: ${t("alertConfigTestSent")}`;
      if (alertChannels.desktop) {
        addToast(desktopMsg, "info", 2500);
        if ("Notification" in window) {
          if (Notification.permission === "granted") {
            new Notification(t("appTitle"), { body: desktopMsg });
          } else if (Notification.permission === "default") {
            Notification.requestPermission().then((perm) => {
              if (perm === "granted") {
                new Notification(t("appTitle"), { body: desktopMsg });
              }
            });
          }
        }
      }

      const res = await testAlertChannels();
      const sent = res.sent ?? [];
      const failed = res.failed ? Object.keys(res.failed) : [];
      if (sent.length > 0) {
        addToast(
          `${t("alertConfigTestSent")}: ${sent.join(", ")}`,
          "success",
          3000,
        );
      }
      if (failed.length > 0) {
        addToast(
          `${t("alertConfigTestFailed")}: ${failed.join(", ")}`,
          "warning",
          3500,
        );
      }
      if (sent.length === 0 && failed.length === 0) {
        addToast(t("alertConfigNoExternalChannels"), "info", 2500);
      }
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Unknown error";
      addToast(`${t("errorPrefix")}${msg}`, "error", 3500);
    } finally {
      setAlertTestLoading(false);
    }
  }, [addToast, t, alertChannels.desktop]);

  const loadPatrons = useCallback(async () => {
    setPatronsLoading(true);
    setPatronsError("");
    try {
      const res = await fetch(patronsDataURL, { cache: "no-store" });
      if (!res.ok) {
        throw new Error(`HTTP ${res.status}`);
      }
      const payload = (await res.json()) as PatronFeed;
      setPatrons(normalizePatronEntries(payload?.patrons));
      setPatronsUpdatedAt(toText(payload?.updated_at, ""));
      setPatronsProject(toText(payload?.project, ""));
    } catch (e) {
      const reason = e instanceof Error ? e.message : t("patronsFetchError");
      setPatronsError(reason);
      setPatrons([]);
    } finally {
      setPatronsLoading(false);
    }
  }, [t]);

  useEffect(() => {
    if (!showPatrons) return;
    void loadPatrons();
  }, [showPatrons, loadPatrons]);

  useEffect(() => {
    if (!hasUpdate || !latestVersion) return;
    if (dismissedForSession) return;
    setShowUpdateModal(true);
  }, [hasUpdate, latestVersion, dismissedForSession]);

  const handleSkipUpdate = useCallback(() => {
    setShowUpdateModal(false);
    if (!latestVersion) return;
    void skipAppUpdateForSession(latestVersion).catch(() => {});
  }, [latestVersion]);

  const handleStartUpdate = useCallback(async () => {
    if (!autoUpdateSupported || updateApplying) return;
    setUpdateApplying(true);
    setUpdateApplyStarted(false);
    setUpdateApplyError("");
    try {
      const result = await applyAppUpdate();
      if (!result.ok) {
        throw new Error(result.message || t("updateModalFailed"));
      }
      setUpdateApplyStarted(true);
    } catch (err) {
      const reason = err instanceof Error ? err.message : t("updateModalFailed");
      setUpdateApplyError(reason);
      setUpdateApplying(false);
    }
  }, [autoUpdateSupported, updateApplying, t]);

  useEffect(() => {
    if (!updateApplying || !updateApplyStarted) return;
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | null = null;
    let sawDisconnect = false;
    let attempts = 0;
    const maxAttempts = 90;

    const poll = async () => {
      if (cancelled) return;
      attempts++;
      try {
        const status = await getUpdateCheckStatus();
        if (cancelled) return;

        const current = (status.current_version || "").trim();
        const versionChanged = current !== "" && current !== appVersion;
        const noPendingUpdate = !status.has_update && !status.check_error;
        if (sawDisconnect || versionChanged || noPendingUpdate) {
          window.location.reload();
          return;
        }
      } catch {
        // Old backend can disappear before the updated process starts.
        sawDisconnect = true;
      }

      if (attempts >= maxAttempts) {
        setUpdateApplying(false);
        setUpdateApplyStarted(false);
        setUpdateApplyError(t("updateModalFinalizeTimeout"));
        return;
      }
      timer = setTimeout(() => {
        void poll();
      }, 1000);
    };

    void poll();
    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
    };
  }, [updateApplying, updateApplyStarted, appVersion, t]);

  // Load config on mount
  useEffect(() => {
    getConfig()
      .then((cfg) => {
        setParams((prev) => ({
          ...prev,
          system_name: cfg.system_name || prev.system_name,
          ignored_system_ids:
            cfg.ignored_system_ids ?? prev.ignored_system_ids ?? [],
          cargo_capacity: cfg.cargo_capacity ?? prev.cargo_capacity,
          buy_radius: cfg.buy_radius ?? prev.buy_radius,
          sell_radius: cfg.sell_radius ?? prev.sell_radius,
          min_margin: cfg.min_margin ?? prev.min_margin,
          sales_tax_percent: cfg.sales_tax_percent ?? prev.sales_tax_percent,
          broker_fee_percent: cfg.broker_fee_percent ?? prev.broker_fee_percent,
          split_trade_fees: cfg.split_trade_fees ?? prev.split_trade_fees,
          buy_broker_fee_percent:
            cfg.buy_broker_fee_percent ?? prev.buy_broker_fee_percent,
          sell_broker_fee_percent:
            cfg.sell_broker_fee_percent ??
            cfg.broker_fee_percent ??
            prev.sell_broker_fee_percent,
          buy_sales_tax_percent:
            cfg.buy_sales_tax_percent ?? prev.buy_sales_tax_percent,
          sell_sales_tax_percent:
            cfg.sell_sales_tax_percent ??
            cfg.sales_tax_percent ??
            prev.sell_sales_tax_percent,
          min_daily_volume: cfg.min_daily_volume ?? prev.min_daily_volume,
          max_investment: cfg.max_investment ?? prev.max_investment,
          min_item_profit: cfg.min_item_profit ?? prev.min_item_profit,
          min_s2b_per_day: cfg.min_s2b_per_day ?? prev.min_s2b_per_day,
          min_bfs_per_day: cfg.min_bfs_per_day ?? prev.min_bfs_per_day,
          min_s2b_bfs_ratio:
            cfg.min_s2b_bfs_ratio ?? prev.min_s2b_bfs_ratio,
          max_s2b_bfs_ratio:
            cfg.max_s2b_bfs_ratio ?? prev.max_s2b_bfs_ratio,
          min_route_security: cfg.min_route_security ?? prev.min_route_security,
          avg_price_period: cfg.avg_price_period ?? prev.avg_price_period,
          min_period_roi: cfg.min_period_roi ?? prev.min_period_roi,
          max_dos: cfg.max_dos ?? prev.max_dos,
          min_demand_per_day:
            cfg.min_demand_per_day ?? prev.min_demand_per_day,
          purchase_demand_days:
            cfg.purchase_demand_days ?? prev.purchase_demand_days,
          shipping_cost_per_m3_jump:
            cfg.shipping_cost_per_m3_jump ?? prev.shipping_cost_per_m3_jump,
          source_regions: cfg.source_regions ?? prev.source_regions,
          target_region: cfg.target_region ?? prev.target_region,
          target_market_system:
            cfg.target_market_system ?? prev.target_market_system,
          target_market_location_id:
            cfg.target_market_location_id ?? prev.target_market_location_id,
          category_ids: cfg.category_ids ?? prev.category_ids,
          sell_order_mode: cfg.sell_order_mode ?? prev.sell_order_mode,
        }));
        setAlertChannels({
          telegram: cfg.alert_telegram ?? false,
          discord: cfg.alert_discord ?? false,
          desktop: cfg.alert_desktop ?? true,
        });
        setAlertTelegramToken(cfg.alert_telegram_token ?? "");
        setAlertTelegramChatID(cfg.alert_telegram_chat_id ?? "");
        setAlertDiscordWebhook(cfg.alert_discord_webhook ?? "");
      })
      .catch(() => {})
      .finally(() => {
        configLoadedRef.current = true;
      });
  }, []);

  // On mount: check localStorage for a recent region scan (< 4 hours old) and offer restore
  useEffect(() => {
    try {
      const raw = localStorage.getItem("eve_flipper_region_scan");
      if (!raw) return;
      const parsed = JSON.parse(raw) as { ts?: number; results?: unknown[] };
      const normalized = normalizeRegionalResults(parsed?.results ?? []);
      if (normalized.length === 0) {
        localStorage.removeItem("eve_flipper_region_scan");
        return;
      }
      const ageMs = Date.now() - (parsed.ts ?? 0);
      if (ageMs > 4 * 60 * 60 * 1000) return; // older than 4 hours → skip
      setRegionRestorePrompt({ ts: parsed.ts ?? Date.now(), results: normalized });
    } catch {
      // ignore parse errors
    }
  }, []);

  // Save config on param change (debounced) — only after initial config is loaded
  const saveTimerRef = useRef<ReturnType<typeof setTimeout>>(undefined);
  useEffect(() => {
    if (!configLoadedRef.current) return;
    clearTimeout(saveTimerRef.current);
    saveTimerRef.current = setTimeout(() => {
      updateConfig({
        ...params,
        alert_telegram: alertChannels.telegram,
        alert_discord: alertChannels.discord,
        alert_desktop: alertChannels.desktop,
        alert_telegram_token: alertTelegramToken,
        alert_telegram_chat_id: alertTelegramChatID,
        alert_discord_webhook: alertDiscordWebhook,
      }).catch(() => {});
    }, 500);
    return () => clearTimeout(saveTimerRef.current);
  }, [params, alertChannels, alertTelegramToken, alertTelegramChatID, alertDiscordWebhook]);

  const handleScan = useCallback(async () => {
    if (scanning) {
      abortRef.current?.abort();
      return;
    }

    const currentTab = tab;
    const controller = new AbortController();
    abortRef.current = controller;
    setScanning(true);
    setProgress(t("scanStarting"));

    // Clear previous results immediately so the user sees a fresh scan
    if (currentTab === "contracts") {
      setContractResults([]);
      setContractScanCompleted(false);
    } else if (currentTab === "radius") {
      setRadiusResults([]);
    } else if (currentTab === "region") {
      setRegionResults([]);
    }

    try {
      const triggerDesktopAlerts = async (
        rows: Array<{
          TypeID: number;
          TypeName: string;
          MarginPercent: number;
          TotalProfit: number;
          ProfitPerUnit: number;
          DailyVolume: number;
        }>,
      ) => {
        if (!alertChannels.desktop || rows.length === 0) return;
        try {
          const wl = await getWatchlist();
          const now = Date.now();
          for (const item of wl) {
            const metric =
              item.alert_metric === "total_profit" ||
              item.alert_metric === "profit_per_unit" ||
              item.alert_metric === "daily_volume" ||
              item.alert_metric === "margin_percent"
                ? item.alert_metric
                : "margin_percent";
            const threshold = Math.max(
              0,
              item.alert_threshold ?? item.alert_min_margin ?? 0,
            );
            const enabled =
              typeof item.alert_enabled === "boolean"
                ? item.alert_enabled
                : threshold > 0;
            if (!enabled || threshold <= 0) continue;

            const match = rows.find((r) => r.TypeID === item.type_id);
            if (!match) continue;

            const current =
              metric === "margin_percent"
                ? match.MarginPercent
                : metric === "total_profit"
                  ? match.TotalProfit
                  : metric === "profit_per_unit"
                    ? match.ProfitPerUnit
                    : match.DailyVolume;

            if (current < threshold) continue;

            const cooldownKey = `${item.type_id}:${metric}:${threshold}`;
            const lastSentAt =
              desktopAlertCooldownRef.current.get(cooldownKey) ?? 0;
            if (now - lastSentAt < 3_600_000) continue;
            desktopAlertCooldownRef.current.set(cooldownKey, now);

            const metricLabel =
              metric === "margin_percent"
                ? t("watchlistMetricMargin")
                : metric === "total_profit"
                  ? t("watchlistMetricTotalProfit")
                  : metric === "profit_per_unit"
                    ? t("watchlistMetricProfitPerUnit")
                    : t("watchlistMetricDailyVolume");
            const currentText =
              metric === "margin_percent"
                ? `${current.toFixed(2)}%`
                : metric === "daily_volume"
                  ? `${Math.round(current).toLocaleString()}`
                  : formatISK(current);
            const thresholdText =
              metric === "margin_percent"
                ? `${threshold.toFixed(2)}%`
                : metric === "daily_volume"
                  ? `${Math.round(threshold).toLocaleString()}`
                  : formatISK(threshold);
            const msg = `${match.TypeName}: ${metricLabel} ${currentText} >= ${thresholdText}`;

            addToast(msg, "success");
            if ("Notification" in window) {
              if (Notification.permission === "granted") {
                new Notification(t("appTitle"), { body: msg });
              } else if (Notification.permission === "default") {
                Notification.requestPermission().then((perm) => {
                  if (perm === "granted") {
                    new Notification(t("appTitle"), { body: msg });
                  }
                });
              }
            }
          }
        } catch (err) {
          const reason = err instanceof Error ? err.message : t("watchlistError");
          addToast(`${t("watchlistError")}: ${reason}`, "error", 3000);
        }
      };

      if (currentTab === "contracts") {
        let meta: StationCacheMeta | undefined;
        const results = await scanContracts(
          params,
          setProgress,
          controller.signal,
          (m) => {
            meta = m;
          },
        );
        setContractResults(results);
        setContractCacheMeta(meta ?? null);
        setContractScanCompleted(true);
      } else if (currentTab === "radius") {
        let meta: StationCacheMeta | undefined;
        const radiusParams =
          (params.restrict_to_target_market ?? true)
            ? params
            : { ...params, target_market_system: "", target_market_location_id: 0 };
        const results = await scan(
          radiusParams,
          setProgress,
          controller.signal,
          (m) => {
            meta = m;
          },
        );
        setRadiusResults(results);
        setRadiusCacheMeta(meta ?? null);
        await triggerDesktopAlerts(results);
      } else if (currentTab === "region") {
        let meta: StationCacheMeta | undefined;
        const rows = await scanRegionalDayTrader(
          params,
          setProgress,
          controller.signal,
          (m) => {
            meta = m;
          },
        );
        const normalizedRows = normalizeRegionalResults(rows as unknown[]);
        setRegionResults(normalizedRows);
        setRegionCacheMeta(meta ?? null);
        queueRegionScanPersistence(normalizedRows);

        const flatRows: Array<{
          TypeID: number;
          TypeName: string;
          MarginPercent: number;
          TotalProfit: number;
          ProfitPerUnit: number;
          DailyVolume: number;
        }> = normalizedRows.map((row) => ({
          TypeID: row.TypeID,
          TypeName: row.TypeName,
          MarginPercent: row.MarginPercent,
          TotalProfit: row.DayNowProfit ?? row.TotalProfit ?? 0,
          ProfitPerUnit:
            row.ProfitPerUnit ??
            (row.UnitsToBuy > 0
              ? (row.DayNowProfit ?? row.TotalProfit ?? 0) / row.UnitsToBuy
              : 0),
          DailyVolume:
            row.DailyVolume ??
            Math.round(row.DayTargetDemandPerDay ?? 0),
        }));
        await triggerDesktopAlerts(flatRows);
      } else {
        // Keep old behavior for any legacy tab alias.
        let meta: StationCacheMeta | undefined;
        const results = await scanMultiRegion(
          params,
          setProgress,
          controller.signal,
          (m) => {
            meta = m;
          },
        );
        setRegionResults(results);
        setRegionCacheMeta(meta ?? null);
        await triggerDesktopAlerts(results);
      }
    } catch (e: unknown) {
      if (e instanceof Error && e.name !== "AbortError") {
        setProgress(t("errorPrefix") + e.message);
      }
    } finally {
      setScanning(false);
    }
  }, [scanning, tab, params, t, addToast, alertChannels, queueRegionScanPersistence]);

  // Auto-refresh: when enabled and radius cache expires, re-trigger scan automatically
  useEffect(() => {
    if (!autoRefreshRadius || tab !== "radius") return;
    const CHECK_INTERVAL = 15_000; // check every 15s
    const COOLDOWN_MS = 90_000; // avoid loops on stale metadata snapshots
    const timer = window.setInterval(() => {
      if (scanning) return;
      if (!radiusCacheMeta?.next_expiry_at) return;
      const expiresAt = Date.parse(radiusCacheMeta.next_expiry_at);
      if (!Number.isFinite(expiresAt) || Date.now() < expiresAt) return;

      const signature = `${radiusCacheMeta.current_revision ?? 0}:${radiusCacheMeta.next_expiry_at}`;
      const now = Date.now();
      const sameSnapshot = radiusAutoRefreshSignatureRef.current === signature;
      if (
        sameSnapshot &&
        now - radiusAutoRefreshLastRunRef.current < COOLDOWN_MS
      ) {
        return;
      }

      radiusAutoRefreshSignatureRef.current = signature;
      radiusAutoRefreshLastRunRef.current = now;
      void handleScan();
    }, CHECK_INTERVAL);
    return () => window.clearInterval(timer);
  }, [autoRefreshRadius, tab, scanning, radiusCacheMeta, handleScan]);

  // Auto-refresh: when enabled and region cache expires, re-trigger scan automatically
  useEffect(() => {
    if (!autoRefreshRegion || tab !== "region") return;
    const CHECK_INTERVAL = 15_000; // check every 15s
    const COOLDOWN_MS = 90_000; // avoid loops on stale metadata snapshots
    const timer = window.setInterval(() => {
      if (scanning) return;
      if (!regionCacheMeta?.next_expiry_at) return;
      const expiresAt = Date.parse(regionCacheMeta.next_expiry_at);
      if (!Number.isFinite(expiresAt) || Date.now() < expiresAt) return;

      const signature = `${regionCacheMeta.current_revision ?? 0}:${regionCacheMeta.next_expiry_at}`;
      const now = Date.now();
      const sameSnapshot = regionAutoRefreshSignatureRef.current === signature;
      if (
        sameSnapshot &&
        now - regionAutoRefreshLastRunRef.current < COOLDOWN_MS
      ) {
        return;
      }

      regionAutoRefreshSignatureRef.current = signature;
      regionAutoRefreshLastRunRef.current = now;
      void handleScan();
    }, CHECK_INTERVAL);
    return () => window.clearInterval(timer);
  }, [autoRefreshRegion, tab, scanning, regionCacheMeta, handleScan]);

  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);

  useEffect(() => {
    const minVisibleMs = 1000;
    const fadeMs = 420;
    const fallbackMs = 2600;
    const mountedAt = performance.now();
    let fadeTimer = 0;
    let hideTimer = 0;
    let fallbackTimer = 0;
    let finished = false;

    const closeSplash = () => {
      if (finished) return;
      finished = true;
      const elapsed = performance.now() - mountedAt;
      const wait = Math.max(0, minVisibleMs - elapsed);
      fadeTimer = window.setTimeout(() => {
        setBootSplashState("fading");
        hideTimer = window.setTimeout(() => {
          setBootSplashState("hidden");
        }, fadeMs);
      }, wait);
    };

    if (document.readyState === "complete") {
      closeSplash();
    } else {
      window.addEventListener("load", closeSplash, { once: true });
      fallbackTimer = window.setTimeout(closeSplash, fallbackMs);
    }

    return () => {
      finished = true;
      window.removeEventListener("load", closeSplash);
      window.clearTimeout(fadeTimer);
      window.clearTimeout(hideTimer);
      window.clearTimeout(fallbackTimer);
    };
  }, []);

  // In desktop runtimes (Wails/Tauri), force external links to open in the
  // system browser instead of inside the embedded WebView.
  useEffect(() => {
    const onDocumentClick = (event: globalThis.MouseEvent) => {
      if (event.defaultPrevented) return;
      const target = event.target;
      if (!(target instanceof Element)) return;

      const anchor = target.closest("a[href]") as HTMLAnchorElement | null;
      if (!anchor) return;

      const href = (anchor.getAttribute("href") || "").trim();
      if (!/^https?:\/\//i.test(href)) return;

      const { isDesktop } = getDesktopRuntimeFlags();
      if (!isDesktop) return;

      event.preventDefault();
      void openExternalURL(href);
    };

    document.addEventListener("click", onDocumentClick);
    return () => document.removeEventListener("click", onDocumentClick);
  }, [openExternalURL]);

  useEffect(() => {
    if (tab !== "region" || regionDefaultsAppliedRef.current) return;
    setParams((prev) => {
      const next = { ...prev };
      // Keep EG-like behavior for now: force Min Period ROI to 0 on region tab init.
      next.min_period_roi = 0;
      if (next.min_demand_per_day == null) next.min_demand_per_day = 1;
      if (next.max_dos == null) next.max_dos = 180;
      if ((next.purchase_demand_days ?? 0) <= 0) next.purchase_demand_days = 0.5;
      return next;
    });
    regionDefaultsAppliedRef.current = true;
  }, [tab]);

  useEffect(() => {
    return () => clearTimeout(regionPersistTimerRef.current);
  }, []);

  function queueRegionScanPersistence(rows: FlipResult[]): void {
    clearTimeout(regionPersistTimerRef.current);
    regionPersistTimerRef.current = window.setTimeout(() => {
      try {
        if (rows.length > REGION_SCAN_RESTORE_MAX_ROWS) {
          localStorage.removeItem("eve_flipper_region_scan");
          return;
        }
        localStorage.setItem(
          "eve_flipper_region_scan",
          JSON.stringify({ ts: Date.now(), results: rows }),
        );
      } catch {
        // ignore storage quota and access errors
      }
    }, 0);
  }

  return (
    <>
      <div
        className={`h-screen flex flex-col gap-1.5 sm:gap-3 p-1.5 sm:p-4 select-none overflow-hidden transition-[opacity,transform,filter] duration-500 ease-out ${
          bootSplashState === "hidden"
            ? "opacity-100 scale-100 blur-0"
            : "opacity-0 scale-[0.995] blur-[1px]"
        } ${bootSplashState !== "hidden" ? "pointer-events-none" : ""}`}
      >
      {/* Header */}
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2 sm:gap-3 min-w-0">
          <div className="min-w-0 flex items-center gap-2 sm:gap-2.5 px-2 sm:px-2.5 py-1 bg-eve-panel border border-eve-border rounded-sm">
            <div className="flex items-center justify-center w-6 h-6 rounded-sm bg-eve-dark border border-eve-border/70">
              <img
                src={logo}
                alt="EVE Flipper logo"
                className="w-4 h-4 shrink-0"
              />
            </div>
            <div className="flex items-center gap-2 min-w-0">
              <h1 className="text-sm sm:text-lg font-semibold text-eve-accent tracking-wide uppercase whitespace-nowrap">
                {t("appTitle")}
              </h1>
              <span
                className="hidden sm:inline-flex px-1.5 py-0.5 text-[10px] font-mono bg-eve-accent/10 text-eve-accent border border-eve-accent/30 rounded-sm"
                title={
                  hasUpdate && latestVersion
                    ? t("versionUpdateHint", { latest: latestVersion })
                    : ""
                }
              >
                {appVersion}
              </span>
              {hasUpdate && latestVersion && (
                <a
                  href={releaseURL || "https://github.com/ilyaux/Eve-flipper/releases/latest"}
                  target="_blank"
                  rel="noreferrer"
                  className="hidden sm:inline-flex px-1.5 py-0.5 text-[9px] uppercase tracking-wide rounded-sm bg-eve-warning/10 text-eve-warning border border-eve-warning/40 hover:bg-eve-warning/20 transition-colors"
                >
                  {t("versionUpdateAvailable")}
                </a>
              )}
            </div>
          </div>
          <div className="hidden sm:flex items-center gap-1.5 text-eve-dim">
            <a
              href="https://github.com/ilyaux/Eve-flipper"
              target="_blank"
              rel="noreferrer"
              className="p-1 rounded-sm hover:bg-eve-panel hover:text-eve-accent transition-colors"
              aria-label="GitHub"
            >
              <svg
                className="w-4 h-4"
                viewBox="0 0 16 16"
                fill="currentColor"
                aria-hidden="true"
              >
                <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27s1.36.09 2 .27c1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8" />
              </svg>
            </a>
            <a
              href="https://www.patreon.com/cw/EVEFlipper"
              target="_blank"
              rel="noreferrer"
              className="p-1 rounded-sm hover:bg-eve-panel hover:text-eve-accent transition-colors"
              aria-label={t("supportPatreon")}
              title={t("supportPatreon")}
            >
            <svg
              className="w-4 h-4"
              viewBox="0 0 1080 1080"
              fill="currentColor"
              aria-hidden="true"
            >
              <path d="M1033.05 324.45c-.19-137.9-107.59-250.92-233.6-291.7-156.48-50.64-362.86-43.3-512.28 27.2-181.1 85.46-237.99 272.66-240.11 459.36-1.74 153.5 13.58 557.79 241.62 560.67 169.44 2.15 194.67-216.18 273.07-321.33 55.78-74.81 127.6-95.94 216.01-117.82 151.95-37.61 255.51-157.53 255.29-316.38z" />
            </svg>
          </a>
            <button
              type="button"
              onClick={() => setShowPatrons(true)}
              className="p-1 rounded-sm hover:bg-eve-panel hover:text-eve-accent transition-colors"
              aria-label={t("patronsOpenList")}
              title={t("patronsOpenList")}
            >
              <svg className="w-4 h-4" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                <path d="M16 11a4 4 0 1 0-3.999-4A4 4 0 0 0 16 11zm-8 0a3 3 0 1 0-3-3 3 3 0 0 0 3 3zm0 2c-2.67 0-8 1.34-8 4v2h10v-2c0-1.2.53-2.29 1.4-3.16A12.6 12.6 0 0 0 8 13zm8 0c-2.67 0-8 1.34-8 4v2h16v-2c0-2.66-5.33-4-8-4z" />
              </svg>
            </button>
            <a
              href="https://discord.gg/rnR2bw6XXX"
              target="_blank"
              rel="noreferrer"
              className="p-1 rounded-sm hover:bg-eve-panel hover:text-eve-accent transition-colors"
              aria-label="Discord"
            >
              <svg
                className="w-4 h-4 discord-icon-animated"
                viewBox="0 0 16 16"
                fill="currentColor"
                aria-hidden="true"
              >
                <path d="M13.545 2.907a13.2 13.2 0 0 0-3.257-1.011.05.05 0 0 0-.052.025c-.141.25-.297.577-.406.833a12.2 12.2 0 0 0-3.658 0 8 8 0 0 0-.412-.833.05.05 0 0 0-.052-.025c-1.125.194-2.22.534-3.257 1.011a.04.04 0 0 0-.021.018C.356 6.024-.213 9.047.066 12.032q.003.022.021.037a13.3 13.3 0 0 0 3.995 2.02.05.05 0 0 0 .056-.019q.463-.63.818-1.329a.05.05 0 0 0-.01-.059l-.018-.011a9 9 0 0 1-1.248-.595.05.05 0 0 1-.02-.066l.015-.019q.127-.095.248-.195a.05.05 0 0 1 .051-.007c2.619 1.196 5.454 1.196 8.041 0a.05.05 0 0 1 .053.007q.121.1.248.195a.05.05 0 0 1-.004.085 8 8 0 0 1-1.249.594.05.05 0 0 0-.03.03.05.05 0 0 0 .003.041c.24.465.515.909.817 1.329a.05.05 0 0 0 .056.019 13.2 13.2 0 0 0 4.001-2.02.05.05 0 0 0 .021-.037c.334-3.451-.559-6.449-2.366-9.106a.03.03 0 0 0-.02-.019m-8.198 7.307c-.789 0-1.438-.724-1.438-1.612s.637-1.613 1.438-1.613c.807 0 1.45.73 1.438 1.613 0 .888-.637 1.612-1.438 1.612m5.316 0c-.788 0-1.438-.724-1.438-1.612s.637-1.613 1.438-1.613c.807 0 1.451.73 1.438 1.613 0 .888-.631 1.612-1.438 1.612" />
              </svg>
            </a>
            <a
              href="https://discord.gg/rnR2bw6XXX"
              target="_blank"
              rel="noreferrer"
              className="group inline-flex items-center gap-1.5 h-7 px-2 rounded-sm border border-[#5865F2]/45 bg-[#5865F2]/12 text-[#9ca8ff] hover:bg-[#5865F2]/20 hover:text-[#c7ceff] transition-colors"
              aria-label={t("discordCta")}
              title={t("discordPitch")}
            >
              <span className="text-[10px] uppercase tracking-[0.14em]">{t("discordCta")}</span>
            </a>
          </div>
        </div>
        <div className="flex items-center gap-1 sm:gap-2 shrink-0">
          {/* Desktop controls — hidden on mobile */}
          <div className="hidden sm:flex items-center gap-2">
            {/* Watchlist button */}
            <button
              onClick={() => setShowWatchlist(true)}
              className="flex items-center gap-1.5 h-[34px] px-3 bg-eve-panel border border-eve-border rounded-sm text-xs text-eve-dim hover:text-eve-accent hover:border-eve-accent/50 transition-colors"
              title={t("tabWatchlist")}
              aria-label={t("tabWatchlist")}
            >
              <span aria-hidden="true">&#11088;</span>
              <span>{t("tabWatchlist")}</span>
            </button>
            {/* History button */}
            <button
              onClick={() => setShowHistory(true)}
              className="flex items-center gap-1.5 h-[34px] px-3 bg-eve-panel border border-eve-border rounded-sm text-xs text-eve-dim hover:text-eve-accent hover:border-eve-accent/50 transition-colors"
              title={t("tabHistory")}
              aria-label={t("tabHistory")}
            >
              <span aria-hidden="true">&#128203;</span>
              <span>{t("tabHistory")}</span>
            </button>
            {/* Auth chip */}
            <div className="flex items-center gap-1 h-[34px] px-3 bg-eve-panel border border-eve-border rounded-sm text-xs">
              {authStatus.logged_in ? (
                <>
                  <button
                    onClick={() => setShowCharacter(true)}
                    className="flex items-center gap-2 hover:bg-eve-dark/50 rounded-sm px-1 py-0.5 transition-colors"
                    title={t("charViewInfo")}
                  >
                    <img
                      src={`https://images.evetech.net/characters/${authStatus.character_id}/portrait?size=32`}
                      alt=""
                      className="w-5 h-5 rounded-sm"
                    />
                    <span className="text-eve-accent font-medium">
                      {authStatus.character_name}
                    </span>
                    {characterCount > 1 && (
                      <span className="text-[10px] text-eve-dim bg-eve-dark px-1.5 py-0.5 rounded-sm">
                        {characterCount}
                      </span>
                    )}
                  </button>
                  <button
                    onClick={handleLogin}
                    disabled={loginPolling}
                    className="ml-1 p-1 text-eve-dim hover:text-eve-accent hover:bg-eve-dark/50 rounded-sm transition-colors disabled:opacity-60"
                    title={t("charAddCharacter")}
                    aria-label={t("charAddCharacter")}
                  >
                    <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 5v14m-7-7h14" />
                    </svg>
                  </button>
                  <button
                    onClick={handleLogout}
                    className="ml-1 p-1 text-eve-dim hover:text-eve-error hover:bg-eve-dark/50 rounded-sm transition-colors"
                    title={t("logout")}
                    aria-label={t("logout")}
                  >
                    <svg
                      className="w-3.5 h-3.5"
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                      aria-hidden="true"
                    >
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                        d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1"
                      />
                    </svg>
                  </button>
                </>
              ) : (
                <button
                  onClick={handleLogin}
                  disabled={loginPolling}
                  className="text-eve-accent hover:text-eve-accent-hover transition-colors disabled:opacity-60"
                >
                  {loginPolling ? t("loginWaiting") : t("loginEve")}
                </button>
              )}
            </div>
            <StatusBar />
          </div>
          <ThemeSwitcher />
          <LanguageSwitcher />
          {/* Hamburger menu — visible only on mobile */}
          <button
            onClick={() => setMobileMenuOpen((v) => !v)}
            className="sm:hidden flex items-center justify-center h-[34px] w-[34px] rounded-sm
                       bg-eve-panel border border-eve-border hover:border-eve-accent/50 transition-colors"
            aria-label="Menu"
          >
            <svg
              className="w-4 h-4 text-eve-dim"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
              strokeWidth={2}
            >
              {mobileMenuOpen ? (
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d="M6 18L18 6M6 6l12 12"
                />
              ) : (
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d="M4 6h16M4 12h16M4 18h16"
                />
              )}
            </svg>
          </button>
        </div>
      </div>

      {/* Mobile menu dropdown */}
      {mobileMenuOpen && (
        <div className="sm:hidden flex flex-wrap items-center gap-1.5 px-1 pb-1 -mt-0.5 animate-in fade-in">
          <button
            onClick={() => {
              setShowWatchlist(true);
              setMobileMenuOpen(false);
            }}
            className="flex items-center gap-1.5 h-9 px-3 bg-eve-panel border border-eve-border rounded-sm text-xs text-eve-dim"
          >
            <span>&#11088;</span>
            <span>{t("tabWatchlist")}</span>
          </button>
          <button
            onClick={() => {
              setShowHistory(true);
              setMobileMenuOpen(false);
            }}
            className="flex items-center gap-1.5 h-9 px-3 bg-eve-panel border border-eve-border rounded-sm text-xs text-eve-dim"
          >
            <span>&#128203;</span>
            <span>{t("tabHistory")}</span>
          </button>
          <div className="flex items-center gap-1 h-9 px-3 bg-eve-panel border border-eve-border rounded-sm text-xs">
            {authStatus.logged_in ? (
              <>
                <button
                  onClick={() => {
                    setShowCharacter(true);
                    setMobileMenuOpen(false);
                  }}
                  className="flex items-center gap-2"
                >
                  <img
                    src={`https://images.evetech.net/characters/${authStatus.character_id}/portrait?size=32`}
                    alt=""
                    className="w-5 h-5 rounded-sm"
                  />
                  <span className="text-eve-accent font-medium">
                    {authStatus.character_name}
                  </span>
                  {characterCount > 1 && (
                    <span className="text-[10px] text-eve-dim bg-eve-dark px-1.5 py-0.5 rounded-sm">
                      {characterCount}
                    </span>
                  )}
                </button>
                <button
                  onClick={handleLogin}
                  disabled={loginPolling}
                  className="ml-1 p-1 text-eve-dim hover:text-eve-accent disabled:opacity-60"
                  title={t("charAddCharacter")}
                  aria-label={t("charAddCharacter")}
                >
                  <svg
                    className="w-3.5 h-3.5"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      strokeWidth={2}
                      d="M12 5v14m-7-7h14"
                    />
                  </svg>
                </button>
                <button
                  onClick={handleLogout}
                  className="ml-1 p-1 text-eve-dim hover:text-eve-error"
                >
                  <svg
                    className="w-3.5 h-3.5"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      strokeWidth={2}
                      d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1"
                    />
                  </svg>
                </button>
              </>
            ) : (
              <button
                onClick={handleLogin}
                disabled={loginPolling}
                className="text-eve-accent disabled:opacity-60"
              >
                {loginPolling ? t("loginWaiting") : t("loginEve")}
              </button>
            )}
          </div>
          <a
            href="https://github.com/ilyaux/Eve-flipper"
            target="_blank"
            rel="noreferrer"
            className="flex items-center justify-center h-9 w-9 bg-eve-panel border border-eve-border rounded-sm text-eve-dim hover:text-eve-accent"
            aria-label="GitHub"
          >
            <svg className="w-4 h-4" viewBox="0 0 16 16" fill="currentColor">
              <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27s1.36.09 2 .27c1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8" />
            </svg>
          </a>
          <a
            href="https://www.patreon.com/cw/EVEFlipper"
            target="_blank"
            rel="noreferrer"
            className="flex items-center justify-center h-9 w-9 bg-eve-panel border border-eve-border rounded-sm text-eve-dim hover:text-eve-accent"
            aria-label={t("supportPatreon")}
            title={t("supportPatreon")}
          >
            <svg className="w-4 h-4" viewBox="0 0 1080 1080" fill="currentColor">
              <path d="M1033.05 324.45c-.19-137.9-107.59-250.92-233.6-291.7-156.48-50.64-362.86-43.3-512.28 27.2-181.1 85.46-237.99 272.66-240.11 459.36-1.74 153.5 13.58 557.79 241.62 560.67 169.44 2.15 194.67-216.18 273.07-321.33 55.78-74.81 127.6-95.94 216.01-117.82 151.95-37.61 255.51-157.53 255.29-316.38z" />
            </svg>
          </a>
          <button
            type="button"
            onClick={() => {
              setShowPatrons(true);
              setMobileMenuOpen(false);
            }}
            className="flex items-center justify-center h-9 w-9 bg-eve-panel border border-eve-border rounded-sm text-eve-dim hover:text-eve-accent"
            aria-label={t("patronsOpenList")}
            title={t("patronsOpenList")}
          >
            <svg className="w-4 h-4" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
              <path d="M16 11a4 4 0 1 0-3.999-4A4 4 0 0 0 16 11zm-8 0a3 3 0 1 0-3-3 3 3 0 0 0 3 3zm0 2c-2.67 0-8 1.34-8 4v2h10v-2c0-1.2.53-2.29 1.4-3.16A12.6 12.6 0 0 0 8 13zm8 0c-2.67 0-8 1.34-8 4v2h16v-2c0-2.66-5.33-4-8-4z" />
            </svg>
          </button>
          <a
            href="https://discord.gg/rnR2bw6XXX"
            target="_blank"
            rel="noreferrer"
            className="flex items-center justify-center h-9 w-9 bg-eve-panel border border-eve-border rounded-sm text-eve-dim hover:text-eve-accent"
            aria-label="Discord"
          >
            <svg className="w-4 h-4 discord-icon-animated" viewBox="0 0 16 16" fill="currentColor">
              <path d="M13.545 2.907a13.2 13.2 0 0 0-3.257-1.011.05.05 0 0 0-.052.025c-.141.25-.297.577-.406.833a12.2 12.2 0 0 0-3.658 0 8 8 0 0 0-.412-.833.05.05 0 0 0-.052-.025c-1.125.194-2.22.534-3.257 1.011a.04.04 0 0 0-.021.018C.356 6.024-.213 9.047.066 12.032q.003.022.021.037a13.3 13.3 0 0 0 3.995 2.02.05.05 0 0 0 .056-.019q.463-.63.818-1.329a.05.05 0 0 0-.01-.059l-.018-.011a9 9 0 0 1-1.248-.595.05.05 0 0 1-.02-.066l.015-.019q.127-.095.248-.195a.05.05 0 0 1 .051-.007c2.619 1.196 5.454 1.196 8.041 0a.05.05 0 0 1 .053.007q.121.1.248.195a.05.05 0 0 1-.004.085 8 8 0 0 1-1.249.594.05.05 0 0 0-.03.03.05.05 0 0 0 .003.041c.24.465.515.909.817 1.329a.05.05 0 0 0 .056.019 13.2 13.2 0 0 0 4.001-2.02.05.05 0 0 0 .021-.037c.334-3.451-.559-6.449-2.366-9.106a.03.03 0 0 0-.02-.019m-8.198 7.307c-.789 0-1.438-.724-1.438-1.612s.637-1.613 1.438-1.613c.807 0 1.45.73 1.438 1.613 0 .888-.637 1.612-1.438 1.612m5.316 0c-.788 0-1.438-.724-1.438-1.612s.637-1.613 1.438-1.613c.807 0 1.451.73 1.438 1.613 0 .888-.631 1.612-1.438 1.612" />
            </svg>
          </a>
          <a
            href="https://discord.gg/rnR2bw6XXX"
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center h-9 px-3 rounded-sm border border-[#5865F2]/45 bg-[#5865F2]/12 text-[#9ca8ff] text-[11px] uppercase tracking-[0.12em]"
            aria-label={t("discordCta")}
            title={t("discordPitch")}
          >
            {t("discordCta")}
          </a>
          <StatusBar />
        </div>
      )}

      {/* Parameters - shown for tabs that use global scan params (Flipper, Regional, Contracts, Route) */}
      {(tab === "radius" ||
        tab === "region" ||
        tab === "contracts" ||
        tab === "route") && (
        <ParametersPanel
          params={params}
          onChange={setParams}
          isLoggedIn={authStatus.logged_in}
          tab={tab}
        />
      )}

      {/* Industry doesn't use global params - has its own settings panel */}

      {/* Tabs */}
      <div className="flex-1 flex flex-col min-h-0 bg-eve-panel border border-eve-border rounded-sm">
        <div className="flex items-stretch border-b border-eve-border">
          <div className="flex-1 min-w-0 overflow-x-auto scrollbar-thin snap-x snap-mandatory sm:snap-none">
            <div
              className="flex items-center min-w-max"
              role="tablist"
              aria-label="Scan modes"
            >
              <TabButton
                active={tab === "radius"}
                onClick={() => setTab("radius")}
                label={t("tabRadius")}
              />
              <TabButton
                active={tab === "region"}
                onClick={() => setTab("region")}
                label={t("tabRegion")}
              />
              <TabButton
                active={tab === "contracts"}
                onClick={() => setTab("contracts")}
                label={t("tabContracts")}
              />
              <TabButton
                active={tab === "route"}
                onClick={() => setTab("route")}
                label={t("tabRoute")}
              />
              {/* Visual separator: scan group vs station/industry */}
              <div
                className="h-6 w-px bg-eve-border mx-1 flex-shrink-0"
                aria-hidden="true"
              />
              <TabButton
                active={tab === "station"}
                onClick={() => setTab("station")}
                label={t("tabStation")}
              />
              <TabButton
                active={tab === "industry"}
                onClick={() => setTab("industry")}
                label={t("tabIndustry")}
              />
              <TabButton
                active={tab === "demand"}
                onClick={() => setTab("demand")}
                label={t("tabDemand") || "War Tracker"}
              />
              <TabButton
                active={tab === "plex"}
                onClick={() => setTab("plex")}
                label={t("tabPlex") || "PLEX+"}
              />
              <div className="w-2 sm:w-4 shrink-0" />
            </div>
          </div>

          {tab !== "route" &&
            tab !== "station" &&
            tab !== "industry" &&
            tab !== "demand" &&
            tab !== "plex" && (
              <div className="shrink-0 border-l border-eve-border px-1.5 sm:px-2 py-1 flex items-center">
                <button
                  data-scan-button
                  onClick={handleScan}
                  disabled={
                    tab === "region"
                      ? !params.target_market_system?.trim()
                      : !params.system_name
                  }
                  title="Ctrl+S"
                  className={`px-3 sm:px-4 py-1.5 rounded-sm text-[10px] sm:text-xs font-semibold uppercase tracking-wider transition-all
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
            )}
        </div>

        {/* Results — all tabs stay mounted to preserve state */}
        <div className="flex-1 min-h-0 flex flex-col p-1.5 sm:p-2">
          <div
            className={`flex-1 min-h-0 flex flex-col ${tab === "radius" ? "" : "hidden"}`}
          >
            {tab === "radius" && (
              <div className="shrink-0 flex items-center gap-2 px-2 py-1 text-xs border-b border-eve-border/20">
                <label className="inline-flex items-center gap-1.5 cursor-pointer select-none text-eve-dim hover:text-eve-text transition-colors">
                  <input
                    type="checkbox"
                    checked={autoRefreshRadius}
                    onChange={(e) => setAutoRefreshRadius(e.target.checked)}
                    className="accent-eve-accent"
                  />
                  Auto-refresh
                </label>
                {autoRefreshRadius && (
                  <span className="flex items-center gap-1 text-eve-accent">
                    <span className="w-1.5 h-1.5 rounded-full bg-eve-accent animate-pulse" />
                    active
                  </span>
                )}
              </div>
            )}
            <ScanResultsTable
              results={radiusResults}
              scanning={scanning && tab === "radius"}
              progress={tab === "radius" ? progress : ""}
              cacheMeta={radiusCacheMeta}
              tradeStateTab="radius"
              salesTaxPercent={params.sales_tax_percent}
              brokerFeePercent={params.broker_fee_percent}
              splitTradeFees={params.split_trade_fees}
              buyBrokerFeePercent={params.buy_broker_fee_percent}
              sellBrokerFeePercent={params.sell_broker_fee_percent}
              buySalesTaxPercent={params.buy_sales_tax_percent}
              sellSalesTaxPercent={params.sell_sales_tax_percent}
              isLoggedIn={authStatus.logged_in}
              cargoLimit={params.cargo_capacity}
            />
          </div>
          <div
            className={`flex-1 min-h-0 flex flex-col ${tab === "region" ? "" : "hidden"}`}
          >
            {/* Auto-refresh toggle for region tab */}
            {tab === "region" && (
              <div className="shrink-0 flex items-center gap-2 px-2 py-1 text-xs border-b border-eve-border/20">
                <label className="inline-flex items-center gap-1.5 cursor-pointer select-none text-eve-dim hover:text-eve-text transition-colors">
                  <input
                    type="checkbox"
                    checked={autoRefreshRegion}
                    onChange={(e) => setAutoRefreshRegion(e.target.checked)}
                    className="accent-eve-accent"
                  />
                  Auto-refresh
                </label>
                {autoRefreshRegion && (
                  <span className="flex items-center gap-1 text-eve-accent">
                    <span className="w-1.5 h-1.5 rounded-full bg-eve-accent animate-pulse" />
                    active
                  </span>
                )}
              </div>
            )}
            {/* Restore prompt: offer to reload last scan from localStorage */}
            {regionRestorePrompt && regionResults.length === 0 && !scanning && (
              <div className="shrink-0 flex items-center gap-3 px-3 py-2 bg-eve-accent/10 border-b border-eve-accent/30 text-xs">
                <span className="text-eve-accent">💾</span>
                <span className="text-eve-text flex-1">
                  Previous scan saved{" "}
                  <span className="text-eve-dim">
                    ({new Date(regionRestorePrompt.ts).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })},{" "}
                    {regionRestorePrompt.results.length} items)
                  </span>
                  . Restore it?
                </span>
                <button
                  className="px-2 py-0.5 rounded bg-eve-accent/20 text-eve-accent hover:bg-eve-accent/40 transition-colors"
                  onClick={() => {
                    setRegionResults(regionRestorePrompt.results);
                    setRegionRestorePrompt(null);
                  }}
                >
                  Restore
                </button>
                <button
                  className="px-2 py-0.5 rounded bg-transparent text-eve-dim hover:text-eve-text transition-colors"
                  onClick={() => {
                    setRegionRestorePrompt(null);
                    localStorage.removeItem("eve_flipper_region_scan");
                  }}
                >
                  Dismiss
                </button>
              </div>
            )}
            <ScanResultsTable
              results={regionResults}
              scanning={scanning && tab === "region"}
              progress={tab === "region" ? progress : ""}
              cacheMeta={regionCacheMeta}
              tradeStateTab="region"
              salesTaxPercent={params.sales_tax_percent}
              brokerFeePercent={params.broker_fee_percent}
              splitTradeFees={params.split_trade_fees}
              buyBrokerFeePercent={params.buy_broker_fee_percent}
              sellBrokerFeePercent={params.sell_broker_fee_percent}
              buySalesTaxPercent={params.buy_sales_tax_percent}
              sellSalesTaxPercent={params.sell_sales_tax_percent}
              isLoggedIn={authStatus.logged_in}
              showRegions
              columnProfile="region_eveguru"
              cargoLimit={params.cargo_capacity}
            />
          </div>
          <div
            className={`flex-1 min-h-0 flex flex-col ${tab === "contracts" ? "" : "hidden"}`}
          >
            {/* Contract-specific settings */}
            <div className="shrink-0 mb-2">
              <ContractParametersPanel params={params} onChange={setParams} />
            </div>
            <ContractResultsTable
              results={contractResults}
              scanning={scanning && tab === "contracts"}
              progress={tab === "contracts" ? progress : ""}
              cacheMeta={contractCacheMeta}
              tradeStateTab="contracts"
              excludeRigPriceIfShip={params.exclude_rigs_with_ship ?? true}
              filterHints={contractFilterHints}
              isLoggedIn={authStatus.logged_in}
            />
          </div>
          <div
            className={`flex-1 min-h-0 flex flex-col ${tab === "station" ? "" : "hidden"}`}
          >
            <StationTrading
              params={params}
              onChange={setParams}
              isLoggedIn={authStatus.logged_in}
              loadedResults={stationLoadedResults}
            />
          </div>
          <div
            className={`flex-1 min-h-0 flex flex-col ${tab === "route" ? "" : "hidden"}`}
          >
            <RouteBuilder
              params={params}
              onChange={setParams}
              loadedResults={routeLoadedResults}
              isLoggedIn={authStatus.logged_in}
            />
          </div>
          <div
            className={`flex-1 min-h-0 flex flex-col ${tab === "industry" ? "" : "hidden"}`}
          >
            <IndustryTab isLoggedIn={authStatus.logged_in} />
          </div>
          <div
            className={`flex-1 min-h-0 flex flex-col ${tab === "demand" ? "" : "hidden"}`}
          >
            <WarTracker
              onError={(msg) => addToast(msg, "error")}
              onOpenRegionArbitrage={(regionName) => {
                // Switch to Regional Trade tab and set target region
                setParams((p) => ({ ...p, target_region: regionName }));
                setTab("region");
                addToast(
                  `${t("targetRegionSet") || "Target region set to"} ${regionName}`,
                  "success",
                );
              }}
            />
          </div>
          <div
            className={`flex-1 min-h-0 flex flex-col ${tab === "plex" ? "" : "hidden"}`}
          >
            <PlexTab isLoggedIn={authStatus.logged_in} activeCharacterId={authStatus.character_id} />
          </div>
        </div>
      </div>

      {/* App Update Modal */}
      <Modal
        open={showUpdateModal}
        onClose={() => {
          if (!updateApplying) handleSkipUpdate();
        }}
        title={t("updateModalTitle")}
        width="max-w-xl"
      >
        <div className="p-4 sm:p-5 space-y-3">
          <p className="text-sm text-eve-text">{t("updateModalBody")}</p>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 text-xs">
            <div className="px-2.5 py-2 rounded-sm border border-eve-border bg-eve-panel/60 text-eve-dim">
              {t("updateModalCurrent")}: <span className="text-eve-text">{appVersion}</span>
            </div>
            <div className="px-2.5 py-2 rounded-sm border border-eve-border bg-eve-panel/60 text-eve-dim">
              {t("updateModalLatest")}: <span className="text-eve-accent">{latestVersion ?? "-"}</span>
            </div>
          </div>
          <div className="text-[11px] text-eve-dim">
            {t("updateModalPlatform")}: {autoUpdateSupported ? t("updateModalAuto") : t("updateModalManual")} ({updatePlatform || "unknown"})
          </div>

          {updateApplyStarted && (
            <div className="text-xs text-eve-warning border border-eve-warning/40 bg-eve-warning/10 rounded-sm px-2.5 py-2">
              {t("updateModalWillRestart")}
            </div>
          )}
          {updateApplyError && (
            <div className="text-xs text-eve-error border border-eve-error/40 bg-eve-error/10 rounded-sm px-2.5 py-2">
              {t("updateModalFailed")}: {updateApplyError}
            </div>
          )}

          <div className="pt-2 flex flex-wrap items-center justify-end gap-2">
            <button
              type="button"
              className="px-3 py-1.5 rounded-sm border border-eve-border text-eve-dim hover:text-eve-text disabled:opacity-60"
              onClick={handleSkipUpdate}
              disabled={updateApplying}
            >
              {t("updateModalSkip")}
            </button>
            <a
              href={releaseURL || "https://github.com/ilyaux/Eve-flipper/releases/latest"}
              target="_blank"
              rel="noreferrer"
              className="px-3 py-1.5 rounded-sm border border-eve-border text-eve-dim hover:text-eve-accent"
            >
              {t("updateModalOpenRelease")}
            </a>
            {autoUpdateSupported && (
              <button
                type="button"
                className="px-3 py-1.5 rounded-sm bg-eve-accent text-black font-medium hover:brightness-110 disabled:opacity-60"
                onClick={() => void handleStartUpdate()}
                disabled={updateApplying || updateApplyStarted}
              >
                {updateApplying ? t("updateModalStarting") : t("updateModalStart")}
              </button>
            )}
          </div>
        </div>
      </Modal>

      {/* Watchlist Modal */}
      <Modal
        open={showWatchlist}
        onClose={() => setShowWatchlist(false)}
        title={t("tabWatchlist")}
        width="max-w-3xl"
      >
        <WatchlistTab
          latestResults={[...radiusResults, ...regionResults]}
          alertChannels={alertChannels}
          toggleAlertChannel={toggleAlertChannel}
          alertTelegramToken={alertTelegramToken}
          setAlertTelegramToken={setAlertTelegramToken}
          alertTelegramChatID={alertTelegramChatID}
          setAlertTelegramChatID={setAlertTelegramChatID}
          alertDiscordWebhook={alertDiscordWebhook}
          setAlertDiscordWebhook={setAlertDiscordWebhook}
          handleTestAlert={handleTestAlert}
          alertTestLoading={alertTestLoading}
        />
      </Modal>

      {/* History Modal */}
      <Modal
        open={showHistory}
        onClose={() => setShowHistory(false)}
        title={t("tabHistory")}
        width="max-w-6xl"
      >
        <ScanHistory
          onLoadResults={(resultTab, results, loadedParams) => {
            // Load historical results into appropriate tab
            if (resultTab === "radius") {
              setRadiusResults(results as FlipResult[]);
              setTab("radius");
            } else if (resultTab === "region") {
              setRegionResults(normalizeRegionalResults(results));
              setTab("region");
            } else if (resultTab === "contracts") {
              setContractResults(results as ContractResult[]);
              setTab("contracts");
            } else if (resultTab === "station") {
              setStationLoadedResults(results as StationTrade[]);
              setTab("station");
            } else if (resultTab === "route") {
              setRouteLoadedResults(results as RouteResult[]);
              setTab("route");
            }
            // Restore only global ScanParams-compatible fields (avoid leaking tab-specific params)
            if (
              loadedParams &&
              (resultTab === "radius" ||
                resultTab === "region" ||
                resultTab === "contracts" ||
                resultTab === "route")
            ) {
              const safeKeys = [
                "system_name",
                "ignored_system_ids",
                "cargo_capacity",
                "buy_radius",
                "sell_radius",
                "min_margin",
                "sales_tax_percent",
                "broker_fee_percent",
                "split_trade_fees",
                "buy_broker_fee_percent",
                "sell_broker_fee_percent",
                "buy_sales_tax_percent",
                "sell_sales_tax_percent",
                "min_daily_volume",
                "max_investment",
                "min_item_profit",
                "min_period_roi",
                "max_dos",
                "min_demand_per_day",
                "purchase_demand_days",
                "min_s2b_per_day",
                "min_bfs_per_day",
                "min_s2b_bfs_ratio",
                "max_s2b_bfs_ratio",
                "avg_price_period",
                "shipping_cost_per_m3_jump",
                "min_route_security",
                "source_regions",
                "min_contract_price",
                "max_contract_margin",
                "min_priced_ratio",
                "require_history",
                "contract_instant_liquidation",
                "contract_hold_days",
                "contract_target_confidence",
                "exclude_rigs_with_ship",
                "target_region",
                "target_market_system",
                "target_market_location_id",
                "category_ids",
                "sell_order_mode",
                "include_structures",
                "route_min_hops",
                "route_max_hops",
                "route_target_system_name",
                "route_min_isk_per_jump",
                "route_allow_empty_hops",
                "route_mode",
                "route_ship_profile",
                "route_cargo_capacity",
                "route_minutes_per_jump",
                "route_dock_minutes",
                "route_safety_delay_percent",
              ];
              const filtered: Record<string, unknown> = {};
              for (const k of safeKeys) {
                if (k in loadedParams) filtered[k] = loadedParams[k];
              }
              // Backward compatibility: older route history stores min_hops/max_hops.
              if (!("route_min_hops" in filtered) && "min_hops" in loadedParams) {
                filtered.route_min_hops = loadedParams.min_hops;
              }
              if (!("route_max_hops" in filtered) && "max_hops" in loadedParams) {
                filtered.route_max_hops = loadedParams.max_hops;
              }
              if (Object.keys(filtered).length > 0) {
                setParams((p) => ({
                  ...p,
                  ...(filtered as Partial<ScanParams>),
                }));
              }
            }
            // Close modal after loading
            setShowHistory(false);
          }}
        />
      </Modal>

      <Modal
        open={showPatrons}
        onClose={() => setShowPatrons(false)}
        title={t("patronsTitle")}
        width="max-w-3xl"
      >
        <div className="p-4 sm:p-5">
          <div className="flex flex-wrap items-center justify-end gap-2 mb-3">
            <button
              type="button"
              onClick={() => void loadPatrons()}
              disabled={patronsLoading}
              className="px-2.5 py-1 text-xs bg-eve-panel border border-eve-border rounded-sm text-eve-dim hover:text-eve-accent disabled:opacity-60"
            >
              {t("patronsRefresh")}
            </button>
          </div>

          {patronsLoading && (
            <div className="py-8 text-center text-sm text-eve-dim">
              {t("patronsLoading")}
            </div>
          )}

          {!patronsLoading && patronsError !== "" && (
            <div className="py-4 px-3 rounded-sm border border-eve-error/50 bg-eve-error/10 text-sm text-eve-error">
              {t("patronsFetchError")}: {patronsError}
            </div>
          )}

          {!patronsLoading && patronsError === "" && patrons.length === 0 && (
            <div className="py-8 text-center text-sm text-eve-dim">
              {t("patronsEmpty")}
            </div>
          )}

          {!patronsLoading && patronsError === "" && patrons.length > 0 && (
            <div>
              <div className="text-xs text-eve-dim mb-2">
                {t("patronsCountLabel", { count: patrons.length })}
                {patronsUpdatedAt ? ` • ${t("patronsUpdatedAt", { date: patronsUpdatedAt })}` : ""}
                {patronsProject ? ` • ${patronsProject}` : ""}
              </div>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                {patrons.map((patron, idx) => (
                  <div
                    key={`${patron.name}-${idx}`}
                    className="rounded-sm border border-eve-border bg-eve-panel/60 px-3 py-2"
                  >
                    <div className="flex items-center justify-between gap-2">
                      {patron.url ? (
                        <a
                          href={patron.url}
                          target="_blank"
                          rel="noreferrer"
                          className="text-sm text-eve-accent hover:text-eve-accent-hover truncate"
                        >
                          {patron.name}
                        </a>
                      ) : (
                        <span className="text-sm text-eve-text truncate">{patron.name}</span>
                      )}
                      {patron.tier && (
                        <span className="text-[10px] uppercase tracking-wide px-1.5 py-0.5 rounded-sm border border-eve-accent/35 text-eve-accent">
                          {patron.tier}
                        </span>
                      )}
                    </div>
                    {patron.since && (
                      <div className="mt-1 text-[11px] text-eve-dim">
                        {t("patronsSince")}: {patron.since}
                      </div>
                    )}
                    {patron.note && (
                      <div className="mt-1 text-[11px] text-eve-dim">{patron.note}</div>
                    )}
                  </div>
                ))}
              </div>
            </div>
          )}

          <div className="mt-4 pt-3 border-t border-eve-border/50">
            <a
              href="https://www.patreon.com/cw/EVEFlipper"
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-2 text-sm text-eve-accent hover:text-eve-accent-hover"
            >
              <svg className="w-4 h-4" viewBox="0 0 1080 1080" fill="currentColor" aria-hidden="true">
                <path d="M1033.05 324.45c-.19-137.9-107.59-250.92-233.6-291.7-156.48-50.64-362.86-43.3-512.28 27.2-181.1 85.46-237.99 272.66-240.11 459.36-1.74 153.5 13.58 557.79 241.62 560.67 169.44 2.15 194.67-216.18 273.07-321.33 55.78-74.81 127.6-95.94 216.01-117.82 151.95-37.61 255.51-157.53 255.29-316.38z" />
              </svg>
              <span>{t("supportPatreon")}</span>
            </a>
          </div>
        </div>
      </Modal>

      {/* Character Info Modal */}
      {authStatus.logged_in && (
        <CharacterPopup
          open={showCharacter}
          onClose={() => setShowCharacter(false)}
          activeCharacterId={authStatus.character_id}
          characters={authStatus.characters ?? []}
          onSelectCharacter={handleSelectCharacter}
          onDeleteCharacter={handleDeleteCharacter}
          onAddCharacter={handleLogin}
          onAuthRefresh={refreshAuthStatus}
        />
      )}

      {/* Keyboard Shortcuts Help */}
      <KeyboardShortcutsHelp
        open={showShortcutsHelp}
        onClose={() => setShowShortcutsHelp(false)}
      />

      {/* Command Palette */}
      <CommandPalette
        open={showCommandPalette}
        onClose={() => setShowCommandPalette(false)}
        onSwitchTab={(t) => setTab(t)}
        onOpenWatchlist={() => setShowWatchlist(true)}
        onOpenHistory={() => setShowHistory(true)}
        onOpenCharacter={() => setShowCharacter(true)}
        onStartScan={() => document.querySelector<HTMLButtonElement>("[data-scan-button]")?.click()}
      />

      {/* ESI Unavailable Overlay */}
      {esiAvailable === false && (
        <div className="fixed inset-0 z-[100] bg-black/80 backdrop-blur-sm flex items-center justify-center">
          <div className="bg-eve-panel border border-eve-error/50 rounded-lg p-8 max-w-md mx-4 text-center shadow-2xl">
            <div className="w-16 h-16 mx-auto mb-4 rounded-full bg-eve-error/20 flex items-center justify-center">
              <svg
                className="w-8 h-8 text-eve-error animate-pulse"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
                />
              </svg>
            </div>
            <h2 className="text-xl font-bold text-eve-error mb-2">
              {t("esiUnavailable")}
            </h2>
            <p className="text-eve-dim mb-4">{t("esiUnavailableDesc")}</p>
            <div className="flex items-center justify-center gap-2 text-sm text-eve-dim">
              <div className="w-2 h-2 bg-eve-accent rounded-full animate-pulse" />
              <span>{t("esiWaiting")}</span>
            </div>
          </div>
        </div>
      )}
      </div>
      {showBootSplash && (
        <div
          aria-hidden="true"
          className={`pointer-events-none fixed inset-0 z-[12000] transition-opacity duration-500 ${
            bootSplashState === "fading" ? "opacity-0" : "opacity-100"
          }`}
        >
          <div className="absolute inset-0 eve-preloader-backdrop" />
          <div className="absolute inset-0 eve-preloader-grid" />
          <div className="absolute inset-0 overflow-hidden">
            <div className="eve-preloader-sweep" />
          </div>
          <div className="relative z-10 flex h-full items-center justify-center px-4 sm:px-6">
            <div className="eve-preloader-radar-field" />
            <div className="eve-preloader-shell w-full max-w-[620px]">
              <div className="eve-preloader-shell-head">
                <span className="eve-preloader-led" />
                <span className="eve-preloader-headline">{t("appTitle")}</span>
                <span className="eve-preloader-build">{appVersion}</span>
              </div>
              <div className="eve-preloader-main">
                <div className="eve-preloader-core-wrap">
                  <div className="eve-preloader-core">
                    <div className="eve-preloader-core-ring eve-preloader-core-ring--outer" />
                    <div className="eve-preloader-core-ring eve-preloader-core-ring--mid" />
                    <div className="eve-preloader-core-ring eve-preloader-core-ring--inner" />
                    <div className="eve-preloader-core-sweep" />
                    <div className="eve-preloader-center">
                      <img src={logo} alt="" className="eve-preloader-logo" />
                    </div>
                  </div>
                  <div className="eve-preloader-core-caption">Neocom uplink</div>
                </div>
                <div className="eve-preloader-copy">
                  <div className="eve-preloader-kicker">
                    <span>Capsuleer channel</span>
                    <span className="eve-preloader-chip">Secure</span>
                  </div>
                  <div className="eve-preloader-title">{t("appTitle")}</div>
                  <div className="eve-preloader-status">
                    <span>{t("loading")}</span>
                    <span className="eve-preloader-dot" />
                    <span className="eve-preloader-dot eve-preloader-dot--delay-1" />
                    <span className="eve-preloader-dot eve-preloader-dot--delay-2" />
                  </div>
                  <div className="eve-preloader-telemetry">
                    <div className="eve-preloader-line">
                      <span className="eve-preloader-label">Cluster</span>
                      <span className="eve-preloader-value">Tranquility</span>
                    </div>
                    <div className="eve-preloader-line">
                      <span className="eve-preloader-label">Uplink</span>
                      <span className="eve-preloader-value eve-preloader-value--accent">Stable</span>
                    </div>
                    <div className="eve-preloader-line">
                      <span className="eve-preloader-label">Session</span>
                      <span className="eve-preloader-value">Handshake</span>
                    </div>
                  </div>
                  <div className="eve-preloader-footer">
                    <span className="eve-preloader-label">Node</span>
                    <span className="eve-preloader-value eve-preloader-value--accent">Jita relay</span>
                  </div>
                </div>
              </div>
              <div className="eve-preloader-bar">
                <span className="eve-preloader-bar-fill" />
              </div>
              <div className="eve-preloader-bar-scale">
                <span>0%</span>
                <span>50%</span>
                <span>100%</span>
              </div>
            </div>
          </div>
        </div>
      )}
    </>
  );
}

function TabButton({
  active,
  onClick,
  label,
}: {
  active: boolean;
  onClick: () => void;
  label: string;
}) {
  return (
    <button
      role="tab"
      aria-selected={active}
      onClick={onClick}
      className={`px-2.5 py-2 sm:px-4 sm:py-2.5 text-[10px] sm:text-xs font-medium uppercase tracking-wider transition-colors relative whitespace-nowrap snap-center shrink-0
        ${active ? "text-eve-accent" : "text-eve-dim hover:text-eve-text"}`}
    >
      {label}
      {active && (
        <div
          className="absolute bottom-0 left-0 right-0 h-[2px] bg-eve-accent"
          aria-hidden="true"
        />
      )}
    </button>
  );
}

export default App;
