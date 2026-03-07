import { useEffect, useRef, useState } from "react";
import { useI18n } from "../lib/i18n";

type TabKey = "radius" | "region" | "contracts" | "station" | "route" | "industry" | "demand" | "plex";

interface CommandItem {
  id: string;
  label: string;
  shortcut?: string;
  action: () => void;
}

interface Props {
  open: boolean;
  onClose: () => void;
  onSwitchTab: (tab: TabKey) => void;
  onOpenWatchlist: () => void;
  onOpenHistory: () => void;
  onOpenCharacter: () => void;
  onStartScan: () => void;
}

export function CommandPalette({ open, onClose, onSwitchTab, onOpenWatchlist, onOpenHistory, onOpenCharacter, onStartScan }: Props) {
  const { t } = useI18n();
  const [query, setQuery] = useState("");
  const [focused, setFocused] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLUListElement>(null);

  const commands: CommandItem[] = [
    { id: "tab-radius",    label: t("cmdSwitchToRadius"),    shortcut: "Alt+1", action: () => { onSwitchTab("radius");    onClose(); } },
    { id: "tab-region",    label: t("cmdSwitchToRegion"),    shortcut: "Alt+2", action: () => { onSwitchTab("region");    onClose(); } },
    { id: "tab-contracts", label: t("cmdSwitchToContracts"), shortcut: "Alt+3", action: () => { onSwitchTab("contracts"); onClose(); } },
    { id: "tab-station",   label: t("cmdSwitchToStation"),   shortcut: "Alt+4", action: () => { onSwitchTab("station");   onClose(); } },
    { id: "tab-route",     label: t("cmdSwitchToRoute"),     shortcut: "Alt+5", action: () => { onSwitchTab("route");     onClose(); } },
    { id: "tab-industry",  label: t("cmdSwitchToIndustry"),  action: () => { onSwitchTab("industry");  onClose(); } },
    { id: "tab-demand",    label: t("cmdSwitchToDemand"),    action: () => { onSwitchTab("demand");    onClose(); } },
    { id: "tab-plex",      label: t("cmdSwitchToPlex"),      action: () => { onSwitchTab("plex");      onClose(); } },
    { id: "watchlist",     label: t("cmdOpenWatchlist"),     shortcut: "Alt+W", action: () => { onOpenWatchlist(); onClose(); } },
    { id: "history",       label: t("cmdOpenHistory"),       shortcut: "Alt+H", action: () => { onOpenHistory();  onClose(); } },
    { id: "character",     label: t("cmdOpenCharacter"),     action: () => { onOpenCharacter(); onClose(); } },
    { id: "scan",          label: t("cmdStartScan"),         shortcut: "Ctrl+S", action: () => { onStartScan();   onClose(); } },
  ];

  const filtered = query.trim()
    ? commands.filter((c) => c.label.toLowerCase().includes(query.toLowerCase()))
    : commands;

  // Reset state when opened
  useEffect(() => {
    if (open) {
      setQuery("");
      setFocused(0);
      setTimeout(() => inputRef.current?.focus(), 0);
    }
  }, [open]);

  // Clamp focused index when filter changes
  useEffect(() => {
    setFocused((prev) => Math.min(prev, Math.max(0, filtered.length - 1)));
  }, [filtered.length]);

  // Scroll focused item into view
  useEffect(() => {
    const item = listRef.current?.children[focused] as HTMLElement | undefined;
    item?.scrollIntoView({ block: "nearest" });
  }, [focused]);

  useEffect(() => {
    if (!open) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") { onClose(); return; }
      if (e.key === "ArrowDown") { e.preventDefault(); setFocused((p) => Math.min(p + 1, filtered.length - 1)); return; }
      if (e.key === "ArrowUp")   { e.preventDefault(); setFocused((p) => Math.max(p - 1, 0)); return; }
      if (e.key === "Enter" && filtered[focused]) { filtered[focused].action(); return; }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [open, filtered, focused, onClose]);

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-[200] flex items-start justify-center pt-[15vh] bg-black/60 backdrop-blur-sm"
      onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}
    >
      <div className="w-full max-w-lg bg-eve-panel border border-eve-accent/30 rounded shadow-eve-glow flex flex-col overflow-hidden">
        <div className="flex items-center gap-2 px-4 py-3 border-b border-eve-accent/20">
          <span className="text-eve-accent opacity-60 text-sm">⌘</span>
          <input
            ref={inputRef}
            className="flex-1 bg-transparent text-eve-text text-sm outline-none placeholder:text-eve-text/40"
            placeholder={t("cmdPalettePlaceholder")}
            value={query}
            onChange={(e) => { setQuery(e.target.value); setFocused(0); }}
          />
          <kbd className="text-xs text-eve-text/40 border border-eve-text/20 rounded px-1">Esc</kbd>
        </div>

        <ul ref={listRef} className="max-h-72 overflow-y-auto py-1">
          {filtered.length === 0 ? (
            <li className="px-4 py-3 text-xs text-eve-text/40">{t("cmdPaletteNoResults")}</li>
          ) : (
            filtered.map((cmd, idx) => (
              <li
                key={cmd.id}
                className={`flex items-center justify-between px-4 py-2.5 text-sm cursor-pointer transition-colors
                  ${idx === focused ? "bg-eve-accent/15 text-eve-accent" : "text-eve-text hover:bg-eve-accent/10"}`}
                onMouseEnter={() => setFocused(idx)}
                onClick={cmd.action}
              >
                <span>{cmd.label}</span>
                {cmd.shortcut && (
                  <kbd className="text-xs text-eve-text/40 border border-eve-text/20 rounded px-1.5 py-0.5 ml-4">
                    {cmd.shortcut}
                  </kbd>
                )}
              </li>
            ))
          )}
        </ul>
      </div>
    </div>
  );
}
