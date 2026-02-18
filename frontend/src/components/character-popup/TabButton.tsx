
export function TabBtn({ active, onClick, label }: { active: boolean; onClick: () => void; label: string }) {
  return (
    <button
      onClick={onClick}
      className={`px-4 py-2 text-xs font-medium transition-colors whitespace-nowrap ${
        active
          ? "text-eve-accent border-b-2 border-eve-accent bg-eve-dark/50"
          : "text-eve-dim hover:text-eve-text"
      }`}
    >
      {label}
    </button>
  );
}
