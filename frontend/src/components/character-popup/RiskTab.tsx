import { type TranslationKey } from "../../lib/i18n";
import type { CharacterInfo } from "../../lib/types";
import { StatCard } from "./shared";
interface RiskTabProps {
  characterId?: number;
  isAllScope: boolean;
  data: CharacterInfo;
  formatIsk: (v: number) => string;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}

export function RiskTab({ characterId, isAllScope, data, formatIsk, t }: RiskTabProps) {
  const risk = data.risk;

  if (!risk) {
    return (
      <div className="flex flex-col items-center justify-center h-full text-eve-dim text-xs space-y-2">
        <div>{t("charRiskNoData")}</div>
        <div className="text-[10px] max-w-md text-center">
          {t("charRiskNoDataHint")}
        </div>
      </div>
    );
  }

  const riskLevelLabel =
    risk.risk_level === "safe"
      ? t("riskLevelSafe")
      : risk.risk_level === "balanced"
      ? t("riskLevelBalanced")
      : t("riskLevelHigh");

  const riskScore = Math.max(0, Math.min(100, risk.risk_score || 0));

  let riskColor = "bg-emerald-500";
  if (riskScore > 70) riskColor = "bg-red-500";
  else if (riskScore > 30) riskColor = "bg-amber-500";

  // Don't mask negative values with Math.max — show real data.
  // typical_daily_pnl and the loss metrics should be displayed as-is.
  const typicalPnl = risk.typical_daily_pnl || 0;
  const var99 = risk.var_99 || 0;
  const es99 = risk.es_99 || 0;
  const worst = risk.worst_day_loss || 0;

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex items-center gap-4 p-4 bg-eve-panel border border-eve-border rounded-sm">
        {characterId ? (
          <img
            src={`https://images.evetech.net/characters/${characterId}/portrait?size=64`}
            alt=""
            className="w-12 h-12 rounded-sm"
          />
        ) : (
          <div className="w-12 h-12 rounded-sm bg-eve-dark border border-eve-border flex items-center justify-center text-[10px] text-eve-accent font-semibold">
            ALL
          </div>
        )}
        <div className="flex-1">
          <div className="text-[10px] uppercase tracking-wider text-eve-dim mb-1">
            {t("charRiskTitle")}
          </div>
          <div className="flex items-baseline gap-2">
            <div className="text-lg font-bold text-eve-text">
              {riskLevelLabel}
            </div>
            <div className="text-xs text-eve-dim">
              {t("charRiskScoreLabel", { score: Math.round(riskScore) })}
            </div>
          </div>
          {isAllScope && (
            <div className="text-[10px] text-eve-dim mt-1">{t("charAllCharacters")}</div>
          )}
          <div className="mt-2 h-2 w-full bg-eve-dark rounded-full overflow-hidden">
            <div
              className={`h-full ${riskColor}`}
              style={{ width: `${riskScore}%` }}
            />
          </div>
        </div>
      </div>

      {/* Low sample warning */}
      {risk.low_sample && (
        <div className="bg-amber-500/10 border border-amber-500/30 rounded-sm px-3 py-2 text-xs text-amber-400">
          {t("charRiskLowSample", { days: risk.sample_days })}
        </div>
      )}

      {/* Worst-case loss + daily behaviour */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
        <StatCard
          label={t("charRiskWorstDay")}
          value={`-${formatIsk(worst)} ISK`}
          subvalue={t("charRiskWorstDayHint", { days: risk.sample_days })}
          color="text-eve-error"
        />
        <StatCard
          label={t("charRiskVar99")}
          value={
            risk.var_99_reliable === false
              ? "—"
              : `-${formatIsk(var99)} ISK`
          }
          subvalue={
            risk.var_99_reliable === false
              ? t("charRiskVar99Unreliable", { min: 30 })
              : t("charRiskVar99Hint")
          }
          color={risk.var_99_reliable === false ? "text-eve-dim" : "text-eve-warning"}
        />
        <StatCard
          label={t("charRiskEs99")}
          value={
            risk.var_99_reliable === false
              ? "—"
              : `-${formatIsk(es99)} ISK`
          }
          subvalue={
            risk.var_99_reliable === false
              ? t("charRiskEs99Unreliable", { min: 30 })
              : t("charRiskEs99Hint")
          }
          color={risk.var_99_reliable === false ? "text-eve-dim" : "text-eve-warning"}
        />
      </div>

      {/* Narrative explanation */}
      <div className="bg-eve-panel border border-eve-border rounded-sm p-3 text-xs text-eve-text space-y-1">
        <div>
          {t("charRiskSentenceLoss", {
            var: formatIsk(var99),
            days: risk.window_days,
          })}
        </div>
        <div>
          {t("charRiskSentenceTail", {
            es: formatIsk(es99),
          })}
        </div>
        <div className="text-eve-dim text-[11px]">
          {t("charRiskSentenceTypical", {
            typical: formatIsk(typicalPnl),
          })}
        </div>
      </div>

      {/* Capacity / suggestion */}
      <div className="bg-eve-panel border border-eve-border rounded-sm p-3 text-xs text-eve-text">
        {risk.capacity_multiplier > 1.05 ? (
          <div>
            {t("charRiskCapacityUp", {
              mult: risk.capacity_multiplier.toFixed(1),
            })}
          </div>
        ) : (
          <div>{t("charRiskCapacityMaxed")}</div>
        )}
      </div>
    </div>
  );
}
