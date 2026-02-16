import { useState, useMemo, useCallback, useEffect, useRef } from "react";
import type { FlipResult, WatchlistItem } from "@/lib/types";
import { formatISK, formatMargin } from "@/lib/format";
import { useI18n, type TranslationKey } from "@/lib/i18n";
import { getWatchlist, addToWatchlist, removeFromWatchlist, openMarketInGame, setWaypointInGame } from "@/lib/api";
import { useGlobalToast } from "./Toast";
import { EmptyState, type EmptyReason } from "./EmptyState";
import { ExecutionPlannerPopup } from "./ExecutionPlannerPopup";
import { handleEveUIError } from "@/lib/handleEveUIError";

const PAGE_SIZE = 100;

type SortKey = keyof FlipResult;
type SortDir = "asc" | "desc";

interface Props {
  results: FlipResult[];
  scanning: boolean;
  progress: string;
  scanCompletedWithZero?: boolean;
  salesTaxPercent?: number;
  brokerFeePercent?: number;
  splitTradeFees?: boolean;
  buyBrokerFeePercent?: number;
  sellBrokerFeePercent?: number;
  buySalesTaxPercent?: number;
  sellSalesTaxPercent?: number;
  showRegions?: boolean;
  isLoggedIn?: boolean;
}

type ColumnDef = {
  key: SortKey;
  labelKey: TranslationKey;
  width: string;
  numeric: boolean;
};

/* ─── Column definitions ─── */

const baseColumnDefs: ColumnDef[] = [
  {
    key: "TypeName",
    labelKey: "colItem",
    width: "min-w-[180px]",
    numeric: false,
  },
  {
    key: "BuyPrice",
    labelKey: "colBuyPrice",
    width: "min-w-[110px]",
    numeric: true,
  },
  {
    key: "BuyStation",
    labelKey: "colBuyStation",
    width: "min-w-[150px]",
    numeric: false,
  },
  {
    key: "SellPrice",
    labelKey: "colSellPrice",
    width: "min-w-[110px]",
    numeric: true,
  },
  {
    key: "SellStation",
    labelKey: "colSellStation",
    width: "min-w-[150px]",
    numeric: false,
  },
  {
    key: "MarginPercent",
    labelKey: "colMargin",
    width: "min-w-[80px]",
    numeric: true,
  },
  {
    key: "UnitsToBuy",
    labelKey: "colUnitsToBuy",
    width: "min-w-[80px]",
    numeric: true,
  },
  {
    key: "FilledQty",
    labelKey: "colFilledQty",
    width: "min-w-[80px]",
    numeric: true,
  },
  {
    key: "CanFill",
    labelKey: "colCanFill",
    width: "min-w-[70px]",
    numeric: false,
  },
  {
    key: "BuyOrderRemain",
    labelKey: "colAcceptQty",
    width: "min-w-[80px]",
    numeric: true,
  },
  {
    key: "RealProfit",
    labelKey: "colRealProfit",
    width: "min-w-[120px]",
    numeric: true,
  },
  {
    key: "TotalProfit",
    labelKey: "colProfit",
    width: "min-w-[120px]",
    numeric: true,
  },
  {
    key: "ExpectedProfit",
    labelKey: "colExpectedProfit",
    width: "min-w-[100px]",
    numeric: true,
  },
  {
    key: "ProfitPerJump",
    labelKey: "colProfitPerJump",
    width: "min-w-[110px]",
    numeric: true,
  },
  {
    key: "TotalJumps",
    labelKey: "colJumps",
    width: "min-w-[60px]",
    numeric: true,
  },
  {
    key: "DailyVolume",
    labelKey: "colDailyVolume",
    width: "min-w-[80px]",
    numeric: true,
  },
  {
    key: "DailyProfit",
    labelKey: "colDailyProfit",
    width: "min-w-[110px]",
    numeric: true,
  },
  {
    key: "PriceTrend",
    labelKey: "colPriceTrend",
    width: "min-w-[70px]",
    numeric: true,
  },
  {
    key: "BuyCompetitors",
    labelKey: "colBuyCompetitors",
    width: "min-w-[70px]",
    numeric: true,
  },
  {
    key: "SellCompetitors",
    labelKey: "colSellCompetitors",
    width: "min-w-[70px]",
    numeric: true,
  },
];

