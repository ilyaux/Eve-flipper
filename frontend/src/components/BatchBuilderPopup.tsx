import { useCallback, useEffect, useMemo, useState } from "react";
import { formatISK } from "@/lib/format";
import { useI18n } from "@/lib/i18n";
import type { FlipResult } from "@/lib/types";
import { Modal } from "./Modal";
import { useGlobalToast } from "./Toast";

type BatchLine = {
  row: FlipResult;
  units: number;
  volume: number;
  profit: number;
  capital: number;
  iskPerM3: number;
};

interface BatchBuilderPopupProps {
  open: boolean;
  onClose: () => void;
  anchorRow: FlipResult | null;
  rows: FlipResult[];
  defaultCargoM3?: number;
}

function safeNumber(value: unknown): number {
  const n = Number(value);
  return Number.isFinite(n) ? n : 0;
}

function rowProfitPerUnit(row: FlipResult): number {
  const filledQty = safeNumber(row.FilledQty);
  if (filledQty > 0 && row.RealProfit != null) {
    const v = safeNumber(row.RealProfit) / filledQty;
    if (Number.isFinite(v)) return v;
  }
  return safeNumber(row.ProfitPerUnit);
}

function rowCapitalPerUnit(row: FlipResult): number {
  const expected = safeNumber(row.ExpectedBuyPrice);
  if (expected > 0) return expected;
  return Math.max(0, safeNumber(row.BuyPrice));
}

function rowMaxUnits(row: FlipResult): number {
  const recommended = Math.floor(safeNumber(row.UnitsToBuy));
  if (recommended > 0) return recommended;
  const buyRemain = Math.floor(Math.max(0, safeNumber(row.BuyOrderRemain)));
  const sellRemain = Math.floor(Math.max(0, safeNumber(row.SellOrderRemain)));
  if (buyRemain > 0 && sellRemain > 0) return Math.min(buyRemain, sellRemain);
  return Math.max(buyRemain, sellRemain);
}

function sameRoute(anchor: FlipResult, row: FlipResult): boolean {
  const anchorBuyLoc = safeNumber(anchor.BuyLocationID);
  const anchorSellLoc = safeNumber(anchor.SellLocationID);
  const rowBuyLoc = safeNumber(row.BuyLocationID);
  const rowSellLoc = safeNumber(row.SellLocationID);
  if (anchorBuyLoc > 0 && anchorSellLoc > 0 && rowBuyLoc > 0 && rowSellLoc > 0) {
    return anchorBuyLoc === rowBuyLoc && anchorSellLoc === rowSellLoc;
  }
  return (
    safeNumber(anchor.BuySystemID) === safeNumber(row.BuySystemID) &&
    safeNumber(anchor.SellSystemID) === safeNumber(row.SellSystemID)
  );
}

function routeLineKey(row: FlipResult): string {
  return [
    row.TypeID,
    safeNumber(row.BuyLocationID) || row.BuyStation || row.BuySystemID,
    safeNumber(row.SellLocationID) || row.SellStation || row.SellSystemID,
  ].join(":");
}

