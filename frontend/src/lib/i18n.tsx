import { createContext, useContext, useState, useCallback, type ReactNode } from "react";
import { ru } from "./locale/ru";
import { en } from "./locale/en";

export type Locale = "ru" | "en";

const translations = { ru, en };

export type TranslationKey = keyof typeof ru;

interface I18nContextType {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}

const I18nContext = createContext<I18nContextType>(null!);

function getDefaultLocale(): Locale {
  // Check localStorage first
  try {
    const saved = localStorage.getItem("eve-flipper-locale");
    if (saved === "en" || saved === "ru") return saved;
  } catch {
    // localStorage may be unavailable (private mode, etc.)
  }
  // Fall back to browser language
  const browserLang = navigator.language?.toLowerCase() ?? "";
  if (browserLang.startsWith("ru")) return "ru";
  // Default to English for all other languages
  return "en";
}

export function I18nProvider({ children }: { children: ReactNode }) {
  const [locale, setLocale] = useState<Locale>(getDefaultLocale);

  const changeLocale = useCallback((l: Locale) => {
    setLocale(l);
    localStorage.setItem("eve-flipper-locale", l);
  }, []);

  const t = useCallback(
    (key: TranslationKey, params?: Record<string, string | number>) => {
      let str: string = translations[locale][key] ?? key;
      if (params) {
        for (const [k, v] of Object.entries(params)) {
          str = str.replace(`{${k}}`, String(v));
        }
      }
      return str;
    },
    [locale]
  );

  return (
    <I18nContext.Provider value={{ locale, setLocale: changeLocale, t }}>
      {children}
    </I18nContext.Provider>
  );
}

export function useI18n() {
  return useContext(I18nContext);
}
