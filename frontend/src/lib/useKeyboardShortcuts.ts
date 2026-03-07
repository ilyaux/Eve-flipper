import { useEffect, useCallback } from "react";

export type ModifierKey = "ctrl" | "alt" | "shift" | "meta";

export interface ShortcutHandler {
  key: string;
  modifiers?: readonly ModifierKey[];
  handler: () => void;
  description?: string;
  /** If true, prevents default browser behavior */
  preventDefault?: boolean;
  /** If true, shortcut works even when input is focused */
  allowInInput?: boolean;
}

export function useKeyboardShortcuts(shortcuts: ShortcutHandler[]) {
  const handleKeyDown = useCallback(
    (event: KeyboardEvent) => {
      // Check if user is typing in an input
      const target = event.target as HTMLElement;
      const isInput = target.tagName === "INPUT" || 
                      target.tagName === "TEXTAREA" || 
                      target.isContentEditable;

      for (const shortcut of shortcuts) {
        // Skip if in input and shortcut doesn't allow it
        if (isInput && !shortcut.allowInInput) continue;

        const modifiers = shortcut.modifiers ?? [];
        const needsCtrl = modifiers.includes("ctrl");
        const needsAlt = modifiers.includes("alt");
        const needsShift = modifiers.includes("shift");
        const needsMeta = modifiers.includes("meta");

        const ctrlMatch = needsCtrl === (event.ctrlKey || event.metaKey);
        const altMatch = needsAlt === event.altKey;
        const shiftMatch = needsShift === event.shiftKey;
        const metaMatch = needsMeta === event.metaKey;

        // Use event.code for layout-independent matching (e.g. Russian keyboard "з" = KeyP)
        const codeKey = event.code.replace(/^(Key|Digit)/, "").toLowerCase();
        const keyMatch =
          codeKey === shortcut.key.toLowerCase() ||
          event.key.toLowerCase() === shortcut.key.toLowerCase();

        if (keyMatch && ctrlMatch && altMatch && shiftMatch && metaMatch) {
          if (shortcut.preventDefault !== false) {
            event.preventDefault();
          }
          shortcut.handler();
          return;
        }
      }
    },
    [shortcuts]
  );

  useEffect(() => {
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [handleKeyDown]);
}

// Utility to format shortcut for display
export function formatShortcut(key: string, modifiers?: ModifierKey[]): string {
  const parts: string[] = [];
  
  if (modifiers?.includes("ctrl")) {
    parts.push(navigator.platform.includes("Mac") ? "⌘" : "Ctrl");
  }
  if (modifiers?.includes("alt")) {
    parts.push(navigator.platform.includes("Mac") ? "⌥" : "Alt");
  }
  if (modifiers?.includes("shift")) {
    parts.push("⇧");
  }
  
  parts.push(key.toUpperCase());
  
  return parts.join("+");
}