function buildBatch(
  anchor: FlipResult,
  rows: FlipResult[],
  cargoLimitM3: number,
): {
  lines: BatchLine[];
  totalVolume: number;
  totalProfit: number;
  totalCapital: number;
  remainingM3: number | null;
  usedPercent: number | null;
} {
  const routeRows = rows.filter((row) => sameRoute(anchor, row));
  if (routeRows.length === 0) {
    return {
      lines: [],
      totalVolume: 0,
      totalProfit: 0,
      totalCapital: 0,
      remainingM3: cargoLimitM3 > 0 ? cargoLimitM3 : null,
      usedPercent: cargoLimitM3 > 0 ? 0 : null,
    };
  }

  const byKey = new Map<
    string,
    {
      row: FlipResult;
      volumePerUnit: number;
      profitPerUnit: number;
      capitalPerUnit: number;
      maxUnits: number;
      density: number;
    }
  >();

  for (const row of routeRows) {
    const volumePerUnit = safeNumber(row.Volume);
    const profitPerUnit = rowProfitPerUnit(row);
    const capitalPerUnit = rowCapitalPerUnit(row);
    const maxUnits = rowMaxUnits(row);
    if (volumePerUnit <= 0 || maxUnits <= 0 || profitPerUnit <= 0) continue;
    const density = profitPerUnit / volumePerUnit;
    const key = routeLineKey(row);
    const existing = byKey.get(key);
    if (!existing || density > existing.density) {
      byKey.set(key, {
        row,
        volumePerUnit,
        profitPerUnit,
        capitalPerUnit,
        maxUnits,
        density,
      });
    }
  }

  const candidates = Array.from(byKey.values()).sort((a, b) => {
    if (b.density !== a.density) return b.density - a.density;
    if (b.profitPerUnit !== a.profitPerUnit) return b.profitPerUnit - a.profitPerUnit;
    return b.maxUnits - a.maxUnits;
  });

  const capacity = cargoLimitM3 > 0 ? cargoLimitM3 : Number.POSITIVE_INFINITY;
  let remaining = capacity;
  const lines: BatchLine[] = [];

  const addCandidate = (candidate: (typeof candidates)[number]) => {
    if (!(remaining > 0)) return;
    const maxByCargo = Number.isFinite(remaining)
      ? Math.floor((remaining + 1e-9) / candidate.volumePerUnit)
      : candidate.maxUnits;
    const units = Math.min(candidate.maxUnits, maxByCargo);
    if (units <= 0) return;
    const volume = units * candidate.volumePerUnit;
    lines.push({
      row: candidate.row,
      units,
      volume,
      profit: units * candidate.profitPerUnit,
      capital: units * candidate.capitalPerUnit,
      iskPerM3: candidate.density,
    });
    if (Number.isFinite(remaining)) {
      remaining -= volume;
    }
  };

  const anchorKey = routeLineKey(anchor);
  const anchorCandidate = candidates.find((c) => routeLineKey(c.row) === anchorKey);
  if (anchorCandidate) addCandidate(anchorCandidate);
  for (const candidate of candidates) {
    if (anchorCandidate && routeLineKey(candidate.row) === anchorKey) continue;
    addCandidate(candidate);
  }

  const totalVolume = lines.reduce((sum, line) => sum + line.volume, 0);
  const totalProfit = lines.reduce((sum, line) => sum + line.profit, 0);
  const totalCapital = lines.reduce((sum, line) => sum + line.capital, 0);
  const remainingM3 = Number.isFinite(capacity) ? Math.max(0, capacity - totalVolume) : null;
  const usedPercent =
    Number.isFinite(capacity) && capacity > 0
      ? Math.min(100, (totalVolume / capacity) * 100)
      : null;

  return { lines, totalVolume, totalProfit, totalCapital, remainingM3, usedPercent };
}

