import { useState } from "react";
import type { SystemDanger } from "@/lib/types";
import { getGankCheck } from "@/lib/api";
import { RouteSafetyModal } from "./RouteSafetyModal";
import { useAchievements } from "./achievements";

interface Props {
  from: number;
  to: number;
  minSec?: number;
}

export function RouteSafetyBadge({ from, to, minSec = 0 }: Props) {
  const [systems, setSystems] = useState<SystemDanger[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const { trackAchievementEvent } = useAchievements();

  if (!from || !to || from === to) return null;

  const handleClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (systems) {
      setModalOpen(true);
      void trackAchievementEvent("route_checked", { gankRiskViewed: true });
      return;
    }
    setLoading(true);
    getGankCheck(from, to, minSec).then((data) => {
      setSystems(data);
      setLoading(false);
      setModalOpen(true);
      void trackAchievementEvent("route_checked", { gankRiskViewed: true });
    }).catch(() => {
      setLoading(false);
    });
  };

  return (
    <>
      <button
        onClick={handleClick}
        title="Check route safety"
        className="inline-flex items-center gap-0.5 text-[10px] text-eve-dim hover:text-eve-accent transition-colors bg-transparent border-0 cursor-pointer px-1 py-0.5 rounded leading-none whitespace-nowrap"
      >
        {loading ? (
          <span className="opacity-50">…</span>
        ) : (
          <span title="Route safety">🛡</span>
        )}
      </button>
      {modalOpen && systems && (
        <RouteSafetyModal
          systems={systems}
          onClose={() => setModalOpen(false)}
        />
      )}
    </>
  );
}
