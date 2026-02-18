import { useState } from "react";
import { formatISK } from "../../lib/format";
import { useI18n } from "../../lib/i18n";
import type {
  ArbitragePath,
  InjectionTier,
  MarketDepthInfo,
  PLEXDashboard,
  PLEXGlobalPrice,
  PLEXIndicators,
} from "../../lib/types";
export function SignalCard({ signal, indicators }: { signal: PLEXDashboard["signal"]; indicators: PLEXIndicators | null | undefined }) {
  const { t } = useI18n();
  const colorMap = { BUY: "text-eve-success", SELL: "text-eve-error", HOLD: "text-eve-warning" };
  const bgMap = { BUY: "bg-eve-success/10 border-eve-success/30", SELL: "bg-eve-error/10 border-eve-error/30", HOLD: "bg-eve-warning/10 border-eve-warning/30" };

  return (
    <div className={`border rounded-sm p-4 flex flex-col gap-2 ${bgMap[signal.action]}`}>
      <div className="flex items-center justify-between">
        <span className="text-xs text-eve-dim uppercase tracking-wider font-medium">{t("plexSignal")}</span>
        {indicators?.ccp_sale_signal && (
          <span className="px-2 py-0.5 text-[10px] font-bold uppercase bg-eve-success/20 text-eve-success border border-eve-success/40 rounded-sm animate-pulse">
            CCP SALE
          </span>
        )}
      </div>
      <div className={`text-3xl font-bold tracking-wider ${colorMap[signal.action]}`}>
        {signal.action}
      </div>
      <div className="flex items-center gap-2">
        <div className="flex-1 h-1.5 bg-eve-dark rounded-full overflow-hidden">
          <div
            className={`h-full rounded-full transition-all ${signal.action === "BUY" ? "bg-eve-success" : signal.action === "SELL" ? "bg-eve-error" : "bg-eve-warning"}`}
            style={{ width: `${signal.confidence}%` }}
          />
        </div>
        <span className="text-xs text-eve-dim">{signal.confidence.toFixed(0)}%</span>
      </div>
      <div className="flex flex-col gap-0.5 mt-1">
        {signal.reasons.map((r, i) => (
          <span key={i} className="text-[11px] text-eve-dim leading-tight">• {r}</span>
        ))}
      </div>
    </div>
  );
}

