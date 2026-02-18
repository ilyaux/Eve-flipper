import { type TranslationKey } from "../../lib/i18n";
import type { CorpDashboard } from "../../lib/types";
import type { CorpTab } from "./types";
import { DailyPnLChart, IncomeSourceChart, KpiCard, MiniKpi, TopContributorsTable } from "./shared";
export function OverviewSection({
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
