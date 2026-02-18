import { useEffect, useState, useCallback, useMemo, useRef } from "react";
import { getOrderDesk, getUndercuts, openMarketInGame, type CharacterScope } from "../../lib/api";
import { type TranslationKey } from "../../lib/i18n";
import type { CharacterOrder, HistoricalOrder, OrderDeskResponse, UndercutStatus } from "../../lib/types";
import { handleEveUIError } from "../../lib/handleEveUIError";
import { useGlobalToast } from "../Toast";
import { FilterBtn, StatCard } from "./shared";
interface CombinedOrdersTabProps {
  characterScope: CharacterScope;
  orders: CharacterOrder[];
  history: HistoricalOrder[];
  formatIsk: (v: number) => string;
  formatDate: (d: string) => string;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}

export function CombinedOrdersTab({ characterScope, orders, history, formatIsk, formatDate, t }: CombinedOrdersTabProps) {
  const [subTab, setSubTab] = useState<"active" | "history">("active");

  return (
    <div className="flex flex-col h-full">
      {/* Sub-tabs */}
      <div className="flex gap-1 border-b border-eve-border bg-eve-panel/50 mb-3 -mt-4 -mx-4 px-4">
        <button
          onClick={() => setSubTab("active")}
          className={`px-3 py-2 text-xs font-medium transition-colors ${
            subTab === "active"
              ? "text-eve-accent border-b-2 border-eve-accent"
              : "text-eve-dim hover:text-eve-text"
          }`}
        >
          {t("charActiveOrders")} ({orders.length})
        </button>
        <button
          onClick={() => setSubTab("history")}
          className={`px-3 py-2 text-xs font-medium transition-colors ${
            subTab === "history"
              ? "text-eve-accent border-b-2 border-eve-accent"
              : "text-eve-dim hover:text-eve-text"
          }`}
        >
          {t("charOrderHistory")} ({history.length})
        </button>
      </div>

      {/* Content */}
      <div className="flex-1 min-h-0 overflow-auto">
        {subTab === "active" && (
          <ActiveOrdersWithDeskTab characterScope={characterScope} orders={orders} formatIsk={formatIsk} t={t} />
        )}
        {subTab === "history" && (
          <HistoryTab history={history} formatIsk={formatIsk} formatDate={formatDate} t={t} />
        )}
      </div>
    </div>
  );
}

interface ActiveOrdersWithDeskTabProps {
  characterScope: CharacterScope;
  orders: CharacterOrder[];
  formatIsk: (v: number) => string;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}