const regionColumnDefs: ColumnDef[] = [
  {
    key: "BuyRegionName" as SortKey,
    labelKey: "colBuyRegion" as TranslationKey,
    width: "min-w-[120px]",
    numeric: false,
  },
  {
    key: "SellRegionName" as SortKey,
    labelKey: "colSellRegion" as TranslationKey,
    width: "min-w-[120px]",
    numeric: false,
  },
];

function buildColumnDefs(showRegions: boolean): ColumnDef[] {
  if (!showRegions) return baseColumnDefs;
  const cols = [...baseColumnDefs];
  const sellIdx = cols.findIndex((c) => c.key === "SellStation");
  if (sellIdx >= 0) cols.splice(sellIdx + 1, 0, regionColumnDefs[1]);
  const buyIdx = cols.findIndex((c) => c.key === "BuyStation");
  if (buyIdx >= 0) cols.splice(buyIdx + 1, 0, regionColumnDefs[0]);
  return cols;
}

/* ─── Row identity ───
 * Stable per-row object id to avoid duplicate keys when data has collisions.
 */
let _nextRowId = 1;
const _rowIdMap = new WeakMap<FlipResult, number>();
function getRowId(row: FlipResult): number {
  let id = _rowIdMap.get(row);
  if (id == null) {
    id = _nextRowId++;
    _rowIdMap.set(row, id);
  }
  return id;
}

/* ─── IndexedRow: carries stable identity for rows ─── */
interface IndexedRow {
  id: number; // stable id from WeakMap
  row: FlipResult;
}

/* ─── Filter helpers ─── */

function passesNumericFilter(num: number, fval: string): boolean {
  const trimmed = fval.trim();
  if (!trimmed) return true;
  // Range: "100-500"
  if (trimmed.includes("-") && !trimmed.startsWith("-")) {
    const [minS, maxS] = trimmed.split("-");
    const mn = parseFloat(minS);
    const mx = parseFloat(maxS);
    if (!isNaN(mn) && !isNaN(mx) && (num < mn || num > mx)) return false;
    return true;
  }
  if (trimmed.startsWith(">=")) {
    const v = parseFloat(trimmed.slice(2));
    return isNaN(v) || num >= v;
  }
  if (trimmed.startsWith(">")) {
    const v = parseFloat(trimmed.slice(1));
    return isNaN(v) || num > v;
  }
  if (trimmed.startsWith("<=")) {
    const v = parseFloat(trimmed.slice(2));
    return isNaN(v) || num <= v;
  }
  if (trimmed.startsWith("<")) {
    const v = parseFloat(trimmed.slice(1));
    return isNaN(v) || num < v;
  }
  if (trimmed.startsWith("=")) {
    const v = parseFloat(trimmed.slice(1));
    return isNaN(v) || num === v;
  }
  // Plain number: >= threshold
  const mn = parseFloat(trimmed);
  return isNaN(mn) || num >= mn;
}

function passesTextFilter(val: unknown, fval: string): boolean {
  return String(val ?? "")
    .toLowerCase()
    .includes(fval.toLowerCase());
}

function passesFilter(row: FlipResult, col: ColumnDef, fval: string): boolean {
  if (!fval) return true;
  const cellVal = row[col.key];
  return col.numeric
    ? passesNumericFilter(cellVal as number, fval)
    : passesTextFilter(cellVal, fval);
}

/* ─── Cell formatting ─── */

