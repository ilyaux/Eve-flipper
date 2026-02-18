import { useEffect, useState } from "react";
import { getCorpDashboard } from "../lib/api";
import { useI18n, type TranslationKey } from "../lib/i18n";
import type { CorpDashboard } from "../lib/types";
import {
  OverviewSection,
  WalletsSection,
  MembersSection,
  IndustrySection,
  MiningSection,
  MarketSection,
  type CorpTab,
} from "./corp-dashboard/CorpDashboardSections";

export function CorpDashboardApp() {
  const { t } = useI18n();
  const [dashboard, setDashboard] = useState<CorpDashboard | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [tab, setTab] = useState<CorpTab>("overview");

  // Read mode from URL search params
  const mode = new URLSearchParams(window.location.search).get("mode") === "live" ? "live" : "demo";

  useEffect(() => {
    setLoading(true);
    setError(null);
    getCorpDashboard(mode)
      .then(setDashboard)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, [mode]);

  const formatIsk = (value: number) => {
    if (Math.abs(value) >= 1e12) return `${(value / 1e12).toFixed(2)}T`;
    if (Math.abs(value) >= 1e9) return `${(value / 1e9).toFixed(2)}B`;
    if (Math.abs(value) >= 1e6) return `${(value / 1e6).toFixed(2)}M`;
    if (Math.abs(value) >= 1e3) return `${(value / 1e3).toFixed(1)}K`;
    return value.toFixed(0);
  };

  if (loading) {
    return (
      <div className="min-h-screen bg-eve-bg flex items-center justify-center">
        <div className="flex flex-col items-center gap-3">
          <span className="inline-block w-8 h-8 border-3 border-eve-accent/40 border-t-eve-accent rounded-full animate-spin" />
          <span className="text-eve-dim text-sm">{t("corpLoading")}</span>
        </div>
      </div>
    );
  }

  if (error || !dashboard) {
    return (
      <div className="min-h-screen bg-eve-bg flex items-center justify-center">
        <div className="flex flex-col items-center gap-3 max-w-md text-center">
          <div className="text-eve-error text-sm">{t("corpError")}</div>
          <div className="text-eve-dim text-xs">{error}</div>
          <button
            onClick={() => window.location.reload()}
            className="px-4 py-2 text-xs bg-eve-accent/10 border border-eve-accent text-eve-accent rounded-sm hover:bg-eve-accent/20 transition-colors"
          >
            Retry
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="h-screen flex flex-col bg-eve-bg text-eve-text overflow-hidden">
      {/* Top Bar - fixed height */}
      <header className="shrink-0 bg-eve-panel border-b border-eve-border px-4 py-3 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <svg className="w-6 h-6 text-eve-accent" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" />
          </svg>
          <div>
            <h1 className="text-lg font-bold text-eve-text">
              [{dashboard.info.ticker}] {dashboard.info.name}
            </h1>
            <div className="text-xs text-eve-dim">
              {dashboard.info.member_count} members
            </div>
          </div>
          {/* Mode badge */}
          <span className={`ml-2 px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider rounded-sm ${
            dashboard.is_demo
              ? "bg-amber-500/20 text-amber-400 border border-amber-500/30"
              : "bg-emerald-500/20 text-emerald-400 border border-emerald-500/30"
          }`}>
            {dashboard.is_demo ? t("corpDemoMode") : t("corpLiveMode")}
          </span>
        </div>
        <button
          onClick={() => window.close()}
          className="px-3 py-1.5 text-xs text-eve-dim hover:text-eve-text border border-eve-border rounded-sm hover:border-eve-accent/50 transition-colors"
        >
          {t("corpBackToFlipper")}
        </button>
      </header>

      {/* Tabs - fixed height */}
      <nav className="shrink-0 bg-eve-panel border-b border-eve-border flex overflow-x-auto scrollbar-thin">
        {(["overview", "wallets", "members", "industry", "mining", "market"] as CorpTab[]).map((ct) => {
          const labels: Record<CorpTab, TranslationKey> = {
            overview: "corpOverview",
            wallets: "corpWallets",
            members: "corpMembers",
            industry: "corpIndustry",
            mining: "corpMining",
            market: "corpMarket",
          };
          return (
            <button
              key={ct}
              onClick={() => setTab(ct)}
              className={`px-5 py-2.5 text-xs font-medium transition-colors whitespace-nowrap ${
                tab === ct
                  ? "text-eve-accent border-b-2 border-eve-accent bg-eve-dark/50"
                  : "text-eve-dim hover:text-eve-text"
              }`}
            >
              {t(labels[ct])}
            </button>
          );
        })}
      </nav>

      {/* Content - scrollable area fills remaining height */}
      <main className="flex-1 overflow-y-auto">
        <div className="max-w-7xl mx-auto p-4 sm:p-6">
          {tab === "overview" && <OverviewSection dashboard={dashboard} formatIsk={formatIsk} t={t} setTab={setTab} />}
          {tab === "wallets" && <WalletsSection wallets={dashboard.wallets} mode={mode} formatIsk={formatIsk} t={t} />}
          {tab === "members" && <MembersSection dashboard={dashboard} mode={mode} formatIsk={formatIsk} t={t} />}
          {tab === "industry" && <IndustrySection dashboard={dashboard} mode={mode} formatIsk={formatIsk} t={t} />}
          {tab === "mining" && <MiningSection dashboard={dashboard} mode={mode} formatIsk={formatIsk} t={t} />}
          {tab === "market" && <MarketSection dashboard={dashboard} mode={mode} formatIsk={formatIsk} t={t} />}
        </div>
      </main>
    </div>
  );
}
