import { useEffect, useState, useCallback } from "react";
import { Modal } from "./Modal";
import {
  getCharacterInfo,
  getCharacterRoles,
  type CharacterScope,
} from "../lib/api";
import { useI18n } from "../lib/i18n";
import type { AuthCharacter, CharacterInfo, CharacterRoles } from "../lib/types";
import {
  CombinedOrdersTab,
  OptimizerTab,
  OverviewTab,
  PnLTab,
  RiskTab,
  TabBtn,
  TransactionsTab,
} from "./character-popup/CharacterPopupTabs";

interface CharacterPopupProps {
  open: boolean;
  onClose: () => void;
  activeCharacterId?: number;
  characters: AuthCharacter[];
  onSelectCharacter: (characterId: number) => Promise<void>;
  onDeleteCharacter: (characterId: number) => Promise<void>;
  onAddCharacter: () => Promise<void>;
  onAuthRefresh: () => Promise<void>;
}

type CharTab = "overview" | "orders" | "transactions" | "pnl" | "risk" | "optimizer";

export function CharacterPopup({
  open,
  onClose,
  activeCharacterId,
  characters,
  onSelectCharacter,
  onDeleteCharacter,
  onAddCharacter,
  onAuthRefresh,
}: CharacterPopupProps) {
  const { t } = useI18n();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [data, setData] = useState<CharacterInfo | null>(null);
  const [tab, setTab] = useState<CharTab>("overview");
  const [corpRoles, setCorpRoles] = useState<CharacterRoles | null>(null);
  const [corpRolesLoading, setCorpRolesLoading] = useState(false);
  const [selectedScope, setSelectedScope] = useState<CharacterScope>(activeCharacterId ?? "all");
  const [scopeBusy, setScopeBusy] = useState(false);
  const [deletingCharacterId, setDeletingCharacterId] = useState<number | null>(null);

  useEffect(() => {
    if (!open) return;
    if (activeCharacterId) {
      setSelectedScope(activeCharacterId);
      return;
    }
    setSelectedScope("all");
  }, [open, activeCharacterId]);

  useEffect(() => {
    if (!open) return;
    if (selectedScope === "all") return;
    if (characters.some((c) => c.character_id === selectedScope)) return;
    if (activeCharacterId) {
      setSelectedScope(activeCharacterId);
      return;
    }
    setSelectedScope("all");
  }, [open, selectedScope, characters, activeCharacterId]);

  const selectedCharacter = selectedScope === "all"
    ? null
    : characters.find((c) => c.character_id === selectedScope);
  const modalTitle = selectedScope === "all"
    ? t("charAllCharacters")
    : selectedCharacter?.character_name ?? t("charOverview");

  const loadData = useCallback(() => {
    setLoading(true);
    setError(null);
    getCharacterInfo(selectedScope)
      .then(setData)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, [selectedScope]);

  useEffect(() => {
    if (!open) return;
    loadData();
    if (selectedScope === "all") {
      setCorpRoles(null);
      setCorpRolesLoading(false);
      return;
    }
    // Also check corp roles for selected character
    setCorpRolesLoading(true);
    getCharacterRoles(undefined, selectedScope)
      .then(setCorpRoles)
      .catch(() => setCorpRoles(null))
      .finally(() => setCorpRolesLoading(false));
  }, [open, loadData, selectedScope]);

  const handleSelectScope = useCallback(async (scope: CharacterScope) => {
    if (scope === "all") {
      setSelectedScope("all");
      return;
    }
    if (selectedScope === scope) return;
    setScopeBusy(true);
    setError(null);
    try {
      await onSelectCharacter(scope);
      setSelectedScope(scope);
    } catch (e: any) {
      setError(e?.message || "Failed to switch character");
    } finally {
      setScopeBusy(false);
    }
  }, [selectedScope, onSelectCharacter]);

  const handleDeleteScope = useCallback(async (characterId: number) => {
    setDeletingCharacterId(characterId);
    setError(null);
    try {
      await onDeleteCharacter(characterId);
      await onAuthRefresh();
      if (selectedScope === characterId) {
        setSelectedScope("all");
      }
    } catch (e: any) {
      setError(e?.message || "Failed to remove character");
    } finally {
      setDeletingCharacterId(null);
    }
  }, [onDeleteCharacter, onAuthRefresh, selectedScope]);

  const handleAdd = useCallback(async () => {
    setScopeBusy(true);
    try {
      await onAddCharacter();
    } finally {
      setScopeBusy(false);
    }
  }, [onAddCharacter]);

  const formatIsk = (value: number) => {
    if (value >= 1e9) return `${(value / 1e9).toFixed(2)}B`;
    if (value >= 1e6) return `${(value / 1e6).toFixed(2)}M`;
    if (value >= 1e3) return `${(value / 1e3).toFixed(1)}K`;
    return value.toFixed(0);
  };

  const formatNumber = (value: number) => value.toLocaleString();

  const formatDate = (dateStr: string) => {
    const d = new Date(dateStr);
    return d.toLocaleDateString() + " " + d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  };

  const buyOrders = data?.orders.filter((o) => o.is_buy_order) ?? [];
  const sellOrders = data?.orders.filter((o) => !o.is_buy_order) ?? [];
  const totalBuyValue = buyOrders.reduce((sum, o) => sum + o.price * o.volume_remain, 0);
  const totalSellValue = sellOrders.reduce((sum, o) => sum + o.price * o.volume_remain, 0);

  // Calculate profit from recent transactions
  const recentTxns = data?.transactions ?? [];
  const buyTxns = recentTxns.filter((t) => t.is_buy);
  const sellTxns = recentTxns.filter((t) => !t.is_buy);
  const totalBought = buyTxns.reduce((sum, t) => sum + t.unit_price * t.quantity, 0);
  const totalSold = sellTxns.reduce((sum, t) => sum + t.unit_price * t.quantity, 0);

  return (
    <Modal open={open} onClose={onClose} title={modalTitle} width="max-w-5xl">
      <div className="flex flex-col h-[70vh]">
        {/* Character selector */}
        <div className="border-b border-eve-border bg-eve-panel/60 px-4 py-3 space-y-2">
          <div className="flex items-center justify-between gap-2">
            <div className="text-[10px] text-eve-dim uppercase tracking-wider">{t("charSelectCharacter")}</div>
            <button
              onClick={() => { void handleAdd(); }}
              disabled={scopeBusy}
              className="px-2 py-1 text-[10px] rounded-sm border border-eve-border bg-eve-dark text-eve-dim hover:text-eve-accent hover:border-eve-accent/50 transition-colors disabled:opacity-50"
            >
              {t("charAddCharacter")}
            </button>
          </div>
          <div className="flex flex-wrap gap-2">
            <button
              onClick={() => { void handleSelectScope("all"); }}
              className={`inline-flex items-center gap-1 px-2 py-1 rounded-sm border text-[11px] transition-colors ${
                selectedScope === "all"
                  ? "border-eve-accent bg-eve-accent/15 text-eve-accent"
                  : "border-eve-border bg-eve-dark text-eve-dim hover:text-eve-text hover:border-eve-accent/50"
              }`}
            >
              {t("charAllCharacters")}
            </button>
            {characters.map((character) => (
              <div key={character.character_id} className="inline-flex items-center rounded-sm border border-eve-border bg-eve-dark overflow-hidden">
                <button
                  onClick={() => { void handleSelectScope(character.character_id); }}
                  className={`inline-flex items-center gap-1.5 px-2 py-1 text-[11px] transition-colors ${
                    selectedScope === character.character_id
                      ? "text-eve-accent bg-eve-accent/10"
                      : "text-eve-dim hover:text-eve-text"
                  }`}
                >
                  <img
                    src={`https://images.evetech.net/characters/${character.character_id}/portrait?size=32`}
                    alt=""
                    className="w-4 h-4 rounded-sm"
                  />
                  <span>{character.character_name}</span>
                  {character.active && <span className="text-[9px] text-eve-dim">({t("charActive")})</span>}
                </button>
                <button
                  onClick={(event) => {
                    event.stopPropagation();
                    void handleDeleteScope(character.character_id);
                  }}
                  disabled={deletingCharacterId === character.character_id}
                  className="px-1.5 py-1 text-eve-dim hover:text-eve-error transition-colors disabled:opacity-50"
                  title={t("charRemoveCharacter")}
                  aria-label={t("charRemoveCharacter")}
                >
                  <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                  </svg>
                </button>
              </div>
            ))}
          </div>
        </div>

        {/* Tabs + Refresh */}
        <div className="flex items-center border-b border-eve-border bg-eve-panel">
          <div className="flex flex-1 overflow-x-auto scrollbar-thin">
            <TabBtn active={tab === "overview"} onClick={() => setTab("overview")} label={t("charOverview")} />
            <TabBtn active={tab === "orders"} onClick={() => setTab("orders")} label={`${t("charOrders")} (${data?.orders.length ?? 0})`} />
            <TabBtn active={tab === "transactions"} onClick={() => setTab("transactions")} label={`${t("charTransactions")} (${data?.transactions?.length ?? 0})`} />
            <TabBtn active={tab === "pnl"} onClick={() => setTab("pnl")} label={t("charPnlTab")} />
            <TabBtn active={tab === "risk"} onClick={() => setTab("risk")} label={t("charRiskTab")} />
            <TabBtn active={tab === "optimizer"} onClick={() => setTab("optimizer")} label={t("charOptimizerTab")} />
          </div>
          {/* Refresh button */}
          <button
            onClick={loadData}
            disabled={loading || scopeBusy}
            className="px-2 py-1.5 mr-2 text-eve-dim hover:text-eve-accent transition-colors disabled:opacity-50"
            title={t("charRefresh")}
          >
            <svg className={`w-4 h-4 ${loading ? "animate-spin" : ""}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
          </button>
        </div>

        {/* Content */}
        <div className="flex-1 overflow-auto p-4">
          {loading && !data && (
            <div className="flex items-center justify-center h-full text-eve-dim">{t("loading")}...</div>
          )}
          {error && !data && (
            <div className="flex items-center justify-center h-full text-eve-error">{error}</div>
          )}
          {data && (
            <>
              {tab === "overview" && (
                <OverviewTab
                  data={data}
                  characterId={selectedScope === "all" ? undefined : selectedScope}
                  isAllScope={selectedScope === "all"}
                  formatIsk={formatIsk}
                  formatNumber={formatNumber}
                  buyOrders={buyOrders}
                  sellOrders={sellOrders}
                  totalBuyValue={totalBuyValue}
                  totalSellValue={totalSellValue}
                  totalBought={totalBought}
                  totalSold={totalSold}
                  corpRoles={corpRoles}
                  corpRolesLoading={corpRolesLoading}
                  t={t}
                />
              )}
              {tab === "orders" && (
                <CombinedOrdersTab
                  characterScope={selectedScope}
                  orders={data.orders}
                  history={data.order_history ?? []}
                  formatIsk={formatIsk}
                  formatDate={formatDate}
                  t={t}
                />
              )}
              {tab === "transactions" && (
                <TransactionsTab transactions={data.transactions ?? []} formatIsk={formatIsk} formatDate={formatDate} t={t} />
              )}
              {tab === "pnl" && (
                <PnLTab formatIsk={formatIsk} characterScope={selectedScope} t={t} />
              )}
              {tab === "risk" && (
                <RiskTab
                  characterId={selectedScope === "all" ? undefined : selectedScope}
                  isAllScope={selectedScope === "all"}
                  data={data}
                  formatIsk={formatIsk}
                  t={t}
                />
              )}
              {tab === "optimizer" && (
                <OptimizerTab formatIsk={formatIsk} characterScope={selectedScope} t={t} />
              )}
            </>
          )}
        </div>
      </div>
    </Modal>
  );
}


