export interface NormalizedColumnPrefs<T extends string> {
  order: T[];
  hidden: Set<T>;
  widths: Partial<Record<T, number>>;
  pinned: Set<T>;
}

export function normalizeColumnPrefs<T extends string>(
  raw: string | null,
  defaultOrder: T[],
): NormalizedColumnPrefs<T> {
  const available = new Set<T>(defaultOrder);
  let order = defaultOrder;
  const hidden = new Set<T>();
  const widths: Partial<Record<T, number>> = {};
  const pinned = new Set<T>();

  try {
    if (raw) {
      const parsed = JSON.parse(raw) as {
        order?: string[];
        hidden?: string[];
        widths?: Record<string, number>;
        pinned?: string[];
      };
      if (Array.isArray(parsed.order)) {
        const saved = parsed.order.filter((key): key is T => available.has(key as T));
        const missing = defaultOrder.filter((key) => !saved.includes(key));
        order = [...saved, ...missing];
      }
      if (Array.isArray(parsed.hidden)) {
        for (const key of parsed.hidden) {
          if (available.has(key as T)) hidden.add(key as T);
        }
      }
      if (parsed.widths && typeof parsed.widths === "object") {
        for (const [key, value] of Object.entries(parsed.widths)) {
          if (available.has(key as T) && typeof value === "number" && Number.isFinite(value)) {
            widths[key as T] = Math.max(44, Math.min(520, Math.round(value)));
          }
        }
      }
      if (Array.isArray(parsed.pinned)) {
        for (const key of parsed.pinned) {
          if (available.has(key as T)) pinned.add(key as T);
        }
      }
    }
  } catch {
    // Malformed table settings should not break rendering.
  }

  if (hidden.size >= order.length && order.length > 0) {
    hidden.delete(order[0]);
  }

  return { order, hidden, widths, pinned };
}
