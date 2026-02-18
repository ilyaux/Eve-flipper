import { type TranslationKey } from "../../lib/i18n";
import type { CharacterInfo, CharacterOrder, CharacterRoles } from "../../lib/types";
import { StatCard } from "./shared";
interface OverviewTabProps {
  data: CharacterInfo;
  characterId?: number;
  isAllScope: boolean;
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

export function OverviewTab({
  data,
  characterId,
  isAllScope,
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
        {characterId ? (
          <img
            src={`https://images.evetech.net/characters/${characterId}/portrait?size=128`}
            alt=""
            className="w-16 h-16 rounded-sm"
          />
        ) : (
          <div className="w-16 h-16 rounded-sm bg-eve-dark border border-eve-border flex items-center justify-center text-xs text-eve-accent font-semibold">
            ALL
          </div>
        )}
        <div>
          <h2 className="text-lg font-bold text-eve-text">{isAllScope ? t("charAllCharacters") : data.character_name}</h2>
          {data.skills && !isAllScope && (
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
      {!isAllScope && (
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
      )}
    </div>
  );
}