export function GlobalPriceCard({ price, indicators: ind }: { price: PLEXGlobalPrice; indicators: PLEXIndicators | null | undefined }) {
  const { t } = useI18n();
  const hasData = price.buy_price > 0 || price.sell_price > 0;

  // Percentile color: green if <30 (cheap), red if >70 (expensive)
  const pctColor = price.percentile_90d < 30 ? "text-eve-success" : price.percentile_90d > 70 ? "text-eve-error" : "text-eve-text";

  // Volatility regime color
  const volColor = ind?.vol_regime === "low" ? "text-eve-success" : ind?.vol_regime === "high" ? "text-eve-error" : "text-eve-warning";
  const volLabel = ind?.vol_regime === "low" ? t("plexVolLow") : ind?.vol_regime === "high" ? t("plexVolHigh") : t("plexVolMedium");

  return (
    <div className="bg-eve-dark border border-eve-accent/30 rounded-sm p-3">
      <h3 className="text-xs font-semibold text-eve-dim uppercase tracking-wider mb-3">{t("plexGlobalPrice")}</h3>
      {hasData ? (
        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-4">
          <div>
            <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-0.5">{t("plexBestBuy")}</div>
            <div className="text-lg font-mono font-bold text-eve-success">{formatISK(price.buy_price)}</div>
            <div className="text-[10px] text-eve-dim">{price.buy_orders} {t("plexOrders")}</div>
          </div>
          <div>
            <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-0.5">{t("plexBestSell")}</div>
            <div className="text-lg font-mono font-bold text-eve-error">{formatISK(price.sell_price)}</div>
            <div className="text-[10px] text-eve-dim">{price.sell_orders} {t("plexOrders")}</div>
          </div>
          <div>
            <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-0.5">{t("plexSpread")}</div>
            <div className="text-lg font-mono font-bold text-eve-text">{formatISK(price.spread)}</div>
            <div className="text-[10px] text-eve-dim">{price.spread_pct.toFixed(2)}%</div>
          </div>
          <div>
            <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-0.5">{t("plexVolume24h")}</div>
            <div className="text-lg font-mono font-bold text-eve-text">{price.volume_24h.toLocaleString()}</div>
          </div>
          {/* 90d Percentile */}
          {price.percentile_90d > 0 && (
            <div title={t("plexPercentileHint").replace("{pct}", (100 - price.percentile_90d).toFixed(0))}>
              <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-0.5">{t("plexPercentile")}</div>
              <div className={`text-lg font-mono font-bold ${pctColor}`}>{price.percentile_90d.toFixed(0)}th</div>
            </div>
          )}
          {/* Volatility */}
          {ind && ind.volatility_20d > 0 && (
            <div>
              <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-0.5">{t("plexVolatility")}</div>
              <div className={`text-lg font-mono font-bold ${volColor}`}>{(ind.volatility_20d * 100).toFixed(1)}%</div>
              <div className={`text-[10px] ${volColor}`}>{volLabel}</div>
            </div>
          )}
        </div>
      ) : (
        <div className="text-sm text-eve-dim">{t("plexNoData")}</div>
      )}

      {/* Technical Indicators (inline) */}
      {ind && (
        <>
          <div className="border-t border-eve-border/30 my-3" />
          <h4 className="text-[10px] font-semibold text-eve-dim uppercase tracking-wider mb-2">{t("plexIndicators")}</h4>
          <div className="grid grid-cols-3 sm:grid-cols-4 lg:grid-cols-8 gap-2">
            <MetricCell label="SMA(7)" value={formatISK(ind.sma7)} />
            <MetricCell label="SMA(30)" value={formatISK(ind.sma30)} />
            <MetricCell label="RSI(14)" value={ind.rsi.toFixed(1)} color={ind.rsi < 30 ? "text-eve-success" : ind.rsi > 70 ? "text-eve-error" : "text-eve-text"} />
            <MetricCell label="BB Upper" value={formatISK(ind.bollinger_upper)} />
            <MetricCell label="BB Lower" value={formatISK(ind.bollinger_lower)} />
            <MetricCell label="1d" value={`${ind.change_24h >= 0 ? "+" : ""}${ind.change_24h.toFixed(2)}%`} color={ind.change_24h >= 0 ? "text-eve-success" : "text-eve-error"} />
            <MetricCell label="7d" value={`${ind.change_7d >= 0 ? "+" : ""}${ind.change_7d.toFixed(2)}%`} color={ind.change_7d >= 0 ? "text-eve-success" : "text-eve-error"} />
            <MetricCell label="30d" value={`${ind.change_30d >= 0 ? "+" : ""}${ind.change_30d.toFixed(2)}%`} color={ind.change_30d >= 0 ? "text-eve-success" : "text-eve-error"} />
          </div>
        </>
      )}
    </div>
  );
}

