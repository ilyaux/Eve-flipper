import { useState, useMemo, useCallback, useEffect } from "react";
import type { FlipResult, WatchlistItem } from "@/lib/types";
import { formatISK, formatMargin } from "@/lib/format";
import { useI18n, type TranslationKey } from "@/lib/i18n";
import { getWatchlist, addToWatchlist, removeFromWatchlist } from "@/lib/api";

type SortKey = keyof FlipResult;
type SortDir = "asc" | "desc";

interface Props {
  results: FlipResult[];
  scanning: boolean;
  progress: string;
}

const columnDefs: { key: SortKey; labelKey: TranslationKey; width: string; numeric: boolean }[] = [
  { key: "TypeName", labelKey: "colItem", width: "min-w-[180px]", numeric: false },
  { key: "BuyPrice", labelKey: "colBuyPrice", width: "min-w-[110px]", numeric: true },
  { key: "BuyStation", labelKey: "colBuyStation", width: "min-w-[150px]", numeric: false },
  { key: "SellPrice", labelKey: "colSellPrice", width: "min-w-[110px]", numeric: true },
  { key: "SellStation", labelKey: "colSellStation", width: "min-w-[150px]", numeric: false },
  { key: "MarginPercent", labelKey: "colMargin", width: "min-w-[80px]", numeric: true },
  { key: "UnitsToBuy", labelKey: "colUnitsToBuy", width: "min-w-[80px]", numeric: true },
  { key: "BuyOrderRemain", labelKey: "colAcceptQty", width: "min-w-[80px]", numeric: true },
  { key: "TotalProfit", labelKey: "colProfit", width: "min-w-[120px]", numeric: true },
  { key: "ProfitPerJump", labelKey: "colProfitPerJump", width: "min-w-[110px]", numeric: true },
  { key: "TotalJumps", labelKey: "colJumps", width: "min-w-[60px]", numeric: true },
];

// Unique key for a row
function rowKey(row: FlipResult) {
  return `${row.TypeID}-${row.BuySystemID}-${row.SellSystemID}`;
}

