import type { TranslationKey } from "./i18n";

/**
 * Handles EVE UI action errors and returns appropriate translation key and duration
 */
export function handleEveUIError(err: any): { messageKey: TranslationKey; duration: number } {
  const errorMsg = err?.message || String(err);

  // Check for 401 Unauthorized - missing scope or expired token
  if (errorMsg.includes("401") || errorMsg.includes("unauthorized") || errorMsg.includes("Unauthorized")) {
    return { messageKey: "reloginRequired", duration: 5000 };
  }

  // Generic error
  return { messageKey: "actionFailed", duration: 3000 };
}
