import { useMemo } from "react";
import { type TranslationKey } from "../../lib/i18n";
import type { CharacterAsset, CharacterOrder, WalletTransaction } from "../../lib/types";
import { StatCard } from "./shared";

interface WalletDashboardTabProps {
  wallet: number;
  orders: CharacterOrder[];
  transactions: WalletTransaction[];
  assets: CharacterAsset[];
  formatIsk: (v: number) => string;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}

interface DailyFlow {
  date: string;
  buy: number;
  sell: number;
  net: number;
  cumulative: number;
}

export function WalletDashboardTab({
  wallet,
  orders,
  transactions,
  assets,
  formatIsk,
  t,
}: WalletDashboardTabProps) {
  const model = useMemo(() => buildWalletDashboardModel(transactions, orders, assets), [transactions, orders, assets]);
  const netWorth = wallet + model.sellOrderValue;

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 lg:grid-cols-5 gap-3">
        <StatCard label={t("charWallet")} value={`${formatIsk(wallet)} ISK`} color="text-eve-profit" />
        <StatCard label={t("ledgerNetFlow")} value={`${model.netFlow >= 0 ? "+" : ""}${formatIsk(model.netFlow)} ISK`} color={model.netFlow >= 0 ? "text-eve-profit" : "text-eve-error"} />
        <StatCard label={t("ledgerOpenBuy")} value={`${formatIsk(model.buyOrderValue)} ISK`} color="text-eve-warning" />
        <StatCard label={t("ledgerOpenSell")} value={`${formatIsk(model.sellOrderValue)} ISK`} color="text-eve-accent" />
        <StatCard label={t("charNetWorth")} value={`${formatIsk(netWorth)} ISK`} color="text-eve-profit" large />
      </div>

      <section className="border-y border-eve-border/70 bg-eve-dark/35 px-3 py-3">
        <div className="flex items-center justify-between gap-3 mb-3">
          <div>
            <div className="text-[10px] uppercase tracking-wider text-eve-dim">{t("ledgerCashflowChart")}</div>
            <div className="text-xs text-eve-text">{t("ledgerCashflowHint")}</div>
          </div>
          <div className="text-right text-[11px] text-eve-dim">
            <div>{model.days.length} {t("ledgerDays")}</div>
            <div>{transactions.length} {t("charTxns")}</div>
          </div>
        </div>
        <CashflowChart days={model.days} formatIsk={formatIsk} emptyLabel={t("ledgerNoCashflow")} />
      </section>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-3">
        <section className="border border-eve-border rounded-sm overflow-hidden">
          <div className="px-3 py-2 bg-eve-panel text-[10px] uppercase tracking-wider text-eve-dim">{t("ledgerTopItems")}</div>
          <table className="w-full text-xs">
            <tbody>
              {model.topItems.length === 0 ? (
                <tr><td className="px-3 py-4 text-center text-eve-dim">{t("charNoTransactions")}</td></tr>
              ) : model.topItems.map((item) => (
                <tr key={item.typeID} className="border-t border-eve-border/50">
                  <td className="px-3 py-2 text-eve-text truncate">{item.name}</td>
                  <td className="px-3 py-2 text-right font-mono text-eve-accent">{formatIsk(item.turnover)}</td>
                  <td className="px-3 py-2 text-right text-eve-dim">{item.trades}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>

        <section className="border border-eve-border rounded-sm overflow-hidden">
          <div className="px-3 py-2 bg-eve-panel text-[10px] uppercase tracking-wider text-eve-dim">{t("ledgerInventoryOrders")}</div>
          <div className="grid grid-cols-2 gap-px bg-eve-border/50 text-xs">
            <LedgerCell label={t("charBuyOrders")} value={String(model.buyOrders)} />
            <LedgerCell label={t("charSellOrders")} value={String(model.sellOrders)} />
            <LedgerCell label={t("ledgerAssetTypes")} value={String(model.assetTypes)} />
            <LedgerCell label={t("ledgerAssetUnits")} value={model.assetUnits.toLocaleString()} />
          </div>
        </section>
      </div>
    </div>
  );
}

function CashflowChart({ days, formatIsk, emptyLabel }: { days: DailyFlow[]; formatIsk: (v: number) => string; emptyLabel: string }) {
  if (days.length === 0) {
    return <div className="h-52 flex items-center justify-center text-eve-dim text-xs">{emptyLabel}</div>;
  }
  const width = 760;
  const height = 220;
  const padX = 24;
  const padY = 18;
  const maxAbs = Math.max(1, ...days.map((d) => Math.abs(d.net)), ...days.map((d) => Math.abs(d.cumulative)));
  const xStep = days.length > 1 ? (width - padX * 2) / (days.length - 1) : 0;
  const y = (value: number) => height / 2 - (value / maxAbs) * (height / 2 - padY);
  const line = days.map((d, i) => `${padX + i * xStep},${y(d.cumulative)}`).join(" ");
  const barWidth = Math.max(2, Math.min(16, (width - padX * 2) / Math.max(1, days.length) - 2));

  return (
    <div className="w-full overflow-hidden">
      <svg viewBox={`0 0 ${width} ${height}`} className="w-full h-52">
        <line x1={padX} x2={width - padX} y1={height / 2} y2={height / 2} stroke="rgba(120,120,120,0.45)" strokeDasharray="3 4" />
        {days.map((d, i) => {
          const x = padX + i * xStep - barWidth / 2;
          const y0 = height / 2;
          const y1 = y(d.net);
          const top = Math.min(y0, y1);
          const h = Math.max(1, Math.abs(y1 - y0));
          return (
            <rect
              key={d.date}
              x={x}
              y={top}
              width={barWidth}
              height={h}
              fill={d.net >= 0 ? "rgba(34,197,94,0.5)" : "rgba(239,68,68,0.55)"}
            >
              <title>{`${d.date}: ${formatIsk(d.net)} ISK`}</title>
            </rect>
          );
        })}
        <polyline points={line} fill="none" stroke="#e69500" strokeWidth="2" />
      </svg>
    </div>
  );
}

function LedgerCell({ label, value }: { label: string; value: string }) {
  return (
    <div className="bg-eve-dark px-3 py-3">
      <div className="text-[10px] uppercase tracking-wider text-eve-dim">{label}</div>
      <div className="mt-1 font-mono text-sm text-eve-text">{value}</div>
    </div>
  );
}

function buildWalletDashboardModel(transactions: WalletTransaction[], orders: CharacterOrder[], assets: CharacterAsset[]) {
  const buyOrders = orders.filter((o) => o.is_buy_order);
  const sellOrders = orders.filter((o) => !o.is_buy_order);
  const buyOrderValue = buyOrders.reduce((sum, o) => sum + o.price * o.volume_remain, 0);
  const sellOrderValue = sellOrders.reduce((sum, o) => sum + o.price * o.volume_remain, 0);
  const byDate = new Map<string, { buy: number; sell: number }>();
  const byItem = new Map<number, { typeID: number; name: string; turnover: number; trades: number }>();

  for (const tx of transactions) {
    const date = tx.date.slice(0, 10);
    const value = tx.unit_price * tx.quantity;
    const day = byDate.get(date) ?? { buy: 0, sell: 0 };
    if (tx.is_buy) day.buy += value;
    else day.sell += value;
    byDate.set(date, day);

    const item = byItem.get(tx.type_id) ?? {
      typeID: tx.type_id,
      name: tx.type_name || `Type #${tx.type_id}`,
      turnover: 0,
      trades: 0,
    };
    item.turnover += value;
    item.trades += 1;
    byItem.set(tx.type_id, item);
  }

  let cumulative = 0;
  const days = Array.from(byDate.entries())
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([date, flow]) => {
      const net = flow.sell - flow.buy;
      cumulative += net;
      return { date, buy: flow.buy, sell: flow.sell, net, cumulative };
    });

  const assetTypes = new Set(assets.filter((a) => a.type_id > 0).map((a) => a.type_id)).size;
  const assetUnits = assets.reduce((sum, a) => sum + Math.max(0, a.quantity || (a.is_singleton ? 1 : 0)), 0);

  return {
    buyOrders: buyOrders.length,
    sellOrders: sellOrders.length,
    buyOrderValue,
    sellOrderValue,
    buyFlow: days.reduce((sum, d) => sum + d.buy, 0),
    sellFlow: days.reduce((sum, d) => sum + d.sell, 0),
    netFlow: days.reduce((sum, d) => sum + d.net, 0),
    days,
    topItems: Array.from(byItem.values()).sort((a, b) => b.turnover - a.turnover).slice(0, 8),
    assetTypes,
    assetUnits,
  };
}
