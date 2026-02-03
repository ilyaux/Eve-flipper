import { useI18n, type Locale } from "@/lib/i18n";

const flags: Record<Locale, string> = {
  ru: "🇷🇺",
  en: "🇬🇧",
};

export function LanguageSwitcher() {
  const { locale, setLocale } = useI18n();
  const next: Locale = locale === "ru" ? "en" : "ru";

  return (
    <button
      onClick={() => setLocale(next)}
      className="flex items-center justify-center gap-1.5 h-[34px] px-3 rounded-sm text-xs
                 bg-eve-panel border border-eve-border hover:border-eve-accent/50
                 transition-colors cursor-pointer select-none"
      title={locale === "ru" ? "Switch to English" : "Переключить на русский"}
    >
      <span className="text-base leading-none">{flags[locale]}</span>
      <span className="text-eve-dim uppercase font-medium">{locale}</span>
    </button>
  );
}
