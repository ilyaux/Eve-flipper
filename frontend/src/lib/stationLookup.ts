import { getStations } from "./api";
import type { StationsResponse } from "./types";

const LOOKUP_RETRY_DELAY_MS = 500;
const LOOKUP_ATTEMPTS = 40;

function waitForRetry(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) {
      reject(new DOMException("Station lookup aborted", "AbortError"));
      return;
    }

    const timer = setTimeout(resolve, ms);
    signal?.addEventListener(
      "abort",
      () => {
        clearTimeout(timer);
        reject(new DOMException("Station lookup aborted", "AbortError"));
      },
      { once: true },
    );
  });
}

export async function getStationsWhenReady(
  systemName: string,
  signal?: AbortSignal,
  fetchStations = getStations,
  attempts = LOOKUP_ATTEMPTS,
  retryDelayMs = LOOKUP_RETRY_DELAY_MS,
): Promise<StationsResponse> {
  let response = await fetchStations(systemName, signal);

  for (let attempt = 1; response.system_id <= 0 && attempt < attempts; attempt++) {
    await waitForRetry(retryDelayMs, signal);
    response = await fetchStations(systemName, signal);
  }

  return response;
}
