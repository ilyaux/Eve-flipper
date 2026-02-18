import { useState } from "react";
import { formatISK } from "../../lib/format";
import { useI18n } from "../../lib/i18n";
import type { CrossHubArbitrage, OmegaComparison, SPFarmResult } from "../../lib/types";
export function OmegaComparatorCard({
  omega,
  omegaUSD,
  onOmegaUSDChange,
  plexPrice,
  nesOmega,
}: {
  omega: OmegaComparison | null;
  omegaUSD: number;
  onOmegaUSDChange: (v: number) => void;
  plexPrice: number;
  nesOmega: number;
}) {
  const { t } = useI18n();
  const totalISK = omega?.total_isk ?? nesOmega * plexPrice;
  const iskPerUSD = omega?.isk_per_usd ?? (omegaUSD > 0 ? totalISK / omegaUSD : 0);

  return (
    <div className="bg-eve-panel border border-eve-border rounded-sm p-3">
      <h3 className="text-xs font-semibold text-eve-dim uppercase tracking-wider mb-2">{t("plexOmegaComparator")}</h3>
      <div className="space-y-2 text-xs">
        <div className="flex items-center gap-2">
          <label className="text-eve-dim w-28">{t("plexOmegaUSDLabel")}</label>
          <input
            type="number"
            min="0"
            step="0.01"
            value={omegaUSD || ""}
            onChange={e => onOmegaUSDChange(parseFloat(e.target.value) || 0)}
            className="w-24 px-1.5 py-0.5 bg-eve-input border border-eve-border rounded-sm text-xs text-eve-text font-mono"
          />
        </div>
        <div className="grid grid-cols-2 gap-2 mt-1">
          <div>
            <span className="text-eve-dim block text-[10px]">PLEX → Omega</span>
            <span className="text-eve-text font-mono">{nesOmega} PLEX = {formatISK(totalISK)}</span>
          </div>
          <div>
            <span className="text-eve-dim block text-[10px]">{t("plexOmegaVsRealMoney")}</span>
            <span className="text-eve-text font-mono">${omegaUSD.toFixed(2)}</span>
          </div>
        </div>
        {iskPerUSD > 0 && (
          <div className="mt-1 pt-1 border-t border-eve-border">
            <span className="text-eve-dim text-[10px]">{t("plexOmegaISKPerUSD")}</span>
            <span className="text-eve-accent font-semibold font-mono ml-2">{formatISK(iskPerUSD)}</span>
          </div>
        )}
      </div>
    </div>
  );
}

// ============================================================
// Cross-Hub Arbitrage Card
// ============================================================

