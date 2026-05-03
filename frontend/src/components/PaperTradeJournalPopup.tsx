import { useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
import {
  createPaperTrade,
  deletePaperTrade,
  getCharacterInfo,
  getPaperTrades,
  reconcilePaperTrades,
  updatePaperTrade,
} from "@/lib/api";
import type { CharacterInfo, CharacterOrder, PaperTrade, PaperTradeCreatePayload, PaperTradePatch, PaperTradeReconcileRow, PaperTradeReconcileSummary, PaperTradeStatus } from "@/lib/types";
import { formatISK, formatMargin } from "@/lib/format";
import { Modal } from "./Modal";
import { useGlobalToast } from "./Toast";

type StatusFilter = "active" | "all" | PaperTradeStatus;

interface Props {
  open: boolean;
  onClose: () => void;
}

type Draft = {
  actual_quantity: string;
  actual_buy_price: string;
  actual_sell_price: string;
  fees_isk: string;
  hauling_cost_isk: string;
  notes: string;
};

type NewTradeDraft = {
  status: PaperTradeStatus;
  type_id: string;
  type_name: string;
  planned_quantity: string;
  planned_buy_price: string;
  planned_sell_price: string;
  fees_isk: string;
  hauling_cost_isk: string;
  buy_station: string;
  sell_station: string;
  buy_system_name: string;
  sell_system_name: string;
  volume_m3: string;
  notes: string;
  source: string;
};

type LiveTradeDraft = {
  id: string;
  source: "wallet_buy" | "active_buy_order" | "active_sell_order";
  label: string;
  detail: string;
  draft: NewTradeDraft;
};

const activeStatuses = new Set(["planned", "bought", "hauled"]);
const tradeStatuses: PaperTradeStatus[] = ["planned", "bought", "hauled", "sold", "cancelled"];

function emptyNewTradeDraft(): NewTradeDraft {
  return {
    status: "planned",
    type_id: "",
    type_name: "",
    planned_quantity: "1",
    planned_buy_price: "0",
    planned_sell_price: "0",
    fees_isk: "0",
    hauling_cost_isk: "0",
    buy_station: "",
    sell_station: "",
    buy_system_name: "",
    sell_system_name: "",
    volume_m3: "0",
    notes: "",
    source: "manual",
  };
}

function draftFromTrade(trade: PaperTrade): Draft {
  return {
    actual_quantity: String(trade.actual_quantity || trade.planned_quantity || 0),
    actual_buy_price: String(trade.actual_buy_price || trade.planned_buy_price || 0),
    actual_sell_price: String(trade.actual_sell_price || trade.planned_sell_price || 0),
    fees_isk: String(trade.fees_isk || 0),
    hauling_cost_isk: String(trade.hauling_cost_isk || 0),
    notes: trade.notes || "",
  };
}

function parseDraftNumber(value: string): number {
  const n = Number(String(value).replace(",", "."));
  return Number.isFinite(n) ? n : 0;
}

function statusTone(status: string): string {
  switch (status) {
    case "sold":
      return "border-green-500/40 text-green-300 bg-green-950/20";
    case "cancelled":
      return "border-red-500/40 text-red-300 bg-red-950/20";
    case "hauled":
      return "border-blue-400/40 text-blue-300 bg-blue-950/20";
    case "bought":
      return "border-eve-accent/40 text-eve-accent bg-eve-accent/10";
    default:
      return "border-eve-border text-eve-dim bg-eve-dark";
  }
}

function statusLabel(status: string): string {
  switch (status) {
    case "planned":
      return "Planned";
    case "bought":
      return "Bought";
    case "hauled":
      return "Hauled";
    case "sold":
      return "Sold";
    case "cancelled":
      return "Cancelled";
    default:
      return status;
  }
}

function shortDate(value: string): string {
  if (!value) return "-";
  const ts = Date.parse(value);
  if (!Number.isFinite(ts)) return value;
  return new Date(ts).toLocaleString();
}

function buildLiveTradeDrafts(data: CharacterInfo, existingTrades: PaperTrade[] = []): LiveTradeDraft[] {
  const orders = Array.isArray(data.orders) ? data.orders : [];
  const txns = Array.isArray(data.transactions) ? data.transactions : [];
  const existingSources = new Set(existingTrades.map((trade) => trade.source).filter(Boolean));
  const activeSellsByType = new Map<number, CharacterOrder>();

  for (const order of orders) {
    if (order.is_buy_order || order.type_id <= 0 || order.volume_remain <= 0) continue;
    const current = activeSellsByType.get(order.type_id);
    if (!current || order.price < current.price) {
      activeSellsByType.set(order.type_id, order);
    }
  }

  const out: LiveTradeDraft[] = [];
  const recentBuys = [...txns]
    .filter((tx) => tx.is_buy && tx.type_id > 0 && tx.quantity > 0)
    .sort((a, b) => Date.parse(b.date || "") - Date.parse(a.date || ""))
    .slice(0, 20);

  for (const tx of recentBuys) {
    const source = `live_txn:${tx.transaction_id}`;
    if (existingSources.has(source)) continue;
    const sell = activeSellsByType.get(tx.type_id);
    const qty = Math.max(1, Math.floor(tx.quantity));
    const buy = tx.unit_price || 0;
    const sellPrice = sell?.price || 0;
    const itemName = tx.type_name || `#${tx.type_id}`;
    out.push({
      id: `txn-${tx.transaction_id}`,
      source: "wallet_buy",
      label: `${itemName} x${qty.toLocaleString()}`,
      detail: `Bought ${shortDate(tx.date)} @ ${formatISK(buy)}${tx.location_name ? ` in ${tx.location_name}` : ""}`,
      draft: {
        ...emptyNewTradeDraft(),
        status: "bought",
        type_id: String(tx.type_id),
        type_name: itemName,
        planned_quantity: String(qty),
        planned_buy_price: String(buy),
        planned_sell_price: String(sellPrice),
        buy_station: tx.location_name || "",
        sell_station: sell?.location_name || "",
        notes: `Live wallet buy ${shortDate(tx.date)}; txn #${tx.transaction_id}`,
        source,
      },
    });
  }

  const activeBuys = orders
    .filter((order) => order.is_buy_order && order.type_id > 0 && order.volume_remain > 0)
    .sort((a, b) => Date.parse(b.issued || "") - Date.parse(a.issued || ""))
    .slice(0, 20);

  for (const order of activeBuys) {
    const source = `live_buy_order:${order.order_id}`;
    if (existingSources.has(source)) continue;
    const sell = activeSellsByType.get(order.type_id);
    const qty = Math.max(1, Math.floor(order.volume_remain));
    const itemName = order.type_name || `#${order.type_id}`;
    out.push({
      id: `buy-order-${order.order_id}`,
      source: "active_buy_order",
      label: `${itemName} x${qty.toLocaleString()}`,
      detail: `Active buy @ ${formatISK(order.price)}${order.location_name ? ` in ${order.location_name}` : ""}`,
      draft: {
        ...emptyNewTradeDraft(),
        status: "planned",
        type_id: String(order.type_id),
        type_name: itemName,
        planned_quantity: String(qty),
        planned_buy_price: String(order.price || 0),
        planned_sell_price: String(sell?.price || 0),
        buy_station: order.location_name || "",
        sell_station: sell?.location_name || "",
        notes: `Live active buy order #${order.order_id}; issued ${shortDate(order.issued)}`,
        source,
      },
    });
  }

  const activeSells = orders
    .filter((order) => !order.is_buy_order && order.type_id > 0 && order.volume_remain > 0)
    .sort((a, b) => Date.parse(b.issued || "") - Date.parse(a.issued || ""))
    .slice(0, 20);

  for (const order of activeSells) {
    const source = `live_sell_order:${order.order_id}`;
    if (existingSources.has(source)) continue;
    const qty = Math.max(1, Math.floor(order.volume_remain));
    const itemName = order.type_name || `#${order.type_id}`;
    out.push({
      id: `sell-order-${order.order_id}`,
      source: "active_sell_order",
      label: `${itemName} x${qty.toLocaleString()}`,
      detail: `Active sell @ ${formatISK(order.price)}${order.location_name ? ` in ${order.location_name}` : ""}`,
      draft: {
        ...emptyNewTradeDraft(),
        status: "hauled",
        type_id: String(order.type_id),
        type_name: itemName,
        planned_quantity: String(qty),
        planned_buy_price: "0",
        planned_sell_price: String(order.price || 0),
        sell_station: order.location_name || "",
        notes: `Live active sell order #${order.order_id}; fill buy price before closing`,
        source,
      },
    });
  }

  return out;
}

function liveDraftSourceLabel(source: LiveTradeDraft["source"]): string {
  switch (source) {
    case "wallet_buy":
      return "wallet buy";
    case "active_buy_order":
      return "buy order";
    case "active_sell_order":
      return "sell order";
    default:
      return source;
  }
}

export function PaperTradeJournalPopup({ open, onClose }: Props) {
  const { addToast } = useGlobalToast();
  const [filter, setFilter] = useState<StatusFilter>("active");
  const [trades, setTrades] = useState<PaperTrade[]>([]);
  const [drafts, setDrafts] = useState<Record<number, Draft>>({});
  const [loading, setLoading] = useState(false);
  const [reconciling, setReconciling] = useState(false);
  const [reconcileScope, setReconcileScope] = useState<"active" | "all">("active");
  const [reconcileRows, setReconcileRows] = useState<PaperTradeReconcileRow[]>([]);
  const [reconcileSummary, setReconcileSummary] = useState<PaperTradeReconcileSummary | null>(null);
  const [reconcileWarnings, setReconcileWarnings] = useState<string[]>([]);
  const [newTradeOpen, setNewTradeOpen] = useState(false);
  const [newTradeDraft, setNewTradeDraft] = useState<NewTradeDraft>(() => emptyNewTradeDraft());
  const [creatingTrade, setCreatingTrade] = useState(false);
  const [liveDrafts, setLiveDrafts] = useState<LiveTradeDraft[]>([]);
  const [liveDraftLoading, setLiveDraftLoading] = useState(false);
  const [busyID, setBusyID] = useState<number | null>(null);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    if (!open) return;
    setLoading(true);
    setError("");
    try {
      const data = await getPaperTrades({ status: filter, limit: 300 });
      setTrades(data.trades);
      const nextDrafts: Record<number, Draft> = {};
      for (const trade of data.trades) {
        nextDrafts[trade.id] = draftFromTrade(trade);
      }
      setDrafts(nextDrafts);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load paper trades");
    } finally {
      setLoading(false);
    }
  }, [filter, open]);

  useEffect(() => {
    void load();
  }, [load]);

  const reconcileByID = useMemo(() => {
    const out = new Map<number, PaperTradeReconcileRow>();
    for (const row of reconcileRows) out.set(row.trade_id, row);
    return out;
  }, [reconcileRows]);

  const summary = useMemo(() => {
    let active = 0;
    let expected = 0;
    let realized = 0;
    let capital = 0;
    for (const trade of trades) {
      if (activeStatuses.has(trade.status)) active += 1;
      expected += trade.expected_profit_isk || 0;
      realized += trade.realized_profit_isk || 0;
      capital += trade.capital_isk || 0;
    }
    return { active, expected, realized, capital, count: trades.length };
  }, [trades]);

  const patchDraft = useCallback((tradeID: number, key: keyof Draft, value: string) => {
    setDrafts((prev) => ({
      ...prev,
      [tradeID]: {
        ...(prev[tradeID] ?? {
          actual_quantity: "0",
          actual_buy_price: "0",
          actual_sell_price: "0",
          fees_isk: "0",
          hauling_cost_isk: "0",
          notes: "",
        }),
        [key]: value,
      },
    }));
  }, []);

  const patchNewTrade = useCallback((key: keyof NewTradeDraft, value: string) => {
    setNewTradeDraft((prev) => ({ ...prev, [key]: value }) as NewTradeDraft);
  }, []);

  const loadLiveDrafts = useCallback(async () => {
    if (liveDraftLoading) return;
    setLiveDraftLoading(true);
    setError("");
    try {
      const [data, existing] = await Promise.all([
        getCharacterInfo(reconcileScope === "all" ? "all" : undefined),
        getPaperTrades({ status: "all", limit: 1000 }),
      ]);
      const drafts = buildLiveTradeDrafts(data, existing.trades);
      setLiveDrafts(drafts);
      setNewTradeOpen(true);
      addToast(`Live drafts: ${drafts.length}`, drafts.length > 0 ? "success" : "info", 2000);
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Live draft fetch failed";
      setError(msg);
      addToast(msg, "error", 3200);
    } finally {
      setLiveDraftLoading(false);
    }
  }, [addToast, liveDraftLoading, reconcileScope]);

  const useLiveDraft = useCallback((draft: LiveTradeDraft) => {
    setNewTradeDraft(draft.draft);
    setNewTradeOpen(true);
    addToast(`Draft loaded: ${draft.label}`, "success", 1600);
  }, [addToast]);

  const createManualTrade = useCallback(async () => {
    if (creatingTrade) return;
    const typeID = Math.floor(parseDraftNumber(newTradeDraft.type_id));
    const quantity = Math.floor(parseDraftNumber(newTradeDraft.planned_quantity));
    const buy = parseDraftNumber(newTradeDraft.planned_buy_price);
    const sell = parseDraftNumber(newTradeDraft.planned_sell_price);
    const typeName = newTradeDraft.type_name.trim();
    if (typeID <= 0 || !typeName || quantity <= 0) {
      const msg = "Type ID, item name and quantity are required";
      setError(msg);
      addToast(msg, "error", 2600);
      return;
    }
    if (buy < 0 || sell < 0) {
      const msg = "Prices must be non-negative";
      setError(msg);
      addToast(msg, "error", 2600);
      return;
    }

    const status = tradeStatuses.includes(newTradeDraft.status) ? newTradeDraft.status : "planned";
    const plannedProfit = (sell - buy) * quantity;
    const capital = buy * quantity;
    const actualKnown = status === "bought" || status === "hauled" || status === "sold";
    const payload: PaperTradeCreatePayload = {
      status,
      type_id: typeID,
      type_name: typeName,
      planned_quantity: quantity,
      planned_buy_price: buy,
      planned_sell_price: sell,
      planned_profit_isk: plannedProfit,
      planned_roi_percent: capital > 0 ? (plannedProfit / capital) * 100 : 0,
      actual_quantity: actualKnown ? quantity : 0,
      actual_buy_price: actualKnown ? buy : 0,
      actual_sell_price: status === "sold" ? sell : 0,
      fees_isk: parseDraftNumber(newTradeDraft.fees_isk),
      hauling_cost_isk: parseDraftNumber(newTradeDraft.hauling_cost_isk),
      buy_station: newTradeDraft.buy_station.trim(),
      sell_station: newTradeDraft.sell_station.trim(),
      buy_system_name: newTradeDraft.buy_system_name.trim(),
      sell_system_name: newTradeDraft.sell_system_name.trim(),
      volume_m3: parseDraftNumber(newTradeDraft.volume_m3),
      notes: newTradeDraft.notes.trim(),
      source: newTradeDraft.source.trim() || "manual",
    };

    setCreatingTrade(true);
    setError("");
    try {
      const res = await createPaperTrade(payload);
      setNewTradeDraft(emptyNewTradeDraft());
      setNewTradeOpen(false);
      setReconcileRows([]);
      setReconcileSummary(null);
      setReconcileWarnings([]);
      addToast(`Trade added: ${res.trade.type_name}`, "success", 2200);
      await load();
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Paper trade create failed";
      setError(msg);
      addToast(msg, "error", 3200);
    } finally {
      setCreatingTrade(false);
    }
  }, [addToast, creatingTrade, load, newTradeDraft]);

  const patchTrade = useCallback(
    async (trade: PaperTrade, patch: PaperTradePatch, success: string): Promise<PaperTrade | null> => {
      if (busyID != null) return null;
      setBusyID(trade.id);
      setError("");
      try {
        const res = await updatePaperTrade(trade.id, patch);
        setTrades((prev) =>
          prev.map((item) => (item.id === trade.id ? res.trade : item)),
        );
        setDrafts((prev) => ({ ...prev, [trade.id]: draftFromTrade(res.trade) }));
        addToast(success, "success", 1800);
        return res.trade;
      } catch (e) {
        const msg = e instanceof Error ? e.message : "Paper trade update failed";
        setError(msg);
        addToast(msg, "error", 3000);
        return null;
      } finally {
        setBusyID(null);
      }
    },
    [addToast, busyID],
  );

  const runLiveReconcile = useCallback(async () => {
    if (reconciling) return;
    setReconciling(true);
    setError("");
    setReconcileWarnings([]);
    try {
      const data = await reconcilePaperTrades({
        status: filter,
        limit: 300,
        scope: reconcileScope,
      });
      setReconcileRows(data.rows);
      setReconcileSummary(data.summary);
      setReconcileWarnings(data.warnings ?? []);
      addToast(`Live sync: ${data.summary.matched}/${data.summary.trades_checked} matched`, "success", 2200);
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Live reconciliation failed";
      setError(msg);
      addToast(msg, "error", 3200);
    } finally {
      setReconciling(false);
    }
  }, [addToast, filter, reconcileScope, reconciling]);

  const applyLivePatch = useCallback(
    async (trade: PaperTrade, row: PaperTradeReconcileRow) => {
      if (!row.suggested_patch) return;
      const updated = await patchTrade(trade, row.suggested_patch, "Live fact applied");
      if (!updated) return;
      setReconcileRows((prev) =>
        prev.map((item) =>
          item.trade_id === trade.id
            ? {
                ...item,
                suggested_status: updated.status,
                suggested_patch: null,
                reason: "Live reconciliation applied",
              }
            : item,
        ),
      );
      if (filter === "active" && (updated.status === "sold" || updated.status === "cancelled")) {
        await load();
      }
    },
    [filter, load, patchTrade],
  );

  const saveTrade = useCallback(
    async (trade: PaperTrade) => {
      const draft = drafts[trade.id] ?? draftFromTrade(trade);
      await patchTrade(
        trade,
        {
          actual_quantity: Math.floor(parseDraftNumber(draft.actual_quantity)),
          actual_buy_price: parseDraftNumber(draft.actual_buy_price),
          actual_sell_price: parseDraftNumber(draft.actual_sell_price),
          fees_isk: parseDraftNumber(draft.fees_isk),
          hauling_cost_isk: parseDraftNumber(draft.hauling_cost_isk),
          notes: draft.notes,
        },
        "Paper trade saved",
      );
    },
    [drafts, patchTrade],
  );

  const setStatus = useCallback(
    async (trade: PaperTrade, status: PaperTradeStatus) => {
      const draft = drafts[trade.id] ?? draftFromTrade(trade);
      await patchTrade(
        trade,
        {
          status,
          actual_quantity: Math.floor(parseDraftNumber(draft.actual_quantity)) || trade.planned_quantity,
          actual_buy_price: parseDraftNumber(draft.actual_buy_price) || trade.planned_buy_price,
          actual_sell_price: parseDraftNumber(draft.actual_sell_price) || trade.planned_sell_price,
          fees_isk: parseDraftNumber(draft.fees_isk),
          hauling_cost_isk: parseDraftNumber(draft.hauling_cost_isk),
          notes: draft.notes,
        },
        `Status: ${statusLabel(status)}`,
      );
      if (filter === "active" && (status === "sold" || status === "cancelled")) {
        await load();
      }
    },
    [drafts, filter, load, patchTrade],
  );

  const removeTrade = useCallback(
    async (trade: PaperTrade) => {
      if (busyID != null) return;
      setBusyID(trade.id);
      try {
        await deletePaperTrade(trade.id);
        setTrades((prev) => prev.filter((item) => item.id !== trade.id));
        addToast("Paper trade deleted", "success", 1800);
      } catch (e) {
        const msg = e instanceof Error ? e.message : "Paper trade delete failed";
        setError(msg);
        addToast(msg, "error", 3000);
      } finally {
        setBusyID(null);
      }
    },
    [addToast, busyID],
  );

  return (
    <Modal open={open} onClose={onClose} title="Paper Trade Journal" width="max-w-7xl">
      <div className="p-4 space-y-3 text-xs text-eve-text">
        <div className="flex flex-wrap items-center gap-2">
          {(["active", "all", "planned", "bought", "hauled", "sold", "cancelled"] as StatusFilter[]).map((item) => (
            <button
              key={item}
              type="button"
              onClick={() => setFilter(item)}
              className={`px-2 py-1 rounded-sm border uppercase tracking-wide transition-colors ${
                filter === item
                  ? "border-eve-accent text-eve-accent bg-eve-accent/10"
                  : "border-eve-border/70 text-eve-dim hover:border-eve-accent/50 hover:text-eve-text"
              }`}
            >
              {item === "active" ? "Active" : item === "all" ? "All" : statusLabel(item)}
            </button>
          ))}
          <button
            type="button"
            onClick={() => void load()}
            disabled={loading}
            className="ml-auto px-2 py-1 rounded-sm border border-eve-border/70 text-eve-dim hover:border-eve-accent/50 hover:text-eve-accent disabled:opacity-50"
          >
            {loading ? "Loading..." : "Refresh"}
          </button>
          <button
            type="button"
            onClick={() => setNewTradeOpen((v) => !v)}
            className={`px-2 py-1 rounded-sm border uppercase tracking-wide ${
              newTradeOpen
                ? "border-eve-accent text-eve-accent bg-eve-accent/10"
                : "border-eve-border/70 text-eve-dim hover:border-eve-accent/50 hover:text-eve-accent"
            }`}
          >
            New trade
          </button>
          <button
            type="button"
            onClick={() => void loadLiveDrafts()}
            disabled={liveDraftLoading}
            className="px-2 py-1 rounded-sm border border-eve-border/70 text-eve-dim hover:border-eve-accent/50 hover:text-eve-accent disabled:opacity-50"
          >
            {liveDraftLoading ? "Loading drafts..." : "Live drafts"}
          </button>
          <select
            value={reconcileScope}
            onChange={(e) => setReconcileScope(e.target.value === "all" ? "all" : "active")}
            className="h-7 px-2 rounded-sm border border-eve-border bg-eve-input text-eve-dim"
          >
            <option value="active">Active char</option>
            <option value="all">All chars</option>
          </select>
          <button
            type="button"
            onClick={() => void runLiveReconcile()}
            disabled={reconciling}
            className="px-2 py-1 rounded-sm border border-eve-accent/60 text-eve-accent hover:bg-eve-accent/10 disabled:opacity-50"
          >
            {reconciling ? "Syncing..." : "Live sync"}
          </button>
        </div>

        {newTradeOpen && (
          <div className="border border-eve-border rounded-sm bg-eve-dark/50 p-3 space-y-3">
            {liveDrafts.length > 0 && (
              <div className="border border-eve-border/70 rounded-sm overflow-hidden">
                <div className="px-2 py-1 bg-eve-panel text-[10px] uppercase tracking-wide text-eve-dim">
                  Live candidates
                </div>
                <div className="max-h-36 overflow-auto eve-scrollbar divide-y divide-eve-border/40">
                  {liveDrafts.slice(0, 30).map((item) => (
                    <button
                      key={item.id}
                      type="button"
                      onClick={() => useLiveDraft(item)}
                      className="w-full px-2 py-1.5 text-left hover:bg-eve-panel/70 transition-colors"
                    >
                      <div className="flex items-center justify-between gap-2">
                        <span className="text-eve-text truncate">{item.label}</span>
                        <span className="text-[10px] text-eve-accent uppercase">{liveDraftSourceLabel(item.source)}</span>
                      </div>
                      <div className="text-[10px] text-eve-dim truncate">{item.detail}</div>
                    </button>
                  ))}
                </div>
              </div>
            )}
            <div className="grid grid-cols-2 md:grid-cols-4 xl:grid-cols-6 gap-2">
              <SelectInput
                label="Status"
                value={newTradeDraft.status}
                onChange={(v) => patchNewTrade("status", v)}
                options={tradeStatuses.map((item) => [item, statusLabel(item)] as const)}
              />
              <TextInput label="Type ID" value={newTradeDraft.type_id} onChange={(v) => patchNewTrade("type_id", v)} />
              <TextInput label="Item" value={newTradeDraft.type_name} onChange={(v) => patchNewTrade("type_name", v)} className="xl:col-span-2" />
              <TextInput label="Qty" value={newTradeDraft.planned_quantity} onChange={(v) => patchNewTrade("planned_quantity", v)} />
              <TextInput label="Volume m3" value={newTradeDraft.volume_m3} onChange={(v) => patchNewTrade("volume_m3", v)} />
              <TextInput label="Buy price" value={newTradeDraft.planned_buy_price} onChange={(v) => patchNewTrade("planned_buy_price", v)} />
              <TextInput label="Sell price" value={newTradeDraft.planned_sell_price} onChange={(v) => patchNewTrade("planned_sell_price", v)} />
              <TextInput label="Fees" value={newTradeDraft.fees_isk} onChange={(v) => patchNewTrade("fees_isk", v)} />
              <TextInput label="Hauling" value={newTradeDraft.hauling_cost_isk} onChange={(v) => patchNewTrade("hauling_cost_isk", v)} />
              <TextInput label="Buy station" value={newTradeDraft.buy_station} onChange={(v) => patchNewTrade("buy_station", v)} />
              <TextInput label="Sell station" value={newTradeDraft.sell_station} onChange={(v) => patchNewTrade("sell_station", v)} />
              <TextInput label="Buy system" value={newTradeDraft.buy_system_name} onChange={(v) => patchNewTrade("buy_system_name", v)} />
              <TextInput label="Sell system" value={newTradeDraft.sell_system_name} onChange={(v) => patchNewTrade("sell_system_name", v)} />
              <TextInput label="Notes" value={newTradeDraft.notes} onChange={(v) => patchNewTrade("notes", v)} className="md:col-span-2 xl:col-span-4" />
            </div>
            <div className="flex items-center justify-between gap-2">
              <div className="font-mono text-[10px] text-eve-dim">
                Est {formatISK((parseDraftNumber(newTradeDraft.planned_sell_price) - parseDraftNumber(newTradeDraft.planned_buy_price)) * Math.max(0, Math.floor(parseDraftNumber(newTradeDraft.planned_quantity))))}
              </div>
              <div className="flex gap-2">
                <button
                  type="button"
                  onClick={() => setNewTradeDraft(emptyNewTradeDraft())}
                  disabled={creatingTrade}
                  className="px-2 py-1 rounded-sm border border-eve-border/70 text-eve-dim hover:border-eve-accent/50 hover:text-eve-text disabled:opacity-50"
                >
                  Reset
                </button>
                <button
                  type="button"
                  onClick={() => void createManualTrade()}
                  disabled={creatingTrade}
                  className="px-3 py-1 rounded-sm border border-eve-accent/60 bg-eve-accent/10 text-eve-accent hover:bg-eve-accent/20 disabled:opacity-50"
                >
                  {creatingTrade ? "Adding..." : "Add trade"}
                </button>
              </div>
            </div>
          </div>
        )}

        <div className="grid grid-cols-2 md:grid-cols-6 gap-2">
          <Metric label="Rows" value={String(summary.count)} />
          <Metric label="Active" value={String(summary.active)} />
          <Metric label="Capital" value={formatISK(summary.capital)} />
          <Metric label="Expected" value={formatISK(summary.expected)} tone={summary.expected >= 0 ? "profit" : "loss"} />
          <Metric label="Realized" value={formatISK(summary.realized)} tone={summary.realized >= 0 ? "profit" : "loss"} />
          <Metric
            label="Live matched"
            value={reconcileSummary ? `${reconcileSummary.matched}/${reconcileSummary.trades_checked}` : "-"}
          />
        </div>

        {reconcileWarnings.length > 0 && (
          <div className="border border-eve-border bg-eve-dark/70 text-eve-dim rounded-sm px-3 py-2">
            {reconcileWarnings.slice(0, 3).join(" | ")}
            {reconcileWarnings.length > 3 ? ` | +${reconcileWarnings.length - 3}` : ""}
          </div>
        )}

        {error && (
          <div className="border border-red-500/50 bg-red-950/30 text-red-300 rounded-sm px-3 py-2">
            {error}
          </div>
        )}

        <div className="border border-eve-border rounded-sm overflow-auto max-h-[62vh] eve-scrollbar">
          <table className="w-full min-w-[1440px]">
            <thead className="sticky top-0 bg-eve-panel text-eve-dim uppercase tracking-wide text-[10px]">
              <tr>
                <th className="px-2 py-1 text-left">Status</th>
                <th className="px-2 py-1 text-left">Item / route</th>
                <th className="px-2 py-1 text-right">Qty</th>
                <th className="px-2 py-1 text-right">Buy</th>
                <th className="px-2 py-1 text-right">Sell</th>
                <th className="px-2 py-1 text-right">Costs</th>
                <th className="px-2 py-1 text-right">Expected</th>
                <th className="px-2 py-1 text-right">Realized</th>
                <th className="px-2 py-1 text-left">Live</th>
                <th className="px-2 py-1 text-left">Notes</th>
                <th className="px-2 py-1 text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {trades.map((trade) => {
                const draft = drafts[trade.id] ?? draftFromTrade(trade);
                const busy = busyID === trade.id;
                const live = reconcileByID.get(trade.id);
                return (
                  <tr key={trade.id} className="border-t border-eve-border/40 align-top">
                    <td className="px-2 py-2">
                      <span className={`inline-flex px-1.5 py-0.5 rounded-sm border text-[10px] uppercase tracking-wide ${statusTone(trade.status)}`}>
                        {statusLabel(trade.status)}
                      </span>
                      <div className="mt-1 text-[10px] text-eve-dim font-mono">{shortDate(trade.updated_at)}</div>
                    </td>
                    <td className="px-2 py-2 min-w-[260px]">
                      <div className="text-eve-text">{trade.type_name}</div>
                      <div className="text-eve-dim mt-0.5 truncate">
                        {trade.buy_station || trade.buy_system_name || "-"} {"->"} {trade.sell_station || trade.sell_system_name || "-"}
                      </div>
                      <div className="text-[10px] text-eve-dim mt-0.5">
                        Plan {trade.planned_quantity.toLocaleString()} @ {formatISK(trade.planned_buy_price)} {"->"} {formatISK(trade.planned_sell_price)}
                      </div>
                    </td>
                    <td className="px-2 py-2 text-right">
                      <NumberInput
                        value={draft.actual_quantity}
                        onChange={(v) => patchDraft(trade.id, "actual_quantity", v)}
                      />
                    </td>
                    <td className="px-2 py-2 text-right">
                      <NumberInput
                        value={draft.actual_buy_price}
                        onChange={(v) => patchDraft(trade.id, "actual_buy_price", v)}
                      />
                    </td>
                    <td className="px-2 py-2 text-right">
                      <NumberInput
                        value={draft.actual_sell_price}
                        onChange={(v) => patchDraft(trade.id, "actual_sell_price", v)}
                      />
                    </td>
                    <td className="px-2 py-2 text-right">
                      <div className="flex flex-col gap-1">
                        <NumberInput
                          value={draft.fees_isk}
                          onChange={(v) => patchDraft(trade.id, "fees_isk", v)}
                          title="Fees"
                        />
                        <NumberInput
                          value={draft.hauling_cost_isk}
                          onChange={(v) => patchDraft(trade.id, "hauling_cost_isk", v)}
                          title="Hauling"
                        />
                      </div>
                    </td>
                    <td className={`px-2 py-2 text-right font-mono ${trade.expected_profit_isk >= 0 ? "text-green-400" : "text-red-300"}`}>
                      <div>{formatISK(trade.expected_profit_isk)}</div>
                      <div className="text-[10px] text-eve-dim">{formatMargin(trade.planned_roi_percent || trade.roi_percent || 0)}</div>
                    </td>
                    <td className={`px-2 py-2 text-right font-mono ${trade.realized_profit_isk >= 0 ? "text-green-400" : "text-red-300"}`}>
                      <div>{formatISK(trade.realized_profit_isk)}</div>
                      <div className="text-[10px] text-eve-dim">{formatMargin(trade.status === "sold" ? trade.roi_percent : 0)}</div>
                    </td>
                    <td className="px-2 py-2 min-w-[180px]">
                      {live ? <LiveReconcileCell row={live} /> : <span className="text-eve-dim">-</span>}
                    </td>
                    <td className="px-2 py-2">
                      <input
                        value={draft.notes}
                        onChange={(e) => patchDraft(trade.id, "notes", e.target.value)}
                        className="w-full min-w-[160px] h-7 px-2 rounded-sm border border-eve-border bg-eve-input text-eve-text"
                      />
                    </td>
                    <td className="px-2 py-2 text-right">
                      <div className="flex justify-end gap-1 flex-wrap">
                        <ActionButton disabled={busy} onClick={() => void saveTrade(trade)}>Save</ActionButton>
                        {live?.suggested_patch && (
                          <ActionButton disabled={busy} onClick={() => void applyLivePatch(trade, live)}>Apply live</ActionButton>
                        )}
                        {trade.status === "planned" && (
                          <ActionButton disabled={busy} onClick={() => void setStatus(trade, "bought")}>Bought</ActionButton>
                        )}
                        {(trade.status === "planned" || trade.status === "bought") && (
                          <ActionButton disabled={busy} onClick={() => void setStatus(trade, "hauled")}>Hauled</ActionButton>
                        )}
                        {trade.status !== "sold" && trade.status !== "cancelled" && (
                          <ActionButton disabled={busy} onClick={() => void setStatus(trade, "sold")}>Sold</ActionButton>
                        )}
                        {trade.status !== "cancelled" && trade.status !== "sold" && (
                          <ActionButton disabled={busy} danger onClick={() => void setStatus(trade, "cancelled")}>Cancel</ActionButton>
                        )}
                        <ActionButton disabled={busy} danger onClick={() => void removeTrade(trade)}>Delete</ActionButton>
                      </div>
                    </td>
                  </tr>
                );
              })}
              {!loading && trades.length === 0 && (
                <tr>
                  <td colSpan={11} className="px-3 py-12 text-center text-eve-dim">
                    No paper trades for this filter.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>
    </Modal>
  );
}

