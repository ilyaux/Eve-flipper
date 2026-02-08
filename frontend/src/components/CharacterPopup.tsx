import { useEffect, useState, useCallback, useMemo } from "react";
import { Modal } from "./Modal";
import { getCharacterInfo, getCharacterRoles, getUndercuts, getPortfolioPnL, getPortfolioOptimization, type OptimizerResult } from "../lib/api";
import { useI18n, type TranslationKey } from "../lib/i18n";
import type { CharacterInfo, CharacterOrder, CharacterRoles, HistoricalOrder, PortfolioPnL, ItemPnL, StationPnL, UndercutStatus, WalletTransaction, AssetStats, AllocationSuggestion } from "../lib/types";

interface CharacterPopupProps {
  open: boolean;
  onClose: () => void;
  characterId: number;
  characterName: string;
}

type CharTab = "overview" | "orders" | "history" | "transactions" | "pnl" | "risk" | "optimizer";

export function CharacterPopup({ open, onClose, characterId, characterName }: CharacterPopupProps) {
  const { t } = useI18n();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [data, setData] = useState<CharacterInfo | null>(null);
  const [tab, setTab] = useState<CharTab>("overview");
  const [corpRoles, setCorpRoles] = useState<CharacterRoles | null>(null);
  const [corpRolesLoading, setCorpRolesLoading] = useState(false);

  const loadData = useCallback(() => {
    setLoading(true);
    setError(null);
    getCharacterInfo()
      .then(setData)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    if (!open) return;
    loadData();
    // Also check corp roles
    setCorpRolesLoading(true);
    getCharacterRoles()
      .then(setCorpRoles)
      .catch(() => setCorpRoles(null))
      .finally(() => setCorpRolesLoading(false));
  }, [open, loadData]);

  const formatIsk = (value: number) => {
    if (value >= 1e9) return `${(value / 1e9).toFixed(2)}B`;
    if (value >= 1e6) return `${(value / 1e6).toFixed(2)}M`;
    if (value >= 1e3) return `${(value / 1e3).toFixed(1)}K`;
    return value.toFixed(0);
  };

  const formatNumber = (value: number) => value.toLocaleString();

  const formatDate = (dateStr: string) => {
    const d = new Date(dateStr);
    return d.toLocaleDateString() + " " + d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  };

  const buyOrders = data?.orders.filter((o) => o.is_buy_order) ?? [];
  const sellOrders = data?.orders.filter((o) => !o.is_buy_order) ?? [];
  const totalBuyValue = buyOrders.reduce((sum, o) => sum + o.price * o.volume_remain, 0);
  const totalSellValue = sellOrders.reduce((sum, o) => sum + o.price * o.volume_remain, 0);

  // Calculate profit from recent transactions
  const recentTxns = data?.transactions ?? [];
  const buyTxns = recentTxns.filter((t) => t.is_buy);
  const sellTxns = recentTxns.filter((t) => !t.is_buy);
  const totalBought = buyTxns.reduce((sum, t) => sum + t.unit_price * t.quantity, 0);
  const totalSold = sellTxns.reduce((sum, t) => sum + t.unit_price * t.quantity, 0);

  return (
    <Modal open={open} onClose={onClose} title={characterName} width="max-w-4xl">
      <div className="flex flex-col h-[70vh]">
        {/* Tabs + Refresh */}
        <div className="flex items-center border-b border-eve-border bg-eve-panel">
          <div className="flex flex-1 overflow-x-auto scrollbar-thin">
            <TabBtn active={tab === "overview"} onClick={() => setTab("overview")} label={t("charOverview")} />
            <TabBtn active={tab === "orders"} onClick={() => setTab("orders")} label={`${t("charActiveOrders")} (${data?.orders.length ?? 0})`} />
            <TabBtn active={tab === "history"} onClick={() => setTab("history")} label={`${t("charOrderHistory")} (${data?.order_history?.length ?? 0})`} />
            <TabBtn active={tab === "transactions"} onClick={() => setTab("transactions")} label={`${t("charTransactions")} (${data?.transactions?.length ?? 0})`} />
            <TabBtn active={tab === "pnl"} onClick={() => setTab("pnl")} label={t("charPnlTab")} />
            <TabBtn active={tab === "risk"} onClick={() => setTab("risk")} label={t("charRiskTab")} />
            <TabBtn active={tab === "optimizer"} onClick={() => setTab("optimizer")} label={t("charOptimizerTab")} />
          </div>
          {/* Refresh button */}
          <button
            onClick={loadData}
            disabled={loading}
            className="px-2 py-1.5 mr-2 text-eve-dim hover:text-eve-accent transition-colors disabled:opacity-50"
            title={t("charRefresh")}
          >
            <svg className={`w-4 h-4 ${loading ? "animate-spin" : ""}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
          </button>
        </div>

        {/* Content */}
        <div className="flex-1 overflow-auto p-4">
          {loading && !data && (
            <div className="flex items-center justify-center h-full text-eve-dim">{t("loading")}...</div>
          )}
          {error && !data && (
            <div className="flex items-center justify-center h-full text-eve-error">{error}</div>
          )}
          {data && (
            <>
              {tab === "overview" && (
                <OverviewTab
                  data={data}
                  characterId={characterId}
                  formatIsk={formatIsk}
                  formatNumber={formatNumber}
                  buyOrders={buyOrders}
                  sellOrders={sellOrders}
                  totalBuyValue={totalBuyValue}
                  totalSellValue={totalSellValue}
                  totalBought={totalBought}
                  totalSold={totalSold}
                  corpRoles={corpRoles}
                  corpRolesLoading={corpRolesLoading}
                  t={t}
                />
              )}
              {tab === "orders" && (
                <OrdersTab orders={data.orders} formatIsk={formatIsk} t={t} />
              )}
              {tab === "history" && (
                <HistoryTab history={data.order_history ?? []} formatIsk={formatIsk} formatDate={formatDate} t={t} />
              )}
              {tab === "transactions" && (
                <TransactionsTab transactions={data.transactions ?? []} formatIsk={formatIsk} formatDate={formatDate} t={t} />
              )}
              {tab === "pnl" && (
                <PnLTab formatIsk={formatIsk} t={t} />
              )}
              {tab === "risk" && (
                <RiskTab
                  characterId={characterId}
                  data={data}
                  formatIsk={formatIsk}
                  t={t}
                />
              )}
              {tab === "optimizer" && (
                <OptimizerTab formatIsk={formatIsk} t={t} />
              )}
            </>
          )}
        </div>
      </div>
    </Modal>
  );
}

function TabBtn({ active, onClick, label }: { active: boolean; onClick: () => void; label: string }) {
  return (
    <button
      onClick={onClick}
      className={`px-4 py-2 text-xs font-medium transition-colors whitespace-nowrap ${
        active
          ? "text-eve-accent border-b-2 border-eve-accent bg-eve-dark/50"
          : "text-eve-dim hover:text-eve-text"
      }`}
    >
      {label}
    </button>
  );
}

interface OverviewTabProps {
  data: CharacterInfo;
  characterId: number;
  formatIsk: (v: number) => string;
  formatNumber: (v: number) => string;
  buyOrders: CharacterOrder[];
  sellOrders: CharacterOrder[];
  totalBuyValue: number;
  totalSellValue: number;
  totalBought: number;
  totalSold: number;
  corpRoles: CharacterRoles | null;
  corpRolesLoading: boolean;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}

function OverviewTab({
  data,
  characterId,
  formatIsk,
  formatNumber,
  buyOrders,
  sellOrders,
  totalBuyValue,
  totalSellValue,
  totalBought,
  totalSold,
  corpRoles,
  corpRolesLoading,
  t,
}: OverviewTabProps) {
  // Net worth = wallet + sell orders value.
  // Wallet balance already accounts for ISK locked in buy order escrow,
  // so adding buy value again would double-count.
  const netWorth = data.wallet + totalSellValue;
  const tradingProfit = totalSold - totalBought;

  return (
    <div className="space-y-4">
      {/* Character Header */}
      <div className="flex items-center gap-4 p-4 bg-eve-panel border border-eve-border rounded-sm">
        <img
          src={`https://images.evetech.net/characters/${characterId}/portrait?size=128`}
          alt=""
          className="w-16 h-16 rounded-sm"
        />
        <div>
          <h2 className="text-lg font-bold text-eve-text">{data.character_name}</h2>
          {data.skills && (
            <div className="text-sm text-eve-dim">{formatNumber(data.skills.total_sp)} SP</div>
          )}
        </div>
      </div>

      {/* Financial Summary */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <StatCard label={t("charWallet")} value={`${formatIsk(data.wallet)} ISK`} color="text-eve-profit" />
        <StatCard label={t("charEscrow")} value={`${formatIsk(totalBuyValue)} ISK`} color="text-eve-warning" />
        <StatCard label={t("charSellOrdersValue")} value={`${formatIsk(totalSellValue)} ISK`} color="text-eve-accent" />
        <StatCard label={t("charNetWorth")} value={`${formatIsk(netWorth)} ISK`} color="text-eve-profit" large />
      </div>

      {/* Orders Summary */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <StatCard label={t("charBuyOrders")} value={String(buyOrders.length)} subvalue={`${formatIsk(totalBuyValue)} ISK`} />
        <StatCard label={t("charSellOrders")} value={String(sellOrders.length)} subvalue={`${formatIsk(totalSellValue)} ISK`} />
        <StatCard label={t("charTotalOrders")} value={String(data.orders.length)} subvalue={`${formatIsk(totalBuyValue + totalSellValue)} ISK`} />
        <StatCard
          label={t("charTradingProfit")}
          value={`${tradingProfit >= 0 ? "+" : ""}${formatIsk(tradingProfit)} ISK`}
          color={tradingProfit >= 0 ? "text-eve-profit" : "text-eve-error"}
        />
      </div>

      {/* Recent Activity */}
      <div className="grid grid-cols-2 gap-3">
        <StatCard label={t("charRecentBuys")} value={`${formatIsk(totalBought)} ISK`} subvalue={`${data.transactions?.filter((t) => t.is_buy).length ?? 0} ${t("charTxns")}`} />
        <StatCard label={t("charRecentSales")} value={`${formatIsk(totalSold)} ISK`} subvalue={`${data.transactions?.filter((t) => !t.is_buy).length ?? 0} ${t("charTxns")}`} />
      </div>

      {/* Corp Dashboard Section */}
      <div className="bg-eve-panel border border-eve-border rounded-sm p-4">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <svg className="w-5 h-5 text-eve-accent" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" />
            </svg>
            <div>
              <div className="text-sm font-medium text-eve-text">{t("corpDashboard")}</div>
              {corpRolesLoading ? (
                <div className="flex items-center gap-1.5 text-xs text-eve-dim">
                  <span className="inline-block w-3 h-3 border-2 border-eve-accent/40 border-t-eve-accent rounded-full animate-spin" />
                  {t("corpRolesChecking")}
                </div>
              ) : corpRoles?.is_director ? (
                <div className="flex items-center gap-1.5 text-xs">
                  <span className="inline-block w-1.5 h-1.5 rounded-full bg-emerald-400" />
                  <span className="text-emerald-400 font-medium">{t("corpDirector")}</span>
                </div>
              ) : (
                <div className="flex items-center gap-1.5 text-xs">
                  <span className="inline-block w-1.5 h-1.5 rounded-full bg-eve-dim" />
                  <span className="text-eve-dim">{t("corpNotDirector")}</span>
                </div>
              )}
            </div>
          </div>
          <div className="flex gap-2">
            {/* Demo button — dev mode only */}
            {import.meta.env.DEV && (
              <button
                onClick={() => window.open("/corp/?mode=demo", "_blank")}
                className="px-3 py-1.5 text-xs font-medium rounded-sm border border-eve-border bg-eve-dark text-eve-dim hover:text-eve-text hover:border-eve-accent/50 transition-colors"
              >
                {t("corpDashboardDemo")}
              </button>
            )}
            {/* Live button — only for directors */}
            {!corpRolesLoading && corpRoles?.is_director && (
              <button
                onClick={() => window.open("/corp/?mode=live", "_blank")}
                className="px-3 py-1.5 text-xs font-medium rounded-sm border border-eve-accent bg-eve-accent/10 text-eve-accent hover:bg-eve-accent/20 transition-colors"
              >
                {t("corpDashboardLive")}
              </button>
            )}
          </div>
        </div>
        {!corpRolesLoading && !corpRoles?.is_director && (
          <div className="mt-2 text-[10px] text-eve-dim">{t("corpDemoOnly")}</div>
        )}
      </div>
    </div>
  );
}

// --- P&L Tab ---

type PnLPeriod = 7 | 30 | 90 | 180;

interface PnLTabProps {
  formatIsk: (v: number) => string;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}

function PnLTab({ formatIsk, t }: PnLTabProps) {
  const [period, setPeriod] = useState<PnLPeriod>(30);
  const [data, setData] = useState<PortfolioPnL | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [chartMode, setChartMode] = useState<"daily" | "cumulative" | "drawdown">("daily");
  const [itemView, setItemView] = useState<"profit" | "loss">("profit");
  const [bottomView, setBottomView] = useState<"items" | "stations">("items");

  useEffect(() => {
    setLoading(true);
    setError(null);
    getPortfolioPnL(period)
      .then(setData)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, [period]);

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full text-eve-dim text-xs">
        <span className="inline-block w-4 h-4 border-2 border-eve-accent/40 border-t-eve-accent rounded-full animate-spin mr-2" />
        {t("loading")}...
      </div>
    );
  }

  if (error) {
    return <div className="flex items-center justify-center h-full text-eve-error text-xs">{error}</div>;
  }

  if (!data || data.daily_pnl.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center h-full text-eve-dim text-xs space-y-2">
        <div>{t("pnlNoData")}</div>
        <div className="text-[10px] max-w-md text-center">{t("pnlNoDataHint")}</div>
      </div>
    );
  }

  const { summary } = data;

  // Separate top items into profit and loss
  const profitItems = data.top_items.filter((item) => item.net_pnl > 0).sort((a, b) => b.net_pnl - a.net_pnl);
  const lossItems = data.top_items.filter((item) => item.net_pnl < 0).sort((a, b) => a.net_pnl - b.net_pnl);

  return (
    <div className="space-y-4">
      {/* Period selector */}
      <div className="flex items-center justify-between">
        <div className="text-xs text-eve-dim uppercase tracking-wider">{t("pnlTitle")}</div>
        <div className="flex gap-1">
          {([7, 30, 90, 180] as PnLPeriod[]).map((p) => (
            <button
              key={p}
              onClick={() => setPeriod(p)}
              className={`px-2.5 py-1 text-[10px] rounded-sm border transition-colors ${
                period === p
                  ? "bg-eve-accent/20 border-eve-accent text-eve-accent"
                  : "bg-eve-panel border-eve-border text-eve-dim hover:text-eve-text hover:border-eve-accent/50"
              }`}
            >
              {t(`pnlPeriod${p}d` as TranslationKey)}
            </button>
          ))}
        </div>
      </div>

      {/* Summary cards row 1: P&L, ROI, Win Rate */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <StatCard
          label={t("pnlTotalPnl")}
          value={`${summary.total_pnl >= 0 ? "+" : ""}${formatIsk(summary.total_pnl)} ISK`}
          color={summary.total_pnl >= 0 ? "text-eve-profit" : "text-eve-error"}
          large
        />
        <StatCard
          label={t("pnlROI")}
          value={`${summary.roi_percent >= 0 ? "+" : ""}${summary.roi_percent.toFixed(1)}%`}
          color={summary.roi_percent >= 0 ? "text-eve-profit" : "text-eve-error"}
        />
        <StatCard
          label={t("pnlWinRate")}
          value={`${summary.win_rate.toFixed(0)}%`}
          subvalue={`${summary.profitable_days}/${summary.total_days} ${t("pnlProfitableDays").toLowerCase()}`}
          color="text-eve-accent"
        />
        <StatCard
          label={t("pnlAvgDaily")}
          value={`${summary.avg_daily_pnl >= 0 ? "+" : ""}${formatIsk(summary.avg_daily_pnl)} ISK`}
          color={summary.avg_daily_pnl >= 0 ? "text-eve-profit" : "text-eve-error"}
        />
      </div>

      {/* Summary cards row 2: Best day, Worst day, Volume */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <StatCard
          label={t("pnlBestDay")}
          value={`+${formatIsk(summary.best_day_pnl)} ISK`}
          subvalue={summary.best_day_date}
          color="text-eve-profit"
        />
        <StatCard
          label={t("pnlWorstDay")}
          value={`${formatIsk(summary.worst_day_pnl)} ISK`}
          subvalue={summary.worst_day_date}
          color="text-eve-error"
        />
        <StatCard
          label={t("pnlTotalBought")}
          value={`${formatIsk(summary.total_bought)} ISK`}
        />
        <StatCard
          label={t("pnlTotalSold")}
          value={`${formatIsk(summary.total_sold)} ISK`}
        />
      </div>

      {/* Summary cards row 3: Sharpe, Max DD, Profit Factor, Expectancy */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <StatCard
          label={t("pnlSharpeRatio")}
          value={(summary.sharpe_ratio ?? 0) !== 0 ? (summary.sharpe_ratio ?? 0).toFixed(2) : "—"}
          subvalue={t("pnlSharpeHint")}
          color={(summary.sharpe_ratio ?? 0) > 1 ? "text-eve-profit" : (summary.sharpe_ratio ?? 0) > 0 ? "text-eve-accent" : "text-eve-error"}
        />
        <StatCard
          label={t("pnlMaxDrawdown")}
          value={(summary.max_drawdown_isk ?? 0) > 0 ? `-${formatIsk(summary.max_drawdown_isk ?? 0)} ISK` : "—"}
          subvalue={(summary.max_drawdown_pct ?? 0) > 0 ? `-${(summary.max_drawdown_pct ?? 0).toFixed(1)}% (${summary.max_drawdown_days ?? 0}d)` : undefined}
          color="text-eve-error"
        />
        <StatCard
          label={t("pnlProfitFactor")}
          value={(summary.profit_factor ?? 0) > 0 ? (summary.profit_factor ?? 0).toFixed(2) : "—"}
          subvalue={t("pnlProfitFactorHint")}
          color={(summary.profit_factor ?? 0) >= 1.5 ? "text-eve-profit" : (summary.profit_factor ?? 0) >= 1 ? "text-eve-accent" : "text-eve-error"}
        />
        <StatCard
          label={t("pnlExpectancy")}
          value={`${(summary.expectancy_per_trade ?? 0) >= 0 ? "+" : ""}${formatIsk(summary.expectancy_per_trade ?? 0)} ISK`}
          subvalue={t("pnlExpectancyHint")}
          color={(summary.expectancy_per_trade ?? 0) >= 0 ? "text-eve-profit" : "text-eve-error"}
        />
      </div>

      {/* Daily P&L Chart */}
      <div className="bg-eve-panel border border-eve-border rounded-sm p-3">
        <div className="flex items-center justify-between mb-3">
          <div className="text-[10px] text-eve-dim uppercase tracking-wider">
            {chartMode === "daily" ? t("pnlDailyChart") : chartMode === "cumulative" ? t("pnlCumulativeChart") : t("pnlDrawdownChart")}
          </div>
          <div className="flex gap-1">
            <button
              onClick={() => setChartMode("daily")}
              className={`px-2 py-0.5 text-[10px] rounded-sm border transition-colors ${
                chartMode === "daily"
                  ? "bg-eve-accent/20 border-eve-accent text-eve-accent"
                  : "bg-eve-dark border-eve-border text-eve-dim hover:text-eve-text"
              }`}
            >
              {t("pnlDailyChart")}
            </button>
            <button
              onClick={() => setChartMode("cumulative")}
              className={`px-2 py-0.5 text-[10px] rounded-sm border transition-colors ${
                chartMode === "cumulative"
                  ? "bg-eve-accent/20 border-eve-accent text-eve-accent"
                  : "bg-eve-dark border-eve-border text-eve-dim hover:text-eve-text"
              }`}
            >
              {t("pnlCumulativeChart")}
            </button>
            <button
              onClick={() => setChartMode("drawdown")}
              className={`px-2 py-0.5 text-[10px] rounded-sm border transition-colors ${
                chartMode === "drawdown"
                  ? "bg-red-500/20 border-red-500 text-red-400"
                  : "bg-eve-dark border-eve-border text-eve-dim hover:text-eve-text"
              }`}
            >
              {t("pnlDrawdownChart")}
            </button>
          </div>
        </div>
        <PnLChart data={data.daily_pnl} mode={chartMode} formatIsk={formatIsk} />
      </div>

      {/* Top Items / Station Breakdown */}
      <div className="bg-eve-panel border border-eve-border rounded-sm p-3">
        <div className="flex items-center justify-between mb-3">
          <div className="flex gap-2">
            <button
              onClick={() => setBottomView("items")}
              className={`px-2 py-0.5 text-[10px] rounded-sm border transition-colors ${
                bottomView === "items"
                  ? "bg-eve-accent/20 border-eve-accent text-eve-accent"
                  : "bg-eve-dark border-eve-border text-eve-dim hover:text-eve-text"
              }`}
            >
              {t("pnlTopItems")}
            </button>
            <button
              onClick={() => setBottomView("stations")}
              className={`px-2 py-0.5 text-[10px] rounded-sm border transition-colors ${
                bottomView === "stations"
                  ? "bg-eve-accent/20 border-eve-accent text-eve-accent"
                  : "bg-eve-dark border-eve-border text-eve-dim hover:text-eve-text"
              }`}
            >
              {t("pnlStationBreakdown")} ({data.top_stations?.length ?? 0})
            </button>
          </div>
          {bottomView === "items" && (
            <div className="flex gap-1">
              <button
                onClick={() => setItemView("profit")}
                className={`px-2 py-0.5 text-[10px] rounded-sm border transition-colors ${
                  itemView === "profit"
                    ? "bg-emerald-500/20 border-emerald-500 text-emerald-400"
                    : "bg-eve-dark border-eve-border text-eve-dim hover:text-eve-text"
                }`}
              >
                {t("pnlTopProfit")} ({profitItems.length})
              </button>
              <button
                onClick={() => setItemView("loss")}
                className={`px-2 py-0.5 text-[10px] rounded-sm border transition-colors ${
                  itemView === "loss"
                    ? "bg-red-500/20 border-red-500 text-red-400"
                    : "bg-eve-dark border-eve-border text-eve-dim hover:text-eve-text"
                }`}
              >
                {t("pnlTopLoss")} ({lossItems.length})
              </button>
            </div>
          )}
        </div>
        {bottomView === "items" ? (
          <PnLItemsTable
            items={itemView === "profit" ? profitItems : lossItems}
            formatIsk={formatIsk}
            t={t}
          />
        ) : (
          <PnLStationsTable
            stations={data.top_stations ?? []}
            formatIsk={formatIsk}
            t={t}
          />
        )}
      </div>
    </div>
  );
}

// --- P&L Bar Chart (CSS-based) ---

function PnLChart({
  data,
  mode,
  formatIsk,
}: {
  data: PortfolioPnL["daily_pnl"];
  mode: "daily" | "cumulative" | "drawdown";
  formatIsk: (v: number) => string;
}) {
  if (data.length === 0) return null;

  const values = data.map((d) =>
    mode === "daily" ? d.net_pnl : mode === "cumulative" ? d.cumulative_pnl : (d.drawdown_pct ?? 0)
  );
  const maxAbs = Math.max(...values.map(Math.abs), 1);

  // For cumulative mode, compute range from min to max.
  const maxVal = Math.max(...values, 0);
  const minVal = Math.min(...values, 0);
  const range = maxVal - minVal || 1;

  // Show fewer bars if too many days
  const maxBars = 60;
  const step = data.length > maxBars ? Math.ceil(data.length / maxBars) : 1;
  const sampled = step > 1 ? data.filter((_, i) => i % step === 0) : data;
  const sampledValues = sampled.map((d) => (mode === "daily" ? d.net_pnl : d.cumulative_pnl));

  const barWidth = Math.max(2, Math.min(12, Math.floor(680 / sampled.length) - 1));
  const chartHeight = 120;
  const midY = chartHeight / 2;

  // For cumulative mode: compute the zero-line position.
  // The chart spans from minVal at bottom to maxVal at top.
  // Zero line is at (1 - (0 - minVal) / range) * chartHeight from top.
  const cumulativeZeroY = range > 0 ? (1 - (0 - minVal) / range) * chartHeight : chartHeight;

  return (
    <div className="relative">
      {/* Chart area */}
      <div className="relative" style={{ height: chartHeight }}>
        {mode === "drawdown" ? (
          /* Drawdown mode: all bars go downward from top (0%) */
          <div className="flex items-start justify-center gap-px h-full">
            {sampled.map((entry, i) => {
              const val = sampledValues[i]; // always <= 0
              const absMin = Math.max(...values.map((v) => Math.abs(v)), 1);
              const barH = Math.max(1, (Math.abs(val) / absMin) * (chartHeight - 8));
              return (
                <div
                  key={entry.date}
                  className="relative group"
                  style={{ width: barWidth, height: chartHeight }}
                >
                  <div
                    className="bg-red-500/60 hover:bg-red-400/80 transition-colors rounded-b-[1px]"
                    style={{ width: barWidth, height: barH }}
                  />
                  {/* Tooltip */}
                  <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-1 hidden group-hover:block z-10 pointer-events-none">
                    <div className="bg-eve-dark border border-eve-border rounded px-2 py-1 text-[10px] whitespace-nowrap shadow-lg">
                      <div className="text-eve-dim">{entry.date}</div>
                      <div className="text-red-400">{val.toFixed(1)}%</div>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        ) : mode === "daily" ? (
          /* Daily mode: bars grow from the center line */
          <div className="flex items-end justify-center gap-px h-full">
            {sampled.map((entry, i) => {
              const val = sampledValues[i];
              const pct = Math.abs(val) / maxAbs;
              const barH = Math.max(1, pct * (chartHeight / 2 - 4));
              const isPositive = val >= 0;

              return (
                <div
                  key={entry.date}
                  className="relative group flex flex-col items-center"
                  style={{ width: barWidth, height: chartHeight }}
                >
                  {/* Top half */}
                  <div className="flex-1 flex items-end justify-center">
                    {isPositive && (
                      <div
                        className="rounded-t-[1px] bg-emerald-500/80 hover:bg-emerald-400 transition-colors"
                        style={{ width: barWidth, height: barH }}
                      />
                    )}
                  </div>
                  {/* Bottom half */}
                  <div className="flex-1 flex items-start justify-center">
                    {!isPositive && (
                      <div
                        className="rounded-b-[1px] bg-red-500/80 hover:bg-red-400 transition-colors"
                        style={{ width: barWidth, height: barH }}
                      />
                    )}
                  </div>

                  {/* Tooltip */}
                  <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-1 hidden group-hover:block z-10 pointer-events-none">
                    <div className="bg-eve-dark border border-eve-border rounded px-2 py-1 text-[10px] whitespace-nowrap shadow-lg">
                      <div className="text-eve-dim">{entry.date}</div>
                      <div className={isPositive ? "text-emerald-400" : "text-red-400"}>
                        {val >= 0 ? "+" : ""}{formatIsk(val)} ISK
                      </div>
                      <div className="text-eve-dim">{entry.transactions} txns</div>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        ) : (
          /* Cumulative mode: bars grow from the zero line, both up and down */
          <div className="flex items-end justify-center gap-px h-full">
            {sampled.map((entry, i) => {
              const val = sampledValues[i];
              const isPositive = val >= 0;

              // Bar top and height relative to chart:
              // Chart: top=maxVal, bottom=minVal
              // Zero line is at cumulativeZeroY from top.
              // For positive val: bar goes from zeroY up by (val/range)*chartHeight
              // For negative val: bar goes from zeroY down by (|val|/range)*chartHeight
              const barH = Math.max(1, (Math.abs(val) / range) * chartHeight);
              const barTop = isPositive ? cumulativeZeroY - barH : cumulativeZeroY;

              return (
                <div
                  key={entry.date}
                  className="relative group"
                  style={{ width: barWidth, height: chartHeight }}
                >
                  <div
                    className={`absolute transition-colors ${
                      isPositive
                        ? "bg-emerald-500/80 hover:bg-emerald-400 rounded-t-[1px]"
                        : "bg-red-500/80 hover:bg-red-400 rounded-b-[1px]"
                    }`}
                    style={{
                      width: barWidth,
                      height: barH,
                      top: barTop,
                    }}
                  />

                  {/* Tooltip */}
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
        )}

        {/* Zero line */}
        {mode === "daily" ? (
          <div
            className="absolute left-0 right-0 border-t border-eve-border/50"
            style={{ top: midY }}
          />
        ) : (
          <div
            className="absolute left-0 right-0 border-t border-eve-border/50"
            style={{ top: cumulativeZeroY }}
          />
        )}
      </div>

      {/* X-axis labels */}
      <div className="flex justify-between mt-1 px-1">
        <span className="text-[9px] text-eve-dim">{sampled[0]?.date.slice(5)}</span>
        {sampled.length > 2 && (
          <span className="text-[9px] text-eve-dim">{sampled[Math.floor(sampled.length / 2)]?.date.slice(5)}</span>
        )}
        <span className="text-[9px] text-eve-dim">{sampled[sampled.length - 1]?.date.slice(5)}</span>
      </div>

      {/* Y-axis labels */}
      <div className="absolute left-0 top-0 bottom-0 flex flex-col justify-between pointer-events-none" style={{ width: 0 }}>
        <span className="text-[9px] text-eve-dim -translate-x-full pr-1">
          {mode === "drawdown" ? "0%" : `+${formatIsk(mode === "daily" ? maxAbs : maxVal)}`}
        </span>
        <span className="text-[9px] text-eve-dim -translate-x-full pr-1">
          {mode === "drawdown" ? "" : "0"}
        </span>
        <span className="text-[9px] text-eve-dim -translate-x-full pr-1">
          {mode === "drawdown"
            ? `${Math.min(...values).toFixed(1)}%`
            : mode === "daily" ? `-${formatIsk(maxAbs)}` : `${formatIsk(minVal)}`}
        </span>
      </div>
    </div>
  );
}

// --- P&L Items Table ---

function PnLItemsTable({
  items,
  formatIsk,
  t,
}: {
  items: ItemPnL[];
  formatIsk: (v: number) => string;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}) {
  if (items.length === 0) {
    return <div className="text-center text-eve-dim text-xs py-4">{t("pnlNoData")}</div>;
  }

  const maxAbsPnl = Math.max(...items.map((i) => Math.abs(i.net_pnl)), 1);

  return (
    <div className="border border-eve-border rounded-sm overflow-hidden">
      <table className="w-full text-xs">
        <thead className="bg-eve-panel">
          <tr className="text-eve-dim">
            <th className="px-3 py-2 text-left">{t("pnlItemName")}</th>
            <th className="px-3 py-2 text-right">{t("pnlItemPnl")}</th>
            <th className="px-3 py-2 text-right">{t("pnlItemMargin")}</th>
            <th className="px-3 py-2 text-right">{t("pnlItemBought")}</th>
            <th className="px-3 py-2 text-right">{t("pnlItemSold")}</th>
            <th className="px-3 py-2 text-right">{t("pnlItemTxns")}</th>
          </tr>
        </thead>
        <tbody>
          {items.slice(0, 20).map((item) => {
            const isProfit = item.net_pnl >= 0;
            const barPct = (Math.abs(item.net_pnl) / maxAbsPnl) * 100;

            return (
              <tr key={item.type_id} className="border-t border-eve-border/50 hover:bg-eve-panel/50">
                <td className="px-3 py-2 text-eve-text">
                  <div className="flex items-center gap-2">
                    <img
                      src={`https://images.evetech.net/types/${item.type_id}/icon?size=32`}
                      alt=""
                      className="w-5 h-5"
                    />
                    <span className="truncate max-w-[180px]">{item.type_name || `Type #${item.type_id}`}</span>
                  </div>
                </td>
                <td className="px-3 py-2 text-right">
                  <div className="flex items-center justify-end gap-2">
                    <div className="w-16 h-1.5 bg-eve-dark rounded-full overflow-hidden">
                      <div
                        className={`h-full rounded-full ${isProfit ? "bg-emerald-500" : "bg-red-500"}`}
                        style={{ width: `${barPct}%` }}
                      />
                    </div>
                    <span className={isProfit ? "text-eve-profit" : "text-eve-error"}>
                      {isProfit ? "+" : ""}{formatIsk(item.net_pnl)}
                    </span>
                  </div>
                </td>
                <td className="px-3 py-2 text-right text-eve-dim">
                  {item.margin_percent !== 0 ? `${item.margin_percent.toFixed(1)}%` : "—"}
                </td>
                <td className="px-3 py-2 text-right text-eve-dim">
                  {formatIsk(item.total_bought)}
                </td>
                <td className="px-3 py-2 text-right text-eve-dim">
                  {formatIsk(item.total_sold)}
                </td>
                <td className="px-3 py-2 text-right text-eve-dim">
                  {item.transactions}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
      {items.length > 20 && (
        <div className="text-center text-eve-dim text-xs py-2 bg-eve-panel">
          {t("andMore", { count: items.length - 20 })}
        </div>
      )}
    </div>
  );
}

// --- P&L Stations Table ---

function PnLStationsTable({
  stations,
  formatIsk,
  t,
}: {
  stations: StationPnL[];
  formatIsk: (v: number) => string;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}) {
  if (stations.length === 0) {
    return <div className="text-center text-eve-dim text-xs py-4">{t("pnlNoData")}</div>;
  }

  const maxAbsPnl = Math.max(...stations.map((s) => Math.abs(s.net_pnl)), 1);

  return (
    <div className="border border-eve-border rounded-sm overflow-hidden">
      <table className="w-full text-xs">
        <thead className="bg-eve-panel">
          <tr className="text-eve-dim">
            <th className="px-3 py-2 text-left">{t("pnlStationName")}</th>
            <th className="px-3 py-2 text-right">{t("pnlStationPnl")}</th>
            <th className="px-3 py-2 text-right">{t("pnlStationBought")}</th>
            <th className="px-3 py-2 text-right">{t("pnlStationSold")}</th>
            <th className="px-3 py-2 text-right">{t("pnlStationTxns")}</th>
          </tr>
        </thead>
        <tbody>
          {stations.map((st) => {
            const isProfit = st.net_pnl >= 0;
            const barPct = (Math.abs(st.net_pnl) / maxAbsPnl) * 100;

            return (
              <tr key={st.location_id} className="border-t border-eve-border/50 hover:bg-eve-panel/50">
                <td className="px-3 py-2 text-eve-text max-w-[220px] truncate" title={st.location_name}>
                  {st.location_name || `#${st.location_id}`}
                </td>
                <td className="px-3 py-2 text-right">
                  <div className="flex items-center justify-end gap-2">
                    <div className="w-16 h-1.5 bg-eve-dark rounded-full overflow-hidden">
                      <div
                        className={`h-full rounded-full ${isProfit ? "bg-emerald-500" : "bg-red-500"}`}
                        style={{ width: `${barPct}%` }}
                      />
                    </div>
                    <span className={isProfit ? "text-eve-profit" : "text-eve-error"}>
                      {isProfit ? "+" : ""}{formatIsk(st.net_pnl)}
                    </span>
                  </div>
                </td>
                <td className="px-3 py-2 text-right text-eve-dim">{formatIsk(st.total_bought)}</td>
                <td className="px-3 py-2 text-right text-eve-dim">{formatIsk(st.total_sold)}</td>
                <td className="px-3 py-2 text-right text-eve-dim">{st.transactions}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

// --- Optimizer Tab ---

type OptPeriod = 30 | 90 | 180;

interface OptimizerTabProps {
  formatIsk: (v: number) => string;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}

function OptimizerTab({ formatIsk, t }: OptimizerTabProps) {
  const [period, setPeriod] = useState<OptPeriod>(90);
  const [result, setResult] = useState<OptimizerResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [fetchError, setFetchError] = useState<string | null>(null);

  useEffect(() => {
    setLoading(true);
    setFetchError(null);
    getPortfolioOptimization(period)
      .then(setResult)
      .catch((e) => setFetchError(e.message))
      .finally(() => setLoading(false));
  }, [period]);

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full text-eve-dim text-xs">
        <span className="inline-block w-4 h-4 border-2 border-eve-accent/40 border-t-eve-accent rounded-full animate-spin mr-2" />
        {t("optLoading")}
      </div>
    );
  }

  if (fetchError) {
    return (
      <div className="flex flex-col items-center justify-center h-full text-xs space-y-2">
        <div className="text-eve-error">{fetchError}</div>
      </div>
    );
  }

  // Diagnostic view: show details when optimization can't run.
  if (result && !result.ok) {
    const diag = result.diagnostic;
    return (
      <div className="flex flex-col items-center justify-center h-full text-xs space-y-4 px-4">
        <div className="text-eve-dim text-sm">{t("optNoData")}</div>

        {diag ? (
          <div className="bg-eve-panel border border-eve-border rounded-sm p-4 max-w-lg w-full space-y-3">
            <div className="text-[10px] text-eve-accent uppercase tracking-wider mb-2">{t("optDiagTitle")}</div>

            <div className="grid grid-cols-2 gap-x-6 gap-y-1 text-[11px]">
              <span className="text-eve-dim">{t("optDiagTotalTxns")}</span>
              <span className="text-eve-text text-right">{diag.total_transactions}</span>
              <span className="text-eve-dim">{t("optDiagWithinLookback")}</span>
              <span className="text-eve-text text-right">{diag.within_lookback}</span>
              <span className="text-eve-dim">{t("optDiagUniqueDays")}</span>
              <span className={`text-right ${diag.unique_days < diag.min_days_required ? "text-eve-error" : "text-eve-text"}`}>
                {diag.unique_days}
              </span>
              <span className="text-eve-dim">{t("optDiagUniqueItems")}</span>
              <span className="text-eve-text text-right">{diag.unique_items}</span>
              <span className="text-eve-dim">{t("optDiagQualified")}</span>
              <span className={`text-right font-bold ${diag.qualified_items < 2 ? "text-eve-error" : "text-eve-profit"}`}>
                {diag.qualified_items} / {t("optDiagMinRequired", { n: 2 })}
              </span>
              <span className="text-eve-dim">{t("optDiagMinDays")}</span>
              <span className="text-eve-accent text-right">{diag.min_days_required} {t("optDiagDays")}</span>
            </div>

            {diag.top_items && diag.top_items.length > 0 && (
              <div>
                <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-1.5 mt-2">{t("optDiagTopItems")}</div>
                <table className="w-full text-[11px]">
                  <thead>
                    <tr className="text-eve-dim border-b border-eve-border">
                      <th className="text-left py-1 font-normal">{t("optAssetName")}</th>
                      <th className="text-right py-1 font-normal">{t("optDiagDays")}</th>
                      <th className="text-right py-1 font-normal">{t("optDiagTxnCount")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {diag.top_items.map((item) => (
                      <tr key={item.type_id} className="border-b border-eve-border/30">
                        <td className="py-1 text-eve-text">{item.type_name || `#${item.type_id}`}</td>
                        <td className={`py-1 text-right ${item.trading_days >= diag.min_days_required ? "text-eve-profit" : "text-eve-error"}`}>
                          {item.trading_days}d
                        </td>
                        <td className="py-1 text-right text-eve-dim">{item.transactions}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}

            <div className="text-[10px] text-eve-dim text-center mt-2 border-t border-eve-border pt-2">
              {t("optDiagExplanation")}
            </div>
          </div>
        ) : (
          <div className="text-[10px] max-w-md text-center text-eve-dim">{t("optNoDataHint")}</div>
        )}
      </div>
    );
  }

  if (!result || !result.ok) {
    return (
      <div className="flex flex-col items-center justify-center h-full text-eve-dim text-xs space-y-2">
        <div>{t("optNoData")}</div>
        <div className="text-[10px] max-w-md text-center">{t("optNoDataHint")}</div>
      </div>
    );
  }

  const data = result.data;

  return (
    <div className="space-y-4">
      {/* Header + Period selector */}
      <div className="flex items-center justify-between">
        <div>
          <div className="text-xs text-eve-dim uppercase tracking-wider">{t("optTitle")}</div>
          <div className="text-[10px] text-eve-dim mt-0.5">{t("optDesc")}</div>
        </div>
        <div className="flex gap-1">
          {([30, 90, 180] as OptPeriod[]).map((p) => (
            <button
              key={p}
              onClick={() => setPeriod(p)}
              className={`px-2.5 py-1 text-[10px] rounded-sm border transition-colors ${
                period === p
                  ? "bg-eve-accent/20 border-eve-accent text-eve-accent"
                  : "bg-eve-panel border-eve-border text-eve-dim hover:text-eve-text hover:border-eve-accent/50"
              }`}
            >
              {t(`optPeriod${p}d` as TranslationKey)}
            </button>
          ))}
        </div>
      </div>

      {/* Portfolio comparison cards */}
      <div className="grid grid-cols-3 gap-3">
        <div className="bg-eve-panel border border-eve-border rounded-sm p-3">
          <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-1">{t("optCurrentPortfolio")}</div>
          <div className="text-lg font-bold text-eve-text">{data.current_sharpe.toFixed(2)}</div>
          <div className="text-xs text-eve-dim">{t("optSharpe")}</div>
        </div>
        <div className="bg-eve-panel border border-eve-accent/30 rounded-sm p-3">
          <div className="text-[10px] text-eve-accent uppercase tracking-wider mb-1">{t("optOptimalPortfolio")}</div>
          <div className="text-lg font-bold text-eve-accent">{data.optimal_sharpe.toFixed(2)}</div>
          <div className="text-xs text-eve-dim">{t("optSharpe")}</div>
        </div>
        <div className="bg-eve-panel border border-eve-border rounded-sm p-3">
          <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-1">{t("optMinVarPortfolio")}</div>
          <div className="text-lg font-bold text-eve-text">{data.min_var_sharpe.toFixed(2)}</div>
          <div className="text-xs text-eve-dim">{t("optSharpe")}</div>
        </div>
      </div>

      {/* Diversification metrics */}
      <div className="grid grid-cols-2 gap-3">
        <StatCard
          label={t("optHHI")}
          value={data.hhi.toFixed(3)}
          subvalue={t("optHHIHint")}
          color={data.hhi < 0.15 ? "text-eve-profit" : data.hhi < 0.25 ? "text-eve-accent" : "text-eve-error"}
        />
        <StatCard
          label={t("optDivRatio")}
          value={data.diversification_ratio.toFixed(2)}
          subvalue={t("optDivRatioHint")}
          color={data.diversification_ratio > 1.2 ? "text-eve-profit" : "text-eve-accent"}
        />
      </div>

      {/* Efficient Frontier */}
      {data.efficient_frontier && data.efficient_frontier.length > 0 && (
        <div className="bg-eve-panel border border-eve-border rounded-sm p-3">
          <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-1">{t("optFrontier")}</div>
          <div className="text-[9px] text-eve-dim mb-2">{t("optFrontierHint")}</div>
          <EfficientFrontierChart
            frontier={data.efficient_frontier}
            currentWeights={data.current_weights}
            optimalWeights={data.optimal_weights}
            minVarWeights={data.min_var_weights}
            means={data.assets.map((a) => a.avg_daily_pnl)}
            covApprox={data.correlation_matrix}
            assets={data.assets}
            formatIsk={formatIsk}
          />
        </div>
      )}

      {/* Correlation Matrix */}
      {data.correlation_matrix && data.assets.length > 1 && (
        <div className="bg-eve-panel border border-eve-border rounded-sm p-3">
          <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-1">{t("optCorrelation")}</div>
          <div className="text-[9px] text-eve-dim mb-2">{t("optCorrelationHint")}</div>
          <CorrelationMatrix assets={data.assets} matrix={data.correlation_matrix} />
        </div>
      )}

      {/* Asset Table */}
      <div className="bg-eve-panel border border-eve-border rounded-sm p-3">
        <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-2">{t("optAssets")}</div>
        <AssetTable assets={data.assets} currentWeights={data.current_weights} optimalWeights={data.optimal_weights} formatIsk={formatIsk} t={t} />
      </div>

      {/* Suggestions */}
      {data.suggestions && data.suggestions.filter((s) => s.action !== "hold").length > 0 && (
        <div className="bg-eve-panel border border-eve-border rounded-sm p-3">
          <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-2">{t("optSuggestions")}</div>
          <SuggestionsPanel suggestions={data.suggestions} t={t} />
        </div>
      )}
    </div>
  );
}

// --- Efficient Frontier Chart (CSS-based scatter plot) ---

function EfficientFrontierChart({
  frontier,
  currentWeights,
  optimalWeights,
  minVarWeights,
  means,
  assets,
  formatIsk,
}: {
  frontier: { risk: number; return: number }[];
  currentWeights: number[];
  optimalWeights: number[];
  minVarWeights: number[];
  means: number[];
  covApprox: number[][];
  assets: AssetStats[];
  formatIsk: (v: number) => string;
}) {
  const chartW = 600;
  const chartH = 140;

  const allRisks = frontier.map((p) => p.risk);
  const allReturns = frontier.map((p) => p.return);

  const minRisk = Math.min(...allRisks) * 0.9;
  const maxRisk = Math.max(...allRisks) * 1.1 || 1;
  const minRet = Math.min(...allReturns) * 1.1;
  const maxRet = Math.max(...allReturns) * 1.1 || 1;

  const scaleX = (r: number) => ((r - minRisk) / (maxRisk - minRisk)) * chartW;
  const scaleY = (ret: number) => chartH - ((ret - minRet) / (maxRet - minRet)) * chartH;

  // Compute portfolio positions.
  const portRet = (w: number[]) => w.reduce((s, wi, i) => s + wi * means[i], 0);
  const portRisk = (w: number[]) => {
    // Approximate: use frontier's closest return point.
    const r = portRet(w);
    const closest = frontier.reduce((a, b) => Math.abs(a.return - r) < Math.abs(b.return - r) ? a : b);
    return closest.risk;
  };

  // Individual assets.
  const assetPoints = assets.map((a) => ({ risk: a.volatility, ret: a.avg_daily_pnl, name: a.type_name }));

  return (
    <div className="relative overflow-hidden" style={{ height: chartH + 20 }}>
      <svg width={chartW} height={chartH + 20} className="w-full" viewBox={`0 0 ${chartW} ${chartH + 20}`}>
        {/* Frontier curve */}
        <polyline
          fill="none"
          stroke="#58a6ff"
          strokeWidth={2}
          opacity={0.6}
          points={frontier.map((p) => `${scaleX(p.risk)},${scaleY(p.return)}`).join(" ")}
        />

        {/* Individual assets as small dots */}
        {assetPoints.map((a, i) => (
          <circle
            key={i}
            cx={scaleX(a.risk)}
            cy={scaleY(a.ret)}
            r={3}
            fill="#8b949e"
            opacity={0.5}
          >
            <title>{a.name}: risk={formatIsk(a.risk)}, return={formatIsk(a.ret)}</title>
          </circle>
        ))}

        {/* Current portfolio */}
        <circle cx={scaleX(portRisk(currentWeights))} cy={scaleY(portRet(currentWeights))} r={6} fill="#f0883e" stroke="#f0883e" strokeWidth={2}>
          <title>Current Portfolio</title>
        </circle>

        {/* Optimal portfolio */}
        <circle cx={scaleX(portRisk(optimalWeights))} cy={scaleY(portRet(optimalWeights))} r={6} fill="#58a6ff" stroke="#58a6ff" strokeWidth={2}>
          <title>Optimal (Max Sharpe)</title>
        </circle>

        {/* Min-var portfolio */}
        <circle cx={scaleX(portRisk(minVarWeights))} cy={scaleY(portRet(minVarWeights))} r={5} fill="#3fb950" stroke="#3fb950" strokeWidth={2}>
          <title>Minimum Variance</title>
        </circle>
      </svg>

      {/* Legend */}
      <div className="flex gap-4 justify-center mt-1 text-[9px]">
        <div className="flex items-center gap-1">
          <div className="w-2 h-2 rounded-full bg-[#f0883e]" />
          <span className="text-eve-dim">Current</span>
        </div>
        <div className="flex items-center gap-1">
          <div className="w-2 h-2 rounded-full bg-[#58a6ff]" />
          <span className="text-eve-dim">Optimal</span>
        </div>
        <div className="flex items-center gap-1">
          <div className="w-2 h-2 rounded-full bg-[#3fb950]" />
          <span className="text-eve-dim">Min Var</span>
        </div>
        <div className="flex items-center gap-1">
          <div className="w-2 h-2 rounded-full bg-[#8b949e] opacity-50" />
          <span className="text-eve-dim">Assets</span>
        </div>
      </div>
    </div>
  );
}

// --- Correlation Matrix Heatmap ---

function CorrelationMatrix({ assets, matrix }: { assets: AssetStats[]; matrix: number[][] }) {
  if (assets.length === 0) return null;

  const cellSize = Math.min(28, Math.floor(600 / assets.length));

  const corrColor = (v: number) => {
    if (v >= 0.5) return "bg-emerald-500/80";
    if (v >= 0.2) return "bg-emerald-500/40";
    if (v >= -0.2) return "bg-eve-dim/20";
    if (v >= -0.5) return "bg-red-500/40";
    return "bg-red-500/80";
  };

  return (
    <div className="overflow-x-auto">
      <div className="inline-flex flex-col gap-px">
        {/* Header row */}
        <div className="flex gap-px items-end" style={{ marginLeft: cellSize * 3 }}>
          {assets.map((a, j) => (
            <div
              key={j}
              className="text-[8px] text-eve-dim truncate text-center"
              style={{ width: cellSize }}
              title={a.type_name}
            >
              {a.type_name.slice(0, 4)}
            </div>
          ))}
        </div>
        {/* Matrix rows */}
        {assets.map((rowAsset, i) => (
          <div key={i} className="flex gap-px items-center">
            <div
              className="text-[8px] text-eve-dim truncate text-right pr-1"
              style={{ width: cellSize * 3 }}
              title={rowAsset.type_name}
            >
              {rowAsset.type_name.slice(0, 12)}
            </div>
            {matrix[i].map((val, j) => (
              <div
                key={j}
                className={`flex items-center justify-center text-[8px] font-mono rounded-[2px] ${corrColor(val)} ${
                  i === j ? "ring-1 ring-eve-accent/30" : ""
                }`}
                style={{ width: cellSize, height: cellSize }}
                title={`${rowAsset.type_name} × ${assets[j].type_name}: ${val.toFixed(2)}`}
              >
                {assets.length <= 10 ? val.toFixed(1) : ""}
              </div>
            ))}
          </div>
        ))}
      </div>
    </div>
  );
}

// --- Asset Table ---

function AssetTable({
  assets,
  currentWeights,
  optimalWeights,
  formatIsk,
  t,
}: {
  assets: AssetStats[];
  currentWeights: number[];
  optimalWeights: number[];
  formatIsk: (v: number) => string;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}) {
  return (
    <div className="border border-eve-border rounded-sm overflow-hidden">
      <table className="w-full text-xs">
        <thead className="bg-eve-panel">
          <tr className="text-eve-dim">
            <th className="px-3 py-2 text-left">{t("optAssetName")}</th>
            <th className="px-3 py-2 text-right">{t("optCurrentPct")}</th>
            <th className="px-3 py-2 text-right">{t("optOptimalPct")}</th>
            <th className="px-3 py-2 text-right">{t("optAssetPnL")}</th>
            <th className="px-3 py-2 text-right">{t("optAssetSharpe")}</th>
            <th className="px-3 py-2 text-right">{t("optAssetVol")}</th>
            <th className="px-3 py-2 text-right">{t("optAssetDays")}</th>
          </tr>
        </thead>
        <tbody>
          {assets.map((asset, i) => {
            return (
              <tr key={asset.type_id} className="border-t border-eve-border/50 hover:bg-eve-panel/50">
                <td className="px-3 py-2 text-eve-text">
                  <div className="flex items-center gap-2">
                    <img
                      src={`https://images.evetech.net/types/${asset.type_id}/icon?size=32`}
                      alt=""
                      className="w-5 h-5"
                    />
                    <span className="truncate max-w-[160px]">{asset.type_name || `#${asset.type_id}`}</span>
                  </div>
                </td>
                <td className="px-3 py-2 text-right text-eve-text">
                  <div className="flex items-center justify-end gap-1">
                    <div className="w-12 h-1.5 bg-eve-dark rounded-full overflow-hidden">
                      <div className="h-full rounded-full bg-[#f0883e]" style={{ width: `${currentWeights[i] * 100}%` }} />
                    </div>
                    <span>{(currentWeights[i] * 100).toFixed(1)}%</span>
                  </div>
                </td>
                <td className="px-3 py-2 text-right">
                  <div className="flex items-center justify-end gap-1">
                    <div className="w-12 h-1.5 bg-eve-dark rounded-full overflow-hidden">
                      <div className="h-full rounded-full bg-eve-accent" style={{ width: `${optimalWeights[i] * 100}%` }} />
                    </div>
                    <span className="text-eve-accent">{(optimalWeights[i] * 100).toFixed(1)}%</span>
                  </div>
                </td>
                <td className={`px-3 py-2 text-right ${asset.total_pnl >= 0 ? "text-eve-profit" : "text-eve-error"}`}>
                  {asset.total_pnl >= 0 ? "+" : ""}{formatIsk(asset.total_pnl)}
                </td>
                <td className={`px-3 py-2 text-right ${(asset.sharpe_ratio ?? 0) > 1 ? "text-eve-profit" : (asset.sharpe_ratio ?? 0) > 0 ? "text-eve-text" : "text-eve-error"}`}>
                  {(asset.sharpe_ratio ?? 0).toFixed(2)}
                </td>
                <td className="px-3 py-2 text-right text-eve-dim">{formatIsk(asset.volatility)}</td>
                <td className="px-3 py-2 text-right text-eve-dim">{asset.trading_days}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

// --- Suggestions Panel ---

function SuggestionsPanel({
  suggestions,
  t,
}: {
  suggestions: AllocationSuggestion[];
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}) {
  const actionable = suggestions.filter((s) => s.action !== "hold");
  if (actionable.length === 0) return null;

  const reasonLabels: Record<string, TranslationKey> = {
    high_sharpe: "optReasonHighSharpe",
    diversification: "optReasonDiversification",
    negative_returns: "optReasonNegativeReturns",
    poor_risk_adjusted: "optReasonPoorRiskAdjusted",
    overweight: "optReasonOverweight",
  };

  return (
    <div className="space-y-1.5">
      {actionable.map((s) => {
        const isIncrease = s.action === "increase";
        return (
          <div
            key={s.type_id}
            className={`flex items-center gap-3 px-3 py-2 rounded-sm border text-xs ${
              isIncrease
                ? "bg-emerald-500/5 border-emerald-500/20"
                : "bg-red-500/5 border-red-500/20"
            }`}
          >
            <span className={`text-[10px] font-bold uppercase tracking-wider ${isIncrease ? "text-emerald-400" : "text-red-400"}`}>
              {isIncrease ? t("optIncrease") : t("optDecrease")}
            </span>
            <img
              src={`https://images.evetech.net/types/${s.type_id}/icon?size=32`}
              alt=""
              className="w-5 h-5"
            />
            <span className="text-eve-text font-medium truncate max-w-[150px]">{s.type_name}</span>
            <span className="text-eve-dim">
              {s.current_pct.toFixed(1)}% → {s.optimal_pct.toFixed(1)}%
            </span>
            <span className={`font-mono ${isIncrease ? "text-emerald-400" : "text-red-400"}`}>
              {s.delta_pct >= 0 ? "+" : ""}{s.delta_pct.toFixed(1)}%
            </span>
            {s.reason && (
              <span className="text-eve-dim text-[10px] ml-auto">
                {t(reasonLabels[s.reason] || ("optReasonOverweight" as TranslationKey))}
              </span>
            )}
          </div>
        );
      })}
    </div>
  );
}

// --- Risk Tab ---

interface RiskTabProps {
  characterId: number;
  data: CharacterInfo;
  formatIsk: (v: number) => string;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}

function RiskTab({ characterId, data, formatIsk, t }: RiskTabProps) {
  const risk = data.risk;

  if (!risk) {
    return (
      <div className="flex flex-col items-center justify-center h-full text-eve-dim text-xs space-y-2">
        <div>{t("charRiskNoData")}</div>
        <div className="text-[10px] max-w-md text-center">
          {t("charRiskNoDataHint")}
        </div>
      </div>
    );
  }

  const riskLevelLabel =
    risk.risk_level === "safe"
      ? t("riskLevelSafe")
      : risk.risk_level === "balanced"
      ? t("riskLevelBalanced")
      : t("riskLevelHigh");

  const riskScore = Math.max(0, Math.min(100, risk.risk_score || 0));

  let riskColor = "bg-emerald-500";
  if (riskScore > 70) riskColor = "bg-red-500";
  else if (riskScore > 30) riskColor = "bg-amber-500";

  // Don't mask negative values with Math.max — show real data.
  // typical_daily_pnl and the loss metrics should be displayed as-is.
  const typicalPnl = risk.typical_daily_pnl || 0;
  const var99 = risk.var_99 || 0;
  const es99 = risk.es_99 || 0;
  const worst = risk.worst_day_loss || 0;

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex items-center gap-4 p-4 bg-eve-panel border border-eve-border rounded-sm">
        <img
          src={`https://images.evetech.net/characters/${characterId}/portrait?size=64`}
          alt=""
          className="w-12 h-12 rounded-sm"
        />
        <div className="flex-1">
          <div className="text-[10px] uppercase tracking-wider text-eve-dim mb-1">
            {t("charRiskTitle")}
          </div>
          <div className="flex items-baseline gap-2">
            <div className="text-lg font-bold text-eve-text">
              {riskLevelLabel}
            </div>
            <div className="text-xs text-eve-dim">
              {t("charRiskScoreLabel", { score: Math.round(riskScore) })}
            </div>
          </div>
          <div className="mt-2 h-2 w-full bg-eve-dark rounded-full overflow-hidden">
            <div
              className={`h-full ${riskColor}`}
              style={{ width: `${riskScore}%` }}
            />
          </div>
        </div>
      </div>

      {/* Low sample warning */}
      {risk.low_sample && (
        <div className="bg-amber-500/10 border border-amber-500/30 rounded-sm px-3 py-2 text-xs text-amber-400">
          {t("charRiskLowSample", { days: risk.sample_days })}
        </div>
      )}

      {/* Worst-case loss + daily behaviour */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
        <StatCard
          label={t("charRiskWorstDay")}
          value={`-${formatIsk(worst)} ISK`}
          subvalue={t("charRiskWorstDayHint", { days: risk.sample_days })}
          color="text-eve-error"
        />
        <StatCard
          label={t("charRiskVar99")}
          value={
            risk.var_99_reliable === false
              ? "—"
              : `-${formatIsk(var99)} ISK`
          }
          subvalue={
            risk.var_99_reliable === false
              ? t("charRiskVar99Unreliable", { min: 30 })
              : t("charRiskVar99Hint")
          }
          color={risk.var_99_reliable === false ? "text-eve-dim" : "text-eve-warning"}
        />
        <StatCard
          label={t("charRiskEs99")}
          value={
            risk.var_99_reliable === false
              ? "—"
              : `-${formatIsk(es99)} ISK`
          }
          subvalue={
            risk.var_99_reliable === false
              ? t("charRiskEs99Unreliable", { min: 30 })
              : t("charRiskEs99Hint")
          }
          color={risk.var_99_reliable === false ? "text-eve-dim" : "text-eve-warning"}
        />
      </div>

      {/* Narrative explanation */}
      <div className="bg-eve-panel border border-eve-border rounded-sm p-3 text-xs text-eve-text space-y-1">
        <div>
          {t("charRiskSentenceLoss", {
            var: formatIsk(var99),
            days: risk.window_days,
          })}
        </div>
        <div>
          {t("charRiskSentenceTail", {
            es: formatIsk(es99),
          })}
        </div>
        <div className="text-eve-dim text-[11px]">
          {t("charRiskSentenceTypical", {
            typical: formatIsk(typicalPnl),
          })}
        </div>
      </div>

      {/* Capacity / suggestion */}
      <div className="bg-eve-panel border border-eve-border rounded-sm p-3 text-xs text-eve-text">
        {risk.capacity_multiplier > 1.05 ? (
          <div>
            {t("charRiskCapacityUp", {
              mult: risk.capacity_multiplier.toFixed(1),
            })}
          </div>
        ) : (
          <div>{t("charRiskCapacityMaxed")}</div>
        )}
      </div>
    </div>
  );
}

function StatCard({
  label,
  value,
  subvalue,
  color = "text-eve-text",
  large = false,
}: {
  label: string;
  value: string;
  subvalue?: string;
  color?: string;
  large?: boolean;
}) {
  return (
    <div className="bg-eve-panel border border-eve-border rounded-sm p-3">
      <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-1">{label}</div>
      <div className={`${large ? "text-xl" : "text-lg"} font-bold ${color}`}>{value}</div>
      {subvalue && <div className="text-xs text-eve-dim">{subvalue}</div>}
    </div>
  );
}

interface OrdersTabProps {
  orders: CharacterOrder[];
  formatIsk: (v: number) => string;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}

function OrdersTab({ orders, formatIsk, t }: OrdersTabProps) {
  const [filter, setFilter] = useState<"all" | "buy" | "sell">("all");
  const [expandedOrder, setExpandedOrder] = useState<number | null>(null);
  const [undercuts, setUndercuts] = useState<Record<number, UndercutStatus>>({});
  const [undercutLoading, setUndercutLoading] = useState(false);
  const [undercutLoaded, setUndercutLoaded] = useState(false);
  const [undercutError, setUndercutError] = useState<string | null>(null);

  const filtered = orders.filter((o) => {
    if (filter === "buy") return o.is_buy_order;
    if (filter === "sell") return !o.is_buy_order;
    return true;
  });

  const loadUndercuts = useCallback(async () => {
    if (undercutLoaded || undercutLoading) return;
    setUndercutLoading(true);
    setUndercutError(null);
    try {
      const data = await getUndercuts();
      const map: Record<number, UndercutStatus> = {};
      for (const u of data) map[u.order_id] = u;
      setUndercuts(map);
      setUndercutLoaded(true);
    } catch (e: any) {
      setUndercutError(e?.message || "Unknown error");
    } finally {
      setUndercutLoading(false);
    }
  }, [undercutLoaded, undercutLoading]);

  const toggleExpand = useCallback((orderId: number) => {
    if (!undercutLoaded && !undercutLoading) loadUndercuts();
    setExpandedOrder((prev) => (prev === orderId ? null : orderId));
  }, [undercutLoaded, undercutLoading, loadUndercuts]);

  if (orders.length === 0) {
    return <div className="text-center text-eve-dim py-8">{t("charNoOrders")}</div>;
  }

  return (
    <div className="space-y-3">
      {/* Filter */}
      <div className="flex gap-2">
        <FilterBtn active={filter === "all"} onClick={() => setFilter("all")} label={t("charAll")} count={orders.length} />
        <FilterBtn active={filter === "buy"} onClick={() => setFilter("buy")} label={t("charBuy")} count={orders.filter((o) => o.is_buy_order).length} color="text-eve-profit" />
        <FilterBtn active={filter === "sell"} onClick={() => setFilter("sell")} label={t("charSell")} count={orders.filter((o) => !o.is_buy_order).length} color="text-eve-error" />
      </div>

      {/* Undercut error */}
      {undercutError && (
        <div className="bg-eve-error/10 border border-eve-error/30 rounded-sm px-3 py-2 text-xs text-eve-error">
          {t("charUndercutError")}: {undercutError}
        </div>
      )}

      {/* Table */}
      <div className="border border-eve-border rounded-sm overflow-hidden">
        <table className="w-full text-xs">
          <thead className="bg-eve-panel">
            <tr className="text-eve-dim">
              <th className="px-3 py-2 text-left">{t("charOrderType")}</th>
              <th className="px-3 py-2 text-left">{t("colItemName")}</th>
              <th className="px-3 py-2 text-right">{t("charPrice")}</th>
              <th className="px-3 py-2 text-right">{t("charVolume")}</th>
              <th className="px-3 py-2 text-right">{t("charTotal")}</th>
              <th className="px-3 py-2 text-left">{t("charLocation")}</th>
              <th className="w-8"></th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((order) => {
              const uc = undercuts[order.order_id];
              const isExpanded = expandedOrder === order.order_id;
              // Undercut indicator color
              let indicatorColor = "bg-eve-dim/30 text-eve-dim"; // gray = #1 or unknown
              if (uc) {
                if (uc.position === 1) {
                  indicatorColor = "bg-emerald-500/20 text-emerald-400";
                } else if (uc.undercut_pct > 1) {
                  indicatorColor = "bg-red-500/20 text-red-400";
                } else if (uc.undercut_pct > 0) {
                  indicatorColor = "bg-amber-500/20 text-amber-400";
                }
              }

              return (
                <OrderRow
                  key={order.order_id}
                  order={order}
                  uc={uc}
                  isExpanded={isExpanded}
                  indicatorColor={indicatorColor}
                  undercutLoading={undercutLoading}
                  formatIsk={formatIsk}
                  toggleExpand={toggleExpand}
                  t={t}
                />
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function OrderRow({
  order,
  uc,
  isExpanded,
  indicatorColor,
  undercutLoading,
  formatIsk,
  toggleExpand,
  t,
}: {
  order: CharacterOrder;
  uc: UndercutStatus | undefined;
  isExpanded: boolean;
  indicatorColor: string;
  undercutLoading: boolean;
  formatIsk: (v: number) => string;
  toggleExpand: (id: number) => void;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}) {
  return (
    <>
      <tr className={`border-t border-eve-border/50 hover:bg-eve-panel/50 ${isExpanded ? "bg-eve-panel/50" : ""}`}>
        <td className="px-3 py-2">
          <span className={`inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-medium ${
            order.is_buy_order ? "bg-eve-profit/20 text-eve-profit" : "bg-eve-error/20 text-eve-error"
          }`}>
            {order.is_buy_order ? "BUY" : "SELL"}
          </span>
        </td>
        <td className="px-3 py-2 text-eve-text font-medium">
          <div className="flex items-center gap-2">
            <img
              src={`https://images.evetech.net/types/${order.type_id}/icon?size=32`}
              alt=""
              className="w-5 h-5"
            />
            {order.type_name || `Type #${order.type_id}`}
          </div>
        </td>
        <td className="px-3 py-2 text-right text-eve-accent">{formatIsk(order.price)}</td>
        <td className="px-3 py-2 text-right text-eve-dim">
          {order.volume_remain.toLocaleString()}/{order.volume_total.toLocaleString()}
        </td>
        <td className="px-3 py-2 text-right text-eve-text">{formatIsk(order.price * order.volume_remain)}</td>
        <td className="px-3 py-2 text-eve-dim text-[11px] max-w-[200px] truncate" title={order.location_name}>
          {order.location_name || `Location #${order.location_id}`}
        </td>
        <td className="px-1 py-2 text-center">
          <button
            onClick={() => toggleExpand(order.order_id)}
            className={`inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[9px] font-medium uppercase tracking-wide transition-colors ${indicatorColor} hover:brightness-125`}
            title={t("undercutBtn")}
          >
            {uc ? `#${uc.position}` : "?"}
            <svg className={`w-2.5 h-2.5 transition-transform ${isExpanded ? "rotate-180" : ""}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={3}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
            </svg>
          </button>
        </td>
      </tr>
      {isExpanded && (
        <tr>
          <td colSpan={7} className="p-0">
            <UndercutPanel
              order={order}
              uc={uc}
              loading={undercutLoading}
              formatIsk={formatIsk}
              t={t}
            />
          </td>
        </tr>
      )}
    </>
  );
}

function UndercutPanel({
  order,
  uc,
  loading,
  formatIsk,
  t,
}: {
  order: CharacterOrder;
  uc: UndercutStatus | undefined;
  loading: boolean;
  formatIsk: (v: number) => string;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}) {
  if (loading && !uc) {
    return (
      <div className="px-4 py-3 bg-eve-dark/60 border-t border-eve-border/30 text-eve-dim text-xs flex items-center gap-2">
        <span className="inline-block w-3 h-3 border-2 border-eve-accent/40 border-t-eve-accent rounded-full animate-spin" />
        {t("undercutLoading")}
      </div>
    );
  }

  if (!uc) {
    return (
      <div className="px-4 py-3 bg-eve-dark/60 border-t border-eve-border/30 text-eve-dim text-xs">
        {t("undercutLoading")}
      </div>
    );
  }

  const isFirst = uc.position === 1;
  const maxVolume = uc.book_levels.length > 0 ? Math.max(...uc.book_levels.map((l) => l.volume)) : 1;

  return (
    <div className="px-4 py-3 bg-eve-dark/60 border-t border-eve-border/30 space-y-3">
      {/* Summary row */}
      <div className="flex flex-wrap gap-4 text-xs">
        {/* Position */}
        <div>
          <div className="text-[10px] text-eve-dim uppercase tracking-wider">{t("undercutPosition")}</div>
          <div className={`font-bold text-sm ${isFirst ? "text-emerald-400" : "text-amber-400"}`}>
            #{uc.position} <span className="text-eve-dim font-normal text-[10px]">{t("undercutOfSellers", { total: uc.total_orders })}</span>
          </div>
        </div>

        {/* Undercut by */}
        {!isFirst && (
          <div>
            <div className="text-[10px] text-eve-dim uppercase tracking-wider">{t("undercutByAmount")}</div>
            <div className="font-bold text-sm text-red-400">
              {formatIsk(uc.undercut_amount)} ISK <span className="text-eve-dim font-normal text-[10px]">({uc.undercut_pct.toFixed(2)}%)</span>
            </div>
          </div>
        )}

        {/* Best market price */}
        <div>
          <div className="text-[10px] text-eve-dim uppercase tracking-wider">{t("undercutBestPrice")}</div>
          <div className="font-bold text-sm text-eve-accent">{formatIsk(uc.best_price)} ISK</div>
        </div>

        {/* Your price */}
        <div>
          <div className="text-[10px] text-eve-dim uppercase tracking-wider">{t("undercutYourPrice")}</div>
          <div className="font-bold text-sm text-eve-text">{formatIsk(order.price)} ISK</div>
        </div>

        {/* Suggested */}
        {!isFirst && (
          <div>
            <div className="text-[10px] text-eve-dim uppercase tracking-wider">{t("undercutSuggested")}</div>
            <div className="font-bold text-sm text-emerald-400">{formatIsk(uc.suggested_price)} ISK</div>
          </div>
        )}

        {isFirst && (
          <div className="flex items-center">
            <span className="px-2 py-1 rounded text-[10px] font-medium bg-emerald-500/20 text-emerald-400">
              {t("undercutNoBeat")}
            </span>
          </div>
        )}
      </div>

      {/* Order book snippet */}
      {uc.book_levels.length > 0 && (
        <div>
          <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-1">{t("undercutOrderBook")}</div>
          <div className="space-y-0.5">
            {uc.book_levels.map((level, i) => {
              const pct = maxVolume > 0 ? (level.volume / maxVolume) * 100 : 0;
              const isSell = !order.is_buy_order;
              const barColor = level.is_player
                ? "bg-eve-accent/30"
                : isSell
                  ? "bg-red-500/15"
                  : "bg-emerald-500/15";
              const textColor = level.is_player ? "text-eve-accent" : "text-eve-text";

              return (
                <div key={i} className="flex items-center gap-2 text-[11px] h-5">
                  <div className={`w-24 text-right font-mono ${textColor}`}>
                    {formatIsk(level.price)}
                  </div>
                  <div className="flex-1 relative h-full rounded-sm overflow-hidden bg-eve-panel/30">
                    <div className={`absolute inset-y-0 left-0 ${barColor} rounded-sm`} style={{ width: `${pct}%` }} />
                    <div className="relative px-1.5 flex items-center h-full">
                      <span className="text-eve-dim text-[10px]">{level.volume.toLocaleString()}</span>
                    </div>
                  </div>
                  {level.is_player && (
                    <span className="text-[9px] font-bold text-eve-accent tracking-wider">{t("undercutYou")}</span>
                  )}
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}

interface HistoryTabProps {
  history: HistoricalOrder[];
  formatIsk: (v: number) => string;
  formatDate: (d: string) => string;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}

function HistoryTab({ history, formatIsk, formatDate, t }: HistoryTabProps) {
  const [filter, setFilter] = useState<"all" | "fulfilled" | "cancelled" | "expired">("all");
  const [search, setSearch] = useState("");
  const [visibleCount, setVisibleCount] = useState(100);

  const sorted = useMemo(() =>
    [...history].sort((a, b) => new Date(b.issued).getTime() - new Date(a.issued).getTime()),
    [history]
  );

  const filtered = useMemo(() => {
    let items = sorted;
    if (filter !== "all") items = items.filter((o) => o.state === filter);
    if (search.trim()) {
      const q = search.toLowerCase();
      items = items.filter((o) => (o.type_name || "").toLowerCase().includes(q));
    }
    return items;
  }, [sorted, filter, search]);

  if (history.length === 0) {
    return <div className="text-center text-eve-dim py-8">{t("charNoHistory")}</div>;
  }

  const stateColors: Record<string, string> = {
    fulfilled: "bg-eve-profit/20 text-eve-profit",
    cancelled: "bg-eve-warning/20 text-eve-warning",
    expired: "bg-eve-dim/20 text-eve-dim",
  };

  return (
    <div className="space-y-3">
      {/* Filter + Search */}
      <div className="flex flex-wrap gap-2 items-center">
        <FilterBtn active={filter === "all"} onClick={() => setFilter("all")} label={t("charAll")} count={history.length} />
        <FilterBtn active={filter === "fulfilled"} onClick={() => setFilter("fulfilled")} label={t("charFulfilled")} count={history.filter((o) => o.state === "fulfilled").length} color="text-eve-profit" />
        <FilterBtn active={filter === "cancelled"} onClick={() => setFilter("cancelled")} label={t("charCancelled")} count={history.filter((o) => o.state === "cancelled").length} color="text-eve-warning" />
        <FilterBtn active={filter === "expired"} onClick={() => setFilter("expired")} label={t("charExpired")} count={history.filter((o) => o.state === "expired").length} color="text-eve-dim" />
        <input
          type="text"
          value={search}
          onChange={(e) => { setSearch(e.target.value); setVisibleCount(100); }}
          placeholder={t("charSearchPlaceholder")}
          className="ml-auto px-2 py-1 text-xs bg-eve-dark border border-eve-border rounded-sm text-eve-text placeholder:text-eve-dim/50 w-40 focus:border-eve-accent outline-none"
        />
      </div>

      {/* Table */}
      <div className="border border-eve-border rounded-sm overflow-hidden">
        <table className="w-full text-xs">
          <thead className="bg-eve-panel">
            <tr className="text-eve-dim">
              <th className="px-3 py-2 text-left">{t("charState")}</th>
              <th className="px-3 py-2 text-left">{t("charOrderType")}</th>
              <th className="px-3 py-2 text-left">{t("colItemName")}</th>
              <th className="px-3 py-2 text-right">{t("charPrice")}</th>
              <th className="px-3 py-2 text-right">{t("charFilled")}</th>
              <th className="px-3 py-2 text-left">{t("charLocation")}</th>
              <th className="px-3 py-2 text-left">{t("charIssued")}</th>
            </tr>
          </thead>
          <tbody>
            {filtered.slice(0, visibleCount).map((order) => (
              <tr key={order.order_id} className="border-t border-eve-border/50 hover:bg-eve-panel/50">
                <td className="px-3 py-2">
                  <span className={`inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium uppercase ${stateColors[order.state] || ""}`}>
                    {order.state}
                  </span>
                </td>
                <td className="px-3 py-2">
                  <span className={`text-[10px] font-medium ${order.is_buy_order ? "text-eve-profit" : "text-eve-error"}`}>
                    {order.is_buy_order ? "BUY" : "SELL"}
                  </span>
                </td>
                <td className="px-3 py-2 text-eve-text">
                  <div className="flex items-center gap-2">
                    <img
                      src={`https://images.evetech.net/types/${order.type_id}/icon?size=32`}
                      alt=""
                      className="w-5 h-5"
                    />
                    {order.type_name || `Type #${order.type_id}`}
                  </div>
                </td>
                <td className="px-3 py-2 text-right text-eve-accent">{formatIsk(order.price)}</td>
                <td className="px-3 py-2 text-right text-eve-dim">
                  {(order.volume_total - order.volume_remain).toLocaleString()}/{order.volume_total.toLocaleString()}
                </td>
                <td className="px-3 py-2 text-eve-dim text-[11px] max-w-[180px] truncate" title={order.location_name}>
                  {order.location_name || `#${order.location_id}`}
                </td>
                <td className="px-3 py-2 text-eve-dim text-[11px]">{formatDate(order.issued)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {filtered.length > visibleCount && (
        <button
          onClick={() => setVisibleCount((prev) => prev + 100)}
          className="w-full text-center text-eve-accent text-xs py-2 hover:bg-eve-panel/50 border border-eve-border rounded-sm transition-colors"
        >
          {t("andMore", { count: filtered.length - visibleCount })} — load more
        </button>
      )}
    </div>
  );
}

interface TransactionsTabProps {
  transactions: WalletTransaction[];
  formatIsk: (v: number) => string;
  formatDate: (d: string) => string;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}

function TransactionsTab({ transactions, formatIsk, formatDate, t }: TransactionsTabProps) {
  const [filter, setFilter] = useState<"all" | "buy" | "sell">("all");
  const [search, setSearch] = useState("");
  const [visibleCount, setVisibleCount] = useState(100);

  const sorted = useMemo(() =>
    [...transactions].sort((a, b) => new Date(b.date).getTime() - new Date(a.date).getTime()),
    [transactions]
  );

  const filtered = useMemo(() => {
    let items = sorted;
    if (filter === "buy") items = items.filter((tx) => tx.is_buy);
    if (filter === "sell") items = items.filter((tx) => !tx.is_buy);
    if (search.trim()) {
      const q = search.toLowerCase();
      items = items.filter((tx) => (tx.type_name || "").toLowerCase().includes(q));
    }
    return items;
  }, [sorted, filter, search]);

  if (transactions.length === 0) {
    return <div className="text-center text-eve-dim py-8">{t("charNoTransactions")}</div>;
  }

  return (
    <div className="space-y-3">
      {/* Filter + Search */}
      <div className="flex flex-wrap gap-2 items-center">
        <FilterBtn active={filter === "all"} onClick={() => setFilter("all")} label={t("charAll")} count={transactions.length} />
        <FilterBtn active={filter === "buy"} onClick={() => setFilter("buy")} label={t("charBuy")} count={transactions.filter((t) => t.is_buy).length} color="text-eve-profit" />
        <FilterBtn active={filter === "sell"} onClick={() => setFilter("sell")} label={t("charSell")} count={transactions.filter((t) => !t.is_buy).length} color="text-eve-error" />
        <input
          type="text"
          value={search}
          onChange={(e) => { setSearch(e.target.value); setVisibleCount(100); }}
          placeholder={t("charSearchPlaceholder")}
          className="ml-auto px-2 py-1 text-xs bg-eve-dark border border-eve-border rounded-sm text-eve-text placeholder:text-eve-dim/50 w-40 focus:border-eve-accent outline-none"
        />
      </div>

      {/* Table */}
      <div className="border border-eve-border rounded-sm overflow-hidden">
        <table className="w-full text-xs">
          <thead className="bg-eve-panel">
            <tr className="text-eve-dim">
              <th className="px-3 py-2 text-left">{t("charOrderType")}</th>
              <th className="px-3 py-2 text-left">{t("colItemName")}</th>
              <th className="px-3 py-2 text-right">{t("charUnitPrice")}</th>
              <th className="px-3 py-2 text-right">{t("charQty")}</th>
              <th className="px-3 py-2 text-right">{t("charTotal")}</th>
              <th className="px-3 py-2 text-left">{t("charLocation")}</th>
              <th className="px-3 py-2 text-left">{t("charDate")}</th>
            </tr>
          </thead>
          <tbody>
            {filtered.slice(0, visibleCount).map((tx) => (
              <tr key={tx.transaction_id} className="border-t border-eve-border/50 hover:bg-eve-panel/50">
                <td className="px-3 py-2">
                  <span className={`inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-medium ${
                    tx.is_buy ? "bg-eve-profit/20 text-eve-profit" : "bg-eve-error/20 text-eve-error"
                  }`}>
                    {tx.is_buy ? "BUY" : "SELL"}
                  </span>
                </td>
                <td className="px-3 py-2 text-eve-text">
                  <div className="flex items-center gap-2">
                    <img
                      src={`https://images.evetech.net/types/${tx.type_id}/icon?size=32`}
                      alt=""
                      className="w-5 h-5"
                    />
                    {tx.type_name || `Type #${tx.type_id}`}
                  </div>
                </td>
                <td className="px-3 py-2 text-right text-eve-accent">{formatIsk(tx.unit_price)}</td>
                <td className="px-3 py-2 text-right text-eve-dim">{tx.quantity.toLocaleString()}</td>
                <td className="px-3 py-2 text-right text-eve-text">{formatIsk(tx.unit_price * tx.quantity)}</td>
                <td className="px-3 py-2 text-eve-dim text-[11px] max-w-[180px] truncate" title={tx.location_name}>
                  {tx.location_name || `#${tx.location_id}`}
                </td>
                <td className="px-3 py-2 text-eve-dim text-[11px]">{formatDate(tx.date)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {filtered.length > visibleCount && (
        <button
          onClick={() => setVisibleCount((prev) => prev + 100)}
          className="w-full text-center text-eve-accent text-xs py-2 hover:bg-eve-panel/50 border border-eve-border rounded-sm transition-colors"
        >
          {t("andMore", { count: filtered.length - visibleCount })} — load more
        </button>
      )}
    </div>
  );
}

function FilterBtn({
  active,
  onClick,
  label,
  count,
  color = "text-eve-text",
}: {
  active: boolean;
  onClick: () => void;
  label: string;
  count: number;
  color?: string;
}) {
  return (
    <button
      onClick={onClick}
      className={`px-3 py-1 text-xs rounded-sm border transition-colors ${
        active
          ? "bg-eve-accent/20 border-eve-accent text-eve-accent"
          : "bg-eve-panel border-eve-border text-eve-dim hover:text-eve-text hover:border-eve-accent/50"
      }`}
    >
      <span className={active ? "" : color}>{label}</span>
      <span className="ml-1 opacity-60">({count})</span>
    </button>
  );
}
