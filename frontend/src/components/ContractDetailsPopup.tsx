import { useEffect, useState } from "react";
import { Modal } from "./Modal";
import { getContractDetails } from "../lib/api";
import type { ContractDetails, ContractItem } from "../lib/types";
import { useI18n } from "../lib/i18n";
import { formatISK } from "../lib/format";

interface ContractDetailsPopupProps {
  open: boolean;
  contractID: number;
  contractTitle: string;
  contractPrice: number;
  onClose: () => void;
}

export function ContractDetailsPopup({ open, contractID, contractTitle, contractPrice, onClose }: ContractDetailsPopupProps) {
  const { t } = useI18n();
  const [details, setDetails] = useState<ContractDetails | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setLoading(true);
    setError(null);
    getContractDetails(contractID)
      .then((data) => {
        setDetails(data);
        setLoading(false);
      })
      .catch((err) => {
        setError(err.message || "Failed to load contract details");
        setLoading(false);
      });
  }, [contractID, open]);

  // Keep raw rows to preserve risk signals (damage/fitted-like markers/BP params).
  const includedItems = details?.items.filter((item) => item.is_included) || [];
  const requestedItems = details?.items.filter((item) => !item.is_included) || [];

  return (
    <Modal open={open} onClose={onClose} title={`${t("contractDetails")} #${contractID}`}>
      <div className="p-4 flex flex-col gap-4">
        {/* Contract info */}
        <div className="border border-eve-border rounded-sm p-3 bg-eve-panel">
          <div className="text-sm text-eve-text">
            <span className="text-eve-dim">{t("colTitle")}:</span> {contractTitle}
          </div>
          <div className="text-sm text-eve-accent font-mono mt-1">
            <span className="text-eve-dim">{t("iskPrice")}:</span> {formatISK(contractPrice)}
          </div>
        </div>

        {/* Warning about damage and fitted items */}
        <div className="border border-yellow-700/50 bg-yellow-900/20 rounded-sm p-3">
          <div className="flex items-start gap-2">
            <span className="text-yellow-400 text-lg">⚠</span>
            <div className="flex-1 text-xs text-yellow-200">
              <div className="font-semibold mb-1">{t("contractDetailsWarningTitle")}</div>
              <div className="text-yellow-300/90">
                • {t("contractDetailsWarningDamage")}
                <br />
                • {t("contractDetailsWarningFitted")}
              </div>
            </div>
          </div>
        </div>

        {loading && (
          <div className="flex items-center justify-center h-40">
            <div className="text-eve-dim">{t("loading")}...</div>
          </div>
        )}

        {error && (
          <div className="text-eve-error bg-red-900/20 border border-red-700 rounded-sm p-3 text-sm">
            {error}
          </div>
        )}

        {!loading && !error && details && (
          <>
            {/* Items included (seller provides) */}
            {includedItems.length > 0 && (
              <div className="border border-eve-border rounded-sm overflow-hidden">
                <div className="px-3 py-2 bg-eve-panel border-b border-eve-border text-xs font-semibold text-green-400 uppercase tracking-wider">
                  ✓ {t("itemsIncluded")} ({includedItems.length})
                </div>
                <table className="w-full text-sm">
                  <thead className="bg-eve-panel border-b border-eve-border">
                    <tr>
                      <th className="text-left px-3 py-1.5 text-xs text-eve-dim uppercase tracking-wider">{t("colItem")}</th>
                      <th className="text-right px-3 py-1.5 text-xs text-eve-dim uppercase tracking-wider">{t("execPlanQuantity")}</th>
                      <th className="text-left px-3 py-1.5 text-xs text-eve-dim uppercase tracking-wider">{t("colType")}</th>
                    </tr>
                  </thead>
                  <tbody className="text-eve-text">
                    {includedItems.map((item, idx) => (
                      <ItemRow key={`${item.record_id}-${item.item_id}-${idx}`} item={item} />
                    ))}
                  </tbody>
                </table>
              </div>
            )}

            {/* Items requested (buyer must provide) */}
            {requestedItems.length > 0 && (
              <div className="border border-eve-border rounded-sm overflow-hidden">
                <div className="px-3 py-2 bg-eve-panel border-b border-eve-border text-xs font-semibold text-yellow-400 uppercase tracking-wider">
                  ⚠ {t("itemsRequested")} ({requestedItems.length})
                </div>
                <table className="w-full text-sm">
                  <thead className="bg-eve-panel border-b border-eve-border">
                    <tr>
                      <th className="text-left px-3 py-1.5 text-xs text-eve-dim uppercase tracking-wider">{t("colItem")}</th>
                      <th className="text-right px-3 py-1.5 text-xs text-eve-dim uppercase tracking-wider">{t("execPlanQuantity")}</th>
                      <th className="text-left px-3 py-1.5 text-xs text-eve-dim uppercase tracking-wider">{t("colType")}</th>
                    </tr>
                  </thead>
                  <tbody className="text-eve-text">
                    {requestedItems.map((item, idx) => (
                      <ItemRow key={`${item.record_id}-${item.item_id}-${idx}`} item={item} />
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </>
        )}
      </div>
    </Modal>
  );
}

function ItemRow({ item }: { item: ContractItem }) {
  const { t } = useI18n();

  let typeLabel = t("colItem");
  if (item.is_blueprint_copy) {
    typeLabel = `${t("blueprintCopy")} (${item.runs || 0} ${t("runs")})`;
  } else if (item.material_efficiency !== undefined || item.time_efficiency !== undefined) {
    typeLabel = `${t("blueprint")} (ME: ${item.material_efficiency || 0}, TE: ${item.time_efficiency || 0})`;
  } else if (item.flag !== undefined && item.flag >= 46 && item.flag <= 53) {
    typeLabel = `Fitted (Rig Slot ${item.flag - 46})`;
  } else if (item.singleton) {
    typeLabel = "Likely fitted/singleton";
  }

  const damagePercent = item.damage ? Math.round(item.damage * 100) : 0;

  return (
    <tr className="border-b border-eve-border last:border-b-0">
      <td className="px-3 py-1.5">
        <div className="flex items-center gap-2">
          <img
            src={`https://images.evetech.net/types/${item.type_id}/icon?size=32`}
            alt={item.type_name}
            className="w-8 h-8 flex-shrink-0"
            onError={(e) => {
              // Fallback if icon fails to load
              e.currentTarget.style.display = 'none';
            }}
          />
          <div className="flex-1">
            <div className="text-eve-text">{item.type_name || `Type ${item.type_id}`}</div>
            {damagePercent > 0 && (
              <div className="text-xs text-red-400">⚠ Damaged {damagePercent}%</div>
            )}
          </div>
        </div>
      </td>
      <td className="px-3 py-1.5 text-right font-mono text-eve-accent">
        {item.quantity.toLocaleString()}
      </td>
      <td className="px-3 py-1.5 text-xs text-eve-dim">{typeLabel}</td>
    </tr>
  );
}
