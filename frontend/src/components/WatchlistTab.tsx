import { useCallback, useEffect, useState } from "react";
import type { FlipResult, WatchlistItem } from "@/lib/types";
import { getWatchlist, removeFromWatchlist, updateWatchlistItem } from "@/lib/api";
import { formatISK, formatMargin } from "@/lib/format";
import { useI18n } from "@/lib/i18n";

interface Props {
  /** Latest scan results (from radius or region tab) to cross-reference prices */
  latestResults: FlipResult[];
}

export function WatchlistTab({ latestResults }: Props) {
  const { t } = useI18n();
  const [items, setItems] = useState<WatchlistItem[]>([]);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [editValue, setEditValue] = useState("");

  const reload = useCallback(() => {
    getWatchlist().then(setItems).catch(() => {});
  }, []);

  useEffect(() => {
    reload();
  }, [reload]);

  const handleRemove = (typeId: number) => {
    removeFromWatchlist(typeId).then(setItems).catch(() => {});
  };

  const handleSaveThreshold = (typeId: number) => {
    const val = parseFloat(editValue);
    if (!isNaN(val) && val >= 0) {
      updateWatchlistItem(typeId, val).then(setItems).catch(() => {});
    }
    setEditingId(null);
  };

  // Cross-reference watchlist with latest results
  const enriched = items.map((item) => {
    const match = latestResults.find((r) => r.TypeID === item.type_id);
    return { ...item, match };
  });

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center gap-3 px-3 py-2 border-b border-eve-border">
        <span className="text-[10px] uppercase tracking-wider text-eve-dim font-medium">
          ⭐ {t("tabWatchlist")} ({items.length})
        </span>
        <div className="flex-1" />
        <button
          onClick={reload}
          className="px-3 py-1 rounded-sm text-xs text-eve-dim hover:text-eve-accent border border-eve-border hover:border-eve-accent/30 transition-colors cursor-pointer"
        >
          ↻
        </button>
      </div>

      {/* Table */}
      <div className="flex-1 min-h-0 overflow-auto">
        {items.length === 0 ? (
          <div className="flex items-center justify-center h-full text-eve-dim text-xs">
            {t("watchlistEmpty")}
            <br />
            <span className="text-[10px] mt-1 block text-eve-dim/70">{t("watchlistHint")}</span>
          </div>
        ) : (
          <table className="w-full text-xs">
            <thead className="sticky top-0 bg-eve-panel z-10">
              <tr className="text-eve-dim text-[10px] uppercase tracking-wider border-b border-eve-border">
                <th className="px-3 py-2 text-left font-medium">{t("colItem")}</th>
                <th className="px-3 py-2 text-right font-medium">{t("watchlistThreshold")}</th>
                <th className="px-3 py-2 text-right font-medium">{t("watchlistCurrentMargin")}</th>
                <th className="px-3 py-2 text-right font-medium">{t("watchlistCurrentProfit")}</th>
                <th className="px-3 py-2 text-right font-medium">{t("watchlistBuyAt")}</th>
                <th className="px-3 py-2 text-right font-medium">{t("watchlistSellAt")}</th>
                <th className="px-3 py-2 text-center font-medium">{t("watchlistAdded")}</th>
                <th className="px-3 py-2 w-10" />
              </tr>
            </thead>
            <tbody>
              {enriched.map((item, i) => {
                const isAlert =
                  item.alert_min_margin > 0 &&
                  item.match &&
                  item.match.MarginPercent > item.alert_min_margin;

                return (
                  <tr
                    key={item.type_id}
                    className={`border-b border-eve-border/30 transition-colors ${
                      isAlert
                        ? "bg-green-900/20 hover:bg-green-900/30"
                        : i % 2 === 0
                          ? "bg-eve-panel hover:bg-eve-accent/5"
                          : "bg-[#161616] hover:bg-eve-accent/5"
                    }`}
                  >
                    {/* Item name */}
                    <td className="px-3 py-2 text-eve-text font-medium">
                      {isAlert && <span className="mr-1">🔔</span>}
                      {item.type_name}
                    </td>

                    {/* Alert threshold */}
                    <td className="px-3 py-2 text-right">
                      {editingId === item.type_id ? (
                        <input
                          autoFocus
                          type="number"
                          value={editValue}
                          onChange={(e) => setEditValue(e.target.value)}
                          onBlur={() => handleSaveThreshold(item.type_id)}
                          onKeyDown={(e) => {
                            if (e.key === "Enter") handleSaveThreshold(item.type_id);
                            if (e.key === "Escape") setEditingId(null);
                          }}
                          className="w-16 px-1 py-0.5 bg-eve-input border border-eve-accent/50 rounded-sm text-eve-text text-xs font-mono text-right
                                     focus:outline-none [appearance:textfield] [&::-webkit-outer-spin-button]:appearance-none [&::-webkit-inner-spin-button]:appearance-none"
                        />
                      ) : (
                        <span
                          onClick={() => {
                            setEditingId(item.type_id);
                            setEditValue(String(item.alert_min_margin));
                          }}
                          className="font-mono text-eve-dim cursor-pointer hover:text-eve-accent transition-colors"
                          title={t("watchlistClickToEdit")}
                        >
                          {item.alert_min_margin > 0
                            ? `${item.alert_min_margin}%`
                            : "—"}
                        </span>
                      )}
                    </td>

                    {/* Current margin */}
                    <td className="px-3 py-2 text-right font-mono">
                      {item.match ? (
                        <span className={item.match.MarginPercent > 10 ? "text-green-400" : "text-eve-accent"}>
                          {formatMargin(item.match.MarginPercent)}
                        </span>
                      ) : (
                        <span className="text-eve-dim">—</span>
                      )}
                    </td>

                    {/* Current profit */}
                    <td className="px-3 py-2 text-right font-mono">
                      {item.match ? (
                        <span className="text-green-400">{formatISK(item.match.TotalProfit)}</span>
                      ) : (
                        <span className="text-eve-dim">—</span>
                      )}
                    </td>

                    {/* Buy at */}
                    <td className="px-3 py-2 text-right font-mono text-eve-text">
                      {item.match ? formatISK(item.match.BuyPrice) : "—"}
                    </td>

                    {/* Sell at */}
                    <td className="px-3 py-2 text-right font-mono text-eve-text">
                      {item.match ? formatISK(item.match.SellPrice) : "—"}
                    </td>

                    {/* Added date */}
                    <td className="px-3 py-2 text-center text-eve-dim">
                      {new Date(item.added_at).toLocaleDateString()}
                    </td>

                    {/* Delete */}
                    <td className="px-3 py-2 text-center">
                      <button
                        onClick={() => handleRemove(item.type_id)}
                        className="text-eve-dim hover:text-eve-error transition-colors cursor-pointer text-sm"
                        title={t("removeFromWatchlist")}
                      >
                        ✕
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>

      {/* Summary */}
      {enriched.some((e) => e.match) && (
        <div className="shrink-0 flex items-center gap-6 px-3 py-1.5 border-t border-eve-border text-xs">
          <span className="text-eve-dim">
            {t("watchlistTracked")}: <span className="text-eve-accent font-mono">{enriched.filter((e) => e.match).length}/{items.length}</span>
          </span>
          <span className="text-eve-dim">
            {t("watchlistAlerts")}: <span className="text-green-400 font-mono">{enriched.filter((e) => e.alert_min_margin > 0 && e.match && e.match.MarginPercent > e.alert_min_margin).length}</span>
          </span>
        </div>
      )}
    </div>
  );
}
