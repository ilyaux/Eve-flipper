import { useCallback } from "react";
import { formatISK } from "@/lib/format";
import type {
  ExecutionRevalidationReport,
  ExecutionRevalidationRow,
  ExecutionRevalidationStatus,
} from "@/lib/types";
import { Modal } from "./Modal";
import { useGlobalToast } from "./Toast";

interface ExecutionRevalidationReportModalProps {
  open: boolean;
  report: ExecutionRevalidationReport | null;
  onClose: () => void;
}

function statusClass(status: ExecutionRevalidationStatus): string {
  switch (status) {
    case "SAFE":
      return "border-green-500/50 bg-green-950/30 text-green-300";
    case "CHANGED":
      return "border-yellow-500/50 bg-yellow-950/30 text-yellow-300";
    case "DANGER":
      return "border-red-500/50 bg-red-950/30 text-red-300";
    default:
      return "border-eve-border bg-eve-panel text-eve-dim";
  }
}

function signedIsk(value: number): string {
  const sign = value > 0 ? "+" : "";
  return `${sign}${formatISK(value)}`;
}

function finalStatus(report: ExecutionRevalidationReport): ExecutionRevalidationStatus {
  if (report.danger > 0) return "DANGER";
  if (report.changed > 0) return "CHANGED";
  return "SAFE";
}

function finalSignalText(status: ExecutionRevalidationStatus): string {
  switch (status) {
    case "SAFE":
      return "SAFE to undock from the current quote set.";
    case "CHANGED":
      return "CHANGED rows need review before undock.";
    case "DANGER":
      return "DANGER rows should be removed before undock.";
    default:
      return "Review revalidation before undock.";
  }
}

function moveClass(oldValue: number, newValue: number, higherIsBetter: boolean): string {
  if (newValue === oldValue) return "text-eve-dim";
  const improved = higherIsBetter ? newValue > oldValue : newValue < oldValue;
  return improved ? "text-green-300" : "text-red-300";
}

function copyableRows(report: ExecutionRevalidationReport): ExecutionRevalidationRow[] {
  return report.rows.filter((row) => !row.avoid && row.nowQty > 0);
}

