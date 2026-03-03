import { useEffect, useMemo, useState } from "react";
import { getSystemsList } from "@/lib/api";
import type { SolarSystemInfo } from "@/lib/types";
import { useI18n } from "@/lib/i18n";
import { Modal } from "./Modal";

interface Props {
  value: number[];
  onChange: (ids: number[]) => void;
  compact?: boolean;
}

function toSortedUnique(ids: number[]): number[] {
  return [...new Set(ids.filter((id) => Number.isFinite(id) && id > 0))].sort(
    (a, b) => a - b,
  );
}

export function SystemBlacklistButton({ value, onChange, compact = false }: Props) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [systems, setSystems] = useState<SolarSystemInfo[]>([]);
  const [search, setSearch] = useState("");
  const [draft, setDraft] = useState<Set<number>>(new Set());
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open) return;
    setDraft(new Set(toSortedUnique(value)));
    setSearch("");
  }, [open, value]);

  useEffect(() => {
    if (!open || systems.length > 0) return;
    const controller = new AbortController();
    setLoading(true);
    setError("");
    getSystemsList(undefined, undefined, controller.signal)
      .then((rows) => {
        setSystems(rows);
      })
      .catch((e: unknown) => {
        if (controller.signal.aborted) return;
        setError(e instanceof Error ? e.message : "Failed to load systems");
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [open, systems.length]);

  const selectedCount = value?.length ?? 0;

  const displaySystems = useMemo(() => {
    const q = search.trim().toLowerCase();
    const selected = systems.filter((row) => draft.has(row.id));
    if (q.length > 0) {
      const filtered = systems.filter((row) =>
        row.name.toLowerCase().includes(q),
      );
      return filtered.slice(0, 1000);
    }
    const remaining = systems
      .filter((row) => !draft.has(row.id))
      .slice(0, 300);
    return [...selected, ...remaining];
  }, [draft, search, systems]);

  const toggleSystem = (systemID: number, checked: boolean) => {
    setDraft((prev) => {
      const next = new Set(prev);
      if (checked) next.add(systemID);
      else next.delete(systemID);
      return next;
    });
  };

  const apply = () => {
    onChange(toSortedUnique(Array.from(draft)));
    setOpen(false);
  };

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        className={
          compact
            ? "relative p-1 text-eve-dim hover:text-eve-accent transition-colors"
            : "h-[34px] px-2.5 rounded-sm border border-eve-border bg-eve-input text-eve-dim hover:text-eve-accent hover:border-eve-accent/50 transition-colors text-xs font-medium"
        }
        title={t("systemBlacklistOpen")}
      >
        {compact ? (
          <>
            <svg
              className={`w-4 h-4 ${selectedCount > 0 ? "text-eve-accent" : ""}`}
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.8"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <circle cx="12" cy="12" r="8" />
              <path d="M8.8 8.8l6.4 6.4" />
            </svg>
            {selectedCount > 0 && (
              <span className="absolute -top-1 -right-1 min-w-[14px] h-[14px] px-[2px] rounded-full bg-eve-accent text-eve-dark text-[9px] font-mono leading-[14px] text-center">
                {selectedCount > 99 ? "99+" : selectedCount}
              </span>
            )}
          </>
        ) : (
          <span className="inline-flex items-center gap-1.5">
            <svg
              className={`w-4 h-4 ${selectedCount > 0 ? "text-eve-accent" : ""}`}
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.8"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <circle cx="12" cy="12" r="8" />
              <path d="M8.8 8.8l6.4 6.4" />
            </svg>
            <span>{selectedCount}</span>
          </span>
        )}
      </button>

      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title={t("systemBlacklistTitle")}
        width="max-w-3xl"
      >
        <div className="p-3 space-y-3">
          <div className="flex items-center gap-2">
            <input
              type="text"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder={t("systemBlacklistSearch")}
              className="flex-1 px-3 py-1.5 bg-eve-input border border-eve-border rounded-sm text-eve-text text-sm focus:outline-none focus:border-eve-accent focus:ring-1 focus:ring-eve-accent/30"
            />
            <button
              type="button"
              onClick={() => setDraft(new Set())}
              className="px-2.5 py-1.5 text-xs border border-eve-border rounded-sm text-eve-dim hover:text-eve-accent hover:border-eve-accent/50 transition-colors"
            >
              {t("clear")}
            </button>
          </div>

          <div className="text-xs text-eve-dim">
            {t("systemBlacklistSelected", { count: draft.size })}
          </div>

          <div className="max-h-[52vh] overflow-y-auto border border-eve-border/60 rounded-sm divide-y divide-eve-border/40">
            {loading && (
              <div className="px-3 py-4 text-sm text-eve-dim">
                {t("loading")}
              </div>
            )}
            {!loading && error && (
              <div className="px-3 py-4 text-sm text-red-400">{error}</div>
            )}
            {!loading && !error && displaySystems.length === 0 && (
              <div className="px-3 py-4 text-sm text-eve-dim">
                {t("systemBlacklistEmpty")}
              </div>
            )}
            {!loading &&
              !error &&
              displaySystems.map((system) => {
                const checked = draft.has(system.id);
                return (
                  <label
                    key={system.id}
                    className="flex items-center gap-2 px-3 py-2 text-sm cursor-pointer hover:bg-eve-panel/50"
                  >
                    <input
                      type="checkbox"
                      checked={checked}
                      onChange={(e) => toggleSystem(system.id, e.target.checked)}
                      className="accent-eve-accent"
                    />
                    <span className="text-eve-text flex-1 min-w-0 truncate">
                      {system.name}
                    </span>
                    <span className="text-[11px] text-eve-dim font-mono">
                      {system.security.toFixed(1)}
                    </span>
                  </label>
                );
              })}
          </div>

          <div className="flex items-center justify-end gap-2">
            <button
              type="button"
              onClick={() => setOpen(false)}
              className="px-3 py-1.5 text-xs border border-eve-border rounded-sm text-eve-dim hover:text-eve-text transition-colors"
            >
              {t("cancel")}
            </button>
            <button
              type="button"
              onClick={apply}
              className="px-3 py-1.5 text-xs rounded-sm bg-eve-accent text-eve-dark hover:bg-eve-accent-hover transition-colors font-medium"
            >
              {t("systemBlacklistApply")}
            </button>
          </div>
        </div>
      </Modal>
    </>
  );
}
