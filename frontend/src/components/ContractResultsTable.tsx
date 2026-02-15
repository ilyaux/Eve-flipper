import { useState, useMemo, useCallback, useRef, useEffect } from "react";
import type { ContractResult } from "@/lib/types";
import { formatISK, formatMargin } from "@/lib/format";
import { useI18n, type TranslationKey } from "@/lib/i18n";
import { useGlobalToast } from "./Toast";
import { EmptyState, type EmptyReason } from "./EmptyState";
import { openContractInGame } from "@/lib/api";
import { handleEveUIError } from "@/lib/handleEveUIError";

type SortKey = keyof ContractResult;
type SortDir = "asc" | "desc";

interface Props {
  results: ContractResult[];
  scanning: boolean;
  progress: string;
  /** When 0 results, show these filter hints (e.g. "Min price: 10M", "Max margin: 100%") */
  filterHints?: string[];
  isLoggedIn?: boolean;
}

const columnDefs: { key: SortKey; labelKey: TranslationKey; width: string; numeric: boolean }[] = [
  { key: "Title", labelKey: "colTitle", width: "min-w-[200px]", numeric: false },
  { key: "Price", labelKey: "colContractPrice", width: "min-w-[120px]", numeric: true },
  { key: "MarketValue", labelKey: "colMarketValue", width: "min-w-[120px]", numeric: true },
  { key: "ExpectedProfit", labelKey: "colContractExpectedProfit", width: "min-w-[120px]", numeric: true },
  { key: "Profit", labelKey: "colContractProfit", width: "min-w-[120px]", numeric: true },
  { key: "SellConfidence", labelKey: "colContractConfidence", width: "min-w-[95px]", numeric: true },
  { key: "EstLiquidationDays", labelKey: "colContractLiqDays", width: "min-w-[85px]", numeric: true },
  { key: "MarginPercent", labelKey: "colContractMargin", width: "min-w-[80px]", numeric: true },
  { key: "Volume", labelKey: "colVolume", width: "min-w-[80px]", numeric: true },
  { key: "StationName", labelKey: "colStation", width: "min-w-[180px]", numeric: false },
  { key: "SystemName", labelKey: "colContractSystem", width: "min-w-[120px]", numeric: false },
  { key: "RegionName", labelKey: "colContractRegion", width: "min-w-[120px]", numeric: false },
  { key: "ItemCount", labelKey: "colItems", width: "min-w-[70px]", numeric: true },
  { key: "ProfitPerJump", labelKey: "colContractPPJ", width: "min-w-[110px]", numeric: true },
  { key: "Jumps", labelKey: "colContractJumps", width: "min-w-[60px]", numeric: true },
];

function rowKey(row: ContractResult) {
  return `contract-${row.ContractID}`;
}

