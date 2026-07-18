import { describe, expect, it } from "vitest";
import { normalizeColumnPrefs } from "./tablePrefs";

describe("table preferences normalization", () => {
  const defaults = ["Item", "Profit", "Jumps"] as const;

  it("ignores malformed stored preferences", () => {
    const prefs = normalizeColumnPrefs("{bad json", [...defaults]);

    expect(prefs.order).toEqual(["Item", "Profit", "Jumps"]);
    expect([...prefs.hidden]).toEqual([]);
    expect(prefs.widths).toEqual({});
    expect([...prefs.pinned]).toEqual([]);
  });

  it("drops stale keys, restores missing columns, and clamps widths", () => {
    const prefs = normalizeColumnPrefs(
      JSON.stringify({
        order: ["Profit", "OldColumn"],
        hidden: ["OldColumn", "Profit"],
        widths: { Item: 12, Profit: 900, Jumps: 88, OldColumn: 100 },
        pinned: ["Jumps", "OldColumn"],
      }),
      [...defaults],
    );

    expect(prefs.order).toEqual(["Profit", "Item", "Jumps"]);
    expect([...prefs.hidden]).toEqual(["Profit"]);
    expect(prefs.widths).toEqual({ Item: 44, Profit: 520, Jumps: 88 });
    expect([...prefs.pinned]).toEqual(["Jumps"]);
  });

  it("keeps at least one column visible", () => {
    const prefs = normalizeColumnPrefs(
      JSON.stringify({
        order: ["Profit", "Item", "Jumps"],
        hidden: ["Profit", "Item", "Jumps"],
      }),
      [...defaults],
    );

    expect(prefs.hidden.has("Profit")).toBe(false);
    expect(prefs.hidden.has("Item")).toBe(true);
    expect(prefs.hidden.has("Jumps")).toBe(true);
  });
});
