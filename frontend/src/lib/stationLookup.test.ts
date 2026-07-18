import { describe, expect, it, vi } from "vitest";
import type { StationsResponse } from "./types";
import { getStationsWhenReady } from "./stationLookup";

const loading: StationsResponse = { stations: [], region_id: 0, system_id: 0 };
const jita: StationsResponse = {
  stations: [
    {
      id: 60003760,
      name: "Jita IV - Moon 4 - Caldari Navy Assembly Plant",
      system_id: 30000142,
      region_id: 10000002,
    },
  ],
  region_id: 10000002,
  system_id: 30000142,
};

describe("getStationsWhenReady", () => {
  it("retries a cold SDE lookup until the system resolves", async () => {
    vi.useFakeTimers();
    const fetchStations = vi
      .fn()
      .mockResolvedValueOnce(loading)
      .mockResolvedValueOnce(jita);

    const resultPromise = getStationsWhenReady(
      "Jita",
      undefined,
      fetchStations,
      3,
      10,
    );
    await vi.advanceTimersByTimeAsync(10);

    await expect(resultPromise).resolves.toEqual(jita);
    expect(fetchStations).toHaveBeenCalledTimes(2);
    vi.useRealTimers();
  });

  it("does not retry a resolved system that has no NPC stations", async () => {
    const emptySystem: StationsResponse = {
      stations: [],
      region_id: 10000069,
      system_id: 30005196,
    };
    const fetchStations = vi.fn().mockResolvedValue(emptySystem);

    await expect(
      getStationsWhenReady("C-J6MT", undefined, fetchStations, 3, 0),
    ).resolves.toEqual(emptySystem);
    expect(fetchStations).toHaveBeenCalledOnce();
  });
});