export function ContractResultsTable({ results, scanning, progress, filterHints, isLoggedIn = false }: Props) {
  const { t } = useI18n();
  const { addToast } = useGlobalToast();
  const emptyReason: EmptyReason = (results.length === 0 && filterHints && filterHints.length > 0)
    ? "filters_too_strict"
    : "no_scan_yet";

  const [sortKey, setSortKey] = useState<SortKey>("ExpectedProfit");
  const [sortDir, setSortDir] = useState<SortDir>("desc");
  const [filters, setFilters] = useState<Record<string, string>>({});
  const [showFilters, setShowFilters] = useState(false);

  // Context menu
  const [contextMenu, setContextMenu] = useState<{ x: number; y: number; row: ContractResult } | null>(null);
  const contextMenuRef = useRef<HTMLDivElement>(null);

  const handleContextMenu = useCallback((e: React.MouseEvent, row: ContractResult) => {
    e.preventDefault();
    setContextMenu({ x: e.clientX, y: e.clientY, row });
  }, []);

  const copyText = (text: string) => {
    navigator.clipboard.writeText(text);
    addToast(t("copied"), "success", 2000);
    setContextMenu(null);
  };

  // Adjust context menu position
  useEffect(() => {
    if (contextMenu && contextMenuRef.current) {
      const menu = contextMenuRef.current;
      const rect = menu.getBoundingClientRect();
      const padding = 10;
      let x = contextMenu.x;
      let y = contextMenu.y;
      if (x + rect.width > window.innerWidth - padding) x = window.innerWidth - rect.width - padding;
      if (y + rect.height > window.innerHeight - padding) y = window.innerHeight - rect.height - padding;
      x = Math.max(padding, x);
      y = Math.max(padding, y);
      menu.style.left = `${x}px`;
      menu.style.top = `${y}px`;
    }
  }, [contextMenu]);

  const filtered = useMemo(() => {
    if (Object.values(filters).every((v) => !v)) return results;
    return results.filter((row) => {
      for (const col of columnDefs) {
        const fval = filters[col.key];
        if (!fval) continue;
        const cellVal = row[col.key];
        if (col.numeric) {
          // Support filters: "100-500" (range), ">100", ">=100", "<500", "<=500", "=100" (exact), or plain number (>= threshold)
          const num = cellVal as number;
          const trimmed = fval.trim();
          if (trimmed.includes("-") && !trimmed.startsWith("-")) {
            // Range: "100-500"
            const [minS, maxS] = trimmed.split("-");
            const min = parseFloat(minS);
            const max = parseFloat(maxS);
            if (!isNaN(min) && !isNaN(max) && (num < min || num > max)) return false;
          } else if (trimmed.startsWith(">=")) {
            const min = parseFloat(trimmed.slice(2));
            if (!isNaN(min) && num < min) return false;
          } else if (trimmed.startsWith(">")) {
            const min = parseFloat(trimmed.slice(1));
            if (!isNaN(min) && num <= min) return false;
          } else if (trimmed.startsWith("<=")) {
            const max = parseFloat(trimmed.slice(2));
            if (!isNaN(max) && num > max) return false;
          } else if (trimmed.startsWith("<")) {
            const max = parseFloat(trimmed.slice(1));
            if (!isNaN(max) && num >= max) return false;
          } else if (trimmed.startsWith("=")) {
            // Exact match
            const target = parseFloat(trimmed.slice(1));
            if (!isNaN(target) && num !== target) return false;
          } else {
            // Plain number: treat as >= (minimum threshold)
            const min = parseFloat(trimmed);
            if (!isNaN(min) && num < min) return false;
          }
        } else {
          if (!String(cellVal).toLowerCase().includes(fval.toLowerCase())) return false;
        }
      }
      return true;
    });
  }, [results, filters]);

  const sorted = useMemo(() => {
    const copy = [...filtered];
    const currentCol = columnDefs.find((c) => c.key === sortKey);
    const numericSort = !!currentCol?.numeric;
    copy.sort((a, b) => {
      const av = a[sortKey];
      const bv = b[sortKey];
      if (numericSort) {
        const an = typeof av === "number" ? av : 0;
        const bn = typeof bv === "number" ? bv : 0;
        return sortDir === "asc" ? an - bn : bn - an;
      }
      return sortDir === "asc"
        ? String(av).localeCompare(String(bv))
        : String(bv).localeCompare(String(av));
    });
    return copy;
  }, [filtered, sortKey, sortDir]);

  const summary = useMemo(() => {
    if (sorted.length === 0) return null;
    const totalProfit = sorted.reduce((sum, r) => sum + r.Profit, 0);
    const totalExpected = sorted.reduce((sum, r) => sum + (r.ExpectedProfit ?? r.Profit), 0);
    const avgMargin = sorted.reduce((sum, r) => sum + (r.ExpectedMarginPercent ?? r.MarginPercent), 0) / sorted.length;
    return { totalProfit, totalExpected, avgMargin, count: sorted.length };
  }, [sorted]);

  const toggleSort = (key: SortKey) => {
    if (sortKey === key) {
      setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    } else {
      setSortKey(key);
      setSortDir("desc");
    }
  };

  const hasActiveFilters = Object.values(filters).some((v) => !!v);

  const formatCell = (col: (typeof columnDefs)[number], row: ContractResult): string => {
    const val = row[col.key];
    if (val == null || val === "") return "\u2014";
    if (
      col.key === "Price" ||
      col.key === "MarketValue" ||
      col.key === "Profit" ||
      col.key === "ExpectedProfit" ||
      col.key === "ProfitPerJump"
    ) {
      return formatISK(val as number);
    }
    if (col.key === "MarginPercent") return formatMargin((row.ExpectedMarginPercent ?? row.MarginPercent) as number);
    if (col.key === "SellConfidence") return `${(val as number).toFixed(1)}%`;
    if (col.key === "Volume") return (val as number).toFixed(1);
    if (col.key === "EstLiquidationDays") return (val as number).toFixed(1);
    if (typeof val === "number") return val.toLocaleString("ru-RU");
    return String(val);
  };

  const exportCSV = () => {
    const header = columnDefs.map((c) => t(c.labelKey)).join(",");
    const csvRows = sorted.map((row) =>
      columnDefs.map((col) => {
        const str = String(row[col.key]);
        return str.includes(",") ? `"${str}"` : str;
      }).join(",")
    );
    const csv = [header, ...csvRows].join("\n");
    const blob = new Blob(["\uFEFF" + csv], { type: "text/csv;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `eve-contracts-${new Date().toISOString().slice(0, 10)}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <div className="flex-1 flex flex-col min-h-0">
      {/* Toolbar */}
      <div className="shrink-0 flex items-center gap-2 px-2 py-1.5 text-xs">
        <div className="flex items-center gap-2 text-eve-dim">
          {scanning ? (
            <span className="flex items-center gap-2">
              <span className="w-2 h-2 rounded-full bg-eve-accent animate-pulse" />
              {progress}
            </span>
          ) : results.length > 0 ? (
            filtered.length !== results.length
              ? t("showing", { shown: filtered.length, total: results.length })
              : t("foundContracts", { count: results.length })
          ) : null}
        </div>
        <div className="flex-1" />
        <button
          onClick={() => setShowFilters((v) => !v)}
          className={`px-2 py-0.5 rounded-sm text-xs font-medium transition-colors cursor-pointer
            ${showFilters ? "bg-eve-accent/20 text-eve-accent border border-eve-accent/30" : "text-eve-dim hover:text-eve-text border border-eve-border hover:border-eve-border-light"}`}
        >
          ⊞
        </button>
        {hasActiveFilters && (
          <button
            onClick={() => setFilters({})}
            className="px-2 py-0.5 rounded-sm text-xs font-medium text-eve-dim hover:text-eve-text border border-eve-border cursor-pointer"
          >
            ✕
          </button>
        )}
        {results.length > 0 && (
          <button
            onClick={exportCSV}
            className="px-2 py-0.5 rounded-sm text-xs font-medium text-eve-dim hover:text-eve-text border border-eve-border cursor-pointer"
          >
            CSV
          </button>
        )}
      </div>

      {/* Table */}
      <div className="flex-1 min-h-0 overflow-auto border border-eve-border rounded-sm table-scroll-wrapper table-scroll-container">
        <table className="w-full text-sm">
          <thead className="sticky top-0 z-10">
            <tr className="bg-eve-dark border-b border-eve-border">
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
            {showFilters && (
              <tr className="bg-eve-dark/80 border-b border-eve-border">
                {columnDefs.map((col) => (
                  <th key={col.key} className={`${col.width} px-1 py-1`}>
                    <input
                      type="text"
                      value={filters[col.key] ?? ""}
                      onChange={(e) => setFilters((f) => ({ ...f, [col.key]: e.target.value }))}
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
            {sorted.map((row, i) => (
              <tr
                key={rowKey(row)}
                onContextMenu={(e) => handleContextMenu(e, row)}
                className={`border-b border-eve-border/50 hover:bg-eve-accent/5 transition-colors ${
                  i % 2 === 0 ? "bg-eve-panel" : "bg-eve-dark"
                }`}
              >
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
            ))}
            {results.length === 0 && !scanning && (
              <tr>
                <td colSpan={columnDefs.length} className="p-0">
                  <EmptyState
                    reason={emptyReason}
                    hints={filterHints}
                    wikiSlug="Contract-Arbitrage"
                  />
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
            {t("colContractExpectedProfit")}:{" "}
            <span className="text-eve-success font-mono font-semibold">{formatISK(summary.totalExpected)}</span>
          </span>
          <span className="text-eve-dim">
            {t("avgMargin")}:{" "}
            <span className="text-eve-accent font-mono font-semibold">{formatMargin(summary.avgMargin)}</span>
          </span>
        </div>
      )}

      {/* Context menu */}
      {contextMenu && (
        <>
          <div className="fixed inset-0 z-50" onClick={() => setContextMenu(null)} />
          <div
            ref={contextMenuRef}
            className="fixed z-50 bg-eve-panel border border-eve-border rounded-sm shadow-eve-glow-strong py-1 min-w-[200px]"
            style={{ left: contextMenu.x, top: contextMenu.y }}
          >
            <ContextItem label={t("copyItem")} onClick={() => copyText(contextMenu.row.Title)} />
            <ContextItem label={t("copyStation")} onClick={() => copyText(contextMenu.row.StationName)} />
            <ContextItem label={t("copyContractID")} onClick={() => copyText(String(contextMenu.row.ContractID))} />
            <div className="h-px bg-eve-border my-1" />
            {/* EVE UI actions */}
            {isLoggedIn && (
              <>
                <ContextItem
                  label={`🎮 ${t("openContract")}`}
                  onClick={async () => {
                    try {
                      await openContractInGame(contextMenu.row.ContractID);
                      addToast(t("actionSuccess"), "success", 2000);
                    } catch (err: any) {
                      const { messageKey, duration } = handleEveUIError(err);
                      addToast(t(messageKey), "error", duration);
                    }
                    setContextMenu(null);
                  }}
                />
                <div className="h-px bg-eve-border my-1" />
              </>
            )}
            <ContextItem
              label={t("openInEveref")}
              onClick={() => { window.open(`https://everef.net/contract/${contextMenu.row.ContractID}`, "_blank"); setContextMenu(null); }}
            />
          </div>
        </>
      )}
    </div>
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
