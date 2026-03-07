import { useEffect } from "react";
import { useI18n } from "../lib/i18n";

interface Props {
  open: boolean;
  onClose: () => void;
}

interface ShortcutRow {
  keys: string[];
  label: string;
}

interface ShortcutGroup {
  title: string;
  rows: ShortcutRow[];
}

function Kbd({ children }: { children: string }) {
  return (
    <kbd className="inline-flex items-center justify-center min-w-[1.6rem] px-1.5 py-0.5 text-[11px] font-mono bg-eve-dark border border-eve-border/60 rounded text-eve-text/80 shadow-sm">
      {children}
    </kbd>
  );
}

function ShortcutLine({ keys, label }: ShortcutRow) {
  return (
    <div className="flex items-center justify-between gap-4 py-1.5 border-b border-eve-border/10 last:border-0">
      <span className="text-[12px] text-eve-text/70">{label}</span>
      <div className="flex items-center gap-1 shrink-0">
        {keys.map((k, i) => (
          <span key={i} className="flex items-center gap-1">
            {i > 0 && <span className="text-eve-text/30 text-[10px]">+</span>}
            <Kbd>{k}</Kbd>
          </span>
        ))}
      </div>
    </div>
  );
}

export function KeyboardShortcutsHelp({ open, onClose }: Props) {
  const { t } = useI18n();

  const groups: ShortcutGroup[] = [
    {
      title: t("shortcutsGroupNav"),
      rows: [
        { keys: ["Alt", "1"], label: t("shortcutTabRadius") },
        { keys: ["Alt", "2"], label: t("shortcutTabRegion") },
        { keys: ["Alt", "3"], label: t("shortcutTabContracts") },
        { keys: ["Alt", "4"], label: t("shortcutTabStation") },
        { keys: ["Alt", "5"], label: t("shortcutTabRoute") },
        { keys: ["Alt", "W"], label: t("shortcutOpenWatchlist") },
        { keys: ["Alt", "H"], label: t("shortcutOpenHistory") },
      ],
    },
    {
      title: t("shortcutsGroupActions"),
      rows: [
        { keys: ["Ctrl", "S"], label: t("shortcutStartScan") },
        { keys: ["Ctrl", "K"], label: t("shortcutCommandPalette") },
        { keys: ["Alt", "Ctrl", "P"], label: t("shortcutOpenShortcuts") },
      ],
    },
    {
      title: t("shortcutsGroupTable"),
      rows: [
        { keys: ["↑", "↓"], label: t("shortcutTableNav") },
        { keys: ["Enter"], label: t("shortcutTableEnter") },
        { keys: ["D"], label: t("shortcutTableDone") },
        { keys: ["I"], label: t("shortcutTableIgnore") },
      ],
    },
  ];

  useEffect(() => {
    if (!open) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-[200] flex items-center justify-center bg-black/60 backdrop-blur-sm"
      onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}
    >
      <div className="w-full max-w-md bg-eve-panel border border-eve-accent/30 rounded shadow-eve-glow flex flex-col overflow-hidden">
        {/* Header */}
        <div className="flex items-center justify-between px-4 py-3 border-b border-eve-accent/20">
          <span className="text-sm font-semibold text-eve-text">{t("shortcutsTitle")}</span>
          <div className="flex items-center gap-3">
            <span className="text-[10px] text-eve-text/30">Esc</span>
            <button
              onClick={onClose}
              className="text-eve-text/40 hover:text-eve-text transition-colors text-lg leading-none"
            >
              ×
            </button>
          </div>
        </div>

        {/* Groups */}
        <div className="p-4 flex flex-col gap-4 overflow-y-auto max-h-[70vh]">
          {groups.map((group) => (
            <div key={group.title}>
              <div className="text-[10px] font-semibold uppercase tracking-wider text-eve-accent/60 mb-2">
                {group.title}
              </div>
              <div>
                {group.rows.map((row) => (
                  <ShortcutLine key={row.label} keys={row.keys} label={row.label} />
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
