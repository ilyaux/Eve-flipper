import { useState } from "react";
import { getCorpJournal } from "../../lib/api";
import { type TranslationKey } from "../../lib/i18n";
import type { CorpJournalEntry, CorpWalletDivision } from "../../lib/types";
export function WalletsSection({
  wallets,
  mode,
  formatIsk,
  t,
}: {
  wallets: CorpWalletDivision[];
  mode: "demo" | "live";
  formatIsk: (v: number) => string;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}) {
  const totalBalance = wallets.reduce((s, w) => s + w.balance, 0);
  const maxBalance = Math.max(...wallets.map((w) => Math.abs(w.balance)), 1);
  const [expandedDiv, setExpandedDiv] = useState<number | null>(null);
  const [journal, setJournal] = useState<CorpJournalEntry[]>([]);
  const [journalLoading, setJournalLoading] = useState(false);

  const toggleDivision = (div: number) => {
    if (expandedDiv === div) {
      setExpandedDiv(null);
      return;
    }
    setExpandedDiv(div);
    setJournalLoading(true);
    getCorpJournal(mode, div, 30)
      .then(setJournal)
      .catch(() => setJournal([]))
      .finally(() => setJournalLoading(false));
  };

  return (
    <div className="space-y-4">
      <div className="bg-eve-panel border border-eve-border rounded-sm p-4">
        <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-1">{t("corpTotalBalance")}</div>
        <div className="text-2xl font-bold text-eve-accent">{formatIsk(totalBalance)} ISK</div>
      </div>
      <div className="space-y-2">
        {wallets.map((w) => {
          const pct = maxBalance > 0 ? (Math.abs(w.balance) / maxBalance) * 100 : 0;
          const isExpanded = expandedDiv === w.division;
          return (
            <div key={w.division}>
              <button
                onClick={() => toggleDivision(w.division)}
                className={`w-full bg-eve-panel border rounded-sm p-3 flex items-center gap-4 transition-colors hover:border-eve-accent/50 ${
                  isExpanded ? "border-eve-accent" : "border-eve-border"
                }`}
              >
                <div className="w-8 h-8 flex items-center justify-center bg-eve-accent/10 rounded-sm text-eve-accent text-sm font-bold">
                  {w.division}
                </div>
                <div className="flex-1 text-left">
                  <div className="flex items-center justify-between mb-1">
                    <span className="text-sm text-eve-text font-medium">{w.name}</span>
                    <span className="text-sm text-eve-accent font-bold">{formatIsk(w.balance)} ISK</span>
                  </div>
                  <div className="h-1.5 bg-eve-dark rounded-full overflow-hidden">
                    <div className="h-full bg-eve-accent/60 rounded-full" style={{ width: `${pct}%` }} />
                  </div>
                </div>
                <svg className={`w-4 h-4 text-eve-dim transition-transform ${isExpanded ? "rotate-180" : ""}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
                </svg>
              </button>
              {isExpanded && (
                <div className="bg-eve-dark/60 border border-eve-border border-t-0 rounded-b-sm p-3">
                  {journalLoading ? (
                    <div className="flex items-center justify-center py-4 text-eve-dim text-xs">
                      <span className="inline-block w-4 h-4 border-2 border-eve-accent/40 border-t-eve-accent rounded-full animate-spin mr-2" />
                      Loading journal...
                    </div>
                  ) : journal.length === 0 ? (
                    <div className="text-center text-eve-dim text-xs py-4">No journal entries</div>
                  ) : (
                    <div className="border border-eve-border rounded-sm overflow-hidden max-h-80 overflow-y-auto">
                      <table className="w-full text-xs">
                        <thead className="bg-eve-panel sticky top-0">
                          <tr className="text-eve-dim">
                            <th className="px-2 py-1.5 text-left">Date</th>
                            <th className="px-2 py-1.5 text-left">Type</th>
                            <th className="px-2 py-1.5 text-left">From</th>
                            <th className="px-2 py-1.5 text-right">Amount</th>
                            <th className="px-2 py-1.5 text-right">Balance</th>
                          </tr>
                        </thead>
                        <tbody>
                          {journal.slice(0, 50).map((j) => (
                            <tr key={j.id} className="border-t border-eve-border/30 hover:bg-eve-panel/50">
                              <td className="px-2 py-1.5 text-eve-dim whitespace-nowrap">
                                {j.date.slice(0, 10)}
                              </td>
                              <td className="px-2 py-1.5 text-eve-dim max-w-[140px] truncate" title={j.ref_type}>
                                {j.ref_type.replace(/_/g, " ")}
                              </td>
                              <td className="px-2 py-1.5 text-eve-text max-w-[120px] truncate" title={j.first_party_name}>
                                {j.first_party_name || "—"}
                              </td>
                              <td className={`px-2 py-1.5 text-right font-mono ${j.amount >= 0 ? "text-eve-profit" : "text-eve-error"}`}>
                                {j.amount >= 0 ? "+" : ""}{formatIsk(j.amount)}
                              </td>
                              <td className="px-2 py-1.5 text-right text-eve-dim font-mono">
                                {formatIsk(j.balance)}
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                      {journal.length > 50 && (
                        <div className="text-center text-eve-dim text-[10px] py-1 bg-eve-panel">
                          +{journal.length - 50} more entries
                        </div>
                      )}
                    </div>
                  )}
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

// ============================================================
// Members Section
// ============================================================

