import { useCallback, useEffect, useMemo, useState } from "react";
import type { FlipResult, WatchlistItem } from "@/lib/types";
import {
  getWatchlist,
  removeFromWatchlist,
  updateWatchlistItem,
  addToWatchlist,
} from "@/lib/api";
import { formatISK, formatMargin } from "@/lib/format";
import { useI18n, type TranslationKey } from "@/lib/i18n";
import { useGlobalToast } from "./Toast";
import { ConfirmDialog } from "./ConfirmDialog";

interface Props {
  /** Latest scan results (from radius or region tab) to cross-reference prices */
  latestResults: FlipResult[];
}

type SortKey =
  | "type_name"
  | "alert_min_margin"
  | "margin"
  | "profit"
  | "buy"
  | "sell"
  | "added_at";
type SortDir = "asc" | "desc";

export function WatchlistTab({ latestResults }: Props) {
  const { t } = useI18n();
  const { addToast } = useGlobalToast();
  const [items, setItems] = useState<WatchlistItem[]>([]);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [editValue, setEditValue] = useState("");
  const [search, setSearch] = useState("");
  const [sortKey, setSortKey] = useState<SortKey>("added_at");
  const [sortDir, setSortDir] = useState<SortDir>("desc");
  const [confirmDelete, setConfirmDelete] = useState<{
    id: number;
    name: string;
  } | null>(null);

  const reload = useCallback(() => {
    getWatchlist()
      .then(setItems)
      .catch(() =>
        addToast(
          t("watchlistError" as TranslationKey) || "Failed to load watchlist",
          "error",
          3000,
        ),
      );
  }, [addToast, t]);

  useEffect(() => {
    reload();
  }, [reload]);

  const handleRemove = (typeId: number) => {
    removeFromWatchlist(typeId)
      .then((list) => {
        setItems(list);
        addToast(
          t("watchlistRemoved" as TranslationKey) || "Removed from watchlist",
          "success",
          2000,
        );
      })
      .catch(() =>
        addToast(
          t("watchlistError" as TranslationKey) || "Operation failed",
          "error",
          3000,
        ),
      );
  };

  const handleSaveThreshold = (typeId: number) => {
    const val = parseFloat(editValue);
    if (!isNaN(val) && val >= 0) {
      updateWatchlistItem(typeId, val)
        .then((list) => {
          setItems(list);
          addToast(
            t("watchlistThresholdSaved" as TranslationKey) ||
              "Threshold updated",
            "success",
            2000,
          );
        })
        .catch(() =>
          addToast(
            t("watchlistError" as TranslationKey) || "Operation failed",
            "error",
            3000,
          ),
        );
    }
    setEditingId(null);
  };

  // Cross-reference with latest scan results
  const enriched = useMemo(
    () =>
      items.map((item) => {
        const match = latestResults.find((r) => r.TypeID === item.type_id);
        return { ...item, match };
      }),
    [items, latestResults],
  );

  // Filter + sort
  const displayed = useMemo(() => {
    let list = enriched;

    // Search filter
    if (search.trim()) {
      const q = search.toLowerCase();
      list = list.filter((item) => item.type_name.toLowerCase().includes(q));
    }

    // Sort
    list = [...list].sort((a, b) => {
      let cmp = 0;
      switch (sortKey) {
        case "type_name":
          cmp = a.type_name.localeCompare(b.type_name);
          break;
        case "alert_min_margin":
          cmp = a.alert_min_margin - b.alert_min_margin;
          break;
        case "margin":
          cmp =
            (a.match?.MarginPercent ?? -1) - (b.match?.MarginPercent ?? -1);
          break;
        case "profit":
          cmp = (a.match?.TotalProfit ?? -1) - (b.match?.TotalProfit ?? -1);
          break;
        case "buy":
          cmp = (a.match?.BuyPrice ?? -1) - (b.match?.BuyPrice ?? -1);
          break;
        case "sell":
          cmp = (a.match?.SellPrice ?? -1) - (b.match?.SellPrice ?? -1);
          break;
        case "added_at":
          cmp =
            new Date(a.added_at).getTime() - new Date(b.added_at).getTime();
          break;
      }
      return sortDir === "asc" ? cmp : -cmp;
    });

    return list;
  }, [enriched, search, sortKey, sortDir]);

  const toggleSort = (key: SortKey) => {
    if (sortKey === key) {
      setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    } else {
      setSortKey(key);
      setSortDir("desc");
    }
  };

  const sortIndicator = (key: SortKey) =>
    sortKey === key ? (sortDir === "asc" ? " ▲" : " ▼") : "";

  // Export watchlist to clipboard
  const handleExport = () => {
    const data = items.map((i) => ({
      type_id: i.type_id,
      type_name: i.type_name,
      alert_min_margin: i.alert_min_margin,
    }));
    navigator.clipboard.writeText(JSON.stringify(data, null, 2));
    addToast(
      t("watchlistExported" as TranslationKey) ||
        "Watchlist copied to clipboard",
      "success",
      2000,
    );
  };

  // Import watchlist from clipboard
  const handleImport = async () => {
    try {
      const json = await navigator.clipboard.readText();
      const parsed = JSON.parse(json);
      if (!Array.isArray(parsed)) throw new Error("not array");
      let imported = 0;
      for (const item of parsed) {
        if (!item.type_id || !item.type_name) continue;
        try {
          const r = await addToWatchlist(
            item.type_id,
            item.type_name,
            item.alert_min_margin ?? 0,
          );
          if (r.inserted) imported++;
        } catch {
          /* skip invalid */
        }
      }
      reload();
      addToast(
        `${t("watchlistImported" as TranslationKey) || "Imported"}: ${imported}`,
        "success",
        2000,
      );
    } catch {
      addToast("Invalid clipboard data", "error", 3000);
    }
  };

  const columns: {
    key: SortKey;
    label: string;
    align: string;
    width: string;
  }[] = [
    {
      key: "type_name",
      label: t("colItem"),
      align: "text-left",
      width: "min-w-[150px]",
    },
    {
      key: "alert_min_margin",
      label: t("watchlistThreshold"),
      align: "text-right",
      width: "min-w-[80px]",
    },
    {
      key: "margin",
      label: t("watchlistCurrentMargin"),
      align: "text-right",
      width: "min-w-[80px]",
    },
    {
      key: "profit",
      label: t("watchlistCurrentProfit"),
      align: "text-right",
      width: "min-w-[90px]",
    },
    {
      key: "buy",
      label: t("watchlistBuyAt"),
      align: "text-right",
      width: "min-w-[90px]",
    },
    {
      key: "sell",
      label: t("watchlistSellAt"),
      align: "text-right",
      width: "min-w-[90px]",
    },
    {
      key: "added_at",
      label: t("watchlistAdded"),
      align: "text-center",
      width: "min-w-[80px]",
    },
  ];

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center gap-3 px-3 py-2 border-b border-eve-border flex-wrap">
        <span className="text-[10px] uppercase tracking-wider text-eve-dim font-medium shrink-0">
          ⭐ {t("tabWatchlist")} ({items.length})
        </span>

        {/* Search */}
        {items.length > 0 && (
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={
              t("watchlistSearch" as TranslationKey) || "Search..."
            }
            className="px-2 py-1 bg-eve-input border border-eve-border rounded-sm text-eve-text text-xs w-36
                       focus:outline-none focus:border-eve-accent focus:ring-1 focus:ring-eve-accent/30 transition-colors"
          />
        )}

        <div className="flex-1" />

        {/* Actions */}
        <div className="flex items-center gap-1.5">
          {items.length > 0 && (
            <>
              <button
                onClick={handleExport}
                className="px-2 py-1 rounded-sm text-[11px] text-eve-dim hover:text-eve-text transition-colors"
                title={
                  t("watchlistExport" as TranslationKey) || "Export"
                }
              >
                {t("presetExport" as TranslationKey) || "Export"}
              </button>
              <span className="text-eve-border">|</span>
            </>
          )}
          <button
            onClick={handleImport}
            className="px-2 py-1 rounded-sm text-[11px] text-eve-dim hover:text-eve-text transition-colors"
            title={
              t("watchlistImport" as TranslationKey) || "Import"
            }
          >
            {t("presetImport" as TranslationKey) || "Import"}
          </button>
          <button
            onClick={reload}
            className="px-3 py-1 rounded-sm text-xs text-eve-dim hover:text-eve-accent border border-eve-border hover:border-eve-accent/30 transition-colors cursor-pointer"
          >
            ↻
          </button>
        </div>
      </div>

      {/* Table */}
      <div className="flex-1 min-h-0 overflow-auto">
        {items.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-full text-eve-dim text-xs">
            <span>{t("watchlistEmpty")}</span>
            <span className="text-[10px] mt-1 text-eve-dim/70">
              {t("watchlistHint")}
            </span>
          </div>
        ) : (
          <table className="w-full text-xs">
            <thead className="sticky top-0 bg-eve-panel z-10">
              <tr className="text-eve-dim text-[10px] uppercase tracking-wider border-b border-eve-border">
                {columns.map((col) => (
                  <th
                    key={col.key}
                    className={`px-3 py-2 font-medium cursor-pointer hover:text-eve-accent transition-colors select-none ${col.align} ${col.width}`}
                    onClick={() => toggleSort(col.key)}
                  >
                    {col.label}
                    {sortIndicator(col.key)}
                  </th>
                ))}
                <th className="px-3 py-2 w-10" />
              </tr>
            </thead>
            <tbody>
              {displayed.map((item, i) => {
                const isAlert =
                  item.alert_min_margin > 0 &&
                  item.match &&
                  item.match.MarginPercent >= item.alert_min_margin;

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
                          onBlur={() =>
                            handleSaveThreshold(item.type_id)
                          }
                          onKeyDown={(e) => {
                            if (e.key === "Enter")
                              handleSaveThreshold(item.type_id);
                            if (e.key === "Escape") setEditingId(null);
                          }}
                          className="w-16 px-1 py-0.5 bg-eve-input border border-eve-accent/50 rounded-sm text-eve-text text-xs font-mono text-right
                                     focus:outline-none [appearance:textfield] [&::-webkit-outer-spin-button]:appearance-none [&::-webkit-inner-spin-button]:appearance-none"
                        />
                      ) : (
                        <span
                          onClick={() => {
                            setEditingId(item.type_id);
                            setEditValue(
                              String(item.alert_min_margin),
                            );
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
                        <span
                          className={
                            item.match.MarginPercent > 10
                              ? "text-green-400"
                              : "text-eve-accent"
                          }
                        >
                          {formatMargin(item.match.MarginPercent)}
                        </span>
                      ) : (
                        <span className="text-eve-dim">—</span>
                      )}
                    </td>

                    {/* Current profit */}
                    <td className="px-3 py-2 text-right font-mono">
                      {item.match ? (
                        <span className="text-green-400">
                          {formatISK(item.match.TotalProfit)}
                        </span>
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
                      {item.match
                        ? formatISK(item.match.SellPrice)
                        : "—"}
                    </td>

                    {/* Added date */}
                    <td className="px-3 py-2 text-center text-eve-dim">
                      {new Date(item.added_at).toLocaleDateString()}
                    </td>

                    {/* Delete */}
                    <td className="px-3 py-2 text-center">
                      <button
                        onClick={() =>
                          setConfirmDelete({
                            id: item.type_id,
                            name: item.type_name,
                          })
                        }
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
            {t("watchlistTracked")}:{" "}
            <span className="text-eve-accent font-mono">
              {enriched.filter((e) => e.match).length}/{items.length}
            </span>
          </span>
          <span className="text-eve-dim">
            {t("watchlistAlerts")}:{" "}
            <span className="text-green-400 font-mono">
              {
                enriched.filter(
                  (e) =>
                    e.alert_min_margin > 0 &&
                    e.match &&
                    e.match.MarginPercent >= e.alert_min_margin,
                ).length
              }
            </span>
          </span>
        </div>
      )}

      {/* Confirm delete dialog */}
      {confirmDelete && (
        <ConfirmDialog
          open={true}
          title={t("removeFromWatchlist")}
          message={`${t("watchlistConfirmRemove" as TranslationKey) || "Remove"} "${confirmDelete.name}"?`}
          onConfirm={() => {
            handleRemove(confirmDelete.id);
            setConfirmDelete(null);
          }}
          onClose={() => setConfirmDelete(null)}
          variant="danger"
        />
      )}
    </div>
  );
}
