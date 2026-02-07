import { useCallback, useEffect, useRef, useState } from "react";
import { getAuthStatus, logout as apiLogout, getLoginUrl } from "./api";
import type { AuthStatus } from "./types";

interface UseAuthReturn {
  authStatus: AuthStatus;
  loginPolling: boolean;
  handleLogin: () => Promise<void>;
  handleLogout: () => Promise<void>;
}

/**
 * Manages EVE SSO authentication state, login polling (Tauri desktop),
 * and logout.
 *
 * Call once at the top level of App — the hook fetches initial auth status
 * on mount and cleans up polling timers on unmount.
 */
export function useAuth(): UseAuthReturn {
  const [authStatus, setAuthStatus] = useState<AuthStatus>({ logged_in: false });
  const [loginPolling, setLoginPolling] = useState(false);

  const loginPollRef = useRef<ReturnType<typeof setInterval>>(undefined);
  const loginTimeoutRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  // Fetch initial auth status on mount
  useEffect(() => {
    getAuthStatus().then(setAuthStatus).catch(() => {});
  }, []);

  // Cleanup login polling on unmount
  useEffect(() => {
    return () => {
      clearInterval(loginPollRef.current);
      clearTimeout(loginTimeoutRef.current);
    };
  }, []);

  const handleLogout = useCallback(async () => {
    await apiLogout();
    setAuthStatus({ logged_in: false });
  }, []);

  // Open EVE SSO login in system browser (Tauri) or same window (web)
  const handleLogin = useCallback(async () => {
    const baseUrl = getLoginUrl();
    // Detect Tauri runtime
    const isTauri = !!(window as unknown as { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__;
    if (isTauri) {
      // Pass ?desktop=1 so the backend knows to show a "close tab" page
      // instead of redirecting back to /
      const url = baseUrl + "?desktop=1";
      try {
        const { openUrl } = await import("@tauri-apps/plugin-opener");
        await openUrl(url);
      } catch {
        // Fallback if plugin fails
        window.open(url, "_blank");
      }
    } else {
      // In regular browser, navigate in same window.
      // Backend will redirect back to / after auth completes.
      window.location.href = baseUrl;
      return;
    }
    // Start polling for auth completion (Tauri only)
    // Clear any previous polling first
    clearInterval(loginPollRef.current);
    clearTimeout(loginTimeoutRef.current);

    setLoginPolling(true);
    loginPollRef.current = setInterval(async () => {
      try {
        const status = await getAuthStatus();
        if (status.logged_in) {
          clearInterval(loginPollRef.current);
          setAuthStatus(status);
          setLoginPolling(false);
        }
      } catch {
        // ignore, keep polling
      }
    }, 2000);
    // Stop polling after 5 minutes
    loginTimeoutRef.current = setTimeout(() => {
      clearInterval(loginPollRef.current);
      setLoginPolling(false);
    }, 5 * 60 * 1000);
  }, []);

  return { authStatus, loginPolling, handleLogin, handleLogout };
}