export function CrossHubCard({ items }: { items: CrossHubArbitrage[] }) {
  const { t } = useI18n();

  return (
    <div className="bg-eve-panel border border-eve-border rounded-sm p-3 shrink-0">
      <h3 className="text-xs font-semibold text-eve-dim uppercase tracking-wider mb-1">{t("plexCrossHub")}</h3>
      <p className="text-[10px] text-eve-dim mb-2">{t("plexCrossHubHint")}</p>
      <table className="w-full text-xs">
        <thead>
          <tr className="text-eve-dim border-b border-eve-border">
            <th className="text-left py-1 px-2 font-medium">Item</th>
            <th className="text-left py-1 px-2 font-medium">{t("plexCheapestHub")}</th>
            <th className="text-right py-1 px-2 font-medium">Price</th>
            <th className="text-right py-1 px-2 font-medium">{t("plexVsJita")}</th>
            <th className="text-right py-1 px-2 font-medium">{t("plexProfit")}</th>
          </tr>
        </thead>
        <tbody>
          {items.map(item => (
            <tr key={item.type_id} className="border-b border-eve-border/30 hover:bg-eve-hover/30 transition-colors">
              <td className="py-1 px-2 text-eve-text">{item.item_name}</td>
              <td className="py-1 px-2">
                <span className={item.best_hub === "Jita" ? "text-eve-dim" : "text-eve-accent"}>
                  {item.best_hub}
                </span>
              </td>
              <td className="py-1 px-2 text-right font-mono text-eve-text">{formatISK(item.best_price)}</td>
              <td className="py-1 px-2 text-right font-mono">
                {item.diff_pct > 0 ? (
                  <span className="text-eve-positive">-{item.diff_pct.toFixed(1)}%</span>
                ) : (
                  <span className="text-eve-dim">0%</span>
                )}
              </td>
              <td className="py-1 px-2 text-right font-mono">
                {item.viable ? (
                  <span className="text-eve-positive">{formatISK(item.profit_isk)}</span>
                ) : (
                  <span className="text-eve-dim">—</span>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// ============================================================
// SP Farm Fleet Manager (frontend-only calculator)
// ============================================================

const FLEET_STORAGE_KEY = "plex_fleet";

interface FleetConfig {
  accounts: number;
  charsPerAccount: number;
}

function loadFleetConfig(): FleetConfig {
  try {
    const raw = localStorage.getItem(FLEET_STORAGE_KEY);
    if (raw) return JSON.parse(raw);
  } catch { /* ignore */ }
  return { accounts: 1, charsPerAccount: 3 };
}

export function FleetManagerCard({ spFarm }: { spFarm: SPFarmResult }) {
  const { t } = useI18n();
  const [cfg, setCfg] = useState(loadFleetConfig);

  const updateCfg = (patch: Partial<FleetConfig>) => {
    setCfg(prev => {
      const next = { ...prev, ...patch };
      localStorage.setItem(FLEET_STORAGE_KEY, JSON.stringify(next));
      return next;
    });
  };

  const totalChars = cfg.accounts * cfg.charsPerAccount;
  // First char on each account is the "main" → Omega only; additional chars need MPTC
  const mptcCharsPerAccount = Math.max(0, cfg.charsPerAccount - 1);
  const totalMPTCChars = cfg.accounts * mptcCharsPerAccount;

  // Costs per account
  const omegaCostPerAcct = spFarm.omega_cost_isk;
  const mptcCostPerAcct = mptcCharsPerAccount * spFarm.mptc_cost_isk;
  const extractorsPerAcct = cfg.charsPerAccount * spFarm.extractors_per_month;
  const extractorCostPerAcct = extractorsPerAcct * (spFarm.extractor_cost_isk / spFarm.extractors_per_month);
  const revenuePerAcct = cfg.charsPerAccount * spFarm.revenue_isk;
  const totalCostPerAcct = omegaCostPerAcct + mptcCostPerAcct + extractorCostPerAcct;
  const profitPerAcct = revenuePerAcct - totalCostPerAcct;

  // Totals
  const totalOmega = cfg.accounts * omegaCostPerAcct;
  const totalMPTC = cfg.accounts * mptcCostPerAcct;
  const totalRevenue = cfg.accounts * revenuePerAcct;
  const totalProfit = cfg.accounts * profitPerAcct;

  return (
    <div className="bg-eve-panel border border-eve-border rounded-sm p-3 shrink-0">
      <h3 className="text-xs font-semibold text-eve-dim uppercase tracking-wider mb-2">{t("plexFleetManager")}</h3>

      {/* Config inputs */}
      <div className="flex items-center gap-4 mb-3 text-xs">
        <div className="flex items-center gap-2">
          <label className="text-eve-dim">{t("plexFleetAccounts")}</label>
          <input
            type="number"
            min="1"
            max="50"
            value={cfg.accounts}
            onChange={e => updateCfg({ accounts: Math.max(1, parseInt(e.target.value) || 1) })}
            className="w-16 px-1.5 py-0.5 bg-eve-input border border-eve-border rounded-sm text-xs text-eve-text font-mono"
          />
        </div>
        <div className="flex items-center gap-2">
          <label className="text-eve-dim">{t("plexFleetCharsPerAcct")}</label>
          <input
            type="number"
            min="1"
            max="3"
            value={cfg.charsPerAccount}
            onChange={e => updateCfg({ charsPerAccount: Math.min(3, Math.max(1, parseInt(e.target.value) || 1)) })}
            className="w-16 px-1.5 py-0.5 bg-eve-input border border-eve-border rounded-sm text-xs text-eve-text font-mono"
          />
        </div>
        <span className="text-eve-dim text-[10px]">{t("plexFleetTotalChars")}: {totalChars} ({totalMPTCChars} MPTC)</span>
      </div>

      {/* Fleet summary table */}
      <table className="w-full text-xs">
        <thead>
          <tr className="text-eve-dim border-b border-eve-border">
            <th className="text-left py-1 px-2 font-medium">#</th>
            <th className="text-right py-1 px-2 font-medium">{t("plexFleetOmegaCost")}</th>
            <th className="text-right py-1 px-2 font-medium">{t("plexFleetMPTCCost")}</th>
            <th className="text-right py-1 px-2 font-medium">{t("plexFleetExtractors")}</th>
            <th className="text-right py-1 px-2 font-medium">{t("plexFleetRevenue")}</th>
            <th className="text-right py-1 px-2 font-medium">{t("plexFleetProfit")}</th>
          </tr>
        </thead>
        <tbody>
          {Array.from({ length: cfg.accounts }, (_, i) => (
            <tr key={i} className="border-b border-eve-border/30">
              <td className="py-1 px-2 text-eve-dim">Acct {i + 1}</td>
              <td className="py-1 px-2 text-right font-mono text-eve-text">{formatISK(omegaCostPerAcct)}</td>
              <td className="py-1 px-2 text-right font-mono text-eve-text">{mptcCharsPerAccount > 0 ? formatISK(mptcCostPerAcct) : "—"}</td>
              <td className="py-1 px-2 text-right font-mono text-eve-text">{extractorsPerAcct.toFixed(1)}</td>
              <td className="py-1 px-2 text-right font-mono text-eve-text">{formatISK(revenuePerAcct)}</td>
              <td className={`py-1 px-2 text-right font-mono font-semibold ${profitPerAcct >= 0 ? "text-eve-positive" : "text-eve-negative"}`}>
                {formatISK(profitPerAcct)}
              </td>
            </tr>
          ))}
          {/* Total row */}
          <tr className="border-t-2 border-eve-border font-semibold">
            <td className="py-1.5 px-2 text-eve-text">{t("plexFleetTotal")}</td>
            <td className="py-1.5 px-2 text-right font-mono text-eve-text">{formatISK(totalOmega)}</td>
            <td className="py-1.5 px-2 text-right font-mono text-eve-text">{totalMPTCChars > 0 ? formatISK(totalMPTC) : "—"}</td>
            <td className="py-1.5 px-2 text-right font-mono text-eve-text">{(cfg.accounts * extractorsPerAcct).toFixed(1)}</td>
            <td className="py-1.5 px-2 text-right font-mono text-eve-text">{formatISK(totalRevenue)}</td>
            <td className={`py-1.5 px-2 text-right font-mono ${totalProfit >= 0 ? "text-eve-positive" : "text-eve-negative"}`}>
              {formatISK(totalProfit)}
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  );
}
