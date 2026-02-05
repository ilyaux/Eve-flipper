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
import { ScanHistory } from "./components/ScanHistory";
import { LanguageSwitcher } from "./components/LanguageSwitcher";
import { useGlobalToast } from "./components/Toast";
import { Modal } from "./components/Modal";
import { CharacterPopup } from "./components/CharacterPopup";
import { getConfig, updateConfig, scan, scanMultiRegion, scanContracts, getWatchlist, getAuthStatus, logout as apiLogout, getLoginUrl, getStatus } from "./lib/api";
import { useI18n } from "./lib/i18n";
import { formatISK } from "./lib/format";
import type { AuthStatus, ContractResult, FlipResult, ScanParams } from "./lib/types";

type Tab = "radius" | "region" | "contracts" | "station" | "route" | "industry" | "demand";

function App() {
  const { t } = useI18n();

  const [params, setParams] = useState<ScanParams>({
    system_name: "Jita",
    cargo_capacity: 5000,
    buy_radius: 5,
    sell_radius: 10,
    min_margin: 5,
    sales_tax_percent: 8,
    max_results: 100,
  });

  const [tab, setTab] = useState<Tab>("radius");
  const [authStatus, setAuthStatus] = useState<AuthStatus>({ logged_in: false });

  const [radiusResults, setRadiusResults] = useState<FlipResult[]>([]);
  const [regionResults, setRegionResults] = useState<FlipResult[]>([]);
  const [contractResults, setContractResults] = useState<ContractResult[]>([]);

  const [scanning, setScanning] = useState(false);
  const [progress, setProgress] = useState("");

  const [showWatchlist, setShowWatchlist] = useState(false);
  const [showHistory, setShowHistory] = useState(false);
  const [showCharacter, setShowCharacter] = useState(false);
  const [esiAvailable, setEsiAvailable] = useState<boolean | null>(null); // null = loading

  const abortRef = useRef<AbortController | null>(null);
  const scanTabRef = useRef<Tab>(tab);
  const { addToast } = useGlobalToast();

  const [contractScanCompleted, setContractScanCompleted] = useState(false);
  const contractFilterHints = useMemo(() => {
    if (contractResults.length > 0 || !contractScanCompleted) return undefined;
    return [
      `${t("minContractPrice")}: ${formatISK(params.min_contract_price ?? 10_000_000)}`,
      `${t("maxContractMargin")}: ${params.max_contract_margin ?? 100}%`,
      `${t("minPricedRatio")}: ${((params.min_priced_ratio ?? 0.8) * 100).toFixed(0)}%`,
    ];
  }, [contractResults.length, contractScanCompleted, params.min_contract_price, params.max_contract_margin, params.min_priced_ratio, t]);

  // Keyboard shortcuts
  const shortcuts = useMemo(() => [
    {
      key: "s",
      modifiers: ["ctrl"] as const,
      handler: () => {
        if (tab !== "route" && tab !== "station" && params.system_name) {
          // Trigger scan via button click simulation
          document.querySelector<HTMLButtonElement>('[data-scan-button]')?.click();
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
  ], [tab, params.system_name]);

  useKeyboardShortcuts(shortcuts);

  // Load config on mount
  useEffect(() => {
    getConfig()
      .then((cfg) => {
        setParams({
          system_name: cfg.system_name || "Jita",
          cargo_capacity: cfg.cargo_capacity || 5000,
          buy_radius: cfg.buy_radius || 5,
          sell_radius: cfg.sell_radius || 10,
          min_margin: cfg.min_margin || 5,
          sales_tax_percent: cfg.sales_tax_percent || 8,
        });
      })
      .catch(() => {});
    getAuthStatus().then(setAuthStatus).catch(() => {});
  }, []);

  // Poll ESI status
  useEffect(() => {
    let mounted = true;
    const checkEsi = async () => {
      try {
        const status = await getStatus();
        if (mounted) setEsiAvailable(status.esi_ok);
      } catch {
        if (mounted) setEsiAvailable(false);
      }
    };
    checkEsi();
    const interval = setInterval(checkEsi, 5000);
    return () => {
      mounted = false;
      clearInterval(interval);
    };
  }, []);

  const handleLogout = useCallback(async () => {
    await apiLogout();
    setAuthStatus({ logged_in: false });
  }, []);

  // Save config on param change (debounced)
  const saveTimerRef = useRef<ReturnType<typeof setTimeout>>(undefined);
  useEffect(() => {
    clearTimeout(saveTimerRef.current);
    saveTimerRef.current = setTimeout(() => {
      updateConfig(params).catch(() => {});
    }, 500);
    return () => clearTimeout(saveTimerRef.current);
  }, [params]);

  const handleScan = useCallback(async () => {
    if (scanning) {
      abortRef.current?.abort();
      return;
    }

    const currentTab = tab;
    scanTabRef.current = currentTab;
    const controller = new AbortController();
    abortRef.current = controller;
    setScanning(true);
    setProgress(t("scanStarting"));

    try {
      if (currentTab === "contracts") {
        const results = await scanContracts(params, setProgress, controller.signal);
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
        // Check watchlist alerts
        try {
          const wl = await getWatchlist();
          for (const item of wl) {
            if (item.alert_min_margin > 0) {
              const match = results.find((r) => r.TypeID === item.type_id && r.MarginPercent > item.alert_min_margin);
              if (match) {
                addToast(`${match.TypeName}: ${t("alertTriggered", { margin: match.MarginPercent.toFixed(1), threshold: item.alert_min_margin.toFixed(0) })}`, "success");
              }
            }
          }
        } catch { /* ignore */ }
      }
    } catch (e: unknown) {
      if (e instanceof Error && e.name !== "AbortError") {
        setProgress(t("errorPrefix") + e.message);
      }
    } finally {
      setScanning(false);
    }
  }, [scanning, tab, params, t, addToast]);

  return (
    <div className="h-screen flex flex-col gap-2 sm:gap-3 p-2 sm:p-4 select-none overflow-hidden">
      {/* Header */}
      <div className="flex items-center justify-between flex-wrap gap-2">
        <div className="flex items-center gap-2">
          <h1 className="text-base sm:text-lg font-semibold text-eve-accent tracking-wide uppercase">
            {t("appTitle")}
          </h1>
          <span className="px-1.5 py-0.5 text-[10px] font-mono bg-eve-accent/10 text-eve-accent border border-eve-accent/30 rounded-sm">
            {import.meta.env.VITE_APP_VERSION || "dev"}
          </span>
        </div>
        <div className="flex items-center gap-1 sm:gap-2 flex-wrap">
          {/* Watchlist button */}
          <button
            onClick={() => setShowWatchlist(true)}
            className="flex items-center gap-1.5 h-[34px] px-3 bg-eve-panel border border-eve-border rounded-sm text-xs text-eve-dim hover:text-eve-accent hover:border-eve-accent/50 transition-colors"
            title={t("tabWatchlist")}
            aria-label={t("tabWatchlist")}
          >
            <span aria-hidden="true">⭐</span>
            <span className="hidden sm:inline">{t("tabWatchlist")}</span>
          </button>
          {/* History button */}
          <button
            onClick={() => setShowHistory(true)}
            className="flex items-center gap-1.5 h-[34px] px-3 bg-eve-panel border border-eve-border rounded-sm text-xs text-eve-dim hover:text-eve-accent hover:border-eve-accent/50 transition-colors"
            title={t("tabHistory")}
            aria-label={t("tabHistory")}
          >
            <span aria-hidden="true">📋</span>
            <span className="hidden sm:inline">{t("tabHistory")}</span>
          </button>
          {/* Auth chip — same style as StatusBar */}
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
                  <span className="text-eve-accent font-medium">{authStatus.character_name}</span>
                </button>
                <button
                  onClick={handleLogout}
                  className="ml-1 p-1 text-eve-dim hover:text-eve-error hover:bg-eve-dark/50 rounded-sm transition-colors"
                  title={t("logout")}
                  aria-label={t("logout")}
                >
                  <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1" />
                  </svg>
                </button>
              </>
            ) : (
              <a href={getLoginUrl()} className="text-eve-accent hover:text-eve-accent-hover transition-colors">
                {t("loginEve")}
              </a>
            )}
          </div>
          <LanguageSwitcher />
          <StatusBar />
        </div>
      </div>

      {/* Parameters - shown for tabs that use global scan params (Flipper, Regional, Contracts, Route) */}
      {(tab === "radius" || tab === "region" || tab === "contracts" || tab === "route") && (
        <ParametersPanel params={params} onChange={setParams} isLoggedIn={authStatus.logged_in} tab={tab} />
      )}

      {/* Industry doesn't use global params - has its own settings panel */}

      {/* Tabs */}
      <div className="flex-1 flex flex-col min-h-0 bg-eve-panel border border-eve-border rounded-sm">
        <div className="flex items-center border-b border-eve-border overflow-x-auto scrollbar-thin" role="tablist" aria-label="Scan modes">
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
          <div className="h-6 w-px bg-eve-border mx-1 flex-shrink-0" aria-hidden="true" />
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
          <div className="flex-1 min-w-[20px]" />
          {tab !== "route" && tab !== "station" && tab !== "industry" && tab !== "demand" && <button
            data-scan-button
            onClick={handleScan}
            disabled={!params.system_name}
            title="Ctrl+S"
            className={`mr-2 sm:mr-3 px-3 sm:px-5 py-1.5 rounded-sm text-xs font-semibold uppercase tracking-wider transition-all shrink-0
              ${
                scanning
                  ? "bg-eve-error/80 text-white hover:bg-eve-error"
                  : "bg-eve-accent text-eve-dark hover:bg-eve-accent-hover shadow-eve-glow"
              }
              disabled:bg-eve-input disabled:text-eve-dim disabled:cursor-not-allowed disabled:shadow-none`}
          >
            {scanning ? t("stop") : t("scan")}
          </button>}
        </div>

        {/* Results — all tabs stay mounted to preserve state */}
        <div className="flex-1 min-h-0 flex flex-col p-2">
          <div className={`flex-1 min-h-0 flex flex-col ${tab === "radius" ? "" : "hidden"}`}>
            <ScanResultsTable results={radiusResults} scanning={scanning && tab === "radius"} progress={tab === "radius" ? progress : ""} salesTaxPercent={params.sales_tax_percent} />
          </div>
          <div className={`flex-1 min-h-0 flex flex-col ${tab === "region" ? "" : "hidden"}`}>
            <ScanResultsTable results={regionResults} scanning={scanning && tab === "region"} progress={tab === "region" ? progress : ""} salesTaxPercent={params.sales_tax_percent} />
          </div>
          <div className={`flex-1 min-h-0 flex flex-col ${tab === "contracts" ? "" : "hidden"}`}>
            {/* Contract-specific settings */}
            <div className="shrink-0 mb-2">
              <ContractParametersPanel params={params} onChange={setParams} />
            </div>
            <ContractResultsTable results={contractResults} scanning={scanning && tab === "contracts"} progress={tab === "contracts" ? progress : ""} filterHints={contractFilterHints} />
          </div>
          <div className={`flex-1 min-h-0 flex flex-col ${tab === "station" ? "" : "hidden"}`}>
            <StationTrading params={params} onChange={setParams} isLoggedIn={authStatus.logged_in} />
          </div>
          <div className={`flex-1 min-h-0 flex flex-col ${tab === "route" ? "" : "hidden"}`}>
            <RouteBuilder params={params} />
          </div>
          <div className={`flex-1 min-h-0 flex flex-col ${tab === "industry" ? "" : "hidden"}`}>
            <IndustryTab isLoggedIn={authStatus.logged_in} />
          </div>
          <div className={`flex-1 min-h-0 flex flex-col ${tab === "demand" ? "" : "hidden"}`}>
            <WarTracker 
              onError={(msg) => addToast(msg, "error")} 
              onOpenRegionArbitrage={(regionName) => {
                // Switch to Regional Arbitrage tab and set target region
                setParams(p => ({ ...p, target_region: regionName }));
                setTab("region");
                addToast(`${t("targetRegionSet") || "Target region set to"} ${regionName}`, "success");
              }}
            />
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
        <WatchlistTab latestResults={[...radiusResults, ...regionResults]} />
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
            }
            // Optionally restore params
            if (loadedParams && Object.keys(loadedParams).length > 0) {
              setParams((p) => ({ ...p, ...loadedParams as Partial<ScanParams> }));
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
          characterId={authStatus.character_id!}
          characterName={authStatus.character_name!}
        />
      )}

      {/* ESI Unavailable Overlay */}
      {esiAvailable === false && (
        <div className="fixed inset-0 z-[100] bg-black/80 backdrop-blur-sm flex items-center justify-center">
          <div className="bg-eve-panel border border-eve-error/50 rounded-lg p-8 max-w-md mx-4 text-center shadow-2xl">
            <div className="w-16 h-16 mx-auto mb-4 rounded-full bg-eve-error/20 flex items-center justify-center">
              <svg className="w-8 h-8 text-eve-error animate-pulse" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
              </svg>
            </div>
            <h2 className="text-xl font-bold text-eve-error mb-2">{t("esiUnavailable")}</h2>
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
      className={`px-4 py-2.5 text-xs font-medium uppercase tracking-wider transition-colors relative
        ${
          active
            ? "text-eve-accent"
            : "text-eve-dim hover:text-eve-text"
        }`}
    >
      {label}
      {active && (
        <div className="absolute bottom-0 left-0 right-0 h-[2px] bg-eve-accent" aria-hidden="true" />
      )}
    </button>
  );
}

export default App;
