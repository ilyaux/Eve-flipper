import { useCallback, useEffect, useRef, useState } from "react";
import { StatusBar } from "./components/StatusBar";
import { ParametersPanel } from "./components/ParametersPanel";
import { ScanResultsTable } from "./components/ScanResultsTable";
import { ContractResultsTable } from "./components/ContractResultsTable";
import { RouteBuilder } from "./components/RouteBuilder";
import { WatchlistTab } from "./components/WatchlistTab";
import { StationTrading } from "./components/StationTrading";
import { LanguageSwitcher } from "./components/LanguageSwitcher";
import { ToastContainer, useToast } from "./components/Toast";
import { getConfig, updateConfig, scan, scanMultiRegion, scanContracts, getWatchlist, getAuthStatus, logout as apiLogout, getLoginUrl } from "./lib/api";
import { useI18n } from "./lib/i18n";
import type { AuthStatus, ContractResult, FlipResult, ScanParams } from "./lib/types";

type Tab = "radius" | "region" | "contracts" | "station" | "route" | "watchlist";

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

  const abortRef = useRef<AbortController | null>(null);
  const scanTabRef = useRef<Tab>(tab);
  const { toasts, addToast } = useToast();

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
                addToast(`🔔 ${match.TypeName}: ${t("alertTriggered", { margin: match.MarginPercent.toFixed(1), threshold: item.alert_min_margin.toFixed(0) })}`);
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
  }, [scanning, tab, params, t]);

  return (
    <div className="h-screen flex flex-col gap-3 p-4 select-none overflow-hidden">
      <ToastContainer toasts={toasts} />
      {/* Header */}
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-semibold text-eve-accent tracking-wide uppercase">
          {t("appTitle")}
        </h1>
        <div className="flex items-center gap-2">
          {/* Auth chip — same style as StatusBar */}
          <div className="flex items-center gap-2 h-[34px] px-4 bg-eve-panel border border-eve-border rounded-sm text-xs">
            {authStatus.logged_in ? (
              <>
                <img
                  src={`https://images.evetech.net/characters/${authStatus.character_id}/portrait?size=32`}
                  alt=""
                  className="w-5 h-5 rounded-sm"
                />
                <span className="text-eve-accent font-medium">{authStatus.character_name}</span>
                <button
                  onClick={handleLogout}
                  className="text-eve-dim hover:text-eve-error transition-colors text-[10px] ml-0.5"
                >
                  ✕
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

      {/* Parameters */}
      <ParametersPanel params={params} onChange={setParams} />

      {/* Tabs */}
      <div className="flex-1 flex flex-col min-h-0 bg-eve-panel border border-eve-border rounded-sm">
        <div className="flex items-center border-b border-eve-border">
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
            active={tab === "station"}
            onClick={() => setTab("station")}
            label={t("tabStation")}
          />
          <TabButton
            active={tab === "route"}
            onClick={() => setTab("route")}
            label={t("tabRoute")}
          />
          <TabButton
            active={tab === "watchlist"}
            onClick={() => setTab("watchlist")}
            label={`⭐ ${t("tabWatchlist")}`}
          />
          <div className="flex-1" />
          {tab !== "route" && tab !== "watchlist" && tab !== "station" && <button
            onClick={handleScan}
            disabled={!params.system_name}
            className={`mr-3 px-5 py-1.5 rounded-sm text-xs font-semibold uppercase tracking-wider transition-all
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
            <ScanResultsTable results={radiusResults} scanning={scanning && tab === "radius"} progress={tab === "radius" ? progress : ""} />
          </div>
          <div className={`flex-1 min-h-0 flex flex-col ${tab === "region" ? "" : "hidden"}`}>
            <ScanResultsTable results={regionResults} scanning={scanning && tab === "region"} progress={tab === "region" ? progress : ""} />
          </div>
          <div className={`flex-1 min-h-0 flex flex-col ${tab === "contracts" ? "" : "hidden"}`}>
            <ContractResultsTable results={contractResults} scanning={scanning && tab === "contracts"} progress={tab === "contracts" ? progress : ""} />
          </div>
          <div className={`flex-1 min-h-0 flex flex-col ${tab === "station" ? "" : "hidden"}`}>
            <StationTrading params={params} />
          </div>
          <div className={`flex-1 min-h-0 flex flex-col ${tab === "route" ? "" : "hidden"}`}>
            <RouteBuilder params={params} />
          </div>
          <div className={`flex-1 min-h-0 flex flex-col ${tab === "watchlist" ? "" : "hidden"}`}>
            <WatchlistTab latestResults={[...radiusResults, ...regionResults]} />
          </div>
        </div>
      </div>
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
        <div className="absolute bottom-0 left-0 right-0 h-[2px] bg-eve-accent" />
      )}
    </button>
  );
}

export default App;