export function ScanResultsTable({ results, scanning, progress }: Props) {
  const { t } = useI18n();

  // Sort
  const [sortKey, setSortKey] = useState<SortKey>("TotalProfit");
  const [sortDir, setSortDir] = useState<SortDir>("desc");

  // Filters
  const [filters, setFilters] = useState<Record<string, string>>({});
  const [showFilters, setShowFilters] = useState(false);

  // Selection & pinning
  const [selectedKeys, setSelectedKeys] = useState<Set<string>>(new Set());
  const [pinnedKeys, setPinnedKeys] = useState<Set<string>>(new Set());

  // Watchlist
  const [watchlist, setWatchlist] = useState<WatchlistItem[]>([]);
  useEffect(() => { getWatchlist().then(setWatchlist).catch(() => {}); }, []);
  const watchlistIds = useMemo(() => new Set(watchlist.map((w) => w.type_id)), [watchlist]);

  // Context menu
  const [contextMenu, setContextMenu] = useState<{ x: number; y: number; row: FlipResult } | null>(null);

  // Filter logic
  const filtered = useMemo(() => {
    if (Object.values(filters).every((v) => !v)) return results;
    return results.filter((row) => {
      for (const col of columnDefs) {
        const fval = filters[col.key];
        if (!fval) continue;
        const cellVal = row[col.key];
        if (col.numeric) {
          // Support range: "100-500", ">100", "<500", or plain number
          const num = cellVal as number;
          const trimmed = fval.trim();
          if (trimmed.includes("-") && !trimmed.startsWith("-")) {
            const [minS, maxS] = trimmed.split("-");
            const min = parseFloat(minS);
            const max = parseFloat(maxS);
            if (!isNaN(min) && !isNaN(max) && (num < min || num > max)) return false;
          } else if (trimmed.startsWith(">")) {
            const min = parseFloat(trimmed.slice(1));
            if (!isNaN(min) && num <= min) return false;
          } else if (trimmed.startsWith("<")) {
            const max = parseFloat(trimmed.slice(1));
            if (!isNaN(max) && num >= max) return false;
          } else {
            const target = parseFloat(trimmed);
            if (!isNaN(target) && !String(num).includes(trimmed)) return false;
          }
        } else {
          if (!String(cellVal).toLowerCase().includes(fval.toLowerCase())) return false;
        }
      }
      return true;
    });
  }, [results, filters]);

  // Sort with pinned on top
  const sorted = useMemo(() => {
    const copy = [...filtered];
    copy.sort((a, b) => {
      // Pinned always on top
      const aPinned = pinnedKeys.has(rowKey(a));
      const bPinned = pinnedKeys.has(rowKey(b));
      if (aPinned && !bPinned) return -1;
      if (!aPinned && bPinned) return 1;

      const av = a[sortKey];
      const bv = b[sortKey];
      if (typeof av === "number" && typeof bv === "number") {
        return sortDir === "asc" ? av - bv : bv - av;
      }
      return sortDir === "asc"
        ? String(av).localeCompare(String(bv))
        : String(bv).localeCompare(String(av));
    });
    return copy;
  }, [filtered, sortKey, sortDir, pinnedKeys]);

  // Summary stats
  const summary = useMemo(() => {
    const rows = selectedKeys.size > 0 ? sorted.filter((r) => selectedKeys.has(rowKey(r))) : sorted;
    if (rows.length === 0) return null;
    const totalProfit = rows.reduce((sum, r) => sum + r.TotalProfit, 0);
    const avgMargin = rows.reduce((sum, r) => sum + r.MarginPercent, 0) / rows.length;
    return { totalProfit, avgMargin, count: rows.length };
  }, [sorted, selectedKeys]);

  const toggleSort = (key: SortKey) => {
    if (sortKey === key) {
      setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    } else {
      setSortKey(key);
      setSortDir("desc");
    }
  };

  const setFilter = (key: string, value: string) => {
    setFilters((f) => ({ ...f, [key]: value }));
  };

  const clearFilters = () => {
    setFilters({});
  };

  const hasActiveFilters = Object.values(filters).some((v) => !!v);

  // Selection
  const toggleSelect = (key: string) => {
    setSelectedKeys((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  const toggleSelectAll = () => {
    if (selectedKeys.size === sorted.length) {
      setSelectedKeys(new Set());
    } else {
      setSelectedKeys(new Set(sorted.map(rowKey)));
    }
  };

  // Pinning
  const togglePin = (key: string) => {
    setPinnedKeys((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  // Context menu
  const handleContextMenu = useCallback(
    (e: React.MouseEvent, row: FlipResult) => {
      e.preventDefault();
      setContextMenu({ x: e.clientX, y: e.clientY, row });
    },
    []
  );

  const copyText = (text: string) => {
    navigator.clipboard.writeText(text);
    setContextMenu(null);
  };

  // Export CSV
  const exportCSV = () => {
    const rows = selectedKeys.size > 0 ? sorted.filter((r) => selectedKeys.has(rowKey(r))) : sorted;
    const header = columnDefs.map((c) => t(c.labelKey)).join(",");
    const csvRows = rows.map((row) =>
      columnDefs
        .map((col) => {
          const val = row[col.key];
          const str = String(val);
          return str.includes(",") ? `"${str}"` : str;
        })
        .join(",")
    );
    const csv = [header, ...csvRows].join("\n");
    const blob = new Blob(["\uFEFF" + csv], { type: "text/csv;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `eve-flipper-${new Date().toISOString().slice(0, 10)}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  };

  // Copy table to clipboard
  const copyTable = () => {
    const rows = selectedKeys.size > 0 ? sorted.filter((r) => selectedKeys.has(rowKey(r))) : sorted;
    const header = columnDefs.map((c) => t(c.labelKey)).join("\t");
    const tsvRows = rows.map((row) =>
      columnDefs.map((col) => formatCell(col, row)).join("\t")
    );
    navigator.clipboard.writeText([header, ...tsvRows].join("\n"));
  };

  const formatCell = (col: (typeof columnDefs)[number], row: FlipResult): string => {
    const val = row[col.key];
    if (col.key === "BuyPrice" || col.key === "SellPrice" || col.key === "TotalProfit" || col.key === "ProfitPerJump") {
      return formatISK(val as number);
    }
    if (col.key === "MarginPercent") return formatMargin(val as number);
    if (typeof val === "number") return val.toLocaleString("ru-RU");
    return String(val);
  };

  return (
    <div className="flex-1 flex flex-col min-h-0">
      {/* Toolbar */}
      <div className="shrink-0 flex items-center gap-2 px-2 py-1.5 text-xs">
        {/* Status / progress */}
        <div className="flex items-center gap-2 text-eve-dim">
          {scanning ? (
            <span className="flex items-center gap-2">
              <span className="w-2 h-2 rounded-full bg-eve-accent animate-pulse" />
              {progress}
            </span>
          ) : results.length > 0 ? (
            filtered.length !== results.length
              ? t("showing", { shown: filtered.length, total: results.length })
              : t("foundDeals", { count: results.length })
          ) : null}
          {pinnedKeys.size > 0 && (
            <span className="text-eve-accent">
              📌 {t("pinned", { count: pinnedKeys.size })}
            </span>
          )}
          {selectedKeys.size > 0 && (
            <span className="text-eve-accent">
              {t("selected", { count: selectedKeys.size })}
            </span>
          )}
        </div>

        <div className="flex-1" />

        {/* Action buttons */}
        <ToolbarBtn
          label="⊞"
          title={showFilters ? t("clearFilters") : t("filterPlaceholder")}
          active={showFilters}
          onClick={() => setShowFilters((v) => !v)}
        />
        {hasActiveFilters && (
          <ToolbarBtn label="✕" title={t("clearFilters")} onClick={clearFilters} />
        )}
        {results.length > 0 && (
          <>
            <ToolbarBtn label="CSV" title={t("exportCSV")} onClick={exportCSV} />
            <ToolbarBtn label="⎘" title={t("copyTable")} onClick={copyTable} />
          </>
        )}
      </div>

      {/* Table */}
      <div className="flex-1 min-h-0 overflow-auto border border-eve-border rounded-sm">
        <table className="w-full text-sm">
          <thead className="sticky top-0 z-10">
            {/* Column headers */}
            <tr className="bg-eve-dark border-b border-eve-border">
              {/* Checkbox column */}
              <th className="w-8 px-1 py-2 text-center">
                <input
                  type="checkbox"
                  checked={sorted.length > 0 && selectedKeys.size === sorted.length}
                  onChange={toggleSelectAll}
                  className="accent-eve-accent cursor-pointer"
                />
              </th>
              {/* Pin column */}
              <th className="w-8 px-1 py-2" />
              {columnDefs.map((col) => (
                <th
                  key={col.key}
                  onClick={() => toggleSort(col.key)}
                  className={`${col.width} px-3 py-2 text-left text-[11px] uppercase tracking-wider
                             text-eve-dim font-medium cursor-pointer select-none
                             hover:text-eve-accent transition-colors ${
                               sortKey === col.key ? "text-eve-accent" : ""
                             }`}
                >
                  {t(col.labelKey)}
                  {sortKey === col.key && (
                    <span className="ml-1">{sortDir === "asc" ? "▲" : "▼"}</span>
                  )}
                </th>
              ))}
            </tr>
            {/* Filter row */}
            {showFilters && (
              <tr className="bg-eve-dark/80 border-b border-eve-border">
                <th className="w-8" />
                <th className="w-8" />
                {columnDefs.map((col) => (
                  <th key={col.key} className={`${col.width} px-1 py-1`}>
                    <input
                      type="text"
                      value={filters[col.key] ?? ""}
                      onChange={(e) => setFilter(col.key, e.target.value)}
                      placeholder={col.numeric ? "e.g. >100" : t("filterPlaceholder")}
                      className="w-full px-2 py-0.5 bg-eve-input border border-eve-border rounded-sm
                                 text-eve-text text-xs font-mono placeholder:text-eve-dim/50
                                 focus:outline-none focus:border-eve-accent/50 transition-colors"
                    />
                  </th>
                ))}
              </tr>
            )}
          </thead>
          <tbody>
            {sorted.map((row, i) => {
              const key = rowKey(row);
              const isPinned = pinnedKeys.has(key);
              const isSelected = selectedKeys.has(key);
              return (
                <tr
                  key={key}
                  onContextMenu={(e) => handleContextMenu(e, row)}
                  className={`border-b border-eve-border/50 hover:bg-eve-accent/5 transition-colors ${
                    isPinned
                      ? "bg-eve-accent/10 border-l-2 border-l-eve-accent"
                      : isSelected
                        ? "bg-eve-accent/5"
                        : i % 2 === 0
                          ? "bg-eve-panel"
                          : "bg-[#161616]"
                  }`}
                >
                  {/* Checkbox */}
                  <td className="w-8 px-1 py-1.5 text-center">
                    <input
                      type="checkbox"
                      checked={isSelected}
                      onChange={() => toggleSelect(key)}
                      className="accent-eve-accent cursor-pointer"
                    />
                  </td>
                  {/* Pin button */}
                  <td className="w-8 px-1 py-1.5 text-center">
                    <button
                      onClick={() => togglePin(key)}
                      className={`text-xs cursor-pointer transition-opacity ${
                        isPinned ? "opacity-100" : "opacity-30 hover:opacity-70"
                      }`}
                      title={isPinned ? t("unpinRow") : t("pinRow")}
                    >
                      📌
                    </button>
                  </td>
                  {columnDefs.map((col) => (
                    <td
                      key={col.key}
                      className={`px-3 py-1.5 ${col.width} truncate ${
                        col.numeric ? "text-eve-accent font-mono" : "text-eve-text"
                      }`}
                    >
                      {formatCell(col, row)}
                    </td>
                  ))}
                </tr>
              );
            })}
            {results.length === 0 && !scanning && (
              <tr>
                <td colSpan={columnDefs.length + 2} className="px-3 py-8 text-center text-eve-dim">
                  {t("scanPrompt")}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {/* Summary footer */}
      {summary && results.length > 0 && (
        <div className="shrink-0 flex items-center gap-6 px-3 py-1.5 border-t border-eve-border text-xs">
          <span className="text-eve-dim">
            {t("totalProfit")}:{" "}
            <span className="text-eve-accent font-mono font-semibold">{formatISK(summary.totalProfit)}</span>
          </span>
          <span className="text-eve-dim">
            {t("avgMargin")}:{" "}
            <span className="text-eve-accent font-mono font-semibold">{formatMargin(summary.avgMargin)}</span>
          </span>
          {selectedKeys.size > 0 && (
            <span className="text-eve-dim italic">
              ({t("selected", { count: selectedKeys.size })})
            </span>
          )}
        </div>
      )}

      {/* Context menu */}
      {contextMenu && (
        <>
          <div className="fixed inset-0 z-50" onClick={() => setContextMenu(null)} />
          <div
            className="fixed z-50 bg-eve-panel border border-eve-border rounded-sm shadow-eve-glow-strong py-1 min-w-[200px]"
            style={{ left: contextMenu.x, top: contextMenu.y }}
          >
            <ContextItem label={t("copyItem")} onClick={() => copyText(contextMenu.row.TypeName)} />
            <ContextItem label={t("copyBuyStation")} onClick={() => copyText(contextMenu.row.BuyStation)} />
            <ContextItem label={t("copySellStation")} onClick={() => copyText(contextMenu.row.SellStation)} />
            <ContextItem
              label={t("copyTradeRoute")}
              onClick={() => copyText(`Buy: ${contextMenu.row.TypeName} x${contextMenu.row.UnitsToBuy} @ ${contextMenu.row.BuyStation} → Sell: @ ${contextMenu.row.SellStation}`)}
            />
            <ContextItem
              label={t("copySystemAutopilot")}
              onClick={() => copyText(contextMenu.row.BuySystemName)}
            />
            <div className="h-px bg-eve-border my-1" />
            <ContextItem
              label={watchlistIds.has(contextMenu.row.TypeID) ? t("removeFromWatchlist") : `⭐ ${t("addToWatchlist")}`}
              onClick={() => {
                const row = contextMenu.row;
                if (watchlistIds.has(row.TypeID)) {
                  removeFromWatchlist(row.TypeID).then(setWatchlist).catch(() => {});
                } else {
                  addToWatchlist(row.TypeID, row.TypeName).then(setWatchlist).catch(() => {});
                }
                setContextMenu(null);
              }}
            />
            <ContextItem
              label={pinnedKeys.has(rowKey(contextMenu.row)) ? t("unpinRow") : t("pinRow")}
              onClick={() => {
                togglePin(rowKey(contextMenu.row));
                setContextMenu(null);
              }}
            />
          </div>
        </>
      )}
    </div>
  );
}

function ToolbarBtn({
  label,
  title,
  active,
  onClick,
}: {
  label: string;
  title: string;
  active?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      onClick={onClick}
      title={title}
      className={`px-2 py-0.5 rounded-sm text-xs font-medium transition-colors cursor-pointer
        ${active ? "bg-eve-accent/20 text-eve-accent border border-eve-accent/30" : "text-eve-dim hover:text-eve-text border border-eve-border hover:border-eve-border-light"}`}
    >
      {label}
    </button>
  );
}

function ContextItem({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <div
      onClick={onClick}
      className="px-4 py-1.5 text-sm text-eve-text hover:bg-eve-accent/20 hover:text-eve-accent cursor-pointer transition-colors"
    >
      {label}
    </div>
  );
}
