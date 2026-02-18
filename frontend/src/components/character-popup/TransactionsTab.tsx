import { useMemo, useState } from "react";
import { type TranslationKey } from "../../lib/i18n";
import type { WalletTransaction } from "../../lib/types";
import { FilterBtn } from "./shared";
interface TransactionsTabProps {
  transactions: WalletTransaction[];
  formatIsk: (v: number) => string;
  formatDate: (d: string) => string;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}

export function TransactionsTab({ transactions, formatIsk, formatDate, t }: TransactionsTabProps) {
  const [filter, setFilter] = useState<"all" | "buy" | "sell">("all");
  const [search, setSearch] = useState("");
  const [visibleCount, setVisibleCount] = useState(100);

  const sorted = useMemo(() =>
    [...transactions].sort((a, b) => new Date(b.date).getTime() - new Date(a.date).getTime()),
    [transactions]
  );

  const filtered = useMemo(() => {
    let items = sorted;
    if (filter === "buy") items = items.filter((tx) => tx.is_buy);
    if (filter === "sell") items = items.filter((tx) => !tx.is_buy);
    if (search.trim()) {
      const q = search.toLowerCase();
      items = items.filter((tx) => (tx.type_name || "").toLowerCase().includes(q));
    }
    return items;
  }, [sorted, filter, search]);

  if (transactions.length === 0) {
    return <div className="text-center text-eve-dim py-8">{t("charNoTransactions")}</div>;
  }

  return (
    <div className="space-y-3">
      {/* Filter + Search */}
      <div className="flex flex-wrap gap-2 items-center">
        <FilterBtn active={filter === "all"} onClick={() => setFilter("all")} label={t("charAll")} count={transactions.length} />
        <FilterBtn active={filter === "buy"} onClick={() => setFilter("buy")} label={t("charBuy")} count={transactions.filter((t) => t.is_buy).length} color="text-eve-profit" />
        <FilterBtn active={filter === "sell"} onClick={() => setFilter("sell")} label={t("charSell")} count={transactions.filter((t) => !t.is_buy).length} color="text-eve-error" />
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
              <th className="px-3 py-2 text-left">{t("charOrderType")}</th>
              <th className="px-3 py-2 text-left">{t("colItemName")}</th>
              <th className="px-3 py-2 text-right">{t("charUnitPrice")}</th>
              <th className="px-3 py-2 text-right">{t("charQty")}</th>
              <th className="px-3 py-2 text-right">{t("charTotal")}</th>
              <th className="px-3 py-2 text-left">{t("charLocation")}</th>
              <th className="px-3 py-2 text-left">{t("charDate")}</th>
            </tr>
          </thead>
          <tbody>
            {filtered.slice(0, visibleCount).map((tx) => (
              <tr key={tx.transaction_id} className="border-t border-eve-border/50 hover:bg-eve-panel/50">
                <td className="px-3 py-2">
                  <span className={`inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-medium ${
                    tx.is_buy ? "bg-eve-profit/20 text-eve-profit" : "bg-eve-error/20 text-eve-error"
                  }`}>
                    {tx.is_buy ? "BUY" : "SELL"}
                  </span>
                </td>
                <td className="px-3 py-2 text-eve-text">
                  <div className="flex items-center gap-2">
                    <img
                      src={`https://images.evetech.net/types/${tx.type_id}/icon?size=32`}
                      alt=""
                      className="w-5 h-5"
                    />
                    {tx.type_name || `Type #${tx.type_id}`}
                  </div>
                </td>
                <td className="px-3 py-2 text-right text-eve-accent">{formatIsk(tx.unit_price)}</td>
                <td className="px-3 py-2 text-right text-eve-dim">{tx.quantity.toLocaleString()}</td>
                <td className="px-3 py-2 text-right text-eve-text">{formatIsk(tx.unit_price * tx.quantity)}</td>
                <td className="px-3 py-2 text-eve-dim text-[11px] max-w-[180px] truncate" title={tx.location_name}>
                  {tx.location_name || `#${tx.location_id}`}
                </td>
                <td className="px-3 py-2 text-eve-dim text-[11px]">{formatDate(tx.date)}</td>
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