export function BatchBuilderPopup({
  open,
  onClose,
  anchorRow,
  rows,
  defaultCargoM3 = 0,
}: BatchBuilderPopupProps) {
  const { t } = useI18n();
  const { addToast } = useGlobalToast();
  const [cargoLimitM3, setCargoLimitM3] = useState<number>(
    defaultCargoM3 > 0 ? defaultCargoM3 : 0,
  );

  useEffect(() => {
    if (!open) return;
    setCargoLimitM3(defaultCargoM3 > 0 ? defaultCargoM3 : 0);
  }, [open, defaultCargoM3]);

  const batch = useMemo(() => {
    if (!anchorRow) {
      return {
        lines: [],
        totalVolume: 0,
        totalProfit: 0,
        totalCapital: 0,
        remainingM3: cargoLimitM3 > 0 ? cargoLimitM3 : null,
        usedPercent: cargoLimitM3 > 0 ? 0 : null,
      };
    }
    return buildBatch(anchorRow, rows, cargoLimitM3);
  }, [anchorRow, rows, cargoLimitM3]);

  const copyManifest = useCallback(async () => {
    if (!anchorRow || batch.lines.length === 0) return;
    const lines: string[] = [];
    lines.push(`Route: ${anchorRow.BuyStation} -> ${anchorRow.SellStation}`);
    lines.push(
      `Cargo m3: ${
        cargoLimitM3 > 0 ? cargoLimitM3.toLocaleString() : t("batchBuilderCargoUnlimited")
      }`,
    );
    lines.push(`Items: ${batch.lines.length}`);
    lines.push(`Total volume: ${batch.totalVolume.toLocaleString(undefined, { maximumFractionDigits: 1 })} m3`);
    lines.push(`Total profit: ${Math.round(batch.totalProfit).toLocaleString()} ISK`);
    lines.push(`Total capital: ${Math.round(batch.totalCapital).toLocaleString()} ISK`);
    lines.push("");
    for (const line of batch.lines) {
      lines.push(
        `${line.row.TypeName} | qty ${line.units.toLocaleString()} | vol ${line.volume.toLocaleString(undefined, { maximumFractionDigits: 1 })} m3 | profit ${Math.round(line.profit).toLocaleString()} ISK`,
      );
    }
    await navigator.clipboard.writeText(lines.join("\n"));
    addToast(t("batchBuilderCopied"), "success", 2200);
  }, [anchorRow, batch, cargoLimitM3, t, addToast]);

  if (!anchorRow) return null;

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={`${t("batchBuilderTitle")}: ${anchorRow.BuyStation} -> ${anchorRow.SellStation}`}
      width="max-w-5xl"
    >
      <div className="p-4 flex flex-col gap-3">
        <p className="text-xs text-eve-dim">{t("batchBuilderHint")}</p>

        <div className="flex flex-wrap items-end gap-3">
          <label className="flex flex-col gap-1 text-xs text-eve-dim">
            <span>{t("batchBuilderCargoLabel")}</span>
            <input
              type="number"
              min={0}
              step={1}
              value={cargoLimitM3}
              onChange={(e) =>
                setCargoLimitM3(Math.max(0, Number.parseInt(e.target.value || "0", 10) || 0))
              }
              className="w-36 px-2 py-1 bg-eve-input border border-eve-border rounded-sm text-eve-text font-mono text-sm"
            />
            <span className="text-[10px] text-eve-dim/80">{t("batchBuilderCargoHint")}</span>
          </label>

          <button
            type="button"
            onClick={() => {
              void copyManifest();
            }}
            disabled={batch.lines.length === 0}
            className="px-3 py-1.5 rounded-sm border border-eve-accent/70 text-eve-accent hover:bg-eve-accent/10 transition-colors disabled:opacity-50 disabled:cursor-not-allowed text-xs font-semibold uppercase tracking-wider"
          >
            {t("batchBuilderCopyManifest")}
          </button>
        </div>

        {batch.lines.length === 0 ? (
          <div className="border border-eve-border rounded-sm p-3 text-sm text-eve-dim">
            {t("batchBuilderNoCandidates")}
          </div>
        ) : (
          <>
            <div className="grid grid-cols-1 md:grid-cols-4 gap-2 text-xs">
              <div className="border border-eve-border rounded-sm p-2 bg-eve-panel">
                <div className="text-eve-dim">{t("batchBuilderTotalVolume")}</div>
                <div className="text-eve-accent font-mono mt-0.5">
                  {batch.totalVolume.toLocaleString(undefined, { maximumFractionDigits: 1 })} m3
                </div>
              </div>
              <div className="border border-eve-border rounded-sm p-2 bg-eve-panel">
                <div className="text-eve-dim">{t("batchBuilderTotalProfit")}</div>
                <div className="text-green-400 font-mono mt-0.5">{formatISK(batch.totalProfit)}</div>
              </div>
              <div className="border border-eve-border rounded-sm p-2 bg-eve-panel">
                <div className="text-eve-dim">{t("batchBuilderTotalCapital")}</div>
                <div className="text-eve-text font-mono mt-0.5">{formatISK(batch.totalCapital)}</div>
              </div>
              <div className="border border-eve-border rounded-sm p-2 bg-eve-panel">
                <div className="text-eve-dim">{t("batchBuilderCargoUsage")}</div>
                <div className="text-yellow-300 font-mono mt-0.5">
                  {batch.usedPercent != null
                    ? `${batch.usedPercent.toFixed(1)}%`
                    : t("batchBuilderCargoUnlimited")}
                </div>
                {batch.remainingM3 != null && (
                  <div className="text-[11px] text-eve-dim mt-0.5">
                    {t("batchBuilderCargoRemaining")}:{" "}
                    {batch.remainingM3.toLocaleString(undefined, {
                      maximumFractionDigits: 1,
                    })}{" "}
                    m3
                  </div>
                )}
              </div>
            </div>

            <div className="border border-eve-border rounded-sm overflow-hidden">
              <table className="w-full text-xs">
                <thead className="bg-eve-panel border-b border-eve-border text-eve-dim uppercase tracking-wider">
                  <tr>
                    <th className="text-left px-2 py-1.5">{t("batchBuilderColItem")}</th>
                    <th className="text-right px-2 py-1.5">{t("batchBuilderColQty")}</th>
                    <th className="text-right px-2 py-1.5">{t("batchBuilderColVolume")}</th>
                    <th className="text-right px-2 py-1.5">{t("batchBuilderColCapital")}</th>
                    <th className="text-right px-2 py-1.5">{t("batchBuilderColProfit")}</th>
                    <th className="text-right px-2 py-1.5">{t("batchBuilderColDensity")}</th>
                  </tr>
                </thead>
                <tbody>
                  {batch.lines.map((line) => (
                    <tr
                      key={routeLineKey(line.row)}
                      className="border-b border-eve-border/50 last:border-b-0"
                    >
                      <td className="px-2 py-1.5 text-eve-text">{line.row.TypeName}</td>
                      <td className="px-2 py-1.5 text-right font-mono text-eve-text">
                        {line.units.toLocaleString()}
                      </td>
                      <td className="px-2 py-1.5 text-right font-mono text-eve-dim">
                        {line.volume.toLocaleString(undefined, { maximumFractionDigits: 1 })}
                      </td>
                      <td className="px-2 py-1.5 text-right font-mono text-eve-dim">
                        {formatISK(line.capital)}
                      </td>
                      <td className="px-2 py-1.5 text-right font-mono text-green-400">
                        {formatISK(line.profit)}
                      </td>
                      <td className="px-2 py-1.5 text-right font-mono text-yellow-300">
                        {formatISK(line.iskPerM3)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </>
        )}
      </div>
    </Modal>
  );
}

