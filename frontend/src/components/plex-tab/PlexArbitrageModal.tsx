import { useEffect } from "react";
import { formatISK } from "../../lib/format";
import { useI18n } from "../../lib/i18n";
import type { ArbitragePath } from "../../lib/types";
function getFlowSteps(arb: ArbitragePath): { label: string; sub: string; color: string }[] {
  const costStr = formatISK(arb.cost_isk);
  const revStr = formatISK(arb.revenue_isk);

  if (arb.type === "nes_sell" && arb.name.includes("Extractor")) {
    return [
      { label: "Buy PLEX", sub: `${arb.plex_cost} PLEX × market`, color: "border-eve-accent/40 bg-eve-accent/5" },
      { label: "NES Store", sub: `Spend ${arb.plex_cost} PLEX`, color: "border-eve-warning/40 bg-eve-warning/5" },
      { label: "Skill Extractor", sub: "Receive 1 item", color: "border-blue-500/40 bg-blue-500/5" },
      { label: "Sell on Market", sub: `${revStr} ISK`, color: "border-eve-success/40 bg-eve-success/5" },
    ];
  }
  if (arb.type === "nes_process") {
    return [
      { label: "Buy PLEX", sub: `${arb.plex_cost} PLEX × market`, color: "border-eve-accent/40 bg-eve-accent/5" },
      { label: "NES Store", sub: `Spend ${arb.plex_cost} PLEX`, color: "border-eve-warning/40 bg-eve-warning/5" },
      { label: "Skill Extractor", sub: "Receive 1 item", color: "border-blue-500/40 bg-blue-500/5" },
      { label: "Extract SP", sub: "500,000 SP from char", color: "border-purple-500/40 bg-purple-500/5" },
      { label: "Large Skill Injector", sub: "Created from SP", color: "border-cyan-500/40 bg-cyan-500/5" },
      { label: "Sell on Market", sub: `${revStr} ISK`, color: "border-eve-success/40 bg-eve-success/5" },
    ];
  }
  if (arb.type === "market_process") {
    return [
      { label: "Buy Extractor", sub: `${formatISK(arb.cost_isk)} ISK`, color: "border-eve-accent/40 bg-eve-accent/5" },
      { label: "Extract SP", sub: "500,000 SP from char", color: "border-purple-500/40 bg-purple-500/5" },
      { label: "Large Skill Injector", sub: "Created from SP", color: "border-cyan-500/40 bg-cyan-500/5" },
      { label: "Sell on Market", sub: `${revStr} ISK`, color: "border-eve-success/40 bg-eve-success/5" },
    ];
  }
  if (arb.type === "nes_sell" && arb.name.includes("MPTC")) {
    return [
      { label: "Buy PLEX", sub: `${arb.plex_cost} PLEX × market`, color: "border-eve-accent/40 bg-eve-accent/5" },
      { label: "NES Store", sub: `Spend ${arb.plex_cost} PLEX`, color: "border-eve-warning/40 bg-eve-warning/5" },
      { label: "MPTC", sub: "Receive 1 certificate", color: "border-blue-500/40 bg-blue-500/5" },
      { label: "Sell on Market", sub: `${revStr} ISK`, color: "border-eve-success/40 bg-eve-success/5" },
    ];
  }
  if (arb.type === "spread") {
    // Extract item name from arb name (e.g. "PLEX Spread ..." → "PLEX")
    const itemName = arb.name.split(" ")[0];
    return [
      { label: `Buy Order`, sub: `${costStr} ISK`, color: "border-eve-success/40 bg-eve-success/5" },
      { label: itemName, sub: "Wait for fill", color: "border-blue-500/40 bg-blue-500/5" },
      { label: `Sell Order`, sub: `${formatISK(arb.revenue_gross)} ISK`, color: "border-eve-error/40 bg-eve-error/5" },
      { label: "Profit", sub: `${formatISK(arb.profit_isk)} ISK`, color: arb.viable ? "border-eve-success/40 bg-eve-success/5" : "border-eve-error/40 bg-eve-error/5" },
    ];
  }
  // Fallback
  return [
    { label: "Cost", sub: costStr, color: "border-eve-error/40 bg-eve-error/5" },
    { label: "Revenue", sub: revStr, color: "border-eve-success/40 bg-eve-success/5" },
  ];
}