function LiveReconcileCell({ row }: { row: PaperTradeReconcileRow }) {
  const tone = confidenceTone(row.confidence);
  return (
    <div className="space-y-1">
      <div className="flex items-center gap-1">
        <span className={`inline-flex px-1.5 py-0.5 rounded-sm border text-[10px] uppercase tracking-wide ${tone}`}>
          {row.confidence === "none" ? "No match" : row.confidence}
        </span>
        {row.suggested_status && row.confidence !== "none" && (
          <span className="text-[10px] text-eve-dim uppercase">{statusLabel(row.suggested_status)}</span>
        )}
      </div>
      <div className="font-mono text-[10px] text-eve-dim">
        B {formatQty(row.matched_buy_qty)}
        {row.avg_buy_price > 0 ? ` @ ${formatISK(row.avg_buy_price)}` : ""} / S {formatQty(row.matched_sell_qty)}
        {row.avg_sell_price > 0 ? ` @ ${formatISK(row.avg_sell_price)}` : ""}
      </div>
      <div className="font-mono text-[10px] text-eve-dim">
        OB {formatQty(row.open_buy_qty)} / OS {formatQty(row.open_sell_qty)} / A {formatQty(row.asset_qty)}
      </div>
      <div className="text-[10px] text-eve-dim line-clamp-2">{row.reason}</div>
    </div>
  );
}