export function ArbitrageRow({ arb, onClick }: { arb: ArbitragePath; onClick: () => void }) {
  const { t } = useI18n();
  // Break-even color: green if current PLEX price is below break-even (profitable zone)
  const beColor = arb.break_even_plex > 0 ? "text-eve-dim" : "";

  return (
    <tr className={`border-b border-eve-border/50 hover:bg-eve-panel/50 transition-colors cursor-pointer ${arb.viable ? "" : arb.no_data ? "opacity-40" : "opacity-50"}`} onClick={onClick}>
      <td className="py-1.5 px-2">
        <div className="flex flex-col gap-0">
          <div className="flex items-center gap-1.5">
            <span className={`w-1.5 h-1.5 rounded-full shrink-0 ${arb.no_data ? "bg-eve-warning" : arb.viable ? "bg-eve-success" : "bg-eve-error"}`} />
            <span className="text-eve-text hover:text-eve-accent transition-colors">{arb.name}</span>
            {arb.no_data && <span className="text-[9px] text-eve-warning uppercase tracking-wider">no data</span>}
          </div>
          {!arb.no_data && arb.break_even_plex > 0 && (
            <span className={`text-[9px] ${beColor} ml-3`}>BE: {formatISK(arb.break_even_plex)}/PLEX</span>
          )}
        </div>
      </td>
      <td className="py-1.5 px-2 text-right font-mono text-eve-dim">{arb.plex_cost > 0 ? arb.plex_cost : "—"}</td>
      <td className="py-1.5 px-2 text-right font-mono text-eve-text">{arb.no_data ? "—" : formatISK(arb.cost_isk)}</td>
      <td className="py-1.5 px-2 text-right font-mono text-eve-text">{arb.no_data ? "—" : formatISK(arb.revenue_isk)}</td>
      <td className={`py-1.5 px-2 text-right font-mono font-semibold ${arb.no_data ? "text-eve-dim" : arb.profit_isk >= 0 ? "text-eve-success" : "text-eve-error"}`}>
        <div>{arb.no_data ? "—" : `${arb.profit_isk >= 0 ? "+" : ""}${formatISK(arb.profit_isk)}`}</div>
        {!arb.no_data && arb.slippage_pct !== 0 && (
          <div className="text-[9px] text-eve-warning">{arb.slippage_pct.toFixed(2)}% slip</div>
        )}
      </td>
      <td className={`py-1.5 px-2 text-right font-mono font-semibold ${arb.no_data ? "text-eve-dim" : arb.roi >= 0 ? "text-eve-success" : "text-eve-error"}`}>
        <div>{arb.no_data ? "—" : `${arb.roi >= 0 ? "+" : ""}${arb.roi.toFixed(1)}%`}</div>
        {!arb.no_data && arb.isk_per_hour > 0 && (
          <div className="text-[9px] text-eve-dim">{formatISK(arb.isk_per_hour)}/{t("plexISKPerHour").split("/")[1] || "hr"}</div>
        )}
        {!arb.no_data && arb.est_minutes === 0 && arb.type === "spread" && (
          <div className="text-[9px] text-eve-dim italic">{t("plexPassive")}</div>
        )}
      </td>
    </tr>
  );
}

