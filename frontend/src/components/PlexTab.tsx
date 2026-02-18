import { useCallback, useEffect, useRef, useState } from "react";
import { getPLEXDashboard, type PLEXDashboardParams } from "../lib/api";
import { useI18n } from "../lib/i18n";
import { useTheme } from "../lib/useTheme";
import type { PLEXDashboard, ArbitragePath } from "../lib/types";
import { usePlexAlerts, PlexAlertPanel } from "./PlexAlerts";
import {
  SignalCard,
  GlobalPriceCard,
  ArbitrageRow,
  SPFarmCard,
  ArbHistoryChart,
  MarketDepthCard,
  InjectionTiersCard,
  FleetManagerCard,
  ArbitrageModal,
  PLEXChart,
  OmegaComparatorCard,
  CrossHubCard,
} from "./plex-tab/PlexTabSections";

type PlexSubTab = "market" | "spfarm" | "analytics";

/** Format seconds as M:SS */
function formatCountdown(sec: number): string {
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  return `${m}:${s.toString().padStart(2, "0")}`;
}

export function PlexTab() {
  const { t } = useI18n();
  const { themeKey } = useTheme();
  const [dashboard, setDashboard] = useState<PLEXDashboard | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [salesTax, setSalesTax] = useState(3.6);
  const [brokerFee, setBrokerFee] = useState(1.0);
  const [nesExtractor, setNesExtractor] = useState(293);
  const [nesMPTC, setNesMPTC] = useState(485);
  const [nesOmega, setNesOmega] = useState(500);
  const [omegaUSD, setOmegaUSD] = useState(14.99);
  const [showNES, setShowNES] = useState(false);
  const [autoRefresh, setAutoRefresh] = useState(false);
  const [autoInterval, setAutoInterval] = useState(5); // minutes
  const [countdown, setCountdown] = useState(0); // seconds remaining

  const abortRef = useRef<AbortController | null>(null);
  const autoTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const countdownRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const fetchData = useCallback(async () => {
    // Abort any in-flight request
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;

    setLoading(true);
    setError("");
    try {
      const params: PLEXDashboardParams = {
        salesTax, brokerFee,
        nesExtractor, nesMPTC, nesOmega, omegaUSD,
      };
      const data = await getPLEXDashboard(params, controller.signal);
      setDashboard(data);
    } catch (e: unknown) {
      if (e instanceof Error && e.name === "AbortError") return;
      setError(e instanceof Error ? e.message : "Failed to load PLEX data");
    } finally {
      setLoading(false);
    }
  }, [salesTax, brokerFee, nesExtractor, nesMPTC, nesOmega, omegaUSD]);

  // Fetch on mount
  useEffect(() => {
    fetchData();
    return () => abortRef.current?.abort();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // Auto-refresh timer
  useEffect(() => {
    // Clear previous timers
    if (autoTimerRef.current) { clearInterval(autoTimerRef.current); autoTimerRef.current = null; }
    if (countdownRef.current) { clearInterval(countdownRef.current); countdownRef.current = null; }
    setCountdown(0);

    if (!autoRefresh) return;

    const intervalMs = autoInterval * 60 * 1000;
    setCountdown(autoInterval * 60);

    // Countdown ticker (every second)
    countdownRef.current = setInterval(() => {
      setCountdown(prev => prev > 0 ? prev - 1 : 0);
    }, 1000);

    // Fetch timer
    autoTimerRef.current = setInterval(() => {
      fetchData();
      setCountdown(autoInterval * 60); // reset countdown after fetch
    }, intervalMs);

    return () => {
      if (autoTimerRef.current) clearInterval(autoTimerRef.current);
      if (countdownRef.current) clearInterval(countdownRef.current);
    };
  }, [autoRefresh, autoInterval, fetchData]);

  const [selectedArb, setSelectedArb] = useState<ArbitragePath | null>(null);
  const [arbTab, setArbTab] = useState<"nes" | "spread">("nes");
  const [showAlerts, setShowAlerts] = useState(false);
  const [subTab, setSubTab] = useState<PlexSubTab>("market");

  // PLEX alerts (Browser Notification API)
  usePlexAlerts(dashboard);

  const signal = dashboard?.signal;
  const ind = dashboard?.indicators;

  return (
    <div className="flex flex-col gap-3 h-full overflow-y-auto pr-1 scrollbar-thin">
      {/* Top bar: controls */}
      <div className="flex items-center gap-3 flex-wrap shrink-0">
        <h2 className="text-sm font-semibold text-eve-accent uppercase tracking-wider">{t("plexTitle")}</h2>
        <div className="flex items-center gap-2 text-xs">
          <label className="text-eve-dim">{t("paramsTax")}</label>
          <input
            type="number"
            step="0.1"
            min="0"
            max="100"
            value={salesTax}
            onChange={(e) => setSalesTax(parseFloat(e.target.value) || 0)}
            className="w-16 px-1.5 py-1 bg-eve-input border border-eve-border rounded-sm text-xs text-eve-text"
          />
          <label className="text-eve-dim">{t("paramsBrokerFee")}</label>
          <input
            type="number"
            step="0.1"
            min="0"
            max="100"
            value={brokerFee}
            onChange={(e) => setBrokerFee(parseFloat(e.target.value) || 0)}
            className="w-16 px-1.5 py-1 bg-eve-input border border-eve-border rounded-sm text-xs text-eve-text"
          />
        </div>
        <button
          onClick={() => setShowNES((v) => !v)}
          className={`px-2 py-1 rounded-sm text-[10px] font-semibold uppercase tracking-wider border transition-all ${showNES ? "border-eve-accent/50 bg-eve-accent/10 text-eve-accent" : "border-eve-border bg-eve-panel text-eve-dim hover:text-eve-text"}`}
        >
          NES ▾
        </button>
        <button
          onClick={fetchData}
          disabled={loading}
          className="px-3 py-1.5 rounded-sm text-xs font-semibold uppercase tracking-wider bg-eve-accent text-eve-dark hover:bg-eve-accent-hover shadow-eve-glow disabled:opacity-50 disabled:cursor-not-allowed transition-all"
        >
          {loading ? t("plexLoading") : t("plexRefresh")}
        </button>
        {/* Auto-refresh toggle */}
        <div className="flex items-center gap-1.5">
          <button
            onClick={() => setAutoRefresh(v => !v)}
            className={`px-2 py-1 rounded-sm text-[10px] font-semibold uppercase tracking-wider border transition-all ${autoRefresh ? "border-eve-success/50 bg-eve-success/10 text-eve-success" : "border-eve-border bg-eve-panel text-eve-dim hover:text-eve-text"}`}
            title={t("plexAutoRefreshHint")}
          >
            {autoRefresh ? `⟳ ${formatCountdown(countdown)}` : t("plexAutoRefresh")}
          </button>
          {autoRefresh && (
            <select
              value={autoInterval}
              onChange={(e) => setAutoInterval(Number(e.target.value))}
              className="px-1 py-0.5 bg-eve-input border border-eve-border rounded-sm text-[10px] text-eve-text"
            >
              <option value={1}>1 {t("plexMin")}</option>
              <option value={2}>2 {t("plexMin")}</option>
              <option value={5}>5 {t("plexMin")}</option>
              <option value={10}>10 {t("plexMin")}</option>
              <option value={15}>15 {t("plexMin")}</option>
              <option value={30}>30 {t("plexMin")}</option>
              <option value={60}>60 {t("plexMin")}</option>
            </select>
          )}
        </div>
        {/* Alert bell */}
        <div className="relative">
          <button
            onClick={() => setShowAlerts(v => !v)}
            className={`px-2 py-1 rounded-sm text-[10px] font-semibold border transition-all ${showAlerts ? "border-eve-warning/50 bg-eve-warning/10 text-eve-warning" : "border-eve-border bg-eve-panel text-eve-dim hover:text-eve-text"}`}
            title={t("plexAlerts")}
          >
            🔔
          </button>
          {showAlerts && <PlexAlertPanel onClose={() => setShowAlerts(false)} />}
        </div>
        {error && <span className="text-xs text-eve-error">{error}</span>}
      </div>

      {/* NES price overrides (collapsible) */}
      {showNES && (
        <div className="flex items-center gap-3 flex-wrap shrink-0 px-2 py-1.5 bg-eve-panel/50 border border-eve-border/50 rounded-sm">
          <span className="text-[10px] text-eve-dim uppercase tracking-wider font-medium">{t("plexNESPrices")}</span>
          <div className="flex items-center gap-1.5 text-xs">
            <label className="text-eve-dim">Extractor</label>
            <input type="number" min="1" value={nesExtractor} onChange={(e) => setNesExtractor(parseInt(e.target.value) || 0)}
              className="w-16 px-1.5 py-0.5 bg-eve-input border border-eve-border rounded-sm text-xs text-eve-text font-mono" />
            <span className="text-eve-dim text-[10px]">PLEX</span>
          </div>
          <div className="flex items-center gap-1.5 text-xs">
            <label className="text-eve-dim">MPTC</label>
            <input type="number" min="1" value={nesMPTC} onChange={(e) => setNesMPTC(parseInt(e.target.value) || 0)}
              className="w-16 px-1.5 py-0.5 bg-eve-input border border-eve-border rounded-sm text-xs text-eve-text font-mono" />
            <span className="text-eve-dim text-[10px]">PLEX</span>
          </div>
          <div className="flex items-center gap-1.5 text-xs">
            <label className="text-eve-dim">Omega</label>
            <input type="number" min="1" value={nesOmega} onChange={(e) => setNesOmega(parseInt(e.target.value) || 0)}
              className="w-16 px-1.5 py-0.5 bg-eve-input border border-eve-border rounded-sm text-xs text-eve-text font-mono" />
            <span className="text-eve-dim text-[10px]">PLEX</span>
          </div>
          <span className="text-[10px] text-eve-dim italic">{t("plexNESHint")}</span>
        </div>
      )}

      {!dashboard && !loading && !error && (
        <div className="flex-1 flex items-center justify-center text-eve-dim text-sm">{t("plexEmpty")}</div>
      )}

      {dashboard && (
        <>
          {/* Sub-tab navigation */}
          <nav className="shrink-0 flex border-b border-eve-border">
            {(["market", "spfarm", "analytics"] as PlexSubTab[]).map(st => {
              const labels: Record<PlexSubTab, string> = {
                market: t("plexSubMarket"),
                spfarm: t("plexSubSPFarm"),
                analytics: t("plexSubAnalytics"),
              };
              return (
                <button
                  key={st}
                  onClick={() => setSubTab(st)}
                  className={`px-4 py-2 text-xs font-semibold uppercase tracking-wider border-b-2 transition-colors ${subTab === st ? "text-eve-accent border-eve-accent" : "text-eve-dim border-transparent hover:text-eve-text"}`}
                >
                  {labels[st]}
                </button>
              );
            })}
          </nav>

          {/* ==================== MARKET SUB-TAB ==================== */}
          {subTab === "market" && (
            <>
              {/* Signal + Global PLEX Price */}
              <div className="grid grid-cols-1 lg:grid-cols-[280px_1fr] gap-3 shrink-0">
                {signal && <SignalCard signal={signal} indicators={ind} />}
                <GlobalPriceCard price={dashboard.plex_price} indicators={ind} />
              </div>

              {/* Price Chart */}
              <div className="bg-eve-dark border border-eve-border rounded-sm p-3 shrink-0">
                <h3 className="text-xs font-semibold text-eve-dim uppercase tracking-wider mb-2">{t("plexPriceChart")}</h3>
                <PLEXChart history={dashboard.history} overlays={dashboard.chart_overlays} themeKey={themeKey} />
              </div>

              {/* Arbitrage Matrix (full width) */}
              <div className="bg-eve-dark border border-eve-border rounded-sm p-3 shrink-0">
                <div className="flex items-center gap-0 mb-2">
                  <button
                    onClick={() => setArbTab("nes")}
                    className={`px-3 py-1.5 text-[10px] font-semibold uppercase tracking-wider border-b-2 transition-colors ${arbTab === "nes" ? "text-eve-accent border-eve-accent" : "text-eve-dim border-transparent hover:text-eve-text"}`}
                  >
                    {t("plexArbTabNES")}
                  </button>
                  <button
                    onClick={() => setArbTab("spread")}
                    className={`px-3 py-1.5 text-[10px] font-semibold uppercase tracking-wider border-b-2 transition-colors ${arbTab === "spread" ? "text-eve-accent border-eve-accent" : "text-eve-dim border-transparent hover:text-eve-text"}`}
                  >
                    {t("plexArbTabSpread")}
                  </button>
                </div>
                <div className="overflow-x-auto table-scroll-wrapper table-scroll-container">
                  {arbTab === "nes" ? (
                    <table className="w-full text-xs">
                      <thead>
                        <tr className="text-eve-dim border-b border-eve-border">
                          <th className="text-left py-1.5 px-2 font-medium">{t("plexPath")}</th>
                          <th className="text-right py-1.5 px-2 font-medium">PLEX</th>
                          <th className="text-right py-1.5 px-2 font-medium">{t("plexCost")}</th>
                          <th className="text-right py-1.5 px-2 font-medium">{t("plexRevenue")}</th>
                          <th className="text-right py-1.5 px-2 font-medium">{t("plexProfit")}</th>
                          <th className="text-right py-1.5 px-2 font-medium">ROI</th>
                        </tr>
                      </thead>
                      <tbody>
                        {dashboard.arbitrage.filter(a => a.type !== "spread").map((arb, i) => (
                          <ArbitrageRow key={`nes-${i}`} arb={arb} onClick={() => setSelectedArb(arb)} />
                        ))}
                      </tbody>
                    </table>
                  ) : (
                    <table className="w-full text-xs">
                      <thead>
                        <tr className="text-eve-dim border-b border-eve-border">
                          <th className="text-left py-1.5 px-2 font-medium">{t("plexPath")}</th>
                          <th className="text-right py-1.5 px-2 font-medium">{t("plexCost")}</th>
                          <th className="text-right py-1.5 px-2 font-medium">{t("plexRevenue")}</th>
                          <th className="text-right py-1.5 px-2 font-medium">{t("plexProfit")}</th>
                          <th className="text-right py-1.5 px-2 font-medium">ROI</th>
                        </tr>
                      </thead>
                      <tbody>
                        {dashboard.arbitrage.filter(a => a.type === "spread").map((arb, i) => (
                          <ArbitrageRow key={`spread-${i}`} arb={arb} onClick={() => setSelectedArb(arb)} />
                        ))}
                      </tbody>
                    </table>
                  )}
                </div>
              </div>

              {/* Cross-Hub Arbitrage */}
              {dashboard.cross_hub && dashboard.cross_hub.length > 0 && (
                <CrossHubCard items={dashboard.cross_hub} />
              )}
            </>
          )}

          {/* ==================== SP FARM SUB-TAB ==================== */}
          {subTab === "spfarm" && (
            <>
              {/* SP Farm Calculator (full width) */}
              <SPFarmCard farm={dashboard.sp_farm} />

              {/* Injection Tiers + Fleet Manager */}
              <div className="grid grid-cols-1 lg:grid-cols-2 gap-3 shrink-0">
                {dashboard.injection_tiers && dashboard.injection_tiers.length > 0 && (
                  <InjectionTiersCard tiers={dashboard.injection_tiers} />
                )}
                <FleetManagerCard spFarm={dashboard.sp_farm} />
              </div>
            </>
          )}

          {/* ==================== ANALYTICS SUB-TAB ==================== */}
          {subTab === "analytics" && (
            <>
              {/* Historical Arb Chart + Market Depth */}
              <div className="grid grid-cols-1 lg:grid-cols-[1fr_340px] gap-3 shrink-0">
                {dashboard.arb_history && (
                  <ArbHistoryChart data={dashboard.arb_history} themeKey={themeKey} />
                )}
                {dashboard.market_depth && (
                  <MarketDepthCard depth={dashboard.market_depth} />
                )}
              </div>

              {/* Omega Comparator */}
              <OmegaComparatorCard
                omega={dashboard.omega_comparison ?? null}
                omegaUSD={omegaUSD}
                onOmegaUSDChange={setOmegaUSD}
                plexPrice={dashboard.plex_price.sell_price}
                nesOmega={nesOmega}
              />
            </>
          )}
        </>
      )}

      {/* Arbitrage detail modal */}
      {selectedArb && (
        <ArbitrageModal arb={selectedArb} onClose={() => setSelectedArb(null)} />
      )}
    </div>
  );
}
