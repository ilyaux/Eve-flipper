export function StatCard({
  label,
  value,
  subvalue,
  color = "text-eve-text",
  large = false,
}: {
  label: string;
  value: string;
  subvalue?: string;
  color?: string;
  large?: boolean;
}) {
  return (
    <div className="bg-eve-panel border border-eve-border rounded-sm p-3">
      <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-1">{label}</div>
      <div className={`${large ? "text-xl" : "text-lg"} font-bold ${color}`}>{value}</div>
      {subvalue && <div className="text-xs text-eve-dim">{subvalue}</div>}
    </div>
  );
}

export function FilterBtn({
  active,
  onClick,
  label,
  count,
  color = "text-eve-text",
}: {
  active: boolean;
  onClick: () => void;
  label: string;
  count: number;
  color?: string;
}) {
  return (
    <button
      onClick={onClick}
      className={`px-3 py-1 text-xs rounded-sm border transition-colors ${
        active
          ? "bg-eve-accent/20 border-eve-accent text-eve-accent"
          : "bg-eve-panel border-eve-border text-eve-dim hover:text-eve-text hover:border-eve-accent/50"
      }`}
    >
      <span className={active ? "" : color}>{label}</span>
      <span className="ml-1 opacity-60">({count})</span>
    </button>
  );
}