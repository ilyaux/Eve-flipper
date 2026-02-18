import { useCallback, useState } from "react";
import { type TranslationKey } from "../../lib/i18n";
import type { DailyPnLEntry, IncomeSource, MemberContribution } from "../../lib/types";
const BAR_COLORS: Record<string, { normal: string; hover: string }> = {
  blue:    { normal: "rgba(59,130,246,0.5)",  hover: "rgba(59,130,246,0.85)" },
  emerald: { normal: "rgba(16,185,129,0.5)",  hover: "rgba(16,185,129,0.85)" },
  amber:   { normal: "rgba(245,158,11,0.5)",  hover: "rgba(245,158,11,0.85)" },
  red:     { normal: "rgba(239,68,68,0.5)",   hover: "rgba(239,68,68,0.85)" },
};

export function BarChart({ data, label, formatValue, color = "blue" }: {
  data: { date: string; value: number }[];
  label: string;
  formatValue: (v: number) => string;
  color?: string;
}) {
  const [hoveredIdx, setHoveredIdx] = useState<number | null>(null);

  // Show last 60 entries
  const visible = data.slice(-60);
  const maxVal = Math.max(...visible.map(d => d.value), 1);

  // Compute totals for the summary line
  const total = visible.reduce((s, d) => s + d.value, 0);
  const avg = visible.length > 0 ? total / visible.length : 0;

  const palette = BAR_COLORS[color] || BAR_COLORS.blue;

  return (
    <div className="bg-eve-panel border border-eve-border rounded-sm p-4">
      <div className="flex items-center justify-between mb-3">
        <div className="text-[10px] text-eve-dim uppercase tracking-wider">{label}</div>
        <div className="flex items-center gap-4 text-[10px] text-eve-dim">
          <span>Total: <span className="text-eve-text font-medium">{formatValue(total)}</span></span>
          <span>Avg: <span className="text-eve-text font-medium">{formatValue(Math.round(avg))}</span></span>
        </div>
      </div>

      {/* Tooltip */}
      <div className="h-5 mb-1">
        {hoveredIdx !== null && visible[hoveredIdx] && (
          <div className="flex items-center gap-3 text-xs">
            <span className="text-eve-dim">{visible[hoveredIdx].date}</span>
            <span className="text-eve-text font-bold">{formatValue(visible[hoveredIdx].value)}</span>
          </div>
        )}
      </div>

      {/* Chart area */}
      <div className="relative">
        {/* Y-axis grid lines */}
        <div className="absolute inset-0 flex flex-col justify-between pointer-events-none">
          {[1, 0.75, 0.5, 0.25, 0].map(pct => (
            <div key={pct} className="border-t border-eve-border/20 w-full" />
          ))}
        </div>

        {/* Bars */}
        <div
          className="flex items-end gap-px h-28 relative"
          onMouseLeave={() => setHoveredIdx(null)}
        >
          {visible.map((d, i) => {
            const pct = (d.value / maxVal) * 100;
            const isHovered = hoveredIdx === i;
            return (
              <div
                key={d.date}
                className="flex-1 min-w-[3px] relative cursor-crosshair"
                style={{ height: "100%" }}
                onMouseEnter={() => setHoveredIdx(i)}
              >
                {/* Hover column highlight */}
                {isHovered && (
                  <div className="absolute inset-0 bg-eve-accent/5 border-x border-eve-accent/20" />
                )}
                {/* Bar */}
                <div
                  className="absolute bottom-0 left-0 right-0 rounded-t-sm transition-all duration-75"
                  style={{
                    height: `${Math.max(pct, 1)}%`,
                    backgroundColor: isHovered ? palette.hover : palette.normal,
                    boxShadow: isHovered ? "0 0 6px rgba(var(--eve-accent-rgb, 200,170,110), 0.15)" : "none",
                  }}
                />
              </div>
            );
          })}
        </div>
      </div>

      {/* X-axis labels */}
      <div className="flex justify-between mt-1.5 text-[9px] text-eve-dim/50">
        <span>{visible[0]?.date.slice(5)}</span>
        {visible.length > 20 && <span>{visible[Math.floor(visible.length / 2)]?.date.slice(5)}</span>}
        <span>{visible[visible.length - 1]?.date.slice(5)}</span>
      </div>
    </div>
  );
}

// ============================================================
// Shared: DateRangeSelector + CsvExportButton
// ============================================================

