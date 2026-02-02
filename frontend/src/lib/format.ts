export function formatISK(value: number): string {
  if (value >= 1_000_000_000) {
    return (value / 1_000_000_000).toFixed(2) + " B";
  }
  if (value >= 1_000_000) {
    return (value / 1_000_000).toFixed(2) + " M";
  }
  if (value >= 1_000) {
    return (value / 1_000).toFixed(1) + " K";
  }
  return value.toFixed(1);
}

export function formatMargin(value: number): string {
  return value.toFixed(1) + "%";
}

export function formatNumber(value: number): string {
  return value.toLocaleString("ru-RU");
}
