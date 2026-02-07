import { useEffect, useState, useMemo, useCallback } from "react";
import { getCorpDashboard, getCorpJournal, getCorpMembers, getCorpOrders, getCorpIndustryJobs, getCorpMiningLedger } from "../lib/api";
import { useI18n, type TranslationKey } from "../lib/i18n";
import type { CorpDashboard, CorpIndustryJob, CorpJournalEntry, CorpMarketOrderDetail, CorpMember, CorpMiningEntry, CorpWalletDivision, DailyPnLEntry, IncomeSource, MemberContribution } from "../lib/types";

type CorpTab = "overview" | "wallets" | "members" | "industry" | "mining" | "market";

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

// ============================================================
// Overview Section
// ============================================================

function OverviewSection({
  dashboard,
  formatIsk,
  t,
  setTab,
}: {
  dashboard: CorpDashboard;
  formatIsk: (v: number) => string;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
  setTab: (tab: CorpTab) => void;
}) {
  return (
    <div className="space-y-6">
      {/* KPI Cards - clickable */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
        <button onClick={() => setTab("wallets")} className="text-left"><KpiCard label={t("corpTotalBalance")} value={`${formatIsk(dashboard.total_balance)} ISK`} color="text-eve-accent" large /></button>
        <KpiCard label={t("corpRevenue30d")} value={`+${formatIsk(dashboard.revenue_30d)} ISK`} color="text-eve-profit" />
        <KpiCard label={t("corpExpenses30d")} value={`-${formatIsk(dashboard.expenses_30d)} ISK`} color="text-eve-error" />
        <KpiCard
          label={t("corpNetIncome30d")}
          value={`${dashboard.net_income_30d >= 0 ? "+" : ""}${formatIsk(dashboard.net_income_30d)} ISK`}
          color={dashboard.net_income_30d >= 0 ? "text-eve-profit" : "text-eve-error"}
          large
        />
      </div>

      {/* 7-day cards */}
      <div className="grid grid-cols-3 gap-4">
        <KpiCard label={t("corpRevenue7d")} value={`+${formatIsk(dashboard.revenue_7d)} ISK`} color="text-eve-profit" />
        <KpiCard label={t("corpExpenses7d")} value={`-${formatIsk(dashboard.expenses_7d)} ISK`} color="text-eve-error" />
        <KpiCard
          label={t("corpNetIncome7d")}
          value={`${dashboard.net_income_7d >= 0 ? "+" : ""}${formatIsk(dashboard.net_income_7d)} ISK`}
          color={dashboard.net_income_7d >= 0 ? "text-eve-profit" : "text-eve-error"}
        />
      </div>

      {/* Income by Source + Daily P&L */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Income by Source */}
        <div className="bg-eve-panel border border-eve-border rounded-sm p-4">
          <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-3">{t("corpIncomeBySource")}</div>
          <IncomeSourceChart sources={dashboard.income_by_source} formatIsk={formatIsk} />
        </div>

        {/* Daily P&L */}
        <div className="bg-eve-panel border border-eve-border rounded-sm p-4">
          <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-3">{t("corpDailyPnl")}</div>
          <DailyPnLChart entries={dashboard.daily_pnl} formatIsk={formatIsk} />
        </div>
      </div>

      {/* Industry + Mining + Market summary cards */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        {/* Industry */}
        <button onClick={() => setTab("industry")} className="bg-eve-panel border border-eve-border rounded-sm p-4 text-left hover:border-eve-accent/50 transition-colors">
          <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-2">{t("corpIndustry")}</div>
          <div className="flex items-center justify-between">
            <div>
              <div className="text-lg font-bold text-eve-accent">{dashboard.industry_summary.active_jobs}</div>
              <div className="text-[10px] text-eve-dim">{t("corpActiveJobs")}</div>
            </div>
            <div className="text-right">
              <div className="text-sm font-bold text-eve-profit">{formatIsk(dashboard.industry_summary.production_value)} ISK</div>
              <div className="text-[10px] text-eve-dim">{t("corpCompletedJobs")}: {dashboard.industry_summary.completed_jobs_30d}</div>
            </div>
          </div>
        </button>
        {/* Mining */}
        <button onClick={() => setTab("mining")} className="bg-eve-panel border border-eve-border rounded-sm p-4 text-left hover:border-eve-accent/50 transition-colors">
          <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-2">{t("corpMining")}</div>
          <div className="flex items-center justify-between">
            <div>
              <div className="text-lg font-bold text-eve-accent">{dashboard.mining_summary.active_miners}</div>
              <div className="text-[10px] text-eve-dim">{t("corpActiveMiners")}</div>
            </div>
            <div className="text-right">
              <div className="text-sm font-bold text-eve-profit">{formatIsk(dashboard.mining_summary.estimated_isk)} ISK</div>
              <div className="text-[10px] text-eve-dim">{dashboard.mining_summary.total_volume_30d.toLocaleString()} units</div>
            </div>
          </div>
        </button>
        {/* Market */}
        <button onClick={() => setTab("market")} className="bg-eve-panel border border-eve-border rounded-sm p-4 text-left hover:border-eve-accent/50 transition-colors">
          <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-2">{t("corpMarket")}</div>
          <div className="flex items-center justify-between">
            <div>
              <div className="text-lg font-bold text-eve-accent">{dashboard.market_summary.unique_traders}</div>
              <div className="text-[10px] text-eve-dim">{t("corpUniqueTraders")}</div>
            </div>
            <div className="text-right">
              <div className="text-sm font-bold text-eve-profit">{formatIsk(dashboard.market_summary.total_sell_value)} ISK</div>
              <div className="text-[10px] text-eve-dim">{dashboard.market_summary.active_buy_orders + dashboard.market_summary.active_sell_orders} {t("corpOrders").toLowerCase()}</div>
            </div>
          </div>
        </button>
      </div>

      {/* Top Contributors */}
      <div className="bg-eve-panel border border-eve-border rounded-sm p-4">
        <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-3">{t("corpTopContributors")}</div>
        <TopContributorsTable contributors={dashboard.top_contributors} formatIsk={formatIsk} />
      </div>

      {/* Member Breakdown */}
      <div className="bg-eve-panel border border-eve-border rounded-sm p-4">
        <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-3">{t("corpMemberBreakdown")}</div>
        <div className="grid grid-cols-2 sm:grid-cols-4 lg:grid-cols-7 gap-3">
          <MiniKpi label={t("corpMembers")} value={dashboard.member_summary.total_members} />
          <MiniKpi label={t("corpMembersActive7d")} value={dashboard.member_summary.active_last_7d} color="text-emerald-400" />
          <MiniKpi label={t("corpMembersActive30d")} value={dashboard.member_summary.active_last_30d} />
          <MiniKpi label={t("corpMembersInactive")} value={dashboard.member_summary.inactive_30d} color="text-eve-error" />
          <MiniKpi label={t("corpMiners")} value={dashboard.member_summary.miners} />
          <MiniKpi label={t("corpRatters")} value={dashboard.member_summary.ratters} />
          <MiniKpi label={t("corpTraders")} value={dashboard.member_summary.traders} />
        </div>
      </div>
    </div>
  );
}

// ============================================================
// Wallets Section
// ============================================================

function WalletsSection({
  wallets,
  mode,
  formatIsk,
  t,
}: {
  wallets: CorpWalletDivision[];
  mode: "demo" | "live";
  formatIsk: (v: number) => string;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}) {
  const totalBalance = wallets.reduce((s, w) => s + w.balance, 0);
  const maxBalance = Math.max(...wallets.map((w) => Math.abs(w.balance)), 1);
  const [expandedDiv, setExpandedDiv] = useState<number | null>(null);
  const [journal, setJournal] = useState<CorpJournalEntry[]>([]);
  const [journalLoading, setJournalLoading] = useState(false);

  const toggleDivision = (div: number) => {
    if (expandedDiv === div) {
      setExpandedDiv(null);
      return;
    }
    setExpandedDiv(div);
    setJournalLoading(true);
    getCorpJournal(mode, div, 30)
      .then(setJournal)
      .catch(() => setJournal([]))
      .finally(() => setJournalLoading(false));
  };

  return (
    <div className="space-y-4">
      <div className="bg-eve-panel border border-eve-border rounded-sm p-4">
        <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-1">{t("corpTotalBalance")}</div>
        <div className="text-2xl font-bold text-eve-accent">{formatIsk(totalBalance)} ISK</div>
      </div>
      <div className="space-y-2">
        {wallets.map((w) => {
          const pct = maxBalance > 0 ? (Math.abs(w.balance) / maxBalance) * 100 : 0;
          const isExpanded = expandedDiv === w.division;
          return (
            <div key={w.division}>
              <button
                onClick={() => toggleDivision(w.division)}
                className={`w-full bg-eve-panel border rounded-sm p-3 flex items-center gap-4 transition-colors hover:border-eve-accent/50 ${
                  isExpanded ? "border-eve-accent" : "border-eve-border"
                }`}
              >
                <div className="w-8 h-8 flex items-center justify-center bg-eve-accent/10 rounded-sm text-eve-accent text-sm font-bold">
                  {w.division}
                </div>
                <div className="flex-1 text-left">
                  <div className="flex items-center justify-between mb-1">
                    <span className="text-sm text-eve-text font-medium">{w.name}</span>
                    <span className="text-sm text-eve-accent font-bold">{formatIsk(w.balance)} ISK</span>
                  </div>
                  <div className="h-1.5 bg-eve-dark rounded-full overflow-hidden">
                    <div className="h-full bg-eve-accent/60 rounded-full" style={{ width: `${pct}%` }} />
                  </div>
                </div>
                <svg className={`w-4 h-4 text-eve-dim transition-transform ${isExpanded ? "rotate-180" : ""}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
                </svg>
              </button>
              {isExpanded && (
                <div className="bg-eve-dark/60 border border-eve-border border-t-0 rounded-b-sm p-3">
                  {journalLoading ? (
                    <div className="flex items-center justify-center py-4 text-eve-dim text-xs">
                      <span className="inline-block w-4 h-4 border-2 border-eve-accent/40 border-t-eve-accent rounded-full animate-spin mr-2" />
                      Loading journal...
                    </div>
                  ) : journal.length === 0 ? (
                    <div className="text-center text-eve-dim text-xs py-4">No journal entries</div>
                  ) : (
                    <div className="border border-eve-border rounded-sm overflow-hidden max-h-80 overflow-y-auto">
                      <table className="w-full text-xs">
                        <thead className="bg-eve-panel sticky top-0">
                          <tr className="text-eve-dim">
                            <th className="px-2 py-1.5 text-left">Date</th>
                            <th className="px-2 py-1.5 text-left">Type</th>
                            <th className="px-2 py-1.5 text-left">From</th>
                            <th className="px-2 py-1.5 text-right">Amount</th>
                            <th className="px-2 py-1.5 text-right">Balance</th>
                          </tr>
                        </thead>
                        <tbody>
                          {journal.slice(0, 50).map((j) => (
                            <tr key={j.id} className="border-t border-eve-border/30 hover:bg-eve-panel/50">
                              <td className="px-2 py-1.5 text-eve-dim whitespace-nowrap">
                                {j.date.slice(0, 10)}
                              </td>
                              <td className="px-2 py-1.5 text-eve-dim max-w-[140px] truncate" title={j.ref_type}>
                                {j.ref_type.replace(/_/g, " ")}
                              </td>
                              <td className="px-2 py-1.5 text-eve-text max-w-[120px] truncate" title={j.first_party_name}>
                                {j.first_party_name || "—"}
                              </td>
                              <td className={`px-2 py-1.5 text-right font-mono ${j.amount >= 0 ? "text-eve-profit" : "text-eve-error"}`}>
                                {j.amount >= 0 ? "+" : ""}{formatIsk(j.amount)}
                              </td>
                              <td className="px-2 py-1.5 text-right text-eve-dim font-mono">
                                {formatIsk(j.balance)}
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                      {journal.length > 50 && (
                        <div className="text-center text-eve-dim text-[10px] py-1 bg-eve-panel">
                          +{journal.length - 50} more entries
                        </div>
                      )}
                    </div>
                  )}
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

// ============================================================
// Members Section
// ============================================================

function MembersSection({
  dashboard,
  mode,
  formatIsk,
  t,
}: {
  dashboard: CorpDashboard;
  mode: "demo" | "live";
  formatIsk: (v: number) => string;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}) {
  const ms = dashboard.member_summary;
  const [members, setMembers] = useState<CorpMember[]>([]);
  const [membersLoading, setMembersLoading] = useState(false);
  const [search, setSearch] = useState("");
  const [sortKey, setSortKey] = useState<"name" | "last_login" | "system" | "ship">("last_login");
  const [sortAsc, setSortAsc] = useState(false);

  // Load members on first render
  useEffect(() => {
    setMembersLoading(true);
    getCorpMembers(mode)
      .then(setMembers)
      .catch(() => setMembers([]))
      .finally(() => setMembersLoading(false));
  }, [mode]);

  const toggleSort = (key: typeof sortKey) => {
    if (sortKey === key) setSortAsc(!sortAsc);
    else { setSortKey(key); setSortAsc(key === "name"); }
  };

  const filteredMembers = useMemo(() => {
    let list = [...members];
    if (search.trim()) {
      const q = search.toLowerCase();
      list = list.filter((m) => m.name.toLowerCase().includes(q) || m.system_name?.toLowerCase().includes(q) || m.ship_name?.toLowerCase().includes(q));
    }
    list.sort((a, b) => {
      let cmp = 0;
      switch (sortKey) {
        case "name": cmp = a.name.localeCompare(b.name); break;
        case "last_login": cmp = (a.last_login || "").localeCompare(b.last_login || ""); break;
        case "system": cmp = (a.system_name || "").localeCompare(b.system_name || ""); break;
        case "ship": cmp = (a.ship_name || "").localeCompare(b.ship_name || ""); break;
      }
      return sortAsc ? cmp : -cmp;
    });
    return list;
  }, [members, search, sortKey, sortAsc]);

  const categories = [
    { label: t("corpMiners"), value: ms.miners, color: "bg-blue-400" },
    { label: t("corpRatters"), value: ms.ratters, color: "bg-emerald-400" },
    { label: t("corpTraders"), value: ms.traders, color: "bg-amber-400" },
    { label: t("corpIndustrialists"), value: ms.industrialists, color: "bg-purple-400" },
    { label: t("corpPvPers"), value: ms.pvpers, color: "bg-red-400" },
    { label: t("corpOther"), value: ms.other, color: "bg-gray-400" },
  ];
  const total = ms.total_members || 1;

  const isOnline = (m: CorpMember) => {
    if (!m.last_login) return false;
    const diff = Date.now() - new Date(m.last_login).getTime();
    return diff < 15 * 60 * 1000;
  };

  const timeSince = (dateStr: string) => {
    if (!dateStr) return "Never";
    const diff = Date.now() - new Date(dateStr).getTime();
    const mins = Math.floor(diff / 60000);
    if (mins < 1) return "Just now";
    if (mins < 60) return `${mins}m ago`;
    const hours = Math.floor(mins / 60);
    if (hours < 24) return `${hours}h ago`;
    const days = Math.floor(hours / 24);
    if (days < 30) return `${days}d ago`;
    return `${Math.floor(days / 30)}mo ago`;
  };

  const SortHeader = ({ label, field }: { label: string; field: typeof sortKey }) => (
    <th
      className="px-2 py-1.5 text-left cursor-pointer hover:text-eve-accent transition-colors select-none"
      onClick={() => toggleSort(field)}
    >
      {label} {sortKey === field && (sortAsc ? "↑" : "↓")}
    </th>
  );

  return (
    <div className="space-y-6">
      {/* Activity summary */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
        <KpiCard label={t("corpMembers")} value={String(ms.total_members)} />
        <KpiCard label={t("corpMembersActive7d")} value={String(ms.active_last_7d)} color="text-emerald-400" />
        <KpiCard label={t("corpMembersActive30d")} value={String(ms.active_last_30d)} color="text-eve-accent" />
        <KpiCard label={t("corpMembersInactive")} value={String(ms.inactive_30d)} color="text-eve-error" />
      </div>

      {/* Category breakdown bar */}
      <div className="bg-eve-panel border border-eve-border rounded-sm p-4">
        <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-3">{t("corpMemberBreakdown")}</div>
        <div className="flex h-6 rounded-sm overflow-hidden">
          {categories.filter((c) => c.value > 0).map((c, i) => (
            <div
              key={i}
              className={`${c.color} flex items-center justify-center text-[9px] font-bold text-black/70`}
              style={{ width: `${(c.value / total) * 100}%` }}
              title={`${c.label}: ${c.value}`}
            >
              {(c.value / total) * 100 > 8 ? c.value : ""}
            </div>
          ))}
        </div>
        <div className="flex flex-wrap gap-3 mt-3">
          {categories.filter((c) => c.value > 0).map((c, i) => (
            <div key={i} className="flex items-center gap-1.5 text-xs text-eve-dim">
              <div className={`w-2.5 h-2.5 rounded-sm ${c.color}`} />
              {c.label} ({c.value})
            </div>
          ))}
        </div>
      </div>

      {/* Top Contributors */}
      <div className="bg-eve-panel border border-eve-border rounded-sm p-4">
        <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-3">{t("corpTopContributors")}</div>
        <TopContributorsTable contributors={dashboard.top_contributors} formatIsk={formatIsk} />
      </div>

      {/* Full member list */}
      <div className="bg-eve-panel border border-eve-border rounded-sm p-4">
        <div className="flex items-center justify-between mb-3">
          <div className="text-[10px] text-eve-dim uppercase tracking-wider">{t("corpMembers")} ({members.length})</div>
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search..."
            className="px-2 py-1 text-xs bg-eve-dark border border-eve-border rounded-sm text-eve-text placeholder:text-eve-dim/50 w-48 focus:border-eve-accent outline-none"
          />
        </div>
        {membersLoading ? (
          <div className="flex items-center justify-center py-8 text-eve-dim text-xs">
            <span className="inline-block w-4 h-4 border-2 border-eve-accent/40 border-t-eve-accent rounded-full animate-spin mr-2" />
            Loading members...
          </div>
        ) : (
          <div className="border border-eve-border rounded-sm overflow-hidden max-h-[500px] overflow-y-auto">
            <table className="w-full text-xs">
              <thead className="bg-eve-panel sticky top-0 text-eve-dim">
                <tr>
                  <th className="px-2 py-1.5 w-6"></th>
                  <SortHeader label="Name" field="name" />
                  <SortHeader label="Last Seen" field="last_login" />
                  <SortHeader label="Ship" field="ship" />
                  <SortHeader label="System" field="system" />
                </tr>
              </thead>
              <tbody>
                {filteredMembers.map((m) => {
                  const online = isOnline(m);
                  return (
                    <tr key={m.character_id} className="border-t border-eve-border/30 hover:bg-eve-panel/50">
                      <td className="px-2 py-1.5 text-center">
                        <span className={`inline-block w-2 h-2 rounded-full ${online ? "bg-emerald-400" : "bg-eve-dim/30"}`} />
                      </td>
                      <td className="px-2 py-1.5">
                        <div className="flex items-center gap-2">
                          <img
                            src={`https://images.evetech.net/characters/${m.character_id}/portrait?size=32`}
                            alt=""
                            className="w-5 h-5 rounded-sm"
                          />
                          <span className="text-eve-text font-medium">{m.name}</span>
                        </div>
                      </td>
                      <td className={`px-2 py-1.5 ${online ? "text-emerald-400" : "text-eve-dim"}`}>
                        {timeSince(m.last_login)}
                      </td>
                      <td className="px-2 py-1.5 text-eve-dim max-w-[140px] truncate" title={m.ship_name}>
                        {m.ship_name || "—"}
                      </td>
                      <td className="px-2 py-1.5 text-eve-dim max-w-[120px] truncate" title={m.system_name}>
                        {m.system_name || "—"}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}

// ============================================================
// Industry Section
// ============================================================

function IndustrySection({
  dashboard,
  mode,
  formatIsk,
  t,
}: {
  dashboard: CorpDashboard;
  mode: "demo" | "live";
  formatIsk: (v: number) => string;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}) {
  const ind = dashboard.industry_summary;
  const [jobs, setJobs] = useState<CorpIndustryJob[]>([]);
  const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState("");
  const [sortKey, setSortKey] = useState<"product" | "installer" | "activity" | "status" | "runs" | "end_date">("end_date");
  const [sortAsc, setSortAsc] = useState(false);
  const [activityFilter, setActivityFilter] = useState("all");
  const [days, setDays] = useState(30);

  useEffect(() => {
    setLoading(true);
    getCorpIndustryJobs(mode).then(setJobs).catch(() => setJobs([])).finally(() => setLoading(false));
  }, [mode]);

  const toggleSort = (key: typeof sortKey) => {
    if (sortKey === key) setSortAsc(!sortAsc);
    else { setSortKey(key); setSortAsc(key === "product" || key === "installer"); }
  };

  const cutoff = useMemo(() => {
    if (days === 0) return "";
    const d = new Date(); d.setDate(d.getDate() - days);
    return d.toISOString().slice(0, 10);
  }, [days]);

  const activities = useMemo(() => {
    const set = new Set(jobs.map(j => j.activity));
    return Array.from(set).sort();
  }, [jobs]);

  const filtered = useMemo(() => {
    let list = [...jobs];
    if (cutoff) list = list.filter(j => j.start_date.slice(0, 10) >= cutoff);
    if (activityFilter !== "all") list = list.filter(j => j.activity === activityFilter);
    if (search.trim()) {
      const q = search.toLowerCase();
      list = list.filter(j => j.product_name?.toLowerCase().includes(q) || j.installer_name?.toLowerCase().includes(q) || j.location_name?.toLowerCase().includes(q));
    }
    list.sort((a, b) => {
      let cmp = 0;
      switch (sortKey) {
        case "product": cmp = (a.product_name || "").localeCompare(b.product_name || ""); break;
        case "installer": cmp = (a.installer_name || "").localeCompare(b.installer_name || ""); break;
        case "activity": cmp = a.activity.localeCompare(b.activity); break;
        case "status": cmp = a.status.localeCompare(b.status); break;
        case "runs": cmp = a.runs - b.runs; break;
        case "end_date": cmp = (a.end_date || "").localeCompare(b.end_date || ""); break;
      }
      return sortAsc ? cmp : -cmp;
    });
    return list;
  }, [jobs, cutoff, activityFilter, search, sortKey, sortAsc]);

  // Daily completed jobs trend
  const dailyTrend = useMemo(() => {
    const completed = jobs.filter(j => j.status === "delivered" && (!cutoff || j.end_date.slice(0, 10) >= cutoff));
    const byDay: Record<string, number> = {};
    completed.forEach(j => { const d = j.end_date.slice(0, 10); byDay[d] = (byDay[d] || 0) + 1; });
    return Object.entries(byDay).sort(([a], [b]) => a.localeCompare(b)).map(([date, count]) => ({ date, count }));
  }, [jobs, cutoff]);

  const SortHeader = ({ label, field }: { label: string; field: typeof sortKey }) => (
    <th className="px-2 py-1.5 text-left cursor-pointer hover:text-eve-accent transition-colors select-none" onClick={() => toggleSort(field)}>
      {label} {sortKey === field && (sortAsc ? "\u2191" : "\u2193")}
    </th>
  );

  const statusColor = (s: string) => {
    if (s === "active") return "bg-blue-500/20 text-blue-400 border-blue-500/30";
    if (s === "delivered") return "bg-emerald-500/20 text-emerald-400 border-emerald-500/30";
    if (s === "cancelled") return "bg-red-500/20 text-red-400 border-red-500/30";
    if (s === "ready") return "bg-amber-500/20 text-amber-400 border-amber-500/30";
    return "bg-eve-dim/20 text-eve-dim border-eve-dim/30";
  };

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 sm:grid-cols-3 gap-4">
        <KpiCard label={t("corpActiveJobs")} value={String(ind.active_jobs)} color="text-eve-accent" />
        <KpiCard label={t("corpCompletedJobs")} value={String(ind.completed_jobs_30d)} />
        <KpiCard label="Production Value" value={`${formatIsk(ind.production_value)} ISK`} color="text-eve-profit" />
      </div>

      {/* Controls */}
      <div className="flex flex-wrap items-center gap-3">
        <DateRangeSelector value={days} onChange={setDays} t={t} />
        <select value={activityFilter} onChange={e => setActivityFilter(e.target.value)} className="px-2 py-1 text-xs bg-eve-dark border border-eve-border rounded-sm text-eve-text">
          <option value="all">{t("corpAllActivities")}</option>
          {activities.map(a => <option key={a} value={a}>{a.replace(/_/g, " ")}</option>)}
        </select>
        <input type="text" value={search} onChange={e => setSearch(e.target.value)} placeholder={t("corpSearch")} className="px-2 py-1 text-xs bg-eve-dark border border-eve-border rounded-sm text-eve-text placeholder:text-eve-dim/50 w-48 focus:border-eve-accent outline-none" />
        <CsvExportButton data={filtered} filename="corp_industry" headers={["product_name","installer_name","activity","runs","status","location_name","start_date","end_date"]} t={t} />
      </div>

      {/* Trend chart */}
      {dailyTrend.length > 0 && (
        <BarChart
          data={dailyTrend.map(d => ({ date: d.date, value: d.count }))}
          label={t("corpJobsTrend")}
          formatValue={(v) => `${v} jobs`}
          color="blue"
        />
      )}

      {/* Jobs table */}
      <div className="bg-eve-panel border border-eve-border rounded-sm p-4">
        <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-3">{t("corpIndustry")} ({filtered.length})</div>
        {loading ? (
          <div className="flex items-center justify-center py-8 text-eve-dim text-xs">
            <span className="inline-block w-4 h-4 border-2 border-eve-accent/40 border-t-eve-accent rounded-full animate-spin mr-2" />
          </div>
        ) : (
          <div className="border border-eve-border rounded-sm overflow-hidden max-h-[500px] overflow-y-auto">
            <table className="w-full text-xs">
              <thead className="bg-eve-panel sticky top-0 text-eve-dim">
                <tr>
                  <SortHeader label={t("corpProduct")} field="product" />
                  <SortHeader label={t("corpInstaller")} field="installer" />
                  <SortHeader label={t("corpActivity")} field="activity" />
                  <SortHeader label={t("corpRuns")} field="runs" />
                  <SortHeader label={t("corpStatus")} field="status" />
                  <th className="px-2 py-1.5 text-left">{t("corpLocation")}</th>
                  <SortHeader label={t("corpEndDate")} field="end_date" />
                </tr>
              </thead>
              <tbody>
                {filtered.map(j => (
                  <tr key={j.job_id} className="border-t border-eve-border/30 hover:bg-eve-panel/50">
                    <td className="px-2 py-1.5">
                      <div className="flex items-center gap-2">
                        <img src={`https://images.evetech.net/types/${j.product_type_id}/icon?size=32`} alt="" className="w-4 h-4" />
                        <span className="text-eve-text">{j.product_name}</span>
                      </div>
                    </td>
                    <td className="px-2 py-1.5 text-eve-dim">{j.installer_name}</td>
                    <td className="px-2 py-1.5 text-eve-dim capitalize">{j.activity.replace(/_/g, " ")}</td>
                    <td className="px-2 py-1.5 text-eve-accent text-right">{j.runs}</td>
                    <td className="px-2 py-1.5">
                      <span className={`px-1.5 py-0.5 text-[10px] font-bold uppercase rounded-sm border ${statusColor(j.status)}`}>{j.status}</span>
                    </td>
                    <td className="px-2 py-1.5 text-eve-dim max-w-[140px] truncate" title={j.location_name}>{j.location_name}</td>
                    <td className="px-2 py-1.5 text-eve-dim whitespace-nowrap">{j.end_date?.slice(0, 10)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Top Products */}
      {ind.top_products && ind.top_products.length > 0 && (
        <div className="bg-eve-panel border border-eve-border rounded-sm p-4">
          <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-3">{t("corpTopProducts")}</div>
          <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3">
            {ind.top_products.slice(0, 10).map((p) => (
              <div key={p.type_id} className="bg-eve-dark/50 border border-eve-border/50 rounded-sm p-3">
                <div className="flex items-center gap-2 mb-1">
                  <img src={`https://images.evetech.net/types/${p.type_id}/icon?size=32`} alt="" className="w-5 h-5" />
                  <span className="text-xs text-eve-text font-medium truncate">{p.type_name}</span>
                </div>
                <div className="text-xs text-eve-accent font-bold">{p.runs} runs</div>
                <div className="text-[10px] text-eve-dim">{p.jobs} jobs</div>
                {p.estimated_isk ? <div className="text-[10px] text-eve-profit font-mono">{formatIsk(p.estimated_isk)} ISK</div> : null}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

// ============================================================
// Mining Section
// ============================================================

function MiningSection({
  dashboard,
  mode,
  formatIsk,
  t,
}: {
  dashboard: CorpDashboard;
  mode: "demo" | "live";
  formatIsk: (v: number) => string;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}) {
  const mining = dashboard.mining_summary;
  const [entries, setEntries] = useState<CorpMiningEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState("");
  const [days, setDays] = useState(30);
  const [expandedMiner, setExpandedMiner] = useState<number | null>(null);
  const [sortKey, setSortKey] = useState<"name" | "volume" | "types" | "isk" | "last">("volume");
  const [sortAsc, setSortAsc] = useState(false);

  useEffect(() => {
    setLoading(true);
    getCorpMiningLedger(mode).then(setEntries).catch(() => setEntries([])).finally(() => setLoading(false));
  }, [mode]);

  const cutoff = useMemo(() => {
    if (days === 0) return "";
    const d = new Date(); d.setDate(d.getDate() - days);
    return d.toISOString().slice(0, 10);
  }, [days]);

  const filteredEntries = useMemo(() => {
    let list = entries;
    if (cutoff) list = list.filter(e => e.date >= cutoff);
    return list;
  }, [entries, cutoff]);

  // Build ore ISK lookup from mining summary top_ores (which now have estimated_isk)
  const oreIskPerUnit = useMemo(() => {
    const m = new Map<number, number>();
    for (const ore of mining.top_ores) {
      if (ore.estimated_isk && ore.quantity > 0) {
        m.set(ore.type_id, ore.estimated_isk / ore.quantity);
      }
    }
    return m;
  }, [mining.top_ores]);

  // Aggregate by miner
  const minerAgg = useMemo(() => {
    const map = new Map<number, { id: number; name: string; volume: number; isk: number; types: Set<string>; lastDate: string }>();
    filteredEntries.forEach(e => {
      const unitIsk = oreIskPerUnit.get(e.type_id) || 0;
      const existing = map.get(e.character_id);
      if (existing) {
        existing.volume += e.quantity;
        existing.isk += e.quantity * unitIsk;
        existing.types.add(e.type_name);
        if (e.date > existing.lastDate) existing.lastDate = e.date;
      } else {
        map.set(e.character_id, { id: e.character_id, name: e.character_name || `Miner ${e.character_id}`, volume: e.quantity, isk: e.quantity * unitIsk, types: new Set([e.type_name]), lastDate: e.date });
      }
    });
    let miners = Array.from(map.values());
    if (search.trim()) {
      const q = search.toLowerCase();
      miners = miners.filter(m => m.name.toLowerCase().includes(q));
    }
    miners.sort((a, b) => {
      let cmp = 0;
      switch (sortKey) {
        case "name": cmp = a.name.localeCompare(b.name); break;
        case "volume": cmp = a.volume - b.volume; break;
        case "types": cmp = a.types.size - b.types.size; break;
        case "isk": cmp = a.isk - b.isk; break;
        case "last": cmp = a.lastDate.localeCompare(b.lastDate); break;
      }
      return sortAsc ? cmp : -cmp;
    });
    return miners;
  }, [filteredEntries, oreIskPerUnit, search, sortKey, sortAsc]);

  // Daily volume trend
  const dailyTrend = useMemo(() => {
    const byDay: Record<string, number> = {};
    filteredEntries.forEach(e => { byDay[e.date] = (byDay[e.date] || 0) + e.quantity; });
    return Object.entries(byDay).sort(([a], [b]) => a.localeCompare(b)).map(([date, vol]) => ({ date, volume: vol }));
  }, [filteredEntries]);

  const toggleSort = (key: typeof sortKey) => {
    if (sortKey === key) setSortAsc(!sortAsc);
    else { setSortKey(key); setSortAsc(key === "name"); }
  };

  const SortHeader = ({ label, field }: { label: string; field: typeof sortKey }) => (
    <th className="px-2 py-1.5 text-left cursor-pointer hover:text-eve-accent transition-colors select-none" onClick={() => toggleSort(field)}>
      {label} {sortKey === field && (sortAsc ? "\u2191" : "\u2193")}
    </th>
  );

  // Get entries for expanded miner
  const minerEntries = useMemo(() => {
    if (expandedMiner === null) return [];
    return filteredEntries.filter(e => e.character_id === expandedMiner).sort((a, b) => b.date.localeCompare(a.date));
  }, [filteredEntries, expandedMiner]);

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 sm:grid-cols-3 gap-4">
        <KpiCard label={t("corpActiveMiners")} value={String(mining.active_miners)} color="text-eve-accent" />
        <KpiCard label={t("corpTotalVolume")} value={`${mining.total_volume_30d.toLocaleString()} units`} />
        <KpiCard label="Est. ISK" value={`${formatIsk(mining.estimated_isk)} ISK`} color="text-eve-profit" />
      </div>

      {/* Controls */}
      <div className="flex flex-wrap items-center gap-3">
        <DateRangeSelector value={days} onChange={setDays} t={t} />
        <input type="text" value={search} onChange={e => setSearch(e.target.value)} placeholder={t("corpSearch")} className="px-2 py-1 text-xs bg-eve-dark border border-eve-border rounded-sm text-eve-text placeholder:text-eve-dim/50 w-48 focus:border-eve-accent outline-none" />
        <CsvExportButton data={filteredEntries} filename="corp_mining" headers={["character_name","date","type_name","quantity"]} t={t} />
      </div>

      {/* Trend chart */}
      {dailyTrend.length > 0 && (
        <BarChart
          data={dailyTrend.map(d => ({ date: d.date, value: d.volume }))}
          label={t("corpMiningTrend")}
          formatValue={(v) => `${v.toLocaleString()} units`}
          color="emerald"
        />
      )}

      {/* Miner aggregation table */}
      <div className="bg-eve-panel border border-eve-border rounded-sm p-4">
        <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-3">{t("corpMiner")}s ({minerAgg.length})</div>
        {loading ? (
          <div className="flex items-center justify-center py-8 text-eve-dim text-xs">
            <span className="inline-block w-4 h-4 border-2 border-eve-accent/40 border-t-eve-accent rounded-full animate-spin mr-2" />
          </div>
        ) : (
          <div className="border border-eve-border rounded-sm overflow-hidden max-h-[500px] overflow-y-auto">
            <table className="w-full text-xs">
              <thead className="bg-eve-panel sticky top-0 text-eve-dim">
                <tr>
                  <SortHeader label={t("corpMiner")} field="name" />
                  <SortHeader label={t("corpTotalVolumeMined")} field="volume" />
                  <SortHeader label={t("corpTypesMined")} field="types" />
                  <SortHeader label="Est. ISK" field="isk" />
                  <SortHeader label={t("corpLastActive")} field="last" />
                </tr>
              </thead>
              <tbody>
                {minerAgg.map(m => (
                  <tr key={m.id} className={`border-t border-eve-border/30 cursor-pointer transition-colors ${expandedMiner === m.id ? "bg-eve-accent/5" : "hover:bg-eve-panel/50"}`} onClick={() => setExpandedMiner(expandedMiner === m.id ? null : m.id)}>
                    <td className="px-2 py-1.5">
                      <div className="flex items-center gap-2">
                        <img src={`https://images.evetech.net/characters/${m.id}/portrait?size=32`} alt="" className="w-5 h-5 rounded-sm" />
                        <span className="text-eve-text font-medium">{m.name}</span>
                      </div>
                    </td>
                    <td className="px-2 py-1.5 text-eve-accent font-mono text-right">{m.volume.toLocaleString()}</td>
                    <td className="px-2 py-1.5 text-eve-dim text-right">{m.types.size}</td>
                    <td className="px-2 py-1.5 text-eve-profit font-mono text-right">{m.isk > 0 ? formatIsk(m.isk) : "—"}</td>
                    <td className="px-2 py-1.5 text-eve-dim">{m.lastDate}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Expanded miner details */}
      {expandedMiner !== null && minerEntries.length > 0 && (
        <div className="bg-eve-dark/60 border border-eve-border rounded-sm p-4">
          <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-3">{t("corpMiningDetails")}: {minerAgg.find(m => m.id === expandedMiner)?.name}</div>
          <div className="border border-eve-border rounded-sm overflow-hidden max-h-60 overflow-y-auto">
            <table className="w-full text-xs">
              <thead className="bg-eve-panel sticky top-0 text-eve-dim">
                <tr>
                  <th className="px-2 py-1.5 text-left">Date</th>
                  <th className="px-2 py-1.5 text-left">{t("corpOreType")}</th>
                  <th className="px-2 py-1.5 text-right">{t("corpQuantity")}</th>
                  <th className="px-2 py-1.5 text-right">Est. ISK</th>
                </tr>
              </thead>
              <tbody>
                {minerEntries.slice(0, 100).map((e, i) => {
                  const unitIsk = oreIskPerUnit.get(e.type_id) || 0;
                  return (
                    <tr key={i} className="border-t border-eve-border/30">
                      <td className="px-2 py-1 text-eve-dim">{e.date}</td>
                      <td className="px-2 py-1 text-eve-text">
                        <div className="flex items-center gap-1.5">
                          <img src={`https://images.evetech.net/types/${e.type_id}/icon?size=32`} alt="" className="w-4 h-4" />
                          {e.type_name}
                        </div>
                      </td>
                      <td className="px-2 py-1 text-eve-accent text-right font-mono">{e.quantity.toLocaleString()}</td>
                      <td className="px-2 py-1 text-eve-profit text-right font-mono">{unitIsk > 0 ? formatIsk(e.quantity * unitIsk) : "—"}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}

// ============================================================
// Market Section
// ============================================================

function MarketSection({
  dashboard,
  mode,
  formatIsk,
  t,
}: {
  dashboard: CorpDashboard;
  mode: "demo" | "live";
  formatIsk: (v: number) => string;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}) {
  const mkt = dashboard.market_summary;
  const [orders, setOrders] = useState<CorpMarketOrderDetail[]>([]);
  const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState("");
  const [sortKey, setSortKey] = useState<"type" | "character" | "price" | "volume" | "location" | "issued">("price");
  const [sortAsc, setSortAsc] = useState(false);
  const [filterSide, setFilterSide] = useState<"all" | "buy" | "sell">("all");

  useEffect(() => {
    setLoading(true);
    getCorpOrders(mode).then(setOrders).catch(() => setOrders([])).finally(() => setLoading(false));
  }, [mode]);

  const toggleSort = (key: typeof sortKey) => {
    if (sortKey === key) setSortAsc(!sortAsc);
    else { setSortKey(key); setSortAsc(key === "type" || key === "character"); }
  };

  const filtered = useMemo(() => {
    let list = [...orders];
    if (filterSide === "buy") list = list.filter(o => o.is_buy_order);
    else if (filterSide === "sell") list = list.filter(o => !o.is_buy_order);
    if (search.trim()) {
      const q = search.toLowerCase();
      list = list.filter(o => o.type_name?.toLowerCase().includes(q) || o.character_name?.toLowerCase().includes(q) || o.location_name?.toLowerCase().includes(q));
    }
    list.sort((a, b) => {
      let cmp = 0;
      switch (sortKey) {
        case "type": cmp = (a.type_name || "").localeCompare(b.type_name || ""); break;
        case "character": cmp = (a.character_name || "").localeCompare(b.character_name || ""); break;
        case "price": cmp = a.price - b.price; break;
        case "volume": cmp = a.volume_remain - b.volume_remain; break;
        case "location": cmp = (a.location_name || "").localeCompare(b.location_name || ""); break;
        case "issued": cmp = (a.issued || "").localeCompare(b.issued || ""); break;
      }
      return sortAsc ? cmp : -cmp;
    });
    return list;
  }, [orders, filterSide, search, sortKey, sortAsc]);

  // Top traders
  const topTraders = useMemo(() => {
    const map = new Map<string, { name: string; orders: number; value: number }>();
    orders.forEach(o => {
      const name = o.character_name || `ID ${o.character_id}`;
      const existing = map.get(name);
      const val = o.price * o.volume_remain;
      if (existing) { existing.orders++; existing.value += val; }
      else map.set(name, { name, orders: 1, value: val });
    });
    return Array.from(map.values()).sort((a, b) => b.value - a.value).slice(0, 5);
  }, [orders]);

  const SortHeader = ({ label, field }: { label: string; field: typeof sortKey }) => (
    <th className="px-2 py-1.5 text-left cursor-pointer hover:text-eve-accent transition-colors select-none" onClick={() => toggleSort(field)}>
      {label} {sortKey === field && (sortAsc ? "\u2191" : "\u2193")}
    </th>
  );

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-4">
        <KpiCard label={t("corpBuyOrders")} value={String(mkt.active_buy_orders)} color="text-eve-profit" />
        <KpiCard label={t("corpSellOrders")} value={String(mkt.active_sell_orders)} color="text-eve-error" />
        <KpiCard label={t("corpTotalBuyValue")} value={`${formatIsk(mkt.total_buy_value)} ISK`} />
        <KpiCard label={t("corpTotalSellValue")} value={`${formatIsk(mkt.total_sell_value)} ISK`} />
        <KpiCard label={t("corpUniqueTraders")} value={String(mkt.unique_traders)} color="text-eve-accent" />
      </div>

      {/* Controls */}
      <div className="flex flex-wrap items-center gap-3">
        <div className="flex rounded-sm overflow-hidden border border-eve-border text-xs">
          {(["all", "buy", "sell"] as const).map(s => (
            <button key={s} onClick={() => setFilterSide(s)} className={`px-3 py-1 capitalize ${filterSide === s ? "bg-eve-accent/20 text-eve-accent" : "text-eve-dim hover:text-eve-text"}`}>
              {s === "all" ? t("corpPeriodAll") : s === "buy" ? t("corpBuy") : t("corpSell")}
            </button>
          ))}
        </div>
        <input type="text" value={search} onChange={e => setSearch(e.target.value)} placeholder={t("corpSearch")} className="px-2 py-1 text-xs bg-eve-dark border border-eve-border rounded-sm text-eve-text placeholder:text-eve-dim/50 w-48 focus:border-eve-accent outline-none" />
        <CsvExportButton data={filtered} filename="corp_orders" headers={["type_name","character_name","is_buy_order","price","volume_remain","volume_total","location_name","issued"]} t={t} />
      </div>

      {/* Top Traders */}
      {topTraders.length > 0 && (
        <div className="bg-eve-panel border border-eve-border rounded-sm p-4">
          <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-3">{t("corpTopTraders")}</div>
          <div className="grid grid-cols-1 sm:grid-cols-5 gap-3">
            {topTraders.map((tr, i) => (
              <div key={tr.name} className="bg-eve-dark/50 border border-eve-border/50 rounded-sm p-3 text-center">
                <div className="text-[10px] text-eve-dim">#{i + 1}</div>
                <div className="text-xs text-eve-text font-medium truncate">{tr.name}</div>
                <div className="text-xs text-eve-accent font-bold">{formatIsk(tr.value)} ISK</div>
                <div className="text-[10px] text-eve-dim">{tr.orders} {t("corpOrders").toLowerCase()}</div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Orders table */}
      <div className="bg-eve-panel border border-eve-border rounded-sm p-4">
        <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-3">{t("corpOrders")} ({filtered.length})</div>
        {loading ? (
          <div className="flex items-center justify-center py-8 text-eve-dim text-xs">
            <span className="inline-block w-4 h-4 border-2 border-eve-accent/40 border-t-eve-accent rounded-full animate-spin mr-2" />
          </div>
        ) : (
          <div className="border border-eve-border rounded-sm overflow-hidden max-h-[500px] overflow-y-auto">
            <table className="w-full text-xs">
              <thead className="bg-eve-panel sticky top-0 text-eve-dim">
                <tr>
                  <SortHeader label={t("corpOrderType")} field="type" />
                  <SortHeader label={t("corpCharacter")} field="character" />
                  <th className="px-2 py-1.5 text-left">{t("corpBuy")}/{t("corpSell")}</th>
                  <SortHeader label={t("corpPrice")} field="price" />
                  <SortHeader label={t("corpVolume")} field="volume" />
                  <SortHeader label={t("corpLocation")} field="location" />
                  <SortHeader label={t("corpIssued")} field="issued" />
                </tr>
              </thead>
              <tbody>
                {filtered.map(o => (
                  <tr key={o.order_id} className="border-t border-eve-border/30 hover:bg-eve-panel/50">
                    <td className="px-2 py-1.5">
                      <div className="flex items-center gap-2">
                        <img src={`https://images.evetech.net/types/${o.type_id}/icon?size=32`} alt="" className="w-4 h-4" />
                        <span className="text-eve-text">{o.type_name}</span>
                      </div>
                    </td>
                    <td className="px-2 py-1.5 text-eve-dim max-w-[100px] truncate">{o.character_name}</td>
                    <td className="px-2 py-1.5">
                      <span className={`px-1.5 py-0.5 text-[10px] font-bold uppercase rounded-sm border ${o.is_buy_order ? "bg-emerald-500/20 text-emerald-400 border-emerald-500/30" : "bg-amber-500/20 text-amber-400 border-amber-500/30"}`}>
                        {o.is_buy_order ? t("corpBuy") : t("corpSell")}
                      </span>
                    </td>
                    <td className="px-2 py-1.5 text-eve-accent text-right font-mono">{formatIsk(o.price)}</td>
                    <td className="px-2 py-1.5 text-right text-eve-dim">{o.volume_remain.toLocaleString()}/{o.volume_total.toLocaleString()}</td>
                    <td className="px-2 py-1.5 text-eve-dim max-w-[140px] truncate" title={o.location_name}>{o.location_name}</td>
                    <td className="px-2 py-1.5 text-eve-dim whitespace-nowrap">{o.issued?.slice(0, 10)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}

// ============================================================
// Shared: Interactive BarChart
// ============================================================

const BAR_COLORS: Record<string, { normal: string; hover: string }> = {
  blue:    { normal: "rgba(59,130,246,0.5)",  hover: "rgba(59,130,246,0.85)" },
  emerald: { normal: "rgba(16,185,129,0.5)",  hover: "rgba(16,185,129,0.85)" },
  amber:   { normal: "rgba(245,158,11,0.5)",  hover: "rgba(245,158,11,0.85)" },
  red:     { normal: "rgba(239,68,68,0.5)",   hover: "rgba(239,68,68,0.85)" },
};

function BarChart({ data, label, formatValue, color = "blue" }: {
  data: { date: string; value: number }[];
  label: string;
  formatValue: (v: number) => string;
  color?: string;
}) {
  const [hoveredIdx, setHoveredIdx] = useState<number | null>(null);

  // Show last 60 entries
  const visible = data.slice(-60);
  const maxVal = Math.max(...visible.map(d => d.value), 1);

  // Compute totals for the summary line
  const total = visible.reduce((s, d) => s + d.value, 0);
  const avg = visible.length > 0 ? total / visible.length : 0;

  const palette = BAR_COLORS[color] || BAR_COLORS.blue;

  return (
    <div className="bg-eve-panel border border-eve-border rounded-sm p-4">
      <div className="flex items-center justify-between mb-3">
        <div className="text-[10px] text-eve-dim uppercase tracking-wider">{label}</div>
        <div className="flex items-center gap-4 text-[10px] text-eve-dim">
          <span>Total: <span className="text-eve-text font-medium">{formatValue(total)}</span></span>
          <span>Avg: <span className="text-eve-text font-medium">{formatValue(Math.round(avg))}</span></span>
        </div>
      </div>

      {/* Tooltip */}
      <div className="h-5 mb-1">
        {hoveredIdx !== null && visible[hoveredIdx] && (
          <div className="flex items-center gap-3 text-xs">
            <span className="text-eve-dim">{visible[hoveredIdx].date}</span>
            <span className="text-eve-text font-bold">{formatValue(visible[hoveredIdx].value)}</span>
          </div>
        )}
      </div>

      {/* Chart area */}
      <div className="relative">
        {/* Y-axis grid lines */}
        <div className="absolute inset-0 flex flex-col justify-between pointer-events-none">
          {[1, 0.75, 0.5, 0.25, 0].map(pct => (
            <div key={pct} className="border-t border-eve-border/20 w-full" />
          ))}
        </div>

        {/* Bars */}
        <div
          className="flex items-end gap-px h-28 relative"
          onMouseLeave={() => setHoveredIdx(null)}
        >
          {visible.map((d, i) => {
            const pct = (d.value / maxVal) * 100;
            const isHovered = hoveredIdx === i;
            return (
              <div
                key={d.date}
                className="flex-1 min-w-[3px] relative cursor-crosshair"
                style={{ height: "100%" }}
                onMouseEnter={() => setHoveredIdx(i)}
              >
                {/* Hover column highlight */}
                {isHovered && (
                  <div className="absolute inset-0 bg-eve-accent/5 border-x border-eve-accent/20" />
                )}
                {/* Bar */}
                <div
                  className="absolute bottom-0 left-0 right-0 rounded-t-sm transition-all duration-75"
                  style={{
                    height: `${Math.max(pct, 1)}%`,
                    backgroundColor: isHovered ? palette.hover : palette.normal,
                    boxShadow: isHovered ? "0 0 6px rgba(var(--eve-accent-rgb, 200,170,110), 0.15)" : "none",
                  }}
                />
              </div>
            );
          })}
        </div>
      </div>

      {/* X-axis labels */}
      <div className="flex justify-between mt-1.5 text-[9px] text-eve-dim/50">
        <span>{visible[0]?.date.slice(5)}</span>
        {visible.length > 20 && <span>{visible[Math.floor(visible.length / 2)]?.date.slice(5)}</span>}
        <span>{visible[visible.length - 1]?.date.slice(5)}</span>
      </div>
    </div>
  );
}

// ============================================================
// Shared: DateRangeSelector + CsvExportButton
// ============================================================

function DateRangeSelector({ value, onChange, t }: { value: number; onChange: (v: number) => void; t: (key: TranslationKey) => string }) {
  const options = [
    { label: t("corpPeriod7d"), days: 7 },
    { label: t("corpPeriod30d"), days: 30 },
    { label: t("corpPeriod90d"), days: 90 },
    { label: t("corpPeriodAll"), days: 0 },
  ];
  return (
    <div className="flex rounded-sm overflow-hidden border border-eve-border text-xs">
      {options.map(o => (
        <button key={o.days} onClick={() => onChange(o.days)} className={`px-3 py-1 ${value === o.days ? "bg-eve-accent/20 text-eve-accent" : "text-eve-dim hover:text-eve-text"}`}>
          {o.label}
        </button>
      ))}
    </div>
  );
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function CsvExportButton({ data, filename, headers, t }: { data: any[]; filename: string; headers: string[]; t: (key: TranslationKey) => string }) {
  const handleExport = useCallback(() => {
    if (!data.length) return;
    const csvHeaders = headers.join(",");
    const csvRows = data.map((row: Record<string, unknown>) => headers.map(h => {
      const val = row[h];
      const str = String(val ?? "");
      return str.includes(",") || str.includes('"') ? `"${str.replace(/"/g, '""')}"` : str;
    }).join(","));
    const csv = [csvHeaders, ...csvRows].join("\n");
    const blob = new Blob([csv], { type: "text/csv;charset=utf-8;" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${filename}_${new Date().toISOString().slice(0, 10)}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  }, [data, filename, headers]);

  return (
    <button onClick={handleExport} className="px-3 py-1 text-xs text-eve-dim border border-eve-border rounded-sm hover:text-eve-accent hover:border-eve-accent/50 transition-colors ml-auto">
      {t("corpExportCsv")}
    </button>
  );
}

// ============================================================
// Shared Components
// ============================================================

function KpiCard({
  label,
  value,
  color = "text-eve-text",
  large = false,
}: {
  label: string;
  value: string;
  color?: string;
  large?: boolean;
}) {
  return (
    <div className="bg-eve-panel border border-eve-border rounded-sm p-4">
      <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-1">{label}</div>
      <div className={`${large ? "text-xl" : "text-lg"} font-bold ${color}`}>{value}</div>
    </div>
  );
}

function MiniKpi({
  label,
  value,
  color = "text-eve-text",
}: {
  label: string;
  value: number;
  color?: string;
}) {
  return (
    <div className="text-center">
      <div className={`text-lg font-bold ${color}`}>{value}</div>
      <div className="text-[10px] text-eve-dim">{label}</div>
    </div>
  );
}

function IncomeSourceChart({
  sources,
  formatIsk,
}: {
  sources: IncomeSource[];
  formatIsk: (v: number) => string;
}) {
  if (!sources || sources.length === 0) {
    return <div className="text-eve-dim text-xs text-center py-4">No data</div>;
  }

  const colors = [
    "bg-emerald-500", "bg-blue-500", "bg-amber-500", "bg-purple-500",
    "bg-red-500", "bg-cyan-500", "bg-pink-500", "bg-teal-500",
  ];

  return (
    <div className="space-y-2">
      {/* Stacked bar */}
      <div className="flex h-6 rounded-sm overflow-hidden">
        {sources.map((s, i) => (
          <div
            key={s.category}
            className={`${colors[i % colors.length]} flex items-center justify-center text-[9px] font-bold text-black/70`}
            style={{ width: `${s.percent}%` }}
            title={`${s.label}: ${formatIsk(s.amount)} ISK (${s.percent.toFixed(1)}%)`}
          >
            {s.percent > 10 ? `${s.percent.toFixed(0)}%` : ""}
          </div>
        ))}
      </div>
      {/* Legend */}
      <div className="flex flex-wrap gap-x-4 gap-y-1">
        {sources.map((s, i) => (
          <div key={s.category} className="flex items-center gap-1.5 text-xs">
            <div className={`w-2.5 h-2.5 rounded-sm ${colors[i % colors.length]}`} />
            <span className="text-eve-dim">{s.label}</span>
            <span className="text-eve-text font-medium">{formatIsk(s.amount)}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function DailyPnLChart({
  entries,
  formatIsk,
}: {
  entries: DailyPnLEntry[];
  formatIsk: (v: number) => string;
}) {
  if (!entries || entries.length === 0) {
    return <div className="text-eve-dim text-xs text-center py-4">No data</div>;
  }

  const values = entries.map((e) => e.net_income);
  const maxAbs = Math.max(...values.map(Math.abs), 1);
  const chartH = 120;

  // Limit bars
  const maxBars = 45;
  const step = entries.length > maxBars ? Math.ceil(entries.length / maxBars) : 1;
  const sampled = step > 1 ? entries.filter((_, i) => i % step === 0) : entries;
  const barWidth = Math.max(3, Math.min(14, Math.floor(600 / sampled.length) - 1));

  return (
    <div className="relative">
      <div className="relative" style={{ height: chartH }}>
        <div className="flex items-end justify-center gap-px h-full">
          {sampled.map((entry) => {
            const val = entry.net_income;
            const pct = Math.abs(val) / maxAbs;
            const barH = Math.max(1, pct * (chartH / 2 - 4));
            const isPositive = val >= 0;

            return (
              <div
                key={entry.date}
                className="relative group flex flex-col items-center"
                style={{ width: barWidth, height: chartH }}
              >
                <div className="flex-1 flex items-end justify-center">
                  {isPositive && (
                    <div
                      className="rounded-t-[1px] bg-emerald-500/80 hover:bg-emerald-400 transition-colors"
                      style={{ width: barWidth, height: barH }}
                    />
                  )}
                </div>
                <div className="flex-1 flex items-start justify-center">
                  {!isPositive && (
                    <div
                      className="rounded-b-[1px] bg-red-500/80 hover:bg-red-400 transition-colors"
                      style={{ width: barWidth, height: barH }}
                    />
                  )}
                </div>
                <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-1 hidden group-hover:block z-10 pointer-events-none">
                  <div className="bg-eve-dark border border-eve-border rounded px-2 py-1 text-[10px] whitespace-nowrap shadow-lg">
                    <div className="text-eve-dim">{entry.date}</div>
                    <div className={isPositive ? "text-emerald-400" : "text-red-400"}>
                      {val >= 0 ? "+" : ""}{formatIsk(val)} ISK
                    </div>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
        <div className="absolute left-0 right-0 border-t border-eve-border/50" style={{ top: chartH / 2 }} />
      </div>
      <div className="flex justify-between mt-1 px-1">
        <span className="text-[9px] text-eve-dim">{sampled[0]?.date.slice(5)}</span>
        <span className="text-[9px] text-eve-dim">{sampled[sampled.length - 1]?.date.slice(5)}</span>
      </div>
    </div>
  );
}

function TopContributorsTable({
  contributors,
  formatIsk,
}: {
  contributors: MemberContribution[];
  formatIsk: (v: number) => string;
}) {
  if (!contributors || contributors.length === 0) {
    return <div className="text-eve-dim text-xs text-center py-4">No data</div>;
  }

  const maxIsk = Math.max(...contributors.map((c) => Math.abs(c.total_isk)), 1);

  return (
    <div className="border border-eve-border rounded-sm overflow-hidden">
      <table className="w-full text-xs">
        <thead className="bg-eve-panel">
          <tr className="text-eve-dim">
            <th className="px-3 py-2 text-left">#</th>
            <th className="px-3 py-2 text-left">Character</th>
            <th className="px-3 py-2 text-left">Role</th>
            <th className="px-3 py-2 text-right">ISK Contributed</th>
          </tr>
        </thead>
        <tbody>
          {contributors.slice(0, 15).map((c, i) => {
            const pct = (Math.abs(c.total_isk) / maxIsk) * 100;
            return (
              <tr key={c.character_id} className="border-t border-eve-border/50 hover:bg-eve-panel/50">
                <td className="px-3 py-2 text-eve-dim">{i + 1}</td>
                <td className="px-3 py-2">
                  <div className="flex items-center gap-2">
                    <img
                      src={`https://images.evetech.net/characters/${c.character_id}/portrait?size=32`}
                      alt=""
                      className="w-5 h-5 rounded-sm"
                    />
                    <span className="text-eve-text font-medium">{c.name}</span>
                    {c.is_online && <span className="w-1.5 h-1.5 rounded-full bg-emerald-400" title="Online" />}
                  </div>
                </td>
                <td className="px-3 py-2 text-eve-dim capitalize">{c.category}</td>
                <td className="px-3 py-2 text-right">
                  <div className="flex items-center justify-end gap-2">
                    <div className="w-20 h-1.5 bg-eve-dark rounded-full overflow-hidden">
                      <div className="h-full bg-eve-accent/60 rounded-full" style={{ width: `${pct}%` }} />
                    </div>
                    <span className="text-eve-accent font-medium">{formatIsk(c.total_isk)}</span>
                  </div>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
