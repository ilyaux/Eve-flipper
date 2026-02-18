import type { TranslationKey } from "../../lib/i18n";

export type CorpTab = "overview" | "wallets" | "members" | "industry" | "mining" | "market";
export type CorpMode = "demo" | "live";
export type CorpTranslationFn = (key: TranslationKey, params?: Record<string, string | number>) => string;
export type FormatIskFn = (v: number) => string;