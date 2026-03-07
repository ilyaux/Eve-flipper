import { useI18n, type Locale } from "@/lib/i18n";

function LanguageFlag({ locale }: { locale: Locale }) {
  if (locale === "ru") {
    return (
      <svg
        viewBox="0 0 60 36"
        className="w-5 h-3.5 rounded-[2px] border border-eve-border/70 shadow-inner shrink-0"
        aria-hidden="true"
      >
        <rect width="60" height="36" fill="#FFFFFF" />
        <rect y="12" width="60" height="12" fill="#0039A6" />
        <rect y="24" width="60" height="12" fill="#D52B1E" />
      </svg>
    );
  }

  return (
    <svg
      viewBox="0 0 64 48"
      className="w-5 h-3.5 rounded-[2px] border border-eve-border/70 shadow-inner shrink-0"
      aria-hidden="true"
    >
      <rect width="64" height="48" fill="#012169" />
      <path d="M0 0L64 48M64 0L0 48" stroke="#FFFFFF" strokeWidth="10" />
      <path d="M0 0L64 48M64 0L0 48" stroke="#C8102E" strokeWidth="6" />
      <rect x="0" y="19" width="64" height="10" fill="#FFFFFF" />
      <rect x="27" y="0" width="10" height="48" fill="#FFFFFF" />
      <rect x="0" y="21" width="64" height="6" fill="#C8102E" />
      <rect x="29" y="0" width="6" height="48" fill="#C8102E" />
    </svg>
  );
}

export function LanguageSwitcher() {
  const { locale, setLocale } = useI18n();
  const next: Locale = locale === "ru" ? "en" : "ru";

  return (
    <button
      onClick={() => setLocale(next)}
      className="flex items-center justify-center gap-1.5 h-[34px] px-3 rounded-sm text-xs
                 bg-eve-panel border border-eve-border hover:border-eve-accent/50
                 transition-colors cursor-pointer select-none"
      title={locale === "ru" ? "Switch to English" : "Switch to Russian"}
    >
      <LanguageFlag locale={locale} />
      <span className="text-eve-dim uppercase font-medium">{locale}</span>
    </button>
  );
}
