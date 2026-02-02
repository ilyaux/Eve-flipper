import type { AppConfig, AppStatus, ContractResult, FlipResult, NdjsonContractMessage, NdjsonMessage, NdjsonRouteMessage, RouteResult, ScanParams, ScanRecord, WatchlistItem } from "./types";

const BASE = import.meta.env.VITE_API_URL || "http://127.0.0.1:13370";

export async function getStatus(): Promise<AppStatus> {
  const res = await fetch(`${BASE}/api/status`);
  return res.json();
}

export async function getConfig(): Promise<AppConfig> {
  const res = await fetch(`${BASE}/api/config`);
  return res.json();
}

export async function updateConfig(patch: Partial<AppConfig>): Promise<AppConfig> {
  const res = await fetch(`${BASE}/api/config`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(patch),
  });
  return res.json();
}

export async function autocomplete(query: string): Promise<string[]> {
  const res = await fetch(`${BASE}/api/systems/autocomplete?q=${encodeURIComponent(query)}`);
  const data = await res.json();
  return data.systems ?? [];
}

export async function scan(
  params: ScanParams,
  onProgress: (msg: string) => void,
  signal?: AbortSignal
): Promise<FlipResult[]> {
  return streamScan(`${BASE}/api/scan`, params, onProgress, signal);
}

export async function scanMultiRegion(
  params: ScanParams,
  onProgress: (msg: string) => void,
  signal?: AbortSignal
): Promise<FlipResult[]> {
  return streamScan(`${BASE}/api/scan/multi-region`, params, onProgress, signal);
}

export async function scanContracts(
  params: ScanParams,
  onProgress: (msg: string) => void,
  signal?: AbortSignal
): Promise<ContractResult[]> {
  const res = await fetch(`${BASE}/api/scan/contracts`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(params),
    signal,
  });

  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error ?? "Contract scan failed");
  }

  const reader = res.body!.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let results: ContractResult[] = [];

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });

    const lines = buffer.split("\n");
    buffer = lines.pop() ?? "";

    for (const line of lines) {
      if (!line.trim()) continue;
      const msg: NdjsonContractMessage = JSON.parse(line);
      if (msg.type === "progress") {
        onProgress(msg.message);
      } else if (msg.type === "result") {
        results = msg.data ?? [];
      } else if (msg.type === "error") {
        throw new Error(msg.message);
      }
    }
  }

  if (buffer.trim()) {
    const msg: NdjsonContractMessage = JSON.parse(buffer);
    if (msg.type === "result") results = msg.data ?? [];
    else if (msg.type === "error") throw new Error(msg.message);
  }

  return results;
}

export async function findRoutes(
  params: ScanParams,
  minHops: number,
  maxHops: number,
  onProgress: (msg: string) => void,
  signal?: AbortSignal
): Promise<RouteResult[]> {
  const res = await fetch(`${BASE}/api/route/find`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      system_name: params.system_name,
      cargo_capacity: params.cargo_capacity,
      min_margin: params.min_margin,
      sales_tax_percent: params.sales_tax_percent,
      min_hops: minHops,
      max_hops: maxHops,
    }),
    signal,
  });

  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error ?? "Route search failed");
  }

  const reader = res.body!.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let results: RouteResult[] = [];

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });

    const lines = buffer.split("\n");
    buffer = lines.pop() ?? "";

    for (const line of lines) {
      if (!line.trim()) continue;
      const msg: NdjsonRouteMessage = JSON.parse(line);
      if (msg.type === "progress") {
        onProgress(msg.message);
      } else if (msg.type === "result") {
        results = msg.data ?? [];
      } else if (msg.type === "error") {
        throw new Error(msg.message);
      }
    }
  }

  if (buffer.trim()) {
    const msg: NdjsonRouteMessage = JSON.parse(buffer);
    if (msg.type === "result") results = msg.data ?? [];
    else if (msg.type === "error") throw new Error(msg.message);
  }

  return results;
}

// --- Watchlist ---

export async function getWatchlist(): Promise<WatchlistItem[]> {
  const res = await fetch(`${BASE}/api/watchlist`);
  return res.json();
}

export async function addToWatchlist(typeId: number, typeName: string, alertMinMargin: number = 0): Promise<WatchlistItem[]> {
  const res = await fetch(`${BASE}/api/watchlist`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ type_id: typeId, type_name: typeName, alert_min_margin: alertMinMargin }),
  });
  return res.json();
}

export async function removeFromWatchlist(typeId: number): Promise<WatchlistItem[]> {
  const res = await fetch(`${BASE}/api/watchlist/${typeId}`, { method: "DELETE" });
  return res.json();
}

export async function updateWatchlistItem(typeId: number, alertMinMargin: number): Promise<WatchlistItem[]> {
  const res = await fetch(`${BASE}/api/watchlist/${typeId}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ alert_min_margin: alertMinMargin }),
  });
  return res.json();
}

// --- Scan History ---

export async function getScanHistory(): Promise<ScanRecord[]> {
  const res = await fetch(`${BASE}/api/scan/history`);
  return res.json();
}

async function streamScan(
  url: string,
  params: ScanParams,
  onProgress: (msg: string) => void,
  signal?: AbortSignal
): Promise<FlipResult[]> {
  const res = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(params),
    signal,
  });

  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error ?? "Scan failed");
  }

  const reader = res.body!.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let results: FlipResult[] = [];

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });

    const lines = buffer.split("\n");
    buffer = lines.pop() ?? "";

    for (const line of lines) {
      if (!line.trim()) continue;
      const msg: NdjsonMessage = JSON.parse(line);
      if (msg.type === "progress") {
        onProgress(msg.message);
      } else if (msg.type === "result") {
        results = msg.data ?? [];
      } else if (msg.type === "error") {
        throw new Error(msg.message);
      }
    }
  }

  if (buffer.trim()) {
    const msg: NdjsonMessage = JSON.parse(buffer);
    if (msg.type === "result") results = msg.data ?? [];
    else if (msg.type === "error") throw new Error(msg.message);
  }

  return results;
}
