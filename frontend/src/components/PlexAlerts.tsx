import { useCallback, useEffect, useRef, useState } from "react";
import { formatISK } from "../lib/format";
import { useI18n } from "../lib/i18n";
import type { PLEXDashboard } from "../lib/types";

/** Alert thresholds stored in localStorage */
interface PlexAlertConfig {
  enabled: boolean;
  belowPrice: number; // alert when PLEX sell price drops below this
  abovePrice: number; // alert when PLEX sell price rises above this
  onCCPSale: boolean;
  onSignalChange: boolean;
}

const STORAGE_KEY = "plex_alerts";
const DEFAULT_CONFIG: PlexAlertConfig = {
  enabled: false,
  belowPrice: 0,
  abovePrice: 0,
  onCCPSale: true,
  onSignalChange: true,
};

function loadConfig(): PlexAlertConfig {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw) return { ...DEFAULT_CONFIG, ...JSON.parse(raw) };
  } catch { /* ignore */ }
  return { ...DEFAULT_CONFIG };
}

function saveConfig(cfg: PlexAlertConfig) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(cfg));
}

/** Hook: check dashboard data against alert thresholds and fire Browser Notifications */
export function usePlexAlerts(dashboard: PLEXDashboard | null) {
  const lastSignalRef = useRef<string>("");
  const lastCCPSaleRef = useRef(false);
  const lastPriceAlertRef = useRef<string>(""); // dedup key
  const configRef = useRef(loadConfig());

  const checkAlerts = useCallback(() => {
    if (!dashboard) return;
    const cfg = configRef.current;
    if (!cfg.enabled) return;
    if (typeof Notification === "undefined" || Notification.permission !== "granted") return;

    const sellPrice = dashboard.plex_price.sell_price;
    const signal = dashboard.signal.action;
    const ccpSale = dashboard.indicators?.ccp_sale_signal ?? false;

    // Price below threshold
    if (cfg.belowPrice > 0 && sellPrice > 0 && sellPrice < cfg.belowPrice) {
      const key = `below_${Math.floor(sellPrice / 100000)}`;
      if (lastPriceAlertRef.current !== key) {
        lastPriceAlertRef.current = key;
        new Notification("EVE Flipper — PLEX Alert", {
          body: `PLEX dropped below ${formatISK(cfg.belowPrice)}! Current: ${formatISK(sellPrice)}`,
          icon: "/favicon.ico",
        });
      }
    }

    // Price above threshold
    if (cfg.abovePrice > 0 && sellPrice > 0 && sellPrice > cfg.abovePrice) {
      const key = `above_${Math.floor(sellPrice / 100000)}`;
      if (lastPriceAlertRef.current !== key) {
        lastPriceAlertRef.current = key;
        new Notification("EVE Flipper — PLEX Alert", {
          body: `PLEX rose above ${formatISK(cfg.abovePrice)}! Current: ${formatISK(sellPrice)}`,
          icon: "/favicon.ico",
        });
      }
    }

    // CCP Sale detected
    if (cfg.onCCPSale && ccpSale && !lastCCPSaleRef.current) {
      new Notification("EVE Flipper — CCP Sale Detected", {
        body: "Abnormal volume + price drop detected — possible CCP PLEX sale!",
        icon: "/favicon.ico",
      });
    }
    lastCCPSaleRef.current = ccpSale;

    // Signal change
    if (cfg.onSignalChange && signal && lastSignalRef.current && signal !== lastSignalRef.current) {
      new Notification("EVE Flipper — Signal Change", {
        body: `PLEX signal changed: ${lastSignalRef.current} → ${signal}`,
        icon: "/favicon.ico",
      });
    }
    if (signal) lastSignalRef.current = signal;
  }, [dashboard]);

  // Run check whenever dashboard updates
  useEffect(() => {
    checkAlerts();
  }, [checkAlerts]);

  return configRef;
}

/** Alert configuration panel (toggle in PlexTab top bar) */
export function PlexAlertPanel({ onClose }: { onClose: () => void }) {
  const { t } = useI18n();
  const [cfg, setCfg] = useState(loadConfig);
  const [permStatus, setPermStatus] = useState<NotificationPermission>(
    typeof Notification !== "undefined" ? Notification.permission : "denied"
  );

  const updateCfg = (patch: Partial<PlexAlertConfig>) => {
    setCfg(prev => {
      const next = { ...prev, ...patch };
      saveConfig(next);
      return next;
    });
  };

  const handleEnable = async (checked: boolean) => {
    if (checked && typeof Notification !== "undefined" && Notification.permission === "default") {
      const perm = await Notification.requestPermission();
      setPermStatus(perm);
      if (perm !== "granted") {
        updateCfg({ enabled: false });
        return;
      }
    }
    updateCfg({ enabled: checked });
  };

  return (
    <div className="absolute top-full right-0 mt-1 z-40 w-72 bg-eve-dark border border-eve-border rounded-sm shadow-xl p-3">
      <div className="flex items-center justify-between mb-2">
        <h4 className="text-[10px] font-semibold text-eve-dim uppercase tracking-wider">{t("plexAlerts")}</h4>
        <button onClick={onClose} className="text-eve-dim hover:text-eve-text text-sm">&times;</button>
      </div>

      {/* Enable toggle */}
      <label className="flex items-center gap-2 mb-2 cursor-pointer">
        <input
          type="checkbox"
          checked={cfg.enabled}
          onChange={e => handleEnable(e.target.checked)}
          className="accent-eve-accent"
        />
        <span className="text-xs text-eve-text">{t("plexAlertEnabled")}</span>
      </label>

      {permStatus === "denied" && (
        <div className="text-[10px] text-eve-error mb-2">{t("plexAlertBlocked")}</div>
      )}

      {/* Price thresholds */}
      <div className="space-y-1.5 text-xs">
        <div className="flex items-center gap-2">
          <label className="text-eve-dim w-24">{t("plexAlertBelow")}</label>
          <input
            type="number"
            min="0"
            step="100000"
            value={cfg.belowPrice || ""}
            onChange={e => updateCfg({ belowPrice: parseFloat(e.target.value) || 0 })}
            placeholder="ISK"
            className="flex-1 px-1.5 py-0.5 bg-eve-input border border-eve-border rounded-sm text-xs text-eve-text font-mono"
          />
        </div>
        <div className="flex items-center gap-2">
          <label className="text-eve-dim w-24">{t("plexAlertAbove")}</label>
          <input
            type="number"
            min="0"
            step="100000"
            value={cfg.abovePrice || ""}
            onChange={e => updateCfg({ abovePrice: parseFloat(e.target.value) || 0 })}
            placeholder="ISK"
            className="flex-1 px-1.5 py-0.5 bg-eve-input border border-eve-border rounded-sm text-xs text-eve-text font-mono"
          />
        </div>

        {/* CCP Sale alert */}
        <label className="flex items-center gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={cfg.onCCPSale}
            onChange={e => updateCfg({ onCCPSale: e.target.checked })}
            className="accent-eve-accent"
          />
          <span className="text-eve-text">{t("plexAlertCCPSale")}</span>
        </label>

        {/* Signal change alert */}
        <label className="flex items-center gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={cfg.onSignalChange}
            onChange={e => updateCfg({ onSignalChange: e.target.checked })}
            className="accent-eve-accent"
          />
          <span className="text-eve-text">{t("plexAlertSignal")}</span>
        </label>
      </div>
    </div>
  );
}