function confidenceTone(confidence: string): string {
  switch (confidence) {
    case "high":
      return "border-green-500/40 text-green-300 bg-green-950/20";
    case "medium":
      return "border-eve-accent/40 text-eve-accent bg-eve-accent/10";
    case "low":
      return "border-blue-400/40 text-blue-300 bg-blue-950/20";
    default:
      return "border-eve-border text-eve-dim bg-eve-dark";
  }
}

function formatQty(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return "0";
  return Math.floor(value).toLocaleString();
}

function NumberInput({
  value,
  onChange,
  title,
}: {
  value: string;
  onChange: (value: string) => void;
  title?: string;
}) {
  return (
    <input
      type="number"
      value={value}
      title={title}
      onChange={(e) => onChange(e.target.value)}
      className="w-28 h-7 px-2 rounded-sm border border-eve-border bg-eve-input text-eve-text text-right font-mono"
    />
  );
}

function ActionButton({
  children,
  disabled,
  danger,
  onClick,
}: {
  children: ReactNode;
  disabled?: boolean;
  danger?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className={`px-2 py-1 rounded-sm border text-[10px] uppercase tracking-wide transition-colors disabled:opacity-40 disabled:cursor-not-allowed ${
        danger
          ? "border-red-500/50 text-red-300 hover:bg-red-500/10"
          : "border-eve-border/70 text-eve-dim hover:border-eve-accent/50 hover:text-eve-accent"
      }`}
    >
      {children}
    </button>
  );
}

