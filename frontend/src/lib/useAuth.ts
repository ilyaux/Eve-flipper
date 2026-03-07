import { useCallback, useEffect, useRef, useState } from "react";
import { deleteAuthCharacter, getAuthStatus, getDesktopLoginUrl, getLoginUrl, logout as apiLogout, selectAuthCharacter } from "./api";
import type { AuthStatus } from "./types";

interface UseAuthReturn {
  authStatus: AuthStatus;
  loginPolling: boolean;
  handleLogin: () => Promise<void>;
  handleLogout: () => Promise<void>;
  handleSelectCharacter: (characterId: number) => Promise<void>;
  handleDeleteCharacter: (characterId: number) => Promise<void>;
  refreshAuthStatus: () => Promise<void>;
}

function normalizeAuthStatus(status: AuthStatus): AuthStatus {
  if (!status.logged_in) return { logged_in: false, characters: [] };

  const characters = [...(status.characters ?? [])];
  if (characters.length === 0 && status.character_id && status.character_name) {
    characters.push({
      character_id: status.character_id,
      character_name: status.character_name,
      active: true,
    });
  }

  return {
    ...status,
    characters,
  };
}

function authFingerprint(status: AuthStatus): string {
  const normalized = normalizeAuthStatus(status);
  const ids = (normalized.characters ?? []).map((c) => c.character_id).sort((a, b) => a - b);
  return JSON.stringify({
    logged_in: normalized.logged_in,
    character_id: normalized.character_id ?? null,
    auth_revision: normalized.auth_revision ?? 0,
    ids,
  });
}

/**
 * Manages EVE SSO authentication state, login polling (Tauri desktop),
 * and logout.
 *
 * Call once at the top level of App — the hook fetches initial auth status
 * on mount and cleans up polling timers on unmount.
 */
export function useAuth(): UseAuthReturn {
  const [authStatus, setAuthStatus] = useState<AuthStatus>({ logged_in: false, characters: [] });
  const [loginPolling, setLoginPolling] = useState(false);

  const loginPollRef = useRef<ReturnType<typeof setInterval>>(undefined);
  const loginTimeoutRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  // Fetch initial auth status on mount
  useEffect(() => {
    getAuthStatus().then((s) => setAuthStatus(normalizeAuthStatus(s))).catch(() => {});
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
    setAuthStatus({ logged_in: false, characters: [] });
  }, []);

  const handleSelectCharacter = useCallback(async (characterId: number) => {
    const status = await selectAuthCharacter(characterId);
    setAuthStatus(normalizeAuthStatus(status));
  }, []);

  const handleDeleteCharacter = useCallback(async (characterId: number) => {
    const status = await deleteAuthCharacter(characterId);
    setAuthStatus(normalizeAuthStatus(status));
  }, []);

  const refreshAuthStatus = useCallback(async () => {
    const status = await getAuthStatus();
    setAuthStatus(normalizeAuthStatus(status));
  }, []);

  // Open EVE SSO login in system browser (Tauri) or same window (web)
  const handleLogin = useCallback(async () => {
    const baseline = normalizeAuthStatus(authStatus);
    const baselineFingerprint = authFingerprint(baseline);
    const wasLoggedIn = baseline.logged_in;
    const baseUrl = getLoginUrl();
    const runtime = window as unknown as {
      __TAURI_INTERNALS__?: unknown;
      runtime?: { BrowserOpenURL?: (url: string) => void };
    };
    // Detect desktop runtimes
    const isTauri = !!runtime.__TAURI_INTERNALS__;
    const isWails = typeof runtime.runtime?.BrowserOpenURL === "function";
    if (isTauri || isWails) {
      // Request auth URL from the in-app webview first so state is bound to
      // the same user scope as polling /api/auth/status.
      let url = "";
      try {
        url = await getDesktopLoginUrl();
        if (isTauri) {
          const { openUrl } = await import("@tauri-apps/plugin-opener");
          await openUrl(url);
        } else {
          runtime.runtime?.BrowserOpenURL?.(url);
        }
      } catch {
        if (!url) {
          // Legacy fallback path (may not sync user scope in desktop mode).
          url = `${baseUrl}?desktop=1`;
        }
        // Fallback if opener bridge fails
        window.open(url, "_blank");
      }
    } else {
      // In regular browser, navigate in same window.
      // Backend will redirect back to / after auth completes.
      window.location.href = baseUrl;
      return;
    }
    // Start polling for auth completion (desktop only)
    // Clear any previous polling first
    clearInterval(loginPollRef.current);
    clearTimeout(loginTimeoutRef.current);

    setLoginPolling(true);
    loginPollRef.current = setInterval(async () => {
      try {
        const status = normalizeAuthStatus(await getAuthStatus());
        const changed = authFingerprint(status) !== baselineFingerprint;
        if (status.logged_in && (!wasLoggedIn || changed)) {
          clearInterval(loginPollRef.current);
          setAuthStatus(normalizeAuthStatus(status));
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
  }, [authStatus]);

  return {
    authStatus,
    loginPolling,
    handleLogin,
    handleLogout,
    handleSelectCharacter,
    handleDeleteCharacter,
    refreshAuthStatus,
  };
}