export function ExecutionRevalidationReportModal({
  open,
  report,
  onClose,
}: ExecutionRevalidationReportModalProps) {
  const { addToast } = useGlobalToast();

  const copyReport = useCallback(async () => {
    if (!report) return;
    const lines: string[] = [];
    const signal = finalStatus(report);
    lines.push(`Revalidate before undock - ${new Date(report.createdAt).toLocaleString()}`);
    lines.push(`Final signal: ${signal}`);
    lines.push(`Safe: ${report.safe}, changed: ${report.changed}, danger: ${report.danger}`);
    lines.push(`Old profit: ${Math.round(report.totalOldProfit).toLocaleString()} ISK`);
    lines.push(`Now profit: ${Math.round(report.totalNowProfit).toLocaleString()} ISK`);
    lines.push(`Delta: ${Math.round(report.totalDeltaProfit).toLocaleString()} ISK`);
    lines.push("");
    for (const row of report.rows) {
      lines.push(
        `${row.status}\t${row.avoid ? "AVOID" : "OK"}\t${row.row.TypeName}\tqty ${row.oldQty}->${row.nowQty}\tbuy ${Math.round(row.oldBuy).toLocaleString()}->${Math.round(row.nowBuy).toLocaleString()}\tsell ${Math.round(row.oldSell).toLocaleString()}->${Math.round(row.nowSell).toLocaleString()}\tprofit ${Math.round(row.oldProfit).toLocaleString()}->${Math.round(row.nowProfit).toLocaleString()} ISK\t${row.reasons.join(", ")}`,
      );
    }
    await navigator.clipboard.writeText(lines.join("\n"));
    addToast("Revalidation report copied", "success", 2200);
  }, [addToast, report]);

  const copyMultibuy = useCallback(async () => {
    if (!report) return;
    const byName = new Map<string, number>();
    for (const row of copyableRows(report)) {
      const itemName = String(row.row.TypeName ?? "").replace(/\s+/g, " ").trim();
      const qty = Math.floor(row.nowQty);
      if (!itemName || qty <= 0) continue;
      byName.set(itemName, (byName.get(itemName) ?? 0) + qty);
    }
    const lines = Array.from(byName.entries()).map(([name, qty]) => `${name}\t${qty}`);
    if (lines.length === 0) {
      addToast("No executable rows to copy", "info", 2200);
      return;
    }
    await navigator.clipboard.writeText(lines.join("\n"));
    addToast(`Copied ${lines.length} executable rows`, "success", 2200);
  }, [addToast, report]);

  if (!report) return null;

  const executableRows = copyableRows(report);
  const signal = finalStatus(report);

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Revalidate before undock"
      width="max-w-7xl"
      allowFullscreen
      defaultFullscreen
    >
      <div className="p-4 flex flex-col gap-4">
        <div className={`border rounded-sm p-3 ${statusClass(signal)}`}>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div>
              <div className="text-[11px] uppercase tracking-wider opacity-80">Final signal</div>
              <div className="font-mono text-lg">{signal}</div>
            </div>
            <div className="text-sm">{finalSignalText(signal)}</div>
          </div>
        </div>

        <div className="grid grid-cols-2 md:grid-cols-6 gap-2 text-xs">
          <div className="border border-green-500/40 bg-green-950/20 rounded-sm p-3">
            <div className="text-eve-dim uppercase tracking-wider">Safe</div>
            <div className="text-green-300 font-mono text-xl">{report.safe}</div>
          </div>
          <div className="border border-yellow-500/40 bg-yellow-950/20 rounded-sm p-3">
            <div className="text-eve-dim uppercase tracking-wider">Changed</div>
            <div className="text-yellow-300 font-mono text-xl">{report.changed}</div>
          </div>
          <div className="border border-red-500/40 bg-red-950/20 rounded-sm p-3">
            <div className="text-eve-dim uppercase tracking-wider">Danger</div>
            <div className="text-red-300 font-mono text-xl">{report.danger}</div>
          </div>
          <div className="border border-eve-border bg-eve-panel rounded-sm p-3">
            <div className="text-eve-dim uppercase tracking-wider">Old profit</div>
            <div className="text-eve-text font-mono">{formatISK(report.totalOldProfit)}</div>
          </div>
          <div className="border border-eve-border bg-eve-panel rounded-sm p-3">
            <div className="text-eve-dim uppercase tracking-wider">Now profit</div>
            <div className="text-eve-text font-mono">{formatISK(report.totalNowProfit)}</div>
          </div>
          <div className="border border-eve-border bg-eve-panel rounded-sm p-3">
            <div className="text-eve-dim uppercase tracking-wider">Delta</div>
            <div
              className={`font-mono ${report.totalDeltaProfit >= 0 ? "text-green-300" : "text-red-300"}`}
            >
              {signedIsk(report.totalDeltaProfit)}
            </div>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <button
            type="button"
            onClick={() => void copyMultibuy()}
            disabled={executableRows.length === 0}
            className="px-3 py-1.5 rounded-sm border border-eve-accent/70 text-eve-accent hover:bg-eve-accent/10 transition-colors disabled:opacity-50 disabled:cursor-not-allowed text-xs font-semibold uppercase tracking-wider"
          >
            Copy non-danger multibuy
          </button>
          <button
            type="button"
            onClick={() => void copyReport()}
            className="px-3 py-1.5 rounded-sm border border-eve-border text-eve-dim hover:text-eve-text hover:border-eve-accent/50 transition-colors text-xs font-semibold uppercase tracking-wider"
          >
            Copy report
          </button>
          <span className="text-xs text-eve-dim">
            Recomputed against the current market order cache/ESI state. ESI can still cache market data.
          </span>
        </div>

        <div className="border border-eve-border rounded-sm overflow-auto">
          <table className="w-full min-w-[1280px] text-xs">
            <thead className="bg-eve-panel border-b border-eve-border text-eve-dim uppercase tracking-wider">
              <tr>
                <th className="text-left px-2 py-2">Status</th>
                <th className="text-left px-2 py-2">Action</th>
                <th className="text-left px-2 py-2">Item</th>
                <th className="text-right px-2 py-2">Qty</th>
                <th className="text-right px-2 py-2">Buy scan -&gt; quote</th>
                <th className="text-right px-2 py-2">Sell scan -&gt; quote</th>
                <th className="text-right px-2 py-2">Old profit</th>
                <th className="text-right px-2 py-2">Now profit</th>
                <th className="text-right px-2 py-2">Delta</th>
                <th className="text-left px-2 py-2">Warnings</th>
              </tr>
            </thead>
            <tbody>
              {report.rows.map((row) => (
                <tr key={row.key} className="border-b border-eve-border/50 last:border-b-0">
                  <td className="px-2 py-2">
                    <span
                      className={`inline-flex items-center px-2 py-0.5 rounded-sm border font-mono text-[11px] ${statusClass(row.status)}`}
                    >
                      {row.status}
                    </span>
                  </td>
                  <td className="px-2 py-2">
                    <span
                      className={`inline-flex items-center px-2 py-0.5 rounded-sm border font-mono text-[11px] ${
                        row.avoid
                          ? "border-red-500/50 bg-red-950/30 text-red-300"
                          : "border-green-500/50 bg-green-950/30 text-green-300"
                      }`}
                    >
                      {row.avoid ? "AVOID" : "OK"}
                    </span>
                  </td>
                  <td className="px-2 py-2 text-eve-text">
                    <div className="font-medium">{row.row.TypeName}</div>
                    <div className="text-[11px] text-eve-dim">
                      {row.row.BuyStation} -&gt; {row.row.SellStation}
                    </div>
                  </td>
                  <td className="px-2 py-2 text-right font-mono text-eve-text">
                    <div
                      className={row.qtyChanged ? "text-yellow-300" : "text-eve-text"}
                    >
                      {row.oldQty.toLocaleString()} -&gt; {row.nowQty.toLocaleString()}
                    </div>
                    {row.qtyChanged && (
                      <div className="text-[10px] text-yellow-300/80">changed</div>
                    )}
                  </td>
                  <td className="px-2 py-2 text-right font-mono">
                    <div className="text-eve-dim">{formatISK(row.oldBuy)}</div>
                    <div className={moveClass(row.oldBuy, row.nowBuy, false)}>
                      {formatISK(row.nowBuy)}
                    </div>
                  </td>
                  <td className="px-2 py-2 text-right font-mono">
                    <div className="text-eve-dim">{formatISK(row.oldSell)}</div>
                    <div className={moveClass(row.oldSell, row.nowSell, true)}>
                      {formatISK(row.nowSell)}
                    </div>
                  </td>
                  <td className="px-2 py-2 text-right font-mono text-eve-dim">
                    {formatISK(row.oldProfit)}
                  </td>
                  <td className="px-2 py-2 text-right font-mono text-eve-text">
                    {formatISK(row.nowProfit)}
                  </td>
                  <td
                    className={`px-2 py-2 text-right font-mono ${
                      row.deltaProfit >= 0 ? "text-green-300" : "text-red-300"
                    }`}
                  >
                    {signedIsk(row.deltaProfit)}
                  </td>
                  <td className="px-2 py-2 text-eve-dim max-w-[320px]">
                    {row.reasons.join(", ")}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </Modal>
  );
}