export function DateRangeSelector({ value, onChange, t }: { value: number; onChange: (v: number) => void; t: (key: TranslationKey) => string }) {
  const options = [
    { label: t("corpPeriod7d"), days: 7 },
    { label: t("corpPeriod30d"), days: 30 },
    { label: t("corpPeriod90d"), days: 90 },
    { label: t("corpPeriodAll"), days: 0 },
  ];
  return (
    <div className="flex rounded-sm overflow-hidden border border-eve-border text-xs">
      {options.map(o => (
        <button key={o.days} onClick={() => onChange(o.days)} className={`px-3 py-1 ${value === o.days ? "bg-eve-accent/20 text-eve-accent" : "text-eve-dim hover:text-eve-text"}`}>
          {o.label}
        </button>
      ))}
    </div>
  );
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function CsvExportButton({ data, filename, headers, t }: { data: any[]; filename: string; headers: string[]; t: (key: TranslationKey) => string }) {
  const handleExport = useCallback(() => {
    if (!data.length) return;
    const csvHeaders = headers.join(",");
    const csvRows = data.map((row: Record<string, unknown>) => headers.map(h => {
      const val = row[h];
      const str = String(val ?? "");
      return str.includes(",") || str.includes('"') ? `"${str.replace(/"/g, '""')}"` : str;
    }).join(","));
    const csv = [csvHeaders, ...csvRows].join("\n");
    const blob = new Blob([csv], { type: "text/csv;charset=utf-8;" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${filename}_${new Date().toISOString().slice(0, 10)}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  }, [data, filename, headers]);

  return (
    <button onClick={handleExport} className="px-3 py-1 text-xs text-eve-dim border border-eve-border rounded-sm hover:text-eve-accent hover:border-eve-accent/50 transition-colors ml-auto">
      {t("corpExportCsv")}
    </button>
  );
}

// ============================================================
// Shared Components
// ============================================================

export function KpiCard({
  label,
  value,
  color = "text-eve-text",
  large = false,
}: {
  label: string;
  value: string;
  color?: string;
  large?: boolean;
}) {
  return (
    <div className="bg-eve-panel border border-eve-border rounded-sm p-4">
      <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-1">{label}</div>
      <div className={`${large ? "text-xl" : "text-lg"} font-bold ${color}`}>{value}</div>
    </div>
  );
}

export function MiniKpi({
  label,
  value,
  color = "text-eve-text",
}: {
  label: string;
  value: number;
  color?: string;
}) {
  return (
    <div className="text-center">
      <div className={`text-lg font-bold ${color}`}>{value}</div>
      <div className="text-[10px] text-eve-dim">{label}</div>
    </div>
  );
}

export function IncomeSourceChart({
  sources,
  formatIsk,
}: {
  sources: IncomeSource[];
  formatIsk: (v: number) => string;
}) {
  if (!sources || sources.length === 0) {
    return <div className="text-eve-dim text-xs text-center py-4">No data</div>;
  }

  const colors = [
    "bg-emerald-500", "bg-blue-500", "bg-amber-500", "bg-purple-500",
    "bg-red-500", "bg-cyan-500", "bg-pink-500", "bg-teal-500",
  ];

  return (
    <div className="space-y-2">
      {/* Stacked bar */}
      <div className="flex h-6 rounded-sm overflow-hidden">
        {sources.map((s, i) => (
          <div
            key={s.category}
            className={`${colors[i % colors.length]} flex items-center justify-center text-[9px] font-bold text-black/70`}
            style={{ width: `${s.percent}%` }}
            title={`${s.label}: ${formatIsk(s.amount)} ISK (${s.percent.toFixed(1)}%)`}
          >
            {s.percent > 10 ? `${s.percent.toFixed(0)}%` : ""}
          </div>
        ))}
      </div>
      {/* Legend */}
      <div className="flex flex-wrap gap-x-4 gap-y-1">
        {sources.map((s, i) => (
          <div key={s.category} className="flex items-center gap-1.5 text-xs">
            <div className={`w-2.5 h-2.5 rounded-sm ${colors[i % colors.length]}`} />
            <span className="text-eve-dim">{s.label}</span>
            <span className="text-eve-text font-medium">{formatIsk(s.amount)}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

export function DailyPnLChart({
  entries,
  formatIsk,
}: {
  entries: DailyPnLEntry[];
  formatIsk: (v: number) => string;
}) {
  if (!entries || entries.length === 0) {
    return <div className="text-eve-dim text-xs text-center py-4">No data</div>;
  }

  const values = entries.map((e) => e.net_income);
  const maxAbs = Math.max(...values.map(Math.abs), 1);
  const chartH = 120;

  // Limit bars
  const maxBars = 45;
  const step = entries.length > maxBars ? Math.ceil(entries.length / maxBars) : 1;
  const sampled = step > 1 ? entries.filter((_, i) => i % step === 0) : entries;
  const barWidth = Math.max(3, Math.min(14, Math.floor(600 / sampled.length) - 1));

  return (
    <div className="relative">
      <div className="relative" style={{ height: chartH }}>
        <div className="flex items-end justify-center gap-px h-full">
          {sampled.map((entry) => {
            const val = entry.net_income;
            const pct = Math.abs(val) / maxAbs;
            const barH = Math.max(1, pct * (chartH / 2 - 4));
            const isPositive = val >= 0;

            return (
              <div
                key={entry.date}
                className="relative group flex flex-col items-center"
                style={{ width: barWidth, height: chartH }}
              >
                <div className="flex-1 flex items-end justify-center">
                  {isPositive && (
                    <div
                      className="rounded-t-[1px] bg-emerald-500/80 hover:bg-emerald-400 transition-colors"
                      style={{ width: barWidth, height: barH }}
                    />
                  )}
                </div>
                <div className="flex-1 flex items-start justify-center">
                  {!isPositive && (
                    <div
                      className="rounded-b-[1px] bg-red-500/80 hover:bg-red-400 transition-colors"
                      style={{ width: barWidth, height: barH }}
                    />
                  )}
                </div>
                <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-1 hidden group-hover:block z-10 pointer-events-none">
                  <div className="bg-eve-dark border border-eve-border rounded px-2 py-1 text-[10px] whitespace-nowrap shadow-lg">
                    <div className="text-eve-dim">{entry.date}</div>
                    <div className={isPositive ? "text-emerald-400" : "text-red-400"}>
                      {val >= 0 ? "+" : ""}{formatIsk(val)} ISK
                    </div>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
        <div className="absolute left-0 right-0 border-t border-eve-border/50" style={{ top: chartH / 2 }} />
      </div>
      <div className="flex justify-between mt-1 px-1">
        <span className="text-[9px] text-eve-dim">{sampled[0]?.date.slice(5)}</span>
        <span className="text-[9px] text-eve-dim">{sampled[sampled.length - 1]?.date.slice(5)}</span>
      </div>
    </div>
  );
}

export function TopContributorsTable({
  contributors,
  formatIsk,
}: {
  contributors: MemberContribution[];
  formatIsk: (v: number) => string;
}) {
  if (!contributors || contributors.length === 0) {
    return <div className="text-eve-dim text-xs text-center py-4">No data</div>;
  }

  const maxIsk = Math.max(...contributors.map((c) => Math.abs(c.total_isk)), 1);

  return (
    <div className="border border-eve-border rounded-sm overflow-hidden">
      <table className="w-full text-xs">
        <thead className="bg-eve-panel">
          <tr className="text-eve-dim">
            <th className="px-3 py-2 text-left">#</th>
            <th className="px-3 py-2 text-left">Character</th>
            <th className="px-3 py-2 text-left">Role</th>
            <th className="px-3 py-2 text-right">ISK Contributed</th>
          </tr>
        </thead>
        <tbody>
          {contributors.slice(0, 15).map((c, i) => {
            const pct = (Math.abs(c.total_isk) / maxIsk) * 100;
            return (
              <tr key={c.character_id} className="border-t border-eve-border/50 hover:bg-eve-panel/50">
                <td className="px-3 py-2 text-eve-dim">{i + 1}</td>
                <td className="px-3 py-2">
                  <div className="flex items-center gap-2">
                    <img
                      src={`https://images.evetech.net/characters/${c.character_id}/portrait?size=32`}
                      alt=""
                      className="w-5 h-5 rounded-sm"
                    />
                    <span className="text-eve-text font-medium">{c.name}</span>
                    {c.is_online && <span className="w-1.5 h-1.5 rounded-full bg-emerald-400" title="Online" />}
                  </div>
                </td>
                <td className="px-3 py-2 text-eve-dim capitalize">{c.category}</td>
                <td className="px-3 py-2 text-right">
                  <div className="flex items-center justify-end gap-2">
                    <div className="w-20 h-1.5 bg-eve-dark rounded-full overflow-hidden">
                      <div className="h-full bg-eve-accent/60 rounded-full" style={{ width: `${pct}%` }} />
                    </div>
                    <span className="text-eve-accent font-medium">{formatIsk(c.total_isk)}</span>
                  </div>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
