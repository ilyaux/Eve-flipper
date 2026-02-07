import { useEffect, useState } from "react";
import { getStatus } from "./api";

interface UseEsiStatusReturn {
  /** `true` = ESI reachable, `false` = down, `null` = still loading initial check */
  esiAvailable: boolean | null;
}

/**
 * Polls the backend `/api/status` endpoint every 5 seconds to track
 * whether the EVE ESI is reachable.  Returns `null` while the first
 * check is in-flight.
 */
export function useEsiStatus(): UseEsiStatusReturn {
  const [esiAvailable, setEsiAvailable] = useState<boolean | null>(null);

  useEffect(() => {
    let mounted = true;
    const checkEsi = async () => {
      try {
        const status = await getStatus();
        if (mounted) setEsiAvailable(status.esi_ok);
      } catch {
        if (mounted) setEsiAvailable(false);
      }
    };
    checkEsi();
    const interval = setInterval(checkEsi, 5000);
    return () => {
      mounted = false;
      clearInterval(interval);
    };
  }, []);

  return { esiAvailable };
}