function fmtCell(col: ColumnDef, row: FlipResult): string {
  const val = row[col.key];
  if (col.key === "ExpectedProfit" || col.key === "RealProfit") {
    if (val == null || Number.isNaN(val)) return "\u2014";
    return formatISK(val as number);
  }
  if (col.key === "CanFill") {
    if (val == null) return "\u2014";
    return val ? "✓" : "✕";
  }
  if (
    col.key === "BuyPrice" ||
    col.key === "SellPrice" ||
    col.key === "TotalProfit" ||
    col.key === "ProfitPerJump" ||
    col.key === "DailyProfit"
  ) {
    return formatISK(val as number);
  }
  if (col.key === "MarginPercent") return formatMargin(val as number);
  if (col.key === "PriceTrend") {
    const v = val as number;
    return (v >= 0 ? "+" : "") + v.toFixed(1) + "%";
  }
  if (typeof val === "number") return val.toLocaleString();
  return String(val ?? "");
}

/* ═══════════════════════════════════════════════════════════════════════
 * COMPONENT
 * ═══════════════════════════════════════════════════════════════════════ */

export function ScanResultsTable({
  results,
  scanning,
  progress,
  scanCompletedWithZero,
  salesTaxPercent,
  brokerFeePercent,
  splitTradeFees,
  buyBrokerFeePercent,
  sellBrokerFeePercent,
  buySalesTaxPercent,
  sellSalesTaxPercent,
  showRegions = false,
  isLoggedIn = false,
}: Props) {
  const { t } = useI18n();
  const emptyReason: EmptyReason = scanCompletedWithZero
    ? "no_results"
    : "no_scan_yet";
  const { addToast } = useGlobalToast();

  const columnDefs = useMemo(() => buildColumnDefs(showRegions), [showRegions]);

  // ── State ──
  const [sortKey, setSortKey] = useState<SortKey>("RealProfit");
  const [sortDir, setSortDir] = useState<SortDir>("desc");
  const [filters, setFilters] = useState<Record<string, string>>({});
  const [showFilters, setShowFilters] = useState(false);
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set());
  const [pinnedIds, setPinnedIds] = useState<Set<number>>(new Set());
  const [page, setPage] = useState(0);
  const [compactMode, setCompactMode] = useState(false);

  // Watchlist
  const [watchlist, setWatchlist] = useState<WatchlistItem[]>([]);
  useEffect(() => {
    getWatchlist()
      .then(setWatchlist)
      .catch(() => {});
  }, []);
  const watchlistIds = useMemo(
    () => new Set(watchlist.map((w) => w.type_id)),
    [watchlist],
  );

  // Context menu
  const [contextMenu, setContextMenu] = useState<{
    x: number;
    y: number;
    id: number;
    row: FlipResult;
  } | null>(null);
  const contextMenuRef = useRef<HTMLDivElement>(null);
  const [execPlanRow, setExecPlanRow] = useState<FlipResult | null>(null);

  useEffect(() => {
    if (contextMenu && contextMenuRef.current) {
      const menu = contextMenuRef.current;
      const rect = menu.getBoundingClientRect();
      const pad = 10;
      let x = contextMenu.x,
        y = contextMenu.y;
      if (x + rect.width > window.innerWidth - pad)
        x = window.innerWidth - rect.width - pad;
      if (y + rect.height > window.innerHeight - pad)
        y = window.innerHeight - rect.height - pad;
      menu.style.left = `${Math.max(pad, x)}px`;
      menu.style.top = `${Math.max(pad, y)}px`;
    }
  }, [contextMenu]);

  // ── Data pipeline: index → filter → sort → paginate (single memo) ──
  const { indexed, filtered, sorted, pageRows, totalPages, safePage } =
    useMemo(() => {
      // Tag each row with stable id
      const indexed: IndexedRow[] = results.map((row) => ({
        id: getRowId(row),
        row,
      }));

      // Filter
      const hasFilters = Object.values(filters).some((v) => !!v);
      const filtered = hasFilters
        ? indexed.filter((ir) => {
            for (const col of columnDefs) {
              const fval = filters[col.key];
              if (!fval) continue;
              if (!passesFilter(ir.row, col, fval)) return false;
            }
            return true;
          })
        : indexed;

      // Sort
      const sorted = filtered.slice(); // shallow copy
      sorted.sort((a, b) => {
        const aPin = pinnedIds.has(a.id);
        const bPin = pinnedIds.has(b.id);
        if (aPin !== bPin) return aPin ? -1 : 1;

        const av = a.row[sortKey];
        const bv = b.row[sortKey];
        if (typeof av === "number" || typeof bv === "number") {
          if (av == null && bv == null) return 0;
          if (av == null) return 1;
          if (bv == null) return -1;
          const diff = (av as number) - (bv as number);
          return sortDir === "asc" ? diff : -diff;
        }
        const cmp = String(av ?? "").localeCompare(String(bv ?? ""));
        return sortDir === "asc" ? cmp : -cmp;
      });

      // Paginate
      const totalPages = Math.max(1, Math.ceil(sorted.length / PAGE_SIZE));
      const safePage = Math.min(page, totalPages - 1);
      const pageRows = sorted.slice(
        safePage * PAGE_SIZE,
        (safePage + 1) * PAGE_SIZE,
      );

      return { indexed, filtered, sorted, pageRows, totalPages, safePage };
    }, [results, filters, columnDefs, sortKey, sortDir, pinnedIds, page]);

  // Reset page when data/filters/sort change
  useEffect(() => {
    setPage(0);
  }, [results, filters, sortKey, sortDir]);

  // Reset selection/pins/context menu when results change
  useEffect(() => {
    setSelectedIds(new Set());
    setPinnedIds(new Set());
    setContextMenu(null);
  }, [results]);

  // Drop filters for columns that are no longer visible
  useEffect(() => {
    const allowed = new Set(columnDefs.map((col) => col.key));
    setFilters((prev) => {
      let changed = false;
      const next: Record<string, string> = {};
      for (const [key, value] of Object.entries(prev)) {
        if (allowed.has(key as SortKey)) {
          next[key] = value;
        } else {
          changed = true;
        }
      }
      return changed ? next : prev;
    });
  }, [columnDefs]);

  // Prune selected rows that are hidden by filters
  useEffect(() => {
    if (selectedIds.size === 0) return;
    const visibleIds = new Set(filtered.map((ir) => ir.id));
    setSelectedIds((prev) => {
      if (prev.size === 0) return prev;
      const next = new Set([...prev].filter((id) => visibleIds.has(id)));
      return next.size === prev.size ? prev : next;
    });
  }, [filtered, selectedIds.size]);

  // ── Summary stats ──
  const summary = useMemo(() => {
    const rows =
      selectedIds.size > 0
        ? sorted.filter((ir) => selectedIds.has(ir.id))
        : sorted;
    if (rows.length === 0) return null;
    const totalProfit = rows.reduce(
      (s, ir) => s + (ir.row.RealProfit ?? ir.row.ExpectedProfit ?? ir.row.TotalProfit),
      0,
    );
    const avgMargin =
      rows.reduce((s, ir) => s + ir.row.MarginPercent, 0) / rows.length;
    return { totalProfit, avgMargin, count: rows.length };
  }, [sorted, selectedIds]);

  // ── Callbacks ──
  const toggleSort = useCallback(
    (key: SortKey) => {
      if (key === sortKey) {
        setSortDir((d) => (d === "asc" ? "desc" : "asc"));
      } else {
        setSortKey(key);
        setSortDir("desc");
      }
    },
    [sortKey],
  );

  const setFilter = useCallback((key: string, value: string) => {
    setFilters((f) => ({ ...f, [key]: value }));
  }, []);

  const clearFilters = useCallback(() => {
    setFilters({});
  }, []);
  const hasActiveFilters = Object.values(filters).some((v) => !!v);

  const toggleSelect = useCallback((id: number) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const toggleSelectAll = useCallback(() => {
    setSelectedIds((prev) => {
      if (prev.size === sorted.length) return new Set();
      return new Set(sorted.map((ir) => ir.id));
    });
  }, [sorted]);

  const togglePin = useCallback((id: number) => {
    setPinnedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const handleContextMenu = useCallback(
    (e: React.MouseEvent, id: number, row: FlipResult) => {
      e.preventDefault();
      setContextMenu({ x: e.clientX, y: e.clientY, id, row });
    },
    [],
  );

  const copyText = useCallback(
    (text: string) => {
      navigator.clipboard.writeText(text);
      addToast(t("copied"), "success", 2000);
      setContextMenu(null);
    },
    [addToast, t],
  );

  const exportCSV = useCallback(() => {
    const rows =
      selectedIds.size > 0
        ? sorted.filter((ir) => selectedIds.has(ir.id))
        : sorted;
    const header = columnDefs.map((c) => t(c.labelKey)).join(",");
    const csvRows = rows.map((ir) =>
      columnDefs
        .map((col) => {
          const str = String(ir.row[col.key] ?? "");
          return str.includes(",") ? `"${str}"` : str;
        })
        .join(","),
    );
    const csv = [header, ...csvRows].join("\n");
    const blob = new Blob(["\uFEFF" + csv], { type: "text/csv;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `eve-flipper-${new Date().toISOString().slice(0, 10)}.csv`;
    a.click();
    URL.revokeObjectURL(url);
    addToast(`${t("exportCSV")}: ${rows.length} rows`, "success", 2000);
  }, [sorted, selectedIds, columnDefs, addToast, t]);

  const copyTable = useCallback(() => {
    const rows =
      selectedIds.size > 0
        ? sorted.filter((ir) => selectedIds.has(ir.id))
        : sorted;
    const header = columnDefs.map((c) => t(c.labelKey)).join("\t");
    const tsv = rows.map((ir) =>
      columnDefs.map((col) => fmtCell(col, ir.row)).join("\t"),
    );
    navigator.clipboard.writeText([header, ...tsv].join("\n"));
    addToast(t("copied"), "success", 2000);
  }, [sorted, selectedIds, columnDefs, addToast, t]);

  // ── Render ──
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
            filtered.length !== indexed.length ? (
              t("showing", { shown: filtered.length, total: indexed.length })
            ) : (
              t("foundDeals", { count: indexed.length })
            )
          ) : null}
          {pinnedIds.size > 0 && (
            <span className="text-eve-accent">
              📌 {t("pinned", { count: pinnedIds.size })}
            </span>
          )}
          {selectedIds.size > 0 && (
            <span className="text-eve-accent">
              {t("selected", { count: selectedIds.size })}
            </span>
          )}
        </div>

        <div className="flex-1" />

        {/* Pagination */}
        {sorted.length > PAGE_SIZE && (
          <div className="flex items-center gap-1 text-eve-dim">
            <button
              onClick={() => setPage(0)}
              disabled={safePage === 0}
              className="px-1.5 py-0.5 rounded-sm hover:text-eve-text disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
            >
              «
            </button>
            <button
              onClick={() => setPage((p) => Math.max(0, p - 1))}
              disabled={safePage === 0}
              className="px-1.5 py-0.5 rounded-sm hover:text-eve-text disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
            >
              ‹
            </button>
            <span className="px-2 text-eve-text font-mono tabular-nums">
              {safePage + 1} / {totalPages}
            </span>
            <button
              onClick={() => setPage((p) => Math.min(totalPages - 1, p + 1))}
              disabled={safePage >= totalPages - 1}
              className="px-1.5 py-0.5 rounded-sm hover:text-eve-text disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
            >
              ›
            </button>
            <button
              onClick={() => setPage(totalPages - 1)}
              disabled={safePage >= totalPages - 1}
              className="px-1.5 py-0.5 rounded-sm hover:text-eve-text disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
            >
              »
            </button>
          </div>
        )}

        {/* Action buttons */}
        <ToolbarBtn
          label="⊞"
          title={showFilters ? t("clearFilters") : t("filterPlaceholder")}
          active={showFilters}
          onClick={() => setShowFilters((v) => !v)}
        />
        {hasActiveFilters && (
          <ToolbarBtn
            label="✕"
            title={t("clearFilters")}
            onClick={clearFilters}
          />
        )}
        {results.length > 0 && (
          <>
            <ToolbarBtn
              label={compactMode ? "⊞" : "⊟"}
              title={compactMode ? t("comfyRows") : t("compactRows")}
              active={compactMode}
              onClick={() => setCompactMode((v) => !v)}
            />
            <ToolbarBtn
              label="CSV"
              title={t("exportCSV")}
              onClick={exportCSV}
            />
            <ToolbarBtn
              label="⎘"
              title={t("copyTable")}
              onClick={copyTable}
            />
          </>
        )}
      </div>

      {/* Table */}
      <div className="flex-1 min-h-0 flex flex-col border border-eve-border rounded-sm overflow-auto table-scroll-container">
        <table className="w-full text-sm">
          <thead className="sticky top-0 z-10">
            <tr className="bg-eve-dark border-b border-eve-border">
              <th className="w-8 px-1 py-2 text-center">
                <input
                  type="checkbox"
                  checked={
                    sorted.length > 0 && selectedIds.size === sorted.length
                  }
                  onChange={toggleSelectAll}
                  className="accent-eve-accent cursor-pointer"
                />
              </th>
              <th className="w-8 px-1 py-2" />
              {columnDefs.map((col) => (
                <th
                  key={col.key}
                  onClick={() => toggleSort(col.key)}
                  className={`${col.width} px-3 py-2 text-left text-[11px] uppercase tracking-wider text-eve-dim font-medium cursor-pointer select-none hover:text-eve-accent transition-colors ${sortKey === col.key ? "text-eve-accent" : ""}`}
                >
                  {t(col.labelKey)}
                  {sortKey === col.key && (
                    <span className="ml-1">
                      {sortDir === "asc" ? "▲" : "▼"}
                    </span>
                  )}
                </th>
              ))}
            </tr>
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
                      placeholder={
                        col.numeric ? "e.g. >100" : t("filterPlaceholder")
                      }
                      className="w-full px-2 py-0.5 bg-eve-input border border-eve-border rounded-sm text-eve-text text-xs font-mono placeholder:text-eve-dim/50 focus:outline-none focus:border-eve-accent/50 transition-colors"
                    />
                  </th>
                ))}
              </tr>
            )}
          </thead>
          <tbody>
            {pageRows.map((ir, i) => {
              const isPinned = pinnedIds.has(ir.id);
              const isSelected = selectedIds.has(ir.id);
              const globalIdx = safePage * PAGE_SIZE + i;
              return (
                <tr
                  key={ir.id}
                  onContextMenu={(e) => handleContextMenu(e, ir.id, ir.row)}
                  className={`border-b border-eve-border/50 hover:bg-eve-accent/5 transition-colors ${compactMode ? "text-xs" : ""} ${
                    isPinned
                      ? "bg-eve-accent/10 border-l-2 border-l-eve-accent"
                      : isSelected
                        ? "bg-eve-accent/5"
                        : globalIdx % 2 === 0
                          ? "bg-eve-panel"
                          : "bg-eve-dark"
                  }`}
                >
                  <td
                    className={`w-8 px-1 text-center ${compactMode ? "py-1" : "py-1.5"}`}
                  >
                    <input
                      type="checkbox"
                      checked={isSelected}
                      onChange={() => toggleSelect(ir.id)}
                      className="accent-eve-accent cursor-pointer"
                    />
                  </td>
                  <td
                    className={`w-8 px-1 text-center ${compactMode ? "py-1" : "py-1.5"}`}
                  >
                    <button
                      onClick={() => togglePin(ir.id)}
                      className={`text-xs cursor-pointer transition-opacity ${isPinned ? "opacity-100" : "opacity-30 hover:opacity-70"}`}
                      title={isPinned ? t("unpinRow") : t("pinRow")}
                    >
                      {"📌"}
                    </button>
                  </td>
                  {columnDefs.map((col) => (
                    <td
                      key={col.key}
                      className={`px-3 ${compactMode ? "py-1" : "py-1.5"} ${col.width} truncate ${col.numeric ? "text-eve-accent font-mono" : "text-eve-text"}`}
                    >
                      {fmtCell(col, ir.row)}
                    </td>
                  ))}
                </tr>
              );
            })}
            {results.length === 0 && !scanning && (
              <tr>
                <td colSpan={columnDefs.length + 2} className="p-0">
                  <EmptyState reason={emptyReason} wikiSlug="Getting-Started" />
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
            {t("colRealProfit")}:{" "}
            <span className="text-eve-accent font-mono font-semibold">
              {formatISK(summary.totalProfit)}
            </span>
          </span>
          <span className="text-eve-dim">
            {t("avgMargin")}:{" "}
            <span className="text-eve-accent font-mono font-semibold">
              {formatMargin(summary.avgMargin)}
            </span>
          </span>
          {selectedIds.size > 0 && (
            <span className="text-eve-dim italic">
              ({t("selected", { count: selectedIds.size })})
            </span>
          )}
        </div>
      )}

      {/* Context menu */}
      {contextMenu && (
        <>
          <div
            className="fixed inset-0 z-50"
            onClick={() => setContextMenu(null)}
          />
          <div
            ref={contextMenuRef}
            className="fixed z-50 bg-eve-panel border border-eve-border rounded-sm shadow-eve-glow-strong py-1 min-w-[200px]"
            style={{ left: contextMenu.x, top: contextMenu.y }}
          >
            <ContextItem
              label={t("copyItem")}
              onClick={() => copyText(contextMenu.row.TypeName ?? "")}
            />
            <ContextItem
              label={t("copyBuyStation")}
              onClick={() => copyText(contextMenu.row.BuyStation ?? "")}
            />
            <ContextItem
              label={t("copySellStation")}
              onClick={() => copyText(contextMenu.row.SellStation ?? "")}
            />
            <ContextItem
              label={t("copyTradeRoute")}
              onClick={() =>
                copyText(
                  `Buy: ${contextMenu.row.TypeName} x${contextMenu.row.UnitsToBuy} @ ${contextMenu.row.BuyStation} \u2192 Sell: @ ${contextMenu.row.SellStation}`,
                )
              }
            />
            <ContextItem
              label={t("copySystemAutopilot")}
              onClick={() => copyText(contextMenu.row.BuySystemName)}
            />
            <div className="h-px bg-eve-border my-1" />
            <ContextItem
              label={t("openInEveref")}
              onClick={() => {
                window.open(
                  `https://everef.net/type/${contextMenu.row.TypeID}`,
                  "_blank",
                );
                setContextMenu(null);
              }}
            />
            <ContextItem
              label={t("openInJitaSpace")}
              onClick={() => {
                window.open(
                  `https://www.jita.space/market/${contextMenu.row.TypeID}`,
                  "_blank",
                );
                setContextMenu(null);
              }}
            />
            <div className="h-px bg-eve-border my-1" />
            <ContextItem
              label={
                watchlistIds.has(contextMenu.row.TypeID)
                  ? t("untrackItem")
                  : `\u2B50 ${t("trackItem")}`
              }
              onClick={() => {
                const row = contextMenu.row;
                if (watchlistIds.has(row.TypeID)) {
                  removeFromWatchlist(row.TypeID)
                    .then(setWatchlist)
                    .then(() =>
                      addToast(t("watchlistRemoved"), "success", 2000),
                    )
                    .catch(() => addToast(t("watchlistError"), "error", 3000));
                } else {
                  addToWatchlist(row.TypeID, row.TypeName)
                    .then((r) => {
                      setWatchlist(r.items);
                      addToast(
                        r.inserted
                          ? t("watchlistItemAdded")
                          : t("watchlistAlready"),
                        r.inserted ? "success" : "info",
                        2000,
                      );
                    })
                    .catch(() => addToast(t("watchlistError"), "error", 3000));
                }
                setContextMenu(null);
              }}
            />
            {(contextMenu.row.BuyRegionID != null ||
              contextMenu.row.SellRegionID != null) && (
              <ContextItem
                label={t("placeDraft")}
                onClick={() => {
                  setExecPlanRow(contextMenu.row);
                  setContextMenu(null);
                }}
              />
            )}
            {/* EVE UI actions */}
            {isLoggedIn && (
              <>
                <div className="h-px bg-eve-border my-1" />
                <ContextItem
                  label={`🎮 ${t("openMarket")}`}
                  onClick={async () => {
                    try {
                      await openMarketInGame(contextMenu.row.TypeID);
                      addToast(t("actionSuccess"), "success", 2000);
                    } catch (err: any) {
                      const { messageKey, duration } = handleEveUIError(err);
                      addToast(t(messageKey), "error", duration);
                    }
                    setContextMenu(null);
                  }}
                />
                <ContextItem
                  label={`🎯 ${t("setDestination")} (Buy)`}
                  onClick={async () => {
                    try {
                      await setWaypointInGame(contextMenu.row.BuySystemID);
                      addToast(t("actionSuccess"), "success", 2000);
                    } catch (err: any) {
                      const { messageKey, duration } = handleEveUIError(err);
                      addToast(t(messageKey), "error", duration);
                    }
                    setContextMenu(null);
                  }}
                />
                {contextMenu.row.SellSystemID !== contextMenu.row.BuySystemID && (
                  <ContextItem
                    label={`🎯 ${t("setDestination")} (Sell)`}
                    onClick={async () => {
                      try {
                        await setWaypointInGame(contextMenu.row.SellSystemID);
                        addToast(t("actionSuccess"), "success", 2000);
                      } catch (err: any) {
                        addToast(t("actionFailed").replace("{error}", err.message), "error", 3000);
                      }
                      setContextMenu(null);
                    }}
                  />
                )}
              </>
            )}
            <div className="h-px bg-eve-border my-1" />
            <ContextItem
              label={
                pinnedIds.has(contextMenu.id) ? t("unpinRow") : t("pinRow")
              }
              onClick={() => {
                togglePin(contextMenu.id);
                setContextMenu(null);
              }}
            />
          </div>
        </>
      )}

      <ExecutionPlannerPopup
        open={execPlanRow !== null}
        onClose={() => setExecPlanRow(null)}
        typeID={execPlanRow?.TypeID ?? 0}
        typeName={execPlanRow?.TypeName ?? ""}
        regionID={execPlanRow?.BuyRegionID ?? 0}
        locationID={execPlanRow?.BuyLocationID ?? 0}
        sellRegionID={execPlanRow?.SellRegionID}
        sellLocationID={execPlanRow?.SellLocationID ?? 0}
        defaultQuantity={execPlanRow?.UnitsToBuy ?? 100}
        brokerFeePercent={brokerFeePercent}
        salesTaxPercent={salesTaxPercent}
        splitTradeFees={splitTradeFees}
        buyBrokerFeePercent={buyBrokerFeePercent}
        sellBrokerFeePercent={sellBrokerFeePercent}
        buySalesTaxPercent={buySalesTaxPercent}
        sellSalesTaxPercent={sellSalesTaxPercent}
      />
    </div>
  );
}

/* ─── Small reusable pieces ─── */

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
      className={`px-2 py-0.5 rounded-sm text-xs font-medium transition-colors cursor-pointer ${
        active
          ? "bg-eve-accent/20 text-eve-accent border border-eve-accent/30"
          : "text-eve-dim hover:text-eve-text border border-eve-border hover:border-eve-border-light"
      }`}
    >
      {label}
    </button>
  );
}

function ContextItem({
  label,
  onClick,
}: {
  label: string;
  onClick: () => void;
}) {
  return (
    <div
      onClick={onClick}
      className="px-4 py-1.5 text-sm text-eve-text hover:bg-eve-accent/20 hover:text-eve-accent cursor-pointer transition-colors"
    >
      {label}
    </div>
  );
}
