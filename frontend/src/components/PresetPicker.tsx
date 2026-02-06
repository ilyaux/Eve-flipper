import { useState, useRef, useEffect, useCallback } from "react";
import { useI18n, type TranslationKey } from "@/lib/i18n";
import { useGlobalToast } from "./Toast";
import {
  loadCustomPresets,
  saveCustomPreset,
  deleteCustomPreset,
  nextPresetId,
  exportPresets,
  importPresets,
  mapTabToPresetTab,
  type SavedPreset,
  type BuiltinPreset,
} from "@/lib/presets";

/* eslint-disable @typescript-eslint/no-explicit-any */
interface Props {
  params: Record<string, any>;
  onApply: (params: any) => void;
  tab: string;
  builtinPresets: BuiltinPreset[];
  /** Which edge the dropdown aligns to. Default "left". */
  align?: "left" | "right";
}
/* eslint-enable @typescript-eslint/no-explicit-any */

export function PresetPicker({ params, onApply, tab, builtinPresets, align = "left" }: Props) {
  const { t } = useI18n();
  const { addToast } = useGlobalToast();
  const [open, setOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saveName, setSaveName] = useState("");
  const activeKey = `eve-flipper-active-preset-${tab}`;
  const [activePresetId, setActivePresetIdRaw] = useState<string | null>(() => {
    try { return localStorage.getItem(activeKey); } catch { return null; }
  });
  const setActivePresetId = useCallback((id: string | null) => {
    setActivePresetIdRaw(id);
    try {
      if (id) localStorage.setItem(activeKey, id);
      else localStorage.removeItem(activeKey);
    } catch { /* ignore */ }
  }, [activeKey]);

  const [customPresets, setCustomPresets] = useState<SavedPreset[]>(() =>
    loadCustomPresets(tab),
  );
  const ref = useRef<HTMLDivElement>(null);

  // Close on outside click
  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
        setSaving(false);
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [open]);

  // Close on Escape
  useEffect(() => {
    if (!open) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setOpen(false);
        setSaving(false);
      }
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [open]);

  // Reload when tab changes
  useEffect(() => {
    setCustomPresets(loadCustomPresets(tab));
  }, [tab]);

  const handleApply = useCallback(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (id: string, presetParams: Record<string, any>) => {
      onApply({ ...params, ...presetParams });
      setActivePresetId(id);
      setOpen(false);
    },
    [params, onApply],
  );

  const handleDelete = (e: React.MouseEvent, id: string) => {
    e.stopPropagation();
    deleteCustomPreset(id);
    setCustomPresets(loadCustomPresets(tab));
    if (activePresetId === id) setActivePresetId(null);
    addToast(t("presetDeleted" as TranslationKey) || "Preset deleted", "success", 2000);
  };

  const handleSave = () => {
    if (!saveName.trim()) return;
    const preset: SavedPreset = {
      id: nextPresetId(),
      name: saveName.trim(),
      tab: mapTabToPresetTab(tab),
      params: { ...params },
      createdAt: Date.now(),
    };
    saveCustomPreset(preset);
    setCustomPresets(loadCustomPresets(tab));
    setActivePresetId(preset.id);
    setSaveName("");
    setSaving(false);
    addToast(t("presetSaved"), "success", 2000);
  };

  const handleUpdate = () => {
    if (!activePresetId) return;
    const existing = customPresets.find((p) => p.id === activePresetId);
    if (!existing) return;
    saveCustomPreset({ ...existing, params: { ...params } });
    setCustomPresets(loadCustomPresets(tab));
    addToast(
      t("presetUpdated" as TranslationKey) || "Preset updated",
      "success",
      2000,
    );
  };

  const handleExport = () => {
    const json = exportPresets();
    navigator.clipboard.writeText(json);
    addToast(
      t("presetExported" as TranslationKey) || "Presets copied to clipboard",
      "success",
      2000,
    );
    setOpen(false);
  };

  const handleImport = async () => {
    try {
      const json = await navigator.clipboard.readText();
      const result = importPresets(json);
      if (result.error) {
        addToast(result.error, "error", 3000);
      } else {
        setCustomPresets(loadCustomPresets(tab));
        addToast(
          `${t("presetImported" as TranslationKey) || "Imported"}: ${result.imported}`,
          "success",
          2000,
        );
      }
    } catch {
      addToast("Clipboard access denied", "error", 3000);
    }
    setOpen(false);
  };

  // Active preset label
  const activeLabel = (() => {
    if (!activePresetId) return null;
    const b = builtinPresets.find((p) => p.id === activePresetId);
    if (b) return t(b.nameKey as TranslationKey);
    const c = customPresets.find((p) => p.id === activePresetId);
    return c?.name || null;
  })();

  const isCustomActive = activePresetId
    ? customPresets.some((p) => p.id === activePresetId)
    : false;

  return (
    <div className="relative" ref={ref}>
      {/* Trigger button */}
      <div className="flex items-center gap-1.5">
        <span className="text-[10px] uppercase tracking-wider text-eve-dim font-medium shrink-0">
          {t("presetLabel")}
        </span>
        <button
          type="button"
          onClick={() => {
            setOpen(!open);
            setSaving(false);
          }}
          className={`flex items-center gap-1.5 min-w-0 max-w-[160px] px-2.5 py-1 bg-eve-input border rounded text-sm transition-colors ${
            open
              ? "border-eve-accent text-eve-accent"
              : "border-eve-border text-eve-text hover:border-eve-accent/50"
          }`}
        >
          <span className="truncate">{activeLabel || "—"}</span>
          <span className="text-[10px] text-eve-dim shrink-0">▾</span>
        </button>
      </div>

      {/* Dropdown panel */}
      {open && (
        <div className={`absolute top-full mt-1 w-72 bg-eve-panel border border-eve-border rounded-sm shadow-2xl z-50 overflow-hidden ${align === "right" ? "right-0" : "left-0"}`}>
          {/* Built-in presets */}
          <div className="px-3 pt-2.5 pb-1.5">
            <div className="text-[10px] uppercase tracking-wider text-eve-dim font-medium mb-1.5">
              {t("presetBuiltin" as TranslationKey) || "Built-in"}
            </div>
            <div className="space-y-0.5">
              {builtinPresets.map((p) => (
                <button
                  key={p.id}
                  onClick={() => handleApply(p.id, p.params)}
                  className={`w-full flex items-center justify-between px-2 py-1.5 rounded-sm text-sm transition-colors ${
                    activePresetId === p.id
                      ? "bg-eve-accent/15 text-eve-accent"
                      : "text-eve-text hover:bg-eve-dark/50"
                  }`}
                >
                  <span>{t(p.nameKey as TranslationKey)}</span>
                  {activePresetId === p.id && (
                    <span className="text-eve-accent text-xs">✓</span>
                  )}
                </button>
              ))}
            </div>
          </div>

          <div className="border-t border-eve-border/50" />

          {/* Custom presets */}
          <div className="px-3 pt-2 pb-1.5">
            <div className="text-[10px] uppercase tracking-wider text-eve-dim font-medium mb-1.5">
              {t("presetCustom" as TranslationKey) || "Custom"}
            </div>
            {customPresets.length === 0 ? (
              <div className="text-xs text-eve-dim py-1 px-2">
                {t("presetNoCustom" as TranslationKey) || "No custom presets yet"}
              </div>
            ) : (
              <div className="space-y-0.5 max-h-[160px] overflow-y-auto">
                {customPresets.map((p) => (
                  <div key={p.id} className="flex items-center gap-1 group">
                    <button
                      onClick={() => handleApply(p.id, p.params)}
                      className={`flex-1 flex items-center justify-between px-2 py-1.5 rounded-sm text-sm transition-colors text-left min-w-0 ${
                        activePresetId === p.id
                          ? "bg-eve-accent/15 text-eve-accent"
                          : "text-eve-text hover:bg-eve-dark/50"
                      }`}
                    >
                      <span className="truncate">{p.name}</span>
                      {activePresetId === p.id && (
                        <span className="text-eve-accent text-xs shrink-0 ml-1">
                          ✓
                        </span>
                      )}
                    </button>
                    <button
                      onClick={(e) => handleDelete(e, p.id)}
                      className="shrink-0 w-6 h-6 flex items-center justify-center text-eve-dim hover:text-red-400 opacity-0 group-hover:opacity-100 transition-all rounded-sm hover:bg-red-500/10"
                      title={
                        t("presetDelete" as TranslationKey) || "Delete"
                      }
                    >
                      ✕
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>

          <div className="border-t border-eve-border/50" />

          {/* Save section */}
          <div className="px-3 py-2">
            {saving ? (
              <div className="space-y-1.5">
                <input
                  type="text"
                  value={saveName}
                  onChange={(e) => setSaveName(e.target.value)}
                  onKeyDown={(e) => e.key === "Enter" && handleSave()}
                  placeholder={
                    t("presetNamePlaceholder" as TranslationKey) ||
                    "Preset name..."
                  }
                  className="w-full px-2 py-1.5 bg-eve-input border border-eve-border rounded text-sm text-eve-text focus:outline-none focus:border-eve-accent focus:ring-1 focus:ring-eve-accent/30"
                  autoFocus
                />
                <div className="flex gap-1.5">
                  <button
                    onClick={handleSave}
                    disabled={!saveName.trim()}
                    className="flex-1 px-2 py-1 text-xs bg-eve-accent text-eve-dark rounded-sm hover:bg-eve-accent-hover disabled:opacity-40 disabled:cursor-not-allowed transition-colors font-medium"
                  >
                    {t("presetSaveBtn" as TranslationKey) || "Save"}
                  </button>
                  <button
                    onClick={() => {
                      setSaving(false);
                      setSaveName("");
                    }}
                    className="px-2 py-1 text-xs bg-eve-dark text-eve-dim rounded-sm hover:text-eve-text transition-colors"
                  >
                    {t("cancel" as TranslationKey) || "Cancel"}
                  </button>
                </div>
              </div>
            ) : (
              <div className="flex gap-1.5">
                <button
                  onClick={() => setSaving(true)}
                  className="flex-1 px-2 py-1.5 text-xs bg-eve-dark border border-eve-border rounded-sm text-eve-text hover:border-eve-accent/50 transition-colors"
                >
                  +{" "}
                  {t("presetSaveNew" as TranslationKey) || "Save current"}
                </button>
                {isCustomActive && (
                  <button
                    onClick={handleUpdate}
                    className="px-2 py-1.5 text-xs bg-eve-dark border border-eve-border rounded-sm text-eve-accent hover:border-eve-accent/50 transition-colors"
                    title={
                      t("presetUpdate" as TranslationKey) ||
                      "Update active preset"
                    }
                  >
                    ↻
                  </button>
                )}
              </div>
            )}
          </div>

          <div className="border-t border-eve-border/50" />

          {/* Export / Import */}
          <div className="px-3 py-2 flex gap-1.5">
            <button
              onClick={handleExport}
              className="flex-1 px-2 py-1 text-[11px] text-eve-dim hover:text-eve-text transition-colors"
            >
              {t("presetExport" as TranslationKey) || "Export"}
            </button>
            <span className="text-eve-border">|</span>
            <button
              onClick={handleImport}
              className="flex-1 px-2 py-1 text-[11px] text-eve-dim hover:text-eve-text transition-colors"
            >
              {t("presetImport" as TranslationKey) || "Import"}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