export function SPFarmCard({ farm }: { farm: PLEXDashboard["sp_farm"] }) {
  const { t } = useI18n();
  const [numChars, setNumChars] = useState(1);
  const [sellMode, setSellMode] = useState<"order" | "instant">("order");

  // Per-char profit based on sell mode (fallback to 0 for fields that may be missing from old cached responses)
  const perCharProfit = sellMode === "instant" ? (farm.instant_sell_profit_isk ?? 0) : farm.profit_isk;
  const perCharROI = sellMode === "instant" ? (farm.instant_sell_roi ?? 0) : farm.roi;
  const perCharRevenue = sellMode === "instant" ? (farm.instant_sell_revenue_isk ?? 0) : farm.revenue_isk;
  const isViable = perCharProfit > 0;

  // Multi-char scaling (same account): 1st char uses Omega + extractors,
  // additional chars need MPTC + extractors only (Omega is shared per account)
  const mptcCost = farm.mptc_cost_isk ?? 0;
  const extractorCostPerChar = farm.total_cost_isk - farm.omega_cost_isk; // just extractor cost
  const totalMonthlyCost = farm.total_cost_isk + (numChars > 1 ? (numChars - 1) * (extractorCostPerChar + mptcCost) : 0);
  const totalMonthlyRevenue = numChars * perCharRevenue;
  const totalMonthlyProfit = totalMonthlyRevenue - totalMonthlyCost;

  return (
    <div className={`border rounded-sm p-3 ${isViable ? "border-eve-success/30 bg-eve-success/5" : "border-eve-error/30 bg-eve-error/5"}`}>
      <h3 className="text-xs font-semibold text-eve-dim uppercase tracking-wider mb-2">{t("plexSPFarm")}</h3>
      <div className="space-y-1 text-xs">
        <Row label={t("plexOmegaCost")} value={`${farm.omega_cost_plex} PLEX = ${formatISK(farm.omega_cost_isk)}`} />
        <Row label={t("plexExtractors")} value={`${farm.extractors_per_month.toFixed(1)}x @ ${farm.extractor_cost_plex} PLEX`} />
        <Row label={t("plexTotalCost")} value={formatISK(farm.total_cost_isk)} dim />
        <div className="border-t border-eve-border/50 my-1.5" />
        <Row label={t("plexInjectors")} value={`${farm.injectors_produced.toFixed(1)}x @ ${formatISK(farm.injector_sell_price)}`} />
        <Row label={t("plexRevenue")} value={formatISK(perCharRevenue)} dim />

        {/* Sell mode toggle */}
        <div className="flex items-center gap-2 mt-1">
          <button
            onClick={() => setSellMode("order")}
            className={`px-2 py-0.5 rounded-sm text-[10px] font-semibold uppercase tracking-wider border transition-all ${sellMode === "order" ? "border-eve-accent/50 bg-eve-accent/10 text-eve-accent" : "border-eve-border bg-eve-panel text-eve-dim hover:text-eve-text"}`}
          >
            {t("plexSellOrder")}
          </button>
          <button
            onClick={() => setSellMode("instant")}
            className={`px-2 py-0.5 rounded-sm text-[10px] font-semibold uppercase tracking-wider border transition-all ${sellMode === "instant" ? "border-eve-warning/50 bg-eve-warning/10 text-eve-warning" : "border-eve-border bg-eve-panel text-eve-dim hover:text-eve-text"}`}
          >
            {t("plexInstantSell")}
          </button>
          {sellMode === "instant" && (
            <span className="text-[9px] text-eve-dim italic">{t("plexInstantSellNote")}</span>
          )}
        </div>

        <div className="border-t border-eve-border/50 my-1.5" />
        <div className="flex justify-between items-center">
          <span className="font-semibold text-eve-text">{t("plexNetProfit")}</span>
          <span className={`font-mono font-bold text-sm ${isViable ? "text-eve-success" : "text-eve-error"}`}>
            {perCharProfit >= 0 ? "+" : ""}{formatISK(perCharProfit)}/mo
          </span>
        </div>
        <div className="flex justify-between items-center">
          <span className="text-eve-dim">{t("plexPerDay")}</span>
          <span className={`font-mono ${isViable ? "text-eve-success" : "text-eve-error"}`}>
            {formatISK(perCharProfit / 30)}/day
          </span>
        </div>
        <div className="flex justify-between items-center">
          <span className="text-eve-dim">ROI</span>
          <span className={`font-mono font-semibold ${perCharROI > 0 ? "text-eve-success" : "text-eve-error"}`}>
            {perCharROI > 0 ? "+" : ""}{perCharROI.toFixed(1)}%
          </span>
        </div>

        {/* +5 implants (respects sell mode) */}
        <div className="border-t border-eve-border/30 my-1.5" />
        {(() => {
          const plus5Profit = sellMode === "instant" ? (farm.instant_sell_profit_plus5 ?? farm.profit_plus5) : farm.profit_plus5;
          const plus5ROI = sellMode === "instant" ? (farm.instant_sell_roi_plus5 ?? farm.roi_plus5) : farm.roi_plus5;
          return (
            <div className="text-[11px] text-eve-dim">
              {t("plexWithImplants")}:
              <span className={`ml-1 font-mono ${plus5Profit > 0 ? "text-eve-success" : "text-eve-error"}`}>
                {plus5Profit >= 0 ? "+" : ""}{formatISK(plus5Profit)}/mo
              </span>
              <span className="mx-1">|</span>
              <span className={`font-mono ${plus5ROI > 0 ? "text-eve-success" : "text-eve-error"}`}>
                {plus5ROI > 0 ? "+" : ""}{plus5ROI.toFixed(1)}%
              </span>
            </div>
          );
        })()}

        {/* Startup cost & payback */}
        {(farm.startup_train_days ?? 0) > 0 && (
          <>
            <div className="border-t border-eve-border/30 my-1.5" />
            <div className="text-[10px] text-eve-dim uppercase tracking-wider font-medium mb-1">{t("plexStartupCost")}</div>
            <Row label={t("plexStartupTrainDays")} value={`~${Math.ceil(farm.startup_train_days)} ${t("plexDays")} (~${(farm.startup_train_days / 30).toFixed(1)} ${t("plexMonths")})`} />
            <Row label={t("plexStartupCost")} value={formatISK(farm.startup_cost_isk ?? 0)} />
            {(farm.payback_days ?? 0) > 0 && (
              <Row label={t("plexPaybackPeriod")} value={`~${Math.ceil(farm.payback_days)} ${t("plexDays")} (~${(farm.payback_days / 30).toFixed(1)} ${t("plexMonths")})`} />
            )}
          </>
        )}

        {/* Multi-character scaling */}
        <div className="border-t border-eve-border/30 my-1.5" />
        <div className="text-[10px] text-eve-dim uppercase tracking-wider font-medium mb-1">{t("plexMultiChar")}</div>
        <div className="flex items-center gap-2">
          <label className="text-eve-dim text-[11px]">{t("plexNumChars")}</label>
          <input
            type="number"
            min="1"
            max="50"
            value={numChars}
            onChange={(e) => setNumChars(Math.max(1, parseInt(e.target.value) || 1))}
            className="w-14 px-1.5 py-0.5 bg-eve-input border border-eve-border rounded-sm text-xs text-eve-text font-mono text-center"
          />
          {numChars > 1 && (
            <span className="text-[10px] text-eve-dim">
              ({t("plexMPTCperChar")}: {farm.mptc_cost_plex ?? 0} PLEX = {formatISK(mptcCost)})
            </span>
          )}
        </div>
        {numChars > 1 && (
          <div className="flex justify-between items-center mt-1">
            <span className="font-semibold text-eve-text text-[11px]">{t("plexTotalMonthlyProfit")} ({numChars}x)</span>
            <span className={`font-mono font-bold ${totalMonthlyProfit > 0 ? "text-eve-success" : "text-eve-error"}`}>
              {totalMonthlyProfit >= 0 ? "+" : ""}{formatISK(totalMonthlyProfit)}/mo
            </span>
          </div>
        )}

        {/* Break-even PLEX price */}
        {(farm.break_even_plex ?? 0) > 0 && (
          <>
            <div className="border-t border-eve-border/30 my-1.5" />
            <div className="flex justify-between items-center">
              <span className="text-eve-dim text-[11px]">{t("plexBreakEven")}</span>
              <span className={`font-mono text-[11px] font-semibold ${(farm.plex_unit_price ?? 0) < farm.break_even_plex ? "text-eve-success" : "text-eve-error"}`}>
                {formatISK(farm.break_even_plex)}/PLEX
              </span>
            </div>
          </>
        )}

        {/* Omega ISK equivalent */}
        {(farm.omega_isk_value ?? 0) > 0 && (
          <>
            <div className="border-t border-eve-border/30 my-1.5" />
            <div className="text-[11px] text-eve-dim">
              {t("plexOmegaISKValue").replace("{plex}", String(farm.omega_cost_plex)).replace("{isk}", formatISK(farm.omega_isk_value))}
              {(farm.plex_unit_price ?? 0) > 0 && (
                <span className="ml-1">({formatISK(farm.plex_unit_price)} {t("plexPerPLEX")})</span>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  );
}

function Row({ label, value, dim }: { label: string; value: string; dim?: boolean }) {
  return (
    <div className="flex justify-between">
      <span className="text-eve-dim">{label}</span>
      <span className={`font-mono ${dim ? "text-eve-dim" : "text-eve-text"}`}>{value}</span>
    </div>
  );
}

function MetricCell({ label, value, color }: { label: string; value: string; color?: string }) {
  return (
    <div className="text-center">
      <div className="text-[10px] text-eve-dim uppercase tracking-wider">{label}</div>
      <div className={`text-sm font-mono font-semibold ${color || "text-eve-text"}`}>{value}</div>
    </div>
  );
}

// ===================================================================
// Historical Arbitrage Profitability Chart
// ===================================================================


export function MarketDepthCard({ depth }: { depth: MarketDepthInfo }) {
  const { t } = useI18n();
  const fmtHrs = (h: number) => h > 0 ? `~${h < 1 ? "<1" : h.toFixed(1)} ${t("plexHours")}` : "";
  return (
    <div className="bg-eve-dark border border-eve-border rounded-sm p-3">
      <h3 className="text-xs font-semibold text-eve-dim uppercase tracking-wider mb-1">{t("plexMarketDepth")}</h3>
      <p className="text-[10px] text-eve-dim mb-2">{t("plexMarketDepthHint")}</p>
      <div className="space-y-2 text-xs">
        {/* PLEX sell depth */}
        <div className="border border-eve-border/50 rounded-sm p-2">
          <div className="text-[10px] text-eve-dim uppercase tracking-wider font-medium mb-1">{t("plexDepthPLEX")} {t("plexDepthSellOrders")}</div>
          <div className="flex justify-between">
            <span className="text-eve-dim">{t("plexDepthVolume")}</span>
            <span className="font-mono text-eve-text">{depth.plex_sell_depth_5.total_volume.toLocaleString()}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-eve-dim">{t("plexDepthLevels")}</span>
            <span className="font-mono text-eve-text">{depth.plex_sell_depth_5.levels}</span>
          </div>
          {depth.plex_sell_depth_5.best_price > 0 && (
            <div className="flex justify-between">
              <span className="text-eve-dim">Best → Worst</span>
              <span className="font-mono text-eve-text text-[11px]">{formatISK(depth.plex_sell_depth_5.best_price)} → {formatISK(depth.plex_sell_depth_5.worst_price)}</span>
            </div>
          )}
          {depth.plex_fill_hours > 0 && (
            <div className="flex justify-between">
              <span className="text-eve-dim">{t("plexEstFillTime")} (100x)</span>
              <span className="font-mono text-eve-dim">{fmtHrs(depth.plex_fill_hours)}</span>
            </div>
          )}
        </div>

        {/* Item depth grid */}
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-1.5">
          <DepthItem label={t("plexDepthExtractor")} sell={depth.extractor_sell_qty} buy={depth.extractor_buy_qty} fillHours={depth.extractor_fill_hours} />
          <DepthItem label={t("plexDepthInjector")} sell={depth.injector_sell_qty} buy={depth.injector_buy_qty} fillHours={depth.injector_fill_hours} />
          <DepthItem label={t("plexDepthMPTC")} sell={depth.mptc_sell_qty} buy={depth.mptc_buy_qty} fillHours={depth.mptc_fill_hours} />
        </div>
      </div>
    </div>
  );
}

function DepthItem({ label, sell, buy, fillHours }: { label: string; sell: number; buy: number; fillHours?: number }) {
  const { t } = useI18n();
  return (
    <div className="border border-eve-border/30 rounded-sm p-1.5 text-center">
      <div className="text-[10px] text-eve-dim uppercase tracking-wider font-medium mb-1">{label}</div>
      <div className="text-[10px]">
        <span className="text-eve-error">{t("plexDepthSellOrders").charAt(0)}: </span>
        <span className="font-mono text-eve-text">{sell.toLocaleString()}</span>
      </div>
      <div className="text-[10px]">
        <span className="text-eve-success">{t("plexDepthBuyOrders").charAt(0)}: </span>
        <span className="font-mono text-eve-text">{buy.toLocaleString()}</span>
      </div>
      {fillHours != null && fillHours > 0 && (
        <div className="text-[9px] text-eve-dim mt-0.5">
          ~{fillHours < 1 ? "<1" : fillHours.toFixed(1)} {t("plexHours")}
        </div>
      )}
    </div>
  );
}

// ===================================================================
// Injection Tiers Card
// ===================================================================

export function InjectionTiersCard({ tiers }: { tiers: InjectionTier[] }) {
  const { t } = useI18n();
  return (
    <div className="bg-eve-dark border border-eve-border rounded-sm p-3">
      <h3 className="text-xs font-semibold text-eve-dim uppercase tracking-wider mb-1">{t("plexInjectionTiers")}</h3>
      <p className="text-[10px] text-eve-dim mb-2">{t("plexInjectionTiersHint")}</p>
      <table className="w-full text-xs">
        <thead>
          <tr className="text-eve-dim border-b border-eve-border">
            <th className="text-left py-1 px-2 font-medium">{t("plexTierLabel")}</th>
            <th className="text-right py-1 px-2 font-medium">{t("plexSPReceived")}</th>
            <th className="text-right py-1 px-2 font-medium">{t("plexISKPerSP")}</th>
            <th className="text-right py-1 px-2 font-medium">{t("plexEfficiency")}</th>
          </tr>
        </thead>
        <tbody>
          {tiers.map((tier, i) => {
            const effColor = tier.efficiency >= 80 ? "text-eve-success" : tier.efficiency >= 50 ? "text-eve-warning" : "text-eve-error";
            return (
              <tr key={i} className="border-b border-eve-border/30">
                <td className="py-1 px-2 text-eve-text">{tier.label}</td>
                <td className="py-1 px-2 text-right font-mono text-eve-text">{tier.sp_received.toLocaleString()}</td>
                <td className="py-1 px-2 text-right font-mono text-eve-text">{formatISK(tier.isk_per_sp)}</td>
                <td className={`py-1 px-2 text-right font-mono font-semibold ${effColor}`}>{tier.efficiency.toFixed(0)}%</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
