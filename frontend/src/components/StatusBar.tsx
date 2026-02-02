import { useEffect, useState } from "react";
import { getStatus, getScanHistory } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import type { AppStatus, ScanRecord } from "@/lib/types";

export function StatusBar() {
  const { t } = useI18n();
  const [status, setStatus] = useState<AppStatus | null>(null);
  const [history, setHistory] = useState<ScanRecord[]>([]);
  const [showHistory, setShowHistory] = useState(false);

  useEffect(() => {
    const poll = () => {
      getStatus().then(setStatus).catch(() => {});
      getScanHistory().then(setHistory).catch(() => {});
    };
    poll();
    const id = setInterval(poll, 5000);
    return () => clearInterval(id);
  }, []);

  return (
    <div className="flex items-center gap-4 px-4 py-2 bg-eve-panel border border-eve-border rounded-sm">
      <StatusDot
        ok={status?.sde_loaded ?? false}
        loading={status === null}
        label={
          status?.sde_loaded
            ? `SDE: ${status.sde_systems} ${t("sdeSystems")}, ${status.sde_types} ${t("sdeTypes")}`
            : t("sdeLoading")
        }
      />
      <div className="w-px h-4 bg-eve-border" />
      <StatusDot
        ok={status?.esi_ok ?? false}
        loading={status === null}
        label={status?.esi_ok ? t("esiApi") : t("esiUnavailable")}
      />
      {history.length > 0 && (
        <>
          <div className="w-px h-4 bg-eve-border" />
          <div className="relative">
            <button
              onClick={() => setShowHistory((v) => !v)}
              className="text-xs text-eve-dim hover:text-eve-accent transition-colors cursor-pointer"
              title={t("scanHistory")}
            >
              📋 {history.length}
            </button>
            {showHistory && (
              <>
                <div className="fixed inset-0 z-40" onClick={() => setShowHistory(false)} />
                <div className="absolute right-0 top-8 z-50 w-72 bg-eve-panel border border-eve-border rounded-sm shadow-2xl py-1 max-h-60 overflow-y-auto">
                  <div className="px-3 py-1.5 text-[10px] uppercase tracking-wider text-eve-dim font-medium border-b border-eve-border">
                    {t("scanHistory")}
                  </div>
                  {history.map((rec, i) => (
                    <div key={i} className="px-3 py-1.5 text-xs border-b border-eve-border/30 hover:bg-eve-accent/5">
                      <div className="flex justify-between">
                        <span className="text-eve-text">{rec.tab} — {rec.system}</span>
                        <span className="text-eve-dim font-mono">{rec.count}</span>
                      </div>
                      <div className="text-[10px] text-eve-dim mt-0.5">
                        {new Date(rec.timestamp).toLocaleTimeString()}
                      </div>
                    </div>
                  ))}
                </div>
              </>
            )}
          </div>
        </>
      )}
    </div>
  );
}

function StatusDot({ ok, loading, label }: { ok: boolean; loading: boolean; label: string }) {
  return (
    <div className="flex items-center gap-2 text-xs">
      <div
        className={`w-2 h-2 rounded-full ${
          loading
            ? "bg-eve-accent animate-pulse"
            : ok
              ? "bg-eve-success"
              : "bg-eve-error"
        }`}
      />
      <span className={ok ? "text-eve-text" : "text-eve-dim"}>{label}</span>
    </div>
  );
}