function TextInput({
  label,
  value,
  onChange,
  className = "",
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  className?: string;
}) {
  return (
    <label className={`block ${className}`}>
      <span className="block text-[10px] uppercase tracking-wide text-eve-dim mb-1">{label}</span>
      <input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="w-full h-8 px-2 rounded-sm border border-eve-border bg-eve-input text-eve-text"
      />
    </label>
  );
}

function SelectInput({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: string;
  options: ReadonlyArray<readonly [string, string]>;
  onChange: (value: string) => void;
}) {
  return (
    <label className="block">
      <span className="block text-[10px] uppercase tracking-wide text-eve-dim mb-1">{label}</span>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="w-full h-8 px-2 rounded-sm border border-eve-border bg-eve-input text-eve-text"
      >
        {options.map(([optionValue, optionLabel]) => (
          <option key={optionValue} value={optionValue}>
            {optionLabel}
          </option>
        ))}
      </select>
    </label>
  );
}

function Metric({
  label,
  value,
  tone,
}: {
  label: string;
  value: string;
  tone?: "profit" | "loss";
}) {
  const color =
    tone === "profit" ? "text-green-400" : tone === "loss" ? "text-red-300" : "text-eve-text";
  return (
    <div className="border border-eve-border rounded-sm bg-eve-dark/60 px-3 py-2">
      <div className="text-[10px] uppercase tracking-wide text-eve-dim">{label}</div>
      <div className={`mt-1 font-mono text-sm ${color}`}>{value}</div>
    </div>
  );
}
