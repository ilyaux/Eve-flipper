import { useEffect, useState } from "react";

interface UseVersionCheckReturn {
  appVersion: string;
  latestVersion: string | null;
  hasUpdate: boolean;
}

/** Compare two semver-like version strings. Returns true when `latest` is newer than `current`. */
function isVersionNewer(latest: string, current: string): boolean {
  const la = latest.split(".").map((n) => parseInt(n, 10) || 0);
  const ca = current.split(".").map((n) => parseInt(n, 10) || 0);
  const len = Math.max(la.length, ca.length);
  for (let i = 0; i < len; i++) {
    const lv = la[i] ?? 0;
    const cv = ca[i] ?? 0;
    if (lv > cv) return true;
    if (lv < cv) return false;
  }
  return false;
}

/**
 * Checks the latest GitHub release on mount (skipped for dev builds)
 * and exposes the current app version, latest remote version, and
 * whether an update is available.
 */
export function useVersionCheck(): UseVersionCheckReturn {
  const appVersion: string = import.meta.env.VITE_APP_VERSION || "dev";
  const [latestVersion, setLatestVersion] = useState<string | null>(null);
  const [hasUpdate, setHasUpdate] = useState(false);

  useEffect(() => {
    if (!appVersion || appVersion === "dev") return;

    const controller = new AbortController();
    const fetchLatest = async () => {
      try {
        const res = await fetch("https://api.github.com/repos/ilyaux/Eve-flipper/releases/latest", {
          signal: controller.signal,
        });
        if (!res.ok) return;
        const data = await res.json() as { tag_name?: string };
        if (!data.tag_name) return;
        const latest = String(data.tag_name).replace(/^v/i, "");
        const current = String(appVersion).replace(/^v/i, "");
        setLatestVersion(latest);
        if (isVersionNewer(latest, current)) {
          setHasUpdate(true);
        }
      } catch {
        // ignore network / API errors
      }
    };

    fetchLatest();
    return () => controller.abort();
  }, [appVersion]);

  return { appVersion, latestVersion, hasUpdate };
}