export function ArbitrageModal({ arb, onClose }: { arb: ArbitragePath; onClose: () => void }) {
  const { t } = useI18n();
  const steps = getFlowSteps(arb);

  // Close on Escape key
  useEffect(() => {
    const handler = (e: KeyboardEvent) => { if (e.key === "Escape") onClose(); };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [onClose]);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4" onClick={onClose}>
      {/* Backdrop */}
      <div className="absolute inset-0 bg-black/70 backdrop-blur-sm" />

      {/* Modal */}
      <div
        className="relative bg-eve-dark border border-eve-border rounded-sm shadow-2xl w-full max-w-2xl max-h-[95vh] sm:max-h-[90vh] mx-2 sm:mx-0 overflow-y-auto"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between p-4 border-b border-eve-border">
          <div className="flex items-center gap-2">
            <span className={`w-2.5 h-2.5 rounded-full ${arb.viable ? "bg-eve-success" : "bg-eve-error"}`} />
            <h2 className="text-sm font-semibold text-eve-text uppercase tracking-wider">{arb.name}</h2>
          </div>
          <button onClick={onClose} className="text-eve-dim hover:text-eve-text transition-colors text-lg leading-none px-1">&times;</button>
        </div>

        {/* Flow diagram */}
        <div className="p-4">
          <h3 className="text-[10px] text-eve-dim uppercase tracking-wider font-medium mb-3">{t("plexArbFlow")}</h3>
          <div className="flex items-stretch gap-0 overflow-x-auto pb-2">
            {steps.map((step, i) => (
              <div key={i} className="flex items-center shrink-0">
                {/* Step box */}
                <div className={`border rounded-sm px-2 py-2 sm:px-3 sm:py-2.5 min-w-[80px] sm:min-w-[110px] text-center ${step.color}`}>
                  <div className="text-xs font-semibold text-eve-text whitespace-nowrap">{step.label}</div>
                  <div className="text-[10px] text-eve-dim mt-0.5 whitespace-nowrap">{step.sub}</div>
                </div>
                {/* Arrow */}
                {i < steps.length - 1 && (
                  <div className="flex items-center px-1 text-eve-dim shrink-0">
                    <svg width="20" height="12" viewBox="0 0 20 12" fill="none" className="text-eve-dim">
                      <path d="M0 6H16M16 6L11 1M16 6L11 11" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                  </div>
                )}
              </div>
            ))}
          </div>
        </div>

        {/* Financial breakdown */}
        <div className="px-4 pb-4">
          <h3 className="text-[10px] text-eve-dim uppercase tracking-wider font-medium mb-3">{t("plexArbBreakdown")}</h3>
          <div className="grid grid-cols-2 gap-3">
            {/* Cost side */}
            <div className="border border-eve-error/20 bg-eve-error/5 rounded-sm p-3">
              <div className="text-[10px] text-eve-error uppercase tracking-wider font-medium mb-2">{t("plexCost")}</div>
              <div className="space-y-1.5 text-xs">
                {arb.type === "spread" ? (
                  <>
                    <div className="flex justify-between">
                      <span className="text-eve-dim">Buy order + broker</span>
                      <span className="font-mono text-eve-text">{formatISK(arb.cost_isk)}</span>
                    </div>
                  </>
                ) : (
                  <>
                    <div className="flex justify-between">
                      <span className="text-eve-dim">PLEX needed</span>
                      <span className="font-mono text-eve-text">{arb.plex_cost}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-eve-dim">{arb.type === "market_process" ? "Market cost" : "PLEX cost (market)"}</span>
                      <span className="font-mono text-eve-text">{formatISK(arb.cost_isk)}</span>
                    </div>
                  </>
                )}
                {(arb.type === "nes_process" || arb.type === "market_process") && (
                  <div className="flex justify-between">
                    <span className="text-eve-dim">Requires char with</span>
                    <span className="font-mono text-eve-text">&ge; 5.5M SP</span>
                  </div>
                )}
              </div>
            </div>

            {/* Revenue side */}
            <div className="border border-eve-success/20 bg-eve-success/5 rounded-sm p-3">
              <div className="text-[10px] text-eve-success uppercase tracking-wider font-medium mb-2">{t("plexRevenue")}</div>
              <div className="space-y-1.5 text-xs">
                <div className="flex justify-between">
                  <span className="text-eve-dim">Sell price (Jita)</span>
                  <span className="font-mono text-eve-text">{formatISK(arb.revenue_gross)}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-eve-dim">After tax + broker</span>
                  <span className="font-mono text-eve-text">{formatISK(arb.revenue_isk)}</span>
                </div>
              </div>
            </div>
          </div>

          {/* Result */}
          <div className={`mt-3 border rounded-sm p-3 ${arb.viable ? "border-eve-success/30 bg-eve-success/5" : "border-eve-error/30 bg-eve-error/5"}`}>
            <div className="flex items-center justify-between">
              <div>
                <div className="text-[10px] text-eve-dim uppercase tracking-wider font-medium mb-1">{t("plexProfit")}</div>
                <div className={`text-xl font-mono font-bold ${arb.viable ? "text-eve-success" : "text-eve-error"}`}>
                  {arb.profit_isk >= 0 ? "+" : ""}{formatISK(arb.profit_isk)}
                </div>
                {arb.slippage_pct !== 0 && (
                  <div className="text-[10px] text-eve-warning mt-0.5">
                    {t("plexAdjustedProfit")}: {arb.adjusted_profit_isk >= 0 ? "+" : ""}{formatISK(arb.adjusted_profit_isk)}
                    <span className="ml-1">({arb.slippage_pct.toFixed(2)}% {t("plexSlippage").toLowerCase()})</span>
                  </div>
                )}
              </div>
              <div className="text-right">
                <div className="text-[10px] text-eve-dim uppercase tracking-wider font-medium mb-1">ROI</div>
                <div className={`text-xl font-mono font-bold ${arb.viable ? "text-eve-success" : "text-eve-error"}`}>
                  {arb.roi >= 0 ? "+" : ""}{arb.roi.toFixed(1)}%
                </div>
                {arb.isk_per_hour > 0 && (
                  <div className="text-[10px] text-eve-dim mt-0.5">
                    {formatISK(arb.isk_per_hour)}/{t("plexISKPerHour").split("/")[1] || "hr"}
                  </div>
                )}
              </div>
            </div>
          </div>

          {/* Tips */}
          <div className="mt-3 text-[11px] text-eve-dim leading-relaxed space-y-1">
            <div className="text-[10px] text-eve-dim uppercase tracking-wider font-medium mb-1">{t("plexArbTips")}</div>
            {arb.type === "nes_sell" && arb.name.includes("Extractor") && (
              <>
                <p>• {t("plexTipExtractor1")}</p>
                <p>• {t("plexTipExtractor2")}</p>
              </>
            )}
            {arb.type === "nes_process" && (
              <>
                <p>• {t("plexTipSPChain1")}</p>
                <p>• {t("plexTipSPChain2")}</p>
                <p>• {t("plexTipSPChain3")}</p>
              </>
            )}
            {arb.type === "market_process" && (
              <>
                <p>• {t("plexTipMarket1")}</p>
                <p>• {t("plexTipSPChain1")}</p>
                <p>• {t("plexTipSPChain3")}</p>
              </>
            )}
            {arb.type === "nes_sell" && arb.name.includes("MPTC") && (
              <>
                <p>• {t("plexTipMPTC1")}</p>
                <p>• {t("plexTipMPTC2")}</p>
              </>
            )}
            {arb.type === "spread" && (
              <>
                <p>• {t("plexTipSpread1")}</p>
                <p>• {t("plexTipSpread2")}</p>
                <p>• {t("plexTipSpread3")}</p>
              </>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