function ActiveOrdersWithDeskTab({ characterScope, orders, formatIsk, t }: ActiveOrdersWithDeskTabProps) {
  const { addToast } = useGlobalToast();
  const [filter, setFilter] = useState<"all" | "buy" | "sell">("all");
  const [expandedOrder, setExpandedOrder] = useState<number | null>(null);
  const [undercuts, setUndercuts] = useState<Record<number, UndercutStatus>>({});
  const [undercutLoading, setUndercutLoading] = useState(false);
  const [undercutLoaded, setUndercutLoaded] = useState(false);
  const [undercutError, setUndercutError] = useState<string | null>(null);

  // Order Desk state
  const [salesTax, setSalesTax] = useState(8);
  const [brokerFee, setBrokerFee] = useState(1);
  const [targetEtaDays, setTargetEtaDays] = useState(3);
  const [deskLoading, setDeskLoading] = useState(false);
  const [deskError, setDeskError] = useState<string | null>(null);
  const [deskData, setDeskData] = useState<OrderDeskResponse | null>(null);
  const [showDesk, setShowDesk] = useState(false);
  const deskReqSeq = useRef(0);

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
      const data = await getUndercuts(characterScope);
      const map: Record<number, UndercutStatus> = {};
      for (const u of data) map[u.order_id] = u;
      setUndercuts(map);
      setUndercutLoaded(true);
    } catch (e: any) {
      setUndercutError(e?.message || "Unknown error");
    } finally {
      setUndercutLoading(false);
    }
  }, [undercutLoaded, undercutLoading, characterScope]);

  const toggleExpand = useCallback((orderId: number) => {
    if (!undercutLoaded && !undercutLoading) loadUndercuts();
    setExpandedOrder((prev) => (prev === orderId ? null : orderId));
  }, [undercutLoaded, undercutLoading, loadUndercuts]);

  const loadDesk = useCallback(() => {
    const reqID = ++deskReqSeq.current;
    setDeskLoading(true);
    setDeskError(null);
    getOrderDesk({ salesTax, brokerFee, targetEtaDays, characterId: characterScope })
      .then((next) => {
        if (reqID !== deskReqSeq.current) return;
        setDeskData(next);
      })
      .catch((e) => {
        if (reqID !== deskReqSeq.current) return;
        setDeskError(e?.message || "Unknown error");
      })
      .finally(() => {
        if (reqID === deskReqSeq.current) {
          setDeskLoading(false);
        }
      });
  }, [salesTax, brokerFee, targetEtaDays, characterScope]);

  const toggleDesk = useCallback(() => {
    if (!showDesk && !deskData) {
      loadDesk();
    }
    setShowDesk((prev) => !prev);
  }, [showDesk, deskData, loadDesk]);

  const hasDeskParamDrift = useMemo(() => {
    if (!deskData) return false;
    const s = deskData.settings;
    return (
      Math.abs(s.sales_tax_percent - salesTax) > 1e-9 ||
      Math.abs(s.broker_fee_percent - brokerFee) > 1e-9 ||
      Math.abs(s.target_eta_days - targetEtaDays) > 1e-9
    );
  }, [deskData, salesTax, brokerFee, targetEtaDays]);

  useEffect(() => {
    if (!showDesk || !deskData || !hasDeskParamDrift) return;
    const timer = window.setTimeout(() => {
      loadDesk();
    }, 400);
    return () => window.clearTimeout(timer);
  }, [showDesk, deskData, hasDeskParamDrift, loadDesk]);

  const deskRows = useMemo(() => {
    const source = deskData?.orders ?? [];
    if (filter === "buy") return source.filter((o) => o.is_buy_order);
    if (filter === "sell") return source.filter((o) => !o.is_buy_order);
    return source;
  }, [deskData, filter]);

  const effectiveTargetEtaDays = deskData?.settings.target_eta_days ?? targetEtaDays;
  const effectiveWarnExpiryDays = deskData?.settings.warn_expiry_days ?? 2;

  const recommendationLabel = useCallback((value: string) => {
    if (value === "cancel") return t("orderDeskActionCancel");
    if (value === "reprice") return t("orderDeskActionReprice");
    return t("orderDeskActionHold");
  }, [t]);

  const openMarketForType = useCallback(async (typeID: number) => {
    if (!typeID) return;
    try {
      await openMarketInGame(typeID);
      addToast(t("actionSuccess"), "success", 2000);
    } catch (err: any) {
      const { messageKey, duration } = handleEveUIError(err);
      if (messageKey === "actionFailed") {
        addToast(t(messageKey, { error: err?.message || "Unknown error" }), "error", duration);
      } else {
        addToast(t(messageKey), "error", duration);
      }
    }
  }, [addToast, t]);

  if (orders.length === 0) {
    return <div className="text-center text-eve-dim py-8">{t("charNoOrders")}</div>;
  }

  return (
    <div className="space-y-3">
      {/* Filter + Order Desk Toggle */}
      <div className="flex gap-2 items-center flex-wrap">
        <FilterBtn active={filter === "all"} onClick={() => setFilter("all")} label={t("charAll")} count={orders.length} />
        <FilterBtn active={filter === "buy"} onClick={() => setFilter("buy")} label={t("charBuy")} count={orders.filter((o) => o.is_buy_order).length} color="text-eve-profit" />
        <FilterBtn active={filter === "sell"} onClick={() => setFilter("sell")} label={t("charSell")} count={orders.filter((o) => !o.is_buy_order).length} color="text-eve-error" />
        <button
          onClick={toggleDesk}
          className={`px-2.5 py-1 text-[10px] rounded-sm border ${
            showDesk
              ? "border-eve-accent/50 bg-eve-accent/10 text-eve-accent"
              : "border-eve-border text-eve-dim hover:text-eve-text hover:border-eve-accent/50"
          }`}
        >
          {showDesk ? t("orderDeskHide") : t("orderDeskOpen")}
        </button>
      </div>

      {/* Undercut error */}
      {undercutError && (
        <div className="bg-eve-error/10 border border-eve-error/30 rounded-sm px-3 py-2 text-xs text-eve-error">
          {t("charUndercutError")}: {undercutError}
        </div>
      )}

      {/* Order Desk Panel */}
      {showDesk && (
        <div className="border border-eve-accent/30 rounded-sm p-3 bg-eve-panel/30 space-y-3">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="text-xs text-eve-accent uppercase tracking-wider">{t("charOrderDeskTab")}</div>
            <div className="flex items-center gap-2 flex-wrap">
              <div className="flex items-center gap-1 text-[10px]">
                <span className="text-eve-dim">{t("pnlSalesTax")}</span>
                <input
                  type="number"
                  min={0}
                  max={100}
                  step={0.1}
                  value={salesTax}
                  onChange={(e) => setSalesTax(parseFloat(e.target.value) || 0)}
                  className="w-14 px-1 py-0.5 rounded-sm border border-eve-border bg-eve-dark text-eve-text"
                />
              </div>
              <div className="flex items-center gap-1 text-[10px]">
                <span className="text-eve-dim">{t("pnlBrokerFee")}</span>
                <input
                  type="number"
                  min={0}
                  max={100}
                  step={0.1}
                  value={brokerFee}
                  onChange={(e) => setBrokerFee(parseFloat(e.target.value) || 0)}
                  className="w-14 px-1 py-0.5 rounded-sm border border-eve-border bg-eve-dark text-eve-text"
                />
              </div>
              <div className="flex items-center gap-1 text-[10px]">
                <span className="text-eve-dim">{t("orderDeskTargetETA")}</span>
                <input
                  type="number"
                  min={0.5}
                  max={60}
                  step={0.5}
                  value={targetEtaDays}
                  onChange={(e) => setTargetEtaDays(parseFloat(e.target.value) || 3)}
                  className="w-14 px-1 py-0.5 rounded-sm border border-eve-border bg-eve-dark text-eve-text"
                />
              </div>
              <button
                onClick={loadDesk}
                disabled={deskLoading}
                className="px-2.5 py-1 text-[10px] rounded-sm border bg-eve-panel border-eve-border text-eve-dim hover:text-eve-text hover:border-eve-accent/50 disabled:opacity-50"
              >
                {t("charRefresh")}
              </button>
            </div>
          </div>

          {deskError && (
            <div className="bg-eve-error/10 border border-eve-error/30 rounded-sm px-3 py-2 text-xs text-eve-error">
              {deskError}
            </div>
          )}

          {deskLoading && !deskData && (
            <div className="flex items-center justify-center py-4 text-eve-dim text-xs">
              <span className="inline-block w-4 h-4 border-2 border-eve-accent/40 border-t-eve-accent rounded-full animate-spin mr-2" />
              {t("loading")}...
            </div>
          )}

          {deskData?.summary && (
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
              <StatCard
                label={t("charTotalOrders")}
                value={String(deskData.summary.total_orders)}
                subvalue={`${deskData.summary.buy_orders} ${t("charBuy")} / ${deskData.summary.sell_orders} ${t("charSell")}`}
              />
              <StatCard
                label={t("orderDeskNeedAction")}
                value={String(deskData.summary.needs_reprice + deskData.summary.needs_cancel)}
                subvalue={`${deskData.summary.needs_reprice} ${t("orderDeskActionReprice")} / ${deskData.summary.needs_cancel} ${t("orderDeskActionCancel")}`}
                color={(deskData.summary.needs_reprice + deskData.summary.needs_cancel) > 0 ? "text-eve-warning" : "text-eve-profit"}
              />
              <StatCard
                label={t("orderDeskMedianETA")}
                value={deskData.summary.median_eta_days > 0 ? `${deskData.summary.median_eta_days.toFixed(1)}d` : "—"}
                subvalue={`${deskData.summary.unknown_eta_count} ${t("orderDeskUnknownETA").toLowerCase()}`}
              />
              <StatCard label={t("orderDeskNotional")} value={`${formatIsk(deskData.summary.total_notional)} ISK`} />
            </div>
          )}

          {deskData && deskRows.length > 0 && (
            <div className="border border-eve-border rounded-sm overflow-hidden">
              <table className="w-full text-xs">
                <thead className="bg-eve-panel">
                  <tr className="text-eve-dim">
                    <th className="px-2 py-2 text-left">{t("orderDeskAction")}</th>
                    <th className="px-2 py-2 text-left">{t("charOrderType")}</th>
                    <th className="px-2 py-2 text-left">{t("colItemName")}</th>
                    <th className="px-2 py-2 text-right">{t("charPrice")}</th>
                    <th className="px-2 py-2 text-right">{t("charVolume")}</th>
                    <th className="px-2 py-2 text-right">{t("orderDeskQueueAhead")}</th>
                    <th className="px-2 py-2 text-right">{t("orderDeskETA")}</th>
                    <th className="px-2 py-2 text-right">{t("orderDeskExpiry")}</th>
                    <th className="px-2 py-2 text-right">{t("orderDeskSuggested")}</th>
                  </tr>
                </thead>
                <tbody>
                  {deskRows.map((row) => {
                    const sideClass = row.is_buy_order ? "text-eve-profit" : "text-eve-error";
                    const etaLabel = row.eta_days >= 0 ? `${row.eta_days.toFixed(1)}d` : "—";
                    const sideLabel = row.is_buy_order ? t("charBuy") : t("charSell");
                    const positionLabel = row.book_available ? `#${row.position}/${row.total_orders}` : "—";
                    const queueLabel = row.book_available ? row.queue_ahead_qty.toLocaleString() : "—";
                    const suggestedLabel = row.book_available ? formatIsk(row.suggested_price) : "—";
                    return (
                      <tr key={row.order_id} className="border-t border-eve-border/50 hover:bg-eve-panel/50">
                        <td className="px-2 py-2">
                          <span
                            className={`inline-flex px-1.5 py-0.5 rounded text-[10px] font-medium ${
                              row.recommendation === "cancel"
                                ? "bg-red-500/20 text-red-400"
                                : row.recommendation === "reprice"
                                  ? "bg-amber-500/20 text-amber-400"
                                  : row.book_available
                                    ? "bg-emerald-500/20 text-emerald-400"
                                    : "bg-eve-dim/20 text-eve-dim"
                            }`}
                            title={row.reason}
                          >
                            {recommendationLabel(row.recommendation)}
                          </span>
                        </td>
                        <td className={`px-2 py-2 ${sideClass}`}>{sideLabel} {positionLabel}</td>
                        <td className="px-2 py-2 text-eve-text max-w-[220px]" title={row.type_name}>
                          <div className="flex items-center justify-between gap-2">
                            <span className="truncate">{row.type_name || `#${row.type_id}`}</span>
                            <button
                              type="button"
                              onClick={() => { void openMarketForType(row.type_id); }}
                              className="shrink-0 px-1.5 py-0.5 rounded border border-eve-border text-[9px] text-eve-dim hover:text-eve-accent hover:border-eve-accent/50 transition-colors"
                              title={t("openMarket")}
                              aria-label={t("openMarket")}
                            >
                              EVE
                            </button>
                          </div>
                        </td>
                        <td className="px-2 py-2 text-right text-eve-accent">{formatIsk(row.price)}</td>
                        <td className="px-2 py-2 text-right text-eve-dim">{row.volume_remain.toLocaleString()}</td>
                        <td className="px-2 py-2 text-right text-eve-dim">{queueLabel}</td>
                        <td className={`px-2 py-2 text-right ${row.eta_days >= 0 && row.eta_days > effectiveTargetEtaDays ? "text-eve-warning" : "text-eve-dim"}`}>{etaLabel}</td>
                        <td className={`px-2 py-2 text-right ${row.days_to_expire >= 0 && row.days_to_expire <= effectiveWarnExpiryDays ? "text-eve-warning" : "text-eve-dim"}`}>{row.days_to_expire >= 0 ? `${row.days_to_expire}d` : "—"}</td>
                        <td className="px-2 py-2 text-right text-eve-text">{suggestedLabel}</td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {/* Active Orders Table */}
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
              let indicatorColor = "bg-eve-dim/30 text-eve-dim";
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
                  onOpenMarket={openMarketForType}
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
  onOpenMarket,
  t,
}: {
  order: CharacterOrder;
  uc: UndercutStatus | undefined;
  isExpanded: boolean;
  indicatorColor: string;
  undercutLoading: boolean;
  formatIsk: (v: number) => string;
  toggleExpand: (id: number) => void;
  onOpenMarket: (typeID: number) => Promise<void>;
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
          <div className="flex items-center justify-between gap-2">
            <div className="flex items-center gap-2 min-w-0">
            <img
              src={`https://images.evetech.net/types/${order.type_id}/icon?size=32`}
              alt=""
              className="w-5 h-5"
            />
              <span className="truncate">{order.type_name || `Type #${order.type_id}`}</span>
            </div>
            <button
              type="button"
              onClick={() => { void onOpenMarket(order.type_id); }}
              className="shrink-0 px-1.5 py-0.5 rounded border border-eve-border text-[9px] text-eve-dim hover:text-eve-accent hover:border-eve-accent/50 transition-colors"
              title={t("openMarket")}
              aria-label={t("openMarket")}
            >
              EVE
            </button>
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
