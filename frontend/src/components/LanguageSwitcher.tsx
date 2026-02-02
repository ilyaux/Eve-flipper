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
      className="flex items-center gap-1.5 px-2 py-1 rounded-sm text-xs
                 bg-eve-input border border-eve-border hover:border-eve-accent/50
                 transition-colors cursor-pointer select-none"
      title={locale === "ru" ? "Switch to English" : "Переключить на русский"}
    >
      <span className="text-base leading-none">{flags[locale]}</span>
      <span className="text-eve-dim uppercase font-medium">{locale}</span>
    </button>
  );
}
