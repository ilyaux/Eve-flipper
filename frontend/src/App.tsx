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
import { LanguageSwitcher } from "./components/LanguageSwitcher";
import { ThemeSwitcher } from "./components/ThemeSwitcher";
import { useGlobalToast } from "./components/Toast";
import { Modal } from "./components/Modal";
import { CharacterPopup } from "./components/CharacterPopup";
import {
  getConfig,
  updateConfig,
  scan,
  scanMultiRegion,
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
  RouteResult,
  ScanParams,
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

function App() {
  const { t } = useI18n();

  const [params, setParams] = useState<ScanParams>({
    system_name: "Jita",
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
    contract_hold_days: 7,
    contract_target_confidence: 80,
  });
  const configLoadedRef = useRef(false);

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
  const { appVersion, latestVersion, hasUpdate } = useVersionCheck();
  const { esiAvailable } = useEsiStatus();

  const [radiusResults, setRadiusResults] = useState<FlipResult[]>([]);
  const [regionResults, setRegionResults] = useState<FlipResult[]>([]);
  const [contractResults, setContractResults] = useState<ContractResult[]>([]);
  const [stationLoadedResults, setStationLoadedResults] = useState<
    StationTrade[] | null
  >(null);
  const [routeLoadedResults, setRouteLoadedResults] = useState<
    RouteResult[] | null
  >(null);

  const [scanning, setScanning] = useState(false);
  const [progress, setProgress] = useState("");

  const [showWatchlist, setShowWatchlist] = useState(false);
  const [showHistory, setShowHistory] = useState(false);
  const [showCharacter, setShowCharacter] = useState(false);
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
  const { addToast } = useGlobalToast();

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
    ],
    [tab, params.system_name],
  );

  useKeyboardShortcuts(shortcuts);

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

  // Load config on mount
  useEffect(() => {
    getConfig()
      .then((cfg) => {
        setParams((prev) => ({
          ...prev,
          system_name: cfg.system_name || prev.system_name,
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
    } else {
      setRegionResults([]);
    }

    try {
      if (currentTab === "contracts") {
        const results = await scanContracts(
          params,
          setProgress,
          controller.signal,
        );
        setContractResults(results);
        setContractScanCompleted(true);
      } else {
        const scanFn = currentTab === "radius" ? scan : scanMultiRegion;
        const results = await scanFn(params, setProgress, controller.signal);
        if (currentTab === "radius") {
          setRadiusResults(results);
        } else {
          setRegionResults(results);
        }
        // Desktop alerts are local-only; external channels are handled by backend scan handlers.
        if (alertChannels.desktop) {
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

              const match = results.find((r) => r.TypeID === item.type_id);
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
        }
      }
    } catch (e: unknown) {
      if (e instanceof Error && e.name !== "AbortError") {
        setProgress(t("errorPrefix") + e.message);
      }
    } finally {
      setScanning(false);
    }
  }, [scanning, tab, params, t, addToast, alertChannels]);

  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);

  return (
    <div className="h-screen flex flex-col gap-1.5 sm:gap-3 p-1.5 sm:p-4 select-none overflow-hidden">
      {/* Header */}
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2 sm:gap-3 min-w-0">
          <div className="flex items-center gap-1.5 sm:gap-2 min-w-0">
            <img
              src={logo}
              alt="EVE Flipper logo"
              className="w-4 h-4 shrink-0"
            />
            <h1 className="text-sm sm:text-lg font-semibold text-eve-accent tracking-wide uppercase truncate">
              {t("appTitle")}
            </h1>
            <div className="hidden sm:flex items-center gap-1">
              <span
                className="px-1.5 py-0.5 text-[10px] font-mono bg-eve-accent/10 text-eve-accent border border-eve-accent/30 rounded-sm"
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
                  href="https://github.com/ilyaux/Eve-flipper/releases/latest"
                  target="_blank"
                  rel="noreferrer"
                  className="px-1.5 py-0.5 text-[9px] uppercase tracking-wide rounded-sm bg-eve-warning/10 text-eve-warning border border-eve-warning/40 hover:bg-eve-warning/20 transition-colors"
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
              href="https://discord.gg/Z9pXSGcJZE"
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
            className="flex items-center justify-center h-9 w-9 bg-eve-panel border border-eve-border rounded-sm text-eve-dim"
          >
            <svg className="w-4 h-4" viewBox="0 0 16 16" fill="currentColor">
              <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27s1.36.09 2 .27c1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8" />
            </svg>
          </a>
          <a
            href="https://discord.gg/Z9pXSGcJZE"
            target="_blank"
            rel="noreferrer"
            className="flex items-center justify-center h-9 w-9 bg-eve-panel border border-eve-border rounded-sm text-eve-dim"
          >
            <svg className="w-4 h-4 discord-icon-animated" viewBox="0 0 16 16" fill="currentColor">
              <path d="M13.545 2.907a13.2 13.2 0 0 0-3.257-1.011.05.05 0 0 0-.052.025c-.141.25-.297.577-.406.833a12.2 12.2 0 0 0-3.658 0 8 8 0 0 0-.412-.833.05.05 0 0 0-.052-.025c-1.125.194-2.22.534-3.257 1.011a.04.04 0 0 0-.021.018C.356 6.024-.213 9.047.066 12.032q.003.022.021.037a13.3 13.3 0 0 0 3.995 2.02.05.05 0 0 0 .056-.019q.463-.63.818-1.329a.05.05 0 0 0-.01-.059l-.018-.011a9 9 0 0 1-1.248-.595.05.05 0 0 1-.02-.066l.015-.019q.127-.095.248-.195a.05.05 0 0 1 .051-.007c2.619 1.196 5.454 1.196 8.041 0a.05.05 0 0 1 .053.007q.121.1.248.195a.05.05 0 0 1-.004.085 8 8 0 0 1-1.249.594.05.05 0 0 0-.03.03.05.05 0 0 0 .003.041c.24.465.515.909.817 1.329a.05.05 0 0 0 .056.019 13.2 13.2 0 0 0 4.001-2.02.05.05 0 0 0 .021-.037c.334-3.451-.559-6.449-2.366-9.106a.03.03 0 0 0-.02-.019m-8.198 7.307c-.789 0-1.438-.724-1.438-1.612s.637-1.613 1.438-1.613c.807 0 1.45.73 1.438 1.613 0 .888-.637 1.612-1.438 1.612m5.316 0c-.788 0-1.438-.724-1.438-1.612s.637-1.613 1.438-1.613c.807 0 1.451.73 1.438 1.613 0 .888-.631 1.612-1.438 1.612" />
            </svg>
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
        <div
          className="flex items-center border-b border-eve-border overflow-x-auto scrollbar-thin snap-x snap-mandatory sm:snap-none"
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
          <div className="flex-1 min-w-[12px] sm:min-w-[20px]" />
          {tab !== "route" &&
            tab !== "station" &&
            tab !== "industry" &&
            tab !== "demand" &&
            tab !== "plex" && (
              <button
                data-scan-button
                onClick={handleScan}
                disabled={!params.system_name}
                title="Ctrl+S"
                className={`mr-1.5 sm:mr-3 px-3 sm:px-5 py-1.5 rounded-sm text-[10px] sm:text-xs font-semibold uppercase tracking-wider transition-all shrink-0
              ${
                scanning
                  ? "bg-eve-error/80 text-white hover:bg-eve-error"
                  : "bg-eve-accent text-eve-dark hover:bg-eve-accent-hover shadow-eve-glow"
              }
              disabled:bg-eve-input disabled:text-eve-dim disabled:cursor-not-allowed disabled:shadow-none`}
              >
                {scanning ? t("stop") : t("scan")}
              </button>
            )}
        </div>

        {/* Results — all tabs stay mounted to preserve state */}
        <div className="flex-1 min-h-0 flex flex-col p-1.5 sm:p-2">
          <div
            className={`flex-1 min-h-0 flex flex-col ${tab === "radius" ? "" : "hidden"}`}
          >
            <ScanResultsTable
              results={radiusResults}
              scanning={scanning && tab === "radius"}
              progress={tab === "radius" ? progress : ""}
              salesTaxPercent={params.sales_tax_percent}
              brokerFeePercent={params.broker_fee_percent}
              splitTradeFees={params.split_trade_fees}
              buyBrokerFeePercent={params.buy_broker_fee_percent}
              sellBrokerFeePercent={params.sell_broker_fee_percent}
              buySalesTaxPercent={params.buy_sales_tax_percent}
              sellSalesTaxPercent={params.sell_sales_tax_percent}
              isLoggedIn={authStatus.logged_in}
            />
          </div>
          <div
            className={`flex-1 min-h-0 flex flex-col ${tab === "region" ? "" : "hidden"}`}
          >
            <ScanResultsTable
              results={regionResults}
              scanning={scanning && tab === "region"}
              progress={tab === "region" ? progress : ""}
              salesTaxPercent={params.sales_tax_percent}
              brokerFeePercent={params.broker_fee_percent}
              splitTradeFees={params.split_trade_fees}
              buyBrokerFeePercent={params.buy_broker_fee_percent}
              sellBrokerFeePercent={params.sell_broker_fee_percent}
              buySalesTaxPercent={params.buy_sales_tax_percent}
              sellSalesTaxPercent={params.sell_sales_tax_percent}
              isLoggedIn={authStatus.logged_in}
              showRegions
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
                // Switch to Regional Arbitrage tab and set target region
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
            <PlexTab />
          </div>
        </div>
      </div>

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
              setRegionResults(results as FlipResult[]);
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
                "min_s2b_per_day",
                "min_bfs_per_day",
                "min_s2b_bfs_ratio",
                "max_s2b_bfs_ratio",
                "min_contract_price",
                "max_contract_margin",
                "min_priced_ratio",
                "require_history",
                "contract_instant_liquidation",
                "contract_hold_days",
                "contract_target_confidence",
                "target_region",
              ];
              const filtered: Record<string, unknown> = {};
              for (const k of safeKeys) {
                if (k in loadedParams) filtered[k] = loadedParams[k];
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
